#!/usr/bin/env python3
"""Report one review regression suite without treating runner errors as violations.

Usage: python3 tools/review-checks/produce.py AC-1
The unittest runner executes each case once. Actual assertion identities,
collection/runtime errors and skipped cases have separate declared targets.
"""
import importlib.util
import json
import os
from pathlib import Path
import sys
import traceback
import unittest

ROOT = Path(__file__).resolve().parents[2]
HERE = Path(__file__).resolve().parent
PRODUCER = "jaekit-review-regressions"
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT / "tools"))
import result_observations as results


def run_suite(entry, detailed=False):
    file = ROOT / entry["file"]
    sys.path.insert(0, str(file.parent))
    try:
        spec = importlib.util.spec_from_file_location("review_" + file.stem, file)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        case = getattr(module, entry["class"])
        suite = unittest.defaultTestLoader.loadTestsFromTestCase(case)
        result = results.run_suite(suite, sys.stderr)
    except Exception:
        traceback.print_exc()
        result = results.failure("collection_error")
    return result if detailed else results.summary(result)


def main():
    if len(sys.argv) != 2:
        return 64
    entries = json.loads((HERE / "suites.json").read_text())
    criterion = sys.argv[1]
    if criterion not in entries:
        return 64
    entry = entries[criterion]
    names = entry["result_targets"]
    results.target_ids(criterion, names)
    facts = run_suite(entry, detailed=True)
    print(json.dumps({"target": criterion, "facts": facts}), file=sys.stderr)
    observations = results.observations(facts, criterion, names)
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "producer": PRODUCER, "producer_version": "2",
                  "attempts_complete": True, "observations": observations}
        with open(os.environ["HA_EVIDENCE_PATH"], "x") as output:
            json.dump(report, output, separators=(",", ":"))
            output.write("\n")
    return 0 if all(item["status"] == "pass" for item in observations) else 1


if __name__ == "__main__":
    raise SystemExit(main())
