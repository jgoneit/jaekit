#!/usr/bin/env python3
"""Repository acceptance checks, not a general-purpose Go test adapter.

Only an executed test's explicit [requirement] assertion is a declared
violation. Compilation, missing selection, skips, crashes and fixture failures
cannot provide a successful baseline. Go JSON and stdout remain in the log.
"""
import json
import os
from pathlib import Path
import re
import subprocess
import sys


def classify(events, exit_code):
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
    target = declaration["targets"][0]
    command = ["go", "test", "-json", "-count=1", package, "-run", selection]
    try:
        result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE,
                                text=True, timeout=600)
        print(result.stdout, end="")
        print(result.stderr, end="", file=sys.stderr)
        events = [json.loads(line) for line in result.stdout.splitlines() if line.strip()]
        state, reason = classify(events, result.returncode)
    except (OSError, subprocess.TimeoutExpired, ValueError):
        state, reason = "error", "execution_error"
    observation = {"target": target["id"], "attempt": 1, "status": state}
    if state == "violation":
        observation["violation"] = target["violation"]
    elif reason:
        observation["reason"] = reason
    report = {
        "schema": "check-result/v1",
        "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
        "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
        "producer": declaration["producer"],
        "producer_version": declaration["producer_version"],
        "attempts_complete": True,
        "observations": [observation],
    }
    Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps(report) + "\n")
    return 0 if state == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
