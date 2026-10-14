#!/usr/bin/env python3
"""Bounded direct regressions; never claims native Windows execution."""
from __future__ import annotations
import importlib.util
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]

def main() -> int:
    spec = importlib.util.spec_from_file_location("shared_verify", ROOT / "tools/verify.py")
    verify = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(verify)
    env = verify.full_go_environment(dict(os.environ))
    for key in ("GOOS", "GOARCH"):
        env.pop(key, None)
    def run(*argv, selected_env=None):
        print("RUN:", " ".join(map(str, argv)), flush=True)
        subprocess.run(list(map(str, argv)), cwd=ROOT, env=selected_env or env, check=True, timeout=450)
    run("go", "test", "-mod=readonly", "-count=1", "./...")
    with tempfile.TemporaryDirectory(prefix="jaekit-windows-local-") as tmp:
        out = Path(tmp).resolve()
        for target in (*verify.TARGETS, "windows/amd64"):
            platform, arch = target.split("/")
            build_env = dict(env, GOOS=platform, GOARCH=arch, CGO_ENABLED="0")
            run("go", "build", "-mod=readonly", "-o", out / target.replace("/", "-"), "./cmd/ha", selected_env=build_env)
        for package in ("run", "record", "evidence"):
            run("go", "test", "-c", "-o", out / (package + ".test.exe"), "./internal/" + package,
                selected_env=dict(env, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0"))
        run("go", "test", "-c", "-o", out / "ha.test.exe", "./cmd/ha",
            selected_env=dict(env, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0"))
        run("go", "build", "-mod=readonly", "-o", out / "ha", "./cmd/ha")
        run(out / "ha", "--version")
        run(out / "ha", "capabilities", "--format", "json")
    for script in ("tools/check_public_tree.py", "tools/check_public_docs.py"):
        run(sys.executable, script)
    for suite in ("tools/release", "tools/windows"):
        run(sys.executable, "-m", "unittest", "discover", "-s", suite, "-p", "test_*.py")
    print("PASS: local regressions and cross-compilation; real Windows remains unverified")
    return 0

if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, subprocess.SubprocessError) as error:
        print(f"FAIL: local checks: {error}", file=sys.stderr)
        raise SystemExit(1)
