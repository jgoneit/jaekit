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
                     "/private/var/folders/ab/x", "/tmp/build/x", "../repo-worktrees/goal", "C:\\work\\x", "D:/src/x",
                     "/opt/runner/repo", "/var/tmp/build", "cd /workspace", "/workspace.", "/builds/group/project",
                     "/usr/src/project", "/srv/예시/project"):
            with self.subTest(text=text):
                self.assertTrue(pattern.search(text))
        for text in ("https://example.invalid/a", "references/bundle.md", "docs/specs/<goal>/PLAN.md", "a: b", "HEAD~n",
                     "https://example.invalid/opt/x", "and/or", "/workspaces-guide", "`/seal:seal`", "2>/dev/null", "../x/y"):
            with self.subTest(text=text):
                self.assertFalse(pattern.search(text))

    def test_placeholder_goal_paths_exempt_only_the_placeholder_segment(self):
        pattern = next(pattern for pattern, label in authoring_check.PRIVATE if label == "concrete goal path")
        for text in ("docs/specs/example-private/PLAN.md", "docs/specs/x-secret/SPEC.md", "docs/specs/sample.v2/SPEC.md",
                     "docs/spec/real-goal/SPEC.md"):
            with self.subTest(text=text):
                self.assertTrue(pattern.search(text))
        for text in ("docs/specs/<goal>/SPEC.md", "docs/specs/example/PLAN.md", "see `docs/specs/x`", "docs/specs/sample."):
            with self.subTest(text=text):
                self.assertFalse(pattern.search(text))

    def test_an_exempt_phrase_does_not_hide_another_user_step(self):
        for sentence, steps in (
                ("Without asking about the boundary, ask the user to approve the migration.", ["ask", "approve"]),
                ("Ask the user to confirm the plan.", ["Ask", "confirm"]),
                ("Ask the user to confirm the plan without asking about non-goals.", ["Ask", "confirm"]),
                ("Without asking the user, ask for approval.", ["ask", "approval"]),
                ("Never ask the user for permission to apply these rules.", []),
                ("Without asking for approval, ask the user to confirm the plan.", ["ask", "confirm"]),
                ("They add no user confirmation or approval and ask the user to approve deployment.", ["ask", "approve"]),
                ("Question the user before applying the migration.", ["Question"]),
                ("Apply these rules without asking for confirmation, approval, or permission.", []),
                ("Never ask or confirm with the user.", []),
                ("Never ask for approval or confirm with the user.", []),
                ("Apply these rules without asking for approval and ask the user.", ["ask"]),
                ("Open questions go to REVIEW.md.", []),
                ("새로 묻지 않는다, 사용자에게 승인을 받는다.", ["승인을 받"]),
                ("They add no user confirmation, approval or question: the executor applies them.", []),
                ("Record it as an explicit assumption, without asking the user.", []),
                ("사용자에게 묻지 않고 기록한다.", [])):
            with self.subTest(sentence=sentence):
                self.assertEqual(authoring_check.new_user_steps(sentence), steps)

    def test_malformed_marketplace_values_are_observed_violations(self):
        for field, value in (("metadata", []), ("plugins", [1]), ("plugins", {"name": "seal"})):
            with self.subTest(field=field, value=value), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                copy(INPUTS, root)
                path = root / ".claude-plugin/marketplace.json"
                market = json.loads(path.read_text())
                market[field] = value
                path.write_text(json.dumps(market))
                observation, problems = authoring_check.observe(root, "authoring", "AC-8")
                self.assertEqual(observation["status"], "violation")

    def test_maintain_example_may_list_a_current_tree_helper(self):
        for check_path, status in ((" tools/checks/current_tree.py ", "pass"), (" origin/main:tools/x.py ", "violation")):
            with self.subTest(check_path=check_path), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                copy(INPUTS, root)
                plan = root / authoring_check.PLAN
                text = plan.read_text()
                row = next(line for line in text.split("\n") if line.startswith("| AC-2 | maintain |"))
                cells = row.strip("|").split("|")
                cells[3] = check_path
                plan.write_text(text.replace(row, "|" + "|".join(cells) + "|"))
                self.assertEqual(authoring_check.observe(root, "authoring", "AC-2")[0]["status"], status)

    def test_malformed_manifest_is_an_observed_violation(self):
        for path, text in (("plugins/seal/.codex-plugin/plugin.json", "{ not json"),
                           ("plugins/seal/.claude-plugin/plugin.json", "[]"),
                           (".claude-plugin/marketplace.json", "{ not json")):
            with self.subTest(path=path), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                copy(INPUTS, root)
                (root / path).write_text(text)
                observation, problems = authoring_check.observe(root, "authoring", "AC-8")
                self.assertEqual(observation["status"], "violation")
                self.assertIn(f"{path} is absent or not a JSON object", problems)


def go_events(results):
    lines = [json.dumps({"Action": "run", "Test": name}) for name in results]
    lines += [json.dumps({"Action": action, "Test": name}) for name, action in results.items()]
    return "\n".join(lines) + "\n"


class ResultMeaning(unittest.TestCase):
    """I2: a failing regression is a violation; a tool that cannot run is never one."""

    def run_regressions(self, go_code, go_out, docs_code=0, which=lambda tool: "/usr/bin/" + tool,
                        starts=lambda tool: 0):
        probes = list(authoring_check.REQUIRED_TOOLS.values())

        def fake(argv, **kwargs):
            if argv in probes:
                # Each tool is started once; a tool that exists but cannot start fails here.
                return subprocess.CompletedProcess(argv, starts(argv[0]), "", "cannot start")
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
            "python3 on PATH cannot start": dict(go_code=1, go_out=go_events(self.passing() | {
                authoring_check.REQUIRED_GO_TESTS[0]: "fail"}), starts=lambda tool: 126 if tool == "python3" else 0),
            "git on PATH cannot start": dict(go_code=0, go_out=go_events(self.passing()),
                                             starts=lambda tool: 1 if tool == "git" else 0),
            "go killed by a signal after a failure": dict(go_code=-9, go_out=go_events(self.passing() | {
                authoring_check.REQUIRED_GO_TESTS[0]: "fail"})),
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
