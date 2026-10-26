#!/usr/bin/env python3
"""Check that a round written by real reviewer CLIs ran each reviewer whose
model the agent chose on that model.

Usage: real_round.py ROUND_FOLDER

Exit 0 when the property holds, 1 when it does not, 2 when the input cannot
be read (missing round.json, JSON that does not parse, another record format,
bad arguments). The round is the one the condition names; nothing else is
read.
"""

import json
import sys
from pathlib import Path

PRESETS = ("claude", "codex", "agy")
MODEL_FLAG = {"claude": "--model", "codex": "-m", "agy": "--model"}


class InputError(Exception):
    pass


def read_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except OSError as error:
        raise InputError(f"{path}: {error}") from error
    except ValueError as error:
        raise InputError(f"{path}: not JSON: {error}") from error


def after(argv, flag):
    for i in range(len(argv) - 1):
        if argv[i] == flag:
            return argv[i + 1]
    return None


def check(round_dir):
    problems = []
    record = read_json(round_dir / "round.json")
    if not isinstance(record, dict) or record.get("schema") != "review-round/v1":
        raise InputError(f"{round_dir / 'round.json'}: not a review-round/v1 record")
    config = record.get("config") if isinstance(record.get("config"), dict) else {}
    choices = config.get("choices")
    if not isinstance(choices, list):
        return ["round.json config.choices is not a list"], 0
    applied = {r.get("name"): r for r in config.get("reviewers") or [] if isinstance(r, dict)}
    states = record.get("reviewers") if isinstance(record.get("reviewers"), dict) else {}
    chosen = [c for c in choices if isinstance(c, dict) and c.get("key") == "model"]
    completed = 0
    for choice in chosen:
        name, value = choice.get("reviewer"), choice.get("value")
        state = states.get(name)
        if not isinstance(state, dict) or state.get("preset") not in PRESETS:
            problems.append(f"{name}: the chosen reviewer is not a claude, codex or agy reviewer of the round")
            continue
        preset = state["preset"]
        if state.get("requested_model") != value or state.get("model_source") != "agent":
            problems.append(f"{name}: requested_model {state.get('requested_model')!r} from "
                            f"{state.get('model_source')!r}, the agent chose {value!r}")
        entry = applied.get(name) or {}
        if entry.get("model") != value or entry.get("model_source") != "agent":
            problems.append(f"{name}: config.reviewers has model {entry.get('model')!r} from {entry.get('model_source')!r}")
        attempts = state.get("attempts") if isinstance(state.get("attempts"), list) else []
        first = attempts[0] if attempts and isinstance(attempts[0], dict) else {}
        argv = first.get("argv") if isinstance(first.get("argv"), list) else []
        if first.get("model") != value or after(argv, MODEL_FLAG[preset]) != value:
            problems.append(f"{name}: the first attempt ran model {first.get('model')!r} with "
                            f"{MODEL_FLAG[preset]} {after(argv, MODEL_FLAG[preset])!r}, the agent chose {value!r}")
        if state.get("status") != "completed":
            continue
        completed += 1
        if not first.get("cli_version"):
            problems.append(f"{name}: the completed attempt records no CLI version")
        if preset == "claude" and not state.get("observed_model"):
            problems.append(f"{name}: claude completed but the record has no observed_model")
    if not chosen:
        problems.append("the agent chose no model in this round")
    elif completed == 0:
        problems.append("no reviewer whose model the agent chose completed")
    return problems, completed


def main(argv):
    if len(argv) != 2:
        print("real_round: error: usage: real_round.py ROUND_FOLDER", file=sys.stderr)
        return 2
    try:
        problems, completed = check(Path(argv[1]))
    except InputError as error:
        print(f"real_round: error: {error}", file=sys.stderr)
        return 2
    for problem in problems:
        print(f"real_round: {problem}")
    if problems:
        return 1
    print(f"real_round: {completed} reviewers ran on the agent's model")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
