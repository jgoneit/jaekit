"""Regression tests for the public checker, using synthetic documents only."""
import importlib.util
from pathlib import Path
import tempfile
import unittest

spec = importlib.util.spec_from_file_location("public_docs", Path(__file__).resolve().parents[1] / "check_public_docs.py")
public_docs = importlib.util.module_from_spec(spec)
spec.loader.exec_module(public_docs)


class PublicLinks(unittest.TestCase):
    def check(self, text, target=None):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            (root / "README.md").write_text(text)
            if target is not None:
                (root / "guide.md").write_text(target)
            return public_docs.check(root, [root / "README.md"])[1]

    def test_missing_file_and_anchor_are_reported(self):
        self.assertIn("missing target", self.check("[guide](guide.md)")[0])
        self.assertIn("missing anchor", self.check("[guide](guide.md#absent)", "# Present")[0])

    def test_unicode_code_underscores_and_duplicate_headings(self):
        text = "[one](guide.md#needs_user-처리) [two](guide.md#needs_user-처리-1)"
        self.assertEqual([], self.check(text, "## `needs_user` 처리\n## `needs_user` 처리\n"))

    def test_examples_and_remote_links_do_not_need_local_files(self):
        text = "```md\n[example](absent.md)\n```\n`[example](absent.md)`\n[web](https://example.com/no-file)"
        self.assertEqual([], self.check(text))

    def test_html_links_and_images_are_checked(self):
        self.assertEqual([], self.check('<a href="guide.md#part">part</a>', '<a id="part"></a>'))
        self.assertIn("missing target", self.check('<img src="missing.png" alt="example">')[0])

    def test_parent_directory_cannot_escape_public_tree(self):
        self.assertIn("leaves the repository", self.check("[outside](../outside.md)")[0])


if __name__ == "__main__":
    unittest.main()
