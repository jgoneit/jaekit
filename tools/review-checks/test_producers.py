"""Behavioral boundaries for Go results.

These checks use synthetic repositories and real local Go processes. They do
not read earlier goals, run a host, publish anything or invoke the recorder.
"""
from contextlib import contextmanager, redirect_stderr
import importlib.util
import io
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch


ROOT = Path(__file__).resolve().parents[2]
CHECKS = Path("tools/goal-checks")
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


@contextmanager
def in_directory(path):
    previous = Path.cwd()
    os.chdir(path)
    try:
        yield
    finally:
        os.chdir(previous)


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
