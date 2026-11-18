#!/usr/bin/env python3
"""cmd-ha-test-speed AC-1 producer: check-result/v1, targets HalfTime and MeasurementsInPR.

Check, from the repository root:

  python3 tools/goal-checks/cmd-ha-test-speed/measure.py tools/goal-checks/cmd-ha-test-speed/AC-1.json

It runs `go test -count=1 -json ./cmd/ha` three times and reads each run's
`Elapsed` from the final package `pass` event of github.com/jgoneit/jaekit/cmd/ha.
The target passes when the median of the three is at most half the median in
base_timing.json; otherwise it is the violation `not-half-time`. It prints the
three times, the base median, the ratio and the 1- and 5-minute load averages.

The times are compared only when the same tests ran. Every top-level test
that passed in all three base runs (base_timing.json `outcomes`) must pass in
all three head runs, or be mapped by renames.json (in this folder) to names
that all pass in all three head runs. Otherwise the result is `error`
(execution_error), not `pass` or `violation`, and each such test is printed
with its head outcomes (pass, skip, or missing).

After a HalfTime pass, and only then, it reads this branch's pull request
description with `gh pr view --json body` (target MeasurementsInPR). The
description passes when it contains both of these, as fixed tokens:

- Every value of base_timing.json's `runs` and its `median`, as a person
  copies it from the file: the recorded decimal rounded half up to one
  decimal, then a space and `s`. 378.66 is `378.7 s`, 379.95 is `380.0 s`
  (not Python's `379.9`), 380.143 is `380.1 s`. A token counts only when no
  digit or `.` comes right before it and no letter, digit or `_` right after
  it, so `1380.0 s` and `380.0 sec` do not count.
- A line with `after:` followed by three times in seconds:
  `after: 121.6 s, 121.6 s, 116.6 s`. `after:` is lowercase with no letter,
  digit or `_` right before it; each time is digits with an optional decimal
  part, then a space and `s`; a comma separates them; spaces or tabs may
  follow `after:` and each comma. Each time is at most half the base median.
  At least one line has this form, and every line that has it meets the
  limit.

A description without them is the violation `measurements-not-in-pr`. When
HalfTime is not a pass, MeasurementsInPR is `skip` (not_selected).

Record the base, on a quiet machine, from a tree whose cmd/ha is the base:

  python3 tools/goal-checks/cmd-ha-test-speed/measure.py --record-base tools/goal-checks/cmd-ha-test-speed/base_timing.json

This runs the same three measurements and writes
{"commit", "go_version", "package", "cpus", "runs", "median", "load", "outcomes", "measured_at"}.
`outcomes` maps every top-level test to its outcome (pass, skip or fail) in
each of the three runs. `measured_at` comes from the clock when it is written.
It refuses a tree where `git status --porcelain` shows anything, tracked or
untracked, outside tools/goal-checks/cmd-ha-test-speed/, and writes nothing
when a run fails, a test fails, or a test's outcome differs between the runs.

`--package` (default ./cmd/ha) measures another package of this module; it
exists to exercise this producer on small packages. A base recorded for one
package is refused for another.

Under `ha`, the report goes to HA_EVIDENCE_PATH with HA_EVIDENCE_INVOCATION and
HA_EVIDENCE_DECLARATION_DIGEST, as in contracts/check-result.md §2. Run alone,
it only prints and exits. The go test children do not inherit the HA_EVIDENCE_*
or HA_JUNIT_XML variables.

Outcomes. Exit 0: both targets pass. Exit 1: a target is a `violation` and
none is an `error`; HalfTime is `not-half-time` when the head median is more
than 0.5 x the base median. Exit 2: a target is an `error`, never `pass`.
HalfTime's error reasons:

- base_timing.json missing: dependency_missing.
- base_timing.json the placeholder, corrupt, with other fields, or recorded for
  another `go version`, CPU count or package: setup_error.
- renames.json missing: dependency_missing; corrupt or not a map of test
  names to non-empty lists of unique test names: setup_error.
- go not on PATH: dependency_missing; go not executable: permission_denied.
- GOFLAGS set (it can change which tests run): setup_error.
- a run that does not build: compile_error.
- a run that exits nonzero, is killed by a signal, exceeds 30 minutes, writes
  output that is not go test JSON, has no single final package pass event, has
  a failed test, or runs no test: execution_error.
- a base-passing test that does not pass in every head run (renames applied):
  execution_error, after the three runs.

MeasurementsInPR's error reasons, never `pass` or `violation`:

- gh not on PATH: dependency_missing; gh not executable: permission_denied.
- gh that cannot start otherwise, is killed by a signal, exceeds 120 seconds
  or exits nonzero (no pull request for this branch, no authentication, no
  network): execution_error. gh's own messages are not printed.
- output that is not UTF-8 JSON, or a `body` that is missing or not a string:
  execution_error.

The base and renames.json are read before any test runs, so the placeholder
fails at once. An invalid declaration, a declaration for another producer, or
a report path that cannot be written gives exit 2 without a report, which `ha`
reads as unknown.
"""
import argparse
import datetime
from decimal import ROUND_HALF_UP, Decimal
import json
import math
import os
from pathlib import Path
import re
import stat
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]
CHECK_DIR = "tools/goal-checks/cmd-ha-test-speed/"
SELF = CHECK_DIR + "measure.py"
PRODUCER, PRODUCER_VERSION = "cmd-ha-timing", "2"
TARGET, VIOLATION = "HalfTime", "not-half-time"
PR_TARGET, PR_VIOLATION = "MeasurementsInPR", "measurements-not-in-pr"
PR_VIEW = ["gh", "pr", "view", "--json", "body"]
DEFAULT_PACKAGE = "./cmd/ha"
RUNS = 3
RUN_TIMEOUT = 1800
BASE_FIELDS = {"commit", "go_version", "package", "cpus", "runs", "median", "load", "outcomes", "measured_at"}
OUTCOMES = ("pass", "skip", "fail")
RENAMES = CHECK_DIR + "renames.json"
TEST_NAME = re.compile(r"Test(?:[A-Z0-9_][A-Za-z0-9_]*)?")
HIDDEN = ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST", "HA_JUNIT_XML")
# The pull request's `after:` line: three times in seconds, on one line.
AFTER = re.compile(r"(?<![A-Za-z0-9_])after:[ \t]*" + r",[ \t]*".join([r"(\d+(?:\.\d+)?) s"] * RUNS)
                   + r"(?![A-Za-z0-9_])")


class Unobserved(Exception):
    def __init__(self, reason, message):
        super().__init__(message)
        self.reason = reason


def local_file(name):
    """A regular file inside the repository, reached without symlinks."""
    if not isinstance(name, str) or not name or "\\" in name:
        raise ValueError(f"invalid path {name!r}")
    path = Path(name)
    if path.is_absolute() or any(part in ("..", ".git") for part in path.parts):
        raise ValueError(f"invalid path {name!r}")
    current = ROOT
    for index, part in enumerate(path.parts):
        current /= part
        mode = current.lstat().st_mode
        if stat.S_ISLNK(mode) or (index < len(path.parts) - 1 and not stat.S_ISDIR(mode)):
            raise ValueError(f"unsafe path {name!r}")
    if not stat.S_ISREG(mode):
        raise ValueError(f"{name} is not a regular file")
    return current


def unique_pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError(f"duplicate key {key!r}")
        result[key] = value
    return result


def read_json(name):
    data = local_file(name).read_bytes()
    if len(data) > 1 << 20:
        raise ValueError(f"{name} is larger than 1 MiB")
    return json.loads(data.decode("utf-8"), object_pairs_hook=unique_pairs)


def declaration(name):
    """Return the base timing path named by a valid AC-1 declaration."""
    data = read_json(name)
    if (not isinstance(data, dict) or set(data) != {"schema", "producer", "producer_version", "producer_paths", "targets"}
            or data["schema"] != "check-declaration/v1" or data["producer"] != PRODUCER
            or data["producer_version"] != PRODUCER_VERSION
            or data["targets"] != [{"id": TARGET, "violation": VIOLATION},
                                   {"id": PR_TARGET, "violation": PR_VIOLATION}]):
        raise ValueError(f"{name} is not the {PRODUCER} declaration for {TARGET} and {PR_TARGET}")
    paths = data["producer_paths"]
    if not isinstance(paths, list):
        raise ValueError(f"{name} producer_paths is not a list")
    bases = [path for path in paths if isinstance(path, str) and Path(path).name == "base_timing.json"]
    if SELF not in paths or len(bases) != 1:
        raise ValueError(f"{name} must list {SELF} and one base_timing.json")
    return bases[0]


def number(value, minimum=0.0):
    return (isinstance(value, (int, float)) and not isinstance(value, bool) and math.isfinite(value)
            and value >= minimum)


def median(values):
    return sorted(values)[len(values) // 2]


def validate_base(data, name):
    """Structural checks of a recorded base; Unobserved(setup_error) when it is not one."""
    if not isinstance(data, dict):
        raise Unobserved("setup_error", f"{name} is not a JSON object")
    if "placeholder" in data:
        raise Unobserved("setup_error", f"{name} is the placeholder; record the base first with --record-base")
    if set(data) != BASE_FIELDS:
        raise Unobserved("setup_error", f"{name} fields are {sorted(data)}, not {sorted(BASE_FIELDS)}")
    problems = []
    if not isinstance(data["commit"], str) or not re.fullmatch(r"[0-9a-f]{40}", data["commit"]):
        problems.append("commit is not a full commit ID")
    if not isinstance(data["go_version"], str) or not data["go_version"].startswith("go version "):
        problems.append("go_version is not `go version` output")
    if not isinstance(data["package"], str) or not data["package"]:
        problems.append("package is empty")
    if not isinstance(data["cpus"], int) or isinstance(data["cpus"], bool) or data["cpus"] < 1:
        problems.append("cpus is not a positive integer")
    runs = data["runs"]
    if not isinstance(runs, list) or len(runs) != RUNS or not all(number(run) and run > 0 for run in runs):
        problems.append(f"runs is not {RUNS} positive seconds")
    elif data["median"] != median(runs):
        problems.append("median is not the median of runs")
    load = data["load"]
    if not isinstance(load, list) or len(load) != 3 or not all(number(value) for value in load):
        problems.append("load is not three load averages")
    outcomes = data["outcomes"]
    if (not isinstance(outcomes, dict) or not outcomes
            or not all(isinstance(name, str) and TEST_NAME.fullmatch(name) for name in outcomes)
            or not all(isinstance(values, list) and len(values) == RUNS and all(value in OUTCOMES for value in values)
                       for values in outcomes.values())):
        problems.append(f"outcomes is not a map of top-level test names to {RUNS} outcomes from {OUTCOMES}")
    elif any(len(set(values)) != 1 or "fail" in values for values in outcomes.values()):
        problems.append("outcomes has a failed test or a test whose outcome differs between runs")
    elif "pass" not in {values[0] for values in outcomes.values()}:
        problems.append("outcomes has no passing test")
    try:
        if datetime.datetime.fromisoformat(data["measured_at"]).tzinfo is None:
            problems.append("measured_at has no time zone")
    except (TypeError, ValueError):
        problems.append("measured_at is not an ISO 8601 time")
    if problems:
        raise Unobserved("setup_error", f"{name} is corrupt: " + "; ".join(problems))
    return data


def read_base(name):
    try:
        data = read_json(name)
    except FileNotFoundError:
        raise Unobserved("dependency_missing", f"{name} is missing") from None
    except PermissionError:
        raise Unobserved("permission_denied", f"{name} cannot be read") from None
    except (OSError, UnicodeError, ValueError) as error:
        raise Unobserved("setup_error", f"{name} is unreadable or corrupt: {error}") from None
    return validate_base(data, name)


def read_renames():
    """renames.json: base test name -> non-empty list of unique head test names."""
    try:
        data = read_json(RENAMES)
    except FileNotFoundError:
        raise Unobserved("dependency_missing", f"{RENAMES} is missing") from None
    except PermissionError:
        raise Unobserved("permission_denied", f"{RENAMES} cannot be read") from None
    except (OSError, UnicodeError, ValueError) as error:
        raise Unobserved("setup_error", f"{RENAMES} is unreadable or corrupt: {error}") from None
    if not isinstance(data, dict) or not all(
            isinstance(old, str) and TEST_NAME.fullmatch(old) and isinstance(new, list) and new
            and all(isinstance(name, str) and TEST_NAME.fullmatch(name) for name in new) and len(set(new)) == len(new)
            for old, new in data.items()):
        raise Unobserved("setup_error", f"{RENAMES} is not a map of test names to non-empty lists of unique test names")
    return data


def base_outcomes(runs):
    """Every top-level test's outcome in each run; Unobserved when a test failed or its outcomes differ."""
    names = sorted(set().union(*runs))
    outcomes = {name: [run.get(name, "missing") for run in runs] for name in names}
    bad = [f"{name} ({', '.join(values)})" for name, values in outcomes.items()
           if len(set(values)) != 1 or "fail" in values]
    if bad:
        raise Unobserved("execution_error", f"test outcomes differ between the base runs or a test failed: "
                         + "; ".join(bad[:20]))
    return outcomes


def outcome_differences(base, runs, renames):
    """Base tests that passed in every base run and do not pass in every given run, renames applied."""
    def passes(name):
        return all(run.get(name) == "pass" for run in runs)

    def show(name):
        return ", ".join(run.get(name, "missing") for run in runs)

    problems = []
    for name, values in sorted(base.items()):
        if set(values) != {"pass"} or passes(name):
            continue
        new = renames.get(name)
        if new and all(passes(other) for other in new):
            continue
        detail = f"{name}: base pass in all {len(values)} runs, head {show(name)}"
        if new:
            detail += "; renamed to " + "; ".join(f"{other} ({show(other)})" for other in new)
        problems.append(detail)
    return problems


def child_env():
    env = dict(os.environ)
    for name in HIDDEN:
        env.pop(name, None)
    return env


def run(argv, timeout=120):
    try:
        result = subprocess.run(argv, cwd=ROOT, env=child_env(), capture_output=True, timeout=timeout)
    except FileNotFoundError:
        raise Unobserved("dependency_missing", f"{argv[0]} is not on PATH") from None
    except PermissionError:
        raise Unobserved("permission_denied", f"{argv[0]} cannot be executed") from None
    except subprocess.TimeoutExpired:
        raise Unobserved("execution_error", f"{' '.join(argv)} did not finish within {timeout}s") from None
    except OSError as error:
        raise Unobserved("execution_error", f"{argv[0]} could not start: {error}") from None
    if result.returncode < 0:
        raise Unobserved("execution_error", f"{' '.join(argv)} was killed by signal {-result.returncode}")
    return result


def text(argv, strip=True):
    result = run(argv)
    if result.returncode:
        raise Unobserved("execution_error", f"{' '.join(argv)} exited {result.returncode}: "
                         + result.stderr.decode("utf-8", "replace").strip()[-400:])
    try:
        output = result.stdout.decode("utf-8")
    except UnicodeDecodeError:
        raise Unobserved("execution_error", f"{' '.join(argv)} output is not UTF-8") from None
    return output.strip() if strip else output


def go_identity(package):
    """Return (go version, import path) and refuse GOFLAGS."""
    version = text(["go", "version"])
    flags = text(["go", "env", "GOFLAGS"])
    if flags:
        raise Unobserved("setup_error", f"GOFLAGS is {flags!r}; it can change which tests run")
    lines = text(["go", "list", "-f", "{{.ImportPath}}", package]).splitlines()
    if len(lines) != 1 or not lines[0]:
        raise Unobserved("setup_error", f"{package} is not one package")
    return version, lines[0]


def measure_once(package, import_path):
    """Run the package's tests once; return (package Elapsed, top-level test outcomes)."""
    result = run(["go", "test", "-count=1", "-json", package], timeout=RUN_TIMEOUT)
    events = []
    try:
        for line in result.stdout.decode("utf-8").splitlines():
            if line.strip():
                event = json.loads(line)
                if not isinstance(event, dict):
                    raise ValueError("not an object")
                events.append(event)
    except (UnicodeDecodeError, ValueError) as error:
        raise Unobserved("execution_error", f"go test output is not go test JSON: {error}") from None
    if any(event.get("Action") == "build-fail" for event in events):
        raise Unobserved("compile_error", f"{package} does not build: "
                         + result.stderr.decode("utf-8", "replace").strip()[-400:])
    tests = {}
    for event in events:
        if event.get("Package") == import_path and event.get("Test") and event.get("Action") in ("pass", "fail", "skip"):
            tests[event["Test"]] = event["Action"]
    failed = sorted(name for name, action in tests.items() if action == "fail")
    if result.returncode or failed:
        raise Unobserved("execution_error", f"go test {package} exited {result.returncode}; failed tests: "
                         + (", ".join(failed[:20]) or "none reported") + "; "
                         + result.stderr.decode("utf-8", "replace").strip()[-400:])
    final = [event for event in events if event.get("Package") == import_path and not event.get("Test")
             and event.get("Action") in ("pass", "fail", "skip")]
    if len(final) != 1 or final[0].get("Action") != "pass":
        raise Unobserved("execution_error", f"go test {package} has no single final package pass event")
    elapsed = final[0].get("Elapsed")
    if not number(elapsed) or elapsed <= 0:
        raise Unobserved("execution_error", f"go test {package} package pass event has no positive Elapsed")
    top = {name: action for name, action in tests.items() if "/" not in name}
    if not any(action == "pass" for action in top.values()):
        raise Unobserved("execution_error", f"go test {package} ran no test")
    return float(elapsed), top


def seconds_token(value):
    """A recorded time as a person copies it: rounded half up to one decimal, then ` s`."""
    return f"{Decimal(repr(value)).quantize(Decimal('0.1'), rounding=ROUND_HALF_UP)} s"


def pr_body():
    """This branch's pull request description; Unobserved when it cannot be read."""
    command = " ".join(PR_VIEW)
    result = run(PR_VIEW)
    if result.returncode:
        # Like cross_platform_ci.py, do not print a failing GitHub request's own output.
        raise Unobserved("execution_error", f"{command} exited {result.returncode}: no pull request for this "
                         "branch, or gh is not authenticated or cannot reach GitHub")
    try:
        data = json.loads(result.stdout.decode("utf-8"), object_pairs_hook=unique_pairs)
    except (UnicodeDecodeError, ValueError) as error:
        raise Unobserved("execution_error", f"{command} output is not UTF-8 JSON: {error}") from None
    if not isinstance(data, dict) or not isinstance(data.get("body"), str):
        raise Unobserved("execution_error", f"{command} output has no text body")
    return data["body"]


def pr_problems(body, base):
    """What the pull request description lacks of the measurements; empty when it has them all."""
    problems = []
    labels = [f"base run {index}" for index in range(1, RUNS + 1)] + ["base median"]
    for label, value in zip(labels, [*base["runs"], base["median"]]):
        token = seconds_token(value)
        if not re.search(r"(?<![0-9.])" + re.escape(token) + r"(?![A-Za-z0-9_])", body):
            problems.append(f"{label} `{token}` is missing")
    half = Decimal(repr(base["median"])) / 2
    lines = list(AFTER.finditer(body))
    if not lines:
        problems.append("no line has `after:` followed by three times in seconds")
    for line in lines:
        over = [value for value in line.groups() if Decimal(value) > half]
        if over:
            problems.append(f"`{line.group(0)}`: " + ", ".join(f"{value} s" for value in over)
                            + f" over half the base median ({half} s)")
    return problems


def load_average():
    try:
        return list(os.getloadavg())
    except (AttributeError, OSError):
        return None


def show_load(label, load):
    if load is None:
        return f"{label} load average unavailable"
    return f"{label} load average 1m {load[0]:.2f}, 5m {load[1]:.2f}"


def measure(package, import_path):
    """Return the three times and each run's top-level test outcomes."""
    times, runs = [], []
    for attempt in range(1, RUNS + 1):
        elapsed, top = measure_once(package, import_path)
        times.append(elapsed)
        runs.append(top)
        passed = sum(action == "pass" for action in top.values())
        skipped = sum(action == "skip" for action in top.values())
        print(f"run {attempt}: {elapsed:.3f}s ({passed} top-level tests passed, {skipped} skipped)", flush=True)
    return times, runs


def check(args):
    try:
        base_path = declaration(args.declaration)
        evidence = os.environ.get("HA_EVIDENCE_PATH")
        if evidence and not (os.environ.get("HA_EVIDENCE_INVOCATION") and os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST")):
            raise ValueError("HA_EVIDENCE_PATH is set without the invocation and declaration digest")
    except (OSError, UnicodeError, ValueError) as error:
        print(f"{TARGET}: not observed: {error}", file=sys.stderr)
        return 2
    observation = {"target": TARGET, "attempt": 1}
    pr = {"target": PR_TARGET, "attempt": 1}
    try:
        base = read_base(base_path)
        version, import_path = go_identity(args.package)
        if base["go_version"] != version:
            raise Unobserved("setup_error", f"go version differs: base {base['go_version']!r}, now {version!r}")
        if base["cpus"] != os.cpu_count():
            raise Unobserved("setup_error", f"CPU count differs: base {base['cpus']}, now {os.cpu_count()}")
        if base["package"] != args.package:
            raise Unobserved("setup_error", f"base was recorded for {base['package']}, not {args.package}")
        renames = read_renames()
        print(show_load("before:", load_average()), flush=True)
        times, runs = measure(args.package, import_path)
        head = median(times)
        ratio = head / base["median"]
        print("head times: " + ", ".join(f"{value:.3f}s" for value in times) + f"; head median {head:.3f}s")
        print(f"base median {base['median']:.3f}s (commit {base['commit'][:12]}, measured {base['measured_at']})")
        print(f"ratio {ratio:.3f} (pass at <= 0.500)")
        print(show_load("after:", load_average()))
        differences = outcome_differences(base["outcomes"], runs, renames)
        for line in differences:
            print(f"outcome differs: {line}")
        if differences:
            raise Unobserved("execution_error", f"{len(differences)} base-passing test(s) do not pass in every head "
                             "run, so the times are not comparable")
        if head <= 0.5 * base["median"]:
            observation["status"] = "pass"
        else:
            observation.update(status="violation", violation=VIOLATION)
    except Unobserved as error:
        observation.update(status="error", reason=error.reason)
        print(f"{TARGET}: error ({error.reason}): {error}", file=sys.stderr)
    print(f"{TARGET} {observation['status']}")
    if observation["status"] == "pass":
        try:
            problems = pr_problems(pr_body(), base)
            for line in problems:
                print(f"pull request description: {line}")
            if problems:
                pr.update(status="violation", violation=PR_VIOLATION)
            else:
                pr["status"] = "pass"
        except Unobserved as error:
            pr.update(status="error", reason=error.reason)
            print(f"{PR_TARGET}: error ({error.reason}): {error}", file=sys.stderr)
    else:
        pr.update(status="skip", reason="not_selected")
    print(f"{PR_TARGET} {pr['status']}")
    observations = [observation, pr]
    if evidence:
        report = {
            "schema": "check-result/v1",
            "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
            "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
            "producer": PRODUCER,
            "producer_version": PRODUCER_VERSION,
            "attempts_complete": True,
            "observations": observations,
        }
        try:
            with open(evidence, "x", encoding="utf-8") as output:
                json.dump(report, output, separators=(",", ":"))
                output.write("\n")
        except OSError as error:
            print(f"{TARGET}: report not written: {error}", file=sys.stderr)
            return 2
    statuses = {item["status"] for item in observations}
    if "error" in statuses:
        return 2
    if "violation" in statuses:
        return 1
    return 0 if statuses == {"pass"} else 2


def git(*args):
    return text(["git", *args], strip=args[0] != "status")


def outside_check_dir():
    """Paths `git status --porcelain` shows, tracked or untracked, outside CHECK_DIR."""
    records = git("status", "--porcelain", "-z", "--untracked-files=all").split("\0")
    paths, index = [], 0
    while index < len(records):
        record = records[index]
        index += 1
        if not record:
            continue
        if len(record) < 4 or record[2] != " ":
            raise Unobserved("execution_error", f"unexpected git status record {record!r}")
        paths.append(record[3:])
        if "R" in record[:2] or "C" in record[:2]:
            # A rename or copy record is followed by its source path.
            if index >= len(records) or not records[index]:
                raise Unobserved("execution_error", f"git status rename record without a source: {record!r}")
            paths.append(records[index])
            index += 1
    return [path for path in paths if not path.startswith(CHECK_DIR)]


def record(args):
    try:
        target = Path(args.record_base)
        if target.is_absolute() or ".." in target.parts or target.is_symlink():
            raise Unobserved("setup_error", f"{args.record_base} must be a repository-relative path")
        changed = outside_check_dir()
        if changed:
            raise Unobserved("setup_error", "git status shows tracked or untracked files outside " + CHECK_DIR + ": "
                             + ", ".join(changed[:10]))
        commit = git("rev-parse", "--verify", "HEAD^{commit}")
        version, import_path = go_identity(args.package)
        load = load_average()
        if load is None:
            raise Unobserved("setup_error", "load averages are unavailable")
        print(show_load("before:", load), flush=True)
        times, runs = measure(args.package, import_path)
        print(show_load("after:", load_average()))
        outcomes = base_outcomes(runs)
        data = {
            "commit": commit,
            "go_version": version,
            "package": args.package,
            "cpus": os.cpu_count(),
            "runs": times,
            "median": median(times),
            "load": load,
            "outcomes": outcomes,
            "measured_at": datetime.datetime.now(datetime.timezone.utc).isoformat(timespec="seconds"),
        }
        validate_base(data, args.record_base)
        path = ROOT / target
        temporary = path.with_name(path.name + ".tmp")
        temporary.write_text(json.dumps(data, indent=1) + "\n", encoding="utf-8")
        os.replace(temporary, path)
    except Unobserved as error:
        print(f"record-base: error ({error.reason}): {error}; nothing written", file=sys.stderr)
        return 2
    except OSError as error:
        print(f"record-base: cannot write {args.record_base}: {error}", file=sys.stderr)
        return 2
    print(f"wrote {args.record_base}: runs " + ", ".join(f"{value:.3f}s" for value in times)
          + f", median {data['median']:.3f}s at {commit[:12]}")
    return 0


def main():
    parser = argparse.ArgumentParser(description="cmd-ha-test-speed AC-1 timing producer")
    parser.add_argument("declaration", nargs="?", help="AC-1 declaration (check mode)")
    parser.add_argument("--record-base", metavar="PATH", help="measure this tree and write the base timing")
    parser.add_argument("--package", default=DEFAULT_PACKAGE, help="package to measure (default ./cmd/ha)")
    args = parser.parse_args()
    if bool(args.declaration) == bool(args.record_base):
        parser.error("give either a declaration or --record-base PATH")
    if Path.cwd().resolve() != ROOT:
        print(f"run from the repository root {ROOT}", file=sys.stderr)
        return 2
    return record(args) if args.record_base else check(args)


if __name__ == "__main__":
    raise SystemExit(main())
