#!/usr/bin/env python3
"""Observe the Seal check-authoring guidance in the checked tree.

Each clause names a product document, the section that must hold it and the
semantic anchors it must contain there, so an anchor elsewhere in the file
does not count. Relations compare the guidance with the Core facts it states
and with the source identity files. Everything is read from the tree this
process runs in: no commit, ref or history is read, so a baseline run on the
base commit and a run on the change judge the same property.
"""
import json
import re
import subprocess
import sys

import check as checker

SUITE = "authoring"
P = checker.SEAL
SKILL = P + "SKILL.md"
BUNDLE = P + "references/bundle.md"
HA = P + "references/ha.md"
PLAN = P + "assets/templates/PLAN.md"
REVIEW = P + "assets/templates/REVIEW.md"
CONTRACT = "contracts/bundle.md"

# (path, heading line of the section, anchors). A section runs from its
# heading to the next heading of the same or a higher level.
SECTIONS = {
    "skill": (SKILL, "## Check inputs"),
    "bundle": (BUNDLE, "## Check inputs"),
    "bundle-review": (BUNDLE, "## REVIEW.md"),
    "ha": (HA, "## What checks read"),
    "plan": (PLAN, "## 조건표"),
    "review": (REVIEW, "## 검증 방법과 환경"),
    "review-decisions": (REVIEW, "## 중요한 미결정"),
    "contract": (CONTRACT, "### 2.1.2 검사 입력과 관측"),
    "contract-review": (CONTRACT, "## 4. `REVIEW.md`"),
}

CLAUSES = {
    "AC-1": [
        ("skill", ("every check in the condition table", "`EX-n`", "task commands",
                   "producer, configuration or helper file", "the tree being checked",
                   "external inputs its condition names", "start or base commit", "fixed commit",
                   "start record or Core output", "other commits, refs or objects",
                   "full clone and a history-less clone", "synthetic repository",
                   "references/bundle.md#check-inputs")),
        ("bundle", ("same verdict", "full clone and a history-less clone", "pinned SHA",
                    "base read from the start record or Core output", "remote-tracking branch", "`HEAD~n`",
                    "merge-base", "log range", "another worktree", "synthetic repository the check builds itself",
                    "producer, configuration or helper file")),
        ("review", ("repository history",)),
    ],
    "AC-2": [
        ("skill", ("`maintain`", "property the goal documents require", "unchanged since the base commit",
                   "`바꿀 수 있는 경로`", "`out_of_scope`", "expected content")),
        ("bundle", ("content, output, format or behavior", "not an observation of that property",
                    "outside `바꿀 수 있는 경로`", "`out_of_scope`", "since the base commit", "tracked check file")),
        ("plan", ("`maintain`", "property", "unchanged since the base commit", "`out_of_scope`")),
    ],
    "AC-3": [
        ("skill", ("only by Core's baseline run", "never checks out, reads or compares the base commit",
                   "violation on the base commit and a pass after the change")),
        ("bundle", ("temporary worktree of the base commit", "check paths copied from HEAD",
                    "never checks out, reads or compares the base commit")),
        ("ha", ("baseline", "temporary worktree of the base commit", "`out_of_scope`",
                "`test_definition_changed`", "Checks themselves")),
    ],
    "AC-4": [
        ("skill", ("about repository history or refs", "only the range and refs the condition names",
                   "error or unobserved", "neither a violation nor a pass", "pre-execution review")),
        ("bundle", ("tag's type", "commit message", "only the range and refs the condition names",
                    "unobserved, environment error", "neither a violation nor a pass", "`REVIEW.md`")),
        ("review", ("range",)),
    ],
    "AC-5": [
        ("skill", ("declares its own scope or trust boundary", "current behavior", "keeps the declaration",
                   "explicit assumption", "without asking the user", "same import boundary",
                   "document-conflict rule")),
        ("bundle", ("pinned to a version or release", "preserves past verdicts", "isolates imports",
                    "limits its inputs", "explicit assumption", "repeat it in the report",
                    "inside the boundary it declares", "without asking the user")),
        ("review-decisions", ("declared scope or trust boundary",)),
    ],
    "AC-8": [
        ("skill", ("Seal 0.1.14", "whichever Seal version started the goal", "never require rewriting")),
        ("bundle", ("Seal 0.1.14", "never require rewriting or converting")),
        ("contract", ("Seal 0.1.14", "다시 쓰거나 변환하라고 요구하지 않는다")),
    ],
    "AC-9": [
        ("contract", ("검사하는 트리", "조건이 지정한 외부 입력", "시작·base 커밋", "고정 커밋",
                      "시작 기록이나 Core 출력", "다른 커밋, ref, 객체", "전체 clone과 히스토리 없는 clone",
                      "합성 저장소", "`maintain`", "성질", "`out_of_scope`", "기대 내용",
                      "baseline", "임시 worktree", "범위와 ref만", "미관측", "범위나 신뢰 경계",
                      "명시적 가정", "사용자에게 새로 묻지 않는다", "문서 충돌 규칙")),
    ],
}

# Table rows of the REVIEW section that must carry a clause: (criterion, section, row label, anchor).
ROWS = [
    ("AC-1", "bundle-review", "검증 방법과 환경", "repository history"),
    ("AC-5", "bundle-review", "중요한 미결정", "declared scope or trust boundary"),
    ("AC-9", "contract-review", "검증 방법과 환경", "저장소 히스토리"),
    ("AC-9", "contract-review", "중요한 미결정", "범위나 신뢰 경계"),
]

# Existing rules this goal keeps (SPEC: 범위와 비목표, 유지). Their anchors
# must stay where they are; the guidance may not drop or weaken them.
KEPT = [
    (SKILL, ("Python 3 is not a prerequisite for Seal", "does not require an official adapter",
             "Existing `/1` and `/2` goals retain their result semantics", "## Assurance policy",
             "ha note <goal> confirm <AC-n | scope:<path> | tests:<path> | spec>")),
    (BUNDLE, ("No particular runner or adapter is required", "Changes outside them need the user's confirmation",
              "Editing or deleting a test that existed at base needs the user's confirmation")),
    (CONTRACT, ("`/1`·`/2`의 기존 목표는 종전 결과 의미를 유지한다",)),
]

# The user-resolved reasons in the Core reference are the existing points that
# reach the user. The guidance adds none and removes none (AC-6).
USER_REASONS = {"record_invalid", "error_limit", "manual_unconfirmed", "record_integrity",
                "out_of_scope", "test_definition_changed", "spec_changed"}
ASKING = re.compile(r"(?i)\b(ask|asks|asking|approval|approve|confirmation|confirm)\b|묻는다|묻고|승인을 받|확인을 받")
NOT_ASKING = re.compile(r"(?i)without asking|never ask|no user confirmation|not ask|묻지 않|새로 묻지|만들지 않|더하지 않|add no")
PRIVATE = (
    (re.compile(r"(?<![0-9A-Za-z])[0-9a-f]{7,40}(?![0-9A-Za-z])"), "commit-like hexadecimal"),
    (re.compile(r"(?i)\bPR ?#\d+|/pull/\d+|discussion_r\d+|(?<![\w`])#\d{2,}\b"), "pull request reference"),
    (re.compile(r"/Users/|/home/|jaekit-worktrees|C:\\\\Users"), "local path"),
    (re.compile(r"docs/specs?/(?!<)[A-Za-z0-9]"), "concrete goal path"),
    (re.compile(r"runs\.jsonl:\d+|seq \d+ \("), "execution record reference"),
)


def section(root, key):
    name, heading = SECTIONS[key]
    path = root / name
    if not path.exists():
        return None
    lines = path.read_text(encoding="utf-8").split("\n")
    level = len(heading) - len(heading.lstrip("#"))
    for i, line in enumerate(lines):
        if line.strip() == heading:
            out = []
            for following in lines[i + 1:]:
                stripped = following.lstrip()
                depth = len(stripped) - len(stripped.lstrip("#"))
                if following.startswith("#") and 0 < depth <= level and stripped[depth:depth + 1] == " ":
                    break
                out.append(following)
            return "\n".join(out)
    return ""


def version(text):
    match = re.search(r'(?m)^\s+version: "(\d+)\.(\d+)\.(\d+)"$', text)
    return tuple(int(part) for part in match.groups()) if match else None


def manifest_version(root, name):
    return json.loads((root / name).read_text(encoding="utf-8")).get("version")


def regressions(root):
    """Run the skill and contract consistency regressions in this tree.

    A failing test is an observed contradiction. A tool that cannot run or
    build is not an observation and raises, which the caller reports as error.
    """
    problems = []
    go = subprocess.run(["go", "test", "-count=1", "./plugins/"], cwd=root, capture_output=True, text=True)
    output = go.stdout + go.stderr
    if go.returncode != 0:
        if go.returncode != 1 or "[build failed]" in output or "[setup failed]" in output:
            raise RuntimeError("plugin regressions could not run: " + output[-400:])
        problems.append("plugin consistency regressions failed: " + output[-400:])
    docs = subprocess.run([sys.executable, "tools/check_public_docs.py"], cwd=root, capture_output=True, text=True)
    if docs.returncode == 1:
        problems.append("public documentation check failed: " + (docs.stdout + docs.stderr)[-400:])
    elif docs.returncode != 0:
        raise RuntimeError("public documentation check could not run: " + (docs.stdout + docs.stderr)[-400:])
    return problems


def new_guidance(root):
    texts = []
    for key in ("skill", "bundle", "ha", "contract", "plan", "review"):
        text = section(root, key)
        if text:
            texts.append((key, text))
    return texts


def observe(root, suite, criterion):
    if suite != SUITE:
        return base_observe(root, suite, criterion)
    problems = []
    for key, anchors in CLAUSES.get(criterion, []):
        name, heading = SECTIONS[key]
        text = section(root, key)
        if text is None:
            problems.append(f"{name}: required product artifact absent")
            continue
        if text == "":
            problems.append(f"{name}: section {heading!r} absent")
            continue
        for anchor in anchors:
            if anchor not in text:
                problems.append(f"{name} {heading}: missing {anchor!r}")
    for owner, key, label, anchor in ROWS:
        if owner != criterion:
            continue
        rows = [line for line in (section(root, key) or "").split("\n") if line.startswith(f"| {label} |")]
        if len(rows) != 1 or anchor not in rows[0]:
            problems.append(f"{SECTIONS[key][0]}: row {label!r} lacks {anchor!r}")
    if criterion == "AC-2":
        # The guidance states Core's out_of_scope; the Core source and reference must say the same.
        status = (root / "internal/status/status.go").read_text(encoding="utf-8")
        if 'add("out_of_scope", "changed since base outside 바꿀 수 있는 경로", outside)' not in status:
            problems.append("Core no longer reports out_of_scope as changes since base outside 바꿀 수 있는 경로")
        if "| `out_of_scope` | user | Files changed since base outside `바꿀 수 있는 경로` |" not in (root / HA).read_text(encoding="utf-8"):
            problems.append("ha reference out_of_scope row changed")
    if criterion == "AC-3":
        reference = (root / HA).read_text(encoding="utf-8")
        if "Runs `change` conditions on a temporary worktree of the base commit with their check paths copied from HEAD" not in reference:
            problems.append("ha reference baseline row changed")
        if "baselineFiles" not in (root / "cmd/ha/baseline.go").read_text(encoding="utf-8"):
            problems.append("Core baseline overlay of check paths not found")
    if criterion == "AC-8":
        skill = version((root / SKILL).read_text(encoding="utf-8"))
        if skill is None or skill <= (0, 1, 13):
            problems.append(f"Seal source version {skill} is not above 0.1.13")
        else:
            want = ".".join(map(str, skill))
            for host in ("plugins/seal/.claude-plugin/plugin.json", "plugins/seal/.codex-plugin/plugin.json"):
                if manifest_version(root, host) != want:
                    problems.append(f"{host} does not carry {want}")
            market = json.loads((root / ".claude-plugin/marketplace.json").read_text(encoding="utf-8"))
            if market.get("metadata", {}).get("version") != "0.1.4":
                problems.append("marketplace release identity is not 0.1.4")
            if [p.get("version") for p in market.get("plugins", []) if p.get("name") == "seal"] != [want]:
                problems.append(f"marketplace seal entry does not carry {want}")
            if f"Seal {want}" not in (section(root, "skill") or ""):
                problems.append(f"guidance does not name Seal {want} as its starting version")
    if criterion == "AC-9":
        for name, anchors in KEPT:
            text = (root / name).read_text(encoding="utf-8")
            for anchor in anchors:
                if anchor not in text:
                    problems.append(f"{name}: kept rule {anchor!r} dropped")
        if not problems:
            problems.extend(regressions(root))
    if criterion == "AC-6":
        reference = (root / HA).read_text(encoding="utf-8")
        found = set(re.findall(r"(?m)^\| `([a-z_]+)` \| user \|", reference))
        if found != USER_REASONS:
            problems.append(f"user-resolved reasons changed: {sorted(found ^ USER_REASONS)}")
        for key, text in new_guidance(root):
            for sentence in re.split(r"(?<=[.!?。])\s+|\n", text):
                if ASKING.search(sentence) and not NOT_ASKING.search(sentence):
                    problems.append(f"{SECTIONS[key][0]}: new guidance may add a user step: {sentence.strip()[:160]}")
        skill = section(root, "skill")
        if skill and "add no user confirmation, approval or question" not in skill:
            problems.append("guidance does not state that it adds no user step")
    if criterion == "AC-10":
        for key, text in new_guidance(root):
            for pattern, label in PRIVATE:
                for match in pattern.finditer(text):
                    problems.append(f"{SECTIONS[key][0]}: {label} in new guidance: {match.group(0)!r}")
    target = f"{suite}.{criterion}"
    observation = {"target": target, "attempt": 1, "status": "pass"}
    if problems:
        observation.update(status="violation", violation=f"{target}.missing-contract")
    return observation, problems


base_observe = checker.observe
checker.REQUIREMENTS[SUITE] = {criterion: [] for criterion in
                               ("AC-1", "AC-2", "AC-3", "AC-4", "AC-5", "AC-6", "AC-8", "AC-9", "AC-10")}
checker.observe = observe

if __name__ == "__main__":
    sys.argv[1:1] = [SUITE] if len(sys.argv) == 2 else []
    raise SystemExit(checker.main())
