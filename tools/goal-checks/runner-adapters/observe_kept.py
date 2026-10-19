"""Behavior the adapters leave as it is (AC-14).

The existing Go suites of the evidence, status, bundle, run and CLI packages
(including the protected result regressions, the producer semantics and every
behavioral scenario) and the producer semantic regressions run as they are.
The scenario `adapter-legacy-rules` must run and pass: a run-rules/2 goal
whose result contract names an adapter declaration keeps its exit-code
results. A synthetic run-rules/4 goal built with the checked `ha` shows that
maintain, EX-n and task commands keep their exit-code judgment next to an
adapter condition, and that `ha log` keeps both output streams of an adapter
condition. Missing Go or Python is an execution error.
"""
import json
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from adapters_common import (GOAL, ROOT, Condition, Repo, Unavailable, build, clean_env, go_declaration, go_test_results,
                             require, run, runner_command, runner_file, summary, temporary, unittest_failures)

PACKAGES = ["./internal/evidence", "./internal/status", "./internal/bundle", "./internal/run", "./internal/record", "./cmd/ha"]
SCENARIO = "adapter-legacy-rules"


def go_env():
    env = clean_env()
    env["GOFLAGS"] = "-mod=readonly"
    env.pop("GOPROXY", None)
    return env


class Kept(unittest.TestCase):

    def test_existing_suites_pass(self):
        failed, _ = go_test_results(PACKAGES, ["-count=1"], go_env(), timeout=3000)
        require(self, not failed, "existing-behavior-changed", f"existing Go tests fail: {failed[:10]}")
        failures = unittest_failures("tools/producer-checks", "test_semantics.py", go_env())
        require(self, failures == 0, "existing-behavior-changed", f"{failures} producer semantic regressions fail")

    def test_legacy_rules_keep_exit_codes(self):
        name = f"github.com/jgoneit/jaekit/cmd/ha TestScenarios/{SCENARIO}"
        try:
            failed, passed = go_test_results(["./cmd/ha"], ["-count=1", "-run", f"^TestScenarios$/^{SCENARIO}$"], go_env(),
                                             timeout=1800)
        except Unavailable as error:
            if "ran no test" not in str(error):
                raise
            failed, passed = [], []
        require(self, not failed and name in passed, "existing-behavior-changed",
                f"scenario {SCENARIO} did not run and pass: failed {failed}, passed {passed[:3]}")

    def test_ordinary_checks_and_log(self):
        problems = []
        with temporary("jaekit-adapter-kept-") as directory:
            ha = build(Path(directory) / "ha")
            conditions = [
                Condition("AC-1", runner_command("go-pass-noisy-stderr"), ["tests/runner.py"],
                          go_declaration([("t", "greeting-missing", "example.com/demo/forms", "TestMarked")])),
                Condition("AC-2", ["sh", "tests/exit.sh", "0"], kind="maintain"),
                Condition("AC-3", ["sh", "tests/exit.sh", "3"], kind="maintain"),
                Condition("EX-1", ["sh", "tests/exit.sh", "4"], kind="maintain"),
            ]
            files = {"tests/runner.py": runner_file(), "tests/exit.sh": 'echo "exit $1"\nexit "$1"\n'}
            repo = Repo(directory, ha, conditions, files=files)
            plan = (repo.root / GOAL / "PLAN.md").read_text(encoding="utf-8")
            repo.write(f"{GOAL}/PLAN.md", plan.replace("| 예 | — |", "| 예 | tasks/T001.md |"))
            repo.write(f"{GOAL}/tasks/T001.md", "# T001\n\n## 목표\nsynthetic\n\n## 완료 조건\n- `sh tests/exit.sh 5`\n")
            repo.commit("task document")
            code, out = repo.start()
            require(self, code == 0, "existing-behavior-changed", f"ha start refused: {out[-1500:]}")
            repo.check("AC-1", "AC-2", "AC-3", "EX-1")
            repo.check("T001")
            want = {"AC-2": ("pass", 0), "AC-3": ("fail", 3), "EX-1": ("fail", 4)}
            for cid, (result, exit_code) in want.items():
                record = repo.last(cid, "check")
                got = None if record is None else (record.get("result"), record.get("exit"), record.get("evidence"))
                if got != (result, exit_code, None):
                    problems.append(f"{cid}: got {got}, want {(result, exit_code, None)}")
            task = [r for r in repo.records() if r.get("kind") == "check" and (r.get("target") or {}).get("task") == "T001"]
            if not task or (task[-1].get("result"), task[-1].get("exit"), task[-1].get("evidence")) != ("fail", 5, None):
                problems.append(f"task command: {[(r.get('result'), r.get('exit')) for r in task]}")
            adapter = repo.last("AC-1", "check")
            if summary(adapter) is None or summary(adapter)[:2] != ("pass", "targets_passed"):
                problems.append(f"the adapter condition beside them: {summary(adapter)}")
            elif adapter.get("seq") is not None:
                code, out, err = repo.ha("log", GOAL, str(adapter["seq"]))
                if code != 0 or '"Action":"pass"' not in out or "go: downloading example.com/x" not in out:
                    problems.append(f"ha log does not show both output streams of the adapter condition: exit {code}")
        require(self, not problems, "existing-behavior-changed", "; ".join(problems))


if __name__ == "__main__":
    unittest.main()
