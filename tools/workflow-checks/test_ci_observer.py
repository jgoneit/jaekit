"""Synthetic faults exercise evidence admission; they are not OS runtime proof."""
import importlib.util
import json
from pathlib import Path
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("ci_observer", ROOT / "tools/verification-checks/cross_platform_ci.py")
observer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(observer)
SOURCE = "a" * 40
HEAD = "b" * 40
BASE = "c" * 40
TREE = "d" * 40


def report(windows=False, shell="7.5"):
    names = observer.SCENARIOS | {"TestNativeCoreGitLifecycle/linked=false", "TestNativeCoreGitLifecycle/linked=true"} | (observer.WINDOWS if windows else set())
    row = {"schema": "jaekit-native-windows/v1" if windows else "jaekit-native-unix/v1",
           "status": "passed", "source_commit": SOURCE,
           "environment": {"shell_version": shell, "goarch": "amd64", "goos": "windows" if windows else "linux", "installation_type": "Server"},
           "native_tests": {"required": sorted(names), "passed": sorted(names), "skipped": []}}
    if windows:
        row.update(package_checks=sorted(observer.PACKAGE_CHECKS),
                   candidate={"source_commit": SOURCE, "capabilities": {"synthetic": True},
                              "binary_sha256": "digest", "archive_sha256": "digest"}, server_rejection={
            "install": {"rejected": True, "preserved": True},
            "uninstall": {"rejected": True, "preserved": True}})
    else:
        row.update(candidates={"current": {"version": "synthetic", "capabilities": {"synthetic": True}, "binary_sha256": "digest"}},
                   installations=[{"guide": guide, "checks": sorted(observer.INSTALL), "binary_sha256": "digest",
                                   "installed_version": "synthetic"} for guide in ("guides/INSTALL.md", "guides/INSTALL.en.md")])
    return row


class NativeAdmission(unittest.TestCase):
    def test_valid_synthetic_reports_are_accepted(self):
        observer.native_report(report(), "Linux", SOURCE)
        observer.native_report(report(True), "Windows", SOURCE)

    def test_unix_incomplete_stale_skipped_and_wrong_binary_are_rejected(self):
        mutations = [lambda r: r.update(status="failed"), lambda r: r.update(source_commit=HEAD),
                     lambda r: r["native_tests"].update(passed=[]),
                     lambda r: r["native_tests"].update(required=[]),
                     lambda r: r["native_tests"].update(skipped=["required"]),
                     lambda r: r["environment"].update(shell_version=None),
                     lambda r: r.update(installations=[]),
                     lambda r: r["installations"][0].update(checks=[]),
                     lambda r: r["installations"][0].update(binary_sha256="other"),
                     lambda r: r["installations"][0].update(installed_version="older")]
        for mutate in mutations:
            row = report()
            mutate(row)
            with self.subTest(row=row), self.assertRaises(observer.Violation):
                observer.native_report(row, "Linux", SOURCE)

    def test_windows_requires_rejection_and_preservation_for_both_entrypoints(self):
        for action in ("install", "uninstall"):
            for field in ("rejected", "preserved"):
                row = report(True)
                row["server_rejection"][action][field] = False
                with self.subTest(action=action, field=field), self.assertRaises(observer.Violation):
                    observer.native_report(row, "Windows", SOURCE)


class CurrentRunAdmission(unittest.TestCase):
    def setUp(self):
        self.run = {"id": 7, "run_attempt": 1, "head_sha": HEAD, "event": "pull_request",
                    "status": "completed", "conclusion": "success", "html_url": "https://example.invalid/run/7"}
        self.runs = [self.run]
        self.after_log = None
        self.local_head, self.local_tree, self.dirty = HEAD, TREE, ""
        self.pr = {"head": {"sha": HEAD}, "base": {"sha": BASE}, "state": "open"}
        self.jobs = [{"id": i, "name": name, "status": "completed", "conclusion": "success",
                      "steps": [{"status": "completed", "conclusion": "success"}]} for i, name in enumerate(sorted(observer.REQUIRED))]
        self.commit = {"parents": [{"sha": HEAD}, {"sha": BASE}], "tree": {"sha": TREE}}
        self.reports = {name: [report(True, "5.1"), report(True, "7.5")] if os == "Windows" else [report()]
                        for name, os in observer.JOBS.items()}
        self.reports["Core / macOS"][0]["environment"]["goos"] = "darwin"

    def api(self, path):
        if path.endswith("/pulls/1"):
            return json.loads(json.dumps(self.pr))
        if "/ci.yml/runs?" in path:
            return {"workflow_runs": self.runs}
        if "/jobs?" in path:
            return {"total_count": len(self.jobs), "jobs": self.jobs}
        if "/git/commits/" in path:
            return self.commit
        raise AssertionError(path)

    def command(self, argv, **kwargs):
        if argv == ["gh", "api", "--help"]:
            return "FLAGS\n  --allow-escape-sequences  Allow escape sequences in API output\n"
        if argv[:3] == ["gh", "repo", "view"]:
            return json.dumps({"nameWithOwner": "synthetic/repository"})
        if argv[:3] == ["gh", "pr", "view"]:
            return json.dumps({"number": 1, "headRefOid": HEAD})
        if argv == ["git", "rev-parse", "HEAD"]:
            return self.local_head
        if argv == ["git", "rev-parse", "HEAD^{tree}"]:
            return self.local_tree
        if argv == ["git", "status", "--porcelain"]:
            return self.dirty
        if argv[:4] == ["gh", "api", "--method", "GET"]:
            return self.job_log(int(argv[-1].split("/")[-2]))
        raise AssertionError(argv)

    def job_log(self, job_id):
        if self.after_log is not None:
            mutate, self.after_log = self.after_log, None
            mutate()
        job = next(j for j in self.jobs if j["id"] == job_id)
        context = {"source_sha": SOURCE, "head_sha": HEAD, "base_sha": BASE, "event": "pull_request",
                   "runner_os": observer.JOBS[job["name"]], "os": "synthetic", "arch": "synthetic", "shell": "synthetic"}
        return "JAEKIT_CI_CONTEXT " + json.dumps(context) + "\n" + "\n".join(
            "JAEKIT_CI_REPORT " + json.dumps(r) for r in self.reports[job["name"]])

    def observe(self):
        with patch.object(observer, "api", self.api), patch.object(observer, "command", self.command):
            return observer.live_run()

    def test_synthetic_success_binds_all_jobs_and_merge_tree(self):
        self.assertEqual(self.observe(), ("synthetic/repository", HEAD))

    def test_extension_validates_each_native_report(self):
        calls = []
        with patch.object(observer, "api", self.api), patch.object(observer, "command", self.command):
            observer.live_run(lambda report, os, source: calls.append((os, source)))
        self.assertEqual(sorted(calls), [("Linux", SOURCE), ("Windows", SOURCE), ("Windows", SOURCE), ("macOS", SOURCE)])

    def test_reports_larger_than_scanner_limit_are_not_truncated(self):
        row = self.reports["Core / Windows"][0]
        row["padding"] = "x" * 100000
        self.assertEqual(self.observe(), ("synthetic/repository", HEAD))

    def test_latest_run_failure_cannot_reuse_older_success(self):
        newer = dict(self.run, id=8, conclusion="failure")
        self.runs.append(newer)
        with self.assertRaises(observer.Violation):
            self.observe()

    def test_newer_pending_run_started_while_reading_logs_is_rejected(self):
        self.after_log = lambda: self.runs.append(dict(self.run, id=8, status="in_progress", conclusion=None))
        with self.assertRaisesRegex(observer.Violation, "latest run changed"):
            self.observe()

    def test_head_base_or_state_changed_while_reading_logs_is_rejected(self):
        for field in ("head", "base", "state"):
            self.setUp()
            def mutate():
                if field == "state":
                    self.pr[field] = "closed"
                else:
                    self.pr[field]["sha"] = "e" * 40
            self.after_log = mutate
            with self.subTest(field=field), self.assertRaisesRegex(observer.Violation, "PR changed"):
                self.observe()

    def test_local_source_changed_while_reading_logs_is_rejected(self):
        for field, value in (("local_head", "e" * 40), ("local_tree", "e" * 40), ("dirty", " M source.go")):
            self.setUp()
            self.after_log = lambda: setattr(self, field, value)
            with self.subTest(field=field), self.assertRaisesRegex(observer.Violation, "local source changed"):
                self.observe()

    def test_missing_job_and_each_non_success_step_are_rejected(self):
        for outcome in ("failure", "skipped", "cancelled", None):
            self.jobs[0]["steps"][0]["conclusion"] = outcome
            with self.subTest(outcome=outcome), self.assertRaises(observer.Violation):
                self.observe()
        self.jobs.pop()
        with self.assertRaises(observer.Violation):
            self.observe()

    def test_wrong_merge_tree_and_parent_are_rejected(self):
        for commit in ({"parents": [{"sha": HEAD}], "tree": {"sha": TREE}},
                       {"parents": [{"sha": HEAD}, {"sha": BASE}], "tree": {"sha": "e" * 40}}):
            self.commit = commit
            with self.subTest(commit=commit), self.assertRaises(observer.Violation):
                self.observe()

    def test_two_reports_from_same_powershell_do_not_cover_both_shells(self):
        self.reports["Core / Windows"] = [report(True, "7.4"), report(True, "7.5")]
        with self.assertRaises(observer.Violation):
            self.observe()


if __name__ == "__main__":
    unittest.main()
