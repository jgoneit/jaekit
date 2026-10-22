# 계약: 실행 기록 (`run/v1`)과 완료 판정

이 문서는 다음에 대한 canonical 정의다.
- 실행 기록 `runs.jsonl`의 줄 형식
- 그 기록으로 조건 충족과 상태를 계산하는 규칙
- 완료 기록

실행 묶음은 [bundle.md](bundle.md)에 있다. 완료 뒤 발견은 [명시적 기록 확장](findings.md)으로 연결한다. 이 확장을 사용하는 자료에는 `run-finding/v1` 줄이 함께 있으며, 기존 `run/v1` 줄과 저장된 실행 규칙은 바꾸지 않는다.

실행 기록은 `ha`만 덧붙인다. 에이전트는 기록을 직접 쓰지 않고 `ha` 명령을 호출한다. 같은 사용자 권한의 에이전트가 파일을 고칠 수는 있으므로, 이 기록의 보증 수준은 `local`을 넘지 않는다.

## 1. `ha` 명령

| 명령 | 하는 일 | 기록 |
| --- | --- | --- |
| `ha lint <goal>` | 목표 문서 형식(조건 ID, `Assurance:` 줄), 실행 묶음 구조, 모든 조건 ID의 조건표 대응, 조건↔task 양방향 대응, 순환, 참조 파일 존재, 검증 방법 누락을 확인한다. 오류 코드는 [bundle.md](bundle.md) §8에 있다. | 없음 |
| `ha estimate <goal> [AC-n \| EX-n \| T001 ...] [--baseline] [--format md\|json]` | 현재 PLAN의 필수 최소 실행량, 상한·부족량, 기본 선택과 지정한 대상의 실행량을 읽기 전용으로 계산한다(§1.1). PLAN의 횟수 상한이 자동이면 산정 값·사전 승인 상한·승인 필요 여부도 보인다(§1.5). | 없음 |
| `ha start <goal> --request <인용> [--skill <SKILL.md> [--check-skill]] [--rules run-rules/4 \| run-rules/5] [--test-authority <해석>] [--host-name …] [--host-version …] [--model …]` | 시작을 기록한다. `--rules`가 없으면 기본 규칙 `run-rules/3`이다(§4.6). `--test-authority`는 `run-rules/4`·`run-rules/5`에서만 받는다(§4.7). `run-rules/5`는 §4.8이다. 이미 있으면 거절한다. 새 시작이 필요하면 사용자가 묶음을 새로 만든다. lint 오류나 필수 최소 실행량보다 작은 PLAN 상한이 있어도 기록 생성 전에 거절한다. dirty 시작은 기존처럼 경고하며 허용한다. `--skill`을 주면 `ha`가 그 머리말에서 이름과 버전을 읽고 Skill 디렉토리의 digest를 계산한다(§2.1). PLAN의 횟수 상한이 자동이면 상한을 산정하고, 사전 승인 상한을 넘거나 그 설정을 쓸 수 없으면 기록 생성 전에 거절한다(§1.5). skill의 Core 요구 선언이 이 실행 파일이 지원하지 않는 것을 요구하거나 손상됐으면 기록 생성 전에 거절한다(§1.6). | `start` |
| `ha check <goal> [AC-n \| T00n ...]` | 지정한 조건이나 task의 검증 명령을 실행한다. 대상이 없으면 모든 required 조건을 실행한다(통합 검증). | `check` |
| `ha check <goal> --baseline [AC-n ...]` | `change` 조건의 검사 경로만 base commit 임시 worktree에 덧씌워 실행한다. | `baseline` |
| `ha budget <goal> --runs N --from P --window W --revision R [--quote <인용> --context <문맥>] [--reason <이유>] [--interpretation <해석>]` | `/3`·`/4`·`/5` 현재 창의 총 실행 횟수 상한을 바꾼다. 양의 정수 인수가 모두 필요하며, 증액은 인용·문맥, 감액은 이유가 필요하다(§5.2). `/4`·`/5`는 사전 승인 상한 안의 증액을 이유만으로 받는다(§4.7). | `budget_change` |
| `ha budget <goal> --preapprove N --window W --revision R --quote <인용> --context <문맥>` | `/4`·`/5`만: 그 창의 사전 승인 상한을 기록한다. 상한 자체는 그대로다(§4.7). | `budget_change` |
| `ha note <goal> <block \| unblock \| confirm \| reopen \| input> [대상] --quote <인용> [--cause <원인>]` | 막힘, 해소, 사용자 확인, 사용자의 재작업 요청, 그 밖의 사용자 입력을 남긴다. 사용자 발화는 인용한다. `block`은 `--cause`가 필요하고, `confirm`은 대상(§2.4)과 인용이 필요하다. | `note` |
| `ha note <goal> <test-change \| wait \| resume \| recover \| allow-recovery> …` | `/4`·`/5`만: 시험 변경의 근거, 사용자 대기의 시작과 끝, 오류 한도 회복과 그 사용자 허용을 남긴다(§4.7). `/1`·`/2`·`/3` 목표는 65로 거부한다. | `note` |
| `ha note <goal> review --gate <file>` | `/5`만: Spar가 쓴 `review-gate/v1` 파일의 digest와 판정에 쓰는 요약을 남긴다(§4.8). `/1`~`/4` 목표는 65로 거부한다. 쓸 수 없는 파일은 이유와 함께 65로 거부하고 기록하지 않는다. | `note` |
| `ha status <goal> [--format md\|json]` | 기록·묶음·git에서 상태를 계산해 출력한다(§4). | 없음 |
| `ha done <goal>` | 완료를 주장한다. `ha status`와 같은 계산을 하고 그 결과를 기록한다. 결과가 `complete`면 이 줄이 **완료 기록**이다(§5). exit code는 `ha status`와 같다. | `done` |
| `ha log <goal> <seq> [--tail N]` | 그 실행의 로컬 출력 원문을 보여 준다. 로컬에서 읽기 위한 것이다. | 없음 |
| `ha timing <goal> [--format md\|json]` | 기록을 읽기만 해서 실행마다 시간 제한·제한 구간 경과·CPU를 보여 준다(§1.2). 검사를 실행하지 않고, 판정·예산에 쓰지 않는다. | 없음 |
| `ha report <goal> [--lang ko\|en]` | 사람이 읽는 다섯 줄 요약과 `PROGRESS.md` `## 현재`에 쓸 도장, 조건별 근거, 완료 범위, 미충족 조건의 원문 위치를 보인다(§1.3). `ha status`와 같은 계산을 읽기만 한다. | 없음 |
| `ha doctor <goal>` | PLAN의 선택 절 `## 실행 환경`에 적은 점검 명령을 실행해 도구마다 `사용 가능`·`사용 불가`를 보인다(§1.4, [bundle.md](bundle.md) §2.6). 판정·예산과 무관하다. | 없음 |
| `ha history <goal> [add \| current \| list \| show] …` | 선택적 [판단 이력](decision-history.md)을 등록·조회한다. 공통 Git 디렉토리의 별도 이력에 쓰며 상태 계산은 이 이력을 읽지 않는다. | 없음 (`runs.jsonl` 밖) |

`ha`는 검증 명령을 셸 없이 argv로 실행한다. 명령마다 시간 제한(기본 900초)을 둔다. 시간 제한이 되면 Unix에서는 그 명령의 프로세스 그룹, Windows 네이티브 후보에서는 실행 전에 배정한 kill-on-close Job Object를 종료한다. Windows는 일반 하위 프로세스가 끝났는지 확인한 뒤 출력을 확정하며, 프로세스 제어·종료 확인에 실패하면 통과 근거나 최종 출력 digest를 만들지 않는다. 시작 뒤 정리 실패도 제한된 시간 안에 오류로 반환한다. 임의의 외부 서비스나 별도 권한으로 실행한 프로세스를 격리하는 보장은 아니다. `check`, `baseline`, `note`, `start`, `budget_change`는 기록에 덧붙이기 전에 git 디렉토리의 `ha/lock`을 잡는다. Unix는 `flock`, Windows 네이티브 후보는 같은 파일의 배타적 `LockFileEx` 잠금을 사용하며, 프로세스 종료로 OS 잠금이 해제되어도 손상된 기록은 기존 연결 검증에서 거부한다. 그래서 여러 `ha`가 동시에 덧붙여도 연결이 끊기지 않는다.

**종료 코드**
- `ha check`: 모든 결과가 기대대로면(`pass`, baseline은 `fail_as_expected`) 0, 아니면 1
- `ha lint`: 오류가 없으면 0, 있으면 1
- `ha estimate`: 필수 최소량을 PLAN 상한이 수용하면 0, 부족하거나 lint 오류가 있으면 1. 자동 예산이면 산정 값이 사전 승인 상한 이하일 때 0, 사용자 승인이 필요하면 1, 사전 승인 상한 설정을 쓸 수 없으면 65(§1.5)
- `ha status`, `ha done`: §4.3
- `ha timing`: 성공 0. 끊긴 기록은 65(거절)이며 아무것도 보여 주지 않는다. 보기를 출력에 쓰지 못하면 70이다
- `ha report`: 보고를 출력하면 상태와 무관하게 0. 사용법 오류(`--lang`이 `ko`·`en`이 아님 포함) 64, 목표 디렉터리 없음 65, 지원하지 않는 규칙 66, Git이 base와 HEAD를 비교할 수 없음(다섯 줄 요약은 출력함, §1.3)·내부 오류·출력 실패 70. 끊긴 기록은 거절하지 않고 `record_integrity`가 있는 계산 결과를 보인다
- `ha doctor`: 모든 도구가 `사용 가능`이거나 절·항목이 없으면 0, 하나라도 `사용 불가`면 1, 절의 입력 오류와 PLAN.md 없음 65(아무 점검도 실행하지 않음), 사용법 오류 64, 프로세스를 제어·확인하지 못하면 70. 기록을 판정에 쓰지 않으므로 시작 전, 지원하지 않는 규칙, 끊긴 기록의 목표에도 같다
- `ha budget`: 성공 0, 잘못된 인수 64, 값·창·개정·완료 상태 등의 거부 65(`/3` 목표의 `--preapprove` 포함), 구규칙 또는 미지원 규칙 66, 내부 오류·쓰기 적용 불확실성 70
- 모든 명령: 64 사용법 오류, 65 거절(시작 전, 이미 시작함, lint 오류, 끊긴 기록, 확인 조건 불충족, dirty 검증, 시작 예산 부족, 자동 예산의 사용자 승인 필요, 사전 승인 상한 설정 오류, skill의 Core 요구 미지원·선언 손상, 쓸 수 없는 검토 gate 파일), 66 지원하지 않는 규칙 버전, 70 내부 오류

출력 원문은 git 디렉토리의 `ha/runs/<goal-key>/<seq>.log`에만 둔다. `<goal-key>`는 저장소 기준 goal 경로를 URL 경로 이스케이프한 것이다(예: `docs%2Fspecs%2Fempty-input`). 목표가 여럿이면 seq가 겹치기 때문이다. 실행 기록과 `ha status`·`ha done`의 출력에는 결과, 이유, digest, 로컬 경로와 명시적으로 기록한 사용자 확인·예산 변경 정보를 넣는다. 출력 원문은 넣지 않는다. 출력에 비밀이 섞일 수 있고, 상태 출력은 완료 보고에 보존되기 때문이다. 목표 문서와 기록의 Git 추적 여부는 프로젝트 정책에 따른다. 원문은 `ha log`로만 본다.

### 1.1 실행 전 진단

**작업 상태.** `check`·`baseline`·task 검증은 각 대상의 프로세스를 시작하기 직전에 적용 규칙의 제외 경로를 반영해 Git 상태를 확인한다. 제외되지 않는 변경이 있으면 exit 65로 거부한다. Git 상태를 읽지 못하면 exit 70이며 실행하지 않는다. 거부된 대상의 검사 기록·실행 횟수·출력 파일은 만들지 않는다. 여러 대상 중 앞선 대상만 실행됐다면 수행/미수행 개수를 출력하고 앞선 결과는 그대로 보존한다. 실행 후 생긴 오류나 stale 결과도 기록과 사용량에 남는다.

진단은 저장소 상대 경로, tracked/untracked, 관측한 symlink 여부를 구분하며 삭제·이름 변경의 경로와 submodule 상태도 표시한다. untracked 경로는 해당 경로의 ignore 규칙 검토를 안내할 수 있으나 자동으로 바꾸지 않는다. tracked 변경을 ignore 추가로 해결할 수 있다고 안내하지 않는다. 실행 중 파일 변경을 원자적으로 막는 기능은 아니며 현재 완료 근거는 §3의 신선도 판정을 계속 적용한다.

이 거부는 지원하는 `/1`·`/2`·`/3` 목표의 앞으로 실행할 검사에 적용한다. 과거 `tree_clean: false` 기록의 결과와 사용량을 삭제하거나 재해석하지 않는다. 시작의 dirty 경고와 기존 Git ignore·규칙별 실행 문서 제외도 유지한다.

**실행량.** `ha estimate`의 필수 최소량은 `필수 change × 2 + 필수 maintain`이다. manual은 0회이며 change 34개면 68회다. 실패 없는 새로운 실행의 하한이므로 재시도·환경 준비·수동 확인 시간·실제 총시간 또는 완료를 보장하지 않는다. 검사나 환경 probe를 실행하지 않고 기록·예산 창·작업 파일도 바꾸지 않는다.

기본 `check`는 필수 change/maintain, 기본 `--baseline`은 조건표의 모든 change(선택 AC와 EX 포함)를 선택한다. `estimate`는 이 기본 실행량과 필수 하한을 넘는 추가량을 함께 표시한다. 명시한 대상은 실제 `check`와 같은 순서로 선택한다. 선택 AC·EX는 추가량, task의 각 명령은 각각 한 회이며, 중복 대상도 실제 실행과 같이 반복 계산한다. `--baseline`의 명시 대상은 change 조건만 허용한다. manual만 있는 기본 선택은 추정치 0이며 실제 `check`의 no-target 오류는 바뀌지 않는다.

상한은 **현재 PLAN의 전체 상한**이다(PLAN의 횟수 상한이 자동이면 §1.5의 산정 값). 진행 중 목표의 남은 예산이 아니다. 새로운 `start`는 상한이 필수 하한보다 작으면 최소량·상한·부족량을 출력하고 기록을 만들기 전에 exit 65로 거부한다. 같은 값이면 시작할 수 있다. 선택 검사나 재시도 여유 때문에 자동 증액하지 않는다. 기존 목표의 상한과 재개 창은 §5.2 그대로이며 진행 중 검사에 예산 제한을 새로 적용하지 않는다.

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

### 1.2 시간 보기 (`ha timing`)

`ha timing <goal>`은 기록의 `check`·`baseline` 줄마다 순번, 대상(조건 ID, 또는 task와 그 명령 순번), 종류(`check`·`baseline`), 결과, `duration_ms`, 적용 제한, 제한 구간 경과와 근접도, 제한 발동 여부, CPU를 보여 준다. 값의 뜻은 §2.2의 `timing`이다. 이 보기는 검사를 실행하지 않고 기록·로그·잠금·작업 트리를 바꾸지 않는다. 상태 계산·신선도·사용량·예산은 이 값을 읽지 않는다. 기록이 끊겼으면 아무것도 보여 주지 않고 65로 끝난다.

`--format json`은 `timing/v1` 문서 하나다.

| 필드 | 의미 |
| --- | --- |
| `schema` | `"timing/v1"` |
| `goal` | 저장소 기준 목표 디렉터리. 명령에 절대 경로를 줘도 상대 경로다 |
| `runs[]` | 기록 순서의 실행. `seq`, `target`, `kind`, `result`, `duration_ms`, `limit_ms`, `limit_elapsed_ms`, `limit_use_permille`, `limit_reached`, `cpu`, `unavailable` |
| `total_duration_ms` | 나열한 실행의 `duration_ms` 합 |
| `total_meaning` | 합계의 뜻. 실행은 겹칠 수 있고, 이 합은 목표 경과 시간이나 예산이 아니다 |

- 단위는 밀리초 정수다. `limit_use_permille`은 `limit_elapsed_ms × 1000 ÷ limit_ms`의 정수 몫(소수 버림)이다. 두 값 중 하나라도 없으면 `null`이다. 몫이 64비트 정수를 넘으면 `null`이고 `invalid`다.
- 값이 없으면 그 필드는 `null`이고, `unavailable`에 이유가 있다. `not_recorded`는 그 값이 줄에 없다는 뜻이다(이 필드가 생기기 전에 기록한 줄 등). `not_measured`는 기록된 `null`, 곧 제한 타이머가 켜지지 않았다는 뜻이다. `invalid`는 타입이나 값이 맞지 않는다는 뜻이다. 0이나 900초 같은 값으로 채우거나 규칙 버전에서 추정하지 않는다.
- 한 값이 손상돼도 그 값만 `invalid`다. 같은 줄의 다른 값과 다른 줄은 그대로 보인다. `timing` 필드가 없으면 다섯 값이 모두 `not_recorded`이고, 객체에 없는 키도 `not_recorded`다. `timing`이 `null`이거나 객체가 아니면 다섯 값이 모두 `invalid`다.
- `cpu`는 기록의 `{ "status", "user_ms", "system_ms", "scope", "source" }`를 키마다 따로 읽어 보인다. 기록에 있는 키는 그대로 나오고, 그 값이 손상됐으면 그 키만 `null`이며 `unavailable`의 `cpu.<키>`가 `invalid`다. 같은 CPU의 다른 키는 그대로 보인다. `status`가 `measured`면 다섯 키가 모두 나오고, 기록에 없는 키도 `null`과 `invalid`다. `unavailable`·`failed`는 기록에 있는 키만 나오며, 그 상태에 시간이 있으면 그 시간은 `null`과 `invalid`다. `status`가 세 값이 아니면 `null`과 `invalid`다. `scope`·`source`는 §2.2의 고정 코드만 보이고, 그 밖의 값은 `null`과 `invalid`다. 그래서 기록에 들어간 임의 문자열이 보기로 나오지 않는다. `cpu`가 객체가 아니면 `cpu`가 `null`이고 `unavailable`의 `cpu`가 `invalid`다.

사람이 읽는 형식은 같은 값을 표 하나로 보이고, 실행 시간의 합과 그 합이 목표 경과 시간이나 예산이 아니라는 문장을 덧붙인다. CPU 칸은 상태가 무엇이든 JSON이 보이는 키를 같은 값이나 같은 이용 불가 표시로 보인다.

### 1.3 사람용 보고 (`ha report`)

`ha report <goal> [--lang ko|en]`은 한 목표를 사람이 읽는 형태로 보인다. `ha status`와 같은 기록·실행 묶음·목표 문서·Git을 같은 계산(§4)으로 읽고, 그 결과만 쓴다. 조건 충족·상태·이유·완료 기록의 유효성을 따로 판단하지 않으므로 보고는 `ha status`와 다른 상태를 말하지 않는다. 기록에 줄을 더하지 않고 `ha/lock`을 잡지 않으며 `.git/ha`·실행 묶음·작업 트리·index·ref를 바꾸지 않는다. 현재 시각을 읽지 않는다: 같은 기록·묶음·Git·`--lang`이면 출력이 같다. `status/v1`과 다른 명령의 출력은 이 명령 때문에 바뀌지 않는다.

`--lang`은 `ko`·`en`이고 없으면 `en`이다. 언어는 표지·제목·설명 문장만 바꾼다. 이유 코드, 분류(`executor`·`user`), 상태 이름, 조건 ID, 종류, seq·hash·commit, 원인 코드, `ha log`·`ha start`·`ha done`·`ha lint` 명령, `none`, `no stored output`, `local`, `## 현재`는 두 언어에서 같다.

**다섯 줄 요약.** 출력의 첫 다섯 줄이며 순서와 표지가 고정이다. 영어는 `Changed`, `Checked`, `To review`, `Remaining limits`, `Next step`, 한국어는 `바뀐 것`, `확인한 것`, `직접 볼 것`, `남은 한계`, `다음 행동`이고, 표지 뒤는 `: `이다.

| 표지 | 내용 |
| --- | --- |
| `Changed` / `바뀐 것` | 시작 기록의 base commit부터 HEAD까지 실행 묶음 문서 밖에서 바뀐 파일 수(`ha status`의 `out_of_scope`와 같은 목록의 길이)와 그 경로를 바꾼 병합 아닌 commit 수. 시작 전이면 base가 없다는 사실, Git이 비교하지 못하면 그 사실(아래) |
| `Checked` / `확인한 것` | 필수 조건 수와 그중 충족한 수(검사 기록으로 충족한 `change`·`maintain` 수와 사용자 확인으로 충족한 `manual` 수), 조건표의 모든 `change` 조건 수와 그중 신선한 `fail_as_expected` baseline이 있는 수. 그 baseline은 상태 계산이 근거로 받는 것이어야 한다(결과와 종료 코드가 맞고, 구조화된 결과의 `change` 조건이면 저장된 근거 요약이 검증을 통과한다) |
| `To review` / `직접 볼 것` | 이 순서의 전체 집합: 분류가 `user`인 남은 이유와 그 대상, 인용 확인으로 충족한 `manual` 조건과 그 확인 seq, 실행자 근거로 면제된 시험 변경(§4.7), `--weakened` 표시가 있는 시험 변경, 시작 뒤 바뀐 검증 명령(`changes.command_changed`), 해결되지 않은 완료 뒤 발견 수. 없으면 `none` / `없음` |
| `Remaining limits` / `남은 한계` | 늘 보증 수준 `local`과 검사 작성자 실행자. 충족되지 않은 선택 조건, 예산 초과(실행·경과의 사용량과 상한), 지금 HEAD를 덮는 완료 기록이 없다는 사실을 해당할 때 더한다 |
| `Next step` / `다음 행동` | 상태마다 하나: 시작 전이면 `ha start <goal>`(lint 문제가 있으면 `ha lint <goal>` 먼저), 그 밖의 `incomplete`는 분류가 `executor`인 첫 이유와 그 대상, `needs_user`는 분류가 `user`인 이유 전부와 그 대상, 완료 기록이 없는 `complete`는 `ha done <goal>`, 유효한 완료 기록이 있으면 그 seq와 남은 할 일이 없다는 사실, `blocked`는 마지막 `block`의 seq와 원인, `budget_exhausted`는 사용자의 예산 결정과 사용량·상한 |

이유의 대상은 조건 이유면 조건 ID(`` AC-2 `criterion_failed` ``), 목표 수준 이유면 그 이유가 가리키는 task나 경로(`` `task_open` (T002) ``, `` `out_of_scope` (lib/other.txt) ``)다. 요약의 값에 줄바꿈이 있으면 `\n`으로 써서 각 줄을 한 줄로 지킨다.

**Git이 base와 HEAD를 비교할 수 없을 때.** base commit이 없는 얕은 clone처럼 Git이 그 범위를 읽지 못하면 `ha status`는 지금처럼 계산하지 못하고 70으로 끝난다. `ha report`는 다섯 줄만 보이고 70으로 끝난다: `Changed`는 비교할 수 없다는 사실, `Checked`와 `To review`는 계산하지 못했다는 사실, `Remaining limits`는 `local`과 검사 작성자 뒤에 나머지 한계를 계산하지 못했다는 사실, `Next step`은 base가 든 이력을 받는 방법(예: `git fetch --unshallow`)과 다시 실행할 명령이다. 상세는 보이지 않는다. 범위를 읽는 일 하나라도 실패하면 이 경우다: `ha status`와 같은 계산이 바뀐 파일을 읽다 실패한 경우와 `ha report`가 파일 수와 commit 수를 읽다 실패한 경우 모두다. 실패한 읽기는 다시 읽지 않는다.

```text
Changed: Git cannot compare base 3c1f0a9e7b25 with HEAD 9a8b7c6d5e4f
Checked: not computed; Git cannot compare base 3c1f0a9e7b25 with HEAD 9a8b7c6d5e4f
To review: not computed; Git cannot compare base 3c1f0a9e7b25 with HEAD 9a8b7c6d5e4f
Remaining limits: assurance local; check author: executor; the other limits were not computed
Next step: fetch the history that contains base 3c1f0a9e7b25 (for example `git fetch --unshallow` in a shallow clone), then run `ha report docs/specs/example` again
```

**근거 출처.** 유효한 완료 기록이 있으면 그 `done` 줄에 저장된 조건(§2.5)을, 없으면 지금 계산한 조건을 쓴다. 충족 수, 인용 확인 `manual`, 충족되지 않은 선택 조건, 조건별 근거 표가 이 출처 하나를 읽는다. 필수 여부는 목표 문서에서 읽는다.

**상세.** 요약 뒤에 빈 줄로 나눈 네 부분이 이 순서로 온다.
- `` ### Stamp for `## 현재` `` / `` ### `## 현재` 도장 ``: 기록 head의 seq와 hash 앞 12자, 그 줄에 저장된 시각(`at`) 문자열, 상태, 유효한 완료 기록 seq(있으면). `PROGRESS.md`의 `## 현재`에 확인 시각과 기록 head로 옮겨 적는 값이다. 기록이 없으면 기록 head는 `none`이고, 저장된 시각을 RFC 3339로 읽을 수 없으면 읽을 수 없다고 보인다.
- `### Evidence per condition` / `### 조건별 근거`: 근거 출처(완료 기록 seq 또는 지금 상태)와 조건마다 ID·종류·필수·충족·근거 기록 seq의 표.
- `### Completion coverage` / `### 완료 범위`: 유효한 완료 기록의 seq·commit과 HEAD를 덮는다는 사실, 또는 무효가 된 가장 최근 완료 기록의 seq·commit, HEAD를 덮지 않는다는 사실, §4.4와 같은 무효 이유, 그 commit 뒤의 차이가 완료에 들지 않는다는 사실. 완료 기록이 없으면 `none`.
- `### Output of unmet conditions` / `### 미충족 조건의 원문 위치`: 충족되지 않은 필수 조건마다 `### Unmet`(§4.4)과 같은 기록 선택과 표기(`ha log <goal> <seq>`, 원문이 없으면 `no stored output`). 영어 줄은 `ha status`의 해당 줄과 같다. 없으면 `none` / `없음`.

합성 예시(완료 기록이 유효한 목표):

```text
Changed: 2 files and 2 commits outside the bundle documents since base 3c1f0a9e7b25
Checked: 2 of 2 required conditions satisfied (2 by check records, 0 by user confirmation); 1 of 1 change conditions have a fresh baseline
To review: none
Remaining limits: assurance local; check author: executor
Next step: none; completion record seq 5 holds and nothing is left to do
```

```text
바뀐 것: base 3c1f0a9e7b25 이후 실행 묶음 문서 밖에서 파일 2개, commit 2개
확인한 것: 필수 조건 2개 중 2개 충족(검사 기록 2개, 사용자 확인 0개); change 조건 1개 중 1개에 신선한 baseline
직접 볼 것: 없음
남은 한계: 보증 수준 local; 검사 작성자: 실행자
다음 행동: 없음; 완료 기록 seq 5이 유효하고 남은 할 일이 없음
```

### 1.4 실행 환경 점검 (`ha doctor`)

`ha doctor <goal>`은 PLAN의 선택 절 `## 실행 환경`([bundle.md](bundle.md) §2.6)에 적은 점검 명령을 항목 순서대로 하나씩 실행한다. 저장소 루트에서, 셸 없이 인자 배열로, 표준 입력을 닫고, `ha`의 환경 변수를 물려 실행한다. 점검마다 시간 제한은 60초로 고정이고 PLAN의 `## 시간 제한`과 무관하다. 시간 제한이 되면 `ha check`와 같은 프로세스 제어(Unix는 프로세스 그룹, Windows 후보는 kill-on-close Job Object)로 그 프로세스와 하위 프로세스를 끝낸다.

도구마다 한 줄 `<이름>: 사용 가능` 또는 `<이름>: 사용 불가 (<이유>)`를 보인다. 종료 0이면 `사용 가능`이다. `사용 불가`의 이유는 다음 다섯 가지가 전체 집합이다.
- `실행 파일 없음`: 권한 거부가 아닌 시작 실패(PATH에 없음, 파일 없음, 실행 형식이 아님)
- `실행 권한 없음`: 권한 거부로 시작하지 못함
- `시간 초과`: 60초 제한에 걸림. `시간 초과, 60초`로 보인다
- `신호로 종료`: 신호로 끝남. `신호로 종료, 신호 <번호>`로 보인다
- `0이 아닌 종료`: 그 밖의 0 아닌 종료. `0이 아닌 종료, 종료 코드 <코드>`로 보인다

`사용 불가` 줄 아래에 그 점검 출력의 끝부분을 줄마다 공백 네 칸을 붙여 보인다. 끝부분은 마지막 20줄이고, 그 20줄이 2,000바이트를 넘으면 마지막 2,000바이트(UTF-8 글자 경계에서 시작)다. 줄 끝의 `\r`은 뗀다. 시작 실패에는 `ha check`의 출력 파일과 같은 시작 실패 진단 한 줄이 보인다. `사용 가능`인 점검의 출력은 보이지 않는다.

`ha doctor`는 아무것도 남기지 않는다. 점검 출력은 운영체제 임시 디렉터리의 새 파일에 받고 그 점검의 줄을 쓴 뒤 지운다. 기록 줄·실행 횟수·`ha/lock`·`ha/runs`의 출력 파일·실행 묶음 변경이 생기지 않는다. 기록을 판정에 쓰지 않으므로 시작 전에도, 어느 규칙의 목표에도 같다. `ha`가 프로세스를 제어하거나 종료를 확인하지 못하면(Windows 후보에서 Job Object를 만들거나 설정하지 못한 경우 포함) 그 도구를 `사용 불가`로 세지 않고 그때까지의 줄을 남긴 채 70으로 끝난다.

절이 없으면 그렇다는 한 줄, 항목이 없으면 그렇다는 한 줄을 쓰고 0으로 끝난다. 절의 입력 오류는 lint와 같은 `코드<탭>파일:줄<탭>설명` 형식으로 모두 보이고 어떤 점검도 실행하지 않은 채 65로 끝난다. 이 절은 `ha doctor`만 읽는다: 절이 있든 없든, 어떤 내용이든 `ha lint`·`ha start`·`ha check`·`ha status`의 결과는 같다.

합성 예시:

```text
셸: 사용 가능
브라우저: 사용 불가 (실행 파일 없음)
    ha: could not start "synthetic-browser": exec: "synthetic-browser": executable file not found in $PATH
```

### 1.5 자동 예산

PLAN `## 예산`의 `검증 실행` 값이 정확히 `자동`이면(`- 검증 실행: 자동`) 횟수 상한을 `ha start`가 정한다. 숫자가 든 `검증 실행` 항목이 하나라도 있으면 지금처럼 그 숫자(여럿이면 마지막 것)를 쓰고, `자동`은 숫자가 든 항목이 하나도 없을 때만 적용된다. 그래서 이 기능 전의 `ha`가 숫자로 읽은 PLAN은 모두 같은 숫자로 읽는다. `경과 시간`에는 자동이 없고 지금처럼 필수다. `ha lint`는 자동 표시를 문제로 보지 않고 사용자 설정을 읽지 않는다.

**산정.** 상한은 `max(50, ⌈최소량 × 1.5⌉)`이다. 최소량은 §1.1의 필수 최소량(`필수 change × 2 + 필수 maintain`)이다. 예: 필수 change 34개, maintain 0개면 최소량 68, 상한 102. 필수 조건이 모두 manual이면 50.

**사전 승인 상한.** 사용자가 미리 승인한 상한은 사용자 설정 폴더의 `ha.json`에서 읽는다. 폴더는 `XDG_CONFIG_HOME`이 비어 있지 않으면 `$XDG_CONFIG_HOME/jaekit/`, 아니면 `~/.config/jaekit/`다. 홈은 `HOME`이 비어 있지 않으면 그 값, 아니면 운영체제의 홈(Windows는 `USERPROFILE`)이다. macOS·Linux·Windows에서 같은 규칙이며 다른 후보로 넘어가지 않는다. Spar 설정(`spar.toml`)과 같은 `jaekit` 폴더다. 파일은 JSON 객체 하나이고, 멤버는 정확히 `preapproved_runs` 하나, 값은 부호·소수점·지수 없는 정수 1~2147483647이다. 맨 앞의 UTF-8 BOM 하나는 허용한다.

```json
{"preapproved_runs": 120}
```

- 파일이 없으면(그 경로에 항목이 없고, 이미 있는 가장 가까운 상위 항목이 폴더) 상한은 50이고 출처는 `default`다. 설정하지 않은 사용자는 50으로 덮이는 목표에서 질문을 받지 않는다.
- 그 밖에는 거부한다: 대상이 없는 symlink(그 경로나 상위 폴더), 일반 파일이 아닌 항목, 64 KiB를 넘는 파일, 읽기 실패, 폴더가 아닌 상위 항목, 절대 경로가 아닌 `XDG_CONFIG_HOME`이나 홈, 정할 수 없는 홈, JSON이 아니거나 잘못된 UTF-8, 중복 키·모르는 키(대소문자만 다른 `preapproved_runs`도)·키 없음·뒤에 붙은 데이터, 정수 리터럴이 아니거나 범위 밖인 값.
- 파일의 실제 경로(symlink 해소)가 목표 저장소의 어느 작업 트리(`git worktree list --porcelain -z`의 최상위들) 안이면 거부한다. 폴더는 이름이 아니라 동일성으로 비교한다. 저장소 안의 파일은 이 상한을 정하지 못한다. 파일이 있는데 그 목록을 읽지 못하면(`-z`가 없는 Git 2.36 전 등) 거부한다. 파일이 없으면 목록을 읽지 않는다.
- 이 파일은 `ha start`와 `ha estimate`가 PLAN이 자동일 때만 읽는다. 숫자 PLAN과 다른 명령은 읽지 않으므로, 손상된 설정이 숫자 예산 목표를 막지 않는다.

설정을 쓸 수 없으면 두 명령은 `ha: pre-approved run limit: <경로>: <이유>`를 보이고 기록 없이 65로 끝난다. 기본값으로 대신하지 않는다.

**시작.** 산정 값이 사전 승인 상한 이하이면 시작한다. 시작 기록의 `budget.runs`가 산정 값이고 정보 객체 `auto`가 산정 근거를 남긴다(§2.1). 출력 둘째 줄은 `minimum required: <n> runs; auto limit: <m> (minimum × 1.5, at least 50); pre-approved limit: <c> from <설정 파일 경로 또는 default>`다. 산정 값이 상한보다 크면 기록 없이 65로 끝나며 표준 오류에 최소량, 산정 값, 상한과 출처, 사용자의 승인이 필요하다는 것을 보인다. 합성 예:

```text
ha: auto budget 102 runs (minimum 68 × 1.5, at least 50) exceeds the pre-approved limit 50 from default; the user's approval is needed: write the approved number in PLAN `검증 실행`, or have the user raise preapproved_runs in <설정 폴더>/jaekit/ha.json
```

사용자가 승인하면 그 숫자를 PLAN `검증 실행`에 쓰거나(숫자 상한은 사전 승인 상한과 비교하지 않는다), 사용자가 설정의 `preapproved_runs`를 올린다. 이 사전 승인 상한은 `/4`의 `ha budget --preapprove`(§4.7)와 별개로 시작 상한만 정한다. 시작 뒤 증액·재개 규칙(§4.7, §5.2)과 상태 계산은 숫자로 시작한 목표와 같다.

**추정.** `ha estimate`는 같은 산정을 기록 없이 한다. 자동 예산이면 JSON에 `auto` 객체(`computed_runs`, `preapproved_runs`, `source`, 설정 파일이 있을 때만 `source_path`, `approval_needed`)를 더하고 `plan_runs_limit`은 산정 값이다. Markdown 형식은 `- auto budget: <m> runs (minimum <n> × 1.5, at least 50); pre-approved limit: <c> from <출처>; user approval needed: yes|no` 줄을 더한다. 숫자 PLAN의 출력은 그대로다.

### 1.6 skill의 Core 요구 확인

`ha start --skill <SKILL.md>`는 그 파일과 같은 폴더의 `core-requirements.json`을 읽는다. 형식은 [Core 지원 정보 계약](core-capabilities.md) §8의 `core-requirements/v1`이다.

- 그 경로에 항목이 없으면 선언이 없는 skill이고 시작은 지금과 같다.
- 선언한 규칙·형식·기능 가운데 이 실행 파일의 지원 정보(`ha capabilities`와 같은 값)에 없는 것이 있으면, 빠진 항목마다 `ha: missing Core support: <rules|formats.<키>|features> <값> (ha <버전>)`을 보이고(`formats`의 키 자체가 없고 그 목록이 비어 있으면 값 없이 `formats.<키>`) 기록 없이 65로 끝난다.
- 선언을 읽을 수 없거나 형식에 맞지 않으면 `ha: skill requirements <경로>: <이유>; nothing recorded`를 보이고 65로 끝난다. 모르는 값을 무시하고 시작하지 않는다.
- `--check-skill`을 주면 선언이 필수다. 선언이 없으면 65, `--skill` 없이 주면 사용법 오류 64다. 이 기능 전의 `ha`는 이 플래그를 모르는 플래그로 64에 끝내고 기록하지 않는다. 그래서 확인이 반드시 일어나야 하는 시작은 이 형식을 쓴다.
- 확인은 작업 트리 확인 뒤, 기록 전에 한다. 선언 경로에 항목이 있으면 Skill 디렉토리 digest를 계산하기 전에 확인하므로, 쓸 수 없는 선언(대상 없는 symlink, 폴더, FIFO 등)은 위 이유로 거부한다. 선언이 없는 skill은 지금처럼 먼저 읽고 그 오류도 지금과 같다. 시작 기록의 `skill`(`name`, `version`, `digest`)은 그대로이고 digest에 선언 파일도 들어간다.

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
- `amendments/**`: 수정안 문서([goal-docs.md](goal-docs.md) §6.6). SPEC.md가 링크해도 목표 문서 집합에 들지 않는다.

같은 부모 아래 다른 목표 디렉토리(§4.6)의 `amendments/**`도 규칙 버전에 관계없이 코드 상태에서 뺀다. goal 디렉토리 안의 다른 파일(검증 스크립트, fixture 등)은 코드로 다룬다. **코드 상태**는 다음 둘을 합친 것이다.
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
| `budget` | PLAN의 예산 `{ "runs", "elapsed_seconds", "cost" }`. 최초 한도다. `/1`·`/2`는 이 값을 계속 쓰고, `/3`의 횟수 상한은 유효한 변경 기록을 반영한다(§5.2). 자동 예산(§1.5)이면 `runs`가 산정 값이고 정보 객체 `"auto": { "minimum_runs", "computed_runs", "preapproved_runs", "source" }`가 더해진다. `source`는 `default` 또는 `user_config`이며 사용자 경로는 넣지 않는다. 상태 계산은 `auto`를 읽지 않는다. |

`/4` 시작 줄에는 `scope`(`allowed`, `tests`: 시작 때 PLAN `## 범위`의 경로 사본)가 있고, `--test-authority`를 주면 `test_authority`(`interpretation`)도 있다(§4.7).

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
| `timing` | 선택적 프로세스 측정값(아래). 판정·신선도·사용량·예산에 쓰지 않는다. |
| `duration_ms`, `output_digest`, `output_path` | 실행 시간, 출력 digest, 로컬 출력 위치(저장소 기준 경로, 예: `.git/ha/runs/<goal-key>/7.log`). `output_path`에는 절대 경로를 쓰지 않는다. linked worktree에서도 형식이 같고, 이때 `.git`은 그 작업 트리의 git 디렉토리를 뜻한다. 이전 `ha`가 남긴 절대 경로는 고쳐 쓰지 않고 그대로 읽는다. `/3`에서 실제 시도 뒤 출력 저장이 실패하면 `error`·`process_output`으로 시도와 사용량을 남기고 `output_path`·`output_digest`는 빈 값일 수 있다. |

`timing`은 이 실행의 측정값이다. 이 필드가 없는 줄(이전 `ha`의 줄)은 그 값이 기록되지 않은 것이다.

| 필드 | 의미 |
| --- | --- |
| `limit_ms` | 이 실행에 적용한 시간 제한. 밀리초 정수 하나이며, 두 목표가 함께 쓰는 적용 제한의 유일한 기록값이다 |
| `limit_elapsed_ms` | 제한 타이머가 켜진 때(프로세스 시작에 성공한 뒤)부터 프로세스 종료를 확인했거나 시간 초과를 결정한 때까지. 그 뒤의 하위 프로세스 정리·종료 확인 시간은 들어가지 않는다. 프로세스가 시작하지 못해 타이머가 켜지지 않았으면 `null`이다 |
| `limit_reached` | 시간 제한이 발동했는가 |
| `cpu` | `{ "status", "user_ms", "system_ms", "scope", "source" }`. `status`는 `measured`, `unavailable`(실행된 프로세스가 없거나 운영체제가 주지 않음), `failed`(읽기 실패)다. `user_ms`·`system_ms`는 `measured`일 때만 있다 |

`cpu.scope`와 `cpu.source`는 고정 코드다. Unix(macOS·Linux)는 `process-and-waited-descendants`·`wait4-rusage`다. `wait4`가 돌려준 값으로, 실행한 프로세스와 그 프로세스가 기다린 하위 프로세스만 센다. 시간 제한으로 프로세스 그룹째 종료된, 기다리지 않은 하위 프로세스는 들어가지 않는다. Windows는 `job-process-tree`·`job-object-accounting`이다. 실행 전에 배정한 Job Object에서 돈 모든 프로세스를, 모두 끝난 뒤 센다. 두 범위가 달라 운영체제 사이의 값을 같은 뜻으로 비교하지 않는다.

`duration_ms`의 뜻은 그대로다. 명령 준비·프로세스 시작 전에 재기 시작해 종료 대기·정리 뒤에 끝난다. 그래서 `duration_ms`를 `limit_ms`로 나눈 값은 정확한 여유가 아니고, 근접도는 `limit_elapsed_ms`로 본다(§1.2). 실행 전 거부와 준비 실패는 실행이 아니므로 줄도 측정값도 만들지 않는다. 측정값에는 절대 경로, 환경 변수 값, 명령 출력이 들어가지 않는다.

`timing`의 값은 판정하지 않으므로, 타입이나 값이 손상된 `timing`은 기록의 무결성 오류가 아니다. 읽는 쪽은 그 값만 이용 불가로 다룬다. JSON 자체의 손상과 `seq`·`prev` 연결 끊김은 지금처럼 무결성 오류다. 알 수 없는 필드를 무시하고 원문 바이트로 해시를 잇는 이전 `ha`(v0.1.4 이하)는 이 필드가 든 기록을 자기 지원 규칙 안에서 읽고 이어 쓴다.

`/3` change 줄에는 `evidence`가 더해진다. [check-result.md](check-result.md)의 `check-evidence/v1` 간결한 근거이며 선언·생산자 입력, 호출, 보고 digest와 대상별 관측을 연결한다. `unknown`은 판정 불가이고 통과가 아니다.

`/4` 줄에는 실행 시작 시각 `started_at`이 더해진다(§4.7). baseline 줄도 같다.

### 2.3 `baseline`

`check`의 필드에 다음이 더해진다. `commit`은 덧씌울 검사 파일을 가져온 HEAD다.

| 필드 | 의미 |
| --- | --- |
| `base_commit` | `start`의 base commit |
| `overlay` | `[{ "path", "digest" }]`. HEAD에서 가져와 덧씌운 검사 파일 |
| `result` | `/1`·`/2`: `fail_as_expected`(exit ≠ 0) \| `unexpected_pass`(exit 0) \| `error`. `/3`는 [구조화 결과 계약](check-result.md)을 적용한다. |

`/1`·`/2`는 실패의 원인(예: 아직 없는 모듈의 import 실패)을 구분하지 않는다. `/3` change는 선언한 대상의 실제 관측을 요구한다. import·수집·기동 오류, 미실행, 불명확한 일반 스크립트 실패는 기대 실패가 아니다. 실행 가능한 검사가 기능 부재를 직접 관측한 경우는 선언한 요구 위반일 수 있다. 사람이 출력 원문을 보려면 `ha log`를 쓴다.

앞으로 수행하는 baseline은 `/1`·`/2`·`/3` 모두 검사 대상 하나의 전체 복사 목록을 파일 적용 전에 확인한다. HEAD 원본은 커밋된 일반 파일(`100644` 또는 `100755`)이어야 하며, 기준 상태의 복사 대상이나 부모가 symlink이면 내부·외부·끊어진 링크 모두 거부한다. 선언·생산자와 추가 검사 경로에 같은 조건을 적용한다. 안전한 일반 파일 교체, 없는 부모·파일 생성, 실행 권한은 유지하며 복사와 무관한 symlink는 허용한다.

Windows의 junction 등 irregular 항목도 안전한 일반 파일·디렉토리로 취급하지 않는다. 준비 오류나 관측된 불안전 경로 변경으로 안전성을 확인할 수 없으면 명령을 시작하지 않는다. 실패 원인과 해당 저장소 상대 경로를 알리고, 원래 작업 트리와 임시 worktree 밖 파일의 내용·권한을 보존한다. 미실행 대상의 검사 기록·사용량은 추가하지 않으며 일괄 요청에서 이미 실행된 대상의 기록·사용량은 유지한다. 이는 신규 실행의 안전 조건으로, 과거 기록을 소급 재분류하지 않는다. 실제 프로세스 시작 시도 이후의 오류·실패는 기존 기록·사용량 규칙을 따른다. 임의의 비협조 외부 프로세스를 격리하거나 검사 명령 자체의 쓰기를 제한하는 기능은 아니다.

### 2.4 `note`

| 필드 | 의미 |
| --- | --- |
| `note` | `block` \| `unblock` \| `confirm` \| `reopen` \| `input`. `/4`·`/5`와 `/5`의 note는 아래 |
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

`/5` 목표는 `review` note(`review`: 검토 gate 요약, §4.8)도 쓴다. 인용은 비어 있다.

`/4`·`/5` 목표는 `test-change`(대상 `tests:<path>`, `bound.blob`, `test_change`: `criteria`, `how`, `moved_to`, `weakened`), `wait`(`reason`), `resume`, `recover`·`allow-recovery`(`recovery`: `criterion`, `input`, `change` 또는 `count`) note도 쓴다(§4.7).

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

`/4`의 사전 승인은 `preapproved_runs`를 가진 `budget_change`이며 `runs`는 `previous_runs`와 같다(§4.7).

## 3. 신선도

결과는 그 결과가 나온 코드 상태, SPEC, 검증 명령에 묶인다.

**전제: 지금 작업 트리가 깨끗하다.** 묶음 문서 밖의 작업 트리에 변경(추적 파일 수정, 추적되지 않은 파일)이 있으면 지금의 코드 상태는 어떤 commit과도 같지 않다. 이때는 어떤 기록도 신선하지 않고, 상태 이유는 `worktree_dirty`다.

여기서 **묶음 문서**는 자기 목표의 목표 문서와 실행 묶음(`amendments/**` 포함), 그리고 같은 부모 아래 다른 목표의 `amendments/**`다. `run-rules/2`와 `run-rules/3`에서는 다른 목표의 실행 묶음 파일도 들어간다(§4.6). 이 계약에서 "묶음 문서 밖"은 모두 이 목록 밖을 뜻한다.

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

`/3` change에는 선언·결과 계약·대상·생산자 버전과 내용의 연결도 필요하다([check-result.md](check-result.md)). 명령 문자열이 같아도 이 의미가 바뀌면 이전 검사와 완료 근거를 재사용하지 않는다. 러너 어댑터 선언(`check-adapter/v1`)이면 생산자 대신 선언 바이트와 그 어댑터의 판정 의미 버전이 연결된다. Core가 같은 러너 출력을 다르게 판정하게 되어 어댑터의 판정 의미 버전이 오르면, 그 어댑터로 얻은 이전 baseline·검사·완료 근거는 신선하지 않다. 선언의 위반이나 연결한 시험 이름이 바뀌어도 같다. 출력 위치의 경로는 기록되지 않으므로 신선도에 쓰이지 않는다([check-result.md](check-result.md#5-러너-어댑터) §5). 상태 재구성은 저장된 간결한 근거로 하므로 원문 로그가 없어지거나 현재 시각이 달라졌다는 이유만으로 판정이 바뀌지 않는다.

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
| `review_missing` | `/5`만: 등급이 검토를 요구하는데 기록된 gate가 없다(§4.8) | 실행자가 고침 |
| `review_blocked` | `/5`만: 기록된 마지막 gate가 `blocked`(§4.8) | 실행자가 고침 |
| `review_degraded` | `/5`만: 기록된 마지막 gate가 `degraded`(§4.8) | 실행자가 고침 |
| `review_open_defect` | `/5`만: 그 gate에 열린 `defect`가 있다(§4.8) | 실행자가 고침 |
| `review_open_question` | `/5`만: 그 gate에 열린 `requirement-question`이 있다(§4.8) | 사용자 확인 |
| `review_code_missing` | `/5`만: 그 gate에 code 라운드가 없다(§4.8) | 실행자가 고침 |
| `review_head_stale` | `/5`만: 그 gate가 검토한 head가 HEAD가 아니다(§4.8) | 실행자가 고침 |
| `review_docs_stale` | `/5`만: 그 gate의 라운드가 지금의 목표 문서를 받지 않았다(§4.8) | 실행자가 고침 |
| `review_plan_missing` | `/5`만: `strict`인데 그 gate에 계획 라운드가 없다(§4.8) | 실행자가 고침 |

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

상태가 `complete`가 아니면 사람용 출력은 조건 표 앞에 **미충족 요약**(`### Unmet`)을 둔다.
- 미충족 required 조건마다 ID, 이유 코드, 분류(§4.1의 `executor`·`user` 등)를 보인다.
- 그 이유가 기대는 신선한 기록이 있으면 그 seq를 보인다. 기록은 다음과 같이 고른다.
  - 기록이 무효면 문제가 된 줄을 고른다.
  - baseline 이유면 기대와 다른 결과를 낸 가장 최근 baseline을 고른다.
  - `error_limit`이면 check와 baseline 가운데 가장 최근 오류·판정 불가 기록을 고른다.
  - 그 밖에는 통과하지 않은 가장 최근 check를 고른다.
  - 그런 기록이 없으면 그 종류의 가장 최근 신선한 기록을 고른다.
- 그 기록에 저장된 원문이 있으면, 정확한 명령 `ha log <goal> <seq>`를 함께 보인다.
  - `<goal>`은 명령에 준 goal 인수 그대로다. 같은 디렉토리에서 그 명령을 실행하면 된다.
  - 인수가 절대 경로면 저장소 기준 goal 경로를 쓴다. 이 명령은 저장소 루트에서 실행한다. 사람용 출력에는 절대 경로가 들어가지 않는다.
  - `ha done` 안내와 로컬 출력 위치의 `ha log` 안내도 같은 goal 표기를 쓴다.
  - goal 표기에 영문자·숫자·`-_./@%+=:,` 밖의 문자가 있으면 작은따옴표로 감싸, 복사한 명령에서 한 인수로 넘어가게 한다.
- 원문이 없는 기록은 `no stored output`이라고 보이고 명령을 만들지 않는다. 원문이 없는 경우는 출력 저장 실패와 출력 경로가 없는 기록이다.
- 신선한 기록이 없는 조건은 `no fresh record`라고 보이고 seq를 만들지 않는다. 실행 전에 거절된 시도는 기록을 남기지 않으므로 이 경우다. `manual` 조건은 기록 대신 확인을 기다리므로 이 표시를 붙이지 않는다.
- 목표 수준의 이유는 코드와 분류를 보인다.
- 표와 `### Reasons` 목록은 그대로 둔다.
- `ha done`의 사람용 출력은 이 요약을 두지 않고, 완료 기록 줄에도 아래 `done` 정보를 붙이지 않는다. `ha done` 출력은 이전 형식 그대로다.

완료 기록 줄은 유효한 완료 기록이 없을 때 다음과 같이 보인다.
- 가장 최근 `done` 줄의 상태가 `complete`가 아니면, 그 seq와 commit, 그리고 그 줄이 완료가 아닌 상태로 기록됐다는 사실을 보인다. 그런 줄은 완료였던 적이 없다.
- 가장 최근 완료 기록(`status: complete`인 `done` 줄)이 있으면, 그 seq와 commit, 그리고 무효가 된 이유를 모두 보인다. 이유는 §5.1의 유효 조건 가운데 깨진 것이고, 이 순서로 보인다.
  1. 기록 연결 끊김
  2. 무효한 저장 기록
  3. 묶음 문서 밖의 미커밋 변경
  4. 그 commit 뒤 코드 변경(Git이 비교할 수 없는 commit 포함)
  5. 목표 문서 변경
  6. required 명령 변경(조건 ID)
  7. 뒤따른 `reopen`(seq)
  8. `/3`에서 required 조건이 모두 충족되지 않음
- `done` 줄이 하나도 없으면 지금처럼 `none`이다. 완료 조건만 충족한 목표에는 지금처럼 `ha done` 안내를 붙인다.

합성 예시(완료 뒤 코드가 바뀌고, `ha done`을 다시 실행해 미완료가 기록된 목표):

```text
- completion record: none; latest done seq 9 (commit 2b7e01c4a9d3) recorded incomplete, not a completion; latest completion seq 6 (commit 8f3a2d91c0b7) is no longer valid: code outside the bundle documents changed since that commit; required conditions are not all satisfied

### Unmet

- AC-2 `criterion_failed` (executor): seq 8, output `ha log docs/specs/example 8`
- AC-3 `criterion_missing` (executor): no fresh record
```

`ha check`는 기대대로 끝나지 않은 대상마다 대상별 결과 줄 뒤에 `output for <대상>: ha log <goal> <seq>`를 출력한다. 뒤의 대상이 실행 전에 거절되거나 기록되지 못해 묶음이 일찍 끝나도, 그 전에 기록된 대상의 줄은 출력한다. `<goal>` 표기는 위와 같다. 원문이 저장되지 않은 대상에는 `output for <대상>: no stored output (seq N)`을 출력한다. 대상별 결과 줄과 종료 코드는 바뀌지 않는다.

이 출력은 판정·`status/v1`·완료 기록·`ha log`를 바꾸지 않는다. 같은 기록·묶음·Git이면 같다.

`--format md`(기본)는 사람이 읽고 `PROGRESS.md`에 붙이는 형식이다. 표지와 이유 코드는 계약 그대로 쓰고, 설명 문장은 영어로 쓴다.

### 4.5 `--format json`

외부 도구가 읽을 수 있는 구조화된 형식이다. 필드 순서는 아래 표의 순서로 고정되고, 들여쓰기는 공백 두 칸이다. 현재 시각을 새로 읽지 않는다. 발견·예산 이력은 저장된 시각만 포함하며 같은 입력이면 바이트 단위로 같다. 시험이 아래 표와 실제 출력의 필드를 비교한다.

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
| `findings` | [완료 뒤 발견](findings.md)의 `findings/v1` 조회. 당시 완료·현재 미해결·재작업 관계이며 기존 상태·종료 결과를 대신하지 않는다 |
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
| `runs_limit` | `/1`·`/2`는 최초 상한, `/3`는 최신 유효 횟수 상한, `/4`는 현재 창의 횟수 상한. `/4`의 재개 창은 §4.7이 정한 새 창의 상한에서 시작한다 |
| `revision` | 현재 창의 최신 start/reopen/budget_change seq |
| `remaining_runs` | 남은 횟수(최소 0) |
| `excess_runs` | 초과 횟수(최소 0) |
| `reached` | 사용량이 횟수 상한과 같은가 |
| `history` | `{ "seq", "at", "change": <budget_change 객체> }` 목록. 이전 변경도 보존 |
| `elapsed_seconds` | 창 안의 경과 시간 |
| `elapsed_limit_seconds` | `start`의 `budget.elapsed_seconds` |
| `exceeded` | 예산을 넘었는가 |

### 4.6 규칙 버전

`start`의 `rules`가 계산 규칙을 정한다. 새 `ha start`는 `run-rules/3`를 기록한다. `ha start --rules run-rules/4`는 `run-rules/4`를(§4.7), `ha start --rules run-rules/5`는 `run-rules/5`를(§4.8) 기록한다. 기본 규칙은 바뀌지 않으며, 새 Seal이 이 옵션으로 새 규칙을 고른다. `/1`·`/2`로 시작한 목표는 새 바이너리에서도 해당 규칙으로 계산하고 이후 실행에도 새 결과·예산 의미를 섞지 않는다. 자동 이전이나 과거 기록 재작성은 하지 않는다. 새 change 검사는 구조화 결과 계약에 연동되어야 유효 근거를 얻는다.

`ha`가 지원하지 않는 규칙 버전에는 계산도 기록도 하지 않는다. `ha status`, `ha done`, `ha check`, `ha note`, `ha budget`이 66으로 끝난다. 시작 규칙을 읽을 수 있으면 그 뒤 무결성 오류보다 미지원 규칙 거부가 우선한다. 지원 규칙에서도 미지 결과·부정합 데이터는 성공 근거가 아니다. 유효한 구기록의 의미만 호환 대상으로 보존한다.

| | `run-rules/1` | `run-rules/2` | `run-rules/3` | `run-rules/4` | `run-rules/5` |
| --- | --- | --- | --- | --- | --- |
| 묶음 문서(코드 상태에서 빼는 경로) | 자기 목표의 목표 문서와 실행 묶음, 다른 목표의 `amendments/**` | 다른 목표의 실행 묶음 파일도 포함. PLAN의 `다른 목표 실행 묶음` 선언은 입력으로 유지 | `/2`와 같음 | `/2`와 같음 | `/2`와 같음 |
| 남은 이유가 `budget_exceeded`뿐일 때 | `budget_exhausted` | `complete`. `ha done`이 완료 기록을 남김 | `/2`와 같음 | `/2`와 같음 | `/2`와 같음 |
| change의 실패·통과 근거 | 프로세스 종료 결과 | `/1`과 같음 | 선언한 대상·요구 위반·오류·호출 연결을 구조화 결과로 확인 | `/3`과 같음 | `/3`과 같음 |
| 현재 창의 횟수 상한 변경 | 미지원 | 미지원 | `budget_change`, 사용량 유지 | `/3`에 사전 승인 상한 안의 이유만 있는 증액을 더함 | `/4`와 같음 |
| 재개 창의 횟수 상한 | 최초 start 상한 | 최초 start 상한 | 최신 유효 횟수 상한 계승 | 시작 상한이나 사용자 인용으로 정한 마지막 상한. 사전 승인은 계승하지 않음 | `/4`와 같음 |
| 기존 시험 변경 | 사용자 확인 | 사용자 확인 | 사용자 확인 | 기록된 권한 안에서 근거가 있으면 확인 없이 충족 | `/4`와 같음 |
| 경과 시간 | 창 전체 | 창 전체 | 창 전체 | 실행자가 신고한 대기를 뺌 | `/4`와 같음 |
| 오류 한도 | 같은 코드에서 E=1 | `/1`과 같음 | 판정 불가도 함께 셈 | `/3`에 회복 note를 더함 | `/4`와 같음 |
| 명령별 시간 제한 | 900초 | 900초 | 900초 | PLAN `## 시간 제한`, 없으면 900초 | `/4`와 같음 |
| 검토 gate | 판정에 쓰지 않음 | 판정에 쓰지 않음 | 판정에 쓰지 않음 | 판정에 쓰지 않음 | `standard`·`strict` 목표는 기록된 마지막 gate가 지금 HEAD와 목표 문서를 덮어야 완료(§4.8) |

**다른 목표의 실행 묶음 파일.** 자기 목표 디렉토리와 같은 부모 아래에서, 지금 작업 트리에 `SPEC.md`가 있는 다른 디렉토리 바로 아래의 `PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `runs.jsonl`, `tasks/`다. 그 디렉토리의 `SPEC.md`와 그것이 링크한 문서, `checks/` 같은 다른 파일은 코드다. 같은 부모 아래가 아니거나 `SPEC.md`가 없는 디렉토리(예: `examples/`, `testdata/` 아래의 목표 모양 디렉토리)도 코드다. 그 다른 목표 디렉토리의 `amendments/`는 실행 묶음 파일이 아니지만, 모든 규칙 버전에서 PLAN의 `다른 목표 실행 묶음` 선언과 관계없이 코드 상태에서 빠진다(§2). 이 목록은 묶음 문서를 쓰는 모든 판단에 같이 쓴다.
- 작업 트리가 깨끗한지: §3의 전제, `check`·`baseline`이 남기는 `tree_clean`, 경로 확인 `confirm`
- 신선도: §3의 3
- base 이후 바뀐 파일: §4.2의 `out_of_scope`, `test_definition_changed`
- 완료 기록의 유효성: §5.1

같은 브랜치에서 여러 목표를 함께 진행할 때, 다른 목표가 진행 기록만 커밋해도 이 목표의 결과가 낡지 않게 하려는 것이다. 다른 목표의 실행 묶음을 검사 입력으로 읽는 목표는 PLAN `## 범위`에 `다른 목표 실행 묶음:` 항목을 두고 값으로 `검사 입력`을 쓴다([bundle.md](bundle.md) §2.2). 선언은 계산하고 기록할 때의 PLAN에서 읽는다.

**예산 초과만 남은 완료.** 필수 조건이 모두 충족되고 남은 이유가 `budget_exceeded`뿐이면 상태는 `complete`이고, `ha done`이 완료 기록을 남긴다. 다른 이유가 하나라도 함께 남으면 §4.3의 순서 그대로다. 예를 들어 사용자 확인 이유와 함께면 `budget_exhausted`이고 두 이유가 모두 나온다. 예산 사용량과 초과 여부는 `budget`에 계속 나온다. 이것은 이미 얻은 근거로 완료를 기록하는 것이고, 예산을 넘겨 더 검증하거나 작업할 권한이 아니다. 추가 사용에는 사용자 근거가 필요하다. `/3`의 같은 창 횟수 증액은 `budget_change`, 명시적 재작업·새 창은 `reopen`이며 `/1`·`/2`는 기존 `reopen`을 쓴다(§5.2). `ha check`는 실행 전에 예산을 확인하지 않는다.


### 4.7 run-rules/4

`ha start --rules run-rules/4`로 시작한 목표는 `run-rules/3`의 규칙을 모두 지키고 아래를 더한다. `run-rules/5` 목표도 이 절을 모두 따르고 §4.8을 더한다. 실행자의 근거·대기 신고·회복 사유는 주장이며 Core는 그 사실을 확인하지 않는다. Core는 기록과 연결만 본다. 새 note와 `--preapprove`는 `/1`·`/2`·`/3` 목표에서 65로 거부되고 기록되지 않으며, 그 목표의 상태 JSON에는 아래 필드가 나오지 않는다. 새 note 옵션(`--reason`, `--criteria`, `--how`, `--moved-to`, `--weakened`, `--input`, `--change`, `--count`)은 그 옵션을 쓰는 note 종류에만 받고, 다른 종류에 주면 규칙과 무관하게 64로 거부된다.

**시험 변경 권한.** `--test-authority <해석>`이 시작 요청을 시험 변경 권한으로 읽은 실행자의 해석을 `start.test_authority.interpretation`에 남긴다. 없으면 권한 없음이다. `/4` 시작 줄은 권한과 무관하게 PLAN `## 범위`의 `바꿀 수 있는 경로`와 `테스트 경로`(기본값 포함) 사본을 `start.scope.allowed`·`start.scope.tests`에 남긴다. base에 있던 파일 중 `scope.tests`에 맞는 것이 수정·삭제되면 시험 변경이다. 시작 뒤 PLAN을 고쳐도 이 분류는 넓어지거나 좁아지지 않는다. 다음을 모두 만족하면 사용자 확인 없이 충족된다.
- 권한이 있고, 경로가 `scope.allowed` 안이다.
- 그 경로의 마지막 `ha note <goal> test-change tests:<path> --criteria <AC-n,…> --how <확인 방법> [--moved-to <path> …] [--weakened]` note가 조건 ID와 확인 방법을 밝히고 `--weakened`가 없다. note는 깨끗한 트리에서만 기록되고 그때 HEAD의 파일 blob(`bound.blob`)과 옮긴 경로마다의 blob(`test_change.moved_to`)을 묶는다.
- HEAD의 그 경로가 note가 묶은 것과 같다. 수정이면 blob이 같고, 삭제면 note도 삭제된 경로에 묶였다(`bound.blob`이 `deleted`). 수정한 버전에 남긴 근거는 뒤의 삭제를 면제하지 않는다.
- 삭제면 note가 옮긴 새 경로를 하나 이상 밝힌다. 수정이든 삭제든 옮긴 새 경로마다 `scope.allowed` 안이며 HEAD의 blob이 note의 것과 같다. 연결된 파일 하나라도 다시 바뀌면 면제되지 않는다.

상태 JSON의 `test_changes`가 시험 변경마다 `path`, `change`(`modified`·`deleted`), `blob`, `exempt`, `confirmed`, 면제되지 않은 이유 `reason`, 실행자 주장 `rationale`(`seq`, `claim: "executor"`, `criteria`, `how`, `moved_to`의 `path`·`blob`, `weakened`)을 보이고, 사람이 읽는 형식도 경로마다 최종 blob(삭제면 `deleted`)과 옮긴 경로의 blob을 보이며, `test_authority`가 `request_quote`, `interpretation`, `allowed`를 보인다. 면제되지 않고 확인되지 않은 경로만 `test_definition_changed`에 남는다.

**신고한 대기.** `/4`의 `check`·`baseline` 줄은 실행 시작 시각 `started_at`(RFC 3339, 초 단위)을 남긴다. `ha note <goal> wait --reason <물은 결정>`이 대기를 시작하고 `ha note <goal> resume`이 끝낸다. 대기는 note 시각의 다음 초부터, 가장 이른 끝까지다: 인용을 가진 `input`·`confirm`·`allow-recovery` note나 인용을 가진 `budget_change`, `resume` note, note와 같은 초나 그 뒤에 시작한 실행의 `started_at`. 같은 초 안의 순서는 알 수 없으므로 그 실행이 대기를 끝낸 것으로 본다. 창(`reopen`)을 넘지 않는다. 겹친 대기는 한 번만 세고, 실행 구간 `[started_at, at]`은 대기와 겹쳐도 빼지 않는다. 발견 줄은 대기와 예산에 영향이 없다. 상태 `budget`의 `elapsed_seconds`가 예산 경과(창 길이에서 신고한 대기를 뺀 값)이고, `reported_wait_seconds`, `span_seconds`, `wait_basis: "executor_reported"`가 함께 나온다.

**사전 승인 상한.** `ha budget <goal> --preapprove <N> --window <W> --revision <R> --quote <인용> --context <문맥>`이 그 창의 사전 승인 상한을 `budget_change.preapproved_runs`로 기록한다. N은 지금 상한보다 커야 한다. 그 창에서 `--runs`를 N 이하로 올리는 데는 `--reason`만 있으면 된다. 상태 `budget.preapproved_runs`가 그 값을 보인다. 새 창의 상한은 시작 상한이나 사용자 인용으로 정한 마지막 상한(그 뒤의 감액은 반영)이고, 사전 승인과 그 안의 증액은 계승하지 않는다.

**오류 회복.** `ha note <goal> recover <AC-n> --input check|baseline --change <바뀐 실행 조건>`은 그 조건·입력의 신선한 오류와 판정 불가가 한도(E=1)를 넘었을 때만 기록된다. 그때 그 입력의 한도에 들어간 오류·판정 불가 줄의 seq를 `recovery.clears`에 남기고, 그 줄들만 그 입력의 한도 계산에서 빠진다. 계산은 note보다 앞선, 같은 조건·같은 입력의 오류·판정 불가 줄만 받아들이고 목록의 다른 seq는 무시한다. 입력(명령·제한·검사 경로 등)이 바뀌었다가 돌아와 다시 신선해진 다른 오류는 그대로 센다. 같은 입력의 `fail`과 baseline `unexpected_pass`, 다른 조건과 다른 입력은 그대로다. 실행자 회복은 조건·입력마다 2회다. `ha note <goal> allow-recovery <AC-n> --input check|baseline --count <N> --quote <인용>`이 N회를 더한다. 한도를 넘지 않은 입력의 회복과 남은 횟수가 없는 회복은 65로 거부된다. 상태 JSON의 `recoveries`가 회복이나 허용이 있거나 한도를 넘은 입력마다 `criterion`, `input`, `errors`(어느 회복도 지정하지 않은 신선한 오류·판정 불가 수)와 그 seq `error_seqs`, `limit_reached`, `used`, `allowed`, `remaining`을 보인다. 사용자 입력·예산 변경·`reopen`·뒤의 통과는 오류 한도를 풀지 않는다.

**명령별 시간 제한.** PLAN의 선택 절 `## 시간 제한`([bundle.md](bundle.md) §2.5)이 조건이나 task 명령의 시간 제한을 정한다. 최대 7200초이고, 넘거나 대상이 없는 선언은 lint 문제라 실행 전에 거부된다. baseline은 그 조건의 제한을 쓴다. 명령 digest에 적용 제한(밀리초)이 들어가므로 제한을 바꾸면 이전 결과는 신선하지 않다. 선언하지 않은 명령은 900초다.

**실행 전 안내.** `ha check`는 대상마다 실행 전에 같은 대상(조건, 또는 task와 명령 순번)의 이전 `check`·`baseline` 줄을 읽고, 표준 출력에 그 대상의 결과 줄보다 먼저 한 줄을 낸다.
- 가장 최근에 제한이 발동했거나 제한 구간 사용이 800‰ 이상인 실행: `time-limit notice: <대상> seq <n> limit reached` 또는 `time-limit notice: <대상> seq <n> <p>‰ of the limit`. 제한 구간이 없어 `duration_ms`로 계산했으면 끝에 ` (approximate)`를 붙인다.
- 이전 실행이 없거나 적용 제한을 기록한 이전 실행이 없으면: `time-limit notice: <대상> risk unknown`.

`<대상>`은 결과 줄과 같은 표기(`AC-3`, `T002#1`)다. 안내 줄은 이 세 형식 가운데 하나 그대로이고 다른 말을 덧붙이지 않는다. 그래서 이전 실행이 없을 때 안전하다는 말이 나오지 않는다. 안내는 실행을 막지 않고 기록·실행 횟수를 쓰지 않는다.

### 4.8 run-rules/5

`ha start --rules run-rules/5`로 시작한 목표는 `run-rules/4`의 규칙(§4.7)을 모두 지키고, SPEC의 `Assurance:` 등급([goal-docs.md](goal-docs.md) §2.5)이 독립 검토를 요구하면 기록된 검토 gate를 완료 조건으로 쓴다. 기본 시작 규칙은 그대로 `run-rules/3`이고, `/5`는 이 옵션으로만 고른다. `run-rules/1`~`run-rules/4`로 시작한 목표는 진행 중이든 완료했든 판정, 상태 출력, 완료 기록이 그대로이고, `review` note를 65로 거부하며 기록하지 않는다. `/5` 목표의 상태 JSON은 `/4`와 같은 필드를 갖고, 아래 이유만 더해진다.

**gate 기록.** `ha note <goal> review --gate <file>`은 Spar의 `spar gate`가 쓴 `review-gate/v1` 파일([review-gate.md](review-gate.md))을 한 번 읽어 note 줄의 `review`에 다음만 남긴다. 판정은 이 요약만 읽고 파일을 다시 읽지 않는다. 기록은 실행자의 주장이고, 실행자가 나중 gate를 기록하지 않는 것은 Core가 막지 못한다. 보증 수준은 `local`이다.

| 필드 | 의미 |
| --- | --- |
| `gate_digest` | 파일 바이트의 sha256 |
| `schema`, `spar_version` | 파일의 값 |
| `state` | `complete` \| `degraded` \| `blocked` |
| `expect_head`, `last_reviewed_head` | 기대 head와 마지막으로 검토된 head |
| `rounds[]` | 파일의 순서대로 `stage`, `sequence`, `base`, `head`, `criteria_digest`, `criteria`(`name`, `sha256`), `required_reviewers`, `completed_required`, `dry_run` |
| `open` | 열린 `defect`, `requirement-question` 수 |

gate의 `reasons`, `open_findings`, 라운드 `path`, `waived_reviewers`(사용자 인용 포함), `growth`, `written_at`, `previous_gate_digest`는 남기지 않는다. 지적 원문은 Spar 기록이 정본이다. 기록할 때 HEAD, 작업 트리, 목표 문서와 대조하지 않고, 같은 파일을 다시 기록하면 같은 요약이 한 번 더 남는다.

파일이 없거나 일반 파일이 아니거나 읽을 수 없음, JSON 객체 하나가 아님(빈 파일, 뒤에 붙은 데이터 포함), `schema`가 `review-gate/v1`이 아님, 위 필드가 없거나 타입이 틀림(`state`가 세 값 밖, `stage`가 `plan`·`plan-delta`·`candidate`·`fixes` 밖, 정수가 아니거나 음수인 수, `criteria` 항목의 빈 `name`이나 64자리 소문자 hex가 아닌 `sha256`)이면 `ha: review gate <file>: <이유>; nothing recorded`를 보이고 65로 끝나며 아무것도 기록하지 않는다. 그 밖의 모르는 필드는 무시한다. `--quote`를 주면 규칙과 무관하게 64이고, `/5` 목표에서 `--gate`가 없거나 비었으면 64다.

**판정.** 판정할 때의 SPEC.md 등급이 `standard`나 `strict`이면, 기록 전체에서 가장 나중의 `review` note(기록된 마지막 gate)로 아래 이유를 계산한다. 등급이 `light`이거나 줄이 없으면 gate가 필요 없고, gate를 기록했어도 이 이유가 나오지 않는다. 등급은 시작 기록이 아니라 지금의 SPEC.md에서 읽는다. 등급을 바꾸면 목표 digest가 바뀌므로 `spec_changed`와 `confirm spec`도 지금처럼 적용된다.

| 이유 | 조건 | 분류 |
| --- | --- | --- |
| `review_missing` | `review` note가 없다. `confirm`, `input` 같은 다른 note는 gate를 대신하지 않는다 | 실행자가 고침 |
| `review_blocked` | gate 상태가 `blocked` | 실행자가 고침 |
| `review_degraded` | gate 상태가 `degraded` | 실행자가 고침 |
| `review_open_defect` | 열린 `defect`가 있다 | 실행자가 고침 |
| `review_open_question` | 열린 `requirement-question`이 있다 | 사용자 확인 |
| `review_code_missing` | dry run이 아닌 `candidate`·`fixes` 라운드가 없다 | 실행자가 고침 |
| `review_head_stale` | `last_reviewed_head`나 목록의 마지막 code 라운드(dry run 제외)의 `head`가 지금 HEAD와 다르다. commit ID를 그대로 비교하므로 묶음 문서만 바꾼 commit도 다른 head다 | 실행자가 고침 |
| `review_docs_stale` | 마지막 code 라운드가, `strict`면 마지막 `plan`·`plan-delta` 라운드도, 지금의 목표 문서를 모두 받지 않았다 | 실행자가 고침 |
| `review_plan_missing` | `strict`인데 dry run이 아닌 `plan`·`plan-delta` 라운드가 없다 | 실행자가 고침 |

라운드가 지금의 목표 문서를 받았다는 것은, 목표 문서 집합([goal-docs.md](goal-docs.md) §1)의 문서마다 그 라운드의 `criteria`에 `sha256`이 지금 내용의 sha256이고 이름이 다음 둘 중 하나인 항목이 있다는 뜻이다: 그 문서의 저장소 기준 경로(`/` 구분), 또는 심볼릭 링크를 푼 이름(푼 파일이 저장소 안이면 그 저장소 기준 경로, 밖이면 파일 이름). 뒤의 것은 Spar가 기준 문서를 부르는 방식이다. 다른 기준 문서가 더 있거나 순서가 달라도 된다. Spar의 `criteria_digest`와 SPEC digest는 정의가 달라 비교하지 않는다. 목표 문서는 모두 검토하는 저장소에서 `--criteria`로 준다. "마지막"은 gate 파일의 라운드 순서다.

fixes 사슬과 기대 head의 확인은 gate 상태 `complete`가 맡는다([review-gate.md](review-gate.md) §2). Core는 사슬을 다시 계산하지 않고, 지적의 옳고 그름도 판단하지 않는다.

이 이유가 하나라도 있으면 `ha done`은 완료를 기록하지 않는다. 완료 기록 뒤에 `review` note를 기록하면 `reopen`처럼 그 완료 기록이 무효가 되고 새 gate로 다시 판정한다(§5.1). 유효한 완료 기록이 있는 동안에는 이 이유를 상태 이유에서 뺀다. 그 완료 기록이 판정한 코드 상태와 목표 문서를 고정하므로, 완료 뒤 묶음 문서만 commit해도 완료가 유지된다. 저장된 `review` 요약이 위 형식에 맞지 않으면 `record_invalid`다.

검토 없이 끝내려면 사용자가 등급을 `light`로 낮추는 목표 문서 수정을 정한다. 그 뒤 `confirm spec`과 신선한 검사로 gate 없이 완료된다.

## 5. 완료 기록과 예산

### 5.1 완료 기록

`ha done`이 `complete`를 계산하면 그 `done` 줄이 완료 기록이다. 완료 기록은 "`ha`가 이 코드 상태, 이 SPEC, 이 기록 head에서 완료를 확인했다"는 사실을 고정한다.

완료 기록 d는 다음을 모두 만족하는 동안 **유효하다**.
- 지금 작업 트리가 깨끗하다.
- d.commit과 HEAD 사이에 묶음 문서 밖의 차이가 없다. 묶음 문서는 규칙 버전에 따른다(§4.6).
- SPEC digest와 required 조건의 명령 digest가 d와 같다. `/3` change의 구조화 선언·생산자 입력도 같은 의미여야 한다.
- d 이후에 `reopen` note가 없다. `/5`에서는 `review` note도 없다(§4.8).
- d와 그 앞 줄의 `prev` 연결이 온전하다. 끊겼으면 완료 기록도 근거가 되지 않는다(`record_integrity`).

유효한 완료 기록이 있으면 상태는 `complete`이고, 그 뒤에 시간이 얼마나 지났는지와 무관하다. 완료 기록이 무효가 되면(코드나 SPEC이 바뀜, 재작업 요청) 기록은 이력으로 남고, 상태는 §4.1–§4.3대로 다시 계산한다.

`/5`에서 완료를 기록하는 계산은 검토 이유(§4.8)도 확인한다.

`ha done`의 결과가 `complete`가 아니어도 `done` 줄은 남는다. 이것은 "완료를 주장했지만 완료가 아니었다"는 기록이다.

### 5.2 예산

예산은 진행 중인 작업에 적용한다. PLAN을 고치는 것만으로 진행 중 한도가 바뀌지는 않는다. 검사 실행 수는 CLI 호출 수가 아니라 `check`·`baseline` 줄 수다. 실패·오류·판정 불가·stale도 실제 시도라면 세며 baseline을 면제하지 않는다. 실행 전 거부와 worktree 준비 실패는 검사 실행과 구분한다. `/3`에서 선언한 overlay 파일을 준비할 수 없어 실행 전에 거부하면 검사 기록과 사용량이 생기지 않는다.

- **창**: 마지막 `start` 또는 `reopen`부터 마지막 저장 기록까지다. 검사 사용량은 기록 추가 순서로 귀속한다. 재개 전에 시작해 재개 뒤 기록된 검사도 새 창에 포함된다.
- **발견 기록**: `run-finding/v1` 줄은 실행 예산 입력에서 제외한다. 전체 체인의 순번·head는 그대로 유지한다. 발견의 등록·판단·정정만으로 사용량·창·한도·경과 시간이 바뀌지 않는다. 기존 `run/v1` 자료의 의미는 유지한다.
- **경과 시간**: 마지막 실행 예산 대상 기록 `at`에서 창 시작 `at`을 뺀 값이다. 현재 시각 조회로 늘지 않는다. 예산 변경을 포함한 새 기록이 붙으면 기다린 시간도 들어간다.
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
