"""Missing runtime cases and changed source/state cannot satisfy console checks."""
from __future__ import annotations

import copy
import unittest

import console_results as results


class ConsoleReportTests(unittest.TestCase):
    def report(self):
        sources = {name: "a" * 64 for name in results.SOURCE_PATHS}
        report = {"schema": "jaekit-windows-console-paths/v1", "status": "passed",
                  "shell_version": "5.1.26100.1", "source_sha256": sources.copy(),
                  "required": sorted(results.CONSOLE_CASES), "passed": sorted(results.CONSOLE_CASES),
                  "skipped": [], "failures": [], "environment_preserved": True,
                  "environment_before_sha256": "b" * 64, "environment_after_sha256": "b" * 64,
                  "unverified": ["fixture-platform-root-lock", "archive-entry-open-observer", "windows11-install-lifecycle"],
                  "observations": []}
        for name in sorted(results.CONSOLE_CASES):
            row = {"case": name, "preserved": True}
            if name in results.REJECTED:
                boundary, path = results.REJECTED[name]
                row.update(rejected=True, boundary=boundary, path=path)
                if boundary == "archive":
                    row.update(entry_open_count=0, destination_absent=True)
            elif name in results.ALLOWED:
                row.update(rejected=False, path=results.ALLOWED[name])
            elif name in results.ORCHESTRATION:
                row.update(rejected=True, exit_code=1, write_boundary_count=0,
                           entry_open_count=0, source_inventory_called=True)
            else:
                row.update(rejected=False)
            report["observations"].append(row)
        return report, sources

    def test_complete_source_bound_report_is_accepted(self):
        report, sources = self.report()
        self.assertEqual(84, len(results.CONSOLE_CASES))
        results.console_report(report, report["shell_version"], sources)

    def test_missing_skipped_changed_source_or_state_is_rejected(self):
        mutations = [lambda r: r["required"].pop(), lambda r: r["passed"].pop(),
                     lambda r: r["passed"].__setitem__(0, r["passed"][1]),
                     lambda r: r["observations"].pop(),
                     lambda r: r.update(skipped=["normal-copy"]),
                     lambda r: r.update(failures=["unexpected input"]),
                     lambda r: r.update(shell_version="7.6.0"),
                     lambda r: r["source_sha256"].update({results.SOURCE_PATHS[0]: "c" * 64}),
                     lambda r: r.update(environment_after_sha256="c" * 64),
                     lambda r: r.update(environment_preserved=False),
                     lambda r: r.update(unverified=[])]
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                report, sources = self.report()
                mutate(report)
                with self.assertRaises(ValueError):
                    results.console_report(report, "5.1.26100.1", sources)

    def test_refusal_requires_correct_path_before_writes_or_data_open(self):
        report, sources = self.report()
        changes = {"archive-conin-root": [{"rejected": False}, {"path": "different"},
                                          {"boundary": "name"}, {"entry_open_count": 1},
                                          {"entry_open_count": False}, {"destination_absent": False}],
                   "install-archive-refusal": [{"exit_code": 0}, {"exit_code": True},
                                               {"write_boundary_count": 1}, {"entry_open_count": 1},
                                               {"source_inventory_called": False}],
                   "inventory-conout-root": [{"preserved": False}],
                   "allowed-coninx": [{"rejected": True}],
                   "normal-copy": [{"rejected": True}]}
        for name, mutations in changes.items():
            for mutate in mutations:
                with self.subTest(case=name, mutation=mutate):
                    changed = copy.deepcopy(report)
                    next(row for row in changed["observations"] if row["case"] == name).update(mutate)
                    with self.assertRaises(ValueError):
                        results.console_report(changed, "5.1.26100.1", sources)


if __name__ == "__main__":
    unittest.main()
