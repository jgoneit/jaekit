"""Report-gate regressions; these are not Windows runtime observations."""
import importlib.util
from contextlib import redirect_stdout
import io
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("windows_native_checks", Path(__file__).with_name("native_checks.py"))
native = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(native)


class WindowsNativeReportTests(unittest.TestCase):
    def valid(self):
        return {"schema": "jaekit-native-windows-package/v1", "status": "passed", "shell_version": "5.1.1",
                "package_checks": native.PACKAGE_CHECKS[:], "failures": [],
                "server_rejection": {action: {"rejected": True, "preserved": True, "exit_code": 1,
                                              "before_sha256": "a" * 64, "after_sha256": "a" * 64}
                                     for action in ("install", "uninstall")}}

    def test_complete_current_shell_and_server_observations(self):
        native.validate_package_report(self.valid(), "5.1.1", True)

    def test_missing_duplicate_checks_and_wrong_shell_cannot_pass(self):
        for mutation in (lambda r: r["package_checks"].pop(),
                         lambda r: r["package_checks"].append(r["package_checks"][0]),
                         lambda r: r.update(shell_version="7.5.0"),
                         lambda r: r.update(status="failed"),
                         lambda r: r.update(schema="other/v1"),
                         lambda r: r["failures"].append("unfinished")):
            report = self.valid()
            mutation(report)
            with self.subTest(report=report), self.assertRaises(RuntimeError):
                native.validate_package_report(report, "5.1.1", True)

    def test_server_refusal_requires_both_actions_and_preservation(self):
        for action in ("install", "uninstall"):
            for values in ({}, {"rejected": False}, {"preserved": False}, {"exit_code": 0},
                           {"before_sha256": "a" * 64, "after_sha256": "b" * 64}):
                report = self.valid()
                if not values:
                    del report["server_rejection"][action]
                else:
                    report["server_rejection"][action].update(values)
                with self.subTest(action=action, values=values), self.assertRaises(RuntimeError):
                    native.validate_package_report(report, "5.1.1", True)

    def test_client_developer_report_does_not_need_or_invent_server_success(self):
        report = self.valid()
        report["server_rejection"] = {"status": "not-applicable"}
        native.validate_package_report(report, "5.1.1", False)
        with self.assertRaises(RuntimeError):
            native.validate_package_report(report, "5.1.1", True)

    def test_developer_powershell_sources_are_ascii_for_ps51(self):
        for name in ("native_checks.ps1", "native_package_checks.ps1"):
            Path(__file__).with_name(name).read_bytes().decode("ascii")

    def test_failure_diagnostics_are_bounded_and_only_selected_public_logs(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            (directory / "go-test.jsonl").write_text("old-output\n" + "x" * 40000 + "actual-failure\n")
            (directory / "private-path-snapshot.json").write_text("must-not-print")
            stream = io.StringIO()
            with redirect_stdout(stream):
                native.failure_tails(directory)
            self.assertIn("actual-failure", stream.getvalue())
            self.assertNotIn("old-output", stream.getvalue())
            self.assertNotIn("must-not-print", stream.getvalue())
            self.assertLess(len(stream.getvalue()), 6500)


if __name__ == "__main__":
    unittest.main()
