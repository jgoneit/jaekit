"""Self-tests of the ha-start-budget-check observers: the shared helpers and how
AC-12's tests report a part they cannot observe.

Run: python3 -m unittest discover -s tools/goal-checks/ha-start-budget-check -p test_budget_common.py
"""
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
sys.path[:0] = [str(HERE), str(HERE.parents[1])]
import budget_common
import observe_skill
import result_observations


def host_tools_that_do_not_start():
    """The host's Go test tools that are missing or fail their no-op, found without go_env."""
    failing = []
    for tool, *args in budget_common.GO_TEST_TOOLS:
        found = shutil.which(tool)
        try:
            started = found is not None and subprocess.run(
                [found, *args], capture_output=True, stdin=subprocess.DEVNULL, timeout=60).returncode == 0
        except (OSError, subprocess.TimeoutExpired):
            started = False
        if not started:
            failing.append(tool)
    return failing


class GoEnv(unittest.TestCase):
    """go_env starts each tool the Go tests run once, before any observation."""

    def setUp(self):
        # Each test replaces at most one tool, so the host's other tools must start for
        # go_env to reach the replaced one.
        failing = host_tools_that_do_not_start()
        if failing:
            self.skipTest(f"the host's {', '.join(failing)} does not start")

    def test_a_tool_on_path_that_cannot_start_is_unobservable(self):
        if os.name == "nt":
            self.skipTest("the stand-in tool is a shell script")
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory) / "bin"
            folder.mkdir()
            broken = folder / "python3"
            broken.write_text("#!/nonexistent/interpreter\n", encoding="utf-8")
            broken.chmod(0o755)
            path = f"{folder}{os.pathsep}{os.environ.get('PATH', '')}"
            with mock.patch.dict(os.environ, {"PATH": path}):
                with self.assertRaises(budget_common.Unavailable) as caught:
                    budget_common.go_env(directory)
            self.assertIn("python3", str(caught.exception))

    def test_a_tool_that_starts_and_fails_is_unobservable(self):
        if os.name == "nt":
            self.skipTest("the stand-in tool is a shell script")
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory) / "bin"
            folder.mkdir()
            failing = folder / "git"
            failing.write_text("#!/bin/sh\nexit 3\n", encoding="utf-8")
            failing.chmod(0o755)
            path = f"{folder}{os.pathsep}{os.environ.get('PATH', '')}"
            with mock.patch.dict(os.environ, {"PATH": path}):
                with self.assertRaises(budget_common.Unavailable) as caught:
                    budget_common.go_env(directory)
            self.assertIn("git", str(caught.exception))

    def test_working_tools_give_the_isolated_environment(self):
        with tempfile.TemporaryDirectory() as directory:
            env = budget_common.go_env(directory)
            self.assertEqual(env["XDG_CONFIG_HOME"], str(Path(directory) / "empty-config"))
            self.assertFalse(any(key.startswith(("GIT_", "HA_")) for key in env))


class HostWithoutTools(unittest.TestCase):
    """The go_env tests skip, rather than fail, on a host whose Go test tools do not start."""

    def test_the_go_env_tests_skip_without_go(self):
        if os.name == "nt":
            self.skipTest("the PATH below holds a symlink")
        with tempfile.TemporaryDirectory() as directory:
            folder = Path(directory) / "bin"
            folder.mkdir()
            (folder / "python3").symlink_to(sys.executable)
            env = {**os.environ, "PATH": str(folder), "PYTHONDONTWRITEBYTECODE": "1"}
            result = subprocess.run([sys.executable, "-B", "-m", "unittest", "-v", "test_budget_common.GoEnv"],
                                    cwd=HERE, env=env, capture_output=True, text=True, timeout=120)
        self.assertEqual(result.returncode, 0, result.stderr[-2000:])
        self.assertIn("OK (skipped=3)", result.stderr)


def ac12_facts():
    """The result facts of every test AC-12's targets file selects, as its producer collects them."""
    config = json.loads((HERE / "AC-12.targets.json").read_text(encoding="utf-8"))
    merged = result_observations.facts()
    for check in config["checks"]:
        suite = unittest.defaultTestLoader.loadTestsFromName(check["test"], observe_skill)
        facts = result_observations.run_suite(suite, io.StringIO())
        for key in ("violations", "errors"):
            merged[key] += facts[key]
    return merged


class AC12Failures(unittest.TestCase):
    """AC-12 never counts a part it cannot observe as a violation, and keeps a violation beside an error."""

    def setUp(self):
        failing = host_tools_that_do_not_start()
        if failing:
            self.skipTest(f"the host's {', '.join(failing)} does not start")

    def test_a_start_that_cannot_be_observed_is_no_violation(self):
        def killed(repo, *extra, env=None):
            raise budget_common.Unavailable("ha ended by signal 9")
        with mock.patch.object(budget_common.Repo, "start", killed):
            facts = ac12_facts()
        self.assertEqual(facts["violations"], [])
        self.assertIn("execution_error", facts["errors"])

    def test_a_refused_helper_check_and_an_unqueried_one_are_both_reported(self):
        real = observe_skill.run

        def helper(args, cwd, env=None, timeout=600, stdin=None):
            operation = args[args.index("--operation") + 1]
            if operation == "budget":
                raise budget_common.Unavailable("check_core.py timed out")
            result = real(args, cwd, env, timeout)
            if operation == "check":
                result.stdout = json.dumps({"compatible": False, "reason": "rules_unsupported"}).encode()
                result.returncode = 1
            return result
        with mock.patch.object(observe_skill, "run", helper):
            facts = ac12_facts()
        self.assertIn("seal-helper-refuses", facts["violations"])
        self.assertIn("execution_error", facts["errors"])


if __name__ == "__main__":
    unittest.main()
