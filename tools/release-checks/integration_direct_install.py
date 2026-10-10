"""Actual local archive installation; synthetic native capture, no model call."""
import copy
import io
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest
from unittest import mock
from unittest import mock

import check
import install_trace

ROOT = Path(__file__).resolve().parents[2]


class DirectInstallation(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory(prefix="jaekit-install-core-")
        cls.core = Path(cls.build.name) / "ha"
        subprocess.run(["go", "build", "-ldflags=-X main.version=0.1.3", "-o", str(cls.core), "./cmd/ha"], cwd=ROOT, check=True,
                       stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=120)

    @classmethod
    def tearDownClass(cls):
        cls.build.cleanup()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="jaekit-direct-install-")
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.target = check.native_platform()
        self.archive = self.root / "release.tar.gz"
        self.destination = self.root / "new-install"
        self.report = self.root / "report.json"
        self.files = {"ha": self.core.read_bytes(), "LICENSE": b"synthetic license\n",
                      "README.md": b"synthetic support\n", "README.en.md": b"support\n",
                      "guides/INSTALL.md": b"synthetic install\n", "guides/INSTALL.en.md": b"install\n",
                      "contracts/run-record.md": b"synthetic contract\n",
                      "tools/check-result-reference.py": b"# synthetic support\n",
                      "examples/check-result/reference.json": b"{}\n"}
        self.write_archive(self.archive, self.files)
        self.checker = check.Checker(self.root)

    def write_archive(self, path, files):
        with tarfile.open(path, "w:gz") as archive:
            for name, data in files.items():
                member = tarfile.TarInfo(f"ha_{check.VERSION}_{self.target}/" + name)
                member.size = len(data)
                archive.addfile(member, io.BytesIO(data))

    def artifact(self, path):
        return {"path": str(path), "sha256": install_trace.digest(path.read_bytes())}

    def run_install(self, collector=None):
        command = [sys.executable, str(collector or Path(install_trace.__file__).resolve()), "run",
                   "--archive", str(self.archive), "--target", self.target,
                   "--destination", str(self.destination), "--output", str(self.report),
                   "--invocation", "synthetic-original-install"]
        result = subprocess.run(command, stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=60)
        out, err = self.root / "stdout", self.root / "stderr"
        out.write_bytes(result.stdout); err.write_bytes(result.stderr)
        capture = {"argv": command, "exit_code": result.returncode,
                   "stdout": self.artifact(out), "stderr": self.artifact(err)}
        if self.report.exists():
            capture["report"] = self.artifact(self.report)
        value = {"install": capture, "package_root": str(self.destination)}
        if (self.destination / "ha").is_file():
            value["binary"] = self.artifact(self.destination / "ha")
        return result, value

    def rewrite_report(self, value, transform, bind_receipt=True):
        data = json.loads(self.report.read_text())
        transform(data)
        self.report.write_text(json.dumps(data, sort_keys=True) + "\n")
        value["install"]["report"] = self.artifact(self.report)
        if bind_receipt:
            receipt = {"schema": "jaekit-install-receipt/v1", "invocation": data["invocation"],
                       "run_id": data["run_id"], "sha256": install_trace.digest(self.report.read_bytes())}
            error = self.root / "stderr"
            error.write_text(install_trace.RECEIPT + json.dumps(receipt) + "\n")
            value["install"]["stderr"] = self.artifact(error)

    def consume(self, value):
        return install_trace.consume(self.checker, value, self.archive.read_bytes(), self.target)

    def test_actual_fresh_install_capture_and_core(self):
        result, value = self.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        selected = self.consume(value)
        self.assertEqual(selected, (self.destination / "ha").resolve())
        self.assertEqual(subprocess.check_output([str(selected), "--version"]).strip(), b"ha 0.1.3")
        report = json.loads(self.report.read_text())
        self.assertTrue(report["before_absent"] and report["completed"])
        self.assertEqual({p.relative_to(self.destination).as_posix() for p in self.destination.rglob("*") if p.is_file()},
                         set(self.files))
        for key, suffix in (("version", ["--version"]), ("capabilities", ["capabilities", "--format", "json"])):
            result = subprocess.run([str(selected), *suffix], capture_output=True, timeout=30)
            output, error = self.root / (key + '.out'), self.root / (key + '.err')
            output.write_bytes(result.stdout); error.write_bytes(result.stderr)
            value[key] = {"argv": [str(selected), *suffix], "exit_code": result.returncode,
                          "stdout": self.artifact(output), "stderr": self.artifact(error)}
        self.checker._evidence = {"installations": {"direct": value}}
        before = {str(path): path.read_bytes() for path in self.root.rglob('*') if path.is_file()}
        with mock.patch.object(self.checker, 'download', return_value=self.archive.read_bytes()):
            self.assertEqual(self.checker.installation('direct'), selected)
            altered = copy.deepcopy(value)
            altered['install']['argv'] = ['/usr/bin/true']
            self.checker._evidence['installations']['direct'] = altered
            with self.assertRaises(check.Failure):
                self.checker.installation('direct')
        self.assertEqual({str(path): path.read_bytes() for path in self.root.rglob('*') if path.is_file()}, before)

    def test_existing_install_and_existing_output_are_preserved(self):
        self.destination.mkdir()
        old = self.destination / "ha"
        old.write_bytes(b"original installation")
        result, value = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(old.read_bytes(), b"original installation")
        self.assertFalse(json.loads(self.report.read_text())["before_absent"])
        with self.assertRaises(check.Failure):
            self.consume(value)
        original_report = self.report.read_bytes()
        result, _ = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.report.read_bytes(), original_report)

    def test_success_and_version_query_do_not_substitute_for_install(self):
        _, value = self.run_install()
        for command in (["/usr/bin/true"], [str(self.destination / "ha"), "--version"]):
            altered = copy.deepcopy(value)
            altered["install"]["argv"] = command
            with self.assertRaises(install_trace.InvalidInstallation):
                self.consume(altered)
        altered = copy.deepcopy(value)
        del altered["install"]["report"]
        with self.assertRaises(install_trace.UnavailableInstallation):
            self.consume(altered)

    def test_different_archive_result_and_original_receipt_are_rejected(self):
        _, value = self.run_install()
        altered_archive = self.root / "other.tar.gz"
        self.write_archive(altered_archive, dict(self.files, **{"README.md": b"other input"}))
        with self.assertRaises(install_trace.InvalidInstallation):
            install_trace.consume(self.checker, value, altered_archive.read_bytes(), self.target)
        (self.destination / "LICENSE").write_bytes(b"modified support")
        with self.assertRaises(install_trace.InvalidInstallation):
            self.consume(value)
        (self.destination / "LICENSE").write_bytes(self.files["LICENSE"])
        self.rewrite_report(value, lambda report: report.update(run_id="a" * 32), bind_receipt=False)
        with self.assertRaises(install_trace.InvalidInstallation):
            self.consume(value)

    def test_report_boundaries_cannot_be_replaced_by_assertions(self):
        _, original = self.run_install()
        raw = self.report.read_bytes()
        mutations = (lambda r: r.update(before_absent=False), lambda r: r.update(completed=False),
                     lambda r: r.update(identity_unchanged=False), lambda r: r.update(installed=[]),
                     lambda r: r.update(destination=str(self.root)),
                     lambda r: r.update(validator=r["producer"]),
                     lambda r: r.update(core={"path": str(self.core), "sha256": install_trace.digest(self.core.read_bytes())}))
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                self.report.write_bytes(raw)
                value = copy.deepcopy(original)
                self.rewrite_report(value, mutate)
                with self.assertRaises(install_trace.InvalidInstallation):
                    self.consume(value)

    def test_unknown_contract_and_missing_observations_are_unavailable(self):
        _, original = self.run_install()
        raw = self.report.read_bytes()
        for mutate in (lambda r: r.update(schema="unknown/v1"),
                       lambda r: r.update(procedure="arbitrary-command/v1"),
                       lambda r: r.pop("before_absent"),
                       lambda r: r.update(completed="true")):
            with self.subTest(mutation=mutate):
                self.report.write_bytes(raw)
                value = copy.deepcopy(original)
                self.rewrite_report(value, mutate)
                with self.assertRaises(install_trace.UnavailableInstallation):
                    self.consume(value)
        self.report.write_bytes(raw)
        value = copy.deepcopy(original)
        error = self.root / "stderr"
        error.write_bytes(b"")
        value["install"]["stderr"] = self.artifact(error)
        with self.assertRaises(install_trace.UnavailableInstallation):
            self.consume(value)

    def test_other_validator_source_is_not_the_verified_procedure(self):
        alternate = self.root / "alternate-procedure"
        alternate.mkdir()
        for path in Path(install_trace.__file__).parent.glob("*.py"):
            shutil.copyfile(path, alternate / path.name)
        validator = alternate / "check.py"
        validator.write_bytes(validator.read_bytes() + b"\n# different validator source bytes\n")
        result, value = self.run_install(alternate / "install_trace.py")
        self.assertEqual(result.returncode, 0, result.stderr)
        with self.assertRaisesRegex(install_trace.InvalidInstallation, "validator"):
            self.consume(value)

    def test_archive_read_is_bounded_before_parser_and_on_growth(self):
        path = self.root / "bounded-input"
        path.write_bytes(b"x" * 17)
        with mock.patch.object(Path, "open", side_effect=AssertionError("must not read oversized input")):
            with self.assertRaisesRegex(OSError, "compressed byte limit"):
                install_trace.regular(path, 16)
        path.write_bytes(b"small")
        # Simulate growth between stat and read. The read itself is bounded,
        # including the second identity check at collector completion.
        stream = mock.MagicMock()
        stream.__enter__.return_value = stream
        stream.read.return_value = b"x" * 17
        with mock.patch.object(Path, "open", return_value=stream):
            with self.assertRaisesRegex(OSError, "compressed byte limit"):
                install_trace.identity(path, 16)
        stream.read.assert_called_once_with(17)

    def test_symlink_input_destination_and_partial_install_are_not_success(self):
        original = self.root / "original.tar.gz"
        self.archive.rename(original); self.archive.symlink_to(original)
        result, _ = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse(self.destination.exists())
        self.assertFalse(json.loads(self.report.read_text())["completed"])
        self.report.unlink(); self.archive.unlink(); original.rename(self.archive)
        original_dir = self.root / "original-dir"; original_dir.mkdir()
        (original_dir / "keep").write_text("preserve")
        self.destination.symlink_to(original_dir, target_is_directory=True)
        result, _ = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual((original_dir / "keep").read_text(), "preserve")
        self.report.unlink(); self.destination.unlink()
        # Both safe-looking members cannot be materialized as files. Keep the
        # partial new tree and its failed report; never turn it into success.
        self.write_archive(self.archive, dict(self.files, **{"collision": b"file", "collision/child": b"child"}))
        result, _ = self.run_install()
        self.assertNotEqual(result.returncode, 0)
        report = json.loads(self.report.read_text())
        self.assertTrue(report["before_absent"])
        self.assertFalse(report["completed"])
        self.assertTrue((self.destination / "collision").is_file())


if __name__ == "__main__":
    unittest.main()
