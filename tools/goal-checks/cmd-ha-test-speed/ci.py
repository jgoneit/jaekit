#!/usr/bin/env python3
"""cmd-ha-test-speed AC-5: required CI and Release jobs succeeded for HEAD.

Usage, from the repository root: python3 tools/goal-checks/cmd-ha-test-speed/ci.py

The CI side is the native admission of
tools/verification-checks/cross_platform_ci.py, reused as
tools/history-checks/ci_observer.py reuses it: `configuration_present()` and
`live_run()`. That ties the open pull request's head to the local HEAD, takes
the latest pull_request run and attempt of ci.yml for it, requires exactly the
jobs `Core / Ubuntu`, `Core / macOS`, `Core / Windows PowerShell 5.1`,
`Core / Windows PowerShell 7`, `Seal Core` and `Public documentation`, each
completed with success and no skipped or failed step, reads every native job's
context and reports, ties the tested source and its tree to the local tree,
and reads the pull request, the run and the local checkout again at the end.

The Release side is this file's judgment: the latest pull_request run and
attempt of release.yml for HEAD must be completed, and its `Build release
files` and `Windows release archive` jobs must each appear once, completed
with success, every step completed with success. The run is read again
afterwards and must still be the latest with the same attempt, status and
conclusion, and the local HEAD must still be clean and unchanged.

Before live_run, a `gh pr view` that finds no pull request for the branch is
reported as that, since live_run reports every gh failure as an access problem
without printing gh output.

Exit 0: both runs passed. Exit 1: a run or job is missing, pending, skipped,
cancelled or failed, or the observation changed while it was read; the reason
is printed. Exit 2: GitHub, git or the API data could not be read. Neither is
ever a pass.
"""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location("cmd_ha_test_speed_native_ci",
                                              ROOT / "tools/verification-checks/cross_platform_ci.py")
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)

REPOSITORY = "jgoneit/jaekit"
RELEASE_JOBS = ("Build release files", "Windows release archive")


def pull_request_present():
    """Name a missing pull request; live_run reports every gh failure as an access problem."""
    try:
        result = subprocess.run(["gh", "pr", "view", "--json", "number"], cwd=ROOT, capture_output=True,
                                text=True, encoding="utf-8", errors="replace", timeout=120)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise ci.ObservationError(f"gh could not run: {type(error).__name__}") from None
    if result.returncode and "no pull requests found" in result.stderr:
        raise ci.Violation("no pull request for the current branch, so HEAD has no pull_request CI or Release run")


def latest_release_run(repo, head):
    runs = ci.api(f"repos/{repo}/actions/workflows/release.yml/runs?event=pull_request&head_sha={head}&per_page=100")
    matches = [run for run in runs.get("workflow_runs", [])
               if run.get("head_sha") == head and run.get("event") == "pull_request"]
    ci.require(matches, f"no Release pull_request run for {head}")
    return max(matches, key=lambda run: (run["id"], run.get("run_attempt", 1)))


def identity(run):
    return run.get("id"), run.get("run_attempt"), run.get("status"), run.get("conclusion")


def release_run(repo, head):
    run = latest_release_run(repo, head)
    label = f"Release run {run['id']} attempt {run.get('run_attempt')}"
    ci.require(run.get("status") == "completed", f"{label} is {run.get('status')}, not completed")
    jobs = ci.api(f"repos/{repo}/actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs?per_page=100")
    rows = jobs.get("jobs", [])
    ci.require(jobs.get("total_count") == len(rows), f"{label}: truncated job list")
    for name in RELEASE_JOBS:
        found = [job for job in rows if job.get("name") == name]
        ci.require(len(found) == 1, f"{label}: job {name!r} is missing or repeated")
        job = found[0]
        ci.require(job.get("status") == "completed" and job.get("conclusion") == "success",
                   f"{label}: job {name!r} is {job.get('status')}/{job.get('conclusion')}")
        steps = job.get("steps", [])
        ci.require(steps and all(step.get("status") == "completed" and step.get("conclusion") == "success"
                                 for step in steps), f"{label}: job {name!r} has a missing, skipped or failed step")
    # API reads are not one snapshot: the admitted run must still be the latest.
    latest = latest_release_run(repo, head)
    ci.require(identity(latest) == identity(run), f"{label} changed or was superseded during observation")
    return run


def main():
    try:
        ci.configuration_present()
        pull_request_present()
        repo, head = ci.live_run()
        ci.require(repo == REPOSITORY, f"repository is {repo}, not {REPOSITORY}")
        run = release_run(repo, head)
        ci.require(ci.command(["git", "rev-parse", "HEAD"]).strip() == head
                   and not ci.command(["git", "status", "--porcelain"]).strip(),
                   "local source changed during observation")
    except ci.Violation as error:
        print(f"AC-5: fail: {error}", file=sys.stderr)
        return 1
    except ci.ObservationError as error:
        print(f"AC-5: not observed: {error}", file=sys.stderr)
        return 2
    except (KeyError, TypeError, ValueError, AttributeError, OSError) as error:
        print(f"AC-5: not observed: unexpected GitHub or local data: {type(error).__name__}: {error}", file=sys.stderr)
        return 2
    print(json.dumps({"head": head, "release_run_id": run["id"], "release_run_attempt": run["run_attempt"],
                      "release_url": run.get("html_url"), "release_jobs": list(RELEASE_JOBS)}))
    print("AC-5: pass: CI (Seal Core jobs and Public documentation) and Release jobs succeeded for HEAD")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
