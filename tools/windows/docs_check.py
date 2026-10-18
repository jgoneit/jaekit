#!/usr/bin/env python3
"""Observe candidate contents and public guidance, never Windows execution."""
from __future__ import annotations
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import zipfile

ROOT = Path(__file__).resolve().parents[2]

def observe():
    required = ["guides/WINDOWS.md", "guides/WINDOWS.en.md"]
    required += ["tools/windows/" + name for name in ("package.py", "install.ps1", "uninstall.ps1", "common.ps1", "acceptance.ps1", "README.md")]
    missing = [name for name in required if not (ROOT / name).is_file()]
    if missing:
        print("Missing candidate behavior:", ", ".join(missing))
        return {"status": "violation", "violation": "native-candidate-missing"}
    for name in required[:2]:
        text = (ROOT / name).read_text(encoding="utf-8")
        for token in ("Windows 11", "x64", "5.1", "PowerShell 7", "CRLF", "Git", "WSL", "uninstall.ps1"):
            if token not in text:
                print(f"Missing public constraint in {name}: {token}")
                return {"status": "violation", "violation": "native-candidate-missing"}
    def run(*args, env=None):
        subprocess.run(list(map(str, args)), cwd=ROOT, env=env, check=True, timeout=120)
    with tempfile.TemporaryDirectory(prefix="jaekit-windows-contents-") as temp:
        output = Path(temp).resolve()
        binary = output / "ha.exe"
        run("go", "build", "-mod=readonly", "-o", binary, "./cmd/ha",
            env=dict(os.environ, GOOS="windows", GOARCH="amd64", CGO_ENABLED="0"))
        run(sys.executable, "tools/windows/package.py", "--binary", binary, "--version", "0.1.6-dev", "--output", output / "candidate")
        candidate = output / "candidate"
        for line in (candidate / "checksums.txt").read_text().splitlines():
            digest, name = line.split("  ", 1)
            assert hashlib.sha256((candidate / name).read_bytes()).hexdigest() == digest
        with zipfile.ZipFile(candidate / "ha_0.1.6-dev_windows_amd64.zip") as archive:
            manifest = json.loads(archive.read("package.json"))
            assert (manifest["os"], manifest["arch"], manifest["version"]) == ("windows", "amd64", "0.1.6-dev")
            inventory = {row["path"] for row in manifest["files"]}
            assert set(required).issubset(inventory | {"tools/windows/package.py"})
            assert set(archive.namelist()) == inventory | {"package.json"}
            for row in manifest["files"]:
                data = archive.read(row["path"])
                assert len(data) == row["size"] and hashlib.sha256(data).hexdigest() == row["sha256"]
            assert all(not name.startswith("docs/") and not name.endswith(("runs.jsonl", ".log")) for name in inventory)
    run(sys.executable, "tools/check_public_tree.py")
    run(sys.executable, "tools/check_public_docs.py")
    print("Candidate content and guidance observed; native runtime/host observations remain separate.")
    return {"status": "pass"}

def main():
    try:
        observation = observe()
    except AssertionError:
        observation = {"status": "violation", "violation": "native-candidate-missing"}
    except (OSError, ValueError, KeyError, subprocess.SubprocessError) as error:
        print(f"Candidate observation could not finish: {error}", file=sys.stderr)
        observation = {"status": "error", "reason": "execution_error"}
    observation.update(target="AC-12", attempt=1)
    report = {"schema": "check-result/v1", "producer": "jaekit-windows-docs", "producer_version": "1",
              "invocation": os.environ.get("HA_EVIDENCE_INVOCATION", ""),
              "declaration_digest": os.environ.get("HA_EVIDENCE_DECLARATION_DIGEST", ""),
              "attempts_complete": True, "observations": [observation]}
    if os.environ.get("HA_EVIDENCE_PATH"):
        Path(os.environ["HA_EVIDENCE_PATH"]).write_text(json.dumps(report), encoding="utf-8")
    return 0 if observation["status"] == "pass" else 1

if __name__ == "__main__":
    raise SystemExit(main())
