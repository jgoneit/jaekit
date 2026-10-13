#!/usr/bin/env python3
"""Observe current-source PR CI. No private goal, cached log or supplied success input.

Native CI is necessary for each change condition. Its absence is the declared
baseline violation. Once wired, live current-head execution is also required.
Authentication failures are errors, never evidence of a missing implementation.
"""
from __future__ import annotations

import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
JOBS = {"Core / Ubuntu": "Linux", "Core / macOS": "macOS", "Core / Windows": "Windows"}
REQUIRED = set(JOBS) | {"Seal Core", "Public documentation"}
SCENARIOS = {
    "TestNativeAdmissionFailureHasNoOutputDigest", "TestNativeUnconfirmedShutdownHasNoOutputDigest",
    "TestNativeArgvEnvironmentAndOutcomes", "TestNativeTimeoutStopsDescendantWrites",
    "TestNativeConcurrentProcessesPreserveRecords", "TestNativeLockIsReleasedWhenOwnerExits",
    "TestNativeCleanCRLFStillRequiresCommittedBytes", "TestNativeCoreGitLifecycle",
}
WINDOWS = {"TestNativeJobLayout", "TestNativeBaselineRejectsJunction", "TestNativeWindowsEvidencePaths"}
PACKAGE_CHECKS = {"candidate-manifest-sha256", "candidate-native-version-capabilities",
                  "space-unicode-drive-path", "checksum-refusal", "archive-traversal-refusal",
                  "archive-case-collision-refusal", "archive-file-directory-collision-refusal",
                  "archive-reserved-name-refusal", "archive-stream-name-refusal", "junction-path-refusal"}
INSTALL = {"fresh", "reinstall", "checksum_rejection_preserves", "conflict_rejection_preserves",
           "update", "old_data_cleanup", "uninstall_preserves"}


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(value, message):
    if not value:
        raise Violation(message)


def command(argv, timeout=120, show=False, env=None):
    try:
        result = subprocess.run(argv, cwd=ROOT, text=True, encoding="utf-8", errors="strict",
                                capture_output=True, timeout=timeout, env=env)
    except (OSError, subprocess.TimeoutExpired, UnicodeError) as error:
        raise ObservationError(type(error).__name__) from None
    if result.returncode:
        if argv[0] == "gh":
            # Do not print credentials/debug output from a failing API request.
            raise ObservationError("GitHub request failed; inspect authentication/permission before continuing")
        if show:
            print(result.stdout, end="")
            print(result.stderr, end="", file=sys.stderr)
        raise Violation(f"command failed ({result.returncode}): {' '.join(argv)}")
    if show:
        print(result.stdout, end="")
    return result.stdout


def api(path):
    try:
        return json.loads(command(["gh", "api", "--method", "GET", path]))
    except json.JSONDecodeError:
        raise ObservationError("invalid GitHub JSON") from None


def records(log, prefix):
    found = []
    for line in log.splitlines():
        offset = line.find(prefix + "{")
        if offset >= 0:
            try:
                found.append(json.loads(line[offset + len(prefix):]))
            except json.JSONDecodeError:
                raise Violation("invalid native JSON report") from None
    return found


def configuration_present():
    workflow = (ROOT / ".github/workflows/ci.yml").read_text()
    require(all(f"name: {name}" in workflow for name in REQUIRED), "native CI jobs or aggregate missing")
    require("tools/ci/native_unix.py" in workflow and "tools/windows/native_checks.ps1" in workflow,
            "native runtime entrypoints missing from CI")
    require("needs:" in workflow and "always()" in workflow, "required aggregate wiring missing")
    return workflow


def native_report(report, operating_system, source):
    require(report.get("status") == "passed", "native observation did not pass")
    require(report.get("source_commit") == source, "native source mismatch")
    tests = report.get("native_tests", {})
    expected = SCENARIOS | (WINDOWS if operating_system == "Windows" else set())
    names = {name.rsplit("/", 1)[-1] for name in tests.get("passed", [])}
    require(expected <= names and not tests.get("skipped"), "required native scenario missing or skipped")
    require(all(any(name.endswith("TestNativeCoreGitLifecycle/linked=" + linked) for name in tests.get("passed", []))
                for linked in ("false", "true")), "ordinary/linked worktree scenario missing")
    require(expected <= {name.rsplit("/", 1)[-1] for name in tests.get("required", [])},
            "native scope declaration incomplete")
    env = report.get("environment", {})
    require(env.get("shell_version") and env.get("goarch") and env.get("goos"), "native environment missing")
    require(env.get("goos") == {"Linux": "linux", "macOS": "darwin", "Windows": "windows"}[operating_system],
            "native toolchain OS does not match runner")
    require(env.get("goarch") in ({"arm64", "amd64"} if operating_system == "macOS" else {"amd64"}),
            "unsupported native architecture")
    if operating_system == "Windows":
        require(report.get("schema") == "jaekit-native-windows/v1", "Windows report schema mismatch")
        require(set(report.get("package_checks", [])) == PACKAGE_CHECKS, "Windows candidate package/path checks missing")
        require(env.get("installation_type") in ("Server", "Server Core"), "actual Windows Server environment missing")
        candidate = report.get("candidate", {})
        require(candidate.get("source_commit") == source and candidate.get("capabilities")
                and candidate.get("binary_sha256") and candidate.get("archive_sha256"), "actual Windows candidate identity missing")
        rejection = report.get("server_rejection", {})
        require(all(rejection.get(action, {}).get("rejected") is True and
                    rejection.get(action, {}).get("preserved") is True for action in ("install", "uninstall")),
                "Server install/uninstall preservation not observed")
    else:
        require(report.get("schema") == "jaekit-native-unix/v1", "Unix report schema mismatch")
        installations = report.get("installations", [])
        require({row.get("guide") for row in installations} == {"guides/INSTALL.md", "guides/INSTALL.en.md"},
                "both actual install guides must execute")
        candidate = report.get("candidates", {}).get("current", {})
        require(candidate.get("version") and candidate.get("capabilities") and candidate.get("binary_sha256"),
                "current native candidate identity missing")
        for row in installations:
            require(INSTALL <= set(row.get("checks", [])), "native lifecycle observation incomplete")
            require(row.get("binary_sha256") == candidate["binary_sha256"] and
                    row.get("installed_version") == candidate["version"], "selected installed binary differs")


def live_run():
    repo = json.loads(command(["gh", "repo", "view", "--json", "nameWithOwner"]))["nameWithOwner"]
    head = command(["git", "rev-parse", "HEAD"]).strip()
    pr_info = json.loads(command(["gh", "pr", "view", "--json", "number,headRefOid"]))
    require(pr_info["headRefOid"] == head, "PR does not point at current local HEAD")
    pr = api(f"repos/{repo}/pulls/{pr_info['number']}")
    require(pr["head"]["sha"] == head and pr["state"] == "open", "PR changed during observation")
    runs = api(f"repos/{repo}/actions/workflows/ci.yml/runs?event=pull_request&head_sha={head}&per_page=100")
    matches = [r for r in runs.get("workflow_runs", []) if r.get("head_sha") == head
               and r.get("event") == "pull_request"]
    require(matches, "current-head PR CI run missing")
    run = max(matches, key=lambda item: (item["id"], item.get("run_attempt", 1)))
    require(run.get("status") == "completed" and run.get("conclusion") == "success", "latest PR CI is not successful")
    jobs = api(f"repos/{repo}/actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs?per_page=100")
    rows = jobs.get("jobs", [])
    require(jobs.get("total_count") == len(rows), "truncated job list")
    require(len(rows) == len(REQUIRED) and {j.get("name") for j in rows} == REQUIRED,
            "required jobs missing or ambiguous")
    sources = set()
    for job in rows:
        require(job.get("status") == "completed" and job.get("conclusion") == "success", "required job did not succeed")
        steps = job.get("steps", [])
        require(steps and all(s.get("status") == "completed" and s.get("conclusion") == "success" for s in steps),
                "required job has missing/skipped/failed step")
        if job["name"] not in JOBS:
            continue
        log = command(["gh", "run", "view", str(run["id"]), "--repo", repo,
                       "--attempt", str(run["run_attempt"]), "--job", str(job["id"]), "--log"])
        contexts = records(log, "JAEKIT_CI_CONTEXT ")
        require(len(contexts) == 1, "unique job context missing")
        context = contexts[0]
        source = context.get("source_sha")
        require(context.get("event") == "pull_request" and context.get("head_sha") == head and
                context.get("base_sha") == pr["base"]["sha"], "PR context/head/base mismatch")
        require(context.get("runner_os") == JOBS[job["name"]] and context.get("os") and context.get("arch")
                and context.get("shell"), "actual runner OS/CPU/shell missing")
        require(isinstance(source, str) and re.fullmatch(r"[0-9a-f]{40}", source), "source SHA missing")
        sources.add(source)
        reports = records(log, "JAEKIT_CI_REPORT ")
        require(len(reports) == (2 if JOBS[job["name"]] == "Windows" else 1), "native report missing/duplicate")
        for report in reports:
            native_report(report, JOBS[job["name"]], source)
        if JOBS[job["name"]] == "Windows":
            versions = [str(r["environment"]["shell_version"]) for r in reports]
            require(any(v.startswith("5.1") for v in versions) and any(v.startswith("7.") for v in versions),
                    "both actual PowerShell 5.1 and 7 required")
    require(len(sources) == 1, "OS jobs tested different sources")
    source = sources.pop()
    commit = api(f"repos/{repo}/git/commits/{source}")
    if source != head:
        require({head, pr["base"]["sha"]} <= {row["sha"] for row in commit["parents"]}, "PR merge parent mismatch")
    local_tree = command(["git", "rev-parse", "HEAD^{tree}"]).strip()
    require(commit["tree"]["sha"] == local_tree, "CI source tree differs from current local tree")
    print(json.dumps({"repository": repo, "pull_request": pr_info["number"], "head": head,
                      "base": pr["base"]["sha"], "tested_source": source, "run_id": run["id"],
                      "run_attempt": run["run_attempt"], "url": run["html_url"], "jobs": sorted(REQUIRED)}))
    return repo, head


def maintain(criterion):
    if criterion == "AC-2":
        command([sys.executable, "-m", "unittest", "discover", "-s", "tools/workflow-checks", "-p", "test_*.py"], show=True)
    elif criterion == "AC-9":
        command([sys.executable, "tools/check_public_tree.py"], show=True)
        command([sys.executable, "tools/check_public_docs.py"], show=True)
        command([sys.executable, "-m", "unittest", "discover", "-s", "tools/ci", "-p", "test_*.py"], show=True)
    elif criterion == "AC-11":
        spec = importlib.util.spec_from_file_location("shared_verify", ROOT / "tools/verify.py")
        shared = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(shared)
        env = dict(os.environ)
        env.pop("GOOS", None)
        env.pop("GOARCH", None)
        try:
            env = shared.full_go_environment(env)
        except shared.CheckFailed as error:
            raise ObservationError(str(error)) from None
        command(["go", "test", "-mod=readonly", "-count=1", "./..."], timeout=600, show=True, env=env)
        for directory in ("tools/release", "tools/contract-checks"):
            command([sys.executable, "-m", "unittest", "discover", "-s", directory, "-p", "test_*.py"], timeout=180, show=True)
    else:
        raise ObservationError("unknown maintain criterion")


def main():
    criterion = sys.argv[1]
    structured = criterion in {"AC-1", "AC-3", "AC-4", "AC-5", "AC-6", "AC-7", "AC-8", "AC-10"}
    observation = {"target": criterion, "attempt": 1, "status": "pass"}
    code = 0
    try:
        if structured:
            configuration_present()
            repo, head = live_run()
            if criterion == "AC-8":
                command([sys.executable, "tools/check_main_policy.py", "--repo", repo, "--commit", head], show=True)
            if criterion in {"AC-7", "AC-8", "AC-10"}:
                command([sys.executable, "-m", "unittest", "discover", "-s", "tools/workflow-checks", "-p", "test_*.py"], show=True)
        else:
            maintain(criterion)
    except Violation as error:
        observation.update(status="violation", violation="native-ci-contract-missing")
        print(str(error), file=sys.stderr)
        code = 1
    except ObservationError as error:
        observation.update(status="error", reason="execution_error")
        print(str(error), file=sys.stderr)
        code = 2
    if structured and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": "cross-platform-ci", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
