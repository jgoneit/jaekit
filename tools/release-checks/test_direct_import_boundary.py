"""Code that imports check.py directly keeps the standard-library boundary.

A caller's import path holds modules named like standard modules that the
asset and archive judgments could import late. They must not run, and the
judgment must be the same as without them. Synthetic archives only; no
network, extraction or installation.
"""
import hashlib
import io
import json
from pathlib import Path
import subprocess
import sys
import tarfile
import tempfile
import unittest

HERE = Path(__file__).resolve().parent
SHADOWS = ("zipfile", "tarfile", "gzip", "bz2", "lzma", "csv")
REQUIRED = ("ha", "LICENSE", "README.md", "README.en.md", "guides/INSTALL.md",
            "guides/INSTALL.en.md", "contracts/run-record.md")
CALLER = r"""
import importlib.util, json, sys
path, shadow, directory = sys.argv[1:4]
shadows = %r
result = {"preloaded": sorted(name for name in shadows if name in sys.modules)}
if shadow:
    sys.path.insert(0, shadow)
spec = importlib.util.spec_from_file_location("directly_imported_check", path)
check = importlib.util.module_from_spec(spec)
spec.loader.exec_module(check)
names = [f"ha_{check.VERSION}_{p}.tar.gz" for p in check.PLATFORMS] + ["checksums.txt"]
data = {name: open(directory + "/" + name, "rb").read() for name in names}
checker = check.Checker()
checker._release = {"tag_name": check.TAG, "draft": False, "prerelease": False,
                    "assets": [{"name": name} for name in names]}
checker.download = lambda name: data[name]
checker.assets()
result["targets"] = sorted(checker.archives())
print(json.dumps(result))
""" % (SHADOWS,)


def package(target):
    """The minimal archive the bounded archive tests accept (test_archive_limits.py)."""
    out = io.BytesIO()
    with tarfile.open(fileobj=out, mode="w:gz", format=tarfile.PAX_FORMAT) as archive:
        for name in REQUIRED:
            member = tarfile.TarInfo(f"ha_0.1.3_{target}/{name}")
            member.size = 1
            archive.addfile(member, io.BytesIO(b"x"))
    return out.getvalue()


class DirectImportBoundary(unittest.TestCase):
    def judge(self, shadow, directory):
        result = subprocess.run([sys.executable, "-c", CALLER, str(HERE / "check.py"), shadow, directory],
                                capture_output=True, text=True, timeout=120, cwd=shadow or HERE)
        self.assertEqual(result.returncode, 0, result.stderr[-2000:])
        return json.loads(result.stdout.strip().splitlines()[-1])

    def test_caller_modules_do_not_run_during_asset_and_archive_judgment(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-direct-import-") as folder:
            root = Path(folder)
            release, shadow, marker = root / "release", root / "caller", root / "executed"
            release.mkdir()
            shadow.mkdir()
            lines = []
            for target in ("darwin_amd64", "darwin_arm64", "linux_amd64", "linux_arm64"):
                data = package(target)
                (release / f"ha_0.1.3_{target}.tar.gz").write_bytes(data)
                lines.append(f"{hashlib.sha256(data).hexdigest()}  ha_0.1.3_{target}.tar.gz\n")
            (release / "checksums.txt").write_text("".join(lines))
            for name in SHADOWS:
                (shadow / f"{name}.py").write_text(
                    f"open({str(marker)!r}, 'a').write({name!r} + '\\n')\nraise ImportError('shadow {name}')\n")
            shadowed = self.judge(str(shadow), str(release))
            control = self.judge("", str(release))
            self.assertEqual([], shadowed["preloaded"], "the interpreter preloaded a shadowed name; nothing is observed")
            self.assertFalse(marker.exists(), marker.read_text() if marker.exists() else "")
            self.assertEqual(shadowed["targets"], control["targets"])
            self.assertEqual(4, len(control["targets"]))


if __name__ == "__main__":
    unittest.main()
