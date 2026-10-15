#!/usr/bin/env python3
"""Compare the Windows archive rule with the installer's own package check.

Each case is one synthetic Windows archive. The archive rule that the pre- and
post-publication checks share (check_archives.windows_files) judges it in
Python; on Windows, the installer's package check (Expand-CheckedPackage in
tools/windows/common.ps1) judges the same file in Windows PowerShell 5.1 and
PowerShell 7. Besides a normal archive, the cases are formats the installer
itself refuses: the manifest size limit, a non-integer size, DOS directory or
reparse-point attributes and entry names with a NUL or a backslash.

  ci --report PATH   Windows only. Writes jaekit-windows-contract/v1 and exits
                     1 unless every verdict is the expected one.

The release workflow keeps this report as an observation: the step never
changes which files are published. Nothing is installed; each archive is
expanded into its own temporary directory.
"""
from __future__ import annotations

import argparse
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import stat
import struct
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]
SCHEMA = "jaekit-windows-contract/v1"
VERSION = "0.0.0"
VERDICT = "JAEKIT_CONTRACT "


def load(name: str, path: str):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


check_archives = load("jaekit_contract_archives", "tools/release/check_archives.py")


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def pe_amd64() -> bytes:
    data = bytearray(512)
    data[:2] = b"MZ"
    struct.pack_into("<I", data, 0x3C, 0x80)
    data[0x80:0x84] = b"PE\0\0"
    struct.pack_into("<H", data, 0x84, 0x8664)
    struct.pack_into("<H", data, 0x80 + 24, 0x20B)
    return bytes(data)


def package_files() -> dict[str, bytes]:
    files = {name: b"Synthetic public document.\n" for name in check_archives.WINDOWS_REQUIRED - {"package.json"}}
    files["ha.exe"] = pe_amd64()
    return files


def manifest(files: dict[str, bytes]) -> dict:
    return {"schema": check_archives.WINDOWS_SCHEMA, "version": VERSION, "os": "windows", "arch": "amd64",
            "files": [{"path": name, "size": len(body), "sha256": sha256(body)} for name, body in sorted(files.items())]}


def archive(files: dict[str, bytes], listed, attributes=None, written=None) -> bytes:
    """The package.py layout. `attributes` overrides an entry's external
    attributes; `written` stores an entry under another raw name of the same
    length, such as one with a NUL or a backslash that ZIP writers normalize."""
    body = listed if isinstance(listed, bytes) else (json.dumps(listed, sort_keys=True, indent=2) + "\n").encode()
    written = written or {}
    placeholders = {name: raw.replace("\0", "!").replace("\\", "!") for name, raw in written.items()}
    output = io.BytesIO()
    with zipfile.ZipFile(output, "w", compression=zipfile.ZIP_DEFLATED) as target:
        for name, data in sorted(dict(files, **{"package.json": body}).items()):
            info = zipfile.ZipInfo(placeholders.get(name, name), date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = (attributes or {}).get(name, (stat.S_IFREG | 0o644) << 16)
            info.compress_type = zipfile.ZIP_DEFLATED
            target.writestr(info, data)
    data = output.getvalue()
    for name, raw in written.items():
        placeholder = placeholders[name].encode("utf-8")
        if len(placeholder) != len(raw.encode("utf-8")) or data.count(placeholder) != 2:
            raise ValueError("a raw entry name must replace exactly one entry's two headers")
        data = data.replace(placeholder, raw.encode("utf-8"))
    return data


def cases() -> list[tuple[str, str, bytes]]:
    """(name, expected verdict, archive bytes)."""
    files = package_files()
    one = dict(files, **{"LICENSE": b"x"})
    boolean = manifest(one)
    for item in boolean["files"]:
        if item["path"] == "LICENSE":
            item["size"] = True
    oversized = json.dumps(manifest(files), sort_keys=True, indent=2).encode() + b" " * (1024 * 1024) + b"\n"
    regular = (stat.S_IFREG | 0o644) << 16
    return [
        ("normal", "accept", archive(files, manifest(files))),
        ("an oversized manifest", "reject", archive(files, oversized)),
        ("a boolean size", "reject", archive(one, boolean)),
        ("a DOS directory attribute", "reject", archive(files, manifest(files), {"LICENSE": regular | 0x10})),
        ("a DOS reparse-point attribute", "reject", archive(files, manifest(files), {"LICENSE": regular | 0x400})),
        ("a NUL in an entry name", "reject", archive(files, manifest(files), written={"LICENSE": "LICENSE\0x"})),
        ("a backslash in an entry name", "reject",
         archive(files, manifest(files), written={"guides/WINDOWS.md": "guides\\WINDOWS.md"})),
    ]


def python_verdict(path: Path) -> tuple[str, str]:
    try:
        check_archives.windows_files(path, sha256(path.read_bytes()), VERSION)
    except ValueError as error:
        return "reject", str(error)
    return "accept", ""


def installer_verdict(shell: str, directory: Path, destination: Path) -> tuple[str, str]:
    """The installer's package check on one archive; `error` when it did not
    report a verdict at all."""
    blocks = load("jaekit_contract_blocks", "tools/windows/public_blocks.py")

    def quote(path):
        return "'" + str(path).replace("'", "''") + "'"

    name = f"ha_{VERSION}_windows_amd64.zip"
    text = "\n".join([
        ". " + quote(ROOT / "tools/windows/common.ps1"),
        "try {",
        f"  Expand-CheckedPackage {quote(directory / name)} {quote(directory / 'checksums.txt')} {quote(destination)} | Out-Null",
        f"  [Console]::Out.WriteLine('{VERDICT}accept')",
        "} catch {",
        f"  [Console]::Out.WriteLine('{VERDICT}reject ' + $_.Exception.Message)",
        "}",
    ])
    code, out, err = blocks.command(blocks.shell_path(shell), text, blocks.base_env(), bypass=True)
    for line in out.splitlines():
        if line.startswith(VERDICT):
            verdict, _, reason = line[len(VERDICT):].partition(" ")
            return verdict, reason
    return "error", f"exit {code}: {err.strip()[-500:]}"


def contract(installer=installer_verdict, shells=("powershell", "pwsh")) -> dict:
    report = {"schema": SCHEMA, "version": VERSION, "cases": []}
    with tempfile.TemporaryDirectory(prefix="jaekit-windows-contract-") as temp:
        for number, (name, expected, data) in enumerate(cases()):
            directory = Path(temp) / f"case-{number}"
            directory.mkdir()
            path = directory / f"ha_{VERSION}_windows_amd64.zip"
            path.write_bytes(data)
            (directory / "checksums.txt").write_text(f"{sha256(data)}  {path.name}\n", encoding="utf-8")
            python, reason = python_verdict(path)
            verdicts, reasons = {}, {}
            for shell in shells:
                verdicts[shell], reasons[shell] = installer(shell, directory, Path(temp) / f"expanded-{number}-{shell}")
            report["cases"].append({"name": name, "expected": expected, "python": python, "python_reason": reason,
                                    "installer": verdicts, "installer_reasons": reasons})
    report["passed"] = all(case["python"] == case["expected"]
                           and all(verdict == case["expected"] for verdict in case["installer"].values())
                           for case in report["cases"])
    return report


def ci(args) -> int:
    if os.name != "nt":
        raise SystemExit("ci mode compares with the installer and runs on Windows only")
    report = contract()
    report["source_commit"] = subprocess.check_output(["git", "rev-parse", "HEAD"], cwd=ROOT, text=True).strip()
    path = Path(args.report)
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    for case in report["cases"]:
        print(f"{case['name']}: expected {case['expected']}, python {case['python']}, "
              + ", ".join(f"{shell} {verdict}" for shell, verdict in sorted(case["installer"].items())))
    print("PASS" if report["passed"] else "FAIL: the archive rule and the installer disagree")
    return 0 if report["passed"] else 1


def main(argv=None) -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = parser.add_subparsers(dest="mode", required=True)
    run = sub.add_parser("ci")
    run.add_argument("--report", required=True)
    args = parser.parse_args(argv)
    return ci(args)


if __name__ == "__main__":
    sys.exit(main())
