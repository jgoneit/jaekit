#!/usr/bin/env python3
"""Reference check-result/v1 producer using only the Python standard library.

Usage: python3 tools/check-result-reference.py declaration.json checks.json
The process reads real files; it does not turn fixture verdicts into evidence.
"""

import json
import os
from pathlib import Path
import stat
import sys


def local_file(name):
    """Only read regular files inside cwd, without following symlinks."""
    if not isinstance(name, str) or not name or "\\" in name:
        raise ValueError("invalid path")
    path = Path(name)
    if not path.parts or path.is_absolute() or any(p in ("..", ".git") for p in path.parts):
        raise ValueError("invalid path")
    current = Path.cwd()
    for index, part in enumerate(path.parts):
        current /= part
        mode = current.lstat().st_mode
        if stat.S_ISLNK(mode) or (index < len(path.parts) - 1 and not stat.S_ISDIR(mode)):
            raise ValueError("unsafe path")
    if not stat.S_ISREG(mode):
        raise ValueError("not a regular file")
    return current


def observe(check, declared):
    observation = {"target": check["target"], "attempt": 1}
    try:
        if check.get("violation") != declared["violation"] or check.get("missing", "error") not in ("error", "violation") or not isinstance(check.get("equals"), str):
            raise ValueError("invalid check configuration")
        for requirement in check.get("requires", []):
            local_file(requirement)
        try:
            content = local_file(check["path"]).read_text(encoding="utf-8")
            passed = content == check["equals"]
        except FileNotFoundError:
            if check.get("missing", "error") != "violation":
                raise
            passed = False
        if passed:
            observation["status"] = "pass"
        else:
            observation.update(status="violation", violation=declared["violation"])
    except FileNotFoundError:
        observation.update(status="error", reason="dependency_missing")
    except PermissionError:
        observation.update(status="error", reason="permission_denied")
    except (OSError, UnicodeError, ValueError, TypeError, KeyError):
        observation.update(status="error", reason="setup_error")
    return observation


def main():
    if len(sys.argv) != 3:
        print("usage: check-result-reference.py declaration.json checks.json", file=sys.stderr)
        return 2
    try:
        declaration = json.loads(local_file(sys.argv[1]).read_text(encoding="utf-8"))
        config = json.loads(local_file(sys.argv[2]).read_text(encoding="utf-8"))
        if declaration["schema"] != "check-declaration/v1" or declaration["producer"] != "jaekit-reference" or declaration["producer_version"] != "1":
            raise ValueError("unsupported declaration")
        targets = {target["id"]: target for target in declaration["targets"]}
        checks = config["checks"]
        if len(checks) != len(targets) or {check["target"] for check in checks} != set(targets):
            raise ValueError("configuration targets do not match declaration")
        observations = [observe(check, targets[check["target"]]) for check in checks]
        report = {
            "schema": "check-result/v1",
            "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
            "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
            "producer": declaration["producer"],
            "producer_version": declaration["producer_version"],
            "attempts_complete": True,
            "observations": observations,
        }
        # Exclusive creation prevents this reference producer from reusing an
        # earlier report. The caller supplies a fresh private invocation path.
        with open(os.environ["HA_EVIDENCE_PATH"], "x", encoding="utf-8") as output:
            json.dump(report, output, separators=(",", ":"))
            output.write("\n")
        for observation in observations:
            print(observation["target"], observation["status"])
        return 0 if all(o["status"] == "pass" for o in observations) else 1
    except (OSError, ValueError, TypeError, KeyError):
        print("reference check configuration or report transport failed", file=sys.stderr)
        return 2


if __name__ == "__main__":
    raise SystemExit(main())
