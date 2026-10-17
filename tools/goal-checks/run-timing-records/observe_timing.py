"""Process timing in run records, the time view and their compatibility.

Every observation builds `ha` from the checked tree and runs that executable
on a synthetic goal repository made from testdata/scenarios/_base, in a
temporary directory. AC-2 also runs a Linux build in the repository's local
check container (`jaekit-check-git`, never pulled, no network) and, for
Windows, reads the GitHub Actions run that tested the checked commit; AC-5 also builds the
released v0.1.4 from its tag, the ref the criterion names. Nothing else reads
history, and the checked tree is never written.

Violations come only from explicit `[violation:...]` failures. A tool, image,
tag, CI run or GitHub response that is missing or unusable raises Unavailable,
an execution error.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
BASE = ROOT / "testdata/scenarios/_base"
GOAL = "docs/specs/greet"
LIMIT_MS = 900000
REQUEST = "타이밍 관측용 합성 목표를 구현해줘"
# Strings a run's environment, output and location carry; none may reach a
# timing field or the time view (AC-9).
SECRET = "jaekit-observed-environment-value"
OUTPUT = "JAEKIT-OBSERVED-COMMAND-OUTPUT"
RUN_TIMING = "github.com/jgoneit/jaekit/internal/run/TestNativeRunTiming/exit"
# The executable's identity at the goal's base commit (AC-8).
BASE_FEATURES = ["estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1",
                 "budget-change/v1", "post-completion-findings/v1", "decision-history/v1"]
BASE_FORMATS = {"finding": ["run-finding/v1"], "decision_history": ["decision-history/v1"],
                "criteria": ["legacy", "nested/1"], "check_declaration": ["check-declaration/v1"],
                "check_result": ["check-result/v1"], "check_evidence": ["check-evidence/v1"]}
BASE_RULES = ["run-rules/1", "run-rules/2", "run-rules/3"]
VIEW_KEYS = {"seq", "target", "kind", "result", "duration_ms", "limit_ms", "limit_elapsed_ms", "limit_use_permille",
             "limit_reached", "cpu", "unavailable"}


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


def build(out, tree=ROOT, goos=None, goarch=None):
    env = dict(os.environ, CGO_ENABLED="0")
    for key, value in (("GOOS", goos), ("GOARCH", goarch)):
        if value:
            env[key] = value
        else:
            env.pop(key, None)
    result = run(["go", "build", "-o", out, "./cmd/ha"], tree, env, timeout=600)
    if result.returncode:
        raise Unavailable(f"go build in {tree.name}: {result.stderr.decode(errors='replace')[-2000:]}")
    return out


def digest_tree(path):
    found = {}
    for item in sorted(Path(path).rglob("*")):
        if item.is_file():
            found[str(item.relative_to(path))] = hashlib.sha256(item.read_bytes()).hexdigest()
    return found


class Fixture:
    """A synthetic goal repository and the record one `ha` executable makes in it."""

    PLAN = """# PLAN — greet

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `sh tests/greeting.sh` | tests/greeting.sh | T001 |
| AC-2 | maintain | `sh tests/keep.sh` | — | T001 |
| EX-1 | maintain | `sh tests/fails.sh` | — | T001 |
| EX-2 | maintain | `jaekit-timing-missing-checker` | — | T001 |

## 범위
- 바꿀 수 있는 경로: `src/**`, `tests/**` (SPEC: 범위와 비목표)
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
    TASK = "# T001\n\n## 목표\ngreeting says hello\n\n## 완료 조건\n- AC-1과 AC-2가 통과한다.\n- `sh tests/keep.sh`\n"

    def __init__(self, directory, ha, rules="run-rules/2"):
        self.root, self.ha_path = Path(directory) / "repo", Path(ha)
        shutil.copytree(BASE, self.root)
        goal = self.root / GOAL
        (goal / "PLAN.md").write_text(self.PLAN, encoding="utf-8")
        (goal / "tasks").mkdir()
        (goal / "tasks/T001.md").write_text(self.TASK, encoding="utf-8")
        (self.root / "tests/fails.sh").write_text(f"echo {OUTPUT}\nexit 3\n", encoding="utf-8")
        (self.root / "tests/keep.sh").write_text(f"echo {OUTPUT}\ngrep -qx keep src/keep.txt\n", encoding="utf-8")
        self.env = {k: v for k, v in os.environ.items() if not k.startswith(("GIT_", "HA_"))}
        self.env.update(JAEKIT_OBSERVED_SECRET=SECRET, GIT_AUTHOR_NAME="Observer", GIT_AUTHOR_EMAIL="observer@example.invalid",
                        GIT_COMMITTER_NAME="Observer", GIT_COMMITTER_EMAIL="observer@example.invalid")
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

    def flow(self):
        """start, baseline, passing, failing and unstartable checks, a task command, a refused check.

        Under the default rule a change baseline needs a structured result, so
        that flow records only the maintain, EX and task runs.
        """
        started = self.ha("start", GOAL, "--request", REQUEST, "--skill-name", "seal", "--skill-version", "0.1.0")
        if started.returncode:
            raise Unavailable(f"ha start: {started.stderr.decode(errors='replace')}")
        if self.rules:
            # The legacy rule keeps a plain-exit baseline without a structured result.
            raw = self.records_path.read_bytes()
            self.records_path.write_bytes(re.sub(rb'"rules":"[^"]*"', b'"rules":"' + self.rules.encode() + b'"', raw))
        if self.rules:
            (self.root / "tests/greeting.sh").write_text("grep -qx hello src/greeting.txt\n", encoding="utf-8")
            self.commit("add greeting check")
            self.expect(self.ha("check", GOAL, "--baseline", "AC-1"), 0, "baseline")
            (self.root / "src/greeting.txt").write_text("hello\n", encoding="utf-8")
            self.commit("say hello")
        self.expect(self.ha("check", GOAL, *(["AC-1", "AC-2"] if self.rules else ["AC-2"])), 0, "checks")
        self.expect(self.ha("check", GOAL, "EX-1"), 1, "failing check")
        self.expect(self.ha("check", GOAL, "EX-2"), 1, "unstartable check")
        self.expect(self.ha("check", GOAL, "T001"), 0, "task command")
        before = self.records_path.read_bytes()
        (self.root / "src/keep.txt").write_text("changed\n", encoding="utf-8")
        refused = self.ha("check", GOAL, "AC-2")
        (self.root / "src/keep.txt").write_text("keep\n", encoding="utf-8")
        if refused.returncode != 65:
            raise Unavailable(f"a dirty check was not refused: {refused.returncode}")
        self.refusal_wrote = self.records_path.read_bytes() != before
        return self.records()

    @staticmethod
    def expect(result, code, what):
        if result.returncode != code:
            raise Unavailable(f"ha {what} exited {result.returncode}, not {code}: "
                              + (result.stdout + result.stderr).decode(errors="replace")[-2000:])


def runs_of(records):
    return [r for r in records if r.get("kind") in ("check", "baseline")]


def label(line):
    target = line.get("target") or {}
    return f"seq {line.get('seq')} {target.get('criterion') or target.get('task')} {line.get('kind')}"


def started(line):
    return (line.get("argv") or [""])[0] != "jaekit-timing-missing-checker"


def timing_problems(line):
    """What a check or baseline line lacks of its limit, limit interval and limit decision (AC-1)."""
    timing = line.get("timing")
    if not isinstance(timing, dict):
        return [f"{label(line)}: no timing"]
    problems = []
    if not isinstance(line.get("duration_ms"), int):
        problems.append(f"{label(line)}: duration_ms lost")
    if timing.get("limit_ms") != LIMIT_MS or isinstance(timing.get("limit_ms"), bool):
        problems.append(f"{label(line)}: limit_ms {timing.get('limit_ms')!r}")
    if timing.get("limit_reached") is not False:
        problems.append(f"{label(line)}: limit_reached {timing.get('limit_reached')!r}")
    elapsed = timing.get("limit_elapsed_ms", "absent")
    if started(line):
        if not isinstance(elapsed, int) or isinstance(elapsed, bool) or not 0 <= elapsed <= line.get("duration_ms", -1):
            problems.append(f"{label(line)}: limit_elapsed_ms {elapsed!r} for duration {line.get('duration_ms')!r}")
    elif elapsed is not None:
        problems.append(f"{label(line)}: a process that never started has limit_elapsed_ms {elapsed!r}, not null")
    return problems


def cpu_problems(line):
    """What a check or baseline line lacks of its CPU measurement (AC-2)."""
    cpu = (line.get("timing") or {}).get("cpu") if isinstance(line.get("timing"), dict) else None
    if not isinstance(cpu, dict):
        return [f"{label(line)}: no cpu"]
    if not started(line):
        return [] if cpu.get("status") == "unavailable" and "user_ms" not in cpu and "system_ms" not in cpu else \
            [f"{label(line)}: a process that never started reports cpu {cpu}"]
    values = [cpu.get("user_ms"), cpu.get("system_ms")]
    if cpu.get("status") != "measured" or not all(isinstance(v, int) and not isinstance(v, bool) and v >= 0 for v in values) \
            or not cpu.get("scope") or not cpu.get("source"):
        return [f"{label(line)}: cpu {cpu}"]
    return []


TIMEOUT_TEST = r'''package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestObservedTimingTimeout reports the runner's limit fields by name, so it
// compiles whether or not they exist.
func TestObservedTimingTimeout(t *testing.T) {
	dir := t.TempDir()
	res, err := Exec(dir, []string{os.Getenv("JAEKIT_OBSERVE_SLEEPER"), "-c", "import time; time.sleep(30)"},
		400*time.Millisecond, filepath.Join(dir, "out.log"))
	facts := map[string]any{"error": err != nil, "outcome": res.Outcome, "reason": res.Reason,
		"duration_ms": res.Duration.Milliseconds()}
	value := reflect.ValueOf(res)
	for _, name := range []string{"Limit", "LimitElapsed", "LimitReached"} {
		field := value.FieldByName(name)
		if !field.IsValid() {
			facts[name] = "absent"
			continue
		}
		switch x := field.Interface().(type) {
		case time.Duration:
			facts[name] = x.Milliseconds()
		case *time.Duration:
			if x == nil {
				facts[name] = nil
			} else {
				facts[name] = x.Milliseconds()
			}
		case bool:
			facts[name] = x
		default:
			facts[name] = "unexpected type"
		}
	}
	data, _ := json.Marshal(facts)
	if err := os.WriteFile(os.Getenv("JAEKIT_OBSERVE_FACTS"), data, 0o600); err != nil {
		t.Fatal(err)
	}
}
'''


# --- AC-1 -------------------------------------------------------------------

class Records(unittest.TestCase):
    """AC-1: every new check and baseline line keeps its limit, limit interval and limit decision."""

    def test_check_and_baseline_lines_keep_their_limit_interval(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"))
            lines = runs_of(fixture.flow())
            kinds = {("baseline", "AC-1"), ("check", "AC-1"), ("check", "AC-2"), ("check", "EX-1"), ("check", "EX-2"),
                     ("check", "T001")}
            seen = {(l["kind"], (l.get("target") or {}).get("criterion") or (l.get("target") or {}).get("task")) for l in lines}
            if not kinds <= seen or fixture.refusal_wrote:
                raise Unavailable(f"the synthetic flow recorded {sorted(seen)}; refusal wrote: {fixture.refusal_wrote}")
            problems = [p for line in lines for p in timing_problems(line)]
            require(self, not problems, "run-timing-not-recorded", "; ".join(problems))

    def test_the_limit_interval_ends_at_the_timeout_decision(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-timeout-") as directory:
            base = Path(directory)
            source, facts = base / "observed_test.go", base / "facts.json"
            source.write_text(TIMEOUT_TEST, encoding="utf-8")
            overlay = base / "overlay.json"
            overlay.write_text(json.dumps({"Replace": {str(ROOT / "internal/run/zz_observed_timing_test.go"): str(source)}}))
            env = dict(os.environ, JAEKIT_OBSERVE_SLEEPER=sys.executable, JAEKIT_OBSERVE_FACTS=str(facts))
            result = run(["go", "test", "-count=1", "-overlay", overlay, "-run", "^TestObservedTimingTimeout$",
                          "./internal/run"], ROOT, env, timeout=300)
            if result.returncode or not facts.exists():
                raise Unavailable("runner observation: " + (result.stdout + result.stderr).decode(errors="replace")[-2000:])
            seen = json.loads(facts.read_text())
            if seen["outcome"] != "error" or seen["reason"] != "process_timeout" or seen["error"]:
                raise Unavailable(f"the sleeping process did not time out: {seen}")
            absent = [name for name in ("Limit", "LimitElapsed", "LimitReached") if seen[name] == "absent"]
            problems = [f"the runner keeps no {name}" for name in absent]
            if not absent:
                elapsed = seen["LimitElapsed"]
                if seen["Limit"] != 400 or seen["LimitReached"] is not True or not isinstance(elapsed, int) \
                        or not 400 <= elapsed <= seen["duration_ms"]:
                    problems.append(f"limit interval of a timed-out run: {seen}")
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
                     f"/out/ha check {GOAL} AC-2\n", encoding="utf-8")
    result = run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--platform", "linux/" + arch,
                  "-e", "GIT_CONFIG_COUNT=1", "-e", "GIT_CONFIG_KEY_0=safe.directory", "-e", "GIT_CONFIG_VALUE_0=*",
                  "-v", f"{out}:/out:ro", "-v", f"{fixture.root}:/work", "--entrypoint", "sh",
                  f"jaekit-check-git:{arch}", "/out/steps.sh"], ROOT, timeout=600)
    if result.returncode:
        raise Unavailable(f"linux/{arch} container: " + (result.stdout + result.stderr).decode(errors="replace")[-2000:])
    return runs_of(fixture.records())


def gh_json(path):
    result = run(["gh", "api", "--method", "GET", path], ROOT, timeout=120)
    if result.returncode:
        raise Unavailable("GitHub request failed; check authentication and permission")
    try:
        return json.loads(result.stdout)
    except ValueError:
        raise Unavailable("invalid GitHub JSON") from None


# This goal's own check files. CI never runs them, so a commit that changes
# only these files is tested by its parent's CI run.
OWN_CHECKS = "tools/goal-checks/run-timing-records/"


def ci_reports():
    """The Windows native reports of the latest CI run of the checked code.

    That is the checked commit's run or, when it has none, the run of the
    nearest first-parent ancestor from which the checked commit changes only
    this goal's own check files.
    """
    head = run(["git", "rev-parse", "HEAD"], ROOT, timeout=60).stdout.decode().strip()
    if not re.fullmatch(r"[0-9a-f]{40}", head):
        raise Unavailable("the checked commit cannot be read")
    repo = run(["gh", "repo", "view", "--json", "nameWithOwner"], ROOT, timeout=120)
    if repo.returncode:
        raise Unavailable("GitHub repository cannot be read")
    try:
        name = json.loads(repo.stdout)["nameWithOwner"]
    except (ValueError, KeyError, TypeError):
        raise Unavailable("GitHub repository cannot be read") from None
    tested = head
    while True:
        runs = [r for r in gh_json(f"repos/{name}/actions/workflows/ci.yml/runs?head_sha={tested}&per_page=100")
                .get("workflow_runs", []) if r.get("head_sha") == tested]
        if runs:
            break
        parent = run(["git", "rev-parse", "--verify", "--quiet", f"{tested}^"], ROOT, timeout=60)
        if parent.returncode or not parent.stdout.strip():
            break
        changed = run(["git", "diff", "--name-only", "-z", parent.stdout.decode().strip(), head], ROOT, timeout=60)
        if changed.returncode or any(not path.startswith(OWN_CHECKS)
                                     for path in filter(None, changed.stdout.decode(errors="replace").split("\0"))):
            break
        tested = parent.stdout.decode().strip()
    if not runs:
        raise Unavailable(f"no CI run has tested the code of {head[:12]} yet")
    latest = max(runs, key=lambda r: (r["id"], r.get("run_attempt", 1)))
    if latest.get("status") != "completed" or latest.get("conclusion") != "success":
        raise Unavailable(f"the latest CI run of {tested[:12]} is {latest.get('status')}/{latest.get('conclusion')}")
    jobs = gh_json(f"repos/{name}/actions/runs/{latest['id']}/attempts/{latest.get('run_attempt', 1)}/jobs?per_page=100")
    # Job logs carry terminal escapes; older gh prints them without the flag.
    flags = ["--allow-escape-sequences"] if "--allow-escape-sequences" in run(["gh", "api", "--help"], ROOT, timeout=60) \
        .stdout.decode(errors="replace").split() else []
    found = []
    for job in jobs.get("jobs", []):
        # One Windows job with both shells, or one job per shell.
        if not str(job.get("name", "")).startswith("Core / Windows"):
            continue
        log = run(["gh", "api", "--method", "GET", *flags, f"repos/{name}/actions/jobs/{job['id']}/logs"], ROOT, timeout=300)
        if log.returncode:
            raise Unavailable(f"the log of {job.get('name')} cannot be read")
        for line in log.stdout.decode(errors="replace").splitlines():
            at = line.find("JAEKIT_CI_REPORT {")
            if at >= 0:
                try:
                    found.append(json.loads(line[at + len("JAEKIT_CI_REPORT "):]))
                except ValueError:
                    raise Unavailable(f"a native report of {job.get('name')} is not JSON") from None
    if len(found) != 2:
        raise Unavailable(f"{len(found)} Windows native reports, not one per PowerShell")
    return found


class Cpu(unittest.TestCase):
    """AC-2: runs recorded on macOS, Linux and Windows keep their CPU time with its scope."""

    def test_local_records_keep_cpu_with_scope(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-cpu-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"))
            problems = [f"{sys.platform}: {p}" for line in runs_of(fixture.flow()) for p in cpu_problems(line)]
            arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(os.uname().machine)
            if arch is None:
                raise Unavailable(f"no Linux check image for {os.uname().machine}")
            lines = container_records(directory, arch)
            if not lines:
                raise Unavailable("the Linux run recorded no check")
            problems += [f"linux/{arch}: {p}" for line in lines for p in cpu_problems(line)]
            require(self, not problems, "cpu-not-recorded", "; ".join(problems))

    def test_windows_ci_measures_cpu(self):
        # macOS and Linux runs are recorded locally; Windows runs only in CI.
        missing = [f"Windows report {index + 1} ({(row.get('environment') or {}).get('shell_version')})"
                   for index, row in enumerate(ci_reports())
                   if RUN_TIMING not in (row.get("native_tests") or {}).get("passed", [])]
        require(self, not missing, "cpu-not-recorded",
                f"Windows native CI did not pass {RUN_TIMING} in: {', '.join(missing)}")


# --- AC-3 -------------------------------------------------------------------

def view(fixture, *extra):
    return fixture.ha("timing", GOAL, *extra)


class TimeView(unittest.TestCase):
    """AC-3: a read-only time view, for people and machines."""

    def test_the_time_view_shows_each_run_and_changes_nothing(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-view-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"))
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
                                "result": line.get("result"), "duration_ms": line.get("duration_ms"),
                                "limit_ms": timing.get("limit_ms"), "limit_elapsed_ms": timing.get("limit_elapsed_ms"),
                                "limit_reached": timing.get("limit_reached")}
                    if set(row) != VIEW_KEYS or any(row.get(k) != v for k, v in expected.items()):
                        problems.append(f"seq {line['seq']}: view {row}")
                        continue
                    elapsed = timing.get("limit_elapsed_ms")
                    use = None if elapsed is None else elapsed * 1000 // timing["limit_ms"]
                    if row.get("limit_use_permille") != use:
                        problems.append(f"seq {line['seq']}: limit use {row.get('limit_use_permille')}, not {use}")
                if report.get("total_duration_ms") != sum(l.get("duration_ms", 0) for l in lines) \
                        or "overlap" not in report.get("total_meaning", "") or "budget" not in report.get("total_meaning", ""):
                    problems.append(f"total {report.get('total_duration_ms')}: {report.get('total_meaning')!r}")
            text = md.stdout.decode(errors="replace")
            for want in ["| seq | target | kind | result | duration | limit | limit elapsed | limit use | limit reached |",
                         "T001 command 1", "not the goal's elapsed time or budget"]:
                if md.returncode or want not in text:
                    problems.append(f"Markdown view lacks {want!r} (exit {md.returncode})")
            contract = (ROOT / "contracts/run-record.md").read_text(encoding="utf-8")
            for word in ("`ha timing`", "`timing/v1`", "`limit_use_permille`", "`limit_elapsed_ms`"):
                if word not in contract:
                    problems.append(f"contracts/run-record.md does not define {word}")
            require(self, not problems, "time-view-missing", "; ".join(problems))


# --- AC-4 -------------------------------------------------------------------

def without_timing(raw):
    """The record bytes with every timing field removed and the chain rehashed."""
    lines, out, prev = raw.splitlines(), [], None
    for line in lines:
        line = re.sub(rb',"timing":\{"limit_ms":[^{}]*"cpu":\{[^{}]*\}\}', b"", line)
        if prev is not None:
            line = re.sub(rb'"prev":"[0-9a-f]{64}"', b'"prev":"' + prev.encode() + b'"', line, count=1)
        out.append(line)
        prev = hashlib.sha256(line).hexdigest()
    return b"\n".join(out) + b"\n"


def status_facts(fixture, *, done=False):
    result = fixture.ha("done" if done else "status", GOAL, "--format", "json")
    try:
        doc = json.loads(result.stdout)
    except ValueError:
        raise Unavailable(f"status JSON: {(result.stdout + result.stderr).decode(errors='replace')[-1000:]}") from None
    return result.returncode, normalize(doc)


def normalize(value):
    """Status JSON without the values that are hashes of the record bytes."""
    if isinstance(value, dict):
        return {k: normalize(v) for k, v in value.items() if k not in ("record_head", "hash", "prev")}
    if isinstance(value, list):
        return [normalize(v) for v in value]
    return value


class Judgment(unittest.TestCase):
    """AC-4: the timing fields change no status, reason, usage or budget."""

    def test_status_is_the_same_with_and_without_timing(self):
        problems = []
        for rules in ("run-rules/1", "run-rules/2", None):
            with tempfile.TemporaryDirectory(prefix="jaekit-timing-status-") as directory:
                fixture = Fixture(directory, build(Path(directory) / "ha"), rules=rules)
                fixture.flow()
                with_timing = status_facts(fixture)
                raw = fixture.records_path.read_bytes()
                fixture.records_path.write_bytes(without_timing(raw))
                if b'"timing"' in fixture.records_path.read_bytes():
                    raise Unavailable("timing fields could not be removed from the synthetic record")
                stripped = status_facts(fixture)
                if with_timing != stripped:
                    problems.append(f"{rules or 'default rules'}: status differs without timing")
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
            fixture.ha("start", GOAL, "--request", REQUEST, "--skill-name", "seal", "--skill-version", "0.1.0")
            fixture.commit("goal started") if fixture.git("status", "--porcelain").strip() else None
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
            after = fixture.records_path.read_bytes()
            if not after.startswith(first):
                problems.append("an earlier line was rewritten")
            again = fixture.ha("status", GOAL, "--format", "json")
            if again.returncode != new_status.returncode or b"integrity" in again.stdout + again.stderr:
                problems.append(f"the current ha reads the extended record with exit {again.returncode}")
            shown = fixture.ha("timing", GOAL, "--format", "json")
            try:
                rows = json.loads(shown.stdout)["runs"]
            except (ValueError, KeyError, TypeError):
                rows = []
            unrecorded = {"limit_ms": "not_recorded", "limit_elapsed_ms": "not_recorded", "limit_reached": "not_recorded",
                          "cpu": "not_recorded"}
            last = rows[-1] if rows else {}
            if len(rows) != 2 or {k: last.get("unavailable", {}).get(k) for k in unrecorded} != unrecorded \
                    or any(last.get(k) is not None for k in unrecorded):
                problems.append(f"the v0.1.4 line is not shown as not recorded: {rows}")
            require(self, not problems, "unrecorded-timing-not-shown", "; ".join(problems))


# --- AC-6 -------------------------------------------------------------------

def damage(raw, seq):
    """raw with one line's timing values given wrong types, and the chain rehashed."""
    lines, out, prev = raw.splitlines(), [], None
    for index, line in enumerate(lines, 1):
        if index == seq:
            line = re.sub(rb'"limit_ms":[0-9]+', b'"limit_ms":"x"', line, count=1)
            line = re.sub(rb'"cpu":\{[^{}]*\}', b'"cpu":7', line, count=1)
        if prev is not None:
            line = re.sub(rb'"prev":"[0-9a-f]{64}"', b'"prev":"' + prev.encode() + b'"', line, count=1)
        out.append(line)
        prev = hashlib.sha256(line).hexdigest()
    return b"\n".join(out) + b"\n"


class Damage(unittest.TestCase):
    """AC-6: a damaged timing value hides only itself; broken JSON or chains stay integrity errors."""

    def test_damaged_timing_values_are_isolated(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-damage-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"))
            lines = runs_of(fixture.flow())
            seq = lines[1]["seq"]
            before = status_facts(fixture)
            fixture.records_path.write_bytes(damage(fixture.records_path.read_bytes(), seq))
            problems = []
            if b'"limit_ms":"x"' not in fixture.records_path.read_bytes():
                # Nothing to damage: the executable keeps no timing value.
                require(self, False, "damaged-timing-not-isolated", f"seq {seq} keeps no timing value to damage")
            if status_facts(fixture) != before:
                problems.append("status changed after a timing value was damaged")
            shown = fixture.ha("timing", GOAL, "--format", "json")
            try:
                rows = {r["seq"]: r for r in json.loads(shown.stdout)["runs"]}
            except (ValueError, KeyError, TypeError):
                rows = {}
            damaged = rows.get(seq, {})
            if damaged.get("unavailable", {}).get("limit_ms") != "invalid" or damaged.get("unavailable", {}).get("cpu") != "invalid" \
                    or damaged.get("limit_elapsed_ms") is None or damaged.get("limit_reached") is not False:
                problems.append(f"damaged line shown as {damaged}")
            others = [r for s, r in rows.items() if s != seq and s != lines[-3]["seq"] and r.get("unavailable")]
            if len(rows) != len(lines) or any(r.get("unavailable") for s, r in rows.items()
                                              if s != seq and r.get("limit_elapsed_ms") is not None):
                problems.append(f"other lines lost values: {others}")
            # A line that is not JSON is still an integrity error.
            raw = fixture.records_path.read_bytes().splitlines()
            raw[-1] = raw[-1][:-5]
            fixture.records_path.write_bytes(b"\n".join(raw) + b"\n")
            broken = fixture.ha("status", GOAL, "--format", "json")
            refused = fixture.ha("timing", GOAL)
            if b"integrity" not in broken.stdout + broken.stderr and b"chain" not in broken.stdout + broken.stderr:
                problems.append("a line that is not JSON is no longer an integrity error")
            if refused.returncode != 65:
                problems.append(f"the time view read a broken record (exit {refused.returncode})")
            require(self, not problems, "damaged-timing-not-isolated", "; ".join(problems))


# --- AC-8 -------------------------------------------------------------------

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
            if features[:len(BASE_FEATURES)] != BASE_FEATURES or any(formats.get(k) != v for k, v in BASE_FORMATS.items()):
                problems.append(f"existing features or formats changed: {features} {formats}")
            if "run-timing/v1" not in features or formats.get("timing") != ["timing/v1"]:
                problems.append(f"the time view is not listed: {features} {formats}")
            core = ROOT / "plugins/seal/skills/seal/scripts/check_core.py"
            checked = run([sys.executable, core, "--ha", ha, "--operation", "start"], ROOT, timeout=120)
            if checked.returncode:
                problems.append("check_core.py start refuses this executable")
            # An executable without the time view is still accepted for ordinary work.
            older = base / "ha-without-timing"
            older.write_text("#!/bin/sh\nif [ \"$1\" = capabilities ]; then printf '%s\\n' '"
                             + json.dumps(dict(caps, features=BASE_FEATURES, formats=BASE_FORMATS)) + "'; exit 0; fi\n"
                             f"exec {ha} \"$@\"\n", encoding="utf-8")
            older.chmod(0o755)
            checked = run([sys.executable, core, "--ha", older, "--operation", "start"], ROOT, timeout=120)
            if checked.returncode:
                problems.append("check_core.py requires the time view for an ordinary start")
            fixture = Fixture(base, ha)
            lines = runs_of(fixture.flow())
            checked = run([sys.executable, core, "--ha", ha, "--operation", "check", "--goal", fixture.root / GOAL],
                          fixture.root, timeout=120)
            if checked.returncode:
                problems.append("check_core.py check refuses this executable for a started goal: "
                                + checked.stdout.decode(errors="replace")[-500:])
            seq = lines[0]["seq"]
            log = fixture.root / ".git/ha/runs" / GOAL.replace("/", "%2F") / f"{seq}.log"
            shown, tail = fixture.ha("log", GOAL, str(seq)), fixture.ha("log", GOAL, str(seq), "--tail", "1")
            if not log.is_file() or shown.stdout != log.read_bytes() or tail.stdout != (log.read_bytes().splitlines(True) or [b""])[-1]:
                problems.append("ha log or --tail no longer prints the raw output")
            estimate = fixture.ha("estimate", GOAL, "--format", "json")
            try:
                if json.loads(estimate.stdout).get("schema") != "estimate/v1":
                    problems.append("estimate is no longer estimate/v1")
            except ValueError:
                problems.append("estimate JSON")
            require(self, not problems, "time-view-capability-missing", "; ".join(problems))


# --- AC-9 -------------------------------------------------------------------

class Privacy(unittest.TestCase):
    """AC-9: no path, environment value or command output in timing fields or the view; documented meaning."""

    def test_timing_carries_no_local_data_and_is_documented(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-timing-privacy-") as directory:
            fixture = Fixture(directory, build(Path(directory) / "ha"))
            lines = runs_of(fixture.flow())
            texts = {"timing fields": json.dumps([l.get("timing") for l in lines]),
                     "time view": view(fixture).stdout.decode(errors="replace"),
                     "time view JSON": view(fixture, "--format", "json").stdout.decode(errors="replace")}
            forbidden = {SECRET, OUTPUT, str(fixture.root), os.path.realpath(fixture.root), str(Path.home())}
            problems = [f"{where} contains {value!r}" for where, text in texts.items() for value in forbidden if value in text]
            docs = {"contracts/run-record.md": ("`timing`", "`limit_ms`", "`limit_elapsed_ms`", "`limit_reached`", "`cpu`",
                                                "`ha timing`"),
                    "guides/USAGE.md": ("ha timing", "판정", "예산"),
                    "guides/USAGE.en.md": ("ha timing", "status", "budget")}
            for path, words in docs.items():
                text = (ROOT / path).read_text(encoding="utf-8")
                problems += [f"{path} does not explain {w}" for w in words if w not in text]
            require(self, not problems, "timing-docs-missing", "; ".join(problems))


if __name__ == "__main__":
    unittest.main()
