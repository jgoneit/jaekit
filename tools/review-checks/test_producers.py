"""Behavioral boundaries for current-tree checks, explicit audits and Go results.

These checks use synthetic repositories and real local Go processes. They do
not read earlier goals, run a host, publish anything or invoke the recorder.
"""
from contextlib import contextmanager, redirect_stderr
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
CHECKS = Path("tools/goal-checks")
CURRENT = CHECKS / "release-0-1-3-review-fixes/test_private_independence.py"
PRESERVED = CHECKS / "release-0-1-3-followups/preserved.py"
ENV = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GOTOOLCHAIN="local",
           GOFLAGS="-mod=readonly", GIT_AUTHOR_NAME="Fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
           GIT_COMMITTER_NAME="Fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid")
for key in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
    ENV.pop(key, None)


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def git(root, *args):
    return subprocess.run(["git", *args], cwd=root, env=ENV, check=True, text=True,
                          capture_output=True, timeout=60).stdout.strip()


def commit(root, message="synthetic change"):
    git(root, "add", ".")
    git(root, "commit", "--quiet", "--allow-empty", "-m", message)
    return git(root, "rev-parse", "HEAD")


@contextmanager
def in_directory(path):
    previous = Path.cwd()
    os.chdir(path)
    try:
        yield
    finally:
        os.chdir(previous)


def forbidden_path():
    # A synthetic forbidden location; no actual person's path is a fixture.
    return "/" + "home" + "/" + "fixture-account" + "/" + "trace.json"


class AC9(unittest.TestCase):
    def test_current_tree_accepts_valid_files_and_rejects_bad_files_without_ancestors(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-current-tree-") as directory:
            full = Path(directory) / "full"
            full.mkdir()
            git(full, "init", "--quiet")
            for name in git(ROOT, "ls-files", "-z").split("\0"):
                source = ROOT / name
                if not name or not source.is_file() or source.is_symlink():
                    continue
                destination = full / name
                destination.parent.mkdir(parents=True, exist_ok=True)
                shutil.copy2(source, destination)
            commit(full, "synthetic current public tree")
            commit(full, "synthetic ancestor boundary")
            shallow = Path(directory) / "shallow"
            git(Path(directory), "clone", "--quiet", "--depth", "1", full.as_uri(), str(shallow))
            self.assertEqual(git(shallow, "rev-list", "--count", "HEAD"), "1")
            for repository in (full, shallow):
                with self.subTest(repository=repository.name):
                    self.assertFalse((repository / "docs").exists())
                    self.assertFalse((repository / ".git/ha").exists())
                    command = [sys.executable, str(CURRENT), "TrackedFiles", "-v"]
                    result = subprocess.run(command, cwd=repository, env=ENV, text=True,
                                            capture_output=True, timeout=60)
                    self.assertEqual(result.returncode, 0,
                                     "[requirement] valid current files require no historical digest: " + result.stderr)
                    bad = repository / "guides/synthetic-review.md"
                    bad.write_text("docs/specs/" + "release-" + "0-1-3" + "/runs.jsonl\n")
                    git(repository, "add", "guides/synthetic-review.md")
                    rejected = subprocess.run(command, cwd=repository, env=ENV, text=True,
                                              capture_output=True, timeout=60)
                    self.assertEqual(rejected.returncode, 1,
                                     "[requirement] forbidden current content must be rejected")
                    self.assertIn("FAIL:", rejected.stderr,
                                  "[requirement] rejection must observe the forbidden content, not a setup error")
                    self.assertNotIn("ERROR:", rejected.stderr)

    def test_missing_observations_stay_unverified_without_implicit_search(self):
        checker = load("tools/release-checks/check.py", "review_release_checker")
        with patch.object(checker, "checked", side_effect=AssertionError("unexpected Git lookup")):
            with self.assertRaises(checker.Unavailable):
                checker.Checker().evidence()
        with tempfile.TemporaryDirectory() as directory:
            index = {"schema": "jaekit-release-evidence/v1", "release": {"tag": checker.TAG}}
            (Path(directory) / "evidence.json").write_text(json.dumps(index))
            self.assertEqual(checker.Checker(directory).evidence(), index)


class AC10(unittest.TestCase):
    def test_both_audits_require_an_explicit_range_and_do_not_fetch_missing_history(self):
        with tempfile.TemporaryDirectory(prefix="jaekit-history-audit-") as directory:
            repository = Path(directory) / "repo"
            repository.mkdir()
            git(repository, "init", "--quiet")
            (repository / "README.md").write_text("synthetic public text\n")
            base = commit(repository)
            (repository / "README.md").write_text("synthetic public text\nA safe addition.\n")
            good = commit(repository)
            (repository / "README.md").write_text(forbidden_path() + "\n")
            bad = commit(repository)
            bindir = Path(directory) / "bin"
            bindir.mkdir()
            network_attempt = Path(directory) / "network-attempt"
            real_git = shutil.which("git")
            wrapper = ("#!" + sys.executable + "\nimport os, sys\nfrom pathlib import Path\n"
                       "if sys.argv[1:2] in ([\"fetch\"], [\"pull\"], [\"clone\"]):\n"
                       f"    Path({str(network_attempt)!r}).write_text('attempted')\n    sys.exit(89)\n"
                       f"os.execv({real_git!r}, [{real_git!r}] + sys.argv[1:])\n")
            (bindir / "git").write_text(wrapper)
            (bindir / "git").chmod(0o755)
            env = dict(ENV, PATH=str(bindir) + os.pathsep + ENV.get("PATH", ""))
            for entry in (PRESERVED, CURRENT):
                for label, head, expected in (("safe range", good, 0), ("forbidden range", bad, 1),
                                              ("missing head", "f" * 40, 2)):
                    with self.subTest(entry=str(entry), case=label):
                        result = subprocess.run([sys.executable, str(ROOT / entry), "--history",
                                                 "--base", base, "--head", head], cwd=repository,
                                                env=env, text=True, capture_output=True, timeout=60)
                        self.assertEqual(result.returncode, expected,
                                         "[requirement] explicit history verdict: " + result.stdout + result.stderr)
                        if expected == 2:
                            self.assertIn("UNVERIFIED", result.stdout + result.stderr,
                                          "[requirement] missing history is explicitly unverified")
                with self.subTest(entry=str(entry), case="missing base"):
                    result = subprocess.run([sys.executable, str(ROOT / entry), "--history",
                                             "--base", "e" * 40, "--head", good], cwd=repository,
                                            env=env, text=True, capture_output=True, timeout=60)
                    self.assertEqual(result.returncode, 2)
                    self.assertIn("UNVERIFIED", result.stdout + result.stderr,
                                  "[requirement] missing base must not become successful audit")
            self.assertFalse(network_attempt.exists(), "[requirement] an audit never fetches history")

    def test_default_preservation_checks_public_release_even_without_a_history_range(self):
        module = load(PRESERVED, "review_preserved")
        for changed in (False, True):
            calls = []

            def public_release(expected, problems):
                calls.append(expected)
                if changed:
                    problems.append("synthetic release identity changed")

            with self.subTest(release_changed=changed), patch.object(module, "public_boundary"), \
                    patch.object(module, "public_release", side_effect=public_release):
                result = module.main(["preserved.py"])
                self.assertEqual(result, 1 if changed else 0,
                                 "[requirement] public release verification is independent of a history range")
                self.assertEqual(len(calls), 1, "[requirement] release identity was actually checked")


class AC11(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.producer = load(CHECKS / "produce.py", "review_producer")

    def go(self, body, *, imports="", extra="", docs=False, expect="pass", append=""):
        with tempfile.TemporaryDirectory(prefix="jaekit-go-result-") as directory:
            repository = Path(directory)
            (repository / "go.mod").write_text("module example.invalid/reviewfixture\n\ngo 1.22\n")
            package = repository / "plugins" if docs else repository
            package.mkdir(exist_ok=True)
            (package / "fixture_test.go").write_text(
                'package fixture\nimport ("testing"\n' + imports + '\n)\n'
                + 'func TestBehavior(t *testing.T) {\n' + body + '\n}\n' + extra)
            (repository / "guide.md").write_text("valid\n")
            stderr = io.StringIO()
            with in_directory(repository), patch.dict(os.environ, ENV), redirect_stderr(stderr):
                if docs:
                    result = self.producer.run_go_docs({"package": "./plugins", "expect": expect,
                                                       "path": "guide.md", "append": append,
                                                       "mentions": "guide.md"})
                else:
                    result = self.producer.run_go({"package": ".", "test": "TestBehavior"})
            return result, stderr.getvalue()

    def test_real_test_child_and_package_panics_and_exit_mismatches_are_execution_errors(self):
        cases = [
            ("test panic", 'panic("synthetic runtime failure")', "", ""),
            ("child panic", 't.Run("child", func(t *testing.T) { panic("child failed") })', "", ""),
            ("assertion then panic", 't.Error("declared assertion"); panic("cleanup failed")', "", ""),
            ("package panic", "", "", 'func TestMain(m *testing.M) { m.Run(); panic("package cleanup failed") }'),
            ("exit mismatch", "", '"os"', 'func TestMain(m *testing.M) { m.Run(); os.Exit(3) }'),
            ("signal", 'syscall.Kill(os.Getpid(), syscall.SIGTERM)', '"os"\n"syscall"', ""),
        ]
        for label, body, imports, extra in cases:
            with self.subTest(case=label):
                result, output = self.go(body, imports=imports, extra=extra)
                self.assertEqual(result, {"status": "error", "reason": "execution_error"},
                                 "[requirement] runtime failure cannot prove the declared violation: " + output)
                self.assertTrue(output.strip(), "[requirement] execution diagnostics remain visible")

    def test_normal_go_results_retain_their_meaning(self):
        for body, expected in (("", {"status": "pass"}),
                               ('t.Fatal("declared assertion")', {"status": "violation"}),
                               ('t.Skip("synthetic unavailable target")', {"status": "skip", "reason": "skipped"}),
                               ("missingSymbol()", {"status": "error", "reason": "compile_error"}),
                               ('t.Log("panic: this is only a printed example")', {"status": "pass"})):
            with self.subTest(body=body):
                self.assertEqual(self.go(body)[0], expected)

    def test_document_probes_do_not_turn_runtime_errors_into_rejection_evidence(self):
        for body, imports, extra, expect, append in (
                ('panic("guide.md fixture failed")', "", "", "pass", ""),
                ('data, _ := os.ReadFile("../guide.md"); if strings.Contains(string(data), "crash") { panic("guide.md crash") }',
                 '"os"\n"strings"', "", "reject", "crash\n"),
                ("", "", 'func TestMain(m *testing.M) { m.Run(); panic("package cleanup failed") }', "pass", ""),
                ("", '"os"', 'func TestMain(m *testing.M) { m.Run(); os.Exit(3) }', "reject", "bad\n")):
            with self.subTest(expect=expect, extra=extra, body=body):
                result, output = self.go(body, imports=imports, extra=extra, docs=True, expect=expect, append=append)
                self.assertEqual(result, {"status": "error", "reason": "execution_error"},
                                 "[requirement] broken document test is not successful rejection: " + output)

    def test_normal_document_acceptance_and_rejection_are_preserved(self):
        body = ('data, _ := os.ReadFile("../guide.md"); '
                'if strings.Contains(string(data), "bad") { t.Fatal("guide.md: rejected statement") }')
        imports = '"os"\n"strings"'
        self.assertEqual(self.go(body, imports=imports, docs=True)[0], {"status": "pass"})
        self.assertEqual(self.go(body, imports=imports, docs=True, expect="reject", append="bad\n")[0],
                         {"status": "pass"})
        self.assertEqual(self.go(body, imports=imports, docs=True, expect="reject", append="safe\n")[0],
                         {"status": "violation"})


if __name__ == "__main__":
    unittest.main()
