#!/usr/bin/env python3
"""Observe stream preservation and live PR evidence; never consume saved success."""
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
ADS_REVIEW = 4265866310
VIOLATION = "windows-stream-boundary-missing"
ISSUE_SECTIONS = ("Review", "Current gap", "Deferral and remaining risk", "Acceptance criteria")
FOLLOWUPS = {
    4265866305: ("tools/windows/package.py", "tools/windows/common.ps1", "CON .txt", "NUL .md", "COM1 .json", "ordinary"),
    4265866315: ("tools/windows/common.ps1", "tools/windows/uninstall.ps1", "managed.json", "owns_path",
                 "schema-valid", "exact", "same-user", "authentication"),
}
DOCS = ("guides/WINDOWS.md", "guides/WINDOWS.en.md", "tools/windows/README.md")
STREAM_SOURCES = tuple("tools/windows/" + name for name in (
    "common.ps1", "install.ps1", "uninstall.ps1", "native_stream_checks.ps1", "stream_cases.json")) + ("guides/WINDOWS.en.md",)


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise Violation(message)


def has_url(text, url):
    # An issue/run number which is only the prefix of another URL is not a link
    # to the reviewed resource. Plain and Markdown canonical URLs both work.
    return re.search(re.escape(url) + r"(?![\w/#?=&%-])", text) is not None


def command(argv, timeout=180):
    try:
        result = subprocess.run(argv, cwd=ROOT, capture_output=True, text=True,
                                encoding="utf-8", errors="strict", timeout=timeout)
    except (OSError, UnicodeError, subprocess.TimeoutExpired) as error:
        raise ObservationError(type(error).__name__) from None
    if result.returncode:
        # Failed request transcripts may contain credentials or local paths.
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


def function_source(source, name):
    match = re.search(r"(?ms)^function " + re.escape(name) + r"\b.*?(?=^function |\Z)", source)
    require(match is not None, "required public product boundary is absent: " + name)
    return match.group(0)


def documentation_boundary():
    for name in DOCS:
        source = (ROOT / name).read_text(encoding="utf-8")
        markers = (r"ADS|stream|스트림", r"Zone\.Identifier", r"preserv|보존",
                   r"input|원본", r"cleanup|정리", r"Windows 11")
        require(all(re.search(marker, source, re.I) for marker in markers),
                "public stream preservation, input or cleanup explanation missing: " + name)


def feature_boundary(criterion):
    # These checks only expose missing baseline implementation. Presence cannot
    # pass a criterion: current-source native CI and the runtime report follow.
    if criterion in {"AC-1", "AC-2", "AC-4", "AC-7"}:
        source = (ROOT / "tools/windows/common.ps1").read_text(encoding="utf-8")
        function_source(source, "Assert-NoNamedStreams")
        require("Assert-NoNamedStreams" in function_source(source, "Assert-TreeInventory"),
                "product inventory has no named-stream boundary")
        if criterion in {"AC-2", "AC-4", "AC-7"}:
            require("Assert-NoNamedStreams" in function_source(source, "Remove-OrdinaryTree"),
                    "product cleanup has no named-stream preservation boundary")
    if criterion in {"AC-4", "AC-7"}:
        require((ROOT / "tools/windows/native_stream_checks.ps1").is_file(),
                "native stream preservation observations are absent")
    if criterion == "AC-5":
        documentation_boundary()


def live():
    ci = load("stream_ci_observer", "tools/verification-checks/cross_platform_ci.py")
    validator = load("stream_native_results", "tools/windows/stream_results.py")
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
    require(set(validator.SOURCE_PATHS) == set(STREAM_SOURCES), "stream source contract is incomplete")
    sources = {name: digest(name) for name in STREAM_SOURCES}
    for report in windows:
        try:
            validator.stream_report(report.get("stream_checks"), report["environment"]["shell_version"], sources)
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


def followup_issues():
    issues = api(f"repos/{REPOSITORY}/issues?state=all&per_page=100", paginated=True)
    rows = comments()
    user = api("user")["login"]
    if not all(isinstance(issue, dict) for issue in issues):
        raise ObservationError("invalid issue response")
    observations = {}
    for review in FOLLOWUPS:
        review_url = f"https://github.com/{REPOSITORY}/pull/{PULL_REQUEST}#discussion_r{review}"
        linked = [issue for issue in issues if not issue.get("pull_request")
                  and has_url(issue.get("body") or "", review_url)]
        replies = [row for row in rows if row.get("in_reply_to_id") == review
                   and row.get("user", {}).get("login") == user]
        observations[review] = (linked, replies)
        print(json.dumps({"review": review, "linked_issues": len(linked), "author_replies": len(replies)}))
    # Both issue/comment sets were obtained live before absence can qualify as
    # a baseline. Missing credentials and failed API calls never qualify.
    linked_numbers = set()
    for review, terms in FOLLOWUPS.items():
        require(sum(row.get("id") == review and not row.get("in_reply_to_id") for row in rows) == 1,
                "target follow-up review is missing or ambiguous")
        linked, replies = observations[review]
        require(len(linked) == 1, "follow-up issue is missing or ambiguous: " + str(review))
        issue = linked[0]
        body = issue.get("body") or ""
        require(all(re.search(r"(?m)^## " + re.escape(section) + r"\s*$", body) for section in ISSUE_SECTIONS)
                and all(term in body for term in terms)
                and all(term in body for term in ("Windows 11", "before public release"))
                and re.search(r"preserv", body, re.I),
                "follow-up lacks the gap, deferral, release boundary or preservation outcomes: " + str(review))
        issue_url = f"https://github.com/{REPOSITORY}/issues/{issue['number']}"
        require(issue.get("html_url") == issue_url, "follow-up issue belongs to another repository")
        require(any(has_url(row.get("body") or "", issue_url)
                    and re.search(r"follow.up|defer|후속|미루", row["body"], re.I)
                    and re.search(r"not implemented|unresolved|미해결|미구현", row["body"], re.I) for row in replies),
                "follow-up review lacks an issue-linked unresolved reply: " + str(review))
        linked_numbers.add(issue["number"])
        print(json.dumps({"review": review, "issue": issue_url, "scope": "follow-up, not implemented"}))
    require(len(linked_numbers) == 2, "the two independent follow-up scopes must have distinct issues")


def ads_reply(evidence):
    rows = comments()
    user = api("user")["login"]
    require(sum(row.get("id") == ADS_REVIEW and not row.get("in_reply_to_id") for row in rows) == 1,
            "ADS review is missing or ambiguous")
    found = [row for row in rows if row.get("in_reply_to_id") == ADS_REVIEW
             and row.get("user", {}).get("login") == user]
    require(any(evidence["head"] in (row.get("body") or "") and has_url(row["body"], evidence["url"])
                and "Windows 11" in row["body"] and re.search(r"ADS|stream|스트림", row["body"], re.I)
                and re.search(r"inject|주입", row["body"], re.I) for row in found),
            "ADS review lacks current-source CI, injection and remaining-environment boundaries")
    print("ADS review links the current commit, observed CI, injection scope and remaining Windows 11 boundary.")


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
        require(criterion in {f"AC-{n}" for n in range(1, 8)}, "unknown criterion")
        feature_boundary(criterion)
        if criterion == "AC-6":
            followup_issues()
        else:
            evidence = live()
            if criterion in {"AC-3", "AC-5"}:
                for script in ("tools/check_public_tree.py", "tools/check_public_docs.py"):
                    command([sys.executable, script])
                print("Current-source required CI and public tree/document checks passed.")
            if criterion == "AC-7":
                ads_reply(evidence)
            revalidate(evidence)
    except Violation as error:
        observation.update(status="violation", violation=VIOLATION)
        print(str(error), file=sys.stderr)
        code = 1
    except (ObservationError, OSError, UnicodeError, ValueError, KeyError, TypeError, AttributeError) as error:
        observation.update(status="error", reason="execution_error")
        print(str(error) if isinstance(error, ObservationError) else type(error).__name__, file=sys.stderr)
        code = 2
    if criterion != "AC-3" and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": "windows-stream-review", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
