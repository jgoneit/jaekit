"""Run one observation of the checked tree's contract producers on another tree.

Usage:
  observe.py authoring ROOT CRITERION      Seal check-authoring producer
  observe.py contract ROOT SUITE CRITERION contract check (check.py's suites)
  observe.py clause ROOT NAME              the text a clause on NAME is looked up in

Prints one JSON object. The producers are the ones beside this file's tree
(tools/contract-checks), so a baseline observes the base producers. Any
exception propagates and the caller treats the non-zero exit as an input
failure, never as a result.
"""
import json
from pathlib import Path
import sys

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / "contract-checks"))
import check  # noqa: E402
# Each producer adds its suite to check.REQUIREMENTS and chains check.observe.
import design_check  # noqa: E402,F401
import finding_docs_check  # noqa: E402,F401
import spec_check  # noqa: E402,F401
import authoring_check  # noqa: E402


def main():
    mode, root = sys.argv[1], Path(sys.argv[2])
    if mode == "authoring" and len(sys.argv) == 4:
        observation, problems = check.observe(root, authoring_check.SUITE, sys.argv[3])
    elif mode == "contract" and len(sys.argv) == 5:
        observation, problems = check.observe(root, sys.argv[3], sys.argv[4])
    elif mode == "clause" and len(sys.argv) == 4:
        print(json.dumps({"text": check.clause_text(root, sys.argv[3])}))
        return 0
    else:
        print(__doc__, file=sys.stderr)
        return 64
    print(json.dumps({"status": observation["status"], "problems": problems}))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
