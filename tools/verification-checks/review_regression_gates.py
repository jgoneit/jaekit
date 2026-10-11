#!/usr/bin/env python3
"""Goal observer for regression registry reachability, discovery, sources, scope and environment.

Each criterion runs selected common regressions from tools/regression-checks in a
child process. A run is the criterion's violation only when every failing case
directly calls self.fail with that criterion's [violation:...] prefix. Untagged
failures, errors, skips, expected failures, unexpected successes and an empty
selection remain errors.
"""
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
SELECTIONS = {
    "AC-1": ("unreachable-selection-counted", ["test_map.ReachableSelection"]),
    "AC-2": ("selector-not-discoverable", ["test_map.DiscoverableSelectors"]),
    "AC-3": ("private-source-accepted", ["test_map.PublicSources"]),
    "AC-4": ("invariant-scope-hidden", ["test_map.InvariantScope"]),
    "AC-5": ("discovery-environment-differs", ["test_checker.LocalToolchain",
                                               "test_checker.Runner.test_selected_file_loads_real_sibling_dependency"]),
    "AC-6": ("archive-guidance-misleading", ["test_map.PackagedGuidance"]),
}
RUNNER = r'''
import json, re, sys, unittest
TAG = re.compile(r"\[violation:([A-Za-z0-9][A-Za-z0-9_.:/-]{0,127})\]")
def failure_identity(err):
    error, traceback = err[1], err[2]
    frames = []
    while traceback is not None:
        frames.append(traceback.tb_frame)
        traceback = traceback.tb_next
    # Capture the actual failure origin before unittest formats and trims it.
    # Standard assertion helpers call fail internally with generated messages.
    explicit_fail = (len(frames) >= 2
                     and frames[-1].f_code is unittest.TestCase.fail.__code__
                     and frames[-2].f_globals.get("__name__") != "unittest.case")
    if explicit_fail and len(error.args) == 1 and type(error.args[0]) is str:
        message = error.args[0]
        tag = TAG.match(message)
        if tag and len(TAG.findall(message)) == 1:
            return tag.group(1)
    return None
class Result(unittest.TestResult):
    def __init__(self):
        super().__init__()
        self.cases = []
    def addSubTest(self, test, subtest, outcome):
        identity = failure_identity(outcome) if outcome is not None else None
        super().addSubTest(test, subtest, outcome)
        if outcome is not None:
            kind = "failure" if issubclass(outcome[0], test.failureException) else "error"
            self.cases.append((kind, subtest.id(), self._exc_info_to_string(outcome, test), identity))
    def addFailure(self, test, err):
        identity = failure_identity(err)
        super().addFailure(test, err)
        self.cases.append(("failure", test.id(), self._exc_info_to_string(err, test), identity))
    def addError(self, test, err):
        super().addError(test, err)
        self.cases.append(("error", test.id(), self._exc_info_to_string(err, test), None))
    def addSkip(self, test, reason):
        super().addSkip(test, reason)
        self.cases.append(("skip", test.id(), reason, None))
    def addExpectedFailure(self, test, err):
        super().addExpectedFailure(test, err)
        self.cases.append(("expected_failure", test.id(), self._exc_info_to_string(err, test), None))
    def addUnexpectedSuccess(self, test):
        super().addUnexpectedSuccess(test)
        self.cases.append(("unexpected_success", test.id(), "expectedFailure test succeeded", None))
result = Result()
unittest.defaultTestLoader.loadTestsFromNames(sys.argv[1:]).run(result)
print("JAEKIT_OBSERVED " + json.dumps({"run": result.testsRun, "cases": result.cases}))
'''


def observe(criterion):
    identity, names = SELECTIONS[criterion]
    env = {key: value for key, value in os.environ.items() if not key.startswith("HA_EVIDENCE_")}
    process = subprocess.run([sys.executable, "-B", "-c", RUNNER, *names], cwd=ROOT / "tools/regression-checks",
                             env=dict(env, PYTHONDONTWRITEBYTECODE="1"), capture_output=True, text=True, timeout=1800)
    lines = [line for line in process.stdout.splitlines() if line.startswith("JAEKIT_OBSERVED ")]
    if process.returncode or len(lines) != 1:
        print(process.stdout + process.stderr, file=sys.stderr)
        return {"status": "error", "reason": "execution_error"}
    summary = json.loads(lines[0][len("JAEKIT_OBSERVED "):])
    for kind, case, detail, _ in summary["cases"]:
        print(f"{kind.upper()}: {case}\n{detail}", file=sys.stderr)
    kinds = {kind for kind, _, _, _ in summary["cases"]}
    if not summary["run"] or kinds - {"failure"}:
        return {"status": "error", "reason": "execution_error"}
    if not kinds:
        return {"status": "pass"}
    if all(observed == identity for _, _, _, observed in summary["cases"]):
        return {"status": "violation", "violation": identity}
    return {"status": "error", "reason": "execution_error"}


def main():
    criterion = sys.argv[1]
    observation = {"target": criterion, "attempt": 1, **observe(criterion)}
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "jaekit-review-regression-observer", "producer_version": "1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "attempts_complete": True, "observations": [observation]}
        with Path(os.environ["HA_EVIDENCE_PATH"]).open("x") as output:
            json.dump(report, output)
    print(criterion, observation["status"])
    return 0 if observation["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
