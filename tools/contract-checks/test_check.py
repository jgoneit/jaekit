"""Public-contract checks and adversarial edits of synthetic documentation."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("document_contracts", Path(__file__).with_name("check.py"))
checker = importlib.util.module_from_spec(spec)
spec.loader.exec_module(checker)
SUITE = sys.argv.pop(1) if len(sys.argv) > 1 and sys.argv[1] in checker.REQUIREMENTS else "reporting"


class PublicContract(unittest.TestCase):
    def test_all_registered_requirements(self):
        for criterion in checker.REQUIREMENTS[SUITE]:
            with self.subTest(criterion=criterion):
                observation, problems = checker.observe(checker.ROOT, SUITE, criterion)
                self.assertEqual(observation["status"], "pass", "\n".join(problems))

    def fixture(self, directory):
        root = Path(directory)
        paths = {name for clauses in checker.REQUIREMENTS[SUITE].values() for name, _ in clauses}
        paths.update({"examples/empty-input/REVIEW.md", "examples/empty-input/PROGRESS.md", "examples/verification-reporting.md"})
        for name in paths:
            source = checker.ROOT / name
            if source.exists():
                destination = root / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(source, destination)
        return root

    def test_adversarial_reporting_links(self):
        if SUITE != "reporting":
            return
        mutations = [
            ("examples/empty-input/REVIEW.md", "| AC-1, AC-2 |", "| AC-1, AC-1 |", "AC-1"),
            ("examples/empty-input/PROGRESS.md", "| AC-2 | G1 |", "| AC-2 | G9 |", "AC-4"),
            ("examples/empty-input/PROGRESS.md", "| example-check-v1 |", "| check-v0 |", "AC-4"),
            ("examples/empty-input/PROGRESS.md", "| 미관측 | 미수행 |", "| 실제 모델 | 통과 |", "AC-2"),
            ("examples/empty-input/PROGRESS.md", "| 미상 | 기록 없음 |", "| 사건 | 작성 시계 |", "AC-5"),
            ("examples/verification-reporting.md", "| fake model | real model |", "| 없음 | 없음 |", "AC-7"),
            ("examples/verification-reporting.md", "| check-v2 | review-v2 |", "| check-v2 | review-v1 |", "AC-7"),
        ]
        for name, old, new, criterion in mutations:
            with self.subTest(mutation=(name, old)), tempfile.TemporaryDirectory() as directory:
                root = self.fixture(directory)
                path = root / name
                before = path.read_text()
                self.assertIn(old, before)
                path.write_text(before.replace(old, new))
                observation, problems = checker.observe(root, SUITE, criterion)
                self.assertEqual(observation["status"], "violation", problems)

    def test_missing_artifact_and_io_error_are_distinct(self):
        criterion = next(iter(checker.REQUIREMENTS[SUITE]))
        with tempfile.TemporaryDirectory() as directory:
            observation, _ = checker.observe(Path(directory), SUITE, criterion)
            self.assertEqual(observation["status"], "violation")
        with patch.object(Path, "read_text", side_effect=PermissionError("synthetic permission failure")):
            with self.assertRaises(PermissionError):
                checker.observe(checker.ROOT, SUITE, criterion)

    def test_unexpected_failure_is_not_declared_violation(self):
        criterion = next(iter(checker.REQUIREMENTS[SUITE]))
        for error in (PermissionError("synthetic"), AssertionError("unrelated implementation assertion")):
            with tempfile.TemporaryDirectory() as directory:
                destination = Path(directory) / "report.json"
                env = {"HA_EVIDENCE_PATH": str(destination), "HA_EVIDENCE_INVOCATION": "synthetic-call",
                       "HA_EVIDENCE_DECLARATION_DIGEST": "synthetic-digest"}
                with patch.dict(os.environ, env), patch.object(sys, "argv", ["check.py", SUITE, criterion]), \
                        patch.object(checker, "observe", side_effect=error), contextlib.redirect_stdout(io.StringIO()):
                    self.assertEqual(checker.main(), 2)
                observed = json.loads(destination.read_text())["observations"][0]
                self.assertEqual(observed["status"], "error")
                self.assertNotIn("violation", observed)

    def test_existing_report_is_not_overwritten(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "report.json"
            path.write_text("previous invocation")
            with patch.dict(os.environ, {"HA_EVIDENCE_PATH": str(path), "HA_EVIDENCE_INVOCATION": "call",
                                         "HA_EVIDENCE_DECLARATION_DIGEST": "digest"}):
                with self.assertRaises(FileExistsError):
                    checker.emit([])
            self.assertEqual(path.read_text(), "previous invocation")


if __name__ == "__main__":
    unittest.main()
