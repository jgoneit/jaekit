#!/usr/bin/env python3
"""cmd-ha-test-speed AC-2 and AC-7: no cmd/ha test is dropped or narrowed.

Usage, from the repository root:

  python3 tools/goal-checks/cmd-ha-test-speed/inventory.py --criterion AC-2
  python3 tools/goal-checks/cmd-ha-test-speed/inventory.py --criterion AC-7
  python3 tools/goal-checks/cmd-ha-test-speed/inventory.py --write-snapshot [--commit REV]

`base_tests.json` is the base snapshot: for every cmd/ha/*_test.go of the base
commit, its name, its build constraint line (or null) and its top-level
`func TestXxx(` names, TestMain included, plus every cmd/ha selector entry
of contracts/regressions.json at that commit as (route id, file, selector),
repeats across routes kept, plus `native_selections`: the test names that
tools/windows/native_checks.py (NATIVE["cmd/ha"], `named`) and
tools/verification-checks/cross_platform_ci.py (SCENARIOS and WINDOWS,
`unqualified`) select by exact name at that commit, each a sorted list, as
`selections()` returns them. ci.py reads the base selections from it, never
from the commit. `renames.json` maps a base test name to the list of head
names that replace it.

AC-2 passes when every base test exists at head as a top-level test of a
compiled cmd/ha/*_test.go file (Go ignores names starting with `_` or `.`),
or is mapped in renames.json to names that all exist; and when every base
selector entry is still in the current contracts/regressions.json, counted
as entries, not unique (file, selector) pairs. The one exception: the base
entry's selector is mapped in renames.json, and the same route at head has
an entry for each name it maps to, naming the head cmd/ha/*_test.go file
that declares that name. Every current cmd/ha selector, and every base
selector not replaced that way, must pass tools/go_selection.py's
validate_test at head: declared exactly once in the named file, which has no
build constraint.

AC-7 passes when every head file that existed at base keeps its base
constraint; a new file has none, unless it holds tests that came from base
files (by name or through renames.json) and all of those had its constraint;
every base test (and each name it was renamed to) sits only in files with its
base constraint; and cmd/ha/main_test.go's TestMain calls
gittest.DisableAutoMaintenance() as a top-level statement before m.Run().
A constraint is the `//go:build` (or `// +build`) lines before the package
clause together with a GOOS/GOARCH file-name suffix.

The reader is lexical (tools/go_selection.py tokens), not a Go compiler.

The checks read only this checkout's files; git runs only for --write-snapshot.

Exit 0: pass. Exit 1: a violation; each missing name, selector or constraint
is printed. Exit 2: not observed: the snapshot or renames file is missing or
corrupt, a Go file cannot be read or tokenized, or git fails (--write-snapshot).
Never a pass.
"""
import argparse
import ast
from collections import Counter, defaultdict
import importlib.util
import json
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[3]
HERE = Path(__file__).resolve().parent
PACKAGE_DIR = "cmd/ha"
SNAPSHOT = HERE / "base_tests.json"
RENAMES = HERE / "renames.json"
REGRESSIONS = "contracts/regressions.json"
SCHEMA = "cmd-ha-test-inventory/v3"
FIELDS = ("schema", "commit", "files", "regression_selectors", "native_selections")
WINDOWS_SELECTION = "tools/windows/native_checks.py"
CI_SELECTION = "tools/verification-checks/cross_platform_ci.py"
GITTEST = "github.com/jgoneit/jaekit/internal/gittest"
CONSTRAINT = re.compile(r"//(?:go:build(?:\s|$)|\s*\+build(?:\s|$))")
IDENTIFIER = re.compile(r"[A-Za-z_][A-Za-z0-9_]*")
FILE_NAME = re.compile(r"[A-Za-z0-9][A-Za-z0-9_.-]*_test\.go")


def load_selection():
    spec = importlib.util.spec_from_file_location("cmd_ha_test_speed_go_selection", ROOT / "tools/go_selection.py")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


gs = load_selection()


class InputFailure(Exception):
    pass


def is_test_name(name):
    """Go's rule: Test, or Test followed by a character that is not lower case."""
    return isinstance(name, str) and bool(IDENTIFIER.fullmatch(name)) and name.startswith("Test") and (
        len(name) == 4 or not name[4].islower())


def filename_constraint(name):
    """The GOOS/GOARCH suffix Go reads from a file name (go/build goodOSArchFile)."""
    stem = name.split(".", 1)[0]
    parts = stem[stem.find("_"):].split("_") if "_" in stem else []
    if parts and parts[-1] == "test":
        parts.pop()
    found = []
    if parts and parts[-1] in gs.GOARCH:
        found.insert(0, parts.pop())
    if parts and parts[-1] in gs.GOOS:
        found.insert(0, parts.pop())
    return "_".join(found) or None


def show(key):
    build, suffix = key
    words = []
    if build:
        words.append(" / ".join(build.splitlines()))
    if suffix:
        words.append(f"file suffix _{suffix}")
    return " + ".join(words) or "none"


def parse(name, text):
    """Return (constraint lines or None, top-level test names, declaration chunks)."""
    try:
        items, comments = gs.tokens(text)
        chunks = gs.declarations(items)
    except gs.SelectionError as error:
        raise InputFailure(f"{PACKAGE_DIR}/{name}: {error}") from None
    if not chunks or chunks[0][0][1] != "package":
        raise InputFailure(f"{PACKAGE_DIR}/{name}: package clause missing or unsupported")
    start = chunks[0][0][2]
    lines = [comment.rstrip() for position, comment in comments if position < start and CONSTRAINT.match(comment)]
    tests = []
    for chunk in chunks[1:]:
        if (chunk[0][1] == "func" and len(chunk) > 2 and chunk[1][0] == "name" and chunk[2][1] == "("
                and is_test_name(chunk[1][1])):
            tests.append(chunk[1][1])
    return ("\n".join(lines) or None), tests, chunks


def decode(name, data):
    try:
        return data.decode("utf-8")
    except UnicodeDecodeError as error:
        raise InputFailure(f"{name} is not UTF-8: {error}") from None


def assigned(source, label, name):
    """The value node assigned to a module-level name, read with ast (the file is not run)."""
    try:
        tree = ast.parse(source, filename=label)
    except (SyntaxError, ValueError) as error:
        raise InputFailure(f"{label} cannot be parsed: {error}") from None
    found = [node.value for node in tree.body if isinstance(node, (ast.Assign, ast.AnnAssign))
             and any(isinstance(target, ast.Name) and target.id == name
                     for target in (node.targets if isinstance(node, ast.Assign) else [node.target]))]
    if len(found) != 1 or found[0] is None:
        raise InputFailure(f"{label} does not assign {name} exactly once at module level")
    return found[0]


def test_names(node):
    """Top-level Go test names in the string literals under an ast node (`TestX/case` gives TestX)."""
    return {item.value.split("/", 1)[0] for item in ast.walk(node)
            if isinstance(item, ast.Constant) and isinstance(item.value, str)
            and is_test_name(item.value.split("/", 1)[0])}


def selections(source_of):
    """(cmd/ha names, other names) the two selecting files list by exact name.

    source_of(path) returns (text, label) for one selecting file."""
    windows_label, ci_label = source_of(WINDOWS_SELECTION), source_of(CI_SELECTION)
    native = assigned(windows_label[0], windows_label[1], "NATIVE")
    if not isinstance(native, ast.Dict):
        raise InputFailure(f"{windows_label[1]} NATIVE is not a dict literal")
    lists = [value for key, value in zip(native.keys, native.values)
             if isinstance(key, ast.Constant) and key.value == PACKAGE_DIR]
    if len(lists) != 1:
        raise InputFailure(f"{windows_label[1]} NATIVE has no single {PACKAGE_DIR!r} entry")
    named = test_names(lists[0])
    if not named:
        raise InputFailure(f"{windows_label[1]} NATIVE[{PACKAGE_DIR!r}] names no test")
    unqualified = set()
    for name in ("SCENARIOS", "WINDOWS"):
        unqualified |= test_names(assigned(ci_label[0], ci_label[1], name))
    if not unqualified:
        raise InputFailure(f"{ci_label[1]} SCENARIOS and WINDOWS name no test")
    return named, unqualified


def head_files():
    """Parse the cmd/ha test files Go compiles in this checkout."""
    directory = ROOT / PACKAGE_DIR
    if not directory.is_dir():
        raise InputFailure(f"{PACKAGE_DIR} is not a directory")
    files = {}
    for path in sorted(directory.glob("*_test.go")):
        if path.name.startswith(("_", ".")):
            continue
        if not path.is_file():
            raise InputFailure(f"{PACKAGE_DIR}/{path.name} is not a regular file")
        try:
            data = path.read_bytes()
        except OSError as error:
            raise InputFailure(f"{PACKAGE_DIR}/{path.name} cannot be read: {error}") from None
        build, tests, chunks = parse(path.name, decode(f"{PACKAGE_DIR}/{path.name}", data))
        files[path.name] = {"build": build, "tests": tests, "chunks": chunks}
    if not files:
        raise InputFailure(f"no {PACKAGE_DIR}/*_test.go files found")
    return files


def unique_pairs(object_pairs):
    result = {}
    for key, value in object_pairs:
        if key in result:
            raise ValueError(f"duplicate key {key!r}")
        result[key] = value
    return result


def strict_json(name, data):
    try:
        return json.loads(decode(name, data), object_pairs_hook=unique_pairs)
    except ValueError as error:
        raise InputFailure(f"{name} is corrupt: {error}") from None


def read_local(path):
    label = path.relative_to(ROOT).as_posix()
    if path.is_symlink() or (path.exists() and not path.is_file()):
        raise InputFailure(f"{label} is not a regular file")
    try:
        return strict_json(label, path.read_bytes())
    except FileNotFoundError:
        raise InputFailure(f"{label} is missing") from None
    except OSError as error:
        raise InputFailure(f"{label} cannot be read: {error}") from None


def selectors(regressions, label):
    """Every cmd/ha check of a regression map as (route id, file, selector), repeats kept."""
    if not isinstance(regressions, dict) or not isinstance(regressions.get("routes"), list):
        raise InputFailure(f"{label} has no routes list")
    found = []
    for route in regressions["routes"]:
        checks = route.get("checks", []) if isinstance(route, dict) else None
        if not isinstance(checks, list):
            raise InputFailure(f"{label} has a route without a checks list")
        for check in checks:
            if not isinstance(check, dict) or not isinstance(check.get("file"), str):
                raise InputFailure(f"{label} has a check without a file")
            if check["file"].startswith(PACKAGE_DIR + "/"):
                if not isinstance(check.get("selector"), str):
                    raise InputFailure(f"{label} has a {PACKAGE_DIR} check without a selector")
                if not isinstance(route.get("id"), str) or not route["id"]:
                    raise InputFailure(f"{label} has a route with a {PACKAGE_DIR} check but no id")
                found.append((route["id"], check["file"], check["selector"]))
    return found


def validate_snapshot(snapshot):
    label = SNAPSHOT.relative_to(ROOT).as_posix()
    if not isinstance(snapshot, dict):
        raise InputFailure(f"{label} is not an object")
    if set(snapshot) != set(FIELDS):
        missing = [field for field in FIELDS if field not in snapshot]
        extra = sorted(field for field in snapshot if field not in FIELDS)
        raise InputFailure(f"{label} does not have the snapshot fields: "
                           + "; ".join(([f"missing {', '.join(missing)}"] if missing else [])
                                       + ([f"unexpected {', '.join(extra)}"] if extra else [])))
    if snapshot["schema"] != SCHEMA:
        raise InputFailure(f"{label} schema is not {SCHEMA}")
    if not isinstance(snapshot["commit"], str) or not re.fullmatch(r"[0-9a-f]{40}", snapshot["commit"]):
        raise InputFailure(f"{label} commit is not a full commit ID")
    files = snapshot["files"]
    if not isinstance(files, list) or not files:
        raise InputFailure(f"{label} lists no files")
    seen = set()
    for row in files:
        if not isinstance(row, dict) or set(row) != {"file", "build", "tests"}:
            raise InputFailure(f"{label} has a file row without file, build and tests")
        if not isinstance(row["file"], str) or not FILE_NAME.fullmatch(row["file"]) or row["file"] in seen:
            raise InputFailure(f"{label} has an invalid or repeated file name {row['file']!r}")
        seen.add(row["file"])
        if row["build"] is not None and not (isinstance(row["build"], str) and all(
                CONSTRAINT.match(line) for line in row["build"].splitlines()) and row["build"]):
            raise InputFailure(f"{label}: {row['file']} build is neither null nor constraint lines")
        tests = row["tests"]
        if not isinstance(tests, list) or not all(is_test_name(name) for name in tests) or len(set(tests)) != len(tests):
            raise InputFailure(f"{label}: {row['file']} tests are not unique Go test names")
    entries = snapshot["regression_selectors"]
    if not isinstance(entries, list) or not entries or not all(
            isinstance(row, dict) and set(row) == {"route", "file", "selector"} and isinstance(row["route"], str)
            and row["route"] and isinstance(row["file"], str) and row["file"].startswith(PACKAGE_DIR + "/")
            and isinstance(row["selector"], str) for row in entries):
        raise InputFailure(f"{label} regression_selectors is not a non-empty list of cmd/ha route/file/selector rows")
    chosen = snapshot["native_selections"]
    if not isinstance(chosen, dict) or set(chosen) != {"named", "unqualified"}:
        raise InputFailure(f"{label} native_selections does not have exactly the fields named and unqualified")
    for key in ("named", "unqualified"):
        names = chosen[key]
        if (not isinstance(names, list) or not names or not all(is_test_name(name) for name in names)
                or names != sorted(set(names))):
            raise InputFailure(f"{label} native_selections {key} is not a non-empty sorted list of unique Go test names")
    return snapshot


def validate_renames(renames, base_names):
    label = RENAMES.relative_to(ROOT).as_posix()
    if not isinstance(renames, dict):
        raise InputFailure(f"{label} is not an object")
    for old, new in renames.items():
        if old not in base_names:
            raise InputFailure(f"{label} maps {old}, which is not a base test")
        if (not isinstance(new, list) or not new or not all(is_test_name(name) for name in new)
                or len(set(new)) != len(new)):
            raise InputFailure(f"{label}: {old} must map to a non-empty list of unique Go test names")
    return renames


def load_inputs():
    snapshot = validate_snapshot(read_local(SNAPSHOT))
    base_names = {name for row in snapshot["files"] for name in row["tests"]}
    renames = validate_renames(read_local(RENAMES), base_names)
    return snapshot, renames


def check_ac2(snapshot, renames, head):
    problems = []
    base_files = defaultdict(list)
    for row in snapshot["files"]:
        for name in row["tests"]:
            base_files[name].append(row["file"])
    present = {name for row in head.values() for name in row["tests"]}
    for name in sorted(base_files):
        if name in present:
            continue
        where = ", ".join(base_files[name])
        if name not in renames:
            problems.append(f"missing test: {name} (base {where}) is not at head and not mapped in renames.json")
            continue
        missing = [new for new in renames[name] if new not in present]
        if missing:
            problems.append(f"missing test: {name} (base {where}) is renamed to {', '.join(missing)}, not at head")
    current = selectors(read_local(ROOT / REGRESSIONS), REGRESSIONS)
    base = [(row["route"], row["file"], row["selector"]) for row in snapshot["regression_selectors"]]
    declared = defaultdict(set)
    for name, row in head.items():
        for test in row["tests"]:
            declared[test].add(f"{PACKAGE_DIR}/{name}")
    have = Counter(current)
    replaced = set()
    for entry, count in sorted(Counter(base).items()):
        route, path, selector = entry
        lost = count - have[entry]
        if lost <= 0:
            continue
        new = renames.get(selector)
        if new and all(any((route, file, name) in have for file in declared[name]) for name in new):
            replaced.add((path, selector))
            continue
        copies = f" ({lost} of {count} entries)" if count > 1 else ""
        problems.append(f"selector removed: route {route}: {path} {selector}{copies} is no longer in {REGRESSIONS}"
                        + (f", and the route does not name each of {', '.join(new)} in the head file declaring it"
                           if new else ""))
    pairs = {(path, selector) for _, path, selector in current}
    pairs |= {(path, selector) for _, path, selector in base if (path, selector) not in replaced}
    for path, selector in sorted(pairs):
        try:
            gs.validate_test(ROOT / path, selector, root=ROOT)
        except FileNotFoundError:
            problems.append(f"selector broken: {path} {selector}: the file does not exist")
        except gs.SelectionError as error:
            problems.append(f"selector broken: {path} {selector}: {error}")
        except UnicodeDecodeError as error:
            raise InputFailure(f"{path} is not UTF-8: {error}") from None
        except OSError as error:
            raise InputFailure(f"{path} cannot be read: {error}") from None
    summary = (f"{len(base_files)} base tests, {len(base)} base and {len(current)} current cmd/ha selector "
               f"entries, {len(pairs)} (file, selector) pairs validated")
    return problems, summary


def import_aliases(chunks, path):
    aliases = []
    for chunk in chunks[1:]:
        if chunk[0][1] != "import":
            continue
        entries = chunk[1:]
        if entries and entries[0][1] == "(":
            entries = entries[1:-1]
        entry = []
        for item in [*entries, ("punctuation", ";", 0)]:
            if item[1] != ";":
                entry.append(item)
                continue
            if entry and entry[-1][0] == "literal" and entry[-1][1][1:-1] == path:
                aliases.append(entry[0][1] if len(entry) == 2 else path.rsplit("/", 1)[-1])
            entry = []
    return aliases


def matches(body, index, values):
    return [item[1] for item in body[index:index + len(values)]] == values


def check_testmain(head):
    """TestMain calls <gittest>.DisableAutoMaintenance() as a top-level statement before <m>.Run()."""
    row = head.get("main_test.go")
    if row is None:
        return ["TestMain: cmd/ha/main_test.go does not exist"]
    chunks = row["chunks"]
    aliases = import_aliases(chunks, GITTEST)
    if len(aliases) != 1 or aliases[0] in ("_", "."):
        return [f"TestMain: main_test.go must import {GITTEST} once under a package name"]
    found = [chunk for chunk in chunks[1:] if chunk[0][1] == "func" and len(chunk) > 2 and chunk[1][1] == "TestMain"
             and chunk[2][1] == "("]
    if len(found) != 1:
        return ["TestMain: main_test.go must declare TestMain exactly once"]
    chunk = found[0]
    values = [item[1] for item in chunk]
    if len(chunk) < 4 or chunk[3][0] != "name" or ")" not in values[3:]:
        return ["TestMain: unsupported TestMain parameter list"]
    receiver = chunk[3][1]
    closing = values.index(")", 3)
    body = chunk[closing + 1:]
    if not body or body[0][1] != "{" or body[-1][1] != "}":
        return ["TestMain: unsupported TestMain signature"]
    braces = parens = 0
    disable = run = None
    for index, item in enumerate(body):
        if item[0] == "punctuation":
            if item[1] == "{":
                braces += 1
            elif item[1] == "}":
                braces -= 1
            elif item[1] in "([":
                parens += 1
            elif item[1] in ")]":
                parens -= 1
        if run is None and matches(body, index, [receiver, ".", "Run", "("]):
            run = index
        if (disable is None and braces == 1 and parens == 0 and item[0] == "name"
                and matches(body, index, [aliases[0], ".", "DisableAutoMaintenance", "(", ")"])
                and body[index - 1][1] in ("{", ";") and index + 5 < len(body)
                and body[index + 5][1] in (";", "}")):
            disable = index
    if run is None:
        return [f"TestMain: it does not call {receiver}.Run()"]
    if disable is None or disable > run:
        return [f"TestMain: {aliases[0]}.DisableAutoMaintenance() is not a top-level statement before {receiver}.Run()"]
    return []


def check_ac7(snapshot, renames, head):
    problems = []
    base_rows = {row["file"]: row for row in snapshot["files"]}
    base_sets, head_sets = defaultdict(set), defaultdict(set)
    for row in snapshot["files"]:
        for name in row["tests"]:
            base_sets[name].add((row["build"], filename_constraint(row["file"])))
    for name, row in head.items():
        for test in row["tests"]:
            head_sets[test].add((row["build"], filename_constraint(name)))
    origins = defaultdict(set)
    for name, keys in base_sets.items():
        origins[name] |= keys
    for old, new in renames.items():
        for name in new:
            origins[name] |= base_sets[old]
    for name, row in sorted(head.items()):
        key = (row["build"], filename_constraint(name))
        if name in base_rows:
            base_key = (base_rows[name]["build"], filename_constraint(name))
            if key != base_key:
                problems.append(f"constraint changed: {name} has {show(key)}, base has {show(base_key)}")
            continue
        if key == (None, None):
            continue
        traced = {test: origins[test] for test in row["tests"] if origins.get(test)}
        if not traced:
            problems.append(f"new constrained file: {name} has {show(key)} but holds no test from a base file")
        for test, keys in sorted(traced.items()):
            if keys != {key}:
                problems.append(f"new constrained file: {name} has {show(key)}; {test} came from "
                                + " or ".join(sorted(show(k) for k in keys)))
    for name in sorted(base_sets):
        if name in head_sets and head_sets[name] != base_sets[name]:
            problems.append(f"test constraint changed: {name} is under "
                            + " and ".join(sorted(show(k) for k in head_sets[name]))
                            + "; base " + " and ".join(sorted(show(k) for k in base_sets[name])))
    for old in sorted(renames):
        for name in renames[old]:
            if name in head_sets and head_sets[name] != base_sets[old]:
                problems.append(f"renamed test constraint changed: {name} (from {old}) is under "
                                + " and ".join(sorted(show(k) for k in head_sets[name]))
                                + "; base " + " and ".join(sorted(show(k) for k in base_sets[old])))
    problems.extend(check_testmain(head))
    return problems, f"{len(head)} head test files, {len(base_rows)} base files"


def git(*args):
    try:
        result = subprocess.run(["git", *args], cwd=ROOT, capture_output=True, timeout=120)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise InputFailure(f"git could not run: {error}") from None
    if result.returncode:
        raise InputFailure(f"git {' '.join(args[:2])} failed: {result.stderr.decode('utf-8', 'replace').strip()}")
    return result.stdout


def write_snapshot(revision):
    commit = decode("git rev-parse", git("rev-parse", "--verify", revision + "^{commit}")).strip()
    rows = []
    for line in decode("git ls-tree", git("ls-tree", "-z", commit, PACKAGE_DIR + "/")).split("\0"):
        if not line:
            continue
        meta, path = line.split("\t", 1)
        name = path.rsplit("/", 1)[-1]
        if not name.endswith("_test.go") or name.startswith(("_", ".")):
            continue
        if meta.split()[1] != "blob":
            raise InputFailure(f"{path} is not a file at {commit}")
        build, tests, _ = parse(name, decode(path, git("show", f"{commit}:{path}")))
        rows.append({"file": name, "build": build, "tests": tests})
    entries = selectors(strict_json(REGRESSIONS, git("show", f"{commit}:{REGRESSIONS}")), REGRESSIONS)

    def at_commit(path):
        label = f"{commit[:12]}:{path}"
        return decode(label, git("show", f"{commit}:{path}")), label

    named, unqualified = selections(at_commit)
    snapshot = {
        "schema": SCHEMA,
        "commit": commit,
        "files": rows,
        "regression_selectors": [{"route": route, "file": path, "selector": selector}
                                 for route, path, selector in sorted(entries)],
        "native_selections": {"named": sorted(named), "unqualified": sorted(unqualified)},
    }
    validate_snapshot(snapshot)
    SNAPSHOT.write_text(json.dumps(snapshot, indent=1) + "\n", encoding="utf-8")
    tests = sum(len(row["tests"]) for row in rows)
    print(f"wrote {SNAPSHOT.relative_to(ROOT).as_posix()}: {len(rows)} files, {tests} tests, "
          f"{len(snapshot['regression_selectors'])} selectors, {len(named)} named and {len(unqualified)} "
          f"unqualified native selections at {commit}")


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    mode = parser.add_mutually_exclusive_group(required=True)
    mode.add_argument("--criterion", choices=("AC-2", "AC-7"))
    mode.add_argument("--write-snapshot", action="store_true")
    parser.add_argument("--commit", default="HEAD", help="snapshot source commit (with --write-snapshot)")
    args = parser.parse_args()
    try:
        if args.write_snapshot:
            write_snapshot(args.commit)
            return 0
        snapshot, renames = load_inputs()
        head = head_files()
        check = check_ac2 if args.criterion == "AC-2" else check_ac7
        problems, summary = check(snapshot, renames, head)
    except InputFailure as error:
        print(f"{args.criterion or 'snapshot'}: not observed: {error}", file=sys.stderr)
        return 2
    for problem in problems:
        print(problem)
    if problems:
        print(f"{args.criterion}: violation: {len(problems)} problem(s); {summary}")
        return 1
    print(f"{args.criterion}: pass: {summary}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
