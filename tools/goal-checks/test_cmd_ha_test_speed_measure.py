#!/usr/bin/env python3
"""Tests of cmd-ha-test-speed/measure.py's pull request item (target MeasurementsInPR).

Full verification runs them with the other goal check regressions:

  python3 -m unittest discover -s tools/goal-checks -p 'test_*.py'

No test runs `go test`: the timing runs are replaced, and `gh` is a fake
script on PATH that prints a given output and exits with a given code.
"""
import argparse
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import unittest
from unittest import mock

PATH = Path(__file__).resolve().parent / "cmd-ha-test-speed" / "measure.py"
SPEC = importlib.util.spec_from_file_location("cmd_ha_measure", PATH)
measure = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(measure)

DECLARATION = "tools/goal-checks/cmd-ha-test-speed/AC-1.json"
# A base like base_timing.json's; the tests do not read that file.
BASE = {"runs": [378.66, 379.95, 380.143], "median": 379.95}
# The head median of GOOD_BODY's after line, as if this run measured it.
HEAD = 121.6
# The form of PR #285's description.
GOOD_BODY = (
    "## Measurements\n\n"
    "`go test -count=1 ./cmd/ha`, three runs each on the same machine:\n\n"
    "- before this change: 378.7 s, 380.0 s, 380.1 s (median 380.0 s)\n"
    "- after: 121.6 s, 121.6 s, 116.6 s (median 121.6 s)\n"
)
FAKE_GH = """#!{python}
import os, sys
with open(os.environ["FAKE_GH_LOG"], "a", encoding="utf-8") as log:
    log.write(" ".join(sys.argv[1:]) + "\\n")
if sys.argv[1:] != ["pr", "view", "--json", "body"]:
    sys.exit(64)
with open(os.environ["FAKE_GH_STDOUT"], "rb") as output:
    sys.stdout.buffer.write(output.read())
sys.exit(int(os.environ["FAKE_GH_EXIT"]))
"""


class FakeGh:
    """A PATH holding only a fake gh that prints stdout and exits with code."""

    def __init__(self, folder, stdout=b"", code=0, present=True):
        self.folder = Path(folder)
        self.log = self.folder / "calls.log"
        self.log.write_text("", encoding="utf-8")
        (self.folder / "stdout").write_bytes(stdout)
        bin_dir = self.folder / "bin"
        bin_dir.mkdir()
        if present:
            gh = bin_dir / "gh"
            gh.write_text(FAKE_GH.format(python=sys.executable), encoding="utf-8")
            gh.chmod(gh.stat().st_mode | stat.S_IXUSR)
        self.env = {"PATH": str(bin_dir), "FAKE_GH_LOG": str(self.log),
                    "FAKE_GH_STDOUT": str(self.folder / "stdout"), "FAKE_GH_EXIT": str(code)}

    def calls(self):
        return self.log.read_text(encoding="utf-8").splitlines()


def body_json(body):
    return json.dumps({"body": body}).encode("utf-8")


class SecondsTokenTest(unittest.TestCase):
    def test_rounds_the_recorded_decimal_half_up(self):
        self.assertEqual(measure.seconds_token(378.66), "378.7 s")
        self.assertEqual(measure.seconds_token(379.95), "380.0 s")  # f"{379.95:.1f}" is 379.9
        self.assertEqual(measure.seconds_token(380.143), "380.1 s")
        self.assertEqual(measure.seconds_token(380), "380.0 s")


class PrProblemsTest(unittest.TestCase):
    def test_pr_285_body_has_every_measurement(self):
        self.assertEqual(measure.pr_problems(GOOD_BODY, BASE, HEAD), [])

    def test_crlf_body_has_every_measurement(self):
        self.assertEqual(measure.pr_problems(GOOD_BODY.replace("\n", "\r\n"), BASE, HEAD), [])

    def test_missing_base_measurements(self):
        body = GOOD_BODY.replace("378.7 s", "378.6 s")
        self.assertEqual(measure.pr_problems(body, BASE, HEAD), ["base run 1 `378.7 s` is missing"])
        body = GOOD_BODY.replace("380.0 s", "379.9 s")
        self.assertEqual(measure.pr_problems(body, BASE, HEAD),
                         ["base run 2 `380.0 s` is missing", "base median `380.0 s` is missing"])

    def test_token_boundaries(self):
        for wrong in ("1378.7 s", "0.378.7 s", "378.7 sec", "378.7 s_", "378.7s"):
            with self.subTest(wrong=wrong):
                body = GOOD_BODY.replace("378.7 s", wrong)
                self.assertIn("base run 1 `378.7 s` is missing", measure.pr_problems(body, BASE, HEAD))

    def test_missing_or_malformed_after_line(self):
        for body in (
            GOOD_BODY.replace("- after: 121.6 s, 121.6 s, 116.6 s (median 121.6 s)\n", ""),
            GOOD_BODY.replace("after:", "After:"),
            GOOD_BODY.replace("after:", "thereafter:"),
            GOOD_BODY.replace("121.6 s, 121.6 s, 116.6 s", "121.6 s, 121.6 s"),
            GOOD_BODY.replace("121.6 s, 121.6 s, 116.6 s", "121.6 s,\n121.6 s, 116.6 s"),
            GOOD_BODY.replace("121.6 s, 121.6 s, 116.6 s", "121.6, 121.6, 116.6 s"),
        ):
            with self.subTest(body=body):
                self.assertEqual(measure.pr_problems(body, BASE, HEAD),
                                 ["no line has `after:` followed by three times in seconds"])

    def after(self, times):
        """GOOD_BODY with its after line's three times replaced."""
        return GOOD_BODY.replace("121.6 s, 121.6 s, 116.6 s", times)

    def test_one_slow_run_on_a_median_passing_line(self):
        # The median is 121.6 s; 300.0 s alone is over half the base median (189.975 s).
        self.assertEqual(measure.pr_problems(self.after("121.6 s, 121.6 s, 300.0 s"), BASE, HEAD), [])
        self.assertEqual(measure.pr_problems(self.after("300.0 s, 116.6 s, 121.6 s"), BASE, HEAD), [])

    def test_times_not_measured_on_this_head(self):
        problems = measure.pr_problems(self.after("1 s, 1 s, 1 s"), BASE, 121.0)
        self.assertEqual(len(problems), 1)
        self.assertIn("`after: 1 s, 1 s, 1 s`", problems[0])
        self.assertIn("its median 1 s is not within ±20% of the head median 121.0 s measured now", problems[0])
        self.assertNotIn("over half", problems[0])

    def test_within_twenty_percent_of_the_head(self):
        # 121.0 s ± 24.2 s is 96.8 s to 145.2 s, both ends included.
        for times in ("140.0 s, 140.0 s, 140.0 s", "145.2 s, 145.2 s, 145.2 s", "96.8 s, 96.8 s, 96.8 s",
                      "100 s, 96.8 s, 150.0 s"):
            with self.subTest(times=times):
                self.assertEqual(measure.pr_problems(self.after(times), BASE, 121.0), [])
        for times in ("145.3 s, 145.3 s, 145.3 s", "96.7 s, 96.7 s, 96.7 s", "60 s, 60 s, 121 s"):
            with self.subTest(times=times):
                problems = measure.pr_problems(self.after(times), BASE, 121.0)
                self.assertEqual(len(problems), 1)
                self.assertIn("not within ±20% of the head median 121.0 s", problems[0])

    def test_median_over_half_the_base(self):
        # Half of 379.95 is 189.975; the line is within ±20% of this head, so only the half condition fails.
        problems = measure.pr_problems(self.after("190.0 s, 190.0 s, 116.6 s"), BASE, 190.0)
        self.assertEqual(len(problems), 1)
        self.assertIn("its median 190.0 s is over half the base median (189.975 s)", problems[0])
        self.assertNotIn("±20%", problems[0])
        self.assertEqual(measure.pr_problems(self.after("189.975 s, 189.975 s, 116.6 s"), BASE, 189.975), [])

    def test_both_conditions_named(self):
        problems = measure.pr_problems(self.after("300.0 s, 300.0 s, 300.0 s"), BASE, HEAD)
        self.assertEqual(len(problems), 1)
        self.assertIn("its median 300.0 s is over half the base median (189.975 s)", problems[0])
        self.assertIn("not within ±20% of the head median 121.6 s", problems[0])

    def test_failing_lines_beside_one_that_meets_both(self):
        for extra in ("after: 1 s, 1 s, 1 s\n", "after: 300.1 s, 300.1 s, 300.1 s\n"):
            with self.subTest(extra=extra):
                self.assertEqual(measure.pr_problems(GOOD_BODY + extra, BASE, HEAD), [])
                self.assertEqual(measure.pr_problems(extra + GOOD_BODY, BASE, HEAD), [])

    def test_every_line_named_when_none_meets_both(self):
        body = self.after("1 s, 1 s, 1 s") + "after: 300.1 s, 300.1 s, 300.1 s\n"
        problems = measure.pr_problems(body, BASE, HEAD)
        self.assertEqual(len(problems), 2)
        self.assertIn("`after: 1 s, 1 s, 1 s`: its median 1 s is not within ±20%", problems[0])
        self.assertIn("`after: 300.1 s, 300.1 s, 300.1 s`: its median 300.1 s is over half", problems[1])


class PrBodyTest(unittest.TestCase):
    def read(self, **fake):
        with tempfile.TemporaryDirectory() as folder:
            gh = FakeGh(folder, **fake)
            with mock.patch.dict(os.environ, gh.env):
                try:
                    return measure.pr_body(), gh.calls()
                except measure.Unobserved as error:
                    return error, gh.calls()

    def test_reads_the_body(self):
        body, calls = self.read(stdout=body_json(GOOD_BODY))
        self.assertEqual(body, GOOD_BODY)
        self.assertEqual(calls, ["pr view --json body"])

    def test_input_failures_are_errors(self):
        cases = {
            "no pull request": (dict(stdout=b"", code=1), "execution_error"),
            "gh not on PATH": (dict(present=False), "dependency_missing"),
            "not JSON": (dict(stdout=b"no pull requests found\n"), "execution_error"),
            "not UTF-8": (dict(stdout=b'{"body": "\xff"}'), "execution_error"),
            "body null": (dict(stdout=b'{"body": null}'), "execution_error"),
            "body a number": (dict(stdout=b'{"body": 1}'), "execution_error"),
            "no body": (dict(stdout=b'{"title": "x"}'), "execution_error"),
            "not an object": (dict(stdout=b'["x"]'), "execution_error"),
            "duplicate body": (dict(stdout=b'{"body": "a", "body": "b"}'), "execution_error"),
        }
        for name, (fake, reason) in cases.items():
            with self.subTest(name):
                error, _ = self.read(**fake)
                self.assertIsInstance(error, measure.Unobserved)
                self.assertEqual(error.reason, reason)


class CheckTest(unittest.TestCase):
    """check() end to end, with the timing runs and the go toolchain replaced."""

    def run_check(self, head_times, **fake):
        base = {"commit": "0" * 40, "go_version": "go version fake", "package": "./cmd/ha",
                "cpus": os.cpu_count(), "runs": BASE["runs"], "median": BASE["median"],
                "load": [0.0, 0.0, 0.0], "outcomes": {"TestA": ["pass"] * 3},
                "measured_at": "2026-11-18T00:00:00+00:00"}
        runs = [{"TestA": "pass"}] * 3
        stdout = io.StringIO()
        with tempfile.TemporaryDirectory() as folder:
            gh = FakeGh(folder, **fake)
            report = Path(folder) / "report.json"
            env = dict(gh.env, HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="inv-1",
                       HA_EVIDENCE_DECLARATION_DIGEST="sha256:test")
            with mock.patch.dict(os.environ, env), \
                    mock.patch.object(measure, "read_base", return_value=base), \
                    mock.patch.object(measure, "go_identity", return_value=("go version fake", "x/cmd/ha")), \
                    mock.patch.object(measure, "read_renames", return_value={}), \
                    mock.patch.object(measure, "measure", return_value=(head_times, runs)), \
                    contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(io.StringIO()):
                code = measure.check(argparse.Namespace(declaration=DECLARATION, package="./cmd/ha"))
            observations = json.loads(report.read_text(encoding="utf-8"))["observations"]
            self.stdout = stdout.getvalue()
            return code, observations, gh.calls()

    def test_pass_with_measurements_in_pr(self):
        code, observations, calls = self.run_check([121.6, 121.6, 116.6], stdout=body_json(GOOD_BODY))
        self.assertEqual(code, 0)
        self.assertEqual(observations, [{"target": "HalfTime", "attempt": 1, "status": "pass"},
                                        {"target": "MeasurementsInPR", "attempt": 1, "status": "pass"}])
        self.assertEqual(calls, ["pr view --json body"])

    def test_violation_without_measurements_in_pr(self):
        body = GOOD_BODY.replace("after:", "later:")
        code, observations, _ = self.run_check([121.6, 121.6, 116.6], stdout=body_json(body))
        self.assertEqual(code, 1)
        self.assertEqual(observations[0]["status"], "pass")
        self.assertEqual(observations[1], {"target": "MeasurementsInPR", "attempt": 1, "status": "violation",
                                           "violation": "measurements-not-in-pr"})

    def test_after_line_compared_with_the_head_measured_now(self):
        # GOOD_BODY's after line has median 121.6 s. A run of the same head
        # that measures 130.0 s is within ±20% of it; one that measures 60.0 s
        # is half time too, but the description's times are not from it.
        code, observations, _ = self.run_check([130.0, 125.0, 140.0], stdout=body_json(GOOD_BODY))
        self.assertEqual(code, 0)
        self.assertEqual(observations[1]["status"], "pass")
        code, observations, _ = self.run_check([60.0, 60.0, 60.0], stdout=body_json(GOOD_BODY))
        self.assertEqual(code, 1)
        self.assertEqual(observations, [{"target": "HalfTime", "attempt": 1, "status": "pass"},
                                        {"target": "MeasurementsInPR", "attempt": 1, "status": "violation",
                                         "violation": "measurements-not-in-pr"}])
        self.assertIn("pull request description: `after: 121.6 s, 121.6 s, 116.6 s`: its median 121.6 s is not "
                      "within ±20% of the head median 60.0 s measured now", self.stdout)

    def test_gh_failure_is_an_error(self):
        code, observations, _ = self.run_check([121.6, 121.6, 116.6], code=1)
        self.assertEqual(code, 2)
        self.assertEqual(observations[1], {"target": "MeasurementsInPR", "attempt": 1, "status": "error",
                                           "reason": "execution_error"})
        code, observations, _ = self.run_check([121.6, 121.6, 116.6], present=False)
        self.assertEqual(code, 2)
        self.assertEqual(observations[1]["reason"], "dependency_missing")

    def test_slow_head_skips_the_pr_item(self):
        code, observations, calls = self.run_check([300.0, 300.0, 300.0], stdout=body_json(GOOD_BODY))
        self.assertEqual(code, 1)
        self.assertEqual(observations, [
            {"target": "HalfTime", "attempt": 1, "status": "violation", "violation": "not-half-time"},
            {"target": "MeasurementsInPR", "attempt": 1, "status": "skip", "reason": "not_selected"}])
        self.assertEqual(calls, [])


if __name__ == "__main__":
    unittest.main()
