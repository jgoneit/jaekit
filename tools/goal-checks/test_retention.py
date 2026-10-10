"""Read-only synthetic worktree retention; actual Git and stored output bytes."""
from contextlib import contextmanager, redirect_stdout, redirect_stderr
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[2]
PRODUCT = ROOT / "tools/gap-checks/preservation.py"
spec = importlib.util.spec_from_file_location("retention_consumer", PRODUCT)
consumer = importlib.util.module_from_spec(spec)
spec.loader.exec_module(consumer)
ENV = dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1",
           GIT_AUTHOR_NAME="Synthetic", GIT_AUTHOR_EMAIL="fixture@example.invalid",
           GIT_COMMITTER_NAME="Synthetic", GIT_COMMITTER_EMAIL="fixture@example.invalid")
for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_COMMON_DIR",
            "HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
    ENV.pop(key, None)


class RetentionViolation(AssertionError):
    def __init__(self, code, message):
        self.code = code
        super().__init__(message)


def require(ok, code, message):
    if not ok:
        raise RetentionViolation(code, message)


def git(root, *args):
    raw = subprocess.check_output(["git", *args], cwd=root, env=ENV)
    return os.fsdecode(raw[:-1] if raw.endswith(b"\n") else raw)


@contextmanager
def repositories():
    with tempfile.TemporaryDirectory(prefix="jaekit-retention-") as name:
        primary = Path(name) / "primary"
        linked = Path(name) / "linked"
        primary.mkdir()
        git(primary, "init", "-q", "-b", "main")
        (primary / "README.md").write_text("synthetic public source\n")
        git(primary, "add", "README.md")
        git(primary, "commit", "-qm", "synthetic base")
        git(primary, "worktree", "add", "-q", "-b", "linked", str(linked))
        yield primary, linked


def fixture(root, output_path=".git/ha/synthetic.log"):
    git_dir = Path(git(root, "rev-parse", "--absolute-git-dir"))
    if output_path.startswith(".git/"):
        output = git_dir / output_path[5:]
    else:
        output = Path(output_path) if Path(output_path).is_absolute() else root / output_path
    output.parent.mkdir(parents=True, exist_ok=True)
    output.write_bytes(b"synthetic observed output\n")
    records = root / "local-records/runs.jsonl"
    records.parent.mkdir(parents=True, exist_ok=True)
    row = {"output_path": output_path, "output_digest": hashlib.sha256(output.read_bytes()).hexdigest()}
    records.write_text(json.dumps(row) + "\n")
    config = {"checkout": str(root), "documents": {str(records.relative_to(root)): hashlib.sha256(records.read_bytes()).hexdigest()}}
    return config, records, output


def invoke(config):
    """Call the actual audit; isolate unrelated existing public network checks.

    The legacy entry point has no audit helper. Its only final subprocess is
    the public release suite, stubbed here; setup Git commands remain real.
    """
    if hasattr(consumer, "audit_retention"):
        try:
            return 0, consumer.audit_retention(config)
        except consumer.RetentionProblem as error:
            return error.status, error.kind
    with tempfile.TemporaryDirectory() as name:
        path = Path(name) / "input.json"
        path.write_text(json.dumps(config))
        out = io.StringIO()
        try:
            with patch.object(sys, "argv", [str(PRODUCT), "--retention", str(path)]), \
                 patch.object(consumer.subprocess, "run", return_value=SimpleNamespace(returncode=0)), \
                 redirect_stdout(out), redirect_stderr(out):
                result = consumer.main()
            return result, out.getvalue()
        except (OSError, ValueError, KeyError, TypeError) as error:
            return 2, type(error).__name__


def linked_success():
    with repositories() as (primary, linked):
        for root in (primary, linked):
            config, records, output = fixture(root)
            before = (records.read_bytes(), output.read_bytes(), git(root, "rev-parse", "HEAD"))
            result, detail = invoke(config)
            require(result == 0, "linked-audit-failed", "valid selected checkout retention audit failed: " + str(detail))
            require(before == (records.read_bytes(), output.read_bytes(), git(root, "rev-parse", "HEAD")),
                    "linked-audit-failed", "read-only audit changed its input")


def no_substitution():
    with repositories() as (primary, linked):
        fixture(primary)
        config, _, output = fixture(linked)
        output.unlink()
        status, detail = invoke(config)
        require(status == 2 and detail == "missing", "checkout-link-not-explicit",
                "missing selected-worktree output was substituted or not identified")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_bytes(b"different selected worktree bytes")
        status, detail = invoke(config)
        require(status == 1 and detail == "output_digest", "checkout-link-not-explicit",
                "selected-worktree digest mismatch was lost")


def causes():
    with repositories() as (_, linked):
        config, records, output = fixture(linked)
        valid = records.read_bytes()
        records.write_bytes(b"changed")
        require(invoke(config) == (1, "document_digest"), "retention-cause-lost", "document mismatch not classified")
        records.write_bytes(valid)
        output.unlink()
        require(invoke(config) == (2, "missing"), "retention-cause-lost", "missing output not classified")
        output.mkdir()
        require(invoke(config) == (2, "unreadable"), "retention-cause-lost", "unreadable output not classified")
        require(invoke({"checkout": str(linked), "documents": {"x": 1}}) == (2, "invalid_input"),
                "retention-cause-lost", "malformed input not classified")
        # A supplied existing directory without Git cannot resolve logical logs.
        plain = linked.parent / "plain"
        plain.mkdir()
        (plain / "local-records").mkdir()
        (plain / "local-records/runs.jsonl").write_bytes(valid)
        config["checkout"] = str(plain)
        require(invoke(config) == (2, "git_lookup"), "retention-cause-lost", "Git lookup failure not classified")


class RetentionChecks(unittest.TestCase):
    def test_primary_and_linked_git_log_paths(self):
        linked_success()
    def test_other_worktree_and_common_logs_never_substitute(self):
        no_substitution()
    def test_integrity_missing_unreadable_lookup_and_invalid_causes(self):
        causes()
    def test_absolute_and_normal_relative_paths(self):
        with repositories() as (primary, linked):
            for output_path in ("local-output/synthetic.log", str(primary.parent / "explicit.log")):
                config, _, _ = fixture(linked, output_path)
                self.assertEqual(invoke(config)[0], 0)
    def test_cli_read_only_success_and_rejection_without_public_network(self):
        with repositories() as (_, linked):
            config, records, output = fixture(linked)
            path = linked.parent / "input.json"
            path.write_text(json.dumps(config))
            argv = [sys.executable, str(PRODUCT), "--retention", str(path), "--retention-only"]
            before = (records.read_bytes(), output.read_bytes(), path.read_bytes())
            result = subprocess.run(argv, env=ENV, capture_output=True, text=True, timeout=20)
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("selected checkout only", result.stdout)
            self.assertEqual(before, (records.read_bytes(), output.read_bytes(), path.read_bytes()))
            output.unlink()
            result = subprocess.run(argv, env=ENV, capture_output=True, text=True, timeout=20)
            self.assertEqual(result.returncode, 2)
            self.assertIn("retention missing", result.stderr)
            self.assertNotIn("PASS", result.stdout)
            self.assertNotIn("Traceback", result.stderr)
    def test_no_input_keeps_public_checks_and_only_option_requires_input(self):
        with patch.object(consumer.subprocess, "run", return_value=SimpleNamespace(returncode=9)) as run:
            self.assertEqual(consumer.main([]), 9)
        self.assertIn("test_preservation.py", run.call_args.args[0])
        with redirect_stderr(io.StringIO()), self.assertRaises(SystemExit) as err:
            consumer.main(["--retention-only"])
        self.assertEqual(err.exception.code, 2)
    def test_inherited_git_override_cannot_redirect_selected_checkout(self):
        with repositories() as (primary, linked):
            config, _, _ = fixture(linked)
            with patch.dict(os.environ, {"GIT_DIR": str(primary / ".git"), "GIT_WORK_TREE": str(primary)}):
                self.assertEqual(invoke(config)[0], 0)
    def test_orphan_output_digest_is_not_a_successful_empty_audit(self):
        with repositories() as (_, linked):
            config, records, _ = fixture(linked)
            path = linked.parent / "input.json"
            for output_path in (None, ""):
                records.write_text(json.dumps({"output_path": output_path, "output_digest": "a" * 64}) + "\n")
                config["documents"][str(records.relative_to(linked))] = hashlib.sha256(records.read_bytes()).hexdigest()
                self.assertEqual(invoke(config), (2, "invalid_input"))
                path.write_text(json.dumps(config))
                result = subprocess.run([sys.executable, str(PRODUCT), "--retention", str(path), "--retention-only"],
                                        env=ENV, capture_output=True, text=True, timeout=20)
                self.assertEqual(result.returncode, 2)
                self.assertNotIn("PASS", result.stdout)
                self.assertIn("invalid_input", result.stderr)
            records.write_text('{"kind":"start"}\n{"output_path":"","output_digest":""}\n')
            config["documents"][str(records.relative_to(linked))] = hashlib.sha256(records.read_bytes()).hexdigest()
            self.assertEqual(invoke(config), (0, (1, 0)))
    def test_invalid_record_paths_are_visible(self):
        with repositories() as (_, linked):
            config, records, _ = fixture(linked)
            for value in ("bad\0path", ".git/../other", False):
                records.write_text(json.dumps({"output_path": value, "output_digest": "a" * 64}) + "\n")
                config["documents"][str(records.relative_to(linked))] = hashlib.sha256(records.read_bytes()).hexdigest()
                self.assertEqual(invoke(config), (2, "invalid_input"))
