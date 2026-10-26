#!/usr/bin/env python3
"""Check that a round written by the real reviewer CLIs ran each reviewer
whose model the agent chose on that model, with a spar built from the
sources in this tree.

Usage: real_round.py ROUND_FOLDER
       real_round.py --source-digest

Exit 0 when the property holds, 1 when it does not, 2 when it cannot be
observed: the input cannot be read (missing round.json or source files,
JSON that does not parse, another record format, a field of the wrong
type, bad arguments), a chosen reviewer's CLI process never started (its
first attempt records no exit_code, which spar records for every process
it started, or no argv), or no chosen reviewer completed. A property that
fails for a reviewer whose CLI started is a violation even when another
reviewer's CLI did not start. Whether the executable is the real CLI is
not observed, whether spar found it on PATH or in a binary key: a pass
prints each executable path and cli_version. The round is the one the
condition names; besides it only the spar sources of this tree are read.

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
    if config.get("choices") is None:
        return problems + ["round.json config has no choices"], unobserved, ran
    choices = typed(config["choices"], list, "config.choices")
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
        state = typed(states.get(name), dict, f"reviewers.{name}", optional=True)
        preset = text(state, "preset", f"reviewers.{name}")
        if preset not in PRESETS:
            problems.append(f"{name}: the chosen reviewer is not a claude, codex or agy reviewer of the round")
            continue
        presets.add(preset)
        requested = text(state, "requested_model", f"reviewers.{name}")
        source = text(state, "model_source", f"reviewers.{name}")
        if requested != value or source != "agent":
            problems.append(f"{name}: requested_model {requested!r} from {source!r}, the agent chose {value!r}")
        entry = applied.get(name, {})
        where = f"config.reviewers[{name}]"
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
        exit_code = first.get("exit_code")
        if exit_code is not None and (not isinstance(exit_code, int) or isinstance(exit_code, bool)):
            raise InputError(f"round.json {where}.exit_code is not an int")
        if exit_code is None or not argv:
            unobserved.append(f"{name}: the CLI process did not start (exit_code {exit_code!r}, argv {bool(argv)}, reason {reason!r})")
            continue
        if ran_model != value or after(argv, MODEL_FLAG[preset]) != value:
            problems.append(f"{name}: the first attempt ran model {ran_model!r} with "
                            f"{MODEL_FLAG[preset]} {after(argv, MODEL_FLAG[preset])!r}, the agent chose {value!r}")
        if text(state, "status", f"reviewers.{name}") != "completed":
            continue
        observed = text(state, "observed_model", f"reviewers.{name}")
        if preset in REPORTS_MODEL and not observed:
            problems.append(f"{name}: {preset} completed but the record has no observed_model")
        ran.append(f"{name}: {argv[0]} ({cli_version}) ran {value!r}, observed {observed!r}")
    missing = [p for p in PRESETS if p not in presets]
    if missing:
        problems.append(f"the agent chose no model for {', '.join(missing)}")
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
