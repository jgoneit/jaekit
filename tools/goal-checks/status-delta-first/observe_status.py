"""Observe `ha status` and failed `ha check` output through the real CLI.

`ha` is built from this checkout once per run and driven against synthetic
Git repositories in temporary directories. Each goal has two maintain
conditions: AC-1 passes on the fixture and AC-2 passes only after
`src/greeting.txt` says hello. Nothing reads private documents, the real HOME
or global Git configuration. A failed build, Git or `ha` invocation that
cannot run is an error, never a violation; only an observed output property is.
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
    """A tool or fixture step could not run; reported as an error."""


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

    def run_ha(self, *args, expect=None):
        try:
            result = subprocess.run([str(self.ha), *args], cwd=self.repo, env=self.env, capture_output=True,
                                    text=True, timeout=300)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Unavailable(f"ha {args[0]}: {error}") from None
        if expect is not None and result.returncode not in expect:
            raise Unavailable(f"ha {' '.join(args)} exited {result.returncode}: {result.stdout}{result.stderr}")
        return result

    def check(self, *targets):
        result = self.run_ha("check", GOAL, *targets, expect=(0, 1))
        seqs = {m.group(1): int(m.group(2))
                for m in re.finditer(r"^(\S+) check \S+ \(seq (\d+)\)$", result.stdout, re.M)}
        return result, seqs

    def status(self):
        return self.run_ha("status", GOAL, expect=(0, 1, 2, 3, 4)).stdout

    def say_hello(self):
        (self.repo / "src/greeting.txt").write_text("hello\n", encoding="utf-8")
        (self.repo / GOAL / "PROGRESS.md").write_text(PROGRESS.format(state="done"), encoding="utf-8")
        self.commit("say hello")

    def complete(self):
        """Record a completion and return its seq and commit."""
        self.say_hello()
        self.check()
        done = self.run_ha("done", GOAL, expect=(0,)).stdout
        match = re.search(r"^- completion record: seq (\d+)$", done, re.M)
        if not match:
            raise Unavailable("ha done did not record a completion: " + done)
        return int(match.group(1)), self.git("rev-parse", "HEAD").strip()

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


class UnmetSummary(Observed):
    def test_unmet_conditions_and_goal_reasons_come_before_the_table(self):
        _, seqs = self.check()
        summary = head_of(self.status())
        failed = item(summary, "AC-2 ")
        open_task = item(summary, "`task_open`")
        self.require(all(part in failed for part in ("`criterion_failed`", "(executor)",
                                                     f"`ha log {GOAL} {seqs['AC-2']}`"))
                     and "(executor)" in open_task and not item(summary, "AC-1 "),
                     "unmet-summary-missing", summary)

    def test_summary_points_to_the_latest_fresh_record(self):
        self.check()
        _, again = self.check("AC-2")
        summary = head_of(self.status())
        failed = item(summary, "AC-2 ")
        self.require(f"`ha log {GOAL} {again['AC-2']}`" in failed and failed.count("ha log") == 1,
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
        self.check("AC-3")
        summary = head_of(self.status())
        line = item(summary, "AC-3 ")
        self.require("`baseline_unexpected_pass`" in line and f"`ha log {GOAL} {match.group(1)}`" in line,
                     "unmet-summary-missing", summary)


class MissingOutput(Observed):
    def test_condition_without_a_fresh_record_gets_no_seq(self):
        self.check("AC-1")
        summary = head_of(self.status())
        missing = item(summary, "AC-2 ")
        self.require("`criterion_missing`" in missing and "no fresh record" in missing
                     and "seq" not in missing and "ha log" not in missing,
                     "no-record-state-missing", summary)

    def test_record_without_stored_output_gets_no_log_command(self):
        self.block_output(2)
        _, seqs = self.check("AC-2")
        if seqs.get("AC-2") != 2:
            raise Unavailable(f"unexpected record sequence: {seqs}")
        summary = head_of(self.status())
        errored = item(summary, "AC-2 ")
        self.require("seq 2" in errored and "no stored output" in errored and "ha log" not in errored,
                     "no-output-state-missing", summary)


class InvalidatedCompletion(Observed):
    def invalidated(self, seq, commit, reason):
        line = completion_line(self.status())
        self.require(f"seq {seq}" in line and commit[:12] in line and "no longer valid" in line and reason in line,
                     "invalidated-completion-hidden", line)

    def test_code_change_after_completion(self):
        seq, commit = self.complete()
        (self.repo / "src/extra.txt").write_text("x\n", encoding="utf-8")
        self.commit("more code")
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

    def test_command_change_after_completion(self):
        seq, commit = self.complete()
        (self.repo / GOAL / "PLAN.md").write_text(PLAN.format(greeting="sh ./tests/greeting.sh"), encoding="utf-8")
        self.commit("change the command")
        self.invalidated(seq, commit, "required command changed: AC-2")
        # Existing forms stay: no completion record at all, and a goal that only
        # needs `ha done` (checked on a fresh repository below).
        self.tearDown()
        self.setUp()
        self.require(completion_line(self.status()) == "- completion record: none",
                     "invalidated-completion-hidden", "a goal without done records changed its line")
        self.say_hello()
        self.check()
        self.require(completion_line(self.status()) == f"- completion record: none; run `ha done {GOAL}` to record completion",
                     "invalidated-completion-hidden", "the ha done hint changed")


class CheckOutput(Observed):
    def test_failed_check_names_the_log_of_each_failed_target(self):
        result, seqs = self.check()
        out = result.stdout
        self.require(f"ha log {GOAL} {seqs['AC-2']}" in out and "<seq>" not in out
                     and f"ha log {GOAL} {seqs['AC-1']}" not in out,
                     "check-log-command-missing", out)

    def test_target_without_stored_output_is_named(self):
        self.block_output(2)
        result, seqs = self.check("AC-2")
        out = result.stdout
        self.require(seqs.get("AC-2") == 2 and "no stored output" in item(out, "") + out
                     and f"ha log {GOAL} 2" not in out and "<seq>" not in out,
                     "check-no-output-state-missing", out)


class Preserved(Observed):
    """Judgment, JSON, exit codes, done, log and per-target lines keep their meaning."""

    def test_judgment_json_and_exit_codes(self):
        result, seqs = self.check()
        self.require(result.returncode == 1 and re.search(r"^AC-1 check pass \(seq \d+\)$", result.stdout, re.M)
                     and re.search(r"^AC-2 check fail \(seq \d+\)$", result.stdout, re.M),
                     "check-result-lines-changed", result.stdout)
        status = self.run_ha("status", GOAL, "--format", "json", expect=(1,))
        report = json.loads(status.stdout)
        criteria = {c["id"]: (c["satisfied"], c["reason"]) for c in report["criteria"]}
        codes = sorted(r["code"] for r in report["reasons"])
        self.require(set(report) == STATUS_FIELDS and report["status"] == "incomplete" and report["exit_code"] == 1
                     and report["completion_record"] is None
                     and criteria == {"AC-1": (True, ""), "AC-2": (False, "criterion_failed")}
                     and codes == ["criterion_failed", "task_open"],
                     "status-json-changed", status.stdout)
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
        check, _ = self.check()
        self.require(check.returncode == 0 and "ha log" not in check.stdout, "check-result-lines-changed",
                     check.stdout)


class Deterministic(Observed):
    def test_human_output_is_stable_and_contains_no_output_paths_or_values(self):
        seq, _ = self.complete()
        (self.repo / "src/greeting.txt").write_text("hi again\n", encoding="utf-8")
        self.commit("break the greeting")
        self.check()
        first = self.status()
        time.sleep(1.1)
        second = self.status()
        leaked = [value for value in (SECRET, ENV_SECRET, str(self.base), str(self.home), str(self.temporary))
                  if value in first]
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
    while end < len(lines):
        match = re.match(r"(#{1,6}) ", lines[end])
        if match and len(match.group(1)) <= level:
            break
        end += 1
    return "\n".join(lines[start:end])


class Documents(unittest.TestCase):
    """Fixed phrases in the output contract and the Seal reference; no build."""

    def require(self, condition, violation, detail=""):
        if not condition:
            self.fail(f"[violation:{violation}] {detail}")

    def test_contract_and_reference_describe_the_new_output(self):
        contract = section((ROOT / "contracts/run-record.md").read_text(encoding="utf-8"), "### 4.4 출력")
        reference = section((ROOT / "plugins/seal/skills/seal/references/ha.md").read_text(encoding="utf-8"),
                            "## Status output")
        missing = ([f"run-record.md §4.4: {p}" for p in (
                        "미충족 요약", "`ha log <goal> <seq>`", "no stored output", "no fresh record",
                        "무효가 된 이유", "output for <대상>: ha log <goal> <seq>", "판정·`status/v1`")
                    if p not in contract]
                   + [f"references/ha.md Status output: {p}" for p in (
                        "unmet summary", "`ha log <goal> <seq>`", "no stored output", "no fresh record",
                        "no longer valid", "output for <target>: ha log <goal> <seq>", "`status/v1`")
                      if p not in reference])
        self.require(not missing, "status-output-docs-missing", "\n".join(missing))
