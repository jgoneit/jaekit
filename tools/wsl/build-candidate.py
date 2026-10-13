#!/usr/bin/env python3
"""Prepare a local Linux amd64 candidate archive from this source checkout.

Requires Git, Python 3, and the Go version in go.mod. Creates only candidate
files in a new output directory; never installs, publishes, tags, or runs ha.
This build is not evidence of WSL2 installation or agent Skill execution.
"""
from __future__ import annotations

import argparse
import gzip
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile


ROOT = Path(__file__).resolve().parents[2]
VERSION = "0.0.0-wsl-candidate"
NAME = f"ha_{VERSION}_linux_amd64"
PUBLIC_FILES = (
    "LICENSE", "README.md", "README.en.md",
    "tools/check-result-reference.py", "tools/wsl/check-environment.sh",
    "examples/check-result/README.md", "examples/check-result/declaration.json",
    "examples/check-result/reference.json", "examples/check-result/input.txt",
)
PUBLIC_DIRECTORIES = ("guides", "contracts", "assets")
EXCLUDED_NAMES = {".git", "__pycache__", "node_modules", ".DS_Store"}


def git(*arguments: str) -> str:
    return subprocess.run(["git", *arguments], cwd=ROOT, check=True,
                          capture_output=True, text=True).stdout.strip()


def copy_public(source: Path, destination: Path):
    if source.is_symlink():
        raise ValueError(f"candidate input must not be a link: {source.relative_to(ROOT)}")
    if source.is_dir():
        destination.mkdir(parents=True, exist_ok=True)
        for child in sorted(source.iterdir()):
            if child.name not in EXCLUDED_NAMES and child.suffix not in {".pyc", ".pyo"}:
                copy_public(child, destination / child.name)
    elif source.is_file():
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, destination)
        destination.chmod(0o755 if source.stat().st_mode & 0o111 else 0o644)
    else:
        raise ValueError(f"candidate input is missing or not a regular file: {source.relative_to(ROOT)}")


def public_metadata(member: tarfile.TarInfo):
    member.uid = member.gid = 0
    member.uname = member.gname = ""
    member.mtime = 0
    return member


def build(output: Path):
    commit = git("rev-parse", "HEAD")
    dirty_before = bool(git("status", "--porcelain=v1", "--untracked-files=normal"))
    with tempfile.TemporaryDirectory(prefix="jaekit-wsl-candidate-build-") as temporary:
        package = Path(temporary) / NAME
        package.mkdir()
        for relative in (*PUBLIC_FILES, *PUBLIC_DIRECTORIES):
            copy_public(ROOT / relative, package / relative)
        environment = dict(os.environ, GOOS="linux", GOARCH="amd64", CGO_ENABLED="0",
                           GOFLAGS=" ", GOWORK="off")
        subprocess.run([
            "go", "build", "-mod=readonly", "-trimpath", "-buildvcs=false",
            "-ldflags", f"-s -w -X main.version={VERSION}",
            "-o", str(package / "ha"), "./cmd/ha",
        ], cwd=ROOT, env=environment, check=True)
        if git("rev-parse", "HEAD") != commit:
            raise ValueError("source commit changed during build; retry from a stable checkout")
        dirty_after = bool(git("status", "--porcelain=v1", "--untracked-files=normal"))
        archive = output / f"{NAME}.tar.gz"
        with archive.open("xb") as raw:
            with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as compressed:
                with tarfile.open(fileobj=compressed, mode="w", format=tarfile.USTAR_FORMAT) as bundle:
                    bundle.add(package, arcname=NAME, filter=public_metadata)
        digest = hashlib.sha256(archive.read_bytes()).hexdigest()
        (output / "checksums.txt").write_text(f"{digest}  {archive.name}\n", encoding="utf-8")
        provenance = {
            "kind": "local-source-candidate",
            "version": VERSION,
            "target": "linux/amd64",
            "source_commit": commit,
            "source_dirty": dirty_before or dirty_after,
            "source_scope": "live working tree; commit identifies its base, not uncommitted contents",
            "archive": archive.name,
            "sha256": digest,
            "observations": {
                "wsl2_installation": "not observed by this builder",
                "core_execution": "not observed by this builder",
                "host_skill_execution": "not observed by this builder",
            },
            "publication": "not published; no release or tag created",
        }
        (output / "candidate-provenance.json").write_text(
            json.dumps(provenance, indent=2) + "\n", encoding="utf-8")
    print(f"Candidate files: {output}")
    print(f"Source commit: {commit}; dirty: {str(provenance['source_dirty']).lower()}")
    print(f"Archive version: {VERSION}; target: linux/amd64")
    print("Build only: no Core execution, WSL2 installation, host Skill execution, or publication.")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path,
                        help="new output directory outside the checkout; default: a temporary directory")
    args = parser.parse_args()
    output = None
    try:
        for command in ("git", "go"):
            if not shutil.which(command):
                raise ValueError(f"{command} is required for a source candidate build")
        for relative in PUBLIC_FILES:
            if not (ROOT / relative).is_file() or (ROOT / relative).is_symlink():
                raise ValueError(f"required candidate input is missing or linked: {relative}")
        if args.output is None:
            output = Path(tempfile.mkdtemp(prefix="jaekit-wsl-candidate-"))
        else:
            target = args.output.expanduser().resolve()
            if target == ROOT or ROOT in target.parents:
                raise ValueError("candidate output must be outside the source checkout")
            target.mkdir(exist_ok=False)
            output = target
        build(output)
    except (OSError, ValueError, subprocess.CalledProcessError) as error:
        if output is not None:
            shutil.rmtree(output)
        print(f"Candidate build failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
