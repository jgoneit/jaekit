"""Platforms and the end-to-end flow with a real `ha` (AC-34, AC-35).

AC-34 requires the Go test TestNativeStartBudgetAndSkill of ./cmd/ha, with the
subtests named below, in the Windows native check list
(tools/windows/native_checks.py), runs it on this machine, and reads the pull
request CI run of the checked commit through the repository's CI admission
(tools/verification-checks/cross_platform_ci.py live_run): every native job
report (Linux, macOS, both Windows shells) must list each of those tests as
passed. Unix CI runs every TestNative* test; Windows CI runs the list.

AC-35 builds `ha` from the checked tree and walks the three flows of the
criterion in a synthetic repository with temporary user folders.
"""
import importlib.util
import json
import os
from pathlib import Path
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from budget_common import (CHECK_FLAG, DECLARATION_SCHEMA, GOAL, ROOT, Repo, Unavailable, build, capabilities, require,
                           run, temporary)

NATIVE_TEST = "TestNativeStartBudgetAndSkill"
# One subtest per form the test covers on every platform (PLAN 구현 결정 R3, R4, R7).
SUBTESTS = ("xdg_folder", "home_folder", "missing_default", "corrupt_refused", "auto_start", "auto_refused",
            "skill_missing_support", "skill_supported")
NAMES = [NATIVE_TEST] + [f"{NATIVE_TEST}/{s}" for s in SUBTESTS]


def native_list():
    sys.path.insert(0, str(ROOT / "tools/windows"))
    try:
        spec = importlib.util.spec_from_file_location("observed_native_checks", ROOT / "tools/windows/native_checks.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return {package: list(tests) for package, tests in module.NATIVE.items()}
    except Exception as error:
        raise Unavailable(f"tools/windows/native_checks.py cannot be loaded: {error!r}") from None
    finally:
        sys.path.remove(str(ROOT / "tools/windows"))


def ci_module():
    sys.path.insert(0, str(ROOT / "tools/verification-checks"))
    try:
        spec = importlib.util.spec_from_file_location("observed_cross_platform_ci",
                                                      ROOT / "tools/verification-checks/cross_platform_ci.py")
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        return module
    except Exception as error:
        raise Unavailable(f"cross_platform_ci.py cannot be loaded: {error!r}") from None
    finally:
        sys.path.remove(str(ROOT / "tools/verification-checks"))


class Platform(unittest.TestCase):
    """AC-34: the same results on macOS, Linux and the Windows x64 candidate."""

    def test_platforms(self):
        listed = native_list().get("cmd/ha", [])
        missing = [name for name in NAMES if name not in listed]
        require(self, not missing, "platform-check-missing",
                f"Windows native checks do not require {', '.join(missing)}")
        env = {k: v for k, v in os.environ.items() if not k.startswith(("GIT_", "HA_"))}
        env.update(GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
        result = run(["go", "test", "-count=1", "-v", "-run", f"^{NATIVE_TEST}$", "./cmd/ha"], ROOT, env, timeout=900)
        text = (result.stdout + result.stderr).decode(errors="replace")
        problems = []
        if result.returncode:
            if "--- FAIL" not in text:
                raise Unavailable(f"go test exit {result.returncode} without a failed test: {text[-1000:]}")
            problems.append("local run failed: " + "\n".join(l for l in text.splitlines() if "FAIL" in l)[:800])
        passed_here = {line.split("--- PASS: ", 1)[1].split(" ", 1)[0] for line in text.splitlines() if "--- PASS: " in line}
        if not set(NAMES) <= passed_here:
            problems.append(f"local run did not pass {sorted(set(NAMES) - passed_here)}")
        ci = ci_module()
        seen = {}

        def validator(report, operating_system, source):
            passed = {name.rsplit("cmd/ha/", 1)[-1] for name in report.get("native_tests", {}).get("passed", [])}
            seen.setdefault(operating_system, []).append(sorted(set(NAMES) - passed))

        try:
            ci.live_run(native_validator=validator)
        except ci.Violation as error:
            raise Unavailable(f"the CI run of the checked commit was not admitted: {error}") from None
        except ci.ObservationError as error:
            raise Unavailable(f"the CI run of the checked commit could not be read: {error}") from None
        for system in ("Linux", "macOS", "Windows"):
            if system not in seen:
                raise Unavailable(f"no {system} native report was read")
            for absent in seen[system]:
                if absent:
                    problems.append(f"{system} CI report lacks passed {absent}")
        require(self, not problems, "platform-check-missing", "; ".join(problems))


class Flow(unittest.TestCase):
    """AC-35: estimate, start and status with a real ha; the approval path; the skill refusal."""

    def test_end_to_end(self):
        with temporary("jaekit-auto-flow-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            repo = Repo(Path(directory) / "goal", ha, changes=34)
            repo.write_config(json.dumps({"preapproved_runs": 120}))
            code, report, text = repo.estimate()
            auto = (report or {}).get("auto") or {}
            require(self, code == 0 and auto.get("computed_runs") == 102 and auto.get("approval_needed") is False,
                    "auto-flow-missing", f"estimate with a limit of 120: exit {code}, auto {auto}: {text[-600:]}")
            problems = []
            code, out, err = repo.start()
            _, status = repo.status() if code == 0 else (None, {})
            if code != 0 or (status.get("budget") or {}).get("runs_limit") != 102:
                problems.append(f"start with a limit of 120: exit {code}, runs_limit {(status.get('budget') or {}).get('runs_limit')}: {err[-300:]}")
            # The same goal without a user configuration needs the user's approval and records nothing.
            fresh = Repo(Path(directory) / "fresh", ha, changes=34)
            code, report, text = fresh.estimate()
            if code != 1 or ((report or {}).get("auto") or {}).get("approval_needed") is not True:
                problems.append(f"estimate without configuration: exit {code}: {text[-300:]}")
            code, out, err = fresh.start()
            if code == 0 or "approval" not in err or fresh.records_path().exists():
                problems.append(f"start without configuration: exit {code}, records {fresh.records_path().exists()}: {err[-300:]}")
            # A skill declaring a feature this ha lacks is refused by name; one declaring only supported items starts.
            caps = capabilities(ha, directory)
            skills = Repo(Path(directory) / "skills", ha, changes=1, runs="60")
            unknown = skills.skill("unknown", {"schema": DECLARATION_SCHEMA, "features": ["future-thing/v1"]})
            code, out, err = skills.start("--skill", str(unknown), CHECK_FLAG)
            if code == 0 or "future-thing/v1" not in err or skills.records_path().exists():
                problems.append(f"unsupported feature: exit {code}: {err[-300:]}")
            skills.reset_records()
            known = skills.skill("known", {"schema": DECLARATION_SCHEMA, "features": caps.get("features", [])})
            code, out, err = skills.start("--skill", str(known), CHECK_FLAG)
            if code != 0 or len(skills.records()) != 1:
                problems.append(f"supported features: exit {code}: {err[-300:]}")
        require(self, not problems, "auto-flow-missing", "; ".join(problems))


if __name__ == "__main__":
    unittest.main()
