"""Actual Python import boundaries in synthetic Git checkouts; no service calls."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("import_provenance_checker", Path(__file__).with_name("check.py"))
check = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(check)

SNAPSHOT = r"""
import json, sys
from pathlib import Path
root = Path(sys.argv[1]).resolve()
sys.path.insert(0, str(root / "tools/release-checks"))
import check
tree = check.source_identity.git(root, "rev-parse", "HEAD^{tree}").decode().strip()
try:
    check.source_identity.snapshot(root, tree, check.LOADED_SOURCE)
except check.source_identity.SourceMismatch as error:
    result = {"accepted": False, "reason": str(error)}
else:
    result = {"accepted": True}
print(json.dumps(result))
"""


class Checkout(unittest.TestCase):
    def fixture(self, ignore=None):
        temporary = tempfile.TemporaryDirectory(prefix="jaekit-import-provenance-")
        self.addCleanup(temporary.cleanup)
        base = Path(temporary.name).resolve()
        root = base / "repo"
        root.mkdir()
        names = {Path(path).relative_to(ROOT) for path in check.LOADED_SOURCE}
        names.add(Path("tools/release-checks/snapshot_worker.py"))
        for name in names:
            target = root / name
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / name, target)
        if ignore is not None:
            (root / ".gitignore").write_text(ignore)
        self.git(root, "init", "-q")
        self.git(root, "add", "-f", ".")
        self.git(root, "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid",
                 "-c", "commit.gpgsign=false", "commit", "-qm", "synthetic checker sources")
        return root, base / "imported"

    def git(self, root, *args):
        return subprocess.check_output(["git", "-c", "core.hooksPath=/dev/null", "-c", "gc.auto=0", *args],
                                       cwd=root, stderr=subprocess.PIPE,
                                       env=dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1"))

    def shadow(self, root, marker, relative):
        path = root / "tools/release-checks" / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(f"with open({str(marker)!r}, 'w') as stream: stream.write('untracked import executed')\n"
                        "raise RuntimeError('synthetic untracked import')\n")
        (root / ".git/info/exclude").write_text("tools/release-checks/" + relative.split("/")[0] + "\n")
        return path

    def run_python(self, root, arguments, *, input=None):
        return subprocess.run([sys.executable, "-B", *arguments], cwd=root, input=input,
                              capture_output=True, text=True, timeout=30,
                              env=dict(os.environ, PYTHONDONTWRITEBYTECODE="1"))

    def snapshot(self, root):
        result = self.run_python(root, ["-I", "-c", SNAPSHOT, str(root)])
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)


class ImportProvenance(Checkout):
    def test_ignored_module_cannot_run_at_public_entry(self):
        for selection in (["--help"], ["AC-1"], ["AC-7"]):
            with self.subTest(selection=selection):
                root, marker = self.fixture()
                self.shadow(root, marker, "json.py")
                result = self.run_python(root, [str(root / "tools/release-checks/check.py"), *selection])
                if marker.exists():
                    self.fail("[violation:unverified-import] ignored module executed before the public source boundary")
                self.assertEqual(result.returncode, 0 if selection == ["--help"] else 2, result.stderr)

    def test_ignored_helper_package_cannot_replace_tracked_module(self):
        root, marker = self.fixture()
        self.shadow(root, marker, "shell_reader/__init__.py")
        result = self.run_python(root, [str(root / "tools/release-checks/check.py"), "--help"])
        if marker.exists():
            self.fail("[violation:unverified-import] an ignored package replaced the tracked helper module")
        self.assertEqual(result.returncode, 0, result.stderr)

    def test_ignored_package_cannot_run_at_worker_entry(self):
        root, marker = self.fixture()
        self.shadow(root, marker, "json/__init__.py")
        result = self.run_python(root, [str(root / "tools/release-checks/snapshot_worker.py")], input="{}")
        if marker.exists():
            self.fail("[violation:unverified-import] ignored package executed before the worker source boundary")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["status"], "unverified")

    def test_ignored_source_rejected_by_snapshot(self):
        for relative in ("shadow.py", "shadow_package/__init__.py"):
            with self.subTest(relative=relative):
                root, marker = self.fixture()
                self.shadow(root, marker, relative)
                report = self.snapshot(root)
                if report["accepted"]:
                    self.fail("[violation:unverified-import] ignored Python source was reported as a clean checker snapshot")
                self.assertIn("untracked observation source", report["reason"])
                self.assertFalse(marker.exists())


class ImportProvenanceKeep(Checkout):
    def test_clean_and_tracked_ignored_sources_are_preserved(self):
        root, _ = self.fixture("tools/release-checks/*.py\ndocs/\n")
        document = root / "docs/specs/local/notes.py"
        document.parent.mkdir(parents=True)
        document.write_text("unrelated private notes, not an import path\n")
        source = root / "tools/release-checks/check.py"
        before = (source.read_bytes(), (root / ".git/index").read_bytes(),
                  (root / ".gitignore").read_bytes(), document.read_bytes())
        self.assertTrue(self.snapshot(root)["accepted"])
        result = self.run_python(root, [str(source), "--help"])
        self.assertEqual(result.returncode, 0, result.stderr)
        after = (source.read_bytes(), (root / ".git/index").read_bytes(),
                 (root / ".gitignore").read_bytes(), document.read_bytes())
        self.assertEqual(after, before)


if __name__ == "__main__":
    unittest.main()
