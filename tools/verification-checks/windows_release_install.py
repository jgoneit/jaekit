#!/usr/bin/env python3
"""Observe the Windows public release and paste-once installation conditions.

Each change condition first observes, in tracked files, whether the behavior
exists at all. Its absence is the declared baseline violation. Present behavior
is then observed through local regressions and the current HEAD's release
workflow run. Missing GitHub access is an observation error, never a violation.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
DECLARATIONS = ROOT / "tools/verification-checks/windows-release-install"


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(value, message):
    if not value:
        raise Violation(message)


def text(path):
    try:
        return (ROOT / path).read_text(encoding="utf-8")
    except OSError:
        return ""


def present(criterion):
    """Observe the tracked files that must exist before anything dynamic runs."""
    workflow = text(".github/workflows/release.yml")
    guide = text("guides/WINDOWS.md") + text("guides/WINDOWS.en.md")
    if criterion in {"AC-1", "AC-3", "AC-10", "AC-11"}:
        require("GOOS=windows" in workflow and "tools/windows/package.py" in workflow,
                "release workflow builds no Windows archive")
    if criterion == "AC-2":
        require("windows_amd64.zip" in text("tools/release/check_archives.py"),
                "pre-publication checks do not verify a Windows archive")
    if criterion in {"AC-3", "AC-11"}:
        require("windows-2025" in workflow and "needs:" in workflow,
                "publication does not wait for a Windows smoke job")
    if criterion in {"AC-4", "AC-5", "AC-6", "AC-7", "AC-9"}:
        require("<!-- BEGIN WINDOWS PUBLIC INSTALL -->" in guide,
                "the Windows guides have no public installation block")
    if criterion == "AC-7":
        require("<!-- BEGIN WINDOWS PUBLIC REMOVE -->" in guide,
                "the Windows guides have no public removal block")
    if criterion in {"AC-8", "AC-10"}:
        require((ROOT / "tools/windows/public_blocks.py").is_file(),
                "no harness executes the public blocks")
    if criterion == "AC-11":
        require((ROOT / "tools/release/windows_gate.py").is_file(),
                "no gate links published Windows archives to Windows 11 acceptance")


def observe(criterion):
    present(criterion)
    raise ObservationError("dynamic observation is not available yet")


def maintain(criterion):
    raise ObservationError("maintain observation is not available yet")


def main():
    criterion = sys.argv[1] if len(sys.argv) == 2 else ""
    declaration = DECLARATIONS / f"{criterion}.json"
    structured = declaration.is_file()
    violation = ""
    if structured:
        violation = json.loads(declaration.read_text(encoding="utf-8"))["targets"][0]["violation"]
    observation = {"target": criterion, "attempt": 1, "status": "pass"}
    code = 0
    try:
        if structured:
            observe(criterion)
        elif criterion in {"AC-12", "AC-13", "AC-14", "AC-15"}:
            maintain(criterion)
        else:
            raise ObservationError("unknown criterion")
    except Violation as error:
        observation.update(status="violation", violation=violation)
        print(str(error), file=sys.stderr)
        code = 1
    except ObservationError as error:
        observation.update(status="error", reason="execution_error")
        print(str(error), file=sys.stderr)
        code = 2
    if structured and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": "windows-release-install", "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
