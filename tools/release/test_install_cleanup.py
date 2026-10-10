"""Run both guides' update cleanup against synthetic direct-install data.

Only folders of versions older than the installed version are removed; the
current and newer versions and entries that are not version folders stay.
Every `$HOME` and `~/` in a block is rebound to a temporary directory.
"""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

import test_install_data as data

CURRENT = "0.1.3"
OLDER = ("0.0.9", "0.0.10", "0.1.2")
NEWER = ("0.1.4", "0.1.10", "0.2.0", "1.0.0")
OTHER_DIRECTORIES = ("notes", "latest", "0.1", "0.1.2-rc1", "0.1.2.1", "v0.1.2", "00.x.1")


def cleanup_block(filename):
    blocks = [b for b in data.direct_blocks(filename, "update") if ".local/share/jaekit" in b]
    if len(blocks) != 1:
        raise AssertionError(f"{filename} update has {len(blocks)} data cleanup blocks, not one")
    return blocks[0]


def update_prose(filename):
    section = data.section(filename, data.SECTIONS[filename]["update"])
    return section.split(cleanup_block(filename), 1)[0].rsplit("\n\n", 2)[-2]


class UpdateCleanup(unittest.TestCase):
    def home(self):
        temp = tempfile.TemporaryDirectory(prefix="jaekit-install-cleanup-")
        self.addCleanup(temp.cleanup)
        return Path(temp.name).resolve()

    def run_block(self, shell, block, home):
        env = dict(os.environ, HOME=str(home))
        argv = [shell, "-f", "-c", "set -e\n" + data.bind(block, home)] if shell.endswith("zsh") else \
            [shell, "-c", "set -e\n" + data.bind(block, home)]
        result = subprocess.run(argv, cwd=home, env=env, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)

    def shells(self):
        return [shell for shell in ("/bin/bash", "/bin/zsh") if Path(shell).exists()]

    def populate(self, share):
        for version in OLDER + (CURRENT,) + NEWER:
            data.DirectData.version_data(share, version)
        for name in OTHER_DIRECTORIES:
            (share / name).mkdir()
            (share / name / "kept.txt").write_text(name)
        (share / "README").write_text("not a version folder\n")
        (share / "0.0.8").write_text("a file named like a version\n")
        outside = share.parent / "outside-0.0.7"
        data.DirectData.version_data(outside, "0.0.7")
        (share / "0.0.7").symlink_to(outside / "0.0.7", target_is_directory=True)
        return outside

    def test_only_versions_older_than_the_installed_one_are_removed(self):
        for filename in data.SECTIONS:
            for shell in self.shells():
                with self.subTest(guide=filename, shell=shell):
                    home = self.home()
                    share = home / ".local/share/jaekit"
                    outside = self.populate(share)
                    self.run_block(shell, cleanup_block(filename), home)
                    kept = {p.name for p in share.iterdir()}
                    self.assertEqual(kept & set(OLDER), set(), "older version folders remain")
                    self.assertTrue({CURRENT, *NEWER, *OTHER_DIRECTORIES, "README", "0.0.8"} <= kept,
                                    f"removed more than older versions: {sorted(kept)}")
                    for name in OTHER_DIRECTORIES:
                        self.assertEqual((share / name / "kept.txt").read_text(), name)
                    self.assertTrue((outside / "0.0.7/tools/check-result-reference.py").is_file(),
                                    "a linked version folder's target was removed")
                    # Running it again changes nothing and still succeeds.
                    self.run_block(shell, cleanup_block(filename), home)
                    self.assertEqual({p.name for p in share.iterdir()}, kept)

    def test_linked_or_missing_data_root_removes_nothing(self):
        for filename in data.SECTIONS:
            with self.subTest(guide=filename):
                home = self.home()
                outside = home / "outside-data"
                for version in OLDER + (CURRENT,):
                    data.DirectData.version_data(outside, version)
                share = home / ".local/share/jaekit"
                share.parent.mkdir(parents=True)
                share.symlink_to(outside, target_is_directory=True)
                self.run_block("/bin/bash", cleanup_block(filename), home)
                self.assertEqual(sorted(p.name for p in outside.iterdir()), sorted(OLDER + (CURRENT,)))
                share.unlink()
                self.run_block("/bin/bash", cleanup_block(filename), home)
                self.assertFalse(share.exists())

    def test_reference_copy_still_works_after_cleanup(self):
        for filename in data.SECTIONS:
            with self.subTest(guide=filename):
                home = self.home()
                share = home / ".local/share/jaekit"
                self.populate(share)
                self.run_block("/bin/bash", cleanup_block(filename), home)
                blocks = data.bash_blocks(data.section(filename, data.SECTIONS[filename]["reference"]))
                direct = [b for b in blocks if ".local/share/jaekit" in b]
                copy = [b for b in blocks if "cp \"$package_root/" in b]
                self.assertEqual((len(direct), len(copy)), (1, 1))
                project = home / "project"
                project.mkdir()
                env = dict(os.environ, HOME=str(home))
                result = subprocess.run(["/bin/bash", "-c", "set -e\n" + data.bind(direct[0], home) + copy[0]],
                                        cwd=project, env=env, capture_output=True, text=True, timeout=30)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual((project / "tools/check-result-reference.py").read_text(),
                                 f"synthetic {CURRENT} tools/check-result-reference.py\n")

    def test_both_guides_share_the_block_and_describe_it(self):
        self.assertEqual(cleanup_block("INSTALL.md"), cleanup_block("INSTALL.en.md"))
        phrases = {"INSTALL.md": ("오래된 버전 폴더만", "더 새 버전 폴더", "버전 이름이 아닌 항목", "symlink"),
                   "INSTALL.en.md": ("versions older than the installed", "newer versions",
                                     "not a three-number version", "symlink")}
        for filename, expected in phrases.items():
            prose = update_prose(filename)
            for phrase in expected:
                with self.subTest(guide=filename, phrase=phrase):
                    self.assertIn(phrase, prose)
            self.assertIn("~/.local/share/jaekit", prose)


if __name__ == "__main__":
    unittest.main()
