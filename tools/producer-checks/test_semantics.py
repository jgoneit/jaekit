"""Real subprocess regressions for repository result producers; synthetic inputs."""
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
ENV = dict(os.environ, PYTHONDONTWRITEBYTECODE="1", GOTOOLCHAIN="local", GOFLAGS="-mod=readonly")
for key in ("HA_EVIDENCE_PATH", "HA_EVIDENCE_INVOCATION", "HA_EVIDENCE_DECLARATION_DIGEST"):
    ENV.pop(key, None)


def names(target="suite", slots=8):
    return {"additional_violations": [target + ".assertion-" + str(i) for i in range(2, slots + 1)],
            "execution": target + ".execution", "selection": target + ".selection",
            "attribution": target + ".attribution"}


def declaration(target="suite", expected="wanted", slots=8):
    facets = names(target, slots)
    ids = [target, *facets["additional_violations"], facets["execution"], facets["selection"], facets["attribution"]]
    return {"schema": "check-declaration/v1", "producer": "jaekit-goal-tests", "producer_version": "2",
            "targets": [{"id": item, "violation": expected if index == 0 else "no-additional-violation"}
                        for index, item in enumerate(ids)]}


class Fixture(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="jaekit-producer-facts-")
        self.addCleanup(temporary.cleanup)
        self.root = Path(temporary.name)
        (self.root / "tools").mkdir()
        shutil.copy2(ROOT / "tools/result_observations.py", self.root / "tools/result_observations.py")

    def run_producer(self, body="", *, producer="goal", expected="wanted", check=None, slots=8, support=True):
        directory = self.root / "tools" / (producer + "-checks")
        directory.mkdir(exist_ok=True)
        shutil.copy2(ROOT / "tools" / (producer + "-checks") / "produce.py", directory / "produce.py")
        (self.root / "fixture.py").write_text("import unittest\nclass Case(unittest.TestCase):\n" + body)
        config = dict(check or {"runner": "unittest", "file": "fixture.py", "test": "Case"},
                      target="suite", result_targets=names(slots=slots))
        if producer == "goal":
            (self.root / "declaration.json").write_text(json.dumps(declaration(expected=expected, slots=slots)))
            (self.root / "targets.json").write_text(json.dumps({"schema": "jaekit-goal-targets/v2", "checks": [config]}))
            command = [sys.executable, str(directory / "produce.py"), "declaration.json", "targets.json"]
        else:
            (directory / "suites.json").write_text(json.dumps({"suite": {"file": "fixture.py", "class": "Case",
                                                                      "result_targets": names(slots=slots)}}))
            command = [sys.executable, str(directory / "produce.py"), "suite"]
        report = self.root / "report.json"
        report.unlink(missing_ok=True)
        env = dict(ENV)
        if support:
            env.update(HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="a" * 64,
                       HA_EVIDENCE_DECLARATION_DIGEST="b" * 64)
        proc = subprocess.run(command, cwd=self.root, env=env, capture_output=True, text=True, timeout=60)
        if not support:
            return proc, {}
        self.assertTrue(report.is_file(), proc.stdout + proc.stderr)
        parsed = json.loads(report.read_text())
        observations = parsed["observations"]
        self.assertTrue(all(item["attempt"] == 1 for item in observations), "distinct facts are not retries")
        self.assertEqual(len({item["target"] for item in observations}), len(observations))
        self.assertEqual(parsed["invocation"], "a" * 64)
        self.assertEqual(parsed["declaration_digest"], "b" * 64)
        return proc, {item["target"]: item for item in observations}


class CommandFacts(Fixture):
    def contract(self):
        return {"schema": "jaekit-command-exits/v1", "pass": [0],
                "violations": {"1": "observed-audit-violation"}, "unverified": [2]}

    def test_pass_violation_unverified_and_unknown_exit_are_distinct(self):
        for code, status in ((0, "pass"), (1, "violation"), (2, "skip"), (17, "error")):
            with self.subTest(code=code):
                proc, report = self.run_producer(check={"runner": "command", "argv": [sys.executable, "-c", f"raise SystemExit({code})"],
                                                       "exit_contract": self.contract()})
                self.assertEqual(report["suite"]["status"], status, proc.stderr)
                if code == 1:
                    self.assertEqual(report["suite"]["violation"], "observed-audit-violation")
                self.assertEqual(proc.returncode, 0 if code == 0 else 1)

    def test_unsupported_command_signal_and_launch_failure_are_not_violations(self):
        for argv in ([sys.executable, "-c", "raise SystemExit(2)"],
                     [sys.executable, "-c", "import os, signal; os.kill(os.getpid(), signal.SIGTERM)"],
                     [str(self.root / "absent-command")]):
            with self.subTest(argv=argv):
                proc, report = self.run_producer(check={"runner": "command", "argv": argv})
                self.assertNotEqual(proc.returncode, 0)
                self.assertFalse(any(item["status"] == "violation" for item in report.values()))
                self.assertEqual(report["suite.execution"]["status"], "error")

    def test_command_without_result_contract_is_only_maintain(self):
        check = {"runner": "command", "argv": [sys.executable, "-c", "print('synthetic command output')"]}
        maintained, _ = self.run_producer(check=check, support=False)
        structured, report = self.run_producer(check=check)
        self.assertEqual(maintained.returncode, 0)
        self.assertNotEqual(structured.returncode, 0)
        self.assertEqual(report["suite.execution"]["status"], "error")
        self.assertIn("synthetic command output", structured.stderr)


class MixedFacts(Fixture):
    def test_failure_skip_error_and_multiple_failures_survive_every_producer(self):
        body = ("    def test_a(self): self.fail('[violation:first] first failure')\n"
                "    def test_b(self): self.skipTest('not available')\n"
                "    def test_c(self): raise RuntimeError('runtime broken')\n"
                "    def test_d(self): self.fail('[violation:second] second failure')\n")
        for producer in ("goal", "review", "gap"):
            with self.subTest(producer=producer):
                proc, report = self.run_producer(body, producer=producer)
                self.assertEqual(proc.returncode, 1)
                self.assertEqual([item["violation"] for item in report.values() if item["status"] == "violation"],
                                 ["first", "second"])
                self.assertEqual(report["suite.selection"]["status"], "skip")
                self.assertEqual(report["suite.execution"]["reason"], "execution_error")
                self.assertEqual(report["suite.attribution"]["status"], "pass")

    def test_normal_pass_skip_decorators_and_zero_selection_keep_their_meaning(self):
        cases = [("    def test_a(self): pass\n", "pass", None),
                 ("    def test_a(self): self.skipTest('skip')\n", "skip", "suite.selection"),
                 ("    @unittest.expectedFailure\n    def test_a(self): self.fail('expected')\n", "error", "suite.execution"),
                 ("    @unittest.expectedFailure\n    def test_a(self): pass\n", "error", "suite.execution"),
                 ("    pass\n", "error", "suite.execution")]
        for producer in ("goal", "review", "gap"):
            for body, status, target in cases:
                with self.subTest(producer=producer, body=body):
                    proc, report = self.run_producer(body, producer=producer)
                    self.assertEqual(report[target or "suite"]["status"], status, proc.stderr)
                    self.assertEqual(proc.returncode, 0 if status == "pass" else 1)

    def test_subtests_and_teardown_keep_independent_facts(self):
        body = ("    def test_a(self):\n"
                "        with self.subTest(part=1): self.fail('[violation:first] failure')\n"
                "        with self.subTest(part=2): self.fail('[violation:second] failure')\n"
                "    def tearDown(self): raise RuntimeError('cleanup failed')\n")
        for producer in ("goal", "review", "gap"):
            with self.subTest(producer=producer):
                _, report = self.run_producer(body, producer=producer)
                self.assertEqual({item.get("violation") for item in report.values() if item["status"] == "violation"},
                                 {"first", "second"})
                self.assertEqual(report["suite.execution"]["status"], "error")

    def test_assertion_slot_overflow_is_explicit_and_diagnostics_remain(self):
        body = "".join(f"    def test_{i}(self): self.fail('[violation:failure-{i}] observed')\n" for i in range(4))
        for producer in ("goal", "review", "gap"):
            with self.subTest(producer=producer):
                proc, report = self.run_producer(body, producer=producer, slots=2)
                self.assertEqual(sum(item["status"] == "violation" for item in report.values()), 2)
                self.assertEqual(report["suite.execution"]["reason"], "collection_error")
                self.assertIn("failure-3", proc.stderr)


class AssertionIdentity(Fixture):
    def test_compared_data_repr_and_suffixes_never_supply_identity(self):
        bodies = [
            "    def test_a(self): self.assertEqual('[violation:wanted]', 'different')\n",
            "    def test_a(self): self.assertEqual('[requirement]', 'different')\n",
            "    def test_a(self):\n        class Data:\n            def __repr__(self): return '[violation:wanted] data'\n        self.assertEqual(Data(), object())\n",
            "    def test_a(self): self.assertEqual(1, 2, '[violation:wanted] suffix')\n",
            "    def test_a(self): self.assertTrue(False, '[requirement] suffix')\n",
            "    def test_a(self): self.fail('other precondition: [violation:wanted] suffix')\n",
            "    def test_a(self): self.fail('other precondition: [requirement] suffix')\n",
            "    def test_a(self): raise AssertionError('[violation:wanted] untyped direct exception')\n",
            "    def test_a(self): self.assertListEqual(['[violation:wanted]'], [])\n",
        ]
        for producer in ("goal", "review", "gap"):
            for body in bodies:
                with self.subTest(producer=producer, body=body):
                    _, report = self.run_producer(body, producer=producer, expected="wanted")
                    self.assertEqual(report["suite"]["violation"], "unclassified-assertion")
                    self.assertEqual(report["suite.attribution"]["status"], "error")

    def test_explicit_fail_prefix_and_legacy_prefix_are_supported(self):
        for producer in ("goal", "review", "gap"):
            for message, identity in (("[violation:actual] observed", "actual"),
                                      ("[requirement] observed", "requirement-not-met")):
                with self.subTest(producer=producer, message=message):
                    _, report = self.run_producer(f"    def test_a(self): self.fail({message!r})\n", producer=producer)
                    self.assertEqual(report["suite"]["violation"], identity)
                    self.assertEqual(report["suite.attribution"]["status"], "pass")

    def test_expected_identifier_changes_do_not_change_observation(self):
        body = "    def test_a(self): self.fail('[violation:actual] failed requirement')\n"
        for expected in ("actual", "wrong-expectation"):
            _, report = self.run_producer(body, expected=expected)
            self.assertEqual(report["suite"]["violation"], "actual")
            self.assertEqual(report["suite.attribution"]["status"], "pass")

    def test_stdout_tags_and_prior_assertions_do_not_classify_an_untagged_failure(self):
        body = ("    def test_a(self):\n"
                "        print('[violation:wanted] only printed')\n"
                "        self.assertTrue(True, '[violation:wanted] not failed')\n"
                "        self.fail('different precondition failed')\n")
        for producer in ("goal", "review", "gap"):
            with self.subTest(producer=producer):
                proc, report = self.run_producer(body, producer=producer, expected="unclassified-assertion")
                self.assertEqual(report["suite"]["violation"], "unclassified-assertion")
                self.assertEqual(report["suite.attribution"]["status"], "error")
                self.assertIn("only printed", proc.stdout + proc.stderr)

    def test_typed_assertions_and_two_different_tagged_failures_are_preserved(self):
        body = ("    def test_a(self):\n"
                "        from result_observations import RequirementViolation\n"
                "        raise RequirementViolation('observed-typed', 'actual violated requirement')\n")
        for producer in ("goal", "review", "gap"):
            with self.subTest(producer=producer):
                _, report = self.run_producer(body, producer=producer)
                self.assertEqual(report["suite"]["violation"], "observed-typed")
                self.assertEqual(report["suite.attribution"]["status"], "pass")


class GoFacts(Fixture):
    def test_same_test_name_in_different_packages_keeps_failure_and_skip(self):
        (self.root / "go.mod").write_text("module example.invalid/facts\n\ngo 1.22\n")
        for package, body in (("a", 't.Error("actual failure")'), ("b", ""),
                              ("c", 't.Skip("unobserved")'), ("d", 'panic("runtime failed")')):
            directory = self.root / package
            directory.mkdir()
            (directory / "fixture_test.go").write_text('package ' + package + '\nimport "testing"\nfunc TestFacts(t *testing.T) {\n' + body + '\n}\n')
        proc, report = self.run_producer(check={"runner": "go", "package": "./...", "test": "TestFacts"})
        self.assertEqual(report["suite"]["violation"], "unclassified-assertion", proc.stderr)
        self.assertEqual(report["suite.selection"]["status"], "skip")
        self.assertEqual(report["suite.execution"]["reason"], "execution_error")
        self.assertEqual(report["suite.attribution"]["status"], "error")

    def test_actual_go_assertion_and_logged_tag_are_not_interchangeable(self):
        (self.root / "go.mod").write_text("module example.invalid/facts\n\ngo 1.22\n")
        for body, identity, attribution in (
                ('t.Fatal("[violation:actual-go] expected failure")', "unclassified-assertion", "error"),
                ('t.Log("[violation:wanted] only logged")\nt.Fail()', "unclassified-assertion", "error")):
            with self.subTest(body=body):
                (self.root / "fixture_test.go").write_text('package facts\nimport "testing"\nfunc TestFacts(t *testing.T) {\n' + body + '\n}\n')
                proc, report = self.run_producer(check={"runner": "go", "package": ".", "test": "TestFacts"})
                self.assertEqual(report["suite"]["violation"], identity, proc.stderr)
                self.assertEqual(report["suite.attribution"]["status"], attribution)


class GoReportBoundary(Fixture):
    def run_cli(self, *, identity="repository-go-check", version="2", existing=False):
        directory = self.root / "tools/goal-checks"
        directory.mkdir(exist_ok=True)
        for filename in ("produce.py", "go-result.py"):
            shutil.copy2(ROOT / "tools/goal-checks" / filename, directory / filename)
        (self.root / "go.mod").write_text("module example.invalid/facts\n\ngo 1.22\n")
        (self.root / "fixture_test.go").write_text('package facts\nimport "testing"\nfunc TestFacts(t *testing.T) {}\n')
        data = declaration()
        data.update(producer=identity, producer_version=version)
        (self.root / "declaration.json").write_text(json.dumps(data))
        report = self.root / "report.json"
        if existing:
            report.write_text("existing observation must survive\n")
        else:
            report.unlink(missing_ok=True)
        env = dict(ENV, HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="a" * 64,
                   HA_EVIDENCE_DECLARATION_DIGEST="b" * 64)
        proc = subprocess.run([sys.executable, str(directory / "go-result.py"), "declaration.json", ".", "^TestFacts$"],
                              cwd=self.root, env=env, capture_output=True, text=True, timeout=60)
        return proc, report

    def test_existing_report_is_never_overwritten(self):
        proc, report = self.run_cli(existing=True)
        self.assertNotEqual(proc.returncode, 0)
        self.assertEqual(report.read_text(), "existing observation must survive\n")

    def test_supported_identity_is_fixed_and_unknown_declaration_is_refused(self):
        proc, report = self.run_cli()
        self.assertEqual(proc.returncode, 0, proc.stderr)
        data = json.loads(report.read_text())
        self.assertEqual((data["producer"], data["producer_version"]), ("repository-go-check", "2"))
        for identity, version in (("someone-else", "2"), ("repository-go-check", "1")):
            with self.subTest(identity=identity, version=version):
                proc, report = self.run_cli(identity=identity, version=version)
                self.assertNotEqual(proc.returncode, 0)
                self.assertFalse(report.exists())


def tagged(test, code, detail):
    """Fail with an explicit requirement identity the producers can attribute."""
    test.fail(f"[violation:{code}] {detail}")


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class ProvidedCommandTargets(Fixture):
    """Command targets this repository provides keep their meaning in structured reports."""

    def tracked_commands(self):
        for path in sorted((ROOT / "tools/goal-checks").rglob("*.json")):
            data = json.loads(path.read_text(encoding="utf-8"))
            if isinstance(data, dict) and data.get("schema") == "jaekit-goal-targets/v2":
                for check in data["checks"]:
                    if check.get("runner") == "command":
                        yield path.relative_to(ROOT).as_posix(), check

    def docs_target(self):
        source = "tools/goal-checks/release-0-1-3-followups/AC-6.targets.json"
        checks = json.loads((ROOT / source).read_text(encoding="utf-8"))["checks"]
        check = next(c for c in checks if c["target"] == "guide-links")
        return {key: value for key, value in check.items() if key not in ("target", "result_targets")}

    def docs_tree(self, readme, guide="# Guide\n"):
        (self.root / "tools/check_public_docs.py").write_bytes((ROOT / "tools/check_public_docs.py").read_bytes())
        (self.root / "guides").mkdir(exist_ok=True)
        (self.root / "README.md").write_bytes(readme if isinstance(readme, bytes) else readme.encode())
        (self.root / "guides/guide.md").write_text(guide)

    def test_every_provided_command_target_has_an_exit_contract(self):
        commands = list(self.tracked_commands())
        self.assertTrue(commands, "no provided command targets were found")
        for path, check in commands:
            contract = check.get("exit_contract")
            if not isinstance(contract, dict) or 0 not in contract.get("pass", []):
                tagged(self, "provided-command-unusable", f"{path}: {check['target']} has no usable exit contract")

    def test_provided_documentation_target_passes_and_keeps_failure_meanings(self):
        check = self.docs_target()
        self.docs_tree("[guide](guides/guide.md)\n")
        _, report = self.run_producer(check=check)
        if report["suite"]["status"] != "pass":
            tagged(self, "provided-command-unusable", f"normal documents did not pass: {report}")
        self.docs_tree("[guide](guides/absent.md)\n")
        _, report = self.run_producer(check=check)
        if report["suite"]["status"] != "violation":
            tagged(self, "provided-command-unusable", f"a broken link was not a violation: {report}")
        self.docs_tree(b"[guide](guides/guide.md)\n\xff\n")
        _, report = self.run_producer(check=check)
        if any(item["status"] == "violation" for item in report.values()):
            tagged(self, "unreadable-documents-as-violation", f"an unreadable document became a violation: {report}")

    def test_documentation_check_separates_problems_from_unobservable_input(self):
        docs = load(ROOT / "tools/check_public_docs.py", "public_docs_exit_meaning")
        cases = (("problem", "[guide](guides/absent.md)\n", 1), ("unreadable", b"\xff\n", None), ("empty", None, None))
        for label, readme, wanted in cases:
            with self.subTest(case=label), tempfile.TemporaryDirectory() as directory:
                root = Path(directory)
                if readme is not None:
                    (root / "README.md").write_bytes(readme if isinstance(readme, bytes) else readme.encode())
                proc = subprocess.run([sys.executable, str(ROOT / "tools/check_public_docs.py"), str(root)],
                                      env=ENV, capture_output=True, text=True, timeout=60)
                if wanted is not None and proc.returncode != wanted:
                    tagged(self, "provided-command-unusable", f"{label}: exit {proc.returncode}")
                if wanted is None and proc.returncode in (0, 1):
                    tagged(self, "unreadable-documents-as-violation", f"{label}: exit {proc.returncode} {proc.stderr}")
        original = docs.check
        docs.check = lambda *args, **kwargs: (_ for _ in ()).throw(RuntimeError("synthetic internal failure"))
        argv = sys.argv
        try:
            sys.argv = ["check_public_docs.py", str(self.root)]
            try:
                code = docs.main()
            except Exception:
                code = 1  # the interpreter's exit status for an uncaught exception
        finally:
            docs.check, sys.argv = original, argv
        if code in (0, 1):
            tagged(self, "unreadable-documents-as-violation", f"an internal failure exited {code}")

    def test_history_audit_internal_failure_is_not_its_violation_exit(self):
        audit = load(ROOT / "tools/goal-checks/release-0-1-3-followups/preserved.py", "history_exit_meaning")
        audit.history_boundary = lambda *args: (_ for _ in ()).throw(TypeError("synthetic internal failure"))
        try:
            code = audit.main(["preserved.py", "--history", "--base", "0" * 40, "--head", "1" * 40])
        except Exception:
            code = 1  # the interpreter's exit status for an uncaught exception
        if code in (0, 1):
            tagged(self, "unreadable-documents-as-violation", f"an internal audit failure exited {code}")


class OverflowWithErrors(Fixture):
    """More failures than slots and another error keep both facts."""

    def test_overflow_and_execution_error_both_remain(self):
        body = "".join(f"    def test_{i}(self): self.fail('[violation:failure-{i}] observed')\n" for i in range(4))
        body += "    def test_z(self): raise RuntimeError('environment failed')\n"
        for producer in ("goal", "review", "gap"):
            with self.subTest(producer=producer):
                proc, report = self.run_producer(body, producer=producer, slots=2)
                reasons = [item.get("reason") for item in report.values() if item["status"] == "error"]
                if "execution_error" not in reasons or "collection_error" not in reasons:
                    tagged(self, "overflow-or-error-lost", f"{producer}: {report}")
                if not any(item["status"] == "violation" for item in report.values()):
                    tagged(self, "overflow-or-error-lost", f"{producer}: slot violations lost: {report}")
                self.assertNotEqual(proc.returncode, 0)
                self.assertIn("failure-3", proc.stderr)

    def test_overflow_and_setup_error_both_remain(self):
        body = "".join(f"    def test_{i}(self): self.fail('[violation:failure-{i}] observed')\n" for i in range(3))
        body += "    @unittest.expectedFailure\n    def test_z(self): self.fail('decorated')\n"
        proc, report = self.run_producer(body, slots=2)
        reasons = [item.get("reason") for item in report.values() if item["status"] == "error"]
        if "setup_error" not in reasons or "collection_error" not in reasons:
            tagged(self, "overflow-or-error-lost", str(report))


class ExitContractKeys(Fixture):
    """Exit contract keys mean exactly the exit they name, or the contract is refused."""

    def command(self, key, code=1):
        marker = self.root / "ran"
        marker.unlink(missing_ok=True)
        contract = {"schema": "jaekit-command-exits/v1", "pass": [0], "violations": {key: "actual"}, "unverified": [2]}
        return marker, {"runner": "command", "exit_contract": contract,
                        "argv": [sys.executable, "-c", f"open({str(marker)!r}, 'w').close(); raise SystemExit({code})"]}

    def test_noncanonical_keys_are_refused_before_the_command_runs(self):
        for key in ("01", "\u0661", "+1", " 1", "1 ", "0", "256", "1.0"):
            with self.subTest(key=key):
                marker, check = self.command(key)
                proc, report = self.run_producer(check=check)
                if marker.exists() or any(item["status"] == "violation" for item in report.values()):
                    tagged(self, "exit-key-mismatch", f"{key!r} was accepted: {report}")
                if report["suite.execution"].get("reason") != "setup_error":
                    tagged(self, "exit-key-mismatch", f"{key!r} was not refused as configuration: {report}")

    def test_duplicate_keys_are_refused(self):
        marker, check = self.command("1")
        directory = self.root / "tools/goal-checks"
        directory.mkdir(parents=True, exist_ok=True)
        shutil.copy2(ROOT / "tools/goal-checks/produce.py", directory / "produce.py")
        config = json.dumps({"schema": "jaekit-goal-targets/v2",
                             "checks": [dict(check, target="suite", result_targets=names())]})
        config = config.replace('"violations": {"1": "actual"}', '"violations": {"1": "actual", "1": "other"}')
        self.assertIn('"1": "other"', config)
        (self.root / "targets.json").write_text(config)
        (self.root / "declaration.json").write_text(json.dumps(declaration()))
        report = self.root / "report.json"
        env = dict(ENV, HA_EVIDENCE_PATH=str(report), HA_EVIDENCE_INVOCATION="a" * 64,
                   HA_EVIDENCE_DECLARATION_DIGEST="b" * 64)
        proc = subprocess.run([sys.executable, str(directory / "produce.py"), "declaration.json", "targets.json"],
                              cwd=self.root, env=env, capture_output=True, text=True, timeout=60)
        if marker.exists() or proc.returncode != 2:
            tagged(self, "exit-key-mismatch", f"duplicate exit keys were accepted: exit {proc.returncode}")

    def test_canonical_keys_reach_their_exit(self):
        for key, code in (("1", 1), ("255", 255)):
            with self.subTest(key=key):
                _, check = self.command(key, code)
                proc, report = self.run_producer(check=check)
                if report["suite"].get("violation") != "actual":
                    tagged(self, "exit-key-mismatch", f"{key!r} did not reach exit {code}: {report}")


if __name__ == "__main__":
    unittest.main()
