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

Each native check step prints `JAEKIT_CI_REPORT` (tools/windows/native_checks.py)
with its shell version, package checks and failures.

Exit 0 when every observation step succeeded and, for AC-1, both shells'
reports passed with the name-character case. Exit 1 only for an observed
failure of the property: the tree does not demand the cases, or a shell's
report names a failed refusal of a built name-character case. A refusal with
another message is such a failure: the installer must refuse with its own name
message. Exit 2 when it was not observed:
GitHub or the commit cannot be read, the run is missing or tested another tree,
a step is unfinished, cancelled or skipped, or a step failed for a reason this
reader cannot attribute to the property, such as a build that stopped before
the cases ran or a case that could not be built.
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
REPORT = re.compile(r"JAEKIT_CI_REPORT (\{.*\})\s*$")
CASE = "archive-name-character-refusal"
# Failure texts that only an installer outcome of a built name-character case
# produces: the package check's "refusal" label (not its "fixture" label, which
# means the case could not be built) and the report validator's message.
REFUSAL_FAILURE = re.compile(r"name character 0x[0-9a-f]{2} refusal:|installer name-character refusals are incomplete")
SHELLS = {"Windows PowerShell 5.1": re.compile(r"5\.1\."), "PowerShell 7": re.compile(r"7\.")}


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


def lines_matching(log, pattern):
    return [match.group(1) for line in log.splitlines() if (match := pattern.search(line))]


def tested_context(log, head, tree, repo, job):
    """The pull request base of the run whose Windows job tested this tree."""
    contexts = lines_matching(log, CONTEXT)
    if len(contexts) != 1:
        raise Unobserved(f"expected one tested-source context in job {job['id']}, found {len(contexts)}")
    context = json.loads(contexts[0])
    if context.get("event") != "pull_request" or context.get("head_sha") != head:
        raise Unobserved("the Windows job did not test this commit's pull request")
    if api(f"repos/{repo}/git/commits/{context['source_sha']}")["tree"]["sha"] != tree:
        raise Unobserved("the Windows job tested a merge whose tree differs from this checkout")
    return context["base_sha"]


def refusal_outcome(log):
    """AC-1 in each shell's native check report: 'success', 'failure' or 'unobserved'."""
    reports = [json.loads(text) for text in lines_matching(log, REPORT)]
    outcomes = []
    for pattern in SHELLS.values():
        found = [report for report in reports
                 if pattern.match(str(report.get("environment", {}).get("shell_version", "")))]
        if len(found) != 1:
            return "unobserved"
        report = found[0]
        if any(REFUSAL_FAILURE.search(str(failure)) for failure in report.get("failures", [])):
            return "failure"
        outcomes.append(report.get("status") == "passed" and CASE in report.get("package_checks", []))
    return "success" if all(outcomes) else "unobserved"


def outcome(job, steps):
    """'success' when every observation step succeeded, otherwise 'unobserved'."""
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
    if all(value == "success" for value in observed) and job.get("conclusion") == "success":
        return "success"
    return "unobserved"


def observe(criterion):
    head = run(["git", "rev-parse", "HEAD"]).strip()
    tree = run(["git", "rev-parse", "HEAD^{tree}"]).strip()
    repo = json.loads(run(["gh", "repo", "view", "--json", "nameWithOwner"]))["nameWithOwner"]
    runs = {workflow: latest_run(repo, workflow, head) for workflow in {workflow for workflow, _ in CRITERIA[criterion]}}
    windows = find_job(repo, runs[CONTEXT_JOB[0]], CONTEXT_JOB[1])
    log = job_log(repo, windows["id"])
    base = tested_context(log, head, tree, repo, windows)
    results = {}
    for workflow, name in CRITERIA[criterion]:
        pulls = [pull for pull in runs[workflow].get("pull_requests", [])
                 if pull["head"]["sha"] == head and pull["base"]["sha"] == base]
        if len(pulls) != 1:
            raise Unobserved(f"the {workflow} run did not test the same pull request merge")
        results[f"{workflow} / {name}"] = outcome(find_job(repo, runs[workflow], name), OBSERVATIONS[(workflow, name)])
    if criterion == "AC-1":
        refusals = refusal_outcome(log)
        steps = results[f"{CONTEXT_JOB[0]} / {CONTEXT_JOB[1]}"]
        results["name-character refusals"] = refusals if refusals == "failure" or steps == "success" else "unobserved"
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
