"""Synthetic transport faults; successful fixtures are not native CI evidence."""
from contextlib import redirect_stdout
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest.mock import patch

import test_ci_observer as fixtures


ROOT = Path(__file__).resolve().parents[2]
PR_NUMBER = 123
spec = importlib.util.spec_from_file_location(
    "windows_validation_logs", ROOT / "tools/verification-checks/windows_validation.py")
validation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(validation)


class WindowsJobLogCollection(unittest.TestCase):
    def setUp(self):
        self.fixture = fixtures.CurrentRunAdmission()
        self.fixture.setUp()
        for row in self.fixture.reports["Core / Windows"]:
            row["synthetic_transport_padding"] = "x" * (92 * 1024)
        self.ci = fixtures.observer
        self.commands = []
        self.api_paths = []
        self.log_error = None

    @staticmethod
    def truncated_view(log):
        # Model gh run view's silent scanner stop at the first oversized line.
        lines = []
        for line in log.splitlines(keepends=True):
            if len(line.encode("utf-8")) > 64 * 1024:
                break
            lines.append(line)
        return "".join(lines)

    def api(self, path):
        self.api_paths.append(path)
        return self.fixture.api(path.replace(f"/pulls/{PR_NUMBER}", "/pulls/1"))

    def command(self, argv, **kwargs):
        self.commands.append(argv)
        if argv[:3] == ["gh", "repo", "view"]:
            return json.dumps({"nameWithOwner": "jgoneit/jaekit"})
        if argv[:3] == ["gh", "pr", "view"]:
            self.assertEqual(argv, ["gh", "pr", "view", "--json", "number,headRefOid"])
            return json.dumps({"number": PR_NUMBER, "headRefOid": fixtures.HEAD})
        if argv[:3] == ["gh", "run", "view"]:
            return self.truncated_view(self.fixture.command(argv, **kwargs))
        if argv[:4] == ["gh", "api", "--method", "GET"]:
            if self.log_error is not None:
                raise self.log_error
            paths = {f"repos/jgoneit/jaekit/actions/jobs/{job['id']}/logs": job
                     for job in self.fixture.jobs if job["name"] in self.ci.JOBS}
            self.assertEqual(argv[4], "--allow-escape-sequences")
            self.assertIn(argv[5], paths)
            self.assertEqual(len(argv), 6)
            job = paths[argv[5]]
            # Reuse the real observer's existing source/runner/report fixture.
            return "\x1b[0m\n" + self.fixture.command([
                "gh", "run", "view", "7", "--repo", "jgoneit/jaekit",
                "--attempt", "1", "--job", str(job["id"]), "--log"])
        return self.fixture.command(argv, **kwargs)

    def observe(self):
        original = self.command
        stdout = io.StringIO()
        with patch.object(validation, "load", return_value=self.ci), \
                patch.object(self.ci, "api", self.api), patch.object(self.ci, "command", original), \
                redirect_stdout(stdout):
            try:
                return validation.live_reports()
            finally:
                self.assertIs(self.ci.command, original, "command hook was not restored")
                self.assertFalse(any(argv[:3] == ["gh", "run", "view"] for argv in self.commands),
                                 "line-scanning run-view logs were used as a fallback")
                self.assertNotIn("\x1b", stdout.getvalue(), "raw log escapes were printed")
                self.assertNotIn("JAEKIT_CI_REPORT ", stdout.getvalue(), "raw reports were printed")

    def test_raw_selected_job_logs_preserve_both_oversized_json_reports(self):
        job = next(job for job in self.fixture.jobs if job["name"] == "Core / Windows")
        raw = self.fixture.command(["gh", "run", "view", "7", "--job", str(job["id"]), "--log"])
        report_lines = [line for line in raw.splitlines() if line.startswith("JAEKIT_CI_REPORT ")]
        self.assertEqual(len(report_lines), 2)
        self.assertTrue(all(len(line.encode("utf-8")) > 64 * 1024 for line in report_lines))
        self.assertEqual(self.ci.records(self.truncated_view(raw), "JAEKIT_CI_REPORT "), [])
        rows = self.observe()
        self.assertEqual([row["environment"]["shell_version"] for row in rows], ["5.1", "7.5"])
        self.assertTrue(all(row["synthetic_transport_padding"] == "x" * (92 * 1024) for row in rows))
        expected = {f"repos/jgoneit/jaekit/actions/jobs/{job['id']}/logs"
                    for job in self.fixture.jobs if job["name"] in self.ci.JOBS}
        actual = [argv[5] for argv in self.commands if argv[:4] == ["gh", "api", "--method", "GET"]]
        self.assertEqual(set(actual), expected)
        self.assertEqual(len(actual), len(expected))
        self.assertIn("repos/jgoneit/jaekit/actions/runs/7/attempts/1/jobs?per_page=100", self.api_paths)

    def test_current_branch_pr_is_used_without_a_fixed_number(self):
        self.observe()
        self.assertEqual(self.api_paths.count(f"repos/jgoneit/jaekit/pulls/{PR_NUMBER}"), 2)
        self.assertNotIn("repos/jgoneit/jaekit/pulls/67", self.api_paths)

    def test_missing_or_duplicate_windows_reports_remain_rejected(self):
        for count in (0, 1, 3):
            self.setUp()
            rows = self.fixture.reports["Core / Windows"]
            self.fixture.reports["Core / Windows"] = (rows + rows)[:count]
            with self.subTest(count=count), self.assertRaisesRegex(validation.Violation, "native report missing/duplicate"):
                self.observe()

    def test_job_log_api_error_propagates_without_run_view_fallback(self):
        self.log_error = self.ci.ObservationError("synthetic job log request failed")
        with self.assertRaisesRegex(self.ci.ObservationError, "job log request failed"):
            self.observe()

    def test_shared_current_source_validation_is_still_required(self):
        self.fixture.commit["tree"]["sha"] = "e" * 40
        with self.assertRaisesRegex(validation.Violation, "CI source tree differs"):
            self.observe()

    def test_newer_attempt_during_raw_log_collection_is_rejected(self):
        self.fixture.after_log = lambda: self.fixture.runs.append(
            dict(self.fixture.run, run_attempt=2, status="in_progress", conclusion=None))
        with self.assertRaisesRegex(validation.Violation, "latest run changed"):
            self.observe()


if __name__ == "__main__":
    unittest.main()
