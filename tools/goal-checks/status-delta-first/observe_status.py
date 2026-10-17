"""Observe `ha status` and failed `ha check` output through the real CLI.

`ha` is built from this checkout once per run and driven against synthetic
Git repositories in temporary directories. Each goal has two maintain
conditions: AC-1 passes on the fixture and AC-2 passes only after
`src/greeting.txt` says hello. Nothing reads private documents, the real HOME
or global Git configuration.

A failed build, a Git or `ha` step that cannot run, and a fixture step that
ends differently than the fixture requires (for example a check that errors
where it should fail) are errors, never violations. Only an observed output
property is a violation, named by an explicit `[violation:...]` tag.
"""
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import time
import unittest


ROOT = Path(__file__).resolve().parents[3]
GOAL = "docs/specs/g"
LOG_DIR = ".git/ha/runs/docs%2Fspecs%2Fg"
SECRET = "status-delta-output-marker-5d1c"
ENV_SECRET = "status-delta-env-marker-93ab"
STATUS_FIELDS = {"schema", "goal", "status", "exit_code", "assurance", "check_author", "rules", "ha_version",
                 "skill", "record_head", "completion_record", "complete_without_record", "commit", "tree_clean",
                 "spec_digest", "criteria", "reasons", "lint", "changes", "budget", "findings", "usage", "logs"}
RESULT_LINE = re.compile(r"^(\S+) (check|baseline) (\S+) \(seq (\d+)\)$", re.M)

SPEC = """# Greeting says hello

Status: Ready

## 목표

`src/greeting.txt` should say hello.

## 범위와 비목표

- 범위: `src/greeting.txt`
- 비목표: `src/keep.txt`

## Acceptance Criteria
- **AC-1** `src/keep.txt` still contains exactly `keep`.
- **AC-2** `src/greeting.txt` contains exactly `hello`.

## Open Decisions

없음
"""
PLAN = """# PLAN — g

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | maintain | `sh tests/keep.sh` | — | T001 |
| AC-2 | maintain | `{greeting}` | — | T001 |

## 범위
- 바꿀 수 있는 경로: `src/**`, `tests/**` (SPEC: 범위와 비목표)
- 테스트 경로: (기본값)
- 유지할 동작: `src/keep.txt`

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | greeting says hello and keep stays | AC-1, AC-2 | — | — | 예 | — |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측
"""
PROGRESS = """# PROGRESS — g

## Task 상태
| Task | 상태 | 메모 |
| --- | --- | --- |
| T001 | {state} | |
"""


class Unavailable(Exception):
    """A tool or fixture step could not run as required; reported as an error."""


def isolated_env(home):
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(("GIT_", "HA_EVIDENCE_"))}
    env.update(HOME=str(home), GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
               GIT_AUTHOR_NAME="Fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
               GIT_COMMITTER_NAME="Fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid",
               STATUS_DELTA_ENV=ENV_SECRET, LC_ALL="C")
    return env


class Observed(unittest.TestCase):
    """Builds ha once; each test gets a fresh synthetic repository."""

    @classmethod
    def setUpClass(cls):
        cls.temporary = Path(tempfile.mkdtemp(prefix="jaekit-status-delta-"))
        cls.ha = cls.temporary / "ha"
        env = dict(os.environ, GOTOOLCHAIN="local", GOFLAGS="-mod=readonly", CGO_ENABLED="0")
        try:
            built = subprocess.run(["go", "build", "-o", str(cls.ha), "./cmd/ha"], cwd=ROOT, env=env,
                                   capture_output=True, text=True, timeout=600)
        except (OSError, subprocess.TimeoutExpired) as error:
            shutil.rmtree(cls.temporary, ignore_errors=True)
            raise Unavailable(f"cannot build ha: {error}") from None
        if built.returncode:
            shutil.rmtree(cls.temporary, ignore_errors=True)
            raise Unavailable("cannot build ha: " + built.stderr)

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.temporary, ignore_errors=True)

    def setUp(self):
        self.base = Path(tempfile.mkdtemp(prefix="repo-", dir=self.temporary))
        self.repo = self.base / "repo"
        self.home = self.base / "home"
        self.home.mkdir()
        self.env = isolated_env(self.home)
        for name, text in self.fixture_files().items():
            path = self.repo / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text, encoding="utf-8")
        self.git("init", "-q", "-b", "main")
        self.commit("fixture")
        self.run_ha("start", GOAL, "--request", "greeting을 hello로 바꿔줘", expect=(0,))
        self.after_start()

    def after_start(self):
        pass

    def fixture_files(self):
        return {
            f"{GOAL}/SPEC.md": SPEC,
            f"{GOAL}/PLAN.md": PLAN.format(greeting="sh tests/greeting.sh"),
            f"{GOAL}/REVIEW.md": "# REVIEW — g\n\n## 결론\nready\n",
            f"{GOAL}/PROGRESS.md": PROGRESS.format(state="todo"),
            "src/greeting.txt": "hi\n",
            "src/keep.txt": "keep\n",
            "tests/keep.sh": "grep -qx keep src/keep.txt\n",
            # The marker is command output: it may reach `ha log`, never status.
            "tests/greeting.sh": f"echo {SECRET}\necho \"$STATUS_DELTA_ENV\"\ngrep -qx hello src/greeting.txt\n",
        }

    def tearDown(self):
        shutil.rmtree(self.base, ignore_errors=True)

    def git(self, *args):
        try:
            result = subprocess.run(["git", *args], cwd=self.repo, env=self.env, capture_output=True,
                                    text=True, timeout=60)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Unavailable(f"git {args[0]}: {error}") from None
        if result.returncode:
            raise Unavailable(f"git {' '.join(args)}: {result.stderr}")
        return result.stdout

    def commit(self, message):
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)

    def run_ha(self, *args, expect=None, cwd=None):
        try:
            result = subprocess.run([str(self.ha), *args], cwd=cwd or self.repo, env=self.env,
                                    capture_output=True, text=True, timeout=300)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Unavailable(f"ha {args[0]}: {error}") from None
        if expect is not None and result.returncode not in expect:
            raise Unavailable(f"ha {' '.join(args)} exited {result.returncode}: {result.stdout}{result.stderr}")
        return result

    def check(self, *targets, want=None, goal=GOAL, cwd=None):
        """Run ha check; `want` maps each target to the result the fixture requires."""
        result = self.run_ha("check", goal, *targets, expect=(0, 1), cwd=cwd)
        found = {m.group(1): (m.group(3), int(m.group(4))) for m in RESULT_LINE.finditer(result.stdout)}
        if want is not None and {t: r for t, (r, _) in found.items()} != want:
            raise Unavailable(f"fixture checks ended {found}, not {want}: {result.stdout}{result.stderr}")
        return result, {t: s for t, (_, s) in found.items()}

    def status(self, goal=GOAL, cwd=None):
        return self.run_ha("status", goal, expect=(0, 1, 2, 3, 4), cwd=cwd).stdout

    def say_hello(self):
        (self.repo / "src/greeting.txt").write_text("hello\n", encoding="utf-8")
        (self.repo / GOAL / "PROGRESS.md").write_text(PROGRESS.format(state="done"), encoding="utf-8")
        self.commit("say hello")

    def complete(self):
        """Record a completion and return its seq and commit."""
        self.say_hello()
        self.check(want={"AC-1": "pass", "AC-2": "pass"})
        done = self.run_ha("done", GOAL, expect=(0,)).stdout
        match = re.search(r"^- completion record: seq (\d+)$", done, re.M)
        if not match:
            raise Unavailable("ha done did not record a completion: " + done)
        return int(match.group(1)), self.git("rev-parse", "HEAD").strip()

    def change_code(self):
        (self.repo / "src/extra.txt").write_text("x\n", encoding="utf-8")
        self.commit("more code")

    def block_output(self, seq):
        """Make storing the output of run `seq` fail after the process ran."""
        blocker = self.repo / LOG_DIR / f"{seq}.log"
        blocker.mkdir(parents=True)
        (blocker / "keep").write_text("x", encoding="utf-8")

    def require(self, condition, violation, detail=""):
        # The tag names the observed requirement for structured results.
        if not condition:
            self.fail(f"[violation:{violation}] {detail}")


def head_of(text, heading="### Criteria"):
    if heading not in text:
        raise Unavailable(f"status output has no {heading!r}: {text}")
    return text.split(heading, 1)[0]


def item(text, start):
    """Return the first list item that starts with `start`, or ""."""
    for line in text.splitlines():
        if line.startswith("- " + start):
            return line
    return ""


def completion_line(text):
    return item(text, "completion record:")


def log_line(criterion, reason, cls, seq, goal=GOAL):
    return f"- {criterion} `{reason}` ({cls}): seq {seq}, output `ha log {goal} {seq}`"


class UnmetSummary(Observed):
    def test_unmet_conditions_and_goal_reasons_come_before_the_table(self):
        _, seqs = self.check(want={"AC-1": "pass", "AC-2": "fail"})
        summary = head_of(self.status())
        self.require(item(summary, "AC-2 ") == log_line("AC-2", "criterion_failed", "executor", seqs["AC-2"])
                     and item(summary, "`task_open` ") == "- `task_open` (executor)"
                     and not item(summary, "AC-1 "),
                     "unmet-summary-missing", summary)

    def test_summary_points_to_the_latest_fresh_record(self):
        self.check(want={"AC-1": "pass", "AC-2": "fail"})
        _, again = self.check("AC-2", want={"AC-2": "fail"})
        summary = head_of(self.status())
        self.require(item(summary, "AC-2 ") == log_line("AC-2", "criterion_failed", "executor", again["AC-2"]),
                     "unmet-summary-missing", summary)

    def test_command_runs_from_the_directory_it_names(self):
        _, seqs = self.check(want={"AC-1": "pass", "AC-2": "fail"})
        sub = self.repo / "src"
        summary = head_of(self.status(goal="../" + GOAL, cwd=sub))
        line = item(summary, "AC-2 ")
        command = re.search(r"`(ha log [^`]+)`", line)
        opened = None
        if command:
            words = command.group(1).split()
            opened = self.run_ha(*words[1:], cwd=sub)
        self.require(line == log_line("AC-2", "criterion_failed", "executor", seqs["AC-2"], goal="../" + GOAL)
                     and opened is not None and opened.returncode == 0 and SECRET in opened.stdout,
                     "unmet-summary-missing", f"{summary}\n{opened.stdout if opened else ''}")


class RecordReasons(Observed):
    """Reasons that rest on error records or on an invalid stored record."""

    def test_error_limit_points_to_the_latest_error(self):
        # Each blocked output makes the attempted run an error without a log.
        self.block_output(2)
        self.block_output(3)
        self.check("AC-2", want={"AC-2": "error"})
        _, last = self.check("AC-2", want={"AC-2": "error"})
        summary = head_of(self.status())
        self.require(item(summary, "AC-2 ") == f"- AC-2 `error_limit` (user): seq {last['AC-2']}, no stored output",
                     "unmet-summary-missing", summary)

    def test_invalid_record_points_to_that_record(self):
        _, seqs = self.check("AC-2", want={"AC-2": "fail"})
        records = self.repo / GOAL / "runs.jsonl"
        lines = records.read_bytes().splitlines(keepends=True)
        # The last line can change without breaking the chain; an unknown
        # result value makes that stored record invalid.
        if len(lines) != seqs["AC-2"] or lines[-1].count(b'"result":"fail"') != 1:
            raise Unavailable("unexpected check record")
        lines[-1] = lines[-1].replace(b'"result":"fail"', b'"result":"invented"')
        records.write_bytes(b"".join(lines))
        summary = head_of(self.status())
        self.require(item(summary, "AC-2 ") == log_line("AC-2", "record_invalid", "user", seqs["AC-2"]),
                     "unmet-summary-missing", summary)


class GoalReasonClass(Observed):
    def test_goal_level_user_reason_shows_its_class(self):
        self.check(want={"AC-1": "pass", "AC-2": "fail"})
        records = self.repo / GOAL / "runs.jsonl"
        lines = records.read_bytes().splitlines(keepends=True)
        if len(lines) < 2 or lines[0].count(b'"quote":"') != 1:
            raise Unavailable("unexpected start record")
        # Editing an earlier line breaks the next line's link to it.
        lines[0] = lines[0].replace(b'"quote":"', b'"quote":"edited ')
        records.write_bytes(b"".join(lines))
        summary = head_of(self.status())
        self.require(item(summary, "`record_integrity` ") == "- `record_integrity` (user)",
                     "unmet-summary-missing", summary)


class BaselineReason(Observed):
    """A change condition whose baseline already passes: the reason rests on that baseline.

    Plain script checks cannot establish run-rules/3 baselines, so the start
    line, still the only record, is rewritten to run-rules/2 as the Go
    scenario runner does. Rewriting the last line keeps the hash chain intact.
    """

    def fixture_files(self):
        files = super().fixture_files()
        files[f"{GOAL}/SPEC.md"] = SPEC.replace("## Open Decisions", "- **AC-3** a vacuous check.\n\n## Open Decisions")
        files[f"{GOAL}/PLAN.md"] = PLAN.format(greeting="sh tests/greeting.sh").replace(
            "| AC-2 | maintain |", "| AC-3 | change | `sh tests/vacuous.sh` | tests/vacuous.sh | T001 |\n| AC-2 | maintain |")
        files[f"{GOAL}/PLAN.md"] = files[f"{GOAL}/PLAN.md"].replace("| AC-1, AC-2 |", "| AC-1, AC-2, AC-3 |")
        files["tests/vacuous.sh"] = "exit 0\n"
        return files

    def after_start(self):
        records = self.repo / GOAL / "runs.jsonl"
        lines = records.read_bytes().splitlines(keepends=True)
        if len(lines) != 1 or lines[0].count(b'"rules":"run-rules/3"') != 1:
            raise Unavailable("unexpected start record")
        records.write_bytes(lines[0].replace(b'"rules":"run-rules/3"', b'"rules":"run-rules/2"'))

    def test_baseline_reason_points_to_its_baseline(self):
        result = self.run_ha("check", GOAL, "--baseline", "AC-3", expect=(1,))
        match = re.search(r"^AC-3 baseline unexpected_pass \(seq (\d+)\)$", result.stdout, re.M)
        if not match:
            raise Unavailable("baseline did not pass unexpectedly: " + result.stdout)
        self.check("AC-3", want={"AC-3": "pass"})
        summary = head_of(self.status())
        self.require(item(summary, "AC-3 ") == log_line("AC-3", "baseline_unexpected_pass", "executor",
                                                        match.group(1)),
                     "unmet-summary-missing", summary)


class MissingOutput(Observed):
    def test_condition_without_a_fresh_record_gets_no_seq(self):
        self.check("AC-1", want={"AC-1": "pass"})
        summary = head_of(self.status())
        self.require(item(summary, "AC-2 ") == "- AC-2 `criterion_missing` (executor): no fresh record",
                     "no-record-state-missing", summary)

    def test_stale_record_is_not_cited(self):
        self.check(want={"AC-1": "pass", "AC-2": "fail"})
        self.change_code()
        summary = head_of(self.status())
        self.require(item(summary, "AC-2 ") == "- AC-2 `criterion_missing` (executor): no fresh record",
                     "no-record-state-missing", summary)

    def test_record_without_stored_output_gets_no_log_command(self):
        self.block_output(2)
        _, seqs = self.check("AC-2", want={"AC-2": "error"})
        if seqs["AC-2"] != 2:
            raise Unavailable(f"unexpected record sequence: {seqs}")
        summary = head_of(self.status())
        self.require(item(summary, "AC-2 ") == "- AC-2 `criterion_error` (executor): seq 2, no stored output",
                     "no-output-state-missing", summary)


class InvalidatedCompletion(Observed):
    def invalidated(self, seq, commit, reason):
        line = completion_line(self.status())
        self.require(f"latest completion seq {seq} (commit {commit[:12]}) is no longer valid: " in line
                     and reason in line, "invalidated-completion-hidden", line)

    def test_code_change_after_completion(self):
        seq, commit = self.complete()
        self.change_code()
        self.invalidated(seq, commit, "code outside the bundle documents changed")

    def test_reopen_after_completion(self):
        seq, commit = self.complete()
        note = self.run_ha("note", GOAL, "reopen", "--quote", "느낌표도 붙여줘", expect=(0,))
        match = re.search(r"seq (\d+)", note.stdout)
        if not match:
            raise Unavailable("reopen note printed no seq: " + note.stdout)
        self.invalidated(seq, commit, f"reopened at seq {match.group(1)}")

    def test_goal_document_change_after_completion(self):
        seq, commit = self.complete()
        with (self.repo / GOAL / "SPEC.md").open("a", encoding="utf-8") as spec:
            spec.write("\nOne more sentence.\n")
        self.commit("edit the goal")
        self.invalidated(seq, commit, "goal documents changed")

    def test_broken_record_chain_after_completion(self):
        seq, commit = self.complete()
        records = self.repo / GOAL / "runs.jsonl"
        lines = records.read_bytes().splitlines(keepends=True)
        if lines[0].count(b'"quote":"') != 1:
            raise Unavailable("unexpected start record")
        # Editing an earlier line breaks the next line's link to it.
        lines[0] = lines[0].replace(b'"quote":"', b'"quote":"edited ')
        records.write_bytes(b"".join(lines))
        self.invalidated(seq, commit, "record chain broken")

    def test_latest_done_of_any_state_is_named(self):
        seq, commit = self.complete()
        self.change_code()
        again = self.run_ha("done", GOAL, expect=(1,)).stdout
        if "- status: **incomplete**" not in again:
            raise Unavailable("the second done did not record incomplete: " + again)
        latest = len((self.repo / GOAL / "runs.jsonl").read_bytes().splitlines())
        head = self.git("rev-parse", "HEAD").strip()
        line = completion_line(self.status())
        self.require(f"latest done seq {latest} (commit {head[:12]}) recorded incomplete, not a completion" in line
                     and f"latest completion seq {seq} (commit {commit[:12]}) is no longer valid" in line,
                     "invalidated-completion-hidden", line)

    def test_command_change_after_completion(self):
        seq, commit = self.complete()
        (self.repo / GOAL / "PLAN.md").write_text(PLAN.format(greeting="sh ./tests/greeting.sh"), encoding="utf-8")
        self.commit("change the command")
        self.invalidated(seq, commit, "required command changed: AC-2")


class DoneForms(Observed):
    """Done records that never completed, and the forms that stay as they are."""

    def test_only_an_unfinished_done_is_named(self):
        self.check(want={"AC-1": "pass", "AC-2": "fail"})
        self.run_ha("done", GOAL, expect=(1,))
        latest = len((self.repo / GOAL / "runs.jsonl").read_bytes().splitlines())
        head = self.git("rev-parse", "HEAD").strip()
        line = completion_line(self.status())
        self.require(line == f"- completion record: none; latest done seq {latest} (commit {head[:12]}) "
                             "recorded incomplete, not a completion",
                     "invalidated-completion-hidden", line)
        # A goal without done records keeps `none`, and one that only needs
        # `ha done` keeps its hint.
        self.tearDown()
        self.setUp()
        self.require(completion_line(self.status()) == "- completion record: none",
                     "invalidated-completion-hidden", "a goal without done records changed its line")
        self.say_hello()
        self.check(want={"AC-1": "pass", "AC-2": "pass"})
        self.require(completion_line(self.status()) == f"- completion record: none; run `ha done {GOAL}` to record completion",
                     "invalidated-completion-hidden", "the ha done hint changed")


# An existing scenario on the same `implement` setup whose output this goal
# does not change; it separates fixture-environment failures from results.
CONTROL = "complete-happy-path"
# The reason phrases the scenario must pin (contract §5.1 order, AC-3).
STALE_REASONS = ("is no longer valid: ", "code outside the bundle documents changed", "goal documents changed",
                 "required command changed: ", "reopened at seq ", "record chain broken")


def scenario_lines(file, depth=0):
    """The step lines of a scenario file, as cmd/ha/scenario_test.go loads them."""
    if depth > 3:
        raise Unavailable(f"include nesting too deep at {file}")
    try:
        text = file.read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"scenario steps: {error}") from None
    out = []
    for raw in text.splitlines():
        line = raw.strip()
        if not line or line.startswith("#"):
            continue
        words = line.split()
        if words[0] == "include" and len(words) > 1:
            out += scenario_lines(ROOT / "testdata/scenarios/_steps" / words[1], depth + 1)
            continue
        out.append(line)
    return out


class ScenarioPinsReasons(unittest.TestCase):
    """The behavioral scenario that pins stale-completion reasons exists and passes."""

    def test_completion_invalidated_scenario_passes(self):
        # Expectations are read the way the Go runner reads them: trimmed lines,
        # with included `_steps` files expanded.
        steps = ROOT / "testdata/scenarios/completion-invalidated/steps"
        lines = scenario_lines(steps) if steps.is_file() else []
        pinned = "\n".join(line for line in lines if line.startswith("expect-output "))
        unpinned = [reason for reason in STALE_REASONS if reason not in pinned]
        if unpinned:
            self.fail(f"[violation:stale-reason-scenario-missing] the scenario does not pin: {unpinned}")
        if not (shutil.which("git") and shutil.which("sh")):
            raise Unavailable("the scenario runner needs git and sh")
        env = dict(os.environ, GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
        try:
            result = subprocess.run(["go", "test", "-count=1", "-v", "-run",
                                     f"^TestScenarios$/^(completion-invalidated|{CONTROL})$", "./cmd/ha"],
                                    cwd=ROOT, env=env, capture_output=True, text=True, timeout=900)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Unavailable(f"go test: {error}") from None
        # A control scenario with the same `implement` setup and fixture
        # commands runs alongside. If it fails before any expectation in its
        # own steps file, the fixture environment (Git, sh, grep, ...) or the
        # shared setup is the cause. A failure at its own expectation is a
        # product result and does not hide the scenario's assertions.
        if (f"--- PASS: TestScenarios/{CONTROL}" not in result.stdout
                and not re.search(rf"{CONTROL}/steps:\d+: expect-", result.stdout)):
            raise Unavailable(f"the fixture environment failed the control scenario {CONTROL}: "
                              + result.stdout[-4000:] + result.stderr[-2000:])
        # With a working fixture, the scenario's own result is a failed
        # expectation in its own steps file, or a failed reason assertion
        # wherever it is pinned. The runner reports the latter as
        # `output does not contain "<text>"` with the text Go-quoted on one
        # line. Shared `_steps` setup expectations, Git and fixture steps that
        # fail are neither.
        failed = [line.split('output does not contain "', 1)[1] for line in result.stdout.splitlines()
                  if 'output does not contain "' in line]
        if ("--- FAIL: TestScenarios/completion-invalidated" in result.stdout
                and (re.search(r"completion-invalidated/steps:\d+: expect-", result.stdout)
                     or any(reason in text for text in failed for reason in STALE_REASONS))):
            self.fail("[violation:stale-reason-scenario-missing] " + result.stdout[-4000:])
        if result.returncode != 0 or "--- PASS: TestScenarios/completion-invalidated" not in result.stdout:
            # Toolchain, module, build or setup failures never reach the scenario.
            raise Unavailable("scenario tests could not run: " + result.stdout + result.stderr)


class CheckOutput(Observed):
    def test_failed_check_names_the_log_of_each_failed_target(self):
        (self.repo / "src/keep.txt").write_text("changed\n", encoding="utf-8")
        self.commit("break keep")
        result, seqs = self.check(want={"AC-1": "fail", "AC-2": "fail"})
        lines = result.stdout.splitlines()
        self.require(f"output for AC-1: ha log {GOAL} {seqs['AC-1']}" in lines
                     and f"output for AC-2: ha log {GOAL} {seqs['AC-2']}" in lines
                     and "<seq>" not in result.stdout,
                     "check-log-command-missing", result.stdout)

    def test_passing_targets_get_no_log_command_and_commands_run_where_named(self):
        sub = self.repo / "src"
        result, seqs = self.check(want={"AC-1": "pass", "AC-2": "fail"}, goal="../" + GOAL, cwd=sub)
        lines = result.stdout.splitlines()
        named = f"output for AC-2: ha log ../{GOAL} {seqs['AC-2']}"
        opened = self.run_ha("log", "../" + GOAL, str(seqs["AC-2"]), cwd=sub) if named in lines else None
        absolute, more = self.check("AC-2", want={"AC-2": "fail"}, goal=str(self.repo / GOAL))
        self.require(named in lines and not any(line.startswith("output for AC-1") for line in lines)
                     and opened is not None and SECRET in opened.stdout
                     and f"output for AC-2: ha log {GOAL} {more['AC-2']}" in absolute.stdout.splitlines()
                     and str(self.repo) not in absolute.stdout,
                     "check-log-command-missing", result.stdout + absolute.stdout)

    def test_target_without_stored_output_is_named(self):
        self.block_output(2)
        result, seqs = self.check("AC-2", want={"AC-2": "error"})
        if seqs["AC-2"] != 2:
            raise Unavailable(f"unexpected record sequence: {seqs}")
        lines = result.stdout.splitlines()
        self.require("output for AC-2: no stored output (seq 2)" in lines and "ha log" not in result.stdout,
                     "check-no-output-state-missing", result.stdout)


class Preserved(Observed):
    """Judgment, JSON, exit codes, done, log, per-target lines, table and reasons keep their meaning."""

    def test_judgment_json_and_exit_codes(self):
        result = self.run_ha("check", GOAL, expect=(0, 1))
        if any(m.group(3) == "error" for m in RESULT_LINE.finditer(result.stdout)):
            # A fixture process that could not run says nothing about the line format.
            raise Unavailable("fixture checks errored: " + result.stdout + result.stderr)
        lines = result.stdout.splitlines()
        self.require(result.returncode == 1 and lines[:2] == ["AC-1 check pass (seq 2)", "AC-2 check fail (seq 3)"],
                     "check-result-lines-changed", result.stdout)
        seqs = {"AC-1": 2, "AC-2": 3}
        status = self.run_ha("status", GOAL, "--format", "json", expect=(1,))
        report = json.loads(status.stdout)
        criteria = {c["id"]: (c["satisfied"], c["reason"]) for c in report["criteria"]}
        codes = sorted(r["code"] for r in report["reasons"])
        self.require(set(report) == STATUS_FIELDS and report["status"] == "incomplete" and report["exit_code"] == 1
                     and report["completion_record"] is None
                     and criteria == {"AC-1": (True, ""), "AC-2": (False, "criterion_failed")}
                     and codes == ["criterion_failed", "task_open"],
                     "status-json-changed", status.stdout)
        markdown = self.status()
        reasons = markdown.split("### Reasons\n\n", 1)[1].split("\n\n", 1)[0].splitlines() \
            if "### Reasons\n\n" in markdown else []
        self.require(re.search(r"^\| AC-2 \| maintain \| yes \| no \| criterion_failed \| — \| checks 1 \(fail 1, error 0\)",
                               markdown, re.M)
                     and re.search(r"^\| AC-1 \| maintain \| yes \| yes \| — \| \d+ \|", markdown, re.M)
                     and reasons[:1] == ["- `criterion_failed` (executor) AC-2"]
                     and len(reasons) == 2 and reasons[1].startswith("- `task_open` (executor)"),
                     "status-table-or-reasons-changed", markdown)
        log = self.run_ha("log", GOAL, str(seqs["AC-2"]), expect=(0,)).stdout
        tail = self.run_ha("log", GOAL, str(seqs["AC-2"]), "--tail", "1", expect=(0,)).stdout
        self.require(SECRET in log and ENV_SECRET in log and SECRET not in tail and ENV_SECRET in tail,
                     "log-output-changed", log + tail)

    def test_done_output_and_completion(self):
        seq, _ = self.complete()
        status = self.run_ha("status", GOAL, "--format", "json", expect=(0,))
        report = json.loads(status.stdout)
        self.require(report["status"] == "complete" and report["completion_record"] == seq
                     and set(report) == STATUS_FIELDS,
                     "status-json-changed", status.stdout)
        check, _ = self.check(want={"AC-1": "pass", "AC-2": "pass"})
        self.require(check.returncode == 0 and "output for" not in check.stdout, "check-result-lines-changed",
                     check.stdout)


class Deterministic(Observed):
    def test_human_output_is_stable_and_contains_no_output_paths_or_values(self):
        self.complete()
        (self.repo / "src/greeting.txt").write_text("hi again\n", encoding="utf-8")
        self.commit("break the greeting")
        self.check(want={"AC-1": "pass", "AC-2": "fail"})
        first = self.status()
        time.sleep(1.1)
        second = self.status()
        absolute = self.status(goal=str(self.repo / GOAL))
        leaked = [value for value in (SECRET, ENV_SECRET, str(self.base), str(self.home), str(self.temporary))
                  if value in first or value in absolute]
        self.require(first == second and not leaked, "status-output-unstable",
                     f"leaked: {leaked}; equal: {first == second}")


def section(text, heading):
    """Return the body under a heading up to the next heading of the same level, or ""."""
    level = len(heading) - len(heading.lstrip("#"))
    lines = text.splitlines()
    try:
        start = lines.index(heading) + 1
    except ValueError:
        return ""
    end = start
    fence = False
    while end < len(lines):
        if lines[end].startswith("```"):
            fence = not fence
        match = re.match(r"(#{1,6}) ", lines[end])
        if match and not fence and len(match.group(1)) <= level:
            break
        end += 1
    return "\n".join(lines[start:end])


class Documents(unittest.TestCase):
    """Fixed phrases and a synthetic example in the contract and the Seal reference; no build."""

    def test_contract_and_reference_describe_the_new_output(self):
        contract = section((ROOT / "contracts/run-record.md").read_text(encoding="utf-8"), "### 4.4 출력")
        reference = section((ROOT / "plugins/seal/skills/seal/references/ha.md").read_text(encoding="utf-8"),
                            "## Status output")
        missing = ([f"run-record.md §4.4: {p}" for p in (
                        "미충족 요약", "`ha log <goal> <seq>`", "no stored output", "no fresh record",
                        "무효가 된 이유", "output for <대상>: ha log <goal> <seq>", "판정·`status/v1`",
                        "합성 예시", "```text", "docs/specs/example")
                    if p not in contract]
                   + [f"references/ha.md Status output: {p}" for p in (
                        "unmet summary", "`ha log <goal> <seq>`", "no stored output", "no fresh record",
                        "no longer valid", "output for <target>: ha log <goal> <seq>", "`status/v1`",
                        "synthetic example", "```text", "docs/specs/example")
                      if p not in reference])
        for name, body in (("run-record.md §4.4", contract), ("references/ha.md Status output", reference)):
            # Every goal the section names, in prose or in any example, is the synthetic one.
            # Goal names are kebab-case ASCII; a trailing period, particle or
            # other punctuation is not part of the name, an inner period is.
            paths = {match.group(1) or match.group(0)
                     for match in re.finditer(r"docs/specs/(?:([A-Za-z0-9._-]*[A-Za-z0-9_-])|[^\x00-\x7f])", body)}
            if paths - {"example"}:
                missing.append(f"{name}: names a goal other than docs/specs/example: {sorted(paths)}")
        if missing:
            self.fail("[violation:status-output-docs-missing] " + "\n".join(missing))
