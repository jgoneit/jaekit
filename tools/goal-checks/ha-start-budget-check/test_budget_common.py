"""Self-tests of the shared observer helpers of the ha-start-budget-check goal.

Run: python3 -m unittest discover -s tools/goal-checks/ha-start-budget-check -p test_budget_common.py
"""
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest import mock

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
import budget_common


class GoEnv(unittest.TestCase):
    """go_env starts each tool the Go tests run once, before any observation."""

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


if __name__ == "__main__":
    unittest.main()
