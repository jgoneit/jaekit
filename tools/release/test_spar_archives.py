"""Synthetic regressions for spar next to ha in the release archives.

No network, installation, or publication. Archives are built in a temporary
directory; executables are headers only, except in the native smoke case,
which packs scripts that print their version.
"""
import importlib.util
import io
import os
from pathlib import Path
import platform
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("spar_release_archives", HERE / "check_archives.py")
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
helper_spec = importlib.util.spec_from_file_location("spar_archive_headers", HERE / "test_archives.py")
helpers = importlib.util.module_from_spec(helper_spec)
helper_spec.loader.exec_module(helpers)
VERSION = "0.1.6"


class SparArchives(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="jaekit-spar-archive-test-")
        self.addCleanup(self.temporary.cleanup)
        self.dist = Path(self.temporary.name) / "dist"
        self.dist.mkdir()
        self.contents = {name: b"Synthetic public document.\n" for name in release.REQUIRED}

    def archive(self, target, spar="same", spar_mode=0o755, bodies=None):
        prefix = f"ha_{VERSION}_{target}"
        files = dict(self.contents)
        files["ha"] = (bodies or {}).get("ha", helpers.executable_header(target))
        if spar:
            spar_target = target if spar == "same" else spar
            files["spar"] = (bodies or {}).get("spar", helpers.executable_header(spar_target))
        with tarfile.open(self.dist / (prefix + ".tar.gz"), "w:gz") as archive:
            for name, value in files.items():
                info = tarfile.TarInfo(prefix + "/" + name)
                info.size = len(value)
                info.mode = 0o755 if name == "ha" else spar_mode if name == "spar" else 0o644
                archive.addfile(info, io.BytesIO(value))

    def checksums(self):
        lines = [release.digest(path) + "  " + path.name for path in sorted(self.dist.glob("*.tar.gz"))]
        (self.dist / "checksums.txt").write_text("\n".join(lines) + "\n")

    def build(self, **changes):
        for target in release.TARGETS:
            self.archive(target, **changes.get(target, {}))
        self.checksums()

    def test_spar_next_to_ha_is_accepted_and_optional_without_the_flag(self):
        self.build()
        self.assertEqual(set(release.check(self.dist, VERSION, require_spar=True)), release.release_names(VERSION))
        self.build(linux_arm64={"spar": None})
        release.check(self.dist, VERSION)
        with self.assertRaisesRegex(ValueError, "missing spar"):
            release.check(self.dist, VERSION, require_spar=True)

    def test_spar_must_be_executable_and_for_the_archive_platform(self):
        self.build(darwin_amd64={"spar_mode": 0o644})
        with self.assertRaisesRegex(ValueError, "spar is not executable"):
            release.check(self.dist, VERSION)
        self.build(linux_amd64={"spar": "linux_arm64"})
        with self.assertRaisesRegex(ValueError, "packaged spar platform"):
            release.check(self.dist, VERSION, require_spar=True)

    def test_ha_only_root_files_stay_unchanged(self):
        self.assertEqual(release.ROOT_FILES, {"ha", "LICENSE", "README.md", "README.ko.md"})
        self.assertNotIn("spar", release.REQUIRED)

    def test_command_line_requires_spar_only_when_asked(self):
        self.build(darwin_arm64={"spar": None})
        base = [sys.executable, str(HERE / "check_archives.py"), "--dist", str(self.dist), "--version", VERSION]
        self.assertEqual(subprocess.run(base, capture_output=True, text=True).returncode, 0)
        result = subprocess.run(base + ["--require-spar"], capture_output=True, text=True)
        self.assertEqual(result.returncode, 1)
        self.assertIn("spar", result.stderr)

    @unittest.skipUnless(os.name == "posix", "the native smoke runs packaged scripts on macOS or Linux")
    def test_native_smoke_compares_spar_and_ha_versions(self):
        system = {"Linux": "linux", "Darwin": "darwin"}.get(platform.system())
        arch = {"x86_64": "amd64", "amd64": "amd64", "arm64": "arm64", "aarch64": "arm64"}.get(platform.machine().lower())
        if not system or not arch:
            self.skipTest("no release archive for this platform")
        native = f"{system}_{arch}"
        script = "#!/bin/sh\necho {} {}\n"
        for spar_version, accepted in ((VERSION, True), ("9.9.9", False)):
            with self.subTest(spar_version=spar_version):
                self.build(**{native: {"bodies": {"ha": script.format("ha", VERSION).encode(),
                                                 "spar": script.format("spar", spar_version).encode()}}})
                # Only the version comparison is under test here; the Core's
                # own capability smoke is covered by test_archives.py.
                calls = []

                def fake(argv, cwd, env=None, expected=0):
                    calls.append(Path(argv[0]).name)
                    if Path(argv[0]).name in ("ha", "spar") and argv[1:] == ["--version"]:
                        return release.subprocess.run(argv, capture_output=True, text=True).stdout
                    raise StopIteration
                with mock.patch.object(release, "invoke", side_effect=fake):
                    if accepted:
                        with self.assertRaises(StopIteration):
                            release.native_smoke(self.dist, VERSION, require_spar=True)
                    else:
                        with self.assertRaisesRegex(ValueError, "spar version"):
                            release.native_smoke(self.dist, VERSION, require_spar=True)
                self.assertEqual(calls[:2], ["ha", "spar"])


if __name__ == "__main__":
    unittest.main()
