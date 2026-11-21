"""The regression registry's shape and its tracked files."""
import json
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[2]


class RegistryShape(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.registry = json.loads((ROOT / "contracts/regressions.json").read_text(encoding="utf-8"))
        listing = subprocess.run(["git", "ls-files", "-z"], cwd=ROOT, check=True, capture_output=True, timeout=60)
        cls.tracked = set(listing.stdout.decode("utf-8").split("\0")) - {""}

    def test_top_level_and_invariants(self):
        self.assertEqual(set(self.registry), {"schema", "invariants", "routes"})
        self.assertEqual(self.registry["schema"], "regression-map/v1")
        invariants = self.registry["invariants"]
        self.assertEqual([item["id"] for item in invariants], ["I1", "I2", "I3", "I4", "I5"])
        for item in invariants:
            self.assertEqual(set(item), {"id", "basis", "property"}, item["id"])
            self.assertIsInstance(item["property"], str)
            self.assertTrue(item["property"])
            self.assertTrue(item["basis"], item["id"])

    def test_routes_and_checks(self):
        routes = self.registry["routes"]
        self.assertTrue(routes)
        invariants = {item["id"] for item in self.registry["invariants"]}
        ids = [route.get("id") for route in routes]
        self.assertEqual(len(ids), len(set(ids)), "route ids repeat")
        for route in routes:
            with self.subTest(route=route.get("id")):
                self.assertEqual(set(route), {"id", "invariant", "sources", "checks"})
                self.assertIsInstance(route["id"], str)
                self.assertIn(route["invariant"], invariants)
                self.assertIsInstance(route["sources"], list)
                self.assertTrue(route["checks"])
                for check in route["checks"]:
                    self.assertEqual(set(check), {"file", "selector", "group"}, check)
                    self.assertIsInstance(check["selector"], str)
                    self.assertTrue(check["selector"], check)
                    self.assertIn(check["group"], ("go", "docs"), check)

    def test_listed_files_are_tracked(self):
        listed = [(item["id"] + " basis", path) for item in self.registry["invariants"] for path in item["basis"]]
        for route in self.registry["routes"]:
            listed += [(route["id"] + " source", path) for path in route["sources"]]
            listed += [(route["id"] + " check", check["file"]) for check in route["checks"]]
        for where, path in listed:
            with self.subTest(where=where, path=path):
                self.assertIn(path, self.tracked, f"{where} {path} is not a tracked file")


if __name__ == "__main__":
    unittest.main()
