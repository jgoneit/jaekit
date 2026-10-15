"""Regression tests for the repository check boundaries, using synthetic modules.

The repository itself must pass; the synthetic violations must be reported by
file and line, and the normal cases must not be.
"""
import importlib.util
from pathlib import Path
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("repository_checks", ROOT / "tools/check_repository_checks.py")
guard = importlib.util.module_from_spec(spec)
spec.loader.exec_module(guard)

BOOTSTRAP = """\
import sys
_CALLER_IMPORT_PATH = sys.path[:]
sys.path[:] = (sys.prefix,)
import json
from contextlib import contextmanager
@contextmanager
def trusted_imports():
    yield
sys.path[:] = _CALLER_IMPORT_PATH
"""


class RepositoryChecks(unittest.TestCase):
    def scan(self, files):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            for name, body in files.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_text(textwrap.dedent(body))
            return guard.scan(root)

    def test_the_repository_passes(self):
        self.assertEqual([], guard.scan(ROOT))

    def test_fixed_commits_passed_to_git_are_reported(self):
        findings = self.scan({"tools/a.py": """\
            import subprocess
            BASE = "7476c87"
            def changed():
                return subprocess.run(["git", "diff", "--quiet", BASE, "HEAD"])
            def shown(git):
                return git("show", "0123abcd4567:README.md")
            def ranged(run):
                return run(["git", "log", "7476c87..HEAD"])
            def nested(run, path):
                return run(["git", "show", f"{BASE}:{path}"])
            """})
        self.assertEqual(["tools/a.py:4", "tools/a.py:6", "tools/a.py:8", "tools/a.py:10"],
                         [finding.split(": ", 1)[0] for finding in findings])

    def test_hex_values_outside_git_and_current_revisions_are_normal(self):
        self.assertEqual([], self.scan({"tools/b.py": """\
            import subprocess
            DIGEST = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
            OID = "0123456789abcdef0123456789abcdef01234567"
            def current(history):
                print(OID, DIGEST)
                history("add", "commit:" + OID)
                return subprocess.run(["git", "rev-parse", "HEAD"])
            """}))

    def test_late_imports_after_the_bootstrap_are_reported(self):
        findings = self.scan({"tools/c.py": BOOTSTRAP + textwrap.dedent("""\
            import zipfile
            def late():
                import csv
                from email import parser
                return csv, parser
            def cached():
                import json
                return json
            def guarded():
                with trusted_imports():
                    import gzip
                return gzip
            @trusted_imports()
            def decorated():
                import bz2
                return bz2
            """)})
        self.assertEqual(["tools/c.py:10", "tools/c.py:12", "tools/c.py:13"],
                         [finding.split(": ", 1)[0] for finding in findings])

    def test_modules_without_a_bootstrap_may_import_late(self):
        self.assertEqual([], self.scan({"tools/d.py": "def late():\n    import zipfile\n    return zipfile\n"}))

    def test_unreadable_modules_are_errors_not_findings(self):
        with self.assertRaises(guard.Unreadable):
            self.scan({"tools/e.py": "def broken(:\n"})


if __name__ == "__main__":
    unittest.main()
