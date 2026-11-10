"""Build public gate fixtures from this scenario's temporary repository only."""

import hashlib
import json
from pathlib import Path
import subprocess


def commit(ref):
    return subprocess.check_output(["git", "rev-parse", ref], text=True).strip()


goal = Path("docs/specs/greet")
start = json.loads((goal / "runs.jsonl").read_text().splitlines()[0])
base, work, head = start["base_commit"], commit("HEAD^"), commit("HEAD")
criteria = [
    {"name": path.as_posix(), "sha256": hashlib.sha256(path.read_bytes()).hexdigest()}
    for path in (goal / "SPEC.md", goal / "contract.md")
]


def write_gate(name, ranges):
    rounds = []
    for sequence, (stage, first, last) in enumerate(ranges, 1):
        rounds.append({
            "path": f"docs/reviews/greet/{sequence:02d}-{stage}",
            "stage": stage,
            "sequence": sequence,
            "base": first,
            "head": last,
            "criteria_digest": "0" * 64,
            "criteria": criteria,
            "packet_digest": "0" * 64,
            "config_digest": "0" * 64,
            "requested_reviewers": ["host"],
            "required_reviewers": 1,
            "completed_required": 1,
            "dry_run": False,
            "counted": True,
        })
    gate = {
        "schema": "review-gate/v1",
        "spar_version": "0.1.6",
        "state": "complete",
        "reasons": [],
        "expect_head": head,
        "last_reviewed_head": head,
        "previous_gate_digest": None,
        "rounds": rounds,
        "open": {"defect": 0, "requirement-question": 0},
        "open_findings": [],
        "auto_deferred": 0,
        "growth": {
            "added_lines": {"check": 0, "other": 0},
            "over_hardening": False,
            "rounds": [],
        },
        "written_at": "2026-01-01T00:00:00Z",
    }
    Path(f".git/{name}.json").write_text(json.dumps(gate) + "\n")


write_gate("partial", [("candidate", work, head)])
write_gate("two-candidates", [("candidate", base, work), ("candidate", work, head)])
write_gate("cumulative", [("candidate", base, head)])
write_gate("fixes", [("candidate", base, work), ("fixes", work, head)])
