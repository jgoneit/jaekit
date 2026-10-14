"""Synthetic report faults must never stand in for Windows history execution."""
import importlib.util
from pathlib import Path
import unittest

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


if __name__ == "__main__":
    unittest.main()
