"""Windows release archive, publication gate and published-tag regressions.

Synthetic files only; no network, installation, tag or publication.
"""
import contextlib
import hashlib
import http.client
import importlib.util
import io
import json
from pathlib import Path
import struct
import tempfile
import unittest
import unittest.mock
import zipfile


def load(name, filename):
    spec = importlib.util.spec_from_file_location(name, Path(__file__).with_name(filename))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


release = load("windows_release_archives", "check_archives.py")
gate = load("windows_release_gate", "windows_gate.py")
published = load("windows_release_published", "check_published.py")
fixtures = load("windows_release_fixtures", "test_archives.py")
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

    def test_manifest_json_the_installer_refuses_is_refused(self):
        manifest = {"schema": "jaekit-windows-package/v1", "version": VERSION, "os": "windows", "arch": "amd64",
                    "files": [{"path": name, "size": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                              for name, body in sorted(self.files.items())]}
        entries = [dict(item, note=1.0) for item in manifest["files"]]
        for label, changed in {"an empty property name": dict(manifest, **{"": 0}),
                               "names differing only in case": dict(manifest, Schema="other"),
                               "a non-finite number": dict(manifest, extra=float("nan")),
                               "an out-of-range number": dict(manifest, extra=float("inf")),
                               "an extra entry field": dict(manifest, files=entries)}.items():
            with self.subTest(label=label):
                self.write(manifest=changed)
                with self.assertRaisesRegex(ValueError, "Windows archive manifest"):
                    release.check(self.dist, VERSION)

    def test_repeated_and_case_colliding_paths_are_refused(self):
        repeated = zipfile.ZipInfo("LICENSE", date_time=(1980, 1, 1, 0, 0, 0))
        folded = zipfile.ZipInfo("license", date_time=(1980, 1, 1, 0, 0, 0))
        for message, extra in {"repeats a path": [(repeated, b"again")],
                               "differ only in case": [(folded, b"again")]}.items():
            with self.subTest(message=message):
                self.write(extra=extra)
                with self.assertRaisesRegex(ValueError, message):
                    release.check(self.dist, VERSION)


    def test_non_ascii_names_are_refused(self):
        # Windows may fold, normalize or ignore parts of them differently.
        for name in ("guides/\u03c2.md", "guides/\u0131.md", "guides/cafe\u0301.md"):
            with self.subTest(name=name):
                self.write(files=dict(self.files, **{name: b"guide\n"}))
                with self.assertRaisesRegex(ValueError, "non-ASCII name"):
                    release.check(self.dist, VERSION)

    def test_a_file_named_like_a_folder_in_another_case_is_refused(self):
        self.write(files=dict(self.files, **{"GUIDES": b"file\n", "guides/EXTRA.md": b"guide\n"}))
        with self.assertRaisesRegex(ValueError, "file/directory collision"):
            release.check(self.dist, VERSION)

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
        message = self.dist.parent / "tag-message.txt"
        message.write_text("")
        self.assertEqual(gate.main(["--dist", str(self.dist), "--version", VERSION,
                                    "--checked-sha256", self.digest(), "--tag-message", str(message)]), 0)
        self.assertFalse((self.dist / ZIP).exists())
        self.write()
        self.assertEqual(gate.main(["--dist", str(self.dist), "--version", VERSION,
                                    "--checked-sha256", "0" * 64, "--tag-message", str(message)]), 1)
        self.assertTrue((self.dist / ZIP).exists())


class PublishedRelease(Fixture):
    def metadata(self, names=None):
        assets = []
        for path in sorted(self.dist.iterdir()):
            if names is not None and path.name not in names:
                continue
            data = path.read_bytes()
            assets.append({"name": path.name, "size": len(data), "digest": "sha256:" + hashlib.sha256(data).hexdigest(),
                           "browser_download_url": f"https://github.com/jgoneit/jaekit/releases/download/v{VERSION}/{path.name}"})
        return {"tag_name": "v" + VERSION, "draft": False, "assets": assets}

    def download(self, url):
        return (self.dist / url.rsplit("/", 1)[1]).read_bytes()

    def tag(self, message, annotated=True):
        return lambda repo, tag: (annotated, message)

    def linked(self):
        return self.tag(f"jaekit v{VERSION}\n\nWindows-Accepted-SHA256: {release.digest(self.dist / ZIP)}\n")

    def test_both_published_shapes_pass(self):
        archives = published.check_release("jgoneit/jaekit", "v" + VERSION, self.metadata(), self.download,
                                           tag_reader=self.linked())
        self.assertIn(ZIP, archives)
        (self.dist / ZIP).unlink()
        self.checksums()

        def unread(repo, tag):
            raise AssertionError("a release without the Windows archive does not need its tag")

        archives = published.check_release("jgoneit/jaekit", "v" + VERSION, self.metadata(), self.download,
                                           tag_reader=unread)
        self.assertNotIn(ZIP, archives)

    def test_a_published_windows_archive_needs_its_acceptance_link(self):
        digest = release.digest(self.dist / ZIP)
        line = "Windows-Accepted-SHA256: " + digest
        for label, reader in {
            "lightweight tag": self.tag("", annotated=False),
            "no line": self.tag(f"jaekit v{VERSION}\n"),
            "two lines": self.tag(f"{line}\n{line}\n"),
            "uppercase digest": self.tag(line.upper().replace("WINDOWS-ACCEPTED-SHA256", "Windows-Accepted-SHA256")),
            "another digest": self.tag("Windows-Accepted-SHA256: " + "0" * 64),
        }.items():
            with self.subTest(label=label), self.assertRaisesRegex(ValueError, "acceptance link"):
                published.check_release("jgoneit/jaekit", "v" + VERSION, self.metadata(), self.download,
                                        tag_reader=reader)

    def test_tag_responses_are_read_like_the_publication_job(self):
        ref = f"https://api.github.com/repos/jgoneit/jaekit/git/ref/tags/v{VERSION}"
        tag_object = "https://api.github.com/repos/jgoneit/jaekit/git/tags/" + "7" * 40
        responses = {
            "lightweight": {ref: {"ref": f"refs/tags/v{VERSION}", "object": {"type": "commit", "sha": "c" * 40}}},
            "annotated": {ref: {"ref": f"refs/tags/v{VERSION}", "object": {"type": "tag", "sha": "7" * 40}},
                          tag_object: {"sha": "7" * 40, "message": "jaekit\n\nWindows-Accepted-SHA256: x\n"}},
            "another ref": {ref: {"ref": "refs/tags/v9.9.9", "object": {"type": "commit", "sha": "c" * 40}}},
            "no message": {ref: {"ref": f"refs/tags/v{VERSION}", "object": {"type": "tag", "sha": "7" * 40}},
                           tag_object: {"sha": "7" * 40}},
            "a list": {ref: [{"ref": f"refs/tags/v{VERSION}"}]},
        }
        for label, table in responses.items():
            def fetch(url, limit, accept="", table=table):
                return json.dumps(table[url]).encode()
            with self.subTest(label=label), unittest.mock.patch.object(published, "fetch", side_effect=fetch):
                if label == "lightweight":
                    self.assertEqual(published.read_tag("jgoneit/jaekit", "v" + VERSION), (False, ""))
                elif label == "annotated":
                    self.assertEqual(published.read_tag("jgoneit/jaekit", "v" + VERSION),
                                     (True, "jaekit\n\nWindows-Accepted-SHA256: x\n"))
                else:
                    with self.assertRaises(OSError):
                        published.read_tag("jgoneit/jaekit", "v" + VERSION)
        def oversized(url, limit, accept=""):
            raise ValueError("download exceeds the release archive limit: " + url)
        with unittest.mock.patch.object(published, "fetch", side_effect=oversized), self.assertRaises(OSError):
            published.read_tag("jgoneit/jaekit", "v" + VERSION)

    def test_truncated_responses_are_unreadable(self):
        class Short(io.BytesIO):
            headers = {"Content-Length": "100"}

            def __enter__(self):
                return self

            def __exit__(self, *args):
                self.close()

        class Broken(Short):
            def read(self, size=-1):
                raise http.client.IncompleteRead(b"partial", 50)

        for response in (Short(b"x" * 50), Broken()):
            with self.subTest(response=type(response).__name__), \
                    unittest.mock.patch.object(published, "urlopen", return_value=response), self.assertRaises(OSError):
                published.fetch("https://example.invalid/asset", 1000)
        with unittest.mock.patch.object(published, "fetch", return_value=b"{not json"), \
                contextlib.redirect_stderr(io.StringIO()) as errors:
            self.assertEqual(published.main(["--tag", "v" + VERSION]), 2)
        self.assertIn("ERROR:", errors.getvalue())

    def test_unexpected_assets_and_changed_downloads_fail(self):
        metadata = self.metadata()
        metadata["assets"].append(dict(metadata["assets"][0], name="install.ps1"))
        with self.assertRaisesRegex(ValueError, "unexpected or repeated"):
            published.check_release("jgoneit/jaekit", "v" + VERSION, metadata, self.download)
        metadata = self.metadata()
        metadata["assets"][0]["digest"] = "sha256:" + "0" * 64
        with self.assertRaisesRegex(ValueError, "digest differs"):
            published.check_release("jgoneit/jaekit", "v" + VERSION, metadata, self.download)
        metadata = self.metadata(names=release.release_names(VERSION) | {ZIP})
        with self.assertRaisesRegex(ValueError, "exactly four"):
            published.check_release("jgoneit/jaekit", "v" + VERSION, metadata, self.download)


    def test_an_unreadable_release_is_an_error_not_a_failure(self):
        def unreachable(*_args, **_kwargs):
            raise OSError("network unreachable")
        with unittest.mock.patch.object(published, "check_release", side_effect=unreachable), \
                contextlib.redirect_stderr(io.StringIO()) as errors:
            self.assertEqual(published.main(["--tag", "v" + VERSION]), 2)
        self.assertIn("ERROR: network unreachable", errors.getvalue())
        with unittest.mock.patch.object(published, "check_release", side_effect=ValueError("bad asset")), \
                contextlib.redirect_stderr(io.StringIO()) as errors:
            self.assertEqual(published.main(["--tag", "v" + VERSION]), 1)
        self.assertIn("FAIL: bad asset", errors.getvalue())

if __name__ == "__main__":
    unittest.main()
