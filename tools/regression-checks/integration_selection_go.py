"""Go selection through the public registry using synthetic tracked sources.

No Go tool or test body is executed. Failure fixtures observe the real registry
reader, including on a baseline without the new declaration helper.
"""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[2]
HEADER = 'package sample\n\nimport "testing"\n\n'
TEST = 'func TestRegistered(t *testing.T) { panic("selection must not execute the body") }\n'


def registry_module():
    spec = importlib.util.spec_from_file_location('go_selection_registry', ROOT / 'tools/regression_map.py')
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def registry(file, selector='TestRegistered'):
    routes = []
    for number in range(1, 6):
        protected = number == 1
        routes.append({'id': f'synthetic-i{number}', 'invariant': f'I{number}',
                       'status': 'protected' if protected else 'gap',
                       'sources': ['product.go'], 'property': 'synthetic property',
                       'examples': ['synthetic example'], 'families': ['synthetic family'],
                       'boundary': 'public static selection',
                       'checks': [{'file': file, 'selector': selector, 'group': 'go'}] if protected else [],
                       'follow_up': [] if protected else ['https://github.com/jgoneit/jaekit/issues/56']})
    return {'schema': 'regression-map/v1',
            'invariants': [{'id': f'I{n}', 'basis': ['product.go'], 'property': 'synthetic invariant'}
                           for n in range(1, 6)], 'routes': routes}


class GoFixture(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix='jaekit-go-selection-')
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name).resolve()
        (self.root / 'tools').mkdir()
        (self.root / 'tools/verify.py').write_text(
            'def verify_go(env):\n'
            '    run("Go tests", ["go", "test", "-mod=readonly", "./..."], env)\n'
            'def verify_docs(env):\n    pass\n')
        (self.root / 'product.go').write_text('package sample\n')
        self.git('init', '-q')
        self.module = registry_module()

    def git(self, *arguments):
        environment = {key: value for key, value in os.environ.items() if not key.startswith('GIT_')}
        environment.update(GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1')
        return subprocess.check_output(['git', '-c', 'core.hooksPath=/dev/null', *arguments],
                                       cwd=self.root, env=environment, stderr=subprocess.PIPE, timeout=30)

    def observe(self, content, file='sample_test.go', selector='TestRegistered'):
        path = self.root / file
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        self.git('add', '-f', '.')
        before = (path.read_bytes(), (self.root / '.git/index').read_bytes())
        # This executes the product registry boundary, not a helper substitute.
        try:
            report = self.module.validate(self.root, registry(file, selector))
        except self.module.Invalid as error:
            result = (None, str(error))
        else:
            result = (report, None)
        self.assertEqual((path.read_bytes(), (self.root / '.git/index').read_bytes()), before)
        return result


class GoDefinitions(GoFixture):
    def test_non_declaration_text_cannot_protect_a_selector(self):
        cases = {
            'line comment': HEADER + '// ' + TEST,
            'block comment': HEADER + '/*\n' + TEST + '*/\n',
            'raw string': HEADER + 'var sample = `\n' + TEST + '`\n',
            'quoted string': HEADER + 'var sample = ' + json.dumps(TEST) + '\n',
            'other declaration text': HEADER + 'var sample = func() {\n' + TEST + '}\n',
        }
        for label, source in cases.items():
            with self.subTest(kind=label):
                report, _ = self.observe(source)
                if report is not None:
                    self.fail('[violation:go-definition-not-real] non-declaration Go text was accepted as a protected test')

    def test_non_test_signatures_are_not_real_selected_tests(self):
        cases = [
            ('Testlower', HEADER + TEST.replace('TestRegistered', 'Testlower')),
            ('TestRegistered', HEADER + TEST.replace('(t *testing.T)', '(t *testing.T) int')),
            ('TestRegistered', HEADER + TEST.replace('(t *testing.T)', '(t *testing.T, other int)')),
            ('TestRegistered', HEADER + TEST.replace('func TestRegistered', 'func (receiver sample) TestRegistered')),
            ('TestRegistered', HEADER + TEST.replace('func TestRegistered', 'func TestRegistered[T any]')),
            ('TestRegistered', 'package sample\n\n' + TEST),
        ]
        for selector, source in cases:
            with self.subTest(source=source):
                report, _ = self.observe(source, selector=selector)
                if report is not None:
                    self.fail('[violation:go-definition-not-real] a non-test signature was accepted as a selected Go test')


class GoBuildSelection(GoFixture):
    def test_build_directives_do_not_establish_unconditional_selection(self):
        for directive in ('//go:build ignore', '// +build ignore', '//go:build windows',
                          '//go:build linux && custom', '//go:build !linux || custom',
                          '//go:build linux || !linux', '//go:build malformed &&'):
            with self.subTest(directive=directive):
                report, reason = self.observe(directive + '\n\n' + HEADER + TEST)
                if report is not None:
                    self.fail('[violation:go-build-selection-mismatch] a conditional Go file was reported as unconditionally selected')
                self.assertRegex(reason, r'(?i)build constraint|conditional')

    def test_filename_conditions_and_ignored_names_are_not_selection(self):
        for file in ('sample_windows_test.go', 'sample_linux_test.go', 'sample_amd64_test.go',
                     'sample_darwin_arm64_test.go', 'sample_nacl_test.go', '_sample_test.go', '.sample_test.go'):
            with self.subTest(file=file):
                report, reason = self.observe(HEADER + TEST, file=file)
                if report is not None:
                    self.fail('[violation:go-build-selection-mismatch] a conditional or excluded Go filename established selection')
                self.assertRegex(reason, r'(?i)filename|GOOS|GOARCH')


class PreservedGoSelection(GoFixture):
    def test_real_declarations_survive_unrelated_comments_and_literals(self):
        cases = [HEADER + TEST,
                 HEADER + '// unrelated signature text\n/* braces { and } */\n' + TEST,
                 HEADER + 'var raw = `\nfunc TestOther(t *testing.T) {}\n`\n' + TEST,
                 HEADER + 'var quote = "func TestOther(t *testing.T) {}"\n' + TEST,
                 HEADER + TEST.replace('(t *testing.T)', '(argument /* type */ *testing.T)'),
                 HEADER.replace('"testing"', 'check "testing"') + TEST.replace('*testing.T', '*check.T'),
                 HEADER + TEST.replace('(t *testing.T)', '(*testing.T)'),
                 HEADER + TEST.replace('(t *testing.T)', '(t *testing.T,)')]
        for source in cases:
            with self.subTest(source=source):
                report, reason = self.observe(source)
                self.assertIsNone(reason)
                self.assertEqual(report['assurance'], 'selection-only')
                self.assertIn('synthetic-i1', report['protected'])

    def test_no_go_tool_is_needed_and_native_environment_is_not_guessed(self):
        real_run = subprocess.run
        def without_go(argv, *args, **kwargs):
            if Path(argv[0]).name in ('go', 'gofmt'):
                raise AssertionError('static registry must not execute a Go tool')
            return real_run(argv, *args, **kwargs)
        for file in ('linux_test.go', 'sample_unix_test.go', 'sample_unknownos_test.go'):
            with self.subTest(file=file), mock.patch.object(subprocess, 'run', side_effect=without_go), \
                    mock.patch.dict(os.environ, {'GOOS': 'synthetic-os', 'GOARCH': 'synthetic-arch', 'GOFLAGS': '-tags=custom'}):
                report, reason = self.observe(HEADER + TEST, file=file)
                self.assertIsNone(reason)
                self.assertEqual(report['assurance'], 'selection-only')

    def test_all_current_registered_go_selectors_remain_connected(self):
        module = registry_module()
        data = module.load(ROOT)
        selected, public, collections = module.selected_suites(ROOT), module.tracked(ROOT), {}
        checks = {(item['file'], item['selector'], item['group']) for route in data['routes']
                  for item in route['checks'] if item['file'].endswith('_test.go')}
        self.assertTrue(checks)
        for file, selector, group in sorted(checks):
            with self.subTest(file=file, selector=selector):
                module.check_definition(ROOT, {'file': file, 'selector': selector, 'group': group},
                                        selected, public, collections)


if __name__ == '__main__':
    unittest.main()
