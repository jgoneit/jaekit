"""Synthetic Git graph cases for the explicit read-only history audit."""
import importlib.util
import os
from pathlib import Path
import subprocess
import tempfile
import unittest


spec = importlib.util.spec_from_file_location(
    "history_audit", Path(__file__).parent / "release-0-1-3-followups/preserved.py")
audit = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit)


class HistoryCompleteness(unittest.TestCase):
    def test_merge_with_a_missing_side_ancestor_is_unverified(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-history-graph-") as directory:
            full = Path(directory) / "full"
            full.mkdir()
            env = dict(os.environ, GIT_AUTHOR_NAME="Fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
                       GIT_COMMITTER_NAME="Fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid")

            def git(root, *args):
                return subprocess.check_output(["git", *args], cwd=root, env=env, text=True,
                                               stderr=subprocess.DEVNULL).strip()

            git(full, "init", "--quiet", "-b", "main")
            git(full, "commit", "--quiet", "--allow-empty", "-m", "root")
            git(full, "checkout", "--quiet", "-b", "side")
            git(full, "commit", "--quiet", "--allow-empty", "-m", "side ancestor")
            git(full, "commit", "--quiet", "--allow-empty", "-m", "side tip")
            git(full, "checkout", "--quiet", "main")
            git(full, "commit", "--quiet", "--allow-empty", "-m", "base")
            base = git(full, "rev-parse", "HEAD")
            git(full, "merge", "--quiet", "--no-ff", "side", "-m", "merge")
            head = git(full, "rev-parse", "HEAD")
            shallow = Path(directory) / "shallow"
            git(Path(directory), "clone", "--quiet", "--depth", "2", full.as_uri(), str(shallow))
            git(shallow, "merge-base", "--is-ancestor", base, head)
            previous = Path.cwd()
            try:
                os.chdir(full)
                problems = []
                audit.history_boundary(base, head, problems)
                self.assertEqual(problems, [])
                os.chdir(shallow)
                with self.assertRaisesRegex(audit.Unavailable, "shallow boundary"):
                    audit.history_boundary(base, head, [])
                # An empty named range needs no hidden ancestors.
                audit.history_boundary(head, head, [])
            finally:
                os.chdir(previous)


if __name__ == "__main__":
    unittest.main()
