#!/usr/bin/env python3
"""Check that a round written by real reviewer CLIs carries their smallest fix
directions through to findings.json and summary.md.

Usage: real_round.py ROUND_FOLDER

Exit 0 when the property holds, 1 when it does not, 2 when the input cannot
be read (missing files, JSON that does not parse, another record format, bad
arguments). The round is the one the condition names; nothing else is read.
"""

import json
import sys
from pathlib import Path

PRESETS = ("claude", "codex", "agy")


class InputError(Exception):
    pass


def read_json(path):
    try:
        return json.loads(path.read_text(encoding="utf-8"))
    except OSError as error:
        raise InputError(f"{path}: {error}") from error
    except ValueError as error:
        raise InputError(f"{path}: not JSON: {error}") from error


def raw_result(folder, preset):
    """The result object the CLI itself left in the reviewer folder."""
    if preset == "codex":
        return read_json(folder / "last_message.json")
    path = folder / "stdout.jsonl"
    try:
        lines = path.read_text(encoding="utf-8").splitlines()
    except OSError as error:
        raise InputError(f"{path}: {error}") from error
    result = None
    for line in lines:
        line = line.strip()
        if not line.startswith("{"):
            continue
        try:
            event = json.loads(line)
        except ValueError:
            continue
        if preset == "claude" and event.get("type") == "result":
            result = event.get("structured_output")
        elif preset == "agy" and isinstance(event.get("result"), dict):
            result = event["result"].get("structured_output")
    if not isinstance(result, dict):
        raise InputError(f"{path}: no structured output")
    return result


def summary_item(finding_id, direction):
    text = direction.strip()
    return f"- {finding_id}: " + text.replace("\n", "\n  ")


def check(round_dir):
    problems = []
    record = read_json(round_dir / "round.json")
    if not isinstance(record, dict) or record.get("schema") != "review-round/v1":
        raise InputError(f"{round_dir / 'round.json'}: not a review-round/v1 record")
    findings = read_json(round_dir / "findings.json")
    if not isinstance(findings, list):
        raise InputError(f"{round_dir / 'findings.json'}: not a list")
    by_id = {f.get("id"): f for f in findings if isinstance(f, dict)}
    try:
        summary = (round_dir / "summary.md").read_text(encoding="utf-8")
    except OSError as error:
        raise InputError(f"{round_dir / 'summary.md'}: {error}") from error
    reviewers = record.get("reviewers") or {}
    directions = 0
    for name, state in sorted(reviewers.items()):
        if not isinstance(state, dict) or state.get("preset") not in PRESETS or state.get("status") != "completed":
            continue
        folder = round_dir / name
        if state.get("fallback_used"):
            folder = folder / "fallback"
        raw = raw_result(folder, state["preset"])
        items = raw.get("findings")
        if not isinstance(items, list):
            raise InputError(f"{folder}: the result has no findings list")
        for n, item in enumerate(items, 1):
            given = item.get("smallest_fix", "") if isinstance(item, dict) else ""
            finding_id = f"{name}-F{n}"
            recorded = by_id.get(finding_id)
            if recorded is None:
                problems.append(f"{finding_id} is not in findings.json")
                continue
            if recorded.get("smallest_fix") != given:
                problems.append(f"{finding_id}: findings.json keeps {recorded.get('smallest_fix')!r}, the reviewer gave {given!r}")
            if isinstance(given, str) and given.strip():
                directions += 1
                if summary_item(finding_id, given) not in summary:
                    problems.append(f"{finding_id}: summary.md does not show the direction {given!r}")
    if directions == 0:
        problems.append("no completed claude, codex or agy reviewer gave a smallest fix direction")
    return problems, directions


def main(argv):
    if len(argv) != 2:
        print("real_round: error: usage: real_round.py ROUND_FOLDER", file=sys.stderr)
        return 2
    try:
        problems, directions = check(Path(argv[1]))
    except InputError as error:
        print(f"real_round: error: {error}", file=sys.stderr)
        return 2
    for problem in problems:
        print(f"real_round: {problem}")
    if problems:
        return 1
    print(f"real_round: {directions} directions carried through")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))
