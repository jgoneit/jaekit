#!/usr/bin/env python3
"""Bounded WSL documentation/diagnostic observations, not native WSL evidence."""
import importlib.util
import json
import os
from pathlib import Path
import sys
import unittest


def observe(criterion):
    observation = {'target': criterion, 'attempt': 1}
    if criterion in ('AC-2', 'AC-3'):
        # Deliberately pending until actual package and host observations exist.
        return dict(observation, status='skip', reason='not_selected')
    required = ['guides/WSL.md', 'guides/WSL.en.md', 'tools/wsl/check-environment.sh']
    if not all(Path(p).is_file() for p in required):
        return dict(observation, status='violation', violation='wsl-route-missing')
    ko, en = [Path(p).read_text(encoding='utf-8') for p in required[:2]]
    if criterion == 'AC-1':
        ok = all(token in ko and token in en for token in
                 ('Ubuntu 24.04', 'WSL2', 'x86_64', '/mnt/c', 'Git', 'Homebrew', 'WSL1'))
        ok = ok and 'Linux home directory' in en and 'same distribution' in en
        for name, guide in [('README.md', 'guides/WSL.md'), ('README.en.md', 'guides/WSL.en.md'),
                            ('guides/INSTALL.md', 'WSL.md'), ('guides/INSTALL.en.md', 'WSL.en.md')]:
            ok = ok and guide in Path(name).read_text(encoding='utf-8')
    elif criterion == 'AC-7':
        test_path = Path('tools/release/test_wsl_environment.py')
        spec = importlib.util.spec_from_file_location('wsl_diagnostic_tests', test_path)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        result = unittest.TextTestRunner().run(unittest.defaultTestLoader.loadTestsFromModule(module))
        if result.errors:
            return dict(observation, status='error', reason='setup_error')
        ok = result.wasSuccessful() and 'does not automatically install' in en and 'current Linux shell' in en
    elif criterion == 'AC-9':
        ok = all(token in en for token in ('not been checked yet', 'candidate package',
                 'ordinary Linux', 'host, model, and Skill version', 'assurance `local`'))
        ok = ok and all(token in ko for token in ('아직 확인하지 않았습니다', '후보', '일반 Linux', 'local'))
        ok = ok and 'tools/wsl/check-environment.sh' in Path('.github/workflows/release.yml').read_text()
    else:
        raise ValueError('unknown criterion')
    return dict(observation, status='pass') if ok else dict(observation, status='violation', violation='wsl-route-missing')


def main():
    criterion = sys.argv[1]
    try:
        observation = observe(criterion)
    except (OSError, ValueError, ImportError) as exc:
        print(type(exc).__name__, str(exc), file=sys.stderr)
        observation = {'target': criterion, 'attempt': 1, 'status': 'error', 'reason': 'setup_error'}
    report = {'schema': 'check-result/v1', 'producer': 'jaekit-wsl-route', 'producer_version': '1',
              'invocation': os.environ['HA_EVIDENCE_INVOCATION'],
              'declaration_digest': os.environ['HA_EVIDENCE_DECLARATION_DIGEST'],
              'attempts_complete': True, 'observations': [observation]}
    with open(os.environ['HA_EVIDENCE_PATH'], 'x', encoding='utf-8') as output:
        json.dump(report, output)
        output.write('\n')
    print(criterion, observation['status'], '(local document/fixture evidence only)')
    return 0 if observation['status'] == 'pass' else 1


if __name__ == '__main__':
    raise SystemExit(main())
