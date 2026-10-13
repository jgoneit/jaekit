#!/usr/bin/env python3
"""Prepare a local Windows x64 candidate; never publish or install it."""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import re
import stat
import struct
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SCHEMA = "jaekit-windows-package/v1"
VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*)?\Z")
RESERVED = re.compile(r"(?:CON|PRN|AUX|NUL|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])(?:\..*)?\Z", re.I)
MAX_FILES = 2048
MAX_FILE = 32 * 1024 * 1024
MAX_TOTAL = 128 * 1024 * 1024


def safe_name(name: str) -> None:
    parts = name.split("/")
    if any(not part or part in (".", "..") or part[-1:] in (" ", ".")
           or RESERVED.fullmatch(part) or any(ord(c) < 32 or c in '\\:<>"|?*' for c in part)
           for part in parts):
        raise ValueError(f"unsafe Windows package path: {name!r}")


def read_regular(path: Path) -> bytes:
    # Walk every source component before reading: a file behind a directory
    # symlink is not part of the reviewed public tree either.
    for item in (path, *path.parents):
        if item.is_symlink():
            raise ValueError(f"symlink source is not allowed: {item}")
    if not stat.S_ISREG(path.stat().st_mode):
        raise ValueError(f"not a regular source file: {path}")
    if path.stat().st_size > MAX_FILE:
        raise ValueError(f"source file exceeds package limit: {path}")
    return path.read_bytes()


def windows_amd64(data: bytes) -> None:
    if len(data) < 64 or data[:2] != b"MZ":
        raise ValueError("binary is not a Windows PE executable")
    offset = struct.unpack_from("<I", data, 0x3C)[0]
    if offset + 26 > len(data) or data[offset:offset + 4] != b"PE\0\0":
        raise ValueError("binary has no valid PE header")
    machine = struct.unpack_from("<H", data, offset + 4)[0]
    optional = struct.unpack_from("<H", data, offset + 24)[0]
    if machine != 0x8664 or optional != 0x20B:
        raise ValueError("binary is not a Windows amd64 PE32+ executable")


def collect(binary: Path, root: Path = ROOT) -> dict[str, bytes]:
    files = {"ha.exe": read_regular(binary)}
    windows_amd64(files["ha.exe"])
    fixed = ["LICENSE", "README.md", "README.en.md", "tools/check-result-reference.py"]
    fixed += [f"examples/check-result/{name}" for name in
              ("README.md", "declaration.json", "reference.json", "input.txt")]
    fixed += [f"tools/windows/{name}" for name in
              ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1", "README.md")]
    for directory in ("guides", "contracts", "assets"):
        for path in sorted((root / directory).rglob("*")):
            if path.is_symlink():
                raise ValueError(f"symlink source is not allowed: {path}")
            if path.is_file():
                fixed.append(path.relative_to(root).as_posix())
    for name in fixed:
        safe_name(name)
        files[name] = read_regular(root / name)
    keys = [name.casefold() for name in files]
    if len(set(keys)) != len(keys):
        raise ValueError("case-insensitive package path collision")
    if len(files) > MAX_FILES - 1 or sum(map(len, files.values())) > MAX_TOTAL:
        raise ValueError("package exceeds size or file-count limit")
    return files


def prepare(binary: Path, version: str, output: Path, root: Path = ROOT) -> Path:
    if not VERSION.fullmatch(version):
        raise ValueError("version must be a three-part version with an optional suffix")
    files = collect(binary, root)
    manifest = {"schema": SCHEMA, "version": version, "os": "windows", "arch": "amd64",
                "files": [{"path": name, "size": len(data), "sha256": hashlib.sha256(data).hexdigest()}
                          for name, data in sorted(files.items())]}
    files["package.json"] = (json.dumps(manifest, ensure_ascii=True, sort_keys=True, indent=2) + "\n").encode()
    if sum(map(len, files.values())) > MAX_TOTAL:
        raise ValueError("package including manifest exceeds total size limit")
    name = f"ha_{version}_windows_amd64.zip"
    # A candidate is a new directory. Never replace an older candidate or mix
    # a new archive with someone else's checksums and installer scripts.
    if output.exists() or output.is_symlink():
        raise ValueError(f"candidate output already exists: {output}")
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix=".jaekit-candidate-", dir=output.parent) as tmp:
        stage = Path(tmp) / "candidate"
        stage.mkdir()
        archive = stage / name
        with zipfile.ZipFile(archive, "w", compression=zipfile.ZIP_DEFLATED) as zf:
            for entry, data in sorted(files.items()):
                info = zipfile.ZipInfo(entry, date_time=(1980, 1, 1, 0, 0, 0))
                info.create_system = 3
                info.external_attr = (stat.S_IFREG | 0o644) << 16
                info.compress_type = zipfile.ZIP_DEFLATED
                zf.writestr(info, data)
        if archive.stat().st_size > MAX_TOTAL:
            raise ValueError("compressed package exceeds size limit")
        for script in ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1"):
            (stage / script).write_bytes(files[f"tools/windows/{script}"])
        checksum_names = [name, "common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1"]
        (stage / "checksums.txt").write_text("".join(
            f"{hashlib.sha256((stage / item).read_bytes()).hexdigest()}  {item}\n"
            for item in checksum_names), encoding="ascii")
        stage.rename(output)
    return output / name


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, required=True, help="already-built Windows amd64 ha.exe")
    parser.add_argument("--version", required=True, help="must match the binary's ha --version output")
    parser.add_argument("--output", type=Path, required=True, help="new local candidate directory")
    args = parser.parse_args()
    try:
        archive = prepare(args.binary.absolute(), args.version, args.output.absolute())
    except (OSError, ValueError, zipfile.BadZipFile) as exc:
        parser.exit(1, f"candidate preparation failed: {exc}\n")
    print(f"Prepared candidate: {archive}")
    print("Not published. Windows installation and execution have not been verified by this command.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
