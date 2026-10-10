#!/usr/bin/env python3
"""Public preservation checks; optional explicitly supplied local retention audit.

No private files are searched automatically. The optional JSON input supplies
an absolute checkout path and a mapping of relative document paths to SHA-256.
"""
import argparse
import hashlib
import json
import os
import re
from pathlib import Path
import subprocess
import sys


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--retention", type=Path)
    args = parser.parse_args()
    if args.retention:
        config = json.loads(args.retention.read_text())
        root = Path(config["checkout"])
        entries = config["documents"]
        if not root.is_absolute() or not root.is_dir() or not isinstance(entries, dict) or not entries:
            raise ValueError("retention needs an existing absolute checkout and nonempty document map")
        for name, expected in entries.items():
            relative = Path(name)
            if relative.is_absolute() or ".." in relative.parts or not re.fullmatch(r"[0-9a-f]{64}", expected):
                raise ValueError("invalid retention document entry")
        documents, logs = 0, 0
        for name, expected in config["documents"].items():
            path = root / name
            raw = path.read_bytes()
            if hashlib.sha256(raw).hexdigest() != expected:
                print("FAIL: a retained local document changed", file=sys.stderr)
                return 1
            documents += 1
            if path.name == "runs.jsonl":
                for line in raw.splitlines():
                    record = json.loads(line)
                    if record.get("output_path"):
                        output = (root / record["output_path"]).read_bytes()
                        if hashlib.sha256(output).hexdigest() != record["output_digest"]:
                            print("FAIL: a retained local output changed", file=sys.stderr)
                            return 1
                        logs += 1
        print(f"PASS: retained local documents {documents}, recorded outputs {logs}")
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
    for key in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
        env.pop(key, None)
    return subprocess.run([sys.executable, "-m", "unittest", "discover", "-s",
                           "tools/review-checks", "-p", "test_preservation.py"], env=env).returncode


if __name__ == "__main__":
    raise SystemExit(main())
