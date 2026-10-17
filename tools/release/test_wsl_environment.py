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
        # The script asks the user's Git about the project, so it is a real
        # repository and Git is not stubbed.
        self.git_init(self.project)
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
        # Paths under WSL_FIXTURE_DRIVE stand for a mounted Windows drive.
        self.stub('stat', '''#!/usr/bin/env python3
import os, sys
drive = os.environ.get('WSL_FIXTURE_DRIVE')
print('9p' if drive and sys.argv[-1].startswith(drive) else os.environ['WSL_FIXTURE_FS'])
''')
        for name in ('ha', 'codex'):
            self.stub(name, '#!/bin/sh\necho DO_NOT_EXECUTE >&2\nexit 99\n')

    def stub(self, name, content):
        path = self.bin / name
        path.write_text(content)
        path.chmod(0o755)

    @staticmethod
    def git_init(path):
        subprocess.run(['git', 'init', '-q', str(path)], check=True)

    def project_files(self):
        return sorted(path.relative_to(self.project) for path in self.project.rglob('*'))

    def run_check(self, project=None, host='codex', **changes):
        return subprocess.run([shutil.which('bash'), str(SCRIPT), str(project or self.project), host],
                              env=dict(self.env, **changes), capture_output=True, text=True)

    def test_scoped_paths_are_reported_without_executing_tools(self):
        before = self.project_files()
        result = self.run_check()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f'Repository: {self.project.resolve()}', result.stdout)
        self.assertIn(f'Common Git directory: {self.project.resolve() / ".git"}', result.stdout)
        self.assertIn('actual goal execution remain unverified', result.stdout)
        self.assertNotIn('DO_NOT_EXECUTE', result.stdout + result.stderr)
        self.assertEqual(self.project_files(), before)

    def test_project_inside_a_repository_reports_its_root(self):
        nested = self.project / 'nested'
        nested.mkdir()
        result = self.run_check(nested)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn(f'Repository: {self.project.resolve()}', result.stdout)

    def test_directory_outside_a_git_worktree_refused(self):
        plain = self.root / 'plain'
        plain.mkdir()
        result = self.run_check(plain)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Git does not recognize the project as a worktree', result.stderr)
        self.assertNotIn('prerequisites identified', result.stdout)

    def test_repository_rooted_at_or_above_home_refused(self):
        # Home itself is a repository, so the plain directory's worktree root
        # is home rather than a project under it.
        self.git_init(self.root)
        plain = self.root / 'plain'
        plain.mkdir()
        result = self.run_check(plain)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('Git repository root under the Linux home', result.stderr)

    def test_git_directories_on_a_windows_drive_refused(self):
        drive = self.root / 'drive'
        drive.mkdir()
        # A linked worktree under home whose repository lives on the drive,
        # and a repository whose separate Git directory is on the drive.
        main = drive / 'main'
        self.git_init(main)
        subprocess.run(['git', '-C', str(main), '-c', 'user.name=Fixture', '-c',
                        'user.email=fixture@example.invalid', 'commit', '-q', '--no-verify',
                        '--allow-empty', '-m', 'fixture'], check=True)
        linked = self.root / 'linked'
        subprocess.run(['git', '-C', str(main), 'worktree', 'add', '-q', str(linked)], check=True)
        separate = self.root / 'separate'
        subprocess.run(['git', 'init', '-q', '--separate-git-dir', str(drive / 'separate.git'),
                        str(separate)], check=True)
        for project in (linked, separate):
            with self.subTest(project=project.name):
                result = self.run_check(project, WSL_FIXTURE_DRIVE=str(drive))
                self.assertNotEqual(result.returncode, 0)
                self.assertIn('filesystem is outside the checked Linux route (9p)', result.stderr)
                self.assertIn(str(drive), result.stderr)
                self.assertNotIn('prerequisites identified', result.stdout)

    def test_unsupported_environments_refused(self):
        for changes in ({'WSL_FIXTURE_OS': 'Darwin'}, {'WSL_FIXTURE_KERNEL': '6.8.0-generic'},
                        {'WSL_FIXTURE_CPU': 'aarch64'}, {'WSL_FIXTURE_DISTRO': 'debian'},
                        {'WSL_FIXTURE_FS': '9p'}, {'WSL_FIXTURE_FS': 'overlayfs'},
                        {'WSL_FIXTURE_FS': 'tmpfs'}):
            with self.subTest(changes=changes):
                result = self.run_check(**changes)
                self.assertNotEqual(result.returncode, 0)
                self.assertNotIn('prerequisites identified', result.stdout)

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
