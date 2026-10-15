"""Verification input preflight, elapsed-time output and validation guidance.

Fixtures are temporary checkouts with recording command wrappers. The input
checks use the real Git executable on a synthetic repository; the current
checkout, the real HOME and global Git configuration are never used.
"""
import importlib.util
import inspect
import os
from pathlib import Path
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
import unittest


ROOT = Path(__file__).resolve().parents[2]
_spec = importlib.util.spec_from_file_location("feedback_verification", ROOT / "tools/workflow-checks/test_verify.py")
verification = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(verification)

STEP_PASS = re.compile(r"^PASS: (?P<label>.+) \((?P<seconds>\d+\.\d)s\)$")
GROUP_PASS = re.compile(r"^PASS: (?P<group>all|go|docs) verification \(total (?P<seconds>\d+\.\d)s\)$")
NOT_STARTED = re.compile(r"^FAIL: verification stopped before any check started \(total \d+\.\ds\)$", re.M)
DOCS_LABELS = ["Public file boundary", "Public documentation", "Public checker regressions",
               "Workflow regressions", "Native CI checker regressions", "Release archive regressions",
               "Windows candidate packaging regressions", "Goal check regressions",
               "Release acceptance checker regressions",
               "Static selection regressions (integration_selection_go.py)",
               "Static selection regressions (integration_selection_go_identifiers.py)",
               "Static selection regressions (integration_selection_go_layout.py)",
               "Static selection regressions (integration_selection_python.py)"]


class Observed(unittest.TestCase):
    def require(self, condition, violation, detail=""):
        # The tag names the observed requirement for structured results.
        if not condition:
            self.fail(f"[violation:{violation}] {detail}")


class TimingFixture(verification.Fixture, Observed):
    """Recording wrappers without Git state: the fake git lists no files."""

    def sleeping_check(self, name, marker):
        # The selected check records its pid, then waits for a signal.
        (self.repo / "tools" / name).write_text(
            "import os, pathlib, time\n"
            f"pathlib.Path({str(marker)!r}).write_text(str(os.getpid()))\n"
            "time.sleep(30)\n")

    def start_and_wait(self, marker, *groups):
        process = subprocess.Popen([sys.executable, str(self.repo / "tools/verify.py"), *groups],
                                   cwd=self.root, env=self.env, stdout=subprocess.PIPE,
                                   stderr=subprocess.PIPE, text=True)
        self.addCleanup(self.stop, process, marker)
        deadline = time.monotonic() + 20
        while not marker.exists() or not marker.read_text():
            if time.monotonic() > deadline or process.poll() is not None:
                raise RuntimeError("the sleeping synthetic check did not start")
            time.sleep(0.05)
        return process

    @staticmethod
    def stop(process, marker):
        if process.poll() is None:
            process.kill()
        TimingFixture.end_check(marker)
        process.communicate()

    @staticmethod
    def end_check(marker):
        try:
            os.kill(int(marker.read_text()), signal.SIGKILL)
        except (OSError, ValueError):
            pass

    def signal_and_collect(self, process, marker, number):
        # A signal that ends only verify.py can leave the child holding the
        # output pipe, so end the synthetic child before reading the output.
        process.send_signal(number)
        process.wait(timeout=30)
        self.end_check(marker)
        return process.communicate(timeout=30)


class GitFixture(verification.Fixture, Observed):
    """A real Git repository whose git wrapper records and delegates."""

    def setUp(self):
        super().setUp()
        real = shutil.which("git")
        if real is None:
            raise RuntimeError("the input fixture requires the existing Git tool")
        home = self.root / "home"
        (home / ".config/git").mkdir(parents=True)
        # Only fixture configuration is visible to Git: no real HOME, system
        # file, inherited repository selection or evidence transport.
        self.env = {key: value for key, value in self.env.items()
                    if not key.startswith(("GIT_", "HA_EVIDENCE_")) and key != "XDG_CONFIG_HOME"}
        self.env.update(HOME=str(home), GIT_CONFIG_NOSYSTEM="1",
                        GIT_CEILING_DIRECTORIES=os.path.realpath(self.root))
        recorder = (f"#!{sys.executable}\nimport json, os, sys\n"
                    "with open(os.environ['VERIFY_TEST_LOG'], 'a') as stream:\n"
                    "    stream.write(json.dumps({'name': 'git', 'args': sys.argv[1:], "
                    "'no_bytecode': os.environ.get('PYTHONDONTWRITEBYTECODE')}) + '\\n')\n"
                    f"os.execv({real!r}, [{real!r}, *sys.argv[1:]])\n")
        (self.bin / "git").write_text(recorder)
        (self.bin / "git").chmod(0o700)
        self.real_git = real
        (self.repo / ".gitignore").write_text("/docs/\n__pycache__/\n")
        self.git(self.repo, "init", "--quiet")
        self.git(self.repo, "add", "--all")
        self.git(self.repo, "commit", "--quiet", "--message", "synthetic checkout")

    def git(self, cwd, *args):
        return subprocess.run([self.real_git, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid",
                               "-c", "init.defaultBranch=main", "-c", "commit.gpgsign=false", *args],
                              cwd=cwd, env=self.env, check=True, capture_output=True, text=True, timeout=30)

    def run_in(self, checkout, *groups):
        return subprocess.run([sys.executable, str(checkout / "tools/verify.py"), *groups],
                              cwd=self.root, env=self.env, capture_output=True, text=True, timeout=30)


class InputPreflight(GitFixture):
    def refused(self, result, *paths):
        output = result.stdout + result.stderr
        return (result.returncode == 1 and "RUN:" not in output
                and all(path in result.stderr for path in paths)
                and "git add" in result.stderr and ".gitignore" in result.stderr)

    def test_untracked_input_stops_every_group_before_checks(self):
        (self.repo / "tools/new_tool.py").write_text("print('not tracked yet')\n")
        (self.repo / "guides").mkdir()
        (self.repo / "guides/new guide.md").write_text("# Untracked public document\n")
        for group in ((), ("all",), ("go",), ("docs",)):
            self.log.unlink(missing_ok=True)
            result = self.run_verify(*group)
            self.require(self.refused(result, "tools/new_tool.py", "guides/new guide.md"),
                         "untracked-input-not-refused",
                         f"group {group or 'default'} did not stop before checks:\n{result.stdout}{result.stderr}")
            names = {item["name"] for item in self.commands()}
            self.require(names <= {"git"}, "untracked-input-not-refused",
                         f"checks ran after an input refusal: {sorted(names)}")
        linked = self.root / "linked"
        self.git(self.repo, "worktree", "add", "--quiet", "--detach", str(linked), "HEAD")
        (linked / "tools/linked_tool.py").write_text("print('linked and untracked')\n")
        result = self.run_in(linked, "docs")
        self.require(self.refused(result, "tools/linked_tool.py"), "untracked-input-not-refused",
                     f"a linked worktree did not stop before checks:\n{result.stdout}{result.stderr}")

    def test_ignored_files_and_tracked_edits_do_not_stop_verification(self):
        (self.repo / "docs/specs/synthetic/PLAN.md").write_text("ignored local goal material\n")
        (self.repo / ".git/info/exclude").write_text("local-notes.txt\n")
        (self.repo / "local-notes.txt").write_text("excluded by repository-local rules\n")
        (self.root / "home/.config/git/ignore").write_text("*.scratch\n")
        (self.repo / "draft.scratch").write_text("excluded by the user's global rules\n")
        with (self.repo / "code.go").open("a") as stream:
            stream.write("// an uncommitted tracked edit\n")
        result = self.run_verify("docs")
        self.require(result.returncode == 0 and "RUN:" in result.stdout, "clean-input-refused",
                     f"ignored files or tracked edits stopped verification:\n{result.stdout}{result.stderr}")
        linked = self.root / "linked"
        self.git(self.repo, "worktree", "add", "--quiet", "--detach", str(linked), "HEAD")
        result = self.run_in(linked, "docs")
        self.require(result.returncode == 0 and "RUN:" in result.stdout, "clean-input-refused",
                     f"a clean linked worktree stopped verification:\n{result.stdout}{result.stderr}")

    def test_unreadable_git_state_stops_before_checks(self):
        shutil.rmtree(self.repo / ".git")
        for group in ((), ("docs",)):
            self.log.unlink(missing_ok=True)
            result = self.run_verify(*group)
            output = result.stdout + result.stderr
            self.require(result.returncode == 1 and "RUN:" not in output
                         and "cannot read untracked files from Git" in result.stderr,
                         "git-state-assumed-clean",
                         f"unreadable Git state was treated as clean:\n{output}")

    def test_input_check_reads_without_changing_files_or_git_state(self):
        before = self.snapshot()
        result = self.run_verify("docs")
        self.require(result.returncode == 0 and self.snapshot() == before, "input-check-mutated-state",
                     f"a successful run changed the checkout:\n{result.stdout}{result.stderr}")
        (self.repo / "tools/new_tool.py").write_text("print('not tracked yet')\n")
        before = self.snapshot()
        self.log.unlink(missing_ok=True)
        result = self.run_verify("docs")
        self.require(self.snapshot() == before, "input-check-mutated-state",
                     f"a run with untracked input changed the checkout:\n{result.stdout}{result.stderr}")
        listing = [item["args"] for item in self.commands() if item["name"] == "git"]
        self.require(all("ls-files" in args and "--others" in args for args in listing),
                     "input-check-mutated-state", f"unexpected Git commands: {listing}")


class ElapsedTime(TimingFixture):
    def test_successful_groups_show_each_step_and_total_time(self):
        for group in ((), ("go",), ("docs",)):
            result = self.run_verify(*group)
            lines = result.stdout.splitlines()
            started = [line.removeprefix("RUN: ") for line in lines if line.startswith("RUN: ")]
            passed = [STEP_PASS.match(line) for line in lines
                      if line.startswith("PASS: ") and not GROUP_PASS.match(line)]
            self.require(result.returncode == 0 and started and all(passed)
                         and [match["label"] for match in passed] == started
                         and GROUP_PASS.match(lines[-1]) is not None,
                         "step-time-missing", f"group {group or 'default'}:\n{result.stdout}{result.stderr}")
        # ha check stores both streams in one local log; the same lines remain.
        with tempfile.TemporaryFile(mode="w+") as log:
            result = subprocess.run([sys.executable, str(self.repo / "tools/verify.py"), "docs"],
                                    cwd=self.root, env=self.env, stdout=log, stderr=subprocess.STDOUT, timeout=30)
            log.seek(0)
            lines = log.read().splitlines()
        self.require(result.returncode == 0 and len([line for line in lines if STEP_PASS.match(line)]) == len(DOCS_LABELS)
                     and GROUP_PASS.match(lines[-1]) is not None, "step-time-missing",
                     "a combined output log lost elapsed times:\n" + "\n".join(lines))

    def test_timing_units_and_intervals_are_documented(self):
        korean = section((ROOT / "guides/CI.md").read_text(), "## 로컬 검사")
        english = section((ROOT / "guides/CI.en.md").read_text(), "## Local checks")
        self.require(all(word in korean for word in ("초", "단조 시계", "`RUN:`", "전체 시간"))
                     and all(word in english for word in ("seconds", "monotonic clock", "`RUN:`", "total time")),
                     "timing-guidance-missing", "the CI guides do not define elapsed-time units and intervals")


class InterruptedTime(TimingFixture):
    def test_failed_step_shows_step_and_total_time(self):
        for failure, line in (("vet", r"FAIL: Go vet: exit 19 \(step \d+\.\ds, total \d+\.\ds\)"),
                              ("format", r"FAIL: Go formatting: files listed above need gofmt; no files were "
                                         r"rewritten \(step \d+\.\ds, total \d+\.\ds\)")):
            result = self.run_verify(VERIFY_TEST_FAIL=failure)
            self.require(result.returncode == 1 and re.search("^" + line + "$", result.stderr, re.M),
                         "failure-time-missing", f"{failure}:\n{result.stdout}{result.stderr}")

    def test_keyboard_interrupt_shows_the_step_and_both_times(self):
        marker = self.root / "started"
        self.sleeping_check("check_public_docs.py", marker)
        process = self.start_and_wait(marker, "docs")
        stdout, stderr = self.signal_and_collect(process, marker, signal.SIGINT)
        self.require(process.returncode == 130 and re.search(
            r"^FAIL: verification interrupted during Public documentation \(step \d+\.\ds, total \d+\.\ds\)$",
            stderr, re.M), "interrupt-time-missing", stdout + stderr)

    def test_preflight_refusal_shows_that_no_check_started_and_total_time(self):
        for tool in ("go", "zsh"):
            self.log.unlink(missing_ok=True)
            hidden = self.bin / (tool + "-hidden")
            (self.bin / tool).rename(hidden)
            try:
                result = self.run_verify()
            finally:
                hidden.rename(self.bin / tool)
            self.require(result.returncode == 1 and "RUN:" not in result.stdout + result.stderr
                         and NOT_STARTED.search(result.stderr), "preflight-time-missing",
                         f"{tool}:\n{result.stdout}{result.stderr}")
        # A shell found on PATH that cannot start fails the shell preflight.
        shell = self.bin / "zsh"
        saved = shell.read_bytes()
        shell.write_text("#!/nonexistent-synthetic-interpreter\n")
        try:
            result = self.run_verify("go")
        finally:
            shell.write_bytes(saved)
        self.require(result.returncode == 1 and "RUN:" not in result.stdout + result.stderr
                     and "zsh" in result.stderr and NOT_STARTED.search(result.stderr), "preflight-time-missing",
                     f"unusable zsh:\n{result.stdout}{result.stderr}")

    def test_other_signals_keep_completed_step_times_and_the_last_run_line(self):
        marker = self.root / "started"
        self.sleeping_check("check_public_docs.py", marker)
        process = self.start_and_wait(marker, "docs")
        stdout, stderr = self.signal_and_collect(process, marker, signal.SIGTERM)
        lines = stdout.splitlines()
        self.require(process.returncode == -signal.SIGTERM
                     and any(STEP_PASS.match(line) and line.startswith("PASS: Public file boundary (") for line in lines)
                     and "RUN: Public documentation" in lines and "interrupted" not in stderr,
                     "completed-step-time-missing", stdout + stderr)


class InputRefusalTime(GitFixture):
    def test_input_refusal_shows_that_no_check_started_and_total_time(self):
        (self.repo / "tools/new_tool.py").write_text("print('not tracked yet')\n")
        result = self.run_verify("docs")
        self.require(result.returncode == 1 and NOT_STARTED.search(result.stderr), "preflight-time-missing",
                     result.stdout + result.stderr)


def section(text, heading):
    body = text.split(heading + "\n", 1)[1]
    return re.split(r"^## ", body, maxsplit=1, flags=re.M)[0]


class ValidationGuidance(Observed):
    def test_one_full_pass_on_the_same_input_satisfies_commit_and_push(self):
        agents = section((ROOT / "AGENTS.md").read_text(), "## Validation").lower()
        korean = section((ROOT / "guides/CI.md").read_text(), "## 로컬 검사")
        english = section((ROOT / "guides/CI.en.md").read_text(), "## Local checks").lower()
        template = (ROOT / ".github/pull_request_template.md").read_text()
        template_ko = "\n".join(line for line in template.splitlines() if "tools/verify.py" in line and "같은" in line)
        template_en = "\n".join(line for line in template.splitlines()
                                if "tools/verify.py" in line and "same" in line).lower()
        english_rules = ("same verification input", "once", "before or after", "commit id",
                         "narrower checks", "seal", "required ci", "command and group", "environment")
        korean_rules = ("같은 검증 입력", "한 번", "커밋 전후", "커밋 식별자", "좁은 검사", "Seal", "필수 CI",
                        "명령과 그룹", "환경")
        missing = ([f"AGENTS.md: {rule}" for rule in english_rules if rule not in agents]
                   + [f"guides/CI.en.md: {rule}" for rule in english_rules if rule not in english]
                   + [f"guides/CI.md: {rule}" for rule in korean_rules if rule not in korean]
                   + [f"PR template (ko): {rule}" for rule in ("같은 검증 입력", "한 번", "커밋 전후")
                      if rule not in template_ko]
                   + [f"PR template (en): {rule}" for rule in ("same verification input", "once", "before or after")
                      if rule not in template_en])
        self.require(not missing, "single-pass-guidance-missing", "\n".join(missing))
        self.require(re.findall(r"```bash\n(.*?)```", section((ROOT / "AGENTS.md").read_text(), "## Validation"),
                                flags=re.S) == ["python3 tools/verify.py\n"]
                     and re.search(r"before[^\n]*(?:push|publish)", agents), "single-pass-guidance-missing",
                     "the single documented command or the pre-publication requirement changed")


class PreservedInterface(TimingFixture):
    def test_line_prefixes_labels_and_module_interface_are_kept(self):
        result = self.run_verify("docs")
        lines = result.stdout.splitlines()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual([line.removeprefix("RUN: ") for line in lines if line.startswith("RUN: ")], DOCS_LABELS)
        self.assertEqual([STEP_PASS.match(line)["label"] for line in lines if STEP_PASS.match(line)], DOCS_LABELS)
        self.assertTrue(lines[-1].startswith("PASS: docs verification"))
        failed = self.run_verify("docs", VERIFY_TEST_FAIL="check_public_docs.py")
        self.assertIn("FAIL: Public documentation: exit 29", failed.stderr)
        # Consumers import the module, replace run and call one group.
        self.log.unlink(missing_ok=True)
        spec = importlib.util.spec_from_file_location("preserved_verify", self.repo / "tools/verify.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        self.assertEqual(self.commands(), [], "importing the module must not start checks or Git reads")
        self.assertEqual(module.TARGETS, ("linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64"))
        self.assertTrue(issubclass(module.CheckFailed, Exception))
        self.assertEqual(list(inspect.signature(module.run).parameters), ["label", "command", "env", "formatting"])
        self.assertEqual(list(inspect.signature(module.full_go_environment).parameters), ["env"])
        labels = []
        module.run = lambda label, command, env, *, formatting=False: labels.append(label)
        module.verify_go({})
        self.assertEqual(labels[:3], ["Go formatting", "Go vet", "Go tests"])
        self.assertEqual(labels[-2:], ["CLI version", "CLI help"])
        self.assertEqual(self.commands(), [], "a replaced runner must not run the input check")


if __name__ == "__main__":
    unittest.main()
