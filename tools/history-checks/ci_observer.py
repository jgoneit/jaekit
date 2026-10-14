"""Observe history on the current PR source, reusing full native CI admission."""
import importlib.util
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location("history_native_ci", ROOT / "tools/verification-checks/cross_platform_ci.py")
ci = importlib.util.module_from_spec(spec)
spec.loader.exec_module(ci)
spec = importlib.util.spec_from_file_location("history_windows_validation", ROOT / "tools/verification-checks/windows_validation.py")
validation = importlib.util.module_from_spec(spec)
spec.loader.exec_module(validation)

# Independent admission expectations: do not derive these from the runner's
# selection list, which could accidentally omit the behavior under review.
HISTORY = {
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryRequestIDPresence',
    'github.com/jgoneit/jaekit/internal/history/TestNativeStorageBoundsHistoryLines',
    'github.com/jgoneit/jaekit/internal/history/TestNativeShowRejectsMalformedIdentifiers',
    'github.com/jgoneit/jaekit/internal/history/TestNativeShowPreservesValidIdentifierLookup',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryFileEvidence',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryFileEvidence/growth_after_check',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryFileEvidence/growth_during_read',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryFileEvidence/initial_size_boundary',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryFileEvidence/replace-directory',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryFileEvidence/replace-file',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryFileEvidence/replace-removed',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryGitLifecycle',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryGitLifecycle/linked=false',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryGitLifecycle/linked=true',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryWindowsFileBoundaries',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryWindowsFileBoundaries/confined-reparse',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryWindowsFileBoundaries/junction-race-parent',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryWindowsFileBoundaries/junction-race-root',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryWindowsFileBoundaries/sharing-denial',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryWindowsFileBoundaries/special-references',
    'github.com/jgoneit/jaekit/cmd/ha/TestNativeHistoryWindowsFileBoundaries/stable-parent-and-outside',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity/absence_damage_and_unsupported',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity/interrupted_marker_catches_up',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity/last_replacement_failure_is_uncertain',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity/snapshot_survives_marker_replacement',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity/snapshot_opens_before_rename_source_closes',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity/trailing_loss',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageIntegrity/uncertain_append_retry',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLifecycle',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLifecycle/case_alias_shares_storage',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLifecycle/concurrent_stores_and_corrections',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLifecycle/evidence_replay_preserves_observation',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLifecycle/lineage_and_current',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLifecycle/registration_replay_and_correction',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLocks',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLocks/first_registration_retry',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLocks/process_exit_releases_ownership',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLocks/reader_waits_for_writer',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStorageLocks/shared_readers_exclude_writer',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/hard-linked_files_cannot_redirect_reads_or_writes',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/hard-linked_files_cannot_redirect_reads_or_writes/events.jsonl',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/hard-linked_files_cannot_redirect_reads_or_writes/last',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/hard-linked_files_cannot_redirect_reads_or_writes/last.tmp',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/hard-linked_files_cannot_redirect_reads_or_writes/lock',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_common',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_events.jsonl',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_ha',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_history',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_last',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_last.tmp',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_lock',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/junction_start',
    'github.com/jgoneit/jaekit/internal/history/TestNativeHistoryStoragePathSafety/open_ancestors_and_lock_cannot_be_replaced',
}


def history_report(report, operating_system, source):
    if operating_system != "Windows":
        return
    tests = report.get("native_tests", {})
    ci.require(HISTORY <= set(tests.get("required", [])), "Windows history declaration incomplete")
    ci.require(HISTORY <= set(tests.get("passed", [])) and not tests.get("skipped"),
               "Windows history execution missing or skipped")
    capabilities = report.get("candidate", {}).get("capabilities", {})
    ci.require("decision-history/v1" in capabilities.get("features", []) and
               capabilities.get("formats", {}).get("decision_history") == ["decision-history/v1"],
               "Windows candidate does not advertise the executed history format")


def observe():
    # The existing consumer must receive the same complete reports admitted by
    # the shared observer, not a separate collection from another run attempt.
    try:
        reports = validation.live_reports(observer=ci)
        validation.validate_boundaries(reports)
    except validation.Violation as error:
        raise ci.Violation(str(error)) from None
    for report in reports:
        history_report(report, "Windows", report["source_commit"])
    return reports


if __name__ == "__main__":
    observe()
