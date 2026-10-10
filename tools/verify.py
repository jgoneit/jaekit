#!/usr/bin/env python3
"""Run shared local/CI checks: python3 tools/verify.py [all|go|docs]."""
from __future__ import annotations

import argparse
import os
from pathlib import Path
import shlex
import shutil
import subprocess
import sys
import tempfile


ROOT = Path(__file__).resolve().parents[1]
TARGETS = ("linux/amd64", "linux/arm64", "darwin/amd64", "darwin/arm64")


class CheckFailed(Exception):
    pass


def run(label: str, command: list[str], env: dict[str, str], *, formatting=False):
    print(f"RUN: {label}\n  {shlex.join(command)}", flush=True)
    try:
        result = subprocess.run(command, cwd=ROOT, env=env, text=True,
                                stdout=subprocess.PIPE if formatting else None)
    except OSError as error:
        raise CheckFailed(f"{label}: could not execute {command[0]} ({error.strerror})") from None
    if formatting and result.stdout:
        print(result.stdout, end="", flush=True)
    if result.returncode:
        raise CheckFailed(f"{label}: exit {result.returncode}")
    if formatting and result.stdout.strip():
        raise CheckFailed(f"{label}: files listed above need gofmt; no files were rewritten")
    print(f"PASS: {label}", flush=True)


def verify_go(env: dict[str, str]):
    run("Go formatting", ["gofmt", "-l", "."], env, formatting=True)
    run("Go vet", ["go", "vet", "-mod=readonly", "./..."], env)
    run("Go tests", ["go", "test", "-mod=readonly", "./..."], env)
    # All build outputs live outside the checkout, including on failed checks.
    with tempfile.TemporaryDirectory(prefix="jaekit-verify-") as temporary:
        output = Path(temporary)
        for target in TARGETS:
            goos, goarch = target.split("/")
            run(f"Build {target}",
                ["go", "build", "-mod=readonly", "-o", str(output / f"ha-{goos}-{goarch}"), "./cmd/ha"],
                dict(env, GOOS=goos, GOARCH=goarch, CGO_ENABLED="0"))
        binary = output / "ha"
        run("Build native smoke binary",
            ["go", "build", "-mod=readonly", "-o", str(binary), "./cmd/ha"], env)
        run("CLI version", [str(binary), "--version"], env)
        run("CLI help", [str(binary), "--help"], env)


def verify_docs(env: dict[str, str]):
    run("Public file boundary", [sys.executable, "tools/check_public_tree.py"], env)
    run("Public documentation", [sys.executable, "tools/check_public_docs.py"], env)
    for label, directory in (("Public checker regressions", "public-checks"),
                             ("Workflow regressions", "workflow-checks")):
        run(label, [sys.executable, "-m", "unittest", "discover", "-s", f"tools/{directory}",
                    "-p", "test_*.py"], env)


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("group", nargs="?", choices=("all", "go", "docs"), default="all")
    args = parser.parse_args()
    required = ("git", "go", "gofmt") if args.group in ("all", "go") else ("git",)
    missing = [tool for tool in required if shutil.which(tool) is None]
    if missing:
        for tool in missing:
            print(f"FAIL: missing required tool: {tool}", file=sys.stderr)
        return 1
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GOTOOLCHAIN="local")
    # A user's cross-compilation settings must not turn the native smoke binary
    # into a binary that cannot run on this host. Targets set these explicitly.
    env.pop("GOOS", None)
    env.pop("GOARCH", None)
    try:
        if args.group in ("all", "go"):
            verify_go(env)
        if args.group in ("all", "docs"):
            verify_docs(env)
    except CheckFailed as error:
        print(f"FAIL: {error}", file=sys.stderr)
        return 1
    except KeyboardInterrupt:
        print("FAIL: verification interrupted", file=sys.stderr)
        return 130
    print(f"PASS: {args.group} verification", flush=True)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
