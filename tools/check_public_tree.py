#!/usr/bin/env python3
"""Check the publication boundary of tracked repository files."""

from pathlib import Path
import re
import subprocess
import sys


ROOT = Path(__file__).resolve().parents[1]
PUBLIC_DIRECTORIES = {
    ".agents", ".claude-plugin", ".github", "assets", "cmd", "contracts",
    "evals", "examples", "guides", "internal", "plugins", "testdata", "tools",
}
PUBLIC_FILES = {
    ".gitignore", "AGENTS.md", "CLAUDE.md", "LICENSE", "README.md",
    "README.en.md", "go.mod", "go.sum",
}
PRIVATE_REFERENCES = [
    # Sibling repositories hold private history; match https, SSH, and link endings.
    re.compile(r"github\.com[/:]jgoneit/jaekit-[\w.-]+"),
    re.compile(r"https?://github\.com/jgoneit/jaekit/(?:blob|commit)/[0-9a-f]{40}"),
    re.compile(r"/(?:Users|home)/(?!runner/|user/|example/)[^\s/]+/"),
]


def main():
    paths = subprocess.check_output(
        ["git", "ls-files", "-z"], cwd=ROOT
    ).decode().split("\0")
    failures = []
    for name in filter(None, paths):
        path = Path(name)
        if name not in PUBLIC_FILES and path.parts[0] not in PUBLIC_DIRECTORIES:
            failures.append(f"{name}: outside the public file allowlist")
        if name == "evals/spec-cases/results.md":
            failures.append(f"{name}: keep evaluation results outside the public tree")
        if {"__pycache__", "node_modules"}.intersection(path.parts) or path.suffix in {".pyc", ".pyo"}:
            failures.append(f"{name}: generated cache is not a publication asset")
        full_path = ROOT / path
        if full_path.is_symlink():
            failures.append(f"{name}: publish ordinary files, not external symlinks")
            continue
        try:
            text = full_path.read_text()
        except UnicodeDecodeError:
            continue
        if name.startswith("testdata/"):
            continue  # Synthetic fixtures may intentionally contain sample paths.
        for pattern in PRIVATE_REFERENCES:
            if pattern.search(text):
                failures.append(f"{name}: contains a private or historical reference")
    if failures:
        print("\n".join(failures), file=sys.stderr)
        return 1
    print("Public file boundary passed.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
