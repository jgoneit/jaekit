#!/usr/bin/env python3
"""Observe producer behavior with real synthetic processes, independent of its expected IDs."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path.cwd()
ENV = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly",
           GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1", GIT_AUTHOR_NAME="Synthetic",
           GIT_AUTHOR_EMAIL="synthetic@example.invalid", GIT_COMMITTER_NAME="Synthetic",
           GIT_COMMITTER_EMAIL="synthetic@example.invalid")
for key in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST",
            "GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE"):
    ENV.pop(key, None)


class ObservedViolation(Exception):
    def __init__(self, code, detail):
        self.code = code
        super().__init__(detail)


def require(condition, code, detail):
    if not condition:
        raise ObservedViolation(code, detail)


def run(argv, cwd, *, env=None, success=True, timeout=900):
    result = subprocess.run(argv, cwd=cwd, env=env or ENV, capture_output=True, text=True, timeout=timeout)
    if success and result.returncode:
        raise RuntimeError("probe environment/command failure: " + repr(argv) + "\n" + result.stdout + result.stderr)
    return result


def prepare(directory, body, *, producer="goal", expected="wanted", command=None):
    target = "sample"
    current = ROOT / "tools" / (producer + "-checks") / "produce.py"
    destination = directory / "tools" / (producer + "-checks")
    destination.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(current, destination / "produce.py")
    helper = ROOT / "tools/result_observations.py"
    if helper.exists():
        shutil.copyfile(helper, directory / "tools/result_observations.py")
    (directory / "checks").mkdir(exist_ok=True)
    (directory / "checks/fixture.py").write_text(body if command else "import unittest\nclass Case(unittest.TestCase):\n" + body)
    facets = {"additional_violations": [target + ".assertion-2", target + ".assertion-3"],
              "execution": target + ".execution", "selection": target + ".selection", "attribution": target + ".attribution"}
    version = "2" if helper.exists() else "1"
    target_ids = [target]
    if version == "2":
        target_ids += facets["additional_violations"] + [facets["execution"], facets["selection"], facets["attribution"]]
    producer_name = "jaekit-goal-tests" if producer == "goal" else "jaekit-" + producer + "-regressions"
    paths = ["tools/" + producer + "-checks/produce.py", "checks/fixture.py"]
    if helper.exists():
        paths.append("tools/result_observations.py")
    check = dict(target=target, runner="unittest", file="checks/fixture.py", test="Case", result_targets=facets)
    if command is not None:
        check = dict(target=target, runner="command", argv=[sys.executable, "checks/fixture.py"], result_targets=facets,
                     exit_contract={"schema": "jaekit-command-exits/v1", "pass": [0],
                                    "violations": {"1": "actual-command-violation"}, "unverified": [2]})
        if command == "unsupported":
            check.pop("exit_contract")
    if producer == "goal":
        config_name = "checks/targets.json"
        (directory / config_name).write_text(json.dumps({"schema": "jaekit-goal-targets/v" + version, "checks": [check]}))
        argv = [sys.executable, "tools/goal-checks/produce.py", "checks/declaration.json", config_name]
    else:
        config_name = "tools/" + producer + "-checks/suites.json"
        (directory / config_name).write_text(json.dumps({target: {"file": "checks/fixture.py", "class": "Case", "result_targets": facets}}))
        argv = [sys.executable, "tools/" + producer + "-checks/produce.py", target]
    paths.append(config_name)
    declaration = {"schema": "check-declaration/v1", "producer": producer_name, "producer_version": version,
                   "producer_paths": paths, "targets": [{"id": item, "violation": expected if index == 0 else "no-additional-violation"}
                                                        for index, item in enumerate(target_ids)]}
    (directory / "checks/declaration.json").write_text(json.dumps(declaration))
    return argv, paths


def observe(body, **kwargs):
    with tempfile.TemporaryDirectory(prefix="jaekit-producer-observation-") as temporary:
        directory = Path(temporary)
        argv, _ = prepare(directory, body, **kwargs)
        report = directory / "report.json"
        env = dict(ENV, HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="a" * 64,
                   HA_EVIDENCE_DECLARATION_DIGEST="b" * 64)
        proc = run(argv, directory, env=env, success=False)
        if not report.exists():
            raise RuntimeError("producer did not provide observations: " + proc.stdout + proc.stderr)
        data = json.loads(report.read_text())
        print(json.dumps({"producer": kwargs.get("producer", "goal"), "exit": proc.returncode, "observations": data["observations"]}))
        return data["observations"], proc


def ac1():
    values, _ = observe("print('UNVERIFIED: unavailable synthetic input')\nraise SystemExit(2)\n", command=True)
    require(not any(v["status"] == "violation" for v in values) and any(v["status"] == "skip" for v in values),
            "unverified-promoted", "UNVERIFIED command was not retained as unobserved")
    values, _ = observe("raise SystemExit(1)\n", command=True)
    require(any(v.get("violation") == "actual-command-violation" for v in values), "unverified-promoted", "known command violation lost")


def ac2():
    for code in (2, 17):
        values, _ = observe(f"raise SystemExit({code})\n", command="unsupported")
        require(not any(v["status"] == "violation" for v in values) and any(v["status"] == "error" for v in values),
                "unsupported-exit-promoted", "undocumented nonzero became requirement violation")


def ac3():
    body = ("    def test_a(self): self.fail('[violation:first] actual failure')\n"
            "    def test_b(self): self.skipTest('unobserved')\n"
            "    def test_c(self): raise RuntimeError('environment failed')\n"
            "    def test_d(self): self.fail('[violation:second] different actual failure')\n")
    for producer in ("goal", "review", "gap"):
        values, _ = observe(body, producer=producer)
        codes = {v.get("violation") for v in values if v["status"] == "violation"}
        require(codes == {"first", "second"} and {"error", "skip"}.issubset({v["status"] for v in values}),
                "mixed-facts-lost", "mixed failure/skip/error or different assertion identities were lost")
        require(all(v["attempt"] == 1 for v in values) and len({v["target"] for v in values}) == len(values),
                "mixed-facts-lost", "separate facts were represented as retries")


def ac4():
    bodies = [("    def test_a(self): self.fail('[violation:actual] observed')\n    def test_b(self): self.skipTest('missing')\n", {"violation", "skip"}),
              ("    def test_a(self): pass\n    def test_b(self): self.skipTest('missing')\n", {"skip"}),
              ("    @unittest.expectedFailure\n    def test_a(self): self.fail('decorated')\n", {"error"}),
              ("    def test_a(self): pass\n", {"pass"})]
    for body, required in bodies:
        observations = []
        for producer in ("goal", "review", "gap"):
            values, _ = observe(body, producer=producer)
            states = {v["status"] for v in values}
            require(required.issubset(states), "producer-meanings-diverge", "a producer discarded the selected outcome")
            observations.append(states - {"pass"})
        require(observations[0] == observations[1] == observations[2], "producer-meanings-diverge", "producer result meanings differ")


def ac5():
    with tempfile.TemporaryDirectory(prefix="jaekit-core-producer-boundary-") as temporary:
        parent = Path(temporary)
        binary = parent / "ha"
        run(["go", "build", "-mod=readonly", "-o", str(binary), "./cmd/ha"], ROOT)
        for producer in ("goal", "review", "gap"):
            fixture = parent / producer
            shutil.copytree(ROOT / "testdata/scenarios/_base", fixture)
            argv, paths = prepare(fixture, "    def test_a(self): self.fail('[violation:actual] actual assertion')\n",
                                  producer=producer, expected="different-expected")
            goal = fixture / "docs/specs/greeting"
            if not goal.exists():
                goals = list((fixture / "docs/specs").glob("*/PLAN.md"))
                if len(goals) != 1:
                    raise RuntimeError("ambiguous controlled Core fixture")
                goal = goals[0].parent
            goal_name = str(goal.relative_to(fixture))
            plan = (goal / "PLAN.md").read_text()
            command = " ".join(argv)
            plan = plan.replace("`sh tests/greeting.sh` | tests/greeting.sh", "`" + command + "` | " + ", ".join(paths + ["checks/declaration.json"]))
            plan = plan.replace("`src/**`, `tests/**`", "`src/**`, `tests/**`, `tools/**`, `checks/**`")
            plan += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
            (goal / "PLAN.md").write_text(plan)
            run(["git", "init", "-q", "-b", "main"], fixture)
            run(["git", "add", "-A"], fixture)
            run(["git", "commit", "-qm", "synthetic producer/Core boundary"], fixture)
            run([str(binary), "start", goal_name, "--request", "synthetic result boundary"], fixture)
            for baseline in (True, False):
                args = [str(binary), "check", goal_name, "AC-1"] + (["--baseline"] if baseline else [])
                proc = run(args, fixture, success=False)
                entries = [json.loads(line) for line in (goal / "runs.jsonl").read_text().splitlines()]
                last = entries[-1]
                if last.get("kind") not in ("baseline", "check"):
                    raise RuntimeError("Core check never reached observation: " + proc.stdout + proc.stderr)
                print(json.dumps({"producer": producer, "baseline": baseline, "record": last.get("result"), "evidence": last.get("evidence")}))
                require(last.get("result") == "fail" and any(v.get("violation") == "actual" for v in last.get("evidence", {}).get("targets", [])),
                        "core-observed-meaning-lost", "actual producer assertion identity did not reach Core current/baseline")


def ac6():
    for argv in ([sys.executable, "-B", "-m", "unittest", "discover", "-s", "tools/review-checks", "-p", "test_producers.py", "-k", "AC11"],
                 [sys.executable, "-B", "-m", "unittest", "discover", "-s", "tools/gap-checks", "-p", "test_history_results.py", "-k", "AC6"],
                 ["go", "test", "./cmd/ha", "-run", "^TestProducerFacts_", "-count=1"]):
        proc = run(argv, ROOT, success=False)
        print(proc.stdout + proc.stderr)
        if proc.returncode:
            raise RuntimeError("preservation regression failed")


def ac7():
    for expected in ("wanted", "other-declaration"):
        values, _ = observe("    def test_a(self): self.fail('[violation:actual] failed')\n", expected=expected)
        require(any(v.get("violation") == "actual" for v in values), "expected-overwrites-observed", "declaration replaced actual failure identity")
    values, _ = observe("    def test_a(self):\n        print('[violation:wanted] just output')\n        self.fail('untagged precondition')\n", expected="unclassified-assertion")
    require(any(v.get("violation") == "unclassified-assertion" for v in values) and any(v["status"] == "error" for v in values),
            "expected-overwrites-observed", "unclassified assertion became expected violation")
    for body in ("    def test_a(self): self.assertEqual('[violation:wanted]', 'different')\n",
                 "    def test_a(self):\n        class Data:\n            def __repr__(self): return '[violation:wanted] data'\n        self.assertEqual(Data(), object())\n",
                 "    def test_a(self): self.assertEqual(1, 2, '[violation:wanted] suffix')\n"):
        values, _ = observe(body, expected="wanted")
        require(any(v.get("violation") == "unclassified-assertion" for v in values) and any(v["status"] == "error" for v in values),
                "expected-overwrites-observed", "compared data or generated assertion message supplied a requirement identity")


def ac8():
    spec = importlib.util.spec_from_file_location("shared_verification", ROOT / "tools/verify.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    selected = []
    module.run = lambda label, command, env, **kwargs: selected.append(command)
    module.verify_go(dict(ENV))
    require(any("tools/producer-checks" in command and "test_semantics.py" in command for command in selected),
            "producer-regression-not-selected", "common Go validation did not select producer regression")
    # Selection and actual execution are distinct. Exercise the selected suite
    # in this exact source tree; a full common validation is a separate gate.
    proc = run([sys.executable, "-B", "-m", "unittest", "discover", "-s", "tools/producer-checks", "-p", "test_semantics.py"], ROOT, success=False)
    print(proc.stdout + proc.stderr)
    if proc.returncode:
        raise RuntimeError("selected producer regression did not execute successfully")


EXPECTED = {"AC-1": "unverified-promoted", "AC-2": "unsupported-exit-promoted", "AC-3": "mixed-facts-lost",
            "AC-4": "producer-meanings-diverge", "AC-5": "core-observed-meaning-lost",
            "AC-7": "expected-overwrites-observed", "AC-8": "producer-regression-not-selected"}


def main():
    criterion = sys.argv[1]
    observation = {"target": criterion, "attempt": 1, "status": "pass"}
    try:
        globals()[criterion.lower().replace("-", "")]()
    except ObservedViolation as error:
        observation.update(status="violation", violation=error.code)
        print(str(error), file=sys.stderr)
    except Exception as error:
        observation.update(status="error", reason="execution_error")
        print(type(error).__name__ + ": " + str(error), file=sys.stderr)
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "jaekit-producer-observer", "producer_version": "1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"], "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "attempts_complete": True, "observations": [observation]}
        with open(os.environ["HA_EVIDENCE_PATH"], "x") as stream:
            json.dump(report, stream)
    return 0 if observation["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
