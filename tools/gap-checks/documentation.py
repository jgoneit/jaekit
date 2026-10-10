#!/usr/bin/env python3
"""Public documentation consistency and established observer-document contracts."""
import os
import subprocess
import sys


def main():
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
    for key in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
        env.pop(key, None)
    commands = [
        [sys.executable, "tools/check_public_docs.py"],
        [sys.executable, "tools/release-checks/test_loader_invocations.py", "Documentation"],
    ]
    for command in commands:
        result = subprocess.run(command, env=env)
        if result.returncode:
            return result.returncode
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
