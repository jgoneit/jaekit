#!/usr/bin/env python3
"""cmd-ha-test-speed AC-3: cmd/ha tests pass in shuffled orders and under -race.

Usage, from the repository root:

  python3 tools/goal-checks/cmd-ha-test-speed/stability.py [--package PKG] [--base PATH]

Before any test runs it builds tools/goal-checks/cmd-ha-test-speed/parallelstate
and runs it on the package directory. That program reports, statically, a
test that calls t.Parallel() and can reach os.Setenv, os.Unsetenv,
os.Clearenv, os.Chdir, syscall.Setenv, syscall.Unsetenv, syscall.Clearenv,
t.Setenv or t.Chdir (its usage text says what it follows). Then it reads the
base outcomes in base_timing.json (--base, default
tools/goal-checks/cmd-ha-test-speed/base_timing.json, recorded by
measure.py --record-base) and renames.json. Then it runs, in order and
stopping at the first failure:

  go test -count=1 -shuffle=<seed> -json ./cmd/ha   for each fixed seed below
  go test -race -count=1 -json ./cmd/ha

`-json` only lets it read the outcome: a run passes when go test exits 0, the
package has exactly one final `pass` event, at least one test passed and no
test failed. A shuffled run must also report its seed (`-test.shuffle N`).
It prints each seed and result. The four runs must then give the same set of
passing top-level tests, and every top-level test that passed in all base
runs must pass in them, or be mapped by renames.json to names that all pass.
Tests that are new at head may pass too. `--package` (default ./cmd/ha) and
`--base` exist to exercise this check on small packages of this module; the
base must be recorded for the same package.

Exit 0: the static check found nothing, all four runs passed, and they passed
the same tests, base's passing tests among them. Exit 1: the static check
found a process-state change reachable from a parallel test; a run failed a
test or the package (a data race included), and the failed tests are printed;
the runs passed different tests; or a base-passing test is skipped or missing
(each is printed). Exit 2: not observed: go is missing or cannot start, GOFLAGS
is set, the static check could not be built or could not parse the package,
base_timing.json is the placeholder, missing, corrupt or recorded for another
package, renames.json is missing or corrupt, the package does not build, or a
run times out, is killed by a signal, writes output that is not go test JSON
or has no final package event. Neither is ever a pass. The go test children do
not inherit the HA_EVIDENCE_* or HA_JUNIT_XML variables.
"""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent
SEEDS = (1, 42, 20261119)
RUN_TIMEOUT = 3600
STATIC_TIMEOUT = 600
BASE = "tools/goal-checks/cmd-ha-test-speed/base_timing.json"
PARALLELSTATE = "./tools/goal-checks/cmd-ha-test-speed/parallelstate"
HIDDEN = ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST", "HA_JUNIT_XML")

spec = importlib.util.spec_from_file_location("cmd_ha_test_speed_measure", HERE / "measure.py")
measure = importlib.util.module_from_spec(spec)
spec.loader.exec_module(measure)


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


def parallel_state(directory):
    """Build and run the static check; Failed on a finding, Unobserved when it cannot observe."""
    with tempfile.TemporaryDirectory(prefix="cmd-ha-parallelstate-") as temporary:
        binary = str(Path(temporary) / ("parallelstate.exe" if os.name == "nt" else "parallelstate"))
        built = run(["go", "build", "-o", binary, PARALLELSTATE], timeout=STATIC_TIMEOUT)
        if built.returncode:
            raise Unobserved(f"{PARALLELSTATE} does not build: " + built.stderr.decode("utf-8", "replace").strip()[-400:])
        result = run([binary, directory], timeout=STATIC_TIMEOUT)
    output = (result.stdout.decode("utf-8", "replace") + result.stderr.decode("utf-8", "replace")).rstrip()
    if result.returncode == 1:
        raise Failed(output)
    if result.returncode:
        raise Unobserved(f"parallelstate exited {result.returncode}: {output[-1500:]}")
    return output


def judge(argv, import_path, seed):
    """Run one go test command; return (summary, top-level outcomes) or raise Failed/Unobserved."""
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
    top = {name: action for name, action in tests.items() if "/" not in name}
    passed = sum(action == "pass" for action in top.values())
    if not passed:
        raise Unobserved("no test ran")
    if seed is not None and f"-test.shuffle {seed}\n" not in package_output:
        raise Unobserved(f"the test binary did not report -test.shuffle {seed}")
    elapsed = next(event.get("Elapsed") for event in events if event.get("Package") == import_path
                   and not event.get("Test") and event.get("Action") == "pass")
    skipped = sum(action == "skip" for action in top.values())
    return f"pass ({passed} top-level tests passed, {skipped} skipped, package {elapsed}s)", top


def passing_set_differences(labels, runs):
    """Top-level tests that passed in some runs but not all, with each run's outcome."""
    passing = [{name for name, action in top.items() if action == "pass"} for top in runs]
    lines = []
    for name in sorted(set().union(*passing) - set.intersection(*passing)):
        lines.append(f"{name}: " + ", ".join(f"{label} {top.get(name, 'missing')}" for label, top in zip(labels, runs)))
    return lines


def main():
    parser = argparse.ArgumentParser(description="cmd-ha-test-speed AC-3 stability check")
    parser.add_argument("--package", default="./cmd/ha", help="package to run (default ./cmd/ha)")
    parser.add_argument("--base", default=BASE, help=f"base outcomes (default {BASE})")
    args = parser.parse_args()
    if Path.cwd().resolve() != ROOT:
        print(f"run from the repository root {ROOT}", file=sys.stderr)
        return 2
    try:
        flags = text(["go", "env", "GOFLAGS"])
        if flags:
            raise Unobserved(f"GOFLAGS is {flags!r}; it can change which tests run")
        lines = text(["go", "list", "-f", "{{.ImportPath}}\t{{.Dir}}", args.package]).splitlines()
        if len(lines) != 1 or lines[0].count("\t") != 1 or not all(lines[0].split("\t")):
            raise Unobserved(f"{args.package} is not one package")
        import_path, directory = lines[0].split("\t")
        print(f"static: parallelstate {directory}", flush=True)
        print(f"static: {parallel_state(directory)}", flush=True)
        try:
            base = measure.read_base(args.base)
            if base["package"] != args.package:
                raise measure.Unobserved("setup_error", f"base was recorded for {base['package']}, not {args.package}")
            renames = measure.read_renames()
        except measure.Unobserved as error:
            raise Unobserved(f"{error} ({error.reason})") from None
    except Failed as error:
        print(f"static: FAIL\n{error}")
        print("AC-3: fail at the static parallel-state check")
        return 1
    except Unobserved as error:
        print(f"AC-3: not observed: {error}", file=sys.stderr)
        return 2
    steps = [(f"seed {seed}", seed, ["go", "test", "-count=1", f"-shuffle={seed}", "-json", args.package])
             for seed in SEEDS]
    steps.append(("race", None, ["go", "test", "-race", "-count=1", "-json", args.package]))
    runs = []
    for label, seed, argv in steps:
        print(f"{label}: {' '.join(argv)}", flush=True)
        try:
            summary, top = judge(argv, import_path, seed)
        except Failed as error:
            print(f"{label}: FAIL\n{error}")
            print(f"AC-3: fail at {label}")
            return 1
        except Unobserved as error:
            print(f"{label}: not observed: {error}", file=sys.stderr)
            print(f"AC-3: not observed at {label}")
            return 2
        runs.append(top)
        print(f"{label}: {summary}", flush=True)
    labels = [label for label, _, _ in steps]
    differing = passing_set_differences(labels, runs)
    for line in differing:
        print(f"passing set differs: {line}")
    missing = measure.outcome_differences(base["outcomes"], runs, renames)
    for line in missing:
        print(f"base-passing test not passing: {line}")
    if differing or missing:
        print(f"AC-3: fail: {len(differing)} test(s) passed in only some runs, {len(missing)} base-passing test(s) "
              "skipped or missing")
        return 1
    print(f"AC-3: pass: no parallel process-state change found; seeds {', '.join(map(str, SEEDS))} and -race "
          "passed the same tests, base's passing tests among them")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
