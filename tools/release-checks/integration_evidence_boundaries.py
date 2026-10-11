"""Controlled local shell and observation boundaries; no host/model invocation."""
import base64
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


    def test_invoked_function_exit_and_return_reach_only_their_scope(self):
        # (source, actual shell reaches the later call, a definite call is required)
        cases = (("f() { exit 0; }; f; ha start goal", False, False),
                 ("f() { exit 0; }\nf\nha start goal", False, False),
                 ("g() { exit 0; }; f() { g; }; f; ha start goal", False, False),
                 ("f() { { exit 0; }; }; f; ha start goal", False, False),
                 ("f() { exec /usr/bin/true; }; f; ha start goal", False, False),
                 ("f() { return 0; }; f; ha start goal", True, True),
                 ("f() { (exit 0); }; f; ha start goal", True, True),
                 ("f() { exit 0; }; f | cat; ha start goal", True, True),
                 ("f() { if false; then exit 0; fi; }; f; ha start goal", True, False))
        for name, argv in shells():
            for source, reached, required in cases:
                with self.subTest(shell=name, source=source), tempfile.TemporaryDirectory(prefix="jaekit-function-scope-") as directory:
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
                    self.assertEqual(bool(actual), reached, "controlled actual shell fixture differs")
                    analysis = check.shell_analysis(source)
                    definite = [[operation, *arguments] for operation, arguments in analysis.calls]
                    if not all(call in actual for call in definite):
                        self.fail(f"[violation:unreachable-call-certified] {name}: an unreached call became definite: {source!r}")
                    if required and ["start", "goal"] not in definite:
                        self.fail(f"[violation:unreachable-call-certified] {name}: a function-local end stopped its caller: {source!r}")
                    if reached and not definite and not analysis.unknown:
                        self.fail(f"[violation:unreachable-call-certified] {name}: an unresolved path was reported as absence: {source!r}")


class FunctionScopeEvidence(fixtures.Case):
    def test_function_exit_is_not_a_spec_execution_violation(self):
        for source in ("f() { exit 0; }; f; ha start goal", "g() { exit 0; }; f() { g; }; f; ha start goal"):
            for host in ("codex", "claude"):
                with self.subTest(host=host, source=source):
                    value = self.codex_commands("spec", (source,)) if host == "codex" else self.bound_claude("spec", (source,))
                    try:
                        self.checker.trace(host, value, "spec")
                    except check.Failure as error:
                        self.fail(f"[violation:unreachable-call-certified] {host}: an unreached Core call became a Spec violation: {error}")
                    except check.Unavailable:
                        pass


FIXTURE = ('import unittest\nfrom pathlib import Path\nclass Case(unittest.TestCase):\n'
           '    def test_actual_selection(self): self.assertEqual(Path("source-marker").read_text(), "product")\n')
# Only the public GitHub responses are synthetic. They are built from real Git
# objects; the checkout, selected copies, child worker and regressions are real.
PUBLIC_ENTRY = """import json, sys
sys.path.insert(0, 'tools/release-checks')
import check
from unittest import mock
responses = json.load(open(sys.argv[1]))
def api(path, **options):
    if path not in responses:
        raise check.Unavailable('GitHub GET failed; release state remains unverified')
    return responses[path]
with mock.patch.object(check, 'api', side_effect=api), mock.patch.object(check.Checker, 'release', return_value={}):
    raise SystemExit(check.main(['AC-10', '--evidence', sys.argv[2]]))
"""


class ShallowProductSource(unittest.TestCase):
    """A checkout without the released commit still verifies the authenticated public product."""
    @staticmethod
    def files():
        # Every source the acceptance checker records as loaded, plus its worker.
        names = {Path(path).relative_to(ROOT).as_posix() for path in check.LOADED_SOURCE}
        return sorted(names | {"tools/release-checks/snapshot_worker.py"})

    def git(self, root, *args):
        return subprocess.check_output(["git", *args], cwd=root, stderr=subprocess.DEVNULL,
                                       env=dict(os.environ, GIT_CONFIG_GLOBAL=os.devnull, GIT_CONFIG_NOSYSTEM="1"))

    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="jaekit-shallow-product-")
        self.addCleanup(temporary.cleanup)
        self.base = Path(temporary.name)
        public = self.public = self.base / "public"
        for directory in ("tools/release", "tools/release-checks"):
            (public / directory).mkdir(parents=True)
            (public / directory / "test_fixture.py").write_text(FIXTURE)
        for name in self.files():
            (public / name).parent.mkdir(parents=True, exist_ok=True)
            shutil.copyfile(ROOT / name, public / name)
        (public / "source-marker").write_text("product")
        for args in (("init", "-q"), ("config", "user.name", "Synthetic"), ("config", "user.email", "synthetic@example.invalid"),
                     ("config", "commit.gpgsign", "false"), ("add", "."), ("commit", "-qm", "synthetic public product")):
            self.git(public, *args)
        self.product = self.git(public, "rev-parse", "HEAD").decode().strip()
        self.product_tree = self.git(public, "rev-parse", "HEAD^{tree}").decode().strip()
        (public / "source-marker").write_text("checker")
        self.git(public, "add", ".")
        self.git(public, "commit", "-qm", "synthetic newer checker")
        self.shallow = self.base / "shallow"
        self.git(self.base, "clone", "-q", "--depth", "1", public.resolve().as_uri(), str(self.shallow))
        with self.assertRaises(subprocess.CalledProcessError, msg="fixture must lack the released commit"):
            self.git(self.shallow, "cat-file", "-e", self.product + "^{commit}")
        self.evidence = self.base / "evidence"
        self.evidence.mkdir()
        (self.evidence / "evidence.json").write_text(json.dumps({
            "schema": "jaekit-release-evidence/v1", "release": {"tag": "v0.1.3", "source_commit": self.product},
            "checker_source": {"commit": self.git(self.shallow, "rev-parse", "HEAD").decode().strip(),
                               "tree": self.git(self.shallow, "rev-parse", "HEAD^{tree}").decode().strip()}}))

    def responses(self):
        listing = []
        for row in self.git(self.public, "ls-tree", "-r", "-t", "-z", self.product).split(b"\0"):
            if row:
                metadata, name = row.split(b"\t", 1)
                mode, kind, oid = metadata.decode().split()
                listing.append({"path": os.fsdecode(name), "mode": mode, "type": kind, "sha": oid})
        result = {f"repos/{check.REPO}/commits/{check.TAG}": {"sha": self.product, "commit": {"tree": {"sha": self.product_tree}}},
                  f"repos/{check.REPO}/git/commits/{self.product}": {"sha": self.product, "tree": {"sha": self.product_tree}},
                  f"repos/{check.REPO}/git/trees/{self.product_tree}?recursive=1":
                      {"sha": self.product_tree, "truncated": False, "tree": listing}}
        for item in listing:
            if item["type"] == "blob":
                data = self.git(self.public, "cat-file", "blob", item["sha"])
                result[f"repos/{check.REPO}/git/blobs/{item['sha']}"] = {
                    "sha": item["sha"], "encoding": "base64", "content": base64.encodebytes(data).decode()}
        return result

    def marker_blob(self):
        return self.git(self.public, "rev-parse", self.product + ":source-marker").decode().strip()

    def state(self):
        git_dir = self.shallow / ".git"
        return ({path.relative_to(git_dir).as_posix(): path.read_bytes() for path in git_dir.rglob("*")
                 if path.is_file() and path.name != "index"},
                self.git(self.shallow, "status", "--porcelain=v1", "-z"),
                {path.relative_to(self.shallow).as_posix(): path.read_bytes() for path in self.shallow.rglob("*")
                 if path.is_file() and ".git" not in path.relative_to(self.shallow).parts})

    def run_entry(self, responses):
        path = self.base / "responses.json"
        path.write_text(json.dumps(responses))
        return subprocess.run([sys.executable, "-c", PUBLIC_ENTRY, str(path), str(self.evidence)], cwd=self.shallow,
                              capture_output=True, text=True, timeout=120,
                              env=dict(os.environ, PYTHONDONTWRITEBYTECODE="1"))

    def test_authenticated_public_objects_replace_missing_local_history(self):
        before = self.state()
        result = self.run_entry(self.responses())
        if result.returncode != 0 or "PASS AC-10" not in result.stdout:
            self.fail("[violation:product-source-boundary] a shallow checkout could not verify the authenticated "
                      f"public product: exit {result.returncode}: {result.stdout}{result.stderr}")
        self.assertEqual(self.state(), before, "the original checkout changed")

    def test_wrong_public_bytes_trees_and_paths_are_not_accepted(self):
        def wrong_bytes(responses):
            responses[f"repos/{check.REPO}/git/blobs/{self.marker_blob()}"]["content"] = base64.b64encode(b"altered").decode()

        def other_tree(responses):
            responses[f"repos/{check.REPO}/git/commits/{self.product}"]["tree"]["sha"] = "1" * 40

        def truncated(responses):
            responses[f"repos/{check.REPO}/git/trees/{self.product_tree}?recursive=1"]["truncated"] = True

        def missing(responses):
            del responses[f"repos/{check.REPO}/git/blobs/{self.marker_blob()}"]

        def unsafe(responses):
            listing = responses[f"repos/{check.REPO}/git/trees/{self.product_tree}?recursive=1"]["tree"]
            listing.append({"path": ".git/hooks/post-checkout", "mode": "100755", "type": "blob", "sha": self.marker_blob()})

        for label, change, expected in (("other bytes", wrong_bytes, 1), ("other tree", other_tree, 1),
                                        ("truncated tree", truncated, 2), ("missing object", missing, 2),
                                        ("unsafe path", unsafe, 2)):
            with self.subTest(label=label):
                before = self.state()
                responses = self.responses()
                change(responses)
                result = self.run_entry(responses)
                if result.returncode != expected or "PASS" in result.stdout:
                    self.fail(f"[violation:product-source-boundary] {label}: exit {result.returncode}, expected {expected}: "
                              f"{result.stdout}{result.stderr}")
                self.assertEqual(self.state(), before, "the original checkout changed")


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


LOADED_PROBE = r"""
import json, sys
from pathlib import Path
root = Path(sys.argv[1]).resolve()
opened = set()
def audit(event, args):
    if event == "open" and isinstance(args[0], str) and args[0].endswith(".py"):
        path = Path(args[0]).resolve()
        if path.is_relative_to(root / "tools"):
            opened.add(str(path))
sys.addaudithook(audit)
sys.path.insert(0, str(root / "tools/release-checks"))
import check
print(json.dumps({"opened": sorted(opened), "loaded": sorted(check.LOADED_SOURCE),
                  "bytes": sorted(path for path, data in check.LOADED_SOURCE.items() if Path(path).read_bytes() == data)}))
"""


class LoadedSourceCoverage(unittest.TestCase):
    """Every checkout source the acceptance checker executes is bound to the verified tree."""

    def test_every_executed_checker_source_is_recorded_as_loaded(self):
        # An empty cache prefix makes every import read its source file.
        with tempfile.TemporaryDirectory(prefix="jaekit-loaded-source-") as cache:
            result = subprocess.run([sys.executable, "-B", "-c", LOADED_PROBE, str(ROOT)], cwd=ROOT,
                                    capture_output=True, text=True, timeout=120,
                                    env=dict(os.environ, PYTHONDONTWRITEBYTECODE="1", PYTHONPYCACHEPREFIX=cache))
        self.assertEqual(result.returncode, 0, result.stderr)
        observed = json.loads(result.stdout)
        self.assertIn(str((ROOT / "tools/release-checks/check.py").resolve()), observed["opened"])
        missing = sorted(set(observed["opened"]) - set(observed["loaded"]))
        if missing:
            self.fail("[violation:integration-source-unbound] executed checker sources are not bound to the verified tree: "
                      + ", ".join(Path(path).relative_to(ROOT).as_posix() for path in missing))
        self.assertEqual(observed["bytes"], observed["loaded"], "recorded bytes are the current tracked sources")


if __name__ == "__main__":
    unittest.main()
