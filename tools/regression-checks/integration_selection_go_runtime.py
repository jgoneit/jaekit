"""Compare registry selection with real local Go discovery and execution.

This suite belongs to the Go verification group only. The registry remains
static; only this separate regression launches the existing Go tool.
"""
import os
from pathlib import Path
import re
import shutil
import subprocess
import unittest

from integration_selection_go import GoFixture, HEADER, TEST


class GoRuntimeSelection(GoFixture):
    def setUp(self):
        super().setUp()
        self.go = shutil.which('go')
        self.assertIsNotNone(self.go, 'Go runtime selection regressions require the existing Go tool')
        self.environment = {key: value for key, value in os.environ.items()
                            if key not in ('GOOS', 'GOARCH', 'GOEXPERIMENT') and not key.startswith('GIT_')}
        self.environment.update(GOENV='off', GOWORK='off', GOTOOLCHAIN='local', GOFLAGS='',
                                GOPROXY='off', GOSUMDB='off', CGO_ENABLED='0')
        (self.root / 'go.mod').write_text('module example.invalid/selection\n\ngo 1.23\n')

    def go_command(self, *arguments, extra=None):
        return subprocess.run([self.go, *arguments], cwd=self.root,
                              env=dict(self.environment, **(extra or {})),
                              capture_output=True, text=True, timeout=120)

    def selected_names(self):
        result = self.go_command('test', '-mod=readonly', '-list', '.', './...')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        return {line for line in result.stdout.splitlines() if re.fullmatch(r'Test\w*', line)}

    def test_comment_and_string_mentions_are_not_real_go_tests(self):
        other = 'func TestOther(t *testing.T) {}\n'
        cases = (HEADER + '/*\n' + TEST + '*/\n' + other,
                 HEADER + 'var text = `\n' + TEST + '`\n' + other)
        for source in cases:
            with self.subTest(source=source):
                report, reason = self.observe(source)
                names = self.selected_names()
                self.assertIn('TestOther', names)
                self.assertNotIn('TestRegistered', names)
                self.assertIsNone(report, 'registry accepted a name absent from real Go discovery')
                self.assertIsNotNone(reason)

    def test_conditional_policy_distinguishes_real_exclusion_from_unsupported_selection(self):
        environment = self.go_command('env', 'GOOS')
        self.assertEqual(environment.returncode, 0, environment.stderr)
        native = environment.stdout.strip()
        other = 'windows' if native != 'windows' else 'linux'
        cases = ((f'sample_{other}_test.go', HEADER + TEST, False),
                 ('sample_test.go', '//go:build ignore\n\n' + HEADER + TEST, False),
                 (f'sample_{native}_test.go', HEADER + TEST, True))
        for file, source, actually_selected in cases:
            with self.subTest(file=file):
                try:
                    report, reason = self.observe(source, file=file)
                    self.assertEqual('TestRegistered' in self.selected_names(), actually_selected)
                    self.assertIsNone(report, 'conditional sources are outside the stated static support scope')
                    self.assertRegex(reason, r'(?i)conditional|GOOS|GOARCH')
                finally:
                    (self.root / file).unlink(missing_ok=True)

    def test_package_layout_agrees_with_real_checkout_root_walk(self):
        cases = (
            ('normal', 'TestOrdinary', True),
            ('testdata', 'TestData', False),
            ('_hidden', 'TestUnderscore', False),
            ('.hidden', 'TestDot', False),
            ('nested', 'TestNested', False),
            ('vendor', 'TestVendorDirectory', True),
            ('vendor/inside', 'TestVendorChild', False),
            ('normal/vendor', 'TestSubVendorDirectory', True),
            ('normal/vendor/inside', 'TestSubVendorChild', False),
        )
        nested = self.root / 'nested'
        nested.mkdir()
        (nested / 'go.mod').write_text('module example.invalid/nested\n\ngo 1.23\n')
        observations = []
        for directory, selector, expected in cases:
            content = HEADER + 'func ' + selector + '(t *testing.T) {}\n'
            report, reason = self.observe(content, file=directory + '/sample_test.go', selector=selector)
            observations.append((selector, expected, report, reason))
        # Start as tracked ordinary source, then replace only the ancestor.
        report, reason = self.observe(HEADER + 'func TestLinked(t *testing.T) {}\n',
                                      file='linked/sample_test.go', selector='TestLinked')
        self.assertIsNone(reason)
        (self.root / 'linked').rename(self.root / '_stored')
        (self.root / 'linked').symlink_to('_stored', target_is_directory=True)
        from integration_selection_go import registry
        try:
            linked = self.module.validate(self.root, registry('linked/sample_test.go', 'TestLinked'))
        except self.module.Invalid as error:
            observations.append(('TestLinked', False, None, str(error)))
        else:
            observations.append(('TestLinked', False, linked, None))
        names = self.selected_names()
        self.assertEqual(names, {selector for _, selector, selected in cases if selected})
        for selector, expected, report, reason in observations:
            with self.subTest(selector=selector):
                self.assertEqual(selector in names, expected)
                if (report is not None) != expected:
                    self.fail('[violation:go-build-selection-mismatch] registry package layout disagrees with real Go ./... selection')
                if expected:
                    self.assertIsNone(reason)
                else:
                    self.assertIsNotNone(reason)

    def test_static_report_does_not_execute_a_real_passing_or_failing_test(self):
        source = (HEADER + 'import "os"\n\n'
                  'func TestRegistered(t *testing.T) {\n'
                  '    if err := os.WriteFile("executed.txt", []byte("real test body"), 0600); err != nil { t.Fatal(err) }\n'
                  '    if os.Getenv("SYNTHETIC_REQUIRED_FAILURE") == "1" { t.Fatal("synthetic required failure") }\n'
                  '}\n')
        marker = self.root / 'executed.txt'
        report, reason = self.observe(source)
        self.assertIsNone(reason)
        self.assertEqual(report['assurance'], 'selection-only')
        self.assertFalse(marker.exists(), 'registry selection executed a Go test body')
        self.assertIn('TestRegistered', self.selected_names())
        self.assertFalse(marker.exists(), 'Go list selection executed a Go test body')
        passed = self.go_command('test', '-mod=readonly', '-count=1', '-run', '^TestRegistered$', './...')
        self.assertEqual(passed.returncode, 0, passed.stdout + passed.stderr)
        self.assertEqual(marker.read_text(), 'real test body')
        marker.unlink()
        failed = self.go_command('test', '-mod=readonly', '-count=1', '-run', '^TestRegistered$', './...',
                                 extra={'SYNTHETIC_REQUIRED_FAILURE': '1'})
        self.assertNotEqual(failed.returncode, 0)
        self.assertIn('synthetic required failure', failed.stdout + failed.stderr)
        self.assertEqual(marker.read_text(), 'real test body')
        # Actual test failure does not turn a static report into execution proof.
        report, reason = self.observe(source)
        self.assertIsNone(reason)
        self.assertEqual(report['assurance'], 'selection-only')


if __name__ == '__main__':
    unittest.main()
