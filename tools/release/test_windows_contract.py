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
spec = importlib.util.spec_from_file_location("windows_contract_under_test", HERE / "windows_contract.py")
contract = importlib.util.module_from_spec(spec)
spec.loader.exec_module(contract)


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
        agreeing = contract.contract(lambda shell, directory, destination: (expected[self.current(directory)], ""))
        self.assertTrue(agreeing["passed"])
        self.assertEqual({"powershell", "pwsh"}, set(agreeing["cases"][0]["installer"]))
        lenient = contract.contract(lambda shell, directory, destination: ("accept", ""))
        self.assertFalse(lenient["passed"])
        silent = contract.contract(lambda shell, directory, destination: ("error", "no verdict"))
        self.assertFalse(silent["passed"])

    def current(self, directory):
        number = int(directory.name.removeprefix("case-"))
        return contract.cases()[number][0]


if __name__ == "__main__":
    unittest.main()
