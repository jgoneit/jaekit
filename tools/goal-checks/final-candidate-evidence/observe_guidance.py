"""Seal's guidance for final-candidate evidence, in the checked tree.

Each criterion names the documents and sections that must state it and the
phrases that make the statement there, so a phrase elsewhere in the file does
not count. A section runs from its heading line to the next heading of the
same or a higher level. Versions come from the checked tree's skill and
manifests. Nothing reads a commit, ref or history, so the baseline on the
base commit and a check after the change judge the same property.

A missing document or section is an observed violation. Violations come only
from explicit `[violation:...]` failures; a file that cannot be read as text
raises Unavailable, an execution error.
"""
import json
from pathlib import Path
import re
import unittest

ROOT = Path(__file__).resolve().parents[3]
SEAL = "plugins/seal/skills/seal/"
SKILL = SEAL + "SKILL.md"
BUNDLE = SEAL + "references/bundle.md"
HA = SEAL + "references/ha.md"
PLAN = SEAL + "assets/templates/PLAN.md"
PROGRESS = SEAL + "assets/templates/PROGRESS.md"
REVIEW = SEAL + "assets/templates/REVIEW.md"
CONTRACT = "contracts/bundle.md"
# The Seal source version before this goal (SPEC 배경); AC-7 needs a later one.
EARLIER_SEAL = (0, 1, 14)

SECTIONS = {
    "skill": (SKILL, "## Final candidate evidence"),
    "skill-completion": (SKILL, "## Completion"),
    "skill-discoveries": (SKILL, "## Post-completion discoveries"),
    "skill-execution-bundle": (SKILL, "## The execution bundle"),
    "skill-check-inputs": (SKILL, "## Check inputs"),
    "skill-resuming": (SKILL, "## Resuming"),
    "bundle-check-inputs": (BUNDLE, "## Check inputs"),
    "bundle": (BUNDLE, "## Final candidate evidence"),
    "bundle-report": (BUNDLE, "## Completion report"),
    "bundle-discoveries": (BUNDLE, "## Post-completion discovery documents"),
    "ha": (HA, "## Final head"),
    "ha-commands": (HA, "## Commands"),
    "plan": (PLAN, "## 조건표"),
    "progress-report": (PROGRESS, "## 완료 보고"),
    "progress-discoveries": (PROGRESS, "## 완료 뒤 발견"),
    "contract": (CONTRACT, "### 2.1.3 최종 후보 근거"),
    "contract-discoveries": (CONTRACT, "### 5.2 완료 뒤 발견"),
    "contract-report": (CONTRACT, "## 6. 완료 보고"),
    "contract-check-inputs": (CONTRACT, "### 2.1.2 검사 입력과 관측"),
}

# {criterion: [(section, phrases that must all be in it)]}
CLAUSES = {
    "AC-1": [
        ("skill", ("narrow checks", "expensive full verification", "recorded once with `ha check` on the final candidate",
                   "`PLAN.md`", "once for development and once more for the record on the same candidate",
                   "a cost to avoid")),
        ("bundle", ("narrow checks", "not evidence", "recorded once with `ha check` on the final candidate",
                    "`## 조건표`", "once for development and once more for the record on the same candidate",
                    "a cost to avoid")),
        ("plan", ("narrow checks while exploring", "expensive full verification once, with `ha check` on the final candidate",
                  "once for development and once more for the record on the same candidate", "a cost to avoid")),
        ("contract", ("좁은 검사", "근거가 아니다", "최종 후보에서 `ha check`로 한 번", "PLAN에 적는다",
                      "개발용으로 전체 검증을 돌린 뒤 기록용으로 다시 돌리는 것", "피할 비용")),
    ],
    "AC-2": [
        ("skill", ("several required conditions need the same full regression", "recommended",
                   "one condition own it", "observe its own property", "not a required procedure",
                   "does not drop or weaken any condition")),
        ("bundle", ("its own process", "several required conditions need the same full regression",
                    "recommended", "one condition owns it", "observes its own property", "not a required procedure",
                    "does not drop or weaken a condition")),
        ("ha", ("its own process", "same full regression", "recommended", "one condition owns it",
                "their own property", "not a required procedure")),
        ("plan", ("same full regression", "recommended", "one condition owns it", "their own property",
                  "does not drop or weaken a condition")),
        ("contract", ("조건마다 명령을 따로 실행한다", "같은 전체 회귀", "한 조건이 그 회귀를 맡고", "자기 성질을 관측하도록 권한다",
                      "강제 절차가 아니다", "삭제하거나 약화하는 것이 아니며")),
    ],
    "AC-3": [
        ("skill-completion", ("a user's request to apply remote review or pull request comments is a rework request",
                              "Record it with `reopen`, quoting that request",
                              "re-record on the final head every required check the change made stale",
                              "record completion again with `ha done` before the head is sent or merged")),
        ("bundle", ("Only the user's request to apply them is rework authority", "quote it in `reopen`",
                    "re-record on the final head the required checks that became stale",
                    "record `ha done` again before sending or merging")),
        ("ha", ("When the user asks, after completion, to apply remote review or pull request comments",
                "`ha note <goal> reopen --quote",
                "a baseline stays fresh while it ran on the start's base commit and its check files equal HEAD's",
                "Re-record the stale required checks on the final head",
                "run `ha done` again before the head is sent or merged")),
        ("contract-report", ("사용자가 원격 리뷰나 PR 지적의 반영을 요청하면 그것이 재작업 요청이다", "그 요청을 인용해 `reopen`을 기록",
                             "최종 head에서 낡은 필수 검사를 다시 기록",
                             "보내거나 병합하기 전에 `ha done`으로 완료를 다시 기록")),
    ],
    "AC-4": [
        ("skill-completion", ("For goals started with Seal 0.1.15 or later", "completion record's seq and commit",
                              "whether the head to be sent or merged is that commit",
                              "the difference is not covered by the completion",
                              "Goals started with an earlier Seal keep the report format they started with")),
        ("bundle-report", ("For goals started with Seal 0.1.15 or later", "completion record's seq and commit",
                           "whether the head to be sent or merged is that commit",
                           "the difference is not covered by the completion",
                           "Goals started with an earlier Seal keep the report format they started with")),
        ("progress-report", ("for goals started with seal 0.1.15 or later", "completion record's seq and commit",
                             "whether the head to be sent or merged is that commit",
                             "the difference is not covered by the completion")),
        ("contract-report", ("Seal 0.1.15 이상으로 시작한 목표", "완료 기록의 seq와 commit", "보내거나 병합하려는 head가 그 commit과 같은지",
                             "그 차이는 완료가 덮지 않는다", "그 전 Seal로 시작한 목표는 시작할 때의 보고 형식을 유지한다")),
    ],
    "AC-5": [
        ("skill-discoveries", ("without the user's request", "not authority to change the completed goal's code",
                               "as post-completion discoveries", "report them")),
        ("bundle-discoveries", ("without the user's request", "not rework authority", "Record them in this format",
                                "report them")),
        ("progress-discoveries", ("without the user's request", "not authority to change the completed goal's code")),
        ("contract-discoveries", ("사용자 요청 없이 받은 리뷰·PR 지적", "완료 목표의 코드를 고칠 권한이 아니다",
                                  "완료 뒤 발견으로 남기고 보고한다")),
    ],
}

# What stays as it is (AC-6): the rework rule and the report and template formats.
KEPT = [
    ("skill-completion", ("take up the goal again only when the user asks for rework, and record that request with `reopen`",
                          "Paste that output unchanged into the completion report",
                          "It always states assurance `local` and that the executor wrote the checks")),
    ("skill-discoveries", ("does not authorize rework", "explicit user request and the existing reopen rule")),
    ("ha-commands", ("| `ha note <goal> reopen --quote \"…\"` | Records a rework request; voids the completion record and opens a new budget window |",)),
    ("bundle-report", ("The `ha done` output (completion) or `ha status --format md` output (stop), pasted unchanged",
                       "Per-condition evidence (condition, result, record seq)",
                       "Changed files and the conditions each served, marked as the agent's claim",
                       "Remaining limits: assumed decisions, `manual` conditions, assurance `local`, and that the executor wrote the checks",
                       "How to continue, when not complete")),
    ("contract-report", ("그대로 붙이고 고쳐 쓰지 않는다", "조건별 근거: 조건, 결과, 기록 seq",
                         "바뀐 파일과 각 파일이 기여한 조건", "보증 수준(`local`), 검사 작성자가 실행자라는 사실",
                         "이어 가는 방법")),
    # Goals started with an earlier Seal keep what they started with.
    ("skill-execution-bundle", ("a goal started with an earlier Seal keeps the PROGRESS form it started with",)),
    ("skill-resuming", ("Do not require `/3` declarations for `/1` or `/2`, convert the goal, or rewrite its history",)),
    ("skill-discoveries", ("Preserve original formats and records of older goals, and never fabricate past observations",)),
    ("skill-check-inputs", ("whichever Seal version started the goal",
                            "never require rewriting or converting an existing goal's checks, records, `PROGRESS.md` or `REVIEW.md`")),
    ("bundle-check-inputs", ("whichever Seal version started the goal",
                             "never require rewriting or converting an existing goal's checks, records, `PROGRESS.md` or `REVIEW.md`")),
    ("contract-check-inputs", ("어느 Seal 버전으로 시작했는지는 따지지 않는다",
                               "기존 목표의 검사, 기록, PROGRESS·REVIEW를 다시 쓰거나 변환하라고 요구하지 않는다")),
]
# Each template's level-two headings, in order, as goals use them now (AC-6).
TEMPLATE_HEADINGS = {
    PLAN: ["## 조건표", "## 결과 계약", "## 범위", "## Task", "## 예산", "## 읽을 자료"],
    PROGRESS: ["## 현재", "## Task 상태", "## 타임라인", "## 계획 변경", "## 막힘", "## 개선 메모", "## 완료 뒤 발견", "## 완료 보고"],
    REVIEW: ["## 요구사항 대응", "## 불필요한 task", "## 문서 충돌", "## 중요한 미결정", "## 검증 방법과 환경", "## 결론"],
}
# The places AC-7 names, by the criteria whose guidance each carries.
PLACES = {
    "skill body": ("skill", "skill-completion", "skill-discoveries"),
    "references/bundle.md": ("bundle", "bundle-report", "bundle-discoveries"),
    "references/ha.md": ("ha",),
    "templates": ("plan", "progress-report", "progress-discoveries"),
    "contracts/bundle.md": ("contract", "contract-report", "contract-discoveries"),
}
MANIFESTS = ("plugins/seal/.claude-plugin/plugin.json", "plugins/seal/.codex-plugin/plugin.json")
MARKETPLACE = ".claude-plugin/marketplace.json"


class Unavailable(Exception):
    """The observation could not be made; never a requirement violation."""


def require(case, condition, violation, message):
    if not condition:
        case.fail(f"[violation:{violation}] {message}")


def read(path):
    """The file's text, or None when the tree has no such file."""
    try:
        return (ROOT / path).read_text(encoding="utf-8")
    except FileNotFoundError:
        return None
    except (OSError, UnicodeDecodeError) as error:
        raise Unavailable(f"{path}: {error}") from None


def headings(text):
    """Heading lines outside fenced code blocks, as (line index, level, line)."""
    found, fenced = [], False
    for index, line in enumerate(text.split("\n")):
        if line.startswith("```"):
            fenced = not fenced
        elif not fenced and re.match(r"#{1,6} ", line):
            found.append((index, len(line) - len(line.lstrip("#")), line))
    return found


def section(key):
    """The section's text, or None when its file or heading is absent."""
    path, heading = SECTIONS[key]
    text = read(path)
    if text is None:
        return None
    lines = text.split("\n")
    marks = headings(text)
    start = [(index, level) for index, level, line in marks if line == heading]
    if len(start) != 1:
        return None
    index, level = start[0]
    end = next((i for i, l, _ in marks if i > index and l <= level), len(lines))
    return "\n".join(lines[index:end])


def missing(clauses):
    """The phrases each section lacks, as messages."""
    problems = []
    for key, phrases in clauses:
        path, heading = SECTIONS[key]
        text = section(key)
        if text is None:
            problems.append(f"{path}: section {heading!r} absent")
            continue
        problems += [f"{path} {heading}: missing {phrase!r}" for phrase in phrases if phrase not in text]
    return problems


def version(text):
    found = re.search(r'(?m)^  version: "(\d+)\.(\d+)\.(\d+)"$', text or "")
    return tuple(int(part) for part in found.groups()) if found else None


class FinalCandidatePlan(unittest.TestCase):
    """AC-1: narrow checks while exploring; the expensive full verification once on the final candidate."""

    def test_guidance_plans_the_full_verification_once_on_the_final_candidate(self):
        problems = missing(CLAUSES["AC-1"])
        require(self, not problems, "final-candidate-plan-missing", "; ".join(problems))


class SharedRegression(unittest.TestCase):
    """AC-2: one condition owns a shared full regression; the others observe their own property."""

    def test_guidance_gives_a_shared_regression_to_one_condition(self):
        problems = missing(CLAUSES["AC-2"])
        require(self, not problems, "shared-regression-guidance-missing", "; ".join(problems))


class ReworkAfterReview(unittest.TestCase):
    """AC-3: a requested review fix after completion is reopened, re-recorded on the final head and completed again."""

    def test_guidance_records_completion_again_on_the_final_head(self):
        problems = missing(CLAUSES["AC-3"])
        require(self, not problems, "rework-after-review-missing", "; ".join(problems))


class CompletionCoverage(unittest.TestCase):
    """AC-4: the report names the completion seq and commit and whether the sent head is that commit."""

    def test_report_format_states_the_covered_commit(self):
        problems = missing(CLAUSES["AC-4"])
        require(self, not problems, "completion-coverage-missing", "; ".join(problems))


class UnrequestedReview(unittest.TestCase):
    """AC-5: review comments without the user's request are discoveries, not rework authority."""

    def test_guidance_keeps_unrequested_comments_as_discoveries(self):
        problems = missing(CLAUSES["AC-5"])
        require(self, not problems, "unrequested-review-guidance-missing", "; ".join(problems))


class Kept(unittest.TestCase):
    """AC-6: the rework rule, the completion report items and the template formats stay as they are."""

    def test_rework_rule_and_formats_stay(self):
        problems = missing(KEPT)
        for path, expected in TEMPLATE_HEADINGS.items():
            text = read(path)
            found = None if text is None else [line for _, level, line in headings(text) if level == 2]
            if found != expected:
                problems.append(f"{path}: level-two headings {found}, not {expected}")
        require(self, not problems, "earlier-formats-changed", "; ".join(problems))


class SameGuidance(unittest.TestCase):
    """AC-7: the skill body, both references, the templates and the contract carry the same guidance."""

    def test_every_place_carries_its_part(self):
        problems = []
        for place, keys in PLACES.items():
            clauses = [(key, phrases) for criterion in ("AC-1", "AC-2", "AC-3", "AC-4", "AC-5")
                       for key, phrases in CLAUSES[criterion] if key in keys]
            problems += [f"{place}: {problem}" for problem in missing(clauses)]
        require(self, not problems, "guidance-differs-between-places", "; ".join(problems))

    def test_seal_version_is_raised_and_agrees(self):
        skill = version(read(SKILL))
        problems = [] if skill and skill > EARLIER_SEAL else [f"{SKILL} version {skill} is not above {EARLIER_SEAL}"]
        wanted = ".".join(map(str, skill)) if skill else None
        for path in MANIFESTS:
            try:
                found = json.loads(read(path) or "null")
            except ValueError as error:
                raise Unavailable(f"{path}: {error}") from None
            if not isinstance(found, dict) or found.get("version") != wanted:
                problems.append(f"{path} version {found.get('version') if isinstance(found, dict) else found!r}, "
                                f"not the skill's {wanted}")
        try:
            plugins = (json.loads(read(MARKETPLACE) or "null") or {}).get("plugins", [])
        except (ValueError, AttributeError) as error:
            raise Unavailable(f"{MARKETPLACE}: {error}") from None
        seal = [p.get("version") for p in plugins if isinstance(p, dict) and p.get("name") == "seal"]
        if seal != [wanted]:
            problems.append(f"{MARKETPLACE} seal versions {seal}, not [{wanted!r}]")
        require(self, not problems, "seal-version-not-raised", "; ".join(problems))


if __name__ == "__main__":
    unittest.main()
