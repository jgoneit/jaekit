"""Adapter conditions from start to completion (AC-1, AC-2, AC-5, AC-9, AC-10).

Each observation builds `ha` from the checked tree and carries a synthetic goal
whose change condition links an existing test through check-adapter/v1:
`go test -json` of the selected Go 1.27 toolchain, or Node's built-in test
runner with its junit reporter writing to the location `ha` names. The
observers read `ha` output, `ha status --format json` and the synthetic
runs.jsonl. AC-9 uses tools/goal-checks/runner-adapters/runner.py, which
reports what it saw at the output location to a file outside the repository.
AC-10's semantics-version cases lay one file over the checked tree with
`go build -overlay`: internal/evidence/adapter_version.go, which holds
`adapterSemantics` (PLAN 구현 결정). When the checked tree has no such file or
identifier, the observation is an error, not a violation.
In AC-1 and AC-2 the user adds the linked test and corrects the declaration's
violation after `ha start` (start needs a declaration that lints), so the
baseline gets both only through the condition's check paths at HEAD.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from adapters_common import (GOAL, PLACEHOLDER, ROOT, Condition, Repo, Unavailable, build, digest_tree,
                             go_declaration, go_typed, junit_declaration, node_runner, require, run, runner_command,
                             runner_file, summary, temporary)

MARK_ID = "greeting-missing"
GO_MOD = "module example.com/demo\n\ngo 1.27\n"
HELLO_BASE = 'package greet\n\n// Hello is the greeting the CLI prints.\nfunc Hello() string { return "hi" }\n'
HELLO_DONE = 'package greet\n\n// Hello is the greeting the CLI prints.\nfunc Hello() string { return "hello" }\n'
CLI = 'package main\n\nimport (\n\t"fmt"\n\n\t"example.com/demo/greet"\n)\n\nfunc main() { fmt.Println(greet.Hello()) }\n'
# The test runs the CLI that exists at the base commit and observes its output.
CLI_TEST = """package main

import (
	"os/exec"
	"strings"
	"testing"
)

func TestGreetsWithHello(t *testing.T) {
	out, err := exec.Command("go", "run", ".").Output()
	if err != nil {
		t.Fatalf("run the greet CLI: %v", err)
	}
	if got := strings.TrimSpace(string(out)); got != "hello" {
		t.Fatalf("jaekit:violation=greeting-missing the CLI says %q", got)
	}
}
"""
GO_FLOW = {"go.mod": GO_MOD, "greet/greet.go": HELLO_BASE, "cmd/greet/main.go": CLI, "tests/keep.sh": "test -f go.mod\n"}
GO_ADDED = {"cmd/greet/greet_test.go": CLI_TEST}
GO_CONDITION = Condition("AC-1", ["go", "test", "-json", "-count=1", "./cmd/greet/"], ["cmd/greet/greet_test.go"],
                         go_declaration([("greeting", MARK_ID, "example.com/demo/cmd/greet", "TestGreetsWithHello")]))
KEEP = Condition("AC-2", ["sh", "tests/keep.sh"], kind="maintain")
NODE_BASE = "export function greet() { return 'hi'; }\n"
NODE_DONE = "export function greet() { return 'hello'; }\n"
NODE_TEST = """import { test } from 'node:test';
import assert from 'node:assert/strict';
import { greet } from '../src/greet.mjs';

test('greets with hello', () => { assert.equal(greet(), 'hello', 'jaekit:violation=greeting-missing wrong greeting'); });
"""
NODE_COMMAND = ["node", "--test", "--test-reporter=junit", f"--test-reporter-destination={PLACEHOLDER}", "test/greet.test.mjs"]
NODE_FLOW = {"src/greet.mjs": NODE_BASE, "tests/keep.sh": "test -f src/greet.mjs\n"}
NODE_ADDED = {"test/greet.test.mjs": NODE_TEST}
NODE_CONDITION = Condition("AC-1", NODE_COMMAND, ["test/greet.test.mjs"],
                           junit_declaration([("greeting", MARK_ID, "test", "greets with hello")]))
WANT_BASELINE = ("fail_as_expected", "declared_violation", [("greeting", 1, "violation", MARK_ID, "")])
WANT_CURRENT = ("pass", "targets_passed", [("greeting", 1, "pass", "", "")])


def draft(condition):
    """The condition with the declaration as first written; the flow corrects its violation after start."""
    declaration = json.loads(json.dumps(condition.declaration))
    declaration["targets"][0]["violation"] = "greeting-draft"
    return Condition(condition.id, condition.command, condition.checks, declaration, condition.kind)


def corrected(condition):
    return {f"checks/{condition.id}.json": json.dumps(condition.declaration, indent=1, ensure_ascii=False) + "\n"}


def mark_done(repo):
    repo.write(f"{GOAL}/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n"
                                      "| --- | --- | --- |\n| T001 | done | |\n")


def flow(case, repo, added, implement, violation, problems):
    """The linked test added after start, then baseline, implementation, check, status and done of AC-1 with the
    AC-2 maintain condition."""
    code, out = repo.start()
    require(case, code == 0, violation, f"ha start refused a goal whose change condition uses an adapter: {out[-1500:]}")
    for path, text in added.items():
        repo.write(path, text)
    repo.commit("add the linked test and correct the declaration")
    repo.check("AC-1", baseline=True)
    got = summary(repo.last("AC-1", "baseline"))
    if got != WANT_BASELINE:
        problems.append(f"baseline: got {got}, want {WANT_BASELINE}")
    implement()
    repo.commit("implement")
    repo.check("AC-1", "AC-2")
    record = repo.last("AC-1", "check")
    got = summary(record)
    if got != WANT_CURRENT:
        problems.append(f"check: got {got}, want {WANT_CURRENT}")
    if record and record.get("argv") != repo.conditions[0].command:
        problems.append(f"the recorded command {record.get('argv')} is not the plan's runner command")
    mark_done(repo)
    _, status = repo.status()
    criterion = next((c for c in status.get("criteria", []) if c.get("id") == "AC-1"), {})
    if status.get("status") != "complete" or not criterion.get("satisfied"):
        problems.append(f"ha status: {status.get('status')}, AC-1 {criterion}, reasons {status.get('reasons')}")
    code, out, err = repo.ha("done", GOAL)
    _, after = repo.status()
    if code != 0 or after.get("completion_record") is None:
        problems.append(f"ha done exit {code} without a completion record: {(out + err)[-800:]}")


class GoFlow(unittest.TestCase):
    """AC-1: a Go project adds one declaration and one test and completes with `go test -json`."""

    def test_go_test_json_condition_completes(self):
        go_typed()
        problems = []
        with temporary("jaekit-adapter-go-flow-") as directory:
            repo = Repo(directory, build(Path(directory) / "ha"), [draft(GO_CONDITION), KEEP], files=GO_FLOW)
            declaration = json.loads((repo.root / "checks/AC-1.json").read_text(encoding="utf-8"))
            if {"producer", "producer_version", "producer_paths"} & set(declaration):
                problems.append("the declaration names a producer")
            flow(self, repo, {**GO_ADDED, **corrected(GO_CONDITION)}, lambda: repo.write("greet/greet.go", HELLO_DONE),
                 "go-adapter-flow-missing", problems)
        require(self, not problems, "go-adapter-flow-missing", "; ".join(problems))


class JUnitFlow(unittest.TestCase):
    """AC-2: a separate runner writes JUnit XML where ha says, and the condition completes."""

    def test_junit_condition_completes(self):
        node_runner()
        problems = []
        with temporary("jaekit-adapter-junit-flow-") as directory:
            repo = Repo(directory, build(Path(directory) / "ha"), [draft(NODE_CONDITION), Condition("AC-2", ["sh", "tests/keep.sh"],
                                                                                                   kind="maintain")],
                        files=NODE_FLOW)
            flow(self, repo, {**NODE_ADDED, **corrected(NODE_CONDITION)}, lambda: repo.write("src/greet.mjs", NODE_DONE),
                 "junit-adapter-flow-missing", problems)
            record = repo.last("AC-1", "check")
            if record and not (record.get("evidence") or {}).get("report_digest"):
                problems.append("the check did not read a runner file (no report digest)")
        require(self, not problems, "junit-adapter-flow-missing", "; ".join(problems))


GREETER_TEST = """package greet

import "testing"

func TestNewGreeter(t *testing.T) {
	if got := New().Say(); got != "hello" {
		t.Fatalf("jaekit:violation=greeting-missing the greeter says %q", got)
	}
}
"""
GREETER = 'package greet\n\ntype Greeter struct{}\n\nfunc New() Greeter { return Greeter{} }\n\nfunc (Greeter) Say() string { return Hello() }\n'
NODE_MISSING_TEST = """import { test } from 'node:test';
import assert from 'node:assert/strict';
import { greeter } from '../src/greeter.mjs';

test('greeter says hello', () => { assert.equal(greeter(), 'hello', 'jaekit:violation=greeting-missing wrong greeting'); });
"""


class MissingEntryPoint(unittest.TestCase):
    """AC-5: a test that needs code the base commit lacks never gives an expected baseline failure."""

    def test_missing_entry_points(self):
        go_typed()
        node_runner()
        problems = []
        with temporary("jaekit-adapter-entry-") as directory:
            conditions = [
                Condition("AC-1", ["go", "test", "-json", "-count=1", "./greet/"], ["greet/greeter_test.go"],
                          go_declaration([("greeter", MARK_ID, "example.com/demo/greet", "TestNewGreeter")])),
                Condition("AC-2", ["node", "--test", "--test-reporter=junit", f"--test-reporter-destination={PLACEHOLDER}",
                                   "test/greeter.test.mjs"], ["test/greeter.test.mjs"],
                          junit_declaration([("greeter", MARK_ID, "test", "greeter says hello")])),
            ]
            files = {"go.mod": GO_MOD, "greet/greet.go": HELLO_DONE, "greet/greeter_test.go": GREETER_TEST,
                     "test/greeter.test.mjs": NODE_MISSING_TEST}
            repo = Repo(directory, build(Path(directory) / "ha"), conditions, files=files)
            code, out = repo.start()
            require(self, code == 0, "missing-entry-point-accepted", f"ha start refused: {out[-1500:]}")
            repo.check("AC-1", "AC-2", baseline=True)
            go_base, node_base = summary(repo.last("AC-1", "baseline")), summary(repo.last("AC-2", "baseline"))
            want = ("error", "environment_reported", [("greeter", 1, "error", "", "compile_error")])
            if go_base != want:
                problems.append(f"Go baseline calling a function the base lacks: got {go_base}, want {want}")
            if node_base is None or node_base[0] not in ("error", "unknown"):
                problems.append(f"JUnit baseline importing a module the base lacks: got {node_base}, want error or unknown")
            repo.write("greet/greeter.go", GREETER)
            repo.write("src/greeter.mjs", "export function greeter() { return 'hello'; }\n")
            repo.commit("implement")
            repo.check("AC-1", "AC-2")
            for cid in ("AC-1", "AC-2"):
                got = summary(repo.last(cid, "check"))
                if got is None or got[:2] != ("pass", "targets_passed"):
                    problems.append(f"{cid} after the implementation: got {got}, want a pass")
        require(self, not problems, "missing-entry-point-accepted", "; ".join(problems))


def in_working_tree(path, root, cwd):
    """Whether path lies in the repository's working tree or the run's (baseline) tree, outside Git metadata."""
    target = Path(path).resolve()
    for tree in {Path(root).resolve(), Path(cwd).resolve()}:
        if target.is_relative_to(tree) and not target.is_relative_to(tree / ".git"):
            # A baseline tree lives under <git dir>/ha/worktrees; the run's own tree still counts.
            return True
    return False


class OutputLocation(unittest.TestCase):
    """AC-9: each file-writing run gets a fresh private location; the working tree and command stay as they are."""

    def test_fresh_private_location(self):
        problems = []
        decl = lambda: junit_declaration([("t", MARK_ID, "tests.test_greet", "test_greets")])
        with temporary("jaekit-adapter-location-") as directory:
            log = Path(directory) / "observed.jsonl"
            conditions = [
                Condition("AC-1", runner_command("junit-probe", PLACEHOLDER), ["tests/runner.py"], decl()),
                Condition("AC-2", runner_command("junit-probe"), ["tests/runner.py"], decl()),
                Condition("AC-3", runner_command("junit-none", PLACEHOLDER), ["tests/runner.py"], decl()),
                Condition("AC-4", runner_command("junit-slow-pass", PLACEHOLDER), ["tests/runner.py"], decl()),
                Condition("AC-5", runner_command("junit-slow-marked", PLACEHOLDER), ["tests/runner.py"], decl()),
                Condition("AC-6", runner_command("junit-probe", PLACEHOLDER), kind="maintain"),
            ]
            planted = '<?xml version="1.0"?>\n<testsuites><testsuite><testcase classname="tests.test_greet" name="test_greets"/></testsuite></testsuites>\n'
            files = {"tests/runner.py": runner_file(), "junit.xml": planted, "tests/junit.xml": planted, PLACEHOLDER: planted}
            repo = Repo(directory, build(Path(directory) / "ha"), conditions, files=files)
            repo.env["JAEKIT_OBSERVE_LOG"] = str(log)
            code, out = repo.start()
            require(self, code == 0, "output-location-unsafe", f"ha start refused: {out[-1500:]}")
            before = digest_tree(repo.root, skip=(".git", "docs"))
            repo.check("AC-1", baseline=True)
            repo.check("AC-1")
            repo.check("AC-1")
            repo.check("AC-2", baseline=True)
            repo.check("AC-2")
            repo.check("AC-3", baseline=True)
            repo.check("AC-3")
            for baseline in (True, False):
                args = ["check", GOAL, *(["--baseline"] if baseline else [])]
                procs = [subprocess.Popen([str(repo.ha_path), *args, cid], cwd=repo.root, env=repo.env,
                                          stdout=subprocess.PIPE, stderr=subprocess.PIPE) for cid in ("AC-4", "AC-5")]
                for proc in procs:
                    proc.communicate(timeout=600)
                    if proc.returncode < 0:
                        raise Unavailable(f"a concurrent ha check ended by signal {-proc.returncode}")
            repo.check("AC-6")
            after = digest_tree(repo.root, skip=(".git", "docs"))
            if before != after:
                changed = sorted(set(before.items()) ^ set(after.items()))
                problems.append(f"the working tree changed: {changed[:6]}")
            try:
                seen = [json.loads(line) for line in log.read_text(encoding="utf-8").splitlines()]
            except (OSError, ValueError) as error:
                raise Unavailable(f"probe log: {error}") from None
            adapter_runs, maintain_runs = seen[:5], seen[5:]
            if len(adapter_runs) != 5 or len(maintain_runs) != 1:
                raise Unavailable(f"expected six probe runs, saw {len(seen)}")
            paths = []
            for index, item in enumerate(adapter_runs):
                path = item["argv"][1] if len(item["argv"]) > 1 else item["env"]
                paths.append(path)
                where = f"probe run {index + 1}"
                if not item["env"] or not os.path.isabs(item["env"]):
                    problems.append(f"{where}: {PLACEHOLDER.strip('{}')} is {item['env']!r}")
                if len(item["argv"]) > 1 and item["argv"][1] != item["env"]:
                    problems.append(f"{where}: the placeholder became {item['argv'][1]!r}, the variable {item['env']!r}")
                if item["existed"] or item["parent_entries"] != []:
                    problems.append(f"{where}: the location was not new and empty ({item['existed']}, {item['parent_entries']})")
                if path and in_working_tree(path, repo.root, item["cwd"]):
                    problems.append(f"{where}: the location {path} is inside the working tree")
            if len(set(paths)) != len(paths):
                problems.append(f"locations repeat across runs: {paths}")
            if maintain_runs[0]["argv"][1:] != [PLACEHOLDER] or maintain_runs[0]["env"] is not None:
                problems.append(f"a condition without an adapter got {maintain_runs[0]['argv']}, variable {maintain_runs[0]['env']!r}")
            # The files the runner wrote at the named location are the ones ha classified.
            for cid in ("AC-1", "AC-2"):
                for kind, want in (("baseline", ("unexpected_pass", "baseline_passed")), ("check", ("pass", "targets_passed"))):
                    got = summary(repo.last(cid, kind))
                    if got is None or got[:2] != want:
                        problems.append(f"{cid} {kind}: the file written at the location was not classified: got {got}")
            for kind in ("baseline", "check"):
                got = summary(repo.last("AC-3", kind))
                if got != ("unknown", "output_missing", []):
                    problems.append(f"AC-3 {kind} with only earlier and same-named files: got {got}")
            for kind, wants in (("check", {"AC-4": "pass", "AC-5": "fail"}),
                                ("baseline", {"AC-4": "unexpected_pass", "AC-5": "fail_as_expected"})):
                for cid, want in wants.items():
                    got = summary(repo.last(cid, kind))
                    if got is None or got[0] != want:
                        problems.append(f"concurrent {kind} {cid}: got {got}, want {want}")
            text = (repo.root / GOAL / "runs.jsonl").read_text(encoding="utf-8")
            leaked = [p for p in paths if p and p in text]
            if leaked:
                problems.append(f"runs.jsonl contains run locations: {leaked[:2]}")
            records = [r for r in repo.records() if r.get("kind") in ("check", "baseline")
                       and (r.get("target") or {}).get("criterion") == "AC-1"]
            digests = {r.get("command_digest") for r in records}
            if len(digests) != 1 or any(r.get("argv") != conditions[0].command for r in records):
                problems.append(f"AC-1 runs do not keep one command identity: {sorted(digests)}, "
                                f"{[r.get('argv') for r in records]}")
        require(self, not problems, "output-location-unsafe", "; ".join(problems))


SEMANTICS_FILE = "internal/evidence/adapter_version.go"
SEMANTICS_DECLARED = 'var adapterSemantics = map[string]string{"go-test-json": "1", "junit-xml": "1"}'


def semantics_changed(adapter):
    """The version file with one adapter's classification meaning changed."""
    versions = {"go-test-json": "1", "junit-xml": "1", adapter: "2"}
    return ("package evidence\n\n// Laid over by the runner-adapters observer: a changed classification meaning.\n"
            f'var adapterSemantics = map[string]string{{"go-test-json": "{versions["go-test-json"]}", '
            f'"junit-xml": "{versions["junit-xml"]}"}}\n')
SECRET = "JAEKIT-SECRET-RUNNER-MESSAGE"


class EvidenceIdentity(unittest.TestCase):
    """AC-10: adapter evidence has the identity and storage properties of other evidence."""

    def test_identity_and_storage(self):
        go_typed()
        problems = []
        with temporary("jaekit-adapter-identity-") as directory:
            ha = build(Path(directory) / "ha")
            secrets = [Condition("AC-3", runner_command("go-secret"), ["tests/runner.py"],
                                 go_declaration([("t", MARK_ID, "example.com/demo/forms", "TestMarked")])),
                       Condition("AC-4", runner_command("junit-secret", PLACEHOLDER), ["tests/runner.py"],
                                 junit_declaration([("t", MARK_ID, "tests.test_greet", "test_greets")]))]
            junit = Condition("AC-5", runner_command("junit-greet", PLACEHOLDER), ["tests/runner.py"],
                              junit_declaration([("t", MARK_ID, "tests.test_greet", "test_greets")]))
            repo = Repo(directory, ha, [GO_CONDITION, KEEP, *secrets, junit],
                        files={**GO_FLOW, **GO_ADDED, "tests/runner.py": runner_file()})
            outputs = []
            code, out = repo.start()
            require(self, code == 0, "adapter-evidence-identity-missing", f"ha start refused: {out[-1500:]}")
            outputs.append(repo.check("AC-1", "AC-3", "AC-4", "AC-5", baseline=True)[1])
            repo.write("greet/greet.go", HELLO_DONE)
            repo.commit("implement")
            outputs.append(repo.check("AC-1", "AC-2", "AC-3", "AC-4", "AC-5")[1])
            mark_done(repo)
            _, before = repo.status()
            satisfied = lambda status, cid="AC-1": next((c for c in status.get("criteria", []) if c.get("id") == cid),
                                                        {}).get("satisfied")
            for cid in ("AC-1", "AC-5"):
                if not satisfied(before, cid):
                    problems.append(f"{cid} is not satisfied before any change: {before.get('reasons')}")
            # A changed declaration (violation, then linked test) leaves the earlier runs stale.
            for label, change in (("violation", lambda d: d["targets"][0].update(violation="greeting-gone")),
                                  ("linked test", lambda d: d["targets"][0]["test"].update(name="TestGreetsAgain"))):
                copy = Path(directory) / f"copy-{label.replace(' ', '-')}"
                shutil.copytree(repo.root, copy / "repo", symlinks=True)
                other = Repo.__new__(Repo)
                other.__dict__.update(repo.__dict__, root=copy / "repo")
                declaration = json.loads((other.root / "checks/AC-1.json").read_text(encoding="utf-8"))
                change(declaration)
                other.write("checks/AC-1.json", json.dumps(declaration, indent=1) + "\n")
                other.commit(f"change the {label}")
                _, status = other.status()
                if satisfied(status):
                    problems.append(f"after changing the declared {label}, the earlier runs still satisfy AC-1")
            # A changed adapter meaning version leaves the same runs of that adapter stale.
            try:
                version_file = (ROOT / SEMANTICS_FILE).read_text(encoding="utf-8")
            except OSError as error:
                raise Unavailable(f"{SEMANTICS_FILE}: {error}") from None
            if SEMANTICS_DECLARED not in version_file:
                raise Unavailable(f"{SEMANTICS_FILE} does not declare {SEMANTICS_DECLARED}")
            code, out, err = repo.ha("status", GOAL, "--format", "json")
            try:
                now = json.loads(out)
            except ValueError:
                raise Unavailable(f"status JSON: {(out + err)[-800:]}") from None
            for cid in ("AC-1", "AC-5"):
                if not satisfied(now, cid):
                    problems.append(f"the unchanged executable no longer satisfies {cid}")
            for adapter, cid in (("go-test-json", "AC-1"), ("junit-xml", "AC-5")):
                changed = build(Path(directory) / adapter / "ha", overlay={SEMANTICS_FILE: semantics_changed(adapter)})
                other_code, other_out, other_err = run_with(changed, repo)
                try:
                    with_change = json.loads(other_out)
                except ValueError:
                    raise Unavailable(f"status JSON: {(other_out + other_err)[-800:]}") from None
                if satisfied(with_change, cid):
                    problems.append(f"an executable with another {adapter} meaning version still uses the earlier runs of {cid}")
            # Removing the raw logs keeps the judgment.
            for log in (repo.root / ".git" / "ha" / "runs").rglob("*.log"):
                log.unlink()
            _, without_logs = repo.status()
            keep = lambda s: (s.get("status"), s.get("reasons"), [(c.get("id"), c.get("satisfied"), c.get("reason"))
                                                                   for c in s.get("criteria", [])])
            if keep(without_logs) != keep(now):
                problems.append(f"removing raw logs changed the judgment: {keep(now)} -> {keep(without_logs)}")
            # Records and ha output hold identifiers, reason codes and digests, never runner messages.
            outputs += [json.dumps(now), json.dumps(without_logs)]
            done_code, done_out, done_err = repo.ha("done", GOAL)
            outputs.append(done_out + done_err)
            text = (repo.root / GOAL / "runs.jsonl").read_text(encoding="utf-8")
            for where, content in (("runs.jsonl", text), ("ha output", "\n".join(outputs))):
                for word in (SECRET, "the CLI says", "wrong greeting", "<testsuite", "OutputType"):
                    if word in content and not (where == "ha output" and word == "OutputType"):
                        problems.append(f"{where} contains runner output {word!r}")
        require(self, not problems, "adapter-evidence-identity-missing", "; ".join(problems))


def run_with(executable, repo):
    result = run([executable, "status", GOAL, "--format", "json"], repo.root, repo.env, timeout=600)
    return result.returncode, result.stdout.decode(errors="replace"), result.stderr.decode(errors="replace")


if __name__ == "__main__":
    unittest.main()
