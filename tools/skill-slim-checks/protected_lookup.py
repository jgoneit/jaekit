#!/usr/bin/env python3
"""Tell, for each requested phrase, whether its protected criterion reports it missing.

Usage: protected_lookup.py <tree> <requests.json>

The requests are a JSON list of {"suite", "criterion", "phrase"}. The script
imports the contract checks of <tree>/tools/contract-checks and calls the
chained observe() for each criterion on <tree>, as their main() does. A phrase
is reported missing when a problem line holds its repr() with "missing" or
"lacks", the forms the checks write for an absent anchor or row anchor.

authoring_check's regressions() runs the plugins Go tests and the public
documentation check; it looks up no phrase, so it is replaced with an empty
result here. Everything else runs as written. It prints a JSON list of
booleans in request order. Any exception exits nonzero.
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
            _, problems[key] = check.observe(root, *key)
        needle = repr(request["phrase"])
        out.append(any(needle in line and (" missing " in line or " lacks " in line or ": missing" in line)
                       for line in problems[key]))
    print(json.dumps(out))


if __name__ == "__main__":
    main()
