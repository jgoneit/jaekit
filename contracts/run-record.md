# 계약: 실행 기록 (`run/v1`)과 완료 판정

이 문서는 다음에 대한 canonical 정의다.
- 실행 기록 `runs.jsonl`의 줄 형식
- 그 기록으로 조건 충족과 상태를 계산하는 규칙
- 완료 기록

실행 묶음은 [bundle.md](bundle.md)에 있다.

실행 기록은 `ha`만 덧붙인다. 에이전트는 기록을 직접 쓰지 않고 `ha` 명령을 호출한다. 같은 사용자 권한의 에이전트가 파일을 고칠 수는 있으므로, 이 기록의 보증 수준은 `local`을 넘지 않는다.

## 1. `ha` 명령

| 명령 | 하는 일 | 기록 |
| --- | --- | --- |
| `ha lint <goal>` | 목표 문서 형식(조건 ID), 실행 묶음 구조, 모든 조건 ID의 조건표 대응, 조건↔task 양방향 대응, 순환, 참조 파일 존재, 검증 방법 누락을 확인한다. 오류 코드는 [bundle.md](bundle.md) §8에 있다. | 없음 |
| `ha estimate <goal> [AC-n \| EX-n \| T001 ...] [--baseline] [--format md\|json]` | 현재 PLAN의 필수 최소 실행량, 상한·부족량, 기본 선택과 지정한 대상의 실행량을 읽기 전용으로 계산한다(§1.1). | 없음 |
| `ha start <goal> --request <인용> [--skill <SKILL.md>] [--host-name …] [--host-version …] [--model …]` | 시작을 기록한다. 이미 있으면 거절한다. 새 시작이 필요하면 사용자가 묶음을 새로 만든다. lint 오류나 필수 최소 실행량보다 작은 PLAN 상한이 있어도 기록 생성 전에 거절한다. dirty 시작은 기존처럼 경고하며 허용한다. `--skill`을 주면 `ha`가 그 머리말에서 이름과 버전을 읽고 Skill 디렉토리의 digest를 계산한다(§2.1). | `start` |
| `ha check <goal> [AC-n \| T00n ...]` | 지정한 조건이나 task의 검증 명령을 실행한다. 대상이 없으면 모든 required 조건을 실행한다(통합 검증). | `check` |
| `ha check <goal> --baseline [AC-n ...]` | `change` 조건의 검사 경로만 base commit 임시 worktree에 덧씌워 실행한다. | `baseline` |
| `ha budget <goal> --runs N --from P --window W --revision R [--quote <인용> --context <문맥>] [--reason <이유>] [--interpretation <해석>]` | `/3` 현재 창의 총 실행 횟수 상한을 바꾼다. 양의 정수 인수가 모두 필요하며, 증액은 인용·문맥, 감액은 이유가 필요하다(§5.2). | `budget_change` |
| `ha note <goal> <block \| unblock \| confirm \| reopen \| input> [대상] --quote <인용> [--cause <원인>]` | 막힘, 해소, 사용자 확인, 사용자의 재작업 요청, 그 밖의 사용자 입력을 남긴다. 사용자 발화는 인용한다. `block`은 `--cause`가 필요하고, `confirm`은 대상(§2.4)과 인용이 필요하다. | `note` |
| `ha status <goal> [--format md\|json]` | 기록·묶음·git에서 상태를 계산해 출력한다(§4). | 없음 |
| `ha done <goal>` | 완료를 주장한다. `ha status`와 같은 계산을 하고 그 결과를 기록한다. 결과가 `complete`면 이 줄이 **완료 기록**이다(§5). exit code는 `ha status`와 같다. | `done` |
| `ha log <goal> <seq> [--tail N]` | 그 실행의 로컬 출력 원문을 보여 준다. 로컬에서 읽기 위한 것이다. | 없음 |

`ha`는 검증 명령을 셸 없이 argv로 실행한다. 명령마다 시간 제한(기본 900초)을 둔다. 시간 제한이 되면 그 명령의 프로세스 그룹 전체를 끝낸다. `check`, `baseline`, `note`, `start`, `budget_change`는 기록에 덧붙이기 전에 git 디렉토리의 `ha/lock`을 잡는다. 그래서 여러 `ha`가 동시에 덧붙여도 연결이 끊기지 않는다.

**종료 코드**
- `ha check`: 모든 결과가 기대대로면(`pass`, baseline은 `fail_as_expected`) 0, 아니면 1
- `ha lint`: 오류가 없으면 0, 있으면 1
- `ha estimate`: 필수 최소량을 PLAN 상한이 수용하면 0, 부족하거나 lint 오류가 있으면 1
- `ha status`, `ha done`: §4.3
- `ha budget`: 성공 0, 잘못된 인수 64, 값·창·개정·완료 상태 등의 거부 65, 구규칙 또는 미지원 규칙 66, 내부 오류·쓰기 적용 불확실성 70
- 모든 명령: 64 사용법 오류, 65 거절(시작 전, 이미 시작함, lint 오류, 끊긴 기록, 확인 조건 불충족, dirty 검증, 시작 예산 부족), 66 지원하지 않는 규칙 버전, 70 내부 오류

출력 원문은 git 디렉토리의 `ha/runs/<goal-key>/<seq>.log`에만 둔다. `<goal-key>`는 저장소 기준 goal 경로를 URL 경로 이스케이프한 것이다(예: `docs%2Fspecs%2Fempty-input`). 목표가 여럿이면 seq가 겹치기 때문이다. 커밋되는 기록과 `ha status`·`ha done`의 출력에는 결과, 이유, digest, 로컬 경로와 명시적으로 기록한 사용자 확인·예산 변경 정보를 넣는다. 출력 원문은 넣지 않는다. 출력에 비밀이 섞일 수 있고, 상태 출력은 완료 보고를 거쳐 커밋되기 때문이다. 원문은 `ha log`로만 본다.

### 1.1 실행 전 진단

**작업 상태.** `check`·`baseline`·task 검증은 각 대상의 프로세스를 시작하기 직전에 적용 규칙의 제외 경로를 반영해 Git 상태를 확인한다. 제외되지 않는 변경이 있으면 exit 65로 거부한다. Git 상태를 읽지 못하면 exit 70이며 실행하지 않는다. 거부된 대상의 검사 기록·실행 횟수·출력 파일은 만들지 않는다. 여러 대상 중 앞선 대상만 실행됐다면 수행/미수행 개수를 출력하고 앞선 결과는 그대로 보존한다. 실행 후 생긴 오류나 stale 결과도 기록과 사용량에 남는다.

진단은 저장소 상대 경로, tracked/untracked, 관측한 symlink 여부를 구분하며 삭제·이름 변경의 경로와 submodule 상태도 표시한다. untracked 경로는 해당 경로의 ignore 규칙 검토를 안내할 수 있으나 자동으로 바꾸지 않는다. tracked 변경을 ignore 추가로 해결할 수 있다고 안내하지 않는다. 실행 중 파일 변경을 원자적으로 막는 기능은 아니며 현재 완료 근거는 §3의 신선도 판정을 계속 적용한다.

이 거부는 지원하는 `/1`·`/2`·`/3` 목표의 앞으로 실행할 검사에 적용한다. 과거 `tree_clean: false` 기록의 결과와 사용량을 삭제하거나 재해석하지 않는다. 시작의 dirty 경고와 기존 Git ignore·규칙별 실행 문서 제외도 유지한다.

**실행량.** `ha estimate`의 필수 최소량은 `필수 change × 2 + 필수 maintain`이다. manual은 0회이며 change 34개면 68회다. 실패 없는 새로운 실행의 하한이므로 재시도·환경 준비·수동 확인 시간·실제 총시간 또는 완료를 보장하지 않는다. 검사나 환경 probe를 실행하지 않고 기록·예산 창·작업 파일도 바꾸지 않는다.

기본 `check`는 필수 change/maintain, 기본 `--baseline`은 조건표의 모든 change(선택 AC와 EX 포함)를 선택한다. `estimate`는 이 기본 실행량과 필수 하한을 넘는 추가량을 함께 표시한다. 명시한 대상은 실제 `check`와 같은 순서로 선택한다. 선택 AC·EX는 추가량, task의 각 명령은 각각 한 회이며, 중복 대상도 실제 실행과 같이 반복 계산한다. `--baseline`의 명시 대상은 change 조건만 허용한다. manual만 있는 기본 선택은 추정치 0이며 실제 `check`의 no-target 오류는 바뀌지 않는다.

상한은 **현재 PLAN의 전체 상한**이다. 진행 중 목표의 남은 예산이 아니다. 새로운 `start`는 상한이 필수 하한보다 작으면 최소량·상한·부족량을 출력하고 기록을 만들기 전에 exit 65로 거부한다. 같은 값이면 시작할 수 있다. 선택 검사나 재시도 여유 때문에 자동 증액하지 않는다. 기존 목표의 상한과 재개 창은 §5.2 그대로이며 진행 중 검사에 예산 제한을 새로 적용하지 않는다.

JSON 출력은 `schema: estimate/v1`이며 다음을 제공한다.

| 필드 | 의미 |
| --- | --- |
| `goal`, `budget_source` | 저장소 상대 목표 경로, `PLAN.md` |
| `required_change`, `required_maintain`, `minimum_runs` | 필수 종류별 수와 최소 실행량 |
| `plan_runs_limit`, `shortfall`, `headroom` | PLAN 상한, 부족량, 하한 위의 여유. 부족량과 여유는 음수가 아니다 |
| `default_check_runs`, `default_baseline_runs`, `default_additional_runs` | 기본 두 명령의 실행량과 합계에서 하한을 뺀 값 |
| `selected` | `mode`, 순서와 중복을 보존한 `targets`, `runs`, `additional_runs`, `optional_runs`, `task_runs`, `repeated_required_runs` |
| `limitation` | 추정치의 범위와 기존 예산 창을 바꾸지 않는다는 설명 |

`selected.additional_runs`는 선택·task·필수 대상 중복의 합이다. 같은 phase에서 필수 대상을 처음 선택한 실행은 이미 하한에 포함되므로 추가량이 아니다. 여러 번의 CLI 호출을 합친 비용은 추적하지 않는다.

## 2. 줄 형식

모든 줄의 공통 필드는 다음과 같다.

| 필드 | 의미 |
| --- | --- |
| `schema` | `"run/v1"` |
| `seq` | 1부터 증가하는 정수 |
| `prev` | 직전 줄의 바이트 sha256. 첫 줄은 `null` |
| `kind` | `start` \| `check` \| `baseline` \| `note` \| `done` \| `budget_change` |
| `at` | UTC 시각 |
| `ha_version` | 기록한 `ha` 버전 |

이 문서에서 **묶음 문서**는 goal 디렉토리 안의 다음 파일만 뜻한다.
- 목표 문서: `SPEC.md`와 링크된 보조 문서([goal-docs.md](goal-docs.md) §1)
- `PLAN.md`
- `REVIEW.md`
- `PROGRESS.md`
- `tasks/**`
- `runs.jsonl`

goal 디렉토리 안의 다른 파일(검증 스크립트, fixture 등)은 코드로 다룬다. **코드 상태**는 다음 둘을 합친 것이다.
- HEAD의 tree에서 묶음 문서를 뺀 것
- 작업 트리의 변경: 추적 파일 수정, 추적되지 않은 파일. 무시된 파일(`.gitignore`)은 빠진다.

### 2.1 `start`

| 필드 | 의미 |
| --- | --- |
| `goal` | goal 디렉토리 경로 |
| `base_commit` | 시작 시점의 HEAD |
| `tree_clean` | 시작 시점에 묶음 문서 밖 작업 트리가 깨끗했는가 |
| `spec_digest` | 목표 문서 집합의 digest. 각 문서의 경로와 내용 sha256을 경로 순으로 이은 것의 sha256([goal-docs.md](goal-docs.md) §1) |
| `request` | `{ "source": "conversation", "quote": "<사용자 요청 인용>" }`. 주장이다. |
| `host` | `{ "name", "version", "model" }`. 주장이다. |
| `skill` | `{ "name", "version", "digest" }`. 에이전트가 읽은 Seal Skill과 템플릿 패키지. `--skill`이면 `ha`가 머리말의 `name`과 `metadata.version`을 읽고, Skill 디렉토리의 파일 전체를 목표 digest와 같은 방식으로 digest한다. 어느 파일을 가리킬지는 에이전트가 정하므로 여전히 주장이다. |
| `rules` | 상태 계산 규칙 버전(예: `"run-rules/1"`). `ha`가 넣는다. 이 목표의 상태는 끝까지 이 버전으로 계산한다(§4). |
| `budget` | PLAN의 예산 `{ "runs", "elapsed_seconds", "cost" }`. 최초 한도다. `/1`·`/2`는 이 값을 계속 쓰고, `/3`의 횟수 상한은 유효한 변경 기록을 반영한다(§5.2). |

### 2.2 `check`

| 필드 | 의미 |
| --- | --- |
| `target` | `{ "criterion": "AC-1" }` 또는 `{ "task": "T001", "index": 1 }` |
| `criterion_kind` | 실행할 때의 조건 종류. task 검사는 없다. 종류 변경을 기록만으로 드러내기 위해 남긴다. |
| `argv` | 실행한 명령 |
| `command_digest` | PLAN에 적힌 명령 문자열의 sha256 |
| `commit` | 실행 시점의 HEAD |
| `tree_clean` | 실행 시점에 묶음 문서 밖 작업 트리가 깨끗했는가 |
| `spec_digest` | 실행 시점의 SPEC digest |
| `exit` | exit code. 시작 실패나 시간 초과면 `null` |
| `result` | `/1`·`/2`와 일반 maintain·task는 `pass`(exit 0) \| `fail`(exit ≠ 0) \| `error`(시작 실패, 시간 초과). `/3` change는 [구조화 결과 계약](check-result.md)에 따라 분류한다. |
| `duration_ms`, `output_digest`, `output_path` | 실행 시간, 출력 digest, 로컬 출력 위치(저장소 기준 경로, 예: `.git/ha/runs/<goal-key>/7.log`). `output_path`에는 절대 경로를 쓰지 않는다. linked worktree에서도 형식이 같고, 이때 `.git`은 그 작업 트리의 git 디렉토리를 뜻한다. 이전 `ha`가 남긴 절대 경로는 고쳐 쓰지 않고 그대로 읽는다. `/3`에서 실제 시도 뒤 출력 저장이 실패하면 `error`·`process_output`으로 시도와 사용량을 남기고 `output_path`·`output_digest`는 빈 값일 수 있다. |

`/3` change 줄에는 `evidence`가 더해진다. [check-result.md](check-result.md)의 `check-evidence/v1` 간결한 근거이며 선언·생산자 입력, 호출, 보고 digest와 대상별 관측을 연결한다. `unknown`은 판정 불가이고 통과가 아니다.

### 2.3 `baseline`

`check`의 필드에 다음이 더해진다. `commit`은 덧씌울 검사 파일을 가져온 HEAD다.

| 필드 | 의미 |
| --- | --- |
| `base_commit` | `start`의 base commit |
| `overlay` | `[{ "path", "digest" }]`. HEAD에서 가져와 덧씌운 검사 파일 |
| `result` | `/1`·`/2`: `fail_as_expected`(exit ≠ 0) \| `unexpected_pass`(exit 0) \| `error`. `/3`는 [구조화 결과 계약](check-result.md)을 적용한다. |

`/1`·`/2`는 실패의 원인(예: 아직 없는 모듈의 import 실패)을 구분하지 않는다. `/3` change는 선언한 대상의 실제 관측을 요구한다. import·수집·기동 오류, 미실행, 불명확한 일반 스크립트 실패는 기대 실패가 아니다. 실행 가능한 검사가 기능 부재를 직접 관측한 경우는 선언한 요구 위반일 수 있다. 사람이 출력 원문을 보려면 `ha log`를 쓴다.

앞으로 수행하는 baseline은 `/1`·`/2`·`/3` 모두 검사 대상 하나의 전체 복사 목록을 파일 적용 전에 확인한다. HEAD 원본은 커밋된 일반 파일(`100644` 또는 `100755`)이어야 하며, 기준 상태의 복사 대상이나 부모가 symlink이면 내부·외부·끊어진 링크 모두 거부한다. 선언·생산자와 추가 검사 경로에 같은 조건을 적용한다. 안전한 일반 파일 교체, 없는 부모·파일 생성, 실행 권한은 유지하며 복사와 무관한 symlink는 허용한다.

준비 오류나 관측된 불안전 경로 변경으로 안전성을 확인할 수 없으면 명령을 시작하지 않는다. 실패 원인과 해당 저장소 상대 경로를 알리고, 원래 작업 트리와 임시 worktree 밖 파일의 내용·권한을 보존한다. 미실행 대상의 검사 기록·사용량은 추가하지 않으며 일괄 요청에서 이미 실행된 대상의 기록·사용량은 유지한다. 이는 신규 실행의 안전 조건으로, 과거 기록을 소급 재분류하지 않는다. 실제 프로세스 시작 시도 이후의 오류·실패는 기존 기록·사용량 규칙을 따른다. 임의의 비협조 외부 프로세스를 격리하거나 검사 명령 자체의 쓰기를 제한하는 기능은 아니다.

### 2.4 `note`

| 필드 | 의미 |
| --- | --- |
| `note` | `block` \| `unblock` \| `confirm` \| `reopen` \| `input` |
| `cause` | `block`일 때 원인 종류([bundle.md](bundle.md) §5) |
| `subject` | `confirm`일 때 대상: `AC-n`(manual 조건), `scope:<path>`, `tests:<path>`, `spec` |
| `bound` | `confirm`일 때 `ha`가 계산해 묶는 값(아래 표). 필드는 `criterion_sha256`, `spec_digest`, `blob` 중 해당하는 것이다. 에이전트가 넣지 않는다. |
| `quote` | 사용자 발화 인용(`confirm`, `reopen`, `input`) 또는 설명. 주장이다. |

`confirm`은 **확인한 그 내용**에만 유효하다. `ha`는 기록할 때 대상의 현재 값을 `bound`에 묶는다. 상태를 계산할 때 그 값이 지금과 다르면 그 확인은 무효다. 무효가 된 확인은 상태 출력에 따로 나열된다.

| 대상 | `bound` | 유효 조건 |
| --- | --- | --- |
| `AC-n` | 목표 문서에 있는 그 조건 문장의 sha256, 목표 digest. `manual` 조건에만 쓸 수 있다 | 둘 다 현재와 같다 |
| `scope:<path>` | 그 경로의 HEAD blob id(삭제면 `deleted`). 기록 때 작업 트리가 깨끗해야 한다 | 현재 HEAD의 같은 경로가 같은 blob이다 |
| `tests:<path>` | 같음 | 같음 |
| `spec` | 현재 SPEC digest | 현재 SPEC digest와 같다 |

`input`은 실행 중 사용자 발화 가운데 `confirm`이나 `reopen`이 아닌 것이다. 예: 진행 지시, 질문에 대한 답, 새 요구, 수정 요청. `ha`는 분류하지 않는다. 분류가 필요하면 사람이 기록을 읽고 판단한다. 상태 계산에는 쓰지 않는다. 개선 후보의 근거가 되고, 과거 목표를 재실행할 때 같은 질문에 답하는 자료가 된다.

### 2.5 `done`

| 필드 | 의미 |
| --- | --- |
| `status` | 계산한 상태(§4) |
| `reasons` | 남은 이유 목록 |
| `commit`, `tree_clean`, `spec_digest` | 계산 시점의 코드 상태와 SPEC |
| `record_head` | 이 줄 직전까지의 기록 head |
| `criteria` | 조건별 `{ "id", "kind", "satisfied", "records": [seq…] }`. 근거가 된 기록 |
| `command_digests` | 조건별 현재 명령 digest |

### 2.6 `budget_change`

`/3` 전용 줄이다. 공통 필드와 다음 `budget_change` 객체를 기록한다.

| 필드 | 의미 |
| --- | --- |
| `goal` | 시작 기록과 같은 목표 |
| `window_start` | 변경 대상 start/reopen seq |
| `revision` | 변경 직전 최신 start/reopen/budget_change seq |
| `previous_runs`, `runs` | 이전·새 총 실행 횟수 상한 |
| `quote`, `context` | 증액을 뒷받침하는 사용자 원문과 요청 문맥. 출처 미검증이면 실행자의 주장 |
| `interpretation` | 선택적인 에이전트 해석. 사용자 원문과 구분 |
| `reason` | 변경 이유. 감액에는 필수 |

명령의 `--window`, `--revision`, `--from`이 각각 현재 창·개정·상한과 일치해야 한다. 잠금 아래 최신 기록을 읽어 비교한 뒤 기록한다. 단순 검사 기록 추가는 예산 개정이 아니며 실제 누적 사용량에 반영한다. 새 변경의 seq가 그 뒤 예산 개정이 된다.

## 3. 신선도

결과는 그 결과가 나온 코드 상태, SPEC, 검증 명령에 묶인다.

**전제: 지금 작업 트리가 깨끗하다.** 묶음 문서 밖의 작업 트리에 변경(추적 파일 수정, 추적되지 않은 파일)이 있으면 지금의 코드 상태는 어떤 commit과도 같지 않다. 이때는 어떤 기록도 신선하지 않고, 상태 이유는 `worktree_dirty`다.

여기서 **묶음 문서**는 자기 목표의 목표 문서와 실행 묶음이다. `run-rules/2`와 `run-rules/3`에서는 다른 목표의 실행 묶음 파일도 들어간다(§4.6). 이 계약에서 "묶음 문서 밖"은 모두 이 목록 밖을 뜻한다.

`check` 줄 r은 다음을 모두 만족할 때 **신선하다**.
1. 지금 작업 트리가 깨끗하다(전제).
2. `r.tree_clean`이 참이다.
3. 기록 commit과 HEAD 사이에 묶음 문서 밖의 차이가 없다. 예: `git diff --quiet r.commit HEAD -- . ':(exclude)<goal>/SPEC.md' ':(exclude)<goal>/PLAN.md' …`
4. `r.spec_digest`가 현재 SPEC digest와 같다.
5. `r.command_digest`가 현재 PLAN의 해당 명령 digest와 같다.

`baseline` 줄 b는 다음을 모두 만족할 때 신선하다. 3은 요구하지 않는다. baseline은 base commit에서 실행했고, 덧씌운 검사 파일의 digest가 HEAD와의 연결이다.
- 위의 1, 2, 4, 5가 성립한다.
- `b.base_commit`이 `start`의 base commit과 같다.
- `b.overlay`의 경로 집합이 지금 조건표의 검사 경로와 같고, 각 digest가 현재 HEAD의 같은 경로 내용과 같다.

`/3` change에는 선언·결과 계약·대상·생산자 버전과 내용의 연결도 필요하다([check-result.md](check-result.md)). 명령 문자열이 같아도 이 의미가 바뀌면 이전 검사와 완료 근거를 재사용하지 않는다. 상태 재구성은 저장된 간결한 근거로 하므로 원문 로그가 없어지거나 현재 시각이 달라졌다는 이유만으로 판정이 바뀌지 않는다.

신선하지 않은 줄은 지우지 않는다. 충족 계산에 쓰지 않을 뿐이다. 이전 실패와 오류는 시도 이력으로 남는다(§4.4).

**한계**: 무시된 파일(`.gitignore`)은 코드 상태에 들어가지 않는다. 검사가 무시된 파일(예: 로컬 설정)에 의존하면 같은 코드 상태에서도 결과가 달라질 수 있다.

## 4. 상태 계산 (`ha status`, `ha done`)

입력은 `runs.jsonl`, 실행 묶음, git뿐이다. 모델을 호출하지 않고, **현재 시각을 쓰지 않는다.** 같은 입력이면 언제 계산해도 같은 결과다.

계산 규칙은 `start`의 `rules` 버전을 따른다. `ha`가 새 규칙 버전을 갖게 되어도 진행 중인 목표에는 시작할 때의 버전을 쓴다. 그 버전을 지원하지 않는 `ha`는 계산하지 않고 종료 코드 66(지원하지 않는 규칙 버전)으로 끝낸다.

### 4.1 조건별 충족

먼저 조건마다 충족 여부를 정한다. 전체 상태는 그다음에 계산한다.

C는 그 조건의 신선한 `check` 줄, B는 신선한 `baseline` 줄이다. E는 같은 코드 상태에서 허용하는 오류 재실행 수다(기본 1). 오류(`error`)는 검사가 판단을 내지 못한 것이다. 실패의 증거도 통과의 증거도 아니다. 그래서 E번까지는 다시 실행할 수 있다. 실패(`fail`)는 다시 실행해도 지워지지 않는다.

| 종류 | 충족 조건 |
| --- | --- |
| `maintain` | C에 `pass`가 하나 이상 있고, `fail`이 없고, `error`가 E개 이하이고, C의 가장 최근 줄이 `pass`다. |
| `change` | `maintain`의 조건을 C가 만족한다. 그리고 B에 `fail_as_expected`가 하나 이상 있고, `unexpected_pass`가 없고, `error`가 E개 이하이고, B의 가장 최근 줄이 `fail_as_expected`다. |
| `manual` | 그 조건에 유효한 `confirm`이 있다(§2.4). `check` 기록은 요구하지 않는다. |

충족되지 않은 조건은 다음 순서로 처음 맞는 이유 하나를 받는다.

| 이유 | 대상 | 조건 | 분류 |
| --- | --- | --- | --- |
| `criterion_missing` | change, maintain | C가 비어 있다 | 실행자가 고침 |
| `criterion_flaky` | change, maintain | C에 `pass`와 `fail`이 함께 있다. 같은 코드에서 결과가 갈렸다 | 실행자가 고침(검사나 코드를 고쳐 다시 검증) |
| `criterion_failed` | change, maintain | C에 `fail`이 있다 | 실행자가 고침 |
| `criterion_error` | change, maintain | C의 `error`가 E개 이하이고, 가장 최근 줄이 `error`다 | 실행자가 고침(다시 실행) |
| `baseline_missing` | change | B가 비어 있다 | 실행자가 고침 |
| `baseline_unexpected_pass` | change | B에 `unexpected_pass`가 있다 | 실행자가 고침(검사를 고치거나, 계획 변경으로 종류를 `maintain`으로 바꾼다) |
| `baseline_failed` | change | `/3`의 신선한 baseline에 의도와 다른 assertion 실패 또는 내부 시도 불일치가 있다 | 실행자가 고침 |
| `evidence_invalid` | change | `/3`의 저장된 간결한 근거가 유효하지 않다 | 실행자가 고침 |
| `baseline_error` | change | B의 `error`가 E개 이하이고, 가장 최근 줄이 `error`다 | 실행자가 고침(다시 실행) |
| `error_limit` | change, maintain | C나 B의 `error`가 E개를 넘었다. 같은 코드에서 검사 환경이 계속 판단을 내지 못한다 | 사용자 확인 |
| `manual_unconfirmed` | manual | 유효한 `confirm`이 없다(없거나, 확인한 뒤 내용이 바뀌었다) | 사용자 확인 |

`/3` change는 위 양의 근거 조건에 [구조화 결과 계약](check-result.md)을 함께 적용한다. `unknown`은 `error`와 같은 재시도 한도를 쓴다. baseline의 의도와 다른 assertion 실패나 내부 시도 불일치는 `fail`로 남고 `baseline_failed`다. 환경 오류·판정 불가와 섞인 경우에도 대상별 불리한 관측을 보존하며, 같은 입력의 뒤 통과로 이를 해소하지 않는다. 현재 입력에 대한 간결한 근거 자체가 부정합이면 `evidence_invalid`다. 선언이 바뀌어 낡은 입력이 된 기록은 신선도에서 제외한다. 알 수 없는 결과값과 부정합 저장 데이터는 모든 지원 규칙에서 `record_invalid`(사용자 확인)로 드러나며 완료 근거가 되지 않는다.

선택 조건(목표 문서의 `(선택)` 표시)과 실행자가 더한 `EX-n`도 같은 방식으로 계산해 출력하지만, 상태를 막지 않는다. 필수 여부는 목표 문서에서만 읽는다.

### 4.2 목표 수준의 이유

| 이유 | 조건 | 분류 |
| --- | --- | --- |
| `lint_error` | `ha lint` 오류가 있다 | 실행자가 고침 |
| `not_started` | `start` 줄이 없다 | 실행자가 고침 |
| `record_integrity` | `prev` 연결이 끊겼거나 `seq`가 어긋난다 | 사용자 확인 |
| `worktree_dirty` | 묶음 문서 밖 작업 트리가 깨끗하지 않다(§3 전제) | 실행자가 고침(commit하거나 되돌린다) |
| `out_of_scope` | base 이후 바뀐 파일(묶음 문서 제외) 중 `바꿀 수 있는 경로`에 맞지 않고, 그 경로에 유효한 `confirm scope:`가 없는 것이 있다 | 사용자 확인 |
| `test_definition_changed` | base에 있던 테스트 경로 파일이 수정·삭제됐고, 그 경로에 유효한 `confirm tests:`가 없다 | 사용자 확인 |
| `spec_changed` | 현재 SPEC digest가 `start`의 digest와 다르고, 유효한 `confirm spec`이 없다 | 사용자 확인 |
| `task_open` | required task가 `PROGRESS.md`에서 `done`이나 `dropped`가 아니다 | 실행자가 고침 |
| `record_invalid` | 알 수 없는 결과값이나 부정합 저장 데이터가 있다 | 사용자 확인 |
| `blocked` | 마지막 `block` 뒤에 `unblock`이 없다 | 원인 해소 |
| `budget_exceeded` | 예산 창(§5.2) 안의 `check`·`baseline` 줄 수나 경과 시간이 예산을 넘었다 | 사용자 결정 |

### 4.3 상태

다음 순서로 처음 맞는 것이 상태다. 모든 이유는 상태와 함께 출력한다.

1. 유효한 완료 기록이 있다(§5.1) → `complete`. 이 경우 예산은 계산하지 않는다.
2. 모든 required 조건이 충족됐고 목표 수준의 이유가 없다 → `complete`. `run-rules/2`와 `run-rules/3`에서는 남은 이유가 `budget_exceeded`뿐이어도 여기에 해당한다(§4.6).
3. `blocked` 이유가 있다 → `blocked`
4. `budget_exceeded` 이유가 있다 → `budget_exhausted`
5. "실행자가 고침" 이유가 하나라도 있다 → `incomplete`
6. 나머지("사용자 확인"만 남음) → `needs_user`

2에 해당하더라도 완료를 선언하려면 `ha done`으로 완료 기록을 남겨야 한다. `ha status`는 이때 "완료 기록 없음"을 함께 출력한다.

**exit code**: `complete` 0, `incomplete` 1, `needs_user` 2, `blocked` 3, `budget_exhausted` 4, 사용법·내부 오류 64 이상(§1).

### 4.4 출력

출력에는 다음을 넣는다. **출력 원문(로그 내용)은 넣지 않는다**(§1).
- **머리말**
  - `보증 수준: local`과 "같은 사용자 권한의 에이전트가 검사와 기록을 바꿀 수 있음"
  - `검사 작성자: 실행자`
  - 기록 head
  - 완료 기록 seq(있으면)
  - 규칙 버전, Skill 이름·버전(주장)
- **조건별**
  - 종류, 필수 여부, 충족 여부, 이유, 근거 기록 seq
  - 시도 이력: 신선하지 않은 것까지 포함한 `fail`·`error` 수와 그 commit. 사소한 commit으로 이전 실패를 지우고 다시 통과시키는 것을 드러낸다.
- **목표 수준의 이유**
- **사람이 봐야 할 변화**
  - 검증 명령이 `start` 이후 바뀐 조건과 바뀌기 전후의 명령
  - 종류가 바뀐 조건과 선택 조건 목록
  - 무효가 된 사용자 확인(§2.4)
  - 목표 진행 중에 `ha_version`이 바뀐 줄
- **로컬 출력 위치**: 실패·오류 기록의 로컬 출력 경로. 내용은 `ha log`로 본다.

`--format md`(기본)는 사람이 읽고 `PROGRESS.md`에 붙이는 형식이다. 표지와 이유 코드는 계약 그대로 쓰고, 설명 문장은 영어로 쓴다.

### 4.5 `--format json`

외부 도구가 읽을 수 있는 구조화된 형식이다. 필드 순서는 아래 표의 순서로 고정되고, 들여쓰기는 공백 두 칸이다. 시각을 담지 않는다. 같은 입력이면 바이트 단위로 같다. 시험이 아래 표와 실제 출력의 필드를 비교한다.

#### 상태 문서 (`status/v1`)

| 필드 | 의미 |
| --- | --- |
| `schema` | `"status/v1"` |
| `goal` | goal 디렉토리(저장소 기준) |
| `status` | §4.3의 상태 |
| `exit_code` | §4.3의 종료 코드 |
| `assurance` | 항상 `"local"` |
| `check_author` | 항상 `"executor"` |
| `rules` | 계산에 쓴 규칙 버전 |
| `ha_version` | 계산한 `ha`의 버전 |
| `skill` | `start`의 Skill(주장) 또는 `null` |
| `record_head` | `{ "seq", "sha256" }` 또는 기록이 없으면 `null` |
| `completion_record` | 유효한 완료 기록의 seq 또는 `null` |
| `complete_without_record` | 모든 조건이 충족됐지만 완료 기록이 없으면 `true` |
| `commit` | 지금 HEAD |
| `tree_clean` | 지금 묶음 문서 밖 작업 트리가 깨끗한가 |
| `spec_digest` | 지금 목표 digest |
| `criteria` | 조건별 평가(아래) |
| `reasons` | 상태를 정한 이유. 필수 조건의 미충족 이유와 목표 수준의 이유 |
| `lint` | lint 오류 `{ "code", "file", "line", "detail" }` |
| `changes` | 사람이 봐야 할 변화(아래) |
| `budget` | 예산 창(아래) 또는 시작 전이면 `null` |
| `usage` | 전체 시도·결과·근거의 서로 다른 집계 축(아래) |
| `logs` | 실패·오류 기록의 `{ "seq", "target", "result", "path" }`와 선택 `reason`(검증된 사유 코드 또는 `evidence_invalid`·`record_invalid`) |

#### 조건 (`criteria[]`)

| 필드 | 의미 |
| --- | --- |
| `id` | `AC-n` 또는 `EX-n` |
| `kind` | `change` \| `maintain` \| `manual`. 조건표에 없으면 빈 문자열 |
| `required` | 필수 조건인가 |
| `satisfied` | §4.1의 충족 여부 |
| `reason` | 충족되지 않았으면 §4.1의 이유, 충족됐으면 빈 문자열 |
| `records` | 충족의 근거가 된 신선한 기록의 seq |
| `attempts` | 시도 이력(아래) |

#### 시도 (`attempts`)

| 필드 | 의미 |
| --- | --- |
| `checks` | 이 조건의 `check` 줄 수(신선하지 않은 것 포함) |
| `fails` | 그중 `fail` |
| `errors` | 그중 `error` |
| `baselines` | `baseline` 줄 수 |
| `unexpected_passes` | 그중 `unexpected_pass` |
| `baseline_errors` | 그중 `error` |
| `unknowns` | check 중 판정 불가(`unknown`) |
| `baseline_unknowns` | baseline 중 판정 불가(`unknown`) |
| `baseline_fails` | baseline 중 의도와 다른 assertion 실패(`fail`) |
| `stale` | 신선하지 않은 `check`·`baseline` 줄 수 |
| `fail_commits` | `fail`·`error`가 난 commit(앞 12자리) |

#### 실행량 (`usage`)

| 필드 | 의미 |
| --- | --- |
| `total` | 전체 check·baseline 시도 수 |
| `checks`, `baselines` | 시도 종류별 수 |
| `environment_errors`, `unknowns` | 오류·판정 불가 결과 수 |
| `current_inputs`, `stale_inputs` | 현재 입력과 연결되는 시도·낡은 입력의 시도 수 |
| `valid_evidence` | 현재 입력의 유효한 pass·기대 baseline 근거 수 |

종류·결과·신선도는 서로 겹치는 축이다. 모든 열을 더해 총실행량으로 만들지 않는다. `valid_evidence`는 현재 입력의 양의 근거 부분집합이며 조건이나 목표 전체의 충족 수가 아니다.

#### 이유 (`reasons[]`)

| 필드 | 의미 |
| --- | --- |
| `code` | §4.1·§4.2의 이유 |
| `class` | `executor`(실행자가 고침) \| `user`(사용자 확인) \| `blocked` \| `budget` |
| `criterion` | 조건의 이유일 때 조건 ID |
| `paths` | `out_of_scope`, `test_definition_changed`, `worktree_dirty`의 저장소 상대 경로 |
| `detail` | 설명(영어). `worktree_dirty`는 경로별 tracked/untracked와 관측한 파일 유형·변경 상태를 포함한다 |

#### 변화 (`changes`)

| 필드 | 의미 |
| --- | --- |
| `command_changed` | `{ "criterion", "seq", "before", "after" }`: 이전 기록과 검증 명령이 다른 조건 |
| `kind_changed` | `{ "criterion", "seq", "before", "after" }`: 이전 기록과 종류가 다른 조건 |
| `optional` | 선택 조건과 `EX-n` |
| `invalid_confirmations` | `{ "seq", "subject", "reason" }`: 효력을 잃은 확인 |
| `ha_version_changed` | `{ "seq", "ha_version" }`: `start`와 다른 `ha` 버전이 쓴 줄 |

#### 예산 (`budget`)

| 필드 | 의미 |
| --- | --- |
| `window_start` | 예산 창이 시작한 줄의 seq |
| `runs` | 창 안의 `check`·`baseline` 줄 수 |
| `runs_limit` | `/1`·`/2`는 최초 상한, `/3`는 최신 유효 횟수 상한 |
| `revision` | 현재 창의 최신 start/reopen/budget_change seq |
| `remaining_runs` | 남은 횟수(최소 0) |
| `excess_runs` | 초과 횟수(최소 0) |
| `reached` | 사용량이 횟수 상한과 같은가 |
| `history` | `{ "seq", "at", "change": <budget_change 객체> }` 목록. 이전 변경도 보존 |
| `elapsed_seconds` | 창 안의 경과 시간 |
| `elapsed_limit_seconds` | `start`의 `budget.elapsed_seconds` |
| `exceeded` | 예산을 넘었는가 |

### 4.6 규칙 버전

`start`의 `rules`가 계산 규칙을 정한다. 새 `ha start`는 `run-rules/3`를 기록한다. `/1`·`/2`로 시작한 목표는 새 바이너리에서도 해당 규칙으로 계산하고 이후 실행에도 새 결과·예산 의미를 섞지 않는다. 자동 이전이나 과거 기록 재작성은 하지 않는다. 새 change 검사는 구조화 결과 계약에 연동되어야 유효 근거를 얻는다.

`ha`가 지원하지 않는 규칙 버전에는 계산도 기록도 하지 않는다. `ha status`, `ha done`, `ha check`, `ha note`, `ha budget`이 66으로 끝난다. 시작 규칙을 읽을 수 있으면 그 뒤 무결성 오류보다 미지원 규칙 거부가 우선한다. 지원 규칙에서도 미지 결과·부정합 데이터는 성공 근거가 아니다. 유효한 구기록의 의미만 호환 대상으로 보존한다.

| | `run-rules/1` | `run-rules/2` | `run-rules/3` |
| --- | --- | --- | --- |
| 묶음 문서(코드 상태에서 빼는 경로) | 자기 목표의 목표 문서와 실행 묶음 | 다른 목표의 실행 묶음 파일도 포함. PLAN의 `다른 목표 실행 묶음` 선언은 입력으로 유지 | `/2`와 같음 |
| 남은 이유가 `budget_exceeded`뿐일 때 | `budget_exhausted` | `complete`. `ha done`이 완료 기록을 남김 | `/2`와 같음 |
| change의 실패·통과 근거 | 프로세스 종료 결과 | `/1`과 같음 | 선언한 대상·요구 위반·오류·호출 연결을 구조화 결과로 확인 |
| 현재 창의 횟수 상한 변경 | 미지원 | 미지원 | `budget_change`, 사용량 유지 |
| 재개 창의 횟수 상한 | 최초 start 상한 | 최초 start 상한 | 최신 유효 횟수 상한 계승 |

**다른 목표의 실행 묶음 파일.** 자기 목표 디렉토리와 같은 부모 아래에서, 지금 작업 트리에 `SPEC.md`가 있는 다른 디렉토리 바로 아래의 `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, `tasks/`다. 그 디렉토리의 `SPEC.md`와 그것이 링크한 문서, `checks/` 같은 다른 파일은 코드다. 같은 부모 아래가 아니거나 `SPEC.md`가 없는 디렉토리(예: `examples/`, `testdata/` 아래의 목표 모양 디렉토리)도 코드다. 이 목록은 묶음 문서를 쓰는 모든 판단에 같이 쓴다.
- 작업 트리가 깨끗한지: §3의 전제, `check`·`baseline`이 남기는 `tree_clean`, 경로 확인 `confirm`
- 신선도: §3의 3
- base 이후 바뀐 파일: §4.2의 `out_of_scope`, `test_definition_changed`
- 완료 기록의 유효성: §5.1

같은 브랜치에서 여러 목표를 함께 진행할 때, 다른 목표가 진행 기록만 커밋해도 이 목표의 결과가 낡지 않게 하려는 것이다. 다른 목표의 실행 묶음을 검사 입력으로 읽는 목표는 PLAN `## 범위`에 `다른 목표 실행 묶음:` 항목을 두고 값으로 `검사 입력`을 쓴다([bundle.md](bundle.md) §2.2). 선언은 계산하고 기록할 때의 PLAN에서 읽는다.

**예산 초과만 남은 완료.** 필수 조건이 모두 충족되고 남은 이유가 `budget_exceeded`뿐이면 상태는 `complete`이고, `ha done`이 완료 기록을 남긴다. 다른 이유가 하나라도 함께 남으면 §4.3의 순서 그대로다. 예를 들어 사용자 확인 이유와 함께면 `budget_exhausted`이고 두 이유가 모두 나온다. 예산 사용량과 초과 여부는 `budget`에 계속 나온다. 이것은 이미 얻은 근거로 완료를 기록하는 것이고, 예산을 넘겨 더 검증하거나 작업할 권한이 아니다. 추가 사용에는 사용자 근거가 필요하다. `/3`의 같은 창 횟수 증액은 `budget_change`, 명시적 재작업·새 창은 `reopen`이며 `/1`·`/2`는 기존 `reopen`을 쓴다(§5.2). `ha check`는 실행 전에 예산을 확인하지 않는다.


## 5. 완료 기록과 예산

### 5.1 완료 기록

`ha done`이 `complete`를 계산하면 그 `done` 줄이 완료 기록이다. 완료 기록은 "`ha`가 이 코드 상태, 이 SPEC, 이 기록 head에서 완료를 확인했다"는 사실을 고정한다.

완료 기록 d는 다음을 모두 만족하는 동안 **유효하다**.
- 지금 작업 트리가 깨끗하다.
- d.commit과 HEAD 사이에 묶음 문서 밖의 차이가 없다. 묶음 문서는 규칙 버전에 따른다(§4.6).
- SPEC digest와 required 조건의 명령 digest가 d와 같다. `/3` change의 구조화 선언·생산자 입력도 같은 의미여야 한다.
- d 이후에 `reopen` note가 없다.
- d와 그 앞 줄의 `prev` 연결이 온전하다. 끊겼으면 완료 기록도 근거가 되지 않는다(`record_integrity`).

유효한 완료 기록이 있으면 상태는 `complete`이고, 그 뒤에 시간이 얼마나 지났는지와 무관하다. 완료 기록이 무효가 되면(코드나 SPEC이 바뀜, 재작업 요청) 기록은 이력으로 남고, 상태는 §4.1–§4.3대로 다시 계산한다.

`ha done`의 결과가 `complete`가 아니어도 `done` 줄은 남는다. 이것은 "완료를 주장했지만 완료가 아니었다"는 기록이다.

### 5.2 예산

예산은 진행 중인 작업에 적용한다. PLAN을 고치는 것만으로 진행 중 한도가 바뀌지는 않는다. 검사 실행 수는 CLI 호출 수가 아니라 `check`·`baseline` 줄 수다. 실패·오류·판정 불가·stale도 실제 시도라면 세며 baseline을 면제하지 않는다. 실행 전 거부와 worktree 준비 실패는 검사 실행과 구분한다. `/3`에서 선언한 overlay 파일을 준비할 수 없어 실행 전에 거부하면 검사 기록과 사용량이 생기지 않는다.

- **창**: 마지막 `start` 또는 `reopen`부터 마지막 저장 기록까지다. 검사 사용량은 기록 추가 순서로 귀속한다. 재개 전에 시작해 재개 뒤 기록된 검사도 새 창에 포함된다.
- **경과 시간**: 마지막 저장 기록 `at`에서 창 시작 `at`을 뺀 값이다. 현재 시각 조회로 늘지 않는다. 예산 변경을 포함한 새 기록이 붙으면 기다린 시간도 들어간다.
- **도달과 초과**: 사용량이 상한과 같으면 도달이고, `사용량 > 상한`일 때 초과다. 남은 횟수는 0 아래로 표시하지 않으며 초과량은 별도로 표시한다.
- **실행 전 강제 제한 없음**: 예산은 동시 실행 슬롯을 예약하지 않고, 진행 중 검사를 종료하거나 검사 시작을 강제 차단하지 않는다. 예산 초과만 남은 완료는 추가 실행 권한이 아니다.

`/1`·`/2`의 한도는 항상 `start.budget`이다. 사용자 발화를 인용한 `reopen`은 같은 최초 상한의 새 창을 연다.

`ha status <goal> --format json`의 `budget.window_start`, `revision`, `runs_limit`을 변경의 비교 대상으로 쓴다. 예를 들어 현재 창 1·개정 1·상한 50인 합성 목표의 변경 명령은 다음과 같다.

```bash
ha budget docs/specs/example --runs 100 --from 50 --window 1 --revision 1 --quote "검증 상한을 100회로 늘려줘" --context "이 목표의 현재 창 총상한 50회를 100회로 변경"
```

상태를 읽은 뒤 창이나 개정이 바뀌었다면 명령은 거부된다. 숫자를 임의로 갱신해 재시도하지 말고 변경된 상태와 사용자 요청의 적용 범위를 확인한다.

`/3`의 `budget_change`는 **현재 창의 총 실행 횟수 상한**만 바꾼다. 48/50에서 100으로 바꾸면 48/100이고 잔여는 52다. 40으로 낮추면 48/40, 잔여 0, 초과 8이다. 시간 상한·창 시작·실제 사용량은 유지하며 변경 자체는 검사 횟수가 아니다. 증액에는 대상과 한도를 뒷받침하는 사용자 원문과 요청 문맥이 필요하고 감액에는 이유를 남긴다. 에이전트 해석은 원문과 구분하며, 호스트 출처가 검증되지 않은 인용은 실행자가 기록한 주장이다. 짧은 동의나 호출 토큰을 형식만으로 거부하지 않는다.

변경은 현재 창·이전 예산 개정·이전 상한과 연결한다. 같은 창에 검사 기록이 추가돼도 사용량은 빠지지 않는다. 다른 변경이나 재개로 창·개정·상한이 달라졌으면 기록하지 않고 충돌을 알린다. 자동 합산·재적용하지 않는다. 빈 변경, 동일 값, 양수가 아닌 값, 지원하지 않는 시간 변경도 상한 변경 기록을 만들지 않는다.

예산 변경만으로 재작업·새 창·unblock이 생기거나 실패·오류 한도가 초기화되지 않는다. 완료 기록이 있는 창은 명시적 재개가 필요하다. `/3`의 `reopen`은 최신 유효 횟수 상한과 최초 시간 상한을 계승하고 새 창 사용량을 센다. 이전 창의 사용량과 변경 이력은 남는다.

출력 파일이 사라지거나 이름 변경에 실패해도 `/3`에서 실제 시도는 출력 참조가 빈 `error`·`process_output` 기록으로 보존한다. 이는 기록 저장소 자체의 쓰기 성공을 보증한다는 뜻은 아니다.

기록 쓰기 후 동기화·닫기 오류는 이미 적용됐을 수 있다. 실패를 성공 또는 미적용으로 단정하지 않으며 저장 상태 확인 없이 다시 적용하지 않는다. 저장소 손상·쓰기 불능에도 무손실 기록을 보증하지 않는다.

`/2`·`/3`는 필수 조건이 모두 충족되고 예산 초과만 남으면 완료를 기록할 수 있다(§4.6). 완료 뒤 재작업 요청은 `ha note reopen`으로 남기며 이전 완료 기록을 무효로 한다.

## 6. 규칙

1. 실행 기록은 덧붙이기만 한다. 실패한 실행을 지우거나 다시 실행해 덮지 않는다. 오류만 §4.1의 한도 안에서 다시 실행할 수 있다. 연결이 끊긴 기록에는 `ha`가 더 덧붙이지 않는다.
2. `ha` 밖에서 돌린 테스트 결과는 상태 계산에 쓰지 않는다.
3. 완료 선언은 `ha done`이 완료 기록을 남긴 뒤에만 한다.
4. `note`의 내용은 주장이다. `confirm`과 `reopen`은 사용자의 발화를 인용해야 하고, 보고서에 "사용자 확인(주장)"으로 표시된다. `confirm`은 §2.4의 `bound`가 지금과 같을 때만 효력이 있다.
5. 출력 원문은 커밋되는 파일(실행 묶음, `runs.jsonl`)에 넣지 않는다. 사람이나 에이전트가 원문을 봐야 하면 `ha log`를 쓴다.
6. 상태 계산 규칙은 `start` 때 고정된다. 판정 규칙의 변경은 새 규칙 버전이며 다음 `start`부터 쓴다.
