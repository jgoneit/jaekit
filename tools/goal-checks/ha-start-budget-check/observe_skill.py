"""Skill Core requirements checked by `ha start --skill` (AC-11, AC-12).

Each test builds `ha` from the checked tree and starts synthetic goals with
synthetic skill folders made outside the synthetic repository
(budget_common.Repo.skill). It reads `ha`'s exit codes and output, the
synthetic runs.jsonl, and `ha capabilities`. AC-12 also runs Seal's
compatibility helper from the checked tree after confirming it is the file
Seal 0.1.17 to 0.1.19 ship (its digest is written below).
"""
import hashlib
import json
import os
from pathlib import Path
import re
import sys
import unittest

sys.dont_write_bytecode = True
sys.path.insert(0, str(Path(__file__).resolve().parent))
from budget_common import (CHECK_FLAG, DECLARATION, DECLARATION_SCHEMA, FEATURES, FORMAT_KEY, GOAL, ROOT, Repo,
                           Unavailable, build, capabilities, clean_env, require, run, temporary)


def declaration(**fields):
    return {"schema": DECLARATION_SCHEMA, **fields}


MALFORMED = [
    ("not-json", "schema = core-requirements/v1\n"),
    ("other-schema", json.dumps({"schema": "core-requirements/v2"})),
    ("no-schema", json.dumps({"features": ["estimate/v1"]})),
    ("unknown-member", json.dumps(declaration(options=["x"]))),
    ("duplicate-key", '{"schema": "core-requirements/v1", "features": ["estimate/v1"], "features": []}'),
    ("empty-string", json.dumps(declaration(features=[""]))),
    ("repeated-value", json.dumps(declaration(features=["estimate/v1", "estimate/v1"]))),
    ("null", json.dumps(declaration(features=None))),
    ("wrong-type", json.dumps(declaration(rules="run-rules/4"))),
    ("formats-list", json.dumps(declaration(formats=["criteria"]))),
    ("formats-value", json.dumps(declaration(formats={"criteria": "nested/1"}))),
    ("trailing", json.dumps(declaration()) + " {}"),
    ("array", "[]"),
]


# check_core.py reasons that say the helper could not query the executable at all
# (missing, too slow, failed to run, replaced while read) rather than how this ha
# answered. They are input failures, never violations. Its verdict on the records this
# ha wrote, such as goal_unreadable, is an observation.
HELPER_INPUT_FAILURES = frozenset({"core_missing", "query_timeout", "query_failed", "binary_changed"})


class SkillCheck(unittest.TestCase):
    """AC-11: a declared requirement this ha lacks, or an unusable declaration, refuses the start without a record."""

    def test_declared_skill(self):
        with temporary("jaekit-skill-check-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            caps = capabilities(ha, directory)
            version = caps.get("ha_version")
            repo = Repo(Path(directory) / "goal", ha, changes=1, runs="60")
            missing = repo.skill("needs-missing", declaration(features=["no-such-feature/v1"]))
            code, out, err = repo.start("--skill", str(missing))
            require(self, code == 65 and not repo.records_path().exists() and "features no-such-feature/v1" in err,
                    "skill-check-missing",
                    f"a skill needing an unsupported feature: exit {code}, records {repo.records_path().exists()}: {(out + err)[-600:]}")
            problems = []
            if f"(ha {version})" not in err:
                problems.append(f"the refusal does not name ha {version}: {err[-300:]}")
            repo.reset_records()
            many = repo.skill("needs-many", declaration(rules=["run-rules/9"], formats={"criteria": ["nested/9"], "nope": ["x/v1"]},
                                                        features=["estimate/v1", "later/v2"]))
            code, out, err = repo.start("--skill", str(many))
            lines = [l for l in err.splitlines() if "missing Core support" in l]
            for item in ("rules run-rules/9", "formats.criteria nested/9", "formats.nope x/v1", "features later/v2"):
                if not any(item in l and f"(ha {version})" in l for l in lines):
                    problems.append(f"no line names {item!r} with the version: {err[-600:]}")
            if code != 65 or repo.records_path().exists() or any("estimate/v1" in l for l in lines):
                problems.append(f"several missing items: exit {code}, lines {lines}")
            repo.reset_records()
            for name, raw in MALFORMED:
                skill = repo.skill("bad-" + name, raw=raw)
                code, out, err = repo.start("--skill", str(skill))
                if code != 65 or repo.records_path().exists() or str(skill.parent / DECLARATION) not in err:
                    problems.append(f"malformed {name}: exit {code}: {err[-300:]}")
                repo.reset_records()
            folder = repo.skill("declaration-folder")
            (folder.parent / DECLARATION).mkdir()
            code, out, err = repo.start("--skill", str(folder))
            if code != 65 or repo.records_path().exists():
                problems.append(f"a folder named {DECLARATION}: exit {code}: {err[-300:]}")
            repo.reset_records()
            if os.name != "nt":
                dangling = repo.skill("declaration-dangling")
                (dangling.parent / DECLARATION).symlink_to(dangling.parent / "moved-away.json")
                code, out, err = repo.start("--skill", str(dangling))
                if code != 65 or repo.records_path().exists():
                    problems.append(f"a {DECLARATION} symlink to nothing: exit {code}: {err[-300:]}")
                repo.reset_records()
            # The capability list names this goal's additions, which skills may declare.
            if (caps.get("formats") or {}).get(FORMAT_KEY) != [DECLARATION_SCHEMA] or \
                    caps.get("features", [])[-len(FEATURES):] != list(FEATURES):
                problems.append(f"capabilities lack formats.{FORMAT_KEY} or end without {FEATURES}: {caps}")
            # --check-skill makes the declaration required; without --skill it is a usage error.
            plain = repo.skill("plain")
            code, out, err = repo.start("--skill", str(plain), CHECK_FLAG)
            if code != 65 or repo.records_path().exists() or DECLARATION not in err:
                problems.append(f"{CHECK_FLAG} without a declaration: exit {code}: {err[-300:]}")
            repo.reset_records()
            code, out, err = repo.start(CHECK_FLAG)
            if code != 64 or repo.records_path().exists():
                problems.append(f"{CHECK_FLAG} without --skill: exit {code}: {err[-300:]}")
            repo.reset_records()
            # Declaring only what this ha supports starts, with and without the flag.
            full = declaration(rules=caps["supported_rules"], formats=caps["formats"], features=caps["features"])
            for name, extra in (("supported", ()), ("supported-flag", (CHECK_FLAG,)), ("schema-only", ())):
                skill = repo.skill(name, full if name != "schema-only" else declaration())
                code, out, err = repo.start("--skill", str(skill), *extra)
                records = repo.records()
                stored = (records[0].get("skill") if records else None) or {}
                if code != 0 or stored.get("name") != name or stored.get("version") != "1.2.3":
                    problems.append(f"{name}: exit {code}, skill {stored}: {(out + err)[-300:]}")
                repo.reset_records()
        require(self, not problems, "skill-check-missing", "; ".join(problems))


# The capability fields and values before this goal, in order (ha built at the base commit).
KEPT = {
    "schema": "ha-capabilities/v1",
    "supported_rules": ["run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4"],
    "default_rules": "run-rules/3",
    "formats": {"assurance_tier": ["light", "standard", "strict"],
                "check_declaration": ["check-declaration/v1", "check-adapter/v1"],
                "check_evidence": ["check-evidence/v1"], "check_result": ["check-result/v1"],
                "criteria": ["legacy", "nested/1"], "decision_history": ["decision-history/v1"],
                "finding": ["run-finding/v1"], "timing": ["timing/v1"]},
    "features": ["estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1",
                 "budget-change/v1", "post-completion-findings/v1", "decision-history/v1", "run-timing/v1",
                 "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1",
                 "command-time-limits/v1", "runner-adapters/v1"],
}
CHECK_CORE = "plugins/seal/skills/seal/scripts/check_core.py"
# The helper shipped unchanged in Seal 0.1.17, 0.1.18 and 0.1.19 (REVIEW 가정 A6).
CHECK_CORE_SHA256 = "da0e6a64b55a3c2e2fcd3a5daebd167c7b8aa8197a907d2e54210c705b9d6b37"


class Kept(unittest.TestCase):
    """AC-12: skills without a declaration and starts without --skill behave as before; capabilities only add.

    Each part is its own test on its own synthetic goal, so a part that cannot be
    observed ends as an error of that test alone and never hides or decides another.
    """

    def test_undeclared_skill(self):
        with temporary("jaekit-skill-kept-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            repo = Repo(Path(directory) / "goal", ha, changes=1, runs="60")
            code, out, err = repo.start("--skill", str(repo.skill("plain")))
            stored = started_skill(repo)
            require(self, code == 0 and stored.get("name") == "plain" and stored.get("version") == "1.2.3"
                    and re.fullmatch(r"[0-9a-f]{64}", stored.get("digest", "")) is not None,
                    "undeclared-skill-changed", f"undeclared skill: exit {code}, skill {stored}: {(out + err)[-300:]}")

    def test_start_without_skill(self):
        with temporary("jaekit-skill-bare-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            repo = Repo(Path(directory) / "goal", ha, changes=1, runs="60")
            code, out, err = repo.start()
            records = repo.records()
            require(self, code == 0 and (records[0].get("skill") if records else None) == {"name": "", "version": "", "digest": ""},
                    "bare-start-changed", f"start without --skill: exit {code}: {(out + err)[-300:]}")

    def test_capabilities_only_add(self):
        problems = []
        with temporary("jaekit-skill-caps-") as directory:
            caps = capabilities(build(Path(directory) / "bin" / "ha"), directory)
        for key in ("schema", "supported_rules", "default_rules"):
            if caps.get(key) != KEPT[key]:
                problems.append(f"{key}: {caps.get(key)}")
        for key, values in KEPT["formats"].items():
            if (caps.get("formats") or {}).get(key) != values:
                problems.append(f"formats.{key}: {(caps.get('formats') or {}).get(key)}")
        features = caps.get("features") or []
        if features[:len(KEPT["features"])] != KEPT["features"]:
            problems.append(f"features no longer start with the earlier list: {features}")
        require(self, not problems, "capabilities-changed", "; ".join(problems))

    def test_seal_helper(self):
        try:
            digest = hashlib.sha256((ROOT / CHECK_CORE).read_bytes()).hexdigest()
        except OSError as error:
            raise Unavailable(f"{CHECK_CORE}: {error}") from None
        if digest != CHECK_CORE_SHA256:
            # The in-tree helper stands in for the installed Seal 0.1.17-0.1.19 ones; another
            # file cannot observe how they treat this ha.
            raise Unavailable(f"{CHECK_CORE} is not the Seal 0.1.17-0.1.19 helper (sha256 {digest})")
        with temporary("jaekit-skill-helper-") as directory:
            ha = build(Path(directory) / "bin" / "ha")
            repo = Repo(Path(directory) / "goal", ha, changes=1, runs="60")
            # The helper's resume, check and budget operations read the records this start
            # writes. A start that cannot be observed raises Unavailable here, before the
            # helper runs; one that ends without its record is this test's observed result.
            code, out, err = repo.start("--skill", str(repo.skill("plain")))
            require(self, code == 0 and started_skill(repo).get("name") == "plain", "seal-helper-refuses",
                    f"the start the helper reads: exit {code}: {(out + err)[-300:]}")
            # Each operation is a subtest: one the helper cannot query is an error of that
            # operation and keeps the others' verdicts.
            for operation in ("start", "resume", "check", "budget"):
                with self.subTest(operation=operation):
                    args = [sys.executable, ROOT / CHECK_CORE, "--ha", ha, "--operation", operation]
                    if operation != "start":
                        args += ["--goal", GOAL]
                    result = run(args, repo.root, repo.env, timeout=120)
                    try:
                        report = json.loads(result.stdout)
                    except ValueError:
                        report = None
                    if not isinstance(report, dict):
                        raise Unavailable(f"check_core.py {operation}: {result.stderr.decode(errors='replace')}")
                    if not report.get("compatible") and report.get("reason") in HELPER_INPUT_FAILURES:
                        raise Unavailable(f"check_core.py {operation} could not query this ha: {report}")
                    require(self, result.returncode == 0 and bool(report.get("compatible")), "seal-helper-refuses",
                            f"Seal's {operation} check: {report}")


def started_skill(repo):
    records = repo.records()
    return (records[0].get("skill") if records else None) or {}


if __name__ == "__main__":
    unittest.main()
