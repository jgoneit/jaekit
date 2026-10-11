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
    # These offline boundary regressions include real Go fixtures and local
    # bash/zsh processes. Keep them in the Go group so docs-only verification
    # does not acquire a Go dependency. Preservation checks are separate:
    # they query a release and invoke this entry point themselves.
    for filename in ("test_observation.py", "test_installation.py", "test_producers.py"):
        run(f"Review boundaries ({filename})",
            [sys.executable, "-m", "unittest", "discover", "-s", "tools/review-checks",
             "-p", filename], env)
    for filename in ("test_observation.py", "test_history_results.py", "test_docs_environment.py"):
        run(f"Review gap regressions ({filename})",
            [sys.executable, "-m", "unittest", "discover", "-s", "tools/gap-checks",
             "-p", filename], env)
    run("Producer semantic regressions (test_semantics.py)",
        [sys.executable, "-m", "unittest", "discover", "-s", "tools/producer-checks",
         "-p", "test_semantics.py"], env)
    for filename in ("integration_evidence_boundaries.py", "integration_core_observation.py", "integration_direct_install.py", "integration_product_source.py"):
        run(f"Release evidence boundaries ({filename})",
            [sys.executable, "-m", "unittest", "discover", "-s", "tools/release-checks",
             "-p", filename], env)
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
                             ("Workflow regressions", "workflow-checks"),
                             ("Release archive regressions", "release"),
                             ("Goal check regressions", "goal-checks"),
                             ("Release acceptance checker regressions", "release-checks")):
        run(label, [sys.executable, "-m", "unittest", "discover", "-s", f"tools/{directory}",
                    "-p", "test_*.py"], env)


def prepare_shells(env: dict[str, str]):
    """Check the exact executables that the Go group's shell fixtures will use."""
    for name in ("bash", "zsh"):
        selected = shutil.which(name, path=env.get("PATH"))
        if selected is None:
            raise CheckFailed(f"missing required tool: {name} (Go shell regressions)")
        path = os.path.abspath(selected)
        try:
            result = subprocess.run([path, "--version"], env=env, timeout=5,
                                    stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        except (OSError, subprocess.TimeoutExpired):
            raise CheckFailed(f"required tool {name} cannot execute: {path} (Go shell regressions)") from None
        if result.returncode:
            raise CheckFailed(f"required tool {name} exited {result.returncode}: {path} (Go shell regressions)")
        env["JAEKIT_TEST_" + name.upper()] = path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("group", nargs="?", choices=("all", "go", "docs"), default="all")
    args = parser.parse_args()
    required = ("git", "go", "gofmt", "bash", "zsh") if args.group in ("all", "go") else ("git",)
    missing = [tool for tool in required if shutil.which(tool) is None]
    if missing:
        for tool in missing:
            scope = " (Go shell regressions)" if tool in ("bash", "zsh") else ""
            print(f"FAIL: missing required tool: {tool}{scope}", file=sys.stderr)
        return 1
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GOTOOLCHAIN="local")
    # A user's cross-compilation settings must not turn the native smoke binary
    # into a binary that cannot run on this host. Targets set these explicitly.
    env.pop("GOOS", None)
    env.pop("GOARCH", None)
    try:
        if args.group in ("all", "go"):
            prepare_shells(env)
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
