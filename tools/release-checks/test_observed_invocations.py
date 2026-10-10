"""Offline cases for invocation evidence: the loaded Claude Skill and Core commands.

A Claude Skill counts as loaded only through the native session transcript:
the slash-command entry and the host's linked `isMeta` entry that names the
installed Skill directory. A Core command counts only in command position;
printed, quoted, commented and here-document text does not run it.
"""
import json
from pathlib import Path
import tempfile
import unittest

import test_loader_invocations as base

check = base.check
SESSION, CORE, GOAL = base.SESSION, base.CORE, base.GOAL
SEAL_COMMANDS = base.SEAL_COMMANDS


class Case(base.TraceCase):
    def claude_spec(self, *commands, transcript=None):
        return self.claude(lambda root: self.bash(*commands), transcript=transcript)[0]

    def claude_seal(self, commands):
        return self.claude(seal_commands=commands)[0]

    def codex_spec(self, *commands):
        def rollout(path):
            return self.session_start() + [self.injection("spec:spec", path)] + [
                {"type": "response_item", "payload": {"type": "function_call", "name": "exec_command",
                                                      "arguments": json.dumps({"cmd": c})}} for c in commands]
        return self.codex(rollout)[0]

    def codex_seal(self, commands):
        return self.codex(lambda path: self.session_start() + [self.injection("spec:spec", path)],
                          seal_commands=commands)[0]


class ClaudeTranscriptLoad(Case):
    """AC-1: the phase's own invocation in the same session's native transcript."""

    def test_whole_session_transcript_loads_each_phase_skill(self):
        value, _ = self.claude()
        self.accepts("claude", value, "spec")
        self.accepts("claude", value, "seal")

    def test_phase_snapshots_load_each_phase_skill(self):
        value, _ = self.claude()
        spec_dir = str(self.root / "claude/plugins/cache/jaekit/spec/0.1.0/skills/spec")
        seal_dir = str(self.root / "claude/plugins/cache/jaekit/seal/0.1.0/skills/seal")
        entries = [json.loads(line) for line in (self.root / value["session_trace"]["path"]).read_text().splitlines()]
        value["spec"]["session_trace"] = self.lines([entry for entry in entries if entry.get("promptId") == "prompt-1"])
        value["seal"]["session_trace"] = self.lines([entry for entry in entries if entry.get("promptId") == "prompt-2"])
        self.accepts("claude", value, "spec")
        self.accepts("claude", value, "seal")

    def test_each_phase_needs_its_own_skill_invocation(self):
        value, _ = self.claude(transcript=lambda spec_dir, seal_dir: self.invocation("seal:seal", seal_dir))
        self.rejects("claude", value, "spec", "loaded Skill")
        self.accepts("claude", value, "seal")
        value, _ = self.claude(transcript=lambda spec_dir, seal_dir: self.invocation("spec:spec", spec_dir))
        self.accepts("claude", value, "spec")
        self.rejects("claude", value, "seal", "loaded Skill")

    def test_queue_and_other_entries_around_the_invocation_are_ignored(self):
        def transcript(spec_dir, seal_dir):
            queue = [{"type": "queue-operation", "operation": "enqueue", "sessionId": SESSION,
                      "content": "/spec:spec synthetic request"},
                     {"type": "queue-operation", "operation": "dequeue", "sessionId": SESSION}]
            reply = {"type": "assistant", "sessionId": SESSION, "uuid": "entry-3", "parentUuid": "entry-1-skill",
                     "message": {"role": "assistant", "content": [{"type": "text", "text": "Writing SPEC.md"}]}}
            return queue + self.invocation("spec:spec", spec_dir) + [reply] + self.invocation(
                "seal:seal", seal_dir, number=2)
        value, _ = self.claude(transcript=transcript)
        self.accepts("claude", value, "spec")
        self.accepts("claude", value, "seal")


class ClaudeNotLoaded(Case):
    """AC-2: listings, requests, other work and foreign records are not a load."""

    def test_init_listing_and_request_argv_alone_are_not_loaded(self):
        value = self.claude_spec("ls", transcript=lambda spec_dir, seal_dir: None)
        self.rejects("claude", value, "spec", "loaded Skill")
        value, _ = self.claude(transcript=lambda spec_dir, seal_dir: None)
        self.rejects("claude", value, "seal", "loaded Skill")

    def test_other_work_without_an_invocation_is_not_loaded(self):
        def transcript(spec_dir, seal_dir):
            prompt = {"type": "user", "userType": "external", "isSidechain": False, "sessionId": SESSION,
                      "uuid": "entry-1", "parentUuid": None,
                      "message": {"role": "user", "content": "/spec:spec synthetic request"}}
            tool = {"type": "user", "isSidechain": False, "sessionId": SESSION, "uuid": "entry-2",
                    "parentUuid": "entry-1", "message": {"role": "user", "content": [
                        {"type": "tool_result", "tool_use_id": "tool-1", "content": spec_dir}]}}
            return [prompt, tool]
        value = self.claude_spec("ls", transcript=transcript)
        self.rejects("claude", value, "spec", "loaded Skill")

    def test_invocation_needs_the_host_injected_skill_entry(self):
        def without_injection(spec_dir, seal_dir):
            return self.invocation("spec:spec", spec_dir)[:1]

        def unlinked(spec_dir, seal_dir):
            entries = self.invocation("spec:spec", spec_dir)
            entries[1]["parentUuid"] = "entry-unrelated"
            return entries

        def not_meta(spec_dir, seal_dir):
            entries = self.invocation("spec:spec", spec_dir)
            del entries[1]["isMeta"]
            return entries

        def injection_first(spec_dir, seal_dir):
            return list(reversed(self.invocation("spec:spec", spec_dir)))

        def sidechain(spec_dir, seal_dir):
            entries = self.invocation("spec:spec", spec_dir)
            for entry in entries:
                entry["isSidechain"] = True
            return entries

        def model_text(spec_dir, seal_dir):
            entries = self.invocation("spec:spec", spec_dir)
            entries[1] = {"type": "assistant", "sessionId": SESSION, "uuid": "entry-1-skill",
                          "parentUuid": "entry-1", "message": {"role": "assistant", "content": [
                              {"type": "text", "text": f"Base directory for this skill: {spec_dir}"}]}}
            return entries
        for transcript in (without_injection, unlinked, not_meta, injection_first, sidechain, model_text):
            with self.subTest(transcript=transcript.__name__):
                value = self.claude_spec("ls", transcript=transcript)
                self.rejects("claude", value, "spec", "loaded Skill")

    def test_other_skill_location_or_session_is_not_loaded(self):
        _, other_skill, _ = self.installed("claude", "spec", marketplace="other")
        other_dir = str(Path(other_skill).parent)

        def other_skill_name(spec_dir, seal_dir):
            return self.invocation("spec:other", spec_dir)

        def other_location(spec_dir, seal_dir):
            return self.invocation("spec:spec", other_dir)

        def other_session(spec_dir, seal_dir):
            return self.invocation("spec:spec", spec_dir, session="session-9999")

        def mixed_session(spec_dir, seal_dir):
            entries = self.invocation("spec:spec", spec_dir)
            entries[0]["sessionId"] = "session-9999"
            return entries
        for transcript in (other_skill_name, other_location, other_session, mixed_session):
            with self.subTest(transcript=transcript.__name__):
                value = self.claude_spec("ls", transcript=transcript)
                self.rejects("claude", value, "spec", "loaded Skill")


class SealCommandPosition(Case):
    """AC-3: printed, quoted, commented and here-document Core text is not run."""

    FORMS = {
        "echo literal": "echo ha {op} " + GOAL,
        "echo absolute": "echo " + CORE + " {op} " + GOAL,
        "echo variable": 'HA=/opt/bin/ha; echo "$HA" {op} ' + GOAL,
        "printf": "printf '%s\\n' 'ha {op} " + GOAL + "'",
        "printf variable": "HA=" + CORE + "; printf '%s' '$HA {op} " + GOAL + "'",
        "commit message": 'git commit -m "ha {op} ' + GOAL + '"',
        "assigned string": 'HA=' + CORE + '; NEXT="$HA {op} ' + GOAL + '"',
        "python string": "python3 -c 'print(\"ha {op} " + GOAL + "\")'",
        "comment": "ls # ha {op} " + GOAL,
        "comment line": "# \"$HA\" {op} " + GOAL + "\nls",
        "here-document": "cat > notes.md <<'EOF'\nha {op} " + GOAL + "\nEOF",
        "here-document variable": "HA=" + CORE + "\ncat <<EOF\n$HA {op} " + GOAL + "\nEOF",
        "indented here-document": "cat <<-EOF\n\tha {op} " + GOAL + "\n\tEOF",
    }
    REAL = {"start": f"ha start {GOAL} --request q", "check": f"ha check {GOAL} AC-1",
            "done": f"ha done {GOAL}"}

    def test_required_operations_in_non_command_text_are_not_observed(self):
        for operation in ("start", "check", "done"):
            for label, form in self.FORMS.items():
                with self.subTest(operation=operation, form=label):
                    commands = [self.REAL[op] for op in ("start", "check", "done") if op != operation]
                    if operation != "check":
                        commands.append(f"ha check {GOAL} --baseline AC-1")
                    printed = form.replace("{op}", operation + (" --baseline" if operation == "check" else ""))
                    self.assertEqual(check.core_operations(printed, check.SEAL_REQUIRED), set())
                    self.rejects("codex", self.codex_seal(commands + [printed]), "seal",
                                 f"does not observe ha {operation}")

    def test_printed_baseline_flag_is_not_a_baseline_attempt(self):
        for form in (f"ha check {GOAL} AC-1; echo --baseline", f'echo "ha check {GOAL} --baseline AC-1"'):
            with self.subTest(form=form):
                commands = [self.REAL["start"], f"ha check {GOAL} AC-1", form, self.REAL["done"]]
                self.rejects("codex", self.codex_seal(commands), "seal", "baseline attempt")

    def test_claude_printed_variable_start_is_not_observed(self):
        commands = [f'HA=/opt/bin/ha; echo "$HA" start {GOAL}'] + SEAL_COMMANDS[1:]
        self.rejects("claude", self.claude_seal(commands), "seal", "does not observe ha start")


class SpecCommandPosition(Case):
    """AC-4: a document-only Spec may print or quote Core commands."""

    def test_printed_or_quoted_core_commands_are_not_execution(self):
        for command in ("HA=/opt/ha; printf '%s' '$HA start goal'", 'echo "ha start"',
                        f"echo ha check {GOAL} --baseline AC-1", f'HA={CORE}; echo "$HA" done {GOAL}',
                        f"printf '%s\\n' \"{CORE} note {GOAL} input\"",
                        f"grep -n 'ha budget' {GOAL}/SPEC.md", f'git commit -m "ha start {GOAL}"',
                        f"cat > {GOAL}/SPEC.md <<'EOF'\nRun `ha start {GOAL}` later.\nEOF",
                        f"# ha done {GOAL}\nls", f"zsh -lc 'echo ha start {GOAL}'"):
            with self.subTest(command=command):
                self.assertEqual(check.core_operations(command, check.SPEC_FORBIDDEN), set())
                self.accepts("codex", self.codex_spec(command), "spec")

    def test_claude_printed_examples_are_not_execution(self):
        for command in ("HA=/opt/ha; printf '%s' '$HA start goal'", 'echo "ha start"'):
            with self.subTest(command=command):
                self.accepts("claude", self.claude_spec(command), "spec")

    def test_core_commands_in_command_position_remain_execution(self):
        for command in (f"ha start {GOAL}", f"{CORE} check {GOAL} AC-1", f'HA={CORE}; "$HA" done {GOAL}',
                        f"echo $(ha note {GOAL} input)", f'out="$({CORE} budget {GOAL} --runs 5)"',
                        f"echo `ha start {GOAL}`", f"zsh -lc 'ha check {GOAL} AC-1'",
                        f'bash -c "HA={CORE}; \\$HA start {GOAL}"', f"ls; ha done {GOAL}"):
            with self.subTest(command=command):
                self.rejects("codex", self.codex_spec(command), "spec", "Spec observation performed execution work")

    def test_claude_core_commands_in_command_position_remain_execution(self):
        for command in (f'HA={CORE}; "$HA" start {GOAL}', f"echo $(ha note {GOAL} input)",
                        f"zsh -lc 'ha check {GOAL} AC-1'"):
            with self.subTest(command=command):
                self.rejects("claude", self.claude_spec(command), "spec", "Spec observation performed execution work")

    def test_codex_exec_strings_are_read_as_commands(self):
        def rollout(code):
            def build(path):
                return self.session_start() + [self.injection("spec:spec", path), {
                    "type": "response_item", "payload": {"type": "custom_tool_call", "name": "exec", "input": code}}]
            return build
        printed = f'exec_command({{cmd: "echo \\"ha start {GOAL}\\""}})'
        value, _ = self.codex(rollout(printed))
        self.accepts("codex", value, "spec")
        executed = f"text(await tools.exec_command({{workdir: \"/repo\", cmd: 'ha note {GOAL} input'}}));"
        value, _ = self.codex(rollout(executed))
        self.rejects("codex", value, "spec", "Spec observation performed execution work")


class KeptRecognition(Case):
    """AC-5: command-position forms the earlier checker recognized stay recognized."""

    def test_wrappers_prefixes_and_env_command_exec_are_observed(self):
        commands = [f'bash -c "HA={CORE}; \\"\\$HA\\" start {GOAL} --request q"',
                    f"GOAL_DIR={GOAL} env HA_TRACE=1 ha check {GOAL} --baseline AC-1",
                    f"command {CORE} check {GOAL} AC-1", f"cd repo; exec ha done {GOAL}"]
        self.accepts("claude", self.claude_seal(commands), "seal")

    def test_command_lookup_is_not_execution(self):
        for command in ("command -v ha", "command -V ha", "type ha", "which ha"):
            with self.subTest(command=command):
                self.accepts("claude", self.claude_spec(command), "spec")

    def test_invocation_arguments_are_reported(self):
        self.assertEqual(check.core_invocations(f'HA={CORE}; "$HA" check {GOAL} --baseline AC-1'),
                         [("check", [GOAL, "--baseline", "AC-1"])])
        self.assertEqual(check.core_operations(f"ha start {GOAL}; ha done {GOAL}", check.SEAL_REQUIRED),
                         {"start", "done"})


class Documentation(unittest.TestCase):
    """AC-10: the README explains transcript evidence, command position and limits."""

    def test_readme_explains_transcripts_command_position_and_limits(self):
        text = Path(__file__).with_name("README.md").read_text(encoding="utf-8")
        for phrase in ("native session transcript", "`<command-name>/spec:spec</command-name>`",
                       "`isMeta`", "`parentUuid`", "Base directory for this skill:", "`sessionId`",
                       "command position", "`printf`", "here-document", "comment", "`env`", "`command`",
                       "`exec`", "`bash -c`", "`exec_command`", "not interpreted", "`sudo`", "`xargs`",
                       "`--evidence DIR`"):
            with self.subTest(phrase=phrase):
                self.assertIn(phrase, text)


class EvidenceLocation(unittest.TestCase):
    """AC-8: the checker reads only the evidence directory it is given."""

    def unverified(self, action):
        try:
            action()
        except check.Unavailable as error:
            return str(error)
        except Exception as error:
            self.fail(f"expected an unverified result, got {type(error).__name__}")
        self.fail("evidence was located without an evidence directory")

    def test_without_an_evidence_directory_nothing_is_located(self):
        checker = check.Checker()
        self.assertIn("--evidence", self.unverified(checker.evidence))
        self.assertIn("--evidence", self.unverified(
            lambda: checker.artifact({"path": "evidence.json", "sha256": "0" * 64})))

    def test_given_directory_is_the_only_evidence_root(self):
        with tempfile.TemporaryDirectory() as directory:
            index = {"schema": "jaekit-release-evidence/v1", "release": {"tag": check.TAG}}
            (Path(directory) / "evidence.json").write_text(json.dumps(index))
            try:
                checker = check.Checker(directory)
            except TypeError:
                self.fail("the checker does not take an evidence directory")
            self.assertEqual(checker.evidence(), index)

    def test_source_does_not_search_the_git_directory(self):
        source = Path(__file__).with_name("check.py").read_text(encoding="utf-8")
        self.assertNotIn("--git-" + "common-dir", source)
        self.assertNotIn('"ha" / "release', source)


if __name__ == "__main__":
    unittest.main()
