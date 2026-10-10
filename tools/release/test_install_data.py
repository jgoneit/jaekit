"""Run the published direct-install data steps against a synthetic home.

The install, update and uninstall blocks are extracted from both guides. Every
`$HOME` and `~/` in a block is rebound to a temporary directory and the child
process gets that directory as HOME, so the real installation is untouched.
"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest

import test_install

ROOT = Path(__file__).resolve().parents[2]
SECTIONS = {
    "INSTALL.md": {"update": "## 업데이트", "uninstall": "## 제거", "reference": "## 참조 검사 가져오기"},
    "INSTALL.en.md": {"update": "## Update", "uninstall": "## Uninstall", "reference": "## Get the reference check"},
}
REFERENCE_FILES = ("tools/check-result-reference.py", "examples/check-result/declaration.json",
                   "examples/check-result/reference.json", "examples/check-result/input.txt")


def section(filename, heading):
    document = (ROOT / "guides" / filename).read_text(encoding="utf-8")
    return document.split(heading + "\n", 1)[1].split("\n## ", 1)[0]


def bash_blocks(text):
    return re.findall(r"```bash\n(.*?)```", text, re.S)


def bind(block, home):
    return block.replace("$HOME", str(home)).replace("~/", str(home) + "/")


def direct_blocks(filename, key):
    """Blocks of a section that act on the direct installation's paths."""
    return [b for b in bash_blocks(section(filename, SECTIONS[filename][key]))
            if ".local/bin/ha" in b or ".local/share/jaekit" in b]


class DirectData(unittest.TestCase):
    def home(self):
        temp = tempfile.TemporaryDirectory(prefix="jaekit-install-data-")
        self.addCleanup(temp.cleanup)
        return Path(temp.name).resolve()

    def run_bash(self, script, home, cwd):
        env = dict(os.environ, HOME=str(home))
        result = subprocess.run(["/bin/bash", "-c", "set -e\n" + script], cwd=cwd, env=env,
                                capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    @staticmethod
    def version_data(share, version):
        for relative in REFERENCE_FILES:
            path = share / version / relative
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(f"synthetic {version} {relative}\n")

    def project(self, home):
        project = home / "project"
        (project / "tools").mkdir(parents=True)
        (project / "tools/check-result-reference.py").write_text("copied producer\n")
        (project / "docs/specs/goal").mkdir(parents=True)
        (project / "docs/specs/goal/runs.jsonl").write_text("{}\n")
        return project

    # Uninstall --------------------------------------------------------------

    def test_uninstall_removes_binary_and_installed_data(self):
        for filename in SECTIONS:
            with self.subTest(guide=filename):
                home = self.home()
                binary = home / ".local/bin/ha"
                binary.parent.mkdir(parents=True)
                binary.write_text("installed\n")
                share = home / ".local/share/jaekit"
                for version in ("0.1.2", "0.1.3"):
                    self.version_data(share, version)
                project = self.project(home)
                blocks = direct_blocks(filename, "uninstall")
                self.assertTrue(any(".local/share/jaekit" in b for b in blocks),
                                f"{filename} uninstall does not remove the direct-install data")
                for block in blocks:
                    self.run_bash(bind(block, home), home, project)
                self.assertFalse(binary.exists() or binary.is_symlink())
                self.assertFalse(share.exists() or share.is_symlink())
                self.assertEqual((project / "tools/check-result-reference.py").read_text(), "copied producer\n")
                self.assertTrue((project / "docs/specs/goal/runs.jsonl").is_file())
                prose = section(filename, SECTIONS[filename]["uninstall"])
                self.assertIn("`tools/check-result-reference.py`", prose)

    def test_uninstall_keeps_the_target_of_a_linked_data_path(self):
        for filename in SECTIONS:
            with self.subTest(guide=filename):
                home = self.home()
                outside = home / "outside-data"
                self.version_data(outside, "0.1.3")
                share = home / ".local/share/jaekit"
                share.parent.mkdir(parents=True)
                share.symlink_to(outside, target_is_directory=True)
                for block in direct_blocks(filename, "uninstall"):
                    if ".local/share/jaekit" in block:
                        self.run_bash(bind(block, home), home, home)
                for relative in REFERENCE_FILES:
                    self.assertEqual((outside / "0.1.3" / relative).read_text(), f"synthetic 0.1.3 {relative}\n")

    # Update -----------------------------------------------------------------

    def test_update_cleanup_keeps_the_current_version(self):
        for filename in SECTIONS:
            with self.subTest(guide=filename):
                home = self.home()
                share = home / ".local/share/jaekit"
                for version in ("0.1.1", "0.1.2", "0.1.3"):
                    self.version_data(share, version)
                cleanup = [b for b in direct_blocks(filename, "update") if ".local/share/jaekit" in b]
                self.assertEqual(len(cleanup), 1, f"{filename} update has no single data cleanup block")
                self.run_bash(bind(cleanup[0], home), home, home)
                self.assertEqual(sorted(p.name for p in share.iterdir()), ["0.1.3"])
                prose = section(filename, SECTIONS[filename]["update"])
                self.assertIn("~/.local/share/jaekit", prose)
                # The current version's reference acquisition still works.
                blocks = bash_blocks(section(filename, SECTIONS[filename]["reference"]))
                direct = [b for b in blocks if ".local/share/jaekit" in b]
                copy = [b for b in blocks if "cp \"$package_root/" in b]
                self.assertEqual((len(direct), len(copy)), (1, 1))
                project = home / "project"
                project.mkdir()
                self.run_bash(bind(direct[0], home) + copy[0], home, project)
                self.assertEqual((project / "tools/check-result-reference.py").read_text(),
                                 "synthetic 0.1.3 tools/check-result-reference.py\n")
                self.assertEqual((project / "sample/input.txt").read_text(),
                                 "synthetic 0.1.3 examples/check-result/input.txt\n")

    def test_update_cleanup_does_not_follow_a_linked_data_root(self):
        for filename in SECTIONS:
            with self.subTest(guide=filename):
                home = self.home()
                outside = home / "outside-data"
                for version in ("0.1.2", "0.1.3"):
                    self.version_data(outside, version)
                share = home / ".local/share/jaekit"
                share.parent.mkdir(parents=True)
                share.symlink_to(outside, target_is_directory=True)
                for block in direct_blocks(filename, "update"):
                    if ".local/share/jaekit" in block:
                        self.run_bash(bind(block, home), home, home)
                self.assertEqual(sorted(p.name for p in outside.iterdir()), ["0.1.2", "0.1.3"])

    # Unsupported platforms ----------------------------------------------------

    def test_unsupported_platform_stops_before_changes(self):
        for filename, heading in test_install.GUIDES:
            for system, machine in (("SunOS", "x86_64"), ("Linux", "sparc64")):
                with self.subTest(guide=filename, platform=system + "/" + machine), \
                        tempfile.TemporaryDirectory(prefix="jaekit-install-test-") as directory:
                    root = Path(directory)
                    env, binary, share = test_install.InstallChecks.fixture(self, root)
                    uname = Path(env["PATH"].split(os.pathsep)[0]) / "uname"
                    uname.write_text(f'#!/bin/sh\nif [ "$1" = -s ]; then echo {system}; else echo {machine}; fi\n')
                    result = test_install.InstallChecks.run_block(self, filename, heading, root, env)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertEqual(binary.read_bytes(), test_install.OLD)
                    self.assertFalse(share.exists() or share.is_symlink())
                    self.assertFalse((root / "installation/.local/share").exists())

    # Reused release data ----------------------------------------------------

    def rejected_reuse(self, change):
        """Install once, alter the reused data, and require a refused second install."""
        for filename, heading in test_install.GUIDES:
            with self.subTest(guide=filename), tempfile.TemporaryDirectory(prefix="jaekit-install-test-") as directory:
                root = Path(directory)
                env, binary, share = test_install.InstallChecks.fixture(self, root)
                first = test_install.InstallChecks.run_block(self, filename, heading, root, env)
                self.assertEqual(first.returncode, 0, first.stderr)
                binary.write_bytes(test_install.OLD)
                binary.chmod(0o755)
                before = binary.stat().st_mode
                entry, verify = change(root, share)
                result = test_install.InstallChecks.run_block(self, filename, heading, root, env)
                self.assertNotEqual(result.returncode, 0, "install reused data containing " + entry)
                self.assertEqual(binary.read_bytes(), test_install.OLD)
                self.assertEqual(binary.stat().st_mode, before)
                self.assertEqual(sorted(p.name for p in binary.parent.iterdir()), ["ha"])
                self.assertIn(entry, result.stdout + result.stderr)
                verify()

    def test_reuse_rejects_a_file_link_with_identical_content(self):
        def change(root, share):
            outside = root / "outside.txt"
            shutil.copyfile(share / "tools/fixture.txt", outside)
            (share / "tools/fixture.txt").unlink()
            (share / "tools/fixture.txt").symlink_to(outside)

            def verify():
                self.assertEqual((share / "tools/fixture.txt").readlink(), outside)
                self.assertEqual(outside.read_text(), "synthetic public file\n")
            return "tools/fixture.txt", verify
        self.rejected_reuse(change)

    def test_reuse_rejects_a_directory_link_with_identical_content(self):
        def change(root, share):
            outside = root / "outside-examples"
            shutil.copytree(share / "examples", outside)
            shutil.rmtree(share / "examples")
            (share / "examples").symlink_to(outside, target_is_directory=True)

            def verify():
                self.assertEqual((share / "examples").readlink(), outside)
                self.assertEqual((outside / "fixture.txt").read_text(), "synthetic public file\n")
            return "examples", verify
        self.rejected_reuse(change)

    def test_reuse_rejects_a_special_file(self):
        def change(root, share):
            fifo = share / "tools/extra.fifo"
            os.mkfifo(fifo)

            def verify():
                self.assertTrue(fifo.is_fifo())
            return "extra.fifo", verify
        self.rejected_reuse(change)


if __name__ == "__main__":
    unittest.main()
