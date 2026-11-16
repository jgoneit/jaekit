"""Observe AC-10 of goal-check-cleanup on the current HEAD.

The full verification runs on this tree. The latest pull request CI run for
HEAD is admitted by the native CI observer in
tools/verification-checks/cross_platform_ci.py (run and attempt identity,
source commit and tree, native reports). The check-runs of HEAD are read with
tools/check_main_policy.py: the three checks main requires, and the Pages
workflow's "Site test", each the latest GitHub Actions run, completed with
success.

Exit 0 only when everything passes. A failed check exits 1; anything this
script cannot observe (no PR, no network, gh unavailable, an unreadable
answer) exits 2. Neither counts as a pass.
"""
import importlib.util
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
PAGES_CHECK = "Site test"


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


ci = load("cleanup_native_ci", "tools/verification-checks/cross_platform_ci.py")
policy = load("cleanup_main_policy", "tools/check_main_policy.py")


class Failed(Exception):
    pass


def full_verification():
    result = subprocess.run([sys.executable, "tools/verify.py"], cwd=ROOT)
    if result.returncode != 0:
        raise Failed(f"tools/verify.py exited {result.returncode}")


def head_checks(repo, head):
    # filter=all, as tools/check_main_policy.py reads them: the default
    # filter can leave out a queued or running rerun and keep an older success.
    runs = policy.pages(f"repos/{repo}/commits/{head}/check-runs?filter=all", "check_runs")
    results, reasons = policy.check_results(runs, head)
    for row in results:
        print(f"{row['name']}: {'success' if row['satisfied'] else row['reason']}")
    site = [run for run in runs if run.get("name") == PAGES_CHECK and isinstance(run.get("app"), dict)
            and run["app"].get("id") == policy.APP_ID]
    if not site:
        reasons.append(f"{PAGES_CHECK}:missing")
    else:
        if any(type(run.get("id")) is not int for run in site):
            raise policy.EvidenceError("invalid_check_run_id")
        latest = max(site, key=lambda run: run["id"])
        if latest.get("head_sha") != head:
            reasons.append(f"{PAGES_CHECK}:stale_commit")
        elif latest.get("status") != "completed":
            reasons.append(f"{PAGES_CHECK}:not_completed")
        elif latest.get("conclusion") != "success":
            reasons.append(f"{PAGES_CHECK}:not_successful")
        else:
            print(f"{PAGES_CHECK}: success")
    if reasons:
        raise Failed("checks of HEAD not satisfied: " + ", ".join(reasons))


def main():
    try:
        full_verification()
        repo, head = ci.live_run()
        head_checks(repo, head)
    except (Failed, ci.Violation) as error:
        print(f"AC-10 failed: {error}", file=sys.stderr)
        return 1
    except (ci.ObservationError, policy.EvidenceError) as error:
        print(f"AC-10 not observed: {error}", file=sys.stderr)
        return 2
    print("AC-10 passed")
    return 0


if __name__ == "__main__":
    sys.exit(main())
