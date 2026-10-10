#!/usr/bin/env python3
"""Read GitHub's classic main protection and, optionally, exact-commit CI results.

This checker never changes GitHub or retries a failed request. Additional active
branch/push rulesets fail closed for review, even if their applicability is not
clear; this is intentionally not a general ruleset interpreter. Exit 1 means an
observed unmet condition; exit 2 means the evidence could not be read reliably.
Only selected public metadata is printed, never raw API responses or stderr.
"""

import argparse
import json
import os
import re
import subprocess
from urllib.parse import quote


CHECKS = ("Seal Core", "Public documentation")
APP_ID = 15368  # github-actions on github.com
TIMEOUT = 20
MAX_PAGES = 10
SHA = re.compile(r"[0-9a-f]{40}\Z")


class EvidenceError(Exception):
    def __init__(self, reason, http_status=None):
        super().__init__(reason)
        self.reason = reason
        self.http_status = http_status


def api(path):
    env = dict(os.environ, GH_PROMPT_DISABLED="1")
    env.pop("GH_DEBUG", None)
    try:
        result = subprocess.run(
            ["gh", "api", "--hostname", "github.com", "--method", "GET",
             "-H", "Accept: application/vnd.github+json",
             "-H", "X-GitHub-Api-Version: 2022-11-28", path],
            capture_output=True, text=True, timeout=TIMEOUT, env=env,
        )
    except subprocess.TimeoutExpired:
        raise EvidenceError("request_timeout") from None
    except OSError:
        raise EvidenceError("gh_unavailable") from None
    try:
        body = json.loads(result.stdout)
    except ValueError:
        body = None
    if result.returncode:
        if result.returncode == 4 or "gh auth login" in result.stderr:
            raise EvidenceError("authentication_required")
        match = re.search(r"\(HTTP (\d{3})\)", result.stderr)
        status = int(match[1]) if match else None
        if isinstance(body, dict) and str(body.get("status", "")).isdigit():
            status = int(body["status"])
        if status == 404 and isinstance(body, dict) and body.get("message") == "Branch not protected":
            raise EvidenceError("branch_unprotected", status)
        if status in (401, 403):
            raise EvidenceError("authentication_or_permission_denied", status)
        raise EvidenceError("api_request_failed" if status else "network_or_gh_error", status)
    if body is None:
        raise EvidenceError("invalid_api_json")
    return body


def pages(path, key=None):
    """Bound pagination and reject truncated/unknown responses, without retries."""
    items = []
    for number in range(1, MAX_PAGES + 1):
        separator = "&" if "?" in path else "?"
        body = api(f"{path}{separator}per_page=100&page={number}")
        page = body.get(key) if key and isinstance(body, dict) else body
        if not isinstance(page, list) or any(not isinstance(item, dict) for item in page):
            raise EvidenceError("invalid_api_shape")
        items.extend(page)
        if len(page) < 100:
            return items
    raise EvidenceError("pagination_limit")


def policy_result(protection, rules, rulesets):
    reasons = []
    if protection is None:
        summary = {"classic_protection": False}
        reasons.append("branch_unprotected")
    else:
        required = protection.get("required_status_checks") or {}
        admins = protection.get("enforce_admins") or {}
        if not isinstance(required, dict) or not isinstance(admins, dict):
            raise EvidenceError("invalid_protection_shape")
        configured = required.get("checks", [])
        contexts = required.get("contexts", [])
        if not isinstance(configured, list) or any(not isinstance(row, dict) for row in configured):
            raise EvidenceError("invalid_protection_shape")
        if any(not isinstance(row.get("context"), str) or type(row.get("app_id")) is not int
               for row in configured):
            raise EvidenceError("invalid_protection_shape")
        expected = {(name, APP_ID) for name in CHECKS}
        pairs = [(row.get("context"), row.get("app_id")) for row in configured]
        if len(pairs) != len(expected) or any(pair not in expected for pair in pairs) or len(set(pairs)) != len(expected):
            reasons.append("required_check_names_or_apps_mismatch")
        if not isinstance(contexts, list) or any(name not in CHECKS for name in contexts):
            reasons.append("additional_required_contexts")
        if required.get("strict") is not True:
            reasons.append("strict_checks_disabled")
        if admins.get("enabled") is not True:
            reasons.append("administrator_bypass_allowed")
        if protection.get("required_pull_request_reviews") is not None:
            reasons.append("additional_review_policy")
        if protection.get("restrictions") is not None:
            reasons.append("push_restrictions_present")
        for name in ("lock_branch", "required_conversation_resolution", "required_linear_history",
                     "required_signatures", "allow_force_pushes", "allow_deletions"):
            value = protection.get(name, {"enabled": False})
            if not isinstance(value, dict) or value.get("enabled") is not False:
                reasons.append(f"unexpected_{name}")
        summary = {
            "classic_protection": True,
            "strict": required.get("strict") is True,
            "enforce_admins": admins.get("enabled") is True,
            "required_checks_match": "required_check_names_or_apps_mismatch" not in reasons,
            "review_policy_present": protection.get("required_pull_request_reviews") is not None,
            "push_restrictions_present": protection.get("restrictions") is not None,
        }
    active = 0
    for ruleset in rulesets:
        enforcement = ruleset.get("enforcement")
        if enforcement not in ("disabled", "evaluate", "active"):
            reasons.append("ruleset_enforcement_unverified")
        elif enforcement == "active" and ruleset.get("target") != "tag":
            active += 1
    if rules or active:
        reasons.append("additional_ruleset_unverified")
    summary.update({"applied_rule_count": len(rules), "unverified_active_rulesets": active})
    return summary, reasons


def check_results(runs, commit):
    results, reasons = [], []
    for name in CHECKS:
        named = [run for run in runs if run.get("name") == name]
        matching = [run for run in named if isinstance(run.get("app"), dict) and run["app"].get("id") == APP_ID]
        row = {"name": name, "app_id": APP_ID, "satisfied": False}
        if not matching:
            row["reason"] = "wrong_app" if named else "missing"
        elif any(type(run.get("id")) is not int for run in matching):
            raise EvidenceError("invalid_check_run_id")
        else:
            # A new pending/failed run must not be hidden by an older success.
            latest = max(matching, key=lambda run: run["id"])
            row["id"] = latest["id"]
            row["status"] = latest.get("status") if latest.get("status") in (
                "queued", "in_progress", "completed", "waiting", "requested", "pending") else "unknown"
            row["conclusion"] = latest.get("conclusion") if latest.get("conclusion") in (
                None, "success", "failure", "neutral", "cancelled", "skipped", "timed_out",
                "action_required", "stale", "startup_failure") else "unknown"
            if latest.get("head_sha") != commit:
                row["reason"] = "stale_commit"
            elif latest.get("status") != "completed":
                row["reason"] = "not_completed"
            elif latest.get("conclusion") != "success":
                row["reason"] = "not_successful"
            else:
                row["satisfied"] = True
        if not row["satisfied"]:
            reasons.append(f"{name}:{row['reason']}")
        results.append(row)
    return results, reasons


def resolve_commit(value):
    if value is None:
        return None
    if value == "HEAD":
        try:
            result = subprocess.run(
                ["git", "rev-parse", "--verify", "HEAD^{commit}"],
                capture_output=True, text=True, timeout=TIMEOUT,
            )
        except (OSError, subprocess.TimeoutExpired):
            raise EvidenceError("local_head_unavailable") from None
        if result.returncode:
            raise EvidenceError("local_head_unavailable")
        value = result.stdout.strip()
    if not SHA.fullmatch(value):
        raise EvidenceError("commit_must_be_full_sha_or_HEAD")
    return value


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True, help="GitHub owner/repository")
    parser.add_argument("--branch", default="main")
    parser.add_argument("--commit", help="Full SHA, or HEAD resolved in the current checkout")
    args = parser.parse_args(argv)
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", args.repo):
        parser.error("--repo must be owner/repository")
    if not args.branch or any(ord(c) < 32 for c in args.branch):
        parser.error("--branch must be a nonempty branch name")
    report = {"repo": args.repo, "branch": args.branch, "required_checks": list(CHECKS),
              "app_id": APP_ID, "commit": None, "policy": None, "results": [], "satisfied": False}
    try:
        report["commit"] = resolve_commit(args.commit)
        branch = quote(args.branch, safe="")
        prefix = f"repos/{args.repo}"
        try:
            protection = api(f"{prefix}/branches/{branch}/protection")
        except EvidenceError as error:
            if error.reason != "branch_unprotected":
                raise
            protection = None
        if protection is not None and not isinstance(protection, dict):
            raise EvidenceError("invalid_protection_shape")
        rules = pages(f"{prefix}/rules/branches/{branch}")
        rulesets = pages(f"{prefix}/rulesets?includes_parents=true")
        report["policy"], reasons = policy_result(protection, rules, rulesets)
        if report["commit"]:
            runs = pages(f"{prefix}/commits/{report['commit']}/check-runs?filter=all", "check_runs")
            report["results"], failures = check_results(runs, report["commit"])
            reasons.extend(failures)
        report.update({"reasons": reasons, "satisfied": not reasons,
                       "outcome": "policy_or_checks_unmet" if reasons else "satisfied"})
        code = int(bool(reasons))
    except EvidenceError as error:
        report.update({"outcome": "evidence_error", "reasons": [error.reason]})
        if error.http_status is not None:
            report["http_status"] = error.http_status
        code = 2
    print(json.dumps(report, sort_keys=True, separators=(",", ":")))
    return code


if __name__ == "__main__":
    raise SystemExit(main())
