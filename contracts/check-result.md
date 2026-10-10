# 계약: change 검사 결과

`run-rules/3`의 change 조건은 baseline과 현재 검사를 같은 선언에 연결한다. 이 계약은 일반 nonzero를 의도한 요구 위반으로 간주하지 않는다. `/1`·`/2`의 기존 실행과 일반 maintain·manual·task 검사의 역할은 그대로다. 새 시작은 `/3`을 사용한다([실행 기록](run-record.md)).

## 1. 선언과 입력

검사 작성자는 실행 전에 요구 대상과 예상 위반을 JSON 파일에 선언하고 PLAN의 `## 결과 계약` 표에서 연결한다.

```markdown
## 결과 계약
| ID | 선언 경로 |
| --- | --- |
| AC-1 | checks/declaration.json |
```

선언 파일과 `producer_paths`의 모든 파일은 조건표의 `검사 경로`에 포함한다. 모두 저장소 상대 경로의 추적된 일반 파일이어야 하며 symlink를 사용하지 않는다. 선언과 생산자·설정 파일의 현재 바이트는 HEAD와 같아야 한다. 다른 실행 문서 제외 경로 안에 있더라도 미커밋 내용으로 분류 의미를 바꿀 수 없다. baseline은 이 파일들을 HEAD에서 기준 commit으로 덧씌운다. 임시 worktree의 의존성과 환경을 자동으로 설치하지 않는다.

아래 선언과 참조 설정은 가상 greeting 예시다. 실제 사용자 자료가 아니다.

### 선언 예시 (`checks/declaration.json`)

```json
{
  "schema": "check-declaration/v1",
  "producer": "jaekit-reference",
  "producer_version": "1",
  "producer_paths": ["tools/check-result-reference.py", "checks/reference.json"],
  "targets": [{"id": "greeting", "violation": "missing-greeting"}]
}
```

`producer`, `producer_version`, 대상 `id`·`violation`은 ASCII 영숫자로 시작하는 1~128자의 ASCII 영숫자·밑줄·점·콜론·슬래시·하이픈 식별자다. 생산자 경로는 1~128개, 대상은 1~1024개다. `targets`의 각 `id`는 고유하며 모든 대상이 필수다. `violation`은 해당 요구가 충족되지 않았을 때 관측할 식별자다. 선언·대상·결과 계약·생산자 버전과 내용은 근거의 정체성에 포함된다. 명령 문자열이 같아도 이 입력이 바뀌면 이전 근거와 완료를 그대로 재사용하지 않는다.

## 2. 호출별 보고

`ha`가 검사 프로세스에 다음 환경 변수를 전달한다.

| 변수 | 의미 |
| --- | --- |
| `HA_EVIDENCE_PATH` | 이번 호출의 구조화 보고를 쓸 로컬 파일 |
| `HA_EVIDENCE_INVOCATION` | 이번 호출의 식별자 |
| `HA_EVIDENCE_DECLARATION_DIGEST` | 이번 선언과 생산자 입력의 digest |

생산자는 보고에 호출 식별자와 digest를 그대로 연결한다. 이전 호출의 파일, 다른 선언·대상·생산자의 보고는 유효하지 않다. stdout/stderr는 기존 로컬 실행 로그로 남으며 보고 파일을 대신하지 않는다.

```json
{
  "schema": "check-result/v1",
  "invocation": "<HA_EVIDENCE_INVOCATION>",
  "declaration_digest": "<HA_EVIDENCE_DECLARATION_DIGEST>",
  "producer": "jaekit-reference",
  "producer_version": "1",
  "attempts_complete": true,
  "observations": [{"target": "greeting", "attempt": 1, "status": "violation", "violation": "missing-greeting"}]
}
```

| `status` | 관측과 추가 필드 |
| --- | --- |
| `pass` | 대상 실행과 통과. `violation`·`reason`을 넣지 않는다 |
| `violation` | 실제 assertion 실패. `violation`에 관측한 위반 식별자를 넣는다 |
| `error` | 검사를 방해한 오류. `reason`은 `dependency_missing`, `collection_error`, `import_error`, `compile_error`, `browser_start_error`, `setup_error`, `execution_error`, `permission_denied` 중 하나 |
| `skip` | 대상이 실행되지 않음. `reason`은 `skipped` 또는 `not_selected` |

참조 생산자는 내부 재시도 없이 대상마다 한 번 관측한다. 공통 계약에서 재시도를 보고하려면 `attempts_complete: true`와 대상별 1부터 빠짐없이 증가하는 전체 `attempt` 이력이 필요하다. 전체 관측은 최대 4096개다. 마지막 attempt만 남거나 같은 번호가 중복되거나 순서가 역전되면 `attempt_history_invalid`, 완전성 표시가 없으면 `attempt_history_missing`으로 판정 불가다. 같은 대상의 pass와 violation이 함께 있으면 최종 exit 0이어도 `fail`·`inconsistent_attempts`로 불리한 결과를 보존한다. 재시도 정보를 확보하지 못하는 연동은 미지원 또는 판정 불가다.

선언과 보고는 1 MiB 이하의 JSON 객체여야 하며 중복 키·알 수 없는 필드·뒤에 붙은 데이터·지원하지 않는 schema는 거부한다. 보고 파일도 일반 파일이어야 한다. 미지원 일반 스크립트는 선언 없이 실제 시도를 기록할 수 있지만 `/3` change의 양의 근거는 얻지 못한다.

## 3. 분류와 보존

| 단계 | 저장하는 `result` | 의미 |
| --- | --- | --- |
| 현재 검사 | `pass` | 정상 종료와 모든 대상 통과 |
| 현재 검사 | `fail` | 실제 assertion 실패 |
| baseline | `fail_as_expected` | 선언한 위반 관측과 비정상 종료, 다른 실패·오류·미실행 없음 |
| baseline | `unexpected_pass` | 기준 상태에서도 모든 대상 통과 |
| baseline | `fail` | 의도와 다른 실제 assertion 실패 또는 내부 시도의 pass·violation 불일치 |
| 둘 다 | `error` | 프로세스 시작·시간 초과·신호·출력 저장 오류 또는 보고된 환경 오류 |
| 둘 다 | `unknown` | 미지원·누락·충돌·미실행 등으로 분류 불가 |

간결한 근거의 `reason`은 분류 이유 코드다. 예를 들어 `unsupported_declaration`, `report_missing`, `report_binding`, `attempt_history_missing`, `attempt_history_invalid`, `target_missing`, `target_skipped`, `report_exit_conflict`는 판정 불가를 설명한다. `environment_reported`는 환경 오류이고 `unexpected_assertion`은 의도와 다른 실패다. 원문 메시지를 사유 코드에 넣지 않는다.

현재 통과는 정상 종료와 선언한 모든 대상의 실제 통과가 함께 필요하며, 전체 내부 시도 이력에도 불리한 관측이 없어야 한다. baseline 기대 실패는 비정상 종료, 선언한 대상 전체의 관측, 적어도 하나의 선언한 위반이 필요하다. 환경 오류·미실행·의도와 다른 assertion 실패가 섞이면 충분하지 않다. 예상 위반과 다른 실제 assertion 실패는 불리한 실패로 남는다.

프로세스 시작 실패·시간 초과·외부 신호 종료, import·컴파일·수집·브라우저 기동 실패는 그 자체로 기대 실패가 아니다. 반면 실행 가능한 검사가 기능의 부재를 직접 관측하고 선언한 위반으로 보고한 경우는 유효할 수 있다. 대상 0개, 모두 skip, 누락·손상·미지원 보고, 종료와 보고의 충돌도 성공이나 기대 실패로 승격하지 않는다.

환경 오류나 skip과 assertion 위반이 함께 있으면 실행 전체는 `error` 또는 `unknown`일 수 있다. 그래도 간결한 근거의 대상별 불리한 관측은 남는다. 같은 입력에서 뒤의 정상 실행으로 그 assertion 위반을 숨기지 않는다.

오류·판정 불가는 기존 오류 재시도 한도에 함께 포함한다. 같은 근거 입력에서 뒤의 통과가 앞선 불리한 assertion 결과·예상 밖 baseline 통과·초과한 오류 이력을 지우지 않는다. 검사 종류 이름을 바꿔 재시도 권한을 늘리지 않는다. 실제 검사 시도와 사용량은 결과 채택 여부와 무관하게 남고 baseline도 센다. 실행 전 입력·dirty 거부와 임시 worktree 준비 실패는 실행한 검사와 구분한다.

실제 시도 뒤 로그가 사라지거나 저장에 실패해도 기록 저장소를 쓸 수 있으면 `error`·`process_output`과 사용량을 남긴다. 이 경우 `output_path`·`output_digest`가 비어 있어 원문을 조회할 수 없을 수 있다. 기록 저장 자체가 실패했다면 성공으로 보고하지 않으며 쓰기 후 실패는 적용 여부가 불확실할 수 있다.

기록에는 `check-evidence/v1`의 간결한 근거를 저장한다: `result`, `reason`, `declaration_digest`, `report_digest`, `invocation`, `targets`. 대상 관측만 넣고 검사한 파일의 실제 값이나 원문 로그를 복사하지 않는다. 현재 상태는 이 근거로 재구성하므로 원문 로그 부재나 현재 시각 변화만으로 판정이 달라지지 않는다. 선언과 보고서는 실행자가 작성할 수 있다. 보증 수준은 계속 `local`이며 독립된 요구 이해나 제3자 검증을 보증하지 않는다.

## 4. 실제 참조 연동

[참조 생산자](../tools/check-result-reference.py)는 Python 3 표준 라이브러리만 사용한다. 필요한 프로젝트에 복사해 추적하고 선언·설정과 함께 검사 경로에 넣는다. 다른 러너·의존성을 자동 설치하지 않는다. pytest·Vitest·Playwright 어댑터가 내장돼 있다는 뜻이 아니다.

### 참조 설정 예시 (`checks/reference.json`)

```json
{
  "checks": [{
    "target": "greeting",
    "path": "src/greeting.txt",
    "equals": "hello\n",
    "violation": "missing-greeting",
    "missing": "violation"
  }]
}
```

`missing: violation`은 이 검사가 대상 파일의 부재를 요구 위반으로 직접 관측한다는 선언이다. 생략하거나 `error`이면 파일 부재는 환경 오류다. 선택 `requires`에는 이 검사의 환경에 필요한 파일 경로 목록을 넣는다. 그 파일이 없으면 `dependency_missing`이며 요구 위반으로 바꾸지 않는다. 실제 파일 내용은 보고에 넣지 않는다.

조건표에서 다음 명령과 검사 경로를 사용한다. 환경 변수를 수동으로 만들어 단독 생산자의 보고를 완료 근거로 삼지 않는다.

```markdown
| ID | 종류 | 검증 명령 | 검사 경로 | Task |
| --- | --- | --- | --- | --- |
| AC-1 | change | `python3 tools/check-result-reference.py checks/declaration.json checks/reference.json` | checks/declaration.json, checks/reference.json, tools/check-result-reference.py | T001 |
```

`src/greeting.txt`가 없거나 다른 값을 갖는 기준 상태에서는 선언한 위반을 관측한다. 현재 코드가 `hello`와 줄바꿈을 저장하면 같은 대상이 통과한다. 이 예시는 선언을 판정기에 주입하는 것만으로 끝나지 않고 별도 프로세스에서 실제 파일을 읽는다. 적용 프로젝트의 실제 동작과 검증 대상은 검사 작성자가 정해야 한다.
