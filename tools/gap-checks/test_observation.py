"""Synthetic native observations for incomplete command-reading boundaries.

Only the receipt preservation case executes a synthetic shell/child through
the existing opt-in collector. The ambiguous command examples are never run.
"""
import copy
import importlib.util
import json
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location(
    "gap_observation_fixtures", ROOT / "tools/review-checks/test_observation.py")
fixtures = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(fixtures)
check = fixtures.check
SEAL = fixtures.base.SEAL_COMMANDS


class Case(fixtures.Case):
    def native(self, host, phase, commands):
        return (self.bound_claude(phase, commands) if host == "claude"
                else self.codex_commands(phase, commands))

    def unverified(self, host, phase, commands, tool_id=None):
        return self.unverified_value(host, self.native(host, phase, commands), phase, tool_id)

    def unverified_value(self, host, value, phase, tool_id=None):
        try:
            self.checker.trace(host, value, phase)
        except check.Unavailable as error:
            message = str(error)
            self.assertTrue(message.strip(), "[requirement] unverified evidence needs a diagnostic")
            if tool_id:
                self.assertIn(tool_id, message, "[requirement] uncertainty must identify the native tool call")
            return message
        except check.Failure as error:
            self.fail("[requirement] unsupported evidence is unverified, not a definite violation: " + str(error))
        self.fail("[requirement] unsupported evidence must not pass")

    def definite_failure(self, host, value, phase, message):
        try:
            self.checker.trace(host, value, phase)
        except check.Unavailable as error:
            self.fail("[requirement] complete supported evidence must have a definite verdict: " + str(error))
        except check.Failure as error:
            self.assertIn(message, str(error))
            return
        self.fail("[requirement] definite requirement violation must fail")


    def definite_candidate_failure(self, host, value, message):
        # The static requirement is still tested, while final acceptance needs
        # actual Core/goal provenance absent from this fixture.
        with self.assertRaisesRegex(check.Failure, message):
            self.checker.trace_candidates(host, value, "seal")
        self.unverified_value(host, value, "seal")


class AC1(Case):
    def test_evaluators_and_sourced_files_are_unverified(self):
        for command in ("eval 'ha start goal'", "source ./runs-core.sh", ". ./runs-core.sh"):
            for host in ("claude", "codex"):
                with self.subTest(host=host, command=command):
                    self.unverified(host, "spec", (command,))

    def test_split_string_forms_are_not_discarded(self):
        for command in ("env -S 'ha start goal'", "env --split-string 'ha start goal'",
                        "env --split-string='ha start goal'", "env -S'ha start goal'",
                        "env -S 'printf' ha start goal"):
            for host in ("claude", "codex"):
                with self.subTest(host=host, command=command):
                    self.unverified(host, "spec", (command,))

    def test_opaque_execution_is_not_a_proof_of_absence(self):
        commands = (
            'python3 -c \'import subprocess; subprocess.run(["ha", "start", "goal"])\'',
            "python3 ./runs-core.py", "sh ./runs-core.sh", "bash ./runs-core.sh",
            "xargs ha start", "sudo ha start goal", "nohup ha start goal",
            "timeout 5 ha start goal", "find . -exec ha start goal \\;", "./custom-runner")
        for command in commands:
            for host in ("claude", "codex"):
                with self.subTest(host=host, command=command):
                    self.unverified(host, "spec", (command,))


class AC2(Case):
    def test_computed_values_and_partial_literals_are_unverified(self):
        scripts = (
            "const c = 'ha start goal'; await tools.exec_command({cmd: c})",
            'await tools.exec_command({"cmd": buildCommand()})',
            "await tools.exec_command({cmd: `ha start ${goal}`})",
            "await tools.exec_command({cmd: 'echo ' + command})",
            "await tools.exec_command({cmd: 'ha start goal' && 'printf data'})",
            "await tools.exec_command({cmd: 'ha start goal', ...options})",
            "await tools.exec_command(options)")
        for script in scripts:
            with self.subTest(script=script):
                self.unverified_value("codex", self.codex_commands("spec", (script,), True), "spec")

    def test_comments_and_string_examples_do_not_invoke_core(self):
        scripts = (
            "// tools.exec_command({cmd: 'ha start goal'})\ntext('example')",
            "/* tools.exec_command({cmd: 'ha start goal'}) */ text('example')",
            'const example = "tools.exec_command({cmd: \'ha start goal\'})"; text(example)',
            "const example = `tools.exec_command({cmd: 'ha start goal'})`; text(example)")
        for script in scripts:
            with self.subTest(script=script):
                self.accepts("codex", self.codex_commands("spec", (script,), True), "spec")

    def test_unresolved_js_control_flow_is_not_definite_execution(self):
        scripts = (
            "if (flag) { await tools.exec_command({cmd: 'ha start goal'}); }",
            "flag && await tools.exec_command({cmd: 'ha start goal'})",
            "flag ? await tools.exec_command({cmd: 'ha start goal'}) : text('none')",
            "function run() { return tools.exec_command({cmd: 'ha start goal'}); } run()")
        for script in scripts:
            with self.subTest(script=script):
                self.unverified_value("codex", self.codex_commands("spec", (script,), True), "spec")

    def test_all_literal_key_spellings_and_native_json_keep_the_same_verdict(self):
        for key in ("cmd", "'cmd'", '"cmd"'):
            script = f"await tools.exec_command({{{key}: 'ha start goal'}})"
            self.definite_failure("codex", self.codex_commands("spec", (script,), True), "spec", "execution work")
        self.definite_failure("codex", self.codex_commands("spec", ("ha start goal",)), "spec", "execution work")


class AC3(Case):
    def test_known_seal_operations_do_not_hide_uncertain_extra_calls(self):
        for command in ("eval 'ha note goal input'", "env -S 'ha note goal input'", "sh ./runs-core.sh"):
            for host in ("claude", "codex"):
                with self.subTest(host=host, command=command):
                    self.unverified(host, "seal", tuple(SEAL) + (command,))

    def test_computed_js_is_not_hidden_by_other_complete_tool_calls(self):
        scripts = [f"await tools.exec_command({{cmd: {json.dumps(c)}}})" for c in SEAL]
        scripts.append("await tools.exec_command({cmd: nextCommand()})")
        self.unverified_value("codex", self.codex_commands("seal", scripts, True), "seal")

    def test_uncertainty_identifies_the_actual_native_call(self):
        for host, tool in (("claude", "tool-1-0"), ("codex", "tool-codex-0")):
            with self.subTest(host=host):
                self.unverified(host, "spec", ("eval 'ha start goal'",), tool)

    def test_complete_missing_operation_is_not_unknown(self):
        for host in ("claude", "codex"):
            self.definite_candidate_failure(host, self.native(host, "seal", ("ls",)), "does not observe ha start")


class AC10(Case):
    def test_direct_calls_static_bindings_and_wrappers_remain_supported(self):
        commands = (
            'HA=/opt/bin/ha; "$HA" start goal',
            "env TRACE=1 ha check goal --baseline AC-1",
            "command /opt/bin/ha check goal AC-1", "exec ha done goal")
        # exec still exposes its command candidate, but shell replacement is
        # no longer a definite execution fact without an original run.
        analysis = check.shell_analysis(commands[-1])
        self.assertEqual(analysis.calls, [("done", ["goal"])])
        self.assertTrue(analysis.unknown)
        for host in ("claude", "codex"):
            value = self.native(host, "seal", commands)
            with self.assertRaisesRegex(check.Unavailable, "exec replacement"):
                self.checker.trace_candidates(host, value, "seal")
            self.unverified_value(host, value, "seal")
            self.accepts_candidates(host, self.native(host, "seal", commands[:-1] + ("ha done goal",)), "seal")
            self.definite_failure(host, self.native(host, "spec", ("bash -c 'ha start goal'",)),
                                  "spec", "execution work")

    def test_supported_data_commands_and_quoted_heredocs_remain_data(self):
        commands = ("ls", "grep -n 'ha budget' SPEC.md", "cat SPEC.md",
                    "HA=/opt/ha; printf '%s' '$HA start goal'", 'echo "ha start"',
                    "cat <<'EOF'\n$(ha start goal)\nEOF", "# ha start goal\nls")
        for host in ("claude", "codex"):
            for command in commands:
                with self.subTest(host=host, command=command):
                    self.accepts(host, self.native(host, "spec", (command,)), "spec")
        # A commit message is data; whether git itself may launch a hook is a
        # separate supported-execution question, not a Core-text match.
        self.assertEqual(check.core_operations("git commit -m 'ha start goal'", check.SPEC_FORBIDDEN), set())

    def test_substitutions_still_count_and_baseline_arguments_stay_bound(self):
        for host in ("claude", "codex"):
            self.definite_failure(host, self.native(host, "spec", ('printf x > "$(ha start goal)"',)),
                                  "spec", "execution work")
            commands = ("ha start goal", "ha check goal AC-1", "echo --baseline", "ha done goal")
            self.definite_candidate_failure(host, self.native(host, "seal", commands), "baseline attempt")

    def test_original_receipts_still_bind_supported_actual_runs(self):
        script = ('if false; then "$JAEKIT_CORE" note unused; fi; '
                  'f() { "$JAEKIT_CORE" start goal; }; f; '
                  '"$JAEKIT_CORE" check goal --baseline AC-1; "$JAEKIT_CORE" done goal')
        argv, artifact, core, actual, result = self.collected(script)
        self.assertEqual(result.returncode, 0)
        self.assertEqual(actual, [["start", "goal"], ["check", "goal", "--baseline", "AC-1"], ["done", "goal"]])
        for host in ("claude", "codex"):
            value = self.observed_value(host, "seal", argv, artifact, core)
            self.assertEqual(self.collected_arguments(host, value, "seal"), actual)
            self.unverified_value(host, value, "seal")
            wrong = copy.deepcopy(value)
            wrong["seal"]["executions"][0]["tool_call_id"] = "another-native-call"
            with self.assertRaises(check.execution_trace.InvalidObservation):
                self.collected_arguments(host, wrong, "seal")
            self.unverified_value(host, wrong, "seal")
            missing = self.observed_value(host, "seal", argv, artifact, core, receipt="")
            with self.assertRaises(check.execution_trace.InvalidObservation):
                self.collected_arguments(host, missing, "seal")
            self.unverified_value(host, missing, "seal")


if __name__ == "__main__":
    unittest.main()
