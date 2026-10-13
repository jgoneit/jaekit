#!/usr/bin/env python3
"""Observe decision history through the real `ha history` CLI.

Each criterion builds `ha` from this checkout and drives it against synthetic
local repositories. No declaration is read to choose an observed violation: a
runnable CLI without the command is reported as the missing feature, distinct
from build, setup, and process errors.
"""
from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]


class Violation(Exception):
    def __init__(self, code, detail):
        super().__init__(code)
        self.code, self.detail = code, detail


class Environment(Exception):
    def __init__(self, reason, detail):
        super().__init__(reason)
        self.reason, self.detail = reason, detail


def require(value, code, detail):
    if not value:
        raise Violation(code, detail)


def run(argv, cwd, env=None, timeout=300):
    try:
        return subprocess.run([str(x) for x in argv], cwd=cwd, env=env,
                              text=True, capture_output=True, timeout=timeout)
    except (OSError, subprocess.TimeoutExpired) as error:
        raise Environment('execution_error', str(error)) from error


def build(temp):
    binary = temp / 'ha'
    result = run(['go', 'build', '-o', binary, './cmd/ha'], ROOT)
    if result.returncode:
        raise Environment('compile_error', result.stderr)
    return binary


def probe(binary):
    cap = run([binary, 'capabilities', '--format', 'json'], ROOT)
    if cap.returncode:
        raise Environment('setup_error', 'capabilities failed: ' + cap.stderr)
    try:
        features = json.loads(cap.stdout).get('features', [])
    except ValueError as error:
        raise Environment('setup_error', 'capabilities output invalid') from error
    help_ = run([binary, 'history', '--help'], ROOT)
    if 'unknown command "history"' in help_.stderr and 'decision-history/v1' not in features:
        raise Violation('history-command-unavailable', 'runnable CLI has no history command')
    require('decision-history/v1' in features, 'history-capability-missing',
            'history command exists without a support declaration')


def observe(binary, n, temp):
    raise Environment('setup_error', f'criterion {n} observations are not written yet')


def documents(n):
    raise Violation('history-progress-roles-missing', 'Seal PROGRESS roles are not described yet')


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--criterion', required=True, type=int)
    args = parser.parse_args()
    result = {'target': f'AC-{args.criterion}', 'attempt': 1, 'status': 'pass'}
    try:
        with tempfile.TemporaryDirectory(prefix='jaekit-history-') as tmp:
            temp = Path(tmp)
            if args.criterion == 8:
                documents(args.criterion)
            else:
                binary = build(temp)
                probe(binary)
                observe(binary, args.criterion, temp)
            print('Observed decision history properties: AC-%d' % args.criterion)
    except Violation as error:
        result.update(status='violation', violation=error.code)
        print(error.code + ': ' + error.detail, file=sys.stderr)
    except Environment as error:
        result.update(status='error', reason=error.reason)
        print(error.detail, file=sys.stderr)
    except Exception as error:  # noqa: BLE001 - every unexpected failure is an error
        result.update(status='error', reason='execution_error')
        print(repr(error), file=sys.stderr)
    if 'HA_EVIDENCE_PATH' in os.environ:
        report = {'schema': 'check-result/v1', 'invocation': os.environ['HA_EVIDENCE_INVOCATION'],
                  'declaration_digest': os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
                  'producer': 'jaekit-decision-history', 'producer_version': '1',
                  'attempts_complete': True, 'observations': [result]}
        Path(os.environ['HA_EVIDENCE_PATH']).write_text(json.dumps(report) + '\n')
    return 0 if result['status'] == 'pass' else 1


if __name__ == '__main__':
    raise SystemExit(main())
