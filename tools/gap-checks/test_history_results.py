"""Synthetic Git histories and real unittest producer results for review gaps.

Only temporary repositories and test files are changed. No private observations,
recorder commands, host sessions or network access are required.
"""
import importlib.util
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
PRESERVED = ROOT / "tools/goal-checks/release-0-1-3-followups/preserved.py"
CURRENT = ROOT / "tools/goal-checks/release-0-1-3-review-fixes/test_private_independence.py"
ENV = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GIT_AUTHOR_NAME="Fixture",
           GIT_AUTHOR_EMAIL="fixture@example.invalid", GIT_COMMITTER_NAME="Fixture",
           GIT_COMMITTER_EMAIL="fixture@example.invalid", GIT_CONFIG_NOSYSTEM="1",
           GIT_CONFIG_GLOBAL=os.devnull, GIT_ALLOW_PROTOCOL="file")
for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "HA_EVIDENCE_PATH",
            "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
    ENV.pop(key, None)


def forbidden():
    return "/" + "home" + "/" + "fixture-account" + "/" + "synthetic.txt"


def git(root, *args):
    return subprocess.run(["git", *args], cwd=root, env=ENV, text=True,
                          capture_output=True, check=True, timeout=30).stdout.strip()


def repository(parent, name="repository"):
    root = parent / name
    root.mkdir()
    git(root, "init", "--quiet", "-b", "main")
    git(root, "config", "gc.auto", "0")
    git(root, "config", "commit.gpgsign", "false")
    git(root, "config", "core.hooksPath", os.devnull)
    (root / "public.txt").write_text("synthetic public text\n")
    return root, commit(root)


def commit(root, message="synthetic change"):
    git(root, "add", ".")
    git(root, "commit", "--quiet", "--allow-empty", "-m", message)
    return git(root, "rev-parse", "HEAD")


class HistoryCase(unittest.TestCase):
    def audit(self, root, base, head, expected, env=None):
        for entry in (PRESERVED, CURRENT):
            with self.subTest(entry=entry.name):
                result = subprocess.run(
                    [sys.executable, str(entry), "--history", "--base", base, "--head", head],
                    cwd=root, env=env or ENV, text=True, capture_output=True, timeout=30)
                output = result.stdout + result.stderr
                self.assertEqual(result.returncode, expected,
                                 "[requirement] history verdict differs: " + output)
                self.assertIn({0: "PASS:", 1: "FAIL:", 2: "UNVERIFIED:"}[expected], output,
                              "[requirement] history result must state what was established")
                self.assertNotIn("Traceback", output,
                                 "[requirement] audit input failures need a defined result")


class AC4(HistoryCase):
    def test_intermediate_additions_survive_deletion_and_revert(self):
        for cleanup in ("delete", "revert"):
            with self.subTest(cleanup=cleanup), tempfile.TemporaryDirectory() as name:
                root, base = repository(Path(name))
                (root / "temporary.txt").write_text(forbidden() + "\n")
                added = commit(root)
                if cleanup == "revert":
                    git(root, "revert", "--no-edit", added)
                else:
                    (root / "temporary.txt").unlink()
                    commit(root)
                self.assertEqual(git(root, "diff", base, "HEAD"), "", "fixture endpoints must match")
                self.audit(root, base, git(root, "rev-parse", "HEAD"), 1)

    def test_merged_side_history_is_not_reduced_to_first_parent(self):
        with tempfile.TemporaryDirectory() as name:
            root, base = repository(Path(name))
            git(root, "checkout", "--quiet", "-b", "side")
            (root / "temporary.txt").write_text(forbidden() + "\n")
            commit(root)
            (root / "temporary.txt").unlink()
            commit(root)
            git(root, "checkout", "--quiet", "main")
            (root / "main.txt").write_text("safe main change\n")
            commit(root)
            git(root, "merge", "--quiet", "--no-ff", "side", "-m", "synthetic merge")
            self.audit(root, base, git(root, "rev-parse", "HEAD"), 1)

    def test_merge_resolution_content_is_audited_even_when_removed_later(self):
        with tempfile.TemporaryDirectory() as name:
            root, base = repository(Path(name))
            git(root, "checkout", "--quiet", "-b", "side")
            (root / "side.txt").write_text("safe side change\n")
            commit(root)
            git(root, "checkout", "--quiet", "main")
            (root / "main.txt").write_text("safe main change\n")
            commit(root)
            git(root, "merge", "--quiet", "--no-ff", "--no-commit", "side")
            (root / "resolution.txt").write_text(forbidden() + "\n")
            merged = commit(root, "synthetic merge resolution")
            self.assertEqual(len(git(root, "show", "-s", "--format=%P", merged).split()), 2)
            (root / "resolution.txt").unlink()
            head = commit(root)
            self.audit(root, base, head, 1)

    def test_preexisting_content_empty_ranges_and_safe_changes_are_not_new_violations(self):
        with tempfile.TemporaryDirectory() as name:
            root, ancestor = repository(Path(name))
            (root / "existing.txt").write_text(forbidden() + "\n")
            base = commit(root)
            git(root, "checkout", "--quiet", "-b", "side", ancestor)
            (root / "side.txt").write_text("safe side change\n")
            commit(root)
            git(root, "checkout", "--quiet", "main")
            git(root, "merge", "--quiet", "--no-ff", "side", "-m", "safe merge")
            head = git(root, "rev-parse", "HEAD")
            self.audit(root, base, head, 0)
            self.audit(root, head, head, 0)

    def test_commit_messages_and_complete_nonancestor_ranges_keep_their_meaning(self):
        with tempfile.TemporaryDirectory() as name:
            root, ancestor = repository(Path(name))
            base = commit(root, "safe base")
            git(root, "checkout", "--quiet", "-b", "other", ancestor)
            good = commit(root, "safe unrelated branch")
            self.audit(root, base, good, 0)
            bad = commit(root, forbidden())
            self.audit(root, base, bad, 1)


class AC5(HistoryCase):
    def no_fetch_environment(self, parent):
        bindir = parent / "bin"
        bindir.mkdir()
        calls = parent / "git-calls.jsonl"
        real_git = shutil.which("git")
        self.assertIsNotNone(real_git, "Git is needed to construct audit fixtures")
        wrapper = bindir / "git"
        wrapper.write_text(
            "#!" + sys.executable + "\nimport json, os, sys\nfrom pathlib import Path\n"
            f"with Path({str(calls)!r}).open('a') as out:\n"
            "    out.write(json.dumps({'argv': sys.argv[1:], 'no_lazy': os.environ.get('GIT_NO_LAZY_FETCH')}) + '\\n')\n"
            "if any(a in ('fetch', 'pull', 'clone') for a in sys.argv[1:]):\n    sys.exit(87)\n"
            f"os.execv({real_git!r}, [{real_git!r}] + sys.argv[1:])\n")
        wrapper.chmod(0o755)
        return dict(ENV, PATH=str(bindir) + os.pathsep + ENV.get("PATH", "")), calls

    def assert_no_fetch(self, calls):
        seen = [json.loads(line) for line in calls.read_text().splitlines()]
        self.assertTrue(seen, "audit must actually have read Git inputs")
        self.assertTrue(all(item["no_lazy"] == "1" for item in seen),
                        "[requirement] every audit Git read must disable lazy fetching")
        self.assertFalse(any(arg in ("fetch", "pull", "clone") for item in seen for arg in item["argv"]),
                         "[requirement] an audit must not fetch its missing inputs")

    def test_missing_intermediate_commit_tree_or_blob_is_unverified_without_fetch(self):
        for kind in ("commit", "tree", "blob"):
            with self.subTest(missing=kind), tempfile.TemporaryDirectory() as name:
                parent = Path(name)
                root, base = repository(parent)
                (root / "transient.txt").write_text("unique intermediate public bytes\n")
                middle = commit(root)
                object_id = {"commit": middle, "tree": git(root, "rev-parse", middle + "^{tree}"),
                             "blob": git(root, "rev-parse", middle + ":transient.txt")}[kind]
                (root / "transient.txt").unlink()
                head = commit(root)
                self.assertEqual(git(root, "diff", base, head), "", "fixture endpoints must match")
                obj = root / ".git/objects" / object_id[:2] / object_id[2:]
                self.assertTrue(obj.is_file(), "synthetic object must be loose before removal")
                obj.unlink()
                env, calls = self.no_fetch_environment(parent)
                self.audit(root, base, head, 2, env)
                self.assert_no_fetch(calls)

    def test_missing_endpoints_and_shallow_side_ancestry_remain_unverified(self):
        with tempfile.TemporaryDirectory() as name:
            parent = Path(name)
            root, _ = repository(parent)
            git(root, "checkout", "--quiet", "-b", "side")
            commit(root)
            commit(root)
            git(root, "checkout", "--quiet", "main")
            base = commit(root)
            git(root, "merge", "--quiet", "--no-ff", "side", "-m", "safe merge")
            head = git(root, "rev-parse", "HEAD")
            shallow = parent / "shallow"
            git(parent, "clone", "--quiet", "--depth", "2", root.as_uri(), str(shallow))
            env, calls = self.no_fetch_environment(parent)
            self.audit(shallow, base, head, 2, env)
            self.audit(root, "e" * 40, head, 2, env)
            self.audit(root, base, "f" * 40, 2, env)
            self.audit(shallow, head, head, 0, env)
            self.assert_no_fetch(calls)

    def test_current_file_check_and_public_release_check_keep_separate_inputs(self):
        with tempfile.TemporaryDirectory() as name:
            parent = Path(name)
            root, _ = repository(parent)
            commit(root)
            shallow = parent / "shallow"
            git(parent, "clone", "--quiet", "--depth", "1", root.as_uri(), str(shallow))
            for checkout in (root, shallow):
                result = subprocess.run([sys.executable, str(CURRENT), "TrackedFiles"], cwd=checkout,
                                        env=ENV, capture_output=True, text=True, timeout=30)
                self.assertEqual(result.returncode, 0,
                                 "[requirement] current-file checks need no private or prior history: " + result.stderr)
        spec = importlib.util.spec_from_file_location("gap_preservation", PRESERVED)
        module = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(module)
        with patch.object(module, "public_boundary"), patch.object(module, "public_release") as release:
            self.assertEqual(module.main(["preserved.py"]), 0)
            release.assert_called_once()


TEST_BODIES = {
    "expected_assertion": "@unittest.expectedFailure\ndef {name}(self): self.fail('synthetic expected assertion')",
    "expected_exception": "@unittest.expectedFailure\ndef {name}(self): raise RuntimeError('synthetic expected exception')",
    "unexpected_success": "@unittest.expectedFailure\ndef {name}(self): self.assertTrue(True)",
    "pass": "def {name}(self): self.assertTrue(True)",
    "failure": "def {name}(self): self.fail('synthetic assertion')",
    "skip": "def {name}(self): self.skipTest('synthetic skip')",
    "error": "def {name}(self): raise RuntimeError('synthetic execution error')",
}


class AC6(unittest.TestCase):
    def producer_result(self, producer, cases, unit="class"):
        with tempfile.TemporaryDirectory(prefix="jaekit-unittest-results-") as name:
            root = Path(name)
            body = "import unittest\nclass Case(unittest.TestCase):\n"
            for number, kind in enumerate(cases):
                method = TEST_BODIES[kind].format(name=f"test_{number:02}")
                body += "\n".join("    " + line for line in method.splitlines()) + "\n"
            (root / "fixture.py").write_text(body)
            report = root / "result.json"
            env = dict(ENV, HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="a" * 64,
                       HA_EVIDENCE_DECLARATION_DIGEST="b" * 64)
            if producer == "goal":
                declared = {"schema": "check-declaration/v1", "producer": "jaekit-goal-tests",
                            "producer_version": "1", "targets": [{"id": "synthetic", "violation": "not-met"}]}
                check = {"runner": "unittest", "target": "synthetic", "file": "fixture.py"}
                if unit != "module":
                    check["test"] = "Case.test_00" if unit == "method" else "Case"
                (root / "declaration.json").write_text(json.dumps(declared))
                (root / "targets.json").write_text(json.dumps({"schema": "jaekit-goal-targets/v1", "checks": [check]}))
                argv = [sys.executable, str(ROOT / "tools/goal-checks/produce.py"), "declaration.json", "targets.json"]
            else:
                directory = root / "tools/review-checks"
                directory.mkdir(parents=True)
                shutil.copyfile(ROOT / "tools/review-checks/produce.py", directory / "produce.py")
                (directory / "suites.json").write_text(json.dumps({"fixture": {"file": "fixture.py", "class": "Case"}}))
                argv = [sys.executable, str(directory / "produce.py"), "fixture"]
            result = subprocess.run(argv, cwd=root, env=env, text=True, capture_output=True, timeout=30)
            self.assertTrue(report.is_file(), "producer must run and write its report: " + result.stderr)
            observations = json.loads(report.read_text())["observations"]
            self.assertEqual(len(observations), 1, "fixture selects exactly one producer target")
            return observations[0], result.returncode, result.stderr

    def expect(self, producer, cases, status, reason=None, unit="class"):
        observation, code, stderr = self.producer_result(producer, cases, unit)
        self.assertEqual(observation["status"], status,
                         "[requirement] unittest outcome must not be promoted: " + str(observation) + stderr)
        self.assertEqual(observation.get("reason"), reason,
                         "[requirement] preserved result reason differs: " + str(observation))
        self.assertEqual(code, 0 if status == "pass" else 1,
                         "[requirement] report and producer exit must agree")
        if status != "violation":
            self.assertNotIn("violation", observation,
                             "[requirement] a decorator result supplies no declared assertion violation")

    def test_decorator_outcomes_are_errors_in_existing_selection_units(self):
        for producer, units in (("goal", ("method", "class", "module")), ("review", ("class",))):
            for kind in ("expected_assertion", "expected_exception", "unexpected_success"):
                for unit in units:
                    with self.subTest(producer=producer, outcome=kind, unit=unit):
                        self.expect(producer, [kind], "error", "setup_error", unit)

    def test_mixed_results_cannot_hide_decorator_outcomes_or_execution_errors(self):
        for producer, units in (("goal", ("class", "module")), ("review", ("class",))):
            for kind in ("expected_assertion", "expected_exception", "unexpected_success"):
                for other in ("pass", "failure", "skip", "error"):
                    for unit in units:
                        with self.subTest(producer=producer, outcome=kind, other=other, unit=unit):
                            self.expect(producer, [kind, other], "error",
                                        "execution_error" if other == "error" else "setup_error", unit)

    def test_undecorated_results_keep_their_meaning(self):
        outcomes = (("pass", "pass", None), ("failure", "violation", None),
                    ("skip", "skip", "skipped"), ("error", "error", "execution_error"))
        for producer in ("goal", "review"):
            for kind, status, reason in outcomes:
                with self.subTest(producer=producer, outcome=kind):
                    self.expect(producer, [kind], status, reason)


if __name__ == "__main__":
    unittest.main()
