"""Require complete, source-bound observations of Windows lifecycle boundaries."""
from __future__ import annotations

import re


STAGES = {
    "first-install": ("first", "install", "5.1"),
    "first-reinstall": ("first", "install", "7."),
    "checksum-refusal": ("first", "install", "5.1"),
    "update-unmanaged-refusal": ("update", "install", "7."),
    "update-locked-refusal": ("update", "install", "7."),
    "update-install": ("update", "install", "7."),
    "update-reinstall": ("update", "install", "5.1"),
    "update-remove": ("update", "uninstall", "5.1"),
    "first-install-again": ("first", "install", "7."),
    "first-remove": ("first", "uninstall", "7."),
    "junction-install-refusal": ("first", "install", "5.1"),
    "junction-remove-refusal": ("first", "uninstall", "7."),
}
FAILURE_STAGES = {
    "update-installer-failure": "update-unmanaged-refusal",
    "update-helper-failure": "update-unmanaged-refusal",
    "update-uninstaller-failure": "update-remove",
    "missing-update-script": "binding",
    "wrong-update-script": "binding",
    "shell-start-failure": "first-install",
    "unexpected-refusal-diagnostic": "checksum-refusal",
    "embedded-pipe-refusal": "checksum-refusal",
    "refusal-zero-exit": "checksum-refusal",
    "refusal-missing-diagnostic": "checksum-refusal",
    "success-nonzero-exit": "first-install",
}
FAILURE_CASES = set(FAILURE_STAGES)
ACCEPTANCE_CASES = {"dispatch-" + name for name in STAGES} | FAILURE_CASES
PATH_REMOVAL_CASES = {
    "first-exact-order", "multiple-later-duplicates", "unowned-exact-preserved",
    "owned-literal-missing", "alias-preserved", "other-spellings-preserved",
    "empty-segments-preserved", "only-owned-entry", "empty-path-preserved",
}


PATH_CASES = PATH_REMOVAL_CASES | {
    "process-remove-" + name for name in PATH_REMOVAL_CASES if name != "unowned-exact-preserved"
} | {"process-add-existing"} | {"uninstall-" + name for name in PATH_REMOVAL_CASES} | {
    "uninstall-owned-preexisting-empty", "uninstall-already-absent-value",
    "uninstall-string-kind-preserved", "uninstall-concurrent-value",
    "uninstall-concurrent-kind", "uninstall-concurrent-existence",
}


def require(condition: bool, message: str) -> None:
    if not condition:
        raise ValueError(message)


def names(value, expected, label):
    require(isinstance(value, list) and all(isinstance(x, str) for x in value), label + " is not a name list")
    require(len(value) == len(expected) and set(value) == set(expected), label + " missing or duplicate cases")


def common(report, schema, shell_version, cases, sources):
    require(isinstance(report, dict), "lifecycle report is not an object")
    require(report.get("schema") == schema and report.get("status") == "passed", "lifecycle report missing or failed")
    require(report.get("shell_version") == shell_version, "lifecycle report came from another shell")
    require(report.get("skipped") == [] and report.get("failures") == [], "lifecycle report skipped or failed observations")
    for field in ("required", "passed"):
        names(report.get(field), cases, schema + ":" + field)
    require(report.get("source_sha256") == sources, "lifecycle observations use different product source")
    require(report.get("environment_preserved") is True, "lifecycle fixture changed user state")


def acceptance_report(report, shell_version, sources):
    common(report, "jaekit-windows-acceptance-dispatch/v1", shell_version, ACCEPTANCE_CASES, sources)
    before = report.get("environment_before_sha256", "")
    require(isinstance(before, str) and re.fullmatch(r"[a-f0-9]{64}", before)
            and before == report.get("environment_after_sha256"), "acceptance user-state preservation missing")
    functions = report.get("source_function_sha256", {})
    require(isinstance(functions, dict) and set(functions) == {
        "Get-JaekitAcceptanceSteps", "Invoke-JaekitAcceptanceStep"}, "production function identities missing")
    require(all(isinstance(v, str) and re.fullmatch(r"[a-f0-9]{64}", v) for v in functions.values()),
            "invalid production function digest")
    require(report.get("production_stages") == list(STAGES), "production stage order differs")
    rows = report.get("stages", [])
    require(isinstance(rows, list) and all(isinstance(r, dict) for r in rows), "invalid stage observations")
    names([row.get("stage") for row in rows], STAGES, "actual stages")
    for row in rows:
        candidate, action, version = STAGES[row["stage"]]
        require(row.get("candidate") == candidate and row.get("helper") == candidate
                and row.get("installer_or_uninstaller") == action, "wrong candidate code executed")
        require(isinstance(row.get("child_shell"), str) and row["child_shell"].startswith(version),
                "candidate stage executed in the wrong child shell")
        require(row.get("child_error_view") == ("NormalView" if version == "5.1" else "ConciseView"),
                "candidate stage did not use the default child error display")
        require(type(row.get("candidate_path_length")) is int and row["candidate_path_length"] >= 100
                and row.get("candidate_path_has_space") is True and row.get("candidate_path_has_unicode") is True,
                "long space/Unicode candidate execution is absent")
    failures = report.get("failure_observations", [])
    require(isinstance(failures, list) and all(isinstance(r, dict) for r in failures), "invalid failure observations")
    names([row.get("case") for row in failures], FAILURE_CASES, "negative cases")
    for row in failures:
        require(row.get("rejected") is True and row.get("stage") == FAILURE_STAGES[row["case"]],
                "candidate failure was not propagated at the expected boundary")
        if row["case"] in {"update-installer-failure", "update-helper-failure", "update-uninstaller-failure"}:
            require(row.get("marker_observed") is True, "failing update code was not executed")


def path_report(report, shell_version, sources):
    common(report, "jaekit-windows-path-order/v1", shell_version, PATH_CASES, sources)
    before = report.get("before_sha256", "")
    require(isinstance(before, str) and re.fullmatch(r"[a-f0-9]{64}", before)
            and before == report.get("after_sha256"), "PATH fixture user-state preservation missing")
    require({"real-registry-write", "platform-inventory-lock-fixture-seams", "windows11-install-lifecycle"}
            <= set(report.get("unverified", [])), "PATH fixture scope must remain explicit")
