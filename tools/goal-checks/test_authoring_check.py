"""History independence and result meaning of the Seal check-authoring producer."""
import json
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path
from unittest import mock

sys.path.insert(0, str(Path(__file__).resolve().parents[1] / "contract-checks"))
import authoring_check  # noqa: E402

checker = authoring_check.checker
FILE_CRITERIA = ("AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6", "AC-8", "AC-10")
INPUTS = sorted({name for name, _ in authoring_check.SECTIONS.values()} | {
    "internal/status/status.go", "cmd/ha/baseline.go", ".claude-plugin/marketplace.json",
    "plugins/seal/.claude-plugin/plugin.json", "plugins/seal/.codex-plugin/plugin.json"})


def copy(paths, root):
    for path in paths:
        destination = root / path
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(checker.ROOT / path, destination)


class CurrentTree(unittest.TestCase):
    """I1: the verdict comes from the checked files alone, with no Git metadata or history."""

    def test_copy_without_git_gives_the_checkout_verdicts(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            copy(INPUTS, root)
            self.assertFalse((root / ".git").exists())
            for criterion in FILE_CRITERIA:
                with self.subTest(criterion=criterion):
                    self.assertEqual(authoring_check.observe(root, "authoring", criterion),
                                     authoring_check.observe(checker.ROOT, "authoring", criterion))

    def test_missing_product_file_is_an_observed_violation(self):
        for criterion in ("AC-1", "AC-10"):
            with self.subTest(criterion=criterion), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                copy([path for path in INPUTS if path != authoring_check.SKILL], root)
                observation, problems = authoring_check.observe(root, "authoring", criterion)
                self.assertEqual(observation["status"], "violation")
                self.assertIn(f"{authoring_check.SKILL}: required product artifact absent", problems)

    def test_local_paths_in_new_guidance_are_found(self):
        pattern = next(pattern for pattern, label in authoring_check.PRIVATE if label == "local path")
        for text in ("/Users/example/x", "/home/user/x", "/root/project/x", "/workspace/repo/x", "/workspaces/repo",
                     "/private/var/folders/ab/x", "/tmp/build/x", "../repo-worktrees/goal", "C:\\work\\x", "D:/src/x"):
            with self.subTest(text=text):
                self.assertTrue(pattern.search(text))
        for text in ("https://example.invalid/a", "references/bundle.md", "docs/specs/<goal>/PLAN.md", "a: b", "HEAD~n"):
            with self.subTest(text=text):
                self.assertFalse(pattern.search(text))


def go_events(results):
    lines = [json.dumps({"Action": "run", "Test": name}) for name in results]
    lines += [json.dumps({"Action": action, "Test": name}) for name, action in results.items()]
    return "\n".join(lines) + "\n"


class ResultMeaning(unittest.TestCase):
    """I2: a failing regression is a violation; a tool that cannot run is never one."""

    def run_regressions(self, go_code, go_out, docs_code=0, which=lambda tool: "/usr/bin/" + tool):
        def fake(argv, **kwargs):
            if argv[0] == "go":
                # A single space overrides GOFLAGS persisted with `go env -w`; empty would not.
                self.assertEqual((kwargs["env"]["GOFLAGS"], kwargs["env"]["GOWORK"]), (" ", "off"))
                return subprocess.CompletedProcess(argv, go_code, go_out, "build output")
            return subprocess.CompletedProcess(argv, docs_code, "", "docs output")
        with mock.patch.object(authoring_check.subprocess, "run", side_effect=fake), \
                mock.patch.object(authoring_check.shutil, "which", side_effect=which):
            return authoring_check.regressions(Path("."))

    def passing(self):
        return {name: "pass" for name in authoring_check.REQUIRED_GO_TESTS}

    def test_all_required_tests_pass(self):
        self.assertEqual(self.run_regressions(0, go_events(self.passing())), [])

    def test_failing_required_test_is_a_violation(self):
        results = self.passing() | {authoring_check.REQUIRED_GO_TESTS[0]: "fail"}
        problems = self.run_regressions(1, go_events(results))
        self.assertEqual(len(problems), 1)
        self.assertIn(authoring_check.REQUIRED_GO_TESTS[0], problems[0])

    def test_failing_public_docs_is_a_violation(self):
        self.assertEqual(len(self.run_regressions(0, go_events(self.passing()), docs_code=1)), 1)

    def test_conditions_that_observe_nothing_raise(self):
        skipped = self.passing() | {authoring_check.REQUIRED_GO_TESTS[0]: "skip"}
        cases = {
            "missing go": dict(go_code=0, go_out=go_events(self.passing()),
                               which=lambda tool: None if tool == "go" else "/bin/" + tool),
            "missing git": dict(go_code=0, go_out=go_events(self.passing()),
                                which=lambda tool: None if tool == "git" else "/bin/" + tool),
            "missing python3": dict(go_code=0, go_out=go_events(self.passing()),
                                    which=lambda tool: None if tool == "python3" else "/bin/" + tool),
            "build failure": dict(go_code=1, go_out=""),
            "required test skipped": dict(go_code=0, go_out=go_events(skipped)),
            "required test absent": dict(go_code=0, go_out=go_events({"TestOther": "pass"})),
            "docs check crashed": dict(go_code=0, go_out=go_events(self.passing()), docs_code=2),
        }
        for label, case in cases.items():
            with self.subTest(case=label), self.assertRaises(RuntimeError):
                self.run_regressions(**case)

    def test_producer_reports_a_raised_observation_as_error(self):
        with tempfile.TemporaryDirectory() as directory, \
                mock.patch.object(authoring_check, "regressions", side_effect=RuntimeError("go is not available")), \
                mock.patch.object(authoring_check, "section", return_value="x"), \
                mock.patch.object(authoring_check, "row", return_value=None), \
                mock.patch.object(authoring_check, "KEPT", []), \
                mock.patch.object(authoring_check, "CLAUSES", {}), \
                mock.patch.object(authoring_check, "ROWS", []), \
                mock.patch.object(sys, "argv", ["authoring_check.py", "authoring", "AC-9"]), \
                mock.patch.dict("os.environ", {"HA_EVIDENCE_PATH": str(Path(directory) / "result.json"),
                                               "HA_EVIDENCE_INVOCATION": "test",
                                               "HA_EVIDENCE_DECLARATION_DIGEST": "sha256:" + "0" * 64}), \
                mock.patch("sys.stdout"), mock.patch("sys.stderr"):
            checker.main()
            result = json.loads((Path(directory) / "result.json").read_text())
        statuses = [item["status"] for item in result["observations"]]
        self.assertEqual(statuses, ["error"])


if __name__ == "__main__":
    unittest.main()
