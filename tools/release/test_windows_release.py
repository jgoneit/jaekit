"""Windows release archive and publication gate regressions.

Synthetic files only; no network, installation, tag or publication.
"""
import hashlib
import importlib.util
import json
from pathlib import Path
import struct
import unittest
import zipfile


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


release = load("windows_release_archives", "check_archives.py")
gate = load("windows_release_gate", "windows_gate.py")
fixtures = load("windows_release_fixtures", "test_archives.py")
# Not imported as a name here, so discovery does not collect its tests twice.
_spec = importlib.util.spec_from_file_location(
    "windows_release_package_tests", Path(__file__).resolve().parents[1] / "windows/test_package.py")
package_tests = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(package_tests)
VERSION = fixtures.VERSION
ZIP = f"ha_{VERSION}_windows_amd64.zip"


def pe_amd64():
    data = bytearray(512)
    data[:2] = b"MZ"
    struct.pack_into("<I", data, 0x3C, 0x80)
    data[0x80:0x84] = b"PE\0\0"
    struct.pack_into("<H", data, 0x84, 0x8664)
    struct.pack_into("<H", data, 0x80 + 24, 0x20B)
    return bytes(data)


def windows_zip(path, files, manifest=None, extra=()):
    """Write the package.py layout: files plus a package.json manifest."""
    if manifest is None:
        manifest = {"schema": "jaekit-windows-package/v1", "version": VERSION, "os": "windows", "arch": "amd64",
                    "files": [{"path": name, "size": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                              for name, body in sorted(files.items())]}
    entries = dict(files)
    if manifest is not False:
        entries["package.json"] = (json.dumps(manifest, sort_keys=True, indent=2) + "\n").encode()
    with zipfile.ZipFile(path, "w", compression=zipfile.ZIP_DEFLATED) as archive:
        for name, body in sorted(entries.items()):
            info = zipfile.ZipInfo(name, date_time=(1980, 1, 1, 0, 0, 0))
            info.create_system = 3
            info.external_attr = (0o100000 | 0o644) << 16
            info.compress_type = zipfile.ZIP_DEFLATED
            archive.writestr(info, body)
        for info, body in extra:
            archive.writestr(info, body)


class Fixture(unittest.TestCase):
    def setUp(self):
        self.base = fixtures.ArchiveChecks("test_four_platform_archives_and_uploaded_names")
        self.base.setUp()
        self.addCleanup(self.base.doCleanups)
        self.dist = self.base.dist
        self.files = {name: b"Synthetic public document.\n" for name in release.WINDOWS_REQUIRED - {"package.json"}}
        self.files["ha.exe"] = pe_amd64()
        self.write()

    def write(self, files=None, manifest=None, extra=()):
        windows_zip(self.dist / ZIP, self.files if files is None else files, manifest, extra)
        self.checksums()

    def checksums(self, names=None):
        paths = sorted(self.dist.glob("*.tar.gz")) + sorted(self.dist.glob("*.zip"))
        lines = [release.digest(path) + "  " + path.name for path in paths if names is None or path.name in names]
        (self.dist / "checksums.txt").write_text("".join(line + "\n" for line in lines))


class WindowsRelease(Fixture):
    def test_four_archives_and_windows_archive_share_one_checksum_file(self):
        names = release.check(self.dist, VERSION, require_windows=True)
        self.assertEqual(set(names), release.release_names(VERSION) | {ZIP})
        lines = (self.dist / "checksums.txt").read_text().splitlines()
        self.assertEqual(len(lines), 5)
        self.assertEqual(sum(line.endswith("  " + ZIP) for line in lines), 1)
        uploaded = self.dist.parent / "uploaded.txt"
        uploaded.write_text("".join(name + "\n" for name in sorted(names + ["checksums.txt"])))
        release.check(self.dist, VERSION, uploaded)

    def test_release_without_the_windows_archive_is_still_valid_unless_required(self):
        (self.dist / ZIP).unlink()
        self.checksums()
        self.assertEqual(set(release.check(self.dist, VERSION)), release.release_names(VERSION))
        with self.assertRaisesRegex(ValueError, "exactly four"):
            release.check(self.dist, VERSION, require_windows=True)

    def test_checksum_file_must_match_the_windows_archive_presence(self):
        self.checksums(names=release.release_names(VERSION))
        with self.assertRaisesRegex(ValueError, "exactly the four"):
            release.check(self.dist, VERSION)
        self.checksums()
        (self.dist / ZIP).unlink()
        with self.assertRaisesRegex(ValueError, "exactly the four"):
            release.check(self.dist, VERSION)

    def test_tampered_windows_archives_are_refused(self):
        changed = dict(self.files)
        manifest = {"schema": "jaekit-windows-package/v1", "version": VERSION, "os": "windows", "arch": "amd64",
                    "files": [{"path": name, "size": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                              for name, body in sorted(self.files.items())]}
        changed["tools/windows/install.ps1"] = b"Write-Output tampered\n"
        elf = dict(self.files, **{"ha.exe": fixtures.executable_header("linux_amd64")})
        elf_manifest = dict(manifest, files=[{"path": name, "size": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                                            for name, body in sorted(elf.items())])
        traversal = zipfile.ZipInfo("../outside.txt", date_time=(1980, 1, 1, 0, 0, 0))
        reserved = zipfile.ZipInfo("guides/CON.md", date_time=(1980, 1, 1, 0, 0, 0))
        link = zipfile.ZipInfo("guides/link.md", date_time=(1980, 1, 1, 0, 0, 0))
        link.external_attr = (0o120000 | 0o777) << 16
        directory = zipfile.ZipInfo("guides/folder/", date_time=(1980, 1, 1, 0, 0, 0))
        cases = {
            "file differs from its manifest": dict(files=changed, manifest=manifest),
            "missing required files: package.json": dict(manifest=False),
            "manifest version differs": dict(manifest=dict(manifest, version="0.1.9")),
            "unsafe path": dict(extra=[(traversal, b"x")]),
            "unsafe path: 'guides/CON.md'": dict(extra=[(reserved, b"x")]),
            "link or special file": dict(extra=[(link, b"target")]),
            "unsafe path: 'guides/folder/'": dict(extra=[(directory, b"")]),
            "differ from its manifest": dict(extra=[(zipfile.ZipInfo("guides/extra.md"), b"extra")]),
            "Windows archive Core": dict(files=elf, manifest=elf_manifest),
        }
        for message, arguments in cases.items():
            with self.subTest(message=message):
                self.write(**arguments)
                with self.assertRaisesRegex(ValueError, message):
                    release.check(self.dist, VERSION)
        self.write()
        with (self.dist / ZIP).open("ab") as output:
            output.write(b"changed")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            release.check(self.dist, VERSION)

    def test_manifest_fields_other_than_package_py_fields_are_refused(self):
        manifest = {"schema": "jaekit-windows-package/v1", "version": VERSION, "os": "windows", "arch": "amd64",
                    "files": [{"path": name, "size": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                              for name, body in sorted(self.files.items())]}
        entries = [dict(item, note=1) for item in manifest["files"]]
        for label, changed in {"an extra field": dict(manifest, note=1),
                               "an extra entry field": dict(manifest, files=entries)}.items():
            with self.subTest(label=label):
                self.write(manifest=changed)
                with self.assertRaisesRegex(ValueError, "fields package.py does not write"):
                    release.check(self.dist, VERSION)

    def test_repeated_paths_are_refused(self):
        self.write(extra=[(zipfile.ZipInfo("LICENSE", date_time=(1980, 1, 1, 0, 0, 0)), b"again")])
        with self.assertRaisesRegex(ValueError, "repeats a path"):
            release.check(self.dist, VERSION)

    def test_paths_package_py_does_not_package_are_refused(self):
        self.write(files=dict(self.files, **{"notes/extra.md": b"x\n"}))
        with self.assertRaisesRegex(ValueError, "path package.py does not package"):
            release.check(self.dist, VERSION)

    def test_a_package_py_archive_passes_the_check(self):
        # package.py's own synthetic Git source fixture and a synthetic PE32+ Core.
        fixture = package_tests.CandidatePackageTests("prepare")
        fixture.setUp()
        self.addCleanup(fixture.doCleanups)
        path = fixture.prepare(version="0.0.0")
        self.assertEqual("ha_0.0.0_windows_amd64.zip", path.name)
        files = release.windows_files(path, hashlib.sha256(path.read_bytes()).hexdigest(), "0.0.0")
        self.assertIn("ha.exe", files)


class PublicationGate(Fixture):
    def digest(self):
        return release.digest(self.dist / ZIP)

    def test_linked_acceptance_keeps_the_windows_archive(self):
        digest = self.digest()
        kept, _ = gate.select(self.dist, VERSION, digest, "jaekit v0.1.4\n\nWindows-Accepted-SHA256: " + digest + "\n")
        self.assertTrue(kept)
        self.assertEqual(set(release.check(self.dist, VERSION, require_windows=True)),
                         release.release_names(VERSION) | {ZIP})

    def test_missing_or_unlinked_acceptance_leaves_out_only_the_windows_archive(self):
        digest = self.digest()
        messages = {
            "no line": "",
            "two lines": f"Windows-Accepted-SHA256: {digest}\nWindows-Accepted-SHA256: {digest}\n",
            "malformed": "Windows-Accepted-SHA256: " + digest.upper() + "\n",
            "other digest": "Windows-Accepted-SHA256: " + "0" * 64 + "\n",
        }
        for label, message in messages.items():
            with self.subTest(label=label):
                self.write()
                kept, reason = gate.select(self.dist, VERSION, self.digest(), message)
                self.assertFalse(kept, reason)
                self.assertFalse((self.dist / ZIP).exists())
                lines = (self.dist / "checksums.txt").read_text().splitlines()
                self.assertEqual(len(lines), 4)
                self.assertTrue(all(line.endswith(".tar.gz") for line in lines))
                self.assertEqual(set(release.check(self.dist, VERSION)), release.release_names(VERSION))

    def test_a_different_checked_archive_stops_publication_without_changes(self):
        before = {path.name: path.read_bytes() for path in self.dist.iterdir()}
        for checked in ("1" * 64, "", "not-a-digest"):
            with self.subTest(checked=checked), self.assertRaises(gate.Refused):
                gate.select(self.dist, VERSION, checked, "Windows-Accepted-SHA256: " + self.digest() + "\n")
        self.assertEqual({path.name: path.read_bytes() for path in self.dist.iterdir()}, before)

    def test_command_line_reports_the_decision(self):
        notes = self.dist.parent / "notes.md"
        notes.write_text("")
        self.assertEqual(gate.main(["--dist", str(self.dist), "--version", VERSION,
                                    "--checked-sha256", self.digest(), "--notes", str(notes)]), 0)
        self.assertFalse((self.dist / ZIP).exists())
        self.write()
        self.assertEqual(gate.main(["--dist", str(self.dist), "--version", VERSION,
                                    "--checked-sha256", "0" * 64, "--notes", str(notes)]), 1)
        self.assertTrue((self.dist / ZIP).exists())


if __name__ == "__main__":
    unittest.main()
