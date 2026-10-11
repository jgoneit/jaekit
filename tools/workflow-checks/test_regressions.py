"""Connect finite regression-registry checks to the existing docs group."""
import importlib.util
from pathlib import Path
import unittest

ROOT = Path(__file__).resolve().parents[2]


class RegistryConnection(unittest.TestCase):
    def test_registry_is_a_selected_public_contract(self):
        spec = importlib.util.spec_from_file_location('registry_connection', ROOT / 'tools/regression_map.py')
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        report = module.validate(ROOT, module.load(ROOT))
        self.assertEqual(report['assurance'], 'selection-only')


def load_tests(loader, tests, pattern):
    suite = tests
    for filename in ('test_map.py', 'test_checker.py'):
        path = ROOT / 'tools/regression-checks' / filename
        spec = importlib.util.spec_from_file_location('registry_' + path.stem, path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        suite.addTests(loader.loadTestsFromModule(module))
    return suite
