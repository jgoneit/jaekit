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
import subprocess
import sys
import importlib.util

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
import result_observations as results

PRODUCER = "repository-go-check"
VERSION = "2"


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
