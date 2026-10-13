"""Local candidate packaging regressions; these do not execute Windows code."""
from __future__ import annotations

import hashlib
import importlib.util
import json
from pathlib import Path
import stat
import struct
import tempfile
import unittest
import zipfile

spec = importlib.util.spec_from_file_location("windows_package", Path(__file__).with_name("package.py"))
package = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(package)


def synthetic_pe(machine: int = 0x8664) -> bytes:
    data = bytearray(128)
    data[:2] = b"MZ"
    struct.pack_into("<I", data, 0x3C, 64)
    data[64:68] = b"PE\0\0"
    struct.pack_into("<H", data, 68, machine)
    struct.pack_into("<H", data, 88, 0x20B)
    return bytes(data)


class CandidatePackageTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="windows-package-test-")
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name).resolve()
        self.source = self.base / "source"
        names = ["LICENSE", "README.md", "README.en.md", "tools/check-result-reference.py",
                 "guides/WINDOWS.md", "guides/WINDOWS.en.md", "contracts/run-record.md",
                 "assets/logo.svg", "docs/specs/private/SPEC.md", ".git/config"]
        names += [f"examples/check-result/{name}" for name in
                  ("README.md", "declaration.json", "reference.json", "input.txt")]
        names += [f"tools/windows/{name}" for name in
                  ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1", "README.md")]
        for name in names:
            path = self.source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("synthetic public content: " + name + "\n", encoding="utf-8")
        self.binary = self.base / "ha.exe"
        self.binary.write_bytes(synthetic_pe())

    def prepare(self, output="candidate", version="0.1.4-dev"):
        return package.prepare(self.binary, version, self.base / output, self.source)

    def test_archive_inventory_integrity_and_public_boundary(self):
        archive = self.prepare()
        with zipfile.ZipFile(archive) as zf:
            manifest = json.loads(zf.read("package.json"))
            self.assertEqual("jaekit-windows-package/v1", manifest["schema"])
            self.assertEqual(("windows", "amd64", "0.1.4-dev"),
                             (manifest["os"], manifest["arch"], manifest["version"]))
            declared = {entry["path"] for entry in manifest["files"]}
            self.assertEqual(declared | {"package.json"}, set(zf.namelist()))
            for entry in manifest["files"]:
                data = zf.read(entry["path"])
                self.assertEqual(entry["size"], len(data))
                self.assertEqual(entry["sha256"], hashlib.sha256(data).hexdigest())
            self.assertFalse(any(name.startswith(("docs/", ".git/")) for name in zf.namelist()))
            self.assertEqual(synthetic_pe(), zf.read("ha.exe"))
            for info in zf.infolist():
                self.assertEqual(stat.S_IFREG, stat.S_IFMT(info.external_attr >> 16))
        sums = archive.with_name("checksums.txt").read_text().splitlines()
        self.assertEqual(5, len(sums))
        for line in sums:
            digest, name = line.split("  ", 1)
            self.assertEqual(digest, hashlib.sha256(archive.with_name(name).read_bytes()).hexdigest())

    def test_reproducible_candidates_and_conflict_preservation(self):
        first = self.prepare("first")
        second = self.prepare("second")
        self.assertEqual(first.read_bytes(), second.read_bytes())
        sentinel = first.with_name("user-file.txt")
        sentinel.write_text("keep")
        original = first.read_bytes()
        with self.assertRaisesRegex(ValueError, "already exists"):
            self.prepare("first")
        self.assertEqual(original, first.read_bytes())
        self.assertEqual("keep", sentinel.read_text())

    def test_rejects_wrong_binary_without_output(self):
        for data in (b"#!/bin/sh\n", synthetic_pe(0xAA64), synthetic_pe(0x14C)):
            with self.subTest(data=data[:8]):
                self.binary.write_bytes(data)
                with self.assertRaises(ValueError):
                    self.prepare()
                self.assertFalse((self.base / "candidate").exists())

    def test_rejects_unsafe_version_and_windows_names(self):
        for version in ("../outside", "1.0", "v1.0.0", "1.0.0/elsewhere", "1.0.0-dev."):
            with self.subTest(version=version), self.assertRaises(ValueError):
                self.prepare(version=version)
        for name in ("../outside", "C:/temp/x", "docs\\x", "assets/CON.txt", "assets/a.",
                     "assets//x", "/absolute", "assets/x:stream", "assets/line\nname",
                     "assets/COM\u00b9.txt", "assets/LPT\u00b3"):
            with self.subTest(name=name), self.assertRaises(ValueError):
                package.safe_name(name)
        for name in ("guides/WINDOWS.md", "assets/공백 name.svg", "contracts/auxiliary.md"):
            package.safe_name(name)

    def test_rejects_public_symlink_file_and_directory(self):
        secret = self.base / "secret.txt"
        secret.write_text("not public")
        link = self.source / "assets" / "linked.txt"
        link.symlink_to(secret)
        with self.assertRaisesRegex(ValueError, "symlink"):
            self.prepare()
        self.assertFalse((self.base / "candidate").exists())
        link.unlink()
        link.symlink_to(secret.parent, target_is_directory=True)
        with self.assertRaisesRegex(ValueError, "symlink"):
            self.prepare()

    def test_installer_sources_are_readable_by_ps51_without_encoding_guess(self):
        # PS5.1 treats BOM-less source as the system code page. Keep executable
        # scripts ASCII; the guides and copied public data remain UTF-8.
        for name in ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1"):
            Path(__file__).with_name(name).read_bytes().decode("ascii")


if __name__ == "__main__":
    unittest.main()
