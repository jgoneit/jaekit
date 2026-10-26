#!/usr/bin/env python3
"""Check that a round written by the real reviewer CLIs ran each reviewer
whose model the agent chose on that model, with a spar built from the
sources in this tree.

Usage: real_round.py ROUND_FOLDER
       real_round.py --source-digest

Exit 0 when the property holds, 1 when it does not, 2 when it cannot be
observed: the input cannot be read (missing round.json or source files, no
output kept for a started claude or agy attempt, JSON that does not parse,
including an output line that starts with "{", another record format, a
field of the wrong type, no config.choices, a chosen reviewer with no
state, preset or config.reviewers entry, more than a first and a fallback
attempt, a completed reviewer whose last attempt never started, bad
arguments), the agent chose no model for one of claude, codex and agy or
chose one for a reviewer of another preset, a chosen reviewer's CLI
process never started (its first attempt records no exit_code, which spar
records for every process it started, or no argv), or no chosen reviewer
completed. A property that fails for a reviewer whose CLI started is a
violation even when another reviewer's CLI did not start. The
observed_model of each attempt, and the reviewer's for its last attempt,
completed or not, must be the model its CLI named in the output the round
kept for that attempt (stdout.jsonl for the first, fallback/stdout.jsonl
for the fallback): claude in its init event, agy in an event's init
object, codex never; none named, or a process that never started, means
none recorded. Whether the executable is the real CLI is not observed,
whether spar found it on PATH or in a binary key: a pass prints each
executable path and cli_version. The round is the one the condition names;
besides it only the spar sources of this tree are read.

--source-digest prints the digest a spar built for the round must carry in
its version (main.version=<version>+src.<digest>): the first twelve hex
digits of the sha256 over cmd/spar and internal/spar's Go files outside
tests, go.mod and go.sum, each as its path, a NUL, its bytes and a NUL.
"""

import hashlib
import json
import sys
from pathlib import Path

PRESETS = ("claude", "codex", "agy")
MODEL_FLAG = {"claude": "--model", "codex": "-m", "agy": "--model"}
ROOT = Path(__file__).resolve().parents[3]


class InputError(Exception):
    pass


def read_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except OSError as error:
        raise InputError(f"{path}: {error}") from error
    except ValueError as error:
        raise InputError(f"{path}: not JSON: {error}") from error


def typed(value, kind, where, optional=False):
    if value is None and optional:
        return kind()
    if not isinstance(value, kind):
        raise InputError(f"round.json {where} is not a {kind.__name__}")
    return value


def source_digest():
    files = sorted(p for folder in ("cmd/spar", "internal/spar") for p in (ROOT / folder).glob("*.go")
                   if not p.name.endswith("_test.go"))
    files += [ROOT / "go.mod", ROOT / "go.sum"]
    digest = hashlib.sha256()
    for path in files:
        try:
            data = path.read_bytes()
        except OSError as error:
            raise InputError(f"{path}: {error}") from error
        digest.update(path.relative_to(ROOT).as_posix().encode() + b"\0" + data + b"\0")
    return digest.hexdigest()[:12]


def text(obj, key, where):
    return typed(obj.get(key), str, f"{where}.{key}", optional=True)


def started(attempt, where):
    """Return the attempt's exit_code, None when its process never started."""
    code = attempt.get("exit_code")
    if code is not None and (not isinstance(code, int) or isinstance(code, bool)):
        raise InputError(f"round.json {where}.exit_code is not an int")
    return code


def named_model(round_dir, name, preset, last):
    """Return the model the reviewer's CLI named in the output of attempt
    last, "" for none. spar keeps the first attempt's output in
    <reviewer>/stdout.jsonl and the fallback's in <reviewer>/fallback/."""
    if preset == "codex":
        return ""
    if last > 1:
        raise InputError(f"round.json reviewers.{name} has more than a first and a fallback attempt")
    path = round_dir / name / ("fallback" if last > 0 else "") / "stdout.jsonl"
    try:
        data = path.read_bytes()
    except OSError as error:
        raise InputError(f"{path}: {error}") from error
    model = ""
    for number, line in enumerate(data.split(b"\n"), 1):
        line = line.decode("utf-8", errors="replace").strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except ValueError as error:
            raise InputError(f"{path}:{number}: an event line is not JSON: {error}") from error
        if preset == "claude" and event.get("type") == "system" and event.get("subtype") == "init":
            model = event.get("model")
        elif preset == "agy" and isinstance(event.get("init"), dict):
            model = event["init"].get("model")
        else:
            continue
        if not isinstance(model, str):
            model = ""
    return model


def after(argv, flag):
    for i in range(len(argv) - 1):
        if argv[i] == flag:
            return argv[i + 1]
    return None


def check(round_dir):
    """Return the violations, the reasons the round cannot be observed and
    the facts of each reviewer that ran on the agent's model."""
    problems, unobserved, ran = [], [], []
    record = read_json(round_dir / "round.json")
    if not isinstance(record, dict) or record.get("schema") != "review-round/v1":
        raise InputError(f"{round_dir / 'round.json'}: not a review-round/v1 record")
    want = "+src." + source_digest()
    version = typed(record.get("spar_version"), str, "spar_version")
    if not version.endswith(want):
        problems.append(f"the round was written by spar {version!r}, not by a spar built from this tree's sources ({want})")
    config = typed(record.get("config"), dict, "config")
    choices = typed(config.get("choices"), list, "config.choices")
    applied = {}
    for i, entry in enumerate(typed(config.get("reviewers"), list, "config.reviewers")):
        entry = typed(entry, dict, f"config.reviewers[{i}]")
        applied[typed(entry.get("name"), str, f"config.reviewers[{i}].name")] = entry
    states = typed(record.get("reviewers"), dict, "reviewers")
    presets = set()
    chosen = 0
    for i, choice in enumerate(choices):
        choice = typed(choice, dict, f"config.choices[{i}]")
        name = typed(choice.get("reviewer"), str, f"config.choices[{i}].reviewer")
        key = typed(choice.get("key"), str, f"config.choices[{i}].key")
        value = typed(choice.get("value"), str, f"config.choices[{i}].value")
        if key != "model":
            continue
        chosen += 1
        state = typed(states.get(name), dict, f"reviewers.{name}")
        preset = typed(state.get("preset"), str, f"reviewers.{name}.preset")
        if preset not in PRESETS:
            unobserved.append(f"{name}: the agent chose a model for a {preset!r} reviewer, not a claude, codex or agy one")
            continue
        presets.add(preset)
        requested = text(state, "requested_model", f"reviewers.{name}")
        source = text(state, "model_source", f"reviewers.{name}")
        if requested != value or source != "agent":
            problems.append(f"{name}: requested_model {requested!r} from {source!r}, the agent chose {value!r}")
        where = f"config.reviewers[{name}]"
        entry = typed(applied.get(name), dict, where)
        model, source = (text(entry, key, where) for key in ("model", "model_source"))
        if model != value or source != "agent":
            problems.append(f"{name}: config.reviewers has model {model!r} from {source!r}")
        attempts = typed(state.get("attempts"), list, f"reviewers.{name}.attempts", optional=True)
        if not attempts:
            unobserved.append(f"{name}: the round records no attempt")
            continue
        first = typed(attempts[0], dict, f"reviewers.{name}.attempts[0]")
        where = f"reviewers.{name}.attempts[0]"
        argv = typed(first.get("argv"), list, f"{where}.argv", optional=True)
        for j, arg in enumerate(argv):
            typed(arg, str, f"{where}.argv[{j}]")
        cli_version, reason, ran_model = (text(first, key, where) for key in ("cli_version", "reason", "model"))
        exit_code = started(first, where)
        if exit_code is None or not argv:
            unobserved.append(f"{name}: the CLI process did not start (exit_code {exit_code!r}, argv {bool(argv)}, reason {reason!r})")
            continue
        if ran_model != value or after(argv, MODEL_FLAG[preset]) != value:
            problems.append(f"{name}: the first attempt ran model {ran_model!r} with "
                            f"{MODEL_FLAG[preset]} {after(argv, MODEL_FLAG[preset])!r}, the agent chose {value!r}")
        for k, attempt in enumerate(attempts):
            where = f"reviewers.{name}.attempts[{k}]"
            attempt = typed(attempt, dict, where)
            last_started = started(attempt, where) is not None
            named = named_model(round_dir, name, preset, k) if last_started else ""
            if text(attempt, "observed_model", where) != named:
                problems.append(f"{name}: attempt {k} records observed_model "
                                f"{text(attempt, 'observed_model', where)!r}, the CLI named {named!r}")
        observed = text(state, "observed_model", f"reviewers.{name}")
        if observed != named:
            problems.append(f"{name}: the record has observed_model {observed!r}, the CLI named {named!r}")
        if text(state, "status", f"reviewers.{name}") != "completed":
            continue
        if not last_started:
            raise InputError(f"round.json reviewers.{name} completed, but its last attempt records no exit_code")
        ran.append(f"{name}: {argv[0]} ({cli_version}) ran {value!r}, observed {observed!r}")
    missing = [p for p in PRESETS if p not in presets]
    if missing:
        unobserved.append(f"the agent chose no model for {', '.join(missing)}")
    if chosen and not ran:
        unobserved.append("no reviewer whose model the agent chose completed")
    return problems, unobserved, ran


def main(argv):
    if argv[1:] == ["--source-digest"]:
        try:
            print(source_digest())
        except InputError as error:
            print(f"real_round: error: {error}", file=sys.stderr)
            return 2
        return 0
    if len(argv) != 2:
        print("real_round: error: usage: real_round.py ROUND_FOLDER | --source-digest", file=sys.stderr)
        return 2
    try:
        problems, unobserved, ran = check(Path(argv[1]))
    except InputError as error:
        print(f"real_round: error: {error}", file=sys.stderr)
        return 2
    for problem in problems:
        print(f"real_round: {problem}")
    if problems:
        return 1
    for reason in unobserved:
        print(f"real_round: error: {reason}", file=sys.stderr)
    if unobserved:
        return 2
    for fact in ran:
        print(f"real_round: {fact}")
    print(f"real_round: {len(ran)} reviewers ran on the agent's model")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
