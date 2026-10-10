#!/usr/bin/env python3
"""Report one review regression suite without treating runner errors as violations.

Usage: python3 tools/review-checks/produce.py AC-1
The unittest runner executes each case once. Only assertion failures are
declared violations; collection/runtime errors and skips remain distinct.
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


def run_suite(entry):
    file = ROOT / entry["file"]
    sys.path.insert(0, str(file.parent))
    try:
        spec = importlib.util.spec_from_file_location("review_" + file.stem, file)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        case = getattr(module, entry["class"])
        suite = unittest.defaultTestLoader.loadTestsFromTestCase(case)
        if suite.countTestCases() == 0:
            return {"status": "skip", "reason": "not_selected"}
        result = unittest.TextTestRunner(verbosity=2).run(suite)
    except Exception:
        traceback.print_exc()
        return {"status": "error", "reason": "collection_error"}
    if result.errors:
        return {"status": "error", "reason": "execution_error"}
    if result.skipped:
        return {"status": "skip", "reason": "skipped"}
    if result.failures or result.unexpectedSuccesses:
        return {"status": "violation", "violation": "requirement-not-met"}
    if result.expectedFailures:
        return {"status": "error", "reason": "setup_error"}
    return {"status": "pass"}


def main():
    if len(sys.argv) != 2:
        return 64
    entries = json.loads((HERE / "suites.json").read_text())
    criterion = sys.argv[1]
    if criterion not in entries:
        return 64
    observation = dict(target=criterion, attempt=1, **run_suite(entries[criterion]))
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "producer": PRODUCER, "producer_version": "1",
                  "attempts_complete": True, "observations": [observation]}
        with open(os.environ["HA_EVIDENCE_PATH"], "x") as output:
            json.dump(report, output, separators=(",", ":"))
            output.write("\n")
    return 0 if observation["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
