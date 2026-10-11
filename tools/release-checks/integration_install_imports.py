"""Real direct collector imports and receipts in isolated synthetic checkouts.

Only the archive download is replaced by local archive bytes. The interpreter,
collector, installed Core, command captures and installation consumer are real.
"""
import importlib.util
import json
import os
from pathlib import Path
import py_compile
import shutil
import subprocess
import sys
import unittest
from unittest import mock

import integration_direct_install as fixtures

check = fixtures.check
install_trace = fixtures.install_trace


class CollectorImports(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        fixtures.DirectInstallation.setUpClass()

    @classmethod
    def tearDownClass(cls):
        fixtures.DirectInstallation.tearDownClass()

    def setUp(self):
        self.case = fixtures.DirectInstallation()
        self.case.setUp()
        self.addCleanup(self.case.doCleanups)

    def checkout(self, name):
        root = self.case.root / name
        for original in check.LOADED_SOURCE:
            relative = Path(original).relative_to(fixtures.ROOT)
            target = root / relative
            target.parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(original, target)
        (root / ".gitignore").write_text("tools/release-checks/\ntools/release/\n")
        self.git(root, "init", "-q")
        self.git(root, "add", "-f", ".")
        self.git(root, "-c", "user.name=Synthetic", "-c", "user.email=synthetic@example.invalid",
                 "-c", "commit.gpgsign=false", "commit", "-qm", "synthetic collector sources")
        return root

    def git(self, root, *args):
        env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
        env.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1")
        return subprocess.check_output(["git", "-c", "core.hooksPath=/dev/null",
                                        "-c", "gc.auto=0", *args], cwd=root, env=env,
                                       stderr=subprocess.PIPE, timeout=30)

    def shadow(self, root, relative, source, *, bytecode=False):
        path = root / relative
        if bytecode:
            payload = self.case.root / "payload.py"
            payload.write_text(source)
            path.parent.mkdir(parents=True, exist_ok=True)
            py_compile.compile(str(payload), cfile=str(path), dfile=str(path.with_suffix(".py")),
                               doraise=True, invalidation_mode=py_compile.PycInvalidationMode.UNCHECKED_HASH)
            payload.unlink()
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(source)
        return path

    def run_install(self, root):
        # A normal trusted interpreter starts a new process, without -I. The
        # product entry point itself must exclude the checkout import paths.
        with mock.patch.dict(os.environ, {"PYTHONDONTWRITEBYTECODE": "1"}):
            return self.case.run_install(root / "tools/release-checks/install_trace.py")

    def acceptance(self, value):
        case = self.case
        selected = (case.destination / "ha").resolve()
        for key, suffix in (("version", ["--version"]), ("capabilities", ["capabilities", "--format", "json"])):
            query = subprocess.run([str(selected), *suffix], capture_output=True, timeout=30)
            self.assertEqual(query.returncode, 0, query.stderr)
            out, err = case.root / (key + ".out"), case.root / (key + ".err")
            out.write_bytes(query.stdout)
            err.write_bytes(query.stderr)
            value[key] = {"argv": [str(selected), *suffix], "exit_code": query.returncode,
                          "stdout": case.artifact(out), "stderr": case.artifact(err)}
        case.checker._evidence = {"installations": {"direct": value}}
        with mock.patch.object(case.checker, "download", return_value=case.archive.read_bytes()):
            return case.checker.installation("direct")

    def test_removed_uuid_shadow_cannot_certify_a_preexisting_install(self):
        case = self.case
        # This source used to run inside the genuine collector, skip its fresh
        # destination check and turn installation into a no-op on existing bytes.
        payload = '''import sys
class Identifier:
    hex = "a" * 32
def uuid4():
    scope = sys._getframe(1).f_globals
    original_need = scope["need"]
    def need(condition, reason):
        if reason != "fresh installation destination already exists":
            original_need(condition, reason)
    def reuse(destination, files, created=None):
        if created:
            created()
        return [{"path": name, "sha256": scope["digest"](data)} for name, data in sorted(files.items())]
    scope["need"] = need
    scope["install_files"] = reuse
    return Identifier()
'''
        for kind, relative in (("module", "uuid.py"), ("package", "uuid/__init__.py"), ("bytecode", "uuid.pyc")):
            with self.subTest(kind=kind):
                root = self.checkout(kind)
                case.destination = case.root / (kind + "-existing-install")
                case.report = case.root / (kind + "-report.json")
                for name, data in case.files.items():
                    path = case.destination / name
                    path.parent.mkdir(parents=True, exist_ok=True)
                    path.write_bytes(data)
                    path.chmod(0o755 if name == "ha" else 0o644)
                before = {str(p): p.read_bytes() for p in case.destination.rglob("*") if p.is_file()}
                shadow = self.shadow(root, "tools/release-checks/" + relative, payload, bytecode=kind == "bytecode")
                self.assertEqual(self.git(root, "check-ignore", str(shadow)).strip(), os.fsencode(shadow))
                result, value = self.run_install(root)
                # The later source check sees a clean checkout: collection must
                # have kept the untracked import out before it could disappear.
                shadow.unlink()
                self.assertEqual(self.git(root, "status", "--porcelain"), b"")
                report = json.loads(case.report.read_text())
                try:
                    self.acceptance(value)
                except (check.Failure, check.Unavailable):
                    accepted = False
                else:
                    accepted = True
                if accepted or result.returncode == 0 or report["before_absent"] or report["completed"]:
                    self.fail("[violation:unverified-import] a removed checkout import certified a preexisting installation as fresh")
                self.assertIsNotNone(report["failure"])
                self.assertEqual({str(p): p.read_bytes() for p in case.destination.rglob("*") if p.is_file()}, before)
                # The failed original report also remains protected from reuse.
                original_report = case.report.read_bytes()
                retry, _ = self.run_install(root)
                self.assertNotEqual(retry.returncode, 0)
                self.assertEqual(case.report.read_bytes(), original_report)

    def test_ignored_stdlib_and_helper_candidates_cannot_execute(self):
        candidates = (
            ("tools/release-checks/json.py", False),
            ("tools/release-checks/locale.py", False),
            ("tools/release-checks/uuid/__init__.py", False),
            ("tools/release-checks/check/__init__.py", False),
            ("tools/release-checks/shell_reader/__init__.py", False),
            ("tools/release-checks/json.pyc", True),
            (str(Path("tools/release-checks") / Path(importlib.util.cache_from_source("check.py"))), True),
            (str(Path("tools/release-checks") / Path(importlib.util.cache_from_source("shell_reader.py"))), True),
            ("tools/release/archive_reader/__init__.py", False),
            (str(Path("tools/release") / Path(importlib.util.cache_from_source("archive_reader.py"))), True),
        )
        for number, (relative, bytecode) in enumerate(candidates):
            with self.subTest(candidate=relative):
                case = self.case
                root = self.checkout("candidate-" + str(number))
                marker = case.root / ("imported-" + str(number))
                payload = f"with open({str(marker)!r}, 'w') as stream: stream.write('shadow executed')\n"
                payload += "raise RuntimeError('synthetic checkout shadow')\n"
                shadow = self.shadow(root, relative, payload, bytecode=bytecode)
                self.assertEqual(self.git(root, "check-ignore", str(shadow)).strip(), os.fsencode(shadow))
                case.destination = case.root / ("fresh-" + str(number))
                case.report = case.root / ("fresh-" + str(number) + ".json")
                # argparse/gettext may import locale while constructing the
                # parser, before the installation function itself starts.
                with mock.patch.dict(os.environ, {"LANGUAGE": "en_US.UTF-8", "LANG": "en_US.UTF-8"}):
                    result, value = self.run_install(root)
                if marker.exists():
                    self.fail("[violation:unverified-import] an untracked module, package or bytecode executed in the installation collector")
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(case.consume(value), (case.destination / "ha").resolve())
                self.assertTrue(shadow.is_file(), "collection removed an unrelated checkout file")

    def test_command_line_lazy_imports_stay_inside_boundary(self):
        candidates = (
            ("check.py", ["AC-1"], "_colorize.py", 2),
            ("check.py", ["--help"], "textwrap.py", 0),
            ("install_trace.py", ["--help"], "textwrap.py", 0),
            ("install_trace.py", ["--help"], "locale.py", 0),
        )
        for number, (script, arguments, module, expected_exit) in enumerate(candidates):
            with self.subTest(script=script, module=module):
                root = self.checkout("cli-" + str(number))
                marker = self.case.root / ("cli-imported-" + str(number))
                payload = f"with open({str(marker)!r}, 'w') as stream: stream.write('shadow executed')\n"
                payload += "raise RuntimeError('synthetic command-line shadow')\n"
                shadow = self.shadow(root, "tools/release-checks/" + module, payload)
                result = subprocess.run([sys.executable, str(root / "tools/release-checks" / script), *arguments],
                                        capture_output=True, timeout=30, cwd=root,
                                        env=dict(os.environ, PYTHONDONTWRITEBYTECODE="1",
                                                 LANGUAGE="en_US.UTF-8", LANG="en_US.UTF-8"))
                if marker.exists():
                    self.fail("[violation:unverified-import] a checkout module executed while parsing original command-line arguments")
                self.assertEqual(result.returncode, expected_exit, result.stderr)
                self.assertTrue(shadow.is_file())

    def test_normal_collector_receipt_reaches_read_only_acceptance(self):
        case = self.case
        root = self.checkout("clean")
        result, value = self.run_install(root)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.acceptance(value), (case.destination / "ha").resolve())
        self.assertEqual(self.git(root, "status", "--porcelain"), b"")
        report = json.loads(case.report.read_text())
        self.assertTrue(report["before_absent"] and report["completed"] and report["identity_unchanged"])


if __name__ == "__main__":
    unittest.main()
