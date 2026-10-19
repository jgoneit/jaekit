"""Declarations, capabilities, documents, the regression registry and Go 1.27 (AC-8, AC-13, AC-15 to AC-18).

AC-8 and AC-13 run the `ha` built from the checked tree (and Seal's own
compatibility helper with a stand-in Core). AC-15 and AC-16 read fixed tokens
in named files and named sections; AC-16 also compares every other section of
the four Seal files it may change, and the whole compatibility helper, with
digests written in this file (the content those parts must keep). AC-17 reads
contracts/regressions.json, runs tools/regression_map.py against the registry
copy kept next to this file, and runs the protected selections with a `go`
stand-in on PATH that records each nested invocation before running the real
`go`. AC-18 reads go.mod, the install guides and the workflows, runs the whole
Go test suite with the selected toolchain, and, for Windows, requires the
native check Windows CI runs and the passing CI run of the checked commit. No
history is read.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import stat
import subprocess
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from adapters_common import (FEATURE, GOAL, PLACEHOLDER, ROOT, SCHEMA, UNMARKED, VARIABLE, Condition, Repo, Unavailable,
                             build, clean_env, go_binary_results, go_declaration, go_test_results, go_typed,
                             junit_declaration, require, run, temporary)
from observe_forms import ci_run_passes, windows_native_tests

HERE = Path(__file__).resolve().parent
MARK_ID = "greeting-missing"


def text(path):
    try:
        return (ROOT / path).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"{path}: {error}") from None


def pieces(content, pattern):
    """[(heading, text)] split before every line matching pattern; the first piece has heading ''."""
    out, head, buf = [], "", []
    for line in content.splitlines(keepends=True):
        if re.match(pattern, line):
            out.append((head, "".join(buf)))
            head, buf = line.rstrip("\n"), [line]
        else:
            buf.append(line)
    out.append((head, "".join(buf)))
    return out


def section(content, heading, pattern=r"#{1,6} "):
    for head, body in pieces(content, pattern):
        if head == heading:
            return body
    return None


def short(value):
    return hashlib.sha256(value.encode()).hexdigest()[:16]


# --- AC-8 --------------------------------------------------------------------

def go_target(test, **extra):
    target = {"id": "t", "violation": MARK_ID, "test": test}
    target.update(extra)
    return target


BAD_DECLARATIONS = {
    "unknown adapter": {"schema": SCHEMA, "adapter": "pytest-json", "targets": [go_target({"package": "example.com/demo", "name": "TestX"})]},
    "target without a test": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [{"id": "t", "violation": MARK_ID}]},
    "empty test name": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [go_target({"package": "example.com/demo", "name": ""})]},
    "control character in a Go name": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [go_target({"package": "example.com/demo", "name": "TestX\u0001"})]},
    "line feed in a JUnit name": {"schema": SCHEMA, "adapter": "junit-xml", "targets": [go_target({"classname": "tests.a", "name": "test_a\nb"})]},
    "two targets, one test": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [
        {"id": "a", "violation": MARK_ID, "test": {"package": "example.com/demo", "name": "TestX"}},
        {"id": "b", "violation": "other-id", "test": {"package": "example.com/demo", "name": "TestX"}}]},
    "Go target with a JUnit name": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [go_target({"classname": "tests.a", "name": "test_a"})]},
    "JUnit target with a Go name": {"schema": SCHEMA, "adapter": "junit-xml", "targets": [go_target({"package": "example.com/demo", "name": "TestX"})]},
    "Go name that is not a test": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [go_target({"package": "example.com/demo", "name": "helperX"})]},
    "reserved jaekit: violation": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [
        go_target({"package": "example.com/demo", "name": "TestX"}, violation=UNMARKED)]},
    "violation ending with a colon": {"schema": SCHEMA, "adapter": "go-test-json", "targets": [
        go_target({"package": "example.com/demo", "name": "TestX"}, violation=MARK_ID + ":")]},
    "violation ending with a period": {"schema": SCHEMA, "adapter": "junit-xml", "targets": [
        go_target({"classname": "tests.a", "name": "test_a"}, violation=MARK_ID + ".")]},
}
VALID = go_declaration([("t", MARK_ID, "example.com/demo/forms", "TestMarked")])
# Declarations that must lint clean: a Go subtest name with empty parts, and a producer
# declaration whose violation ends with a colon, which check-declaration/v1 keeps accepting.
VALID_OTHERS = {
    "Go subtest name with empty parts": go_declaration([("t", MARK_ID, "example.com/demo/forms", "TestMarked/a//b")]),
    "check-declaration/v1 violation ending with a colon": {
        "schema": "check-declaration/v1", "producer": "x", "producer_version": "1", "producer_paths": ["tests/x.py"],
        "targets": [{"id": "t", "violation": MARK_ID + ":"}]},
}


class Lint(unittest.TestCase):
    """AC-8: ha lint shows adapter declaration problems as evidence_invalid, and start and check refuse them."""

    def test_adapter_declaration_problems(self):
        problems = []
        with temporary("jaekit-adapter-lint-") as directory:
            ha = build(Path(directory) / "ha")
            names = list(BAD_DECLARATIONS)
            conditions = [Condition("AC-1", ["python3", "tests/x.py"], ["tests/x.py"], VALID)]
            conditions += [Condition(f"AC-{i}", ["python3", "tests/x.py"], ["tests/x.py"], BAD_DECLARATIONS[n])
                           for i, n in enumerate(names, 2)]
            others = list(VALID_OTHERS)
            first_other = len(names) + 2
            conditions += [Condition(f"AC-{i}", ["python3", "tests/x.py"], ["tests/x.py"], VALID_OTHERS[n])
                           for i, n in enumerate(others, first_other)]
            repo = Repo(directory, ha, conditions, files={"tests/x.py": "print('x')\n"})
            code, out, err = repo.ha("lint", GOAL)
            lines = (out + err).splitlines()
            valid_problems = [line for line in lines if "AC-1:" in line or re.search(r"\bAC-1\b ", line)]
            require(self, not valid_problems, "adapter-lint-missing",
                    f"a valid check-adapter/v1 declaration has lint problems: {valid_problems}")
            for i, name in enumerate(names, 2):
                if not any(line.startswith("evidence_invalid") and f"AC-{i}:" in line for line in lines):
                    problems.append(f"{name} (AC-{i}) is not shown as evidence_invalid")
            for i, name in enumerate(others, first_other):
                shown = [line for line in lines if f"AC-{i}:" in line or re.search(rf"\bAC-{i}\b ", line)]
                if shown:
                    problems.append(f"{name} (AC-{i}) has lint problems: {shown}")
            code, out = repo.start()
            if code != 65 or repo.records():
                problems.append(f"ha start with those problems: exit {code}, records {len(repo.records())}")
        with temporary("jaekit-adapter-lint-rules-") as directory:
            ha = build(Path(directory) / "ha")
            conditions = [Condition("AC-1", ["python3", "tests/x.py"], ["tests/x.py"], VALID)]
            repo = Repo(directory, ha, conditions, files={"tests/x.py": "print('x')\n"})
            code, out = repo.start()
            require(self, code == 0, "adapter-lint-missing", f"ha start refused a valid adapter declaration: {out[-1500:]}")
            before = len(repo.records())
            for label, mutate in (("bad declaration", self.bad), ("declaration outside check paths", self.outside),
                                  ("uncommitted declaration", self.uncommitted), ("symlinked declaration", self.symlinked)):
                copy = Path(directory) / ("copy-" + label.replace(" ", "-"))
                shutil.copytree(repo.root, copy / "repo", symlinks=True)
                other = Repo.__new__(Repo)
                other.__dict__.update(repo.__dict__, root=copy / "repo")
                want = mutate(other)
                code, out = other.check("AC-1")
                lint = other.ha("lint", GOAL)
                if code != 65 or len(other.records()) != before or want not in lint[1] + lint[2]:
                    problems.append(f"{label}: ha check exit {code}, records {len(other.records())}, lint {lint[1][-300:]!r}")
        require(self, not problems, "adapter-lint-missing", "; ".join(problems))

    @staticmethod
    def bad(repo):
        repo.write("checks/AC-1.json", json.dumps(BAD_DECLARATIONS["unknown adapter"]))
        repo.commit("bad declaration")
        return "evidence_invalid"

    @staticmethod
    def outside(repo):
        plan = (repo.root / GOAL / "PLAN.md").read_text(encoding="utf-8")
        repo.write(f"{GOAL}/PLAN.md", plan.replace("tests/x.py, checks/AC-1.json |", "tests/x.py |"))
        repo.commit("declaration outside check paths")
        return "evidence_overlay_missing"

    @staticmethod
    def uncommitted(repo):
        repo.git("rm", "-q", "--cached", "checks/AC-1.json")
        repo.write(".git/info/exclude", "checks/AC-1.json\n")
        repo.git("commit", "-q", "-m", "untrack the declaration")
        return "evidence_invalid"

    @staticmethod
    def symlinked(repo):
        real = repo.root / "checks/real.json"
        real.write_text((repo.root / "checks/AC-1.json").read_text(encoding="utf-8"), encoding="utf-8")
        (repo.root / "checks/AC-1.json").unlink()
        os.symlink("real.json", repo.root / "checks/AC-1.json")
        repo.commit("symlinked declaration")
        return "evidence_invalid"


# --- AC-13 -------------------------------------------------------------------

# The capability values the checked Core must keep (contracts/core-capabilities.md).
KEPT = {
    "schema": "ha-capabilities/v1",
    "supported_rules": ["run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4"],
    "default_rules": "run-rules/3",
    "formats": {"finding": ["run-finding/v1"], "decision_history": ["decision-history/v1"], "criteria": ["legacy", "nested/1"],
                "check_declaration": ["check-declaration/v1"], "check_result": ["check-result/v1"],
                "check_evidence": ["check-evidence/v1"], "timing": ["timing/v1"]},
    "features": ["estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1", "budget-change/v1",
                 "post-completion-findings/v1", "decision-history/v1", "run-timing/v1", "test-authority/v1", "reported-wait/v1",
                 "preapproved-runs/v1", "error-recovery/v1", "command-time-limits/v1"],
}
CHECK_CORE = "plugins/seal/skills/seal/scripts/check_core.py"


class Capabilities(unittest.TestCase):
    """AC-13: capabilities add the declaration format and the adapter feature; Seal's start and resume do not need them."""

    def test_capabilities_and_seal_compatibility(self):
        problems = []
        with temporary("jaekit-adapter-capabilities-") as directory:
            ha = build(Path(directory) / "ha")
            result = run([ha, "capabilities", "--format", "json"], directory, clean_env(), timeout=60)
            try:
                capabilities = json.loads(result.stdout)
            except ValueError:
                raise Unavailable(f"ha capabilities: exit {result.returncode}: {result.stderr.decode(errors='replace')}") from None
            declarations = capabilities.get("formats", {}).get("check_declaration", [])
            features = capabilities.get("features", [])
            require(self, SCHEMA in declarations and FEATURE in features, "adapter-capability-missing",
                    f"capabilities lack {SCHEMA} or {FEATURE}: {declarations}, {features}")
            for key in ("schema", "supported_rules", "default_rules"):
                if capabilities.get(key) != KEPT[key]:
                    problems.append(f"{key} changed: {capabilities.get(key)}")
            for name, values in KEPT["formats"].items():
                got = capabilities.get("formats", {}).get(name)
                want = values + [SCHEMA] if name == "check_declaration" else values
                if got != want:
                    problems.append(f"formats.{name}: got {got}, want {want}")
            if features != KEPT["features"] + [FEATURE]:
                problems.append(f"features: got {features}, want the earlier list followed by {FEATURE}")
            # A Core without the adapter additions still satisfies Seal's start and resume requirements.
            older = dict(capabilities, formats=dict(capabilities["formats"], check_declaration=["check-declaration/v1"]),
                         features=[f for f in features if f != FEATURE])
            stand_in = Path(directory) / "older-ha"
            stand_in.write_text("#!/bin/sh\nif [ \"$1\" = capabilities ]; then\n"
                                f"  printf '%s\\n' '{json.dumps(older)}'\n  exit 0\nfi\n"
                                f"exec '{ha}' \"$@\"\n", encoding="utf-8")
            stand_in.chmod(0o755)
            conditions = [Condition("AC-1", ["python3", "tests/x.py"], ["tests/x.py"], VALID)]
            repo = Repo(Path(directory) / "goal", ha, conditions, files={"tests/x.py": "print('x')\n"})
            code, out = repo.start()
            if code != 0:
                problems.append(f"ha start refused the synthetic goal: {out[-500:]}")
            for operation in ("start", "resume", "check"):
                args = [sys.executable, ROOT / CHECK_CORE, "--ha", stand_in, "--operation", operation]
                if operation != "start":
                    args += ["--goal", GOAL]
                result = run(args, repo.root, repo.env, timeout=120)
                try:
                    report = json.loads(result.stdout)
                except ValueError:
                    raise Unavailable(f"check_core.py {operation}: {result.stderr.decode(errors='replace')}") from None
                if result.returncode != 0 or not report.get("compatible"):
                    problems.append(f"Seal's {operation} check needs the adapter additions: {report}")
        require(self, not problems, "adapter-capability-missing", "; ".join(problems))


# --- AC-15 -------------------------------------------------------------------

CONTRACT_HEADING = "## 5. 러너 어댑터"
CONTRACT_LINK = "check-result.md#5-러너-어댑터"
DOCUMENT_TOKENS = {
    "contracts/check-result.md": [CONTRACT_HEADING, SCHEMA, "go-test-json", "junit-xml", "jaekit:violation=", UNMARKED,
                                  PLACEHOLDER, VARIABLE, "output_missing", "output_corrupt", "output_too_large", "64 MiB",
                                  "16 MiB", "Go 1.27", "OutputType", FEATURE, "G1", "G16", "J1", "J14", "비교값",
                                  "--test-reporter=junit", "pytest", "Vitest", "jest-junit", "compile_error", "execution_error",
                                  "target_missing", "target_skipped", "attempt_history_missing", "report_exit_conflict",
                                  "observation_invalid",
                                  # The runner versions whose real output was checked once (SPEC 결정과 이유).
                                  "pytest 9.1.1", "Vitest 5.0.5", "Jest 30.5.2", "jest-junit 17.0.0"],
    "contracts/core-capabilities.md": [SCHEMA, FEATURE],
    "contracts/bundle.md": [SCHEMA],
    "contracts/run-record.md": [SCHEMA, "판정 의미 버전"],
}
# Fixed file and directory names a runner installation or dependency declaration adds.
RUNNER_FILES = {"package.json", "package-lock.json", "npm-shrinkwrap.json", "yarn.lock", "pnpm-lock.yaml",
                "requirements.txt", "Pipfile", "Pipfile.lock", "poetry.lock", "pyproject.toml", "setup.py", "setup.cfg"}
RUNNER_DIRECTORIES = {"node_modules", ".venv", "venv", "site-packages"}
GUIDE_SECTIONS = {"guides/OPERATIONS.md": "## 수정 전 실패가 뜻하는 것",
                  "guides/OPERATIONS.en.md": "## What a failure before the change means"}


class Documents(unittest.TestCase):
    """AC-15: the contracts and both operations guides describe the adapters."""

    def test_contracts_and_guides(self):
        problems = []
        for path, tokens in DOCUMENT_TOKENS.items():
            content = text(path)
            missing = [t for t in tokens if t not in content]
            if missing:
                problems.append(f"{path} lacks {missing}")
        for path, heading in GUIDE_SECTIONS.items():
            body = section(text(path), heading, r"## ")
            if body is None:
                problems.append(f"{path} has no section {heading!r}")
            elif CONTRACT_LINK not in body or SCHEMA not in body:
                problems.append(f"{path} {heading!r} does not point to {CONTRACT_LINK} with {SCHEMA}")
        result = run(["git", "ls-files", "-z"], ROOT, timeout=120)
        if result.returncode != 0:
            raise Unavailable(f"git ls-files: {result.stderr.decode(errors='replace')}")
        for name in result.stdout.decode("utf-8", errors="replace").split("\0"):
            parts = name.split("/")
            if parts[-1] in RUNNER_FILES or RUNNER_DIRECTORIES.intersection(parts[:-1]):
                problems.append(f"tracked runner installation or dependency declaration {name}")
        require(self, not problems, "adapter-contract-missing", "; ".join(problems))


# --- AC-16 -------------------------------------------------------------------

SEAL = "plugins/seal/skills/seal"
SEAL_VERSION = "0.1.18"
CHECKS_BULLET = "- Choose the checks, runner and verification method"
HA_SECTION = "## Structured change results"
BUNDLE_SECTION = "### `## 결과 계약`"
TEMPLATE_SECTION = "## 결과 계약"
SEAL_TOKENS = {
    "checks bullet": [FEATURE, "adapter", "A structured result contract does not require an official adapter or the reference producer."],
    # "opening line": the Seal references never use ordering words such as "first" (plugins_test.go).
    "ha.md structured results": [SCHEMA, "go-test-json", "junit-xml", "jaekit:violation=", "opening line", "CLI", "HTTP",
                                 "public API", "dedicated producer", FEATURE, PLACEHOLDER, "optional"],
    "bundle.md result contract": [SCHEMA],
    "PLAN.md result contract": [SCHEMA],
}
# Digests of the parts the goal leaves as they are (SPEC 범위 목록 1–5 only change the parts above).
KEPT_DIGESTS = {
    "SKILL.md|front matter without the version": "ad6b9465964f5726",
    "SKILL.md|## Authority": "452109bd4e2e7767",
    "SKILL.md|## The execution bundle": "673429596758904c",
    "SKILL.md|## Seal Core|": "a39c7e53fdebf56e",
    "SKILL.md|## Seal Core|- If `ha` is missing, record nothing and stop. F": "7cbc92bd3d7e1a93",
    "SKILL.md|## Seal Core|- Before creating a start record, resolve the ac": "fd641e43f6a4afbc",
    "SKILL.md|## Seal Core|- `ha start <goal> --rules run-rules/4 --request": "2acf8a5837c91a37",
    "SKILL.md|## Seal Core|- A `change` condition needs a baseline record: ": "370d52a5a8756031",
    "SKILL.md|## Seal Core|- A result counts only when `ha check` recorded ": "a717754a911e84d1",
    "SKILL.md|## Seal Core|- An `error` result may be rerun once on the sam": "36766f7dd101db04",
    "SKILL.md|## Check inputs": "7eae14e50684bc07",
    "SKILL.md|## Final candidate evidence": "a4d9f3acdabcfcc7",
    "SKILL.md|## Assurance policy": "90a5a9087c4d146e",
    "SKILL.md|## Autonomy under run-rules/4": "8d1a97bc7a80cc5f",
    "SKILL.md|## Completion": "5906b4858e5eb665",
    "SKILL.md|## Post-completion discoveries": "758f8f2c38bd0de7",
    "SKILL.md|## Decision history": "f94de79777bff37f",
    "SKILL.md|## Resuming": "81e939382794c207",
    "SKILL.md|## Budget": "76107f24f57535d3",
    "SKILL.md|## Forbidden": "4ff478e5c1d94f76",
    "SKILL.md|## Language": "40bb96487de7ae6f",
    "ha.md|## Core compatibility": "1ec979842423ab92",
    "ha.md|## Commands": "b58161f8169a10d2",
    "ha.md|## Before execution": "06e98c2c5d2905ea",
    "ha.md|## run-rules/4": "06b24f0133ae2dc2",
    "ha.md|## Budget changes": "b7cb0e6800d28451",
    "ha.md|## States": "2fdd42e0cdcf2da1",
    "ha.md|## Status output": "56822193ba2b8de3",
    "ha.md|## Reasons": "46a60601f12f0940",
    "ha.md|## What checks read": "684e7a3341bac9fd",
    "ha.md|## Freshness": "bb4583c36323b759",
    "ha.md|## Final head": "210f467c23f2b4d8",
    "bundle.md|": "7b330fa0e9519bbd",
    "bundle.md|## PLAN.md": "5225a83be0926f17",
    "bundle.md|### `## 조건표`": "88b2b92dbb262945",
    "bundle.md|### `## 범위`": "ef3b3b28fb6f2aab",
    "bundle.md|### `## Task`": "23b73ab0a3bd341a",
    "bundle.md|### `## 예산`": "eaa8ee6132e74c03",
    "bundle.md|### `## 읽을 자료` (when tasks stay inside PLAN.md)": "c5be32f788002ab1",
    "bundle.md|## Check inputs": "92f2ad8e92c996bc",
    "bundle.md|## Final candidate evidence": "69e8825a1cca979a",
    "bundle.md|## tasks/T001.md": "dea1226b81d018cb",
    "bundle.md|## REVIEW.md": "f5abf293f4cce87b",
    "bundle.md|### Evidence and observation scope": "ce4e2be677bd8d06",
    "bundle.md|## PROGRESS.md": "fa602b335c395afb",
    "bundle.md|### `## 타임라인`": "f74dbcec806d4245",
    "bundle.md|### PROGRESS with decision history": "f65211d57dc6442d",
    "bundle.md|## Assurance policy and counterexamples": "be95b14f49739942",
    "bundle.md|## Post-completion discovery documents": "94147f1264b5d55b",
    "bundle.md|## Completion report": "192822db3d2d5810",
    "PLAN.md|": "7e86968ea0f57ef0",
    "PLAN.md|## 조건표": "04de96c6e0f55056",
    "PLAN.md|## 범위": "a16f29153a87ecef",
    "PLAN.md|## Task": "71f1d2f943c534a9",
    "PLAN.md|## 예산": "59dd3fedfa34e291",
    "PLAN.md|## 읽을 자료": "680d0fcee9170a19",
    "check_core.py": "da0e6a64b55a3c2e",
}


def seal_parts():
    """{part: text} of every part of the Seal files this goal may change, split by heading (and SKILL.md's Seal Core bullets)."""
    parts = {}
    skill = text(f"{SEAL}/SKILL.md")
    for head, body in pieces(skill, r"## "):
        if head == "## Seal Core":
            for bullet, item in pieces(body, r"- "):
                parts[f"SKILL.md|{head}|{bullet[:48]}"] = item
        elif head == "":
            parts["SKILL.md|front matter without the version"] = re.sub(r'(?m)^  version: ".*"\n', "", body)
        else:
            parts[f"SKILL.md|{head}"] = body
    for head, body in pieces(text(f"{SEAL}/references/ha.md"), r"## "):
        parts[f"ha.md|{head}"] = body
    for head, body in pieces(text(f"{SEAL}/references/bundle.md"), r"#{2,3} "):
        parts[f"bundle.md|{head}"] = body
    for head, body in pieces(text(f"{SEAL}/assets/templates/PLAN.md"), r"## "):
        parts[f"PLAN.md|{head}"] = body
    parts["check_core.py"] = text(CHECK_CORE)
    return parts


CHANGED_PARTS = {f"SKILL.md|## Seal Core|{CHECKS_BULLET[:48]}", "ha.md|", f"ha.md|{HA_SECTION}",
                 f"bundle.md|{BUNDLE_SECTION}", f"PLAN.md|{TEMPLATE_SECTION}"}


class SealGuidance(unittest.TestCase):
    """AC-16: Seal guidance changes by the five listed items only."""

    def test_listed_guidance_and_nothing_else(self):
        problems = []
        parts = seal_parts()
        found = {
            "checks bullet": parts.get(f"SKILL.md|## Seal Core|{CHECKS_BULLET[:48]}", ""),
            "ha.md structured results": parts.get(f"ha.md|{HA_SECTION}", ""),
            "bundle.md result contract": parts.get(f"bundle.md|{BUNDLE_SECTION}", ""),
            "PLAN.md result contract": parts.get(f"PLAN.md|{TEMPLATE_SECTION}", ""),
        }
        for name, tokens in SEAL_TOKENS.items():
            missing = [t for t in tokens if t not in found[name]]
            if missing:
                problems.append(f"{name} lacks {missing}")
        if f"Seal {SEAL_VERSION}" not in parts.get("ha.md|", ""):
            problems.append(f"references/ha.md does not name Seal {SEAL_VERSION}")
        versions = {
            f"{SEAL}/SKILL.md": f'  version: "{SEAL_VERSION}"',
            "plugins/seal/.claude-plugin/plugin.json": f'"version": "{SEAL_VERSION}"',
            "plugins/seal/.codex-plugin/plugin.json": f'"version": "{SEAL_VERSION}"',
        }
        for path, token in versions.items():
            if token not in text(path):
                problems.append(f"{path} does not carry Seal {SEAL_VERSION}")
        try:
            market = json.loads(text(".claude-plugin/marketplace.json"))
            seal = [p for p in market.get("plugins", []) if p.get("name") == "seal"]
        except ValueError as error:
            raise Unavailable(f"marketplace.json: {error}") from None
        if [p.get("version") for p in seal] != [SEAL_VERSION]:
            problems.append(f"the marketplace seal entry is {[p.get('version') for p in seal]}")
        require(self, not problems, "seal-adapter-guidance-missing", "; ".join(problems))
        changed = [name for name, want in KEPT_DIGESTS.items() if short(parts.get(name, "")) != want]
        added = [name for name in parts if name not in KEPT_DIGESTS and name not in CHANGED_PARTS]
        require(self, not changed and not added, "seal-adapter-guidance-missing",
                f"Seal guidance outside the listed items changed: {changed + added}")


# --- AC-17 -------------------------------------------------------------------

ROUTE_SOURCE = "internal/evidence/adapter.go"
EXISTING_I2 = ("structured-classification", "stored-result-judgment", "reference-producer-to-cli",
               "seal-evidence-and-assurance-contract")


def registry_before():
    try:
        return json.loads((HERE / "regressions-before.json").read_text(encoding="utf-8"))
    except (OSError, ValueError) as error:
        raise Unavailable(f"regressions-before.json: {error}") from None


def go_selection(checks):
    """[(package directory, regex)] for Go selections of a route."""
    by_package = {}
    for check in checks:
        if check.get("group") != "go" or not check.get("file", "").endswith("_test.go"):
            continue
        by_package.setdefault("./" + str(Path(check["file"]).parent), []).append(check["selector"])
    return [(package, "^(" + "|".join(sorted(set(names))) + ")$") for package, names in sorted(by_package.items())]


class Registry(unittest.TestCase):
    """AC-17: an I2 route protects the adapters with a real Go 1.27 `go test -json` run; existing routes stay."""

    def test_adapter_route(self):
        problems = []
        try:
            registry = json.loads(text("contracts/regressions.json"))
        except ValueError as error:
            raise Unavailable(f"contracts/regressions.json: {error}") from None
        routes = [r for r in registry.get("routes", []) if ROUTE_SOURCE in r.get("sources", [])]
        require(self, len(routes) == 1, "adapter-regression-unprotected",
                f"{len(routes)} routes name {ROUTE_SOURCE} as a source")
        route = routes[0]
        if route.get("invariant") != "I2" or route.get("status") != "protected" or not route.get("checks"):
            problems.append(f"the adapter route is {route.get('invariant')}/{route.get('status')} with {len(route.get('checks', []))} checks")
        boundary = route.get("boundary", "")
        if "JUnit" not in boundary or "go test -json" not in boundary:
            problems.append("the route boundary does not say how JUnit and go test -json were observed")
        before = registry_before()
        old = {r["id"]: r for r in before["routes"]}
        new = {r["id"]: r for r in registry.get("routes", [])}
        for rid, entry in old.items():
            if new.get(rid) != entry:
                problems.append(f"existing route {rid} changed")
        if [i for i in registry.get("invariants", [])] != before["invariants"]:
            problems.append("the invariants changed")
        result = run([sys.executable, "tools/regression_map.py", "--previous", HERE / "regressions-before.json", "--format", "json"],
                     ROOT, clean_env(), timeout=600)
        try:
            report = json.loads(result.stdout)
        except ValueError:
            raise Unavailable(f"regression_map.py: {result.stderr.decode(errors='replace')[-800:]}") from None
        comparison = report.get("comparison", {})
        if result.returncode or route.get("id") not in report.get("protected", []):
            problems.append(f"regression_map.py does not report the route as protected (exit {result.returncode})")
        if comparison.get("lost_protection") or comparison.get("changed_invariants") or comparison.get("added_routes") != [route.get("id")]:
            problems.append(f"registry comparison: {comparison}")
        require(self, not problems, "adapter-regression-unprotected", "; ".join(problems))
        version = go_typed()
        real_go = shutil.which("go")
        if real_go is None:
            raise Unavailable("go is not on PATH")
        with temporary("jaekit-adapter-registry-") as directory:
            shim_dir, log = Path(directory) / "bin", Path(directory) / "go-calls.txt"
            shim_dir.mkdir()
            shim = shim_dir / "go"
            shim.write_text(f"#!/bin/sh\nprintf '%s\\n' \"$*\" >> '{log}'\nexec '{real_go}' \"$@\"\n", encoding="utf-8")
            shim.chmod(0o755)
            plain = clean_env()
            plain["GOFLAGS"] = "-mod=readonly"
            # Only the adapter route's test binaries find the logging go first on PATH (they run without
            # `go test`, which would put $GOROOT/bin first), so the log holds their nested runs.
            shimmed = dict(plain, PATH=str(shim_dir) + os.pathsep + plain.get("PATH", ""))
            failures = []
            for rid in (route.get("id"), *EXISTING_I2):
                checks = new[rid]["checks"]
                env = shimmed if rid == route.get("id") else plain
                for package, pattern in go_selection(checks):
                    if env is shimmed:
                        failed, passed = go_binary_results(package, pattern, env, timeout=1800, go=real_go)
                    else:
                        failed, passed = go_test_results([package], ["-count=1", "-run", pattern], env, timeout=1800, go=real_go)
                    ran = {name.split(" ", 1)[1].split("/")[0] for name in passed}
                    wanted = set(pattern[2:-2].split("|"))
                    if failed or not wanted <= ran:
                        failures.append(f"{rid} {package}: failed {failed[:5]}, not run {sorted(wanted - ran)}")
            try:
                calls = log.read_text(encoding="utf-8").splitlines() if log.exists() else []
            except OSError as error:
                raise Unavailable(f"go call log: {error}") from None
            nested = [c for c in calls if c.split()[:1] == ["test"] and "-json" in c.split()]
            require(self, not failures, "adapter-regression-unprotected", "; ".join(failures))
            require(self, bool(nested), "adapter-regression-unprotected",
                    f"the adapter route's regressions ran no real `go test -json` ({version}); go calls: {calls[:5]}")


# --- AC-18 -------------------------------------------------------------------

RELEASE_LINES = ['for target in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64; do',
                 'name="ha_${version}_${os}_${arch}"', 'tar -C "${build}" -czf "${dist}/${name}.tar.gz" "${name}"',
                 'GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -buildvcs=false \\',
                 'cp "${build}/windows-candidate/ha_${version}_windows_amd64.zip" "${dist}/"']
WINDOWS_PATH_TEST = {"internal/evidence": "TestNativeWindowsEvidencePaths"}


class GoVersion(unittest.TestCase):
    """AC-18: the repository requires Go 1.27; release targets, names and existing tests stay."""

    def test_go_1_27(self):
        problems = []
        mod = text("go.mod")
        lines = [line for line in mod.splitlines() if line.startswith("go ")]
        require(self, len(lines) == 1 and re.fullmatch(r"go 1\.27(\.\d+)?", lines[0]), "go-1-27-not-required",
                f"go.mod says {lines}")
        for path, token in (("guides/INSTALL.md", "Go 1.27 이상"), ("guides/INSTALL.en.md", "Go 1.27 or later")):
            content = text(path)
            if token not in content or "Go 1.26" in content:
                problems.append(f"{path} does not say {token!r} alone")
        ci, release = text(".github/workflows/ci.yml"), text(".github/workflows/release.yml")
        if ci.count("go-version-file: go.mod") != 4 or "go-version:" in ci:
            problems.append("CI does not read the Go version from go.mod in its four Go jobs")
        if "go-version-file: go.mod" not in release or "go-version:" in release:
            problems.append("the release workflow does not read the Go version from go.mod")
        missing = [line for line in RELEASE_LINES if line not in release]
        if missing:
            problems.append(f"release targets or file names changed: {missing}")
        require(self, not problems, "go-1-27-not-required", "; ".join(problems))
        version = go_typed()
        env = clean_env()
        env["GOFLAGS"] = "-mod=readonly"
        env.pop("GOPROXY", None)
        failed, _ = go_test_results(["./..."], ["-count=1"], env, timeout=3600)
        require(self, not failed, "go-1-27-not-required", f"existing Go tests fail with {version}: {failed[:10]}")
        required = windows_native_tests()
        missing = [f"{p} {t}" for p, t in WINDOWS_PATH_TEST.items() if t not in required.get(p, [])]
        require(self, not missing, "go-1-27-not-required", f"Windows native CI does not require {missing}")
        ci_run_passes()


if __name__ == "__main__":
    if sys.argv[1:] == ["--print-kept-digests"]:
        parts = seal_parts()
        print(json.dumps({name: short(body) for name, body in parts.items() if name not in CHANGED_PARTS}, indent=1,
                         ensure_ascii=False))
    else:
        unittest.main()
