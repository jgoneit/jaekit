#!/usr/bin/env python3
"""Repository acceptance checks, not a general-purpose Go test adapter.

The CLI reports independent facts using the shared producer semantics. Go
output cannot identify which assertion failed; fail events retain a generic
violation and an attribution error. Output tags never prove expected failure.
Compilation, missing selection, skips and crashes cannot establish baseline.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import importlib.util

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import result_observations as results

PRODUCER = "repository-go-check"
VERSION = "2"


def classify(events, exit_code):
    """Historical v1 debug helper retained only for legacy regression fixtures.

    Its lossy output-marker heuristic is not an evidence API and is never used
    by the CLI. Existing fixtures preserve its historical limitations; new
    evidence regressions must execute the CLI and inspect all fact targets.
    """
    tests = {}
    broken = False
    for event in events:
        if any(word in event.get("Output", "") for word in (
            "panic:", "fatal error:", "WARNING: DATA RACE")):
            broken = True
        key = (event.get("Package", ""), event.get("Test", ""))
        if not key[1]:
            if event.get("Action") == "build-fail":
                broken = True
            continue
        item = tests.setdefault(key, {"run": False, "end": None, "output": ""})
        if event.get("Action") == "run":
            item["run"] = True
        if event.get("Action") in ("pass", "fail", "skip"):
            item["end"] = event["Action"]
        item["output"] += event.get("Output", "") + "\n"
    if broken or exit_code not in (0, 1):
        return "error", "execution_error"
    if not tests:
        return ("error", "compile_error") if exit_code else ("skip", "not_selected")
    if any(not v["run"] or v["end"] is None for v in tests.values()):
        return "error", "execution_error"
    if any(v["end"] == "skip" for v in tests.values()):
        return "skip", "skipped"
    failed = [k for k, v in tests.items() if v["end"] == "fail"]
    for k in failed:
        for line in tests[k]["output"].splitlines():
            if re.match(r"^\s*\S+\.go:\d+:\s", line) and "[requirement]" not in line:
                return "error", "setup_error"
    # A parent may summarize failed children AND fail in its own fixture.
    # Only the harness's summary lines can be ignored, never its diagnostics.
    leaves = [k for k in failed if not any(
        q[0] == k[0] and q[1].startswith(k[1] + "/") for q in failed)]
    for k in set(failed) - set(leaves):
        diagnostics = [line for line in tests[k]["output"].splitlines()
                       if line.strip() and not re.match(
                           r"^\s*(?:=== (?:RUN|PAUSE|CONT)\b|--- (?:FAIL|PASS|SKIP):)", line)]
        if diagnostics and not any("[requirement]" in line for line in diagnostics):
            return "error", "setup_error"
    if leaves:
        if exit_code != 1:
            return "error", "execution_error"
        for k in leaves:
            output = tests[k]["output"]
            if "[requirement]" not in output or any(word in output for word in (
                "panic:", "fatal error:", "WARNING: DATA RACE")):
                return "error", "setup_error"
        return "violation", None
    return ("pass", None) if exit_code == 0 else ("error", "execution_error")


def main():
    declaration_path, package, selection = sys.argv[1:]
    declaration = json.loads(Path(declaration_path).read_text())
    if (declaration.get("schema") != "check-declaration/v1"
            or declaration.get("producer") != PRODUCER
            or declaration.get("producer_version") != VERSION):
        print("unsupported Go producer declaration", file=sys.stderr)
        return 2
    target = declaration["targets"][0]
    names = results.target_names(target["id"])
    ids = [item["id"] for item in declaration["targets"]]
    if len(ids) != len(set(ids)) or set(results.target_ids(target["id"], names)) != set(ids):
        print("unsupported legacy single-target result declaration; use independent v2 result targets", file=sys.stderr)
        return 2
    command = ["go", "test", "-json", "-count=1", package, "-run", selection]
    try:
        spec = importlib.util.spec_from_file_location("goal_producer", Path(__file__).with_name("produce.py"))
        producer = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(producer)
        error, finals, outputs = producer.go_events(command, Path.cwd())
        observed = producer.go_facts(error, finals, outputs, Path.cwd() / package)
    except (OSError, subprocess.TimeoutExpired, ValueError):
        observed = results.failure("execution_error")
    print(json.dumps({"target": target["id"], "facts": observed}), file=sys.stderr)
    observations = results.observations(observed, target["id"], names)
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
        output.write(json.dumps(report) + "\n")
    return 0 if all(item["status"] == "pass" for item in observations) else 1


if __name__ == "__main__":
    raise SystemExit(main())
