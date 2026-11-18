#!/usr/bin/env python3
"""cmd-ha-test-speed AC-8 (optional): full verification within 480 seconds.

Usage, from the repository root: python3 tools/goal-checks/cmd-ha-test-speed/verify_time.py

It runs `python3 tools/verify.py` (with this interpreter) and measures its
wall time with a monotonic clock. The verification output streams through.
It is stopped after 1800 seconds.

Exit 0: verification succeeded within 480 seconds. Exit 1: it failed or took
longer. Exit 2: it could not start, was stopped at the cap, or was killed by
a signal. Neither is ever a pass. The time is printed in every case where the
command started. The child does not inherit the HA_EVIDENCE_* or HA_JUNIT_XML
variables.
"""
import os
from pathlib import Path
import subprocess
import sys
import time

ROOT = Path(__file__).resolve().parents[3]
COMMAND = [sys.executable, "tools/verify.py"]
LIMIT = 480.0
CAP = 1800
HIDDEN = ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST", "HA_JUNIT_XML")


def main(command=COMMAND, limit=LIMIT, cap=CAP):
    env = {name: value for name, value in os.environ.items() if name not in HIDDEN}
    start = time.monotonic()
    try:
        result = subprocess.run(command, cwd=ROOT, env=env, timeout=cap)
    except OSError as error:
        print(f"AC-8: not observed: {command[0]} could not start: {error}", file=sys.stderr)
        return 2
    except subprocess.TimeoutExpired:
        print(f"AC-8: not observed: verification was stopped after {time.monotonic() - start:.1f}s (cap {cap}s)",
              file=sys.stderr)
        return 2
    elapsed = time.monotonic() - start
    print(f"AC-8: {' '.join(command[1:])} exited {result.returncode} after {elapsed:.1f}s (limit {limit:.0f}s)")
    if result.returncode < 0:
        print(f"AC-8: not observed: killed by signal {-result.returncode}", file=sys.stderr)
        return 2
    if result.returncode:
        print("AC-8: fail: verification failed")
        return 1
    if elapsed > limit:
        print(f"AC-8: fail: verification took {elapsed:.1f}s, more than {limit:.0f}s")
        return 1
    print("AC-8: pass")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
