"""Fail closed when native guide execution reports are partial or ambiguous."""
import copy
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("native_preflight_report", Path(__file__).with_name("native_checks.py"))
native = importlib.util.module_from_spec(spec)
spec.loader.exec_module(native)


class PreflightReportTests(unittest.TestCase):
    def report(self):
        return {"schema": "jaekit-windows-preflight/v1", "status": "passed", "shell_version": "5.1.1",
                "failures": [], "guides": [
                    {"path": name, "required": sorted(native.PREFLIGHT_CASES),
                     "passed": sorted(native.PREFLIGHT_CASES), "skipped": [],
                     "definition_sha256": "a" * 64,
                     "action_sha256": {name: "b" * 64 for name in ("install", "remove", "acceptance")}}
                    for name in ("guides/WINDOWS.md", "guides/WINDOWS.en.md")]}

    def test_both_guides_and_all_cases_are_required_in_same_shell(self):
        native.validate_preflight_report(self.report(), "5.1.1")
        mutations = [lambda r: r.update(status="failed"), lambda r: r.update(shell_version="7.0.0"),
                     lambda r: r["guides"].pop(), lambda r: r["failures"].append("unfinished"),
                     lambda r: r["guides"][0]["passed"].pop(),
                     lambda r: r["guides"][0]["required"].pop(),
                     lambda r: r["guides"][0]["passed"].append(r["guides"][0]["passed"][0]),
                     lambda r: r["guides"][0]["skipped"].append("actual-candidate"),
                     lambda r: r["guides"][0].update(definition_sha256="missing"),
                     lambda r: r["guides"][0].update(action_sha256={}),
                     lambda r: r["guides"][0].update(path="guides/WINDOWS.en.md")]
        for mutate in mutations:
            report = copy.deepcopy(self.report())
            mutate(report)
            with self.subTest(report=report), self.assertRaises(RuntimeError):
                native.validate_preflight_report(report, "5.1.1")


if __name__ == "__main__":
    unittest.main()
