"""The archive rule agrees with the expected verdict of each contract case.

The installer side runs only on Windows (release workflow); here it is
replaced to check how the report combines the verdicts.
"""
import importlib.util
from pathlib import Path
import tempfile
import unittest
import zipfile

HERE = Path(__file__).resolve().parent


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


contract = load("windows_contract_under_test", HERE / "windows_contract.py")
# Not imported as a name here, so discovery does not collect its tests twice.
package_tests = load("windows_contract_package_tests", HERE.parent / "windows/test_package.py")


class WindowsContract(unittest.TestCase):
    def test_the_archive_rule_gives_each_expected_verdict(self):
        with tempfile.TemporaryDirectory() as folder:
            for name, expected, data in contract.cases():
                with self.subTest(case=name):
                    path = Path(folder) / f"ha_{contract.VERSION}_windows_amd64.zip"
                    path.write_bytes(data)
                    self.assertEqual(contract.python_verdict(path)[0], expected)

    def test_raw_entry_names_reach_the_archive(self):
        cases = {name: data for name, _, data in contract.cases()}
        with tempfile.TemporaryDirectory() as folder:
            for name, raw in (("a NUL in an entry name", "LICENSE\0x"),
                              ("a backslash in an entry name", "guides\\WINDOWS.md")):
                path = Path(folder) / "case.zip"
                path.write_bytes(cases[name])
                with zipfile.ZipFile(path) as archive:
                    self.assertIn(raw, [entry.orig_filename for entry in archive.infolist()])

    def test_the_report_passes_only_when_every_verdict_is_expected(self):
        expected = {name: verdict for name, verdict, _ in contract.cases()}
        agreeing = contract.contract(lambda shell, directory, destination: (
            expected[self.current(directory)], "System.Management.Automation.RuntimeException: Unsafe package path: x"))
        self.assertTrue(agreeing["passed"])
        self.assertEqual({"powershell", "pwsh"}, set(agreeing["cases"][0]["installer"]))
        lenient = contract.contract(lambda shell, directory, destination: ("accept", ""))
        self.assertFalse(lenient["passed"])
        silent = contract.contract(lambda shell, directory, destination: ("error", "no verdict"))
        self.assertFalse(silent["passed"])
        # An access error is no refusal: it neither agrees nor disagrees.
        denied = contract.contract(lambda shell, directory, destination: (
            "reject", "System.UnauthorizedAccessException: Access to the path is denied."))
        self.assertFalse(denied["passed"])
        self.assertEqual({"error"}, {verdict for case in denied["cases"] for verdict in case["installer"].values()})
        # Refusing the normal archive for any other reason is a disagreement.
        refused = contract.contract(lambda shell, directory, destination: (
            "reject", "System.Management.Automation.RuntimeException: Installed/package file differs: LICENSE"))
        self.assertEqual("reject", refused["cases"][0]["installer"]["pwsh"])

    def current(self, directory):
        number = int(directory.name.removeprefix("case-"))
        return contract.cases()[number][0]

    def test_the_report_is_marked_complete_only_at_the_end(self):
        seen = []
        report = contract.contract(lambda shell, directory, destination: ("accept", ""),
                                   progress=lambda partial: seen.append(partial["complete"]))
        self.assertEqual([False] * len(contract.cases()), seen)
        self.assertTrue(report["complete"])


class PackagedArchive(unittest.TestCase):
    def test_a_package_py_archive_passes_the_rule(self):
        # package.py's own synthetic Git source fixture and a synthetic PE32+ Core.
        fixture = package_tests.CandidatePackageTests("prepare")
        fixture.setUp()
        self.addCleanup(fixture.doCleanups)
        path = fixture.prepare(version="0.0.0")
        self.assertEqual("ha_0.0.0_windows_amd64.zip", path.name)
        files = contract.check_archives.windows_files(path, contract.sha256(path.read_bytes()), "0.0.0")
        self.assertIn("ha.exe", files)


if __name__ == "__main__":
    unittest.main()
