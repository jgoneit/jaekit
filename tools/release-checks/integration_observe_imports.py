"""Benign checkout shadows must never execute in the original observer.

The interpreter, shell, collector and Core process are real local executions.
Shadow payloads only write a marker and raise; no report or history is forged.
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

ROOT = Path(__file__).resolve().parents[2]
SOURCE = ROOT / "tools/release-checks"


class ObserverImports(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory(prefix="jaekit-observer-import-core-")
        cls.addClassCleanup(cls.build.cleanup)
        cls.core = Path(cls.build.name).resolve() / "ha"
        subprocess.run(["go", "build", "-mod=readonly", "-ldflags=-X main.version=0.1.3",
                        "-o", str(cls.core), "./cmd/ha"], cwd=ROOT, check=True,
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120)
        cls.shell = os.environ.get("JAEKIT_TEST_BASH") or shutil.which("bash")
        if not cls.shell:
            raise RuntimeError("required local shell unavailable: bash")

    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="jaekit-observer-imports-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()

    def checkout(self, name):
        root = self.root / name
        directory = root / "tools/release-checks"
        directory.mkdir(parents=True)
        for filename in ("observe.py", "shell_reader.py", "execution_provenance.py"):
            shutil.copyfile(SOURCE / filename, directory / filename)
        (root / ".gitignore").write_text("tools/release-checks/\n")
        self.git(root, "init", "-q")
        self.git(root, "add", "-f", ".")
        self.git(root, "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid",
                 "-c", "commit.gpgsign=false", "commit", "-qm", "synthetic observer sources")
        return root

    def git(self, root, *args):
        env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
        env.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1")
        return subprocess.check_output(["git", "-c", "core.hooksPath=/dev/null",
                                        "-c", "gc.auto=0", *args], cwd=root, env=env,
                                       stderr=subprocess.PIPE, timeout=30)

    def shadow(self, root, relative, marker, *, bytecode=False):
        path = root / "tools/release-checks" / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        payload = f"with open({str(marker)!r}, 'w') as stream: stream.write('shadow executed')\n"
        payload += "raise RuntimeError('synthetic observer shadow')\n"
        if bytecode:
            source = self.root / "payload.py"
            source.write_text(payload)
            py_compile.compile(str(source), cfile=str(path), dfile=str(path.with_suffix(".py")),
                               doraise=True, invalidation_mode=py_compile.PycInvalidationMode.UNCHECKED_HASH)
            source.unlink()
        else:
            path.write_text(payload)
        self.assertEqual(self.git(root, "check-ignore", str(path)).strip(), os.fsencode(path))
        return path

    def run_python(self, root, arguments):
        return subprocess.run([sys.executable, *arguments], cwd=root, capture_output=True, timeout=60,
                              env=dict(os.environ, PYTHONDONTWRITEBYTECODE="1",
                                       LANG="en_US.UTF-8", LANGUAGE="en_US.UTF-8"))

    def observe(self, root, output):
        argv = [str(root / "tools/release-checks/observe.py"), "run", "--core", str(self.core),
                "--shell", self.shell, "--output", str(output), "--invocation", "synthetic-original-observation",
                "--", '"$JAEKIT_CORE" --version']
        return self.run_python(root, argv)

    def test_module_package_and_bytecode_shadows_never_run(self):
        candidates = (
            ("uuid.py", False), ("uuid/__init__.py", False), ("uuid.pyc", True),
            ("json.py", False), ("json/__init__.py", False), ("json.pyc", True),
            ("shell_reader/__init__.py", False),
            (importlib.util.cache_from_source("shell_reader.py"), True),
            ("execution_provenance/__init__.py", False),
            (importlib.util.cache_from_source("execution_provenance.py"), True),
        )
        for number, (relative, bytecode) in enumerate(candidates):
            with self.subTest(candidate=relative):
                root = self.checkout("shadow-" + str(number))
                marker = self.root / ("imported-" + str(number))
                shadow = self.shadow(root, relative, marker, bytecode=bytecode)
                output = self.root / ("observation-" + str(number) + ".json")
                result = self.observe(root, output)
                if marker.exists():
                    self.fail("[violation:unverified-import] a checkout shadow executed in the original observer or launcher child")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout, b"ha 0.1.3\n")
                report = json.loads(output.read_bytes())
                self.assertEqual([event["kind"] for event in report["events"]],
                                 ["shell_start", "core_start", "core_exit", "shell_exit"])
                self.assertEqual(report["events"][2]["returncode"], 0)
                self.assertTrue(report["identity_unchanged"])
                self.assertTrue(shadow.is_file(), "observation removed an unrelated checkout file")

    def test_command_line_lazy_imports_never_load_checkout_modules(self):
        for number, module in enumerate(("locale.py", "textwrap.py", "_colorize.py")):
            with self.subTest(module=module):
                root = self.checkout("cli-" + str(number))
                marker = self.root / ("cli-imported-" + str(number))
                self.shadow(root, module, marker)
                result = self.run_python(root, [str(root / "tools/release-checks/observe.py"), "--help"])
                if marker.exists():
                    self.fail("[violation:unverified-import] a checkout module executed during observer command-line parsing")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn(b"--invocation", result.stdout)

    def test_import_and_embedded_entry_restore_caller_paths(self):
        root = self.checkout("embedded")
        # Importing from an embedding caller must not permanently rewrite its
        # Python search path or replace its already-loaded policy modules.
        source = r'''
import contextlib, importlib.util, io, json, sys
from pathlib import Path
directory = Path(sys.argv[1]) / "tools/release-checks"
sys.path.insert(0, str(directory))
import shell_reader, execution_provenance
before = sys.path[:]
policies = (sys.modules["shell_reader"], sys.modules["execution_provenance"])
spec = importlib.util.spec_from_file_location("embedded_observer", directory / "observe.py")
observer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(observer)
import_restored = before == sys.path
sys.argv = [str(directory / "observe.py"), "--help"]
with contextlib.redirect_stdout(io.StringIO()):
    try:
        observer.main()
    except SystemExit as error:
        if error.code != 0:
            raise
print(json.dumps({"import_restored": import_restored, "main_restored": before == sys.path,
                  "policies_preserved": policies == (sys.modules["shell_reader"], sys.modules["execution_provenance"])}))
'''
        result = self.run_python(root, ["-I", "-c", source, str(root)])
        self.assertEqual(result.returncode, 0, result.stderr)
        report = json.loads(result.stdout)
        self.assertEqual(report, {"import_restored": True, "main_restored": True, "policies_preserved": True})


if __name__ == "__main__":
    unittest.main()
