"""Synthetic report faults must never stand in for Windows history execution."""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("history_ci", ROOT / "tools/history-checks/ci_observer.py")
observer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(observer)


def report():
    return {"native_tests": {"required": sorted(observer.HISTORY), "passed": sorted(observer.HISTORY), "skipped": []},
            "candidate": {"capabilities": {"features": ["decision-history/v1"],
                                            "formats": {"decision_history": ["decision-history/v1"]}}}}


class HistoryAdmission(unittest.TestCase):
    def test_all_declared_history_cases_and_capability_are_required(self):
        observer.history_report(report(), "Windows", "source")
        for name in observer.HISTORY:
            for field in ("required", "passed"):
                row = report()
                row["native_tests"][field].remove(name)
                with self.subTest(name=name, field=field), self.assertRaises(observer.ci.Violation):
                    observer.history_report(row, "Windows", "source")

    def test_subtest_or_same_name_in_another_package_cannot_replace_a_case(self):
        for name in observer.HISTORY:
            for replacement in (name + "/child", "other/package/" + name.rsplit("/", 1)[-1]):
                row = report()
                row["native_tests"]["passed"].remove(name)
                row["native_tests"]["passed"].append(replacement)
                with self.subTest(name=name, replacement=replacement), self.assertRaises(observer.ci.Violation):
                    observer.history_report(row, "Windows", "source")

    def test_skip_or_missing_executed_feature_or_format_is_rejected(self):
        for mutate in (lambda r: r["native_tests"].update(skipped=["boundary"]),
                       lambda r: r["candidate"]["capabilities"].update(features=[]),
                       lambda r: r["candidate"]["capabilities"].update(formats={})):
            row = report()
            mutate(row)
            with self.assertRaises(observer.ci.Violation):
                observer.history_report(row, "Windows", "source")


class HistoryConsumerFlow(unittest.TestCase):
    def setUp(self):
        self.reports = [dict(report(), source_commit="source", synthetic_padding="x" * 100000)
                        for _ in range(2)]

    def collect(self, native_validator):
        native_validator({"synthetic": "unix"}, "Linux", "source")
        for row in self.reports:
            native_validator(row, "Windows", "source")
        return "jgoneit/jaekit", "head"

    def observe(self, consumer_error=None, collection_error=None):
        with patch.object(observer.ci, "configuration_present"), \
                patch.object(observer.ci, "live_run", side_effect=collection_error or self.collect) as collect, \
                patch.object(observer.validation, "validate_boundaries", side_effect=consumer_error) as consume:
            rows = observer.observe()
            collect.assert_called_once()
            consume.assert_called_once_with(self.reports)
            self.assertEqual(rows, self.reports)
            return rows

    def test_actual_consumer_receives_both_complete_admitted_reports(self):
        self.observe()

    def test_consumer_rejection_remains_a_violation(self):
        with self.assertRaisesRegex(observer.ci.Violation, "synthetic rejected boundary"):
            self.observe(consumer_error=observer.validation.Violation("synthetic rejected boundary"))

    def test_collection_failure_remains_an_observation_error(self):
        with self.assertRaisesRegex(observer.ci.ObservationError, "synthetic collection failure"):
            self.observe(collection_error=observer.ci.ObservationError("synthetic collection failure"))

    def test_consumed_report_still_requires_history_observations(self):
        self.reports[1]["native_tests"]["passed"].pop()
        with self.assertRaisesRegex(observer.ci.Violation, "Windows history execution missing"):
            self.observe()


if __name__ == "__main__":
    unittest.main()
