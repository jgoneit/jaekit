"""Real selected reader, pinned worker and Python children in synthetic Git trees.

Public release identity is supplied locally; no network or release service is
used. Source verification, snapshot materialization and child execution are real.
"""
import importlib.util
import json
import os
from pathlib import Path
import py_compile
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("child_import_checker", Path(__file__).with_name("check.py"))
check = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(check)

SELECTED = r'''
import sys
from pathlib import Path
root = Path(sys.argv[1])
check = type(sys)("check")
check.__file__ = str(root / "tools/release-checks/check.py")
sys.modules["check"] = check
exec(compile(Path(check.__file__).read_bytes(), check.__file__, "exec"), check.__dict__)
import json
commit, tree = sys.argv[2:4]
check.api = lambda path: {"sha": commit, "commit": {"tree": {"sha": tree}}}
checker = check.Checker(root / "private-evidence")
checker._evidence = {"release": {"source_commit": commit},
                     "checker_source": {"commit": commit, "tree": tree}}
checker.release = lambda: {}
try:
    checker.selected("AC-10")
except (check.Failure, check.Unavailable) as error:
    result = {"status": "rejected", "reason": str(error)}
else:
    result = {"status": "accepted"}
print(json.dumps(result))
'''


class ChildImports(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="jaekit-child-imports-")
        self.addCleanup(temporary.cleanup)
        self.base = Path(temporary.name).resolve()
        self.environment = {key: value for key, value in os.environ.items()
                            if not key.startswith("GIT_") and key != "PYTHONPATH"}
        self.environment.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
                                PYTHONDONTWRITEBYTECODE="1")

    def git(self, root, *arguments):
        return subprocess.check_output(["git", "-c", "core.hooksPath=/dev/null", "-c", "gc.auto=0", *arguments],
                                       cwd=root, env=self.environment, stderr=subprocess.PIPE, timeout=30)

    def checkout(self, name, passing=False):
        root = self.base / name
        root.mkdir()
        names = {Path(path).relative_to(ROOT) for path in check.LOADED_SOURCE}
        names.add(Path("tools/release-checks/snapshot_worker.py"))
        for relative in names:
            target = root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / relative, target)
        for number, directory in enumerate(("tools/release", "tools/release-checks")):
            suite = root / directory
            (suite / "synthetic_helper.py").write_text("VALUE = 42\n")
            marker = self.base / (name + "-suite-" + str(number))
            (suite / "test_synthetic.py").write_text(
                "import unittest\nfrom pathlib import Path\nimport synthetic_helper\n"
                "class Required(unittest.TestCase):\n"
                "    def test_requirement(self):\n"
                f"        Path({str(marker)!r}).write_text('tracked test executed')\n"
                f"        self.assertEqual(synthetic_helper.VALUE, {42 if passing else 0})\n")
        (root / ".gitignore").write_text("docs/\n")
        self.git(root, "init", "-q")
        self.git(root, "add", "-f", ".")
        self.git(root, "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid",
                 "-c", "commit.gpgsign=false", "commit", "-qm", "synthetic selected sources")
        return root

    def shadow(self, root, kind):
        directory = root / "docs"
        directory.mkdir()
        marker = self.base / (root.name + "-untracked-import")
        source = (f"with open({str(marker)!r}, 'a') as stream: stream.write('untracked import executed\\n')\n"
                  "import sys\nsys.stderr.write('Ran 1 test in 0.001s\\n\\nOK\\n')\n")
        if kind == "module":
            path = directory / "unittest.py"
            path.write_text(source)
        elif kind == "package":
            package = directory / "unittest"
            package.mkdir()
            (package / "__init__.py").write_text("")
            path = package / "__main__.py"
            path.write_text(source)
        else:
            payload = self.base / (root.name + "-payload.py")
            payload.write_text(source)
            path = directory / "unittest.pyc"
            py_compile.compile(str(payload), cfile=str(path), dfile=str(directory / "unittest.py"),
                               doraise=True, invalidation_mode=py_compile.PycInvalidationMode.UNCHECKED_HASH)
            payload.unlink()
        self.assertEqual(self.git(root, "check-ignore", str(path)).strip(), os.fsencode(path))
        return path, marker

    def selected(self, root, pythonpath=None):
        commit = self.git(root, "rev-parse", "HEAD").decode().strip()
        tree = self.git(root, "rev-parse", "HEAD^{tree}").decode().strip()
        env = dict(self.environment)
        if pythonpath is not None:
            env["PYTHONPATH"] = str(pythonpath)
        result = subprocess.run([sys.executable, "-I", "-c", SELECTED, str(root), commit, tree],
                                cwd=root, env=env, capture_output=True, text=True, timeout=60)
        self.assertEqual(result.returncode, 0, result.stderr)
        return json.loads(result.stdout)

    def test_ignored_pythonpath_cannot_replace_pinned_failure_regressions(self):
        for kind in ("module", "package", "bytecode"):
            with self.subTest(kind=kind):
                root = self.checkout(kind)
                shadow, marker = self.shadow(root, kind)
                before = ((root / ".git/index").read_bytes(), shadow.read_bytes())
                self.assertEqual(self.selected(root)["status"], "rejected")
                result = self.selected(root, root / "docs")
                if result["status"] != "rejected" or marker.exists():
                    self.fail("[violation:unverified-import] an original checkout import replaced pinned failure regressions")
                self.assertEqual(self.git(root, "status", "--porcelain"), b"")
                self.assertEqual(((root / ".git/index").read_bytes(), shadow.read_bytes()), before)

    def test_pinned_tracked_sibling_tests_still_execute(self):
        root = self.checkout("passing", passing=True)
        shadow, marker = self.shadow(root, "module")
        result = self.selected(root, shadow.parent)
        if marker.exists():
            self.fail("[violation:unverified-import] an ignored import supplied the successful regression count")
        self.assertEqual(result["status"], "accepted")
        for number in range(2):
            self.assertEqual((self.base / ("passing-suite-" + str(number))).read_text(), "tracked test executed")
        self.assertTrue(shadow.is_file())

    def test_script_children_keep_sibling_imports_without_checkout_pythonpath(self):
        selected, original = self.base / "selected", self.base / "original"
        selected.mkdir()
        original.mkdir()
        marker = self.base / "script-shadow-imported"
        (original / "json.py").write_text(
            f"with open({str(marker)!r}, 'w') as stream: stream.write('untracked import executed')\n"
            "raise RuntimeError('synthetic checkout import')\n")
        (selected / "sibling.py").write_text("VALUE = 42\n")
        script = selected / "check_synthetic.py"
        script.write_text("import json\nimport sibling\nprint(json.dumps({'value': sibling.VALUE}))\n")
        with mock.patch.dict(os.environ, {"PYTHONPATH": str(original)}):
            result = check.command([sys.executable, str(script)], cwd=selected)
            self.assertEqual(os.environ["PYTHONPATH"], str(original))
        if marker.exists():
            self.fail("[violation:unverified-import] a script check loaded an original checkout module")
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout), {"value": 42})


if __name__ == "__main__":
    unittest.main()
