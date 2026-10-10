#!/usr/bin/env python3
"""Exercise the published result example and check the consumer documentation.

Uses an isolated synthetic repository and a temporary source-built binary. Never
reads private goals or invokes an installed ha. No package installation/network.
"""
from __future__ import annotations

import json
import os
from pathlib import Path
import re
import shlex
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[2]


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


def read(path: str) -> str:
    file = ROOT / path
    require(file.is_file(), f"missing published contract: {path}")
    return file.read_text(encoding="utf-8")


def example(text: str, heading: str, language: str) -> str:
    require(heading in text, f"missing documented example: {heading}")
    section = text.split(heading, 1)[1].split("\n## ", 1)[0]
    match = re.search(r"```" + re.escape(language) + r"\n(.*?)\n```", section, re.S)
    require(match is not None, f"missing {language} block under {heading}")
    return match.group(1)


def run(argv: list[str], cwd: Path, env: dict[str, str], expected: int = 0) -> str:
    result = subprocess.run(argv, cwd=cwd, env=env, text=True, capture_output=True, timeout=120)
    require(result.returncode == expected,
            f"{Path(argv[0]).name} {argv[1:3]} exited {result.returncode}, expected {expected}: "
            + result.stderr.strip()[-1500:])
    return result.stdout


def check() -> None:
    contract = read("contracts/check-result.md")
    record = read("contracts/run-record.md")
    bundle = read("contracts/bundle.md")
    operations = read("guides/OPERATIONS.md")
    guide = read("plugins/seal/skills/seal/references/ha.md")
    skill = read("plugins/seal/skills/seal/SKILL.md")
    require("run-rules/3" in record and "budget_change" in record,
            "run-record contract must describe /3 and same-window budget changes")
    require("check-result.md" in record and "check-result.md" in bundle and "check-result.md" in operations,
            "record, bundle and user guidance must lead to the result contract")
    require("check-declaration/v1" in guide and "check-result/v1" in guide,
            "installed Seal reference must explain the producer protocol")
    require("run-rules/3" in skill and "ha budget" in skill and "reopen" in skill,
            "Seal must distinguish new evidence and budget changes from reopening")
    for name, text in (("record", record), ("Seal reference", guide)):
        for token in ("run-rules/1", "run-rules/2", "run-rules/3"):
            require(token in text, f"{name} omits compatibility for {token}")
    # The examples are executable input, not an implementation re-created here.
    declaration = json.loads(example(contract, "### 선언 예시", "json"))
    config = json.loads(example(contract, "### 참조 설정 예시", "json"))
    tables = re.findall(r"```markdown\n(.*?)\n```", contract, re.S)
    condition = next((text for text in tables if "| 검증 명령 |" in text), "")
    mapping = next((text for text in tables if "## 결과 계약" in text), "")
    require(condition and mapping, "published PLAN example must include command and declaration mapping")
    command_match = re.search(r"\| AC-1 \| change \| `([^`]+)`", condition)
    require(command_match is not None, "published reference command is missing")
    command = shlex.split(command_match.group(1))
    require(command[0] == "python3" and len(command) == 4,
            "reference example must be a direct Python producer with declaration and config inputs")
    producer = read(command[1])
    env = dict(os.environ, GIT_CONFIG_NOSYSTEM="1", GIT_CONFIG_GLOBAL=os.devnull,
               GIT_AUTHOR_NAME="Synthetic Check", GIT_AUTHOR_EMAIL="check@example.invalid",
               GIT_COMMITTER_NAME="Synthetic Check", GIT_COMMITTER_EMAIL="check@example.invalid",
               GOTOOLCHAIN="local", PYTHONDONTWRITEBYTECODE="1")
    for key in ("GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GOOS", "GOARCH"):
        env.pop(key, None)
    with tempfile.TemporaryDirectory(prefix="jaekit-contract-") as directory:
        temp = Path(directory)
        binary = temp / "ha"
        run(["go", "build", "-mod=readonly", "-o", str(binary), "./cmd/ha"], ROOT, env)
        repo = temp / "repo"
        repo.mkdir()
        goal = "docs/specs/example"

        def put(path: str, content: str) -> None:
            destination = repo / path
            destination.parent.mkdir(parents=True, exist_ok=True)
            destination.write_text(content, encoding="utf-8")

        put(command[1], producer)
        put(command[2], json.dumps(declaration) + "\n")
        put(command[3], json.dumps(config) + "\n")
        put(goal + "/SPEC.md", "# Greeting\nStatus: Ready\nCriteria-Format: nested/1\n\n"
            "## Acceptance Criteria\n- **AC-1** The saved greeting is hello.\n\n## Open Decisions\nNone.\n")
        put(goal + "/PLAN.md", "# Plan\n\n## 조건표\n" + condition + "\n\n" + mapping
            + "\n\n## 범위\n- 바꿀 수 있는 경로: `src/**` (goal scope)\n"
            "- 테스트 경로: (기본값)\n- 유지할 동작: no other output\n\n"
            "## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n"
            "| --- | --- | --- | --- | --- | --- | --- |\n"
            "| T001 | Store hello | AC-1 | — | — | 예 | — |\n\n"
            "## 예산\n- 검증 실행: 50\n- 경과 시간: 4시간\n- 비용: 미관측\n")
        put(goal + "/REVIEW.md", "# Review\n\n## 결론\nready\n")
        put(goal + "/PROGRESS.md", "# Progress\n\n## Task 상태\n"
            "| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | synthetic |\n")
        run(["git", "init", "-q", "-b", "main"], repo, env)
        run(["git", "add", "."], repo, env)
        run(["git", "commit", "-q", "-m", "Synthetic base"], repo, env)
        run([str(binary), "start", goal, "--request", "Implement the synthetic greeting"], repo, env)
        records = repo / goal / "runs.jsonl"
        require(json.loads(records.read_text().splitlines()[0])["rules"] == "run-rules/3",
                "documented new-start default /3 differs from CLI")
        run([str(binary), "check", goal, "--baseline", "AC-1"], repo, env)
        require(json.loads(records.read_text().splitlines()[-1])["result"] == "fail_as_expected",
                "published example did not observe the declared missing behavior")
        for item in config["checks"]:
            put(item["path"], item["equals"])
        run(["git", "add", "src"], repo, env)
        run(["git", "commit", "-q", "-m", "Provide the greeting"], repo, env)
        run([str(binary), "check", goal, "AC-1"], repo, env)
        report = json.loads(run([str(binary), "status", goal, "--format", "json"], repo, env))
        require(report["status"] == "complete" and report["criteria"][0]["satisfied"],
                "published baseline/current example did not reach criterion satisfaction")
        require(report["assurance"] == "local" and report["check_author"] == "executor",
                "published example must retain the local executor-authored assurance boundary")
        require(report["budget"]["runs"] == 2,
                "baseline and current attempts must both count as use")
        # Execute the documented budget command unchanged apart from the binary.
        budget_line = next((line for line in record.splitlines() if line.startswith("ha budget ")), "")
        require(budget_line, "run-record contract lacks an executable budget example")
        budget_command = shlex.split(budget_line)
        require(budget_command[2] == goal, "budget example must name the synthetic example goal")
        run([str(binary), *budget_command[1:]], repo, env)
        changed = json.loads(run([str(binary), "status", goal, "--format", "json"], repo, env))["budget"]
        require(changed["runs"] == 2 and changed["runs_limit"] == 100 and changed["remaining_runs"] == 98,
                "documented budget command must change the total cap while retaining actual use")
        require(changed["window_start"] == report["budget"]["window_start"]
                and changed["elapsed_limit_seconds"] == report["budget"]["elapsed_limit_seconds"],
                "documented same-window budget change must preserve the window and time limit")
        require(len(changed["history"]) == 1 and changed["revision"] > report["budget"]["revision"],
                "documented budget change must remain in history and advance the budget revision")


if __name__ == "__main__":
    try:
        check()
    except (AssertionError, OSError, ValueError, subprocess.TimeoutExpired) as error:
        print(f"FAIL: {error}", file=sys.stderr)
        raise SystemExit(1)
    print("PASS: published declaration/config and budget command execute through /3 records and status")
