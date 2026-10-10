"""Synthetic release regressions; no network, installation, or publication."""
import importlib.util
import io
import json
from pathlib import Path
import struct
import tarfile
import tempfile
import unittest
from unittest import mock

spec = importlib.util.spec_from_file_location("release_archives", Path(__file__).with_name("check_archives.py"))
release = importlib.util.module_from_spec(spec)
spec.loader.exec_module(release)
ROOT = Path(__file__).resolve().parents[2]
VERSION = "0.1.3"


def executable_header(target):
    header = bytearray(64)
    system, arch = target.split("_")
    if system == "linux":
        header[:6] = b"\x7fELF\x02\x01"
        struct.pack_into("<H", header, 18, {"amd64": 62, "arm64": 183}[arch])
    else:
        header[:4] = b"\xcf\xfa\xed\xfe"
        struct.pack_into("<I", header, 4, {"amd64": 0x1000007, "arm64": 0x100000C}[arch])
    return bytes(header)


class ArchiveChecks(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="jaekit-package-test-")
        self.addCleanup(self.temporary.cleanup)
        self.dist = Path(self.temporary.name) / "dist"
        self.dist.mkdir()
        self.contents = {name: b"Synthetic public document.\n" for name in release.REQUIRED}
        self.contents["tools/check-result-reference.py"] = (ROOT / "tools/check-result-reference.py").read_bytes()
        self.contents["examples/check-result/declaration.json"] = json.dumps({
            "schema": "check-declaration/v1", "producer": "jaekit-reference", "producer_version": "1",
            "producer_paths": ["tools/check-result-reference.py", "checks/reference.json"],
            "targets": [{"id": "greeting", "violation": "wrong-greeting"}],
        }).encode()
        self.contents["examples/check-result/reference.json"] = json.dumps({"checks": [{
            "target": "greeting", "path": "sample/input.txt", "equals": "hello\n",
            "violation": "wrong-greeting", "missing": "violation",
        }]}).encode()
        self.contents["examples/check-result/input.txt"] = b"before\n"
        for target in release.TARGETS:
            self.archive(target)
        self.checksums()

    def archive(self, target, omit=None, extra=None, binary_target=None):
        prefix = f"ha_{VERSION}_{target}"
        archive_path = self.dist / (prefix + ".tar.gz")
        with tarfile.open(archive_path, "w:gz") as archive:
            for name, value in self.contents.items():
                if omit and (name == omit or name.startswith(omit.rstrip("/") + "/")):
                    continue
                if name == "ha":
                    value = executable_header(binary_target or target)
                info = tarfile.TarInfo(prefix + "/" + name)
                info.size, info.mode = len(value), 0o755 if name == "ha" else 0o644
                archive.addfile(info, io.BytesIO(value))
            if extra:
                archive.addfile(extra)
        return archive_path

    def checksums(self):
        lines = [release.digest(path) + "  " + path.name for path in sorted(self.dist.glob("*.tar.gz"))]
        (self.dist / "checksums.txt").write_text("\n".join(lines) + "\n")

    def test_four_platform_archives_and_uploaded_names(self):
        self.assertEqual(set(release.check(self.dist, VERSION)), release.release_names(VERSION))
        uploaded = self.dist.parent / "uploaded.txt"
        uploaded.write_text("\n".join(sorted(release.release_names(VERSION) | {"checksums.txt"})) + "\n")
        release.check(self.dist, VERSION, uploaded)

    def test_missing_archive_or_extra_upload_input_is_refused(self):
        (self.dist / f"ha_{VERSION}_linux_arm64.tar.gz").unlink()
        with self.assertRaisesRegex(ValueError, "exactly four"):
            release.check(self.dist, VERSION)
        self.archive("linux_arm64")
        (self.dist / "extra.txt").write_text("not a release asset")
        with self.assertRaisesRegex(ValueError, "exactly four"):
            release.check(self.dist, VERSION)

    def test_corrupt_download_is_refused_before_smoke(self):
        path = self.dist / f"ha_{VERSION}_linux_amd64.tar.gz"
        with path.open("ab") as output:
            output.write(b"changed")
        with self.assertRaisesRegex(ValueError, "checksum mismatch"):
            release.check(self.dist, VERSION)

    def test_checksum_list_cannot_drop_or_duplicate_an_asset(self):
        path = self.dist / "checksums.txt"
        original = path.read_text()
        path.write_text("\n".join(original.splitlines()[1:]) + "\n")
        with self.assertRaisesRegex(ValueError, "exactly the four"):
            release.check(self.dist, VERSION)
        path.write_text(original + original.splitlines()[0] + "\n")
        with self.assertRaisesRegex(ValueError, "repeats"):
            release.check(self.dist, VERSION)

    def test_producer_and_example_are_required_in_every_archive(self):
        for missing in ("tools/check-result-reference.py", "examples/check-result/reference.json", "LICENSE"):
            with self.subTest(missing=missing):
                self.archive("darwin_arm64", omit=missing)
                self.checksums()
                with self.assertRaisesRegex(ValueError, "missing required"):
                    release.check(self.dist, VERSION)

    def test_installation_and_linked_public_materials_are_required(self):
        # Keep the expected public paths independent of REQUIRED: dropping a
        # validator requirement must not silently weaken this regression too.
        for missing in (
            "assets", "assets/readme/flow.ko.svg", "assets/readme/flow.en.svg",
            "assets/readme/example.ko.svg", "assets/readme/example.en.svg",
            "assets/readme/SOURCES.md", "assets/readme/SOURCES.en.md",
            "assets/readme/generate-flow.py", "assets/readme/generate-example.py",
            "guides/USAGE.md", "guides/USAGE.en.md", "guides/OPERATIONS.md",
            "contracts/README.md", "contracts/bundle.md", "contracts/goal-docs.md",
            "contracts/run-record.md", "contracts/role-card.md",
        ):
            with self.subTest(missing=missing):
                self.archive("darwin_arm64", omit=missing)
                self.checksums()
                with self.assertRaisesRegex(ValueError, "missing required") as failure:
                    release.check(self.dist, VERSION)
                self.assertIn(missing, str(failure.exception))

    def test_unsafe_private_or_link_members_are_refused(self):
        prefix = f"ha_{VERSION}_linux_amd64"
        for name, member_type in ((prefix + "/../escape", tarfile.REGTYPE),
                                  (prefix + "/docs/private.txt", tarfile.REGTYPE),
                                  (prefix + "/examples/check-result/runs.jsonl", tarfile.REGTYPE),
                                  (prefix + "/guides/link", tarfile.SYMTYPE),
                                  (prefix + "/guides/hardlink", tarfile.LNKTYPE)):
            with self.subTest(name=name):
                extra = tarfile.TarInfo(name)
                extra.type, extra.linkname = member_type, "outside"
                self.archive("linux_amd64", extra=extra)
                self.checksums()
                with self.assertRaises(ValueError):
                    release.check(self.dist, VERSION)

    def test_executable_platform_must_match_the_archive_name(self):
        self.archive("darwin_arm64", binary_target="linux_arm64")
        self.checksums()
        with self.assertRaisesRegex(ValueError, "platform differs"):
            release.check(self.dist, VERSION)

    def test_missing_or_duplicate_public_upload_is_not_success(self):
        uploaded = self.dist.parent / "uploaded.txt"
        names = sorted(release.release_names(VERSION) | {"checksums.txt"})
        for items in (names[:-1], names + names[:1]):
            uploaded.write_text("\n".join(items) + "\n")
            with self.assertRaisesRegex(ValueError, "published release asset"):
                release.check(self.dist, VERSION, uploaded)

    def test_packaged_reference_really_observes_violation_and_pass(self):
        original = release.invoke
        observed = []

        def invoke(argv, cwd, env=None, expected=0):
            if Path(argv[0]).name == "ha":
                if argv[1:] == ["--version"]:
                    return "ha " + VERSION + "\n"
                return json.dumps({
                    "schema": "ha-capabilities/v1", "ha_version": VERSION,
                    "default_rules": "run-rules/3", "supported_rules": ["run-rules/1", "run-rules/2", "run-rules/3"],
                    "features": ["estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1", "budget-change/v1"],
                    "formats": {"criteria": ["legacy", "nested/1"], "check_declaration": ["check-declaration/v1"],
                                "check_result": ["check-result/v1"], "check_evidence": ["check-evidence/v1"]},
                })
            observed.append(expected)
            return original(argv, cwd, env, expected)

        with mock.patch.object(release, "invoke", side_effect=invoke):
            release.native_smoke(self.dist, VERSION)
        self.assertEqual(observed, [1, 0])


if __name__ == "__main__":
    unittest.main()
