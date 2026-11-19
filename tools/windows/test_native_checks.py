"""Report-gate regressions; these are not Windows runtime observations."""
import importlib.util
from contextlib import redirect_stdout
import io
import json
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("windows_native_checks", Path(__file__).with_name("native_checks.py"))
native = importlib.util.module_from_spec(spec)
assert spec.loader is not None
spec.loader.exec_module(native)


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


class WindowsNativeSelectionTests(unittest.TestCase):
    def test_existing_core_cases_are_required(self):
        report = parse_native_events(native_events())
        self.assertIn(native.MODULE + "cmd/ha/TestNativeCoreGitLifecycle/linked=true", report["required"])
        self.assertIn(native.MODULE + "internal/record/TestNativeLockIsReleasedWhenOwnerExits", report["required"])

    def test_required_case_cannot_be_missing_skipped_failed_or_interrupted(self):
        # A required subtest under a passing parent: missing, skipped, failed,
        # never run or never finished is not a pass.
        package, name = native.MODULE + "cmd/ha", "TestNativeCoreGitLifecycle/linked=true"
        for kind in ("missing", "skip", "fail", "no-run", "interrupted"):
            events = native_events()
            if kind == "missing":
                events = [e for e in events if not (e["Package"] == package and e.get("Test") == name)]
            elif kind in ("no-run", "interrupted"):
                action = "run" if kind == "no-run" else "pass"
                events = [e for e in events if not (e["Package"] == package and e.get("Test") == name and e["Action"] == action)]
            else:
                for e in events:
                    if e["Package"] == package and e.get("Test") == name and e["Action"] == "pass":
                        e["Action"] = kind
            with self.subTest(outcome=kind), self.assertRaises(ValueError):
                parse_native_events(events)


class WindowsNativeReportTests(unittest.TestCase):
    def valid(self):
        return {"schema": "jaekit-native-windows-package/v1", "status": "passed", "shell_version": "5.1.1",
                "package_checks": native.PACKAGE_CHECKS[:], "failures": [],
                "name_character_refusals": list(native.NAME_RULE_CHARACTERS),
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

    def test_early_failed_subtest_output_survives_later_successes(self):
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            events = [
                {"Action": "output", "Package": "fixture/one", "Test": "TestParent/child",
                 "Output": "junction rename: synthetic access denied\n"},
                {"Action": "fail", "Package": "fixture/one", "Test": "TestParent/child"},
                {"Action": "output", "Package": "fixture/two", "Test": "TestParent/child",
                 "Output": "unrelated successful output\n"},
                {"Action": "pass", "Package": "fixture/two", "Test": "TestParent/child"},
                {"Action": "output", "Package": "fixture/one", "Test": "TestLater",
                 "Output": "later passing output " * 4000},
                {"Action": "pass", "Package": "fixture/one", "Test": "TestLater"},
            ]
            (directory / "go-test.jsonl").write_text("\n".join(json.dumps(e) for e in events))
            stream = io.StringIO()
            with redirect_stdout(stream):
                native.failure_tails(directory)
            self.assertIn("fixture/one/TestParent/child: junction rename: synthetic access denied", stream.getvalue())
            self.assertNotIn("unrelated successful output", stream.getvalue())
            self.assertLess(len(stream.getvalue()), 25000)

    def test_failed_test_output_has_a_bounded_explicit_truncation(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "go-test.jsonl"
            path.write_text("\n".join(json.dumps(e) for e in [
                {"Action": "output", "Package": "fixture", "Test": "TestFail", "Output": "x" * 40000},
                {"Action": "fail", "Package": "fixture", "Test": "TestFail"},
            ]))
            output = native.failed_native_output(path)
            self.assertLessEqual(len(output), 12000)
            self.assertIn(b"failed native output truncated", output)

    def test_failure_at_output_limit_still_reports_later_omission(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "go-test.jsonl"
            label = "fixture/TestFail: "
            events = [
                {"Action": "output", "Package": "fixture", "Test": "TestFail",
                 "Output": "x" * (128 - len(label))},
                {"Action": "output", "Package": "fixture", "Test": "TestFail", "Output": "later reason"},
                {"Action": "fail", "Package": "fixture", "Test": "TestFail"},
            ]
            path.write_text("\n".join(json.dumps(e) for e in events))
            output = native.failed_native_output(path, limit=128)
            self.assertLessEqual(len(output), 128)
            self.assertIn(b"failed native output truncated", output)

    def test_build_failure_reason_survives_later_passing_output(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "go-test.jsonl"
            events = [
                {"Action": "build-output", "ImportPath": "fixture/broken", "Output": "broken.go:2: undefined: syntheticName\n"},
                {"Action": "build-fail", "ImportPath": "fixture/broken"},
                {"Action": "output", "Package": "fixture/ok", "Test": "TestOK", "Output": "later output " * 4000},
                {"Action": "pass", "Package": "fixture/ok", "Test": "TestOK"},
            ]
            path.write_text("\n".join(json.dumps(e) for e in events))
            selected = native.failed_native_output(path)
            self.assertIn(b"fixture/broken: broken.go:2: undefined: syntheticName", selected)
            self.assertNotIn(b"later output", selected)

    def test_unavailable_or_malformed_diagnostics_are_explicit(self):
        with tempfile.TemporaryDirectory() as temporary:
            path = Path(temporary) / "go-test.jsonl"
            self.assertIn(b"unavailable", native.failed_native_output(path))
            path.write_text('{"Action": "output"\n')
            self.assertIn(b"incomplete", native.failed_native_output(path))

    def test_diagnostic_collection_failure_preserves_original_failed_report(self):
        with tempfile.TemporaryDirectory() as temporary:
            output = Path(temporary) / "new"
            argv = ["native_checks.py", "--output", str(output), "--shell-executable", "synthetic",
                    "--shell-version", "5.1", "--shell-edition", "Desktop",
                    "--installation-type", "Server", "--os-build", "26100"]
            stream = io.StringIO()
            with patch.object(native.sys, "argv", argv), patch.object(native.sys, "platform", "synthetic"), \
                    patch.object(native, "failure_tails", side_effect=OSError("synthetic diagnostic failure")), \
                    redirect_stdout(stream):
                result = native.main()
            self.assertEqual(result, 1)
            report = json.loads(stream.getvalue().split("JAEKIT_CI_REPORT ", 1)[1])
            self.assertEqual(report["status"], "failed")
            self.assertIn("requires actual native Windows", report["failures"][0])
            self.assertEqual(report["diagnostics"]["status"], "unavailable")
            self.assertEqual(json.loads((output / "summary.json").read_text()), report)


if __name__ == "__main__":
    unittest.main()
