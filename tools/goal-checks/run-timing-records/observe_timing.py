"""Process timing in run records, the time view and their compatibility.

Every observation builds `ha` from the checked tree and runs that executable
on a synthetic goal repository made from testdata/scenarios/_base, in a
temporary directory. Some builds lay a few files over the checked tree with
`go build -overlay`. The short-limit build sets the fixed time limit in
`cmd/ha/main.go` to a few seconds, so a timed-out run is recorded in seconds
instead of fifteen minutes, and pauses in the process cleanup of
`internal/run/process_unix.go`, after the limit interval ends. The CPU-failure
build adds a file to `internal/run` that makes every CPU collection fail
(AC-7). AC-2 also runs a Linux build in the
repository's local check container (`jaekit-check-git`, never pulled, no
network) and, for Windows, runs the repository's CI reader
(tools/verification-checks/cross_platform_ci.py AC-1) on the real pull request
run of the checked commit, accepting only its pass. AC-5 also builds the
released v0.1.4 from its tag, the ref the criterion names. Nothing else reads
history, and the checked tree is never written.

Violations come only from explicit `[violation:...]` failures. A tool, image,
tag, CI run or GitHub response that is missing or unusable raises Unavailable,
an execution error.
"""
import hashlib
import importlib.util
import json
import math
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import time
import unittest

ROOT = Path(__file__).resolve().parents[3]
BASE = ROOT / "testdata/scenarios/_base"
GOAL = "docs/specs/greet"
LIMIT_MS = 900000
# The fixed limit as the checked tree writes it, and the shortened build's.
LIMIT_LINE = "const checkTimeout = 900 * time.Second"
SHORT_LIMIT_MS = 3000
# A timed-out run's limit interval ends at the timeout decision: within this
# much of the limit, however long the cleanup after it takes.
DECISION_SLACK_MS = 1000
# The short-limit build pauses this long when it kills a timed-out process
# group and again when it finishes any process. Both come after the limit
# interval ends, so they separate it from duration_ms (AC-1).
CLEANUP_PAUSE_MS = 1500
PROCESS_UNIX = "internal/run/process_unix.go"
KILL_LINE = "func (g *processGroup) kill() error {"
FINISH_LINE = "func (g *processGroup) finish() error { return nil }"
PAUSE_SOURCE = f"""//go:build !windows

package run

import "time"

// Added by the run-timing-records observer: a cleanup that takes a known time.
func observedCleanupPause() {{ time.Sleep({CLEANUP_PAUSE_MS} * time.Millisecond) }}
"""
# A file the CPU-failure build adds to internal/run: the implementation's
# collection seam (PLAN 판단 근거) fails for every process (AC-7).
CPU_FAILURE = {"internal/run/zz_observed_cpu_failure.go": """package run

import "os/exec"

// Added by the run-timing-records observer: every CPU collection fails.
func init() {
	collectCPU = func(*exec.Cmd, processController) CPU { return CPU{Status: CPUFailed} }
}
"""}
# Shell loops that use measurable processor time (EX-3) and outlast the short
# limit (EX-4).
BURN = "i=0\nwhile [ $i -lt 400000 ]; do i=$((i+1)); done\n"
SLEEP = "sleep 20\n"
REQUEST = "타이밍 관측용 합성 목표를 구현해줘"
# Strings a run's environment, output and location carry; none may reach a
# timing field or the time view (AC-9).
SECRET = "jaekit-observed-environment-value"
OUTPUT = "JAEKIT-OBSERVED-COMMAND-OUTPUT"
# The executable's identity at the goal's base commit (AC-8).
BASE_FEATURES = ["estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1",
                 "budget-change/v1", "post-completion-findings/v1", "decision-history/v1"]
BASE_FORMATS = {"finding": ["run-finding/v1"], "decision_history": ["decision-history/v1"],
                "criteria": ["legacy", "nested/1"], "check_declaration": ["check-declaration/v1"],
                "check_result": ["check-result/v1"], "check_evidence": ["check-evidence/v1"]}
BASE_RULES = ["run-rules/1", "run-rules/2", "run-rules/3"]
# `ha estimate --format json` for the synthetic goal at the goal's base commit,
# without the goal path (AC-8).
BASE_ESTIMATE = '{"budget_source": "PLAN.md", "default_additional_runs": 0, "default_baseline_runs": 1, "default_check_runs": 2, "headroom": 47, "limitation": "Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run\'s remaining budget; existing records and budget windows are unchanged.", "minimum_runs": 3, "plan_runs_limit": 50, "required_change": 1, "required_maintain": 1, "schema": "estimate/v1", "selected": {"additional_runs": 0, "mode": "check", "optional_runs": 0, "repeated_required_runs": 0, "runs": 2, "targets": ["AC-1", "AC-2"], "task_runs": 0}, "shortfall": 0}'
# Values every time view row shows, either as a value or as unavailable.
VIEW_VALUES = ("limit_ms", "limit_elapsed_ms", "limit_reached", "cpu")


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def run(argv, cwd, env=None, timeout=600, stdin=None):
    try:
        return subprocess.run([str(a) for a in argv], cwd=cwd, env=env, capture_output=True, timeout=timeout,
                              stdin=subprocess.DEVNULL if stdin is None else stdin)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unavailable(f"{argv[0]}: {error}") from None


def build(out, tree=ROOT, goos=None, goarch=None, overlay=None):
    env = dict(os.environ, CGO_ENABLED="0")
    for key, value in (("GOOS", goos), ("GOARCH", goarch)):
        if value:
            env[key] = value
        else:
            env.pop(key, None)
    flags = ["-overlay", overlay] if overlay else []
    result = run(["go", "build", *flags, "-o", out, "./cmd/ha"], tree, env, timeout=600)
    if result.returncode:
        raise Unavailable(f"go build in {tree.name}: {result.stderr.decode(errors='replace')[-2000:]}")
    return out


def tree_text(path):
    try:
        return (ROOT / path).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"{path}: {error}") from None


def build_overlaid(out, files):
    """`ha` from the checked tree with files ({tree path: content}) laid over it."""
    out = Path(out)
    out.parent.mkdir(parents=True, exist_ok=True)
    replace = {}
    for index, (path, content) in enumerate(sorted(files.items())):
        laid = out.parent / f"overlay-{index}-{Path(path).name}"
        laid.write_text(content, encoding="utf-8")
        replace[str(ROOT / path)] = str(laid)
    overlay = out.parent / "overlay.json"
    overlay.write_text(json.dumps({"Replace": replace}), encoding="utf-8")
    return build(out, overlay=overlay)


def short_files():
    """The short-limit build's files: a few-second limit and a known cleanup pause."""
    main, process = tree_text("cmd/ha/main.go"), tree_text(PROCESS_UNIX)
    if main.count(LIMIT_LINE) != 1:
        raise Unavailable("cmd/ha/main.go does not declare the fixed limit as one constant")
    if process.count(KILL_LINE) != 1 or process.count(FINISH_LINE) != 1:
        raise Unavailable(f"{PROCESS_UNIX} does not kill and finish a process group as the pause expects")
    return {"cmd/ha/main.go": main.replace(LIMIT_LINE, f"const checkTimeout = {SHORT_LIMIT_MS} * time.Millisecond"),
            PROCESS_UNIX: process.replace(KILL_LINE, KILL_LINE + "\n\tobservedCleanupPause()").replace(
                FINISH_LINE, "func (g *processGroup) finish() error { observedCleanupPause(); return nil }"),
            "internal/run/zz_observed_cleanup_pause.go": PAUSE_SOURCE}


def build_short(out, extra=None):
    """`ha` from the checked tree with the short limit and cleanup pause (and extra files)."""
    return build_overlaid(out, {**short_files(), **(extra or {})})


def digest_tree(path):
    found = {}
    for item in sorted(Path(path).rglob("*")):
        if item.is_file():
            found[str(item.relative_to(path))] = hashlib.sha256(item.read_bytes()).hexdigest()
    return found


class Fixture:
    """A synthetic goal repository and the record `ha` executables make in it.

    Legacy rules (`/1`, `/2`) check AC-1 by exit code. The default rule (`/3`)
    checks it with the repository's reference producer and a structured
    result declaration, so both record a change baseline and check.
    """

    PLAN = """# PLAN — greet

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | {ac1} | T001 |
| AC-2 | maintain | `sh tests/keep.sh` | — | T001 |
| EX-1 | maintain | `sh tests/fails.sh` | — | T001 |
| EX-2 | maintain | `jaekit-timing-missing-checker` | — | T001 |
| EX-3 | maintain | `sh tests/burn.sh` | — | T001 |
| EX-4 | maintain | `sh tests/sleep.sh` | — | T001 |
{contract}
## 범위
- 바꿀 수 있는 경로: `src/**`, `tests/**`, `checks/**` (SPEC: 범위와 비목표)
- 테스트 경로: (기본값)
- 유지할 동작: `src/keep.txt`

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | greeting says hello and keep stays | AC-1, AC-2 | — | — | 예 | tasks/T001.md |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측
"""
    LEGACY_AC1 = "`sh tests/greeting.sh` | tests/greeting.sh"
    STRUCTURED_AC1 = ("`python3 checks/reference.py checks/declaration.json checks/reference.json` | "
                      "checks/reference.py, checks/reference.json, checks/declaration.json")
    CONTRACT = "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
    TASK = "# T001\n\n## 목표\ngreeting says hello\n\n## 완료 조건\n- AC-1과 AC-2가 통과한다.\n- `sh tests/keep.sh`\n"

    def __init__(self, directory, ha, rules="run-rules/2", short=None):
        self.root, self.ha_path, self.short = Path(directory) / "repo", Path(ha), short
        shutil.copytree(BASE, self.root)
        goal = self.root / GOAL
        structured = rules is None
        (goal / "PLAN.md").write_text(self.PLAN.format(
            ac1=self.STRUCTURED_AC1 if structured else self.LEGACY_AC1,
            contract=self.CONTRACT if structured else ""), encoding="utf-8")
        (goal / "tasks").mkdir()
        (goal / "tasks/T001.md").write_text(self.TASK, encoding="utf-8")
        (self.root / "tests/fails.sh").write_text(f"echo {OUTPUT}\nexit 3\n", encoding="utf-8")
        (self.root / "tests/keep.sh").write_text(f"echo {OUTPUT}\ngrep -qx keep src/keep.txt\n", encoding="utf-8")
        (self.root / "tests/burn.sh").write_text(BURN, encoding="utf-8")
        (self.root / "tests/sleep.sh").write_text(SLEEP, encoding="utf-8")
        (self.root / "tests/greeting.sh").write_text("grep -qx hello src/greeting.txt\n", encoding="utf-8")
        checks = self.root / "checks"
        checks.mkdir()
        try:
            shutil.copyfile(ROOT / "tools/check-result-reference.py", checks / "reference.py")
        except OSError as error:
            raise Unavailable(f"tools/check-result-reference.py: {error}") from None
        (checks / "declaration.json").write_text(json.dumps({
            "schema": "check-declaration/v1", "producer": "jaekit-reference", "producer_version": "1",
            "producer_paths": ["checks/reference.py", "checks/reference.json"],
            "targets": [{"id": "greeting", "violation": "missing-greeting"}]}), encoding="utf-8")
        (checks / "reference.json").write_text(json.dumps({"mode": "reference", "checks": [
            {"target": "greeting", "path": "src/greeting.txt", "equals": "hello\n", "violation": "missing-greeting",
             "missing": "violation"}]}), encoding="utf-8")
        self.env = {k: v for k, v in os.environ.items() if not k.startswith(("GIT_", "HA_"))}
        # Fixed dates make two fixtures' commits the same (AC-7 compares two records).
        self.env.update(JAEKIT_OBSERVED_SECRET=SECRET, GIT_AUTHOR_NAME="Observer", GIT_AUTHOR_EMAIL="observer@example.invalid",
                        GIT_COMMITTER_NAME="Observer", GIT_COMMITTER_EMAIL="observer@example.invalid",
                        GIT_AUTHOR_DATE="2026-01-01T00:00:00+00:00", GIT_COMMITTER_DATE="2026-01-01T00:00:00+00:00")
        # Record seqs of the runs the short-limit build made, and each step's exit.
        self.short_seqs, self.exits, self.lenient = set(), [], False
        self.git("init", "-q", "-b", "main")
        self.commit("base")
        self.rules = rules

    def git(self, *args):
        result = run(["git", *args], self.root, self.env, timeout=120)
        if result.returncode:
            raise Unavailable(f"git {args[0]}: {result.stderr.decode(errors='replace')}")
        return result.stdout.decode()

    def commit(self, message):
        self.git("add", "-A")
        self.git("commit", "-q", "-m", message)

    def ha(self, *args, ha=None):
        return run([ha or self.ha_path, *args], self.root, self.env, timeout=900)

    @property
    def records_path(self):
        return self.root / GOAL / "runs.jsonl"

    def records(self):
        try:
            return [json.loads(line) for line in self.records_path.read_bytes().splitlines()]
        except (OSError, ValueError) as error:
            raise Unavailable(f"runs.jsonl: {error}") from None

    def start(self):
        started = self.ha("start", GOAL, "--request", REQUEST, "--skill-name", "seal", "--skill-version", "0.1.0")
        if started.returncode:
            raise Unavailable(f"ha start: {started.stderr.decode(errors='replace')}")
        if self.rules:
            # The legacy rules keep a plain-exit baseline without a structured result.
            raw = self.records_path.read_bytes()
            self.records_path.write_bytes(re.sub(rb'"rules":"[^"]*"', b'"rules":"' + self.rules.encode() + b'"', raw))

    def flow(self):
        """A change baseline and check, passing, failing, unstartable, CPU-using and (with the
        short-limit build) timed-out and finished checks, a task command, and a refused check."""
        self.start()
        return self.flow_after_start()

    def flow_after_start(self):
        if self.rules is None:
            # An undecidable attempt: the producer cannot read its configuration and writes no report.
            config = (self.root / "checks/reference.json").read_bytes()
            (self.root / "checks/reference.json").write_text("{", encoding="utf-8")
            self.commit("break the producer configuration")
            self.expect(self.ha("check", GOAL, "AC-1"), 1, "undecidable check")
            (self.root / "checks/reference.json").write_bytes(config)
            self.commit("restore the producer configuration")
        self.expect(self.ha("check", GOAL, "--baseline", "AC-1"), 0, "baseline")
        (self.root / "src/greeting.txt").write_text("hello\n", encoding="utf-8")
        self.commit("say hello")
        self.expect(self.ha("check", GOAL, "AC-1", "AC-2"), 0, "checks")
        self.expect(self.ha("check", GOAL, "EX-1"), 1, "failing check")
        self.expect(self.ha("check", GOAL, "EX-2"), 1, "unstartable check")
        self.expect(self.ha("check", GOAL, "EX-3"), 0, "CPU-using check")
        self.expect(self.ha("check", GOAL, "T001"), 0, "task command")
        if self.short:
            self.expect(self.ha("check", GOAL, "EX-4", ha=self.short), 1, "timed-out check")
            self.short_seqs.add(self.records()[-1]["seq"])
            self.expect(self.ha("check", GOAL, "EX-1", ha=self.short), 1, "finished check, short-limit build")
            self.short_seqs.add(self.records()[-1]["seq"])
        before = self.records_path.read_bytes()
        (self.root / "src/keep.txt").write_text("changed\n", encoding="utf-8")
        refused = self.ha("check", GOAL, "AC-2")
        (self.root / "src/keep.txt").write_text("keep\n", encoding="utf-8")
        if refused.returncode != 65:
            raise Unavailable(f"a dirty check was not refused: {refused.returncode}")
        self.refusal_wrote = self.records_path.read_bytes() != before
        return self.records()

    def expect(self, result, code, what):
        """Keep each step's exit; a lenient fixture (AC-7) lets the caller compare them instead of stopping."""
        self.exits.append((what, result.returncode))
        if result.returncode != code and not self.lenient:
            raise Unavailable(f"ha {what} exited {result.returncode}, not {code}: "
                              + (result.stdout + result.stderr).decode(errors="replace")[-2000:])


def short_fixture(directory, rules=None):
    """A fixture whose flow also records a timed-out run with the short-limit build."""
    base = Path(directory)
    base.mkdir(parents=True, exist_ok=True)
    return Fixture(base, build(base / "ha"), rules=rules, short=build_short(base / "short" / "ha"))


def runs_of(records):
    return [r for r in records if r.get("kind") in ("check", "baseline")]


def label(line):
    target = line.get("target") or {}
    return f"seq {line.get('seq')} {target.get('criterion') or target.get('task')} {line.get('kind')}"


def criterion(line):
    return (line.get("target") or {}).get("criterion")


def started(line):
    return (line.get("argv") or [""])[0] != "jaekit-timing-missing-checker"


def timed_out(line):
    return criterion(line) == "EX-4"


def whole(value):
    return isinstance(value, int) and not isinstance(value, bool)


def timing_problems(line, short=False):
    """What a check or baseline line lacks of its limit, limit interval and limit decision (AC-1).

    A line the short-limit build made (short) has the limit SHORT_LIMIT_MS,
    and its duration includes the cleanup pauses after the limit interval:
    one after every process, another after killing a timed-out one (EX-4).
    """
    timing = line.get("timing")
    if not isinstance(timing, dict):
        return [f"{label(line)}: no timing"]
    problems = []
    duration = line.get("duration_ms")
    if not whole(duration):
        problems.append(f"{label(line)}: duration_ms lost")
        duration = -1
    limit = SHORT_LIMIT_MS if short else LIMIT_MS
    if timing.get("limit_ms") != limit or not whole(timing.get("limit_ms")):
        problems.append(f"{label(line)}: limit_ms {timing.get('limit_ms')!r}, not the applied {limit}")
    if timing.get("limit_reached") is not timed_out(line):
        problems.append(f"{label(line)}: limit_reached {timing.get('limit_reached')!r}")
    elapsed = timing.get("limit_elapsed_ms", "absent")
    if not started(line):
        if elapsed is not None:
            problems.append(f"{label(line)}: a process that never started has limit_elapsed_ms {elapsed!r}, not null")
    elif not whole(elapsed) or not 0 <= elapsed <= duration:
        problems.append(f"{label(line)}: limit_elapsed_ms {elapsed!r} for duration {duration!r}")
    elif timed_out(line) and not limit <= elapsed <= limit + DECISION_SLACK_MS:
        problems.append(f"{label(line)}: a timed-out run's limit interval {elapsed} ms does not end at the "
                        f"timeout decision ({limit} ms limit)")
    elif short and duration - elapsed < CLEANUP_PAUSE_MS * (2 if timed_out(line) else 1):
        problems.append(f"{label(line)}: limit interval {elapsed} ms of duration {duration} ms includes "
                        f"the cleanup pause after it")
    return problems


def cpu_problems(line):
    """What a check or baseline line lacks of its CPU measurement (AC-2).

    A started process has measured, whole, non-negative times with a scope and
    a source; the CPU-using check (EX-3) has more than zero in total, so a 0
    written for an unmeasured value does not pass. A process that never
    started is unavailable, with no times.
    """
    cpu = (line.get("timing") or {}).get("cpu") if isinstance(line.get("timing"), dict) else None
    if not isinstance(cpu, dict):
        return [f"{label(line)}: no cpu"]
    if not started(line):
        return [] if cpu.get("status") == "unavailable" and "user_ms" not in cpu and "system_ms" not in cpu else \
            [f"{label(line)}: a process that never started reports cpu {cpu}"]
    values = [cpu.get("user_ms"), cpu.get("system_ms")]
    if cpu.get("status") != "measured" or not all(whole(v) and v >= 0 for v in values) \
            or not cpu.get("scope") or not cpu.get("source"):
        return [f"{label(line)}: cpu {cpu}"]
    if criterion(line) == "EX-3" and sum(values) <= 0:
        return [f"{label(line)}: a CPU-using run reports {values} ms"]
    return []


# --- AC-1 -------------------------------------------------------------------

class Records(unittest.TestCase):
    """AC-1: every new check and baseline line keeps its limit, limit interval and limit decision."""

    KINDS = {("baseline", "AC-1"), ("check", "AC-1"), ("check", "AC-2"), ("check", "EX-1"), ("check", "EX-2"),
             ("check", "EX-3"), ("check", "EX-4"), ("check", "T001")}

    def recorded(self, directory, rules):
        fixture = short_fixture(directory, rules=rules)
        lines = runs_of(fixture.flow())
        seen = {(l["kind"], criterion(l) or (l.get("target") or {}).get("task")) for l in lines}
        undecided = [l for l in lines if l.get("result") == "unknown"]
        if not self.KINDS <= seen or fixture.refusal_wrote or (rules is None) != bool(undecided) \
                or len(fixture.short_seqs) != 2:
            raise Unavailable(f"the synthetic flow recorded {sorted(seen)}; refusal wrote: {fixture.refusal_wrote}")
        return [(line, line["seq"] in fixture.short_seqs) for line in lines]

    def test_check_and_baseline_lines_keep_their_limit_interval(self):
        problems = []
        for rules in ("run-rules/2", None):
            with tempfile.TemporaryDirectory(prefix="jaekit-timing-") as directory:
                problems += [f"{rules or 'default rules'}: {p}" for line, short in self.recorded(directory, rules)
                             if not timed_out(line) for p in timing_problems(line, short)]
        require(self, not problems, "run-timing-not-recorded", "; ".join(problems))

    def test_the_limit_interval_ends_at_the_timeout_decision(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-timeout-") as directory:
            lines = [(line, short) for line, short in self.recorded(directory, None) if timed_out(line)]
            if len(lines) != 1 or lines[0][0].get("result") != "error" or not lines[0][1]:
                raise Unavailable(f"the short-limit check did not record one timed-out run: {lines}")
            problems = timing_problems(*lines[0])
        require(self, not problems, "run-timing-not-recorded", "; ".join(problems))


# --- AC-2 -------------------------------------------------------------------

def container_records(directory, arch):
    """Record lines a Linux build of ha writes in the local check container."""
    if shutil.which("docker") is None:
        raise Unavailable("docker is required to run the Linux build")
    out = Path(directory) / f"linux-{arch}"
    out.mkdir()
    ha = build(out / "ha", goos="linux", goarch=arch)
    fixture = Fixture(out, ha, rules=None)
    steps = out / "steps.sh"
    steps.write_text("set -e\ncd /work\n"
                     f"/out/ha start {GOAL} --request '{REQUEST}' --skill-name seal --skill-version 0.1.0\n"
                     f"/out/ha check {GOAL} AC-2\n/out/ha check {GOAL} EX-3\n", encoding="utf-8")
    result = run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--platform", "linux/" + arch,
                  "-e", "GIT_CONFIG_COUNT=1", "-e", "GIT_CONFIG_KEY_0=safe.directory", "-e", "GIT_CONFIG_VALUE_0=*",
                  "-v", f"{out}:/out:ro", "-v", f"{fixture.root}:/work", "--entrypoint", "sh",
                  f"jaekit-check-git:{arch}", "/out/steps.sh"], ROOT, timeout=600)
    if result.returncode:
        raise Unavailable(f"linux/{arch} container: " + (result.stdout + result.stderr).decode(errors="replace")[-2000:])
    return runs_of(fixture.records())


# The native test that reads a recorded check's timing line on Windows (AC-2).
# It is cmd/ha/native_timing_test.go, written with these checks: it asserts the
# applied limit, the limit interval and measured CPU with the Windows scope and
# source, and changing it after the base commit is a changed test.
WINDOWS_TIMING = {"cmd/ha": "TestNativeRecordedTiming"}


def windows_native_tests():
    """{package: [test]} the checked tree's Windows native checks require."""
    sys.path.insert(0, str(ROOT / "tools/windows"))
    try:
        spec = importlib.util.spec_from_file_location("observed_native_checks", ROOT / "tools/windows/native_checks.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return {package: list(tests) for package, tests in module.NATIVE.items()}
    except Exception as error:
        raise Unavailable(f"tools/windows/native_checks.py cannot be loaded: {error!r}") from None
    finally:
        sys.path.remove(str(ROOT / "tools/windows"))


def real_run_passes():
    """Raise Unavailable unless the repository's CI reader passes the real pull request run of the checked commit.

    The reader binds the run to the open pull request whose head is the
    checked commit and to the checked tree, and requires every job and step
    and every Windows native report to have passed; the native checks report
    `passed` only when all their required tests passed.
    """
    env = {k: v for k, v in os.environ.items() if not k.startswith("HA_EVIDENCE")}
    result = run([sys.executable, "tools/verification-checks/cross_platform_ci.py", "AC-1"], ROOT, env, timeout=900)
    if result.returncode:
        raise Unavailable(f"the CI reader did not pass a real run of the checked commit (exit {result.returncode}): "
                          + result.stderr.decode(errors="replace").strip()[-500:])


class Cpu(unittest.TestCase):
    """AC-2: runs recorded on macOS, Linux and Windows keep their CPU time with its scope."""

    def test_local_records_keep_cpu_with_scope(self):
        # The local half is macOS; Linux runs in the container below.
        if sys.platform != "darwin":
            raise Unavailable(f"the local CPU observation runs on macOS, not {sys.platform}")
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-cpu-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"), rules=None)
            problems = [f"macOS: {p}" for line in runs_of(fixture.flow()) for p in cpu_problems(line)]
            arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(os.uname().machine)
            if arch is None:
                raise Unavailable(f"no Linux check image for {os.uname().machine}")
            lines = container_records(directory, arch)
            if {criterion(line) for line in lines} != {"AC-2", "EX-3"}:
                raise Unavailable(f"the Linux run recorded {[label(line) for line in lines]}")
            problems += [f"linux/{arch}: {p}" for line in lines for p in cpu_problems(line)]
            require(self, not problems, "cpu-not-recorded", "; ".join(problems))

    def test_windows_ci_measures_cpu(self):
        # macOS and Linux runs are recorded locally; Windows runs only in CI.
        required = windows_native_tests()
        missing = [f"{package} {test}" for package, test in WINDOWS_TIMING.items() if test not in required.get(package, [])]
        require(self, not missing, "cpu-not-recorded", f"Windows native CI does not require {', '.join(missing)}")
        real_run_passes()


# --- AC-3 -------------------------------------------------------------------

def view(fixture, *extra):
    return fixture.ha("timing", GOAL, *extra)


def table(text):
    """(header cells, {seq: row cells}) of the time view's Markdown table."""
    rows = [line for line in text.splitlines() if line.startswith("|")]
    if len(rows) < 2:
        return [], {}
    cells = lambda line: [cell.strip() for cell in line.strip().strip("|").split("|")]
    body = {}
    for line in rows[2:]:
        row = cells(line)
        if row and row[0].isdigit():
            body[int(row[0])] = row
    return cells(rows[0]), body


# A number not inside a word, with what follows it: `12 ms`, `1,234 ms`, `1.250 s`, `41.2%`, `1m`.
NUMBER = re.compile(r"(?<!\w)(\d{1,3}(?:,\d{3})+|\d+)(?:\.(\d+))?\s*(%|[^\W\d_]+)?")


def shown_ms(text):
    """The times a Markdown cell shows, in milliseconds, leaving out percentages.

    A number in another notation (`1m`, `250 us`, a bare `3`, a fraction of a
    millisecond) is None: this check cannot read its value.
    """
    found = []
    for whole_part, fraction, unit in NUMBER.findall(text):
        whole_part = int(whole_part.replace(",", ""))
        if unit == "%":
            continue
        if unit == "ms":
            found.append(None if fraction else whole_part)
        elif unit == "s":
            found.append(None if len(fraction) > 3 else whole_part * 1000 + int(fraction.ljust(3, "0") or 0))
        else:
            found.append(None)
    return found


def shows_times(text, values):
    """Whether a Markdown cell shows the recorded times, in order, among the other times it shows.

    A number this check cannot read leaves the cell unobserved, whether or not
    the recorded times are also there: `1m 3s` would otherwise read as 3 s.
    """
    found = shown_ms(text)
    if None in found:
        raise Unavailable(f"the Markdown view writes a time this check cannot read: {text!r}")
    rest = iter(found)
    return all(any(value == shown for shown in rest) for value in values)


def cpu_order(cpu, *texts):
    """The recorded CPU times in the order the view names them: the cell's labels, else its column's, else user first."""
    for text in map(str.lower, texts):
        if "user" in text and "system" in text:
            names = ["system_ms", "user_ms"] if text.index("system") < text.index("user") else ["user_ms", "system_ms"]
            return [cpu.get(name) for name in names]
    return [cpu.get("user_ms"), cpu.get("system_ms")]


def column(header, word):
    found = [index for index, name in enumerate(header) if word in name.lower()]
    return found[0] if len(found) == 1 else None


class TimeView(unittest.TestCase):
    """AC-3: a read-only time view, for people and machines."""

    def test_the_time_view_shows_each_run_and_changes_nothing(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-view-") as directory:
            fixture = short_fixture(directory)
            lines = runs_of(fixture.flow())
            before = (fixture.records_path.read_bytes(), digest_tree(fixture.root / ".git/ha"),
                      fixture.git("status", "--porcelain", "--untracked-files=all"))
            md, js = view(fixture), view(fixture, "--format", "json")
            after = (fixture.records_path.read_bytes(), digest_tree(fixture.root / ".git/ha"),
                     fixture.git("status", "--porcelain", "--untracked-files=all"))
            problems = []
            if before != after:
                problems.append("the time view changed the record, the logs or the working tree")
            try:
                report = json.loads(js.stdout) if js.returncode == 0 else None
            except ValueError:
                report = None
            if report is None or report.get("schema") != "timing/v1":
                problems.append(f"no timing/v1 report (exit {js.returncode})")
            else:
                rows = report.get("runs", [])
                if [r.get("seq") for r in rows] != [l["seq"] for l in lines]:
                    problems.append(f"runs {[r.get('seq') for r in rows]}, records {[l['seq'] for l in lines]}")
                for row, line in zip(rows, lines):
                    timing = line.get("timing") or {}
                    expected = {"seq": line["seq"], "target": line.get("target"), "kind": line["kind"],
                                "result": line.get("result"), "duration_ms": line.get("duration_ms")}
                    expected.update({name: timing.get(name) for name in VIEW_VALUES})
                    if any(row.get(k) != v for k, v in expected.items()):
                        problems.append(f"seq {line['seq']}: view {row}, record {expected}")
                        continue
                    # A value the row does not show is named unavailable, and only then.
                    unavailable = row.get("unavailable") or {}
                    for name in VIEW_VALUES:
                        if (row.get(name) is None) != (name in unavailable):
                            problems.append(f"seq {line['seq']}: {name} {row.get(name)!r}, unavailable {unavailable}")
                    elapsed = timing.get("limit_elapsed_ms")
                    use = None if elapsed is None else elapsed * 1000 // timing["limit_ms"]
                    if row.get("limit_use_permille") != use:
                        problems.append(f"seq {line['seq']}: limit use {row.get('limit_use_permille')}, not {use}")
                if report.get("total_duration_ms") != sum(l.get("duration_ms", 0) for l in lines) \
                        or "overlap" not in report.get("total_meaning", "") or "budget" not in report.get("total_meaning", ""):
                    problems.append(f"total {report.get('total_duration_ms')}: {report.get('total_meaning')!r}")
            text = md.stdout.decode(errors="replace")
            header, body = table(text)
            by = {criterion(l) or (l.get("target") or {}).get("task"): l for l in lines if l["kind"] == "check"}
            places = {word: column(header, word) for word in ("duration", "reached", "elapsed", "cpu")}
            if md.returncode or set(body) != {l["seq"] for l in lines} or None in places.values():
                problems.append(f"Markdown view (exit {md.returncode}) has columns {header} and rows {sorted(body)}")
            else:
                cell = lambda key, word: body[by[key]["seq"]][places[word]]
                # The timed-out run reads differently from a finished one; an unstarted run shows no time.
                if cell("EX-4", "reached") == cell("AC-2", "reached"):
                    problems.append("Markdown shows the timed-out run's limit as not reached")
                for word in ("elapsed", "cpu"):
                    if re.search(r"\d", cell("EX-2", word)):
                        problems.append(f"Markdown shows a time for the unstarted run's {word}: {cell('EX-2', word)!r}")
                for line in lines:
                    timing = line.get("timing") or {}
                    cpu = timing.get("cpu") or {}
                    row = body[line["seq"]]
                    if cpu.get("status") == "measured" and cpu.get("scope") not in row[places["cpu"]]:
                        problems.append(f"Markdown CPU of seq {line['seq']} lacks its scope")
                    # Each time the record keeps is shown as that time.
                    times = {"duration": [line.get("duration_ms")]}
                    if timing.get("limit_elapsed_ms") is not None:
                        times["elapsed"] = [timing["limit_elapsed_ms"]]
                    if cpu.get("status") == "measured":
                        times["cpu"] = cpu_order(cpu, row[places["cpu"]], header[places["cpu"]])
                    for word, values in times.items():
                        if not shows_times(row[places[word]], values):
                            problems.append(f"Markdown {word} of seq {line['seq']} shows {row[places[word]]!r}, "
                                            f"not {values} ms")
            for want in ("T001 command 1", "not the goal's elapsed time or budget"):
                if want not in text:
                    problems.append(f"Markdown view lacks {want!r}")
            contract = (ROOT / "contracts/run-record.md").read_text(encoding="utf-8")
            for word in ("`ha timing`", "`timing/v1`", "`limit_use_permille`", "`limit_elapsed_ms`"):
                if word not in contract:
                    problems.append(f"contracts/run-record.md does not define {word}")
            require(self, not problems, "time-view-missing", "; ".join(problems))


# --- AC-4 -------------------------------------------------------------------

def member_span(line, key):
    """(start, end) of `,"key":value` (or `"key":value,`) in one compact JSON object line, or None."""
    marker = b'"' + key.encode() + b'":'
    at = line.find(marker)
    if at < 0:
        return None
    index, depth, quoted = at + len(marker), 0, False
    while index < len(line):
        char = line[index:index + 1]
        if quoted:
            if char == b"\\":
                index += 1
            elif char == b'"':
                quoted = False
        elif char == b'"':
            quoted = True
        elif char in b"{[":
            depth += 1
        elif char in b"}]":
            if depth == 0:
                break
            depth -= 1
        elif char == b"," and depth == 0:
            break
        index += 1
    if line[at - 1:at] == b",":
        return at - 1, index
    return at, index + 1 if line[index:index + 1] == b"," else index


def rechain(lines):
    """Record bytes from lines with each prev hash recomputed."""
    out, prev = [], None
    for line in lines:
        if prev is not None:
            line = re.sub(rb'"prev":"[0-9a-f]{64}"', b'"prev":"' + prev.encode() + b'"', line, count=1)
        out.append(line)
        prev = hashlib.sha256(line).hexdigest()
    return b"\n".join(out) + b"\n"


def without_timing(raw):
    """The record bytes with every timing member removed, whatever its key order, and the chain rehashed."""
    lines = []
    for line in raw.splitlines():
        span = member_span(line, "timing")
        lines.append(line if span is None else line[:span[0]] + line[span[1]:])
    return rechain(lines)


def status_facts(fixture, *, done=False):
    result = fixture.ha("done" if done else "status", GOAL, "--format", "json")
    try:
        doc = json.loads(result.stdout)
    except ValueError:
        raise Unavailable(f"status JSON: {(result.stdout + result.stderr).decode(errors='replace')[-1000:]}") from None
    return result.returncode, normalize(doc)


def untimed(facts):
    """Status facts without the budget window's elapsed time, which comes from record times."""
    return json.loads(re.sub(r'"elapsed_seconds": [0-9]+', '"elapsed_seconds": null', json.dumps(facts)))


def window_elapsed(facts):
    found = re.findall(r'"elapsed_seconds": ([0-9]+)', json.dumps(facts))
    return int(found[0]) if len(found) == 1 else None


def judged(fixture, raw, clocks=None):
    """(status, done) facts of the record bytes raw, leaving raw in place.

    `ha done` appends a line even when it does not complete, so the record is
    put back after it. That line carries the time of the call, so done's
    window elapsed time is compared apart: clocks receives it with the
    monotonic times before and after the call.
    """
    fixture.records_path.write_bytes(raw)
    status = status_facts(fixture)
    called = time.monotonic()
    facts = status_facts(fixture, done=True)
    if clocks is not None:
        clocks.append((window_elapsed(facts), called, time.monotonic()))
    done = untimed(facts)
    fixture.records_path.write_bytes(raw)
    return status, done


def normalize(value):
    """Status JSON without the values that are hashes of the record bytes."""
    if isinstance(value, dict):
        return {k: normalize(v) for k, v in value.items() if k not in ("record_head", "hash", "prev")}
    if isinstance(value, list):
        return [normalize(v) for v in value]
    return value


class Judgment(unittest.TestCase):
    """AC-4: the timing fields change no status, completion, reason, usage or budget."""

    def test_status_is_the_same_with_and_without_timing(self):
        problems = []
        for rules in ("run-rules/1", "run-rules/2", None):
            with tempfile.TemporaryDirectory(prefix="jaekit-timing-status-") as directory:
                fixture = short_fixture(directory, rules=rules)
                fixture.flow()
                raw = fixture.records_path.read_bytes()
                if b'"timing"' in without_timing(raw):
                    raise Unavailable("timing fields could not be removed from the synthetic record")
                clocks = []
                with_timing, stripped = judged(fixture, raw, clocks), judged(fixture, without_timing(raw), clocks)
                for name, kept, removed in zip(("status", "done"), with_timing, stripped):
                    if kept != removed:
                        problems.append(f"{rules or 'default rules'}: {name} differs without timing")
                # Done's window time may differ only by the time from the first call to the end of the second.
                (first, at, _), (second, _, ended) = clocks
                if None in (first, second) or abs(first - second) > math.ceil(ended - at) + 1:
                    problems.append(f"{rules or 'default rules'}: done's window elapsed time {first} s became "
                                    f"{second} s within {ended - at:.1f} s")
        require(self, not problems, "timing-changes-judgment", "; ".join(problems))


# --- AC-5 -------------------------------------------------------------------

class Compatibility(unittest.TestCase):
    """AC-5: lines without timing stay readable and unrewritten; v0.1.4 reads and extends new records."""

    def test_released_ha_reads_and_extends_records_with_timing(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-compat-") as directory:
            base = Path(directory)
            tag = run(["git", "rev-parse", "--verify", "--quiet", "refs/tags/v0.1.4^{commit}"], ROOT, timeout=60)
            if tag.returncode:
                raise Unavailable("tag v0.1.4 is not available in this clone")
            released_tree = base / "v0.1.4"
            added = run(["git", "worktree", "add", "--detach", released_tree, "refs/tags/v0.1.4"], ROOT, timeout=120)
            if added.returncode:
                raise Unavailable(f"v0.1.4 worktree: {added.stderr.decode(errors='replace')}")
            try:
                released = build(base / "ha-0.1.4", tree=released_tree)
            finally:
                run(["git", "worktree", "remove", "--force", released_tree], ROOT, timeout=120)
            fixture = Fixture(base, build(base / "ha"), rules=None)
            fixture.start()
            fixture.expect(fixture.ha("check", GOAL, "AC-2"), 0, "check")
            problems = []
            first = fixture.records_path.read_bytes()
            old_status = fixture.ha("status", GOAL, "--format", "json", ha=released)
            new_status = fixture.ha("status", GOAL, "--format", "json")
            if old_status.returncode != new_status.returncode or b"integrity" in old_status.stdout + old_status.stderr:
                problems.append(f"v0.1.4 status exit {old_status.returncode}, current {new_status.returncode}")
            extended = fixture.ha("check", GOAL, "AC-2", ha=released)
            if extended.returncode:
                problems.append(f"v0.1.4 could not extend the record: exit {extended.returncode}")
            second = fixture.records_path.read_bytes()
            if not second.startswith(first):
                problems.append("v0.1.4 rewrote an earlier line")
            # The current ha extends the record after the line without timing.
            again = fixture.ha("check", GOAL, "AC-2")
            third = fixture.records_path.read_bytes()
            if again.returncode or not third.startswith(second) or len(third.splitlines()) != len(second.splitlines()) + 1:
                problems.append(f"the current ha did not append after the v0.1.4 line without rewriting it: exit {again.returncode}")
            status = fixture.ha("status", GOAL, "--format", "json")
            if status.returncode != new_status.returncode or b"integrity" in status.stdout + status.stderr:
                problems.append(f"the current ha reads the extended record with exit {status.returncode}")
            shown = fixture.ha("timing", GOAL, "--format", "json")
            try:
                rows = json.loads(shown.stdout)["runs"]
            except (ValueError, KeyError, TypeError):
                rows = []
            unrecorded = {name: "not_recorded" for name in VIEW_VALUES}
            if len(rows) != 3 or {k: (rows[1].get("unavailable") or {}).get(k) for k in unrecorded} != unrecorded \
                    or any(rows[1].get(k) is not None for k in unrecorded) \
                    or any(row.get("unavailable") for row in (rows[0], rows[2])):
                problems.append(f"the v0.1.4 line is not shown as not recorded between timed lines: {rows}")
            require(self, not problems, "unrecorded-timing-not-shown", "; ".join(problems))


# --- AC-6 -------------------------------------------------------------------

def damage(raw, damaged):
    """raw with the given lines' timing members rewritten, and the chain rehashed.

    damaged: {seq: [(pattern, replacement), ...]}, each applied once inside that line's timing member.
    """
    lines, applied = raw.splitlines(), 0
    for index, line in enumerate(lines):
        for pattern, replacement in damaged.get(index + 1, []):
            span = member_span(line, "timing")
            if span is None:
                continue
            member, count = re.subn(pattern, replacement, line[span[0]:span[1]], count=1)
            line, applied = line[:span[0]] + member + line[span[1]:], applied + count
        lines[index] = line
    return rechain(lines), applied


class Damage(unittest.TestCase):
    """AC-6: a damaged timing value hides only itself; broken JSON or chains stay integrity errors."""

    def timing_rows(self, fixture):
        shown = fixture.ha("timing", GOAL, "--format", "json")
        try:
            return {r["seq"]: r for r in json.loads(shown.stdout)["runs"]}
        except (ValueError, KeyError, TypeError):
            return {}

    def test_damaged_timing_values_are_isolated(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-damage-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"))
            lines = runs_of(fixture.flow())
            by = {criterion(l): l["seq"] for l in lines if l["kind"] == "check"}
            typed, valued = by["AC-2"], by["EX-3"]
            intact = fixture.records_path.read_bytes()
            before, original = judged(fixture, intact), self.timing_rows(fixture)
            raw, applied = damage(intact, {
                # Wrong types on one line, wrong values on another.
                typed: [(rb'"limit_ms":[0-9]+', b'"limit_ms":"x"'), (rb'"cpu":\{[^{}]*\}', b'"cpu":7')],
                valued: [(rb'"user_ms":[0-9]+', b'"user_ms":-5'), (rb'"limit_elapsed_ms":[0-9]+', b'"limit_elapsed_ms":-1')]})
            if applied != 4:
                # Nothing to damage: the executable keeps no timing value.
                require(self, False, "damaged-timing-not-isolated", f"the record keeps {applied} of 4 timing values to damage")
            problems = []
            if judged(fixture, raw) != before:
                problems.append("status or done changed after timing values were damaged")
            rows = self.timing_rows(fixture)
            invalid = lambda row, name: (row.get("unavailable") or {}).get(name) == "invalid" and row.get(name) is None
            kept = lambda seq, *names: all(rows.get(seq, {}).get(n) == original.get(seq, {}).get(n) for n in names)
            if not (invalid(rows.get(typed, {}), "limit_ms") and invalid(rows.get(typed, {}), "cpu")
                    and kept(typed, "limit_elapsed_ms", "limit_reached")):
                problems.append(f"type-damaged line shown as {rows.get(typed)}")
            def only_user_hidden(row):
                # The damaged user_ms alone unavailable, the rest of the CPU value as recorded.
                cpu, recorded = row.get("cpu"), (original.get(valued) or {}).get("cpu") or {}
                rest = lambda value: {k: v for k, v in value.items() if k != "user_ms"}
                return isinstance(cpu, dict) and cpu.get("user_ms") is None and rest(cpu) == rest(recorded) and any(
                    "user_ms" in key and reason == "invalid" for key, reason in (row.get("unavailable") or {}).items())
            if not ((invalid(rows.get(valued, {}), "cpu") or only_user_hidden(rows.get(valued, {})))
                    and invalid(rows.get(valued, {}), "limit_elapsed_ms") and kept(valued, "limit_ms", "limit_reached")):
                problems.append(f"value-damaged line shown as {rows.get(valued)}")
            others = [seq for seq in original if seq not in (typed, valued) and rows.get(seq) != original[seq]]
            if set(rows) != set(original) or others:
                problems.append(f"undamaged lines changed in the view: {others}")
            # A line that is not JSON is still an integrity error.
            broken_lines = fixture.records_path.read_bytes().splitlines()
            broken_lines[-1] = broken_lines[-1][:-5]
            fixture.records_path.write_bytes(b"\n".join(broken_lines) + b"\n")
            broken = fixture.ha("status", GOAL, "--format", "json")
            refused = fixture.ha("timing", GOAL)
            if b"integrity" not in broken.stdout + broken.stderr and b"chain" not in broken.stdout + broken.stderr:
                problems.append("a line that is not JSON is no longer an integrity error")
            if refused.returncode != 65:
                problems.append(f"the time view read a broken record (exit {refused.returncode})")
            require(self, not problems, "damaged-timing-not-isolated", "; ".join(problems))


# --- AC-8 -------------------------------------------------------------------

def estimate_facts(fixture):
    result = fixture.ha("estimate", GOAL, "--format", "json")
    try:
        facts = json.loads(result.stdout)
    except ValueError:
        raise Unavailable(f"estimate JSON (exit {result.returncode})") from None
    facts.pop("goal", None)
    return json.dumps(facts, sort_keys=True)


class Capabilities(unittest.TestCase):
    """AC-8: the time view is an optional capability; log, tail, estimate and Core checks stay the same."""

    def test_the_time_view_is_an_optional_capability(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-capabilities-") as directory:
            base = Path(directory)
            ha = build(base / "ha")
            result = run([ha, "capabilities", "--format", "json"], ROOT, timeout=60)
            try:
                caps = json.loads(result.stdout)
            except ValueError:
                raise Unavailable("capabilities JSON") from None
            problems = []
            if caps.get("supported_rules") != BASE_RULES or caps.get("default_rules") != "run-rules/3":
                problems.append(f"rules {caps.get('supported_rules')} / {caps.get('default_rules')}")
            features, formats = caps.get("features", []), caps.get("formats", {})
            if not set(BASE_FEATURES) <= set(features) or any(formats.get(k) != v for k, v in BASE_FORMATS.items()):
                problems.append(f"existing features or formats changed: {features} {formats}")
            if "run-timing/v1" not in features or formats.get("timing") != ["timing/v1"]:
                problems.append(f"the time view is not listed: {features} {formats}")
            # An executable without the time view is still accepted for ordinary work.
            older = base / "ha-without-timing"
            older.write_text("#!/bin/sh\nif [ \"$1\" = capabilities ]; then printf '%s\\n' '"
                             + json.dumps(dict(caps, features=BASE_FEATURES, formats=BASE_FORMATS)) + "'; exit 0; fi\n"
                             f"exec {ha} \"$@\"\n", encoding="utf-8")
            older.chmod(0o755)
            core = ROOT / "plugins/seal/skills/seal/scripts/check_core.py"
            for name, executable in (("this executable", ha), ("an executable without the time view", older)):
                checked = run([sys.executable, core, "--ha", executable, "--operation", "start"], ROOT, timeout=120)
                if checked.returncode:
                    problems.append(f"check_core.py start refuses {name}")
            fixture = Fixture(base, ha, rules=None)
            fixture.start()
            if estimate_facts(fixture) != BASE_ESTIMATE:
                problems.append(f"estimate after start differs from the base commit's: {estimate_facts(fixture)}")
            lines = runs_of(fixture.flow_after_start())
            for name, executable in (("this executable", ha), ("an executable without the time view", older)):
                for operation in ("resume", "check"):
                    checked = run([sys.executable, core, "--ha", executable, "--operation", operation,
                                   "--goal", fixture.root / GOAL], fixture.root, timeout=120)
                    if checked.returncode:
                        problems.append(f"check_core.py {operation} refuses {name} for a started goal: "
                                        + checked.stdout.decode(errors="replace")[-300:])
            if estimate_facts(fixture) != BASE_ESTIMATE:
                problems.append("estimate after the runs differs from the base commit's")
            seq = lines[0]["seq"]
            log = fixture.root / ".git/ha/runs" / GOAL.replace("/", "%2F") / f"{seq}.log"
            shown, tail = fixture.ha("log", GOAL, str(seq)), fixture.ha("log", GOAL, str(seq), "--tail", "1")
            if not log.is_file() or shown.stdout != log.read_bytes() or tail.stdout != (log.read_bytes().splitlines(True) or [b""])[-1]:
                problems.append("ha log or --tail no longer prints the raw output")
            require(self, not problems, "time-view-capability-missing", "; ".join(problems))


# --- AC-9 -------------------------------------------------------------------

def blocks(text):
    """Paragraphs, with each table row and list item as its own block."""
    found = []
    for paragraph in re.split(r"\n\s*\n", text):
        lines = paragraph.splitlines()
        if lines and all(line.startswith(("|", "- ", "  ")) for line in lines):
            found += lines
        else:
            found.append(paragraph)
    return found


# What each document explains, element by element; the words of one element
# appear together in one block (AC-9).
EXPLAINED = {
    "contracts/run-record.md": {
        "the limit interval and what it leaves out": ("`limit_elapsed_ms`", "제한 타이머", "정리"),
        "the CPU states": ("`cpu`", "`measured`", "`unavailable`", "`failed`"),
        "that a missing value is not filled": ("`unavailable`", "0"),
        "the CPU scope per system": ("`cpu.scope`", "운영체제", "비교하지"),
        "that judgment and budget do not use it": ("`timing`", "판정", "예산"),
    },
    "guides/USAGE.md": {
        "the view and that judgment and budget do not use it": ("ha timing", "판정", "예산"),
        "the limit interval and what it leaves out": ("제한 구간", "타이머", "정리"),
        "the CPU scope and that unmeasured CPU is not 0": ("CPU", "범위", "0이 아니라"),
    },
    "guides/USAGE.en.md": {
        "the view and that judgment and budget do not use it": ("ha timing", "status", "budget"),
        "the limit interval and what it leaves out": ("limit interval", "timer", "cleanup"),
        "the CPU scope and that unmeasured CPU is not 0": ("CPU", "Job Object", "never as 0"),
    },
}


class Privacy(unittest.TestCase):
    """AC-9: no path, environment value or command output in timing fields or the view; documented meaning."""

    def test_timing_carries_no_local_data_and_is_documented(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-privacy-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"))
            lines = runs_of(fixture.flow())
            texts = {"timing fields": json.dumps([l.get("timing") for l in lines]),
                     "time view": view(fixture).stdout.decode(errors="replace"),
                     "time view JSON": view(fixture, "--format", "json").stdout.decode(errors="replace")}
            forbidden = {SECRET, OUTPUT, str(fixture.root), os.path.realpath(fixture.root)}
            home = str(Path.home())
            if len(home.strip("/")) > 0:
                forbidden.add(home)
            problems = [f"{where} contains {value!r}" for where, text in texts.items() for value in forbidden if value in text]
            for path, elements in EXPLAINED.items():
                try:
                    found = blocks((ROOT / path).read_text(encoding="utf-8"))
                except (OSError, UnicodeDecodeError) as error:
                    raise Unavailable(f"{path}: {error}") from None
                problems += [f"{path} does not explain {element}" for element, words in elements.items()
                             if not any(all(word in block for word in words) for block in found)]
            require(self, not problems, "timing-docs-missing", "; ".join(problems))


# --- AC-7 -------------------------------------------------------------------

def comparable(line):
    """A record line without what differs between two runs of the same check: its chain, time and timing."""
    kept = {k: v for k, v in line.items() if k not in ("prev", "at", "duration_ms", "timing")}
    if isinstance(kept.get("evidence"), dict):
        # The producer's report carries the attempt's own invocation ID.
        kept["evidence"] = {k: v for k, v in kept["evidence"].items() if k not in ("invocation", "report_digest")}
    return kept


def by_order(value, commits):
    """value with each commit ID, full or abbreviated, named by its place in commits.

    The fixture commits its record too, so two fixtures with the same flow
    have different commit IDs for the same code.
    """
    if isinstance(value, dict):
        return {k: by_order(v, commits) for k, v in value.items()}
    if isinstance(value, (list, tuple)):
        return [by_order(v, commits) for v in value]
    if isinstance(value, str) and re.fullmatch(r"[0-9a-f]{7,40}", value):
        found = [index for index, commit in enumerate(commits) if commit.startswith(value)]
        if len(found) == 1:
            return f"commit {found[0]}"
    return value


def differences(first, second, path="status"):
    """The paths at which two JSON values differ."""
    if isinstance(first, dict) and isinstance(second, dict):
        return [d for key in sorted(set(first) | set(second), key=str)
                for d in differences(first.get(key), second.get(key), f"{path}.{key}")]
    if isinstance(first, list) and isinstance(second, list) and len(first) == len(second):
        return [d for index, pair in enumerate(zip(first, second)) for d in differences(*pair, f"{path}[{index}]")]
    return [] if first == second else [f"{path}: {first!r} became {second!r}"]


def cpu_of(line):
    timing = line.get("timing")
    return (timing.get("cpu") or {}) if isinstance(timing, dict) else {}


class CpuFailure(unittest.TestCase):
    """AC-7: a CPU collection failure leaves the run's result, evidence, usage and process handling as they were.

    The same flow runs twice from the same commits: with the checked tree's
    `ha`, and with builds whose every CPU collection fails.
    """

    def test_cpu_collection_failure_changes_no_run_handling(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-cpu-failure-") as directory:
            base = Path(directory)
            measured = short_fixture(base / "measured", rules=None)
            kept_lines = measured.flow()
            if not any(cpu_of(line).get("status") == "measured" for line in runs_of(kept_lines)):
                # Nothing collects CPU here, so a collection failure cannot change anything.
                return
            failing = Fixture(base / "failing", build_overlaid(base / "failing" / "ha", CPU_FAILURE), rules=None,
                              short=build_short(base / "failing-short" / "ha", CPU_FAILURE))
            failing.lenient = True
            failed_lines = failing.flow()
            orders = [fixture.git("rev-list", "--reverse", "--all").split() for fixture in (measured, failing)]
            kept = lambda line: by_order(comparable(line), orders[0])
            failed = lambda line: by_order(comparable(line), orders[1])
            problems = [f"ha {what} exited {b} instead of {a}"
                        for (what, a), (_, b) in zip(measured.exits, failing.exits) if a != b]
            if [kept(line) for line in kept_lines] != [failed(line) for line in failed_lines]:
                problems += [f"seq {a.get('seq')}: {kept(a)} became {failed(b)}"
                             for a, b in zip(kept_lines, failed_lines) if kept(a) != failed(b)] or \
                            [f"{len(kept_lines)} lines became {len(failed_lines)}"]
            for line in runs_of(failed_lines):
                cpu = cpu_of(line)
                if started(line) and (cpu.get("status") != "failed" or "user_ms" in cpu or "system_ms" in cpu):
                    problems.append(f"{label(line)}: a failed collection is recorded as {cpu}")
                if measured.ha("log", GOAL, str(line["seq"])).stdout != failing.ha("log", GOAL, str(line["seq"])).stdout:
                    problems.append(f"{label(line)}: its output differs")
            # The budget window's elapsed time comes from record times, which two flows never share.
            problems += differences(by_order(untimed(status_facts(measured)), orders[0]),
                                    by_order(untimed(status_facts(failing)), orders[1]))
        require(self, not problems, "cpu-failure-changes-the-run", "; ".join(problems))


if __name__ == "__main__":
    unittest.main()
