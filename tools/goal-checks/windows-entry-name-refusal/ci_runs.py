#!/usr/bin/env python3
"""Goal check: the pull request CI jobs that tested this checkout's tree succeeded.

Usage: python3 ci_runs.py AC-1|AC-3

Reads GitHub Actions through the gh CLI for the commit checked out here.

- The latest CI pull_request run's Windows job prints the tested source
  (`JAEKIT_CI_CONTEXT`, tools/ci/describe_run.py). That commit must have this
  checkout's tree, and its pull request head must be this commit.
- A release run counts only when its pull request has the same head and base
  as that context, so it tested the same merge.
- AC-1 also requires the checked tree's native checks to demand the installer's
  name-character refusals. The Windows job runs them in Windows PowerShell 5.1
  and PowerShell 7.

Exit 0 when every observation step succeeded. Exit 1 when an observation step
failed or the tree does not demand the cases. Exit 2 when nothing was observed:
GitHub or the commit cannot be read, the run is missing or for another tree, or
a job or step is unfinished, cancelled, skipped or stopped in setup.
"""
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import sys
import unittest

ROOT = Path(__file__).resolve().parents[3]
CONTEXT_JOB = ("ci.yml", "Core / Windows")
OBSERVATIONS = {
    ("ci.yml", "Core / Windows"): ("Verify native developer checks on PowerShell 5.1",
                                   "Verify native developer checks on PowerShell 7"),
    ("release.yml", "Windows release archive"): ("Check the archive and public blocks on Windows PowerShell 5.1",
                                                 "Check the archive and public blocks on PowerShell 7"),
}
CRITERIA = {
    "AC-1": [("ci.yml", "Core / Windows")],
    "AC-3": [("ci.yml", "Core / Windows"), ("release.yml", "Windows release archive")],
}
CONTEXT = re.compile(r"JAEKIT_CI_CONTEXT (\{.*\})\s*$")


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


def job_log(repo, job_id):
    return run(["gh", "api", "--method", "GET", "--allow-escape-sequences", f"repos/{repo}/actions/jobs/{job_id}/logs"])


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


def latest_run(repo, workflow, head):
    runs = [item for item in api(f"repos/{repo}/actions/workflows/{workflow}/runs"
                                 f"?event=pull_request&head_sha={head}&per_page=100")["workflow_runs"]
            if item.get("head_sha") == head]
    if not runs:
        raise Unobserved(f"no {workflow} pull_request run for {head}")
    return max(runs, key=lambda item: (item["run_number"], item["run_attempt"]))


def find_job(repo, run, name):
    listed = api(f"repos/{repo}/actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs?per_page=100")["jobs"]
    matches = [job for job in listed if job.get("name") == name]
    if len(matches) != 1 or matches[0].get("status") != "completed":
        raise Unobserved(f"job {name!r} is absent or unfinished in run {run['id']}")
    return matches[0]


def tested_context(repo, job, head, tree):
    """The pull request base of the run whose Windows job tested this tree."""
    contexts = [match.group(1) for line in job_log(repo, job["id"]).splitlines()
                if (match := CONTEXT.search(line))]
    if len(contexts) != 1:
        raise Unobserved(f"expected one tested-source context in job {job['id']}, found {len(contexts)}")
    context = json.loads(contexts[0])
    if context.get("event") != "pull_request" or context.get("head_sha") != head:
        raise Unobserved("the Windows job did not test this commit's pull request")
    if api(f"repos/{repo}/git/commits/{context['source_sha']}")["tree"]["sha"] != tree:
        raise Unobserved("the Windows job tested a merge whose tree differs from this checkout")
    return context["base_sha"]


def outcome(job, steps):
    """'success', 'failure' (an observation step failed) or 'unobserved'."""
    conclusions = {}
    for step in job.get("steps", []):
        if step.get("name") in steps:
            conclusions.setdefault(step["name"], []).append((step.get("status"), step.get("conclusion")))
    observed = []
    for name in steps:
        found = conclusions.get(name, [])
        if len(found) != 1 or found[0][0] != "completed":
            return "unobserved"
        observed.append(found[0][1])
    if "failure" in observed:
        return "failure"
    if all(value == "success" for value in observed) and job.get("conclusion") == "success":
        return "success"
    return "unobserved"


def observe(criterion):
    head = run(["git", "rev-parse", "HEAD"]).strip()
    tree = run(["git", "rev-parse", "HEAD^{tree}"]).strip()
    repo = json.loads(run(["gh", "repo", "view", "--json", "nameWithOwner"]))["nameWithOwner"]
    runs = {workflow: latest_run(repo, workflow, head) for workflow in {workflow for workflow, _ in CRITERIA[criterion]}}
    base = tested_context(repo, find_job(repo, runs[CONTEXT_JOB[0]], CONTEXT_JOB[1]), head, tree)
    results = {}
    for workflow, name in CRITERIA[criterion]:
        pulls = [pull for pull in runs[workflow].get("pull_requests", [])
                 if pull["head"]["sha"] == head and pull["base"]["sha"] == base]
        if len(pulls) != 1:
            raise Unobserved(f"the {workflow} run did not test the same pull request merge")
        results[f"{workflow} / {name}"] = outcome(find_job(repo, runs[workflow], name), OBSERVATIONS[(workflow, name)])
    return results


def main(argv):
    if len(argv) != 2 or argv[1] not in CRITERIA:
        print("usage: ci_runs.py AC-1|AC-3", file=sys.stderr)
        return 2
    try:
        demanded = tree_demands_cases() if argv[1] == "AC-1" else True
        results = observe(argv[1])
    except Unobserved as error:
        print(f"unobserved: {error}", file=sys.stderr)
        return 2
    except (KeyError, TypeError, ValueError) as error:
        print(f"unobserved: unexpected GitHub response: {error!r}", file=sys.stderr)
        return 2
    for name, value in results.items():
        print(f"{name}: {value}")
    if not demanded:
        print("the checked tree does not demand the name-character refusals", file=sys.stderr)
    if not demanded or "failure" in results.values():
        return 1
    return 0 if all(value == "success" for value in results.values()) else 2


if __name__ == "__main__":
    sys.exit(main(sys.argv))
