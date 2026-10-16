"""Source binding and outcome meaning of the windows-entry-name-refusal CI reader."""
import contextlib
import copy
import importlib.util
import io
import json
from pathlib import Path
import unittest
from unittest import mock

PATH = Path(__file__).resolve().parent / "windows-entry-name-refusal" / "ci_runs.py"
spec = importlib.util.spec_from_file_location("entry_name_ci_runs", PATH)
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)

REPO, HEAD, TREE, BASE, MERGE = "owner/repo", "a" * 40, "b" * 40, "c" * 40, "d" * 40


def job(job_id, name, steps, conclusion="success"):
    return {"id": job_id, "name": name, "status": "completed", "conclusion": conclusion,
            "steps": [{"name": "Set up job", "status": "completed", "conclusion": "success"},
                      *({"name": step, "status": "completed", "conclusion": "success"} for step in steps)]}


class World:
    """A pull request whose CI and release runs tested this checkout's tree."""

    def __init__(self):
        pull = {"head": {"sha": HEAD}, "base": {"sha": BASE}}
        self.runs = {
            "ci.yml": {"id": 1, "run_number": 5, "run_attempt": 1, "head_sha": HEAD, "pull_requests": [pull]},
            "release.yml": {"id": 2, "run_number": 7, "run_attempt": 1, "head_sha": HEAD, "pull_requests": [copy.deepcopy(pull)]},
        }
        self.jobs = {1: [job(10, "Core / Windows", ci.OBSERVATIONS[("ci.yml", "Core / Windows")])],
                     2: [job(20, "Windows release archive", ci.OBSERVATIONS[("release.yml", "Windows release archive")])]}
        self.context = {"event": "pull_request", "head_sha": HEAD, "base_sha": BASE, "source_sha": MERGE}
        self.reports = [{"status": "passed", "environment": {"shell_version": version},
                         "package_checks": ["checksum-refusal", ci.CASE], "failures": []}
                        for version in ("5.1.26100.1", "7.5.3")]
        self.trees = {MERGE: TREE}
        self.log_lines = None

    def run(self, argv):
        if argv[:2] == ["git", "rev-parse"]:
            return {"HEAD": HEAD, "HEAD^{tree}": TREE}[argv[2]] + "\n"
        if argv[:3] == ["gh", "repo", "view"]:
            return json.dumps({"nameWithOwner": REPO})
        raise AssertionError(argv)

    def api(self, path):
        if "/actions/workflows/" in path:
            workflow = path.split("/actions/workflows/")[1].split("/")[0]
            return {"workflow_runs": [self.runs[workflow]] if workflow in self.runs else []}
        if "/actions/runs/" in path:
            return {"jobs": self.jobs[int(path.split("/actions/runs/")[1].split("/")[0])]}
        if "/git/commits/" in path:
            return {"tree": {"sha": self.trees[path.rsplit("/", 1)[1]]}}
        raise AssertionError(path)

    def job_log(self, repo, job_id):
        lines = self.log_lines if self.log_lines is not None else [
            "2026-01-01T00:00:00Z python tools/ci/describe_run.py --shell powershell",
            "2026-01-01T00:00:01Z JAEKIT_CI_CONTEXT " + json.dumps(self.context),
            *("2026-01-01T00:00:02Z JAEKIT_CI_REPORT " + json.dumps(report) for report in self.reports)]
        return "\n".join(lines) + "\n"

    def main(self, criterion, demanded=True):
        with mock.patch.object(ci, "run", self.run), mock.patch.object(ci, "api", self.api), \
                mock.patch.object(ci, "job_log", self.job_log), \
                mock.patch.object(ci, "tree_demands_cases", lambda: demanded), \
                contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return ci.main(["ci_runs.py", criterion])


def step(world, run_id, name):
    return next(item for item in world.jobs[run_id][0]["steps"] if item["name"] == name)


class CiRuns(unittest.TestCase):
    def test_successful_observation_steps_on_this_tree_pass(self):
        for criterion in ("AC-1", "AC-3"):
            with self.subTest(criterion=criterion):
                self.assertEqual(World().main(criterion), 0)

    def test_a_named_refusal_failure_in_a_shell_report_is_a_failure(self):
        for index, failure in ((0, "name character 0x00: Expected refusal was not observed: Unsafe package path"),
                               (1, "installer name-character refusals are incomplete; missing: 0x22")):
            world = World()
            world.reports[index].update(status="failed", failures=["package/runtime report contains a failure", failure])
            name = ci.OBSERVATIONS[("ci.yml", "Core / Windows")][index]
            step(world, 1, name)["conclusion"] = "failure"
            world.jobs[1][0]["conclusion"] = "failure"
            with self.subTest(failure=failure):
                self.assertEqual(world.main("AC-1"), 1)

    def test_tree_that_does_not_demand_the_cases_is_a_failure(self):
        self.assertEqual(World().main("AC-1", demanded=False), 1)

    def test_runs_that_observed_nothing_or_another_tree_are_unobserved(self):
        def cancelled(world):
            step(world, 1, "Verify native developer checks on PowerShell 7")["conclusion"] = "cancelled"
            world.jobs[1][0]["conclusion"] = "cancelled"

        def setup_failed(world):
            world.jobs[1][0]["steps"][0]["conclusion"] = "failure"
            for item in world.jobs[1][0]["steps"][1:]:
                item.update(conclusion="skipped")
            world.jobs[1][0]["conclusion"] = "failure"

        def post_step_failed(world):
            world.jobs[2][0]["conclusion"] = "failure"

        def build_stopped(world):
            world.reports[0].update(status="failed", failures=["candidate build timed out"])
            step(world, 1, "Verify native developer checks on PowerShell 5.1")["conclusion"] = "failure"
            world.jobs[1][0]["conclusion"] = "failure"

        def release_check_failed(world):
            step(world, 2, "Check the archive and public blocks on PowerShell 7")["conclusion"] = "failure"
            world.jobs[2][0]["conclusion"] = "failure"

        cases = {
            "cancelled observation step": cancelled,
            "setup failure before the checks": setup_failed,
            "failure outside the observation steps": post_step_failed,
            "unfinished job": lambda world: world.jobs[1][0].update(status="in_progress"),
            "missing observation step": lambda world: world.jobs[2][0]["steps"].pop(),
            "no run for this commit": lambda world: world.runs.pop("ci.yml"),
            "tested merge has another tree": lambda world: world.trees.update({MERGE: "e" * 40}),
            "context for another head": lambda world: world.context.update(head_sha="f" * 40),
            "context from a push": lambda world: world.context.update(event="push"),
            "no tested-source context": lambda world: setattr(world, "log_lines", ["no context"]),
            "two tested-source contexts": lambda world: setattr(world, "log_lines", [
                "JAEKIT_CI_CONTEXT " + json.dumps(world.context)] * 2),
            "release run tested another base": lambda world: world.runs["release.yml"]["pull_requests"][0]["base"].update(sha="9" * 40),
            "native step stopped before the cases ran": build_stopped,
            "release check failed for an unattributed reason": release_check_failed,
        }
        for label, change in cases.items():
            world = World()
            change(world)
            with self.subTest(case=label):
                self.assertEqual(world.main("AC-3"), 2)
        ac1 = {
            "native step stopped before the cases ran": build_stopped,
            "no report for PowerShell 7": lambda world: world.reports.pop(),
            "report without the case": lambda world: world.reports[1].update(package_checks=["checksum-refusal"]),
            "two reports for one shell": lambda world: world.reports.append(copy.deepcopy(world.reports[0])),
        }
        for label, change in ac1.items():
            world = World()
            change(world)
            with self.subTest(criterion="AC-1", case=label):
                self.assertEqual(world.main("AC-1"), 2)


if __name__ == "__main__":
    unittest.main()
