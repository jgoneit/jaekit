"""Validate complete, source-bound NTFS stream preservation observations."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import re

MATRIX = json.loads(Path(__file__).with_name("stream_cases.json").read_text(encoding="utf-8"))
TARGETS = {boundary + "-" + target["id"]: (boundary, target)
           for target in MATRIX["targets"] for boundary in MATRIX["boundaries"]}
NORMAL = set(MATRIX["normal"])
ERRORS = set(MATRIX["errors"])
LATE = set(MATRIX["late"])
STREAM_CASES = set(TARGETS) | NORMAL | ERRORS | LATE
SOURCE_PATHS = ("tools/windows/common.ps1", "tools/windows/install.ps1", "tools/windows/uninstall.ps1",
                "tools/windows/native_stream_checks.ps1", "tools/windows/stream_cases.json", "guides/WINDOWS.en.md")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def digest(value):
    return isinstance(value, str) and re.fullmatch(r"[a-f0-9]{64}", value) is not None


def snapshot(value):
    require(isinstance(value, dict) and digest(value.get("default_sha256")), "invalid default-data snapshot")
    streams = value.get("streams")
    require(isinstance(streams, list), "missing independent stream snapshot")
    names = []
    for row in streams:
        require(isinstance(row, dict) and isinstance(row.get("path"), str)
                and isinstance(row.get("name"), str) and row["name"].startswith(":")
                and row["name"].endswith(":$DATA") and row["name"] != "::$DATA"
                and type(row.get("size")) is int and row["size"] >= 0 and digest(row.get("sha256")),
                "invalid named-stream observation")
        names.append((row["path"], row["name"]))
    require(len(names) == len(set(names)), "duplicate named-stream observation")


def stream_report(report, shell_version, sources):
    require(isinstance(report, dict) and report.get("schema") == "jaekit-windows-streams/v1"
            and report.get("status") == "passed", "stream observations missing or failed")
    require(report.get("shell_version") == shell_version and report.get("filesystem") == "NTFS"
            and report.get("actual_named_stream_roundtrip") is True, "actual same-shell NTFS observation is absent")
    require(report.get("source_sha256") == sources and set(sources) == set(SOURCE_PATHS)
            and all(digest(value) for value in sources.values()), "stream observations bind to other sources")
    for field in ("required", "passed"):
        names = report.get(field)
        require(isinstance(names, list) and all(isinstance(name, str) for name in names)
                and len(names) == len(STREAM_CASES) and set(names) == STREAM_CASES,
                "stream cases missing or duplicated: " + field)
    require(report.get("failures") == [] and report.get("skipped") == [], "stream cases failed or skipped")
    require(digest(report.get("environment_before_sha256")) and report.get("environment_preserved") is True
            and report["environment_before_sha256"] == report.get("environment_after_sha256"),
            "stream fixtures changed real user state")
    snapshot(report.get("unrelated_before")); snapshot(report.get("unrelated_after"))
    require(report.get("unrelated_preserved") is True and report["unrelated_before"] == report["unrelated_after"]
            and report["unrelated_before"]["streams"], "unrelated project data or streams changed")
    rows = report.get("observations")
    require(isinstance(rows, list) and all(isinstance(row, dict) for row in rows), "invalid stream observations")
    ids = [row.get("case") for row in rows]
    require(all(isinstance(name, str) for name in ids) and len(ids) == len(STREAM_CASES)
            and set(ids) == STREAM_CASES, "actual stream observations missing or duplicated")
    for row in rows:
        name = row["case"]
        snapshot(row.get("before")); snapshot(row.get("after"))
        require(row["before"] == row["after"] and row.get("preserved") is True,
                "default data or stream names/bytes changed: " + name)
        require(row.get("outcome_verified") is True, "product outcome was not observed: " + name)
        if name in TARGETS:
            boundary, target = TARGETS[name]
            require(row.get("kind") == "actual_ntfs" and row.get("boundary") == boundary
                    and row.get("rejected") is True and row.get("diagnostic_identifies_target") is True,
                    "wrong actual stream refusal: " + name)
            wanted = {"path": target["path"], "name": ":" + target["stream"] + ":$DATA",
                      "size": len(bytes.fromhex(target["hex"])),
                      "sha256": hashlib.sha256(bytes.fromhex(target["hex"])).hexdigest()}
            require(wanted in row["before"]["streams"], "actual named-stream bytes were not observed: " + name)
            if boundary in ("reinstall", "update", "uninstall"):
                require(type(row.get("exit_code")) is int and row["exit_code"] == 1
                        and row.get("source_inventory_called") is True
                        and type(row.get("path_write_count")) is int and row["path_write_count"] == 0,
                        "operation did not refuse before PATH mutation: " + name)
        elif name in NORMAL:
            require(row.get("kind") == "actual_ntfs" and row.get("rejected") is False,
                    "ordinary operation did not succeed: " + name)
            if name == "read-only-inputs":
                require(row.get("input_stream_count") == 6 and len(row["before"]["streams"]) == 6,
                        "all six read-only candidate inputs must retain download metadata")
        elif name in ERRORS:
            unsupported = name == "unsupported-filesystem"
            require(row.get("kind") == ("injected_unsupported" if unsupported else "injected_error")
                    and row.get("rejected") is (not unsupported)
                    and row.get("injected_branch_observed") is True,
                    "uncertain stream result was accepted or misrepresented: " + name)
        else:
            require(row.get("kind") == "actual_ntfs_injected_stage" and row["before"]["streams"]
                    and row.get("cleanup_preserved") is True and row.get("warning_identifies_preserved_tree") is True,
                    "later cleanup discarded streams or hid its result: " + name)
            committed = name != "staged-stream-refusal"
            require(row.get("committed") is committed and row.get("rejected") is (not committed)
                    and type(row.get("exit_code")) is int and row["exit_code"] == (0 if committed else 1),
                    "applied operation and cleanup refusal were conflated: " + name)
    require({"fixture-platform-root-lock-path", "injected-errors-and-stage-timing",
             "unsupported-filesystem-simulated", "windows11-install-lifecycle"}
            <= set(report.get("unverified", [])), "stream fixture limits are absent")
