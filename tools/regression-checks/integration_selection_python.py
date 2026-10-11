"""Compare the public selection report with real unittest discovery in Git fixtures."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]
TESTS = '''import os
from pathlib import Path
import unittest

class Registered(unittest.TestCase):
    def test_kept(self):
        with open(os.environ['SELECTION_LOG'], 'a') as output:
            output.write('Registered.test_kept\\n')

    def test_other(self):
        with open(os.environ['SELECTION_LOG'], 'a') as output:
            output.write('Registered.test_other\\n')
'''


class PythonFixture(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='python-selection-')
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.root = self.directory / 'checkout'
        (self.root / 'tools').mkdir(parents=True)
        (self.root / 'checks').mkdir()
        (self.root / 'contracts').mkdir()
        shutil.copy2(ROOT / 'tools/regression_map.py', self.root / 'tools/regression_map.py')
        self.log = self.directory / 'executed'
        self.env = dict(os.environ, PYTHONDONTWRITEBYTECODE='1', SELECTION_LOG=str(self.log),
                        GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1')
        (self.root / 'checks/test_sample.py').write_text(TESTS)
        self.git('init', '-q')
        self.configure()

    def git(self, *args):
        return subprocess.run(['git', *args], cwd=self.root, env=self.env,
                              capture_output=True, text=True, check=True).stdout

    def configure(self, flags=(), selector='Registered.test_kept', file='checks/test_sample.py',
                  discovery=None):
        self.argv = [sys.executable, '-m', 'unittest', 'discover',
                     *(discovery if discovery is not None else ['-s', 'checks', '-p', 'test_*.py']),
                     *flags]
        (self.root / 'tools/verify.py').write_text(
            'def verify_go(env):\n    pass\n\ndef verify_docs(env):\n'
            f'    run("synthetic discovery", {self.argv!r}, env)\n')
        invariants = [{'id': f'I{n}', 'basis': ['tools/verify.py'],
                       'property': 'synthetic invariant'} for n in range(1, 6)]
        routes = [{'id': f'gap-{n}', 'invariant': f'I{n}', 'status': 'gap',
                   'sources': ['tools/verify.py'], 'property': 'synthetic property',
                   'examples': ['synthetic example'], 'families': ['synthetic family'],
                   'boundary': 'synthetic boundary', 'checks': [],
                   'follow_up': ['https://github.com/jgoneit/jaekit/issues/57']}
                  for n in range(1, 6)]
        routes.append(dict(routes[0], id='sample-selected', status='protected', follow_up=[],
                           checks=[{'file': file, 'selector': selector, 'group': 'docs'}]))
        (self.root / 'contracts/regressions.json').write_text(json.dumps(
            {'schema': 'regression-map/v1', 'invariants': invariants, 'routes': routes}))
        self.git('add', '.')

    def report(self):
        result = subprocess.run([sys.executable, '-B', 'tools/regression_map.py', '--format', 'json'],
                                cwd=self.root, env=self.env, capture_output=True, text=True, timeout=30)
        if result.returncode == 0:
            data = json.loads(result.stdout)
            self.assertEqual(data['assurance'], 'selection-only')
            self.assertIn('sample-selected', data['protected'])
        return result

    def execute(self):
        self.log.unlink(missing_ok=True)
        result = subprocess.run(self.argv, cwd=self.root, env=self.env,
                                capture_output=True, text=True, timeout=30)
        # Python 3.14 reports an empty discovery as exit 5; earlier versions
        # report it as success. Either still means no selector was executed.
        empty = result.returncode == 5 and 'Ran 0 tests' in result.stderr and not self.log.exists()
        self.assertTrue(result.returncode == 0 or empty, result.stdout + result.stderr)
        return self.log.read_text().splitlines() if self.log.exists() else []


class PythonFilterSelection(PythonFixture):
    def test_name_filters_match_the_real_discovery_selection(self):
        cases = [
            ((), 'Registered.test_kept', True),
            (('-k', 'kept'), 'Registered.test_kept', True),
            (('-kkept',), 'Registered.test_kept', True),
            (('-k', '*.Registered.test_k*'), 'Registered.test_kept', True),
            (('-k', 'other'), 'Registered.test_kept', False),
            (('-k', 'missing'), 'Registered.test_kept', False),
            (('-k', 'Registered.TEST_KEPT'), 'Registered.test_kept', False),
            (('-k', 'missing', '-k', 'kept'), 'Registered.test_kept', True),
            (('-k', 'other', '-k', 'missing'), 'Registered.test_kept', False),
            (('-k', 'other'), 'Registered', True),
            (('-k', 'missing'), 'Registered', False),
        ]
        for flags, selector, expected in cases:
            with self.subTest(flags=flags, selector=selector):
                self.configure(flags, selector)
                executed = self.execute()
                selected = any(name == selector or name.startswith(selector + '.') for name in executed)
                self.assertEqual(selected, expected, executed)
                result = self.report()
                if (result.returncode == 0) != selected:
                    self.fail('[violation:python-filter-selection-mismatch] '
                              f'{flags!r} executed {executed!r} for {selector}, but report '
                              f'exit={result.returncode}: {result.stdout}{result.stderr}')

    def test_unknown_or_incomplete_selection_is_explicitly_refused(self):
        for flags in (('-k',), ('--unknown-selection', 'kept')):
            with self.subTest(flags=flags):
                self.configure(flags)
                result = self.report()
                if result.returncode == 0 or 'unittest discovery' not in result.stderr:
                    self.fail('[violation:python-filter-selection-mismatch] unsupported discovery '
                              f'was not explicitly refused: {flags!r}; {result.stdout}{result.stderr}')


class PythonPatternSelection(PythonFixture):
    def test_load_tests_receives_the_actual_discovery_pattern(self):
        hook = '''
def load_tests(loader, tests, pattern):
    name = 'test_kept' if pattern == 'test_*.py' else 'test_other'
    return unittest.TestSuite([Registered(name)])
'''
        (self.root / 'checks/test_sample.py').write_text(TESTS + hook)
        for pattern in ('test_*.py', 'test_sample.py'):
            for selector in ('Registered.test_kept', 'Registered.test_other', 'Registered'):
                with self.subTest(pattern=pattern, selector=selector):
                    self.configure(selector=selector, discovery=['-s', 'checks', '-p', pattern])
                    executed = self.execute()
                    selected = any(name == selector or name.startswith(selector + '.') for name in executed)
                    result = self.report()
                    if (result.returncode == 0) != selected:
                        self.fail('[violation:python-pattern-selection-mismatch] '
                                  f'pattern {pattern!r} executed {executed!r} for {selector}, '
                                  f'but report exit={result.returncode}: {result.stdout}{result.stderr}')

    def test_another_modules_same_selector_is_not_the_registered_test(self):
        (self.root / 'checks/test_sample.py').write_text(
            TESTS + '\ndef load_tests(loader, tests, pattern):\n'
            '    return unittest.TestSuite() if pattern == "test_*.py" else tests\n')
        (self.root / 'checks/test_other.py').write_text(TESTS)
        self.configure()
        self.assertIn('Registered.test_kept', self.execute())
        result = self.report()
        if result.returncode == 0:
            self.fail('[violation:python-pattern-selection-mismatch] a same-named test from '
                      'another module replaced the registered module excluded by load_tests')

    def test_start_and_top_directories_preserve_module_identity(self):
        (self.root / 'checks/__init__.py').write_text('')
        for discovery in (['-s', 'checks', '-t', '.', '-p', 'test_*.py'],
                          ['--start-directory=checks', '--top-level-directory=.', '--pattern=test_*.py'],
                          ['checks', 'test_*.py', '.']):
            with self.subTest(discovery=discovery):
                self.configure(('-k', 'checks.test_sample.Registered.test_kept'), discovery=discovery)
                executed = self.execute()
                self.assertEqual(executed, ['Registered.test_kept'])
                result = self.report()
                if result.returncode:
                    self.fail('[violation:python-pattern-selection-mismatch] a top-level-qualified '
                              f'normal selector was refused: {discovery!r}; {result.stderr}')


class PreservedPythonSelection(PythonFixture):
    def test_collection_never_executes_test_bodies_and_needs_no_go(self):
        self.env['PATH'] = str(Path(shutil.which('git')).parent)
        self.assertFalse(self.log.exists())
        result = self.report()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertFalse(self.log.exists(), 'selection-only collection executed a test body')

    def test_filter_independent_hook_and_sibling_import_remain_selected(self):
        (self.root / 'checks/helper.py').write_text('VALUE = 1\n')
        (self.root / 'checks/test_sample.py').write_text(
            'from helper import VALUE\n' + TESTS +
            '\ndef load_tests(loader, tests, pattern):\n    return tests\n')
        self.configure(('-v', '-b', '-f', '--locals'), selector='Registered')
        result = self.report()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(set(self.execute()), {'Registered.test_kept', 'Registered.test_other'})

    def test_individual_runner_preserves_selection_and_sibling_import(self):
        (self.root / 'checks/helper.py').write_text('VALUE = 1\n')
        (self.root / 'checks/test_sample.py').write_text('from helper import VALUE\n' + TESTS)
        report = self.directory / 'runner.json'
        result = subprocess.run([sys.executable, '-B', str(ROOT / 'tools/regression-checks/run_unittest.py'),
                                 '--file', str(self.root / 'checks/test_sample.py'),
                                 '--selector', 'Registered.test_other', '--report', str(report)],
                                cwd=self.root, env=self.env, capture_output=True, text=True, timeout=30)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.log.read_text().splitlines(), ['Registered.test_other'])
        self.assertEqual(json.loads(report.read_text())['tests_run'], 1)

    def test_collection_and_actual_test_failures_remain_distinct(self):
        (self.root / 'checks/test_sample.py').write_text(
            'import unittest\nclass Registered(unittest.TestCase):\n'
            '    def test_kept(self): self.fail("synthetic execution failure")\n')
        self.configure()
        result = self.report()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        execution = subprocess.run(self.argv, cwd=self.root, env=self.env,
                                   capture_output=True, text=True, timeout=30)
        self.assertNotEqual(execution.returncode, 0)
        self.assertIn('synthetic execution failure', execution.stderr)
        (self.root / 'checks/test_sample.py').write_text('import missing_synthetic_dependency\n')
        result = self.report()
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('could not be collected', result.stderr)


if __name__ == '__main__':
    unittest.main()
