"""Current-file checks need no private goal material or ancestral commits.

Run the unittest cases from the repository root. A separate history audit is
available with --history --base SHA --head SHA; it never runs implicitly as
part of the current-file tests and never fetches missing history.
"""
from concurrent.futures import ThreadPoolExecutor
import importlib.util
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

ROOT = Path.cwd()
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


def git(*args, cwd=ROOT):
    return subprocess.run(["git", *args], cwd=cwd, check=True, capture_output=True, text=True,
                          timeout=300).stdout


def environment():
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
               GH_PROMPT_DISABLED="1")
    for name in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
        env.pop(name, None)
    return env


def tracked_texts():
    for name in git("ls-files", "-z").split("\0"):
        if name and (ROOT / name).is_file() and not (ROOT / name).is_symlink():
            yield name, (ROOT / name).read_bytes().decode("utf-8", errors="replace")


class TrackedFiles(unittest.TestCase):
    """AC-8: current tracked files contain no forbidden private references.

    Arbitrary hashes are not classified as private by their shape. Any one-off
    comparison with an earlier private file stays outside this public check.
    """

    def test_tracked_files_hold_no_private_goal_references(self):
        found = []
        for name, text in tracked_texts():
            found += [f"{name}: {m.group(0)}" for pattern in PRIVATE_REFERENCES for m in pattern.finditer(text)]
        self.assertEqual(found, [])


class FreshClone(unittest.TestCase):
    """AC-8: current-file checks succeed in full and depth-1 clones."""

    CHECKS = [
        ("Go tests", ["go", "test", "-count=1", "./..."]),
        ("public documentation and checker regressions", [sys.executable, "tools/verify.py", "docs"]),
        ("goal check regressions", [sys.executable, "-m", "unittest", "discover", "-s", "tools/goal-checks",
                                    "-p", "test_*.py"]),
        ("current private-reference check", [sys.executable,
                                             f"tools/goal-checks/{GOAL}-review-fixes/test_private_independence.py",
                                             "TrackedFiles"]),
    ]

    @staticmethod
    def status(argv, cwd):
        return subprocess.run(argv, cwd=cwd, env=environment(), capture_output=True, text=True, timeout=900)

    def test_fresh_clone_gives_the_same_judgments(self):
        head = git("rev-parse", "HEAD").strip()
        with tempfile.TemporaryDirectory(prefix="jaekit-fresh-clone-") as directory:
            clones = []
            for label, options in (("full", []), ("shallow", ["--depth", "1"])):
                clone = Path(directory) / label
                git("clone", "--quiet", "--no-checkout", *options, ROOT.as_uri(), str(clone), cwd=directory)
                git("checkout", "--quiet", "--detach", head, cwd=clone)
                self.assertFalse((clone / "docs").exists(), "a clone must not have local goal documents")
                self.assertFalse((clone / ".git" / "ha").exists(), "a clone must not have run records")
                if options:
                    self.assertEqual(git("rev-list", "--count", "HEAD", cwd=clone).strip(), "1")
                clones.append(clone)
            with ThreadPoolExecutor(3) as pool:
                for label, argv in self.CHECKS:
                    for cwd, result in zip((ROOT, *clones), pool.map(lambda cwd: self.status(argv, cwd), (ROOT, *clones))):
                        with self.subTest(check=label, repository=cwd.name):
                            self.assertEqual(result.returncode, 0,
                                             f"{label}: expected successful check in {cwd.name}: "
                                             + (result.stdout + result.stderr)[-3000:])


def history_main(argv):
    path = Path(__file__).resolve().parent.parent / (GOAL + "-followups") / "preserved.py"
    spec = importlib.util.spec_from_file_location("public_history_audit", path)
    audit = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(audit)
    return audit.main(argv)


if __name__ == "__main__":
    if "--history" in sys.argv:
        raise SystemExit(history_main(sys.argv))
    unittest.main()
