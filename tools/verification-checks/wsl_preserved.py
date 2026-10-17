#!/usr/bin/env python3
"""Direct product regressions for WSL preparation; no WSL runtime claim.

The repository's full tools/verify.py is still required before commits. This
bounded check runs product tests, builds, public checks and release lifecycle
tests without recursively testing the verification entrypoint in copied repos.
"""
import importlib.util
import os
from pathlib import Path
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]
spec = importlib.util.spec_from_file_location('jaekit_verify', ROOT / 'tools/verify.py')
verify = importlib.util.module_from_spec(spec)
spec.loader.exec_module(verify)


def main():
    env = dict(os.environ, PYTHONDONTWRITEBYTECODE='1', GOTOOLCHAIN='local')
    env.pop('GOOS', None)
    env.pop('GOARCH', None)
    try:
        verify.prepare_shells(env)
        verify.run('Go formatting', ['gofmt', '-l', '.'], env, formatting=True)
        verify.run('Go vet', ['go', 'vet', '-mod=readonly', './...'], env)
        verify.run('All Core and plugin tests', ['go', 'test', '-mod=readonly', './...'], env)
        with tempfile.TemporaryDirectory(prefix='jaekit-wsl-preserved-') as directory:
            for target in verify.TARGETS:
                goos, goarch = target.split('/')
                verify.run('Build ' + target, ['go', 'build', '-mod=readonly', '-o',
                           str(Path(directory) / (goos + '-' + goarch)), './cmd/ha'],
                           dict(env, GOOS=goos, GOARCH=goarch, CGO_ENABLED='0'))
            binary = str(Path(directory) / 'ha')
            verify.run('Native smoke build', ['go', 'build', '-mod=readonly', '-o', binary, './cmd/ha'], env)
            verify.run('Native version', [binary, '--version'], env)
            verify.run('Native capabilities', [binary, 'capabilities', '--format', 'json'], env)
        for path in ('tools/check_public_tree.py', 'tools/check_public_docs.py'):
            verify.run(path, [sys.executable, path], env)
        verify.run('Release archives and install lifecycle', [sys.executable, '-m', 'unittest',
                   'discover', '-s', 'tools/release', '-p', 'test_*.py'], env)
    except verify.CheckFailed as error:
        print('FAIL:', error, file=sys.stderr)
        return 1
    print('PASS: local product preservation; actual WSL2 execution remains unverified.')
    return 0


if __name__ == '__main__':
    raise SystemExit(main())
