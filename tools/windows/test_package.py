"""Local candidate packaging regressions; these do not execute Windows code."""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import stat
import struct
import subprocess
import tempfile
import unittest
from unittest import mock
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
        # Every Git operation, including the packager's subprocesses, uses only
        # this fixture's config/index. Never consult or alter user Git settings.
        environment = {name: value for name, value in os.environ.items() if not name.startswith("GIT_")}
        environment.update(GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
                           GIT_CONFIG_SYSTEM=os.devnull, GIT_TERMINAL_PROMPT="0")
        isolated = mock.patch.dict(os.environ, environment, clear=True)
        isolated.start()
        self.addCleanup(isolated.stop)
        names = ["LICENSE", "README.md", "README.en.md", "tools/check-result-reference.py",
                 "guides/WINDOWS.md", "guides/WINDOWS.en.md", "contracts/run-record.md",
                 "assets/logo.svg", "docs/specs/private/SPEC.md", "private.txt"]
        names += [f"examples/check-result/{name}" for name in
                  ("README.md", "declaration.json", "reference.json", "input.txt")]
        names += [f"tools/windows/{name}" for name in
                  ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1", "README.md")]
        for name in names:
            path = self.source / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(("synthetic public content: " + name + "\n").encode("utf-8"))
        (self.source / ".gitignore").write_bytes(b"*.ignored\n")
        self.git("init", "--quiet")
        self.git("config", "core.autocrlf", "false")
        self.git("config", "core.safecrlf", "false")
        self.git("add", ".")
        self.commit()
        self.binary = self.base / "ha.exe"
        self.binary.write_bytes(synthetic_pe())

    def git(self, *args):
        result = subprocess.run(["git", "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                                 "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + str(self.base / "no-hooks"),
                                 "-C", str(self.source), *args],
                                stdout=subprocess.PIPE, stderr=subprocess.PIPE, check=False)
        self.assertEqual(0, result.returncode, result.stderr.decode("utf-8", errors="replace"))
        return result.stdout

    def commit(self):
        self.git("commit", "--quiet", "-m", "Synthetic package source")

    def source_snapshot(self):
        # Include the real index/config and every file's bytes, even ignored
        # files. No git-status refresh is needed to test nonmutation.
        return {path.relative_to(self.source).as_posix(): (path.read_bytes(), stat.S_IMODE(path.stat().st_mode))
                for path in self.source.rglob("*") if path.is_file() and not path.is_symlink()}

    def assert_rejected_without_mutation(self, pattern=None):
        before = self.source_snapshot()
        with self.assertRaises((ValueError, OSError)) as error:
            self.prepare()
        if pattern:
            self.assertRegex(str(error.exception), pattern)
        self.assertFalse((self.base / "candidate").exists())
        self.assertEqual(before, self.source_snapshot())

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
            self.assertNotIn("private.txt", zf.namelist())
            self.assertEqual(synthetic_pe(), zf.read("ha.exe"))
            for info in zf.infolist():
                self.assertEqual(stat.S_IFREG, stat.S_IFMT(info.external_attr >> 16))
        sums = archive.with_name("checksums.txt").read_text().splitlines()
        self.assertEqual(5, len(sums))
        for line in sums:
            digest, name = line.split("  ", 1)
            self.assertEqual(digest, hashlib.sha256(archive.with_name(name).read_bytes()).hexdigest())

    def test_untracked_and_ignored_public_files_are_excluded_without_mutation(self):
        for directory in ("guides", "contracts", "assets"):
            for suffix in ("draft", "ignored"):
                path = self.source / directory / ("local." + suffix)
                path.write_bytes(b"synthetic-private-sentinel-" + directory.encode() + suffix.encode())
            self.git("check-ignore", directory + "/local.ignored")
        self.assertIn(b"?", self.git("status", "--porcelain", "--untracked-files=all"))
        before = self.source_snapshot()
        archive = self.prepare()
        with zipfile.ZipFile(archive) as zf:
            manifest = json.loads(zf.read("package.json"))
            expected = set(package.PUBLIC_FILES) | {"guides/WINDOWS.md", "guides/WINDOWS.en.md",
                                                   "contracts/run-record.md", "assets/logo.svg", "ha.exe"}
            self.assertEqual(expected, {entry["path"] for entry in manifest["files"]})
            self.assertEqual(expected | {"package.json"}, set(zf.namelist()))
            for name in zf.namelist():
                self.assertNotIn(b"synthetic-private-sentinel-", zf.read(name))
        for path in archive.parent.iterdir():
            if path != archive:
                self.assertNotIn(b"synthetic-private-sentinel-", path.read_bytes())
        self.assertEqual(before, self.source_snapshot())

    def test_staged_public_add_modify_delete_and_unmerged_inputs_are_rejected(self):
        path = self.source / "guides/WINDOWS.md"
        original = path.read_bytes()
        for state in ("add", "modify", "delete", "unmerged"):
            with self.subTest(state=state):
                if state == "add":
                    added = self.source / "assets/staged.txt"
                    added.write_bytes(b"not committed")
                    self.git("add", "assets/staged.txt")
                elif state == "modify":
                    path.write_bytes(original + b"not committed\n")
                    self.git("add", "guides/WINDOWS.md")
                elif state == "delete":
                    self.git("rm", "--cached", "guides/WINDOWS.md")
                else:
                    oid = self.git("rev-parse", "HEAD:guides/WINDOWS.md").strip().decode("ascii")
                    entry = f"100644 {oid} 1\tguides/WINDOWS.md\n".encode("ascii")
                    result = subprocess.run(["git", "-C", str(self.source), "update-index", "--index-info"],
                                            input=entry, capture_output=True, check=False)
                    self.assertEqual(0, result.returncode, result.stderr)
                self.assert_rejected_without_mutation("staged|unmerged")
                self.git("reset", "--quiet", "HEAD", "--", "guides/WINDOWS.md", "assets/staged.txt")
                path.write_bytes(original)
                if (self.source / "assets/staged.txt").exists():
                    (self.source / "assets/staged.txt").unlink()

    def test_worktree_changes_deletions_and_missing_committed_inputs_are_rejected(self):
        path = self.source / "tools/windows/common.ps1"
        original = path.read_bytes()
        path.write_bytes(original + b"uncommitted\n")
        self.assert_rejected_without_mutation("bytes differ from HEAD")
        path.unlink()
        self.assert_rejected_without_mutation()
        self.git("rm", "tools/windows/common.ps1")
        self.commit()
        self.assert_rejected_without_mutation("missing from HEAD")

    @unittest.skipIf(os.name == "nt", "Windows does not provide Git's Unix executable-bit semantics")
    def test_unstaged_executable_bit_changes_follow_repository_filemode_policy(self):
        path = self.source / "guides/WINDOWS.md"
        self.git("config", "core.filemode", "true")
        path.chmod(path.stat().st_mode | stat.S_IXUSR)
        self.assertIn(b" M guides/WINDOWS.md", self.git("status", "--porcelain"))
        self.assert_rejected_without_mutation("mode differs from HEAD")
        self.git("config", "core.filemode", "false")
        self.assertEqual(b"", self.git("status", "--porcelain"))
        self.prepare()

    def test_git_clean_crlf_and_assume_unchanged_do_not_hide_changed_bytes(self):
        path = self.source / "guides/WINDOWS.md"
        original = path.read_bytes()
        (self.source / ".gitattributes").write_bytes(b"guides/WINDOWS.md text\n")
        self.git("add", ".gitattributes")
        self.commit()
        self.git("config", "core.autocrlf", "true")
        path.write_bytes(original.replace(b"\n", b"\r\n"))
        # Refreshing the fixture index here makes Git's own clean claim exact;
        # the production packager must still reject its different raw bytes.
        self.git("add", "guides/WINDOWS.md")
        self.assertEqual(b"", self.git("status", "--porcelain"))
        self.assert_rejected_without_mutation("bytes differ from HEAD")
        path.write_bytes(original + b"hidden edit\n")
        self.git("update-index", "--assume-unchanged", "guides/WINDOWS.md")
        self.assertEqual(b"", self.git("status", "--porcelain"))
        self.assert_rejected_without_mutation("bytes differ from HEAD")

    def test_unrelated_tracked_changes_do_not_become_package_inputs(self):
        (self.source / "private.txt").write_bytes(b"unrelated synthetic private edit")
        self.git("add", "private.txt")
        before = self.source_snapshot()
        archive = self.prepare()
        with zipfile.ZipFile(archive) as zf:
            self.assertNotIn("private.txt", zf.namelist())
        self.assertEqual(before, self.source_snapshot())

    def test_non_git_source_is_rejected_without_output(self):
        (self.source / ".git").rename(self.source / "not-git")
        self.assert_rejected_without_mutation("cannot read committed")

    def test_committed_symlink_mode_is_rejected_even_if_checkout_is_regular(self):
        oid = self.git("rev-parse", "HEAD:assets/logo.svg").strip().decode("ascii")
        self.git("update-index", "--cacheinfo", "120000," + oid + ",assets/logo.svg")
        self.commit()
        self.assertTrue((self.source / "assets/logo.svg").is_file())
        self.assertFalse((self.source / "assets/logo.svg").is_symlink())
        self.assert_rejected_without_mutation("not a regular file")

    def test_case_collision_and_package_limits_are_preserved(self):
        data = (self.source / "assets/logo.svg").read_bytes()
        oid = self.git("rev-parse", "HEAD:assets/logo.svg").strip().decode("ascii")
        # Install both Git names even on a case-insensitive development volume.
        # Identical bytes let the reader reach the intended collision guard.
        (self.source / "assets/LOGO.svg").write_bytes(data)
        self.git("update-index", "--add", "--cacheinfo", "100644," + oid + ",assets/LOGO.svg")
        self.commit()
        self.assert_rejected_without_mutation("case-insensitive")

    def test_file_count_and_byte_limits_are_preserved(self):
        large = self.source / "assets/large.txt"
        large.write_bytes(b"x" * 512)
        self.git("add", "assets/large.txt")
        self.commit()
        for name, limit in (("MAX_FILES", 2), ("MAX_FILE", 256), ("MAX_TOTAL", 512)):
            with self.subTest(limit=name), mock.patch.object(package, name, limit):
                self.assert_rejected_without_mutation("limit")

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
