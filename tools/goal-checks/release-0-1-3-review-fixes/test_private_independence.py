"""Goal checks: tracked checks give the same judgments without private goal material.

Run from the repository root. The names of the private goals and their record
locations are assembled from parts, so this file does not contain them and
does not match itself.
"""
from concurrent.futures import ThreadPoolExecutor
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

ROOT = Path.cwd()
# The commit the goal started from; its tracked preservation config held the
# private digests that must not remain in tracked files.
BASE = "12071916201f0241f84b014ea7a07c4480062cd9"
GOAL = "release-" + "0-1-3"
PRIVATE_REFERENCES = [
    re.compile(re.escape(prefix + GOAL)) for prefix in
    ("docs/specs/", "docs%2Fspecs%2F", "ha/", "worktrees/")
] + [
    re.compile(re.escape('"' + GOAL + '"')),
    re.compile("--git-" + "common-dir"),
    re.compile(r"(?:codex|claude)(?:-direct)?-(?:spec|seal|host)[\w.-]*\.(?:stdout|stderr|jsonl?)\b"),
    re.compile(r"\bevidence-(?:seq|final|before)[\w-]*\.json"),
]
NATIVE_IDENTIFIERS = [
    re.compile(r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b"),
    re.compile(r"\brollout-\d{4}-\d{2}-\d{2}T"),
    re.compile(re.escape("." + "claude/projects/")),
    re.compile(re.escape("." + "codex/sessions/")),
]


def git(*args, cwd=ROOT):
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True,
                          timeout=300).stdout


def environment():
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
               GH_PROMPT_DISABLED="1")
    for name in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
        env.pop(name, None)
    return env


def private_digests():
    config = json.loads(git("show", f"{BASE}:tools/goal-checks/{GOAL}-followups/preserved.json"))
    return set(config["local_goal"]["files"].values())


def tracked_texts():
    for name in git("ls-files", "-z").split("\0"):
        if name and (ROOT / name).is_file() and not (ROOT / name).is_symlink():
            yield name, (ROOT / name).read_bytes().decode("utf-8", errors="replace")


class TrackedFiles(unittest.TestCase):
    """AC-8: tracked files name no private goal path, digest or record."""

    def test_tracked_files_hold_no_private_goal_references(self):
        digests = private_digests()
        self.assertTrue(digests, "the base commit's private digests could not be read")
        found = []
        for name, text in tracked_texts():
            found += [f"{name}: {m.group(0)}" for pattern in PRIVATE_REFERENCES for m in pattern.finditer(text)]
            found += [f"{name}: private file digest" for value in digests if value in text]
        self.assertEqual(found, [])


class FreshClone(unittest.TestCase):
    """AC-8: every tracked check entry point judges a fresh clone the same way."""

    CHECKS = [
        ("Go tests", ["go", "test", "-count=1", "./..."]),
        ("public documentation and checker regressions", [sys.executable, "tools/verify.py", "docs"]),
        ("goal check regressions", [sys.executable, "-m", "unittest", "discover", "-s", "tools/goal-checks",
                                    "-p", "test_*.py"]),
        ("v0.1.3 preservation check", [sys.executable, f"tools/goal-checks/{GOAL}-followups/preserved.py", BASE]),
        ("release observation checker", [sys.executable, "tools/release-checks/check.py", "AC-1"]),
    ]

    @staticmethod
    def status(argv, cwd):
        result = subprocess.run(argv, cwd=cwd, env=environment(), stdout=subprocess.DEVNULL,
                                stderr=subprocess.DEVNULL, timeout=900)
        return result.returncode

    def test_fresh_clone_gives_the_same_judgments(self):
        head = git("rev-parse", "HEAD").strip()
        with tempfile.TemporaryDirectory(prefix="jaekit-fresh-clone-") as directory:
            clone = Path(directory) / "repo"
            git("clone", "--quiet", "--no-local", "--no-checkout", str(ROOT), str(clone), cwd=directory)
            git("checkout", "--quiet", "--detach", head, cwd=clone)
            self.assertFalse((clone / "docs").exists(), "a fresh clone must not have local goal documents")
            self.assertFalse((clone / ".git" / "ha").exists(), "a fresh clone must not have run records")
            with ThreadPoolExecutor(2) as pool:
                for label, argv in self.CHECKS:
                    here, there = pool.map(lambda cwd: self.status(argv, cwd), (ROOT, clone))
                    with self.subTest(check=label):
                        self.assertEqual(there, here, f"{label}: exit {here} here, {there} in a fresh clone")


class AddedLines(unittest.TestCase):
    """AC-11: changed public files and commit messages carry only synthetic data."""

    def test_added_lines_hold_no_native_identifiers(self):
        added = [line[1:] for line in git("diff", "--unified=0", "--no-color", BASE, "HEAD").splitlines()
                 if line.startswith("+") and not line.startswith("+++")]
        messages = git("log", "--format=%B", f"{BASE}..HEAD").splitlines()
        found = [f"{kind}: {line.strip()[:120]}" for kind, lines in (("added line", added), ("commit", messages))
                 for line in lines for pattern in NATIVE_IDENTIFIERS + PRIVATE_REFERENCES if pattern.search(line)]
        self.assertEqual(found, [])


if __name__ == "__main__":
    unittest.main()
