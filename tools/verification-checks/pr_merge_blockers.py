#!/usr/bin/env python3
"""Observe focused merge-blocker regressions with independent adverse facts."""
import argparse
import ast
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / 'tools'))
from result_observations import facts, observations, target_names

SELECTIONS = {
    'AC-1': ('regression-checks', ['test_merge_blockers.StaticGoSelection', 'integration_go_filters.FullGoEnvironment']),
    'AC-2': ('regression-checks', ['test_merge_blockers.ObserverOutcomes', 'integration_observer_core.ObserverCore']),
    'AC-3': ('release-checks', ['test_import_provenance.ImportProvenance', 'integration_install_imports.CollectorImports']),
    'AC-4': ('release-checks', ['integration_install_destination.InstallationPathContract']),
    'AC-5': ('release-checks', ['integration_install_destination.InstallationPathBinding']),
    'AC-6': ('regression-checks', ['test_map', 'test_checker', 'test_merge_blockers.PreservedOutcomes', 'test_merge_blockers.PreservedStaticSelection', 'test_merge_blockers.PreservedSelectedRunner', 'integration_go_filters.PreservedGoSelection', 'test_observer_attribution']),
    'AC-7': ('release-checks', ['test_import_provenance.ImportProvenanceKeep', 'integration_product_source']),
    'AC-8': ('release-checks', ['integration_direct_install', 'test_archive_limits']),
}
SLOTS = {"AC-1": 32, "AC-2": 16, "AC-3": 32, "AC-4": 4, "AC-5": 4}
RUNNER = r'''
import json, sys, unittest
sys.path.insert(0, sys.argv[1])
from result_observations import run_suite
suite = unittest.defaultTestLoader.loadTestsFromNames(sys.argv[3:])
result = run_suite(suite, sys.stderr)
with open(sys.argv[2], 'x') as output:
    json.dump(result, output)
'''


def observe(criterion):
    directory, names = SELECTIONS[criterion]
    env = {k: v for k, v in os.environ.items() if not k.startswith('HA_EVIDENCE_')}
    env['PYTHONDONTWRITEBYTECODE'] = '1'
    with tempfile.TemporaryDirectory(prefix='jaekit-merge-blocker-results-') as temporary:
        output = Path(temporary) / 'facts.json'
        try:
            process = subprocess.run([sys.executable, '-B', '-c', RUNNER, str(ROOT / 'tools'), str(output), *names],
                                     cwd=ROOT / 'tools' / directory, env=env, timeout=1800)
            if process.returncode or not output.is_file():
                raise RuntimeError('selected regression process did not produce facts')
            return json.loads(output.read_text())
        except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
            print(error, file=sys.stderr)
            result = facts()
            result['errors'].append('execution_error')
            return result


def verify_components(go_root, release_root):
    """Bind component checks to clean commits and the integrated product bytes."""
    groups = [
        (go_root, ['tools/regression_map.py', 'tools/verification-checks/review_regression_gates.py',
                   'tools/regression-checks/test_merge_blockers.py',
                   'tools/regression-checks/integration_go_filters.py',
                   'tools/regression-checks/integration_observer_core.py', 'tools/regression-checks/test_observer_attribution.py', 'tools/regression-checks/test_checker.py'],
         'regression-checks', ['test_merge_blockers.py', 'integration_go_filters.py', 'integration_observer_core.py', 'test_observer_attribution.py']),
        (release_root, ['tools/release-checks/check.py', 'tools/release-checks/source_identity.py',
                        'tools/release-checks/snapshot_worker.py', 'tools/release-checks/install_trace.py',
                        'tools/release-checks/test_import_provenance.py',
                        'tools/release-checks/integration_install_destination.py', 'tools/release-checks/integration_install_imports.py'],
         'release-checks', ['test_import_provenance.py', 'integration_install_destination.py', 'integration_install_imports.py']),
    ]
    env = {k: v for k, v in os.environ.items() if not k.startswith('HA_EVIDENCE_')}
    env['PYTHONDONTWRITEBYTECODE'] = '1'
    for component, paths, directory, filenames in groups:
        dirty = subprocess.run(['git', 'status', '--porcelain'], cwd=component, env=env,
                               check=True, capture_output=True, text=True).stdout
        if dirty:
            raise RuntimeError('component checkout is dirty: ' + str(component))
        head = subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=component, env=env,
                              check=True, capture_output=True, text=True).stdout.strip()
        print('Component commit:', directory, head, flush=True)
        for path in paths:
            if (component / path).read_bytes() != (ROOT / path).read_bytes():
                raise RuntimeError('component and integrated inputs differ: ' + path)
        def functions(root):
            tree = ast.parse((root / 'tools/verify.py').read_text())
            return {item.name: ast.dump(item, include_attributes=False) for item in tree.body
                    if isinstance(item, ast.FunctionDef)}
        integrated, applicable = functions(ROOT), functions(component)
        for name in ('go_flag_fields', 'narrowing_go_flag', 'full_go_environment', 'run'):
            if integrated.get(name) != applicable.get(name):
                raise RuntimeError('component Go verification differs: ' + name)
        for filename in filenames:
            subprocess.run([sys.executable, '-B', '-m', 'unittest', 'discover', '-s', 'tools/' + directory,
                            '-p', filename], cwd=component, env=env, check=True, timeout=1800)
        if component == release_root:
            subprocess.run([sys.executable, '-B', '-m', 'unittest', 'discover', '-s',
                            'tools/regression-checks', '-p', 'integration_go_filters.py'],
                           cwd=component, env=env, check=True, timeout=1800)
    subprocess.run([sys.executable, 'tools/verify.py'], cwd=ROOT, env=env, check=True)
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('criterion', choices=[*SELECTIONS, 'AC-9'])
    parser.add_argument('--component-go', type=Path)
    parser.add_argument('--component-release', type=Path)
    args = parser.parse_args()
    criterion = args.criterion
    if criterion == 'AC-9':
        if args.component_go is None or args.component_release is None:
            parser.error('AC-9 requires both explicit component checkouts')
        return verify_components(args.component_go.resolve(), args.component_release.resolve())
    result = observe(criterion)
    report_observations = observations(result, criterion, target_names(criterion, slots=SLOTS.get(criterion, 64)))
    if 'HA_EVIDENCE_PATH' in os.environ:
        report = {'schema': 'check-result/v1', 'producer': 'jaekit-merge-blockers', 'producer_version': '1',
                  'invocation': os.environ['HA_EVIDENCE_INVOCATION'],
                  'declaration_digest': os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
                  'attempts_complete': True, 'observations': report_observations}
        with Path(os.environ['HA_EVIDENCE_PATH']).open('x') as output:
            json.dump(report, output)
    passed = all(item['status'] == 'pass' for item in report_observations)
    print(criterion, 'pass' if passed else 'non-pass')
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
