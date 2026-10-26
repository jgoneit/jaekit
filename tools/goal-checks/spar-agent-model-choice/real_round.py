#!/usr/bin/env python3
"""Check that a round written by the real reviewer CLIs ran each reviewer
whose model the agent chose on that model, with a spar built from the
sources in this tree.

Usage: real_round.py ROUND_FOLDER
       real_round.py --source-digest

Exit 0 when the property holds, 1 when it does not, 2 when it cannot be
observed: the input cannot be read (missing round.json or source files,
JSON that does not parse, another record format, a field of the wrong
type, bad arguments), a chosen reviewer's CLI never started (no argv or
cli_version recorded, or spar's "could not start" reason), or no chosen
reviewer completed. A property that fails for a reviewer whose CLI started
is a violation even when another reviewer's CLI did not start. The round is
the one the condition names; besides it only the spar sources of this tree
are read.

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
# The reason spar gives when the executable it resolved fails to start.
START_FAILED = "could not start "
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
        preset = state.get("preset")
        if preset not in PRESETS:
            problems.append(f"{name}: the chosen reviewer is not a claude, codex or agy reviewer of the round")
            continue
        presets.add(preset)
        if state.get("requested_model") != value or state.get("model_source") != "agent":
            problems.append(f"{name}: requested_model {state.get('requested_model')!r} from "
                            f"{state.get('model_source')!r}, the agent chose {value!r}")
        entry = applied.get(name, {})
        if entry.get("model") != value or entry.get("model_source") != "agent":
            problems.append(f"{name}: config.reviewers has model {entry.get('model')!r} from {entry.get('model_source')!r}")
        if entry.get("binary"):
            problems.append(f"{name}: the CLI is {entry['binary']!r}, not the {preset} found on PATH")
        attempts = typed(state.get("attempts"), list, f"reviewers.{name}.attempts", optional=True)
        if not attempts:
            unobserved.append(f"{name}: the round records no attempt")
            continue
        first = typed(attempts[0], dict, f"reviewers.{name}.attempts[0]")
        argv = typed(first.get("argv"), list, f"reviewers.{name}.attempts[0].argv", optional=True)
        cli_version = typed(first.get("cli_version"), str, f"reviewers.{name}.attempts[0].cli_version", optional=True)
        reason = typed(first.get("reason"), str, f"reviewers.{name}.attempts[0].reason", optional=True)
        if not argv or not cli_version or reason.startswith(START_FAILED):
            unobserved.append(f"{name}: the CLI did not start (argv {bool(argv)}, version {cli_version!r}, reason {reason!r})")
            continue
        if first.get("model") != value or after(argv, MODEL_FLAG[preset]) != value:
            problems.append(f"{name}: the first attempt ran model {first.get('model')!r} with "
                            f"{MODEL_FLAG[preset]} {after(argv, MODEL_FLAG[preset])!r}, the agent chose {value!r}")
        if state.get("status") != "completed":
            continue
        observed = state.get("observed_model")
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
