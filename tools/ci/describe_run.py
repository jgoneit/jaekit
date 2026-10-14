#!/usr/bin/env python3
"""Print selected public CI source and environment metadata, without modifying it."""
import argparse
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[2]
SHA = re.compile(r"[0-9a-f]{40}\Z")
SHELLS = {
    "bash": ["bash", "--version"],
    "powershell": ["powershell", "-NoProfile", "-NonInteractive", "-Command",
                   "$PSVersionTable.PSVersion.ToString()"],
    "pwsh": ["pwsh", "-NoProfile", "-NonInteractive", "-Command",
             "$PSVersionTable.PSVersion.ToString()"],
}


def checked_sha(value):
    if not isinstance(value, str) or not SHA.fullmatch(value):
        raise ValueError("missing or invalid source commit")
    return value


def context(env, event, source_sha, shell, os_name, arch):
    source_sha = checked_sha(source_sha)
    if source_sha != checked_sha(env.get("GITHUB_SHA")):
        raise ValueError("checkout differs from the workflow event commit")
    name = env.get("GITHUB_EVENT_NAME")
    if name not in ("pull_request", "push"):
        raise ValueError("unsupported CI event")
    head_sha, base_sha = source_sha, None
    if name == "pull_request":
        try:
            head_sha = checked_sha(event["pull_request"]["head"]["sha"])
            base_sha = checked_sha(event["pull_request"]["base"]["sha"])
        except (KeyError, TypeError) as error:
            raise ValueError("pull request source metadata is missing") from error
    runner_os, runner_arch = env.get("RUNNER_OS"), env.get("RUNNER_ARCH")
    if runner_os not in ("Linux", "macOS", "Windows") or runner_arch not in ("X64", "ARM64"):
        raise ValueError("runner OS or architecture is missing or unsupported")
    if {"Linux": "Linux", "macOS": "Darwin", "Windows": "Windows"}[runner_os] != os_name:
        raise ValueError("actual OS differs from runner metadata")
    if arch.lower() not in {"X64": ("x86_64", "amd64"), "ARM64": ("arm64", "aarch64")}[runner_arch]:
        raise ValueError("actual CPU differs from runner metadata")
    return {
        "schema": "jaekit-ci-context/v1", "source_sha": source_sha,
        "event": name, "head_sha": head_sha, "base_sha": base_sha,
        "os": os_name, "arch": arch, "shell": shell,
        "runner_os": runner_os, "runner_arch": runner_arch,
        "image_os": env.get("ImageOS"), "image_version": env.get("ImageVersion"),
    }


def output(argv):
    result = subprocess.run(argv, cwd=ROOT, check=True, capture_output=True,
                            text=True, timeout=30)
    value = result.stdout.strip()
    if not value:
        raise ValueError("a metadata command returned no output")
    return value


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--shell", choices=tuple(SHELLS), required=True)
    args = parser.parse_args()
    try:
        event_path = os.environ.get("GITHUB_EVENT_PATH")
        if not event_path:
            raise ValueError("GitHub event metadata is unavailable")
        event = json.loads(Path(event_path).read_text(encoding="utf-8"))
        source_sha = output(["git", "rev-parse", "HEAD"])
        shell = args.shell + ": " + output(SHELLS[args.shell]).splitlines()[0]
        result = context(os.environ, event, source_sha, shell,
                         platform.system(), platform.machine())
    except (OSError, ValueError, subprocess.SubprocessError) as error:
        # Never dump the event payload, command output, or inherited environment.
        print("FAIL: CI context: " + type(error).__name__, file=sys.stderr)
        return 1
    print("JAEKIT_CI_CONTEXT " + json.dumps(result, sort_keys=True, separators=(",", ":")))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
