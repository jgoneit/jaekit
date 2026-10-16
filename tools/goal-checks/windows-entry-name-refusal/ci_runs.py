#!/usr/bin/env python3
"""Goal check: the pull request CI jobs that ran on this checkout's tree succeeded.

Usage: python3 ci_runs.py AC-1|AC-3

Reads GitHub Actions through the gh CLI for the commit checked out here. The
pull request's test merge commit must have this checkout's tree, so the jobs
ran on these files. AC-1 also requires the checked tree's native checks to
demand the installer's name-character refusals, which the Windows job then runs
in Windows PowerShell 5.1 and PowerShell 7.

Exit 0 when every named job of the latest pull_request run completed with
success, 1 when one completed otherwise or the tree does not demand the cases,
2 when the commit, pull request, run, job or GitHub cannot be read or a job has
not finished. Exit 2 is not an observation of the jobs.
"""
import importlib.util
import json
from pathlib import Path
import subprocess
import sys
import unittest

ROOT = Path(__file__).resolve().parents[3]
JOBS = {
    "AC-1": {"ci.yml": ["Core / Windows"]},
    "AC-3": {"ci.yml": ["Core / Windows"], "release.yml": ["Build release files", "Windows release archive"]},
}


class Unobserved(Exception):
    pass


def run(argv):
    try:
        process = subprocess.run(argv, cwd=ROOT, capture_output=True, text=True, timeout=120)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Unobserved(f"{argv[0]} could not run: {error}")
    if process.returncode:
        raise Unobserved(f"{' '.join(argv[:3])} exited {process.returncode}: {process.stderr.strip()[-300:]}")
    return process.stdout


def api(path):
    return json.loads(run(["gh", "api", "--method", "GET", path]))


def tree_demands_cases():
    """Run the name-character regressions of the checked tree in this process."""
    sys.path.insert(0, str(ROOT / "tools/windows"))
    spec = importlib.util.spec_from_file_location("name_refusals", ROOT / "tools/windows/test_name_character_refusals.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    result = unittest.TextTestRunner(stream=sys.stderr, verbosity=1).run(
        unittest.defaultTestLoader.loadTestsFromModule(module))
    if result.errors:
        raise Unobserved("the name-character regressions could not run")
    return result.wasSuccessful()


def jobs(criterion):
    head = run(["git", "rev-parse", "HEAD"]).strip()
    tree = run(["git", "rev-parse", "HEAD^{tree}"]).strip()
    repo = json.loads(run(["gh", "repo", "view", "--json", "nameWithOwner"]))["nameWithOwner"]
    pulls = [pull for pull in api(f"repos/{repo}/commits/{head}/pulls")
             if pull.get("state") == "open" and pull["head"]["sha"] == head]
    if len(pulls) != 1:
        raise Unobserved(f"expected one open pull request whose head is {head}, found {len(pulls)}")
    merge = api(f"repos/{repo}/pulls/{pulls[0]['number']}").get("merge_commit_sha")
    if not merge or api(f"repos/{repo}/git/commits/{merge}")["tree"]["sha"] != tree:
        raise Unobserved("the pull request's test merge does not have this checkout's tree")
    outcomes = {}
    for workflow, names in JOBS[criterion].items():
        runs = [item for item in api(f"repos/{repo}/actions/workflows/{workflow}/runs"
                                     f"?event=pull_request&head_sha={head}&per_page=100")["workflow_runs"]
                if item.get("head_sha") == head]
        if not runs:
            raise Unobserved(f"no {workflow} pull_request run for {head}")
        latest = max(runs, key=lambda item: (item["run_number"], item["run_attempt"]))
        listed = api(f"repos/{repo}/actions/runs/{latest['id']}/attempts/{latest['run_attempt']}/jobs?per_page=100")["jobs"]
        for name in names:
            matches = [job for job in listed if job.get("name") == name]
            if len(matches) != 1 or matches[0].get("status") != "completed":
                raise Unobserved(f"{workflow} job {name!r} is absent or unfinished in run {latest['id']}")
            outcomes[f"{workflow} / {name}"] = matches[0].get("conclusion")
    return outcomes


def main(argv):
    if len(argv) != 2 or argv[1] not in JOBS:
        print("usage: ci_runs.py AC-1|AC-3", file=sys.stderr)
        return 2
    try:
        demanded = tree_demands_cases() if argv[1] == "AC-1" else True
        outcomes = jobs(argv[1])
    except Unobserved as error:
        print(f"unobserved: {error}", file=sys.stderr)
        return 2
    except (KeyError, TypeError, ValueError) as error:
        print(f"unobserved: unexpected GitHub response: {error!r}", file=sys.stderr)
        return 2
    for name, conclusion in outcomes.items():
        print(f"{name}: {conclusion}")
    if not demanded:
        print("the checked tree does not demand the name-character refusals", file=sys.stderr)
    return 0 if demanded and all(value == "success" for value in outcomes.values()) else 1


if __name__ == "__main__":
    sys.exit(main(sys.argv))
