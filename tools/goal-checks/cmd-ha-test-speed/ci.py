#!/usr/bin/env python3
"""cmd-ha-test-speed AC-5: native test names kept, and required CI and Release jobs succeeded for HEAD.

Usage, from the repository root:

  python3 tools/goal-checks/cmd-ha-test-speed/ci.py [--static-only]

The static part runs first, before GitHub is read (alone with --static-only).
It reads base_tests.json, renames.json and the cmd/ha test files at HEAD
through inventory.py, and requires:

- every base cmd/ha test whose name starts with TestNative to exist at head
  as a top-level test of the same name;
- no renames.json key starting with TestNative;
- every cmd/ha test that tools/windows/native_checks.py (its NATIVE["cmd/ha"]
  list) or tools/verification-checks/cross_platform_ci.py (its SCENARIOS and
  WINDOWS sets) selects by exact name, now or at the base commit, to exist at
  head, and, when it is a base test, only in files whose constraint (build
  lines and GOOS/GOARCH file suffix) is that of its base file. For a subtest
  path the part before the first `/` is the test. A cross_platform_ci.py name
  is a cmd/ha name when it is a base cmd/ha test or listed in NATIVE["cmd/ha"];
  its other names belong to other packages. No other tools/windows/ file
  selects a cmd/ha test by name.

The base selections are base_tests.json's `native_selections`, which
`inventory.py --write-snapshot` records from the base commit; the static part
reads no commit, only this checkout's files.

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
This file then judges the run live_run admitted (its printed run ID and
attempt) itself: it must still be the latest, completed with conclusion
`success`, and when .github/workflows/ci.yml has a `name: Seal Core` job, that
job must appear exactly once in the attempt, completed with success, every
step completed with success.

The Release side is this file's judgment: the latest pull_request run and
attempt of release.yml for HEAD must be completed with conclusion `success`,
and its `Build release files`, `Windows release archive` and `Release
candidate` jobs must each appear once, completed with success, every step
completed with success. The run is read again afterwards and must still be
the latest with the same attempt, status and conclusion, and the local HEAD
must still be clean and unchanged. guides/CI.en.md names the required checks
`Seal Core`, `Public documentation` and `Release candidate`.

Before live_run, a `gh pr view` that finds no pull request for the branch is
reported as that, since live_run reports every gh failure as an access problem
without printing gh output.

Exit 0: the static part and both runs passed. Exit 1: a native test name or
constraint changed, a TestNative rename is listed, or a run or job is missing,
pending, skipped, cancelled or failed, or the observation changed while it was
read; the reason is printed. Exit 2: base_tests.json (its native_selections
included), renames.json, a Go test file or a selecting file is missing,
malformed or cannot be read, or GitHub, git or the API data could not be read.
Neither is ever a pass.
"""
import argparse
from collections import defaultdict
import contextlib
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]
spec = importlib.util.spec_from_file_location("cmd_ha_test_speed_native_ci",
                                              ROOT / "tools/verification-checks/cross_platform_ci.py")
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)

spec = importlib.util.spec_from_file_location("cmd_ha_test_speed_inventory", Path(__file__).resolve().parent
                                              / "inventory.py")
inventory = importlib.util.module_from_spec(spec)
spec.loader.exec_module(inventory)

REPOSITORY = "jgoneit/jaekit"
RELEASE_JOBS = ("Build release files", "Windows release archive", "Release candidate")
CI_GATE = "Seal Core"
NATIVE_PREFIX = "TestNative"


def static_part():
    """Problems with native cmd/ha test names; InputFailure when an input cannot be read."""
    snapshot, renames = inventory.load_inputs()
    head = inventory.head_files()
    base_sets, head_sets = defaultdict(set), defaultdict(set)
    for row in snapshot["files"]:
        for name in row["tests"]:
            base_sets[name].add((row["build"], inventory.filename_constraint(row["file"])))
    for file, row in head.items():
        for name in row["tests"]:
            head_sets[name].add((row["build"], inventory.filename_constraint(file)))
    problems = []
    for name in sorted(base_sets):
        if name.startswith(NATIVE_PREFIX) and name not in head_sets:
            problems.append(f"native test missing: {name} is not a top-level test at head")
    for old in sorted(renames):
        if old.startswith(NATIVE_PREFIX):
            problems.append(f"native test renamed: renames.json maps {old}; native tests keep their names")

    def at_head(path):
        try:
            data = (inventory.ROOT / path).read_bytes()
        except OSError as error:
            raise inventory.InputFailure(f"{path} cannot be read: {error}") from None
        return inventory.decode(path, data), path

    base = snapshot["native_selections"]
    selected = defaultdict(set)
    for where, (named, unqualified) in (("now", inventory.selections(at_head)),
                                         ("at base", (set(base["named"]), set(base["unqualified"])))):
        # Other unqualified names are tests of other packages (internal/run, internal/record, ...).
        for name in named | {name for name in unqualified if name in base_sets}:
            selected[name].add(where)
    for name in sorted(selected):
        by = " and ".join(sorted(selected[name]))
        if name not in head_sets:
            problems.append(f"selected test missing: {name} (selected by name {by}) is not a top-level test at head")
        elif name in base_sets and head_sets[name] != base_sets[name]:
            problems.append(f"selected test constraint changed: {name} (selected by name {by}) is under "
                            + " and ".join(sorted(inventory.show(key) for key in head_sets[name]))
                            + "; base " + " and ".join(sorted(inventory.show(key) for key in base_sets[name])))
    native = sorted(name for name in base_sets if name.startswith(NATIVE_PREFIX))
    return problems, (f"{len(native)} base TestNative tests, {len(selected)} cmd/ha tests selected by name in "
                      f"{inventory.WINDOWS_SELECTION} or {inventory.CI_SELECTION}")


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


def require_jobs(repo, run, label, names):
    """Each named job appears once in the run's attempt, completed with success, every step too."""
    jobs = ci.api(f"repos/{repo}/actions/runs/{run['id']}/attempts/{run['run_attempt']}/jobs?per_page=100")
    rows = jobs.get("jobs", [])
    ci.require(jobs.get("total_count") == len(rows), f"{label}: truncated job list")
    for name in names:
        found = [job for job in rows if job.get("name") == name]
        ci.require(len(found) == 1, f"{label}: job {name!r} is missing or repeated")
        job = found[0]
        ci.require(job.get("status") == "completed" and job.get("conclusion") == "success",
                   f"{label}: job {name!r} is {job.get('status')}/{job.get('conclusion')}")
        steps = job.get("steps", [])
        ci.require(steps and all(step.get("status") == "completed" and step.get("conclusion") == "success"
                                 for step in steps), f"{label}: job {name!r} has a missing, skipped or failed step")


def release_run(repo, head):
    run = latest_release_run(repo, head)
    label = f"Release run {run['id']} attempt {run.get('run_attempt')}"
    ci.require(run.get("status") == "completed" and run.get("conclusion") == "success",
               f"{label} is {run.get('status')}/{run.get('conclusion')}, not completed/success")
    require_jobs(repo, run, label, RELEASE_JOBS)
    # API reads are not one snapshot: the admitted run must still be the latest.
    latest = latest_release_run(repo, head)
    ci.require(identity(latest) == identity(run), f"{label} changed or was superseded during observation")
    return run


def latest_ci_run(repo, head):
    runs = ci.api(f"repos/{repo}/actions/workflows/ci.yml/runs?event=pull_request&head_sha={head}&per_page=100")
    matches = [run for run in runs.get("workflow_runs", [])
               if run.get("head_sha") == head and run.get("event") == "pull_request"]
    ci.require(matches, f"no CI pull_request run for {head}")
    return max(matches, key=lambda run: (run["id"], run.get("run_attempt", 1)))


def ci_run(repo, head, admitted, workflow):
    """The CI run live_run admitted: still the latest, success, and its Seal Core gate once with success."""
    run = latest_ci_run(repo, head)
    label = f"CI run {run['id']} attempt {run.get('run_attempt')}"
    ci.require((run.get("id"), run.get("run_attempt")) == admitted,
               f"{label} is not the run live_run admitted ({admitted[0]} attempt {admitted[1]})")
    ci.require(run.get("status") == "completed" and run.get("conclusion") == "success",
               f"{label} is {run.get('status')}/{run.get('conclusion')}, not completed/success")
    gate = f"name: {CI_GATE}" in {line.strip() for line in workflow.splitlines()}
    if gate:
        require_jobs(repo, run, label, (CI_GATE,))
    latest = latest_ci_run(repo, head)
    ci.require(identity(latest) == identity(run), f"{label} changed or was superseded during observation")
    return run, gate


def admitted_run(printed):
    """live_run's printed run ID and attempt (its last stdout line is one JSON object)."""
    lines = printed.strip().splitlines()
    try:
        row = json.loads(lines[-1]) if lines else None
    except json.JSONDecodeError:
        row = None
    if not isinstance(row, dict) or not isinstance(row.get("run_id"), int) or not isinstance(row.get("run_attempt"), int):
        raise ci.ObservationError("live_run did not print its admitted run ID and attempt")
    return row["run_id"], row["run_attempt"]


def main():
    parser = argparse.ArgumentParser(description="cmd-ha-test-speed AC-5 check")
    parser.add_argument("--static-only", action="store_true", help="run only the static native-name part")
    args = parser.parse_args()
    try:
        problems, summary = static_part()
    except inventory.InputFailure as error:
        print(f"AC-5: not observed: {error}", file=sys.stderr)
        return 2
    for problem in problems:
        print(problem)
    if problems:
        print(f"AC-5: fail: {len(problems)} native test name problem(s); {summary}")
        return 1
    print(f"AC-5: static part pass: {summary}")
    if args.static_only:
        return 0
    try:
        workflow = ci.configuration_present()
        pull_request_present()
        printed = io.StringIO()
        try:
            with contextlib.redirect_stdout(printed):
                repo, head = ci.live_run()
        finally:
            sys.stdout.write(printed.getvalue())
        ci.require(repo == REPOSITORY, f"repository is {repo}, not {REPOSITORY}")
        ci_admitted, gate = ci_run(repo, head, admitted_run(printed.getvalue()), workflow)
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
    print(json.dumps({"head": head, "ci_run_id": ci_admitted["id"], "ci_run_attempt": ci_admitted["run_attempt"],
                      "ci_gate": CI_GATE if gate else None, "release_run_id": run["id"],
                      "release_run_attempt": run["run_attempt"], "release_url": run.get("html_url"),
                      "release_jobs": list(RELEASE_JOBS)}))
    print("AC-5: pass: native test names kept; CI (Seal Core and Public documentation) and Release "
          "(Release candidate) succeeded for HEAD")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
