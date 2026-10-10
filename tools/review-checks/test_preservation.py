"""Public product invariants and the repository's standard verification entry."""
import hashlib
import json
import os
from pathlib import Path
import subprocess
import sys
import unittest

ROOT = Path(__file__).resolve().parents[2]


class AC13(unittest.TestCase):
    def test_core_skills_and_contracts_are_unchanged(self):
        config = json.loads(Path(__file__).with_name("preserved-public.json").read_text())
        digest = hashlib.sha256()
        for name in sorted(config["protected_paths"]):
            digest.update(name.encode() + b"\0" + hashlib.sha256((ROOT / name).read_bytes()).digest())
        self.assertEqual(digest.hexdigest(), config["protected_sha256"])

    def test_published_release_identity_is_preserved(self):
        expected = json.loads(Path(__file__).with_name("preserved-public.json").read_text())["release"]
        def api(endpoint):
            return json.loads(subprocess.check_output(["gh", "api", endpoint], cwd=ROOT, timeout=60))
        prefix = "repos/" + expected["repository"]
        commit = api(prefix + "/commits/" + expected["tag"])
        release = api(prefix + "/releases/tags/" + expected["tag"])
        self.assertEqual(commit["sha"], expected["commit"])
        self.assertEqual(release["id"], expected["release_id"])
        self.assertEqual(hashlib.sha256((release.get("body") or "").encode()).hexdigest(), expected["body_sha256"])
        assets = sorted(({k: a.get(k) for k in ("digest", "id", "name", "size")}
                         for a in release["assets"]), key=lambda a: a["name"])
        self.assertEqual(assets, expected["assets"])

    def test_standard_repository_verification(self):
        env = dict(os.environ, PYTHONDONTWRITEBYTECODE="1")
        for name in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
            env.pop(name, None)
        result = subprocess.run([sys.executable, "tools/verify.py"], cwd=ROOT, env=env, timeout=900)
        self.assertEqual(result.returncode, 0, "standard repository verification failed")
