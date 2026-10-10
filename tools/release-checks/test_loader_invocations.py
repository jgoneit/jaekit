"""Offline cases for loaded-Skill identification and Core invocation forms.

Synthetic traces follow the native event shapes. A Codex rollout lists every
available Skill in its session metadata and injects an invoked Skill as a
separate `<skill>` message. Claude stream-json starts each invocation with a
`system`/`init` event listing every available plugin and Skill; its native
session transcript records a slash-command invocation and the host's injected
Skill directory as two linked entries.
"""
import importlib.util
import json
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location("release_acceptance_loader", Path(__file__).with_name("check.py"))
check = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(check)

SESSION = "session-1234"
MODEL = "observed-model"
CORE = "/opt/jaekit/bin/ha"
GOAL = "docs/specs/greeting"
SEAL_COMMANDS = [f"ha start {GOAL} --request q", f"ha check {GOAL} --baseline AC-1",
                 f"ha check {GOAL} AC-1", f"ha done {GOAL}"]


class TraceCase(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory()
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name).resolve()
        self.checker = object.__new__(check.Checker)
        self.checker.private = self.root
        self.checker._evidence = None
        self.checker._release = None
        self.checker._downloads = {}
        self.checker._source = {}
        self.count = 0

    def write(self, relative, data):
        if isinstance(data, str):
            data = data.encode()
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_bytes(data)
        return {"path": relative, "sha256": check.digest(data)}

    def lines(self, events):
        self.count += 1
        return self.write(f"capture-{self.count}.jsonl", "".join(json.dumps(e) + "\n" for e in events))

    def capture(self, argv, events, rollout=None):
        value = {"argv": argv, "exit_code": 0, "stdout": self.lines(events),
                 "stderr": self.write(f"stderr-{self.count}", "")}
        if rollout is not None:
            value["session_trace"] = self.lines(rollout)
        return value

    def installed(self, host, plugin, marketplace="jaekit"):
        root = f"{host}/plugins/cache/{marketplace}/{plugin}/0.1.0"
        artifact = self.write(f"{root}/skills/{plugin}/SKILL.md", f"synthetic {plugin} skill\n")
        return artifact, str(self.root / root / "skills" / plugin / "SKILL.md"), str(self.root / root)

    def accepts(self, host, value, phase):
        try:
            return self.checker.trace(host, value, phase)
        except check.Failure as error:
            self.fail(f"{host} {phase} trace was rejected: {error}")

    def rejects(self, host, value, phase, message):
        with self.assertRaisesRegex((check.Failure, check.Unavailable), message):
            self.checker.trace(host, value, phase)

    # Codex shapes -----------------------------------------------------------

    @staticmethod
    def listing(*paths):
        text = "## Skills\n" + "".join(f"- skill: synthetic (file: {p})\n" for p in paths)
        return {"type": "response_item", "payload": {"type": "message", "role": "developer",
                                                      "content": [{"type": "input_text", "text": text}]}}

    @staticmethod
    def message(role, text):
        kind = "output_text" if role == "assistant" else "input_text"
        return {"type": "response_item", "payload": {"type": "message", "role": role,
                                                      "content": [{"type": kind, "text": text}]}}

    def injection(self, skill, path):
        return self.message("user", f"<skill>\n<name>{skill}</name>\n<path>{path}</path>\n---\nname: synthetic\n")

    @staticmethod
    def session_start():
        return [{"type": "session_meta", "payload": {"id": SESSION}},
                {"type": "turn_context", "payload": {"model": MODEL}}]

    @staticmethod
    def codex_stdout(*commands):
        events = [{"type": "thread.started", "thread_id": SESSION}]
        events += [{"type": "item.completed", "item": {"type": "command_execution", "command": c}}
                   for c in commands or ("ls",)]
        return events + [{"type": "turn.completed"}]

    def codex(self, spec_rollout, seal_rollout=None, seal_commands=SEAL_COMMANDS, shared=None):
        spec, spec_path, _ = self.installed("codex", "spec")
        seal, seal_path, _ = self.installed("codex", "seal")
        value = {"model": MODEL, "session_id": SESSION, "plugins": {"spec": spec, "seal": seal}}
        value["spec"] = self.capture(["codex", "exec", "--json", "$spec:spec a synthetic goal"],
                                     self.codex_stdout(), spec_rollout(spec_path) if spec_rollout else None)
        if seal_rollout is None:
            seal_rollout = lambda path: self.session_start() + [self.injection("seal:seal", path)]
        value["seal"] = self.capture(["codex", "exec", "resume", "--json", SESSION, "$seal:seal " + GOAL],
                                     self.codex_stdout(*seal_commands), seal_rollout(seal_path))
        if shared is not None:
            value["session_trace"] = self.lines(shared(spec_path, seal_path))
        return value, spec_path

    # Claude shapes ----------------------------------------------------------

    def claude_init(self, plugins):
        return {"type": "system", "subtype": "init", "session_id": SESSION, "model": MODEL,
                "plugins": plugins, "skills": ["spec:spec", "seal:seal"]}

    @staticmethod
    def bash(*commands):
        return [{"type": "assistant", "message": {"content": [
            {"type": "tool_use", "name": "Bash", "input": {"command": c}} for c in commands]}}]

    @staticmethod
    def invocation(skill, directory, session=SESSION, number=1):
        """A transcript's slash-command entry and the host's injected Skill entry."""
        uuid = f"entry-{number}"
        return [{"type": "user", "userType": "external", "isSidechain": False, "sessionId": session,
                 "uuid": uuid, "parentUuid": None, "promptId": f"prompt-{number}",
                 "message": {"role": "user", "content": f"<command-message>{skill}</command-message>\n"
                                                         f"<command-name>/{skill}</command-name>\n"
                                                         "<command-args>synthetic request</command-args>"}},
                {"type": "user", "userType": "external", "isSidechain": False, "isMeta": True,
                 "sessionId": session, "uuid": f"{uuid}-skill", "parentUuid": uuid, "promptId": f"prompt-{number}",
                 "message": {"role": "user", "content": [
                     {"type": "text", "text": f"Base directory for this skill: {directory}\n\n# Synthetic\n"}]}}]

    def claude(self, spec_events=None, seal_commands=SEAL_COMMANDS, plugins=None, transcript=None):
        """A Claude observation; `transcript(spec_dir, seal_dir)` gives its session entries.

        By default the transcript records both invocations. Pass a function
        returning None to omit the transcript.
        """
        spec, spec_skill, spec_root = self.installed("claude", "spec")
        seal, seal_skill, seal_root = self.installed("claude", "seal")
        if plugins is None:
            plugins = [{"name": "spec", "path": spec_root, "source": "spec@jaekit", "version": "0.1.0"},
                       {"name": "seal", "path": seal_root, "source": "seal@jaekit", "version": "0.1.0"}]
        done = [{"type": "result", "subtype": "success", "is_error": False}]
        init = self.claude_init(plugins)
        value = {"model": MODEL, "session_id": SESSION, "plugins": {"spec": spec, "seal": seal}}
        body = spec_events(spec_root) if spec_events else self.bash("ls")
        value["spec"] = self.capture(["claude", "-p", "--verbose", "--output-format", "stream-json",
                                      "/spec:spec a synthetic goal"], [init] + body + done)
        value["seal"] = self.capture(["claude", "-p", "--verbose", "--output-format", "stream-json",
                                      "--resume", SESSION, "/seal:seal " + GOAL],
                                     [init] + self.bash(*seal_commands) + done)
        if transcript is None:
            transcript = lambda spec_dir, seal_dir: (self.invocation("spec:spec", spec_dir)
                                                     + self.invocation("seal:seal", seal_dir, number=2))
        entries = transcript(str(Path(spec_skill).parent), str(Path(seal_skill).parent))
        # Native stdout and transcript share message UUIDs. Keep these fixture
        # anchors separate from the slash-command/isMeta pair being tested.
        for phase in ("spec", "seal"):
            events = [json.loads(line) for line in (self.root / value[phase]["stdout"]["path"]).read_text().splitlines()]
            requests = [entry for entry in entries or [] if entry.get("type") == "user"
                        and entry.get("sessionId") == SESSION and entry.get("isMeta") is not True
                        and f"<command-name>/{phase}:{phase}</command-name>" in str(entry.get("message", {}).get("content"))]
            request = requests[0] if requests else {"type": "user", "sessionId": SESSION,
                "uuid": f"request-{phase}", "promptId": f"prompt-{phase}",
                "message": {"role": "user", "content": "synthetic request without a loaded Skill"}}
            if entries is not None and not requests:
                entries.append(request)
            for index, event in enumerate(events):
                if event.get("type") in ("assistant", "user"):
                    event["uuid"] = f"capture-{phase}-{index}"
                    if entries is not None:
                        entries.append(dict(event, sessionId=SESSION, promptId=request.get("promptId"),
                                            parentUuid=request["uuid"]))
            value[phase]["stdout"] = self.lines(events)
        if entries is not None:
            value["session_trace"] = self.lines(entries)
        return value, spec_root


class LoaderEvidence(TraceCase):
    """A loaded Skill comes from the phase's own native loader metadata."""

    def test_codex_injected_skill_is_loaded_in_both_phases(self):
        value, _ = self.codex(lambda path: self.session_start() + [self.listing(path), self.injection("spec:spec", path)])
        self.accepts("codex", value, "spec")
        self.accepts("codex", value, "seal")

    def test_claude_session_plugin_is_loaded_in_both_phases(self):
        value, _ = self.claude()
        self.accepts("claude", value, "spec")
        self.accepts("claude", value, "seal")

    def test_codex_available_listing_is_not_loaded(self):
        value, _ = self.codex(lambda path: self.session_start() + [self.listing(path)])
        self.rejects("codex", value, "spec", "loaded Skill")

    def test_codex_request_answer_and_tool_mentions_are_not_loaded(self):
        def rollout(path):
            return self.session_start() + [
                self.message("user", f"$spec:spec read {path}"),
                self.message("assistant", f"I will follow {path}"),
                {"type": "response_item", "payload": {"type": "function_call", "name": "exec_command",
                                                      "arguments": json.dumps({"cmd": f"cat {path}"})}},
                {"type": "response_item", "payload": {"type": "function_call_output", "output": f"{path}\n"}},
                self.message("user", f"Note: <skill>\n<name>spec:spec</name>\n<path>{path}</path>")]
        value, _ = self.codex(rollout)
        self.rejects("codex", value, "spec", "loaded Skill")

    def test_codex_later_phase_events_do_not_load_the_spec_skill(self):
        def shared(spec_path, seal_path):
            return self.session_start() + [self.injection("seal:seal", seal_path),
                                           self.injection("spec:spec", spec_path)]
        value, _ = self.codex(None, shared=shared)
        self.rejects("codex", value, "spec", "loaded Skill")

    def test_codex_same_name_from_another_location_is_not_loaded(self):
        _, other, _ = self.installed("codex", "spec", marketplace="other")
        value, _ = self.codex(lambda path: self.session_start() + [self.listing(path, other),
                                                                     self.injection("spec:spec", other)])
        self.rejects("codex", value, "spec", "loaded Skill")

    def test_claude_same_name_from_another_marketplace_is_not_loaded(self):
        _, other_skill, other = self.installed("claude", "spec", marketplace="other")

        def spec_events(root):
            return self.bash("ls") + [{"type": "assistant", "message": {"content": [
                {"type": "text", "text": f"Skill base directory: {root}"}]}}]
        value, _ = self.claude(spec_events, plugins=[
            {"name": "spec", "path": other, "source": "spec@other", "version": "9.9.9"}],
            transcript=lambda spec_dir, seal_dir: self.invocation("spec:spec", str(Path(other_skill).parent)))
        self.rejects("claude", value, "spec", "loaded Skill")

    def test_claude_tool_output_mention_is_not_loaded(self):
        def spec_events(root):
            return self.bash(f"ls {root}") + [{"type": "user", "message": {"content": [
                {"type": "tool_result", "content": f"{root}/skills/spec/SKILL.md"}]}}]

        def transcript(spec_dir, seal_dir):
            return [{"type": "user", "sessionId": SESSION, "uuid": "entry-1", "message": {"role": "user", "content": [
                {"type": "tool_result", "content": f"Base directory for this skill: {spec_dir}"}]}}]
        value, _ = self.claude(spec_events, plugins=[], transcript=transcript)
        self.rejects("claude", value, "spec", "loaded Skill")


class CoreInvocations(TraceCase):
    """Core commands observed literally or through a statically assigned variable."""

    def test_claude_static_variable_forms_are_observed(self):
        forms = {
            "semicolon": [f"HA={CORE}; $HA start {GOAL} --request q",
                                 f'HA={CORE}; "$HA" check {GOAL} --baseline AC-1',
                                 f'HA={CORE}; "$HA" check {GOAL} AC-1', f"HA={CORE}; $HA done {GOAL}"],
            "quoted values": [f"HA='{CORE}'; \"$HA\" start {GOAL}", f'HA="{CORE}"; $HA check {GOAL} --baseline AC-1',
                              f"HA='{CORE}'; $HA check {GOAL} AC-1", f'HA="{CORE}"; "$HA" done {GOAL}'],
            "export, newline and braces": [f"export HA={CORE}\n${{HA}} start {GOAL}",
                                           f'export HA={CORE}\n"${{HA}}" check {GOAL} --baseline AC-1',
                                           f"export HA={CORE}\n${{HA}} check {GOAL} AC-1",
                                           f'export HA={CORE}\n"${{HA}}" done {GOAL}'],
        }
        for label, commands in forms.items():
            with self.subTest(form=label):
                value, _ = self.claude(seal_commands=commands)
                self.accepts("claude", value, "seal")

    def test_codex_static_variable_in_shell_and_exec_payloads_is_observed(self):
        def seal_rollout(path):
            code = f'exec_command({{cmd: "HA={CORE}; \\"$HA\\" done {GOAL}"}})'
            return self.session_start() + [self.injection("seal:seal", path),
                                           {"type": "response_item", "payload": {"type": "custom_tool_call",
                                                                                 "name": "exec", "input": code}}]
        commands = [f"/bin/zsh -lc 'HA={CORE}; \"$HA\" start {GOAL} --request q'",
                    f"/bin/zsh -lc 'HA={CORE}; \"$HA\" check {GOAL} --baseline AC-1'",
                    f"/bin/zsh -lc 'HA={CORE}; $HA check {GOAL} AC-1'"]
        value, _ = self.codex(lambda path: self.session_start() + [self.injection("spec:spec", path)],
                              seal_rollout, seal_commands=commands)
        self.accepts("codex", value, "seal")

    def test_spec_phase_variable_execution_is_rejected(self):
        for command in (f"HA={CORE}; $HA start {GOAL} --request q", f'export HA="{CORE}"\n"${{HA}}" note {GOAL} input'):
            with self.subTest(command=command):
                value, _ = self.claude(lambda root: self.bash(command))
                self.rejects("claude", value, "spec", "Spec observation performed execution work")

    def unobserved_start(self, start):
        """Seal traces whose start is only the given form are rejected."""
        value, _ = self.claude(seal_commands=list(start) + SEAL_COMMANDS[1:])
        self.rejects("claude", value, "seal", "does not observe ha start|unverified")

    def test_unassigned_variables_are_not_core(self):
        for form in (f"$ha start {GOAL}", f"$HA start {GOAL}", f'"${{ha}}" start {GOAL}'):
            with self.subTest(form=form):
                self.unobserved_start([form])

    def test_dynamic_values_are_not_core(self):
        for form in (f"ha=$(command -v ha); $ha start {GOAL}", f"HA=$(command -v ha) && $HA start {GOAL}",
                     f"ha=`which ha`; $ha start {GOAL}", f"HA={CORE}; HA=$(command -v ha); $HA start {GOAL}"):
            with self.subTest(form=form):
                self.unobserved_start([form])

    def test_assignment_in_another_call_is_not_core(self):
        self.unobserved_start([f"HA={CORE}", f"$HA start {GOAL}"])

    def test_variable_naming_another_program_is_not_core(self):
        for form in (f"HA=/usr/bin/env; $HA start {GOAL}", f"HA={CORE}; HA=/usr/bin/env; $HA start {GOAL}",
                     f"$HA start {GOAL}; HA={CORE}"):
            with self.subTest(form=form):
                self.unobserved_start([form])

    def test_literal_and_absolute_invocations_remain_observed(self):
        value, _ = self.claude(seal_commands=[f"ha start {GOAL} --request q",
                                              f"/opt/homebrew/bin/ha check {GOAL} --baseline AC-1",
                                              f'"{CORE}" check {GOAL} AC-1', f"cd repo; ha done {GOAL}"])
        self.accepts("claude", value, "seal")

    def test_spec_phase_literal_execution_remains_rejected(self):
        for command in (f"ha note {GOAL} input", f"{CORE} budget {GOAL} --runs 5"):
            with self.subTest(command=command):
                value, _ = self.claude(lambda root: self.bash(command))
                self.rejects("claude", value, "spec", "Spec observation performed execution work")


class Documentation(unittest.TestCase):
    def test_readme_names_accepted_and_rejected_forms(self):
        text = (Path(__file__).with_name("README.md")).read_text(encoding="utf-8")
        for phrase in ("`<skill>`", "`system`/`init`", "available-Skill listing", "statically assigned",
                       "`${VAR}`", "command substitution", "another tool call", "never evaluated"):
            self.assertIn(phrase, text)

    def test_checker_does_not_evaluate_observed_commands(self):
        source = Path(__file__).with_name("check.py").read_text(encoding="utf-8")
        for forbidden in ("eval(", "shell=True", "os.system(", "exec(compile"):
            self.assertNotIn(forbidden, source)


if __name__ == "__main__":
    unittest.main()
