#!/usr/bin/env python3
"""Observe public document requirements; never infer violations from runner exits.

These checks establish document contracts and synthetic examples, not model
compliance. Each registered condition owns its observed violation identifier.
Declarations are consumed by Core, never used as the observation's answer.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[2]
PRODUCER = "jaekit-document-contracts"
VERSION = "1"
SEAL = "plugins/seal/skills/seal/"
SPEC = "plugins/spec/skills/spec/"

# A clause is a product path and required semantic anchors. The checks read
# only these public product artifacts, never this source or private goals.
REQUIREMENTS: dict[str, dict[str, list[tuple[str, tuple[str, ...]]]]] = {
    "reporting": {
        "AC-1": [
            ("contracts/bundle.md", ("기대 결과의 근거", "관측할 경계", "시험 방법과 대체 환경", "조건 그룹")),
            (SEAL + "assets/templates/REVIEW.md", ("Expected-result grounds", "Planned observation boundary", "Method and substitutes", "condition groups")),
            (SEAL + "references/bundle.md", ("Expected-result grounds", "Planned observation boundary", "Method and substitutes", "not the authority for correctness")),
        ],
        "AC-2": [
            ("contracts/bundle.md", ("예정·미수행·실제 수행", "결과에 영향을 주는 환경만", "미관측")),
            (SEAL + "references/bundle.md", ("planned, not performed, and observed", "only environments that affect the condition", "unobserved")),
        ],
        "AC-3": [
            ("contracts/bundle.md", ("덮지 않는 실패 종류", "예시를 전체 입력의 정답으로 확대하지", "검사 개수")),
            (SEAL + "assets/templates/REVIEW.md", ("Failure classes not covered", "general property", "not a sufficiency score")),
        ],
        "AC-4": [
            ("contracts/bundle.md", ("검토의 예정과 보고의 실제 결과", "조건·검사·환경이 바뀌면", "기록으로 확인한 사실과 실행자의 해석")),
            (SEAL + "references/bundle.md", ("planned review to observed report", "condition, check, or environment changes", "recorded facts from the executor's interpretation")),
            (SEAL + "assets/templates/PROGRESS.md", ("planned review to observed report", "unobserved", "changed condition, check, or environment")),
        ],
        "AC-5": [
            ("contracts/bundle.md", ("사건 시각과 작성 시각", "시각 출처", "남은 조건", "새 사실", "가설의 근거", "줄일 불확실성")),
            (SEAL + "assets/templates/PROGRESS.md", ("event time and writing time", "time source", "remaining conditions", "new facts", "hypothesis evidence", "uncertainty to reduce")),
            (SEAL + "references/bundle.md", ("event time and writing time", "time source", "not every tool call")),
        ],
        "AC-6": [
            ("contracts/bundle.md", ("확인한 것·확인하지 못한 것·위험한 경계", "배포·설치·실제 모델 실행의 증거를 대신하지", "검사 작성자가 실행자")),
            (SEAL + "SKILL.md", ("expected-result grounds", "observed boundaries", "methods and substitutes", "does not establish deployment")),
        ],
        "AC-7": [
            ("examples/empty-input/REVIEW.md", ("기대 결과의 근거", "관측할 경계", "시험 방법과 대체 환경", "AC-1, AC-2", "덮지 않는 실패 종류")),
            ("examples/empty-input/PROGRESS.md", ("미수행", "미관측", "시각 출처", "기록 없음", "AC-1, AC-2")),
            ("guides/OPERATIONS.md", ("기대 결과의 근거", "관측 경계", "시험 방법과 대체 환경", "기존 목표")),
            ("examples/verification-reporting.md", ("가상 형식 예시", "실제 수행 기록이 아니다", "review-v1", "review-v2")),
        ],
    },
}


def rows(root: Path, name: str, heading: str) -> list[dict[str, str]]:
    path = root / name
    if not path.exists():
        return []
    source = path.read_text(encoding="utf-8")
    marker = "\n" + heading + "\n"
    if marker not in source:
        return []
    section = source.split(marker, 1)[1].split("\n## ", 1)[0]
    lines = [line for line in section.splitlines() if line.startswith("| ")]
    if len(lines) < 2:
        return []
    columns = [value.strip() for value in lines[0].strip("|").split("|")]
    result = []
    for line in lines[2:]:
        values = [value.strip() for value in line.strip("|").split("|")]
        if len(values) != len(columns) or len(set(columns)) != len(columns):
            raise ValueError(f"{name}: malformed public example table")
        result.append(dict(zip(columns, values)))
    return result


def reporting_links(root: Path, criterion: str) -> list[str]:
    """Compare public planned/observed fields, not just their word presence."""
    if criterion not in {"AC-1", "AC-2", "AC-4", "AC-5", "AC-7"}:
        return []
    planned = rows(root, "examples/empty-input/REVIEW.md", "## 근거 연결 예시")
    observed = rows(root, "examples/empty-input/PROGRESS.md", "## 관측 연결 예시")
    if not planned or not observed:
        return ["planned/observed public example relationship absent"]
    required_plan = {"그룹", "조건", "기대 결과의 근거", "관측할 경계", "시험 방법과 대체 환경", "검사 버전"}
    required_observed = {"조건", "검토 그룹", "검사 버전", "실제 관측 경계", "결과", "미관측", "시각 종류", "시각 출처"}
    if any(set(row) != required_plan for row in planned) or any(set(row) != required_observed for row in observed):
        return ["public example lacks unambiguous evidence columns"]
    problems = []
    groups = {row["그룹"]: row for row in planned}
    if len(groups) != len(planned):
        problems.append("duplicate planned group")
    planned_conditions = [ac.strip() for row in planned for ac in row["조건"].split(",")]
    if sorted(planned_conditions) != ["AC-1", "AC-2"]:
        problems.append("shared group must cover each example condition exactly once")
    if sorted(row["조건"] for row in observed) != ["AC-1", "AC-2"]:
        problems.append("observed rows must cover each example condition exactly once")
    for row in observed:
        plan = groups.get(row["검토 그룹"])
        if plan is None or row["조건"] not in [ac.strip() for ac in plan["조건"].split(",")]:
            problems.append("observed condition does not belong to referenced group")
            continue
        if row["검사 버전"] != plan["검사 버전"]:
            problems.append("planned and observed check versions differ without a new review")
        # This public example expressly has no execution. Plans cannot become
        # observations merely by copying the planned boundary or a timestamp.
        if row["결과"] != "미수행" or row["실제 관측 경계"] != "미관측":
            problems.append("unexecuted example promoted a plan to observed success")
        if row["미관측"] != plan["관측할 경계"]:
            problems.append("unobserved boundary lost the planned scope")
        if row["시각 종류"] != "미상" or row["시각 출처"] != "기록 없음":
            problems.append("unexecuted example invented an event time source")
    if criterion in {"AC-2", "AC-4", "AC-7"}:
        mixed = rows(root, "examples/verification-reporting.md", "## 관측 비교")
        required = {"대상", "기대 근거", "코드", "검사", "검토", "관측 경계", "대체 환경", "미관측"}
        if len(mixed) != 3 or any(set(row) != required for row in mixed):
            problems.append("mixed observation example absent or malformed")
        else:
            by_target = {row["대상"]: row for row in mixed}
            for target, review, check, boundary, substitute, unobserved in (
                    ("shell-v1", "review-v1", "check-v1", "Bash process", "없음", "Zsh, real model"),
                    ("model-v1", "review-v1", "check-v1", "adapter output", "fake model", "real model"),
                    ("shell-v2", "review-v2", "check-v2", "Bash process", "없음", "Zsh, real model")):
                row = by_target.get(target, {})
                expected = (review, check, boundary, substitute, unobserved)
                actual = tuple(row.get(key) for key in ("검토", "검사", "관측 경계", "대체 환경", "미관측"))
                if actual != expected or not row.get("기대 근거") or not row.get("코드"):
                    problems.append(f"{target}: boundary/substitute/review version link incorrect")
    return problems


def observe(root: Path, suite: str, criterion: str) -> tuple[dict, list[str]]:
    clauses = REQUIREMENTS[suite][criterion]
    target = f"{suite}.{criterion}"
    missing = []
    for name, anchors in clauses:
        path = root / name
        # An absent required product document is directly observed absence.
        # Permission, decoding, I/O and programming errors are not violations.
        if not path.exists():
            missing.append(f"{name}: required product artifact absent")
            continue
        source = path.read_text(encoding="utf-8")
        for anchor in anchors:
            if anchor not in source:
                missing.append(f"{name}: missing contract clause {anchor!r}")
    if suite == "reporting":
        missing.extend(reporting_links(root, criterion))
        if criterion == "AC-6":
            for name in (SEAL + "SKILL.md", SEAL + "references/bundle.md"):
                source = (root / name).read_text(encoding="utf-8")
                if re.search(r"(?i)(?:real execution|real model success) (?:alone )?(?:proves|guarantees) correctness", source):
                    missing.append(f"{name}: execution is incorrectly a correctness guarantee")
    observation = {"target": target, "attempt": 1, "status": "pass"}
    if missing:
        observation.update(status="violation", violation=f"{target}.missing-contract")
    return observation, missing


def emit(observations: list[dict]) -> None:
    destination = os.environ.get("HA_EVIDENCE_PATH")
    if destination is None:
        return
    report = {
        "schema": "check-result/v1",
        "invocation": os.environ["HA_EVIDENCE_INVOCATION"],
        "declaration_digest": os.environ["HA_EVIDENCE_DECLARATION_DIGEST"],
        "producer": PRODUCER,
        "producer_version": VERSION,
        "attempts_complete": True,
        "observations": observations,
    }
    with Path(destination).open("x", encoding="utf-8") as output:
        output.write(json.dumps(report) + "\n")


def main() -> int:
    if len(sys.argv) != 3 or sys.argv[1] not in REQUIREMENTS:
        print("usage: check.py <suite> <AC-n|all>", file=sys.stderr)
        return 64
    suite, selected = sys.argv[1:]
    criteria = list(REQUIREMENTS[suite]) if selected == "all" else [selected]
    if any(criterion not in REQUIREMENTS[suite] for criterion in criteria):
        print("unknown criterion", file=sys.stderr)
        return 64
    observations = []
    for criterion in criteria:
        try:
            observation, messages = observe(ROOT, suite, criterion)
        except Exception as error:
            observation = {"target": f"{suite}.{criterion}", "attempt": 1,
                           "status": "error", "reason": "execution_error"}
            messages = [f"{type(error).__name__}: {error}"]
        observations.append(observation)
        for message in messages:
            print(message)
    emit(observations)
    if any(item["status"] == "error" for item in observations):
        return 2
    return int(any(item["status"] != "pass" for item in observations))


if __name__ == "__main__":
    raise SystemExit(main())
