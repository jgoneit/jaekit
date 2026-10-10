#!/usr/bin/env python3
"""Check the public boundary of this change and the preserved v0.1.3 records.

Usage: python3 tools/goal-checks/release-0-1-3-followups/preserved.py BASE_COMMIT

It runs the public file and document checks, scans the lines and commit
messages added since BASE_COMMIT for private references, compares the public
v0.1.3 tag and Release with recorded identities, and compares the earlier
local goal's documents and raw logs with recorded digests. It reads only.
"""

import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys

CONFIG = Path(__file__).with_name("preserved.json")
PRIVATE = [
    re.compile(r"/(?:Users|home)/(?!runner/|user/|example/)[^\s/]+/"),
    re.compile("jaekit-" + "legacy"),
    re.compile(r"github\.com[/:]jgoneit/jaekit-[\w.-]+"),
    re.compile(r"(?:codex|claude)(?:-direct)?-host[\w-]*\.json"),
    re.compile(r"evidence-(?:seq|final|before)[\w-]*\.json"),
    re.compile(r"/private/tmp/jaekit-"),
]


class Unavailable(Exception):
    pass


def run(argv, **kwargs):
    proc = subprocess.run(argv, capture_output=True, text=True, timeout=300, **kwargs)
    if proc.returncode != 0:
        raise Unavailable(f"{argv[0]} failed: {proc.stderr.strip()[:300]}")
    return proc.stdout


def digest(data):
    return hashlib.sha256(data).hexdigest()


def public_boundary(base, problems):
    for script in ("tools/check_public_tree.py", "tools/check_public_docs.py"):
        proc = subprocess.run([sys.executable, script], capture_output=True, text=True, timeout=300)
        if proc.returncode != 0:
            problems.append(f"{script} failed: {(proc.stdout + proc.stderr).strip()[:300]}")
    added = [line[1:] for line in run(["git", "diff", "--unified=0", "--no-color", base, "HEAD"]).splitlines()
             if line.startswith("+") and not line.startswith("+++")]
    messages = run(["git", "log", "--format=%B", f"{base}..HEAD"]).splitlines()
    for kind, lines in (("added line", added), ("commit message", messages)):
        for line in lines:
            for pattern in PRIVATE:
                if pattern.search(line):
                    problems.append(f"{kind} has a private reference: {line.strip()[:120]}")


def public_release(expected, problems):
    repo, tag = expected["repository"], expected["tag"]
    commit = json.loads(run(["gh", "api", f"repos/{repo}/commits/{tag}"]))
    if commit.get("sha") != expected["commit"]:
        problems.append(f"{tag} now names commit {commit.get('sha')}")
    release = json.loads(run(["gh", "api", f"repos/{repo}/releases/tags/{tag}"]))
    if release.get("id") != expected["release_id"]:
        problems.append(f"{tag} Release id changed")
    if digest((release.get("body") or "").encode("utf-8")) != expected["body_sha256"]:
        problems.append(f"{tag} Release body changed")
    assets = sorted(({k: a.get(k) for k in ("digest", "id", "name", "size")} for a in release.get("assets", [])),
                    key=lambda a: a["name"])
    if assets != expected["assets"]:
        problems.append(f"{tag} Release assets changed")


def local_goal(expected, problems):
    common = Path(run(["git", "rev-parse", "--path-format=absolute", "--git-common-dir"]).strip())
    goal = common.parent / expected["path"]
    for name, sha in expected["files"].items():
        try:
            if digest((goal / name).read_bytes()) != sha:
                problems.append(f"{expected['path']}/{name} changed")
        except OSError:
            problems.append(f"{expected['path']}/{name} is missing")
    try:
        records = [json.loads(line) for line in (goal / "runs.jsonl").read_text().splitlines()]
    except (OSError, ValueError):
        return
    for record in records:
        path = record.get("output_path")
        if not path:
            continue
        copies = [common / base / path.removeprefix(".git/") for base in expected["log_dirs"]]
        present = [copy for copy in copies if copy.is_file()]
        if not present:
            problems.append(f"raw log of seq {record.get('seq')} is missing")
        for copy in present:
            if digest(copy.read_bytes()) != record.get("output_digest"):
                problems.append(f"raw log of seq {record.get('seq')} changed")


def main(argv):
    if len(argv) != 2 or not re.fullmatch(r"[0-9a-f]{40}", argv[1]):
        print("usage: preserved.py BASE_COMMIT", file=sys.stderr)
        return 2
    config = json.loads(CONFIG.read_text(encoding="utf-8"))
    problems = []
    try:
        public_boundary(argv[1], problems)
        public_release(config["release"], problems)
        local_goal(config["local_goal"], problems)
    except (Unavailable, subprocess.TimeoutExpired, ValueError) as error:
        print(f"preservation could not be observed: {error}", file=sys.stderr)
        return 2
    for problem in problems:
        print(problem)
    print("preserved" if not problems else f"{len(problems)} preservation problems")
    return 1 if problems else 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
