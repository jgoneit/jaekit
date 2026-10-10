"""Regression tests for the public file boundary, using synthetic references only."""
import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location("public_tree", Path(__file__).resolve().parents[1] / "check_public_tree.py")
public_tree = importlib.util.module_from_spec(spec)
spec.loader.exec_module(public_tree)

# Built by concatenation so this file does not trip the checker it tests.
SIBLING = "jgoneit/" + "jaekit-notes"


def flagged(text):
    return any(pattern.search(text) for pattern in public_tree.PRIVATE_REFERENCES)


class PrivateReferences(unittest.TestCase):
    def test_sibling_repository_links_are_flagged(self):
        self.assertTrue(flagged(f"[history](https://github.com/{SIBLING})"))
        self.assertTrue(flagged(f"see https://github.com/{SIBLING}/pull/15"))
        self.assertTrue(flagged(f"git clone git@github.com:{SIBLING}.git"))

    def test_sibling_repository_links_are_case_insensitive(self):
        for url in (
            f"https://GITHUB.COM/{SIBLING}",
            f"https://github.com/{SIBLING.replace('jgoneit', 'JGONEIT')}",
            f"git@github.com:{SIBLING.replace('jaekit', 'JaeKit')}.git",
        ):
            with self.subTest(url=url):
                self.assertTrue(flagged(url))

    def test_public_repository_and_release_assets_pass(self):
        self.assertFalse(flagged("https://github.com/jgoneit/jaekit"))
        self.assertFalse(flagged("https://github.com/jgoneit/jaekit/releases/download/v0.1.2/jaekit-darwin-arm64.tar.gz"))


if __name__ == "__main__":
    unittest.main()
