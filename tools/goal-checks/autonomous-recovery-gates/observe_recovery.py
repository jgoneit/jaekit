"""Autonomous recovery under run-rules/4: test-change authority, reported waits,
pre-approved run limits, error recovery and command time limits.

Every observation builds `ha` from the checked tree and runs it on a synthetic
goal repository made from testdata/scenarios/_base in a temporary directory.
Cases that need chosen times (reported waits) take the record `ha` wrote,
set its times and re-chain the hashes in a copy, then read `ha status`.
AC-10 also runs the checked tree's Seal Core checker and its CI identity
function. AC-11 also builds the checked tree with `go build -overlay`
replacing the rule-support function by one without run-rules/4, the Core a
run-rules/4 goal meets when it lacks the rule. Nothing reads history, and the
checked tree is never written.

Violations come only from explicit `[violation:...]` failures. A tool that is
missing or unusable raises Unavailable, an execution error. An `ha` that
refuses a new option, note or budget form as a usage error (exit 64) has not
implemented it: that is the declared violation. Any other unexpected exit is
Unavailable.
"""
import datetime
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
BASE = ROOT / "testdata/scenarios/_base"
GOAL = "docs/specs/greet"
RULES = "run-rules/4"
REQUEST = "복구 관측용 합성 목표를 구현해줘"
AUTHORITY = "the request asks for the implementation; the goal scope covers tests/**"
LIMIT_MS = 900000
# The maximum declared command limit and the proximity notice threshold the
# plan fixes (PLAN 구현 결정).
MAX_LIMIT_S = 7200
NOTICE_PERMILLE = 800
EXECUTOR_RECOVERIES = 2
NEW_FEATURES = ["test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1",
                "command-time-limits/v1"]
T0 = datetime.datetime(2026, 1, 1, 0, 0, 0, tzinfo=datetime.timezone.utc)


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def run(argv, cwd, env=None, timeout=600):
    try:
        return subprocess.run([str(a) for a in argv], cwd=cwd, env=env, capture_output=True, timeout=timeout,
                              stdin=subprocess.DEVNULL)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unavailable(f"{argv[0]}: {error}") from None


def text(result):
    return (result.stdout + result.stderr).decode(errors="replace")


def supported(result, what):
    """True when a new form ran, False when `ha` refused it as unknown usage (64).

    Any other failure is not evidence either way.
    """
    if result.returncode == 0:
        return True
    if result.returncode == 64:
        return False
    raise Unavailable(f"ha {what} exited {result.returncode}: {text(result)[-1000:]}")


def outcome(result, what):
    """'recorded' (0), 'unknown' for a form `ha` does not know (64), or 'refused'
    for a known form the goal's state or rule does not allow (65, or 66, the
    contract's refusal of a budget change under the old rules).

    Any other exit is not evidence either way.
    """
    known = {0: "recorded", 64: "unknown", 65: "refused", 66: "refused"}
    if result.returncode in known:
        return known[result.returncode]
    raise Unavailable(f"ha {what} exited {result.returncode}: {text(result)[-1000:]}")


_BUILT = {}


def ha_binary():
    """`ha` built once from the checked tree."""
    if "ha" not in _BUILT:
        out = Path(tempfile.mkdtemp(prefix="jaekit-recovery-build-")) / "ha"
        env = dict(os.environ, CGO_ENABLED="0")
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        result = run(["go", "build", "-o", out, "./cmd/ha"], ROOT, env, timeout=600)
        if result.returncode:
            raise Unavailable(f"go build: {result.stderr.decode(errors='replace')[-2000:]}")
        _BUILT["ha"] = out
    return _BUILT["ha"]


def stamp(seconds):
    return (T0 + datetime.timedelta(seconds=seconds)).strftime("%Y-%m-%dT%H:%M:%SZ")


def rechain(lines):
    """Record bytes from lines with each prev hash recomputed."""
    out, prev = [], None
    for line in lines:
        if prev is not None:
            line = re.sub(rb'"prev":"[0-9a-f]{64}"', b'"prev":"' + prev.encode() + b'"', line, count=1)
        out.append(line)
        prev = hashlib.sha256(line).hexdigest()
    return b"".join(item + b"\n" for item in out)


def retimed(raw, times):
    """Record bytes with `at` (and `started_at`, when the line has one) set per seq.

    times: {seq: at_seconds or (started_seconds, at_seconds)}; seconds after T0.
    """
    lines = raw.splitlines()
    out = []
    for line in lines:
        seq = json.loads(line)["seq"]
        if seq in times:
            value = times[seq]
            started, at = value if isinstance(value, tuple) else (None, value)
            line = re.sub(rb'"at":"[^"]*"', b'"at":"' + stamp(at).encode() + b'"', line, count=1)
            if started is not None:
                if b'"started_at":"' not in line:
                    raise Unavailable(f"seq {seq} has no started_at to set")
                line = re.sub(rb'"started_at":"[^"]*"', b'"started_at":"' + stamp(started).encode() + b'"', line,
                              count=1)
        out.append(line)
    return rechain(out)


def spaced(raw, step=10):
    """Record bytes where line seq N is at N*step seconds, and each run started a second earlier."""
    times = {}
    for line in raw.splitlines():
        item = json.loads(line)
        at = item["seq"] * step
        times[item["seq"]] = (at - 1, at) if "started_at" in item else at
    return retimed(raw, times)


def without_findings(raw):
    """Record bytes without finding lines, renumbered and re-chained."""
    kept = []
    for line in raw.splitlines():
        if json.loads(line).get("schema") == "run-finding/v1":
            continue
        kept.append(re.sub(rb'"seq":\d+', b'"seq":%d' % (len(kept) + 1), line, count=1))
    return rechain(kept)


class Fixture:
    """A synthetic goal repository; AC-1 is a structured change, AC-2 a maintain condition."""

    PLAN = """# PLAN — greet

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | {ac1} | checks/reference.py, checks/reference.json, checks/declaration.json | T001 |
| AC-2 | maintain | {ac2} | — | T001 |
{manual}| EX-1 | maintain | `sh tests/fails.sh` | — | T001 |
| EX-4 | maintain | `sh tests/sleep.sh` | — | T001 |
| EX-5 | maintain | `sh tests/nap.sh` | — | T001 |

## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | checks/declaration.json |

## 범위
- 바꿀 수 있는 경로: {allowed} (SPEC: 범위와 비목표)
- 테스트 경로: {tests}
- 유지할 동작: `src/keep.txt`

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | greeting says hello and keep stays | AC-1, AC-2 | — | — | 예 | tasks/T001.md |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측
{limits}"""
    TASK = "# T001\n\n## 목표\ngreeting says hello\n\n## 완료 조건\n- AC-1과 AC-2가 통과한다.\n- `sh tests/keep.sh`\n- `sh tests/nap.sh`\n"

    REFERENCE = "`python3 checks/reference.py checks/declaration.json checks/reference.json`"

    def __init__(self, directory, ac1=REFERENCE, ac2="`sh tests/keep.sh`", allowed="`src/**`, `tests/**`, `checks/**`",
                 tests="(기본값)", limits="", outside_test=False, manual=False, ha=None):
        self.root = Path(directory) / "repo"
        self.ha_path = ha or ha_binary()
        shutil.copytree(BASE, self.root)
        self.plan = dict(ac1=ac1, ac2=ac2, allowed=allowed, tests=tests, limits=limits,
                         manual="| AC-3 | manual | — 사람이 인사말을 읽는다 | — | T001 |\n" if manual else "")
        self.write_plan()
        if manual:
            spec = self.root / GOAL / "SPEC.md"
            body = spec.read_text(encoding="utf-8")
            spec.write_text(body.replace("\n## Open Decisions", "- **AC-3** A person reads the greeting.\n\n## Open Decisions"),
                            encoding="utf-8")
        goal = self.root / GOAL
        (goal / "tasks").mkdir()
        (goal / "tasks/T001.md").write_text(self.TASK, encoding="utf-8")
        (goal / "PROGRESS.md").write_text("# PROGRESS\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n"
                                          "| T001 | done | |\n", encoding="utf-8")
        (self.root / "tests/fails.sh").write_text("exit 3\n", encoding="utf-8")
        (self.root / "tests/sleep.sh").write_text("sleep 20\n", encoding="utf-8")
        (self.root / "tests/nap.sh").write_text("sleep 2.7\n", encoding="utf-8")
        if outside_test:
            (self.root / "lib/check_test.sh").write_text("grep -q . lib/other.txt\n", encoding="utf-8")
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
        self.env.update(GIT_AUTHOR_NAME="Observer", GIT_AUTHOR_EMAIL="observer@example.invalid",
                        GIT_COMMITTER_NAME="Observer", GIT_COMMITTER_EMAIL="observer@example.invalid")
        self.git("init", "-q", "-b", "main")
        self.commit("base")

    def write_plan(self, **changes):
        """PLAN.md from the fixture's values, with any of them changed from now on."""
        self.plan.update(changes)
        (self.root / GOAL / "PLAN.md").write_text(self.PLAN.format(**self.plan), encoding="utf-8")

    def git(self, *args):
        result = run(["git", *args], self.root, self.env, timeout=120)
        if result.returncode:
            raise Unavailable(f"git {args[0]}: {result.stderr.decode(errors='replace')}")
        return result.stdout.decode()

    def commit(self, message):
        self.git("add", "-A")
        self.git("commit", "-q", "--allow-empty", "-m", message)

    def ha(self, *args, ha=None, timeout=900):
        return run([ha or self.ha_path, *args], self.root, self.env, timeout=timeout)

    @property
    def records_path(self):
        return self.root / GOAL / "runs.jsonl"

    def records(self):
        try:
            return [json.loads(line) for line in self.records_path.read_bytes().splitlines()]
        except (OSError, ValueError) as error:
            raise Unavailable(f"runs.jsonl: {error}") from None

    def start(self, *extra, rules=RULES):
        """`ha start`, choosing run-rules/4 unless rules is None (the default rule).

        The result tells whether `ha` knew the options; nothing is recorded when it did not.
        """
        chosen = ("--rules", rules) if rules else ()
        return self.ha("start", GOAL, "--request", REQUEST, "--skill-name", "seal", "--skill-version", "0.1.16",
                       *chosen, *extra)

    def start_new(self, *extra):
        """Whether a run-rules/4 goal started; Unavailable when `ha` failed for another reason."""
        return supported(self.start(*extra), "start --rules " + RULES) and self.started_rules() == RULES

    def tool(self, name, body):
        """An executable outside the repository; None removes it (a missing tool)."""
        path = self.root.parent / "tools" / name
        if body is None:
            path.unlink(missing_ok=True)
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("#!/bin/sh\n" + body, encoding="utf-8")
            path.chmod(0o755)
        return path

    def must(self, result, what, codes=(0,)):
        if result.returncode not in codes:
            raise Unavailable(f"ha {what} exited {result.returncode}: {text(result)[-2000:]}")
        return result

    def status(self, raw=None):
        """`ha status --format json`, for the record as it is or for given record bytes."""
        if raw is not None:
            saved = self.records_path.read_bytes()
            self.records_path.write_bytes(raw)
        try:
            result = self.ha("status", GOAL, "--format", "json")
        finally:
            if raw is not None:
                self.records_path.write_bytes(saved)
        try:
            return json.loads(result.stdout)
        except ValueError:
            raise Unavailable(f"ha status: {text(result)[-2000:]}") from None

    def implement_greeting(self):
        (self.root / "src/greeting.txt").write_text("hello\n", encoding="utf-8")
        self.commit("say hello")

    def implement(self):
        """Baseline and passing checks for AC-1 and AC-2."""
        self.must(self.ha("check", GOAL, "--baseline", "AC-1"), "baseline")
        self.implement_greeting()
        self.must(self.ha("check", GOAL, "AC-1", "AC-2"), "checks")

    def started_rules(self):
        if not self.records_path.exists():
            return None
        lines = self.records()
        return lines[0].get("rules") if lines else None


def reasons(report, code):
    return [r for r in report.get("reasons", []) if r.get("code") == code]


def changed_paths(report):
    return {p for r in reasons(report, "test_definition_changed") for p in r.get("paths", [])}


def test_change(report, path):
    for item in report.get("test_changes") or []:
        if item.get("path") == path:
            return item
    return None


def criterion(report, cid):
    for item in report.get("criteria", []):
        if item.get("id") == cid:
            return item
    return {}


class Directory:
    def __enter__(self):
        self.path = tempfile.mkdtemp(prefix="jaekit-recovery-")
        return self.path

    def __exit__(self, *exc):
        shutil.rmtree(self.path, ignore_errors=True)


# --- AC-1 / AC-2 -------------------------------------------------------------

class TestAuthority(unittest.TestCase):
    """AC-1: authority recorded at start; a reasoned change inside it needs no confirmation."""

    def test_reasoned_test_change_inside_the_authority_needs_no_confirmation(self):
        with Directory() as d:
            f = Fixture(d)
            require(self, f.start_new("--test-authority", AUTHORITY), "test-authority-missing",
                    "ha cannot start a run-rules/4 goal with --test-authority")
            start = f.records()[0]
            scope = start.get("scope") or {}
            require(self, (start.get("test_authority") or {}).get("interpretation") == AUTHORITY
                    and "tests/**" in scope.get("allowed", []) and "tests/**" in scope.get("tests", []),
                    "test-authority-missing", f"start line keeps no authority or scope copy: {start}")
            f.implement()
            keep = f.root / "tests/keep.sh"
            keep.write_text(keep.read_text(encoding="utf-8") + "test \"$(wc -l < src/keep.txt)\" -eq 1\n",
                            encoding="utf-8")
            f.commit("strengthen keep")
            problems = []
            if "tests/keep.sh" not in changed_paths(f.status()):
                problems.append("a test change without a reason was not held for confirmation")
            # A reason that names no check method is not a reason, whether refused or recorded.
            f.ha("note", GOAL, "test-change", "tests:tests/keep.sh", "--criteria", "AC-2", "--how", "")
            if "tests/keep.sh" not in changed_paths(f.status()):
                problems.append("a reason without how the condition is checked exempted the change")
            noted = f.ha("note", GOAL, "test-change", "tests:tests/keep.sh", "--criteria", "AC-2", "--how",
                         "keeps the exact keep check and adds a one-line count")
            require(self, supported(noted, "note test-change"), "test-authority-missing",
                    "ha cannot record a test-change reason")
            report = f.status()
            item = test_change(report, "tests/keep.sh") or {}
            rationale = item.get("rationale") or {}
            authority = report.get("test_authority") or {}
            blob = f.git("rev-parse", "HEAD:tests/keep.sh").strip()
            if "tests/keep.sh" in changed_paths(report):
                problems.append("a reasoned change inside the authority still needs confirmation")
            if not (item.get("exempt") is True and item.get("blob") == blob and rationale.get("criteria") == ["AC-2"]
                    and rationale.get("claim") == "executor" and authority.get("request_quote") == REQUEST
                    and authority.get("interpretation") == AUTHORITY):
                problems.append(f"status does not show the path's version, reason and authority: {item} {authority}")
            markdown = text(f.ha("status", GOAL))
            if "tests/keep.sh" not in markdown or "executor" not in markdown or AUTHORITY not in markdown:
                problems.append("the Markdown status does not show the exempted path with its reason and authority")
            # The reason covers the version it named, not a later one.
            keep.write_text(keep.read_text(encoding="utf-8") + "true\n", encoding="utf-8")
            f.commit("touch keep again")
            later = test_change(f.status(), "tests/keep.sh") or {}
            if later.get("exempt") is not False or not later.get("reason"):
                problems.append(f"a reason recorded for an earlier version still exempts the file: {later}")
            require(self, not problems, "test-authority-missing", "; ".join(problems))

    def test_authority_and_classification_stay_as_started(self):
        with Directory() as d:
            f = Fixture(d)
            require(self, f.start_new(), "test-authority-missing", "ha cannot start a run-rules/4 goal")
            f.implement()
            keep = f.root / "tests/keep.sh"
            keep.write_text(keep.read_text(encoding="utf-8") + "true\n", encoding="utf-8")
            f.commit("change keep")
            noted = f.ha("note", GOAL, "test-change", "tests:tests/keep.sh", "--criteria", "AC-2", "--how",
                         "keeps the exact keep check")
            require(self, supported(noted, "note test-change"), "test-authority-missing",
                    "ha cannot record a test-change reason")
            problems = []
            item = test_change(f.status(), "tests/keep.sh") or {}
            if "tests/keep.sh" not in changed_paths(f.status()) or item.get("exempt") is not False:
                problems.append(f"a goal started without authority exempted a test change: {item}")
            # Narrowing the plan's test paths after the start does not drop the file.
            f.write_plan(tests="`docs/**`")
            f.commit("narrow the test paths")
            if "tests/keep.sh" not in changed_paths(f.status()):
                problems.append("narrowing the plan's test paths after the start dropped a started test file")
            # Widening them after the start does not add a base file to the test changes.
            f.write_plan(tests="`tests/**`, `lib/**`", allowed="`src/**`, `tests/**`, `checks/**`, `lib/**`")
            (f.root / "lib/other.txt").write_text("changed\n", encoding="utf-8")
            f.commit("widen the test paths and change a lib file")
            if "lib/other.txt" in changed_paths(f.status()):
                problems.append("widening the plan's test paths after the start added a base file to the test changes")
            require(self, not problems, "test-authority-missing", "; ".join(problems))


class TestMoves(unittest.TestCase):
    """AC-2: weakened, unlinked, or outside changes still need the user; linked moves do not."""

    def test_moves_need_every_new_path_and_weakening_stays_with_the_user(self):
        with Directory() as d:
            f = Fixture(d, outside_test=True)
            f.commit("outside test")
            require(self, f.start_new("--test-authority", AUTHORITY), "test-move-unsupported",
                    "ha cannot start a run-rules/4 goal with --test-authority")
            f.implement()
            original = (f.root / "tests/keep.sh").read_text(encoding="utf-8")
            (f.root / "tests/keep.sh").unlink()
            (f.root / "tests/keep_a.sh").write_text(original, encoding="utf-8")
            (f.root / "tests/keep_b.sh").write_text("test -f src/keep.txt\n", encoding="utf-8")
            (f.root / "lib/check_test.sh").write_text("true\n", encoding="utf-8")
            f.commit("split keep and weaken the outside test")
            problems = []
            if "tests/keep.sh" not in changed_paths(f.status()):
                problems.append("an unlinked deletion was not held for confirmation")
            moved = f.ha("note", GOAL, "test-change", "tests:tests/keep.sh", "--moved-to", "tests/keep_a.sh",
                         "--moved-to", "tests/keep_b.sh", "--criteria", "AC-2", "--how",
                         "keep_a keeps the exact check; keep_b adds an existence check")
            require(self, supported(moved, "note test-change --moved-to"), "test-move-unsupported",
                    "ha cannot record a moved test file")
            f.ha("note", GOAL, "test-change", "tests:lib/check_test.sh", "--criteria", "AC-2", "--how", "n/a")
            report = f.status()
            if "tests/keep.sh" in changed_paths(report):
                problems.append("a split linked to every new path still needs confirmation")
            if "lib/check_test.sh" not in changed_paths(report):
                problems.append("a test change outside the authority's paths was exempted")
            (f.root / "tests/keep_b.sh").write_text("true\n", encoding="utf-8")
            f.commit("change a linked file")
            if "tests/keep.sh" not in changed_paths(f.status()):
                problems.append("a linked file changed after the link, and the old link still exempts the deletion")
            # A change the executor marks as weakening stays with the user.
            (f.root / "tests/keep.sh").write_text(original + "true\n", encoding="utf-8")
            f.commit("restore keep with an extra line")
            weakened = outcome(f.ha("note", GOAL, "test-change", "tests:tests/keep.sh", "--criteria", "AC-2", "--how",
                                    "drops nothing but is marked", "--weakened"), "note test-change --weakened")
            if weakened != "recorded":
                problems.append(f"a change marked as weakening could not be recorded ({weakened})")
            elif "tests/keep.sh" not in changed_paths(f.status()):
                problems.append("a change marked as weakening was exempted")
            require(self, not problems, "test-move-unsupported", "; ".join(problems))


# --- AC-3 / AC-4 -------------------------------------------------------------

class Waits(unittest.TestCase):
    """AC-3 and AC-4: reported waits leave the elapsed budget, and nothing else does."""

    def flow(self, d, steps, fixture=None):
        """The fixture and its record bytes after steps, or None when `ha` lacks a new form.

        Each step is ('note', args), ('check', args) or ('implement', None).
        """
        f = fixture or Fixture(d)
        if not f.start_new():
            return f, None
        for kind, args in steps:
            if kind == "note":
                if not supported(f.ha("note", GOAL, *args), "note " + args[0]):
                    return f, None
            elif kind == "check":
                f.must(f.ha("check", GOAL, *args), "check " + " ".join(args), codes=(0, 1))
            elif kind == "implement":
                f.implement()
        return f, f.records_path.read_bytes()

    def budget(self, f, raw, times):
        return f.status(retimed(raw, times)).get("budget") or {}

    def test_reported_wait_leaves_the_elapsed_budget(self):
        with Directory() as d:
            f, raw = self.flow(d, [("note", ["wait", "--reason", "which greeting format to use"]),
                                   ("note", ["input", "--quote", "plain hello"]),
                                   ("check", ["AC-2"])])
            require(self, raw is not None, "reported-wait-unsupported", "ha cannot record a reported wait")
            # start 0, wait 100, answer 400, check from 500 to 510: the wait counts from 101 to 400.
            times = {1: 0, 2: 100, 3: 400, 4: (500, 510)}
            first, second = self.budget(f, raw, times), self.budget(f, raw, times)
            require(self, first == second and first.get("reported_wait_seconds") == 299
                    and first.get("span_seconds") == 510 and first.get("elapsed_seconds") == 211
                    and first.get("wait_basis") == "executor_reported", "reported-wait-unsupported",
                    f"budget {first} (again {second})")

    def test_only_reported_waits_outside_runs_are_excluded(self):
        problems = []

        def expect(f, raw, times, wait, elapsed, what):
            budget = self.budget(f, raw, times)
            if budget.get("reported_wait_seconds") != wait or budget.get("elapsed_seconds") != elapsed:
                problems.append(f"{what}: {budget}, want wait {wait} and elapsed {elapsed}")

        with Directory() as d:
            f, raw = self.flow(d, [("note", ["input", "--quote", "go on"]), ("check", ["AC-2"])])
            require(self, raw is not None, "wait-boundary-unsupported", "no run-rules/4 goal")
            expect(f, raw, {1: 0, 2: 300, 3: (400, 410)}, 0, 410, "a gap without a reported wait")
        with Directory() as d:
            f, raw = self.flow(d, [("note", ["wait", "--reason", "q"]), ("check", ["AC-2"]),
                                   ("note", ["input", "--quote", "a"])])
            require(self, raw is not None, "wait-boundary-unsupported", "ha cannot record a reported wait")
            require(self, "started_at" in f.records()[2], "wait-boundary-unsupported",
                    "a run-rules/4 check line keeps no started_at")
            # A check that starts during the wait ends it, and its run stays elapsed.
            expect(f, raw, {1: 0, 2: 100, 3: (300, 310), 4: 400}, 199, 201, "a check started during the wait")
            # A run that began before the wait stays elapsed where they overlap.
            expect(f, raw, {1: 0, 2: 100, 3: (50, 150), 4: 300}, 150, 150, "a run overlapping the wait")
        with Directory() as d:
            f, raw = self.flow(d, [("note", ["wait", "--reason", "q"]), ("note", ["resume"]), ("check", ["AC-2"]),
                                   ("note", ["input", "--quote", "a"])])
            if raw is None:
                problems.append("ha cannot record the end of a wait")
            else:
                expect(f, raw, {1: 0, 2: 100, 3: 250, 4: (300, 310), 5: 400}, 149, 251, "a wait ended by resume")
        with Directory() as d:
            f, raw = self.flow(d, [("note", ["wait", "--reason", "q"]), ("check", ["--baseline", "AC-1"]),
                                   ("note", ["input", "--quote", "a"])])
            if raw is not None:
                expect(f, raw, {1: 0, 2: 100, 3: (200, 210), 4: 400}, 99, 301, "a baseline started during the wait")
        with Directory() as d:
            f, raw = self.flow(d, [("note", ["wait", "--reason", "q"]), ("check", ["T001"]),
                                   ("note", ["input", "--quote", "a"])])
            if raw is not None:
                expect(f, raw, {1: 0, 2: 100, 3: (150, 160), 4: (160, 170), 5: 400}, 49, 351,
                       "a task command started during the wait")
        with Directory() as d:
            f, raw = self.flow(d, [("note", ["wait", "--reason", "q1"]), ("note", ["wait", "--reason", "q2"]),
                                   ("note", ["input", "--quote", "a"]), ("check", ["AC-2"])])
            if raw is not None:
                expect(f, raw, {1: 0, 2: 100, 3: 200, 4: 300, 5: (400, 410)}, 199, 211, "overlapping waits")
        with Directory() as d:
            f, raw = self.flow(d, [("note", ["wait", "--reason", "q"]), ("note", ["reopen", "--quote", "다시 해줘"]),
                                   ("check", ["AC-2"]), ("note", ["input", "--quote", "a"])])
            if raw is not None:
                expect(f, raw, {1: 0, 2: 100, 3: 200, 4: (250, 260), 5: 300}, 0, 100, "a wait across reopen")
        with Directory() as d:
            # A finding recorded during a wait neither ends it nor changes the budget.
            f, raw = self.flow(d, [("implement", None)])
            if raw is not None:
                f.must(f.ha("done", GOAL), "done")
                done = f.records_path.read_bytes().splitlines()[-1]
                completion = {"seq": json.loads(done)["seq"], "sha256": hashlib.sha256(done).hexdigest()}
                f.must(f.ha("note", GOAL, "wait", "--reason", "q"), "wait after completion")
                finding = f.root / ".git/finding.json"
                finding.write_text(json.dumps({
                    "id": "one", "request_id": "create-one", "revision": 0, "completion": completion,
                    "criteria": [], "mapping": "unmapped", "status": "candidate", "category": "unknown",
                    "summary": "Synthetic finding", "source": "synthetic-review", "evidence": []}), encoding="utf-8")
                f.must(f.ha("finding", GOAL, "--input", str(finding)), "finding")
                f.must(f.ha("note", GOAL, "input", "--quote", "a"), "answer")
                full = spaced(f.records_path.read_bytes())
                with_finding = f.status(full).get("budget") or {}
                plain = f.status(without_findings(full)).get("budget") or {}
                keys = ("reported_wait_seconds", "elapsed_seconds", "span_seconds", "runs")
                if not with_finding.get("reported_wait_seconds") or \
                        any(with_finding.get(k) != plain.get(k) for k in keys):
                    problems.append(f"a finding changed the wait or the budget: {with_finding} vs {plain}")
        require(self, not problems, "wait-boundary-unsupported", "; ".join(problems))


# --- AC-5 --------------------------------------------------------------------

class Preapproval(unittest.TestCase):
    """AC-5: a pre-approved limit lets the executor raise the run limit with a reason."""

    def test_executor_raises_within_the_preapproved_limit(self):
        with Directory() as d:
            f = Fixture(d)
            require(self, f.start_new(), "preapproved-runs-unsupported", "no run-rules/4 goal")
            problems = []

            def budget():
                return f.status().get("budget") or {}

            approved = outcome(f.ha("budget", GOAL, "--preapprove", "100", "--window", "1", "--revision", "1",
                                    "--quote", "최대 100회까지 올려도 돼", "--context", "이 창의 실행 상한을 100회까지 허용"),
                               "budget --preapprove")
            require(self, approved == "recorded", "preapproved-runs-unsupported",
                    f"ha budget --preapprove was {approved}")
            state = budget()
            if state.get("preapproved_runs") != 100 or state.get("runs_limit") != 50:
                problems.append(f"status does not show the pre-approved limit apart from the limit: {state}")
            raised = outcome(f.ha("budget", GOAL, "--runs", "80", "--from", "50", "--window", "1",
                                  "--revision", str(state.get("revision")), "--reason", "two more change rounds"),
                             "budget --runs")
            state = budget()
            if raised != "recorded" or state.get("runs_limit") != 80:
                problems.append(f"an executor raise within the pre-approved limit was {raised}: {state}")
            over = outcome(f.ha("budget", GOAL, "--runs", "120", "--from", "80", "--window", "1",
                                "--revision", str(state.get("revision")), "--reason", "more"), "budget --runs")
            if over == "recorded" or budget().get("runs_limit") != 80:
                problems.append("an executor raise above the pre-approved limit was recorded without a quote")
            if not {"runs", "runs_limit", "preapproved_runs"} <= set(state):
                problems.append(f"status lacks the limit, pre-approved limit or usage: {state}")
            f.must(f.ha("note", GOAL, "reopen", "--quote", "다시 해줘"), "reopen")
            state = budget()
            if state.get("runs_limit") != 50 or state.get("preapproved_runs") is not None:
                problems.append(f"the new window inherited the pre-approval or its raise: {state}")
            unapproved = outcome(f.ha("budget", GOAL, "--runs", "60", "--from", "50", "--window",
                                      str(state.get("window_start")), "--revision", str(state.get("revision")),
                                      "--reason", "more"), "budget --runs")
            if unapproved == "recorded":
                problems.append("an executor raise without a pre-approved limit was recorded without a quote")
            require(self, not problems, "preapproved-runs-unsupported", "; ".join(problems))


# --- AC-6 --------------------------------------------------------------------

class Recovery(unittest.TestCase):
    """AC-6: a recovery lets a later valid result satisfy a condition past its error limit.

    AC-1 runs the reference producer through an external wrapper and AC-2
    runs an external check, so removing or installing a tool outside the
    repository makes errors, failures and passes without changing the tree.
    """

    def fixture(self, d):
        wrapper, keep = Path(d) / "tools/run-producer", Path(d) / "tools/keep-check"
        f = Fixture(d, ac1=f"`{wrapper} checks/reference.py checks/declaration.json checks/reference.json`",
                    ac2=f"`{keep}`")
        require(self, f.start_new(), "error-recovery-unsupported", "no run-rules/4 goal")
        return f

    @staticmethod
    def recover(f, cid, input_, change):
        return f.ha("note", GOAL, "recover", cid, "--input", input_, "--change", change)

    def test_recovery_is_bound_to_its_condition_and_input(self):
        with Directory() as d:
            f = self.fixture(d)
            problems = []
            f.must(f.ha("check", GOAL, "AC-2"), "missing keep-check", codes=(1,))
            early = outcome(self.recover(f, "AC-2", "check", "installed the tool"), "note recover")
            require(self, early != "unknown", "error-recovery-unsupported", "ha does not know recover")
            if early == "recorded":
                problems.append("a recovery was recorded before the error limit was passed")
            f.must(f.ha("check", GOAL, "AC-2"), "missing keep-check again", codes=(1,))
            for _ in range(2):
                f.must(f.ha("check", GOAL, "--baseline", "AC-1"), "missing wrapper", codes=(0, 1))
            report = f.status()
            if criterion(report, "AC-2").get("reason") != "error_limit" or \
                    criterion(report, "AC-1").get("reason") != "error_limit":
                raise Unavailable(f"the conditions did not reach the error limit: {report.get('criteria')}")
            if outcome(self.recover(f, "AC-1", "check", "installed the wrapper"), "note recover") != "refused":
                problems.append("a check-input recovery was recorded for AC-1, whose errors were all baseline errors")
            # AC-1 now passes its check error limit as well.
            for _ in range(2):
                f.must(f.ha("check", GOAL, "AC-1"), "missing wrapper", codes=(1,))
            f.tool("run-producer", 'exec python3 "$@"\n')
            f.tool("keep-check", "grep -qx keep src/keep.txt\n")
            recovered = outcome(self.recover(f, "AC-2", "check", "installed the tool"), "note recover")
            require(self, recovered == "recorded", "error-recovery-unsupported",
                    f"a recovery past the error limit was {recovered}")
            if criterion(f.status(), "AC-1").get("reason") != "error_limit":
                problems.append("a recovery of AC-2 cleared AC-1's error limit")
            if outcome(self.recover(f, "AC-1", "check", "installed the wrapper"), "note recover") != "recorded":
                problems.append("a check-input recovery past AC-1's check error limit was refused")
            if criterion(f.status(), "AC-1").get("reason") != "error_limit":
                problems.append("a check-input recovery cleared AC-1's baseline error limit")
            f.must(f.ha("check", GOAL, "AC-2"), "recovered AC-2")
            report = f.status()
            item = criterion(report, "AC-2")
            if not item.get("satisfied"):
                problems.append(f"a valid pass after the recovery did not satisfy AC-2: {item}")
            if item.get("attempts", {}).get("errors") != 2:
                problems.append(f"the errors before the recovery left the attempts: {item.get('attempts')}")
            recovery = next((r for r in report.get("recoveries") or []
                             if r.get("criterion") == "AC-2" and r.get("input") == "check"), {})
            if recovery.get("used") != 1 or recovery.get("remaining") != EXECUTOR_RECOVERIES - 1:
                problems.append(f"status does not show used and remaining recoveries: {report.get('recoveries')}")
            if outcome(self.recover(f, "AC-1", "baseline", "installed the wrapper"), "note recover") != "recorded":
                problems.append("a baseline-input recovery was refused past the baseline error limit")
            f.must(f.ha("check", GOAL, "--baseline", "AC-1"), "recovered baseline")
            f.implement_greeting()
            f.must(f.ha("check", GOAL, "AC-1"), "AC-1 check")
            if not criterion(f.status(), "AC-1").get("satisfied"):
                problems.append(f"a valid baseline after the recovery did not satisfy AC-1: {criterion(f.status(), 'AC-1')}")
            require(self, not problems, "error-recovery-unsupported", "; ".join(problems))

    def test_recovery_limit_and_kept_failures(self):
        with Directory() as d:
            f = self.fixture(d)
            problems = []
            # A failure and an unexpected baseline pass come before any recovery.
            f.tool("keep-check", "exit 1\n")
            f.must(f.ha("check", GOAL, "AC-2"), "failing AC-2", codes=(1,))
            f.tool("run-producer", 'printf \'hello\\n\' > src/greeting.txt\nexec python3 "$@"\n')
            f.must(f.ha("check", GOAL, "--baseline", "AC-1"), "baseline passing at base", codes=(0, 1))
            if f.records()[-1].get("result") != "unexpected_pass":
                raise Unavailable(f"the baseline did not pass at base: {f.records()[-1].get('result')}")
            f.tool("keep-check", None)
            f.tool("run-producer", None)
            recorded = []
            for round_ in range(EXECUTOR_RECOVERIES + 1):
                f.must(f.ha("check", GOAL, "AC-2"), "missing keep-check", codes=(1,))
                f.must(f.ha("check", GOAL, "AC-2"), "missing keep-check", codes=(1,))
                recorded.append(outcome(self.recover(f, "AC-2", "check", f"attempt {round_}"), "note recover"))
                require(self, recorded[0] == "recorded", "error-recovery-unsupported",
                        f"a recovery past the error limit was {recorded[0]}")
            if recorded != ["recorded"] * EXECUTOR_RECOVERIES + ["refused"]:
                problems.append(f"executor recoveries were not limited to {EXECUTOR_RECOVERIES}: {recorded}")
            allowed = outcome(f.ha("note", GOAL, "allow-recovery", "AC-2", "--input", "check", "--count", "1",
                                   "--quote", "한 번 더 복구해도 돼"), "note allow-recovery")
            again = outcome(self.recover(f, "AC-2", "check", "after the user allowed"), "note recover")
            if (allowed, again) != ("recorded", "recorded"):
                problems.append("a user's allowance did not permit another recovery")
            f.tool("keep-check", "exit 0\n")
            f.must(f.ha("check", GOAL, "AC-2"), "passing AC-2")
            if criterion(f.status(), "AC-2").get("reason") != "criterion_flaky":
                problems.append(f"a recovery erased the failure before it: {criterion(f.status(), 'AC-2')}")
            for _ in range(2):
                f.must(f.ha("check", GOAL, "--baseline", "AC-1"), "missing wrapper", codes=(0, 1))
            f.tool("run-producer", 'exec python3 "$@"\n')
            if outcome(self.recover(f, "AC-1", "baseline", "installed the wrapper"), "note recover") != "recorded":
                problems.append("a baseline-input recovery was refused past the baseline error limit")
            f.must(f.ha("check", GOAL, "--baseline", "AC-1"), "recovered baseline")
            f.implement_greeting()
            f.must(f.ha("check", GOAL, "AC-1"), "AC-1 check")
            if criterion(f.status(), "AC-1").get("reason") != "baseline_unexpected_pass":
                problems.append(f"a recovery erased the unexpected baseline pass before it: "
                                f"{criterion(f.status(), 'AC-1')}")
            require(self, not problems, "error-recovery-unsupported", "; ".join(problems))


# --- AC-7 / AC-8 -------------------------------------------------------------

def timing_limit(line):
    return (line.get("timing") or {}).get("limit_ms")


class TimeLimits(unittest.TestCase):
    """AC-7: plans declare command limits within the maximum; the runs use them."""

    def test_declared_limits_are_applied_and_bound_to_the_command(self):
        with Directory() as d:
            slow = Path(d) / "tools/slow-producer"
            f = Fixture(d, ac1=f"`{slow} checks/reference.py checks/declaration.json checks/reference.json`",
                        limits="\n## 시간 제한\n- AC-1: 3초\n- EX-4: 3초\n- T001 명령 2: 3초\n")
            f.tool("slow-producer", 'sleep 20\nexec python3 "$@"\n')
            require(self, f.start_new(), "command-time-limit-unsupported", "no run-rules/4 goal")
            problems = []
            f.must(f.ha("check", GOAL, "EX-4", timeout=120), "limited EX-4", codes=(1,))
            line = f.records()[-1]
            if timing_limit(line) != 3000 or line.get("result") != "error":
                problems.append(f"EX-4 did not run under its declared 3 s limit: {line.get('result')} "
                                f"{line.get('timing')}")
            f.must(f.ha("check", GOAL, "--baseline", "AC-1", timeout=120), "limited baseline", codes=(0, 1))
            line = f.records()[-1]
            if line.get("kind") != "baseline" or timing_limit(line) != 3000 or \
                    not (line.get("timing") or {}).get("limit_reached"):
                problems.append(f"the AC-1 baseline did not run under its declared 3 s limit: {line.get('result')} "
                                f"{line.get('timing')}")
            f.must(f.ha("check", GOAL, "AC-2"), "undeclared AC-2")
            if timing_limit(f.records()[-1]) != LIMIT_MS:
                problems.append(f"an undeclared command did not use 900 s: {f.records()[-1].get('timing')}")
            f.must(f.ha("check", GOAL, "T001"), "task", codes=(0, 1))
            task_lines = [l for l in f.records() if (l.get("target") or {}).get("task") == "T001"]
            limits = {(l.get("target") or {}).get("index"): timing_limit(l) for l in task_lines}
            if limits.get(1) != LIMIT_MS or limits.get(2) != 3000:
                problems.append(f"task commands did not each use their own limit: {limits}")
            before = criterion(f.status(), "EX-4").get("attempts", {})
            f.write_plan(limits="\n## 시간 제한\n- AC-1: 3초\n- EX-4: 4초\n- T001 명령 2: 3초\n")
            f.commit("change EX-4's limit")
            after = criterion(f.status(), "EX-4").get("attempts", {})
            if after.get("stale", 0) <= before.get("stale", 0):
                problems.append(f"changing EX-4's limit kept its earlier result fresh: {before} -> {after}")
            f.write_plan(limits=f"\n## 시간 제한\n- EX-4: {MAX_LIMIT_S + 1}초\n")
            f.commit("over the maximum")
            count = len(f.records())
            over = f.ha("check", GOAL, "EX-4", timeout=120)
            if over.returncode == 0 or len(f.records()) != count:
                problems.append(f"a limit over the maximum ran ({over.returncode})")
            lint = f.ha("lint", GOAL)
            if lint.returncode != 1 or "EX-4" not in text(lint):
                problems.append(f"ha lint did not name the limit over the maximum: {text(lint)[-300:]}")
            require(self, not problems, "command-time-limit-unsupported", "; ".join(problems))

    def test_earlier_rules_keep_nine_hundred_seconds(self):
        with Directory() as d:
            f = Fixture(d, limits="\n## 시간 제한\n- EX-5: 1초\n")
            f.must(f.start(rules=None), "start")
            if f.started_rules() != "run-rules/3":
                raise Unavailable(f"the default rule is {f.started_rules()}")
            result = f.ha("check", GOAL, "EX-5", timeout=120)
            line = f.records()[-1]
            require(self, result.returncode == 0 and timing_limit(line) == LIMIT_MS,
                    "earlier-rules-changed",
                    f"a run-rules/3 goal applied a declared limit: exit {result.returncode}, {line.get('timing')}")


NOTICE = "time-limit notice:"


def notices(result, target):
    """The notice lines `ha check` printed on stdout for a target before its result line."""
    lines = result.stdout.decode(errors="replace").splitlines()
    end = next((i for i, line in enumerate(lines) if line.startswith(f"{target} ")
                and not line.startswith(NOTICE)), len(lines))
    return [line for line in lines[:end] if line.startswith(NOTICE) and f" {target} " in f"{line} "]


class TimeLimitNotice(unittest.TestCase):
    """AC-8: before a run, the executor learns of earlier runs at or near their limit.

    The plan fixes the form: lines on stdout starting with `time-limit notice:`
    and naming the target, before that target's result line.
    """

    LIMITS = "\n## 시간 제한\n- EX-4: 3초\n- EX-5: 3초\n"

    def test_notice_before_a_run_that_reached_or_neared_its_limit(self):
        with Directory() as d:
            f = Fixture(d, limits=self.LIMITS)
            require(self, f.start_new(), "time-limit-notice-missing", "no run-rules/4 goal")
            problems = []
            first = notices(f.must(f.ha("check", GOAL, "EX-5", timeout=120), "first EX-5"), "EX-5")
            # The contract (run-record.md §4.7) fixes each notice line to one of
            # three forms with nothing added; with no earlier run that is
            # `risk unknown`, which never calls the run safe.
            if first != [f"{NOTICE} EX-5 risk unknown"]:
                problems.append(f"the first run's notice is not exactly `{NOTICE} EX-5 risk unknown`: {first}")
            near = f.records()[-1]
            f.must(f.ha("check", GOAL, "EX-4", timeout=120), "timed-out EX-4", codes=(1,))
            reached = f.records()[-1]
            count = len(f.records())
            again = notices(f.ha("check", GOAL, "EX-4", timeout=120), "EX-4")
            if not any(f"seq {reached['seq']}" in line and "limit reached" in line for line in again):
                problems.append(f"no notice of EX-4's earlier timed-out run before it ran: {again}")
            if len(f.records()) != count + 1:
                problems.append("the notice added records or ran more than the check")
            use = (near.get("timing") or {}).get("limit_elapsed_ms")
            if use is None:
                raise Unavailable(f"EX-5 recorded no limit interval: {near.get('timing')}")
            if use * 1000 >= NOTICE_PERMILLE * 3000:
                lines = notices(f.ha("check", GOAL, "EX-5", timeout=120), "EX-5")
                if not any(f"seq {near['seq']}" in line and "approximate" not in line for line in lines):
                    problems.append(f"no exact notice of EX-5's run at {use} ms of 3000: {lines}")
            require(self, not problems, "time-limit-notice-missing", "; ".join(problems))

    def test_notice_from_duration_alone_is_approximate(self):
        with Directory() as d:
            f = Fixture(d, limits=self.LIMITS)
            require(self, f.start_new(), "time-limit-notice-missing", "no run-rules/4 goal")
            f.must(f.ha("check", GOAL, "EX-5", timeout=120), "only EX-5 run")
            only = f.records()[-1]
            if (only.get("duration_ms") or 0) * 1000 < NOTICE_PERMILLE * 3000:
                raise Unavailable(f"EX-5 ran for {only.get('duration_ms')} ms, under the notice threshold")
            # The only earlier run keeps its duration but loses its limit interval.
            lines = f.records_path.read_bytes().splitlines()
            index = next(i for i, line in enumerate(lines) if json.loads(line)["seq"] == only["seq"])
            lines[index] = re.sub(rb'"limit_elapsed_ms":\d+', b'"limit_elapsed_ms":null', lines[index], count=1)
            f.records_path.write_bytes(rechain(lines))
            found = notices(f.ha("check", GOAL, "EX-5", timeout=120), "EX-5")
            require(self, any(f"seq {only['seq']}" in line and "approximate" in line for line in found),
                    "time-limit-notice-missing", f"a notice from duration alone was not marked approximate: {found}")


# --- AC-9 / AC-13: fixed tokens per document section ---------------------------

def section(path, heading):
    """The text under a heading up to the next heading of the same or higher level.

    A bold paragraph label (`**label**` on its own line, as in role-card.md)
    runs to the next such label or heading.
    """
    try:
        body = (ROOT / path).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError):
        return None
    start = body.find("\n" + heading + "\n")
    if start < 0:
        return None
    rest = body[start + len(heading) + 2:]
    if heading.startswith("**"):
        end = re.search(r"\n(?:\*\*[^\n]*\*\*\n|#)", rest)
    else:
        level = len(heading) - len(heading.lstrip("#"))
        end = re.search(r"\n#{1,%d} " % level, rest)
    return rest[:end.start()] if end else rest


SKILL = "plugins/seal/skills/seal/SKILL.md"
GUIDANCE = {
    (SKILL, "## Autonomy under run-rules/4"): (
        "--test-authority", "ha note <goal> test-change", "ha note <goal> wait", "ha note <goal> resume",
        "ha note <goal> recover", "--preapprove", "`## 시간 제한`", "without asking the user again",
        "outside the recorded authority", "the recovery limit", "keep working on independent work",
        "record a block only when that is all that remains"),
    ("plugins/seal/skills/seal/references/ha.md", "## run-rules/4"): (
        "--test-authority", "test-change", "--moved-to", "--weakened", "wait", "resume", "allow-recovery",
        "--preapprove", "started_at", "reported_wait_seconds"),
}
DOCS = {
    ("contracts/run-record.md", "### 4.7 run-rules/4"): (
        "run-rules/4", "test-change", "wait", "resume", "recover", "allow-recovery", "--preapprove",
        "started_at", "reported_wait_seconds", "span_seconds", "preapproved_runs", f"{MAX_LIMIT_S}",
        f"{NOTICE_PERMILLE}‰", f"{EXECUTOR_RECOVERIES}", "time-limit notice:"),
    ("contracts/bundle.md", "### 2.5 시간 제한 (`## 시간 제한`)"): ("## 시간 제한", f"{MAX_LIMIT_S}", "run-rules/4"),
    ("contracts/core-capabilities.md", "## 6. run-rules/4 기능"): tuple(NEW_FEATURES) + ("run-rules/4",),
    ("contracts/role-card.md", "**run-rules/4 자율 처리**"): ("run-rules/4", "test-change", "wait", "recover"),
    ("guides/USAGE.md", "## 자율 처리 (run-rules/4)"): (
        "--test-authority", "test-change", "wait", "resume", "recover", "allow-recovery", "--preapprove",
        "## 시간 제한"),
    ("guides/USAGE.en.md", "## Autonomy (run-rules/4)"): (
        "--test-authority", "test-change", "wait", "resume", "recover", "allow-recovery", "--preapprove",
        "## 시간 제한"),
}


def missing(places):
    problems = []
    for (path, heading), tokens in places.items():
        body = section(path, heading)
        if body is None:
            problems.append(f"{path} has no {heading!r}")
            continue
        lacking = [t for t in tokens if t not in body]
        if lacking:
            problems.append(f"{path} {heading!r} lacks {lacking}")
    return problems


class Guidance(unittest.TestCase):
    """AC-9: Seal says how to proceed within the new authority and when to stop."""

    def test_seal_guidance_covers_the_new_autonomy(self):
        problems = missing(GUIDANCE)
        require(self, not problems, "autonomy-guidance-missing", "; ".join(problems))


class Documents(unittest.TestCase):
    """AC-13: contracts and both usage guides describe the new rule with the same tokens."""

    def test_contracts_and_guides_describe_run_rules_4(self):
        problems = missing(DOCS)
        require(self, not problems, "autonomy-docs-missing", "; ".join(problems))


# --- AC-10 -------------------------------------------------------------------

def load(path, name):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    if spec is None:
        raise Unavailable(f"{path} cannot be loaded")
    module = importlib.util.module_from_spec(spec)
    sys.path.insert(0, str((ROOT / path).parent))
    try:
        spec.loader.exec_module(module)
    except (OSError, SyntaxError) as error:
        raise Unavailable(f"{path}: {error}") from None
    finally:
        sys.path.pop(0)
    return module


CHECKER = ROOT / "plugins/seal/skills/seal/scripts/check_core.py"


def checker(ha, operation, goal=None):
    """The Seal Core checker's exit code and decision for an operation."""
    argv = [sys.executable, CHECKER, "--ha", ha, "--operation", operation]
    if goal is not None:
        argv += ["--goal", goal]
    result = run(argv, ROOT, dict(os.environ, PYTHONDONTWRITEBYTECODE="1"))
    try:
        return result.returncode, json.loads(result.stdout)
    except ValueError:
        raise Unavailable(f"check_core.py {operation}: {text(result)[-1000:]}") from None


# The identity functions' messages for a Core whose version or capabilities do
# not match. Their other refusals (a process that exited unexpectedly, a
# packaged file or producer problem) are not about the Core's identity.
MISMATCH = ("version does not match", "capability contract", "Core lacks", "capabilities are not an object")


def identity_mismatch(path, error):
    if any(marker in str(error) for marker in MISMATCH):
        return f"{path} does not accept the new Core: {error}"
    raise Unavailable(f"{path}: {error}")


class CoreIdentity(unittest.TestCase):
    """AC-10: the new Core is identified; Seal starts and resumes run-rules/4 only on a Core that supports it.

    Core keeps run-rules/3 as its default rule; a goal chooses run-rules/4 with
    `ha start --rules run-rules/4`, which the new Seal passes (PLAN 구현 결정).
    The CI and release identity functions run on a build stamped with a
    release-like version, as they run on a candidate.
    """

    VERSION = "9.9.9"

    def stamped(self, d):
        out = Path(d) / "stamped/ha"
        env = dict(os.environ, CGO_ENABLED="0")
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        built = run(["go", "build", "-ldflags", f"-X main.version={self.VERSION}", "-o", out, "./cmd/ha"],
                    ROOT, env)
        if built.returncode:
            raise Unavailable(f"go build: {text(built)[-1000:]}")
        return out

    def archive(self, d, binary, release):
        """A release archive of the checked tree's packaged files around the stamped Core."""
        systems = {"Linux": "linux", "Darwin": "darwin"}
        architectures = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}
        system, arch = systems.get(platform.system()), architectures.get(platform.machine().lower())
        if not (system and arch):
            raise Unavailable(f"no release target for {platform.system()} {platform.machine()}")
        prefix = f"ha_{self.VERSION}_{system}_{arch}"
        dist = Path(d) / "dist"
        dist.mkdir()
        with tarfile.open(dist / (prefix + ".tar.gz"), "w:gz") as tar:
            root = tarfile.TarInfo(prefix)
            root.type, root.mode = tarfile.DIRTYPE, 0o755
            tar.addfile(root)
            for name in sorted(release.REQUIRED):
                source = binary if name == "ha" else ROOT / name
                if not source.is_file():
                    raise Unavailable(f"packaged file {name} is missing from the checked tree")
                info = tar.gettarinfo(str(source), f"{prefix}/{name}")
                info.mode = 0o755 if name == "ha" else 0o644
                info.uid = info.gid = 0
                info.uname = info.gname = ""
                with source.open("rb") as body:
                    tar.addfile(info, body)
        return dist

    def test_new_seal_starts_and_resumes_only_on_a_supporting_core(self):
        problems = []
        ha = ha_binary()
        try:
            caps = json.loads(run([ha, "capabilities", "--format", "json"], ROOT).stdout)
        except ValueError:
            raise Unavailable("ha capabilities printed no JSON") from None
        # The default rule stays run-rules/3, so earlier Seal versions and identity paths keep working.
        if caps.get("default_rules") != "run-rules/3" or RULES not in caps.get("supported_rules", []) \
                or not {"run-rules/1", "run-rules/2", "run-rules/3"} <= set(caps.get("supported_rules", [])) \
                or not set(NEW_FEATURES) <= set(caps.get("features", [])):
            problems.append(f"capabilities do not state run-rules/4 and its features beside the kept default: {caps}")
        code, verdict = checker(ha, "start")
        if code or verdict.get("rules") != RULES:
            problems.append(f"the Seal Core checker does not start run-rules/4 on the new Core: {verdict}")
        with Directory() as d:
            fake = Path(d) / "fake/ha"
            fake.parent.mkdir()
            lacking = dict(caps, features=[x for x in caps.get("features", []) if x != "reported-wait/v1"])
            fake.write_text("#!/bin/sh\nif [ \"$1\" = capabilities ]; then cat <<'EOF'\n" + json.dumps(lacking)
                            + "\nEOF\nexit 0; fi\nif [ \"$1\" = --version ]; then echo \"ha "
                            + str(caps.get("ha_version")) + "\"; exit 0; fi\nexit 70\n", encoding="utf-8")
            fake.chmod(0o755)
            if checker(fake, "start")[0] == 0:
                problems.append("the Seal Core checker started on a Core without a run-rules/4 feature")
            # A run-rules/4 goal resumes, checks and changes its budget only on a Core that supports it.
            f = Fixture(Path(d) / "goal4")
            if not f.start_new():
                problems.append("the checked tree's ha cannot start a run-rules/4 goal")
            else:
                for operation in ("resume", "check", "budget"):
                    code, verdict = checker(ha, operation, f.root / GOAL)
                    if code or verdict.get("rules") != RULES:
                        problems.append(f"the Seal Core checker refused {operation} of a run-rules/4 goal: {verdict}")
                if checker(fake, "resume", f.root / GOAL)[0] == 0:
                    problems.append("the Seal Core checker resumed a run-rules/4 goal on a Core without its feature")
            # An existing run-rules/3 goal resumes as it did.
            g = Fixture(Path(d) / "goal3")
            g.must(g.start(rules=None), "start")
            if g.started_rules() != "run-rules/3":
                raise Unavailable(f"the default rule is {g.started_rules()}")
            code, verdict = checker(ha, "resume", g.root / GOAL)
            if code or verdict.get("rules") != "run-rules/3":
                problems.append(f"a run-rules/3 goal no longer resumes: {verdict}")
            stamped = self.stamped(d)
            native = load("tools/ci/native_unix.py", "native_unix_identity")
            try:
                native.identity(stamped, dict(os.environ), self.VERSION, ROOT)
            except ValueError as error:  # native_unix.require
                problems.append(identity_mismatch("tools/ci/native_unix.py", error))
            release = load("tools/release/check_archives.py", "check_archives_identity")
            try:
                release.native_smoke(self.archive(d, stamped, release), self.VERSION)
            except ValueError as error:  # check_archives.require
                problems.append(identity_mismatch("tools/release/check_archives.py", error))
        for path in ("tools/windows/install.ps1", "tools/windows/native_package_checks.ps1"):
            if "run-rules/3" not in (ROOT / path).read_text(encoding="utf-8"):
                problems.append(f"{path} no longer checks run-rules/3 support")
        require(self, not problems, "run-rules-4-unsupported", "; ".join(problems))


# --- AC-11 -------------------------------------------------------------------

def with_rules(f, rules):
    """The fixture's start record, which is its only line, set to another rule."""
    raw = f.records_path.read_bytes()
    if len(raw.splitlines()) != 1:
        raise Unavailable("the start record is not the only line")
    f.records_path.write_bytes(re.sub(rb'"rules":"[^"]*"', b'"rules":"' + rules.encode() + b'"', raw, count=1))


class OldRules(unittest.TestCase):
    """AC-11: goals started with /1, /2 and /3 keep their meanings and refuse the new record kinds."""

    def test_earlier_rules_keep_confirmations_error_limits_and_budget(self):
        problems = []
        for rules in ("run-rules/1", "run-rules/2", "run-rules/3"):
            with Directory() as d:
                tool = Path(d) / "tools/keep-check"
                f = Fixture(d, ac2=f"`{tool}`")
                f.must(f.start(rules=None), "start")
                with_rules(f, rules)
                before = f.records_path.read_bytes()
                for args in (("note", GOAL, "wait", "--reason", "q"), ("note", GOAL, "resume"),
                             ("note", GOAL, "test-change", "tests:tests/keep.sh", "--criteria", "AC-2",
                              "--how", "sh tests/keep.sh"),
                             ("note", GOAL, "recover", "AC-2", "--input", "check", "--change", "c"),
                             ("note", GOAL, "allow-recovery", "AC-2", "--input", "check", "--count", "1",
                              "--quote", "q"),
                             ("budget", GOAL, "--preapprove", "100", "--window", "1", "--revision", "1",
                              "--quote", "q", "--context", "c")):
                    if outcome(f.ha(*args), " ".join(args[2:4])) == "recorded":
                        problems.append(f"{rules}: {' '.join(args[2:4])} was recorded")
                if f.records_path.read_bytes() != before:
                    problems.append(f"{rules}: a refused run-rules/4 form wrote to the record")
                    f.records_path.write_bytes(before)
                f.must(f.ha("note", GOAL, "input", "--quote", "go on"), "input")
                f.must(f.ha("check", GOAL, "AC-2"), "missing tool", codes=(1,))
                f.must(f.ha("check", GOAL, "AC-2"), "missing tool", codes=(1,))
                f.tool("keep-check", "grep -qx keep src/keep.txt\n")
                f.must(f.ha("check", GOAL, "AC-2"), "passing tool")
                if criterion(f.status(), "AC-2").get("reason") != "error_limit":
                    problems.append(f"{rules}: two errors and a pass left {criterion(f.status(), 'AC-2')}")
                keep = f.root / "tests/keep.sh"
                keep.write_text(keep.read_text(encoding="utf-8") + "true\n", encoding="utf-8")
                f.commit("change keep")
                report = f.status()
                if "tests/keep.sh" not in changed_paths(report):
                    problems.append(f"{rules}: a changed test was not held for confirmation")
                if "test_changes" in report or "recoveries" in report:
                    problems.append(f"{rules}: status/v1 gained run-rules/4 fields")
                lines = f.records_path.read_bytes().splitlines()
                times = {json.loads(line)["seq"]: 100 * i for i, line in enumerate(lines)}
                budget = f.status(retimed(f.records_path.read_bytes(), times)).get("budget") or {}
                if budget.get("elapsed_seconds") != 100 * (len(lines) - 1):
                    problems.append(f"{rules}: elapsed time is not the whole window: {budget}")
                if any(k in budget for k in ("reported_wait_seconds", "preapproved_runs")):
                    problems.append(f"{rules}: budget gained run-rules/4 fields: {budget}")
                raised = f.ha("budget", GOAL, "--runs", "80", "--from", "50", "--window", "1", "--revision", "1",
                              "--reason", "more")
                if raised.returncode == 0:
                    problems.append(f"{rules}: a raise without the user's quote was recorded")
        require(self, not problems, "earlier-rules-changed", "; ".join(problems))


SUPPORTED = re.compile(r"^func Supported\(rules string\) bool \{(?:[^\n]*\}$|.*?^\}$)", re.M | re.S)


class OldCoreRefuses(unittest.TestCase):
    """AC-11: an ha without run-rules/4 refuses such a goal and writes nothing.

    The checked tree is built with internal/status/status.go replaced, through
    `go build -overlay`, by a copy whose Supported function knows only
    run-rules/1 to /3, as a released Core does (PLAN 구현 결정).
    """

    def overlay_core(self, d):
        status_go = ROOT / "internal/status/status.go"
        try:
            source = status_go.read_text(encoding="utf-8")
        except OSError as error:
            raise Unavailable(f"internal/status/status.go: {error}") from None
        replaced, count = SUPPORTED.subn('func Supported(rules string) bool {\n\treturn rules == "run-rules/1" '
                                         '|| rules == "run-rules/2" || rules == "run-rules/3"\n}', source)
        if count != 1:
            raise Unavailable("internal/status/status.go has no single Supported(rules string) bool function")
        copy = Path(d) / "status.go"
        copy.write_text(replaced, encoding="utf-8")
        overlay = Path(d) / "overlay.json"
        overlay.write_text(json.dumps({"Replace": {str(status_go): str(copy)}}), encoding="utf-8")
        out = Path(d) / "old-ha"
        env = dict(os.environ, CGO_ENABLED="0")
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        built = run(["go", "build", "-overlay", overlay, "-o", out, "./cmd/ha"], ROOT, env)
        if built.returncode:
            raise Unavailable(f"go build -overlay: {text(built)[-1000:]}")
        return out

    def test_core_without_run_rules_4_refuses_such_a_goal(self):
        with Directory() as d:
            out = self.overlay_core(d)
            problems = []
            old = Fixture(Path(d) / "goal3")
            old.must(old.start(rules=None), "start")
            f = Fixture(Path(d) / "goal")
            if f.start_new():
                # The real start line keeps every field an earlier reader knows, with the same JSON type.
                problems += [f"the run-rules/4 start line changes {name}" for name in
                             changed_types(old.records()[0], f.records()[0])]
            else:
                # The checked ha has no run-rules/4: give an earlier line that rule.
                f.records_path.unlink(missing_ok=True)
                f.must(f.start(rules=None), "start")
                with_rules(f, RULES)
            raw = f.records_path.read_bytes()
            for args in (("status", GOAL), ("check", GOAL, "AC-2"), ("note", GOAL, "input", "--quote", "x"),
                         ("budget", GOAL, "--runs", "60", "--from", "50", "--window", "1", "--revision", "1",
                          "--quote", "q", "--context", "c")):
                result = f.ha(*args, ha=out)
                if result.returncode != 66:
                    problems.append(f"an ha without run-rules/4: {args[0]} exited {result.returncode}")
            if f.records_path.read_bytes() != raw:
                problems.append("an ha without run-rules/4 wrote to a run-rules/4 record")
            require(self, not problems, "old-core-accepts-new-rules", "; ".join(problems))


def changed_types(before, after, path=""):
    """Keys of `before` whose JSON type differs in `after`, recursively; added keys are fine."""
    # An integer stays an integer: a Core that decodes it as one rejects 50.0.
    kind = lambda value: type(value).__name__
    changed = []
    for key, value in before.items():
        name = f"{path}{key}"
        if key not in after:
            if key != "rules":
                changed.append(f"{name} (removed)")
        elif kind(value) != kind(after[key]):
            changed.append(f"{name} ({kind(value)} to {kind(after[key])})")
        elif isinstance(value, dict):
            changed += changed_types(value, after[key], name + ".")
    return changed


# --- AC-12 -------------------------------------------------------------------

class KeptGuarantees(unittest.TestCase):
    """AC-12: run-rules/4 keeps every attempt and the guarantees of run-rules/3."""

    def test_run_rules_4_keeps_failures_baselines_and_refusals(self):
        with Directory() as d:
            tool = Path(d) / "tools/keep-check"
            f = Fixture(d, ac2=f"`{tool}`")
            require(self, f.start_new(), "rules-4-integrity-missing", "no run-rules/4 goal")
            problems = []
            # A change condition is not satisfied without its baseline.
            f.implement_greeting()
            f.must(f.ha("check", GOAL, "AC-1"), "AC-1 without a baseline")
            if criterion(f.status(), "AC-1").get("reason") != "baseline_missing":
                problems.append(f"AC-1 without a baseline: {criterion(f.status(), 'AC-1')}")
            # A failure and a later pass of the same input stay flaky.
            f.tool("keep-check", "exit 1\n")
            f.must(f.ha("check", GOAL, "AC-2"), "failing AC-2", codes=(1,))
            f.tool("keep-check", "exit 0\n")
            f.must(f.ha("check", GOAL, "AC-2"), "passing AC-2")
            if criterion(f.status(), "AC-2").get("reason") != "criterion_flaky":
                problems.append(f"a later pass erased a failure of the same input: {criterion(f.status(), 'AC-2')}")
            # A dirty tree is refused before running, without a record.
            count = len(f.records())
            (f.root / "src/keep.txt").write_text("dirty\n", encoding="utf-8")
            refused = f.ha("check", GOAL, "AC-2")
            (f.root / "src/keep.txt").write_text("keep\n", encoding="utf-8")
            if refused.returncode != 65 or len(f.records()) != count:
                problems.append(f"a dirty check was not refused without a record ({refused.returncode})")
            report = f.status()
            if report.get("assurance") != "local" or report.get("check_author") != "executor":
                problems.append(f"status lost the local assurance disclosure: {report.get('assurance')}")
            if not report.get("usage"):
                problems.append("status lost usage")
            require(self, not problems, "rules-4-integrity-missing", "; ".join(problems))

    def test_run_rules_4_keeps_an_unexpected_baseline_pass(self):
        with Directory() as d:
            f = Fixture(d)
            (f.root / "src/greeting.txt").write_text("hello\n", encoding="utf-8")
            f.commit("hello at base")
            require(self, f.start_new(), "rules-4-integrity-missing", "no run-rules/4 goal")
            f.must(f.ha("check", GOAL, "--baseline", "AC-1"), "unexpected baseline pass", codes=(0, 1))
            f.must(f.ha("check", GOAL, "AC-1"), "AC-1")
            require(self, criterion(f.status(), "AC-1").get("reason") == "baseline_unexpected_pass",
                    "rules-4-integrity-missing",
                    f"a pass erased an unexpected baseline pass: {criterion(f.status(), 'AC-1')}")

    def test_run_rules_4_keeps_confirmations_and_record_integrity(self):
        with Directory() as d:
            f = Fixture(d, manual=True)
            require(self, f.start_new(), "rules-4-integrity-missing", "no run-rules/4 goal")
            problems = []
            f.implement()
            (f.root / "notes.txt").write_text("outside the scope\n", encoding="utf-8")
            f.commit("outside the scope")
            spec = f.root / GOAL / "SPEC.md"
            spec.write_text(spec.read_text(encoding="utf-8") + "\nA later sentence.\n", encoding="utf-8")
            report = f.status()
            codes = {r.get("code") for r in report.get("reasons", [])}
            for code in ("manual_unconfirmed", "out_of_scope", "spec_changed"):
                if code not in codes:
                    problems.append(f"{code} is gone: {sorted(codes)}")
            # Dropping a middle line breaks the hash chain at the line after it.
            lines = f.records_path.read_bytes().splitlines()
            broken = f.status(b"\n".join(lines[:1] + lines[2:]) + b"\n")
            if "record_integrity" not in {r.get("code") for r in broken.get("reasons", [])}:
                problems.append(f"a broken hash chain was accepted: {broken.get('reasons')}")
            require(self, not problems, "rules-4-integrity-missing", "; ".join(problems))

    def test_run_rules_4_clears_an_error_limit_only_by_recovery(self):
        with Directory() as d:
            tool = Path(d) / "tools/keep-check"
            f = Fixture(d, ac2=f"`{tool}`")
            require(self, f.start_new(), "rules-4-integrity-missing", "no run-rules/4 goal")
            problems = []
            f.must(f.ha("check", GOAL, "AC-2"), "missing tool", codes=(1,))
            f.must(f.ha("check", GOAL, "AC-2"), "missing tool", codes=(1,))
            f.tool("keep-check", "grep -qx keep src/keep.txt\n")

            def limited(after):
                if criterion(f.status(), "AC-2").get("reason") != "error_limit":
                    problems.append(f"{after} cleared the error limit: {criterion(f.status(), 'AC-2')}")

            f.must(f.ha("note", GOAL, "input", "--quote", "계속해줘"), "input")
            limited("a user's input")
            budget = f.status().get("budget") or {}
            f.must(f.ha("budget", GOAL, "--runs", "60", "--from", str(budget.get("runs_limit")), "--window",
                        str(budget.get("window_start")), "--revision", str(budget.get("revision")),
                        "--quote", "60회까지 늘려도 돼", "--context", "실행 상한"), "budget change")
            limited("a budget change")
            f.must(f.ha("note", GOAL, "reopen", "--quote", "다시 해줘"), "reopen")
            limited("reopen")
            f.must(f.ha("check", GOAL, "AC-2"), "passing AC-2")
            limited("a later pass")
            require(self, not problems, "rules-4-integrity-missing", "; ".join(problems))

    def test_run_rules_4_completes_with_only_the_budget_exceeded(self):
        with Directory() as d:
            f = Fixture(d)
            require(self, f.start_new(), "rules-4-integrity-missing", "no run-rules/4 goal")
            f.implement()
            f.must(f.ha("budget", GOAL, "--runs", "2", "--from", "50", "--window", "1", "--revision", "1",
                        "--reason", "lower"), "lower the limit")
            done = f.ha("done", GOAL)
            report = f.status()
            require(self, done.returncode == 0 and report.get("status") == "complete"
                    and (report.get("budget") or {}).get("exceeded") is True, "rules-4-integrity-missing",
                    f"a run-rules/4 goal with only the budget exceeded: done {done.returncode}, "
                    f"{report.get('status')}, {report.get('budget')}")


if __name__ == "__main__":
    unittest.main()
