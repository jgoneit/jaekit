"""Actual assertion provenance at the observer report and Core boundaries."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

import integration_observer_core as core_fixture

ROOT = Path(__file__).resolve().parents[2]
TAG = '[violation:unreachable-selection-counted]'


class ObserverFixture(unittest.TestCase):
    def observe(self, body):
        with tempfile.TemporaryDirectory(prefix='observer-attribution-') as directory:
            root = Path(directory)
            observer = root / 'tools/verification-checks/review_regression_gates.py'
            tests = root / 'tools/regression-checks/test_map.py'
            observer.parent.mkdir(parents=True)
            tests.parent.mkdir(parents=True)
            shutil.copyfile(ROOT / 'tools/verification-checks/review_regression_gates.py', observer)
            tests.write_text('import unittest\nclass ReachableSelection(unittest.TestCase):\n' + body)
            report = root / 'result.json'
            env = {key: value for key, value in os.environ.items() if not key.startswith('HA_EVIDENCE_')}
            env.update(PYTHONDONTWRITEBYTECODE='1', HA_EVIDENCE_PATH=str(report),
                       HA_EVIDENCE_INVOCATION='a' * 64, HA_EVIDENCE_DECLARATION_DIGEST='b' * 64)
            result = subprocess.run([sys.executable, '-B', str(observer), 'AC-1'], cwd=root,
                                    env=env, capture_output=True, text=True, timeout=30)
            self.assertTrue(report.is_file(), result.stdout)
            data = json.loads(report.read_text())
            self.assertEqual(data['schema'], 'check-result/v1')
            self.assertEqual(data['invocation'], 'a' * 64)
            self.assertEqual(data['declaration_digest'], 'b' * 64)
            self.assertTrue(data['attempts_complete'])
            self.assertEqual(len(data['observations']), 1)
            return result, data['observations'][0]


class UnattributedOutcomes(ObserverFixture):
    def test_compared_source_and_chained_tags_do_not_establish_a_violation(self):
        cases = {
            'compared-value': f"        self.assertEqual('actual', {TAG!r})\n",
            'source-comment': f"        self.assertEqual('actual', 'wrong')  # {TAG}\n",
            'assertion-message': f"        self.assertTrue(False, {TAG!r})\n",
            'message-suffix': f"        self.fail('unrelated ' + {TAG!r})\n",
            'plain-assertion': f"        raise AssertionError({TAG!r})\n",
            'multiple-tags': f"        self.fail({(TAG + ' [violation:other]')!r})\n",
            'subtest-comparison': "        with self.subTest(case='synthetic'):\n"
                                  f"            self.assertEqual('actual', {TAG!r})\n",
            'chained-assertion': "        try:\n"
                                 f"            self.fail({TAG!r})\n"
                                 "        except AssertionError:\n"
                                 "            self.fail('unrelated final failure')\n",
            'wrong-identity': "        self.fail('[violation:other] actual different requirement')\n",
        }
        for label, action in cases.items():
            for mixed in (False, True):
                with self.subTest(case=label, mixed=mixed):
                    body = '    def test_unrelated(self):\n' + action
                    if mixed:
                        body += f'    def test_declared(self): self.fail({TAG!r})\n'
                    result, observation = self.observe(body)
                    if (observation.get('status') != 'error'
                            or observation.get('reason') != 'execution_error' or result.returncode == 0):
                        self.fail('[violation:unittest-outcome-hidden] unrelated assertion attribution '
                                  f'was accepted: {label}, {observation!r}; exit={result.returncode}')


class ExplicitOutcomes(ObserverFixture):
    def test_direct_fail_and_subtest_fail_preserve_the_actual_identity(self):
        cases = {
            'direct': f'        self.fail({TAG!r})\n',
            'subtest': "        with self.subTest(case='synthetic'):\n"
                       f'            self.fail({TAG!r})\n',
            'unrelated-cause': "        try:\n"
                               "            raise ValueError('[violation:other] unrelated cause')\n"
                               "        except ValueError:\n"
                               f'            self.fail({TAG!r})\n',
        }
        for label, action in cases.items():
            with self.subTest(case=label):
                result, observation = self.observe('    def test_declared(self):\n' + action)
                self.assertEqual(observation.get('status'), 'violation')
                self.assertEqual(observation.get('violation'), 'unreachable-selection-counted')
                self.assertEqual(result.returncode, 1)


class AttributionCore(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        core_fixture.ObserverCore.setUpClass.__func__(cls)

    def test_unrelated_assertions_cannot_satisfy_the_actual_core_baseline(self):
        cases = {
            'compared-value': f'    def test_unrelated(self): self.assertEqual("actual", {TAG!r})\n',
            'subtest-comparison': '    def test_unrelated(self):\n'
                                 '        with self.subTest(case="synthetic"):\n'
                                 f'            self.assertEqual("actual", {TAG!r})\n',
        }
        for label, body in cases.items():
            with self.subTest(case=label):
                core_fixture.ObserverCore.run_case(self, body)


if __name__ == '__main__':
    unittest.main()
