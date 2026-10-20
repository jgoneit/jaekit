# 계약: Core 지원 정보와 Seal 호환 확인

이 계약은 v0.1.6의 Core `0.1.6`·Seal `0.1.17`에 적용한다. 지원 정보 조회는 v0.1.3에서 도입했으며 이전 배포판 `v0.1.2`에는 이 명령이 없다. 업데이트 뒤에도 실제 실행 파일의 지원 정보를 확인하며 버전 숫자만으로 호환성을 판단하지 않는다. Spec은 Core 설치나 이 조회 없이 문서를 작성한다.

## 1. 목표 없는 조회

```text
ha capabilities --format json
```

`--format` 생략 시에도 JSON을 출력한다. 목표 인수와 JSON 이외의 형식은 받지 않는다. 이 실행 파일이 지원하는 내용을 stdout에 JSON 객체 하나와 줄바꿈으로 출력한다. 목표·저장소·설정·실행 기록을 열거나 바꾸지 않고, Git·검사·모델 프로세스도 시작하지 않는다. 같은 바이너리의 조회 내용은 현재 시각이나 작업 폴더에 따라 달라지지 않는다.

종료 코드는 성공 `0`, 잘못된 인수 `64`, 출력 실패 `70`이다. 구 Core의 알 수 없는 명령 종료 `64`는 조회 미지원이며, 버전 출력 성공으로 대신 통과시키지 않는다. 다른 실패·시간 초과·읽을 수 없는 출력도 지원 확인 성공이 아니다.

```json
{
  "schema": "ha-capabilities/v1",
  "ha_version": "0.1.6",
  "supported_rules": ["run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4"],
  "default_rules": "run-rules/3",
  "formats": {
    "criteria": ["legacy", "nested/1"],
    "check_declaration": ["check-declaration/v1"],
    "check_result": ["check-result/v1"],
    "check_evidence": ["check-evidence/v1"],
    "finding": ["run-finding/v1"],
    "decision_history": ["decision-history/v1"],
    "timing": ["timing/v1"]
  },
  "features": [
    "estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1",
    "structured-change-results/v1", "budget-change/v1",
    "post-completion-findings/v1", "decision-history/v1", "run-timing/v1",
    "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1",
    "error-recovery/v1", "command-time-limits/v1"
  ]
}
```

| 필드 | 의미 |
| --- | --- |
| `schema` | 정확히 `ha-capabilities/v1`. 다른 schema는 이 계약으로 해석하지 않는다 |
| `ha_version` | 조회한 Core의 비어 있지 않은 버전 식별자. 숫자 비교만으로 기능을 추정하지 않는다 |
| `supported_rules` | 저장된 목표에서 지원하는 규칙 목록 |
| `default_rules` | 새 `start`가 선택하는 규칙. `supported_rules` 안에 있어야 한다 |
| `formats.criteria` | `legacy`는 선택자 없는 기존 문서, `nested/1`은 명시적으로 선택한 새 목록 형식 |
| `formats.check_declaration`, `check_result`, `check_evidence` | 지원하는 선언·생산자 보고·저장 요약의 schema 목록 |
| `formats.timing` | 시간 보기(`ha timing --format json`)의 schema 목록. 선택적이며 없을 수 있다 |
| `formats.assurance_tier` | 목표 문서의 `Assurance:` 줄에서 읽고 검증하는 값 목록(`light`, `standard`, `strict`). 선택적이며 없을 수 있다 |
| `formats.core_requirements` | `ha start --skill`이 읽는 skill의 Core 요구 선언 schema 목록(`core-requirements/v1`, §8). 선택적이며 없을 수 있다 |
| `features` | 아래 기능 식별자 목록 |

선택적이라고 적은 `formats.timing`, `formats.assurance_tier`, `formats.core_requirements`를 뺀 위 필드는 모두 필수다. 목록은 중복 없는 문자열 배열이며 각 문자열은 비어 있지 않다. 빈 형식·기능 목록은 해당 지원이 없다는 뜻이다. 중복 JSON 키, 잘못된 타입, 필수 정보 누락, 손상·뒤에 붙은 JSON은 해석 불가다. 같은 schema의 추가 필드·새 목록 항목은 허용하되 소비자가 아는 필수 지원을 대신하지 않는다.

| 기능 | 약속 |
| --- | --- |
| `estimate/v1` | 기록을 바꾸지 않는 시작 전 최소 검증 횟수 추정 |
| `dirty-preflight/v1` | 실제 검사 대상 실행 전에 dirty 상태와 조회 실패를 구분해 거부 |
| `baseline-safe-copy/v1` | 모든 지원 규칙의 앞으로 실행할 baseline에서 대상별 전체 복사 목록을 확인하고, HEAD 비일반 파일과 목적지·부모 symlink를 실행 전에 거부 |
| `structured-change-results/v1` | `/3` change의 호출에 연결된 구조화 분류와 전체 시도 이력 보존 |
| `budget-change/v1` | `/3` 현재 창의 횟수 총상한 변경, 사용량·시간 상한 보존과 개정 충돌 거부 |
| `post-completion-findings/v1` | 역사적 완료에 연결된 발견 주장·개정·재작업 관계를 보존하는 선택적 확장 |
| `decision-history/v1` | 실행 기록과 분리된 판단 이력의 등록·관계·조회를 제공하는 선택적 확장(§5) |
| `run-timing/v1` | `check`·`baseline` 줄의 `timing` 측정값 기록과 읽기 전용 시간 보기(`ha timing`)를 제공하는 선택적 확장 |
| `test-authority/v1` | `run-rules/4`의 시험 변경 권한·시작 범위 사본과 `test-change` 근거(§6) |
| `reported-wait/v1` | `run-rules/4`의 `wait`·`resume` 신고와 `started_at`, 신고한 대기를 뺀 예산 경과(§6) |
| `preapproved-runs/v1` | `run-rules/4`의 사전 승인 상한과 그 안의 이유만 있는 증액(§6) |
| `error-recovery/v1` | `run-rules/4`의 `recover`·`allow-recovery`와 회복 횟수 표시(§6) |
| `command-time-limits/v1` | `run-rules/4`의 PLAN `## 시간 제한`, 명령 digest의 적용 제한, 실행 전 안내(§6) |
| `runner-adapters/v1` | `/3`·`/4` change 조건의 `check-adapter/v1` 선언으로 `go test -json` 표준 출력과 JUnit XML을 직접 분류하는 선택적 확장(§7) |
| `auto-budget/v1` | PLAN `검증 실행: 자동`의 시작 상한 산정, 사용자 설정의 사전 승인 상한, 그 상한을 넘는 시작의 거부와 `ha estimate`의 표시(§8, [실행 기록](run-record.md) §1.3) |
| `core-requirements/v1` | `ha start --skill`의 skill Core 요구 선언 확인과 선언을 필수로 하는 `--check-skill`(§8) |

시간 보기를 지원하는 Core는 `formats.timing`에 `timing/v1`, `features`에 `run-timing/v1`을 제공한다([실행 기록](run-record.md) §1.2, §2.2). 측정값은 판정·예산에 쓰이지 않으므로 이 확장은 새 시작이나 기존 목표의 재개·검사·예산·발견 작업의 요구에 추가되지 않는다. 지원이 없는 Core에서도 일반 실행은 그대로이고, 시간 보기만 쓸 수 없다.

목표 문서의 `Assurance:` 줄([목표 문서](goal-docs.md) §2.5)을 읽고 검증하는 Core는 `formats.assurance_tier`에 `light`, `standard`, `strict`를 제공한다. v0.1.6 배포판(위 예시)에는 이 항목이 없다. 등급은 구현 담당이 받을 검토의 범위를 정할 뿐이며, 판정, `ha status`와 `ha done`의 출력, 예산은 등급 때문에 바뀌지 않는다. 줄은 SPEC.md의 일부이므로 목표 digest에 들어가고, 올바른 줄이 있으면 `ha lint` 성공 출력에 `assurance tier: <등급>` 줄이 더해지며, 형식이 틀린 줄은 다른 lint 오류처럼 `ha start`와 `ha check`를 막는다([목표 문서](goal-docs.md) §2.5, [실행 묶음](bundle.md) §8). 그래서 이 항목은 새 시작이나 기존 목표의 재개·검사·예산·발견 작업의 요구에 더하지 않는다. 항목이 없는 Core는 줄을 검증하지 않을 뿐이고 일반 실행은 그대로다. 이 항목의 이름은 `ha status`의 `assurance` 필드(근거 표시, 항상 `local`)와 섞이지 않도록 따로 정했다.

기능의 상세 의미는 [실행 기록](run-record.md), [결과 계약](check-result.md), [목표 문서](goal-docs.md)에 따른다. 지원 정보는 독립 인증이나 코드 무결성 증명이 아니다.

## 2. Seal 0.1.17의 소비 계약

호환 확인은 상태를 바꾸는 작업 전에 수행한다. 실제 사용할 실행 파일의 절대 경로를 확정하고 **그 경로**로 조회한다. 성공한 실행 파일과 이후 사용할 Core가 달라지면 이전 확인을 재사용하지 않는다. PATH의 다른 파일, 다른 세션의 결과, 버전 문자열만으로 대체하지 않는다. 확인 뒤에도 경로·바이너리가 바뀌었으면 다시 확인한다. 조회는 자동 설치·권한 확대·목표 또는 기록 변경의 권한을 주지 않는다.

에이전트는 위 JSON을 아래 표와 직접 비교할 수 있다. Python이나 별도 호환 검사 프로그램은 Seal의 필수 의존성이 아니다.

| 요청 | 선택할 규칙과 필요한 지원 |
| --- | --- |
| 새 시작 | `default_rules`가 `run-rules/3`이며 지원 목록에도 있어야 한다. `criteria`의 `legacy`·`nested/1`, 선언·보고·요약의 각 v1, 완료 뒤 발견 확장을 제외한 위 다섯 실행 기능이 모두 필요하다. Seal 0.1.16부터 지원 목록에 `run-rules/4`, 기능에 §6의 다섯도 요구하고 `ha start --rules run-rules/4`로 시작한다 |
| 기존 목표 재개·검사 | `start`에 저장된 `/1`·`/2`·`/3`·`/4`와 목표가 선택한 criteria 형식을 지원해야 한다. `dirty-preflight/v1`, `baseline-safe-copy/v1`이 필요하다. 저장 규칙이 `/3`·`/4`면 선언·보고·요약 v1과 `structured-change-results/v1`도 필요하고, `/4`면 §6의 다섯 기능도 필요하다 |
| 기존 목표 횟수 상한 변경 | 저장 규칙이 `/3`·`/4`이고 `budget-change/v1`을 지원해야 한다. 해당 목표의 criteria 형식과 선언·보고·요약 형식도 지원해야 하고, `/4`면 §6의 다섯 기능도 필요하다. `/1`·`/2`에는 이 동작을 적용하지 않는다 |

새 시작에서 모르는 기본 규칙이나 이전 기본 규칙으로 조용히 대체하지 않는다. 기존 목표는 기본 시작 규칙을 적용하지 않는다. 예를 들어 기본이 미래 `/4`여도 저장된 `/2`와 요청에 필요한 지원이 있다면 기존 `/2`의 확인은 통과할 수 있다. 새 `/3` 지원 때문에 `/1`·`/2` 목표나 기록을 변환하거나 결과 선언을 추가하도록 요구하지 않는다.

조회 미지원·실패, 필수 정보 누락·해석 불가, 필요한 규칙·형식·기능 부족이면 확인한 대상과 이유를 알리고 새 시작·재개 등 상태 변경 전에 멈춘다. 구버전이면 [설치 안내](../guides/INSTALL.md#업데이트)를 안내하되 자동 설치·업데이트하거나 구규칙으로 우회하지 않는다.

호환 확인은 입력 lint, 저장 기록의 무결성, 사용자 구현 권한, 완료 판정을 대신하지 않는다. 이 표의 확인은 Seal이 한다. Core는 skill이 §8의 선언을 두면 `ha start --skill`에서 그 선언을 이 실행 파일의 지원 정보와 대조해 빠진 지원이 있으면 기록 없이 시작을 거부하고, 선언이 없는 skill의 시작은 그대로 받는다. 직접 Core를 사용하는 미선언 `/3` change는 기존처럼 실제 시도를 기록하고 `unknown`으로 분류할 수 있다. 일반 maintain·manual·task 검사는 구조화 change 계약으로 강제 이전하지 않는다.

## 3. 선택적 결정적 소비 도구

Python 3가 이미 있다면 Seal에 포함된 [check_core.py](https://github.com/jgoneit/jaekit/blob/v0.1.6/plugins/seal/skills/seal/scripts/check_core.py)로 같은 계약을 확인할 수 있다. 도구가 없거나 Python을 쓰지 않는 경우에도 위 계약을 직접 소비할 수 있다.

```text
python3 <seal-skill>/scripts/check_core.py --ha <실제 Core 경로> --operation start
python3 <seal-skill>/scripts/check_core.py --ha <실제 Core 경로> --operation resume --goal <기존 목표>
```

`--ha`는 기본 `ha`이며 PATH에서 처음 찾은 실행 파일을 사용한다. `--operation`은 `start`(기본), `resume`, `check`, `budget`, `finding`이다. `start`에는 목표를 받지 않고 나머지 요청에는 기존 목표를 받는다. 기존 목표의 첫 `start`, 기록의 schema와 SPEC 형식 선택자를 읽으며 실제 기록 무결성 검증은 Core에 남긴다.

도구는 symlink를 해소한 절대 경로로 한 번 조회한다. 조회 전후 SHA-256이 다르면 거부한다. 성공 시 응답의 `executable` 경로를 실제 Core 호출에도 사용한다. 해시는 영구 캐시나 비협조 외부 프로세스에 대한 교체 방지 보증이 아니다.

stdout은 `seal-core-compatibility/v1` JSON이다. `compatible`, `reason`, `requested_executable`, `executable`, `sha256`, `ha_version`, `operation`, `rules`, `missing`을 제공한다. 아직 확인하지 못한 값은 null이며, 원문 Core 출력·목표 내용·사용자 발화를 복사하지 않는다. 실행 파일 경로는 로컬 진단 정보이므로 공개 보고에 그대로 옮길 필요가 없다.

- 종료 `0`: 호환 확인 성공.
- 종료 `2`: 확인 거부. `core_missing`, `query_unsupported`, `query_failed`, `query_timeout`, `binary_changed`, `unsupported_schema`, `invalid_capabilities`, `unsupported_default_rule`, `unsupported_stored_rule`, `unsupported_record_schema`, `unsupported_operation`, `missing_support`, `goal_unreadable`, `goal_record_oversized`, `goal_format_invalid` 중 사유를 제공한다. 저장된 발견 줄이 [계약 상한](findings.md#입력과-조회)을 넘으면 `goal_record_oversized`, 읽을 수 없거나 손상된 기록은 `goal_unreadable`, 알 수 없는 기록 schema는 `unsupported_record_schema`다.
- 종료 `64`: 도구 인수 오류.

조회는 5초 후 중단하며 자동 재시도하지 않는다. 이 도구는 실제 시작·재개·검사·설치를 실행하거나 결과를 파일로 저장하지 않는다. 설치된 Codex·Claude가 이 지침을 실제로 따르는지는 별도 호스트 검증 대상이다.

## 4. 선택적 완료 뒤 발견 확장

Core 0.1.4부터 [완료 뒤 발견](findings.md)을 포함하며 `formats.finding`에 `run-finding/v1`, `features`에 `post-completion-findings/v1`을 제공한다. 위 v0.1.6 배포 예시에도 두 항목이 있다. v0.1.3 Core에는 이 확장이 없으므로 실제 지원 정보를 확인하며, 버전 숫자만으로 기능을 추정하지 않는다. 기존 시작·검사·예산 작업의 요구는 유지한다. 발견 입력과 조회에는 실제 실행 파일의 두 추가 지원, 저장된 규칙과 현재 조건 형식의 지원을 확인한다. 발견 작업만을 위해 실행 기능이나 `/3` 선언 지원을 요구하지 않는다.

Seal 0.1.12에 포함된 선택형 도구는 `--operation finding --goal <기존 목표>`를 지원한다. 기존 목표에 `run-finding/v1` 줄이 있으면 resume·check·budget도 두 추가 지원을 요구한다. 도구는 알려진 줄 schema의 존재를 읽으며 해시·이력의 무결성은 Core가 검증한다. 알 수 없는 schema는 거부한다. 이전 설치의 도구에 새 인수가 있다고 가정하지 않으며, 실제 capability를 직접 확인하는 방법도 유효하다. 지원을 확인하지 못하면 새 발견 기록을 쓰지 않는다.

## 5. 선택적 판단 이력 확장

[판단 이력](decision-history.md)을 지원하는 Core는 `formats.decision_history`에 `decision-history/v1`, `features`에 `decision-history/v1`을 제공한다. 배포판 v0.1.5(`ha 0.1.5`)부터 이 확장을 제공하며 v0.1.4(`ha 0.1.4`)에는 없다. 이력의 등록과 조회에는 실제 실행 파일의 두 추가 지원, 저장된 시작 규칙과 현재 조건 형식의 지원을 확인한다. 판단 이력은 `runs.jsonl` 밖에 있으므로 이 확장은 새 시작이나 기존 목표의 재개·검사·예산·발견 작업의 요구에 추가되지 않는다. 지원이 없으면 이력을 쓰지 않고 기존 Markdown으로 인계한다.

Seal 0.1.13에 포함된 선택형 도구는 `--operation history --goal <기존 목표>`로 같은 확인을 한다. 이전 설치의 도구에 이 인수가 있다고 가정하지 않으며, 실제 capability를 직접 확인하는 방법도 유효하다.

현재 소스의 macOS·Linux 실행 파일과 Windows x64 로컬 후보는 두 판단 이력 항목을 같은 의미로 제공한다. Windows에서도 등록·재전송·정정·기본/current·list·두 방식의 show가 대상이며, 지원 정보만 있고 직접 호출은 일괄 미지원인 형태가 아니다. 이 기능 광고가 공개 Windows 릴리스나 Windows 11 설치·호스트 사용 인수를 뜻하지는 않는다. 플랫폼과 저장소 범위는 [판단 이력 계약](decision-history.md#1-지원과-호환)과 [Windows 후보 안내](../guides/WINDOWS.md)에 따른다.

## 6. run-rules/4 기능

`run-rules/4`를 지원하는 Core는 `supported_rules`에 `run-rules/4`, `features`에 `test-authority/v1`, `reported-wait/v1`, `preapproved-runs/v1`, `error-recovery/v1`, `command-time-limits/v1`을 함께 제공한다. `default_rules`는 `run-rules/3` 그대로다. 그래서 기본 규칙을 확인하는 이전 Seal과 배포·설치 식별은 바뀌지 않는다. 새 규칙은 `ha start --rules run-rules/4`로만 고른다. 다섯 기능의 의미는 [실행 기록](run-record.md) §4.7이다.

Seal 0.1.16부터 확인 도구는 새 시작에서 이 규칙과 다섯 기능을 모두 요구하고 시작 규칙으로 `run-rules/4`를 낸다. 하나라도 없으면 기록 전에 거부한다. 저장 규칙이 `/4`인 목표의 재개·검사·횟수 상한 변경도 다섯 기능을 요구한다. `/1`·`/2`·`/3` 목표의 요구는 그대로다. `/4`를 지원하지 않는 Core는 그 목표를 66으로 거부하고 기록하지 않는다.

## 7. 러너 어댑터

러너 어댑터를 지원하는 Core는 `formats.check_declaration`의 끝에 `check-adapter/v1`, `features`의 끝에 `runner-adapters/v1`을 더해 제공한다. 두 항목의 의미는 [change 검사 결과](check-result.md#5-러너-어댑터) §5다. 기존 항목과 그 순서, `default_rules`는 그대로다.

어댑터는 선택이므로 이 확장은 새 시작이나 기존 목표의 재개·검사·예산·발견 작업의 요구에 추가되지 않는다. Seal과 호환 확인 도구의 요구도 그대로다. 실행자는 실제 실행 파일의 지원 정보에 두 항목이 모두 있을 때만 어댑터 선언을 쓴다. 지원이 없는 Core는 어댑터 선언을 알 수 없는 선언으로 보고 lint 문제(`evidence_invalid`)로 시작과 검사를 거부하며 기록하지 않는다. 그때는 참조 생산자나 전용 생산자를 쓴다.

## 8. skill의 Core 요구 선언

skill은 SKILL.md와 같은 폴더의 `core-requirements.json`에 자신에게 필요한 Core 지원을 선언할 수 있다. `ha start --skill <SKILL.md>`가 이 선언을 읽어 실행 파일의 지원 정보(§1의 `ha capabilities`와 같은 값)와 대조한다([실행 기록](run-record.md) §1.4). 지원하는 Core는 `formats.core_requirements`에 `core-requirements/v1`, `features`에 `core-requirements/v1`을 제공한다.

```json
{
  "schema": "core-requirements/v1",
  "rules": ["run-rules/4"],
  "formats": {"criteria": ["legacy", "nested/1"], "check_result": ["check-result/v1"]},
  "features": ["estimate/v1", "test-authority/v1"]
}
```

| 필드 | 의미 |
| --- | --- |
| `schema` | 필수. 정확히 `core-requirements/v1` |
| `rules` | 선택. 각 값이 `supported_rules`에 있어야 한다 |
| `formats` | 선택. 키마다 그 키가 `formats`에 있고(목록이 비어 있어도) 각 값이 그 목록에 있어야 한다 |
| `features` | 선택. 각 값이 `features`에 있어야 한다 |

- 목록은 비어 있지 않은 문자열의 배열이고 한 목록 안에서 값이 겹치지 않는다. `formats`는 그런 목록을 값으로 갖는 객체다. 빈 목록이나 없는 필드는 그 종류의 요구가 없다는 뜻이다. 단 `formats`의 키는 목록이 비어 있어도 그 키 자체를 요구한다.
- UTF-8 JSON 객체 하나이며 64 KiB 이하다. 맨 앞 BOM 하나는 허용한다. 필드 이름은 정확한 철자여야 하며 대소문자만 다른 이름도 모르는 필드다. 다른 schema 값, 모르는 필드, 중복 키, 뒤에 붙은 데이터, `null`, 잘못된 타입은 형식 위반이다. 대상이 없는 symlink나 폴더처럼 읽을 수 없는 항목도 거부한다.
- 빠진 항목이 있으면 항목마다 `ha: missing Core support: <rules|formats.<키>|features> <값> (ha <버전>)`을 보이고(`formats`의 키 자체가 없고 그 목록이 비어 있으면 값 없이 `formats.<키>`), 형식 위반이면 그 이유를 보이며, 둘 다 기록 없이 65로 끝난다. 모르는 값을 무시하고 시작하지 않는다.
- 선언 파일이 없는 skill과 `--skill` 없는 시작은 이 확인을 하지 않는다. 그래서 선언하지 않은 Seal 0.1.17~0.1.19는 지금처럼 시작하며, 그 Seal들의 §2 확인과 [check_core.py](https://github.com/jgoneit/jaekit/blob/v0.1.6/plugins/seal/skills/seal/scripts/check_core.py)는 추가된 항목을 허용하므로 같은 결론을 낸다.
- `--check-skill`은 선언을 필수로 한다. 이 기능 전의 Core(배포판 v0.1.6 등)는 이 플래그를 모르는 플래그로 64에 끝내고 기록하지 않으므로, 확인이 반드시 일어나야 하는 시작은 `ha start <goal> --skill <SKILL.md> --check-skill` 형식을 쓴다.
- 선언은 Core 지원의 요구일 뿐이다. 사용자 구현 권한, 입력 lint, 완료 판정을 대신하지 않는다.

`auto-budget/v1`은 PLAN `검증 실행: 자동`의 시작 상한 산정을 제공한다는 뜻이다([실행 기록](run-record.md) §1.3). 두 기능은 선택이므로 §2 표의 새 시작이나 기존 목표의 재개·검사·예산·발견 작업의 요구에 더하지 않는다. skill이 쓰려면 §8의 선언에 적는다.
