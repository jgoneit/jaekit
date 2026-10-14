"""Missing ADS, changed bytes and simulated evidence must fail validation."""
from copy import deepcopy
import hashlib
import unittest

import stream_results as results


def valid_report():
    sources = {name: "a" * 64 for name in results.SOURCE_PATHS}
    report = {"schema": "jaekit-windows-streams/v1", "status": "passed", "shell_version": "5.1.1",
              "filesystem": "NTFS", "actual_named_stream_roundtrip": True, "source_sha256": sources,
              "required": sorted(results.STREAM_CASES), "passed": sorted(results.STREAM_CASES),
              "skipped": [], "failures": [], "environment_before_sha256": "b" * 64,
              "environment_after_sha256": "b" * 64, "environment_preserved": True,
              "unverified": ["fixture-platform-root-lock-path", "injected-errors-and-stage-timing",
                             "unsupported-filesystem-simulated", "windows11-install-lifecycle"], "observations": []}
    report["diagnostic_controls"] = {"required": sorted(results.REFUSAL_CONTROLS),
                                     "passed": sorted(results.REFUSAL_CONTROLS),
                                     "observations": [{"case": name, "accepted": wanted, "expected_acceptance": wanted}
                                                      for name, wanted in results.REFUSAL_CONTROLS.items()]}
    report["unrelated_before"] = {"default_sha256": "e" * 64, "streams": [
        {"path": ".", "name": ":notes:$DATA", "size": 1, "sha256": "f" * 64}]}
    report["unrelated_after"] = deepcopy(report["unrelated_before"])
    report["unrelated_preserved"] = True
    for name in sorted(results.STREAM_CASES):
        row = {"case": name, "kind": "actual_ntfs", "rejected": False, "preserved": True, "outcome_verified": True}
        streams = []
        if name in results.TARGETS:
            boundary, target = results.TARGETS[name]
            data = bytes.fromhex(target["hex"])
            streams = [{"path": target["path"], "name": ":" + target["stream"] + ":$DATA",
                        "size": len(data), "sha256": hashlib.sha256(data).hexdigest()}]
            row.update(boundary=boundary, rejected=True, diagnostic_identifies_target=True,
                       exit_code=1, source_inventory_called=True, path_write_count=0, child_error_view="NormalView")
        elif name == "read-only-inputs":
            streams = [{"path": str(i), "name": ":Zone.Identifier:$DATA", "size": 1, "sha256": "c" * 64} for i in range(6)]
            row["input_stream_count"] = 6
        elif name in results.ERRORS:
            unsupported = name == "unsupported-filesystem"
            row.update(kind="injected_unsupported" if unsupported else "injected_error",
                       rejected=not unsupported, injected_branch_observed=True)
        elif name in results.LATE:
            committed = name != "staged-stream-refusal"
            streams = [{"path": ".", "name": ":notes:$DATA", "size": 1, "sha256": "c" * 64}]
            row.update(kind="actual_ntfs_injected_stage", rejected=not committed, committed=committed,
                       cleanup_preserved=True, warning_identifies_preserved_tree=True, exit_code=0 if committed else 1)
        row["before"] = {"default_sha256": "d" * 64, "streams": streams}
        row["after"] = deepcopy(row["before"])
        report["observations"].append(row)
    return report, sources


class StreamReportTests(unittest.TestCase):
    def test_complete_report_accepts_both_selected_shells(self):
        report, sources = valid_report()
        self.assertEqual(56, len(results.STREAM_CASES))
        for shell in ("5.1.1", "7.5.2"):
            report["shell_version"] = shell
            results.stream_report(report, shell, sources)

    def test_missing_source_cases_platform_and_error_evidence_reject(self):
        report, sources = valid_report()
        mutations = [lambda r: r.update(filesystem="ReFS"),
                     lambda r: r.update(actual_named_stream_roundtrip=False),
                     lambda r: r.update(shell_version="7.1"),
                     lambda r: r["source_sha256"].update({results.SOURCE_PATHS[0]: "e" * 64}),
                     lambda r: r["passed"].pop(), lambda r: r["required"].append(r["required"][0]),
                     lambda r: r["observations"].pop(), lambda r: r["skipped"].append("inventory-root"),
                     lambda r: r["failures"].append("denied"),
                     lambda r: r.update(environment_after_sha256="c" * 64),
                     lambda r: r["unrelated_after"]["streams"].clear(),
                     lambda r: r.update(unverified=[])]
        mutations.extend((lambda r: r.pop("diagnostic_controls"),
                          lambda r: r["diagnostic_controls"]["passed"].pop(),
                          lambda r: r["diagnostic_controls"]["observations"].pop(),
                          lambda r: r["diagnostic_controls"]["required"].append("plain-refusal"),
                          lambda r: r["diagnostic_controls"]["observations"][2].update(accepted=True),
                          lambda r: r["diagnostic_controls"]["observations"][0].update(accepted=False),
                          lambda r: r["diagnostic_controls"]["observations"][0].update(expected_acceptance=False)))
        for mutate in mutations:
            changed = deepcopy(report); mutate(changed)
            with self.subTest(mutation=mutate), self.assertRaises(ValueError):
                results.stream_report(changed, "5.1.1", sources)

    def test_changed_streams_and_false_success_reject(self):
        report, sources = valid_report()
        mutations = [
            ("inventory-root", lambda r: r["after"]["streams"].clear()),
            ("inventory-root", lambda r: r["after"].update(default_sha256="e" * 64)),
            ("inventory-state", lambda r: (r["before"]["streams"].clear(), r["after"]["streams"].clear())),
            ("inventory-root", lambda r: r.update(kind="injected_error")),
            ("inventory-root", lambda r: r.update(diagnostic_identifies_target=False)),
            ("reinstall-root", lambda r: r.update(exit_code=True)),
            ("reinstall-root", lambda r: r.update(child_error_view="ConciseView")),
            ("update-root", lambda r: r.update(path_write_count=1)),
            ("uninstall-root", lambda r: r.update(source_inventory_called=False)),
            ("normal-update", lambda r: r.update(rejected=True)),
            ("read-only-inputs", lambda r: r.update(input_stream_count=5)),
            ("enumeration-cleanup", lambda r: r.update(rejected=False)),
            ("capability-api-failure", lambda r: r.update(kind="actual_ntfs")),
            ("unsupported-filesystem", lambda r: r.update(injected_branch_observed=False)),
            ("staged-stream-refusal", lambda r: r.update(cleanup_preserved=False)),
            ("install-backup-preserved", lambda r: r.update(committed=False)),
            ("uninstall-backup-preserved", lambda r: r.update(warning_identifies_preserved_tree=False))]
        for name, mutate in mutations:
            changed = deepcopy(report)
            mutate(next(row for row in changed["observations"] if row["case"] == name))
            with self.subTest(case=name), self.assertRaises(ValueError):
                results.stream_report(changed, "5.1.1", sources)


if __name__ == "__main__":
    unittest.main()
