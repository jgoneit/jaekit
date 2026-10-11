"""Public metadata boundary regressions on disposable, synthetic Git trees."""
import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
GOAL = "release-" + "0-1-3"
HISTORY = ROOT / "tools/goal-checks" / (GOAL + "-followups") / "preserved.py"
INDEPENDENCE = ROOT / "tools/goal-checks" / (GOAL + "-review-fixes") / "test_private_independence.py"
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


def audit(root, base, head):
    results = []
    for entry in (HISTORY, INDEPENDENCE):
        p = subprocess.run([sys.executable, str(entry), "--history", "--base", base, "--head", head],
                           cwd=root, env=ENV, capture_output=True, text=True, timeout=30)
        if p.returncode not in (0, 1, 2):
            raise RuntimeError("audit did not run: " + p.stderr)
        results.append((p.returncode, p.stdout + p.stderr))
    return results


def current(root, private=False):
    if private:
        argv = [sys.executable, str(INDEPENDENCE), "TrackedFiles"]
    else:
        path = root / "tools/check_public_tree.py"
        path.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(ROOT / "tools/check_public_tree.py", path)
        argv = [sys.executable, str(path)]
    result = subprocess.run(argv, cwd=root, env=ENV, capture_output=True, text=True, timeout=30)
    if result.returncode not in (0, 1):
        raise RuntimeError("current boundary did not run: " + result.stderr)
    return result.returncode, result.stdout + result.stderr


def current_paths():
    for private in (False, True):
        bad = "tools/" + ("evidence-" + "seq-synthetic.json" if private else "Users/synthetic-person/note.bin")
        for data in (b"safe", b"\xff\xfe"):
            with tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                repository(root)
                put(root, bad, data)
                commit(root)
                status, output = current(root, private)
                require(status == 1 and "path" in output and bad in output,
                        "path-not-rejected", "tracked path-only reference was not identified")


def history_paths():
    for mode in ("delete", "rename", "rename-in", "revert", "side", "merge-result"):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            base = repository(root)
            bad = "tools/" + "evidence-" + "final-synthetic.json"
            if mode in ("side", "merge-result"):
                git(root, "checkout", "--quiet", "-b", "side")
            if mode == "merge-result":
                put(root, "tools/side.txt")
                commit(root)
                git(root, "checkout", "--quiet", "main")
                put(root, "tools/main.txt")
                commit(root)
                git(root, "merge", "--quiet", "--no-ff", "--no-commit", "side")
            if mode == "rename-in":
                put(root, "tools/safe.txt")
                commit(root)
                git(root, "mv", "tools/safe.txt", bad)
            else:
                put(root, bad, b"\xff\xfe" if mode == "delete" else b"safe\n")
            introduced = commit(root)
            if mode == "rename":
                git(root, "mv", bad, "tools/safe.txt")
                commit(root)
            elif mode == "revert":
                git(root, "revert", "--no-edit", introduced)
            elif mode in ("delete", "merge-result"):
                (root / bad).unlink()
                commit(root)
            elif mode == "side":
                git(root, "checkout", "--quiet", "main")
                git(root, "merge", "--quiet", "--no-ff", "side", "-m", "synthetic merge")
            for status, output in audit(root, base, git(root, "rev-parse", "HEAD")):
                require(status == 1 and "path" in output, "history-path-not-rejected",
                        "transient or merged path introduction was not reported: " + mode)


def special_names():
    for prefix in ("a b", "a\nb", "a\tb", 'a"b', "a\\b", "a\rb"):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            base = repository(root)
            bad = "tools/" + prefix + "/" + "evidence-" + "seq-synthetic.json"
            put(root, bad)
            head = commit(root)
            for status, output in audit(root, base, head):
                require(status == 1 and repr(bad) in output, "name-not-preserved",
                        "whole path was not preserved in audit diagnostic")
            status, output = current(root, private=True)
            require(status == 1 and repr(bad) in output, "name-not-preserved",
                    "current independence boundary lost path metadata")
            public_bad = "tools/" + prefix + "/Users/" + "synthetic-person/sample.bin"
            put(root, public_bad, b"\xff")
            commit(root)
            status, output = current(root)
            require(status == 1 and repr(public_bad) in output, "name-not-preserved",
                    "current public boundary lost path metadata")
    # Encodings already named by the policy remain recognizable in paths.
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        base = repository(root)
        bad = "tools/docs%2Fspecs%2F" + GOAL + ".txt"
        put(root, bad)
        head = commit(root)
        require(all(code == 1 for code, _ in audit(root, base, head)),
                "name-not-preserved", "existing encoded-reference policy was lost")


class CurrentPathChecks(unittest.TestCase):
    def test_path_only_binary_and_text(self):
        current_paths()


class HistoryPathChecks(unittest.TestCase):
    def test_transient_and_merge_paths(self):
        history_paths()


class NameChecks(unittest.TestCase):
    def test_metadata_names_are_not_line_records(self):
        special_names()


class PreservedBoundaries(unittest.TestCase):
    def test_merge_inheritance_outside_range_is_not_an_introduction(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository(root)
            put(root, "tools/" + "evidence-" + "seq-inherited.json")
            ancestor = commit(root)
            git(root, "checkout", "--quiet", "-b", "side")
            put(root, "tools/side.txt")
            commit(root)
            git(root, "checkout", "--quiet", "main")
            put(root, "tools/main.txt")
            commit(root)
            git(root, "merge", "--quiet", "--no-ff", "side", "-m", "synthetic merge")
            self.assertTrue(all(code == 0 for code, _ in audit(root, ancestor, git(root, "rev-parse", "HEAD"))))

    def test_empty_range_and_inherited_path_are_not_new(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            repository(root)
            put(root, "tools/" + "evidence-" + "before-synthetic.json")
            base = commit(root)
            put(root, "tools/new.txt")
            head = commit(root)
            self.assertTrue(all(code == 0 for code, _ in audit(root, base, head)))
            self.assertTrue(all(code == 0 for code, _ in audit(root, head, head)))

    def test_missing_history_is_unverified(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            head = repository(root)
            self.assertTrue(all(code == 2 for code, _ in audit(root, "a" * 40, head)))

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
