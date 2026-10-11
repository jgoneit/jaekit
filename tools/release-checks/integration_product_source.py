"""Actual Git objects: old public product and newer reader stay independent."""
import os
from pathlib import Path
import tempfile
import unittest
from unittest import mock

import source_identity as source


class ProductSource(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory(prefix="jaekit-product-source-")
        self.addCleanup(self.directory.cleanup)
        self.root = Path(self.directory.name) / "repo"
        self.root.mkdir()
        source.git(self.root, "init", "-q")
        source.git(self.root, "config", "user.name", "Synthetic source")
        source.git(self.root, "config", "user.email", "source@example.invalid")
        source.git(self.root, "config", "commit.gpgsign", "false")
        (self.root / "product.txt").write_bytes(b"public product\n")
        (self.root / "name\nwith tab\t.txt").write_bytes(b"exact path\n")
        executable = self.root / "tool"
        executable.write_bytes(b"#!/bin/sh\nexit 0\n"); executable.chmod(0o755)
        (self.root / "product-link").symlink_to("product.txt")
        source.git(self.root, "add", "--", ".")
        source.git(self.root, "commit", "-qm", "Synthetic public product")
        self.commit = source.git(self.root, "rev-parse", "HEAD").decode().strip()
        self.tree = source.git(self.root, "rev-parse", "HEAD^{tree}").decode().strip()
        (self.root / "product.txt").write_bytes(b"new checker checkout\n")
        (self.root / "reader.py").write_bytes(b"# updated checker\n")
        source.git(self.root, "add", "--", ".")
        source.git(self.root, "commit", "-qm", "Synthetic newer checker")

    def originals(self):
        return (source.git(self.root, "rev-parse", "HEAD"),
                source.git(self.root, "status", "--porcelain=v1", "-z"),
                (self.root / ".git/index").read_bytes(),
                (self.root / "product.txt").read_bytes())

    def test_old_product_objects_and_new_dirty_checker_are_separate(self):
        (self.root / "product.txt").write_bytes(b"user work in progress\n")
        source.git(self.root, "add", "--", "product.txt")
        (self.root / "reader.py").write_bytes(b"unstaged reader change\n")
        (self.root / "untracked.txt").write_bytes(b"private untracked\n")
        before = self.originals()
        with source.pinned_product(self.root, self.commit, self.tree) as product:
            self.assertNotEqual(product.resolve(), self.root.resolve())
            self.assertEqual((product / "product.txt").read_bytes(), b"public product\n")
            self.assertFalse((product / "reader.py").exists())
            self.assertFalse((product / "untracked.txt").exists())
            self.assertEqual((product / "name\nwith tab\t.txt").read_bytes(), b"exact path\n")
            self.assertEqual(os.readlink(product / "product-link"), "product.txt")
            self.assertTrue((product / "tool").stat().st_mode & 0o111)
            self.assertEqual(source.git(product, "rev-parse", "HEAD^{tree}").decode().strip(), self.tree)
            self.assertEqual(source.git(product, "status", "--porcelain=v1"), b"")
        self.assertFalse(product.exists())
        self.assertEqual(self.originals(), before)

    def test_missing_commit_or_blob_remains_unavailable_without_fetch(self):
        before = self.originals()
        with self.assertRaises(source.SourceUnavailable):
            with source.pinned_product(self.root, "0" * 40, self.tree):
                self.fail("missing commit cannot produce a snapshot")
        oid = source.git(self.root, "rev-parse", self.commit + ":product.txt").decode().strip()
        object_path = self.root / ".git/objects" / oid[:2] / oid[2:]
        object_bytes = object_path.read_bytes()
        object_path.unlink()
        try:
            with self.assertRaises(source.SourceUnavailable):
                with source.pinned_product(self.root, self.commit, self.tree):
                    self.fail("missing blob cannot produce a snapshot")
        finally:
            object_path.write_bytes(object_bytes)
        self.assertEqual(self.originals(), before)

    def test_other_tree_or_non_commit_is_a_mismatch(self):
        newer = source.git(self.root, "rev-parse", "HEAD^{tree}").decode().strip()
        for commit, tree in ((self.commit, newer), (self.tree, self.tree)):
            with self.subTest(commit=commit, tree=tree), self.assertRaises(source.SourceMismatch):
                with source.pinned_product(self.root, commit, tree):
                    self.fail("mismatched public identities must not pass")

    def test_promisor_missing_blob_is_not_lazily_fetched(self):
        remote = Path(self.directory.name) / "public-remote.git"
        source.git(self.root, "clone", "--bare", "--no-hardlinks", str(self.root), str(remote))
        source.git(self.root, "config", "extensions.partialClone", "origin")
        source.git(self.root, "config", "remote.origin.promisor", "true")
        source.git(self.root, "config", "remote.origin.partialclonefilter", "blob:none")
        source.git(self.root, "config", "remote.origin.url", remote.resolve().as_uri())
        oid = source.git(self.root, "rev-parse", self.commit + ":product.txt").decode().strip()
        object_path = self.root / ".git/objects" / oid[:2] / oid[2:]
        data = object_path.read_bytes()
        object_path.unlink()
        before = {path.relative_to(self.root / ".git/objects").as_posix(): path.read_bytes()
                  for path in (self.root / ".git/objects").rglob("*") if path.is_file()}
        try:
            with self.assertRaises(source.SourceUnavailable):
                with source.pinned_product(self.root, self.commit, self.tree):
                    self.fail("locally missing public object must not be fetched")
            after = {path.relative_to(self.root / ".git/objects").as_posix(): path.read_bytes()
                     for path in (self.root / ".git/objects").rglob("*") if path.is_file()}
            self.assertEqual(after, before)
        finally:
            object_path.write_bytes(data)

    def test_environment_cannot_redirect_source_or_snapshot_git(self):
        other = Path(self.directory.name) / "other"
        other.mkdir()
        before = self.originals()
        with mock.patch.dict(os.environ, {"GIT_DIR": str(other), "GIT_WORK_TREE": str(other),
                                          "GIT_INDEX_FILE": str(other / "index"),
                                          "GIT_OBJECT_DIRECTORY": str(other / "objects")}):
            with source.pinned_product(self.root, self.commit, self.tree) as product:
                self.assertEqual((product / "product.txt").read_bytes(), b"public product\n")
        self.assertEqual(list(other.iterdir()), [])
        self.assertEqual(self.originals(), before)

    def test_symbolic_or_malformed_identity_is_not_resolved_or_executed(self):
        for identity in ("HEAD", "main", "-x", "A" * 40, "../source", "0" * 39):
            with self.subTest(identity=identity), mock.patch.object(source, "git", side_effect=AssertionError("no Git")):
                with self.assertRaises(source.SourceUnavailable):
                    with source.pinned_product(self.root, identity, self.tree):
                        self.fail("unsupported identity must not be interpreted")

    def test_both_snapshots_reject_unverified_link_targets(self):
        outside = Path(self.directory.name) / "outside-sentinel"
        outside.write_bytes(b"outside must remain untouched\n")
        link = self.root / "unsafe-link"
        for target in (str(outside), "../outside-sentinel", "absent", "unsafe-link", "."):
            with self.subTest(target=target):
                if link.is_symlink():
                    link.unlink()
                link.symlink_to(target)
                source.git(self.root, "add", "--", "unsafe-link")
                source.git(self.root, "commit", "-qm", "Synthetic unsafe link")
                commit = source.git(self.root, "rev-parse", "HEAD").decode().strip()
                tree = source.git(self.root, "rev-parse", "HEAD^{tree}").decode().strip()
                before = self.originals()
                with self.assertRaises(source.SourceUnavailable):
                    with source.pinned_product(self.root, commit, tree):
                        self.fail("unsafe product symlink must not enter a run")
                with self.assertRaises(source.SourceUnavailable):
                    with source.pinned(self.root, tree, {}):
                        self.fail("unsafe checker symlink must not enter a run")
                self.assertEqual(outside.read_bytes(), b"outside must remain untouched\n")
                self.assertEqual(self.originals(), before)

    def test_both_snapshots_accept_relative_links_to_verified_files(self):
        subdir = self.root / "subdir"
        subdir.mkdir()
        (subdir / "relative-link").symlink_to("../product-link")
        source.git(self.root, "add", "--", "subdir")
        source.git(self.root, "commit", "-qm", "Synthetic internal link chain")
        commit = source.git(self.root, "rev-parse", "HEAD").decode().strip()
        tree = source.git(self.root, "rev-parse", "HEAD^{tree}").decode().strip()
        for selected in (source.pinned(self.root, tree, {}), source.pinned_product(self.root, commit, tree)):
            with selected as product:
                self.assertEqual((product / "subdir/relative-link").read_bytes(), b"new checker checkout\n")
                self.assertEqual(source.git(product, "rev-parse", "HEAD^{tree}").decode().strip(), tree)


if __name__ == "__main__":
    unittest.main()
