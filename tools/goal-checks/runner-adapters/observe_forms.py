"""Runner output forms through the built `ha` (AC-3, AC-4, AC-6, AC-7, AC-11, AC-12).

Each observation makes one synthetic goal whose change conditions each link one
test to an adapter declaration (check-adapter/v1). Go forms run the real
`go test -json` of the selected Go 1.27 toolchain on a synthetic module;
JUnit forms come from tools/goal-checks/runner-adapters/runner.py, a separate
process that writes one fixed XML file per case in the shapes pytest, Vitest
and jest-junit write, to the location `ha` names. Every condition is recorded
by `ha check --baseline` and `ha check`, and the stored result, reason and
target observations are compared with the SPEC's expectation (판정 규칙).

AC-12 also runs the same fixed output bytes through a Linux build of `ha` in
the local `alpine/git` container image (never pulled, no network) and, for
Windows, requires the native check the Windows CI jobs run and the passing CI
run of the checked commit (tools/verification-checks/cross_platform_ci.py).
"""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import stat
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from adapters_common import (GOAL, PLACEHOLDER, ROOT, UNMARKED, Condition, Repo, Unavailable, build, clean_env,
                             go_declaration, go_test_results, go_typed, junit_declaration, node_runner, require, run,
                             runner_command, runner_file, summary, temporary)

PKG = "example.com/demo/forms"
MARK_ID = "greeting-missing"
GO_MOD = "module example.com/demo\n\ngo 1.27\n"
FORMS = """package forms

import (
	"fmt"
	"testing"
	"time"
)

const mark = "jaekit:violation=greeting-missing"

var flips int

func TestPassLog(t *testing.T)               { t.Log(mark + " logged") }
func TestMarkedFatal(t *testing.T)           { t.Fatalf("%s broke", mark) }
func TestErrorfMarked(t *testing.T)          { t.Errorf("%s via Errorf", mark) }
func TestLogThenUnmarked(t *testing.T)       { t.Log(mark); t.Fatal("plain failure") }
func TestTwoErrorsOneMarked(t *testing.T)    { t.Error(mark + " one"); t.Error("two") }
func TestMultilineSecondMarked(t *testing.T) { t.Error("first line\\n" + mark + " second") }
func TestStdoutMarked(t *testing.T)          { fmt.Println(mark + " stdout"); t.Fail() }
func TestOtherMarker(t *testing.T)           { t.Fatal("jaekit:violation=other-id broke") }
func TestMixedMarkers(t *testing.T)          { t.Error(mark); t.Error("jaekit:violation=other-id") }
func TestSkipped(t *testing.T)               { t.Skip(mark + " skipped") }
func TestAlwaysMarked(t *testing.T)          { t.Fatal(mark + " every run") }
func TestSlowly(t *testing.T)                { time.Sleep(5 * time.Second) }

func TestParentChildMarked(t *testing.T) {
	t.Run("child", func(t *testing.T) { t.Fatal(mark + " child") })
}

func TestFlip(t *testing.T) {
	flips++
	if flips == 2 {
		t.Fatal(mark + " second run")
	}
}
"""
PACKAGES = {
    "forms/forms_test.go": FORMS,
    "panics/panics_test.go": 'package panics\n\nimport "testing"\n\n'
                             'func TestPanics(t *testing.T) { panic("jaekit:violation=greeting-missing panicked") }\n',
    "exits/exits_test.go": 'package exits\n\nimport (\n\t"os"\n\t"testing"\n)\n\n'
                           'func TestExits(t *testing.T) { os.Exit(3) }\n',
    "broken/broken_test.go": 'package broken\n\nimport "testing"\n\n'
                             'func TestCallsMissing(t *testing.T) { Missing() }\n',
    "one/same_test.go": 'package one\n\nimport "testing"\n\n'
                        'func TestSame(t *testing.T) { t.Log("jaekit:violation=greeting-missing logged") }\n',
    "two/same_test.go": 'package two\n\nimport "testing"\n\n'
                        'func TestSame(t *testing.T) { t.Fatal("jaekit:violation=greeting-missing in two") }\n',
}
GO_FILES = {"go.mod": GO_MOD, **PACKAGES}


class Row:
    """One linked condition: its command, declaration and expected baseline and current results."""

    def __init__(self, name, command, declaration, current, baseline=None, checks=("tests/runner.py",), limit=None):
        self.name, self.command, self.declaration, self.limit = name, command, declaration, limit
        self.current, self.baseline = current, current if baseline is None else baseline
        self.checks = list(checks)


def go_row(name, test, run_pattern, current, baseline=None, packages=("./forms/",), package=PKG,
           violation=MARK_ID, flags=()):
    files = sorted({f"{p.strip('./')}/" for p in packages})
    checks = [path for path in PACKAGES if any(path.startswith(f) for f in files)] + ["go.mod"]
    return Row(name, ["go", "test", "-json", "-count=1", *flags, "-run", run_pattern, *packages],
               go_declaration([("t", violation, package, test)]), current, baseline, checks)


def runner_go_row(name, case, current, baseline=None, test="TestMarked"):
    return Row(name, runner_command(case), go_declaration([("t", MARK_ID, PKG, test)]), current, baseline)


def junit_row(name, case, current, baseline=None, classname="tests.test_greet", test="test_greets",
              violation=MARK_ID, placeholder=True):
    command = runner_command(case, PLACEHOLDER) if placeholder else runner_command(case)
    return Row(name, command, junit_declaration([("t", violation, classname, test)]), current, baseline)


def obs(status="pass", violation="", reason="", attempt=1, target="t"):
    return (target, attempt, status, violation, reason)


PASS = ("pass", "targets_passed", [obs()])
UNEXPECTED_PASS = ("unexpected_pass", "baseline_passed", [obs()])
DECLARED_CURRENT = ("fail", "assertion_failed", [obs("violation", MARK_ID)])
DECLARED_BASELINE = ("fail_as_expected", "declared_violation", [obs("violation", MARK_ID)])
UNMARKED_FAIL = ("fail", "unexpected_assertion", [obs("violation", UNMARKED)])


def unknown(reason, targets=()):
    return ("unknown", reason, list(targets))


def error(reason, targets=()):
    return ("error", reason, list(targets))


def environment(reason):
    return error("environment_reported", [obs("error", reason=reason)])


def declared(row_factory, *args, **kwargs):
    return row_factory(*args, DECLARED_CURRENT, DECLARED_BASELINE, **kwargs)


def passes(row_factory, *args, **kwargs):
    return row_factory(*args, PASS, UNEXPECTED_PASS, **kwargs)


def table(case, rows, violation, extra_files=None, solo=()):
    """Record every row's baseline and current run; return [(row, kind, got, want)] that differ, and outputs."""
    with temporary("jaekit-adapter-forms-") as directory:
        ha = build(Path(directory) / "ha")
        conditions = [Condition(f"AC-{i}", r.command, r.checks, r.declaration, limit=r.limit)
                      for i, r in enumerate(rows, 1)]
        files = {"tests/runner.py": runner_file(), "tests/noexec.sh": "exit 0\n", **GO_FILES, **(extra_files or {})}
        repo = Repo(directory, ha, conditions, files=files, runs=4 * len(rows) + 10)
        code, out = repo.start()
        require(case, code == 0, violation, f"ha start refused a goal with check-adapter/v1 declarations: {out[-1500:]}")
        ids = {r.name: c.id for r, c in zip(rows, conditions)}
        batch = [ids[r.name] for r in rows if r.name not in solo]
        outputs = {}
        for baseline in (True, False):
            if batch:
                _, outputs[baseline] = repo.check(*batch, baseline=baseline)
            for name in solo:
                _, text = repo.check(ids[name], baseline=baseline)
                outputs[baseline] = outputs.get(baseline, "") + text
                restore_logs(repo)
        differences = []
        for r in rows:
            for kind, want in (("baseline", r.baseline), ("check", r.current)):
                got = summary(repo.last(ids[r.name], kind))
                if got is None or got[0] != want[0] or got[1] != want[1] or sorted(got[2]) != sorted(want[2]):
                    differences.append(f"{r.name} {kind}: got {got}, want {want}")
        return differences, outputs, repo, ids


def restore_logs(repo):
    runs = repo.root / ".git" / "ha" / "runs"
    for path in [runs, *runs.glob("*")]:
        if path.is_dir():
            path.chmod(0o755)


class Markers(unittest.TestCase):
    """AC-3: a linked test that ran to its end follows the marker rules for both adapters."""

    def test_go_and_junit_marker_rules(self):
        go_typed()
        rows = [
            passes(go_row, "G1 log marker on a passing test", "TestPassLog", "^TestPassLog"),
            declared(go_row, "G2 t.Fatalf marker", "TestMarkedFatal", "^TestMarkedFatal"),
            declared(go_row, "G2 t.Errorf marker", "TestErrorfMarked", "^TestErrorfMarked"),
            go_row("G3 logged marker then unmarked Fatal", "TestLogThenUnmarked", "^TestLogThenUnmarked", UNMARKED_FAIL),
            go_row("G4 one of two errors marked", "TestTwoErrorsOneMarked", "^TestTwoErrorsOneMarked", UNMARKED_FAIL),
            go_row("G5 marker on a continuation line", "TestMultilineSecondMarked", "^TestMultilineSecondMarked", UNMARKED_FAIL),
            go_row("G6 panic", "TestPanics", "^TestPanics", UNMARKED_FAIL, packages=("./panics/",),
                   package="example.com/demo/panics"),
            go_row("G7 parent of a marked child", "TestParentChildMarked", "^TestParentChildMarked", UNMARKED_FAIL),
            declared(go_row, "G8 marked child", "TestParentChildMarked/child", "^TestParentChildMarked"),
            go_row("marker on standard output only", "TestStdoutMarked", "^TestStdoutMarked", UNMARKED_FAIL),
            go_row("another identifier", "TestOtherMarker", "^TestOtherMarker", ("fail", "unexpected_assertion", [obs("violation", "other-id")])),
            go_row("mixed identifiers", "TestMixedMarkers", "^TestMixedMarkers", UNMARKED_FAIL),
            go_row("G12 linked package passes, the other fails marked", "TestSame", "^TestSame",
                   unknown("report_exit_conflict", [obs()]), packages=("./one/", "./two/"), package="example.com/demo/one"),
            declared(go_row, "G12 linked package fails marked", "TestSame", "^TestSame", packages=("./one/", "./two/"),
                     package="example.com/demo/two"),
            passes(junit_row, "J1 testcase without children", "J1"),
            declared(junit_row, "J2 pytest assert message", "J2"),
            junit_row("J3 pytest traceback source line", "J3", UNMARKED_FAIL),
            declared(junit_row, "J4 Vitest expect message", "J4", classname="tests/greet.test.ts", test="greets with hello"),
            declared(junit_row, "J5 body first line", "J5", classname="greet greets with hello", test="greets with hello"),
            junit_row("J10 marker only in system-out", "J10", UNMARKED_FAIL),
            junit_row("JUnit another identifier", "junit-other-id", ("fail", "unexpected_assertion", [obs("violation", "other-id")])),
            junit_row("JUnit mixed identifiers", "junit-mixed-ids", UNMARKED_FAIL),
            declared(junit_row, "JUnit every failure marked", "junit-two-marked"),
            junit_row("JUnit one of two failures marked", "junit-one-of-two", UNMARKED_FAIL),
            junit_row("JUnit empty message attribute, marker in body", "junit-empty-message", UNMARKED_FAIL),
            junit_row("JUnit body whose first line is blank", "junit-blank-body", UNMARKED_FAIL),
            junit_row("JUnit identifier over 128 characters", "junit-long-id", UNMARKED_FAIL),
        ]
        differences, _, _, _ = table(self, rows, "marker-rules-unmet")
        require(self, not differences, "marker-rules-unmet", "; ".join(differences))


class InputFailures(unittest.TestCase):
    """AC-4: every input failure of the SPEC table records its result and reason, never success."""

    def test_input_failure_table(self):
        go_typed()
        go_missing = go_row("missing Go test", "TestAbsent", "^TestPassLog", unknown("target_missing"))
        rows = [
            Row("runner not on PATH", ["jaekit-no-such-runner", PLACEHOLDER],
                junit_declaration([("t", MARK_ID, "tests.test_greet", "test_greets")]), error("process_start")),
            Row("runner without execute permission", ["./tests/noexec.sh", PLACEHOLDER],
                junit_declaration([("t", MARK_ID, "tests.test_greet", "test_greets")]), error("process_start"),
                checks=("tests/noexec.sh",)),
            Row("ha time limit", runner_command("sleep"), go_declaration([("t", MARK_ID, PKG, "TestMarked")]),
                error("process_timeout"), limit="2초"),
            Row("signal", runner_command("signal"), go_declaration([("t", MARK_ID, PKG, "TestMarked")]),
                error("process_signal")),
            Row("output storage failure", runner_command("lock-log"), go_declaration([("t", MARK_ID, PKG, "TestMarked")]),
                error("process_output")),
            go_row("G9 package does not build", "TestCallsMissing", "^TestCallsMissing",
                   environment("compile_error"), packages=("./broken/",), package="example.com/demo/broken"),
            go_row("G10 test binary exits during the test", "TestExits", "^TestExits",
                   environment("execution_error"), packages=("./exits/",), package="example.com/demo/exits"),
            go_row("Go test timeout", "TestSlowly", "^TestSlowly", environment("execution_error"), flags=("-timeout", "1s")),
            junit_row("J6 error element", "J6", environment("execution_error")),
            junit_row("error element with a failure", "junit-error-and-failure", environment("execution_error")),
            go_missing,
            junit_row("JUnit other names only", "junit-other-names", unknown("target_missing")),
            junit_row("J8 nothing collected", "J8", unknown("target_missing")),
            go_row("Go skip", "TestSkipped", "^TestSkipped", unknown("target_skipped", [obs("skip", reason="skipped")])),
            junit_row("J7 skipped", "J7", unknown("target_skipped", [obs("skip", reason="skipped")])),
            runner_go_row("G14 standard error only", "go-stderr-only", unknown("output_missing")),
            junit_row("J11 no file", "junit-none", unknown("output_missing")),
            junit_row("JUnit directory at the location", "junit-dir", unknown("output_missing")),
            junit_row("JUnit symlink at the location", "junit-symlink", unknown("output_missing")),
            runner_go_row("G13 non-JSON line", "go-garbage", unknown("output_corrupt")),
            runner_go_row("truncated JSON", "go-truncated", unknown("output_corrupt")),
            runner_go_row("blank line", "go-blank-line", unknown("output_corrupt")),
            runner_go_row("invalid UTF-8", "go-bad-utf8", unknown("output_corrupt")),
            junit_row("J11 truncated XML", "J11-truncated", unknown("output_corrupt")),
            junit_row("J11 DTD and entity declaration", "J11-dtd", unknown("output_corrupt")),
            junit_row("XML syntax error", "junit-syntax", unknown("output_corrupt")),
            junit_row("two root elements", "junit-two-roots", unknown("output_corrupt")),
            junit_row("not a JUnit document", "junit-not-junit", unknown("output_corrupt")),
            junit_row("non-UTF-8 encoding", "junit-latin1", unknown("output_corrupt")),
            junit_row("invalid UTF-8 bytes", "junit-bad-utf8", unknown("output_corrupt")),
            runner_go_row("go output over 64 MiB", "go-oversize", unknown("output_too_large")),
            junit_row("JUnit file over 16 MiB", "junit-oversize", unknown("output_too_large")),
            junit_row("J9 duplicate testcase", "J9", unknown("attempt_history_missing")),
            junit_row("rerunFailure element", "junit-rerun", unknown("attempt_history_missing")),
            junit_row("flakyFailure element", "junit-flaky", unknown("attempt_history_missing")),
            junit_row("rerunError element", "junit-rerun-error", unknown("attempt_history_missing")),
            junit_row("flakyError element", "junit-flaky-error", unknown("attempt_history_missing")),
            junit_row("error element with exit 0", "junit-error-exit0",
                      unknown("report_exit_conflict", [obs("error", reason="execution_error")])),
            junit_row("all passing with exit 1", "junit-pass-exit1", unknown("report_exit_conflict", [obs()])),
            junit_row("marked failure with exit 0", "junit-marked-exit0", unknown("report_exit_conflict", [obs("violation", MARK_ID)])),
            runner_go_row("Go marked failure with exit 0", "go-marked-exit0", unknown("report_exit_conflict", [obs("violation", MARK_ID)])),
            runner_go_row("unknown Go action on the linked test", "go-unknown-action", unknown("observation_invalid")),
            junit_row("unknown testcase child", "junit-unknown-child", unknown("observation_invalid")),
            junit_row("skipped with a failure", "junit-skip-and-failure", unknown("observation_invalid")),
            runner_go_row("standard error is not read", "go-pass-noisy-stderr", PASS, UNEXPECTED_PASS),
        ]
        differences, outputs, _, _ = table(self, rows, "input-failure-misclassified", solo=("output storage failure",))
        for reason in ("output_missing", "output_corrupt", "output_too_large"):
            for baseline, text in outputs.items():
                if f"classification: {reason}" not in text:
                    differences.append(f"{'baseline' if baseline else 'current'} ha check output does not show classification {reason}")
        require(self, not differences, "input-failure-misclassified", "; ".join(differences))


GREET_BASE = 'package greet\n\nfunc Hello() string { return "hi" }\n'
GREET_DONE = 'package greet\n\nfunc Hello() string { return "hello" }\n'
GREET_TEST = """package greet

import "testing"

const mark = "jaekit:violation=greeting-missing"

func TestLinkedGreets(t *testing.T) {
	if got := Hello(); got != "hello" {
		t.Fatalf("%s greeting is %q", mark, got)
	}
}

func TestUnlinkedFails(t *testing.T) { t.Fatal(mark + " unrelated") }
"""
NODE_BASE = "export function greet() { return 'hi'; }\n"
NODE_DONE = "export function greet() { return 'hello'; }\n"
NODE_TEST = """import { test } from 'node:test';
import assert from 'node:assert/strict';
import { greet } from '../src/greet.mjs';

test('linked greets', () => { assert.equal(greet(), 'hello', 'jaekit:violation=greeting-missing wrong greeting'); });
test('unlinked fails', () => { assert.fail('jaekit:violation=greeting-missing unrelated'); });
"""


class Unlinked(unittest.TestCase):
    """AC-6: tests a command runs but no target links never change a target's observation."""

    def test_unlinked_failures(self):
        go_typed()
        node_runner()
        node = "node"
        problems = []
        with temporary("jaekit-adapter-unlinked-") as directory:
            ha = build(Path(directory) / "ha")
            conditions = [
                Condition("AC-1", ["go", "test", "-json", "-count=1", "./greet/"], ["greet/greet_test.go", "go.mod"],
                          go_declaration([("linked", MARK_ID, "example.com/demo/greet", "TestLinkedGreets")])),
                Condition("AC-2", [node, "--test", "--test-reporter=junit", f"--test-reporter-destination={PLACEHOLDER}",
                                   "test/greet.test.mjs"], ["test/greet.test.mjs"],
                          junit_declaration([("linked", MARK_ID, "test", "linked greets")])),
            ]
            files = {"go.mod": GO_MOD, "greet/greet.go": GREET_BASE, "greet/greet_test.go": GREET_TEST,
                     "src/greet.mjs": NODE_BASE, "test/greet.test.mjs": NODE_TEST}
            repo = Repo(directory, ha, conditions, files=files)
            code, out = repo.start()
            require(self, code == 0, "unlinked-test-counted", f"ha start refused: {out[-1500:]}")
            repo.check("AC-1", "AC-2", baseline=True)
            repo.write("greet/greet.go", GREET_DONE)
            repo.write("src/greet.mjs", NODE_DONE)
            repo.commit("implement")
            repo.check("AC-1", "AC-2")
            for cid in ("AC-1", "AC-2"):
                base, current = summary(repo.last(cid, "baseline")), summary(repo.last(cid, "check"))
                want_base = ("fail_as_expected", "declared_violation", [("linked", 1, "violation", MARK_ID, "")])
                want_current = ("unknown", "report_exit_conflict", [("linked", 1, "pass", "", "")])
                if base != want_base:
                    problems.append(f"{cid} baseline with a failing unlinked test: got {base}, want {want_base}")
                if current != want_current:
                    problems.append(f"{cid} check with a failing unlinked test: got {current}, want {want_current}")
        require(self, not problems, "unlinked-test-counted", "; ".join(problems))


class AttemptsAndNames(unittest.TestCase):
    """AC-7: attempts stay in order and same-named tests never overwrite each other."""

    def test_attempts_and_names(self):
        go_typed()
        flip = ("fail", "inconsistent_attempts", [obs(), obs("violation", MARK_ID, attempt=2)])
        three = [obs("violation", MARK_ID, attempt=a) for a in (1, 2, 3)]
        rows = [
            go_row("-count=2 pass then marked", "TestFlip", "^TestFlip", flip, flags=("-count=2",)),
            go_row("-count=3 marked", "TestAlwaysMarked", "^TestAlwaysMarked", ("fail", "assertion_failed", three),
                   ("fail_as_expected", "declared_violation", three), flags=("-count=3",)),
            go_row("same name, linked package passes", "TestSame", "^TestSame", unknown("report_exit_conflict", [obs()]),
                   packages=("./one/", "./two/"), package="example.com/demo/one"),
            declared(go_row, "same name, linked package fails", "TestSame", "^TestSame", packages=("./one/", "./two/"),
                     package="example.com/demo/two"),
            go_row("parent of a marked child", "TestParentChildMarked", "^TestParentChildMarked", UNMARKED_FAIL),
            declared(go_row, "marked child", "TestParentChildMarked/child", "^TestParentChildMarked"),
            junit_row("same name, linked classname passes", "junit-same-name", unknown("report_exit_conflict", [obs()]),
                      classname="tests.test_a"),
            declared(junit_row, "same name, linked classname fails", "junit-same-name", classname="tests.test_b"),
            declared(junit_row, "same name flipped, linked classname fails", "junit-same-name-flipped", classname="tests.test_a"),
            junit_row("duplicate testcase", "J9", unknown("attempt_history_missing")),
            junit_row("rerun element", "junit-rerun", unknown("attempt_history_missing")),
        ]
        # The flip test needs -count=2 only; drop the -count=1 the row helper adds.
        for row in rows[:2]:
            row.command.remove("-count=1")
        differences, _, _, _ = table(self, rows, "attempts-or-names-mixed")
        require(self, not differences, "attempts-or-names-mixed", "; ".join(differences))


class UntypedToolchain(unittest.TestCase):
    """AC-11: output without failure-output types never yields the declared violation, and ha says why."""

    def test_untyped_output(self):
        rows = [
            runner_go_row("untyped marked failure", "go-untyped-fail", UNMARKED_FAIL),
            runner_go_row("untyped pass", "go-untyped-pass", PASS, UNEXPECTED_PASS),
            runner_go_row("typed marked failure", "go-typed-fail", DECLARED_CURRENT, DECLARED_BASELINE),
        ]
        differences, outputs, _, ids = table(self, rows, "untyped-output-misread")
        failing = ids["untyped marked failure"]
        for baseline, text in outputs.items():
            notices = [line for line in text.splitlines() if line.startswith("notice: ") and failing in line]
            if not any("OutputType" in line for line in notices):
                differences.append(f"{'baseline' if baseline else 'current'} ha check does not tell why the marker of "
                                   f"{failing} was not read (no `notice:` line naming OutputType)")
            for other in (ids["untyped pass"], ids["typed marked failure"]):
                if any(line.startswith("notice: ") and other in line.split() for line in text.splitlines()):
                    differences.append(f"{other} has an untyped-output notice although its markers were readable or unneeded")
        require(self, not differences, "untyped-output-misread", "; ".join(differences))


# Output pairs whose LF and CRLF bytes must classify the same (J12).
CRLF_PAIRS = [("go-typed-fail", "go-crlf-fail"), ("go-typed-unmarked", "go-crlf-unmarked"), ("go-typed-pass", "go-crlf-pass"),
              ("J1", "J1-crlf"), ("J2", "J2-crlf"), ("J3", "J3-crlf"), ("J5", "J5-crlf"), ("J10", "J10-crlf")]
JUNIT_TESTS = {"J5": ("greet greets with hello", "greets with hello")}
# The Windows native check of the adapter bytes (cmd: go test ./internal/evidence -run ...).
WINDOWS_NATIVE = {"internal/evidence": "TestNativeAdapterClassification"}
# The cases that test reads (PLAN 구현 결정, Windows 관측): each LF form of CRLF_PAIRS with the runner's bytes, the
# standard declaration and the current-check classification the SPEC gives. The test classifies each output as
# written and with CRLF line endings in subtests <case>/lf and <case>/crlf, reading the file named by
# PLATFORM_VARIABLE when it is set. Run here with these cases it must pass, and with every expected result changed
# every subtest must fail; so a passing Windows run shows these results on Windows.
PLATFORM_FIXTURE = "internal/evidence/testdata/adapter-platform.json"
PLATFORM_VARIABLE = "JAEKIT_ADAPTER_PLATFORM_FIXTURE"
PLATFORM_TEST = "TestNativeAdapterClassification"
PLATFORM_DECLARED = ("fail", "assertion_failed", [["t", 1, "violation", MARK_ID, ""]])
PLATFORM_UNMARKED = ("fail", "unexpected_assertion", [["t", 1, "violation", UNMARKED, ""]])
PLATFORM_PASSED = ("pass", "targets_passed", [["t", 1, "pass", "", ""]])
PLATFORM_EXPECTED = {"go-typed-fail": PLATFORM_DECLARED, "go-typed-unmarked": PLATFORM_UNMARKED,
                     "go-typed-pass": PLATFORM_PASSED, "J1": PLATFORM_PASSED, "J2": PLATFORM_DECLARED,
                     "J3": PLATFORM_UNMARKED, "J5": PLATFORM_DECLARED, "J10": PLATFORM_UNMARKED}
EMIT = 'if [ "$1" = go ]; then cat "$2"; else cp "$2" "$4"; fi\nexit "$3"\n'


def crlf_row(case):
    base = case[:-5] if case.endswith("-crlf") else case
    if base.startswith("go-") or base.startswith("go-crlf"):
        return Row(case, runner_command(case), go_declaration([("t", MARK_ID, PKG, "TestMarked")]), None)
    classname, test = JUNIT_TESTS.get(base, ("tests.test_greet", "test_greets"))
    return Row(case, runner_command(case, PLACEHOLDER), junit_declaration([("t", MARK_ID, classname, test)]), None)


def fixture_outputs(directory, cases):
    """{case: (adapter, bytes, exit)} as runner.py writes them."""
    found = {}
    for case in cases:
        out = Path(directory) / f"fixture-{case}"
        result = run([sys.executable, Path(__file__).resolve().parent / "runner.py", case, out], directory, timeout=120)
        if case.startswith("go-"):
            found[case] = ("go", result.stdout, result.returncode)
        else:
            try:
                found[case] = ("junit", out.read_bytes(), result.returncode)
            except OSError as error:
                raise Unavailable(f"runner.py {case}: {error}") from None
    return found


def windows_native_tests():
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


def ci_run_passes():
    """Raise Unavailable unless the repository's CI reader passes the real pull request run of the checked commit."""
    env = {k: v for k, v in os.environ.items() if not k.startswith("HA_EVIDENCE")}
    result = run([sys.executable, "tools/verification-checks/cross_platform_ci.py", "AC-1"], ROOT, env, timeout=900)
    if result.returncode:
        raise Unavailable(f"the CI reader did not pass a real run of the checked commit (exit {result.returncode}): "
                          + result.stderr.decode(errors="replace").strip()[-500:])


class Platform(unittest.TestCase):
    """AC-12: classification depends only on output bytes, declaration and exit status."""

    def test_crlf_classifies_as_lf(self):
        cases = [c for pair in CRLF_PAIRS for c in pair]
        rows = [crlf_row(c) for c in cases]
        for row in rows:
            row.current = row.baseline = ("*", "*", [])
        with temporary("jaekit-adapter-crlf-") as directory:
            ha = build(Path(directory) / "ha")
            conditions = [Condition(f"AC-{i}", r.command, r.checks, r.declaration) for i, r in enumerate(rows, 1)]
            repo = Repo(directory, ha, conditions, files={"tests/runner.py": runner_file()}, runs=200)
            code, out = repo.start()
            require(self, code == 0, "classification-platform-dependent", f"ha start refused: {out[-1500:]}")
            ids = {r.name: c.id for r, c in zip(rows, conditions)}
            problems = []
            for baseline in (True, False):
                repo.check(*ids.values(), baseline=baseline)
                kind = "baseline" if baseline else "check"
                for lf, crlf in CRLF_PAIRS:
                    a, b = summary(repo.last(ids[lf], kind)), summary(repo.last(ids[crlf], kind))
                    if a is None or b is None or a != b:
                        problems.append(f"{kind}: {lf} gives {a}, its CRLF form gives {b}")
        require(self, not problems, "classification-platform-dependent", "; ".join(problems))

    def test_linux_build_classifies_the_same_bytes(self):
        if shutil.which("docker") is None:
            raise Unavailable("docker is required for the Linux observation")
        arch = {"arm64": "arm64", "aarch64": "arm64", "x86_64": "amd64", "amd64": "amd64"}.get(os.uname().machine)
        if arch is None:
            raise Unavailable(f"no Linux build for {os.uname().machine}")
        cases = ["go-typed-fail", "go-crlf-unmarked", "go-typed-pass", "go-untyped-fail", "go-garbage",
                 "J2", "J3-crlf", "J6", "J7", "J9", "J11-dtd"]
        with temporary("jaekit-adapter-linux-") as directory:
            outputs = fixture_outputs(directory, cases)
            files = {"tests/emit.sh": EMIT}
            conditions = []
            for i, case in enumerate(cases, 1):
                adapter, data, code = outputs[case]
                path = f"fixtures/{case}.{'jsonl' if adapter == 'go' else 'xml'}"
                files[path] = data
                if adapter == "go":
                    conditions.append(Condition(f"AC-{i}", ["sh", "tests/emit.sh", "go", path, str(code)], ["tests/emit.sh", path],
                                                go_declaration([("t", MARK_ID, PKG, "TestMarked")])))
                else:
                    classname, test = JUNIT_TESTS.get(case.replace("-crlf", ""), ("tests.test_greet", "test_greets"))
                    conditions.append(Condition(f"AC-{i}", ["sh", "tests/emit.sh", "junit", path, str(code), PLACEHOLDER],
                                                ["tests/emit.sh", path], junit_declaration([("t", MARK_ID, classname, test)])))
            host_ha = build(Path(directory) / "host" / "ha")
            linux_ha = build(Path(directory) / "linux" / "ha", goos="linux", goarch=arch)
            host = Repo(Path(directory) / "host", host_ha, conditions, files=files)
            linux = Repo(Path(directory) / "linux", linux_ha, conditions, files=files)
            code, out = host.start()
            require(self, code == 0, "classification-platform-dependent", f"ha start refused: {out[-1500:]}")
            ids = [c.id for c in conditions]
            host.check(*ids, baseline=True)
            host.check(*ids)
            steps = Path(directory) / "steps.sh"
            quoted = " ".join(ids)
            steps.write_text("set -e\ncd /work\n"
                             f"/out/ha start {GOAL} --request x --skill-name seal --skill-version 0.1.0 --rules run-rules/4\n"
                             f"/out/ha check {GOAL} --baseline {quoted} || true\n/out/ha check {GOAL} {quoted} || true\n",
                             encoding="utf-8")
            result = run(["docker", "run", "--rm", "--pull", "never", "--network", "none", "--platform", "linux/" + arch,
                          "-e", "GIT_CONFIG_COUNT=1", "-e", "GIT_CONFIG_KEY_0=safe.directory", "-e", "GIT_CONFIG_VALUE_0=*",
                          "-e", "GIT_AUTHOR_NAME=o", "-e", "GIT_AUTHOR_EMAIL=o@example.invalid",
                          "-v", f"{linux_ha.parent}:/out:ro", "-v", f"{steps}:/steps.sh:ro", "-v", f"{linux.root}:/work",
                          "--entrypoint", "sh", "alpine/git:latest", "/steps.sh"], directory, timeout=900)
            if result.returncode:
                raise Unavailable("linux container: " + (result.stdout + result.stderr).decode(errors="replace")[-2000:])
            problems = []
            for cid, case in zip(ids, cases):
                for kind in ("baseline", "check"):
                    a, b = host.last(cid, kind), linux.last(cid, kind)
                    pick = lambda r: None if r is None else (summary(r), (r.get("evidence") or {}).get("report_digest"))
                    if pick(a) != pick(b) or a is None:
                        problems.append(f"{case} {kind}: macOS {pick(a)}, Linux {pick(b)}")
        require(self, not problems, "classification-platform-dependent", "; ".join(problems))

    def test_windows_ci_classifies_the_same_bytes(self):
        required = windows_native_tests()
        missing = [f"{p} {t}" for p, t in WINDOWS_NATIVE.items() if t not in required.get(p, [])]
        require(self, not missing, "classification-platform-dependent",
                f"Windows native CI does not require {', '.join(missing)}")
        problems = []
        name = PLATFORM_FIXTURE.rsplit("/", 1)[1]
        sources = [path for path in (ROOT / "internal/evidence").glob("*_test.go")
                   if "func TestNativeAdapterClassification(" in path.read_text(encoding="utf-8", errors="replace")]
        if not sources or not any(name in path.read_text(encoding="utf-8", errors="replace") for path in sources):
            problems.append(f"TestNativeAdapterClassification does not read {name}")
        try:
            fixture = json.loads((ROOT / PLATFORM_FIXTURE).read_text(encoding="utf-8"))
            cases = {item["case"]: item for item in fixture["cases"]}
        except (OSError, ValueError, KeyError, TypeError) as error:
            raise Unavailable(f"{PLATFORM_FIXTURE}: {error!r}") from None
        with temporary("jaekit-adapter-windows-") as directory:
            outputs = fixture_outputs(directory, list(PLATFORM_EXPECTED))
        for case, (result, reason, targets) in PLATFORM_EXPECTED.items():
            item = cases.get(case)
            if not isinstance(item, dict):
                problems.append(f"{PLATFORM_FIXTURE} has no case {case}")
                continue
            adapter, data, code = outputs[case]
            classname, test = JUNIT_TESTS.get(case, ("tests.test_greet", "test_greets"))
            want = {"adapter": "go-test-json" if adapter == "go" else "junit-xml", "exit": code,
                    "output": data.decode("utf-8"), "violation": MARK_ID,
                    "test": {"package": PKG, "name": "TestMarked"} if adapter == "go" else {"classname": classname, "name": test},
                    "result": result, "reason": reason, "targets": targets}
            wrong = sorted(key for key, value in want.items() if item.get(key) != value)
            if wrong:
                problems.append(f"{PLATFORM_FIXTURE} case {case} differs in {wrong}")
        require(self, not problems, "classification-platform-dependent", "; ".join(problems))
        go_typed()
        env = clean_env()
        env["GOFLAGS"] = "-mod=readonly"
        prefix = f"github.com/jgoneit/jaekit/internal/evidence {PLATFORM_TEST}/"
        subtests = {f"{prefix}{case}/{form}" for case in PLATFORM_EXPECTED for form in ("lf", "crlf")}
        selection = ["-count=1", "-run", f"^{PLATFORM_TEST}$"]
        failed, passed = go_test_results(["./internal/evidence"], selection, env, timeout=900)
        if failed or not subtests <= set(passed):
            problems.append(f"{PLATFORM_TEST} with the fixture: failed {failed[:4]}, missing {sorted(subtests - set(passed))[:4]}")
        with temporary("jaekit-adapter-mutated-") as directory:
            mutated = Path(directory) / "adapter-platform.json"
            changed = [dict(item, result="unknown", reason="output_missing", targets=[]) for item in fixture["cases"]]
            mutated.write_text(json.dumps({"cases": changed}), encoding="utf-8")
            env[PLATFORM_VARIABLE] = str(mutated)
            failed, _ = go_test_results(["./internal/evidence"], selection, env, timeout=900)
        if not subtests <= set(failed):
            problems.append(f"{PLATFORM_TEST} does not compare every case and line ending with the fixture: "
                            f"{sorted(subtests - set(failed))[:4]} passed with changed expectations")
        require(self, not problems, "classification-platform-dependent", "; ".join(problems))
        ci_run_passes()


if __name__ == "__main__":
    unittest.main()
