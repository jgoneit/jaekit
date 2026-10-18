"""The CI reader expects exactly the package checks the Windows native checks report.

The two lists are kept by hand in two files. When they drift, the reader
rejects every real Windows report, however its checks went.
"""
import importlib.util
from pathlib import Path
import sys
import unittest


ROOT = Path(__file__).resolve().parents[2]


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class NativeReportPackageChecks(unittest.TestCase):
    def test_reader_expects_the_package_checks_the_native_checks_report(self):
        sys.path.insert(0, str(ROOT / "tools/windows"))
        try:
            native = load("reported_native_checks", "tools/windows/native_checks.py")
        finally:
            sys.path.remove(str(ROOT / "tools/windows"))
        reader = load("reading_cross_platform_ci", "tools/verification-checks/cross_platform_ci.py")
        self.assertEqual(reader.PACKAGE_CHECKS, set(native.PACKAGE_CHECKS))
        self.assertEqual(len(native.PACKAGE_CHECKS), len(set(native.PACKAGE_CHECKS)))


if __name__ == "__main__":
    unittest.main()
