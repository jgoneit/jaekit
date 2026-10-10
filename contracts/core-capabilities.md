# 계약: Core 지원 정보와 Seal 호환 확인

이 계약은 미배포 개발 조합인 Core `0.1.3-dev`·Seal `0.1.9`에 적용한다. 현재 배포판 `v0.1.2`에는 이 조회 명령이 없다. 개발 기능을 쓰기 위해 단순 업데이트만 하면 된다는 뜻이 아니며, 공개 배포와 설치본 확인은 별도다. Spec은 Core 설치나 이 조회 없이 문서를 작성한다.

## 1. 목표 없는 조회

```text
ha capabilities --format json
```

`--format` 생략 시에도 JSON을 출력한다. 목표 인수와 JSON 이외의 형식은 받지 않는다. 이 실행 파일이 지원하는 내용을 stdout에 JSON 객체 하나와 줄바꿈으로 출력한다. 목표·저장소·설정·실행 기록을 열거나 바꾸지 않고, Git·검사·모델 프로세스도 시작하지 않는다. 같은 바이너리의 조회 내용은 현재 시각이나 작업 폴더에 따라 달라지지 않는다.

종료 코드는 성공 `0`, 잘못된 인수 `64`, 출력 실패 `70`이다. 구 Core의 알 수 없는 명령 종료 `64`는 조회 미지원이며, 버전 출력 성공으로 대신 통과시키지 않는다. 다른 실패·시간 초과·읽을 수 없는 출력도 지원 확인 성공이 아니다.

```json
{
  "schema": "ha-capabilities/v1",
  "ha_version": "0.1.3-dev",
  "supported_rules": ["run-rules/1", "run-rules/2", "run-rules/3"],
  "default_rules": "run-rules/3",
  "formats": {
    "criteria": ["legacy", "nested/1"],
    "check_declaration": ["check-declaration/v1"],
    "check_result": ["check-result/v1"],
    "check_evidence": ["check-evidence/v1"]
  },
  "features": [
    "estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1",
    "structured-change-results/v1", "budget-change/v1"
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
| `features` | 아래 기능 식별자 목록 |

위 필드는 모두 필수다. 목록은 중복 없는 문자열 배열이며 각 문자열은 비어 있지 않다. 빈 형식·기능 목록은 해당 지원이 없다는 뜻이다. 중복 JSON 키, 잘못된 타입, 필수 정보 누락, 손상·뒤에 붙은 JSON은 해석 불가다. 같은 schema의 추가 필드·새 목록 항목은 허용하되 소비자가 아는 필수 지원을 대신하지 않는다.

| 기능 | 약속 |
| --- | --- |
| `estimate/v1` | 기록을 바꾸지 않는 시작 전 최소 검증 횟수 추정 |
| `dirty-preflight/v1` | 실제 검사 대상 실행 전에 dirty 상태와 조회 실패를 구분해 거부 |
| `baseline-safe-copy/v1` | 모든 지원 규칙의 앞으로 실행할 baseline에서 대상별 전체 복사 목록을 확인하고, HEAD 비일반 파일과 목적지·부모 symlink를 실행 전에 거부 |
| `structured-change-results/v1` | `/3` change의 호출에 연결된 구조화 분류와 전체 시도 이력 보존 |
| `budget-change/v1` | `/3` 현재 창의 횟수 총상한 변경, 사용량·시간 상한 보존과 개정 충돌 거부 |

기능의 상세 의미는 [실행 기록](run-record.md), [결과 계약](check-result.md), [목표 문서](goal-docs.md)에 따른다. 지원 정보는 독립 인증이나 코드 무결성 증명이 아니다.

## 2. Seal 0.1.9의 소비 계약

호환 확인은 상태를 바꾸는 작업 전에 수행한다. 실제 사용할 실행 파일의 절대 경로를 확정하고 **그 경로**로 조회한다. 성공한 실행 파일과 이후 사용할 Core가 달라지면 이전 확인을 재사용하지 않는다. PATH의 다른 파일, 다른 세션의 결과, 버전 문자열만으로 대체하지 않는다. 확인 뒤에도 경로·바이너리가 바뀌었으면 다시 확인한다. 조회는 자동 설치·권한 확대·목표 또는 기록 변경의 권한을 주지 않는다.

에이전트는 위 JSON을 아래 표와 직접 비교할 수 있다. Python이나 별도 호환 검사 프로그램은 Seal의 필수 의존성이 아니다.

| 요청 | 선택할 규칙과 필요한 지원 |
| --- | --- |
| 새 시작 | `default_rules`가 `run-rules/3`이며 지원 목록에도 있어야 한다. `criteria`의 `legacy`·`nested/1`, 선언·보고·요약의 각 v1, 위 다섯 기능이 모두 필요하다 |
| 기존 목표 재개·검사 | `start`에 저장된 `/1`·`/2`·`/3`과 목표가 선택한 criteria 형식을 지원해야 한다. `dirty-preflight/v1`, `baseline-safe-copy/v1`이 필요하다. 저장 규칙이 `/3`이면 선언·보고·요약 v1과 `structured-change-results/v1`도 필요하다 |
| 기존 목표 횟수 상한 변경 | 저장 규칙이 `/3`이고 `budget-change/v1`을 지원해야 한다. 해당 목표의 criteria 형식과 `/3` 선언·보고·요약 형식도 지원해야 한다. `/1`·`/2`에는 이 동작을 적용하지 않는다 |

새 시작에서 모르는 기본 규칙이나 이전 기본 규칙으로 조용히 대체하지 않는다. 기존 목표는 기본 시작 규칙을 적용하지 않는다. 예를 들어 기본이 미래 `/4`여도 저장된 `/2`와 요청에 필요한 지원이 있다면 기존 `/2`의 확인은 통과할 수 있다. 새 `/3` 지원 때문에 `/1`·`/2` 목표나 기록을 변환하거나 결과 선언을 추가하도록 요구하지 않는다.

조회 미지원·실패, 필수 정보 누락·해석 불가, 필요한 규칙·형식·기능 부족이면 확인한 대상과 이유를 알리고 새 시작·재개 등 상태 변경 전에 멈춘다. 호환 배포판이 아직 없으면 현재 `v0.1.2`를 다시 설치하면 해결된다고 안내하지 않는다.

호환 확인은 입력 lint, 저장 기록의 무결성, 사용자 구현 권한, 완료 판정을 대신하지 않는다. Core 자체에 새 시작 강제 차단을 추가하는 계약도 아니다. 직접 Core를 사용하는 미선언 `/3` change는 기존처럼 실제 시도를 기록하고 `unknown`으로 분류할 수 있다. 일반 maintain·manual·task 검사는 구조화 change 계약으로 강제 이전하지 않는다.

## 3. 선택적 결정적 소비 도구

Python 3가 이미 있다면 Seal에 포함된 [check_core.py](../plugins/seal/skills/seal/scripts/check_core.py)로 같은 계약을 확인할 수 있다. 도구가 없거나 Python을 쓰지 않는 경우에도 위 계약을 직접 소비할 수 있다.

```text
python3 <seal-skill>/scripts/check_core.py --ha <실제 Core 경로> --operation start
python3 <seal-skill>/scripts/check_core.py --ha <실제 Core 경로> --operation resume --goal <기존 목표>
```

`--ha`는 기본 `ha`이며 PATH에서 처음 찾은 실행 파일을 사용한다. `--operation`은 `start`(기본), `resume`, `check`, `budget`이다. `start`에는 목표를 받지 않고 나머지 요청에는 기존 목표를 받는다. 기존 목표의 첫 `start`와 SPEC 형식 선택자만 읽으며 실제 기록 무결성 검증은 Core에 남긴다.

도구는 symlink를 해소한 절대 경로로 한 번 조회한다. 조회 전후 SHA-256이 다르면 거부한다. 성공 시 응답의 `executable` 경로를 실제 Core 호출에도 사용한다. 해시는 영구 캐시나 비협조 외부 프로세스에 대한 교체 방지 보증이 아니다.

stdout은 `seal-core-compatibility/v1` JSON이다. `compatible`, `reason`, `requested_executable`, `executable`, `sha256`, `ha_version`, `operation`, `rules`, `missing`을 제공한다. 아직 확인하지 못한 값은 null이며, 원문 Core 출력·목표 내용·사용자 발화를 복사하지 않는다. 실행 파일 경로는 로컬 진단 정보이므로 공개 보고에 그대로 옮길 필요가 없다.

- 종료 `0`: 호환 확인 성공.
- 종료 `2`: 확인 거부. `core_missing`, `query_unsupported`, `query_failed`, `query_timeout`, `binary_changed`, `unsupported_schema`, `invalid_capabilities`, `unsupported_default_rule`, `unsupported_stored_rule`, `unsupported_operation`, `missing_support`, `goal_unreadable`, `goal_format_invalid` 중 사유를 제공한다.
- 종료 `64`: 도구 인수 오류.

조회는 5초 후 중단하며 자동 재시도하지 않는다. 이 도구는 실제 시작·재개·검사·설치를 실행하거나 결과를 파일로 저장하지 않는다. 설치된 Codex·Claude가 이 지침을 실제로 따르는지는 별도 호스트 검증 대상이다.
