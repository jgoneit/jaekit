"""Public-source packaging refuses console names before any unsafe file read."""
from __future__ import annotations

import json
from pathlib import Path
import unittest
from unittest import mock

import test_package as fixtures

MATRIX = json.loads(Path(__file__).with_name("console_cases.json").read_text(encoding="utf-8"))


class ConsolePackageTests(unittest.TestCase):
    setUp = fixtures.CandidatePackageTests.setUp
    git = fixtures.CandidatePackageTests.git
    commit = fixtures.CandidatePackageTests.commit
    source_snapshot = fixtures.CandidatePackageTests.source_snapshot
    prepare = fixtures.CandidatePackageTests.prepare

    def test_console_policy_and_normal_lookalikes(self):
        for item in MATRIX["rejected"]:
            with self.subTest(path=item["path"]), self.assertRaisesRegex(ValueError, "unsafe Windows package path"):
                fixtures.package.safe_name(item["path"])
        for item in MATRIX["allowed"]:
            with self.subTest(path=item["path"]):
                fixtures.package.safe_name(item["path"])

    def test_tracked_console_components_refuse_before_read_and_preserve_data(self):
        existing = self.prepare("existing")
        sentinel = existing.with_name("user-sentinel.txt")
        sentinel.write_bytes(b"unrelated candidate data")
        existing_before = {p.name: p.read_bytes() for p in existing.parent.iterdir()}
        oid = self.git("rev-parse", "HEAD:assets/logo.svg").strip().decode("ascii")
        for item in MATRIX["rejected"]:
            # Populate Git metadata only: never create or open a device spelling.
            name = item["path"] if item["path"].startswith("assets/") else "assets/" + item["path"]
            self.git("update-index", "--add", "--cacheinfo", "100644," + oid + "," + name)
            self.commit()
            before = self.source_snapshot()
            original_read = fixtures.package.read_regular
            def guarded_read(path):
                self.assertNotEqual(self.source / name, path, "unsafe tracked input was opened")
                return original_read(path)
            with self.subTest(path=name), mock.patch.object(fixtures.package, "read_regular", side_effect=guarded_read):
                with self.assertRaisesRegex(ValueError, "unsafe Windows package path"):
                    self.prepare()
            self.assertFalse((self.base / "candidate").exists())
            self.assertEqual(before, self.source_snapshot())
            self.assertEqual(existing_before, {p.name: p.read_bytes() for p in existing.parent.iterdir()})
            self.git("update-index", "--force-remove", name)
            self.commit()

    def test_normal_names_remain_in_real_package_inventory(self):
        for item in MATRIX["allowed"]:
            path = self.source / item["path"]
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(b"ordinary fixture bytes")
            self.git("add", item["path"])
        self.commit()
        before = self.source_snapshot()
        archive = self.prepare()
        with fixtures.zipfile.ZipFile(archive) as zipped:
            manifest = json.loads(zipped.read("package.json"))
            listed = {item["path"] for item in manifest["files"]}
            for item in MATRIX["allowed"]:
                self.assertEqual(b"ordinary fixture bytes", zipped.read(item["path"]))
                self.assertIn(item["path"], listed)
        self.assertEqual(before, self.source_snapshot())


if __name__ == "__main__":
    unittest.main()
