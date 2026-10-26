#!/usr/bin/env python3
"""Check that a round written by the real reviewer CLIs ran each reviewer
whose model the agent chose on that model, with a spar built from the
sources in this tree.

Usage: real_round.py ROUND_FOLDER
       real_round.py --source-digest

Exit 0 when the property holds, 1 when it does not, 2 when the input cannot
be read (missing round.json or source files, JSON that does not parse,
another record format, bad arguments). The round is the one the condition
names; besides it only the spar sources of this tree are read.

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
# CLIs that name the model they ran on in their output.
REPORTS_MODEL = ("claude", "agy")
# Reasons of a reviewer whose CLI never started.
NOT_STARTED = ("not found", "could not start", "is not an executable file")
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
    want = "+src." + source_digest()
    version = record.get("spar_version")
    if not isinstance(version, str) or not version.endswith(want):
        problems.append(f"the round was written by spar {version!r}, not by a spar built from this tree's sources ({want})")
    config = record.get("config") if isinstance(record.get("config"), dict) else {}
    choices = config.get("choices")
    if not isinstance(choices, list):
        return problems + ["round.json config.choices is not a list"], 0
    applied = {r.get("name"): r for r in config.get("reviewers") or [] if isinstance(r, dict)}
    states = record.get("reviewers") if isinstance(record.get("reviewers"), dict) else {}
    chosen = [c for c in choices if isinstance(c, dict) and c.get("key") == "model"]
    presets = set()
    completed = 0
    for choice in chosen:
        name, value = choice.get("reviewer"), choice.get("value")
        state = states.get(name)
        if not isinstance(state, dict) or state.get("preset") not in PRESETS:
            problems.append(f"{name}: the chosen reviewer is not a claude, codex or agy reviewer of the round")
            continue
        preset = state["preset"]
        presets.add(preset)
        if state.get("requested_model") != value or state.get("model_source") != "agent":
            problems.append(f"{name}: requested_model {state.get('requested_model')!r} from "
                            f"{state.get('model_source')!r}, the agent chose {value!r}")
        entry = applied.get(name) or {}
        if entry.get("model") != value or entry.get("model_source") != "agent":
            problems.append(f"{name}: config.reviewers has model {entry.get('model')!r} from {entry.get('model_source')!r}")
        if entry.get("binary"):
            problems.append(f"{name}: the CLI is {entry['binary']!r}, not the {preset} found on PATH")
        attempts = state.get("attempts") if isinstance(state.get("attempts"), list) else []
        first = attempts[0] if attempts and isinstance(attempts[0], dict) else {}
        argv = first.get("argv") if isinstance(first.get("argv"), list) else []
        if first.get("model") != value or after(argv, MODEL_FLAG[preset]) != value:
            problems.append(f"{name}: the first attempt ran model {first.get('model')!r} with "
                            f"{MODEL_FLAG[preset]} {after(argv, MODEL_FLAG[preset])!r}, the agent chose {value!r}")
        reason = str(first.get("reason") or "")
        if not first.get("cli_version") or any(r in reason for r in NOT_STARTED):
            problems.append(f"{name}: the CLI did not start: version {first.get('cli_version')!r}, {reason!r}")
        if state.get("status") != "completed":
            continue
        completed += 1
        if preset in REPORTS_MODEL and not state.get("observed_model"):
            problems.append(f"{name}: {preset} completed but the record has no observed_model")
    missing = [p for p in PRESETS if p not in presets]
    if missing:
        problems.append(f"the agent chose no model for {', '.join(missing)}")
    if chosen and completed == 0:
        problems.append("no reviewer whose model the agent chose completed")
    return problems, completed


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
