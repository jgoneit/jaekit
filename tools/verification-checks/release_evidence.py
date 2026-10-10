#!/usr/bin/env python3
"""Independent, bounded observations of release evidence; no remote/model calls."""
import contextlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
from unittest import mock

ROOT = Path.cwd()
ENV = {key: value for key, value in os.environ.items() if not key.startswith(('GIT_', 'HA_EVIDENCE_'))}
ENV.update(PYTHONDONTWRITEBYTECODE='1', GOTOOLCHAIN='local', GOFLAGS='-mod=readonly',
           GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM='1', GIT_AUTHOR_NAME='Synthetic',
           GIT_AUTHOR_EMAIL='synthetic@example.invalid', GIT_COMMITTER_NAME='Synthetic', GIT_COMMITTER_EMAIL='synthetic@example.invalid')


class ObservedViolation(Exception):
    def __init__(self, identity, detail):
        self.identity = identity
        super().__init__(detail)


def require(condition, identity, detail):
    if not condition:
        raise ObservedViolation(identity, detail)


def load(name, path):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def run(argv, cwd=ROOT, success=True):
    result = subprocess.run(argv, cwd=cwd, env=ENV, capture_output=True, text=True, timeout=900)
    if success and result.returncode:
        raise RuntimeError('controlled command failed: ' + repr(argv) + '\n' + result.stdout + result.stderr)
    return result


def regression(filename, name=None):
    argv = [sys.executable, '-B', '-m', 'unittest', 'discover', '-s', 'tools/release-checks', '-p', filename]
    if name:
        argv += ['-k', name]
    result = run(argv, success=False)
    print(result.stdout + result.stderr)
    if result.returncode or 'Ran 0 tests' in result.stderr:
        raise RuntimeError('selected current regression did not complete')


def ac1():
    checker = load('release_observed', 'tools/release-checks/check.py')
    command = 'echo `unterminated\nha start goal\nha check goal --baseline AC-1\nha done goal'
    for shell in ('bash', 'zsh'):
        executable = ENV.get('JAEKIT_TEST_' + shell.upper()) or shutil.which(shell)
        if not executable:
            raise RuntimeError('required local shell unavailable: ' + shell)
        result = run([executable, '-n', '-c', command], success=False)
        if result.returncode == 0:
            raise RuntimeError('controlled invalid shell fixture was unexpectedly valid')
    try:
        unknown = checker.shell_analysis(command).unknown
    except checker.Unavailable:
        unknown = True
    require(unknown, 'invalid-syntax-certified', 'invalid actual shell syntax became a definite execution/absence claim')
    regression('integration_evidence_boundaries.py', 'SyntaxEvidence')


def ac2():
    checker = load('release_observed', 'tools/release-checks/check.py')
    for command in ('exec /usr/bin/true\nha start goal', 'exit 0\nha start goal'):
        analysis = checker.shell_analysis(command)
        require(not any(op == 'start' for op, _ in analysis.calls), 'unreached-call-certified', 'unreached call became definite execution')
    regression('integration_evidence_boundaries.py', 'ReachabilityEvidence')


def ac3():
    fixture = load('release_fixture', 'tools/release-checks/test_loader_invocations.py')
    case = fixture.TraceCase()
    case.setUp()
    try:
        value, _ = case.claude()
        accepted = True
        try:
            case.checker.trace('claude', value, 'seal')
        except (fixture.check.Unavailable, fixture.check.Failure):
            accepted = False
        require(not accepted, 'unbound-execution-certified', 'static Seal text supplied execution identity and original completion')
    finally:
        case.doCleanups()
    regression('integration_core_observation.py', 'test_execution_identity')
    regression('integration_core_observation.py', 'test_other_goal')
    regression('integration_core_observation.py', 'CoreImageIsolation')


def ac4():
    checker = load('release_observed', 'tools/release-checks/check.py')
    with tempfile.TemporaryDirectory(prefix='jaekit-install-observer-') as directory:
        root = Path(directory)
        binary = root / 'ha'
        binary.write_text(f'#!{sys.executable}\nimport sys,json\nprint("ha 0.1.3" if "--version" in sys.argv else json.dumps({{"schema":"ha-capabilities/v1","ha_version":"0.1.3","default_rules":"run-rules/3"}}))\n')
        binary.chmod(0o755)
        def artifact(path):
            return {'path': str(path), 'sha256': checker.digest(path.read_bytes())}
        def capture(argv, output):
            path = root / str(len(list(root.iterdir())))
            path.write_bytes(output)
            empty = root / (path.name + '-err'); empty.write_bytes(b'')
            return {'argv': argv, 'exit_code': 0, 'stdout': artifact(path), 'stderr': artifact(empty)}
        value = {'binary': artifact(binary), 'package_root': str(root), 'install': capture(['/usr/bin/true'], b'')}
        for name, flags in (('version', ['--version']), ('capabilities', ['capabilities', '--format', 'json'])):
            argv = [str(binary), *flags]
            value[name] = capture(argv, run(argv).stdout.encode())
        selected = checker.Checker(root)
        selected._evidence = {'installations': {'direct': value}}
        accepted = True
        with mock.patch.object(selected, 'download', return_value=b'synthetic archive'), \
             mock.patch.object(checker, 'archive_files', return_value={'ha': binary.read_bytes()}):
            try:
                selected.installation('direct')
            except (checker.Unavailable, checker.Failure):
                accepted = False
        require(not accepted, 'installation-substituted', 'preplaced binary and true capture supplied fresh installation proof')
    regression('integration_direct_install.py')


def ac5():
    checker = load('release_observed', 'tools/release-checks/check.py')
    with tempfile.TemporaryDirectory(prefix='jaekit-source-observer-') as directory:
        root = Path(directory)
        path = root / 'source.py'; path.write_text('verified source\n')
        for arguments in (('init', '-q'), ('add', '.'), ('commit', '-qm', 'synthetic source')):
            run(['git', *arguments], root)
        tree = run(['git', 'rev-parse', 'HEAD^{tree}'], root).stdout.strip()
        commit = run(['git', 'rev-parse', 'HEAD'], root).stdout.strip()
        evidence = root / 'evidence'; evidence.mkdir()
        (evidence / 'evidence.json').write_text(json.dumps({'schema':'jaekit-release-evidence/v1',
            'release':{'tag':'v0.1.3','source_commit':'a' * 40}, 'checker_source':{'commit':commit,'tree':tree}}))
        path.write_text('different source\n')
        out, err = io.StringIO(), io.StringIO()
        with mock.patch.object(checker, 'ROOT', root), \
             mock.patch.object(checker, 'LOADED_SOURCE', {str(path):path.read_bytes()}, create=True), \
             mock.patch.object(checker, 'api', return_value={'sha':'a' * 40,'commit':{'tree':{'sha':tree}}}), \
             mock.patch.object(checker.Checker, 'release', return_value={}), \
             mock.patch.object(checker.Checker, 'check_10'), contextlib.redirect_stdout(out), contextlib.redirect_stderr(err):
            result = checker.main(['AC-10', '--evidence', str(evidence)])
        require(result != 0, 'unverified-source-used', 'individual selection accepted a dirty source checkout')
        require(path.read_text() == 'different source\n', 'unverified-source-used', 'source guard removed a user change')
    regression('integration_evidence_boundaries.py', 'SourceIdentityEvidence')
    regression('integration_product_source.py')


def ac6():
    # Exercise the collector protocol, not source text or the expected label.
    with tempfile.TemporaryDirectory(prefix='jaekit-flow-observer-') as directory:
        root = Path(directory)
        goal = root / 'docs/specs/synthetic'; goal.mkdir(parents=True)
        (root / 'sentinel').write_text('synthetic project\n')
        for arguments in (('init', '-q'), ('add', '.'), ('commit', '-qm', 'synthetic project')):
            run(['git', *arguments], root)
        binary = root / 'core'
        run(['go', 'build', '-mod=readonly', '-ldflags=-X main.version=0.1.3', '-o', str(binary), './cmd/ha'])
        shell = ENV.get('JAEKIT_TEST_BASH') or shutil.which('bash')
        if not shell:
            raise RuntimeError('required local shell unavailable: bash')
        report = root / 'observation.json'
        result = run([sys.executable, str(ROOT / 'tools/release-checks/observe.py'), 'run', '--core', str(binary),
                      '--shell', shell, '--output', str(report), '--invocation', 'synthetic-flow',
                      '--project', str(root), '--goal', 'docs/specs/synthetic', '--', '"$JAEKIT_CORE" --version'], root, success=False)
        if result.returncode and 'unrecognized arguments: --project' in result.stderr:
            raise ObservedViolation('connected-local-flow-missing', 'collector cannot collect the required original project/goal boundary')
        if result.returncode or not report.is_file():
            raise RuntimeError('collector could not observe local Core: ' + result.stderr)
        value = json.loads(report.read_bytes())
        require(isinstance(value.get('context'), dict) and value['context'].get('goal') == 'docs/specs/synthetic',
                'connected-local-flow-missing', 'actual local collector omitted the project/goal binding')
    regression('integration_core_observation.py', 'test_actual_supported')
    regression('integration_core_observation.py', 'test_valid_baseline')
    regression('integration_direct_install.py', 'test_actual_fresh')


def ac8():
    verifier = load('shared_verification', 'tools/verify.py')
    selected = []
    verifier.run = lambda label, command, env, **kwargs: selected.append(command)
    verifier.verify_go(dict(ENV))
    expected = ('integration_evidence_boundaries.py', 'integration_core_observation.py', 'integration_direct_install.py', 'integration_product_source.py')
    require(all(any('tools/release-checks' in command and name in command for command in selected) for name in expected),
            'release-regression-not-selected', 'common Go entry omits required release evidence regressions')
    for name in expected:
        regression(name)
    result = run([sys.executable, '-B', '-m', 'unittest', 'discover', '-s', 'tools/workflow-checks', '-p', 'test_verify.py'], success=False)
    print(result.stdout + result.stderr)
    if result.returncode:
        raise RuntimeError('common selection and failure propagation regression failed')



def main():
    criterion = sys.argv[1]
    observation = {'target':criterion,'attempt':1,'status':'pass'}
    try:
        globals()[criterion.lower().replace('-', '')]()
    except ObservedViolation as error:
        observation.update(status='violation', violation=error.identity)
        print(str(error), file=sys.stderr)
    except Exception as error:
        observation.update(status='error', reason='execution_error')
        print(type(error).__name__ + ': ' + str(error), file=sys.stderr)
    if 'HA_EVIDENCE_PATH' in os.environ:
        report = {'schema':'check-result/v1','producer':'jaekit-release-observer','producer_version':'1',
                  'invocation':os.environ['HA_EVIDENCE_INVOCATION'],'declaration_digest':os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
                  'attempts_complete':True,'observations':[observation]}
        with open(os.environ['HA_EVIDENCE_PATH'], 'x') as stream:
            json.dump(report, stream)
    return 0 if observation['status'] == 'pass' else 1


if __name__ == '__main__':
    raise SystemExit(main())
