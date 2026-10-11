"""Invocation-bound evidence from actual checkout retention probes.

Only RetentionViolation raised at an observed property boundary is a violation.
Fixture, import and process failures remain errors, independent of declaration.
"""
import json
import os
from pathlib import Path
import sys
import traceback

from test_retention import RetentionViolation, linked_success, no_substitution, causes

PROBES = {"AC-1": linked_success, "AC-2": no_substitution, "AC-3": causes}


def main():
    ac = sys.argv[1]
    observation = {"target": ac, "attempt": 1}
    try:
        PROBES[ac]()
        observation["status"] = "pass"
    except RetentionViolation as error:
        observation.update(status="violation", violation=error.code)
        print(str(error), file=sys.stderr)
    except Exception:
        traceback.print_exc()
        observation.update(status="error", reason="execution_error")
    if "HA_EVIDENCE_PATH" in os.environ:
        report = {"schema": "check-result/v1", "producer": "retention-probes", "producer_version": "1",
                  "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
                  "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
                  "attempts_complete": True, "observations": [observation]}
        with Path(os.environ["HA_EVIDENCE_PATH"]).open("x") as output:
            json.dump(report, output)
    print(ac, observation["status"])
    return 0 if observation["status"] == "pass" else 1


if __name__ == "__main__":
    raise SystemExit(main())
