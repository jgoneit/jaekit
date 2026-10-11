"""Offline regression tests for rejection and evidence boundaries, never host runs."""
import importlib.util
import io
import json
from pathlib import Path
import subprocess
import tarfile
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location("release_acceptance", Path(__file__).with_name("check.py"))
check = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(check)


class EvidenceChecks(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.checker = object.__new__(check.Checker)
        self.checker.product_root = self.root
        self.checker.private = self.root
        self.checker._evidence = None
        self.checker._release = None
        self.checker._downloads = {}
        self.checker._source = {}

    def artifact(self, name, data):
        if isinstance(data, str):
            data = data.encode()
        (self.root / name).write_bytes(data)
        return {"path": name, "sha256": check.digest(data)}

    def capture(self, argv, events):
        return {"argv": argv, "exit_code": 0,
                "stdout": self.artifact("stdout", "\n".join(json.dumps(e) for e in events) + "\n"),
                "stderr": self.artifact("stderr", "")}

    def test_artifact_requires_file_and_unchanged_digest(self):
        for assertion in (True, {"success": True}, {"path": "missing", "sha256": "bad"}):
            with self.subTest(assertion=assertion), self.assertRaises(check.Failure):
                self.checker.artifact(assertion)
        item = self.artifact("observed", "original")
        self.assertEqual(self.checker.artifact(item)[1], b"original")
        (self.root / "observed").write_text("changed")
        with self.assertRaisesRegex(check.Failure, "changed"):
            self.checker.artifact(item)

    def test_missing_observation_is_unverified(self):
        with self.assertRaises(check.Unavailable):
            self.checker.artifact({"path": "missing", "sha256": "0" * 64})

    def test_failed_capture_cannot_become_success(self):
        value = self.capture(["claude", "--version"], [{"actual": "output"}])
        for result in (1, False, "0", None):
            value["exit_code"] = result
            with self.subTest(result=result), self.assertRaises(check.Failure):
                self.checker.capture(value)

    def test_release_absence_precedes_unprepared_receipts(self):
        missing = subprocess.CompletedProcess([], 1, b"", b"gh: Not Found (HTTP 404)")
        for number in range(1, 7):
            with self.subTest(criterion=number), patch.object(check, "command", return_value=missing), \
                    self.assertRaisesRegex(check.Failure, "public v0.1.3 release does not exist"):
                self.checker._release = None
                getattr(self.checker, f"check_{number}")()

    def test_permission_failure_is_not_reported_as_missing_release(self):
        denied = subprocess.CompletedProcess([], 1, b"", b"gh: Forbidden (HTTP 403)")
        with patch.object(check, "command", return_value=denied), self.assertRaises(check.Unavailable):
            self.checker.release()

    def test_preservation_includes_asset_identity_and_content(self):
        release = {"tag_name": "v0.1.2", "id": 7, "assets": [
            {"name": "checksums.txt", "id": 8, "size": 10, "digest": "sha256:" + "a" * 64}]}
        original = check.release_snapshot(release, "b" * 40)
        for key, value in (("id", 9), ("size", 11), ("digest", "sha256:" + "c" * 64)):
            changed = json.loads(json.dumps(release))
            changed["assets"][0][key] = value
            self.assertNotEqual(original, check.release_snapshot(changed, "b" * 40))

    def test_native_identity_does_not_accept_assistant_claim(self):
        claim = [{"type": "assistant", "message": {"content": [
            {"type": "text", "text": "model observed-model session session-1234"}]}}]
        self.assertEqual(check.native_identity(claim), (set(), set()))
        actual = [{"type": "thread.started", "thread_id": "session-1234"},
                  {"type": "turn_context", "payload": {"model": "observed-model"}}]
        self.assertEqual(check.native_identity(actual), ({"session-1234"}, {"observed-model"}))

    def test_native_tool_events_required_not_receipt_booleans(self):
        events = [{"success": True, "model": "observed-model", "done": True}]
        self.assertFalse(check.native_tools(events))
        self.assertEqual(check.tool_commands(events), [])
        events = [{"type": "assistant", "message": {"content": [
            {"type": "tool_use", "name": "Write", "input": {"file_path": "goal/SPEC.md"}}]}}]
        self.assertTrue(check.native_tools(events))

    def test_native_rollout_and_mcp_command_shapes(self):
        events = [
            {"type": "response_item", "payload": {"type": "function_call", "name": "exec_command",
              "arguments": json.dumps({"cmd": "ha start goal"})}},
            {"type": "response_item", "payload": {"type": "custom_tool_call", "name": "functions.exec",
              "input": 'text(await tools.exec_command({cmd: "ha check goal --baseline"}));'}},
            {"type": "item.completed", "item": {"type": "mcp_tool_call", "tool": "exec_command",
              "arguments": {"cmd": "ha done goal"}}},
        ]
        commands = check.tool_commands(events)
        self.assertEqual(len(commands), 3)
        self.assertIn("ha check goal --baseline", commands[1])
        self.assertTrue(check.native_tools(events))
        self.assertEqual(check.call_commands("unknown_tool", {"cmd": "ha done goal"}), [])

    def test_spec_trace_rejects_execution_and_foreign_session(self):
        skill = self.artifact("SKILL.md", "synthetic skill")
        path = str((self.root / "SKILL.md").resolve())
        events = [{"type": "thread.started", "thread_id": "session-1234"},
                  {"type": "item.completed", "item": {"type": "command_execution",
                                                        "command": "ha start goal"}}, {"type": "turn.completed"}]
        context = [{"type": "session_meta", "payload": {"id": "session-1234"}},
                   {"type": "turn_context", "payload": {"model": "observed-model"}},
                   {"type": "response_item", "payload": {"type": "message", "role": "user",
                    "content": [{"type": "input_text", "text":
                                 f"<skill>\n<name>spec:spec</name>\n<path>{path}</path>\n---\nname: synthetic\n"}]}}]
        value = {"model": "observed-model", "session_id": "session-1234", "plugins": {"spec": skill},
                 "spec": self.capture(["codex", "exec", "--json", "$spec:spec a synthetic goal"], events),
                 "session_trace": self.artifact("rollout", "\n".join(json.dumps(e) for e in context))}
        # This rollout is a snapshot of the Spec request, not a whole-session
        # record that could contain a later unrelated Skill injection.
        value["spec"]["session_trace"] = value.pop("session_trace")
        with self.assertRaisesRegex(check.Failure, "Spec observation performed"):
            self.checker.trace("codex", value, "spec")
        value["session_id"] = "different-session"
        with self.assertRaises(check.Failure):
            self.checker.trace("codex", value, "spec")

    def test_record_chain_and_raw_logs_preserve_failed_attempts(self):
        gitdir = self.root / ".git"
        raw_path = gitdir / "ha/runs/synthetic/2.log"
        raw_path.parent.mkdir(parents=True)
        raw_path.write_bytes(b"actual check output\n")
        records = [
            {"kind": "start", "rules": "run-rules/3", "host": {"model": "observed-model"}},
            {"kind": "check", "result": "error", "output_path": "", "output_digest": "",
             "evidence": {"result": "error", "reason": "process_output"}},
            {"kind": "baseline", "target": {"criterion": "AC-1"}, "result": "fail_as_expected",
             "evidence": {"result": "fail_as_expected"}},
            {"kind": "check", "target": {"criterion": "AC-1"}, "result": "pass", "evidence": {"result": "pass"}},
            {"kind": "done", "status": "complete"},
        ]
        prev, lines = None, []
        for seq, row in enumerate(records, 1):
            row.update(schema="run/v1", seq=seq, prev=prev, ha_version="0.1.3")
            if row["kind"] in ("check", "baseline") and row.get("result") != "error":
                row.update(output_path=".git/ha/runs/synthetic/2.log", output_digest=check.digest(raw_path.read_bytes()))
            line = json.dumps(row).encode()
            lines.append(line)
            prev = check.digest(line)
        raw = b"\n".join(lines) + b"\n"
        check.validate_records(raw, self.root, gitdir, {"model": "observed-model"})
        with self.assertRaisesRegex(check.Failure, "chain"):
            check.validate_records(raw.replace(b'"seq": 2', b'"seq": 5'), self.root, gitdir,
                                   {"model": "observed-model"})
        raw_path.write_bytes(b"changed output")
        with self.assertRaisesRegex(check.Failure, "digest"):
            check.validate_records(raw, self.root, gitdir, {"model": "observed-model"})

    def test_satisfied_conditions_do_not_replace_valid_completion_record(self):
        report = {"schema": "status/v1", "status": "complete", "rules": "run-rules/3",
                  "completion_record": 5}
        check.validate_completion(report, 5)
        for value in (None, 4, True, "5"):
            report["completion_record"] = value
            with self.subTest(value=value), self.assertRaises(check.Failure):
                check.validate_completion(report, 5)

    def test_duplicate_json_keys_are_rejected(self):
        with self.assertRaises(check.Failure):
            check.decode('{"exit_code": 1, "exit_code": 0}')

    def test_empty_regression_suite_does_not_pass(self):
        result = subprocess.CompletedProcess([], 0, b"", b"Ran 0 tests in 0s\nOK")
        with patch.object(check, "command", return_value=result), self.assertRaises(check.Failure):
            self.checker.check_10()

    def test_formula_binds_each_hash_to_its_platform(self):
        assets = {}
        formula = 'version "0.1.3"\n'
        for system, selector in (("darwin", "macos"), ("linux", "linux")):
            formula += f"on_{selector} do\n"
            for arch, architecture in (("arm64", "arm"), ("amd64", "intel")):
                name = f"ha_0.1.3_{system}_{arch}.tar.gz"
                hash_value = check.digest(name.encode())
                assets[name] = {"digest": "sha256:" + hash_value}
                formula += (f'on_{architecture} do\nurl "https://github.com/jgoneit/jaekit/releases/download/'
                            f'v0.1.3/{name}"\nsha256 "{hash_value}"\nend\n')
            formula += "end\n"
        formula += "def install\nend\n"
        check.validate_formula(formula, assets)
        hashes = [value["digest"][7:] for value in assets.values()]
        swapped = formula.replace(hashes[0], "SWAP").replace(hashes[1], hashes[0]).replace("SWAP", hashes[1])
        with self.assertRaisesRegex(check.Failure, "pair"):
            check.validate_formula(swapped, assets)

    def test_binary_only_installation_does_not_supply_reference_example(self):
        files = {"ha": b"synthetic binary", "tools/check-result-reference.py": b"synthetic producer",
                 "examples/check-result/reference.json": b"{}"}
        with self.assertRaisesRegex(check.Failure, "installed support"):
            check.validate_support_files(self.root, files)
        for relative, data in files.items():
            if relative != "ha":
                path = self.root / relative
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(data)
        check.validate_support_files(self.root, files)
        (self.root / "tools/check-result-reference.py").write_bytes(b"different producer")
        with self.assertRaises(check.Failure):
            check.validate_support_files(self.root, files)

    def test_actual_path_selection_cannot_be_replaced_with_absolute_path_receipt(self):
        binary = self.root / "ha"
        binary.write_bytes(b"old Core")
        with patch.object(check.shutil, "which", return_value=str(binary)), \
                patch.object(check, "checked") as query:
            with self.assertRaisesRegex(check.Failure, "PATH selects"):
                check.validate_selected_core(b"public Core")
            query.assert_not_called()
            binary.write_bytes(b"public Core")
            query.side_effect = [b"ha 0.1.3\n", json.dumps({"schema": "ha-capabilities/v1",
                                 "ha_version": "0.1.3", "default_rules": "run-rules/3"}).encode()]
            check.validate_selected_core(b"public Core")


class ArchiveChecks(unittest.TestCase):
    def archive(self, name, kind=None):
        data = io.BytesIO()
        with tarfile.open(fileobj=data, mode="w:gz") as archive:
            member = tarfile.TarInfo(name)
            if kind:
                member.type = kind
                member.linkname = "outside"
            archive.addfile(member, io.BytesIO())
        return data.getvalue()

    def test_archive_rejects_traversal_and_links_without_extracting(self):
        for name, kind in (("../escape", None), ("/absolute", None),
                           ("ha_0.1.3_linux_amd64/link", tarfile.SYMTYPE)):
            with self.subTest(name=name), self.assertRaises(check.Failure):
                check.archive_files(self.archive(name, kind), "linux_amd64")


if __name__ == "__main__":
    unittest.main()
