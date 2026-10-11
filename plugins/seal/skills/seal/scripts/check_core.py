#!/usr/bin/env python3
"""Optional, read-only consumer of the Seal/Core compatibility contract.

Agents can consume the same JSON contract directly; this helper does not make
Python a prerequisite for Seal. It never starts/resumes a goal or installs tools.
"""
from __future__ import annotations

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys

KNOWN_RULES = {"run-rules/1", "run-rules/2", "run-rules/3"}
FORMATS = {
    "check_declaration": "check-declaration/v1",
    "check_result": "check-result/v1",
    "check_evidence": "check-evidence/v1",
}
EXECUTION_FEATURES = {"dirty-preflight/v1", "baseline-safe-copy/v1"}
START_FEATURES = EXECUTION_FEATURES | {
    "estimate/v1", "structured-change-results/v1", "budget-change/v1",
}


class Refusal(Exception):
    def __init__(self, reason: str, missing: list[str] | None = None):
        self.reason, self.missing = reason, missing or []


def pairs(items):
    result = {}
    for key, value in items:
        if key in result:
            raise ValueError("duplicate JSON key")
        result[key] = value
    return result


def decode(text: str):
    return json.loads(text, object_pairs_hook=pairs,
                      parse_constant=lambda _: (_ for _ in ()).throw(ValueError("invalid JSON number")))


def strings(value) -> bool:
    return (isinstance(value, list) and all(isinstance(x, str) and x for x in value)
            and len(value) == len(set(value)))


def validate(value):
    if not isinstance(value, dict):
        raise Refusal("invalid_capabilities")
    if value.get("schema") != "ha-capabilities/v1":
        raise Refusal("unsupported_schema")
    for key in ("ha_version", "default_rules"):
        if not isinstance(value.get(key), str) or not value[key]:
            raise Refusal("invalid_capabilities", [key])
    for key in ("supported_rules", "features"):
        if not strings(value.get(key)):
            raise Refusal("invalid_capabilities", [key])
    if value["default_rules"] not in value["supported_rules"]:
        raise Refusal("invalid_capabilities", ["default_rules not supported"])
    formats = value.get("formats")
    if not isinstance(formats, dict):
        raise Refusal("invalid_capabilities", ["formats"])
    for key in ("criteria", *FORMATS):
        if not strings(formats.get(key)):
            raise Refusal("invalid_capabilities", ["formats:" + key])

    if "finding" in formats and not strings(formats["finding"]):
        raise Refusal("invalid_capabilities", ["formats:finding"])
    return value


def digest(path: Path) -> str:
    result = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            result.update(chunk)
    return result.hexdigest()


def selected_format(source: str) -> str:
    """Read only the explicit selector; Core still validates the goal itself."""
    selected, in_section, fence = [], False, None
    for line in source.replace("\r\n", "\n").split("\n"):
        marker = re.match(r"^[ \t]*(`{3,}|~{3,})(.*)$", line)
        if fence:
            if marker and marker[1][0] == fence[0] and len(marker[1]) >= len(fence) and not marker[2].strip(" \t"):
                fence = None
            continue
        if marker and (marker[1][0] == "~" or "`" not in marker[2]):
            fence = marker[1]
            continue
        if line.startswith("## "):
            in_section = True
        if line.startswith("Criteria-Format:"):
            if in_section:
                raise Refusal("goal_format_invalid")
            selected.append(line.partition(":")[2].strip())
        elif not in_section and line.lstrip(" \t").startswith("Criteria-Format:"):
            raise Refusal("goal_format_invalid")
    if not selected:
        return "legacy"
    if selected != ["nested/1"]:
        raise Refusal("goal_format_invalid")
    return selected[0]


def stored_goal(goal: str):
    try:
        path = Path(goal)
        with (path / "runs.jsonl").open(encoding="utf-8") as source:
            first = source.readline(1024 * 1024 + 1)
            finding = False
            for raw in source:
                if len(raw) > 1024 * 1024:
                    raise ValueError("oversized record")
                event = decode(raw)
                if not isinstance(event, dict):
                    raise ValueError("invalid record")
                schema = event.get("schema")
                if schema == "run-finding/v1":
                    finding = True
                elif schema != "run/v1":
                    raise Refusal("unsupported_record_schema")
        if len(first) > 1024 * 1024:
            raise ValueError("oversized start")
        start = decode(first)
        if not isinstance(start, dict) or start.get("schema") != "run/v1" or start.get("kind") != "start":
            raise ValueError("missing start")
        rules = start.get("rules")
        if not isinstance(rules, str) or not rules:
            raise ValueError("missing rules")
        criteria = selected_format((path / "SPEC.md").read_text(encoding="utf-8"))
        return rules, criteria, finding
    except (OSError, UnicodeError, ValueError, RecursionError):
        raise Refusal("goal_unreadable") from None


def requirements(capabilities, operation: str, goal: str | None):
    finding = operation == "finding"
    if operation == "start":
        rules, criteria = capabilities["default_rules"], {"legacy", "nested/1"}
        if rules != "run-rules/3":
            raise Refusal("unsupported_default_rule", [rules])
        features = START_FEATURES
    else:
        rules, criteria_format, has_findings = stored_goal(goal)
        finding = finding or has_findings
        criteria = {criteria_format}
        features = (EXECUTION_FEATURES if operation in ("resume", "check")
                    else {"budget-change/v1"} if operation == "budget" else set())
        if rules not in KNOWN_RULES or rules not in capabilities["supported_rules"]:
            raise Refusal("unsupported_stored_rule", [rules])
        if operation == "budget" and rules != "run-rules/3":
            raise Refusal("unsupported_operation", ["budget:" + rules])
        if rules == "run-rules/3" and operation in ("resume", "check"):
            features = features | {"structured-change-results/v1"}
    required_formats = {"criteria": criteria}
    if rules == "run-rules/3" and operation != "finding":
        required_formats.update({key: {value} for key, value in FORMATS.items()})
    if finding:
        features = features | {"post-completion-findings/v1"}
        required_formats["finding"] = {"run-finding/v1"}
    missing = ["features:" + key for key in sorted(features - set(capabilities["features"]))]
    for kind, values in required_formats.items():
        missing += ["formats:" + kind + ":" + key
                    for key in sorted(values - set(capabilities["formats"].get(kind, [])))]
    if missing:
        raise Refusal("missing_support", missing)
    return rules


def check(ha: str, operation: str, goal: str | None):
    report = {"schema": "seal-core-compatibility/v1", "compatible": False,
              "requested_executable": ha, "executable": None, "sha256": None,
              "ha_version": None, "operation": operation, "rules": None, "missing": []}
    try:
        found = shutil.which(ha)
        if not found:
            raise Refusal("core_missing")
        target = Path(found).resolve(strict=True)
        if not target.is_file() or not os.access(target, os.X_OK):
            raise Refusal("core_missing")
        report["executable"] = str(target)
        report["sha256"] = before = digest(target)
        try:
            result = subprocess.run([str(target), "capabilities", "--format", "json"],
                                    capture_output=True, text=True, timeout=5)
        except subprocess.TimeoutExpired:
            raise Refusal("query_timeout") from None
        if digest(target) != before:
            raise Refusal("binary_changed")
        if result.returncode:
            raise Refusal("query_unsupported" if result.returncode == 64 else "query_failed")
        try:
            capabilities = validate(decode(result.stdout))
        except (ValueError, UnicodeError, RecursionError):
            raise Refusal("invalid_capabilities") from None
        report["ha_version"] = capabilities["ha_version"]
        report["rules"] = requirements(capabilities, operation, goal)
        report.update(compatible=True, reason="compatible")
    except Refusal as error:
        report.update(reason=error.reason, missing=error.missing)
    except (OSError, UnicodeError):
        report.update(reason="query_failed")
    return report


class Parser(argparse.ArgumentParser):
    def error(self, message):
        self.print_usage(sys.stderr)
        self.exit(64, message + "\n")


def main() -> int:
    parser = Parser(description=__doc__)
    parser.add_argument("--ha", default="ha")
    parser.add_argument("--operation", choices=("start", "resume", "check", "budget", "finding"), default="start")
    parser.add_argument("--goal")
    args = parser.parse_args()
    if (args.operation == "start") == (args.goal is not None):
        parser.error("start takes no --goal; resume/check/budget/finding require --goal")
    report = check(args.ha, args.operation, args.goal)
    print(json.dumps(report, sort_keys=True))
    return 0 if report["compatible"] else 2


if __name__ == "__main__":
    raise SystemExit(main())
