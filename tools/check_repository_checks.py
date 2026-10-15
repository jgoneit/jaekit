#!/usr/bin/env python3
"""Refuse two recurring defects in the Python checks under tools/.

Fixed history: a check passes a fixed commit ID to Git. Product checks judge
the current files by their properties, without an earlier commit (AGENTS.md).
A finding is a 7-40 digit lowercase hexadecimal object name, optionally
followed by `:path`, `..`, `^` or `~` (`7476c87:README.md`, `7476c87..HEAD`),
written in the arguments of a Git call or bound to a name used there. A Git
call is a call whose function name ends in `git`, or whose positional or
`args=` arguments (or an argument list) contain the string "git"; other
keyword values such as `input` are not Git arguments. Hexadecimal values that never reach
Git, such as SHA-256 digests or synthetic object IDs, and `HEAD` are fine.

Late imports: a module that declares a standard-library import boundary
restores the caller's import path after its bootstrap
(`sys.path[:] = _CALLER_IMPORT_PATH`). Any later import of a module the
bootstrap did not already load (an import inside a function defined during
the bootstrap runs later) could then run a module from the caller's
path. Such an import must sit inside `with trusted_imports():` or in a
function decorated with `@trusted_imports()`; a submodule of a package the
bootstrap loaded resolves through that package and is fine.

In a Git checkout the tracked files are read; in a directory without Git (a
source snapshot or a fixture) every Python file under tools/ is. Prints
`path:line: message` for each finding and exits 1; exits 0 when there is
none, and 2 when a file cannot be listed, read or parsed. This is a static
reading of the source: arguments built elsewhere (an argv variable, a shell
string, a value read from a file or imported from another module), all-decimal
object names and dynamic imports (`importlib.import_module`) are outside it.
"""
from __future__ import annotations

import argparse
import ast
import os
from pathlib import Path
import re
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[1]
OBJECT = re.compile(r"[0-9a-f]{7,40}(?:[:.^~].*)?\Z", re.S)
DECIMAL = re.compile(r"[0-9]+")


class Unreadable(Exception):
    pass


def python_files(root: Path) -> list[str]:
    env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    env["GIT_OPTIONAL_LOCKS"] = "0"

    def git(*args):
        return subprocess.run(["git", "-C", str(root), *args], capture_output=True, env=env, check=False)

    try:
        top = git("rev-parse", "--show-toplevel")
    except OSError:
        top = None
    if top is not None and top.returncode == 0 and Path(os.fsdecode(top.stdout.strip())).resolve() == root.resolve():
        listed = git("ls-files", "-z", "--", "tools")
        if listed.returncode:
            raise Unreadable("tracked files could not be listed")
        names = [os.fsdecode(name) for name in listed.stdout.split(b"\0") if name]
    else:
        names = [path.relative_to(root).as_posix() for path in (root / "tools").rglob("*.py") if path.is_file()]
    return sorted(name for name in names if name.endswith(".py"))


def call_name(node: ast.Call) -> str:
    function = node.func
    if isinstance(function, ast.Name):
        return function.id
    if isinstance(function, ast.Attribute):
        return function.attr
    return ""


def arguments(node: ast.Call):
    """Positional arguments, an `args=` value and their list items: the
    command line. Other keyword values (input, cwd, env) are not Git arguments."""
    for value in [*node.args, *(keyword.value for keyword in node.keywords if keyword.arg == "args")]:
        if isinstance(value, ast.Starred):
            value = value.value
        yield value
        if isinstance(value, (ast.List, ast.Tuple)):
            for item in value.elts:
                yield item.value if isinstance(item, ast.Starred) else item


def is_text(node, value=None):
    return isinstance(node, ast.Constant) and isinstance(node.value, str) and (value is None or node.value == value)


def is_object(value: str) -> bool:
    """A hexadecimal object name; a decimal count such as "1000000" is not."""
    return OBJECT.match(value) is not None and not DECIMAL.fullmatch(value.split(":")[0].split(".")[0])


def bound_objects(tree: ast.Module) -> set[str]:
    """Names assigned a hexadecimal object name anywhere in the module."""
    names = set()
    for node in ast.walk(tree):
        targets, value = [], None
        if isinstance(node, ast.Assign):
            targets, value = node.targets, node.value
        elif isinstance(node, ast.AnnAssign) and node.value is not None:
            targets, value = [node.target], node.value
        if is_text(value) and is_object(value.value):
            names.update(target.id for target in targets if isinstance(target, ast.Name))
    return names


def history_findings(tree: ast.Module):
    names = bound_objects(tree)
    for node in ast.walk(tree):
        if not isinstance(node, ast.Call):
            continue
        values = list(arguments(node))
        if not (call_name(node).lower().endswith("git") or any(is_text(value, "git") for value in values)):
            continue
        for value in values:
            for part in ast.walk(value):
                if is_text(part) and is_object(part.value):
                    yield node.lineno, f"a fixed commit {part.value!r} is passed to Git"
                    break
                if isinstance(part, ast.Name) and part.id in names:
                    yield node.lineno, f"{part.id} holds a fixed commit and is passed to Git"
                    break


def restores_caller_path(node) -> bool:
    if not (isinstance(node, ast.Assign) and len(node.targets) == 1):
        return False
    target = node.targets[0]
    return (isinstance(target, ast.Subscript) and isinstance(target.slice, ast.Slice)
            and isinstance(target.value, ast.Attribute) and target.value.attr == "path"
            and isinstance(target.value.value, ast.Name) and target.value.value.id == "sys"
            and isinstance(node.value, ast.Name) and node.value.id == "_CALLER_IMPORT_PATH")


def imported(node) -> list[str]:
    if isinstance(node, ast.Import):
        return [alias.name for alias in node.names]
    if node.level:
        return ["." * node.level + (node.module or "")]
    return [node.module or ""]


def trusted(call) -> bool:
    if isinstance(call, ast.Call):
        call = call.func
    return (isinstance(call, ast.Name) and call.id == "trusted_imports") or (
        isinstance(call, ast.Attribute) and call.attr == "trusted_imports")


def import_findings(tree: ast.Module):
    restore = next((node.lineno for node in tree.body if restores_caller_path(node)), None)
    if restore is None:
        return
    loaded = set()

    def executed(node):
        # Imports the bootstrap runs itself; a function body runs later.
        if isinstance(node, (ast.Import, ast.ImportFrom)):
            loaded.update(imported(node))
        if not isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda)):
            for child in ast.iter_child_nodes(node):
                executed(child)

    for node in tree.body:
        if node.lineno >= restore:
            break
        executed(node)
    found = []

    def visit(node, inside_trust, at_module):
        if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef, ast.Lambda)):
            decorators = getattr(node, "decorator_list", [])
            inside_trust, at_module = any(trusted(item) for item in decorators), False
        elif isinstance(node, (ast.With, ast.AsyncWith)) and any(trusted(item.context_expr) for item in node.items):
            for child in node.body:
                visit(child, True, at_module)
            return
        elif isinstance(node, (ast.Import, ast.ImportFrom)) and not inside_trust:
            late = not at_module or node.lineno > restore
            for name in imported(node):
                # A submodule of a package the bootstrap loaded resolves
                # through that package's own path, not the caller's.
                parts = name.split(".")
                if late and not any(".".join(parts[:end]) in loaded for end in range(1, len(parts) + 1)):
                    found.append((node.lineno, f"{name} is imported after the caller's import path is restored, "
                                               "outside trusted_imports()"))
        for child in ast.iter_child_nodes(node):
            visit(child, inside_trust, at_module)

    for node in tree.body:
        visit(node, False, True)
    yield from found


def scan(root: Path) -> list[str]:
    findings = []
    for name in python_files(root):
        try:
            source = (root / name).read_bytes()
            tree = ast.parse(source, filename=name)
        except (OSError, SyntaxError, ValueError) as error:
            raise Unreadable(f"{name}: {error}") from None
        for line, message in sorted({*history_findings(tree), *import_findings(tree)}):
            findings.append(f"{name}:{line}: {message}")
    return findings


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--root", type=Path, default=ROOT)
    args = parser.parse_args(argv)
    try:
        findings = scan(args.root)
    except Unreadable as error:
        print("ERROR: " + str(error), file=sys.stderr)
        return 2
    for finding in findings:
        print(finding)
    return 1 if findings else 0


if __name__ == "__main__":
    raise SystemExit(main())
