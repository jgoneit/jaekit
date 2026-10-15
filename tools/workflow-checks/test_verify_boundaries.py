"""Boundary regressions for the verification input preflight and elapsed times.

These extend test_verify_feedback.py: inherited Git selection, fsmonitor
hooks, interrupts during tool discovery, an unavailable clock and the
pull-request guidance. Only temporary checkouts are written.
"""
import contextlib
import importlib.util
import io
import os
from pathlib import Path
import re
import sys
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
_spec = importlib.util.spec_from_file_location("boundary_feedback", ROOT / "tools/workflow-checks/test_verify_feedback.py")
feedback = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(feedback)


def load_verify(path):
    spec = importlib.util.spec_from_file_location("boundary_verify", path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class InheritedGitState(feedback.GitFixture):
    def test_inherited_repository_selection_cannot_hide_untracked_input(self):
        (self.repo / "tools/new_tool.py").write_text("print('not tracked yet')\n")
        empty = self.root / "empty-worktree"
        empty.mkdir()
        index = self.root / "other-index"
        result = self.run_verify("docs", GIT_DIR=str(self.repo / ".git"), GIT_WORK_TREE=str(empty),
                                 GIT_INDEX_FILE=str(index))
        output = result.stdout + result.stderr
        self.require(result.returncode == 1 and "RUN:" not in output and "tools/new_tool.py" in result.stderr,
                     "untracked-input-not-refused",
                     f"inherited Git selection hid untracked checkout input:\n{output}")

    def test_configured_fsmonitor_hook_is_not_run(self):
        marker = self.root / "fsmonitor-ran"
        hook = self.root / "fsmonitor-hook"
        hook.write_text(f"#!{sys.executable}\nimport pathlib\npathlib.Path({str(marker)!r}).write_text('ran')\n")
        hook.chmod(0o700)
        self.git(self.repo, "config", "core.fsmonitor", str(hook))
        (self.repo / "tools/new_tool.py").write_text("print('not tracked yet')\n")
        before = self.snapshot()
        for group in ("docs", "go"):
            self.run_verify(group)
        self.require(not marker.exists() and self.snapshot() == before, "input-check-mutated-state",
                     "the input listing ran a configured fsmonitor hook or changed the checkout")

    def test_inherited_command_line_configuration_cannot_run_fsmonitor(self):
        # A Git alias or hook can launch verification with `git -c` settings.
        marker = self.root / "fsmonitor-ran"
        hook = self.root / "fsmonitor-hook"
        hook.write_text(f"#!{sys.executable}\nimport pathlib\npathlib.Path({str(marker)!r}).write_text('ran')\n")
        hook.chmod(0o700)
        (self.repo / "tools/new_tool.py").write_text("print('not tracked yet')\n")
        before = self.snapshot()
        inherited = f"'core.fsmonitor'='{hook}'"
        for group in ("docs", "go"):
            self.run_verify(group, GIT_CONFIG_PARAMETERS=inherited)
        self.require(not marker.exists() and self.snapshot() == before, "input-check-mutated-state",
                     "an inherited command-line fsmonitor setting ran during the input listing")


class EarlyInterrupt(feedback.TimingFixture):
    def test_interrupt_during_tool_discovery_reports_that_no_check_started(self):
        module = load_verify(self.repo / "tools/verify.py")
        stderr = io.StringIO()
        with mock.patch.object(sys, "argv", ["verify.py", "docs"]), \
                mock.patch.object(module.shutil, "which", side_effect=KeyboardInterrupt), \
                contextlib.redirect_stderr(stderr), contextlib.redirect_stdout(io.StringIO()):
            try:
                code = module.main()
            except KeyboardInterrupt:
                code = None
        self.require(code == 130 and re.search(
            r"^FAIL: verification interrupted before any check started \(total \d+\.\ds\)$",
            stderr.getvalue(), re.M), "interrupt-time-missing",
            f"exit {code}: {stderr.getvalue()!r}")


class UnavailableClock(feedback.TimingFixture):
    def test_unavailable_clock_is_shown_without_changing_the_result(self):
        module = load_verify(self.repo / "tools/verify.py")

        def unavailable():
            raise OSError("synthetic clock failure")
        stdout = io.StringIO()
        with mock.patch.object(module, "CLOCK", unavailable, create=True), contextlib.redirect_stdout(stdout):
            try:
                module.run("Synthetic step", [sys.executable, "-c", "pass"], dict(os.environ))
                raised = None
            except Exception as error:  # the result itself must not change
                raised = error
        self.require(raised is None and "PASS: Synthetic step (time unavailable)" in stdout.getvalue(),
                     "unavailable-time-hidden", f"{raised!r}: {stdout.getvalue()!r}")


class PullRequestGuidance(feedback.Observed):
    def test_pull_request_template_states_the_input_and_reuse_limits(self):
        template = (ROOT / ".github/pull_request_template.md").read_text()
        section = template.split("## 검증 방법과 결과 / Validation", 1)[1].split("\n## ", 1)[0]
        korean = ("같은 검증 입력", "한 번", "커밋 전후", "검증 명령과 그룹", "환경", "커밋 식별자", "좁은 검사",
                  "Seal", "필수 CI")
        english = ("same verification input", "once", "before or after", "command and group", "environment",
                   "commit id", "narrower checks", "seal", "required ci")
        missing = ([rule for rule in korean if rule not in section]
                   + [rule for rule in english if rule not in section.lower()])
        self.require(not missing, "single-pass-guidance-missing", f"pull request template lacks: {missing}")


if __name__ == "__main__":
    unittest.main()
