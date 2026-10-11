"""Actual direct installs through parent aliases and read-only receipt checks.

The Core, archive, files and collector receipts are real local artifacts. Alias
retargeting is controlled at an observation boundary, not an OS race test.
"""
import argparse
import contextlib
import copy
import io
import json
from pathlib import Path
import shutil
import subprocess
import sys
import unittest
from unittest import mock

import integration_direct_install as fixtures

check = fixtures.check
install_trace = fixtures.install_trace


class DestinationFixture(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        fixtures.DirectInstallation.setUpClass()

    @classmethod
    def tearDownClass(cls):
        fixtures.DirectInstallation.tearDownClass()

    def setUp(self):
        self.case = fixtures.DirectInstallation()
        self.case.setUp()
        self.addCleanup(self.case.doCleanups)

    def parent_alias(self):
        case = self.case
        real = case.root / "actual-parent"
        real.mkdir()
        alias = case.root / "user-parent"
        alias.symlink_to(real, target_is_directory=True)
        case.destination = alias / "new-install"
        return alias, (real / "new-install").resolve()

    def capture_in_process(self):
        case = self.case
        argv = [sys.executable, str(Path(install_trace.__file__).resolve()), "run",
                "--archive", str(case.archive), "--target", case.target,
                "--destination", str(case.destination), "--output", str(case.report),
                "--invocation", "synthetic-original-install"]
        args = argparse.Namespace(archive=str(case.archive), target=case.target,
                                  destination=str(case.destination), output=str(case.report),
                                  invocation="synthetic-original-install")
        stderr = io.StringIO()
        with contextlib.redirect_stderr(stderr):
            result = install_trace.collect(args, launcher_argv=argv)
        out, err = case.root / "stdout", case.root / "stderr"
        out.write_bytes(b"")
        err.write_text(stderr.getvalue())
        value = {"install": {"argv": argv, "exit_code": result,
                             "stdout": case.artifact(out), "stderr": case.artifact(err),
                             "report": case.artifact(case.report)},
                 "package_root": str(case.destination)}
        return result, value

    def assert_rejected(self, value, tag):
        try:
            self.case.consume(value)
        except (install_trace.InvalidInstallation, install_trace.UnavailableInstallation,
                check.Failure, check.Unavailable):
            return
        self.fail(f"[violation:{tag}] contradictory installation evidence was accepted")


class InstallationPathContract(DestinationFixture):
    def test_parent_alias_chain_keeps_requested_and_canonical_paths(self):
        case = self.case
        alias, canonical = self.parent_alias()
        chain = case.root / "user-chain"
        chain.symlink_to(alias, target_is_directory=True)
        case.destination = chain / "new-install"
        requested = str(case.destination)
        result, value = case.run_install()
        if result.returncode:
            self.fail("[violation:installation-path-contract] a fresh parent-alias installation was refused")
        report = json.loads(case.report.read_text())
        options = dict(zip(report["launcher_argv"][3::2], report["launcher_argv"][4::2]))
        if options["--destination"] != requested or report["destination"] != str(canonical):
            self.fail("[violation:installation-path-contract] requested and actual installation paths were not recorded")
        self.assertEqual((canonical / "ha").read_bytes(), case.files["ha"])
        self.assertEqual(case.consume(value), canonical / "ha")

    def test_leaf_links_and_existing_destinations_are_preserved(self):
        case = self.case
        alias, canonical = self.parent_alias()
        original = canonical.parent / "original"
        original.mkdir()
        (original / "keep").write_bytes(b"preserved original")
        for kind in ("directory", "file", "symlink", "dangling"):
            with self.subTest(kind=kind):
                case.destination = alias / kind
                case.report = case.root / (kind + ".json")
                if kind == "directory":
                    case.destination.mkdir()
                    (case.destination / "keep").write_bytes(b"preserved directory")
                elif kind == "file":
                    case.destination.write_bytes(b"preserved file")
                else:
                    target = original if kind == "symlink" else canonical.parent / "absent"
                    case.destination.symlink_to(target, target_is_directory=True)
                result, _ = case.run_install()
                report = json.loads(case.report.read_text())
                if result.returncode == 0 or report["before_absent"] or report["completed"]:
                    self.fail("[violation:installation-path-contract] an existing destination or leaf link was accepted")
                self.assertEqual((original / "keep").read_bytes(), b"preserved original")
                if kind == "directory":
                    self.assertEqual(list(case.destination.iterdir()), [case.destination / "keep"])
                elif kind == "file":
                    self.assertEqual(case.destination.read_bytes(), b"preserved file")
                else:
                    self.assertTrue(case.destination.is_symlink())
                    self.assertFalse((canonical.parent / "absent").exists())

    def test_another_parent_alias_does_not_make_an_existing_install_fresh(self):
        case = self.case
        _, canonical = self.parent_alias()
        result, _ = case.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        original = {p.relative_to(canonical).as_posix(): p.read_bytes()
                    for p in canonical.rglob("*") if p.is_file()}
        second = case.root / "second-alias"
        second.symlink_to(canonical.parent, target_is_directory=True)
        case.destination = second / canonical.name
        case.report = case.root / "second-report.json"
        result, _ = case.run_install()
        if result.returncode == 0 or json.loads(case.report.read_text())["before_absent"]:
            self.fail("[violation:installation-path-contract] another alias reused an existing installation as fresh")
        self.assertEqual({p.relative_to(canonical).as_posix(): p.read_bytes()
                          for p in canonical.rglob("*") if p.is_file()}, original)

    def test_collector_retains_failure_when_requested_parent_changes(self):
        case = self.case
        alias, canonical = self.parent_alias()
        other = case.root / "other-parent"
        other.mkdir()
        original_install = install_trace.install_files

        def install_then_retarget(*args, **kwargs):
            installed = original_install(*args, **kwargs)
            alias.unlink()
            alias.symlink_to(other, target_is_directory=True)
            return installed

        with mock.patch.object(install_trace, "install_files", side_effect=install_then_retarget):
            result, value = self.capture_in_process()
        report = json.loads(case.report.read_text())
        if result == 0 or report["completed"]:
            self.fail("[violation:installation-path-contract] collector completed after the requested parent selected another destination")
        self.assertTrue(report["before_absent"])
        self.assertIsNotNone(report["failure"])
        self.assertEqual(report["destination"], str(canonical))
        self.assertEqual((canonical / "ha").read_bytes(), case.files["ha"])
        self.assertFalse((other / "new-install").exists())
        self.assert_rejected(value, "installation-path-contract")


class InstallationPathBinding(DestinationFixture):
    def test_parent_alias_original_receipt_reaches_read_only_installation_acceptance(self):
        case = self.case
        _, canonical = self.parent_alias()
        result, value = case.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        selected = canonical / "ha"
        for key, suffix in (("version", ["--version"]), ("capabilities", ["capabilities", "--format", "json"])):
            query = subprocess.run([str(selected), *suffix], capture_output=True, timeout=30)
            self.assertEqual(query.returncode, 0, query.stderr)
            out, err = case.root / (key + ".out"), case.root / (key + ".err")
            out.write_bytes(query.stdout)
            err.write_bytes(query.stderr)
            value[key] = {"argv": [str(selected), *suffix], "exit_code": query.returncode,
                          "stdout": case.artifact(out), "stderr": case.artifact(err)}
        case.checker._evidence = {"installations": {"direct": value}}
        before = {str(p): p.read_bytes() for p in case.root.rglob("*") if p.is_file()}
        with mock.patch.object(case.checker, "download", return_value=case.archive.read_bytes()):
            try:
                accepted = case.checker.installation("direct")
            except (check.Failure, check.Unavailable) as error:
                self.fail(f"[violation:installation-path-binding] valid parent-alias receipt was refused: {error}")
        self.assertEqual(accepted, selected)
        self.assertEqual({str(p): p.read_bytes() for p in case.root.rglob("*") if p.is_file()}, before)

    def test_matching_archive_bytes_cannot_replace_the_requested_destination(self):
        case = self.case
        _, canonical = self.parent_alias()
        result, value = case.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        other = case.root / "other-install"
        shutil.copytree(canonical, other)
        altered = copy.deepcopy(value)
        altered["package_root"] = str(other)
        altered["binary"] = case.artifact(other / "ha")
        case.rewrite_report(altered, lambda report: report.update(
            destination=str(other), core=case.artifact(other / "ha")))
        self.assert_rejected(altered, "installation-path-binding")
        self.assertEqual((canonical / "ha").read_bytes(), case.files["ha"])

    def test_consumer_rechecks_parent_after_reading_installed_core(self):
        case = self.case
        alias, canonical = self.parent_alias()
        result, value = case.run_install()
        self.assertEqual(result.returncode, 0, result.stderr)
        other = case.root / "other-parent"
        other.mkdir()
        shutil.copytree(canonical, other / "new-install")
        report_before = case.report.read_bytes()
        archive_before = case.archive.read_bytes()
        original_artifact = case.checker.artifact

        def read_then_retarget(artifact):
            result = original_artifact(artifact)
            if artifact is value["binary"]:
                alias.unlink()
                alias.symlink_to(other, target_is_directory=True)
            return result

        with mock.patch.object(case.checker, "artifact", side_effect=read_then_retarget):
            self.assert_rejected(value, "installation-path-binding")
        self.assertEqual(case.report.read_bytes(), report_before)
        self.assertEqual(case.archive.read_bytes(), archive_before)
        self.assertEqual((canonical / "ha").read_bytes(), case.files["ha"])
        self.assertEqual((other / "new-install/ha").read_bytes(), case.files["ha"])


if __name__ == "__main__":
    unittest.main()
