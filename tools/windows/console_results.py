"""Require source-bound native observations of the conservative package policy."""
from __future__ import annotations

import json
from pathlib import Path
import re

MATRIX = json.loads(Path(__file__).with_name("console_cases.json").read_text(encoding="utf-8"))
BOUNDARIES = ("name", "join", "manifest", "inventory", "copy", "archive")
REJECTED = {boundary + "-" + item["id"]: (boundary, item["path"])
            for item in MATRIX["rejected"] for boundary in BOUNDARIES}
ALLOWED = {"allowed-" + item["id"]: item["path"] for item in MATRIX["allowed"]}
NORMAL = {"normal-manifest", "normal-inventory", "normal-copy", "normal-archive"}
ORCHESTRATION = {"install-archive-refusal", "reinstall-inventory-refusal",
                 "update-inventory-refusal", "uninstall-inventory-refusal"}
CONSOLE_CASES = set(REJECTED) | set(ALLOWED) | NORMAL | ORCHESTRATION
SOURCE_PATHS = ("tools/windows/package.py", "tools/windows/common.ps1",
                "tools/windows/install.ps1", "tools/windows/uninstall.ps1")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def console_report(report, shell_version, sources):
    require(isinstance(report, dict), "console path report is not an object")
    require(report.get("schema") == "jaekit-windows-console-paths/v1"
            and report.get("status") == "passed", "console path observations missing or failed")
    require(report.get("shell_version") == shell_version, "console paths ran in another shell")
    require(report.get("source_sha256") == sources and set(sources) == set(SOURCE_PATHS),
            "console paths observed different product sources")
    for field in ("required", "passed"):
        names = report.get(field)
        require(isinstance(names, list) and all(isinstance(x, str) for x in names)
                and len(names) == len(CONSOLE_CASES) and set(names) == CONSOLE_CASES,
                "console path cases missing or duplicated: " + field)
    require(report.get("failures") == [] and report.get("skipped") == [], "console path cases failed or skipped")
    before = report.get("environment_before_sha256", "")
    require(isinstance(before, str) and re.fullmatch(r"[a-f0-9]{64}", before)
            and before == report.get("environment_after_sha256")
            and report.get("environment_preserved") is True, "console fixtures changed user state")
    rows = report.get("observations")
    require(isinstance(rows, list) and all(isinstance(row, dict) for row in rows), "invalid console observations")
    ids = [row.get("case") for row in rows]
    require(all(isinstance(x, str) for x in ids) and len(ids) == len(CONSOLE_CASES)
            and set(ids) == CONSOLE_CASES, "actual console observations missing or duplicated")
    for row in rows:
        name = row["case"]
        require(row.get("preserved") is True, "console fixture data changed: " + name)
        if name in REJECTED:
            boundary, path = REJECTED[name]
            require(row.get("rejected") is True and row.get("boundary") == boundary
                    and row.get("path") == path, "wrong path or refusal boundary: " + name)
            if boundary == "archive":
                require(type(row.get("entry_open_count")) is int and row["entry_open_count"] == 0
                        and row.get("destination_absent") is True,
                        "archive reached a data stream or destination: " + name)
        elif name in ALLOWED:
            require(row.get("rejected") is False and row.get("path") == ALLOWED[name],
                    "ordinary package name was not accepted: " + name)
        elif name in ORCHESTRATION:
            require(row.get("rejected") is True and type(row.get("exit_code")) is int and row["exit_code"] == 1
                    and type(row.get("write_boundary_count")) is int and row["write_boundary_count"] == 0
                    and type(row.get("entry_open_count")) is int and row["entry_open_count"] == 0
                    and row.get("source_inventory_called") is True,
                    "product operation did not refuse before mutation: " + name)
        else:
            require(row.get("rejected") is False, "ordinary package boundary failed: " + name)
    require({"fixture-platform-root-lock", "archive-entry-open-observer", "windows11-install-lifecycle"}
            <= set(report.get("unverified", [])), "console fixture scope is not explicit")
