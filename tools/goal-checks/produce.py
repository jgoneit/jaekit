#!/usr/bin/env python3
"""check-result/v1 producer that observes repository tests.

Usage: python3 tools/goal-checks/produce.py DECLARATION TARGETS

Each target runs one existing unittest or Go test, a whole unittest module, or
the documentation tests of ./plugins in a temporary copy whose file received an
appended statement. Observed assertion identities never come from declarations.
Import, collection, build and setup problems are errors, never violations.
The `command` runner needs an explicit exit contract for structured evidence;
an undocumented command can only establish a successful maintain exit.
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

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import result_observations as results

PRODUCER = "jaekit-goal-tests"
VERSION = "2"
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
        key = (event.get("Package", ""), test or "")
        action = event.get("Action")
        if action == "output":
            output = event.get("Output", "")
            diagnostics.append(output)
            outputs.setdefault(key, []).append(output)
            # Package output includes TestMain failures and post-test panics.
            sys.stderr.write(output)
        if not test:
            if action in ("pass", "fail", "skip"):
                package_finals[event.get("Package", "")] = action
            continue
        if action == "run":
            started.add(key)
        if action in ("pass", "fail", "skip"):
            finals[key] = action
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


def go_facts(error, finals, outputs, directory):
    result = results.facts()
    if error:
        result["errors"].append(error)
    for name, state in finals.items():
        if state == "pass":
            result["passed"] += 1
        elif state == "skip":
            result["skipped"].append("skipped")
        elif state == "fail":
            # Parent fail events can merely summarize failing subtests.
            if any(child[0] == name[0] and child[1].startswith(name[1] + "/") and final == "fail"
                   for child, final in finals.items()):
                continue
            output = "".join(outputs.get(name, []))
            if re.search(r"^(?:panic:|fatal error:|runtime:|signal:|WARNING: DATA RACE)", output, re.MULTILINE):
                continue
            # Go's event stream cannot identify the assertion responsible for
            # a failure: Log and Error share its output channel. Keep the
            # observed failure but never infer a requirement ID from that text.
            result["violations"].append("unclassified-assertion")
            result["unclassified"] = True
    if not finals and not error:
        result["errors"].append("collection_error")
    return result


def run_go(check, detailed=False):
    name = check["test"]
    if not isinstance(name, str) or not name.startswith("Test") or not name.isidentifier():
        raise ValueError("invalid go test")
    error, finals, outputs = go_events(["go", "test", "-count=1", "-json", "-run", "^" + name + "$", check["package"]], Path.cwd())
    result = go_facts(error, finals, outputs, Path.cwd() / check["package"])
    return result if detailed else results.summary(result)


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


def run_go_docs(check, detailed=False):
    result = go_docs_facts(check)
    return result if detailed else results.summary(result)


def go_docs_facts(check):
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
        initial = go_facts(error, finals, outputs, copy / "plugins")
        if error or not finals:
            return initial
        if expect == "pass":
            return initial
        if initial["violations"] or initial["errors"] or initial["skipped"]:
            return initial
        before = failure_lines(finals, outputs)
        relative = local_file(check["path"]).relative_to(Path.cwd())
        with (copy / relative).open("a", encoding="utf-8") as output:
            output.write(check["append"])
        error, finals, outputs = go_events(argv, copy)
        observed = go_facts(error, finals, outputs, copy / "plugins")
    if observed["errors"] or observed["skipped"] or not finals:
        return observed
    added = failure_lines(finals, outputs) - before
    if any(check["mentions"] in line for line in added):
        result = results.facts()
        result["passed"] = 1
        return result
    result = results.facts()
    result["violations"].append("document-mutation-not-rejected")
    return result


def run_unittest(check, detailed=False):
    with tempfile.TemporaryDirectory(prefix="jaekit-unittest-observation-") as temporary:
        output = Path(temporary) / "facts.json"
        argv = [sys.executable, str(Path(__file__).resolve()), "--unittest", check["file"], check.get("test", ""), str(output)]
        proc = subprocess.run(argv, cwd=Path.cwd(), env=go_env(), capture_output=True, text=True, timeout=TIMEOUT)
        sys.stderr.write(proc.stdout + proc.stderr)
        try:
            verdict = json.loads(output.read_text())
        except (OSError, ValueError):
            verdict = results.failure("execution_error")
    if (set(verdict) != set(results.facts()) or proc.returncode != 0):
        verdict = results.failure("execution_error")
    return verdict if detailed else results.summary(verdict)


def unittest_child(file, test):
    """Run one unittest id (or a module) in this process and print a verdict."""
    try:
        path = local_file(file)
    except (OSError, ValueError):
        return results.failure("dependency_missing")
    sys.path.insert(0, str(path.parent))
    try:
        spec = importlib.util.spec_from_file_location(path.stem, path)
        module = importlib.util.module_from_spec(spec)
        sys.modules[path.stem] = module
        spec.loader.exec_module(module)
    except Exception:
        traceback.print_exc()
        return results.failure("import_error")
    loader = unittest.TestLoader()
    try:
        suite = loader.loadTestsFromName(test, module) if test else loader.loadTestsFromModule(module)
    except Exception:
        traceback.print_exc()
        return results.failure("collection_error")

    def cases(item):
        if isinstance(item, unittest.TestSuite):
            for child in item:
                yield from cases(child)
        else:
            yield item
    if any(type(case).__name__ == "_FailedTest" for case in cases(suite)) or loader.errors:
        return results.failure("collection_error")
    return results.run_suite(suite, sys.stderr)


def run_command(check, detailed=False):
    """Only an explicit command contract can identify observed violations."""
    argv = check["argv"]
    if not isinstance(argv, list) or not argv or not all(isinstance(a, str) and a for a in argv):
        raise ValueError("invalid command")
    if argv[0] == "python3":
        argv = [sys.executable, *argv[1:]]
    proc = subprocess.run(argv, cwd=Path.cwd(), env=go_env(), capture_output=True, text=True, timeout=TIMEOUT)
    sys.stderr.write(proc.stdout + proc.stderr)
    result = results.facts()
    contract = check.get("exit_contract")
    if contract is None:
        # The old command runner remains useful for maintain, not change.
        if proc.returncode == 0 and "HA_EVIDENCE_PATH" not in os.environ:
            result["passed"] = 1
        else:
            result["errors"].append("execution_error")
    elif (not isinstance(contract, dict) or set(contract) != {"schema", "pass", "violations", "unverified"}
          or contract["schema"] != "jaekit-command-exits/v1"):
        raise ValueError("invalid command exit contract")
    else:
        passes, unknown, violations = contract["pass"], contract["unverified"], contract["violations"]
        if (not isinstance(passes, list) or not isinstance(unknown, list) or not isinstance(violations, dict)
                or any(type(x) is not int or not 0 <= x <= 255 for x in passes + unknown)
                or any(not key.isdecimal() or not 1 <= int(key) <= 255
                       or not isinstance(value, str) or not results.IDENTIFIER.fullmatch(value)
                       for key, value in violations.items())
                or len(set(passes + unknown + [int(k) for k in violations])) != len(passes) + len(unknown) + len(violations)):
            raise ValueError("overlapping or invalid command exit contract")
        if proc.returncode in passes:
            result["passed"] = 1
        elif proc.returncode in unknown:
            result["skipped"].append("not_selected")
        elif str(proc.returncode) in violations:
            result["violations"].append(violations[str(proc.returncode)])
        else:
            result["errors"].append("execution_error")
    return result if detailed else results.summary(result)


RUNNERS = {"go": run_go, "go-docs": run_go_docs, "unittest": run_unittest, "command": run_command}


def observe(check, declared=None):
    try:
        verdict = RUNNERS[check["runner"]](check, detailed=True)
    except subprocess.TimeoutExpired:
        verdict = results.failure("execution_error")
    except FileNotFoundError:
        verdict = results.failure("dependency_missing")
    except PermissionError:
        verdict = results.failure("permission_denied")
    except (OSError, ValueError, TypeError, KeyError):
        traceback.print_exc()
        verdict = results.failure("setup_error")
    # Full facts remain in the local output even when bounded slots overflow.
    print(json.dumps({"target": check["target"], "facts": verdict}), file=sys.stderr)
    return results.observations(verdict, check["target"], check["result_targets"])


def main(argv):
    if len(argv) == 5 and argv[1] == "--unittest":
        verdict = unittest_child(argv[2], argv[3])
        with open(argv[4], "x", encoding="utf-8") as output:
            json.dump(verdict, output)
        return 0
    if len(argv) != 3:
        print("usage: produce.py DECLARATION TARGETS", file=sys.stderr)
        return 2
    try:
        declaration = json.loads(local_file(argv[1]).read_text(encoding="utf-8"))
        config = json.loads(local_file(argv[2]).read_text(encoding="utf-8"))
        legacy_maintain = ("HA_EVIDENCE_PATH" not in os.environ
                           and declaration.get("producer_version") == "1"
                           and config.get("schema") == "jaekit-goal-targets/v1")
        if legacy_maintain:
            declaration["producer_version"] = VERSION
            config["schema"] = "jaekit-goal-targets/v2"
            expanded = []
            expected = {item["id"]: item["violation"] for item in declaration["targets"]}
            for check in config["checks"]:
                check["result_targets"] = results.target_names(check["target"])
                expanded.extend(results.declaration_targets(check["target"], expected[check["target"]], check["result_targets"]))
            declaration["targets"] = expanded
        if (declaration["schema"] != "check-declaration/v1" or declaration["producer"] != PRODUCER
                or declaration["producer_version"] != VERSION or config.get("schema") != "jaekit-goal-targets/v2"):
            raise ValueError("unsupported declaration or targets")
        targets = {target["id"]: target for target in declaration["targets"]}
        checks = config["checks"]
        declared_ids = [target["id"] for target in declaration["targets"]]
        selected_ids = [name for check in checks for name in results.target_ids(check["target"], check["result_targets"])]
        if len(set(declared_ids)) != len(declared_ids) or len(set(selected_ids)) != len(selected_ids) or set(selected_ids) != set(targets):
            raise ValueError("targets do not match the declaration")
        observations = []
        for check in checks:
            items = observe(check)
            for observation in items:
                print(observation["target"], observation["status"], flush=True)
            observations.extend(items)
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
