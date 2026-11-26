"""Real unittest producer results for review gaps.

Only temporary directories and test files are changed. No private observations,
recorder commands, host sessions or network access are required.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import result_observations as result_facts


ROOT = Path(__file__).resolve().parents[2]
ENV = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GIT_AUTHOR_NAME="Fixture",
           GIT_AUTHOR_EMAIL="fixture@example.invalid", GIT_COMMITTER_NAME="Fixture",
           GIT_COMMITTER_EMAIL="fixture@example.invalid", GIT_CONFIG_NOSYSTEM="1",
           GIT_CONFIG_GLOBAL=os.devnull, GIT_ALLOW_PROTOCOL="file")
for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "HA_EVIDENCE_PATH",
            "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
    ENV.pop(key, None)


TEST_BODIES = {
    "expected_assertion": "@unittest.expectedFailure\ndef {name}(self): self.fail('synthetic expected assertion')",
    "expected_exception": "@unittest.expectedFailure\ndef {name}(self): raise RuntimeError('synthetic expected exception')",
    "unexpected_success": "@unittest.expectedFailure\ndef {name}(self): self.assertTrue(True)",
    "pass": "def {name}(self): self.assertTrue(True)",
    "failure": "def {name}(self): self.fail('synthetic assertion')",
    "skip": "def {name}(self): self.skipTest('synthetic skip')",
    "error": "def {name}(self): raise RuntimeError('synthetic execution error')",
}


class AC6(unittest.TestCase):
    def producer_result(self, producer, cases, unit="class"):
        with tempfile.TemporaryDirectory(prefix="jaekit-unittest-results-") as name:
            root = Path(name)
            body = "import unittest\nclass Case(unittest.TestCase):\n"
            for number, kind in enumerate(cases):
                method = TEST_BODIES[kind].format(name=f"test_{number:02}")
                body += "\n".join("    " + line for line in method.splitlines()) + "\n"
            (root / "fixture.py").write_text(body)
            report = root / "result.json"
            env = dict(ENV, HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="a" * 64,
                       HA_EVIDENCE_DECLARATION_DIGEST="b" * 64)
            if producer == "goal":
                declared = {"schema": "check-declaration/v1", "producer": "jaekit-goal-tests",
                            "producer_version": "2", "targets": result_facts.declaration_targets(
                                "synthetic", "not-met", result_facts.target_names("synthetic"))}
                check = {"runner": "unittest", "target": "synthetic", "file": "fixture.py",
                         "result_targets": result_facts.target_names("synthetic")}
                if unit != "module":
                    check["test"] = "Case.test_00" if unit == "method" else "Case"
                (root / "declaration.json").write_text(json.dumps(declared))
                (root / "targets.json").write_text(json.dumps({"schema": "jaekit-goal-targets/v2", "checks": [check]}))
                argv = [sys.executable, str(ROOT / "tools/goal-checks/produce.py"), "declaration.json", "targets.json"]
            else:
                directory = root / "tools/review-checks"
                directory.mkdir(parents=True)
                shutil.copyfile(ROOT / "tools/review-checks/produce.py", directory / "produce.py")
                shutil.copyfile(ROOT / "tools/result_observations.py", root / "tools/result_observations.py")
                (directory / "suites.json").write_text(json.dumps({"fixture": {"file": "fixture.py", "class": "Case",
                    "result_targets": result_facts.target_names("fixture")}}))
                argv = [sys.executable, str(directory / "produce.py"), "fixture"]
            result = subprocess.run(argv, cwd=root, env=env, text=True, capture_output=True, timeout=30)
            self.assertTrue(report.is_file(), "producer must run and write its report: " + result.stderr)
            observations = json.loads(report.read_text())["observations"]
            self.assertEqual(len(observations), 11, "one run has independent assertion and completeness targets")
            self.assertTrue(all(o['attempt'] == 1 for o in observations), "case outcomes are not retries")
            # Retain the old decorator/error expectations while the new
            # producer checks separately assert every preserved observation.
            execution = next(o for o in observations if o['target'].endswith('.execution'))
            selected = next(o for o in observations if o['target'].endswith('.selection'))
            aggregate = execution if execution['status'] == 'error' else observations[0]
            if aggregate['status'] not in ('violation', 'error') and selected['status'] == 'skip':
                aggregate = selected
            return aggregate, result.returncode, result.stderr

    def expect(self, producer, cases, status, reason=None, unit="class"):
        observation, code, stderr = self.producer_result(producer, cases, unit)
        self.assertEqual(observation["status"], status,
                         "[requirement] unittest outcome must not be promoted: " + str(observation) + stderr)
        self.assertEqual(observation.get("reason"), reason,
                         "[requirement] preserved result reason differs: " + str(observation))
        self.assertEqual(code, 0 if status == "pass" else 1,
                         "[requirement] report and producer exit must agree")
        if status != "violation":
            self.assertNotIn("violation", observation,
                             "[requirement] a decorator result supplies no declared assertion violation")

    def test_decorator_outcomes_are_errors_in_existing_selection_units(self):
        for producer, units in (("goal", ("method", "class", "module")), ("review", ("class",))):
            for kind in ("expected_assertion", "expected_exception", "unexpected_success"):
                for unit in units:
                    with self.subTest(producer=producer, outcome=kind, unit=unit):
                        self.expect(producer, [kind], "error", "setup_error", unit)

    def test_mixed_results_cannot_hide_decorator_outcomes_or_execution_errors(self):
        for producer, units in (("goal", ("class", "module")), ("review", ("class",))):
            for kind in ("expected_assertion", "expected_exception", "unexpected_success"):
                for other in ("pass", "failure", "skip", "error"):
                    for unit in units:
                        with self.subTest(producer=producer, outcome=kind, other=other, unit=unit):
                            self.expect(producer, [kind, other], "error",
                                        "execution_error" if other == "error" else "setup_error", unit)

    def test_undecorated_results_keep_their_meaning(self):
        outcomes = (("pass", "pass", None), ("failure", "violation", None),
                    ("skip", "skip", "skipped"), ("error", "error", "execution_error"))
        for producer in ("goal", "review"):
            for kind, status, reason in outcomes:
                with self.subTest(producer=producer, outcome=kind):
                    self.expect(producer, [kind], status, reason)


if __name__ == "__main__":
    unittest.main()
