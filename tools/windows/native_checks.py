#!/usr/bin/env python3
"""Run current-source Windows fixtures; no installation or platform impersonation."""
from __future__ import annotations

import argparse
import hashlib
import json
from pathlib import Path
import platform
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
sys.dont_write_bytecode = True
sys.path.insert(0, str(ROOT / "tools/ci"))
from native_results import native_results
sys.path.insert(0, str(ROOT / "tools/windows"))
from lifecycle_results import acceptance_report, path_report
from console_results import console_report
from stream_results import stream_report, SOURCE_PATHS as STREAM_SOURCE_PATHS

MODULE = "github.com/jgoneit/jaekit/"
NATIVE = {
    "internal/run": ["TestNativeAdmissionFailureHasNoOutputDigest",
                     "TestNativeUnconfirmedShutdownHasNoOutputDigest",
                     "TestNativeArgvEnvironmentAndOutcomes", "TestNativeTimeoutStopsDescendantWrites",
                     "TestNativeJobLayout", "TestNativeRunTiming", "TestNativeRunTiming/exit",
                     "TestNativeRunTiming/timeout", "TestNativeRunTiming/start_failure",
                     "TestNativeStdoutDrainBeforeCleanup", "TestNativeStdoutDrainBeforeCleanup/cleanup_ends_descendants",
                     "TestNativeStdoutDrainBeforeCleanup/native_cleanup"],
    "internal/record": ["TestNativeConcurrentProcessesPreserveRecords", "TestNativeLockIsReleasedWhenOwnerExits"],
    "internal/evidence": ["TestNativeCleanCRLFStillRequiresCommittedBytes", "TestNativeWindowsEvidencePaths",
                          "TestNativeAdapterClassification",
                          *("TestNativeAdapterClassification/" + case + "/" + ending for case in
                            ("go-typed-fail", "go-typed-unmarked", "go-typed-pass", "J1", "J2", "J3", "J5", "J10")
                            for ending in ("lf", "crlf"))],
    "internal/history": ["TestNativeHistoryStorageLifecycle", "TestNativeHistoryStorageLocks",
                         "TestNativeHistoryStorageIntegrity", "TestNativeHistoryStoragePathSafety",
                         "TestNativeStorageBoundsHistoryLines", "TestNativeShowRejectsMalformedIdentifiers",
                         "TestNativeShowPreservesValidIdentifierLookup",
                         *("TestNativeHistoryStorageLifecycle/" + case for case in
                           ("registration_replay_and_correction", "concurrent_stores_and_corrections",
                            "lineage_and_current", "evidence_replay_preserves_observation", "case_alias_shares_storage")),
                         *("TestNativeHistoryStorageLocks/" + case for case in
                           ("reader_waits_for_writer", "first_registration_retry", "shared_readers_exclude_writer",
                            "process_exit_releases_ownership")),
                         *("TestNativeHistoryStorageIntegrity/" + case for case in
                           ("absence_damage_and_unsupported", "trailing_loss", "interrupted_marker_catches_up",
                            "uncertain_append_retry", "last_replacement_failure_is_uncertain",
                            "snapshot_survives_marker_replacement", "snapshot_opens_before_rename_source_closes")),
                         *("TestNativeHistoryStoragePathSafety/junction_" + case for case in
                           ("common", "ha", "history", "start", "events.jsonl", "lock", "last", "last.tmp")),
                         "TestNativeHistoryStoragePathSafety/open_ancestors_and_lock_cannot_be_replaced",
                         "TestNativeHistoryStoragePathSafety/hard-linked_files_cannot_redirect_reads_or_writes",
                         *("TestNativeHistoryStoragePathSafety/hard-linked_files_cannot_redirect_reads_or_writes/" + case
                           for case in ("events.jsonl", "lock", "last", "last.tmp"))],
    "cmd/ha": ["TestNativeRecordedTiming", "TestNativeStartBudgetAndSkill",
               *("TestNativeStartBudgetAndSkill/" + case for case in
                 ("ancestors", "folder_identity", "home_before_userprofile", "xdg_folder", "home_folder",
                  "missing_default", "corrupt_refused", "auto_start", "auto_refused", "skill_missing_support",
                  "skill_supported")),
               "TestNativeCoreGitLifecycle", "TestNativeCoreGitLifecycle/linked=false",
               "TestNativeCoreGitLifecycle/linked=true", "TestNativeBaselineRejectsJunction",
               "TestNativeHistoryGitLifecycle", "TestNativeHistoryFileEvidence",
               "TestNativeHistoryRequestIDPresence",
               "TestNativeHistoryWindowsFileBoundaries",
               *("TestNativeHistoryGitLifecycle/linked=" + value for value in ("false", "true")),
               *("TestNativeHistoryFileEvidence/replace-" + kind for kind in ("file", "directory", "removed")),
               *("TestNativeHistoryFileEvidence/" + case for case in
                 ("initial_size_boundary", "growth_after_check", "growth_during_read")),
               *("TestNativeHistoryWindowsFileBoundaries/" + case for case in
                 ("special-references", "stable-parent-and-outside", "junction-race-parent",
                  "junction-race-root", "confined-reparse", "sharing-denial")),
               "TestNativeWindowsBaselineJunctionScenarios",
               *("TestNativeWindowsBaselineJunctionScenarios/" + case for case in
                 ("regular-before", "parent-junction", "leaf-junction", "partial-batch", "regular-after")),
               "TestNativeEvidencePathScenarios", "TestNativeWindowsEvidencePathScenarios",
               "TestNativeWindowsProcessControlScenarios", "TestNativeProcessTimeoutScenarios",
               "TestNativeProcessUnconfirmedShutdownScenarios",
               *("TestNativeWindowsProcessControlScenarios/" + case for case in
                 ("parent-pass-check", "parent-violation-check", "parent-pass-baseline", "parent-violation-baseline")),
               *("TestNativeProcessTimeoutScenarios/" + case for case in ("timeout-check", "timeout-baseline")),
               *("TestNativeProcessUnconfirmedShutdownScenarios/" + case for case in
                 ("rules-3-check", "rules-3-baseline", "rules-2-check", "rules-2-baseline", "rules-1-check", "rules-1-baseline")),
               "TestNativeReportOutput", "TestNativeReportOutput/en", "TestNativeReportOutput/ko",
               "TestNativeReportOutput/uncompared",
               "TestNativeDoctorProbes",
               *("TestNativeDoctorProbes/" + case for case in ("available", "missing", "exit", "timeout", "tail"))],
}
REQUIRED = sorted(MODULE + package + "/" + test for package, tests in NATIVE.items() for test in tests)
PACKAGE_CHECKS = ["candidate-manifest-sha256", "candidate-native-version-capabilities",
                  "space-unicode-drive-path", "checksum-refusal", "archive-traversal-refusal",
                  "archive-case-collision-refusal", "archive-file-directory-collision-refusal",
                  "archive-reserved-name-refusal", "archive-stream-name-refusal", "archive-name-character-refusal",
                  "junction-path-refusal"]
# Every character the installer's name rule refuses (Assert-PackageName in
# common.ps1). Each is placed in an entry name of the actual candidate and must
# be refused with the installer's own message in the shell being checked; on
# Windows PowerShell 5.1 some fail while .NET Framework lists the entries.
NAME_RULE_CHARACTERS = tuple(range(0x20)) + tuple(ord(character) for character in ':<>"|?*')
PREFLIGHT_FILES = ("archive", "common", "install", "uninstall", "acceptance")
PREFLIGHT_CASES = {"actual-candidate", "valid-install", "valid-remove", "valid-acceptance",
                   "invalid-digest", "mismatched-digest", "duplicate-entry", "ambiguous-case-entry",
                   "unsafe-relative-entry", "unsafe-absolute-entry", "unexpected-entry",
                   "unexpected-script", "directory-entry"}
PREFLIGHT_CASES.update(action + "-tampered-" + name for action in
                       ("install", "remove", "acceptance-first", "acceptance-update") for name in PREFLIGHT_FILES)
PREFLIGHT_CASES.update("missing-file-" + name for name in (*PREFLIGHT_FILES, "checksums"))
PREFLIGHT_CASES.update("missing-entry-" + name for name in PREFLIGHT_FILES)


def validate_preflight_report(report: dict, shell_version: str) -> None:
    if (report.get("schema") != "jaekit-windows-preflight/v1" or report.get("status") != "passed"
            or report.get("shell_version") != shell_version or report.get("failures")):
        raise RuntimeError("same-shell guide preflight report is absent, invalid, or failed")
    guides = report.get("guides", [])
    if len(guides) != 2 or {row.get("path") for row in guides} != {"guides/WINDOWS.md", "guides/WINDOWS.en.md"}:
        raise RuntimeError("both public guide preflight observations are required")
    for guide in guides:
        for field in ("required", "passed"):
            names = guide.get(field, [])
            if len(names) != len(PREFLIGHT_CASES) or set(names) != PREFLIGHT_CASES:
                raise RuntimeError("guide preflight cases missing or duplicated: " + field)
        if guide.get("skipped") or not re.fullmatch(r"[a-f0-9]{64}", guide.get("definition_sha256", "")):
            raise RuntimeError("guide preflight skipped or source digest missing")
        actions = guide.get("action_sha256", {})
        if set(actions) != {"install", "remove", "acceptance"} or any(
                not isinstance(value, str) or not re.fullmatch(r"[a-f0-9]{64}", value) for value in actions.values()):
            raise RuntimeError("guide action source identities missing")


def validate_package_report(report: dict, shell_version: str, server: bool) -> None:
    if report.get("schema") != "jaekit-native-windows-package/v1" or report.get("status") != "passed":
        raise RuntimeError("package runtime report is absent, invalid, or failed")
    if report.get("shell_version") != shell_version:
        raise RuntimeError("package/path checks did not execute in the selected PowerShell")
    checks = report.get("package_checks", [])
    if len(checks) != len(PACKAGE_CHECKS) or set(checks) != set(PACKAGE_CHECKS) or report.get("failures"):
        raise RuntimeError("package runtime report is incomplete or has failures")
    refused = report.get("name_character_refusals")
    if (not isinstance(refused, list) or any(type(code) is not int for code in refused)
            or len(refused) != len(set(refused)) or set(refused) != set(NAME_RULE_CHARACTERS)):
        observed = set(refused) if isinstance(refused, list) else set()
        missing = sorted(set(NAME_RULE_CHARACTERS) - {code for code in observed if type(code) is int})
        raise RuntimeError("installer name-character refusals are incomplete; missing: "
                           + (", ".join("0x%02x" % code for code in missing) or "none (duplicate or invalid entries)"))
    if server:
        for action in ("install", "uninstall"):
            actual = report.get("server_rejection", {}).get(action, {})
            if actual.get("rejected") is not True or actual.get("preserved") is not True:
                raise RuntimeError("Windows Server refusal/preservation was not observed: " + action)
            before, after = actual.get("before_sha256", ""), actual.get("after_sha256", "")
            if (not re.fullmatch(r"[a-f0-9]{64}", before) or before != after
                    or not isinstance(actual.get("exit_code"), int) or actual["exit_code"] == 0):
                raise RuntimeError("Server refusal has invalid preservation/exit observations: " + action)


def digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def capture(argv: list[str], output: Path, label: str, timeout: int = 300) -> subprocess.CompletedProcess:
    # Binary capture preserves Go JSON bytes independently of the PowerShell
    # version's redirection encoding. No shell parses checker/build arguments.
    with (output / (label + ".stdout.log")).open("wb") as stdout, (output / (label + ".stderr.log")).open("wb") as stderr:
        result = subprocess.run(argv, cwd=ROOT, stdout=stdout, stderr=stderr, timeout=timeout, check=False)
    return result


def checked(argv: list[str], output: Path, label: str, timeout: int = 300) -> str:
    result = capture(argv, output, label, timeout)
    if result.returncode:
        raise RuntimeError(f"{label} exited {result.returncode}; see {label}.stderr.log")
    return (output / (label + ".stdout.log")).read_text(encoding="utf-8-sig").strip()


def failed_native_output(path: Path, limit: int = 12000) -> bytes:
    """Keep failed tests' messages even when later passing tests fill the tail."""
    incomplete = False
    try:
        events = []
        with path.open(encoding="utf-8", errors="replace") as stream:
            for line in stream:
                try:
                    event = json.loads(line)
                except ValueError:
                    incomplete = True
                    continue
                # Go's JSON compiler diagnostics use ImportPath, not Package.
                if (isinstance(event, dict) and event.get("Action") in ("build-output", "build-fail")
                        and isinstance(event.get("ImportPath"), str)):
                    event = dict(event, Package=event["ImportPath"], Test=None,
                                 Action=event["Action"].removeprefix("build-"))
                if (isinstance(event, dict) and isinstance(event.get("Package"), str)
                        and (event.get("Test") is None or isinstance(event.get("Test"), str))):
                    events.append(event)
                else:
                    incomplete = True
    except OSError as error:
        return ("[native test diagnostics unavailable: " + type(error).__name__ + "]\n").encode()[:limit]
    failed = {(event.get("Package"), event.get("Test")) for event in events
              if event.get("Action") == "fail"}
    selected = bytearray()
    for event in events:
        key = (event.get("Package"), event.get("Test"))
        if event.get("Action") != "output" or key not in failed or not isinstance(event.get("Output"), str):
            continue
        label = "/".join(str(part) for part in key if part)
        message = (label + ": " + event["Output"]).encode("utf-8", errors="backslashreplace")
        room = limit + 1 - len(selected)
        selected.extend(message[:room])
        if len(message) > room:
            break
    if incomplete:
        selected.extend(b"\n[native test diagnostics incomplete: malformed events omitted]\n")
    if len(selected) > limit:
        marker = b"\n[failed native output truncated]\n"
        return bytes(selected[:max(0, limit - len(marker))]) + marker[:limit]
    return bytes(selected)


def failure_tails(output: Path) -> None:
    """Keep first native failures diagnosable without publishing full transcripts."""
    remaining = 24000
    failed = failed_native_output(output / "go-test.jsonl")
    if failed:
        remaining -= len(failed)
        print("Captured failed native test output:")
        encoding = getattr(sys.stdout, "encoding", None) or "utf-8"
        diagnostic = failed.decode("utf-8", errors="replace")
        print(diagnostic.encode(encoding, errors="backslashreplace").decode(encoding))
    for name in ("source-status.stdout.log", "acceptance-runtime.stderr.log", "acceptance-runtime.stdout.log",
                 "stream-runtime.stderr.log", "stream-runtime.stdout.log",
                 "console-path-runtime.stderr.log", "console-path-runtime.stdout.log",
                 "path-order-runtime.stderr.log", "path-order-runtime.stdout.log", "preflight-runtime.stderr.log", "preflight-runtime.stdout.log",
                 "package-runtime/install-refusal.log", "package-runtime/uninstall-refusal.log",
                 "package-runtime.stderr.log", "package-runtime.stdout.log", "package.stderr.log",
                 "build.stderr.log", "go-test.stderr.log", "go-test.jsonl", "go-test.stdout.log"):
        path = output / name
        if not path.is_file():
            continue
        if remaining <= 0:
            print("[remaining public fixture diagnostics omitted: output limit]")
            break
        try:
            with path.open("rb") as stream:
                offset = max(0, path.stat().st_size - min(6000, remaining))
                stream.seek(offset)
                data = stream.read(min(6000, remaining))
        except OSError as error:
            print("[public fixture diagnostics unavailable: " + name + "; " + type(error).__name__ + "]")
            continue
        if data:
            remaining -= len(data)
            print("Captured public fixture log tail: " + name)
            if offset:
                print("[earlier public fixture output omitted: tail limit]")
            diagnostic = data.decode("utf-8", errors="replace")
            # Windows redirected stdout may be cp1252 even when the raw log is
            # UTF-8. Escape only characters this console cannot encode, so a
            # diagnostic cannot prevent the final failed report from printing.
            encoding = getattr(sys.stdout, "encoding", None) or "utf-8"
            print(diagnostic.encode(encoding, errors="backslashreplace").decode(encoding))


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--shell-executable", required=True)
    parser.add_argument("--shell-version", required=True)
    parser.add_argument("--shell-edition", required=True)
    parser.add_argument("--installation-type", required=True)
    parser.add_argument("--os-build", type=int, required=True)
    args = parser.parse_args()
    output = args.output.resolve()
    # Keep artifacts outside the source tree and refuse to replace old evidence.
    if output == ROOT or ROOT in output.parents or output.exists():
        parser.error("output must be a new directory outside the source checkout")
    output.mkdir(parents=True)
    report = {"schema": "jaekit-native-windows/v1", "status": "failed", "source_commit": None,
              "environment": {"os": platform.system().lower(), "arch": platform.machine(),
                              "installation_type": args.installation_type, "os_build": args.os_build,
                              "shell_version": args.shell_version, "shell_edition": args.shell_edition},
              "native_tests": {"required": REQUIRED, "passed": [], "skipped": []},
              "package_checks": [], "preflight_checks": {}, "acceptance_dispatch": {}, "path_order_checks": {},
              "console_path_checks": {},
              "stream_checks": {},
              "server_rejection": {}, "failures": [],
              "unverified": ["windows11-install-lifecycle", "wsl2", "host-skills", "public-release"]}
    try:
        if sys.platform != "win32" or platform.machine().lower() not in ("amd64", "x86_64"):
            raise RuntimeError("this runner requires actual native Windows x64")
        report["environment"]["arch"] = "amd64"
        source = checked(["git", "rev-parse", "HEAD"], output, "source")
        if not re.fullmatch(r"[a-f0-9]{40}", source):
            raise RuntimeError("cannot identify the checked-out source commit")
        report["source_commit"] = source
        if checked(["git", "status", "--porcelain"], output, "source-status"):
            raise RuntimeError("native evidence requires a clean source checkout")
        config = json.loads(checked(["go", "env", "-json", "GOOS", "GOARCH", "GOFLAGS"], output, "go-env"))
        if config != {"GOOS": "windows", "GOARCH": "amd64", "GOFLAGS": ""}:
            raise RuntimeError("effective Go target must be windows/amd64 with empty GOFLAGS")
        report["environment"].update(goos=config["GOOS"], goarch=config["GOARCH"])
        selection = "^(" + "|".join(sorted({name.split("/")[0] for names in NATIVE.values() for name in names})) + ")$"
        result = capture(["go", "test", "-json", "-mod=readonly", "-count=1", "-timeout=5m", "-run", selection,
                          *("./" + package for package in NATIVE)], output, "go-test", 360)
        # The same raw stream is the artifact, not a re-serialized interpretation.
        (output / "go-test.stdout.log").rename(output / "go-test.jsonl")
        native_output = (output / "go-test.jsonl").read_text(encoding="utf-8")
        for line in native_output.splitlines():
            event = json.loads(line)
            if event.get("Action") in ("skip", "fail"):
                field = "skipped" if event["Action"] == "skip" else "failed"
                name = event.get("Package", "")
                if event.get("Test"):
                    name += "/" + event["Test"]
                report["native_tests"].setdefault(field, []).append(name)
        if result.returncode:
            raise RuntimeError(f"native tests exited {result.returncode}; see go-test.jsonl")
        report["native_tests"] = native_results(native_output,
                                                set(REQUIRED), {MODULE + p for p in NATIVE})
        version = "0.0.0-ci." + source[:12]
        binary = output / "ha.exe"
        checked(["go", "build", "-mod=readonly", "-trimpath", "-ldflags", "-X main.version=" + version,
                 "-o", str(binary), "./cmd/ha"], output, "build")
        checked([sys.executable, str(ROOT / "tools/windows/package.py"), "--binary", str(binary),
                 "--version", version, "--output", str(output / "candidate")], output, "package")
        archive = output / "candidate" / f"ha_{version}_windows_amd64.zip"
        report["candidate"] = {"version": version, "source_commit": source, "binary_sha256": digest(binary),
                               "archive_sha256": digest(archive)}
        preflight_result = output / "preflight-results.json"
        result = capture([args.shell_executable, "-NoProfile", "-NonInteractive", "-File",
                          str(ROOT / "tools/windows/native_preflight_checks.ps1"), "-CandidateDirectory",
                          str(output / "candidate"), "-OutputPath", str(preflight_result)], output, "preflight-runtime")
        if not preflight_result.is_file():
            raise RuntimeError("guide preflight produced no report")
        report["preflight_checks"] = json.loads(preflight_result.read_text(encoding="utf-8-sig"))
        validate_preflight_report(report["preflight_checks"], args.shell_version)
        expected_candidate = {"archive_sha256": digest(archive),
                              "checksums_sha256": digest(output / "candidate/checksums.txt"),
                              "loose_sha256": {name: digest(output / "candidate" / name) for name in
                                               ("common.ps1", "install.ps1", "uninstall.ps1", "acceptance.ps1")}}
        if any(guide.get("actual_candidate") != expected_candidate
               for guide in report["preflight_checks"]["guides"]):
            raise RuntimeError("public preflight verified a different candidate")
        if result.returncode:
            raise RuntimeError("guide preflight execution failed")
        for script, label, key, validate, sources in (
                ("native_acceptance_checks.ps1", "acceptance-runtime", "acceptance_dispatch", acceptance_report,
                 {name: digest(ROOT / name) for name in
                  ("tools/windows/acceptance.ps1", "tools/windows/README.md")}),
                ("native_path_checks.ps1", "path-order-runtime", "path_order_checks", path_report,
                 {"common": digest(ROOT / "tools/windows/common.ps1"),
                  "uninstall": digest(ROOT / "tools/windows/uninstall.ps1")})):
            result_path = output / (label + "-results.json")
            result = capture([args.shell_executable, "-NoProfile", "-NonInteractive", "-File",
                              str(ROOT / "tools/windows" / script), "-OutputPath", str(result_path)], output, label)
            if not result_path.is_file():
                raise RuntimeError(label + " produced no report")
            report[key] = json.loads(result_path.read_text(encoding="utf-8-sig"))
            validate(report[key], args.shell_version, sources)
            if result.returncode:
                raise RuntimeError(label + " execution failed")
        console_result = output / "console-path-results.json"
        result = capture([args.shell_executable, "-NoProfile", "-NonInteractive", "-File",
                          str(ROOT / "tools/windows/native_console_checks.ps1"), "-CandidateDirectory",
                          str(output / "candidate"), "-OutputPath", str(console_result)], output, "console-path-runtime")
        if not console_result.is_file():
            raise RuntimeError("console path checks produced no report")
        report["console_path_checks"] = json.loads(console_result.read_text(encoding="utf-8-sig"))
        console_report(report["console_path_checks"], args.shell_version,
                       {name: digest(ROOT / name) for name in
                        ("tools/windows/package.py", "tools/windows/common.ps1",
                         "tools/windows/install.ps1", "tools/windows/uninstall.ps1")})
        if result.returncode:
            raise RuntimeError("console path checks execution failed")
        stream_result = output / "stream-results.json"
        result = capture([args.shell_executable, "-NoProfile", "-NonInteractive", "-File",
                          str(ROOT / "tools/windows/native_stream_checks.ps1"), "-CandidateDirectory",
                          str(output / "candidate"), "-OutputPath", str(stream_result)], output, "stream-runtime")
        if not stream_result.is_file():
            raise RuntimeError("stream checks produced no report")
        report["stream_checks"] = json.loads(stream_result.read_text(encoding="utf-8-sig"))
        stream_report(report["stream_checks"], args.shell_version,
                      {name: digest(ROOT / name) for name in STREAM_SOURCE_PATHS})
        if result.returncode:
            raise RuntimeError("stream checks execution failed")
        package_result = output / "package-results.json"
        result = capture([args.shell_executable, "-NoProfile", "-NonInteractive", "-File",
                          str(ROOT / "tools/windows/native_package_checks.ps1"), "-CandidateDirectory",
                          str(output / "candidate"), "-OutputPath", str(package_result), "-NameCharacterCodes",
                          ",".join(str(code) for code in NAME_RULE_CHARACTERS)], output, "package-runtime")
        if package_result.is_file():
            package_report = json.loads(package_result.read_text(encoding="utf-8-sig"))
            report["package_checks"] = package_report.get("package_checks", [])
            report["server_rejection"] = package_report.get("server_rejection", {})
            report["candidate"].update(package_report.get("candidate", {}))
            report["failures"].extend(package_report.get("failures", []))
            validate_package_report(package_report, args.shell_version,
                                    args.installation_type in ("Server", "Server Core"))
            observed = package_report.get("candidate", {})
            if (observed.get("executed_binary_sha256") != digest(binary)
                    or observed.get("capabilities", {}).get("ha_version") != version):
                raise RuntimeError("executed packaged binary differs from the current-source build")
        if result.returncode or set(report["package_checks"]) != set(PACKAGE_CHECKS):
            raise RuntimeError("actual candidate/package/path checks failed or did not all execute")
        if report["failures"]:
            raise RuntimeError("package/runtime report contains a failure")
        if checked(["git", "status", "--porcelain"], output, "final-source-status"):
            raise RuntimeError("native checks changed the source checkout")
        if checked(["git", "rev-parse", "HEAD"], output, "final-source") != source:
            raise RuntimeError("source commit changed during native checks")
        report["status"] = "passed"
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as exc:
        report["failures"].append(str(exc))
    finally:
        if report["status"] != "passed":
            try:
                failure_tails(output)
            except (OSError, UnicodeError, ValueError) as exc:
                report["diagnostics"] = {"status": "unavailable", "reason": type(exc).__name__}
        (output / "summary.json").write_text(json.dumps(report, indent=2, ensure_ascii=True) + "\n", encoding="utf-8")
        print("JAEKIT_CI_REPORT " + json.dumps(report, separators=(",", ":"), ensure_ascii=True))
    return 0 if report["status"] == "passed" else 1


if __name__ == "__main__":
    raise SystemExit(main())
