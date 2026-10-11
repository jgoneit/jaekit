"""Real Go test selection through the public shared verification entrypoint.

Other verifier stages use synthetic fixtures. These checks establish the Go
test/environment boundary, not successful real builds or release validation.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


class GoFixture(unittest.TestCase):
    def setUp(self):
        self.go = shutil.which('go')
        self.assertIsNotNone(self.go, 'these Go-group regressions require the existing Go tool')
        temporary = tempfile.TemporaryDirectory(prefix='full-go-selection-')
        self.addCleanup(temporary.cleanup)
        self.directory = Path(temporary.name)
        self.root = self.directory / 'checkout'
        self.bin = self.directory / 'bin'
        self.bin.mkdir()
        (self.root / 'tools').mkdir(parents=True)
        shutil.copy2(ROOT / 'tools/verify.py', self.root / 'tools/verify.py')
        self.log = self.directory / 'go.jsonl'
        self.marker = self.directory / 'selected'
        self.config = self.directory / 'go.env'
        smoke = f'#!{sys.executable}\nraise SystemExit(0)\n'
        go_wrapper = (
            f'#!{sys.executable}\nimport json, os, pathlib, sys\n'
            'with open(os.environ["FILTER_LOG"], "a") as output:\n'
            '    output.write(json.dumps({"args": sys.argv[1:], "flags": os.environ.get("GOFLAGS")}) + "\\n")\n'
            'if sys.argv[1] in ("env", "test"):\n'
            f'    os.execv({self.go!r}, [{self.go!r}, *sys.argv[1:]])\n'
            'if sys.argv[1] == "build":\n'
            '    path = pathlib.Path(sys.argv[sys.argv.index("-o") + 1])\n'
            f'    path.write_text({smoke!r})\n'
            '    path.chmod(0o700)\n'
        )
        for name, content in [('go', go_wrapper), *[(name, smoke) for name in ('gofmt', 'git', 'bash', 'zsh')]]:
            path = self.bin / name
            path.write_text(content)
            path.chmod(0o700)
        # Supply every currently selected synthetic Python suite, including new
        # integration hooks, without recursively running the real verifier.
        for group in ('review-checks', 'gap-checks', 'producer-checks', 'release-checks', 'regression-checks'):
            directory = self.root / 'tools' / group
            directory.mkdir()
            for source in (ROOT / 'tools' / group).glob('*.py'):
                if source.name.startswith(('test_', 'integration_')):
                    (directory / source.name).write_text(
                        'import unittest\nclass Synthetic(unittest.TestCase):\n    def test_pass(self): pass\n')
        (self.root / 'go.mod').write_text('module example.invalid/selection\n\ngo 1.20\n')
        (self.root / 'selection_test.go').write_text('''package selection
import ("os"; "testing")
var Linked = "default"
func TestRegistered(t *testing.T) {
    if testing.Short() { t.Skip("synthetic short selection") }
    if expected := os.Getenv("SELECTED_LINK"); expected != "" && Linked != expected { t.Fatalf("linked value changed: %q", Linked) }
    if err := os.WriteFile(os.Getenv("SELECTED_MARKER"), []byte("selected"), 0600); err != nil { t.Fatal(err) }
    if os.Getenv("SELECTED_FAILURE") == "1" { t.Fatal("synthetic registered failure") }
}
func TestOther(t *testing.T) {}
''')
        self.env = {key: value for key, value in os.environ.items() if not key.startswith('HA_EVIDENCE_')}
        self.env.update(PATH=str(self.bin) + os.pathsep + os.environ.get('PATH', ''),
                        FILTER_LOG=str(self.log), SELECTED_MARKER=str(self.marker), SELECTED_LINK='',
                        GOENV='off', GOFLAGS=' ', GOTOOLCHAIN='local', GOPROXY='off', GOSUMDB='off',
                        PYTHONDONTWRITEBYTECODE='1')
        self.env.pop('GOOS', None)
        self.env.pop('GOARCH', None)

    def verify(self, **updates):
        self.marker.unlink(missing_ok=True)
        self.log.unlink(missing_ok=True)
        result = subprocess.run([sys.executable, '-B', str(self.root / 'tools/verify.py'), 'go'],
                                cwd=self.root, env=dict(self.env, **updates), text=True,
                                capture_output=True, timeout=120)
        commands = [json.loads(line) for line in self.log.read_text().splitlines()] if self.log.exists() else []
        return result, commands


class FullGoEnvironment(GoFixture):
    def test_effective_environment_filters_cannot_hide_a_registered_failure(self):
        for flags in ('-run=^$', '-run=^TestOther$', '--test.run=^$', '-skip=TestRegistered',
                      '-short', '-count=0', '-list=.'):
            with self.subTest(flags=flags):
                result, _ = self.verify(GOFLAGS=flags, SELECTED_FAILURE='1')
                if (result.returncode == 0 or not self.marker.is_file() or
                        'synthetic registered failure' not in result.stdout + result.stderr or
                        'GOFLAGS test filters' not in result.stdout):
                    self.fail('[violation:go-selection-filtered] inherited filters hid a registered '
                              f'failure or were not reported: {flags!r}\n{result.stdout}{result.stderr}')

    def test_persistent_filter_is_overridden_without_rewriting_configuration(self):
        original = b'GOFLAGS=-run=^$\nGOPROXY=off\n'
        self.config.write_bytes(original)
        result, commands = self.verify(GOENV=str(self.config), GOFLAGS='', SELECTED_FAILURE='1')
        tests = [item for item in commands if item['args'][0] == 'test']
        if (result.returncode == 0 or not self.marker.is_file() or not tests or
                not tests[0]['flags'] or tests[0]['flags'].strip() or self.config.read_bytes() != original):
            self.fail('[violation:go-selection-filtered] persistent Go filters bypassed full selection '
                      f'or their configuration changed\n{result.stdout}{result.stderr}')

    def test_quoted_nonselection_options_survive_reported_filter_removal(self):
        retained = '"-ldflags=-s -w" -p=1'
        flags = retained + ' -run=^TestOther$'
        result, commands = self.verify(GOFLAGS=flags, SELECTED_FAILURE='0')
        tests = [item for item in commands if item['args'][0] == 'test']
        if (result.returncode or not self.marker.is_file() or not tests or
                tests[0]['flags'] != retained or '-run=^TestOther$' not in result.stdout):
            self.fail('[violation:go-selection-filtered] quoted unrelated flags or the full selection '
                      f'were not preserved\n{result.stdout}{result.stderr}')

    def test_compile_only_substituted_execution_and_ambiguous_flags_are_not_full_success(self):
        executable = shutil.which('true')
        self.assertIsNotNone(executable)
        for flags in ('-c', '-exec=' + executable, '-run', "'-run=^$"):
            with self.subTest(flags=flags):
                result, _ = self.verify(GOFLAGS=flags, SELECTED_FAILURE='1')
                if result.returncode == 0 or 'PASS: go verification' in result.stdout:
                    self.fail('[violation:go-selection-filtered] an uninterpretable execution mode '
                              f'was accepted as full verification: {flags!r}\n{result.stdout}{result.stderr}')

    def test_quoted_linker_value_preserves_literal_backslash_carriage_return_and_unicode(self):
        value = 'back\\slash\r합성'
        retained = '"-ldflags=-X \'example.invalid/selection.Linked=' + value + '\'"'
        result, commands = self.verify(GOFLAGS=retained + ' -run=^TestOther$',
                                       SELECTED_FAILURE='0', SELECTED_LINK=value)
        tests = [item for item in commands if item['args'][0] == 'test']
        if result.returncode or not self.marker.is_file() or not tests or tests[0]['flags'] != retained:
            self.fail('[violation:go-selection-filtered] a quoted Go linker value was changed or '
                      f'the full selection was lost\n{result.stdout}{result.stderr}')


class PreservedGoSelection(GoFixture):
    def test_normal_full_suite_and_environment_precedence_are_unchanged(self):
        original = b'GOFLAGS=-run=^$\n'
        self.config.write_bytes(original)
        result, _ = self.verify(GOENV=str(self.config), GOFLAGS='-mod=readonly', SELECTED_FAILURE='0')
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(self.marker.is_file())
        self.assertNotIn('GOFLAGS test filters', result.stdout)
        self.assertEqual(self.config.read_bytes(), original)

if __name__ == '__main__':
    unittest.main()
