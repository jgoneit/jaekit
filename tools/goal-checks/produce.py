#!/usr/bin/env python3
"""check-result/v1 producer that observes repository tests.

Usage: python3 tools/goal-checks/produce.py DECLARATION TARGETS

Each target runs one existing unittest or Go test, a whole unittest module, or
the documentation tests of ./plugins in a temporary copy whose file received an
appended statement. A test assertion failure is the declared violation.
Import, collection, build and setup problems are errors, never violations.
The `command` runner reports only a repository script's exit status and is
meant for maintain conditions, whose results do not use the structured report.
Without the ha evidence environment the producer only sets its exit status,
so maintain conditions and local runs use the same target files.
"""

import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import tempfile
import traceback
import unittest

PRODUCER = "jaekit-goal-tests"
VERSION = "1"
TIMEOUT = 900
SKIP_COPY = {".git", ".claude", ".private", "docs"}


def local_file(name):
    """Return a regular file inside the working directory without symlinks."""
    if not isinstance(name, str) or not name or "\\" in name:
        raise ValueError("invalid path")
    path = Path(name)
    if path.is_absolute() or any(p in ("", ".", "..", ".git") for p in path.parts):
        raise ValueError("invalid path")
    current = Path.cwd()
    for index, part in enumerate(path.parts):
        current /= part
        mode = current.lstat().st_mode
        if stat.S_ISLNK(mode) or (index < len(path.parts) - 1 and not stat.S_ISDIR(mode)):
            raise ValueError("unsafe path")
    if not stat.S_ISREG(mode):
        raise ValueError("not a regular file")
    return current


def go_env():
    env = dict(os.environ, GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
    for name in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
        env.pop(name, None)
    return env


def go_events(argv, cwd):
    proc = subprocess.run(argv, cwd=cwd, env=go_env(), capture_output=True, text=True, timeout=TIMEOUT)
    sys.stderr.write(proc.stderr)
    events = []
    for line in proc.stdout.splitlines():
        if line.startswith("{"):
            try:
                event = json.loads(line)
                if isinstance(event, dict):
                    events.append(event)
            except ValueError:
                pass
        else:
            sys.stderr.write(line + "\n")
    built = not any(e.get("Action") == "build-fail" or e.get("FailedBuild") for e in events)
    built = built and "[setup failed]" not in proc.stderr and "build failed" not in proc.stderr
    finals = {}
    outputs = {}
    started = set()
    package_finals = {}
    diagnostics = [proc.stderr]
    for event in events:
        test = event.get("Test")
        action = event.get("Action")
        if action == "output":
            output = event.get("Output", "")
            diagnostics.append(output)
            outputs.setdefault(test or "", []).append(output)
            # Package output includes TestMain failures and post-test panics.
            sys.stderr.write(output)
        if not test:
            if action in ("pass", "fail", "skip"):
                package_finals[event.get("Package", "")] = action
            continue
        if action == "run":
            started.add(test)
        if action in ("pass", "fail", "skip"):
            finals[test] = action
    if not built:
        return "compile_error", finals, outputs
    failed = "fail" in finals.values()
    package_failed = "fail" in package_finals.values()
    runtime_failure = re.search(
        r"^(?:panic:|fatal error:|runtime:|signal:|WARNING: DATA RACE|exit status (?:[2-9]|[1-9][0-9]+)\b)",
        "".join(diagnostics), re.MULTILINE)
    # Exit 1 with an ordinary assertion is valid. A process/package failure
    # without that assertion, or a test that never finished, is not its proof.
    if (proc.returncode not in (0, 1)
            or (proc.returncode != 0 and runtime_failure)
            or started.difference(finals)
            or (proc.returncode == 0 and (failed or package_failed))
            or (proc.returncode != 0 and not failed)):
        return "execution_error", finals, outputs
    return None, finals, outputs


def run_go(check):
    name = check["test"]
    if not isinstance(name, str) or not name.startswith("Test") or not name.isidentifier():
        raise ValueError("invalid go test")
    error, finals, _ = go_events(["go", "test", "-count=1", "-json", "-run", "^" + name + "$", check["package"]], Path.cwd())
    if error:
        return {"status": "error", "reason": error}
    result = finals.get(name)
    if result == "pass":
        return {"status": "pass"}
    if result == "fail":
        return {"status": "violation"}
    if result == "skip":
        return {"status": "skip", "reason": "skipped"}
    return {"status": "error", "reason": "collection_error"}


def copy_tree(source, destination):
    def ignore(directory, names):
        return [n for n in names if Path(directory) == source and n in SKIP_COPY]
    shutil.copytree(source, destination, symlinks=True, ignore=ignore)


def failure_lines(finals, outputs):
    lines = set()
    for test, action in finals.items():
        if action == "fail":
            lines.update(line.strip() for line in "".join(outputs.get(test, [])).splitlines())
    return lines


def run_go_docs(check):
    """Run the plugins documentation tests on a copy, before and after a mutation.

    With `expect: pass` the unmodified copy must pass. With `expect: reject`
    the appended statement must cause a failure line naming the mutated file
    that the unmodified copy does not already report; failures that exist
    either way say nothing about this statement.
    """
    expect = check["expect"]
    if expect not in ("pass", "reject") or check["package"] != "./plugins":
        raise ValueError("invalid documentation probe")
    argv = ["go", "test", "-count=1", "-json", check["package"]]
    with tempfile.TemporaryDirectory(prefix="jaekit-goal-docs-") as directory:
        copy = Path(directory) / "repo"
        copy_tree(Path.cwd(), copy)
        error, finals, outputs = go_events(argv, copy)
        if error:
            return {"status": "error", "reason": error}
        if not finals:
            return {"status": "error", "reason": "collection_error"}
        if all(action == "skip" for action in finals.values()):
            return {"status": "skip", "reason": "skipped"}
        if expect == "pass":
            failed = any(action == "fail" for action in finals.values())
            return {"status": "violation"} if failed else {"status": "pass"}
        before = failure_lines(finals, outputs)
        relative = local_file(check["path"]).relative_to(Path.cwd())
        with (copy / relative).open("a", encoding="utf-8") as output:
            output.write(check["append"])
        error, finals, outputs = go_events(argv, copy)
    if error:
        return {"status": "error", "reason": error}
    if not finals:
        return {"status": "error", "reason": "collection_error"}
    if all(action == "skip" for action in finals.values()):
        return {"status": "skip", "reason": "skipped"}
    added = failure_lines(finals, outputs) - before
    if any(check["mentions"] in line for line in added):
        return {"status": "pass"}
    return {"status": "violation"}


def run_unittest(check):
    argv = [sys.executable, str(Path(__file__).resolve()), "--unittest", check["file"], check.get("test", "")]
    proc = subprocess.run(argv, cwd=Path.cwd(), env=go_env(), capture_output=True, text=True, timeout=TIMEOUT)
    sys.stderr.write(proc.stderr)
    lines = [line for line in proc.stdout.splitlines() if line.strip()]
    try:
        verdict = json.loads(lines[-1])
    except (IndexError, ValueError):
        return {"status": "error", "reason": "execution_error"}
    if verdict.get("status") not in ("pass", "violation", "error", "skip"):
        return {"status": "error", "reason": "execution_error"}
    return verdict


def unittest_child(file, test):
    """Run one unittest id (or a module) in this process and print a verdict."""
    try:
        path = local_file(file)
    except (OSError, ValueError):
        return {"status": "error", "reason": "dependency_missing"}
    sys.path.insert(0, str(path.parent))
    try:
        spec = importlib.util.spec_from_file_location(path.stem, path)
        module = importlib.util.module_from_spec(spec)
        sys.modules[path.stem] = module
        spec.loader.exec_module(module)
    except Exception:
        traceback.print_exc()
        return {"status": "error", "reason": "import_error"}
    loader = unittest.TestLoader()
    try:
        suite = loader.loadTestsFromName(test, module) if test else loader.loadTestsFromModule(module)
    except Exception:
        traceback.print_exc()
        return {"status": "error", "reason": "collection_error"}

    def cases(item):
        if isinstance(item, unittest.TestSuite):
            for child in item:
                yield from cases(child)
        else:
            yield item
    if any(type(case).__name__ == "_FailedTest" for case in cases(suite)) or loader.errors:
        return {"status": "error", "reason": "collection_error"}
    result = unittest.TextTestRunner(stream=sys.stderr, verbosity=2).run(suite)
    if result.testsRun == 0:
        return {"status": "error", "reason": "collection_error"}
    if result.errors:
        return {"status": "error", "reason": "execution_error"}
    # unittest's expected outcomes are decorator states, not evidence that a
    # declared requirement passed or failed. They can also wrap runtime errors.
    if result.expectedFailures or result.unexpectedSuccesses:
        return {"status": "error", "reason": "setup_error"}
    if result.failures:
        return {"status": "violation"}
    if len(result.skipped) == result.testsRun:
        return {"status": "skip", "reason": "skipped"}
    return {"status": "pass"}


def run_command(check):
    """Exit status of a repository script, for maintain conditions only."""
    argv = check["argv"]
    if not isinstance(argv, list) or not argv or not all(isinstance(a, str) and a for a in argv):
        raise ValueError("invalid command")
    if argv[0] == "python3":
        argv = [sys.executable, *argv[1:]]
    proc = subprocess.run(argv, cwd=Path.cwd(), env=go_env(), capture_output=True, text=True, timeout=TIMEOUT)
    sys.stderr.write(proc.stdout + proc.stderr)
    return {"status": "pass"} if proc.returncode == 0 else {"status": "violation"}


RUNNERS = {"go": run_go, "go-docs": run_go_docs, "unittest": run_unittest, "command": run_command}


def observe(check, declared):
    observation = {"target": check["target"], "attempt": 1}
    try:
        verdict = RUNNERS[check["runner"]](check)
    except subprocess.TimeoutExpired:
        verdict = {"status": "error", "reason": "execution_error"}
    except FileNotFoundError:
        verdict = {"status": "error", "reason": "dependency_missing"}
    except PermissionError:
        verdict = {"status": "error", "reason": "permission_denied"}
    except (OSError, ValueError, TypeError, KeyError):
        traceback.print_exc()
        verdict = {"status": "error", "reason": "setup_error"}
    observation["status"] = verdict["status"]
    if verdict["status"] == "violation":
        observation["violation"] = declared["violation"]
    elif verdict["status"] in ("error", "skip"):
        observation["reason"] = verdict["reason"]
    return observation


def main(argv):
    if len(argv) == 4 and argv[1] == "--unittest":
        print(json.dumps(unittest_child(argv[2], argv[3])))
        return 0
    if len(argv) != 3:
        print("usage: produce.py DECLARATION TARGETS", file=sys.stderr)
        return 2
    try:
        declaration = json.loads(local_file(argv[1]).read_text(encoding="utf-8"))
        config = json.loads(local_file(argv[2]).read_text(encoding="utf-8"))
        if (declaration["schema"] != "check-declaration/v1" or declaration["producer"] != PRODUCER
                or declaration["producer_version"] != VERSION or config.get("schema") != "jaekit-goal-targets/v1"):
            raise ValueError("unsupported declaration or targets")
        targets = {target["id"]: target for target in declaration["targets"]}
        checks = config["checks"]
        if len(checks) != len(targets) or {check["target"] for check in checks} != set(targets):
            raise ValueError("targets do not match the declaration")
        observations = []
        for check in checks:
            observation = observe(check, targets[check["target"]])
            print(observation["target"], observation["status"], flush=True)
            observations.append(observation)
        if "HA_EVIDENCE_PATH" in os.environ:
            report = {
                "schema": "check-result/v1",
                "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                "producer": PRODUCER,
                "producer_version": VERSION,
                "attempts_complete": True,
                "observations": observations,
            }
            with open(os.environ["HA_EVIDENCE_PATH"], "x", encoding="utf-8") as output:
                json.dump(report, output, separators=(",", ":"))
                output.write("\n")
        return 0 if all(o["status"] == "pass" for o in observations) else 1
    except (OSError, ValueError, TypeError, KeyError):
        traceback.print_exc()
        print("goal check configuration or report transport failed", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
