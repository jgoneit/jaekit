"""Behavior the adapters leave as it is (AC-14).

The existing Go suites of the evidence, status, bundle, run and CLI packages
(including the protected result regressions, the producer semantics and every
behavioral scenario) and the producer semantic regressions run as they are.
A run-rules/1 and a run-rules/2 goal whose result contract names
check-adapter/v1 declarations keep their exit-code results, and the
placeholder stays literal: each starts with the checked `ha` and gets its
rule by rewriting the start record and the record chain, as the behavioral
scenarios do (`rewrite-record`). A synthetic run-rules/4 goal built with the checked `ha` shows that
maintain, EX-n and task commands keep their exit-code judgment next to an
adapter condition, and that `ha log` keeps both output streams of an adapter
condition. Missing Go or Python is an execution error.
"""
import hashlib
import json
from pathlib import Path
import re
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from adapters_common import (GOAL, PLACEHOLDER, Condition, Repo, Unavailable, build, clean_env, go_declaration,
                             go_test_results, junit_declaration, require, runner_command, runner_file, summary, temporary,
                             unittest_failures)

PACKAGES = ["./internal/evidence", "./internal/status", "./internal/bundle", "./internal/run", "./internal/record", "./cmd/ha"]
LEGACY_RULES = ("run-rules/1", "run-rules/2")


def legacy(repo, rules):
    """Rewrite the start record's rule and re-chain later records (contracts/run-record.md, `prev`)."""
    path = repo.root / GOAL / "runs.jsonl"
    lines = path.read_bytes().split(b"\n")
    if lines[-1] != b"":
        raise Unavailable("runs.jsonl does not end with a newline")
    lines = lines[:-1]
    first = re.sub(rb'"rules":"[^"]*"', b'"rules":"' + rules.encode() + b'"', lines[0], count=1)
    if first == lines[0]:
        raise Unavailable("the start record has no rules field")
    lines[0] = first
    for i in range(1, len(lines)):
        previous = hashlib.sha256(lines[i - 1]).hexdigest().encode()
        lines[i] = re.sub(rb'"prev":"[0-9a-f]*"', b'"prev":"' + previous + b'"', lines[i], count=1)
    path.write_bytes(b"\n".join(lines) + b"\n")


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
        problems = []
        for rules in LEGACY_RULES:
            with temporary("jaekit-adapter-legacy-") as directory:
                log = Path(directory) / "observed.jsonl"
                conditions = [
                    Condition("AC-1", runner_command("go-typed-fail"), ["tests/runner.py"],
                              go_declaration([("t", "greeting-missing", "example.com/demo/forms", "TestMarked")])),
                    Condition("AC-2", runner_command("junit-probe", PLACEHOLDER), ["tests/runner.py"],
                              junit_declaration([("t", "greeting-missing", "tests.test_greet", "test_greets")])),
                ]
                repo = Repo(directory, build(Path(directory) / "ha"), conditions, files={"tests/runner.py": runner_file()},
                            rules="run-rules/3")
                repo.env["JAEKIT_OBSERVE_LOG"] = str(log)
                code, out = repo.start()
                require(self, code == 0, "existing-behavior-changed", f"ha start refused: {out[-1500:]}")
                legacy(repo, rules)
                _, status = repo.status()
                if status.get("rules") != rules:
                    raise Unavailable(f"the rewritten start record reads as {status.get('rules')}, not {rules}")
                repo.check("AC-1", "AC-2", baseline=True)
                repo.check("AC-1", "AC-2")
                want = {("AC-1", "baseline"): ("fail_as_expected", 1), ("AC-1", "check"): ("fail", 1),
                        ("AC-2", "baseline"): ("unexpected_pass", 0), ("AC-2", "check"): ("pass", 0)}
                for (cid, kind), (result, exit_code) in want.items():
                    record = repo.last(cid, kind)
                    got = None if record is None else (record.get("result"), record.get("exit"), record.get("evidence"))
                    if got != (result, exit_code, None):
                        problems.append(f"{rules} {cid} {kind}: got {got}, want {(result, exit_code, None)}")
                try:
                    seen = [json.loads(line) for line in log.read_text(encoding="utf-8").splitlines()]
                except (OSError, ValueError) as error:
                    raise Unavailable(f"probe log: {error}") from None
                if len(seen) != 2 or any(item["argv"][1:] != [PLACEHOLDER] or item["env"] is not None for item in seen):
                    problems.append(f"{rules} junit-xml runs got a location: {[(i['argv'], i['env']) for i in seen]}")
        require(self, not problems, "existing-behavior-changed", "; ".join(problems))

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
                if code != 0 or not re.search(r'"Action":\s*"pass"', out) or "go: downloading example.com/x" not in out:
                    problems.append(f"ha log does not show both output streams of the adapter condition: exit {code}")
        require(self, not problems, "existing-behavior-changed", "; ".join(problems))


if __name__ == "__main__":
    unittest.main()
