#!/usr/bin/env python3
"""cmd-ha-test-speed AC-3: cmd/ha tests pass in shuffled orders and under -race.

Usage, from the repository root:

  python3 tools/goal-checks/cmd-ha-test-speed/stability.py [--package PKG]

It runs, in order and stopping at the first failure:

  go test -count=1 -shuffle=<seed> -json ./cmd/ha   for each fixed seed below
  go test -race -count=1 -json ./cmd/ha

`-json` only lets it read the outcome: a run passes when go test exits 0, the
package has exactly one final `pass` event, at least one test passed and no
test failed. A shuffled run must also report its seed (`-test.shuffle N`).
It prints each seed and result. `--package` (default ./cmd/ha) exists to
exercise this check on small packages of this module.

Exit 0: all four runs passed. Exit 1: a run failed a test or the package
(a data race included); the failed tests are printed. Exit 2: a run was not
observed: go is missing or cannot start, GOFLAGS is set, the package does not
build, a run times out, is killed by a signal, writes output that is not go
test JSON or has no final package event. Neither is ever a pass. The go test
children do not inherit the HA_EVIDENCE_* or HA_JUNIT_XML variables.
"""
import argparse
import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]
SEEDS = (1, 42, 20261119)
RUN_TIMEOUT = 3600
HIDDEN = ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST", "HA_JUNIT_XML")


class Failed(Exception):
    pass


class Unobserved(Exception):
    pass


def child_env():
    env = dict(os.environ)
    for name in HIDDEN:
        env.pop(name, None)
    return env


def run(argv, timeout=120):
    try:
        result = subprocess.run(argv, cwd=ROOT, env=child_env(), capture_output=True, timeout=timeout)
    except OSError as error:
        raise Unobserved(f"{argv[0]} could not start: {error}") from None
    except subprocess.TimeoutExpired:
        raise Unobserved(f"{' '.join(argv)} did not finish within {timeout}s") from None
    if result.returncode < 0:
        raise Unobserved(f"{' '.join(argv)} was killed by signal {-result.returncode}")
    return result


def text(argv):
    result = run(argv)
    if result.returncode:
        raise Unobserved(f"{' '.join(argv)} exited {result.returncode}: "
                         + result.stderr.decode("utf-8", "replace").strip()[-400:])
    return result.stdout.decode("utf-8", "replace").strip()


def judge(argv, import_path, seed):
    """Run one go test command; return a one-line summary or raise Failed/Unobserved."""
    result = run(argv, timeout=RUN_TIMEOUT)
    stderr = result.stderr.decode("utf-8", "replace").strip()
    events = []
    try:
        for line in result.stdout.decode("utf-8").splitlines():
            if line.strip():
                event = json.loads(line)
                if not isinstance(event, dict):
                    raise ValueError("not an object")
                events.append(event)
    except (UnicodeDecodeError, ValueError) as error:
        raise Unobserved(f"output is not go test JSON: {error}; {stderr[-400:]}") from None
    if any(event.get("Action") == "build-fail" for event in events):
        raise Unobserved(f"the package does not build: {stderr[-400:]}")
    tests, output = {}, {}
    for event in events:
        if event.get("Package") != import_path:
            continue
        name = event.get("Test")
        if event.get("Action") == "output":
            output.setdefault(name, []).append(event.get("Output", ""))
        elif name and event.get("Action") in ("pass", "fail", "skip"):
            tests[name] = event["Action"]
    final = [event["Action"] for event in events if event.get("Package") == import_path and not event.get("Test")
             and event.get("Action") in ("pass", "fail", "skip")]
    failed = sorted(name for name, action in tests.items() if action == "fail")
    package_output = "".join(output.get(None, []))
    if failed or final == ["fail"]:
        lines = [f"failed tests: {', '.join(failed) or 'none; the package failed'}"]
        for name in failed[:5]:
            lines.append(f"--- {name}:\n" + "".join(output.get(name, []))[-1500:].rstrip())
        if "WARNING: DATA RACE" in package_output:
            lines.append("data race reported outside a test:\n" + package_output[-1500:].rstrip())
        elif not failed:
            lines.append(package_output[-1500:].rstrip())
        raise Failed("\n".join(lines))
    if len(final) != 1 or final[0] != "pass":
        raise Unobserved(f"no single final package pass event (go test exited {result.returncode}); {stderr[-400:]}")
    if result.returncode:
        raise Unobserved(f"go test exited {result.returncode} after a package pass event; {stderr[-400:]}")
    passed = sum(action == "pass" for name, action in tests.items() if "/" not in name)
    if not passed:
        raise Unobserved("no test ran")
    if seed is not None and f"-test.shuffle {seed}\n" not in package_output:
        raise Unobserved(f"the test binary did not report -test.shuffle {seed}")
    elapsed = next(event.get("Elapsed") for event in events if event.get("Package") == import_path
                   and not event.get("Test") and event.get("Action") == "pass")
    return f"pass ({passed} top-level tests passed, package {elapsed}s)"


def main():
    parser = argparse.ArgumentParser(description="cmd-ha-test-speed AC-3 stability check")
    parser.add_argument("--package", default="./cmd/ha", help="package to run (default ./cmd/ha)")
    args = parser.parse_args()
    try:
        flags = text(["go", "env", "GOFLAGS"])
        if flags:
            raise Unobserved(f"GOFLAGS is {flags!r}; it can change which tests run")
        lines = text(["go", "list", "-f", "{{.ImportPath}}", args.package]).splitlines()
        if len(lines) != 1 or not lines[0]:
            raise Unobserved(f"{args.package} is not one package")
        import_path = lines[0]
    except Unobserved as error:
        print(f"AC-3: not observed: {error}", file=sys.stderr)
        return 2
    steps = [(f"seed {seed}", seed, ["go", "test", "-count=1", f"-shuffle={seed}", "-json", args.package])
             for seed in SEEDS]
    steps.append(("race", None, ["go", "test", "-race", "-count=1", "-json", args.package]))
    for label, seed, argv in steps:
        print(f"{label}: {' '.join(argv)}", flush=True)
        try:
            summary = judge(argv, import_path, seed)
        except Failed as error:
            print(f"{label}: FAIL\n{error}")
            print(f"AC-3: fail at {label}")
            return 1
        except Unobserved as error:
            print(f"{label}: not observed: {error}", file=sys.stderr)
            print(f"AC-3: not observed at {label}")
            return 2
        print(f"{label}: {summary}", flush=True)
    print(f"AC-3: pass: seeds {', '.join(map(str, SEEDS))} and -race all passed")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
