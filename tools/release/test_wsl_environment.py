"""Synthetic diagnostic tests; these do not establish actual WSL2 support."""
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / 'tools/wsl/check-environment.sh'


class WSLDiagnosticTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='jaekit-wsl-fixture-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve()
        self.bin = self.root / 'bin'
        self.bin.mkdir()
        self.project = self.root / '프로젝트 with spaces'
        self.project.mkdir()
        self.env = dict(os.environ, PATH=str(self.bin) + os.pathsep + os.environ['PATH'],
                        WSL_FIXTURE_ROOT=str(self.root), WSL_FIXTURE_FS='ext2/ext3')
        # Do not change HOME. Stub canonicalization only for the fixture's home;
        # all other paths use actual filesystem resolution.
        self.stub('realpath', '''#!/usr/bin/env python3
import os, pathlib, sys
p = sys.argv[-1]
print(os.environ['WSL_FIXTURE_ROOT'] if p == os.environ['HOME'] else pathlib.Path(p).resolve(strict=True))
''')
        self.stub('uname', '''#!/usr/bin/env python3
import os, sys
print({'-s':os.environ.get('WSL_FIXTURE_OS','Linux'), '-r':os.environ.get('WSL_FIXTURE_KERNEL','6.6.87.2-microsoft-standard-WSL2'), '-m':os.environ.get('WSL_FIXTURE_CPU','x86_64')}[sys.argv[1]])
''')
        self.stub('cat', '''#!/usr/bin/env python3
import os, sys
assert sys.argv[1:] == ['/etc/os-release']
print('ID=' + os.environ.get('WSL_FIXTURE_DISTRO','ubuntu'))
print('VERSION_ID="24.04"')
''')
        self.stub('stat', '''#!/usr/bin/env python3
import os
print(os.environ['WSL_FIXTURE_FS'])
''')
        for name in ('git', 'ha', 'codex'):
            self.stub(name, '#!/bin/sh\necho DO_NOT_EXECUTE >&2\nexit 99\n')

    def stub(self, name, content):
        path = self.bin / name
        path.write_text(content)
        path.chmod(0o755)

    def run_check(self, project=None, host='codex', **changes):
        return subprocess.run([shutil.which('bash'), str(SCRIPT), str(project or self.project), host],
                              env=dict(self.env, **changes), capture_output=True, text=True)

    def test_scoped_paths_are_reported_without_executing_tools(self):
        result = self.run_check()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(str(self.project.resolve()), result.stdout)
        self.assertIn('actual goal execution remain unverified', result.stdout)
        self.assertNotIn('DO_NOT_EXECUTE', result.stdout + result.stderr)
        self.assertEqual(list(self.project.iterdir()), [])

    def test_unsupported_environments_refused(self):
        for changes in ({'WSL_FIXTURE_OS': 'Darwin'}, {'WSL_FIXTURE_KERNEL': '6.8.0-generic'},
                        {'WSL_FIXTURE_CPU': 'aarch64'}, {'WSL_FIXTURE_DISTRO': 'debian'},
                        {'WSL_FIXTURE_FS': '9p'}):
            with self.subTest(changes=changes):
                self.assertNotEqual(self.run_check(**changes).returncode, 0)

    def test_symlink_escape_refused(self):
        with tempfile.TemporaryDirectory() as outside:
            link = self.project / 'external'
            link.symlink_to(outside, target_is_directory=True)
            self.assertNotEqual(self.run_check(link).returncode, 0)

    def test_windows_executable_and_nonexistent_host_refused(self):
        self.stub('host.exe', '#!/bin/sh\nexit 0\n')
        self.assertNotEqual(self.run_check(host='host.exe').returncode, 0)
        self.assertNotEqual(self.run_check(host='no-such-jaekit-host').returncode, 0)

    def test_os_release_is_not_executed(self):
        marker = self.project / 'unexpected'
        self.stub('cat', '#!/bin/sh\nprintf \'%s\\n\' \'ID=ubuntu\' \'VERSION_ID="24.04"\' ' +
                  "'IGNORED=$(touch \"" + str(marker) + "\")'\n")
        self.assertEqual(self.run_check().returncode, 0)
        self.assertFalse(marker.exists())


if __name__ == '__main__':
    unittest.main()
