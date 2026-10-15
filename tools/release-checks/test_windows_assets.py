"""The v0.1.3 acceptance checker accepts an optional, verified Windows archive.

Synthetic inputs only; no network, extraction or installation.
"""
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import struct
import unittest
from unittest.mock import patch
import zipfile

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("windows_assets_consumer", HERE / "check.py")
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)


def pe_amd64():
    data = bytearray(256)
    data[:2] = b"MZ"
    struct.pack_into("<I", data, 0x3C, 0x80)
    data[0x80:0x84] = b"PE\0\0"
    struct.pack_into("<H", data, 0x84, 0x8664)
    return bytes(data)


def windows_zip(files, version=None):
    manifest = {"schema": "jaekit-windows-package/v1", "version": version or check.VERSION, "os": "windows",
                "arch": "amd64", "files": [{"path": name, "size": len(body), "sha256": hashlib.sha256(body).hexdigest()}
                                           for name, body in sorted(files.items())]}
    out = io.BytesIO()
    with zipfile.ZipFile(out, "w") as archive:
        for name, body in sorted(files.items()):
            archive.writestr(name, body)
        archive.writestr("package.json", json.dumps(manifest))
    return out.getvalue()


class WindowsAssets(unittest.TestCase):
    def assets(self, names):
        collector = check.Checker()
        release = {"tag_name": check.TAG, "draft": False, "prerelease": False,
                   "assets": [{"name": name} for name in names]}
        patcher = patch.object(collector, "release", return_value=release)
        patcher.start()
        self.addCleanup(patcher.stop)
        return collector.assets()

    def test_both_published_shapes_are_accepted(self):
        base = [f"ha_{check.VERSION}_{p}.tar.gz" for p in check.PLATFORMS] + ["checksums.txt"]
        self.assertEqual(set(self.assets(base)), set(base))
        self.assertEqual(set(self.assets(base + [check.WINDOWS_ARCHIVE])), set(base) | {check.WINDOWS_ARCHIVE})
        for names in (base + ["install.ps1"], base[1:], base + [check.WINDOWS_ARCHIVE, check.WINDOWS_ARCHIVE]):
            with self.subTest(names=names), self.assertRaises(check.Failure):
                self.assets(names)

    def test_windows_archive_content_is_verified(self):
        files = {"ha.exe": pe_amd64(), "LICENSE": b"license\n"}
        self.assertIn("ha.exe", check.windows_archive(windows_zip(files)))
        for label, data in {
            "other version": windows_zip(files, version="9.9.9"),
            "not PE": windows_zip(dict(files, **{"ha.exe": b"\x7fELF" + b"\0" * 60})),
            "not ZIP": b"not a zip",
        }.items():
            with self.subTest(label=label), self.assertRaises(check.Failure):
                check.windows_archive(data)


if __name__ == "__main__":
    unittest.main()
