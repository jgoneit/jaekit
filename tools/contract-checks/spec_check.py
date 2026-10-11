#!/usr/bin/env python3
"""Public Spec decision-boundary requirements; no model behavior inference."""
import check as checker
import re

P = checker.SPEC
checker.REQUIREMENTS["spec-boundaries"] = {
    "AC-1": [
        (P + "SKILL.md", ("uninterpretable input", "observation unit", "before/after set invariants", "supported environment")),
        ("contracts/goal-docs.md", ("판정 불가 입력", "관측 단위", "전후 집합 불변식", "지원 환경")),
        (P + "references/goal-docs.md", ("observation unit", "only relevant boundaries")),
    ],
    "AC-2": [
        (P + "SKILL.md", ("Do not ask again when a public contract", "Implementation and verification methods", "unresolved outcome")),
        ("contracts/goal-docs.md", ("공개 계약이 이미 답하는", "구현·검증 방법", "출처")),
    ],
    "AC-3": [
        (P + "SKILL.md", ("exhaustive set or representative examples", "observable general property", "outside the supported scope")),
        (P + "references/goal-docs.md", ("exhaustive set or representative examples", "general property")),
        (P + "assets/templates/small-change.md", ("exhaustive or representative",)),
        (P + "assets/templates/backend-feature.md", ("exhaustive or representative",)),
        (P + "assets/templates/cross-module-feature.md", ("exhaustive or representative",)),
        (P + "assets/templates/product.md", ("exhaustive or representative",)),
    ],
    "AC-4": [
        (P + "SKILL.md", ("newline-containing names", "symlinks", "shallow clones", "concurrent ordering", "do not turn these examples into a checklist")),
        ("contracts/goal-docs.md", ("줄바꿈 이름", "symlink", "얕은 clone", "동시 실행 순서", "고정 점검표")),
    ],
    "AC-7": [
        ("evals/spec-cases/decision-boundaries.md", ("가상 시나리오", "실제 사용자 대화나 실행 기록이 아닙니다", "관측 단위", "판정 불가", "표본", "묻지 않아야 할 것", "## 사용자 답")),
        ("evals/spec-cases/README.md", ("[decision-boundaries](decision-boundaries.md)",)),
        ("evals/spec-cases/readme-typo.md", ("질문 상한: 0",)),
    ],
}

base_observe = checker.observe

def observe(root, suite, criterion):
    observation, problems = base_observe(root, suite, criterion)
    source = root / "evals/spec-cases/decision-boundaries.md"
    if criterion in {"AC-1", "AC-2", "AC-3", "AC-4", "AC-7"}:
        rows = checker.rows(root, "evals/spec-cases/decision-boundaries.md", "## 판정 연결")
        expected = {
            "observation-draft": ("Draft", "2", "미정", "미정", "표본", "실행하지 않은 텍스트는 실행 근거가 아니다", "근거 아님", "요청 A", "Bash", "AC-1, AC-2", "관측 단위; 판정 불가"),
            "observation-ready": ("Ready", "0", "요청", "근거 불충분", "표본", "실행하지 않은 텍스트는 실행 근거가 아니다", "근거 아님", "사용자 답 A", "Bash", "AC-1, AC-2", "없음"),
            "cleanup-ready": ("Ready", "0", "정리 호출", "관계없는 파일 보존", "표본", "선택한 구버전 자료 외 모든 파일은 보존된다", "보존", "공개 계약 B", "Bash, Zsh, shallow clone", "AC-1, AC-2", "없음"),
            "allowlist-ready": ("Ready", "0", "입력 하나", "거부", "전체 목록", "열거한 값만 허용", "거부", "요청 C", "플랫폼 무관", "AC-1", "없음"),
        }
        columns = ("상태", "질문 수", "관측 단위", "판정 불가", "예시 범위", "일반 성질", "목록 밖", "결정 출처", "지원 환경", "조건", "열린 결정")
        by_case = {row.get("사례"): row for row in rows}
        if set(by_case) != set(expected) or len(rows) != len(expected):
            problems.append("public boundary examples absent or case identity ambiguous")
        for name, values in expected.items():
            row = by_case.get(name, {})
            if tuple(row.get(column) for column in columns) != values:
                problems.append(name + ": boundary/source/open-decision/general-property relation incorrect")
        if source.exists():
            text = source.read_text(encoding="utf-8")
            for name, values in expected.items():
                heading = "## 목표 예시: " + name + "\n"
                section = text.split(heading, 1)[-1] if heading in text else ""
                body = section.split("```markdown\n", 1)[-1].split("\n```", 1)[0] if "```markdown\n" in section else ""
                criteria = set(re.findall(r"^- \*\*(AC-\d+)\*\*", body, re.M))
                referenced = set(re.findall(r"\bAC-\d+\b", values[9]))
                scenarios = re.findall(r"^\| (?:흐름|경계|유지) \|.*\| (AC-[\d, -]+) \|$", body, re.M)
                if not criteria or criteria != referenced or not scenarios:
                    problems.append(name + ": concrete criteria or scenarios missing")
                if any(set(re.findall(r"AC-\d+", ids)) - criteria for ids in scenarios):
                    problems.append(name + ": scenario references absent criterion")
                if "Status: " + values[0] not in body or "Criteria-Format: nested/1" not in body:
                    problems.append(name + ": example format or status contradicts decision table")
                open_body = body.split("## Open Decisions\n", 1)[-1].split("```", 1)[0].strip() if "## Open Decisions\n" in body else ""
                expected_open = "관측 단위는 요청인가 세션인가? 판독할 수 없는 입력은 어떤 결과인가?" if name.endswith("-draft") else "없음."
                if open_body != expected_open:
                    problems.append(name + ": concrete Open Decisions contradicts status")
                required_body = {
                    "observation-draft": ("출처: 요청 A.", "실행하지 않은 텍스트를 실행 근거로 인정하지 않는다.", "대표 사례이며 예시 밖의 비실행 텍스트에도 같은 성질이 적용된다."),
                    "observation-ready": ("출처: 사용자 답 A.", "해당 요청에서 실제로 실행하지 않은 텍스트는 실행 근거가 아니다.", "판독 불가는 근거 불충분이고 다른 요청의 근거는 섞이지 않는다."),
                    "cleanup-ready": ("출처: 공개 계약 B.", "호출 전후 선택 범위 밖의 모든 파일은 추가·삭제·변경되지 않는다.", "목록 밖의 관계없는 파일도 보존된다.", "대상인지 판단할 수 없으면 해당 파일을 보존한다."),
                    "allowlist-ready": ("출처: 요청 C.", "`a`와 `b`만 허용한다.", "전체 목록 밖의 값과 판정 불가는 거부한다."),
                }[name]
                if any(clause not in body for clause in required_body):
                    problems.append(name + ": concrete criterion or decision source omits the agreed boundary")
    if problems:
        observation.update(status="violation", violation=f"{suite}.{criterion}.missing-contract")
    return observation, problems

checker.observe = observe

if __name__ == "__main__":
    raise SystemExit(checker.main())
