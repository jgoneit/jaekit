"""Report-gate regressions; these are not Windows runtime observations."""
import importlib.util
from contextlib import redirect_stdout
import io
import json
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("windows_native_checks", Path(__file__).with_name("native_checks.py"))
native = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(native)


HISTORY_CASES = {
    "internal/history": ("TestNativeHistoryStorageLifecycle", "TestNativeHistoryStorageLocks",
                         "TestNativeHistoryStorageIntegrity", "TestNativeHistoryStoragePathSafety",
                         "TestNativeHistoryStorageLocks/first_registration_retry",
                         "TestNativeHistoryStorageLocks/process_exit_releases_ownership",
                         "TestNativeHistoryStorageIntegrity/last_replacement_failure_is_uncertain",
                         "TestNativeHistoryStorageIntegrity/snapshot_survives_marker_replacement",
                         "TestNativeHistoryStorageIntegrity/snapshot_opens_before_rename_source_closes",
                         "TestNativeHistoryStoragePathSafety/junction_common"),
    "cmd/ha": ("TestNativeHistoryGitLifecycle", "TestNativeHistoryFileEvidence",
               "TestNativeHistoryWindowsFileBoundaries", "TestNativeHistoryGitLifecycle/linked=false",
               "TestNativeHistoryGitLifecycle/linked=true", "TestNativeHistoryFileEvidence/replace-file",
               "TestNativeHistoryFileEvidence/growth_during_read",
               "TestNativeHistoryWindowsFileBoundaries/special-references",
               "TestNativeHistoryWindowsFileBoundaries/sharing-denial"),
}


def native_events():
    events = []
    for package, tests in native.NATIVE.items():
        package = native.MODULE + package
        events.append({"Action": "start", "Package": package})
        for test in tests:
            events.extend({"Action": action, "Package": package, "Test": test} for action in ("run", "pass"))
        events.append({"Action": "pass", "Package": package})
    return events


def parse_native_events(events):
    return native.native_results("\n".join(json.dumps(event) for event in events),
                                 set(native.REQUIRED), {native.MODULE + p for p in native.NATIVE})


class WindowsHistorySelectionTests(unittest.TestCase):
    def test_history_and_existing_core_require_actual_completed_observations(self):
        report = parse_native_events(native_events())
        for package, names in HISTORY_CASES.items():
            for name in names:
                key = native.MODULE + package + "/" + name
                with self.subTest(case=key):
                    self.assertIn(key, report["required"])
                    self.assertIn(key, report["passed"])
        self.assertIn(native.MODULE + "cmd/ha/TestNativeCoreGitLifecycle/linked=true", report["required"])
        self.assertIn(native.MODULE + "internal/record/TestNativeLockIsReleasedWhenOwnerExits", report["required"])

    def test_each_history_boundary_cannot_be_missing_skipped_failed_or_interrupted(self):
        for package, names in native.NATIVE.items():
            package = native.MODULE + package
            for name in (name for name in names if name.startswith("TestNativeHistory")):
                for kind in ("missing", "skip", "fail", "no-run", "interrupted"):
                    events = native_events()
                    if kind == "missing":
                        events = [event for event in events if not
                                  (event["Package"] == package and event.get("Test") == name)]
                    elif kind in ("no-run", "interrupted"):
                        action = "run" if kind == "no-run" else "pass"
                        events = [event for event in events if not
                                  (event["Package"] == package and event.get("Test") == name
                                   and event["Action"] == action)]
                    else:
                        for event in events:
                            if event["Package"] == package and event.get("Test") == name and event["Action"] == "pass":
                                event["Action"] = kind
                    with self.subTest(case=name, outcome=kind), self.assertRaises(ValueError):
                        parse_native_events(events)

    def test_package_success_without_history_tests_cannot_pass(self):
        events = [event for event in native_events() if not event.get("Test", "").startswith("TestNativeHistory")]
        with self.assertRaises(ValueError):
            parse_native_events(events)

    def test_parent_success_does_not_replace_history_subtests(self):
        events = [event for event in native_events() if not
                  (event.get("Test", "").startswith("TestNativeHistory") and "/" in event["Test"])]
        with self.assertRaises(ValueError):
            parse_native_events(events)

    def test_old_core_case_still_required_when_history_passes(self):
        events = [event for event in native_events() if event.get("Test") != "TestNativeCoreGitLifecycle/linked=true"]
        with self.assertRaises(ValueError):
            parse_native_events(events)


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
