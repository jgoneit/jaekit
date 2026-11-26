"""Public metadata boundary regressions on disposable, synthetic Git trees."""
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
ENV = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
           GIT_AUTHOR_NAME="Synthetic", GIT_AUTHOR_EMAIL="example@example.invalid",
           GIT_COMMITTER_NAME="Synthetic", GIT_COMMITTER_EMAIL="example@example.invalid")
for key in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
    ENV.pop(key, None)


class BoundaryViolation(AssertionError):
    def __init__(self, code, message):
        self.code = code
        super().__init__(message)


def require(value, code, message):
    if not value:
        raise BoundaryViolation(code, message)


def git(root, *args):
    return subprocess.run(["git", *args], cwd=root, env=ENV, check=True,
                          capture_output=True, timeout=30).stdout.decode("utf-8", "surrogateescape").strip()


def commit(root):
    git(root, "add", "-A")
    git(root, "commit", "--quiet", "--allow-empty", "-m", "synthetic change")
    return git(root, "rev-parse", "HEAD")


def repository(root):
    git(root, "init", "--quiet", "-b", "main")
    git(root, "config", "core.hooksPath", os.devnull)
    git(root, "config", "commit.gpgsign", "false")
    git(root, "config", "gc.auto", "0")
    return commit(root)


def put(root, name, data=b"safe\n"):
    target = root / name
    target.parent.mkdir(parents=True, exist_ok=True)
    target.write_bytes(data)
    return target


def current(root):
    path = root / "tools/check_public_tree.py"
    path.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(ROOT / "tools/check_public_tree.py", path)
    argv = [sys.executable, str(path)]
    result = subprocess.run(argv, cwd=root, env=ENV, capture_output=True, text=True, timeout=30)
    if result.returncode not in (0, 1):
        raise RuntimeError("current boundary did not run: " + result.stderr)
    return result.returncode, result.stdout + result.stderr


def current_paths():
    bad = "tools/" + "Users/synthetic-person/note.bin"
    for data in (b"safe", b"\xff\xfe"):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository(root)
            put(root, bad, data)
            commit(root)
            status, output = current(root)
            require(status == 1 and "path" in output and bad in output,
                    "path-not-rejected", "tracked path-only reference was not identified")


def special_names():
    for prefix in ("a b", "a\nb", "a\tb", 'a"b', "a\\b", "a\rb"):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository(root)
            public_bad = "tools/" + prefix + "/Users/" + "synthetic-person/sample.bin"
            put(root, public_bad, b"\xff")
            commit(root)
            status, output = current(root)
            require(status == 1 and repr(public_bad) in output, "name-not-preserved",
                    "current public boundary lost path metadata")


class CurrentPathChecks(unittest.TestCase):
    def test_path_only_binary_and_text(self):
        current_paths()


class NameChecks(unittest.TestCase):
    def test_metadata_names_are_not_line_records(self):
        special_names()


class PreservedBoundaries(unittest.TestCase):
    def test_normal_current_files_need_no_history(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository(root)
            put(root, "guides/ordinary.txt")
            put(root, "testdata/synthetic.txt", ("/Users/" + "synthetic-person/sample").encode())
            commit(root)
            self.assertEqual(current(root)[0], 0)
            put(root, "guides/link.txt", ("https://github.com/jgoneit/" + "jaekit-fixture").encode())
            commit(root)
            self.assertEqual(current(root)[0], 1)

    def test_symlinks_and_nonpublic_paths_keep_their_rejection(self):
        for name, symlink in (("guides/link", True), ("private.txt", False)):
            with tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                repository(root)
                target = root / name
                target.parent.mkdir(parents=True, exist_ok=True)
                if symlink:
                    target.symlink_to("not-read")
                else:
                    target.write_text("safe")
                commit(root)
                self.assertEqual(current(root)[0], 1)


if __name__ == "__main__":
    unittest.main()
