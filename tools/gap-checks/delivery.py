#!/usr/bin/env python3
"""Read-only verification of public issue and review delivery for PR #8.

This explicit acceptance check uses authenticated GitHub reads; ordinary CI
does not run it. It never creates issues, comments, or review resolutions.
"""
import json
import subprocess
import sys

REPOSITORY = "jgoneit/jaekit"
REVIEWS = (4237909183, 4237909186, 4237909190, 4237909195,
           4237909197, 4237956221, 4237956224, 4237956231)
ISSUES = {9: 4237909197, 10: 4237909195}


def api(endpoint):
    return json.loads(subprocess.check_output(["gh", "api", endpoint], text=True, timeout=60))


def main():
    head = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
    prefix = "repos/" + REPOSITORY
    pr = api(prefix + "/pulls/8")
    if pr["head"]["sha"] != head:
        raise ValueError("remote PR head does not match the checked code")
    comments = api(prefix + "/pulls/8/comments?per_page=100")
    for parent in REVIEWS:
        replies = [c for c in comments if c.get("in_reply_to_id") == parent
                   and c.get("user", {}).get("login") == "jgoneit" and head in c.get("body", "")]
        if not replies:
            raise ValueError(f"review {parent} has no reply bound to this commit")
    for number, review in ISSUES.items():
        issue = api(prefix + f"/issues/{number}")
        if not issue.get("body") or f"/pull/8#discussion_r{review}" not in issue["body"]:
            raise ValueError(f"issue {number} is not linked to its review")
    for number in (6, 7):
        comments = api(prefix + f"/issues/{number}/comments?per_page=100")
        if not any(c.get("user", {}).get("login") == "jgoneit" and head in c.get("body", "")
                   for c in comments):
            raise ValueError(f"issue {number} lacks the current follow-up")
    print("PASS: eight review replies, two issue records and existing-issue follow-ups match the PR head")
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        print("FAIL: delivery could not be verified: " + str(error), file=sys.stderr)
        raise SystemExit(1)
