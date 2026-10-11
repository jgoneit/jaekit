"""Read-only consumer of bounded original-run execution observations."""
import hashlib
import json
from pathlib import Path
import shlex
import re

from shell_reader import covered
import execution_provenance


class InvalidObservation(Exception):
    pass


def need(condition, reason):
    if not condition:
        raise InvalidObservation(reason)


def digest(data):
    return hashlib.sha256(data).hexdigest()


def launcher(command):
    try:
        argv = shlex.split(command)
    except ValueError:
        return None
    return argv if len(argv) > 2 and Path(argv[1]).name == "observe.py" and argv[2] == "run" else None


def output_texts(value):
    if isinstance(value, str):
        try:
            decoded = json.loads(value)
        except ValueError:
            return [value]
        return output_texts(decoded)
    if isinstance(value, dict):
        return [text for child in value.values() for text in output_texts(child)]
    if isinstance(value, list):
        return [text for child in value for text in output_texts(child)]
    return []


def native_results(events):
    results = {}
    def add(call, content):
        if isinstance(call, str):
            results.setdefault(call, []).extend(output_texts(content))
    for event in events:
        payload = event.get("payload", {})
        if event.get("type") == "response_item" and payload.get("type") in ("function_call_output", "custom_tool_call_output"):
            add(payload.get("call_id"), payload.get("output"))
        item = event.get("item", {})
        if event.get("type") == "item.completed" and item.get("type") in ("command_execution", "mcp_tool_call"):
            add(item.get("id"), item.get("aggregated_output", item.get("result")))
        message = event.get("message", {})
        content = message.get("content") if isinstance(message, dict) else None
        if event.get("type") == "user":
            for block in content if isinstance(content, list) else []:
                if isinstance(block, dict) and block.get("type") == "tool_result":
                    add(block.get("tool_use_id"), block.get("content"))
    return results


def consume(checker, value, capture, calls, events, phase="spec"):
    """Map native command strings to verified executed Core invocations.

    All association is through native tool-call IDs and the exact collector
    argv, including a unique invocation. The capture itself supplies the
    session/request boundary; this sidecar cannot choose a different turn.
    """
    entries = capture.get("executions", [])
    need(isinstance(entries, list), "execution observation index is invalid")
    result, used = {}, set()
    outputs = native_results(events)
    expected = original = chain = None
    if phase == "seal" and entries:
        expected = execution_provenance.context(value.get("project", ""), value.get("goal", ""))
        original = Path(expected["records"]).read_bytes()
        chain = execution_provenance.records(original)
    for entry in entries:
        need(isinstance(entry, dict) and isinstance(entry.get("tool_call_id"), str), "native tool-call binding is missing")
        _, raw = checker.artifact(entry.get("report"))
        try:
            report = json.loads(raw)
        except (ValueError, UnicodeDecodeError):
            raise InvalidObservation("execution report is incomplete or malformed") from None
        need(isinstance(report, dict) and report.get("schema") == "jaekit-execution/v2", "unsupported execution report schema")
        argv = report.get("launcher_argv")
        need(isinstance(argv, list) and all(isinstance(a, str) for a in argv), "execution launcher argv is missing")
        matching = [(call_id, command) for call_id, command in calls if launcher(command) == argv]
        need(len(matching) == 1 and matching[0][0] == entry["tool_call_id"], "execution belongs to a different or ambiguous native tool call")
        command = matching[0][1]
        need(command not in result, "duplicate execution observation")
        try:
            separator = argv.index("--")
            options = argv[3:separator]
            need(len(options) in (8, 12) and len(argv) == separator + 2, "unsupported collector invocation")
            need(len(set(options[::2])) == len(options[::2]), "duplicate collector options")
            options = dict(zip(options[::2], options[1::2]))
            base_options = {"--core", "--shell", "--output", "--invocation"}
            need(set(options) in (base_options, base_options | {"--project", "--goal"}), "unsupported collector options")
        except (ValueError, IndexError):
            raise InvalidObservation("invalid collector invocation") from None
        invocation = report.get("invocation")
        need(isinstance(invocation, str) and invocation and invocation == options["--invocation"] and invocation not in used,
             "execution invocation binding is missing or conflicts")
        used.add(invocation)
        run_id = report.get("run_id")
        need(isinstance(run_id, str) and re.fullmatch(r"[0-9a-f]{32}", run_id), "execution run identity is missing")
        expected_receipt = {"schema": "jaekit-execution-receipt/v1", "invocation": invocation,
                            "run_id": run_id, "sha256": digest(raw)}
        receipts = []
        for text in outputs.get(entry["tool_call_id"], []):
            for line in text.splitlines():
                if line.startswith("JAEKIT_EXECUTION_RECEIPT "):
                    try:
                        receipt = json.loads(line[len("JAEKIT_EXECUTION_RECEIPT "):])
                    except ValueError:
                        raise InvalidObservation("native execution receipt is malformed") from None
                    if receipt not in receipts:
                        receipts.append(receipt)
        need(receipts == [expected_receipt], "native tool result does not corroborate this exact execution report")
        script = argv[-1]
        need(report.get("script_sha256") == digest(script.encode()), "observed script digest differs")
        need(report.get("coverage") == "bounded-launcher/v1" and report.get("supported") is True and covered(script),
             "execution is outside the supported coverage or bypasses the Core launcher")
        need(report.get("identity_unchanged") is True, "execution identities changed")
        producer = report.get("producer")
        need(isinstance(producer, dict), "execution producer identity is missing")
        producer_path, producer_data = checker.artifact(producer)
        need(str(producer_path.resolve()) == str(Path(argv[1]).resolve())
             and producer_data == Path(__file__).with_name("observe.py").read_bytes(), "unsupported execution producer")
        _, reader_data = checker.artifact(report.get("reader"))
        need(reader_data == Path(__file__).with_name("shell_reader.py").read_bytes(), "unsupported collection coverage policy")
        _, provenance_data = checker.artifact(report.get("provenance"))
        need(provenance_data == Path(__file__).with_name("execution_provenance.py").read_bytes(),
             "unsupported record provenance policy")
        if phase == "seal":
            need(report.get("context") == expected and options.get("--project") == expected["project"]
                 and options.get("--goal") == expected["goal"], "execution project or goal context differs")
        core_path, core_bytes = checker.artifact(value.get("core"))
        core = {"path": str(core_path.resolve()), "sha256": digest(core_bytes)}
        need(report.get("core") == core and str(Path(options["--core"]).resolve()) == core["path"], "executed Core identity differs")
        executable = report.get("execution_image")
        need(isinstance(executable, dict) and set(executable) == {"path", "sha256"}
             and isinstance(executable.get("path"), str) and Path(executable["path"]).is_absolute()
             and executable["sha256"] == core["sha256"], "isolated executed Core bytes differ")
        shell_path, _ = checker.artifact(report.get("shell"))
        need(str(shell_path.resolve()) == str(Path(options["--shell"]).resolve()), "observed shell identity differs")
        events = report.get("events")
        need(isinstance(events, list) and len(events) >= 2, "execution event sequence is incomplete")
        previous, active, finished, invocations = None, {}, set(), []
        for index, event in enumerate(events, 1):
            need(isinstance(event, dict) and event.get("seq") == index and event.get("prev") == previous,
                 "execution event sequence is missing, reordered or corrupt")
            previous = digest(json.dumps(event, sort_keys=True, separators=(",", ":")).encode())
            kind = event.get("kind")
            if index == 1:
                need(kind == "shell_start" and event.get("invocation") == invocation
                     and event.get("script_sha256") == report["script_sha256"], "execution start binding differs")
            elif index == len(events):
                need(kind == "shell_exit" and type(event.get("returncode")) is int and not active,
                     "execution completion is missing or a Core child is unfinished")
            elif kind == "core_start":
                call, arguments = event.get("call"), event.get("argv")
                need(isinstance(call, str) and call and call not in active and call not in finished,
                     "Core invocation is duplicated")
                need(event.get("executable") == executable and type(event.get("pid")) is int
                     and isinstance(arguments, list) and len(arguments) >= 2
                     and all(isinstance(a, str) for a in arguments) and arguments[0] == executable["path"],
                     "actual Core start/argv identity is invalid")
                active[call] = event
                invocations.append((arguments[1], arguments[2:]))
            elif kind == "core_exit":
                call = event.get("call")
                need(isinstance(call, str) and call in active and event.get("pid") == active[call]["pid"] and type(event.get("returncode")) is int,
                     "Core completion has no matching start")
                if phase == "seal":
                    execution_provenance.record_delta(active[call], event, original, chain, expected)
                del active[call]
                finished.add(call)
            else:
                raise InvalidObservation("execution is incomplete or a process failed to start")
        result[command] = invocations
    for _, command in calls:
        need(launcher(command) is None or command in result, "original execution observation is missing")
    return result
