#!/usr/bin/env python3
"""Observe public Windows lifecycle boundaries and current-source PR evidence."""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys

sys.dont_write_bytecode = True
ROOT = Path(__file__).resolve().parents[2]
REPOSITORY = 'jgoneit/jaekit'
PULL_REQUEST = 67
REVIEWS = {4261377171, 4261548788}
VIOLATION = 'windows-lifecycle-boundary-missing'


class Violation(Exception):
    pass


class ObservationError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise Violation(message)


def run(argv, timeout=180):
    try:
        result = subprocess.run(argv, cwd=ROOT, capture_output=True, text=True, encoding='utf-8', timeout=timeout)
    except (OSError, UnicodeError, subprocess.TimeoutExpired) as error:
        raise ObservationError(type(error).__name__) from None
    if result.returncode:
        print(result.stdout, end='')
        print(result.stderr, end='', file=sys.stderr)
        raise ObservationError(f'{argv[0]} exited {result.returncode}')
    return result.stdout


def load(name, relative):
    spec = importlib.util.spec_from_file_location(name, ROOT / relative)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def digest(relative):
    return hashlib.sha256((ROOT / relative).read_bytes()).hexdigest()


def feature_boundary(criterion):
    acceptance = (ROOT / 'tools/windows/acceptance.ps1').read_text()
    common = (ROOT / 'tools/windows/common.ps1').read_text()
    uninstall = (ROOT / 'tools/windows/uninstall.ps1').read_text()
    readme = (ROOT / 'tools/windows/README.md').read_text()
    if criterion in {'AC-1', 'AC-2', 'AC-5', 'AC-7'}:
        require('$UpdateInstaller' in acceptance and '$UpdateUninstaller' in acceptance,
                'acceptance has no separate update-candidate code inputs; first-candidate code is fixed')
    if criterion in {'AC-3', 'AC-5', 'AC-7'}:
        require('function Get-OwnedPathRemoval' in common and 'Get-OwnedPathRemoval' in uninstall,
                'installer and process PATH lack the shared first-owned-entry removal boundary')
    if criterion in {'AC-5', 'AC-7'}:
        require((ROOT / 'tools/windows/native_acceptance_checks.ps1').is_file()
                and (ROOT / 'tools/windows/native_path_checks.ps1').is_file(),
                'required native candidate-dispatch/PATH runtime observations are absent')
    if criterion == 'AC-6':
        require('-UpdateInstaller $next.Install' in readme and '-UpdateUninstaller $next.Uninstall' in readme,
                'public acceptance example does not bind the verified update scripts')
        for name, markers in [('guides/WINDOWS.md', ('앞의 설치기 항목', '새 후보 코드')),
                              ('guides/WINDOWS.en.md', ('earlier installer entry', "new candidate's code"))]:
            source = (ROOT / name).read_text()
            require(all(marker in source for marker in markers), 'public lifecycle/PATH explanation missing: ' + name)


def live():
    ci = load('lifecycle_ci_observer', 'tools/verification-checks/cross_platform_ci.py')
    validator = load('lifecycle_native_results', 'tools/windows/lifecycle_results.py')
    repo = json.loads(run(['gh', 'repo', 'view', '--json', 'nameWithOwner']))['nameWithOwner']
    require(repo == REPOSITORY, 'wrong repository')
    original = ci.command
    reports = []

    def command(argv, *args, **kwargs):
        if argv[:3] == ['gh', 'pr', 'view']:
            argv = [*argv[:3], str(PULL_REQUEST), '--repo', repo, *argv[3:]]
        output = original(argv, *args, **kwargs)
        if argv[:3] == ['gh', 'run', 'view'] and '--log' in argv:
            reports.extend(ci.records(output, 'JAEKIT_CI_REPORT '))
        return output

    ci.command = command
    try:
        ci.configuration_present()
        repo, head = ci.live_run()
    except ci.Violation as error:
        raise Violation(str(error)) from None
    except ci.ObservationError as error:
        raise ObservationError(str(error)) from None
    windows = [r for r in reports if r.get('schema') == 'jaekit-native-windows/v1']
    require(len(windows) == 2, 'both Windows shell reports required')
    for report in windows:
        shell = report['environment']['shell_version']
        try:
            validator.acceptance_report(report.get('acceptance_dispatch'), shell, {
                name: digest(name) for name in ('tools/windows/acceptance.ps1', 'tools/windows/README.md')})
            validator.path_report(report.get('path_order_checks'), shell, {
                'common': digest('tools/windows/common.ps1'), 'uninstall': digest('tools/windows/uninstall.ps1')})
        except (ValueError, TypeError, KeyError) as error:
            raise Violation(str(error)) from None
        preflight = report.get('preflight_checks', {})
        require(preflight.get('status') == 'passed' and preflight.get('shell_version') == shell
                and not preflight.get('failures'), 'existing pre-execution checks did not pass in the same shell')
    return repo, head


def replies(repo, head):
    comments = json.loads(run(['gh', 'api', '--method', 'GET', f'repos/{repo}/pulls/{PULL_REQUEST}/comments?per_page=100']))
    require(len(comments) < 100, 'review page is truncated')
    user = json.loads(run(['gh', 'api', 'user']))['login']
    for review in REVIEWS:
        require(sum(row.get('id') == review and not row.get('in_reply_to_id') for row in comments) == 1,
                'target review missing')
        found = [row for row in comments if row.get('in_reply_to_id') == review
                 and row.get('user', {}).get('login') == user and head in row['body']]
        require(any('/actions/runs/' in row['body'] and 'Windows 11' in row['body'] for row in found),
                'current-commit reply with CI and environment boundary missing: ' + str(review))
    print('Both lifecycle review threads have current-commit replies with actual CI scope.')


def main():
    criterion = sys.argv[1]
    observation = {'target': criterion, 'attempt': 1, 'status': 'pass'}
    code = 0
    try:
        require(criterion in {f'AC-{n}' for n in range(1, 8)}, 'unknown criterion')
        feature_boundary(criterion)
        if criterion == 'AC-6':
            print(run([sys.executable, 'tools/check_public_tree.py']))
            print(run([sys.executable, 'tools/check_public_docs.py']))
        else:
            repo, head = live()
            if criterion == 'AC-7':
                replies(repo, head)
    except Violation as error:
        observation.update(status='violation', violation=VIOLATION)
        print(str(error), file=sys.stderr)
        code = 1
    except (ObservationError, OSError, ValueError, KeyError, TypeError) as error:
        observation.update(status='error', reason='execution_error')
        print(str(error), file=sys.stderr)
        code = 2
    if criterion != 'AC-4' and os.environ.get('HA_EVIDENCE_PATH'):
        Path(os.environ['HA_EVIDENCE_PATH']).write_text(json.dumps({
            'schema': 'check-result/v1', 'producer': 'windows-lifecycle-review', 'producer_version': '1',
            'invocation': os.environ.get('HA_EVIDENCE_INVOCATION', ''),
            'declaration_digest': os.environ.get('HA_EVIDENCE_DECLARATION_DIGEST', ''),
            'attempts_complete': True, 'observations': [observation],
        }) + '\n', encoding='utf-8')
    return code


if __name__ == '__main__':
    raise SystemExit(main())
