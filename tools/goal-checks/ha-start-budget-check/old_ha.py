#!/usr/bin/env python3
"""AC-31: the start form that checks a skill's declaration ends without a record on earlier `ha`.

Usage: python3 old_ha.py PATH=VERSION [PATH=VERSION ...]

Each argument names an earlier `ha` executable and the version its
`--version` must print (`ha VERSION`). The executables are external inputs
the condition names: the v0.1.6 release and a build of the commit this goal
started from. For each one this script makes a synthetic goal repository with
a valid numeric budget and a skill folder with a Core requirements
declaration, runs `ha start <goal> --request … --skill <SKILL.md>
--check-skill` (the contract's form) and requires a nonzero exit and no
runs.jsonl.

Exit 0: every executable refused without a record. Exit 1: one started or
wrote a record. Exit 2: an executable is missing, does not run, prints another
version, or the synthetic repository could not be made; nothing was observed.
"""
from pathlib import Path
import sys

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from budget_common import CHECK_FLAG, DECLARATION_SCHEMA, Repo, Unavailable, clean_env, run, temporary


def observe(path, version):
    result = run([path, "--version"], Path.cwd(), clean_env(), timeout=60)
    printed = result.stdout.decode(errors="replace").strip()
    if result.returncode or printed != f"ha {version}":
        raise Unavailable(f"{path} --version: exit {result.returncode}, {printed!r}, want 'ha {version}'")
    with temporary("jaekit-old-ha-") as directory:
        repo = Repo(Path(directory) / "goal", path, changes=1, runs="60")
        skill = repo.skill("declared", {"schema": DECLARATION_SCHEMA, "features": ["estimate/v1"]})
        code, out, err = repo.start("--skill", str(skill), CHECK_FLAG)
        written = repo.records_path().exists()
        print(f"ha {version}: exit {code}, runs.jsonl {'written' if written else 'absent'}: {err.strip()[-200:]}")
        return code != 0 and not written


def main(argv):
    if len(argv) < 2:
        print(__doc__, file=sys.stderr)
        return 2
    inputs = []
    for item in argv[1:]:
        path, sep, version = item.rpartition("=")
        if not sep or not path or not version:
            print(f"argument {item!r} is not PATH=VERSION", file=sys.stderr)
            return 2
        inputs.append((Path(path), version))
    try:
        refused = [observe(path, version) for path, version in inputs]
    except Unavailable as error:
        print(f"not observed: {error}", file=sys.stderr)
        return 2
    return 0 if all(refused) else 1


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
