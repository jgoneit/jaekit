"""Regressions for native evidence omissions and guide isolation, not OS claims."""
import contextlib
import io
import json
import os
from pathlib import Path
import tarfile
import tempfile
import unittest
from unittest import mock
from urllib.error import HTTPError
from urllib.request import urlopen

import native_unix as native


def complete_events():
    events = []
    for package, names in native.REQUIRED.items():
        package = native.MODULE + package
        events.append({"Action": "start", "Package": package})
        for name in names:
            events += [{"Action": action, "Package": package, "Test": name} for action in ("run", "pass")]
        events.append({"Action": "pass", "Package": package})
    return events


def results(output):
    return native.native_results(output,
        {native.MODULE + package + "/" + name for package, names in native.REQUIRED.items() for name in names},
        {native.MODULE + package for package in native.PACKAGES}, native.HELPERS)


def encode(events):
    return "\n".join(json.dumps(event) for event in events)


class NativeEvidence(unittest.TestCase):
    def test_real_test_events_required_not_only_package_or_helper_success(self):
        events = complete_events()
        self.assertEqual(len(results(encode(events))["required"]), 10)
        for kind in ("missing", "skip", "fail", "no-run", "unfinished", "missing-package", "malformed"):
            with self.subTest(kind=kind):
                altered = [dict(event) for event in events]
                if kind == "missing":
                    altered = [e for e in altered if e.get("Test") != "TestNativeCoreGitLifecycle/linked=true"]
                    altered += [{"Action": a, "Package": native.MODULE + "internal/run", "Test": "TestNativeProcessHelper"}
                                for a in ("run", "pass")]
                elif kind in ("skip", "fail"):
                    altered[2]["Action"] = kind
                elif kind == "no-run":
                    altered.pop(1)
                elif kind == "unfinished":
                    altered.pop(2)
                elif kind == "missing-package":
                    altered.pop()
                text = encode(altered) + ("\nnot-json" if kind == "malformed" else "")
                with self.assertRaises(ValueError):
                    results(text)

    def test_destination_binding_quotes_spaces_and_keeps_home_unchanged(self):
        if os.name == "nt":
            # This test only checks POSIX guide quoting; Windows runs its own
            # selected suite, never imports this Unix suite as native evidence.
            return
        with tempfile.TemporaryDirectory(prefix="jaekit-ci-bind-") as temporary:
            target = Path(temporary) / "설치 root"
            target.mkdir()
            script = native.bind_destination('printf "%s\\n" "$HOME/file" ~/other')
            result = native.invoke(["bash", "-c", script], root=target,
                                   env=dict(os.environ, JAEKIT_CI_INSTALL_ROOT=str(target)))
            self.assertEqual(result.stdout.splitlines(), [str(target / "file"), str(target / "other")])
            self.assertNotIn("$HOME", script)

    def test_public_candidate_archive_and_transport_are_bounded_to_named_bytes(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-ci-archive-") as temporary:
            binary = Path(temporary) / "ha"
            binary.write_bytes(b"synthetic archive unit-test input")
            name, data, checksum = native.make_archive(binary, {"guides/INSTALL.md": b"public"}, "0.0.0", "linux_amd64")
            self.assertEqual(checksum.decode(), native.digest(data) + "  " + name + "\n")
            with tarfile.open(fileobj=io.BytesIO(data), mode="r:gz") as archive:
                members = archive.getmembers()
                self.assertEqual({m.name.split("/", 1)[1] for m in members}, {"ha", "guides/INSTALL.md"})
                self.assertTrue(next(m for m in members if m.name.endswith("/ha")).mode & 0o111)
            with native.local_transport({"/candidate/" + name: data}) as url:
                with urlopen(url + "/candidate/" + name, timeout=5) as response:
                    self.assertEqual(response.read(), data)
                for unknown in ("/", "/../private", "/candidate/checksums.txt"):
                    with self.subTest(path=unknown), self.assertRaises(HTTPError) as caught:
                        urlopen(url + unknown, timeout=5)
                    self.assertEqual(caught.exception.code, 404)
                    caught.exception.close()

    def test_existing_report_is_preserved_and_never_runs_observer(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-ci-report-") as temporary:
            root = Path(temporary)
            (root / "source").mkdir()
            report = root / "report.json"
            report.write_text("preserve existing report\n")
            with mock.patch.object(native, "ROOT", root / "source"), mock.patch.object(native, "observe") as observe, \
                    mock.patch("sys.argv", ["native_unix.py", "--report", str(report)]), \
                    contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(native.main(), 1)
                observe.assert_not_called()
            self.assertEqual(report.read_text(), "preserve existing report\n")

    def test_report_in_checkout_cannot_mutate_source(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-ci-report-") as temporary:
            root = Path(temporary)
            with mock.patch.object(native, "ROOT", root), mock.patch.object(native, "observe") as observe, \
                    mock.patch("sys.argv", ["native_unix.py", "--report", str(root / "report.json")]), \
                    contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(native.main(), 1)
                observe.assert_not_called()
            self.assertFalse((root / "report.json").exists())

    def test_incomplete_observation_is_failed_in_file_and_stdout(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-ci-report-") as temporary:
            root = Path(temporary)
            (root / "source").mkdir()
            output = io.StringIO()
            report = root / "report.json"
            with mock.patch.object(native, "ROOT", root / "source"), \
                    mock.patch.object(native, "observe", side_effect=ValueError("synthetic missing native test")), \
                    mock.patch("sys.argv", ["native_unix.py", "--report", str(report)]), \
                    contextlib.redirect_stdout(output), contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(native.main(), 1)
            saved = json.loads(report.read_text())
            self.assertEqual(saved["status"], "failed")
            printed = json.loads(output.getvalue().removeprefix(native.PREFIX))
            self.assertEqual(printed, saved)


if __name__ == "__main__":
    unittest.main()
