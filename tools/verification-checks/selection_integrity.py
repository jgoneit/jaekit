#!/usr/bin/env python3
"""Observe real Go/Python selection boundaries and preserve execution distinctions."""
import argparse
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
    'AC-1': ['integration_selection_go.GoDefinitions', 'integration_selection_go_identifiers.GoIdentifierDefinitions'],
    'AC-2': ['integration_selection_go.GoBuildSelection', 'integration_selection_go_layout.GoLayoutSelection'],
    'AC-3': ['integration_selection_python.PythonFilterSelection'],
    'AC-4': ['integration_selection_python.PythonPatternSelection'],
    'AC-5': ['integration_selection_go.PreservedGoSelection',
             'integration_selection_python.PreservedPythonSelection',
             'test_map', 'test_checker', 'test_merge_blockers'],
}
SLOTS = 32
RUNNER = r'''
import json, sys, unittest
sys.path.insert(0, sys.argv[1])
from result_observations import run_suite
suite = unittest.defaultTestLoader.loadTestsFromNames(sys.argv[3:])
result = run_suite(suite, sys.stderr)
with open(sys.argv[2], 'x') as output:
    json.dump(result, output)
'''


def environment():
    env = {key: value for key, value in os.environ.items() if not key.startswith('HA_EVIDENCE_')}
    env['PYTHONDONTWRITEBYTECODE'] = '1'
    return env


def observe(criterion):
    with tempfile.TemporaryDirectory(prefix='jaekit-selection-results-') as temporary:
        output = Path(temporary) / 'facts.json'
        try:
            process = subprocess.run(
                [sys.executable, '-B', '-c', RUNNER, str(ROOT / 'tools'), str(output),
                 *SELECTIONS[criterion]], cwd=ROOT / 'tools/regression-checks',
                env=environment(), timeout=1800)
            if process.returncode or not output.is_file():
                raise RuntimeError('selected regression process did not produce facts')
            return json.loads(output.read_text())
        except (OSError, ValueError, RuntimeError, subprocess.TimeoutExpired) as error:
            print(error, file=sys.stderr)
            result = facts()
            result['errors'].append('execution_error')
            return result


def verify_component(component):
    """Check matching committed inputs, real component suites, then integration."""
    env = environment()
    status = subprocess.run(['git', 'status', '--porcelain'], cwd=component, env=env,
                            check=True, capture_output=True, text=True).stdout
    if status:
        raise RuntimeError('component checkout is dirty: ' + str(component))
    head = subprocess.run(['git', 'rev-parse', 'HEAD'], cwd=component, env=env,
                          check=True, capture_output=True, text=True).stdout.strip()
    print('Component commit:', head, flush=True)
    paths = ['tools/go_selection.py', 'tools/regression_map.py',
             'tools/regression-checks/integration_selection_go.py',
             'tools/regression-checks/integration_selection_python.py',
             'tools/regression-checks/integration_selection_go_identifiers.py',
             'tools/regression-checks/integration_selection_go_layout.py',
             'tools/regression-checks/integration_selection_go_runtime.py']
    for path in paths:
        if (component / path).read_bytes() != (ROOT / path).read_bytes():
            raise RuntimeError('component and integrated inputs differ: ' + path)
    subprocess.run([sys.executable, '-B', 'tools/regression_map.py', '--format', 'json'],
                   cwd=component, env=env, check=True, timeout=300)
    for filename in ('integration_selection_go.py', 'integration_selection_go_identifiers.py',
                     'integration_selection_go_layout.py', 'integration_selection_python.py',
                     'integration_selection_go_runtime.py'):
        subprocess.run([sys.executable, '-B', '-m', 'unittest', 'discover',
                        '-s', 'tools/regression-checks', '-p', filename],
                       cwd=component, env=env, check=True, timeout=1800)
    subprocess.run([sys.executable, '-B', 'tools/verify.py'],
                   cwd=ROOT, env=env, check=True, timeout=1800)
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('criterion', choices=[*SELECTIONS, 'AC-6'])
    parser.add_argument('--component-go', type=Path,
                        help='explicit component checkout for the AC-6 comparison')
    args = parser.parse_args()
    if args.criterion == 'AC-6':
        if args.component_go is None:
            parser.error('AC-6 requires an explicit component checkout')
        try:
            return verify_component(args.component_go.resolve())
        except (OSError, RuntimeError, subprocess.SubprocessError) as error:
            print('AC-6 non-pass:', error, file=sys.stderr)
            return 1
    if args.component_go is not None:
        parser.error('--component-go is only supported for AC-6')
    result = observe(args.criterion)
    report_observations = observations(result, args.criterion,
                                       target_names(args.criterion, slots=SLOTS))
    if 'HA_EVIDENCE_PATH' in os.environ:
        report = {'schema': 'check-result/v1', 'producer': 'jaekit-selection-integrity',
                  'producer_version': '1', 'invocation': os.environ['HA_EVIDENCE_INVOCATION'],
                  'declaration_digest': os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
                  'attempts_complete': True, 'observations': report_observations}
        with Path(os.environ['HA_EVIDENCE_PATH']).open('x') as output:
            json.dump(report, output)
    passed = all(item['status'] == 'pass' for item in report_observations)
    print(args.criterion, 'pass' if passed else 'non-pass')
    return 0 if passed else 1


if __name__ == '__main__':
    raise SystemExit(main())
