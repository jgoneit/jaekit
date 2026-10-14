#!/usr/bin/env python3
"""Observe public review boundaries and live PR evidence, without saved success input."""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
REPOSITORY = "jgoneit/jaekit"
PULL_REQUEST = 67
CAPABILITY_REVIEW = 4262575830
FIX_REVIEWS = {4262575810, 4262575822}
VIOLATION = "windows-review-boundary-missing"
PROCESS_SCENARIO = "testdata/scenarios/_native/windows-process-control/cases.json"
PROCESS_TESTS = {
    "TestNativeWindowsProcessControlScenarios", "TestNativeProcessTimeoutScenarios",
    "TestNativeProcessUnconfirmedShutdownScenarios",
}
PROCESS_SUBCASES = {
    "TestNativeWindowsProcessControlScenarios": (
        "parent-pass-check", "parent-violation-check", "parent-pass-baseline", "parent-violation-baseline"),
    "TestNativeProcessTimeoutScenarios": ("timeout-check", "timeout-baseline"),
    "TestNativeProcessUnconfirmedShutdownScenarios": (
        "rules-3-check", "rules-3-baseline", "rules-2-check", "rules-2-baseline", "rules-1-check", "rules-1-baseline"),
}
CONSOLE_SOURCES = tuple("tools/windows/" + name for name in
                        ("package.py", "common.ps1", "install.ps1", "uninstall.ps1"))
ISSUE_SECTIONS = ("Review", "Current gap", "Deferral and remaining risk", "Acceptance criteria")
ISSUE_TERMS = ("default_rules", "supported_rules", "formats", "features", "field types",
               "existing installation", "PATH", "optional")


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise Violation(message)


def command(argv, timeout=180):
    try:
        result = subprocess.run(argv, cwd=ROOT, capture_output=True, text=True,
                                encoding="utf-8", errors="strict", timeout=timeout)
    except (OSError, UnicodeError, subprocess.TimeoutExpired) as error:
        raise ObservationError(type(error).__name__) from None
    if result.returncode:
        # Never expose failed GitHub requests, credentials, or private paths.
        raise ObservationError("GitHub request failed" if argv[0] == "gh" else "local observation command failed")
    return result.stdout


def api(path, paginated=False):
    argv = ["gh", "api", "--method", "GET", path]
    if paginated:
        argv += ["--paginate", "--slurp"]
    try:
        value = json.loads(command(argv))
    except json.JSONDecodeError:
        raise ObservationError("invalid GitHub JSON") from None
    if paginated:
        if not isinstance(value, list) or not all(isinstance(page, list) for page in value):
            raise ObservationError("invalid paginated GitHub response")
        return [row for page in value for row in page]
    return value


def load(name, relative):
    spec = importlib.util.spec_from_file_location(name, ROOT / relative)
    if spec is None or spec.loader is None:
        raise ObservationError("observer module is unavailable")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def digest(relative):
    return hashlib.sha256((ROOT / relative).read_bytes()).hexdigest()


def feature_boundary(criterion):
    if criterion in {"AC-1", "AC-2", "AC-6", "AC-8"}:
        require((ROOT / PROCESS_SCENARIO).is_file(), "public command-level process scenario is absent")
        tests = "\n".join(path.read_text(encoding="utf-8")
                          for path in (ROOT / "cmd/ha").glob("native_process*_test.go"))
        names = (PROCESS_TESTS if criterion != "AC-2" else {"TestNativeProcessUnconfirmedShutdownScenarios"})
        require(all("func " + name + "(" in tests for name in names), "command-level process cases are absent")
    if criterion in {"AC-3", "AC-4", "AC-6", "AC-8"}:
        package = load("triage_package_boundary", "tools/windows/package.py")
        for name in ("CONIN$", "conout$", "guides/ConIn$.txt", "guides/conout$.log/child.txt"):
            try:
                package.safe_name(name)
            except ValueError:
                pass
            else:
                raise Violation("package consumer still accepts a console-name component")
        for name in ("CONINX", "CONOUT$extra", "guides/ordinary space/\ud55c\uae00.txt"):
            try:
                package.safe_name(name)
            except ValueError:
                raise Violation("package policy rejects an ordinary name") from None
    if criterion in {"AC-6", "AC-8"}:
        require((ROOT / "tools/windows/native_console_checks.ps1").is_file(),
                "native console-name observations are absent")


def live():
    ci = load("triage_ci_observer", "tools/verification-checks/cross_platform_ci.py")
    validator = load("triage_console_results", "tools/windows/console_results.py")
    reports, run_ids, snapshots = [], set(), {}
    original, original_api = ci.command, ci.api

    def observed_command(argv, *args, **kwargs):
        if argv[:3] == ["gh", "pr", "view"]:
            argv = [*argv[:3], str(PULL_REQUEST), "--repo", REPOSITORY, *argv[3:]]
        output = original(argv, *args, **kwargs)
        if argv[:3] == ["gh", "run", "view"] and "--log" in argv:
            run_ids.add(int(argv[3]))
            reports.extend(ci.records(output, "JAEKIT_CI_REPORT "))
        return output

    def observed_api(path):
        value = original_api(path)
        if path == f"repos/{REPOSITORY}/pulls/{PULL_REQUEST}":
            snapshots["pr"] = value
        if "/actions/workflows/ci.yml/runs?" in path:
            snapshots["runs"] = value
        return value

    ci.command, ci.api = observed_command, observed_api
    try:
        ci.configuration_present()
        repo, head = ci.live_run()
    except ci.Violation as error:
        raise Violation(str(error)) from None
    except ci.ObservationError as error:
        raise ObservationError(str(error)) from None
    require(repo == REPOSITORY and len(run_ids) == 1, "wrong repository or ambiguous observed CI run")
    windows = [row for row in reports if row.get("schema") == "jaekit-native-windows/v1"]
    require(len(windows) == 2, "both Windows shell observations are required")
    expected = {"github.com/jgoneit/jaekit/cmd/ha/" + name for name in PROCESS_TESTS}
    expected |= {"github.com/jgoneit/jaekit/cmd/ha/" + name + "/" + case
                 for name, cases in PROCESS_SUBCASES.items() for case in cases}
    for report in windows:
        tests = report.get("native_tests", {})
        require(expected <= set(tests.get("required", [])) and expected <= set(tests.get("passed", []))
                and tests.get("skipped") == [], "native command-level process observations are incomplete")
        injected = {name for name in expected if "UnconfirmedShutdown" in name}
        require(injected <= set(tests.get("synthetic_failure_injection", [])),
                "command-boundary shutdown injection must remain distinct from an actual Windows API failure")
        try:
            validator.console_report(report.get("console_path_checks"), report["environment"]["shell_version"],
                                     {name: digest(name) for name in CONSOLE_SOURCES})
        except (ValueError, TypeError, KeyError) as error:
            raise Violation(str(error)) from None
    run_id = run_ids.pop()
    runs = [row for row in snapshots["runs"]["workflow_runs"] if row.get("id") == run_id]
    require(len(runs) == 1, "observed CI run is missing or ambiguous")
    return {"head": head, "base": snapshots["pr"]["base"]["sha"], "run_id": run_id,
            "run_identity": tuple(runs[0].get(key) for key in ("id", "run_attempt", "status", "conclusion")),
            "url": f"https://github.com/{REPOSITORY}/actions/runs/{run_id}"}


def comments():
    rows = api(f"repos/{REPOSITORY}/pulls/{PULL_REQUEST}/comments?per_page=100", paginated=True)
    if not all(isinstance(row, dict) for row in rows):
        raise ObservationError("invalid review comment response")
    return rows


def capability_followup():
    review_url = f"https://github.com/{REPOSITORY}/pull/{PULL_REQUEST}#discussion_r{CAPABILITY_REVIEW}"
    issues = api(f"repos/{REPOSITORY}/issues?state=all&per_page=100", paginated=True)
    rows = comments()
    require(sum(row.get("id") == CAPABILITY_REVIEW and not row.get("in_reply_to_id") for row in rows) == 1,
            "capability review is missing or ambiguous")
    user = api("user")["login"]
    linked = [issue for issue in issues if not issue.get("pull_request")
              and review_url in (issue.get("body") or "")]
    replies = [row for row in rows if row.get("in_reply_to_id") == CAPABILITY_REVIEW
               and row.get("user", {}).get("login") == user]
    # Observe both live resources before declaring their absence. An API error
    # never becomes a baseline proving that the follow-up does not exist.
    print(json.dumps({"review": CAPABILITY_REVIEW, "linked_issues": len(linked), "author_replies": len(replies)}))
    require(len(linked) == 1, "capability follow-up issue is missing or ambiguous")
    issue = linked[0]
    body = issue.get("body") or ""
    require(all(re.search(r"(?m)^## " + re.escape(section) + r"\s*$", body) for section in ISSUE_SECTIONS)
            and all(term in body for term in ISSUE_TERMS)
            and "tools/windows/install.ps1" in body and "contracts/core-capabilities.md" in body,
            "capability follow-up lacks the gap, deferral, compatibility or preservation contract")
    issue_url = f"https://github.com/{REPOSITORY}/issues/{issue['number']}"
    require(issue.get("html_url") == issue_url, "follow-up issue belongs to another repository")
    require(any(issue_url in (row.get("body") or "") and
                re.search(r"follow.up|defer|\ud6c4\uc18d|\ubbf8\ub8e8", row["body"], re.I) for row in replies),
            "capability review has no issue-linked deferral reply")
    print(json.dumps({"review": CAPABILITY_REVIEW, "issue": issue_url, "scope": "follow-up, not implemented"}))


def fix_replies(evidence):
    rows = comments()
    user = api("user")["login"]
    for review in FIX_REVIEWS:
        require(sum(row.get("id") == review and not row.get("in_reply_to_id") for row in rows) == 1,
                "target fix review is missing or ambiguous")
        found = [row for row in rows if row.get("in_reply_to_id") == review
                 and row.get("user", {}).get("login") == user]
        require(any(evidence["head"] in (row.get("body") or "") and evidence["url"] in row["body"]
                    and "Windows 11" in row["body"]
                    and (review != 4262575810 or re.search(r"inject|\uc8fc\uc785", row["body"], re.I)) for row in found),
                "fix review lacks current-source CI and remaining-environment evidence")
    print("Both fix reviews link the current commit, observed CI and remaining Windows 11 boundary.")


def revalidate(evidence):
    pr = api(f"repos/{REPOSITORY}/pulls/{PULL_REQUEST}")
    require(pr.get("state") == "open" and pr["head"]["sha"] == evidence["head"]
            and pr["base"]["sha"] == evidence["base"], "PR changed during observation")
    runs = api(f"repos/{REPOSITORY}/actions/workflows/ci.yml/runs?event=pull_request&head_sha={evidence['head']}&per_page=100")
    matched = [row for row in runs["workflow_runs"] if row.get("head_sha") == evidence["head"]
               and row.get("event") == "pull_request"]
    require(matched, "CI disappeared during observation")
    latest = max(matched, key=lambda row: (row["id"], row.get("run_attempt", 1)))
    require(tuple(latest.get(key) for key in ("id", "run_attempt", "status", "conclusion"))
            == evidence["run_identity"], "latest CI changed during observation")
    require(command(["git", "rev-parse", "HEAD"]).strip() == evidence["head"]
            and not command(["git", "status", "--porcelain"]).strip(), "local source changed during observation")


def main():
    criterion = sys.argv[1] if len(sys.argv) == 2 else ""
    observation = {"target": criterion, "attempt": 1, "status": "pass"}
    code = 0
    try:
        require(criterion in {f"AC-{n}" for n in range(1, 9)}, "unknown criterion")
        feature_boundary(criterion)
        if criterion == "AC-7":
            capability_followup()
        else:
            evidence = live()
            if criterion == "AC-5":
                for script in ("tools/check_public_tree.py", "tools/check_public_docs.py"):
                    command([sys.executable, script])
                print("Current-source required CI and public tree/document checks passed.")
            if criterion == "AC-8":
                fix_replies(evidence)
            revalidate(evidence)
    except Violation as error:
        observation.update(status="violation", violation=VIOLATION)
        print(str(error), file=sys.stderr)
        code = 1
    except (ObservationError, OSError, UnicodeError, ValueError, KeyError, TypeError, AttributeError) as error:
        observation.update(status="error", reason="execution_error")
        print(str(error) if isinstance(error, ObservationError) else type(error).__name__, file=sys.stderr)
        code = 2
    if criterion != "AC-5" and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": "windows-review-triage", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
