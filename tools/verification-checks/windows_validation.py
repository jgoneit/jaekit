#!/usr/bin/env python3
"""Observe diagnostic/CLI coverage and live current-source native CI.

Absent regression coverage is a baseline violation of this verification change,
not evidence that every product operation fails. Source presence never passes:
the candidate requires complete live reports from both actual Windows shells.
"""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
PRODUCER = "windows-validation-stability"
VIOLATION = "windows-validation-boundary-missing"
JUNCTION = "TestNativeWindowsBaselineJunctionScenarios"
JUNCTION_CASES = {"regular-before", "parent-junction", "leaf-junction", "partial-batch", "regular-after"}
ACCEPTANCE_CONTROLS = {"refusal-zero-exit", "refusal-missing-diagnostic", "success-nonzero-exit"}


class Violation(Exception):
    pass


def require(condition, message):
    if not condition:
        raise Violation(message)


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    if spec is None or spec.loader is None:
        raise OSError("observation module unavailable")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def digest(path):
    return hashlib.sha256((ROOT / path).read_bytes()).hexdigest()


def feature_boundary(criterion):
    if criterion == "AC-1":
        fixture = (ROOT / "tools/windows/native_stream_checks.ps1").read_text()
        require("diagnostic_controls" in fixture and "target-prefix-collision" in fixture,
                "stream diagnostic acceptance/rejection controls are absent")
    elif criterion == "AC-2":
        results = load("validation_lifecycle_boundary", "tools/windows/lifecycle_results.py")
        require(ACCEPTANCE_CONTROLS <= results.FAILURE_CASES,
                "public acceptance exit/diagnostic negative controls are absent")
    elif criterion == "AC-3":
        policy = load("validation_windows_policy", "tools/workflow-checks/test_windows_shell_conditions.py")
        try:
            policy.check_policy((ROOT / ".github/workflows/ci.yml").read_text())
        except AssertionError as error:
            raise Violation(str(error)) from None
    elif criterion == "AC-4":
        tests = "\n".join(path.read_text() for path in (ROOT / "cmd/ha").glob("*_test.go"))
        require("func " + JUNCTION + "(" in tests, "native junction CLI entrypoint is absent")
        native = (ROOT / "tools/windows/native_checks.py").read_text()
        require(JUNCTION in native, "junction CLI entrypoint is not required by native checks")


def live_reports():
    ci = load("validation_ci", "tools/verification-checks/cross_platform_ci.py")
    reports = []
    original = ci.command

    def observed(argv, *args, **kwargs):
        if argv[:3] == ["gh", "run", "view"] and "--log" in argv:
            # The shared observer already selected this job from the current
            # run attempt. gh run view can silently truncate long JSON lines;
            # fetch the raw job log without that line-scanning renderer.
            repository = argv[argv.index("--repo") + 1]
            job = argv[argv.index("--job") + 1]
            # Preserve escapes in captured raw data; never print the log here.
            output = original(["gh", "api", "--method", "GET", "--allow-escape-sequences",
                               f"repos/{repository}/actions/jobs/{job}/logs"], *args, **kwargs)
            reports.extend(ci.records(output, "JAEKIT_CI_REPORT "))
            return output
        return original(argv, *args, **kwargs)

    ci.command = observed
    try:
        ci.configuration_present()
        repository, _ = ci.live_run()
        require(repository == "jgoneit/jaekit", "wrong repository")
    except ci.Violation as error:
        raise Violation(str(error)) from None
    finally:
        ci.command = original
    windows = [row for row in reports if row.get("schema") == "jaekit-native-windows/v1"]
    require(len(windows) == 2, "two Windows reports required")
    return windows


def validate_boundaries(reports):
    stream = load("validation_stream", "tools/windows/stream_results.py")
    lifecycle = load("validation_lifecycle", "tools/windows/lifecycle_results.py")
    stream_sources = {path: digest(path) for path in stream.SOURCE_PATHS}
    acceptance_sources = {path: digest(path) for path in
                          ("tools/windows/acceptance.ps1", "tools/windows/README.md")}
    for report in reports:
        version = report["environment"]["shell_version"]
        try:
            stream.stream_report(report.get("stream_checks"), version, stream_sources)
            lifecycle.acceptance_report(report.get("acceptance_dispatch"), version, acceptance_sources)
        except ValueError as error:
            raise Violation(str(error)) from None
        native = report["native_tests"]
        for field in ("required", "passed"):
            names = native[field]
            require(all(any(name.endswith("/" + JUNCTION + "/" + case) for name in names)
                        for case in JUNCTION_CASES), "junction CLI cases missing: " + field)
        require(not native.get("skipped"), "native cases were skipped")


def main():
    criterion = sys.argv[1] if len(sys.argv) == 2 else ""
    observation = {"target": criterion, "attempt": 1, "status": "pass"}
    code = 0
    try:
        require(criterion in {"AC-1", "AC-2", "AC-3", "AC-4", "AC-6"}, "unknown condition")
        # Each baseline can observe its own absent boundary independently.
        feature_boundary(criterion)
        if criterion == "AC-6":
            for name in ("AC-1", "AC-2", "AC-3", "AC-4"):
                feature_boundary(name)
        validate_boundaries(live_reports())
    except Violation as error:
        observation.update(status="violation", violation=VIOLATION)
        print(str(error), file=sys.stderr)
        code = 1
    except Exception as error:
        # Collection/authentication/decoding errors never count as an intended
        # violation. No retries or saved CI success can hide this attempt.
        observation.update(status="error", reason="execution_error")
        print(type(error).__name__ + ": " + str(error), file=sys.stderr)
        code = 2
    if criterion != "AC-6" and os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps({
            "schema": "check-result/v1", "producer": PRODUCER, "producer_version": "1",
            "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
            "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
            "attempts_complete": True, "observations": [observation],
        }) + "\n", encoding="utf-8")
    return code


if __name__ == "__main__":
    raise SystemExit(main())
