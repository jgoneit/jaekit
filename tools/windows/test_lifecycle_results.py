"""Reject partial or unrelated native lifecycle evidence, even with status=passed."""
import copy
import unittest

import lifecycle_results as results


class LifecycleResults(unittest.TestCase):
    def acceptance(self):
        return {
            "schema": "jaekit-windows-acceptance-dispatch/v1", "status": "passed", "shell_version": "5.1.1",
            "required": sorted(results.ACCEPTANCE_CASES), "passed": sorted(results.ACCEPTANCE_CASES),
            "skipped": [], "failures": [], "source_sha256": {"source": "a" * 64},
            "environment_preserved": True, "environment_before_sha256": "b" * 64,
            "environment_after_sha256": "b" * 64,
            "source_function_sha256": {name: "c" * 64 for name in
                                       ("Get-JaekitAcceptanceSteps", "Invoke-JaekitAcceptanceStep")},
            "production_stages": list(results.STAGES),
            "stages": [{"stage": name, "candidate": candidate, "helper": candidate,
                        "installer_or_uninstaller": action, "child_shell": version + "1",
                        "candidate_path_length": 140, "candidate_path_has_space": True, "candidate_path_has_unicode": True,
                        "child_error_view": "NormalView" if version == "5.1" else "ConciseView"}
                       for name, (candidate, action, version) in results.STAGES.items()],
            "failure_observations": [{"case": name, "stage": results.FAILURE_STAGES[name], "rejected": True,
                                      "marker_observed": True} for name in sorted(results.FAILURE_CASES)],
        }

    def path(self):
        return {"schema": "jaekit-windows-path-order/v1", "status": "passed", "shell_version": "5.1.1",
                "required": sorted(results.PATH_CASES), "passed": sorted(results.PATH_CASES),
                "skipped": [], "failures": [], "source_sha256": {"source": "a" * 64},
                "environment_preserved": True, "before_sha256": "b" * 64, "after_sha256": "b" * 64,
                "unverified": ["real-registry-write", "platform-inventory-lock-fixture-seams", "windows11-install-lifecycle"]}

    def test_partial_or_other_source_observations_cannot_pass(self):
        for factory, validate in ((self.acceptance, results.acceptance_report), (self.path, results.path_report)):
            original = factory()
            validate(original, "5.1.1", {"source": "a" * 64})
            for mutate in (
                    lambda r: r.update(status="failed"), lambda r: r.update(shell_version="7.1"),
                    lambda r: r.update(source_sha256={"source": "f" * 64}),
                    lambda r: r["required"].pop(), lambda r: r["passed"].pop(),
                    lambda r: r["passed"].append(r["passed"][0]),
                    lambda r: r["skipped"].append("missing"), lambda r: r["failures"].append("unfinished"),
                    lambda r: r.update(environment_preserved=False), lambda r: r.update(passed=None)):
                changed = copy.deepcopy(original)
                mutate(changed)
                with self.subTest(schema=original["schema"], value=changed), self.assertRaises(ValueError):
                    validate(changed, "5.1.1", {"source": "a" * 64})

    def test_dispatch_needs_real_new_candidate_helper_and_expected_child_shell(self):
        for mutate in (
                lambda r: r["stages"][5].update(candidate="first"),
                lambda r: r["stages"][5].update(helper="first"),
                lambda r: r["stages"][5].update(child_shell="5.1.1"),
                lambda r: r["stages"][5].update(child_error_view="NormalView"),
                lambda r: r["stages"][5].update(candidate_path_length=20),
                lambda r: r["stages"][5].update(candidate_path_has_space=False),
                lambda r: r["stages"][5].update(candidate_path_has_unicode=False),
                lambda r: r["stages"].pop(), lambda r: r["production_stages"].reverse(),
                lambda r: r.update(environment_after_sha256="d" * 64),
                lambda r: r.update(source_function_sha256={}),
                lambda r: r["failure_observations"].pop(),
                lambda r: r["failure_observations"][0].update(rejected=False),
                lambda r: r["failure_observations"][0].update(stage="update-install"),
                lambda r: next(x for x in r["failure_observations"]
                               if x["case"] == "update-helper-failure").update(marker_observed=False)):
            report = self.acceptance()
            mutate(report)
            with self.subTest(report=report), self.assertRaises(ValueError):
                results.acceptance_report(report, "5.1.1", {"source": "a" * 64})


    def test_path_needs_callers_rollback_and_preservation_observations(self):
        for mutate in (
                lambda r: r["passed"].remove("process-remove-first-exact-order"),
                lambda r: r["passed"].remove("uninstall-concurrent-value"),
                lambda r: r.update(after_sha256="c" * 64),
                lambda r: r.update(unverified=[])):
            report = self.path()
            mutate(report)
            with self.subTest(report=report), self.assertRaises(ValueError):
                results.path_report(report, "5.1.1", {"source": "a" * 64})


if __name__ == "__main__":
    unittest.main()
