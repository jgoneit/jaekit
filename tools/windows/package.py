#!/usr/bin/env python3
"""Prepare a local Windows x64 candidate; never publish or install it."""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import struct
import subprocess
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SCHEMA = "jaekit-windows-package/v1"
VERSION = re.compile(r"[0-9]+\.[0-9]+\.[0-9]+(?:-[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)*)?\Z")
# Package policy is deliberately conservative across supported Windows hosts:
# console names with extensions are refused even where the OS treats them as files.
RESERVED = re.compile(r"(?:CON|CONIN\$|CONOUT\$|PRN|AUX|NUL|COM[1-9\u00b9\u00b2\u00b3]|LPT[1-9\u00b9\u00b2\u00b3])(?:\..*)?\Z", re.I)
MAX_FILES = 2048
MAX_FILE = 32 * 1024 * 1024
MAX_TOTAL = 128 * 1024 * 1024
PUBLIC_DIRECTORIES = ("guides", "contracts", "assets")
PUBLIC_FILES = ("LICENSE", "README.md", "README.en.md", "tools/check-result-reference.py",
                *(f"examples/check-result/{name}" for name in
                  ("README.md", "declaration.json", "reference.json", "input.txt")),
                *(f"tools/windows/{name}" for name in
                  ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1", "README.md")))


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


def git(root: Path, *args: str) -> bytes:
    # Read-only commands must not refresh the user's index as a side effect.
    result = subprocess.run(["git", "--no-replace-objects", "-C", str(root), *args],
                            env={**os.environ, "GIT_OPTIONAL_LOCKS": "0"},
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode:
        raise ValueError("cannot read committed package sources with git " + args[0])
    return result.stdout


def public_path(name: str) -> bool:
    return name in PUBLIC_FILES or any(name.startswith(directory + "/")
                                       for directory in PUBLIC_DIRECTORIES)


def public_index(root: Path) -> dict[str, tuple[str, str]]:
    entries = {}
    for entry in git(root, "ls-files", "--stage", "-z").split(b"\0"):
        if not entry:
            continue
        metadata, path = entry.split(b"\t", 1)
        name = os.fsdecode(path)
        if public_path(name):
            mode, oid, stage = metadata.decode("ascii").split()
            if stage != "0" or name in entries:
                raise ValueError(f"unmerged public package source: {name}")
            entries[name] = (mode, oid)
    return entries


def reject_public_links(root: Path) -> None:
    # Preserve the existing refusal of even untracked links, without collecting
    # or reading untracked/ignored regular files for the candidate.
    def raise_walk_error(error: OSError) -> None:
        raise error

    for directory in PUBLIC_DIRECTORIES:
        parent = root / directory
        if parent.is_symlink():
            raise ValueError(f"symlink source is not allowed: {parent}")
        for current, directories, names in os.walk(parent, onerror=raise_walk_error, followlinks=False):
            for name in directories + names:
                path = Path(current) / name
                if path.is_symlink():
                    raise ValueError(f"symlink source is not allowed: {path}")


def committed_public_files(root: Path = ROOT) -> dict[str, bytes]:
    """The public files package.py packages, as committed at HEAD.

    Read from Git's objects rather than the working tree, so untracked,
    ignored, staged or changed files and the host's file-name rules play no
    part. A source snapshot without Git gives the public files it holds.
    """
    if not (root / ".git").exists():
        # Links above the snapshot (such as /var on macOS) are the host's, not the source's.
        root = root.resolve()
        files = {name: read_regular(root / name) for name in PUBLIC_FILES if (root / name).exists()}
        for directory in PUBLIC_DIRECTORIES:
            for current, directories, names in os.walk(root / directory, followlinks=False):
                for name in directories + names:
                    if (Path(current) / name).is_symlink():
                        raise ValueError(f"symlink source is not allowed: {Path(current) / name}")
                for name in names:
                    files[(Path(current) / name).relative_to(root).as_posix()] = read_regular(Path(current) / name)
        return files
    head = git(root, "rev-parse", "--verify", "HEAD^{commit}").strip().decode("ascii")
    entries = []
    for entry in git(root, "ls-tree", "-r", "-z", "--full-tree", head).split(b"\0"):
        if not entry:
            continue
        metadata, path = entry.split(b"\t", 1)
        name = os.fsdecode(path)
        if public_path(name):
            mode, kind, oid = metadata.decode("ascii").split()
            if kind != "blob" or mode not in ("100644", "100755"):
                raise ValueError(f"committed package source is not a regular file: {name}")
            entries.append((name, oid))
    result = subprocess.run(["git", "--no-replace-objects", "-C", str(root), "cat-file", "--batch"],
                            input="".join(oid + "\n" for _, oid in entries).encode("ascii"),
                            env={**os.environ, "GIT_OPTIONAL_LOCKS": "0"},
                            stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
    if result.returncode:
        raise ValueError("cannot read committed package sources with git cat-file")
    files, output, offset = {}, result.stdout, 0
    for name, oid in entries:
        end = output.find(b"\n", offset)
        header = output[offset:end].decode("ascii", "replace").split()
        if end < 0 or len(header) != 3 or header[0] != oid or header[1] != "blob" or not header[2].isdigit():
            raise ValueError(f"cannot read committed package source: {name}")
        size = int(header[2])
        files[name] = output[end + 1:end + 1 + size]
        offset = end + 1 + size + 1
    return files


def collect(binary: Path, root: Path = ROOT) -> dict[str, bytes]:
    # The explicitly supplied build is not a source-tree input. It keeps its
    # existing PE and regular-file checks, independently of Git tracking.
    files = {"ha.exe": read_regular(binary)}
    windows_amd64(files["ha.exe"])
    top = os.fsdecode(git(root, "rev-parse", "--show-toplevel")).rstrip("\r\n")
    if Path(top).resolve() != root.resolve():
        raise ValueError("package source must be the Git working-tree root")
    head = git(root, "rev-parse", "--verify", "HEAD^{commit}").strip().decode("ascii")
    committed = {}
    for entry in git(root, "ls-tree", "-r", "-z", "--full-tree", head).split(b"\0"):
        if not entry:
            continue
        metadata, path = entry.split(b"\t", 1)
        name = os.fsdecode(path)
        if public_path(name):
            mode, kind, oid = metadata.decode("ascii").split()
            if kind != "blob" or mode not in ("100644", "100755"):
                raise ValueError(f"committed package source is not a regular file: {name}")
            committed[name] = (mode, oid)
    if not set(PUBLIC_FILES).issubset(committed):
        raise ValueError("required public package source is missing from HEAD")
    if public_index(root) != committed:
        raise ValueError("staged public package sources differ from HEAD")
    reject_public_links(root)
    filemode = git(root, "config", "--bool", "--default=true", "--get", "core.filemode").strip() == b"true"
    for name, (mode, oid) in sorted(committed.items()):
        safe_name(name)
        data = read_regular(root / name)
        if filemode and bool((root / name).stat().st_mode & stat.S_IXUSR) != (mode == "100755"):
            raise ValueError(f"public package source mode differs from HEAD: {name}")
        # Git's clean filters and CRLF conversion can hide byte differences
        # from status/diff. Hash the actual bytes as a Git blob, without filters.
        algorithm = {40: "sha1", 64: "sha256"}.get(len(oid))
        if algorithm is None:
            raise ValueError("unsupported Git object identity")
        digest = hashlib.new(algorithm, b"blob " + str(len(data)).encode("ascii") + b"\0" + data)
        if digest.hexdigest() != oid:
            raise ValueError(f"public package source bytes differ from HEAD: {name}")
        files[name] = data
    if (git(root, "rev-parse", "--verify", "HEAD^{commit}").strip().decode("ascii") != head
            or public_index(root) != committed):
        raise ValueError("public package source identity changed during collection")
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
