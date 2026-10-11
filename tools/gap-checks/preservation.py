#!/usr/bin/env python3
"""Public checks and an explicit, read-only checkout retention audit."""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import sys


class RetentionProblem(Exception):
    def __init__(self, kind, message, status=2):
        self.kind, self.status = kind, status
        super().__init__(message)


def read_bytes(path, what):
    try:
        return path.read_bytes()
    except ValueError:
        raise RetentionProblem("invalid_input", what + " has an invalid path") from None
    except FileNotFoundError:
        raise RetentionProblem("missing", what + " is missing") from None
    except OSError:
        raise RetentionProblem("unreadable", what + " cannot be read") from None


def decode(raw, what):
    try:
        return json.loads(raw)
    except (ValueError, UnicodeError):
        raise RetentionProblem("invalid_input", what + " is not valid JSON") from None


def digest_value(value):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value) is not None


def checkout_git_dir(root):
    env = dict(os.environ)
    for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE"):
        env.pop(key, None)
    try:
        result = subprocess.run(["git", "-C", str(root), "rev-parse", "--absolute-git-dir"],
                                env=env, check=True, capture_output=True, timeout=30)
        path = Path(os.fsdecode(result.stdout[:-1] if result.stdout.endswith(b"\n") else result.stdout))
    except (OSError, subprocess.SubprocessError, ValueError):
        raise RetentionProblem("git_lookup", "selected checkout Git directory is unavailable") from None
    if not path.is_absolute() or not path.is_dir():
        raise RetentionProblem("git_lookup", "selected checkout returned an invalid Git directory")
    return path


def audit_retention(config):
    if not isinstance(config, dict) or not isinstance(config.get("checkout"), str):
        raise RetentionProblem("invalid_input", "retention needs an absolute checkout")
    root = Path(config["checkout"])
    entries = config.get("documents")
    if not root.is_absolute() or not root.is_dir() or not isinstance(entries, dict) or not entries:
        raise RetentionProblem("invalid_input", "retention needs an existing absolute checkout and nonempty document map")
    for name, expected in entries.items():
        if not isinstance(name, str) or not name or Path(name).is_absolute() or ".." in Path(name).parts or not digest_value(expected):
            raise RetentionProblem("invalid_input", "invalid retention document entry")
    documents, logs, git_dir = 0, 0, None
    for name, expected in entries.items():
        path = root / name
        raw = read_bytes(path, "retained document")
        if hashlib.sha256(raw).hexdigest() != expected:
            raise RetentionProblem("document_digest", "a retained document changed", 1)
        documents += 1
        if path.name != "runs.jsonl":
            continue
        for line in raw.splitlines():
            record = decode(line, "retained record")
            if not isinstance(record, dict):
                raise RetentionProblem("invalid_input", "retained record must be an object")
            name = record.get("output_path")
            if name is None or name == "":
                if record.get("output_digest") not in (None, ""):
                    raise RetentionProblem("invalid_input", "recorded output digest has no path")
                continue
            if not isinstance(name, str) or not digest_value(record.get("output_digest")):
                raise RetentionProblem("invalid_input", "recorded output needs a path and SHA-256")
            output_path = Path(name)
            if not output_path.is_absolute() and output_path.parts and output_path.parts[0] == ".git":
                if ".." in output_path.parts or len(output_path.parts) < 2:
                    raise RetentionProblem("invalid_input", "invalid logical Git output path")
                if git_dir is None:
                    git_dir = checkout_git_dir(root)
                output_path = git_dir.joinpath(*output_path.parts[1:])
            elif not output_path.is_absolute():
                output_path = root / output_path
            output = read_bytes(output_path, "selected checkout recorded output")
            if hashlib.sha256(output).hexdigest() != record["output_digest"]:
                raise RetentionProblem("output_digest", "selected checkout recorded output changed", 1)
            logs += 1
    return documents, logs


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--retention", type=Path)
    parser.add_argument("--retention-only", action="store_true", help="audit explicit local input without public release checks")
    args = parser.parse_args(argv)
    if args.retention_only and not args.retention:
        parser.error("--retention-only requires --retention")
    if args.retention:
        try:
            config = decode(read_bytes(args.retention, "retention input"), "retention input")
            documents, logs = audit_retention(config)
        except RetentionProblem as error:
            print(f"FAIL: retention {error.kind}: {error}", file=sys.stderr)
            return error.status
        print(f"PASS: retained local documents {documents}, recorded outputs {logs}; selected checkout only")
    if args.retention_only:
        return 0
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
    for key in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
        env.pop(key, None)
    return subprocess.run([sys.executable, "-m", "unittest", "discover", "-s",
                           "tools/review-checks", "-p", "test_preservation.py"], env=env).returncode


if __name__ == "__main__":
    raise SystemExit(main())
