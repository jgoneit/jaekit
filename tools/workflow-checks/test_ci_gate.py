"""The required Core check must not accept partial or stale job results."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest


ROOT = Path(__file__).resolve().parents[2]


def load(name):
    path = ROOT / "tools/ci" / (name + ".py")
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


gate = load("require_success")
describe = load("describe_run")


class RequiredNativeJobs(unittest.TestCase):
    def results(self):
        return {name: {"result": "success", "outputs": {}} for name in ("go", "macos", "windows")}

    def test_all_three_successful_jobs_pass(self):
        gate.require_success(json.dumps(self.results()))

    def test_every_non_success_is_rejected_for_every_native_job(self):
        for name in ("go", "macos", "windows"):
            for status in ("failure", "cancelled", "skipped", "pending", "queued", "", None, True, "Success"):
                with self.subTest(job=name, status=status):
                    jobs = self.results()
                    jobs[name]["result"] = status
                    with self.assertRaises(ValueError):
                        gate.require_success(json.dumps(jobs))

    def test_missing_extra_or_invalid_jobs_do_not_pass(self):
        for name in ("go", "macos", "windows"):
            jobs = self.results()
            del jobs[name]
            with self.assertRaises(ValueError):
                gate.require_success(json.dumps(jobs))
        jobs = self.results()
        jobs["previous_run"] = {"result": "success"}
        with self.assertRaises(ValueError):
            gate.require_success(json.dumps(jobs))
        for row in ({}, None, "success", []):
            jobs = self.results()
            jobs["macos"] = row
            with self.assertRaises(ValueError):
                gate.require_success(json.dumps(jobs))

    def test_malformed_or_duplicate_input_fails(self):
        for raw in (None, "", "not json", "null", "[]", "{}",
                    '{"go":{"result":"failure","result":"success"},"macos":{"result":"success"},"windows":{"result":"success"}}'):
            with self.subTest(raw=raw), self.assertRaises(ValueError):
                gate.require_success(raw)

    def test_command_exit_propagates_gate_failure(self):
        env = dict(os.environ)
        env.pop("JAEKIT_CI_NEEDS", None)
        command = [sys.executable, str(ROOT / "tools/ci/require_success.py")]
        result = subprocess.run(command, env=env, capture_output=True, text=True)
        self.assertNotEqual(result.returncode, 0)
        self.assertNotIn("PASS:", result.stdout)
        env["JAEKIT_CI_NEEDS"] = json.dumps(self.results())
        self.assertEqual(subprocess.run(command, env=env, capture_output=True).returncode, 0)


class PublicRunContext(unittest.TestCase):
    def setUp(self):
        self.commit = "a" * 40
        self.env = {"GITHUB_SHA": self.commit, "GITHUB_EVENT_NAME": "pull_request",
                    "RUNNER_OS": "macOS", "RUNNER_ARCH": "ARM64", "UNRELATED_SECRET": "never-print"}
        self.event = {"pull_request": {"head": {"sha": "b" * 40}, "base": {"sha": "c" * 40}},
                      "private_unneeded_field": "never-print"}

    def context(self, **changes):
        return describe.context(dict(self.env, **changes), self.event, self.commit,
                                "bash: fixture version", "Darwin", "arm64")

    def test_merge_source_and_pr_head_base_remain_distinct(self):
        result = self.context()
        self.assertEqual(result["source_sha"], self.commit)
        self.assertEqual(result["head_sha"], "b" * 40)
        self.assertEqual(result["base_sha"], "c" * 40)
        self.assertNotIn("never-print", json.dumps(result))

    def test_push_uses_its_own_source_without_claiming_a_pr(self):
        result = self.context(GITHUB_EVENT_NAME="push")
        self.assertEqual(result["head_sha"], self.commit)
        self.assertIsNone(result["base_sha"])

    def test_stale_checkout_missing_metadata_and_wrong_runner_fail(self):
        for changes in ({"GITHUB_SHA": "d" * 40}, {"GITHUB_SHA": None},
                        {"GITHUB_EVENT_NAME": "pull_request_target"},
                        {"RUNNER_OS": "Windows"}, {"RUNNER_ARCH": "X64"}):
            with self.subTest(changes=changes), self.assertRaises(ValueError):
                self.context(**changes)
        self.event = {}
        with self.assertRaises(ValueError):
            self.context()


if __name__ == "__main__":
    unittest.main()
