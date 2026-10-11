"""Public selection and observer regressions using only synthetic inputs."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

from integration_go_filters import GoFixture


ROOT = Path(__file__).resolve().parents[2]


def registry_module():
    spec = importlib.util.spec_from_file_location('merge_blocker_registry', ROOT / 'tools/regression_map.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class StaticGoSelection(unittest.TestCase):
    def test_restrictive_explicit_arguments_never_claim_the_global_suite(self):
        module = registry_module()
        cases = [('-run', '^$'), ('--run=^Other$',), ('-skip', '.'), ('-test.skip=.',),
                 ('-short',), ('-short=true',), ('-count=0',), ('-count=-1',), ('-list=.',),
                 ('-fuzz=FuzzOne',), ('-c',), ('-exec', 'true'), ('-args', '-test.run=^$')]
        for flags in cases:
            with self.subTest(flags=flags), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / 'tools').mkdir()
                argv = ['go', 'test', '-mod=readonly', './...', *flags]
                source = ('def verify_go(env):\n'
                          f'    run("Go tests", {argv!r}, env)\n'
                          'def verify_docs(env):\n    pass\n')
                (root / 'tools/verify.py').write_text(source)
                try:
                    selected = module.selected_suites(root)
                except module.Invalid:
                    continue
                if ('<go>', './...') in selected['go']:
                    self.fail('[violation:go-selection-filtered] restrictive explicit arguments '
                              f'were counted as full selection: {flags!r}')


class PreservedStaticSelection(unittest.TestCase):
    def test_benign_explicit_options_keep_full_selection(self):
        module = registry_module()
        for flags in ((), ('-count=1',), ('-count', '2'), ('-short=false',), ('-run=', '-skip=')):
            with self.subTest(flags=flags), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                (root / 'tools').mkdir()
                argv = ['go', 'test', '-mod=readonly', './...', *flags]
                (root / 'tools/verify.py').write_text(
                    'def verify_go(env):\n' + f'    run("Go tests", {argv!r}, env)\n'
                    'def verify_docs(env):\n    pass\n')
                self.assertIn(('<go>', './...'), module.selected_suites(root)['go'])


class ObserverFixture(unittest.TestCase):
    def observe(self, body):
        with tempfile.TemporaryDirectory(prefix='observer-outcome-') as directory:
            root = Path(directory)
            scripts = root / 'tools/verification-checks'
            tests = root / 'tools/regression-checks'
            scripts.mkdir(parents=True)
            tests.mkdir(parents=True)
            shutil.copy2(ROOT / 'tools/verification-checks/review_regression_gates.py',
                         scripts / 'review_regression_gates.py')
            (tests / 'test_map.py').write_text(
                'import unittest\nclass ReachableSelection(unittest.TestCase):\n' + body)
            report = root / 'result.json'
            env = {key: value for key, value in os.environ.items() if not key.startswith('HA_EVIDENCE_')}
            env.update(PYTHONDONTWRITEBYTECODE='1', HA_EVIDENCE_PATH=str(report),
                       HA_EVIDENCE_INVOCATION='a' * 64, HA_EVIDENCE_DECLARATION_DIGEST='b' * 64)
            result = subprocess.run([sys.executable, '-B', str(scripts / 'review_regression_gates.py'), 'AC-1'],
                                    cwd=root, env=env, text=True, capture_output=True, timeout=30)
            self.assertTrue(report.is_file(), result.stdout + result.stderr)
            data = json.loads(report.read_text())
            self.assertEqual(data['schema'], 'check-result/v1')
            self.assertEqual(data['invocation'], 'a' * 64)
            self.assertEqual(data['declaration_digest'], 'b' * 64)
            self.assertTrue(data['attempts_complete'])
            self.assertEqual(len(data['observations']), 1)
            return result, data['observations'][0]


class ObserverOutcomes(ObserverFixture):
    def test_expected_failure_and_unexpected_success_remain_errors_at_report_boundary(self):
        decorated = [
            ('expected_failure', 'self.fail("synthetic expected assertion")'),
            ('expected_failure', 'raise RuntimeError("synthetic expected exception")'),
            ('unexpected_success', 'pass'),
        ]
        companions = ['', '    def test_normal(self): pass\n',
                      '    def test_violation(self): self.fail("[violation:unreachable-selection-counted] observed")\n']
        for category, action in decorated:
            for companion in companions:
                with self.subTest(category=category, action=action, companion=bool(companion)):
                    result, observation = self.observe(
                        '    @unittest.expectedFailure\n'
                        f'    def test_decorated(self): {action}\n' + companion)
                    if (observation.get('status') != 'error' or observation.get('reason') != 'execution_error'
                            or result.returncode == 0 or category.upper() not in result.stderr):
                        self.fail('[violation:unittest-outcome-hidden] a non-passing unittest category '
                                  f'was hidden or accepted: {category}, {observation!r}, {result.stderr!r}')


class PreservedOutcomes(ObserverFixture):
    def test_normal_violation_empty_skip_and_error_keep_their_meaning(self):
        cases = [
            ('    def test_one(self): pass\n', 'pass'),
            ('    def test_one(self): self.fail("[violation:unreachable-selection-counted] observed")\n', 'violation'),
            ('    def test_one(self): self.fail("unrelated assertion")\n', 'error'),
            ('    def test_one(self): raise RuntimeError("synthetic error")\n', 'error'),
            ('    def test_one(self): self.skipTest("synthetic skip")\n', 'error'),
            ('    pass\n', 'error'),
        ]
        for body, expected in cases:
            with self.subTest(expected=expected, body=body):
                result, observation = self.observe(body)
                self.assertEqual(observation['status'], expected, result.stderr)
                self.assertEqual(result.returncode, 0 if expected == 'pass' else 1)
                self.assertEqual(observation['target'], 'AC-1')
                self.assertEqual(observation['attempt'], 1)
                if expected == 'violation':
                    self.assertEqual(observation['violation'], 'unreachable-selection-counted')


class PreservedSelectedRunner(GoFixture):
    def test_intentional_per_selector_runner_keeps_its_explicit_selection(self):
        driver = '''import importlib.util, pathlib, sys
spec = importlib.util.spec_from_file_location("selected_check", sys.argv[1])
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
module.ROOT = pathlib.Path(sys.argv[2])
module.tests(["go", "test", "-json", "-count=1", "./...", "-run", "^TestOther$"], "go")
'''
        result = subprocess.run([sys.executable, '-B', '-c', driver,
                                 str(ROOT / 'tools/regression-checks/check.py'), str(self.root)],
                                cwd=self.root, env=dict(self.env, SELECTED_FAILURE='1'),
                                capture_output=True, text=True, timeout=120)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.marker.exists(), 'the intentional selector must not run TestRegistered')

if __name__ == '__main__':
    unittest.main()
