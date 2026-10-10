#!/usr/bin/env python3
"""Structured observations from one selected regression class, without retries."""
import importlib.util
import json
import os
from pathlib import Path
import sys
import traceback
import unittest

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT / "tools"))
import result_observations as results


def observe(entry, detailed=False):
    path = ROOT / entry["file"]
    sys.path.insert(0, str(path.parent))
    try:
        spec = importlib.util.spec_from_file_location("gap_" + path.stem, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        suite = unittest.defaultTestLoader.loadTestsFromTestCase(getattr(module, entry["class"]))
        result = results.run_suite(suite, sys.stderr)
    except Exception:
        traceback.print_exc()
        result = results.failure("collection_error")
    return result if detailed else results.summary(result)


def main():
    if len(sys.argv) != 2:
        return 64
    entries = json.loads((HERE / "suites.json").read_text())
    target = sys.argv[1]
    if target not in entries:
        return 64
    entry = entries[target]
    names = entry["result_targets"]
    results.target_ids(target, names)
    facts = observe(entry, detailed=True)
    print(json.dumps({"target": target, "facts": facts}), file=sys.stderr)
    observations = results.observations(facts, target, names)
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "jaekit-gap-regressions",
                  "producer_version": "2", "attempts_complete": True,
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "observations": observations}
        with open(os.environ["HA_EVIDENCE_PATH"], "x") as output:
            json.dump(report, output, separators=(",", ":"))
            output.write("\n")
    return 0 if all(item["status"] == "pass" for item in observations) else 1


if __name__ == "__main__":
    raise SystemExit(main())
