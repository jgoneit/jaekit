#!/usr/bin/env python3
"""Require every native job from this workflow run to finish successfully.

Input is GitHub's needs context, passed as data in JAEKIT_CI_NEEDS. This gate
does not query an older run or replace missing native execution with a check
of the workflow's configuration.
"""
import json
import os
import sys


# Each Windows shell runs in its own job, so both are required.
REQUIRED_JOBS = ("go", "macos", "windows_ps51", "windows_ps7")


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ValueError("duplicate result key")
        result[key] = value
    return result


def require_success(raw):
    try:
        jobs = json.loads(raw, object_pairs_hook=unique_object)
    except (TypeError, ValueError) as error:
        raise ValueError("native job results are missing or invalid") from error
    if not isinstance(jobs, dict) or set(jobs) != set(REQUIRED_JOBS):
        raise ValueError("native job results must contain exactly " + ", ".join(REQUIRED_JOBS))
    failed = [name for name in REQUIRED_JOBS
              if not isinstance(jobs[name], dict) or jobs[name].get("result") != "success"]
    if failed:
        raise ValueError("native jobs did not succeed: " + ", ".join(failed))


def main():
    try:
        require_success(os.environ.get("JAEKIT_CI_NEEDS"))
    except ValueError as error:
        print("FAIL: Seal Core: " + str(error), file=sys.stderr)
        return 1
    print("PASS: Seal Core: Ubuntu, macOS, Windows PowerShell 5.1 and 7 native jobs succeeded in this run.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
