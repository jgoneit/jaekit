"""Public release wording and verification prerequisites, with isolated fixtures.

Only temporary checkouts and command wrappers are written. The real HOME,
installed shells, product documents and Git state remain untouched.
"""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import sys
import unittest


ROOT = Path(__file__).resolve().parents[2]


def load(relative, name):
    spec = importlib.util.spec_from_file_location(name, ROOT / relative)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


installation = load("tools/review-checks/test_installation.py", "gap_installation")
verification = load("tools/workflow-checks/test_verify.py", "gap_verification")


class AC7(unittest.TestCase):
    def test_colon_claims_report_stale_visible_versions_and_statement_lines(self):
        claims = (
            ("README.en.md", "The current release: v0.1.2.\n"),
            ("README.md", "현재 배포판: v0.1.2입니다.\n"),
            ("guides/USAGE.md", "현재 릴리스:\n[__v0.1.2__](https://example.invalid/release)입니다.\n"),
            ("examples/README.md", "The current release:\n**[`v0.1.2`](https://example.invalid/release)**.\n"),
        )
        for path, claim in claims:
            with self.subTest(path=path, claim=claim):
                installation.AC12.check_claim(self, path, claim, True)

    def test_colon_keeps_current_versions_url_only_history_and_paragraph_boundaries(self):
        claims = (
            "The current release: [__v0.1.3__](https://example.invalid/v0.1.2).\n",
            "현재 배포판:\n[`v0.1.3`](https://example.invalid/v0.1.2)입니다.\n",
            "The current release: [download](https://example.invalid/v0.1.2).\n",
            "The current release:\n\n[v0.1.2](https://example.invalid/history).\n",
            "현재 릴리스:\r\n\r\n[v0.1.2](https://example.invalid/history).\r\n",
        )
        for claim in claims:
            with self.subTest(claim=claim):
                installation.AC12.check_claim(self, "README.md", claim, False)


class AC8(verification.Fixture):
    def setUp(self):
        super().setUp()
        bash = shutil.which("bash")
        if bash is None:
            raise RuntimeError("the isolated shell-launch fixture requires bash")
        # Both names delegate to an existing shell. This checks executable
        # selection, not bash/zsh semantic equivalence (covered elsewhere).
        for name in ("bash", "zsh"):
            source = (f"#!{sys.executable}\nimport json, os, sys\n"
                      "with open(os.environ['VERIFY_TEST_LOG'], 'a') as stream:\n"
                      f"    stream.write(json.dumps({{'name': {name!r}, 'args': sys.argv[1:]}}) + '\\n')\n"
                      f"os.execv({bash!r}, [{bash!r}, *sys.argv[1:]])\n")
            path = self.bin / name
            path.write_text(source)
            path.chmod(0o700)

    def test_required_shells_fail_before_checks_when_missing_or_not_executable(self):
        for group in ((), ("go",)):
            for shell in ("bash", "zsh"):
                for fault in ("missing", "permission", "interpreter"):
                    with self.subTest(group=group or "all", shell=shell, fault=fault):
                        path = self.bin / shell
                        saved = path.read_bytes()
                        self.log.unlink(missing_ok=True)
                        if fault == "missing":
                            path.unlink()
                        elif fault == "permission":
                            path.chmod(0o600)
                        else:
                            path.write_text("#!/nonexistent-synthetic-interpreter\n")
                        before = self.snapshot()
                        try:
                            result = self.run_verify(*group)
                            output = result.stdout + result.stderr
                            self.assertNotEqual(result.returncode, 0,
                                                "[requirement] a missing/unusable required shell must refuse verification")
                            self.assertIn(shell, output, "[requirement] name the unavailable shell")
                            self.assertIn("FAIL:", output, "[requirement] report a preflight failure")
                            self.assertNotIn("RUN:", output,
                                             "[requirement] prerequisites must be checked before verification starts")
                            self.assertFalse(any(c["name"] not in ("bash", "zsh") for c in self.commands()),
                                             "[requirement] no checks may run after failed shell preflight")
                            self.assertEqual(self.snapshot(), before,
                                             "[requirement] preflight must not rewrite the checkout")
                        finally:
                            path.write_bytes(saved)
                            path.chmod(0o700)

    def test_actual_review_launches_use_the_shell_paths_available_to_preflight(self):
        probe = '''import importlib.util, tempfile, unittest
from pathlib import Path
def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module
class LaunchProbe(unittest.TestCase):
    def test_installation(self):
        module = load(INSTALLATION, 'selected_installation')
        with tempfile.TemporaryDirectory() as directory:
            for shell in module.SHELLS:
                result = module.run_cleanup('printf installation-fixture', Path(directory), shell)
                self.assertEqual(result.returncode, 0, result.stderr)
    def test_observation(self):
        module = load(OBSERVATION, 'selected_observation')
        case = module.Case()
        case.setUp()
        try:
            for shell in ('bash', 'zsh'):
                case.collected('printf observation-fixture', shell)
        finally:
            case.doCleanups()
'''
        paths = ("INSTALLATION = " + repr(str(ROOT / "tools/review-checks/test_installation.py")) + "\n"
                 + "OBSERVATION = " + repr(str(ROOT / "tools/review-checks/test_observation.py")) + "\n")
        (self.repo / "tools/review-checks/test_installation.py").write_text(paths + probe)
        before = self.snapshot()
        result = self.run_verify("go")
        self.assertEqual(result.returncode, 0,
                         "[requirement] available PATH shells must run the actual review launch helpers: "
                         + result.stdout + result.stderr)
        calls = self.commands()
        for shell in ("bash", "zsh"):
            for marker in ("installation-fixture", "observation-fixture"):
                self.assertTrue(any(c["name"] == shell and any(marker in a for a in c["args"]) for c in calls),
                                f"[requirement] {marker} bypassed selected {shell} executable")
        self.assertEqual(self.snapshot(), before, "[requirement] successful preflight/checks preserve source")

    def test_docs_needs_no_go_zsh_or_docker_and_documents_the_required_shells(self):
        for name in ("go", "gofmt", "zsh"):
            (self.bin / name).unlink()
        result = self.run_verify("docs")
        self.assertEqual(result.returncode, 0,
                         "[requirement] docs-only validation must not gain Go, zsh or Docker requirements: "
                         + result.stdout + result.stderr)
        self.assertFalse(any(c["name"] in ("go", "gofmt", "zsh", "docker") for c in self.commands()))
        agents = (ROOT / "AGENTS.md").read_text().split("## Validation\n", 1)[1]
        for name in ("bash", "zsh"):
            self.assertIn(name, agents, f"[requirement] contributor prerequisites must name {name}")
        guide = (ROOT / "tools/review-checks/README.md").read_text()
        self.assertIn("does not require Docker", guide,
                      "[requirement] optional container requirements stay separate from ordinary verification")


if __name__ == "__main__":
    unittest.main()
