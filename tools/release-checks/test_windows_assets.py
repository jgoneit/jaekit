"""The v0.1.3 acceptance checker accepts only the four archives and checksums.txt.

v0.1.3 was published without a Windows archive; this checker keeps no rule for
one. Synthetic inputs only; no network, extraction or installation.
"""
import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("windows_assets_consumer", HERE / "check.py")
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)
FOUR = [f"ha_{check.VERSION}_{p}.tar.gz" for p in check.PLATFORMS] + ["checksums.txt"]
WINDOWS = f"ha_{check.VERSION}_windows_amd64.zip"


class WindowsAssets(unittest.TestCase):
    def assets(self, names):
        collector = check.Checker()
        release = {"tag_name": check.TAG, "draft": False, "prerelease": False,
                   "assets": [{"name": name} for name in names]}
        patcher = patch.object(collector, "release", return_value=release)
        patcher.start()
        self.addCleanup(patcher.stop)
        return collector.assets()

    def test_only_the_published_v013_shape_is_accepted(self):
        self.assertEqual(set(self.assets(FOUR)), set(FOUR))
        for names in (FOUR + [WINDOWS], FOUR + ["install.ps1"], FOUR[1:], FOUR + [FOUR[0]]):
            with self.subTest(names=names), self.assertRaisesRegex(check.Failure, "exactly four archives"):
                self.assets(names)

    def test_no_separate_windows_archive_rule_remains(self):
        self.assertFalse(hasattr(check, "windows_archive") or hasattr(check, "WINDOWS_ARCHIVE"))


if __name__ == "__main__":
    unittest.main()
