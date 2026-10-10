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


def observe(entry):
    path = ROOT / entry["file"]
    sys.path.insert(0, str(path.parent))
    try:
        spec = importlib.util.spec_from_file_location("gap_" + path.stem, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        suite = unittest.defaultTestLoader.loadTestsFromTestCase(getattr(module, entry["class"]))
        if suite.countTestCases() == 0:
            return {"status": "error", "reason": "collection_error"}
        result = unittest.TextTestRunner(verbosity=2).run(suite)
    except Exception:
        traceback.print_exc()
        return {"status": "error", "reason": "collection_error"}
    if result.errors:
        return {"status": "error", "reason": "execution_error"}
    if result.testsRun == 0:
        return {"status": "error", "reason": "collection_error"}
    if result.expectedFailures or result.unexpectedSuccesses:
        return {"status": "error", "reason": "setup_error"}
    if result.skipped:
        return {"status": "skip", "reason": "skipped"}
    if result.failures:
        return {"status": "violation", "violation": "requirement-not-met"}
    return {"status": "pass"}


def main():
    if len(sys.argv) != 2:
        return 64
    entries = json.loads((HERE / "suites.json").read_text())
    target = sys.argv[1]
    if target not in entries:
        return 64
    result = dict(target=target, attempt=1, **observe(entries[target]))
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "jaekit-gap-regressions",
                  "producer_version": "1", "attempts_complete": True,
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "observations": [result]}
        with open(os.environ["HA_EVIDENCE_PATH"], "x") as output:
            json.dump(report, output, separators=(",", ":"))
            output.write("\n")
    return 0 if result["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
