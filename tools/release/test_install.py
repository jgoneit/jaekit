"""Execute the published install blocks with local, synthetic downloads.

Only $HOME references in the block are rebound to INSTALL_TEST_ROOT. The real
process HOME and CODEX_HOME are unchanged. Command wrappers inject failures;
these checks do not install a public release or establish platform support.
"""
import hashlib
import os
from pathlib import Path
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
NAME = "ha_0.1.3_linux_amd64"
OLD = b"existing executable\n"
NEW = b"synthetic replacement executable\n"
GUIDES = (("INSTALL.md", "## Release 파일로 설치"),
          ("INSTALL.en.md", "## Install from a Release file"))


def install_block(filename, heading):
    document = (ROOT / "guides" / filename).read_text(encoding="utf-8")
    section = document.split(heading + "\n", 1)[1].split("\n## ", 1)[0]
    blocks = re.findall(r"```bash\n(.*?)```", section, re.S)
    if len(blocks) != 1:
        raise AssertionError("direct-install section must contain exactly one shell block")
    return blocks[0].replace("$HOME", "$INSTALL_TEST_ROOT")


WRAPPER = r'''
import os
from pathlib import Path
import shutil
import subprocess
import sys

name, *args = sys.argv[1:]
failure = os.environ.get("INSTALL_TEST_FAILURE", "")
if name == "uname":
    print("Linux" if args == ["-s"] else "x86_64")
    sys.exit(0)
if name == "curl":
    filename = args[-1].rsplit("/", 1)[-1]
    if filename not in {"ha_0.1.3_linux_amd64.tar.gz", "checksums.txt"}:
        sys.exit(9)
    shutil.copyfile(Path(os.environ["INSTALL_TEST_DIST"]) / filename, filename)
    sys.exit(0)
source, destination = Path(args[-2]), Path(args[-1])
if name == "cp" and source.name == "ha" and failure == "copy":
    destination.write_bytes(b"partial copy\n")
    sys.exit(1)
if name == "chmod" and failure == "chmod":
    sys.exit(1)
if name == "mv" and destination.name == "ha":
    if source.parent != destination.parent:
        # Model mv's cross-filesystem copy fallback failing after it has
        # opened the old destination. Staging avoids this destructive path.
        if failure == "copy":
            destination.write_bytes(b"partial cross-filesystem copy\n")
            sys.exit(1)
        raise SystemExit("binary replacement must rename within its destination directory")
    if failure == "rename":
        sys.exit(1)
sys.exit(subprocess.run([os.environ["INSTALL_TEST_REAL_" + name.upper()], *args]).returncode)
'''


class InstallChecks(unittest.TestCase):
    def fixture(self, root):
        package = root / NAME
        package.mkdir()
        for folder in ("tools", "examples", "guides", "contracts", "assets"):
            (package / folder).mkdir()
            (package / folder / "fixture.txt").write_text("synthetic public file\n")
        for name in ("README.md", "README.en.md", "LICENSE"):
            (package / name).write_text("synthetic public document\n")
        (package / "ha").write_bytes(NEW)
        (package / "ha").chmod(0o755)
        dist = root / "dist"
        dist.mkdir()
        archive = dist / (NAME + ".tar.gz")
        with tarfile.open(archive, "w:gz") as output:
            output.add(package, arcname=NAME)
        checksum = hashlib.sha256(archive.read_bytes()).hexdigest()
        (dist / "checksums.txt").write_text(checksum + "  " + archive.name + "\n")
        install_root = root / "installation"
        binary = install_root / ".local/bin/ha"
        binary.parent.mkdir(parents=True)
        binary.write_bytes(OLD)
        binary.chmod(0o755)
        wrapper = root / "wrapper.py"
        wrapper.write_text(WRAPPER)
        commands = root / "commands"
        commands.mkdir()
        env = dict(os.environ, INSTALL_TEST_ROOT=str(install_root), INSTALL_TEST_DIST=str(dist))
        for command in ("uname", "curl", "cp", "chmod", "mv"):
            real = shutil.which(command)
            self.assertIsNotNone(real, "required fixture command is missing: " + command)
            env["INSTALL_TEST_REAL_" + command.upper()] = real
            script = commands / command
            script.write_text("#!/bin/sh\nexec " + shlex.quote(sys.executable) + " " + shlex.quote(str(wrapper)) + " " + command + ' "$@"\n')
            script.chmod(0o755)
        env["PATH"] = str(commands) + os.pathsep + os.environ.get("PATH", "")
        return env, binary, install_root / ".local/share/jaekit/0.1.3"

    def run_block(self, filename, heading, root, env):
        return subprocess.run(["/bin/bash", "-c", install_block(filename, heading)],
                              cwd=root, env=env, capture_output=True, text=True, timeout=30)

    def test_success_and_identical_data_retry(self):
        for filename, heading in GUIDES:
            with self.subTest(guide=filename), tempfile.TemporaryDirectory(prefix="jaekit-install-test-") as directory:
                root = Path(directory)
                env, binary, share = self.fixture(root)
                for _ in range(2):
                    result = self.run_block(filename, heading, root, env)
                    self.assertEqual(result.returncode, 0, result.stderr)
                    self.assertEqual(binary.read_bytes(), NEW)
                    self.assertTrue(binary.stat().st_mode & 0o111)
                    self.assertEqual((share / "tools/fixture.txt").read_text(), "synthetic public file\n")
                    self.assertEqual(sorted(p.name for p in binary.parent.iterdir()), ["ha"])

    def test_checksum_data_and_replacement_failures_preserve_binary(self):
        for filename, heading in GUIDES:
            for failure in ("checksum", "share-conflict", "copy", "chmod", "rename"):
                with self.subTest(guide=filename, failure=failure), tempfile.TemporaryDirectory(prefix="jaekit-install-test-") as directory:
                    root = Path(directory)
                    env, binary, share = self.fixture(root)
                    before_mode = binary.stat().st_mode
                    if failure == "checksum":
                        (Path(env["INSTALL_TEST_DIST"]) / "checksums.txt").write_text("0" * 64 + "  " + NAME + ".tar.gz\n")
                    elif failure == "share-conflict":
                        share.mkdir(parents=True)
                        (share / "preserve.txt").write_text("user data\n")
                    else:
                        env["INSTALL_TEST_FAILURE"] = failure
                    result = self.run_block(filename, heading, root, env)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertEqual(binary.read_bytes(), OLD)
                    self.assertEqual(binary.stat().st_mode, before_mode)
                    self.assertEqual(sorted(p.name for p in binary.parent.iterdir()), ["ha"])
                    if failure == "share-conflict":
                        self.assertEqual(sorted(p.name for p in share.iterdir()), ["preserve.txt"])
                        self.assertEqual((share / "preserve.txt").read_text(), "user data\n")

    def test_destination_directory_and_symlinks_are_preserved(self):
        for filename, heading in GUIDES:
            for kind in ("directory", "file-link", "directory-link", "dangling-link"):
                with self.subTest(guide=filename, kind=kind), tempfile.TemporaryDirectory(prefix="jaekit-install-test-") as directory:
                    root = Path(directory)
                    env, binary, _ = self.fixture(root)
                    binary.unlink()
                    target = root / "preserve"
                    if kind == "directory":
                        binary.mkdir()
                    elif kind == "file-link":
                        target.write_bytes(OLD)
                        binary.symlink_to(target)
                    elif kind == "directory-link":
                        target.mkdir()
                        binary.symlink_to(target, target_is_directory=True)
                    else:
                        binary.symlink_to(target)
                    result = self.run_block(filename, heading, root, env)
                    self.assertNotEqual(result.returncode, 0, result.stdout)
                    self.assertEqual(sorted(p.name for p in binary.parent.iterdir()), ["ha"])
                    if kind == "directory":
                        self.assertTrue(binary.is_dir())
                        self.assertEqual(list(binary.iterdir()), [])
                    else:
                        self.assertTrue(binary.is_symlink())
                        self.assertEqual(binary.readlink(), target)
                        if kind == "file-link":
                            self.assertEqual(target.read_bytes(), OLD)
                        elif kind == "directory-link":
                            self.assertEqual(list(target.iterdir()), [])
                        else:
                            self.assertFalse(target.exists())


if __name__ == "__main__":
    unittest.main()
