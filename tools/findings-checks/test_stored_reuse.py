"""Stored findings stay usable through the real CLI and the support helper.

A finding the Core accepts is stored with its record envelope and re-encoded.
The support helper, queries and retries must keep working on those exact
bytes, also near the 1 MiB input limit and for values the encoder escapes.
Set JAEKIT_FINDINGS_BINARY to test a built binary; otherwise one is built.
"""
import copy
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
HELPER = ROOT / "plugins/seal/skills/seal/scripts/check_core.py"
INPUT_LIMIT = 1024 * 1024

_spec = importlib.util.spec_from_file_location("findings_cli_fixture", ROOT / "tools/findings-checks/check.py")
CLI = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(CLI)


def compact(data):
    return json.dumps(data, ensure_ascii=False, separators=(",", ":")).encode()


def contract_status_fields():
    text = (ROOT / "contracts/run-record.md").read_text(encoding="utf-8")
    section = text.split("#### 상태 문서 (`status/v1`)", 1)[1].split("\n\n####", 1)[0]
    return re.findall(r"^\| `([a-z_]+)` \|", section, re.MULTILINE)


class Goal(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="jaekit-findings-reuse-")
        cls.binary = os.environ.get("JAEKIT_FINDINGS_BINARY")
        if not cls.binary:
            cls.binary = str(Path(cls.temporary.name) / "ha")
            subprocess.run(["go", "build", "-mod=readonly", "-o", cls.binary, "./cmd/ha"], cwd=ROOT,
                           env=dict(os.environ, GOTOOLCHAIN="local"), check=True, capture_output=True, timeout=600)

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    def setUp(self):
        directory = tempfile.TemporaryDirectory(prefix="jaekit-findings-goal-")
        self.addCleanup(directory.cleanup)
        root = Path(directory.name) / "repo"
        root.mkdir()
        self.f = CLI.Fixture(self.binary, root)

    def submit(self, data=None, raw=None):
        """Send one request from outside the repository; return the process."""
        with tempfile.NamedTemporaryFile(suffix=".json") as handle:
            handle.write(compact(data) if raw is None else raw)
            handle.flush()
            return self.f.cli("finding", self.f.goal, "--input", handle.name)

    def stored(self, data):
        result = self.submit(data)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return json.loads(result.stdout)

    def refused(self, data, code):
        before = self.f.records.read_bytes()
        result = self.submit(data)
        self.assertEqual(result.returncode, code, result.stdout + result.stderr)
        self.assertEqual(before, self.f.records.read_bytes(), "a refused request changed the record")

    def support(self, operation):
        result = subprocess.run([sys.executable, str(HELPER), "--ha", self.binary, "--operation", operation,
                                 "--goal", str(self.f.root / self.f.goal)],
                                cwd=self.f.root, env=self.f.env, capture_output=True, text=True, timeout=60)
        return result.returncode, json.loads(result.stdout)

    def assertSupported(self):
        for operation in ("finding", "resume"):
            code, report = self.support(operation)
            self.assertEqual((code, report["compatible"]), (0, True), report)

    def query(self, *args):
        result = self.f.cli("finding", self.f.goal, *args)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return result.stdout

    def padded(self, item, fill):
        """Grow the summary until the compact UTF-8 request is just under 1 MiB."""
        item = copy.deepcopy(item)
        item["summary"] += fill * ((INPUT_LIMIT - len(compact(item))) // len(fill.encode()))
        self.assertLessEqual(len(compact(item)), INPUT_LIMIT)
        self.assertGreater(len(compact(item)), INPUT_LIMIT - 8)
        return item

    def last_line(self):
        return self.f.records.read_bytes().splitlines()[-1]

    def rewrite_last(self, line):
        """Replace the last line of this synthetic record and keep its chain valid."""
        lines = self.f.records.read_bytes().splitlines()
        lines[-1] = line
        self.f.records.write_bytes(b"\n".join(lines) + b"\n")


class StoredSupport(Goal):
    """AC-1: what the Core stores from an accepted input stays supported."""

    def test_near_limit_ascii_record_stays_supported(self):
        item = self.padded(self.f.request(), "x")
        self.stored(item)
        self.assertGreater(len(self.last_line()), INPUT_LIMIT, "the envelope must take the line past the input limit")
        self.assertSupported()
        self.assertEqual(json.loads(self.query())["findings"][0]["change"]["summary"], item["summary"])

    def test_escaped_line_separators_stay_supported(self):
        # The record encoder writes U+2028 as a six-byte escape: twice its input size.
        item = self.padded(self.f.request(), "\u2028")
        self.stored(item)
        self.assertGreater(len(self.last_line()), 2 * INPUT_LIMIT - 64 * 1024)
        self.assertSupported()
        self.assertEqual(json.loads(self.query())["findings"][0]["change"]["summary"], item["summary"])

    def test_multilingual_record_stays_supported(self):
        item = self.padded(self.f.request(), "발견")
        self.stored(item)
        self.assertSupported()
        self.assertEqual(json.loads(self.query())["findings"][0]["change"]["summary"], item["summary"])

    def test_input_over_the_limit_is_refused(self):
        item = self.padded(self.f.request(), "x")
        raw = compact(item)
        raw = raw[:-2] + b"xx" + raw[-2:]
        self.assertGreater(len(raw), INPUT_LIMIT)
        before = self.f.records.read_bytes()
        result = self.submit(raw=raw)
        self.assertEqual(result.returncode, 64, result.stdout + result.stderr)
        self.assertEqual(before, self.f.records.read_bytes())

    def test_oversized_damaged_and_unsupported_lines_are_told_apart(self):
        self.stored(self.f.request())
        line = json.loads(self.last_line())
        line["finding"]["summary"] = "x" * (3 * INPUT_LIMIT)
        self.rewrite_last(compact(line))
        self.assertEqual(self.support("finding")[1]["reason"], "goal_record_oversized")
        line["finding"]["summary"] = "short"
        line["schema"] = "run-finding/future"
        self.rewrite_last(compact(line))
        self.assertEqual(self.support("finding")[1]["reason"], "unsupported_record_schema")
        self.rewrite_last(compact(line)[:-3])
        self.assertEqual(self.support("finding")[1]["reason"], "goal_unreadable")


class QueryContract(Goal):
    """AC-5: machine and human views show the same per-finding classification."""

    def test_status_json_follows_the_contract_order(self):
        self.stored(self.f.request())
        result = self.f.cli("status", self.f.goal, "--format", "json")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        fields = [key for key, _ in json.loads(result.stdout, object_pairs_hook=list)]
        self.assertEqual(fields, contract_status_fields())

    def test_markdown_rows_show_each_category(self):
        one = self.f.request("one")
        two = self.f.request("two")
        two.update(mapping="unmapped", criteria=[], category="new_requirement")
        self.stored(one)
        self.stored(two)
        for item in (one, two):
            report = json.loads(self.query())
            view = next(v for v in report["findings"] if v["id"] == item["id"])
            nxt = copy.deepcopy(item)
            nxt.update(request_id="confirm-" + item["id"], revision=view["revision"], status="confirmed",
                       reason="Synthetic review", evidence=["synthetic:reproduction"])
            self.stored(nxt)
        status_md = self.f.cli("status", self.f.goal).stdout
        for text in (self.query("--format", "md"), status_md):
            rows = {line.split(":", 1)[0][2:]: line for line in text.splitlines() if line.startswith("- one:") or line.startswith("- two:")}
            self.assertIn("existing_condition", rows["one"], text)
            self.assertIn("new_requirement", rows["two"], text)
            self.assertNotIn("existing_condition", rows["two"], text)


class StoredRoundTrip(Goal):
    """AC-6: store, check support, query and retry the same request in sequence."""

    def test_store_support_query_and_retry(self):
        item = self.f.request()
        item.update(mapping="unmapped", criteria=[], resolution_refs=[])
        item = self.padded(item, "\u2028발견x")
        receipt = self.stored(item)
        self.assertFalse(receipt["replayed"])
        raw = self.last_line()
        self.assertEqual(receipt["sha256"], hashlib.sha256(raw).hexdigest())
        self.assertSupported()
        change = json.loads(self.query())["findings"][0]["change"]
        self.assertEqual((change["criteria"], change["evidence"]), ([], []))
        before = self.f.records.read_bytes()
        again = self.stored(item)
        self.assertEqual((again["seq"], again["sha256"], again["replayed"]), (receipt["seq"], receipt["sha256"], True))
        self.assertEqual(before, self.f.records.read_bytes())
        self.assertSupported()
        small = self.f.request()
        small.update(mapping="unmapped", criteria=[])
        bad = dict(small, request_id="null-lists", criteria=None)
        self.refused(bad, 64)
        bad = dict(small)
        bad.update(request_id="kept-refs", revision=receipt["seq"], status="confirmed", reason="Synthetic review",
                   evidence=["synthetic:reproduction"], resolution_refs=["synthetic:fix"])
        self.refused(bad, 65)
        fields = [key for key, _ in json.loads(self.f.cli("status", self.f.goal, "--format", "json").stdout, object_pairs_hook=list)]
        self.assertEqual(fields, contract_status_fields())
        row = next(line for line in self.query("--format", "md").splitlines() if line.startswith("- one:"))
        self.assertIn(item["category"], row)


if __name__ == "__main__":
    unittest.main()
