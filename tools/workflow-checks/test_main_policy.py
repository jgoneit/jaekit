"""Offline policy regressions; all GitHub data and subprocess results are synthetic."""

import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import unittest
from unittest import mock


SOURCE = Path(__file__).resolve().parents[1] / "check_main_policy.py"
COMMIT = "a" * 40


def protection():
    return {
        "required_status_checks": {
            "strict": True, "contexts": ["Seal Core", "Public documentation"],
            "checks": [{"context": name, "app_id": 15368}
                       for name in ("Seal Core", "Public documentation")],
        },
        "enforce_admins": {"enabled": True},
        "required_pull_request_reviews": None,
        "restrictions": None,
    }


def runs():
    return [{"id": number, "name": name, "app": {"id": 15368}, "head_sha": COMMIT,
             "status": "completed", "conclusion": "success"}
            for number, name in enumerate(("Seal Core", "Public documentation"), 1)]


class MainPolicy(unittest.TestCase):
    def setUp(self):
        self.assertTrue(SOURCE.is_file(), "missing read-only policy checker: tools/check_main_policy.py")
        spec = importlib.util.spec_from_file_location("check_main_policy", SOURCE)
        self.checker = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(self.checker)
        # Every test rejects accidental real subprocess/network access.
        patcher = mock.patch.object(
            self.checker.subprocess, "run", side_effect=AssertionError("unexpected real subprocess"))
        self.process = patcher.start()
        self.addCleanup(patcher.stop)

    def invoke(self, policy=None, check_runs=None, rules=None, rulesets=None, commit=COMMIT):
        responses = [protection() if policy is None else policy,
                     [] if rules is None else rules, [] if rulesets is None else rulesets]
        if commit is not None:
            responses.append({"check_runs": runs() if check_runs is None else check_runs})
        output = io.StringIO()
        args = ["--repo", "example/project"]
        if commit is not None:
            args.extend(["--commit", commit])
        with mock.patch.object(self.checker, "api", side_effect=responses) as api:
            with contextlib.redirect_stdout(output):
                code = self.checker.main(args)
        return code, json.loads(output.getvalue()), api

    def test_valid_classic_policy_and_exact_commit(self):
        code, report, api = self.invoke()
        self.assertEqual(0, code)
        self.assertTrue(report["satisfied"])
        self.assertEqual(COMMIT, report["commit"])
        self.assertTrue(all(row["satisfied"] for row in report["results"]))
        self.assertIn(f"/commits/{COMMIT}/check-runs?filter=all", api.call_args.args[0])

    def test_policy_only_does_not_query_checks(self):
        code, report, api = self.invoke(commit=None)
        self.assertEqual(0, code)
        self.assertIsNone(report["commit"])
        self.assertEqual([], report["results"])
        self.assertEqual(3, api.call_count)

    def test_policy_regressions_fail(self):
        for label, mutate, expected in (
            ("admin", lambda p: p["enforce_admins"].update(enabled=False), "administrator_bypass_allowed"),
            ("stale", lambda p: p["required_status_checks"].update(strict=False), "strict_checks_disabled"),
            ("name", lambda p: p["required_status_checks"]["checks"][0].update(context="Wrong name"), "required_check_names_or_apps_mismatch"),
            ("app", lambda p: p["required_status_checks"]["checks"][0].update(app_id=-1), "required_check_names_or_apps_mismatch"),
            ("missing", lambda p: p["required_status_checks"]["checks"].pop(), "required_check_names_or_apps_mismatch"),
            ("extra", lambda p: p["required_status_checks"]["contexts"].append("Another gate"), "additional_required_contexts"),
            ("approval", lambda p: p.update(required_pull_request_reviews={"required_approving_review_count": 1}), "additional_review_policy"),
            ("restriction", lambda p: p.update(restrictions={"users": []}), "push_restrictions_present"),
            ("lock", lambda p: p.update(lock_branch={"enabled": True}), "unexpected_lock_branch"),
            ("force", lambda p: p.update(allow_force_pushes={"enabled": True}), "unexpected_allow_force_pushes"),
        ):
            with self.subTest(label=label):
                policy = protection()
                mutate(policy)
                code, report, _ = self.invoke(policy=policy)
                self.assertEqual(1, code)
                self.assertIn(expected, report["reasons"])

    def test_failed_pending_missing_stale_wrong_app_and_name_fail(self):
        for label, mutate, reason in (
            ("failed", lambda r: r[0].update(conclusion="failure"), "not_successful"),
            ("pending", lambda r: r[0].update(status="in_progress", conclusion=None), "not_completed"),
            ("missing", lambda r: r.pop(0), "missing"),
            ("stale", lambda r: r[0].update(head_sha="b" * 40), "stale_commit"),
            ("app", lambda r: r[0].update(app={"id": 999}), "wrong_app"),
            ("name", lambda r: r[0].update(name="Seal core"), "missing"),
            ("skipped", lambda r: r[0].update(conclusion="skipped"), "not_successful"),
            ("neutral", lambda r: r[0].update(conclusion="neutral"), "not_successful"),
        ):
            with self.subTest(label=label):
                check_runs = runs()
                mutate(check_runs)
                code, report, _ = self.invoke(check_runs=check_runs)
                self.assertEqual(1, code)
                self.assertIn(f"Seal Core:{reason}", report["reasons"])

    def test_old_success_does_not_mask_new_pending_or_failure(self):
        for status, conclusion in (("in_progress", None), ("completed", "failure")):
            with self.subTest(status=status):
                check_runs = runs()
                latest = dict(check_runs[0], id=100, status=status, conclusion=conclusion)
                code, report, _ = self.invoke(check_runs=[latest] + check_runs)
                self.assertEqual(1, code)
                self.assertEqual(100, report["results"][0]["id"])

    def test_additional_active_rulesets_fail_for_review(self):
        for kwargs in (
            {"rules": [{"type": "pull_request", "ruleset_id": 7}]},
            {"rulesets": [{"enforcement": "active", "target": "branch", "bypass_actors": []}]},
            {"rulesets": [{"enforcement": "active", "target": "push", "bypass_actors": [{"actor_type": "RepositoryRole"}]}]},
            {"rulesets": [{"enforcement": "active"}]},
        ):
            with self.subTest(kwargs=kwargs):
                code, report, _ = self.invoke(**kwargs)
                self.assertEqual(1, code)
                self.assertIn("additional_ruleset_unverified", report["reasons"])
        code, _, _ = self.invoke(rulesets=[{"enforcement": mode, "target": "branch"}
                                         for mode in ("disabled", "evaluate")])
        self.assertEqual(0, code)

    def test_unprotected_is_distinct_from_unreadable(self):
        for reason, status, expected in (("branch_unprotected", 404, 1),
                                         ("authentication_or_permission_denied", 403, 2),
                                         ("network_or_gh_error", None, 2)):
            with self.subTest(reason=reason):
                code, report, _ = self.invoke(policy=self.checker.EvidenceError(reason, status), commit=None)
                self.assertEqual(expected, code)
                self.assertIn(reason, report["reasons"])
                self.assertFalse(report["satisfied"])

    def test_head_is_resolved_locally_before_api_requests(self):
        self.process.side_effect = None
        self.process.return_value = subprocess.CompletedProcess([], 0, COMMIT + "\n", "")
        code, report, api = self.invoke(commit="HEAD")
        self.assertEqual(0, code)
        self.assertEqual(COMMIT, report["commit"])
        self.assertEqual(["git", "rev-parse", "--verify", "HEAD^{commit}"], self.process.call_args.args[0])
        self.assertIn(COMMIT, api.call_args.args[0])

    def test_invalid_commit_never_queries_github(self):
        code, report, api = self.invoke(commit="old-branch")
        self.assertEqual(2, code)
        self.assertEqual(["commit_must_be_full_sha_or_HEAD"], report["reasons"])
        api.assert_not_called()

    def test_api_only_uses_bounded_get_and_never_prints_raw_errors(self):
        self.process.side_effect = None
        self.process.return_value = subprocess.CompletedProcess([], 0, "{}", "")
        self.assertEqual({}, self.checker.api("repos/example/project/branches/main/protection"))
        call = self.process.call_args
        self.assertEqual(["gh", "api", "--hostname", "github.com", "--method", "GET"], call.args[0][:6])
        self.assertEqual(self.checker.TIMEOUT, call.kwargs["timeout"])
        self.assertNotIn("shell", call.kwargs)
        for body, stderr, reason in (
            ({"message": "Branch not protected", "status": "404"}, "private diagnostic", "branch_unprotected"),
            ({"message": "Not Found", "status": "404"}, "private diagnostic", "api_request_failed"),
            ({"message": "Bad credentials", "status": "401"}, "private diagnostic", "authentication_or_permission_denied"),
            (None, "Run gh auth login to authenticate", "authentication_required"),
            (None, "private diagnostic", "network_or_gh_error"),
        ):
            with self.subTest(reason=reason):
                self.process.reset_mock()
                self.process.return_value = subprocess.CompletedProcess([], 1, json.dumps(body), stderr)
                with self.assertRaises(self.checker.EvidenceError) as caught:
                    self.checker.api("repos/example/project/branches/main/protection")
                self.assertEqual(reason, str(caught.exception))
                self.assertEqual(1, self.process.call_count)

    def test_timeout_and_missing_gh_are_evidence_errors_without_retries(self):
        for error, reason in ((subprocess.TimeoutExpired("gh", 20), "request_timeout"),
                              (FileNotFoundError("synthetic"), "gh_unavailable")):
            with self.subTest(reason=reason):
                self.process.reset_mock()
                self.process.side_effect = error
                with self.assertRaises(self.checker.EvidenceError) as caught:
                    self.checker.api("repos/example/project/branches/main/protection")
                self.assertEqual(reason, str(caught.exception))
                self.assertEqual(1, self.process.call_count)

    def test_pagination_includes_later_results_and_has_a_bound(self):
        filler = [{"id": 9, "name": "Unrelated"}] * 100
        with mock.patch.object(self.checker, "api", side_effect=[{"check_runs": filler}, {"check_runs": runs()}]) as api:
            result = self.checker.pages("repos/example/project/commits/ref/check-runs?filter=all", "check_runs")
        self.assertEqual(102, len(result))
        self.assertIn("page=2", api.call_args.args[0])
        with mock.patch.object(self.checker, "api", return_value=filler) as api:
            with self.assertRaisesRegex(self.checker.EvidenceError, "pagination_limit"):
                self.checker.pages("repos/example/project/rulesets")
        self.assertEqual(self.checker.MAX_PAGES, api.call_count)

    def test_unknown_policy_shape_and_ruleset_enforcement_fail_closed(self):
        policy = protection()
        policy["required_status_checks"]["checks"][0]["app_id"] = [15368]
        code, report, _ = self.invoke(policy=policy)
        self.assertEqual(2, code)
        self.assertEqual(["invalid_protection_shape"], report["reasons"])
        code, report, _ = self.invoke(rulesets=[{"enforcement": "unrecognized"}])
        self.assertEqual(1, code)
        self.assertIn("ruleset_enforcement_unverified", report["reasons"])

    def test_invalid_api_response_and_local_head_failure_are_not_success(self):
        self.process.side_effect = None
        self.process.return_value = subprocess.CompletedProcess([], 0, "not json", "")
        with self.assertRaisesRegex(self.checker.EvidenceError, "invalid_api_json"):
            self.checker.api("repos/example/project/branches/main/protection")
        self.process.return_value = subprocess.CompletedProcess([], 1, "", "private diagnostic")
        code, report, api = self.invoke(commit="HEAD")
        self.assertEqual(2, code)
        self.assertEqual(["local_head_unavailable"], report["reasons"])
        api.assert_not_called()

    def test_cli_does_not_expose_raw_api_diagnostics(self):
        self.process.side_effect = None
        self.process.return_value = subprocess.CompletedProcess(
            [], 1, json.dumps({"message": "synthetic private text", "status": "403"}),
            "synthetic private stderr")
        output = io.StringIO()
        with contextlib.redirect_stdout(output):
            code = self.checker.main(["--repo", "example/project"])
        self.assertEqual(2, code)
        self.assertNotIn("synthetic private", output.getvalue())
        self.assertEqual("evidence_error", json.loads(output.getvalue())["outcome"])


if __name__ == "__main__":
    unittest.main()
