#!/usr/bin/env python3
"""Observe the producer-result and document-attribution review follow-ups.

AC-1 to AC-4 run the regressions that the common checks select. Only failures
those regressions tag with an explicit requirement identity become observed
violations; any other failure, error or skip is an environment error. AC-5
runs the real producers through a Core built from this tree.
Usage: review_producer_results.py AC-n
"""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import unittest

ROOT = Path.cwd()
sys.path.insert(0, str(ROOT / "tools"))
sys.path.insert(0, str(ROOT / "tools/verification-checks"))
import result_observations as results  # noqa: E402
import producer_results as base  # noqa: E402

CONTRACT_TESTS = (("tools/contract-checks/test_check.py", "ReportingAttribution"),
                  ("tools/contract-checks/test_design_check.py", "DesignAttribution"),
                  ("tools/contract-checks/test_finding_docs_check.py", "FindingAttribution"),
                  ("tools/contract-checks/test_spec_check.py", "SpecAttribution"))


class Observed(Exception):
    """Every distinct requirement identity a run observed."""
    def __init__(self, codes):
        self.codes = sorted(set(codes))
        super().__init__("observed: " + ", ".join(self.codes))


def regressions(*selected):
    """Run named regression classes; tagged requirement failures are observations."""
    violations = []
    for path, name in selected:
        file = ROOT / path
        sys.path.insert(0, str(file.parent))
        spec = importlib.util.spec_from_file_location(file.stem + "_observed", file)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        facts = results.run_suite(unittest.defaultTestLoader.loadTestsFromName(name, module), sys.stderr)
        if facts["errors"] or facts["skipped"] or facts["unclassified"] or not (facts["passed"] or facts["violations"]):
            raise RuntimeError(f"{path}::{name} did not run cleanly: {facts}")
        violations += facts["violations"]
    if violations:
        raise Observed(violations)


def ac1():
    regressions(("tools/producer-checks/test_semantics.py", "ProvidedCommandTargets"))


def ac2():
    regressions(("tools/producer-checks/test_semantics.py", "OverflowWithErrors"))


def ac3():
    regressions(("tools/producer-checks/test_semantics.py", "ExitContractKeys"))


def ac4():
    regressions(*CONTRACT_TESTS)


class Core:
    """One synthetic goal whose AC-1 runs a real producer under a built Core."""

    def __init__(self, parent, binary, name, files, command, inputs, targets):
        self.binary, self.root = binary, parent / name
        shutil.copytree(ROOT / "testdata/scenarios/_base", self.root)
        for path, data in files.items():
            destination = self.root / path
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_bytes(data if isinstance(data, bytes) else data.encode())
        declaration = {"schema": "check-declaration/v1", "producer": targets["producer"],
                       "producer_version": targets["version"], "producer_paths": inputs,
                       "targets": targets["targets"]}
        (self.root / "checks").mkdir(exist_ok=True)
        (self.root / "checks/declaration.json").write_text(json.dumps(declaration))
        goals = list((self.root / "docs/specs").glob("*/PLAN.md"))
        if len(goals) != 1:
            raise RuntimeError("ambiguous controlled Core fixture")
        self.goal = goals[0].parent
        plan = self.goal.joinpath("PLAN.md").read_text()
        plan = plan.replace("`sh tests/greeting.sh` | tests/greeting.sh",
                            "`" + " ".join(command) + "` | " + ", ".join(inputs + ["checks/declaration.json"]))
        plan = plan.replace("`src/**`, `tests/**`", "`src/**`, `tests/**`, `tools/**`, `checks/**`, `README.md`, `guides/**`")
        plan += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
        self.goal.joinpath("PLAN.md").write_text(plan)
        for argv in (["git", "init", "-q", "-b", "main"], ["git", "add", "-A"], ["git", "commit", "-qm", "synthetic"]):
            base.run(argv, self.root)
        self.name = str(self.goal.relative_to(self.root))
        base.run([str(binary), "start", self.name, "--request", "synthetic producer boundary"], self.root)

    def check(self, baseline):
        args = [str(self.binary), "check", self.name, "AC-1"] + (["--baseline"] if baseline else [])
        proc = base.run(args, self.root, success=False)
        last = json.loads((self.goal / "runs.jsonl").read_text().splitlines()[-1])
        if last.get("kind") not in ("baseline", "check"):
            raise RuntimeError("Core check never reached observation: " + proc.stdout + proc.stderr)
        print(json.dumps({"fixture": self.root.name, "baseline": baseline, "result": last.get("result"),
                          "evidence": last.get("evidence")}))
        return last


def goal_targets(expected, slots=3):
    facets = results.target_names("suite", slots)
    ids = results.target_ids("suite", facets)
    return facets, {"producer": "jaekit-goal-tests", "version": "2",
                    "targets": [{"id": item, "violation": expected if i == 0 else "no-additional-violation"}
                                for i, item in enumerate(ids)]}


def goal_files(check):
    facets, targets = goal_targets(check.pop("expected"))
    files = {"tools/goal-checks/produce.py": (ROOT / "tools/goal-checks/produce.py").read_bytes(),
             "tools/result_observations.py": (ROOT / "tools/result_observations.py").read_bytes(),
             "checks/targets.json": json.dumps({"schema": "jaekit-goal-targets/v2",
                                                "checks": [dict(check, target="suite", result_targets=facets)]})}
    inputs = ["tools/goal-checks/produce.py", "tools/result_observations.py", "checks/targets.json"]
    return files, inputs, targets


def ac5():
    with tempfile.TemporaryDirectory(prefix="jaekit-review-producer-core-") as temporary:
        parent = Path(temporary)
        binary = parent / "ha"
        base.run(["go", "build", "-mod=readonly", "-o", str(binary), "./cmd/ha"], ROOT)
        command = ["python3", "tools/goal-checks/produce.py", "checks/declaration.json", "checks/targets.json"]

        # Overflow and an execution error both reach the stored evidence; neither is success.
        body = "".join(f"    def test_{i}(self): self.fail('[violation:failure-{i}] observed')\n" for i in range(5))
        body += "    def test_z(self): raise RuntimeError('environment failed')\n"
        files, inputs, targets = goal_files({"runner": "unittest", "file": "checks/fixture.py", "test": "Case", "expected": "failure-0"})
        files["checks/fixture.py"] = "import unittest\nclass Case(unittest.TestCase):\n" + body
        fixture = Core(parent, binary, "overflow", files, command, inputs + ["checks/fixture.py"], targets)
        for baseline in (True, False):
            record = fixture.check(baseline)
            reasons = {item.get("reason") for item in record.get("evidence", {}).get("targets", [])}
            base.require(record.get("result") == "error" and {"execution_error", "collection_error"} <= reasons,
                         "core-meaning-lost", "overflow and execution error did not both reach Core")

        # A provided command target keeps pass and violation meanings through Core.
        tracked = json.loads((ROOT / "tools/goal-checks/release-0-1-3-followups/AC-6.targets.json").read_text())
        docs = next(c for c in tracked["checks"] if c["target"] == "guide-links")
        contract = docs.get("exit_contract")
        base.require(isinstance(contract, dict), "core-meaning-lost", "provided documentation target has no exit contract")
        check = {"runner": docs["runner"], "argv": docs["argv"], "exit_contract": contract,
                 "expected": contract["violations"].get("1", "absent")}
        for name, readme, baseline, wanted in (("docs-broken", "[guide](guides/absent.md)\n", True, "fail_as_expected"),
                                                ("docs-normal", "[guide](guides/guide.md)\n", False, "pass")):
            files, inputs, targets = goal_files(dict(check))
            files.update({"tools/check_public_docs.py": (ROOT / "tools/check_public_docs.py").read_bytes(),
                          "README.md": readme, "guides/guide.md": "# Guide\n"})
            fixture = Core(parent, binary, name, files, command, inputs + ["tools/check_public_docs.py"], targets)
            record = fixture.check(baseline)
            base.require(record.get("result") == wanted, "core-meaning-lost", f"{name}: {record.get('result')}")

        # A noncanonical exit key is refused before the command runs.
        files, inputs, targets = goal_files({"runner": "command", "argv": ["python3", "checks/fixture.py"], "expected": "actual",
                                             "exit_contract": {"schema": "jaekit-command-exits/v1", "pass": [0],
                                                               "violations": {"01": "actual"}, "unverified": [2]}})
        files["checks/fixture.py"] = "from pathlib import Path\nPath('ignored').mkdir(exist_ok=True)\nPath('ignored/ran').write_text('ran')\nraise SystemExit(1)\n"
        fixture = Core(parent, binary, "noncanonical-key", files, command, inputs + ["checks/fixture.py"], targets)
        record = fixture.check(False)
        base.require(record.get("result") == "error" and not (fixture.root / "ignored/ran").exists(),
                     "core-meaning-lost", "a noncanonical exit key ran or was classified")

        # A document relation owned by AC-3 is a Core violation for AC-3 only.
        observer = ["tools/contract-checks/check.py", "tools/contract-checks/design_check.py"]
        sys.path.insert(0, str(ROOT / "tools/contract-checks"))
        import design_check  # noqa: E402
        documents = {name for clauses in design_check.checker.REQUIREMENTS["design"].values() for name, _ in clauses}
        for criterion, wanted in (("AC-3", "fail_as_expected"), ("AC-1", "unexpected_pass")):
            files = {name: (ROOT / name).read_bytes() for name in observer + sorted(documents)}
            example = files["examples/verification-design.md"].decode()
            changed = example.replace("| 별도 반례 검토자 | CP1 수정 재확인", "| 구현자 | CP1 수정 재확인", 1)
            base.require(changed != example, "core-meaning-lost", "design example relation not found")
            files["examples/verification-design.md"] = changed
            targets = {"producer": "jaekit-document-contracts", "version": "1",
                       "targets": [{"id": "design." + criterion, "violation": f"design.{criterion}.missing-contract"}]}
            fixture = Core(parent, binary, "design-" + criterion, files,
                           ["python3", "tools/contract-checks/design_check.py", "design", criterion], observer, targets)
            record = fixture.check(True)
            base.require(record.get("result") == wanted, "core-meaning-lost",
                         f"design {criterion} baseline was {record.get('result')}, wanted {wanted}")


# The identities each condition's observation can report; one target per identity.
IDENTITIES = {"AC-1": ("provided-command-unusable", "unreadable-documents-as-violation"),
              "AC-2": ("overflow-or-error-lost",), "AC-3": ("exit-key-mismatch",),
              "AC-4": ("unrelated-relation-attributed", "owned-relation-missed"),
              "AC-5": ("core-meaning-lost",)}


def main():
    criterion = sys.argv[1] if len(sys.argv) == 2 else ""
    if criterion not in IDENTITIES:
        print(__doc__, file=sys.stderr)
        return 64
    codes, failure = set(), None
    try:
        globals()[criterion.lower().replace("-", "")]()
    except Observed as error:
        codes = set(error.codes)
        print(str(error), file=sys.stderr)
    except base.ObservedViolation as error:
        codes = {error.code}
        print(str(error), file=sys.stderr)
    except Exception as error:
        failure = "execution_error"
        print(type(error).__name__ + ": " + str(error), file=sys.stderr)
    unknown = codes - set(IDENTITIES[criterion])
    if unknown:
        print("unregistered requirement identity: " + ", ".join(sorted(unknown)), file=sys.stderr)
        failure = "execution_error"
    observations = []
    for code in IDENTITIES[criterion]:
        item = {"target": criterion + "." + code, "attempt": 1, "status": "pass"}
        if failure:
            item.update(status="error", reason=failure)
        elif code in codes:
            item.update(status="violation", violation=code)
        observations.append(item)
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "jaekit-review-producer-observer", "producer_version": "1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "attempts_complete": True, "observations": observations}
        with open(os.environ["HA_EVIDENCE_PATH"], "x", encoding="utf-8") as stream:
            json.dump(report, stream)
    return 0 if all(item["status"] == "pass" for item in observations) else 1


if __name__ == "__main__":
    raise SystemExit(main())
