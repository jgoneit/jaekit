#!/usr/bin/env python3
"""Goal observer for regression registry reachability, discovery, sources, scope and environment.

Each criterion runs selected common regressions from tools/regression-checks in a
child process. A run is the criterion's violation only when every failing case
names that criterion's identity with an explicit [violation:...] tag. Untagged
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
import json, sys, unittest
class Result(unittest.TestResult):
    def __init__(self):
        super().__init__()
        self.cases = []
    def addSubTest(self, test, subtest, outcome):
        super().addSubTest(test, subtest, outcome)
        if outcome is not None:
            kind = "failure" if issubclass(outcome[0], test.failureException) else "error"
            self.cases.append((kind, subtest.id(), self._exc_info_to_string(outcome, test)))
    def addFailure(self, test, err):
        super().addFailure(test, err)
        self.cases.append(("failure", test.id(), self._exc_info_to_string(err, test)))
    def addError(self, test, err):
        super().addError(test, err)
        self.cases.append(("error", test.id(), self._exc_info_to_string(err, test)))
    def addSkip(self, test, reason):
        super().addSkip(test, reason)
        self.cases.append(("skip", test.id(), reason))
    def addExpectedFailure(self, test, err):
        super().addExpectedFailure(test, err)
        self.cases.append(("expected_failure", test.id(), self._exc_info_to_string(err, test)))
    def addUnexpectedSuccess(self, test):
        super().addUnexpectedSuccess(test)
        self.cases.append(("unexpected_success", test.id(), "expectedFailure test succeeded"))
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
    for kind, case, detail in summary["cases"]:
        print(f"{kind.upper()}: {case}\n{detail}", file=sys.stderr)
    kinds = {kind for kind, _, _ in summary["cases"]}
    if not summary["run"] or kinds - {"failure"}:
        return {"status": "error", "reason": "execution_error"}
    if not kinds:
        return {"status": "pass"}
    if all(f"[violation:{identity}]" in detail for _, _, detail in summary["cases"]):
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
