#!/usr/bin/env python3
"""Check the public boundary of this change and the preserved v0.1.3 release.

Usage: python3 tools/goal-checks/release-0-1-3-followups/preserved.py
       python3 tools/goal-checks/release-0-1-3-followups/preserved.py --history --base SHA --head SHA

The default checks current public files and the public release identity, with
no dependency on earlier commits. The explicit history mode instead audits
added lines and commit messages in the named range. Missing history is
unverified; neither mode fetches Git objects or reads private goal material.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys

CONFIG = Path(__file__).with_name("preserved.json")
# Literal fragments are split so that these definitions, which are added
# lines too, do not match themselves.
PRIVATE = [
    re.compile(r"/(?:Users|home)/(?!runner/|user/|example/)[^\s/]+/"),
    re.compile("jaekit-" + "legacy"),
    re.compile(r"github\.com[/:]jgoneit/jaekit-[\w.-]+"),
    re.compile(r"(?:codex|claude)(?:-direct)?-host[\w-]*\.json"),
    re.compile(r"evidence-(?:seq|final|before)[\w-]*\.json"),
    re.compile("/private/tmp/" + "jaekit-"),
    re.compile(re.escape("docs/specs/" + "release-" + "0-1-3")),
    re.compile(re.escape("docs%2Fspecs%2F" + "release-" + "0-1-3")),
    re.compile(re.escape("ha/" + "release-" + "0-1-3")),
    re.compile(re.escape("worktrees/" + "release-" + "0-1-3")),
    re.compile(r"\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b"),
    re.compile(r"\brollout-\d{4}-\d{2}-\d{2}T"),
    re.compile(re.escape("." + "claude/projects/")),
    re.compile(re.escape("." + "codex/sessions/")),
]


class Unavailable(Exception):
    pass


def run(argv, **kwargs):
    # Git's partial-clone lazy fetching must not turn an audit into a download.
    env = dict(os.environ, GIT_NO_LAZY_FETCH="1", GIT_TERMINAL_PROMPT="0", GIT_NO_REPLACE_OBJECTS="1")
    proc = subprocess.run(argv, capture_output=True, text=True, timeout=300, env=env, **kwargs)
    if proc.returncode != 0:
        raise Unavailable(f"{argv[0]} could not read the requested input")
    return proc.stdout


def digest(data):
    return hashlib.sha256(data).hexdigest()


def public_boundary(problems):
    for script in ("tools/check_public_tree.py", "tools/check_public_docs.py"):
        proc = subprocess.run([sys.executable, script], capture_output=True, text=True, timeout=300)
        if proc.returncode != 0:
            problems.append(f"{script} failed: {(proc.stdout + proc.stderr).strip()[:300]}")


def history_boundary(base, head, problems):
    if not all(re.fullmatch(r"[0-9a-f]{40}", ref or "") for ref in (base, head)):
        raise Unavailable("history audit requires explicit full base and head commit ids")
    for ref in (base, head):
        run(["git", "cat-file", "-e", ref + "^{commit}"])
    commits = set(run(["git", "rev-list", base + ".." + head]).splitlines())
    # Reaching the base is not enough for a merge: another parent may stop at
    # a shallow boundary, hiding commits that also belong to the audit range.
    if run(["git", "rev-parse", "--is-shallow-repository"]).strip() == "true":
        run(["git", "merge-base", "--is-ancestor", base, head])
        shallow = Path(run(["git", "rev-parse", "--git-path", "shallow"]).strip())
        if commits.intersection(shallow.read_text(encoding="ascii").splitlines()):
            raise Unavailable("history range crosses a shallow boundary")
    added = [line[1:] for line in run(["git", "diff", "--no-ext-diff", "--no-textconv", "--unified=0", "--no-color", base, head, "--"]).splitlines()
             if line.startswith("+") and not line.startswith("+++")]
    messages = run(["git", "log", "--format=%B", base + ".." + head, "--"]).splitlines()
    for kind, lines in (("added line", added), ("commit message", messages)):
        for line in lines:
            for pattern in PRIVATE:
                if pattern.search(line):
                    problems.append(f"{kind} contains a forbidden private reference")


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


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--history", action="store_true", help="audit only the explicit commit range")
    parser.add_argument("--base", help="full base commit id, required with --history")
    parser.add_argument("--head", help="full head commit id, required with --history")
    args = parser.parse_args(argv[1:])
    if (args.base or args.head) and not args.history:
        parser.error("--base and --head belong to the separate --history audit")
    problems = []
    try:
        if args.history:
            history_boundary(args.base, args.head, problems)
        else:
            config = json.loads(CONFIG.read_text(encoding="utf-8"))
            public_boundary(problems)
            public_release(config["release"], problems)
    except (Unavailable, subprocess.TimeoutExpired, OSError, ValueError, KeyError) as error:
        message = str(error) if isinstance(error, Unavailable) else "required audit input is unavailable"
        print(f"UNVERIFIED: {message}", file=sys.stderr)
        return 2
    for problem in problems:
        print(problem)
    label = "history audit" if args.history else "public preservation"
    print(f"PASS: {label}" if not problems else f"FAIL: {label}: {len(problems)} problems")
    return 1 if problems else 0


if __name__ == "__main__":
    raise SystemExit(main(sys.argv))
