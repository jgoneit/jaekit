"""Offline CLI regressions for the shared verification entrypoint."""
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


VERIFY = Path(__file__).resolve().parents[1] / "verify.py"


class Fixture(unittest.TestCase):
    def setUp(self):
        self.assertTrue(VERIFY.is_file(), "required verification entrypoint tools/verify.py is missing")
        temporary = tempfile.TemporaryDirectory(prefix="verify-fixture-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.repo = self.root / "checkout"
        self.bin = self.root / "bin"
        self.bin.mkdir()
        (self.repo / "tools").mkdir(parents=True)
        shutil.copyfile(VERIFY, self.repo / "tools/verify.py")
        self.log = self.root / "commands.jsonl"
        self.env = dict(os.environ, PATH=str(self.bin), VERIFY_TEST_LOG=str(self.log),
                        PYTHONDONTWRITEBYTECODE="1")
        # Each fake command records its invocation. No GitHub or network is used.
        recorder = (
            "import json, os, sys\n"
            "with open(os.environ['VERIFY_TEST_LOG'], 'a') as stream:\n"
            "    stream.write(json.dumps({'name': NAME, 'args': sys.argv[1:], "
            "'goos': os.environ.get('GOOS'), 'goarch': os.environ.get('GOARCH'), "
            "'cgo': os.environ.get('CGO_ENABLED'), 'toolchain': os.environ.get('GOTOOLCHAIN'), "
            "'no_bytecode': os.environ.get('PYTHONDONTWRITEBYTECODE')}) + '\\n')\n"
        )
        smoke = (f"#!{sys.executable}\nNAME = 'ha'\n" + recorder +
                 "if os.environ.get('VERIFY_TEST_FAIL') == 'smoke:' + sys.argv[1]: sys.exit(23)\n")
        for name in ("git", "gofmt", "go"):
            source = f"#!{sys.executable}\nNAME = {name!r}\n" + recorder
            source += (
                "failure = os.environ.get('VERIFY_TEST_FAIL', '')\n"
                "key = NAME\n"
                "if NAME == 'go':\n"
                "    key = sys.argv[1]\n"
                "    if key == 'build' and os.environ.get('GOOS'):\n"
                "        key += ':' + os.environ['GOOS'] + '/' + os.environ['GOARCH']\n"
                "if NAME == 'gofmt' and failure == 'format': print('bad.go')\n"
                "if NAME == 'gofmt' and '-w' in sys.argv:\n"
                "    from pathlib import Path\n"
                "    Path('code.go').write_text('unexpected automatic rewrite\\n')\n"
                "if failure == key: sys.exit(19)\n"
                "if NAME == 'go' and sys.argv[1] == 'build':\n"
                "    from pathlib import Path\n"
                "    target = Path(sys.argv[sys.argv.index('-o') + 1])\n"
                f"    target.write_text({smoke!r})\n"
                "    target.chmod(0o700)\n"
            )
            (self.bin / name).write_text(source)
            (self.bin / name).chmod(0o700)
        for name in ("bash", "zsh"):
            # Availability probes are separate from verification commands.
            (self.bin / name).write_text(f"#!{sys.executable}\nimport sys\nsys.exit(0)\n")
            (self.bin / name).chmod(0o700)
        for name in ("check_public_tree.py", "check_public_docs.py"):
            (self.repo / "tools" / name).write_text(
                f"NAME = {name!r}\n" + recorder +
                "if os.environ.get('VERIFY_TEST_FAIL') == NAME: sys.exit(29)\n"
            )
        for group in ("public-checks", "workflow-checks", "release", "goal-checks", "release-checks"):
            directory = self.repo / "tools" / group
            directory.mkdir()
            (directory / "test_fixture.py").write_text(
                f"NAME = {group!r}\n" + recorder +
                "import unittest\n"
                "class SyntheticCase(unittest.TestCase):\n"
                "    def test_result(self):\n"
                "        self.assertNotEqual(os.environ.get('VERIFY_TEST_FAIL'), NAME)\n"
            )
        for group, filenames in (
                ("review", ("test_observation.py", "test_installation.py", "test_producers.py")),
                ("gap", ("test_observation.py", "test_history_results.py", "test_docs_environment.py"))):
            directory = self.repo / "tools" / (group + "-checks")
            directory.mkdir()
            for filename in filenames:
                label = group + ":" + filename
                (directory / filename).write_text(
                    f"NAME = {label!r}\n" + recorder +
                    "import unittest\n"
                    "class SyntheticCase(unittest.TestCase):\n"
                    "    def test_result(self):\n"
                    "        self.assertNotEqual(os.environ.get('VERIFY_TEST_FAIL'), NAME)\n"
                )
        (self.repo / "docs/specs/synthetic").mkdir(parents=True)
        (self.repo / "docs/specs/synthetic/SPEC.md").write_text("Local synthetic goal\n")
        (self.repo / "code.go").write_text("package synthetic\n")

    def run_verify(self, *groups, **env):
        return subprocess.run([sys.executable, str(self.repo / "tools/verify.py"), *groups],
                              cwd=self.root, env=dict(self.env, **env), capture_output=True,
                              text=True, timeout=30)

    def commands(self):
        return [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []

    def snapshot(self):
        return {str(path.relative_to(self.repo)): (hashlib.sha256(path.read_bytes()).hexdigest(), path.stat().st_mode)
                for path in self.repo.rglob("*") if path.is_file()}


class InvocationCases(Fixture):
    def test_default_runs_all_groups_and_four_targets(self):
        result = self.run_verify()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        commands = self.commands()
        self.assertEqual([item["name"] for item in commands],
                         ["gofmt", "go", "go", "review:test_observation.py", "review:test_installation.py",
                          "review:test_producers.py", "gap:test_observation.py", "gap:test_history_results.py",
                          "gap:test_docs_environment.py", "go", "go", "go", "go", "go", "ha", "ha",
                          "check_public_tree.py", "check_public_docs.py", "public-checks", "workflow-checks", "release", "goal-checks", "release-checks"])
        builds = [item for item in commands if item["name"] == "go" and item["args"][0] == "build"]
        self.assertEqual([(item["goos"], item["goarch"]) for item in builds],
                         [("linux", "amd64"), ("linux", "arm64"), ("darwin", "amd64"), ("darwin", "arm64"), (None, None)])
        self.assertTrue(all(item["cgo"] == "0" for item in builds[:4]))
        for item in commands:
            self.assertEqual(item["no_bytecode"], "1")
            if item["name"] == "go":
                self.assertEqual(item["toolchain"], "local")
                self.assertIn("-mod=readonly", item["args"])
        self.assertIn("PASS: all verification", result.stdout)

    def test_group_selection_does_not_require_unrelated_tools(self):
        (self.bin / "go").unlink()
        (self.bin / "gofmt").unlink()
        (self.bin / "zsh").unlink()
        result = self.run_verify("docs")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([item["name"] for item in self.commands()],
                         ["check_public_tree.py", "check_public_docs.py", "public-checks", "workflow-checks", "release", "goal-checks", "release-checks"])

    def test_go_group_uses_native_smoke_despite_cross_environment(self):
        result = self.run_verify("go", GOOS="other", GOARCH="other", GOTOOLCHAIN="auto")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([item["name"] for item in self.commands()][-2:], ["ha", "ha"])
        self.assertFalse(any("checks" in item["name"] for item in self.commands()))


class FailureCases(Fixture):
    def test_nonempty_format_output_is_failure_without_rewriting(self):
        before = self.snapshot()
        result = self.run_verify(VERIFY_TEST_FAIL="format")
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("FAIL: Go formatting", result.stdout + result.stderr)
        self.assertIn("bad.go", result.stdout + result.stderr)
        self.assertEqual([item["name"] for item in self.commands()], ["gofmt"])
        self.assertEqual(self.snapshot(), before)

    def test_required_command_failures_are_identified_and_stop_execution(self):
        cases = (("gofmt", "Go formatting"), ("vet", "Go vet"), ("test", "Go tests"),
                 ("review:test_observation.py", "Review boundaries (test_observation.py)"),
                 ("review:test_installation.py", "Review boundaries (test_installation.py)"),
                 ("review:test_producers.py", "Review boundaries (test_producers.py)"),
                 ("gap:test_observation.py", "Review gap regressions (test_observation.py)"),
                 ("gap:test_history_results.py", "Review gap regressions (test_history_results.py)"),
                 ("gap:test_docs_environment.py", "Review gap regressions (test_docs_environment.py)"),
                 ("build:darwin/arm64", "Build darwin/arm64"), ("build", "Build native smoke binary"),
                 ("smoke:--version", "CLI version"), ("smoke:--help", "CLI help"),
                 ("check_public_tree.py", "Public file boundary"),
                 ("check_public_docs.py", "Public documentation"),
                 ("public-checks", "Public checker regressions"),
                 ("workflow-checks", "Workflow regressions"),
                 ("release", "Release archive regressions"),
                 ("goal-checks", "Goal check regressions"),
                 ("release-checks", "Release acceptance checker regressions"))
        for failure, label in cases:
            with self.subTest(failure=failure):
                self.log.unlink(missing_ok=True)
                result = self.run_verify(VERIFY_TEST_FAIL=failure)
                output = result.stdout + result.stderr
                self.assertNotEqual(result.returncode, 0, output)
                self.assertIn(f"FAIL: {label}", output)
                self.assertIn("exit ", output)
                self.assertNotIn("PASS: all verification", output)
                self.assertNotIn(f"PASS: {label}", output)
                last = self.commands()[-1]
                expected = "ha" if failure.startswith("smoke:") else "go" if failure.split(":")[0] in {"vet", "test", "build"} else failure
                self.assertEqual(last["name"], expected, "checks continued after a failure")
                if expected == "go":
                    self.assertEqual(last["args"][0], failure.split(":")[0])
                elif expected == "ha":
                    self.assertEqual(last["args"], [failure.removeprefix("smoke:")])

    def test_missing_tools_fail_before_any_check(self):
        for tool in ("go", "gofmt", "git", "bash", "zsh"):
            with self.subTest(tool=tool):
                path = self.bin / tool
                path.rename(self.bin / (tool + "-hidden"))
                result = self.run_verify()
                self.assertNotEqual(result.returncode, 0)
                self.assertIn(f"missing required tool: {tool}", result.stdout + result.stderr)
                self.assertEqual(self.commands(), [])
                (self.bin / (tool + "-hidden")).rename(path)

    def test_invalid_group_is_not_treated_as_success(self):
        result = self.run_verify("unknown")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.commands(), [])


class NonMutationCases(Fixture):
    def test_success_and_failure_preserve_checkout_and_remove_build_artifacts(self):
        # Synthetic Git state is included in the byte-for-byte snapshot.
        (self.repo / ".git/refs/heads").mkdir(parents=True)
        (self.repo / ".git/index").write_bytes(b"synthetic index")
        (self.repo / ".git/refs/heads/main").write_text("synthetic ref\n")
        before = self.snapshot()
        for failure in ("", "smoke:--help"):
            with self.subTest(failure=failure):
                self.log.unlink(missing_ok=True)
                result = self.run_verify(VERIFY_TEST_FAIL=failure)
                self.assertEqual(result.returncode == 0, not failure, result.stdout + result.stderr)
                self.assertEqual(self.snapshot(), before)
                for item in self.commands():
                    self.assertNotEqual(item["name"], "git", "verification must not change Git state")
                    if item["name"] == "go" and item["args"][0] == "build":
                        binary = Path(item["args"][item["args"].index("-o") + 1])
                        self.assertFalse(binary.is_relative_to(self.repo))
                        self.assertFalse(binary.exists())
                self.assertEqual(list(self.repo.rglob("__pycache__")), [])


if __name__ == "__main__":
    unittest.main()
