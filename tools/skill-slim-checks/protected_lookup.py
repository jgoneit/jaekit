#!/usr/bin/env python3
"""Tell, for each requested phrase, whether its protected clause reports it missing.

Usage: protected_lookup.py <tree> <requests.json>

The requests are a JSON list of {"suite", "criterion", "file", "section",
"row", "phrase"}: one clause of a criterion and one of its anchors. The script
imports the contract checks of <tree>/tools/contract-checks and calls the
chained observe() for each criterion on <tree>, as their main() does. The
clause reports the phrase missing when a problem line starts with the
clause's file and holds the phrase's repr() with "missing" or "lacks", the
forms the checks write for an absent anchor or row anchor; for a section
clause the line also names the section heading, for a row clause the row
label, and for a whole-file clause the line names no section heading. So the
same phrase in another clause of the criterion is judged on its own.

authoring_check's regressions() runs the plugins Go tests and the public
documentation check; it looks up no phrase, so it is replaced with an empty
result here. Everything else runs as written. It prints a JSON list in
request order: true when the clause reports the phrase missing, false when
it does not, and "error" when the criterion's observe() raises, so the check
refuses the tree without reaching a result. Any other exception exits
nonzero.
"""
import importlib
import json
from pathlib import Path
import sys


def main():
    root = Path(sys.argv[1]).resolve()
    requests = json.loads(Path(sys.argv[2]).read_text(encoding="utf-8"))
    sys.path.insert(0, str(root / "tools" / "contract-checks"))
    check = importlib.import_module("check")
    for module in ("design_check", "finding_docs_check", "spec_check"):
        importlib.import_module(module)
    authoring = importlib.import_module("authoring_check")
    authoring.regressions = lambda tree: []
    problems = {}
    out = []
    for request in requests:
        key = (request["suite"], request["criterion"])
        if key not in problems:
            try:
                _, problems[key] = check.observe(root, *key)
            except Exception as error:  # the check refuses the tree
                print(f"{key[0]} {key[1]}: {type(error).__name__}: {error}", file=sys.stderr)
                problems[key] = None
        if problems[key] is None:
            out.append("error")
        else:
            out.append(any(reports(line, request) for line in problems[key]))
    print(json.dumps(out))


def reports(line, request):
    """Whether one problem line is this clause reporting this phrase missing."""
    if not (line.startswith(request["file"] + ":") or line.startswith(request["file"] + " ")):
        return False
    if repr(request["phrase"]) not in line or not (" missing " in line or " lacks " in line or ": missing" in line):
        return False
    if request["row"]:
        return f"row {request['row']!r}" in line
    if request["section"]:
        return f" {request['section']}: " in line
    return " ## " not in line and " ### " not in line


if __name__ == "__main__":
    main()
