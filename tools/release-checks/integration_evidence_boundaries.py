"""Controlled local shell and observation boundaries; no host/model invocation."""
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SPEC = importlib.util.spec_from_file_location("boundary_fixtures", ROOT / "tools/review-checks/test_observation.py")
fixtures = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(fixtures)
check = fixtures.check


def shells():
    for name in ("bash", "zsh"):
        path = os.environ.get("JAEKIT_TEST_" + name.upper()) or shutil.which(name)
        if not path:
            raise RuntimeError("required local shell unavailable: " + name)
        yield name, [path, *( ["--noprofile", "--norc"] if name == "bash" else ["-f"])]


class SyntaxEvidence(fixtures.Case):
    def test_incomplete_syntax_is_not_execution_or_absence_in_either_host(self):
        examples = ("echo `unterminated\nha start goal", "echo ${unclosed\nha start goal",
                    "echo $((1 + 2)\nha start goal", 'echo "$(true\nha start goal',
                    'echo "unterminated\nha start goal')
        for source in examples:
            for name, argv in shells():
                with self.subTest(shell=name, source=source):
                    proc = subprocess.run([*argv, "-n", "-c", source], capture_output=True, timeout=10)
                    self.assertNotEqual(proc.returncode, 0, "fixture must actually be invalid shell syntax")
            for host in ("codex", "claude"):
                with self.subTest(host=host, source=source):
                    value = self.codex_commands("spec", (source,)) if host == "codex" else self.bound_claude("spec", (source,))
                    with self.assertRaisesRegex(check.Unavailable, "unverified.*(tool-codex-0|tool-1-0).*(unterminated|syntax)"):
                        self.checker.trace(host, value, "spec")


class ReachabilityEvidence(unittest.TestCase):
    def test_definite_calls_agree_with_actual_supported_shell_reachability(self):
        cases = (("exec /usr/bin/true\nha start goal", False),
                 ("exit 0\nha start goal", False),
                 ("{ exit 0; }; ha start goal", False),
                 ("(exit 0); ha start goal", True),
                 ("exec > installed-output\nha start goal", True),
                 ("exec /usr/bin/true | cat\nha start goal", True),
                 ("if false; then exec /usr/bin/true; fi\nha start goal", True))
        for name, argv in shells():
            for source, expected in cases:
                with self.subTest(shell=name, source=source), tempfile.TemporaryDirectory(prefix="jaekit-reachability-") as directory:
                    root = Path(directory)
                    marker = root / "actual.jsonl"
                    binary = root / "ha"
                    binary.write_text(f"#!{sys.executable}\nimport json,sys\nwith open({str(marker)!r},'a') as stream: stream.write(json.dumps(sys.argv[1:])+'\\n')\n")
                    binary.chmod(0o755)
                    env = {k: v for k, v in os.environ.items() if k not in ("BASH_ENV", "ENV", "ZDOTDIR") and not k.startswith("BASH_FUNC_")}
                    env["PATH"] = str(root) + os.pathsep + env.get("PATH", "")
                    proc = subprocess.run([*argv, "-c", source], cwd=root, env=env, capture_output=True, timeout=10)
                    self.assertEqual(proc.returncode, 0, proc.stderr)
                    actual = [json.loads(line) for line in marker.read_text().splitlines()] if marker.exists() else []
                    self.assertEqual(bool(actual), expected, "controlled actual shell fixture differs")
                    analysis = check.shell_analysis(source)
                    definite = [[operation, *arguments] for operation, arguments in analysis.calls]
                    self.assertTrue(all(call in actual for call in definite), "unreached command became definite execution")
                    if "(exit 0)" in source or "exec >" in source:
                        self.assertIn(["start", "goal"], definite, "child exit or redirection-only exec stopped parent")


class SourceIdentityEvidence(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="jaekit-source-boundary-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        self.source = self.root / 'tools/release-checks/tool.py'
        self.source.parent.mkdir(parents=True)
        self.source.write_text('original source\n')
        self.git('init', '-q')
        self.git('config', 'user.name', 'Synthetic Test')
        self.git('config', 'user.email', 'synthetic@example.invalid')
        self.git('add', '.')
        self.git('commit', '-qm', 'synthetic source')
        self.tree = self.git('rev-parse', 'HEAD^{tree}').decode().strip()
        self.loaded = {str(self.source): self.source.read_bytes()}

    def git(self, *args):
        return subprocess.check_output(['git', *args], cwd=self.root, stderr=subprocess.DEVNULL)

    def test_clean_actual_source_accepts_and_dirty_index_or_bytes_do_not(self):
        identity = check.source_identity
        with identity.verified(self.root, self.tree, self.loaded):
            pass
        for staged in (False, True):
            with self.subTest(staged=staged):
                self.source.write_text('different source\n')
                if staged:
                    self.git('add', '.')
                with self.assertRaises(identity.SourceMismatch):
                    with identity.verified(self.root, self.tree, self.loaded):
                        self.fail('dirty source entered the selected check')
                self.git('reset', '--hard', '-q', 'HEAD')

    def test_source_change_diagnostic_names_paths_without_rendering_filename_controls(self):
        identity = check.source_identity
        unusual = self.source.with_name('source\n\x1b[31m한글.py')
        unusual.write_text('original source\n')
        self.git('add', '.')
        self.git('commit', '-qm', 'synthetic unusual source path')
        tree = self.git('rev-parse', 'HEAD^{tree}').decode().strip()
        for staged in (False, True):
            with self.subTest(staged=staged):
                self.source.write_text('ordinary changed source\n')
                unusual.write_text('unusual changed source\n')
                if staged:
                    self.git('add', '.')
                before = (self.source.read_bytes(), unusual.read_bytes(), self.git('status', '--porcelain=v1', '-z'))
                with self.assertRaises(identity.SourceMismatch) as caught:
                    identity.snapshot(self.root, tree, self.loaded)
                reason = str(caught.exception)
                for path in (self.source, unusual):
                    self.assertIn(ascii(path.relative_to(self.root).as_posix()), reason)
                self.assertNotIn('\n', reason)
                self.assertNotIn('\x1b', reason)
                self.assertEqual((self.source.read_bytes(), unusual.read_bytes(), self.git('status', '--porcelain=v1', '-z')), before)
                self.git('reset', '--hard', '-q', 'HEAD')

    def test_staged_change_is_rejected_when_worktree_bytes_match_head(self):
        identity = check.source_identity
        original = self.source.read_bytes()
        self.source.write_text('different staged source\n')
        self.git('add', '.')
        self.source.write_bytes(original)
        self.assertEqual(self.git('diff', '--name-only', 'HEAD', '--'), b'')
        self.assertTrue(self.git('diff', '--cached', '--name-only', 'HEAD', '--'))
        index = self.root / '.git/index'
        before = (self.source.read_bytes(), index.read_bytes())
        with self.assertRaises(identity.SourceMismatch):
            with identity.verified(self.root, self.tree, self.loaded):
                self.fail('different staged source was hidden by original worktree bytes')
        self.assertEqual((self.source.read_bytes(), index.read_bytes()), before)

    def test_late_changes_and_loaded_other_bytes_are_detected_without_cleanup(self):
        identity = check.source_identity
        with self.assertRaises(identity.SourceMismatch):
            with identity.verified(self.root, self.tree, self.loaded):
                self.source.write_text('changed during observation\n')
        self.assertEqual(self.source.read_text(), 'changed during observation\n')
        self.git('reset', '--hard', '-q', 'HEAD')
        with self.assertRaises(identity.SourceMismatch):
            with identity.verified(self.root, self.tree, {str(self.source): b'other loaded code'}):
                self.fail('different loaded source entered the selected check')

    def test_offline_public_entry_runs_a_valid_selection_inside_the_verified_copy(self):
        for name in ('check.py', 'shell_reader.py', 'execution_trace.py', 'execution_provenance.py', 'install_trace.py', 'source_identity.py', 'snapshot_worker.py'):
            shutil.copyfile(ROOT / 'tools/release-checks' / name, self.root / 'tools/release-checks' / name)
        for directory in ('tools/release', 'tools/release-checks'):
            path = self.root / directory / 'test_fixture.py'
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text('import unittest\nfrom pathlib import Path\nclass Case(unittest.TestCase):\n    def test_actual_selection(self): self.assertEqual(Path("source-marker").read_text(), "product")\n')
        (self.root / 'source-marker').write_text('product')
        self.git('add', '.')
        self.git('commit', '-qm', 'synthetic public product')
        product_tree = self.git('rev-parse', 'HEAD^{tree}').decode().strip()
        product_commit = self.git('rev-parse', 'HEAD').decode().strip()
        (self.root / 'source-marker').write_text('checker')
        self.git('add', '.')
        self.git('commit', '-qm', 'synthetic newer checker')
        tree = self.git('rev-parse', 'HEAD^{tree}').decode().strip()
        commit = self.git('rev-parse', 'HEAD').decode().strip()
        evidence = self.root / 'evidence'
        evidence.mkdir()
        (evidence / 'evidence.json').write_text(json.dumps({'schema': 'jaekit-release-evidence/v1',
            'release': {'tag': 'v0.1.3', 'source_commit': product_commit},
            'checker_source': {'commit': commit, 'tree': tree}}))
        # Only the public remote response is synthetic; selected source copy,
        # child worker, local Python regression commands and final CLI are real.
        script = ("import sys; sys.path.insert(0, 'tools/release-checks'); import check\n"
                  "from unittest import mock\n"
                  f"with mock.patch.object(check, 'api', return_value={{'sha':{product_commit!r}, 'commit':{{'tree':{{'sha':{product_tree!r}}}}}}}), mock.patch.object(check.Checker, 'release', return_value={{}}):\n"
                  f"    raise SystemExit(check.main(['AC-10', '--evidence', {str(evidence)!r}]))\n")
        result = subprocess.run([sys.executable, '-c', script], cwd=self.root, capture_output=True, text=True,
                                timeout=30, env=dict(os.environ, PYTHONDONTWRITEBYTECODE='1'))
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertIn('PASS AC-10', result.stdout)

    def test_intermediate_change_and_restore_cannot_change_executed_source(self):
        identity = check.source_identity
        original = self.source.read_bytes()
        with identity.pinned(self.root, self.tree, self.loaded) as selected:
            self.source.write_text('intermediate different source\n')
            # The child reads the selected source while the original is B.
            observed = subprocess.check_output([sys.executable, '-c',
                'from pathlib import Path; print(Path("tools/release-checks/tool.py").read_text(), end="")'], cwd=selected)
            self.source.write_bytes(original)
            self.assertEqual(observed, original)
        self.assertEqual(self.source.read_bytes(), original)

    def test_snapshot_preserves_tracked_ignored_paths_modes_and_exact_blobs(self):
        (self.root / '.gitignore').write_text('ignored-*\n')
        (self.root / '.gitattributes').write_text('*.data text eol=crlf\n')
        data = self.root / 'ignored-\nfile.data'
        data.write_bytes(b'exact bytes\n')
        executable = self.root / 'executable'
        executable.write_bytes(b'#!/bin/sh\nexit 0\n')
        executable.chmod(0o755)
        (self.root / 'link').symlink_to(data.name)
        self.git('add', '-f', '.')
        self.git('commit', '-qm', 'tracked unusual names and ignored source')
        tree = self.git('rev-parse', 'HEAD^{tree}').decode().strip()
        with check.source_identity.pinned(self.root, tree, self.loaded) as selected:
            self.assertEqual((selected / data.name).read_bytes(), b'exact bytes\n')
            self.assertTrue((selected / 'executable').stat().st_mode & 0o111)
            self.assertEqual(os.readlink(selected / 'link'), data.name)

    def test_git_environment_cannot_redirect_snapshot_writes(self):
        from unittest import mock
        with tempfile.TemporaryDirectory(prefix='jaekit-git-sentinel-') as directory:
            sentinel = Path(directory)
            identity = check.source_identity
            identity.git(sentinel, 'init', '-q')
            (sentinel / 'data').write_bytes(b'preserve this repository\n')
            identity.git(sentinel, 'config', 'user.name', 'Sentinel')
            identity.git(sentinel, 'config', 'user.email', 'sentinel@example.invalid')
            identity.git(sentinel, 'add', '.')
            identity.git(sentinel, 'commit', '-qm', 'sentinel')
            def contents(root):
                return {str(path.relative_to(root)): path.read_bytes() for path in root.rglob('*') if path.is_file()}
            original, other = contents(self.root), contents(sentinel)
            overrides = dict(GIT_DIR=str(sentinel / '.git'), GIT_WORK_TREE=str(sentinel),
                             GIT_INDEX_FILE=str(sentinel / '.git/index'), GIT_COMMON_DIR=str(sentinel / '.git'),
                             GIT_OBJECT_DIRECTORY=str(sentinel / '.git/objects'),
                             GIT_CONFIG_COUNT='1', GIT_CONFIG_KEY_0='core.bare', GIT_CONFIG_VALUE_0='true')
            with mock.patch.dict(os.environ, overrides):
                with identity.pinned(self.root, self.tree, self.loaded) as selected:
                    self.assertEqual(identity.git(selected, 'rev-parse', 'HEAD^{tree}').decode().strip(), self.tree)
            self.assertEqual(contents(self.root), original)
            self.assertEqual(contents(sentinel), other)

    def test_untracked_import_source_and_individual_cli_bypass_are_rejected(self):
        from unittest import mock
        identity = check.source_identity
        other = self.source.with_name('shadow.py')
        other.write_text('import side effect\n')
        with self.assertRaises(identity.SourceMismatch):
            with identity.verified(self.root, self.tree, self.loaded):
                self.fail('untracked source entered the selected check')
        other.unlink()
        checker = check.Checker()
        checker._evidence = {'release': {'source_commit': 'a' * 40},
                             'checker_source': {'commit': self.git('rev-parse', 'HEAD').decode().strip(), 'tree': self.tree}}
        self.source.write_text('individual selection changed\n')
        with mock.patch.object(check, 'ROOT', self.root), mock.patch.object(check, 'LOADED_SOURCE', self.loaded), \
             mock.patch.object(check, 'api', return_value={'sha': 'a' * 40, 'commit': {'tree': {'sha': self.tree}}}), \
             mock.patch.object(checker, 'release'), mock.patch.object(checker, 'check_7') as selected:
            with self.assertRaises(check.Failure):
                checker.selected('AC-7')
            selected.assert_not_called()


if __name__ == "__main__":
    unittest.main()
