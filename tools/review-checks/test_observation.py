"""Synthetic request binding and original-run observation regressions.

The native envelopes are fixtures. Collector cases actually launch a local
shell and a tiny executable; they do not run a host model or a real goal.
"""
import copy
import importlib.util
import json
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import unittest

ROOT = Path(__file__).resolve().parents[2]
RELEASE = ROOT / "tools/release-checks"
spec = importlib.util.spec_from_file_location("observation_fixtures", RELEASE / "test_loader_invocations.py")
base = importlib.util.module_from_spec(spec)
spec.loader.exec_module(base)
check = base.check


class Case(base.TraceCase):
    def read_events(self, artifact):
        return [json.loads(line) for line in (self.root / artifact["path"]).read_text().splitlines()]

    def bound_claude(self, phase="spec", commands=("ls",), current_skill=True):
        value, _ = self.claude()
        entries = self.read_events(value["session_trace"])
        number = 1 if phase == "spec" else 2
        prompt = f"prompt-{number}"
        if not current_skill:
            entries = [e for e in entries if e["promptId"] != prompt]
            entries += [{"type": "user", "sessionId": base.SESSION, "uuid": f"entry-{number}",
                         "promptId": prompt, "message": {"role": "user", "content": "List files."}}]
        captured = self.read_events(value[phase]["stdout"])
        assistant = {"type": "assistant", "session_id": base.SESSION, "uuid": f"reply-{number}",
                     "message": {"content": [{"type": "tool_use", "id": f"tool-{number}-{i}",
                                              "name": "Bash", "input": {"command": command}}
                                             for i, command in enumerate(commands)]}}
        entries.append(dict(assistant, sessionId=base.SESSION, promptId=prompt,
                            parentUuid=f"entry-{number}-skill" if current_skill else f"entry-{number}"))
        value[phase]["stdout"] = self.lines([captured[0], assistant, captured[-1]])
        value["session_trace"] = self.lines(entries)
        return value

    def unknown(self, host, value, phase):
        try:
            self.checker.trace(host, value, phase)
        except check.Unavailable:
            return
        except check.Failure as error:
            self.fail("[requirement] ambiguous evidence must be unverified, not a definite violation: " + str(error))
        self.fail("[requirement] ambiguous evidence must not pass")

    def codex_commands(self, phase, commands, javascript=False):
        def rollout(path):
            events = self.session_start() + [self.injection(phase + ":" + phase, path)]
            for i, command in enumerate(commands):
                events.append({"type": "response_item", "payload": {
                    "type": "custom_tool_call" if javascript else "function_call",
                    "name": "exec" if javascript else "exec_command", "call_id": f"tool-codex-{i}",
                    "input" if javascript else "arguments": command if javascript else json.dumps({"cmd": command})}})
            return events
        if phase == "spec":
            value, _ = self.codex(rollout)
        else:
            value, _ = self.codex(lambda path: self.session_start() + [self.injection("spec:spec", path)],
                                  seal_rollout=rollout, seal_commands=("ls",))
        return value

    def collected(self, script, shell="bash", core_source=None):
        collector = RELEASE / "observe.py"
        self.assertTrue(collector.is_file(), "[requirement] an original-run collector must exist")
        shell_path = shutil.which(shell)
        if shell_path is None:
            raise RuntimeError(f"test environment lacks {shell}")
        self.count += 1
        directory = self.root / f"observation-{self.count}"
        directory.mkdir()
        core = directory / "ha"
        marker = directory / "actual.jsonl"
        source = core_source or (
            f"#!{sys.executable}\nimport json,sys\n"
            f"with open({str(marker)!r}, 'a') as f: f.write(json.dumps(sys.argv[1:])+'\\n')\n"
            "print('core-output')\n"
            "sys.exit(7 if '--fail' in sys.argv else 0)\n")
        core.write_text(source)
        core.chmod(0o755)
        script = script.replace("@CORE@", shlex.quote(str(core)))
        output = directory / "execution.json"
        argv = [sys.executable, str(collector), "run", "--core", str(core), "--shell", shell_path,
                "--output", str(output), "--invocation", f"observation-{self.count}", "--", script]
        result = subprocess.run(argv, capture_output=True, timeout=20)
        self.assertTrue(output.is_file(), "[requirement] collector must preserve a report, including failed runs")
        artifact = {"path": str(output), "sha256": check.digest(output.read_bytes())}
        if not hasattr(self, "receipts"):
            self.receipts = {}
        self.receipts[str(output)] = result.stderr.decode()
        actual = [json.loads(line) for line in marker.read_text().splitlines()] if marker.exists() else []
        return argv, artifact, core, actual, result

    def observed_value(self, host, phase, argv, artifact, core, receipt=None):
        command = shlex.join(argv)
        if host == "claude":
            value = self.bound_claude(phase, (command,))
            tool = f"tool-{1 if phase == 'spec' else 2}-0"
        else:
            value = self.codex_commands(phase, (command,))
            tool = "tool-codex-0"
        value["core"] = {"path": str(core), "sha256": check.digest(core.read_bytes())}
        value[phase]["executions"] = [{"tool_call_id": tool, "report": artifact}]
        receipt = self.receipts.get(artifact["path"], "") if receipt is None else receipt
        if host == "claude":
            number = 1 if phase == "spec" else 2
            event = {"type": "user", "uuid": f"result-{number}", "message": {"content": [
                {"type": "tool_result", "tool_use_id": tool, "content": receipt}]}}
            events = self.read_events(value[phase]["stdout"])
            events.insert(-1, event)
            value[phase]["stdout"] = self.lines(events)
            entries = self.read_events(value["session_trace"])
            entries.append(dict(event, sessionId=base.SESSION, promptId=f"prompt-{number}", parentUuid=f"reply-{number}"))
            value["session_trace"] = self.lines(entries)
        else:
            entries = self.read_events(value[phase]["session_trace"])
            entries.append({"type": "response_item", "payload": {"type": "function_call_output",
                            "call_id": tool, "output": json.dumps({"output": receipt})}})
            value[phase]["session_trace"] = self.lines(entries)
        return value


class AC1(Case):
    def test_same_request_accepts_full_and_phase_transcripts(self):
        for phase, commands in (("spec", ("ls",)), ("seal", base.SEAL_COMMANDS)):
            value = self.bound_claude(phase, commands)
            self.accepts("claude", value, phase)
            number = 1 if phase == "spec" else 2
            entries = [e for e in self.read_events(value["session_trace"]) if e.get("promptId") == f"prompt-{number}"]
            value[phase]["session_trace"] = self.lines(entries)
            self.accepts("claude", value, phase)

    def test_other_request_cannot_supply_the_skill(self):
        for phase, commands in (("spec", ("ls",)), ("seal", base.SEAL_COMMANDS)):
            for order in ("earlier", "later"):
                with self.subTest(phase=phase, order=order):
                    value = self.bound_claude(phase, commands, current_skill=False)
                    installed = self.root / value["plugins"][phase]["path"]
                    other = self.invocation(phase + ":" + phase, str(installed.parent), number=9)
                    entries = self.read_events(value["session_trace"])
                    value["session_trace"] = self.lines(other + entries if order == "earlier" else entries + other)
                    self.rejects("claude", value, phase, "loaded Skill")


class AC2(Case):
    def test_no_native_request_anchor_is_unverified(self):
        value, _ = self.claude()
        events = self.read_events(value["spec"]["stdout"])
        for event in events:
            event.pop("uuid", None)
        value["spec"]["stdout"] = self.lines(events)
        self.unknown("claude", value, "spec")

    def test_conflicting_or_foreign_anchor_is_unverified(self):
        for change in ("duplicate", "foreign"):
            value = self.bound_claude()
            entries = self.read_events(value["session_trace"])
            if change == "duplicate":
                entries.append(dict(entries[-1], promptId="another-request"))
            else:
                entries[-1]["sessionId"] = "other-session"
            value["session_trace"] = self.lines(entries)
            self.unknown("claude", value, "spec")

    def test_one_matched_text_anchor_cannot_cover_an_unmatched_tool_call(self):
        value = self.bound_claude()
        events = self.read_events(value["spec"]["stdout"])
        events.insert(-1, {"type": "assistant", "uuid": "unmatched-tool-message", "message": {"content": [
            {"type": "tool_use", "id": "unmatched-tool", "name": "Bash", "input": {"command": "ls"}}]}})
        value["spec"]["stdout"] = self.lines(events)
        self.unknown("claude", value, "spec")


class AC3(Case):
    def test_quoted_cmd_keys_have_same_phase_verdicts(self):
        for key in ("cmd", "'cmd'", '"cmd"'):
            with self.subTest(key=key):
                code = f"await tools.exec_command({{{key}: 'ha note goal input'}})"
                self.rejects("codex", self.codex_commands("spec", (code,), True), "spec", "execution work")
                codes = [f"await tools.exec_command({{{key}: {json.dumps(c)}}})" for c in base.SEAL_COMMANDS]
                self.accepts("codex", self.codex_commands("seal", codes, True), "seal")

    def test_dynamic_cmd_values_are_not_evaluated(self):
        self.assertEqual(check.script_commands('tools.exec_command({"cmd": buildCommand()})'), [])


class AC4(Case):
    def test_redirection_and_expanding_heredoc_substitutions_count(self):
        for command in ('printf x > "$(ha start goal)"', 'cat <<< "$(ha start goal)"',
                        'cat <<EOF\n$(ha start goal)\nEOF', 'cat <<-EOF\n\t`ha start goal`\n\tEOF'):
            with self.subTest(command=command):
                self.assertEqual(check.core_operations(command, check.SPEC_FORBIDDEN), {"start"},
                                 "[requirement] executed substitution must not disappear with its redirection")
                self.rejects("codex", self.codex_commands("spec", (command,)), "spec", "execution work")

    def test_quoted_and_escaped_heredocs_remain_data(self):
        for command in ("cat <<'EOF'\n$(ha start goal)\nEOF", 'cat <<EOF\n\\$(ha start goal)\nEOF',
                        'cat <<EOF\nha start goal\nEOF', 'cat <<E"O"F\n$(ha start goal)\nEOF'):
            self.assertEqual(check.core_operations(command, check.SPEC_FORBIDDEN), set())
            self.accepts("codex", self.codex_commands("spec", (command,)), "spec")


class AC5(Case):
    def test_definition_is_not_execution_or_parent_assignment(self):
        for command in ("f() { ha start goal; }", "function f { ha start goal; }",
                        "f() { HA=/opt/bin/ha; }; $HA start goal"):
            self.assertEqual(check.core_operations(command, check.SEAL_REQUIRED), set(),
                             "[requirement] an uncalled function must not run or mutate parent scope")

    def test_child_assignments_do_not_leak_or_clobber_parent(self):
        for child in ("(HA=/opt/bin/ha)", "HA=/opt/bin/ha &", "HA=/opt/bin/ha | cat"):
            self.assertEqual(check.core_operations(child + "\n$HA start goal", check.SEAL_REQUIRED), set())
        for child in ("(HA=/usr/bin/printf)", "HA=/usr/bin/printf &", "HA=/usr/bin/printf | cat"):
            self.assertEqual(check.core_operations("HA=/opt/bin/ha; " + child + "\n$HA start goal",
                                                   check.SEAL_REQUIRED), {"start"})


class AC6(Case):
    def test_uncertain_core_flow_without_collection_is_unverified(self):
        for command in ("if false; then ha start goal; fi", "false && ha start goal",
                        "true || ha start goal", "f() { ha start goal; }; f",
                        "if false; then ha() { printf data; }; fi; ha start goal",
                        "{ f() { ha start goal; }; }; f"):
            for host in ("claude", "codex"):
                with self.subTest(host=host, command=command):
                    value = self.bound_claude(commands=(command,)) if host == "claude" else self.codex_commands("spec", (command,))
                    self.unknown(host, value, "spec")

    def test_unrelated_control_flow_does_not_block_spec(self):
        value = self.codex_commands("spec", ("if false; then printf unused; fi",))
        self.accepts("codex", value, "spec")


class AC14(unittest.TestCase):
    def test_public_instructions_cover_collection_and_request_binding(self):
        text = (RELEASE / "README.md").read_text()
        for phrase in ("observe.py", "promptId", "tool_call_id", "original execution", "incomplete",
                       "unquoted", "unsupported", "never replay"):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, text, "[requirement] public instructions must explain the supported evidence boundary")


class AC15(Case):
    def test_shell_pipeline_scope_is_observed_not_predicted(self):
        for shell, expected in (("bash", "outer"), ("zsh", "inner")):
            argv, artifact, core, actual, result = self.collected(
                'VALUE=outer; printf x | VALUE=inner; "$JAEKIT_CORE" note "$VALUE"', shell)
            self.assertEqual(result.returncode, 0)
            self.assertEqual(actual, [["note", expected]])
            self.rejects("codex", self.observed_value("codex", "spec", argv, artifact, core), "spec", "execution work")

    def test_an_old_report_cannot_replace_a_later_identical_command(self):
        argv, first, core, _, _ = self.collected('if false; then "$JAEKIT_CORE" start goal; fi')
        original = Path(first["path"]).read_bytes()
        saved = self.write("first-execution.json", original)
        # Two original synthetic executions, not checker replay. Keep the first
        # bytes and collect a fresh native result for the identical second argv.
        Path(first["path"]).unlink()
        second = subprocess.run(argv, capture_output=True, timeout=20)
        self.assertEqual(second.returncode, 0)
        self.assertNotEqual(json.loads(original)["run_id"], json.loads(Path(first["path"]).read_bytes())["run_id"])
        for host in ("claude", "codex"):
            value = self.observed_value(host, "spec", argv, saved, core, second.stderr.decode())
            self.unknown(host, value, "spec")

    def test_real_collection_taken_untaken_functions_and_expanded_arguments(self):
        scripts = [
            ('if false; then "$JAEKIT_CORE" start goal; fi; f() { "$JAEKIT_CORE" done goal; }; printf "ha start goal"', []),
            ('value="two words"; f() { "$JAEKIT_CORE" note "$value"; }; if true; then f; fi', [["note", "two words"]]),
            ('false || "$JAEKIT_CORE" start goal; true && "$JAEKIT_CORE" check goal --baseline AC-1; "$JAEKIT_CORE" done goal',
             [["start", "goal"], ["check", "goal", "--baseline", "AC-1"], ["done", "goal"]])]
        for shell in ("bash", "zsh"):
            for script, expected in scripts:
                with self.subTest(shell=shell, script=script):
                    argv, artifact, core, actual, result = self.collected(script, shell)
                    self.assertEqual(result.returncode, 0)
                    self.assertEqual(actual, expected, "[requirement] collection must not replay or alter arguments")
                    for host in ("claude", "codex"):
                        phase = "seal" if len(expected) == 3 else "spec"
                        value = self.observed_value(host, phase, argv, artifact, core)
                        if len(expected) == 1:
                            self.rejects(host, value, phase, "execution work")
                        else:
                            self.accepts(host, value, phase)

    def test_complete_absence_is_missing_operation_not_unknown(self):
        argv, artifact, core, actual, _ = self.collected('if false; then "$JAEKIT_CORE" start goal; fi')
        self.assertEqual(actual, [])
        value = self.observed_value("claude", "seal", argv, artifact, core)
        self.rejects("claude", value, "seal", "does not observe ha start")

    def test_original_exit_output_and_invocation_are_preserved(self):
        argv, artifact, core, actual, result = self.collected('"$JAEKIT_CORE" check goal --fail')
        self.assertEqual(result.returncode, 7)
        self.assertEqual(result.stdout, b"core-output\n")
        self.assertEqual(actual, [["check", "goal", "--fail"]])
        value = self.observed_value("codex", "spec", argv, artifact, core)
        self.rejects("codex", value, "spec", "execution work")

    def test_misbound_incomplete_or_changed_evidence_is_never_static_fallback(self):
        argv, artifact, core, _, _ = self.collected('if false; then "$JAEKIT_CORE" start goal; fi')
        original = json.loads(Path(artifact["path"]).read_bytes())
        for mutation in ("tool", "invocation", "truncate", "core", "schema"):
            with self.subTest(mutation=mutation):
                value = self.observed_value("claude", "spec", argv, artifact, core)
                report = copy.deepcopy(original)
                if mutation == "tool":
                    value["spec"]["executions"][0]["tool_call_id"] = "different-call"
                elif mutation == "core":
                    value["core"]["sha256"] = "0" * 64
                else:
                    if mutation == "invocation":
                        report["invocation"] = "another-execution"
                    elif mutation == "schema":
                        report["schema"] = "unknown-format/v999"
                    else:
                        report["events"] = report["events"][:-1]
                    value["spec"]["executions"][0]["report"] = self.write(
                        "damaged-" + mutation + ".json", json.dumps(report))
                self.unknown("claude", value, "spec")

    def test_unwrapped_absolute_core_and_unsupported_execution_are_unverified(self):
        for script in ('@CORE@ start goal', 'eval \'"$JAEKIT_CORE" start goal\''):
            argv, artifact, core, _, _ = self.collected(script)
            value = self.observed_value("codex", "spec", argv, artifact, core)
            self.unknown("codex", value, "spec")

    def test_spawn_failure_is_not_a_core_start(self):
        argv, artifact, core, actual, result = self.collected('"$JAEKIT_CORE" start goal',
                                                            core_source="#!/no/such/interpreter\n")
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(actual, [])
        self.unknown("claude", self.observed_value("claude", "spec", argv, artifact, core), "spec")

    def test_implicit_builtin_assignment_and_arithmetic_are_outside_coverage(self):
        for script in ('printf -v JAEKIT_CORE "@CORE@"; "$JAEKIT_CORE" start goal',
                       '[[ -v \'array[$(@CORE@ start goal)]\' ]]',
                       'printf "%n" JAEKIT_CORE'):
            argv, artifact, core, _, _ = self.collected(script)
            self.unknown("claude", self.observed_value("claude", "spec", argv, artifact, core), "spec")

    def test_forward_conditional_and_nested_definitions_do_not_authorize_external_commands(self):
        for script in ('missing_f; missing_f() { "$JAEKIT_CORE" start goal; }',
                       'if false; then missing_f() { "$JAEKIT_CORE" start goal; }; fi; missing_f',
                       'f() { missing_g() { "$JAEKIT_CORE" start goal; }; }; missing_g'):
            argv, artifact, core, _, _ = self.collected(script)
            self.unknown("codex", self.observed_value("codex", "spec", argv, artifact, core), "spec")

    def test_launcher_word_splitting_and_ifs_changes_are_outside_coverage(self):
        for script in ('$JAEKIT_CORE start goal', 'IFS=/; "$JAEKIT_CORE" start goal'):
            argv, artifact, core, _, _ = self.collected(script)
            self.unknown("claude", self.observed_value("claude", "spec", argv, artifact, core), "spec")

    def test_launcher_rewrite_cannot_claim_complete_coverage(self):
        script = 'printf \'#!/bin/sh\\nexec @CORE@ "$@"\\n\' > "$JAEKIT_CORE"; "$JAEKIT_CORE" start goal'
        argv, artifact, core, actual, _ = self.collected(script)
        self.assertEqual(actual, [["start", "goal"]])
        self.unknown("codex", self.observed_value("codex", "spec", argv, artifact, core), "spec")


if __name__ == "__main__":
    unittest.main()
