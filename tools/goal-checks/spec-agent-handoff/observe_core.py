"""Observe how a Core built from this checkout reads goal documents.

`ha` is built from this checkout once per test class and run against
synthetic Git repositories in temporary directories. Nothing reads private
documents, the real HOME or global Git configuration.

A failed build, a Git or `ha` step that cannot run or ends with an exit the
fixture does not expect, unreadable product files and undecodable output raise
Unavailable: an execution error, never a violation. Only an observed property
of the built executable or of a named product file is a violation, raised as
`RequirementViolation` with one identifier per test.
"""
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[3]
sys.path.insert(0, str(ROOT / "tools"))
from result_observations import RequirementViolation  # noqa: E402

GOAL = "docs/specs/g"
TIERS = ("light", "standard", "strict")
TEMPLATES = ("small-change.md", "backend-feature.md", "cross-module-feature.md", "product.md")
OK = "ok: no lint problems"
BODY = """## 목표

`src/keep.txt` keeps its content.

## Acceptance Criteria
- **AC-1** `src/keep.txt` still contains exactly `keep`.
- **AC-2** `tests/keep.sh` still passes.

## Open Decisions

없음
"""
PLAN = """# PLAN — {name}

## 조건표
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | maintain | `sh tests/keep.sh` | — | T001 |
| AC-2 | maintain | `sh tests/keep.sh` | — | T001 |

## 범위
- 바꿀 수 있는 경로: `src/**`, `tests/**` (SPEC: 목표)
- 테스트 경로: (기본값)
- 유지할 동작: `src/keep.txt`

## Task
| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |
| --- | --- | --- | --- | --- | --- | --- |
| T001 | keep stays | AC-1, AC-2 | — | — | 예 | — |

## 예산
- 검증 실행: 50
- 경과 시간: 4시간
- 비용: 미관측
"""
PROGRESS = """# PROGRESS — {name}

## Task 상태
| Task | 상태 | 메모 |
| --- | --- | --- |
| T001 | done | |
"""
PROBLEM = re.compile(r"^([a-z_]+)\t([^\t]+)\t(.+)$")


class Unavailable(Exception):
    """A tool or fixture step could not run as required; reported as an error."""


def isolated_env(home, **extra):
    env = {key: value for key, value in os.environ.items()
           if not key.startswith(("GIT_", "HA_EVIDENCE_"))}
    env.update(HOME=str(home), GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
               GIT_AUTHOR_NAME="Fixture", GIT_AUTHOR_EMAIL="fixture@example.invalid",
               GIT_COMMITTER_NAME="Fixture", GIT_COMMITTER_EMAIL="fixture@example.invalid", LC_ALL="C")
    env.update(extra)
    return env


def read(rel):
    try:
        return (ROOT / rel).read_text(encoding="utf-8")
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"cannot read {rel}: {error}") from None


def spec(header_lines, body=BODY):
    return "# Synthetic goal\n\n" + "\n".join(header_lines) + "\n\n" + body


def goal_digest(docs):
    """The goal digest of contracts/goal-docs.md §1 for {path: text}."""
    parts = "".join(f"{path}\0{hashlib.sha256(docs[path].encode('utf-8')).hexdigest()}\n" for path in sorted(docs))
    return hashlib.sha256(parts.encode("utf-8")).hexdigest()


class Built(unittest.TestCase):
    """Builds ha once; each test gets fresh synthetic repositories."""

    @classmethod
    def setUpClass(cls):
        cls.temporary = Path(tempfile.mkdtemp(prefix="jaekit-handoff-"))
        cls.ha = cls.temporary / "ha"
        env = dict(os.environ, GOTOOLCHAIN="local", GOFLAGS="-mod=readonly", CGO_ENABLED="0")
        try:
            built = subprocess.run(["go", "build", "-o", str(cls.ha), "./cmd/ha"], cwd=ROOT, env=env,
                                   capture_output=True, text=True, timeout=600)
        except (OSError, subprocess.TimeoutExpired) as error:
            shutil.rmtree(cls.temporary, ignore_errors=True)
            raise Unavailable(f"cannot build ha: {error}") from None
        if built.returncode:
            shutil.rmtree(cls.temporary, ignore_errors=True)
            raise Unavailable("cannot build ha: " + built.stderr)

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.temporary, ignore_errors=True)

    def setUp(self):
        self.base = Path(tempfile.mkdtemp(prefix="case-", dir=self.temporary))
        self.home = self.base / "home"
        self.home.mkdir()
        self.env = isolated_env(self.home)

    def tearDown(self):
        shutil.rmtree(self.base, ignore_errors=True)

    def repo(self, goals, name="repo"):
        """A committed repository with one goal bundle per {goal dir: SPEC text}."""
        root = self.base / name
        files = {"src/keep.txt": "keep\n", "tests/keep.sh": "grep -qx keep src/keep.txt\n"}
        for goal, text in goals.items():
            short = goal.rsplit("/", 1)[-1]
            files.update({f"{goal}/SPEC.md": text, f"{goal}/PLAN.md": PLAN.format(name=short),
                          f"{goal}/REVIEW.md": f"# REVIEW — {short}\n\n## 결론\nready\n",
                          f"{goal}/PROGRESS.md": PROGRESS.format(name=short)})
        for rel, text in files.items():
            path = root / rel
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_bytes(text.encode("utf-8"))
        self.git(root, "init", "-q", "-b", "main")
        self.commit(root, "fixture")
        return root

    def git(self, root, *args):
        try:
            result = subprocess.run(["git", *args], cwd=root, env=self.env, capture_output=True, text=True, timeout=60)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Unavailable(f"git {args[0]}: {error}") from None
        if result.returncode:
            raise Unavailable(f"git {' '.join(args)}: {result.stderr}")
        return result.stdout

    def commit(self, root, message):
        self.git(root, "add", "-A")
        self.git(root, "commit", "-q", "-m", message)

    def ha_run(self, root, *args, expect=None, cwd=None, env=None):
        try:
            result = subprocess.run([str(self.ha), *args], cwd=cwd or root, env=env or self.env,
                                    capture_output=True, text=True, timeout=300)
        except (OSError, subprocess.TimeoutExpired) as error:
            raise Unavailable(f"ha {args[0]}: {error}") from None
        if expect is not None and result.returncode not in expect:
            raise Unavailable(f"ha {' '.join(args)} exited {result.returncode}: {result.stdout}{result.stderr}")
        return result

    def lint(self, root, goal=GOAL, **kwargs):
        return self.ha_run(root, "lint", goal, expect=(0, 1), **kwargs)

    def status_json(self, root, goal=GOAL, command="status"):
        result = self.ha_run(root, command, goal, "--format", "json", expect=(0, 1, 2, 3, 4))
        try:
            return json.loads(result.stdout)
        except ValueError:
            raise Unavailable("status output is not JSON: " + result.stdout) from None

    def problems(self, result):
        found = []
        for line in result.stdout.splitlines():
            match = PROBLEM.match(line)
            if match:
                found.append(match.groups())
        return found


def lint_tier(result):
    """The tier `ha lint` printed after its success line, or None."""
    lines = result.stdout.splitlines()
    if result.returncode == 0 and lines[:1] == [OK] and len(lines) == 2 and lines[1].startswith("assurance: "):
        return lines[1][len("assurance: "):]
    return None


class AC11(Built):
    def test_valid_tiers_are_read(self):
        for tier in TIERS:
            root = self.repo({GOAL: spec(["Status: Ready", "Criteria-Format: nested/1", f"Assurance: {tier}"])}, name=tier)
            result = self.lint(root)
            if lint_tier(result) != tier:
                raise RequirementViolation("assurance-not-read", f"{tier}: {result.stdout}")

    def test_trailing_space_and_crlf(self):
        variants = {"space": spec(["Status: Ready", "Assurance: strict \t "]),
                    "crlf": spec(["Status: Ready", "Assurance: strict"]).replace("\n", "\r\n")}
        for name, text in variants.items():
            result = self.lint(self.repo({GOAL: text}, name=name))
            if lint_tier(result) != "strict":
                raise RequirementViolation("assurance-not-read", f"{name}: {result.stdout}")

    def test_absent_line_reads_no_tier(self):
        result = self.lint(self.repo({GOAL: spec(["Status: Ready", "Criteria-Format: nested/1"])}))
        if result.returncode != 0 or result.stdout != OK + "\n":
            raise RequirementViolation("absent-tier-misread", result.stdout)

    def test_fenced_lines_are_not_read(self):
        header = ["Status: Ready", "Assurance: light", "", "````markdown", "Assurance: Strict", "```",
                  "Assurance: unknown", "```` not a close", "Assurance: high", "````", "",
                  "~~~", "  Assurance: strict", "Assurance: strict", "~~~"]
        body = BODY.replace("## Open Decisions", "```text\nAssurance: none\n```\n\n## Open Decisions")
        result = self.lint(self.repo({GOAL: spec(header, body)}))
        if lint_tier(result) != "light":
            raise RequirementViolation("assurance-not-read", result.stdout)

    def test_independent_of_directory_time_and_environment(self):
        root = self.repo({GOAL: spec(["Status: Ready", "Assurance: standard"])})
        outputs = set()
        for cwd, goal in ((root, GOAL), (root / "src", "../" + GOAL), (root / "tests", str(root / GOAL))):
            for extra in ({"TZ": "UTC"}, {"TZ": "Asia/Seoul", "LANG": "ko_KR.UTF-8"}, {"HA_UNRELATED": "1"}):
                result = self.lint(root, goal=goal, cwd=cwd, env=isolated_env(self.home, **extra))
                outputs.add((result.returncode, result.stdout))
        if len(outputs) != 1:
            raise RequirementViolation("assurance-not-read", repr(outputs))
        code, stdout = outputs.pop()
        if lint_tier(subprocess.CompletedProcess([], code, stdout, "")) != "standard":
            raise RequirementViolation("assurance-not-read", stdout)


class AC12(Built):
    def expect_problem(self, header, code, line, extra_codes=()):
        result = self.lint(self.repo({GOAL: spec(header)}, name=f"case-{len(list(self.base.iterdir()))}"))
        found = self.problems(result)
        wanted = (code, f"{GOAL}/SPEC.md:{line}")
        if result.returncode != 1 or not any((c, where) == wanted and detail.strip() for c, where, detail in found):
            raise RequirementViolation("assurance-error-unreported", f"{header}: {result.stdout}")
        if not set(extra_codes) <= {c for c, _, _ in found}:
            raise RequirementViolation("assurance-error-unreported", f"{header}: {result.stdout}")

    # The synthetic SPEC starts with a title and a blank line, so header
    # line i of the list is line i + 3 of SPEC.md.
    def test_values_outside_the_three(self):
        for value in ("Light", "", "high", "STRICT", "light # comment"):
            self.expect_problem(["Status: Ready", f"Assurance: {value}".rstrip()], "assurance_format", 4)

    def test_indented_before_the_first_section(self):
        for indent in ("  ", "\t", " "):
            self.expect_problem(["Status: Ready", indent + "Assurance: light"], "assurance_format", 4)

    def test_duplicate_line(self):
        self.expect_problem(["Status: Ready", "Assurance: light", "Assurance: light"], "assurance_format_duplicate", 5)

    def test_line_after_the_first_section(self):
        text = spec(["Status: Ready"], BODY.replace("## Acceptance Criteria", "Assurance: strict\n\n## Acceptance Criteria"))
        result = self.lint(self.repo({GOAL: text}))
        line = text.split("\n").index("Assurance: strict") + 1
        if result.returncode != 1 or not any(c == "assurance_format" and where == f"{GOAL}/SPEC.md:{line}"
                                             for c, where, _ in self.problems(result)):
            raise RequirementViolation("assurance-error-unreported", result.stdout)

    def test_criteria_format_error_reported_too(self):
        self.expect_problem(["Status: Ready", "Criteria-Format: nested/2", "Assurance: Light"], "assurance_format", 5,
                            extra_codes=("criteria_format",))

    def test_start_refuses(self):
        root = self.repo({GOAL: spec(["Status: Ready", "Assurance: maximum"])})
        result = self.ha_run(root, "start", GOAL, "--request", "synthetic")
        if result.returncode == 0:
            raise RequirementViolation("assurance-error-unreported", "start accepted an invalid Assurance line")
        if result.returncode != 65 or (root / GOAL / "runs.jsonl").exists():
            raise Unavailable(f"start ended {result.returncode}: {result.stdout}{result.stderr}")

    def test_codes_documented(self):
        for rel in ("contracts/goal-docs.md", "contracts/bundle.md"):
            text = read(rel)
            if "`assurance_format`" not in text or "`assurance_format_duplicate`" not in text:
                raise RequirementViolation("assurance-codes-undocumented", rel)


class AC13(Built):
    def capabilities(self):
        result = self.ha_run(self.base, "capabilities", "--format", "json", expect=(0,))
        try:
            return json.loads(result.stdout)
        except ValueError:
            raise Unavailable("capabilities output is not JSON") from None

    def test_capabilities_declare_the_tiers(self):
        report = self.capabilities()
        if report.get("formats", {}).get("assurance") != list(TIERS):
            raise RequirementViolation("assurance-capability-missing", json.dumps(report.get("formats")))

    def test_contract_describes_the_entry(self):
        text = read("contracts/core-capabilities.md")
        if "`formats.assurance`" not in text or "요구에 더하지 않는다" not in text:
            raise RequirementViolation("assurance-capability-undocumented", "contracts/core-capabilities.md")

    def test_seal_requirements_do_not_add_it(self):
        report = self.capabilities()
        report.get("formats", {}).pop("assurance", None)
        fake = self.base / "fake-ha"
        fake.write_text("#!/bin/sh\ncat <<'JSON'\n" + json.dumps(report) + "\nJSON\n", encoding="utf-8")
        fake.chmod(0o755)
        helper = ROOT / "plugins/seal/skills/seal/scripts/check_core.py"
        for executable in (fake, self.ha):
            try:
                result = subprocess.run([sys.executable, str(helper), "--ha", str(executable), "--operation", "start"],
                                        cwd=self.base, env=self.env, capture_output=True, text=True, timeout=120)
            except (OSError, subprocess.TimeoutExpired) as error:
                raise Unavailable(f"check_core.py: {error}") from None
            if result.returncode == 2:
                raise RequirementViolation("seal-requirement-added", result.stdout)
            if result.returncode != 0:
                raise Unavailable(f"check_core.py exited {result.returncode}: {result.stdout}{result.stderr}")


def header_of(text):
    """Lines before the first level-two heading."""
    out = []
    for line in text.split("\n"):
        if line.startswith("## "):
            break
        out.append(line)
    return out


def fenced_examples(text):
    return re.findall(r"^```markdown\n(.*?)\n```$", text, re.M | re.S)


class AC14(Built):
    def read_header(self, lines, where):
        root = self.repo({GOAL: "\n".join(lines).rstrip("\n") + "\n\n" + BODY}, name=f"h{len(list(self.base.iterdir()))}")
        written = [line[len("Assurance: "):] for line in lines if line.startswith("Assurance: ")]
        result = self.lint(root)
        if len(written) != 1 or lint_tier(result) != written[0]:
            raise RequirementViolation("header-not-read", f"{where}: {written} {result.stdout}")

    def test_template_headers(self):
        for name in TEMPLATES:
            text = read("plugins/spec/skills/spec/assets/templates/" + name)
            self.read_header(header_of(text), name)

    def test_contract_example_headers(self):
        seen = {"plain": False, "agent": False}
        for rel in ("contracts/goal-docs.md", "plugins/spec/skills/spec/references/goal-docs.md"):
            for example in fenced_examples(read(rel)):
                header = header_of(example)
                if not any(line.startswith("Assurance:") for line in header):
                    continue
                if rel == "contracts/goal-docs.md":
                    seen["agent" if "Proposed-By: agent" in header else "plain"] = True
                self.read_header(header, rel)
        if not all(seen.values()):
            raise RequirementViolation("header-not-read", f"contract examples with an Assurance line: {seen}")


def goal_names(*goals):
    """A goal path as status prints it and as its log directory escapes it."""
    return [form for goal in goals for form in (goal, goal.replace("/", "%2F"))]


def normalized(value, names):
    """Replace identities that differ only because the goal path or bytes differ."""
    if isinstance(value, dict):
        return {k: ("<v>" if k in ("sha256", "spec_digest", "commit", "elapsed_seconds", "at") else normalized(v, names))
                for k, v in value.items()}
    if isinstance(value, list):
        return [normalized(v, names) for v in value]
    if isinstance(value, str):
        for name in names:
            value = value.replace(name, "<goal>")
        return re.sub(r"\b[0-9a-f]{12,64}\b", "<hex>", value)
    return value


def normalized_text(text, names):
    for name in names:
        text = text.replace(name, "<goal>")
    text = re.sub(r"\b[0-9a-f]{12,64}\b", "<hex>", text)
    return re.sub(r"\d+/(\d+) seconds", r"N/\1 seconds", text)


class Preserved(Built):
    """AC-17: a goal without the line reads as before, and the line changes no judgment."""

    def test_no_line_output_and_digest(self):
        text = spec(["Status: Ready", "Criteria-Format: nested/1"])
        root = self.repo({GOAL: text})
        result = self.lint(root)
        if result.returncode != 0 or result.stdout != OK + "\n":
            raise RequirementViolation("no-line-goal-changed", result.stdout)
        self.ha_run(root, "start", GOAL, "--request", "synthetic", expect=(0,))
        if self.status_json(root).get("spec_digest") != goal_digest({f"{GOAL}/SPEC.md": text}):
            raise RequirementViolation("no-line-goal-changed", "the goal digest no longer follows contracts/goal-docs.md §1")

    def test_tier_changes_no_judgment(self):
        plain, tiered = "docs/specs/a", "docs/specs/b"
        for rules in ((), ("--rules", "run-rules/4")):
            root = self.repo({plain: spec(["Status: Ready"]), tiered: spec(["Status: Ready", "Assurance: strict"])},
                             name="rules4" if rules else "rules3")
            outputs = {}
            for goal in (plain, tiered):
                self.ha_run(root, "start", goal, "--request", "synthetic", *rules, expect=(0,))
                self.ha_run(root, "check", goal, expect=(0,))
            for goal in (plain, tiered):
                names = goal_names(plain, tiered)
                outputs[goal] = [normalized(self.status_json(root, goal), names),
                                 normalized_text(self.ha_run(root, "status", goal, expect=(0,)).stdout, names),
                                 normalized(self.status_json(root, goal, "done"), names),
                                 normalized_text(self.ha_run(root, "done", goal, expect=(0,)).stdout, names)]
            if outputs[plain] != outputs[tiered]:
                raise RequirementViolation("tier-changes-judgment", json.dumps(outputs, ensure_ascii=False)[:4000])


class AC22(Built):
    AMENDMENT = "amendments/wording.md"

    def amendment_text(self):
        return "# 수정안\n\n## 계기\n\n검토 지적 F1\n\n## 바꿀 곳\n\nAC-1\n\n## 바뀌기 전\n\n가\n\n## 바뀐 뒤\n\n나\n"

    def test_linked_amendment_stays_outside_the_goal(self):
        text = spec(["Status: Ready"], BODY.replace("## Open Decisions", f"수정안: [wording]({self.AMENDMENT})\n\n## Open Decisions"))
        root = self.repo({GOAL: text})
        result = self.lint(root)
        if result.returncode != 0:
            raise RequirementViolation("amendment-in-goal-set", result.stdout)
        self.ha_run(root, "start", GOAL, "--request", "synthetic", expect=(0,))
        expected = goal_digest({f"{GOAL}/SPEC.md": text})
        path = root / GOAL / self.AMENDMENT
        path.parent.mkdir(parents=True)
        for step in ("written", "committed"):
            if step == "written":
                path.write_text(self.amendment_text(), encoding="utf-8")
            else:
                self.commit(root, "amendment")
            report = self.status_json(root)
            reasons = {r.get("reason") if isinstance(r, dict) else r for r in report.get("reasons", [])}
            if report.get("spec_digest") != expected or "spec_changed" in reasons:
                raise RequirementViolation("amendment-in-goal-set", f"{step}: {report.get('spec_digest')} {reasons}")

    def code_state(self, rules):
        root = self.repo({GOAL: spec(["Status: Ready"])}, name=rules.replace("/", "-"))
        extra = ("--rules", rules) if rules == "run-rules/4" else ()
        self.ha_run(root, "start", GOAL, "--request", "synthetic", *extra, expect=(0,))
        if rules in ("run-rules/1", "run-rules/2"):
            records = root / GOAL / "runs.jsonl"
            raw = records.read_bytes()
            if raw.count(b"\n") != 1 or raw.count(b'"rules":"run-rules/3"') != 1:
                raise Unavailable("unexpected start record")
            # The start line is the last line, so the chain stays intact.
            records.write_bytes(raw.replace(b'"rules":"run-rules/3"', b'"rules":"' + rules.encode() + b'"'))
        self.ha_run(root, "check", GOAL, expect=(0,))

        def view():
            report = self.status_json(root)
            if report.get("rules") != rules:
                raise Unavailable(f"goal runs under {report.get('rules')}, not {rules}")
            return (report.get("status"), json.dumps(report.get("reasons")), report.get("spec_digest"),
                    [(c.get("id"), c.get("satisfied"), c.get("reason")) for c in report.get("criteria", [])])

        before = view()
        path = root / GOAL / self.AMENDMENT
        path.parent.mkdir(parents=True)
        path.write_text(self.amendment_text(), encoding="utf-8")
        if view() != before:
            raise RequirementViolation("amendment-in-code-state", f"untracked: {view()} != {before}")
        checked = self.ha_run(root, "check", GOAL, "AC-2")
        if checked.returncode != 0:
            raise RequirementViolation("amendment-in-code-state", "check refused or failed: " + checked.stdout + checked.stderr)
        before = view()
        self.commit(root, "amendment")
        if view() != before:
            raise RequirementViolation("amendment-in-code-state", f"committed: {view()} != {before}")
        path.unlink()
        self.commit(root, "drop amendment")
        if view() != before:
            raise RequirementViolation("amendment-in-code-state", f"deleted: {view()} != {before}")

    def test_code_state_rules_1(self):
        self.code_state("run-rules/1")

    def test_code_state_rules_2(self):
        self.code_state("run-rules/2")

    def test_code_state_rules_3(self):
        self.code_state("run-rules/3")

    def test_code_state_rules_4(self):
        self.code_state("run-rules/4")

    def test_location_documented(self):
        goal_docs, run_record = read("contracts/goal-docs.md"), read("contracts/run-record.md")
        if ("`amendments/`" not in goal_docs or "목표 문서 집합에 들지 않는다" not in goal_docs
                or "`amendments/**`" not in run_record):
            raise RequirementViolation("amendment-location-undocumented", "contracts")


if __name__ == "__main__":
    unittest.main()
