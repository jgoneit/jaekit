# 세부 동작과 규칙

**한국어** · [English](RULES.en.md)

[사용 안내](USAGE.md)에 다 적지 않은 동작을 정확히 적은 참고 문서입니다. 필드와 판정의 정의는 [실행 기록 계약](../contracts/run-record.md)에 있습니다.

> 처음 쓰는 데는 이 문서가 필요 없습니다. 목표를 맡기고 보고를 읽는 법은 [사용 안내](USAGE.md)에 있습니다.

## 사람용 보고와 도구 점검

`ha report <goal>`은 목표의 상태를 다섯 줄로 요약합니다. 기본은 영어이고 `--lang ko`를 붙이면 한국어로 나옵니다. `ha status`와 같은 계산을 읽기만 하며 기록을 바꾸지 않습니다.

| 표지 | 읽는 것 |
| --- | --- |
| `바뀐 것` | 시작 이후 실행 묶음 문서 밖에서 바뀐 파일과 commit 수 |
| `확인한 것` | 충족한 필수 조건 수(검사 기록, 사용자 확인, 검토 gate)와 수정 전 실패를 확인한 change 조건 수 |
| `직접 볼 것` | 사용자가 정할 것, 인용으로 확인한 조건, 근거만으로 면제된 시험 변경, 바뀐 검증 명령, 보류하거나 처리 없이 남은 검토 지적처럼 사람이 봐야 할 항목. 검토 지적은 라운드, 지적 ID, 분류와 보류 대상을 보입니다 |
| `남은 한계` | 보증 수준 `local`, 검사 작성자, 충족되지 않은 선택 조건, 예산 초과, 지금 HEAD를 덮는 완료 기록이 없는지 |
| `다음 행동` | 지금 상태에서 할 일 하나 |

base commit이 없는 얕은 clone처럼 Git이 base와 HEAD를 비교할 수 없으면 다섯 줄만 보이고 70으로 끝납니다. 이때 `다음 행동`이 base를 받는 방법(예: `git fetch --unshallow`)을 알립니다.

요약 뒤에는 `PROGRESS.md`의 `## 현재`에 옮겨 적을 도장(기록 head의 seq·hash, 기록 시각, 상태), 조건별 근거, 완료 범위, 충족되지 않은 조건의 원문 위치(`ha log <goal> <seq>`)가 이어집니다. 합성 예시:

```text
바뀐 것: base 3c1f0a9e7b25 이후 실행 묶음 문서 밖에서 파일 2개, commit 2개
확인한 것: 필수 조건 2개 중 2개 충족(검사 기록 2개, 사용자 확인 0개); change 조건 1개 중 1개가 baseline을 요구하고 그중 1개에 신선한 baseline
직접 볼 것: 없음
남은 한계: 보증 수준 local; 검사 작성자: 실행자
다음 행동: 없음; 완료 기록 seq 5이 유효하고 남은 할 일이 없음
```

검사에 필요한 도구를 미리 확인하려면 PLAN에 `## 실행 환경` 절을 두고 한 줄에 도구 하나씩 `` - <이름>: `<점검 명령>` `` 형식으로 씁니다.

```markdown
## 실행 환경
- 브라우저: `node scripts/probe-browser.mjs`
- 셸: `sh tests/env/plain.sh`
```

`ha doctor <goal>`은 이 점검 명령을 차례로 60초 제한으로 실행하고 도구마다 `사용 가능` 또는 `사용 불가 (<이유>)`를 보입니다. 사용 불가면 그 출력의 끝부분이 아래에 나옵니다. 시작 전에도 쓸 수 있고 기록·실행 횟수에는 아무것도 남지 않습니다. 형식이 틀린 줄은 파일과 줄을 알리고 아무 점검도 실행하지 않습니다. 이 절은 판정에 쓰이지 않습니다.

## 검토 gate (run-rules/5)

v0.1.7의 Seal(0.1.21 이상)은 새 목표를 `ha start --rules run-rules/5`로 시작했고, v0.1.9의 Seal 0.1.36은 `run-rules/8`로 시작합니다. `run-rules/5`는 `run-rules/4`의 자율 처리를 모두 그대로 두고, SPEC의 등급이 `standard`나 `strict`인 목표에 독립 검토의 기록을 완료 조건으로 더합니다. 그래서 `run-rules/5` 이후로 시작한 `standard`·`strict` 목표는 기록된 Spar gate가 있어야 완료됩니다. `light` 목표, `Assurance:` 줄이 없는 목표, `run-rules/4` 이하로 시작한 목표에는 Spar가 필요 없습니다. 검토 없이 끝내려면 등급을 `light`로 낮추는 목표 문서 수정을 사용자가 정합니다(아래 목록).

설치된 `ha`가 `run-rules/5`나 Seal이 요구하는 다른 지원을 모르면, Seal은 기록 없이 멈추고 이전 규칙으로 물러나지 않습니다. 할 일은 그 지원을 갖춘 `ha`를 설치하는 것입니다.

- 누적 라운드가 열린 지적 없이 끝나면 Seal이 그 series의 라운드로 `spar gate`를 HEAD에 대해 만들고, `ha note <goal> review --gate <file>`로 gate 파일을 기록한 다음 `ha done`을 실행합니다. 기록에는 파일 digest와 판정에 필요한 요약만 남고 지적 원문은 남지 않습니다.
- gate가 없으면 `review_missing`, 검토 뒤 commit이 더 있으면 `review_head_stale`, 검토 뒤 목표 문서가 바뀌었거나 라운드가 목표 문서를 모두 받지 않았으면 `review_docs_stale`이 나오고 완료가 기록되지 않습니다. gate가 `blocked`·`degraded`이거나 열린 `defect`·`requirement-question`이 있을 때, code 라운드가 없을 때, `strict`인데 plan 라운드가 없을 때도 각자의 이유가 나옵니다. 사용자 확인이나 다른 note는 gate를 대신하지 않습니다.
- Spar를 쓸 수 없어 검토 없이 끝내려면, 사용자가 SPEC을 `Assurance: light`로 낮추는 수정을 정합니다. 그 뒤 `confirm spec`과 신선한 검사로 gate 없이 완료됩니다. `run-rules/8` 목표에 `review` 조건이 있으면 그 기준도 바꿔야 하며, 그렇지 않으면 `review_tier_missing`으로 완료되지 않습니다.
- `light`이거나 등급 줄이 없는 목표, 그리고 `run-rules/1`~`run-rules/4`로 시작한 목표는 지금처럼 gate 없이 판정합니다.

Seal 0.1.26은 위 시작 규칙을 대신해 새 목표를 `ha start --rules run-rules/6 … --check-skill`로 시작합니다. `run-rules/6`을 지원하는 Core가 필요하며, ha 0.1.8은 이를 지원하지 않습니다. 지원이 없으면 기록 없이 멈추고 이전 규칙으로 물러나지 않습니다. `/6`은 `/5`의 규칙 가운데 다음 네 판정만 바꿉니다.

- `standard`·`strict` 목표의 마지막 candidate 라운드(dry run 제외)는 시작 기록의 `base_commit`부터 검토해야 합니다. 다르면 `review_base_mismatch`가 gate seq와 두 base를 보이고 완료를 막습니다. 기록된 fixes 라운드만 있어도 같은 이유가 나옵니다. 시작 base부터의 candidate 뒤에 기록된 fixes 라운드는 그대로 유효하고, 그 사슬은 Spar가 확인합니다. `reopen`도 시작 base를 옮기지 않습니다.
- `light`이거나 등급 줄이 없는 목표는 완료 뒤 `review` note를 남겨도 다른 조건이 유효하면 완료 기록이 유지됩니다. `standard`·`strict`는 이전처럼 그 기록이 무효가 되어 새 gate로 다시 판정합니다.
- `record_head`가 앞선 줄의 hash이고 그 줄과 `done` 줄 사이에 발견 줄(`run-finding/v1`)만 있거나 아무 줄도 없으면, 그 `done` 줄은 판정한 줄입니다. `record_head`가 가리키는 줄 자체는 발견 줄이어도 됩니다. 모든 `done` 줄은 전체 기록에서 먼저 판정하므로, `record_head`가 판정하지 않은 줄을 가리켜도 판정한 줄이 될 수 있습니다. 그런 다음 판정하지 않은 `done` 줄은 완료 여부와 관계없이 상태 계산에서 없는 것으로 취급합니다. 완료 기록과 그 설명, 최신 done 줄, 예산 창, 경과 시간, 완료 뒤 구간, 예산 변경 거부가 모두 이에 따르며 상태·보고·비용·예산 변경은 같은 뷰를 씁니다. 앞의 기록도 다른 유효성 조건을 만족해야 하며, 판정했지만 다른 이유로 무효인 줄에서는 이전처럼 멈춥니다.
- 완료 `done`부터 다음 검사·baseline의 시작이나 완료가 아닌 `done`까지의 시간을 예산 경과에서 뺍니다. 둘 다 없으면 창의 마지막 예산 기록까지이고, 그 사이의 note와 발견 줄은 구간을 끝내지 않습니다. `post_completion_seconds`가 신고 대기·실행 구간과 겹치지 않는 추가 공제 초를 보이며, 상태·보고·비용은 같은 경과를 씁니다. 실행 횟수와 한도는 그대로이고, 완료 뒤 코드 재작업은 `reopen`으로 새 창을 엽니다.

`run-rules/1`~`run-rules/5`로 시작한 목표는 기존 판정을 유지하며 자동 이전하지 않습니다. 구간과 출력의 정확한 정의는 [실행 기록 계약](../contracts/run-record.md#49-run-rules6)에 있습니다.

## baseline 선택과 검사 동결 (run-rules/7)

현재 Seal은 새 목표를 `run-rules/8`로 시작합니다. `/8`은 아래 baseline 선택을 그대로 두고 검사 동결은 없습니다([실행 기록 계약](../contracts/run-record.md#411-run-rules8)). 지원하지 않는 Core에서는 기록 없이 멈추며 이전 규칙으로 물러나지 않습니다. 직접 실행하는 `ha start`의 기본 규칙은 `run-rules/3` 그대로이고, 이미 `/1`~`/7`로 시작한 목표의 판정·출력·기록은 바뀌지 않습니다.

PLAN 조건표의 선택 `baseline` 열에 `요구`를 적은 change 조건만 baseline을 요구합니다. 빈 칸·`—`나 열이 없으면 요구하지 않습니다. Seal은 두 가지에만 표시합니다. base commit에 이미 있는 결함을 고치는 버그 수정·회귀와, "X가 일어나지 않는다" 같은 금지 조건입니다. 새 기능을 더하는 조건은 기본으로 표시하지 않습니다. 표시한 조건마다 이유를 plan 라운드 전에 REVIEW에 적고, 표시하지 않은 조건에는 이유가 필요 없습니다. 이미 시작한 목표는 시작할 때의 표시를 그대로 둡니다. 표시하지 않은 change 조건도 검사 경로와 구조화 결과 계약은 필요하지만, base에도 있는 진입점에 묶을 필요 없이 새 함수의 시험에 연결할 수 있습니다.

필수 최소량은 `표시한 필수 change × 2 + 표시하지 않은 필수 change + 필수 maintain`입니다. manual과 review는 0회입니다. 표시한 change 1개, 표시하지 않은 change 4개, maintain 3개가 모두 필수라면 최소 9회이고, 자동 상한은 `max(50, ⌈9 × 1.5⌉) = 50`입니다. 시작 전에는 `ha estimate <goal> --rules run-rules/8`로 이 값을 봅니다. 시작한 목표는 저장된 규칙을 쓰며 다른 `--rules`를 주면 거절합니다.

대상 없는 `ha check <goal> --baseline`은 표시한 change 조건만 고릅니다(표시한 선택 조건도 포함). 표시가 하나도 없으면 기록 없이 거절합니다. 표시하지 않은 조건을 직접 지정해 baseline을 실행할 수도 있고, 실제 실행은 예산에 셉니다. 그 baseline의 신선한 불리한 결과도 판정에 남습니다. 표시한 조건의 baseline은 마지막 검토 라운드 뒤, 완료 기록 직전에 한 번 기록합니다. 그 baseline이 선언한 위반 대신 통과하거나 오류를 내면 Seal은 검사를 고치고, 고친 검사를 고친 코드처럼 검토받은 뒤 baseline을 다시 기록합니다. 검사 수정마다 기록하는 습관을 줄이자는 안내이며, 이미 낡은 근거를 신선하게 만드는 예외가 아닙니다.

`/7`과 `/8`에서 "baseline 전" 질문 모으기·`ha doctor`·`strict`의 plan 라운드는 baseline이 없는 목표에서도 현재 검사나 baseline 어느 쪽이든 첫 `ha check` 기록 전에 마칩니다. 시작 전에 알 수 있는 질문을 모으되, 물을 것이 없으면 바로 일합니다.

- 입력 실패인 대상 파일 없음, 손상된 입력, 시작하지 못하는 도구, 신호 종료는 통과가 아니다.
- baseline을 요구하지 않는 검사(maintain, `EX-n`, task 명령, 표시하지 않은 change 조건)는 입력 실패를 위반, 오류, 판정 불가 중 하나로 세어도 그것만으로 검사의 결함이 아니다. 위반으로 기록된 실패는 입력이 달라질 때까지 남는다.
- baseline을 요구하는 change 조건의 전용 생산자는 입력 실패를 위반으로 보고하지 않는다. 결과 계약이 허용하는 경우(예: `missing: violation`)는 예외이며 그 선택은 plan 라운드 전에, plan이 없으면 어떤 검토 라운드보다도 전에 `REVIEW.md`에 적는다.

baseline을 요구하는지는 규칙에 따라 읽습니다. `run-rules/6`까지는 모든 change 조건이 요구하고 `/7`과 `/8`은 표시한 조건만 요구합니다. 러너 어댑터는 입력 실패를 스스로 분류하며, [결과 계약](../contracts/check-result.md)의 분류와 입력 실패 표는 바뀌지 않습니다.

`run-rules/7`에서 검사 동결 시점은 기록된 마지막 gate의 dry run이 아닌 라운드 중 `strict`의 첫 `plan` 라운드 또는 `standard`의 첫 `candidate` 라운드입니다. 조건표에서 검사 경로를 적은 모든 행의 경로에 있는 선언·생산자·시험·도우미·설정이 대상입니다. Seal은 동결 라운드 바로 뒤 `ha note <goal> review --gate <file>`로 gate와 그때 조건표의 검사 경로를 기록합니다. 동결 라운드에서 끝난 gate note가 없으면 뺀 검사 경로를 관측할 수 없고, 상태와 보고에 `동결 때의 조건표가 기록되지 않음`이 나옵니다.

동결 뒤 검사 파일을 고치면 commit하고 `ha note <goal> check-change checks:<path> --reason "<지적 ID, 또는 조건과 이유>"`로 까닭을 남깁니다. 검토 지적을 고친 변경이면 그 지적 ID를 씁니다. 까닭은 지금 blob에 묶이므로 파일을 다시 바꾸면 그 변경의 까닭도 다시 남깁니다. 다음 candidate 라운드에 바뀐 검사와 까닭을 함께 주면 그 라운드가 바뀐 검사를 검토합니다. 동결 전 변경에는 이 까닭이 필요 없고, 동결 뒤에도 검사 결함을 고칠 수 있습니다. 동결 표시는 자체로 완료를 막지 않습니다.

`ha status`는 change 조건마다 `baseline_required`와 사람용 `baseline required` 줄을 보입니다. `ha report`의 `확인한 것`은 모든 change 조건 중 baseline을 요구한 수와 그중 신선한 baseline 수를 나눠 보입니다. `직접 볼 것`에서 시작 뒤 뺀 baseline 표시, 동결 뒤 바뀐 검사 파일과 까닭, 까닭 없는 변경을 읽습니다. 뺀 경로에는 비교에 쓴 `review` note seq가 붙습니다. `light`나 기록된 gate가 없는 목표에는 동결 표시가 없고, 검수 요약의 `### 보지 않은 것`에는 baseline 없이 충족한 change 조건을 적습니다. [실행 기록 계약 §4.10](../contracts/run-record.md#410-run-rules7)에 정확한 필드와 표시가 있습니다.

## 목표 하나의 비용 보기

`ha cost <goal>`은 목표 하나가 검증과 검토에 들인 비용을 한 화면에 모읍니다. 기록을 읽기만 하고 아무것도 쓰지 않으며, 현재 시각을 읽지 않고 모델을 부르지 않습니다. Spar 검토 기록은 `--series`로 그 목표의 series 폴더를 가리킬 때만 읽습니다. 위치를 짐작하지 않으므로 series가 여럿이면 `--series`를 여러 번 씁니다.

```bash
ha cost docs/specs/<goal> --series docs/reviews/<goal>
```

- **종류와 크기:** 등급, 규칙 버전, 필수·선택 조건 수, base 이후 바뀐 파일과 commit 수(`ha report`의 `Changed`와 같음), 유효한 완료 기록.
- **검증:** 실행 수와 시간, 조건별 실행 수, 다시 기록한 실행마다 앞 기록과 달라진 입력(코드, 목표 문서, 검증 명령, 덧씌운 검사 파일). 달라진 입력이 없으면 앞 기록의 결과를 보입니다. 예산 창의 값은 `ha status`와 같습니다.
- **질문과 대기:** 신고한 대기마다 물은 결정과 길이, Spar의 `requirement-question`과 그 답, 상한 연장, 리뷰어 waiver. 출처별로 세고 나열만 합니다. 범위·권한 결정인지 불필요한 재확인인지는 목록을 읽는 사람이 판단합니다.
- **검토 라운드와 지적:** 단계별 라운드 수(상한에 센 라운드와 재실행), 리뷰어 시간, 분류·위치별 지적과 처리, 기록된 fixes 라운드가 더한 줄(gate의 `growth`). `--series`가 없으면 주지 않았다고 보이고 0으로 보이지 않습니다.
- **토큰:** 어느 기록에도 없어 "기록 없음"으로 보입니다.

`--format json`은 같은 값을 `cost/v1` 문서로 보이고, `--lang ko`는 표지와 문장만 바꿉니다. 형식과 각 값의 출처는 [실행 기록 계약](../contracts/run-record.md) §1.7에 있습니다.

**견줄 때.** 요약은 숫자만 보이고 다른 목표와 비교하거나 나아졌다고 말하지 않습니다. 목표끼리 견줄 때는 같은 등급, 비슷한 크기(조건 수, 바뀐 파일과 commit 수)의 목표를 나란히 놓으세요. 과정을 바꾸기 전과 뒤의 목표 몇 개를 견준 차이에는 목표마다 다른 내용과 우연이 섞여 있어, 그 차이는 바꾼 과정의 인과 효과가 아닙니다.

## 자동 예산

PLAN의 `## 예산`에 횟수를 숫자 대신 `- 검증 실행: 자동`으로 쓰면 `ha start`가 상한을 정합니다. 상한은 `max(50, ⌈필수 최소량 × 1.5⌉)`입니다. `run-rules/6`까지를 쓰는 필수 change가 34개인 목표는 최소량 68회, 상한 102회입니다. `경과 시간`은 지금처럼 직접 쓰며, `run-rules/8`에서는 쓰지 않아도 되는 참고값입니다. 숫자를 쓴 PLAN은 지금과 같습니다.

산정한 상한이 미리 승인한 상한 이하이면 묻지 않고 시작합니다. 미리 승인한 상한은 사용자 설정 폴더의 `ha.json`에 한 번 적어 둡니다. 폴더는 `~/.config/jaekit/`이고, `XDG_CONFIG_HOME`이 설정되어 있으면 `$XDG_CONFIG_HOME/jaekit/`입니다. 파일이 없으면 상한은 50회입니다. 저장소 안의 파일은 이 상한을 정하지 못합니다. 이를 확인하려고 파일이 있으면 `git worktree list -z`로 작업 트리를 읽으므로 Git 2.36 이상이 필요합니다.

```bash
folder="${XDG_CONFIG_HOME:-$HOME/.config}/jaekit"
mkdir -p "$folder"
printf '{"preapproved_runs": 120}\n' > "$folder/ha.json"
```

홈은 환경 변수 `HOME`이 있으면 그 값이고, 없으면 Windows에서는 `USERPROFILE`입니다. Windows PowerShell에서는 같은 순서로 폴더를 골라 씁니다.

```powershell
$folder = if ($env:XDG_CONFIG_HOME) { "$env:XDG_CONFIG_HOME\jaekit" } elseif ($env:HOME) { "$env:HOME\.config\jaekit" } else { "$env:USERPROFILE\.config\jaekit" }
New-Item -ItemType Directory -Force $folder | Out-Null
Set-Content -Path "$folder\ha.json" -Value '{"preapproved_runs": 120}'
```

`ha estimate docs/specs/example`이 시작 전에 같은 산정 값과 승인이 필요한지를 보입니다. 산정 값이 상한을 넘으면 `ha start`는 기록을 남기지 않고 다음처럼 멈춥니다.

```text
ha: auto budget 102 runs (minimum 68 × 1.5, at least 50) exceeds the pre-approved limit 50 from default; the user's approval is needed: write the approved number in PLAN `검증 실행`, or have the user raise preapproved_runs in ~/.config/jaekit/ha.json
```

이때 사용자가 그 횟수를 승인하면 PLAN에 숫자로 적거나 `ha.json`의 `preapproved_runs`를 올립니다. 파일 형식이 틀렸거나 읽을 수 없으면 `ha: pre-approved run limit: <경로>: <이유>`로 멈추며 50회로 대신하지 않습니다. 이 상한은 시작할 때만 쓰입니다. 시작 뒤의 증액·재개 규칙은 그대로입니다. 정확한 규칙은 [실행 기록 계약](../contracts/run-record.md#15-automatic-budget)에 있습니다.

## 자율 처리 (run-rules/4)

Seal 0.1.16부터 Core가 `run-rules/4`와 그 다섯 기능을 지원하면 새 목표를 `ha start --rules run-rules/4`로 시작합니다. Core의 기본 규칙은 그대로 `run-rules/3`이고, 이미 시작한 목표는 시작할 때의 규칙으로 계속 계산합니다. 새 규칙에서 Seal은 이미 받은 권한 안의 일을 다시 묻지 않습니다.

- 요청이 범위 안의 시험 변경을 허용하면 `--test-authority`로 그 해석을 남깁니다. 기존 시험을 고치거나 옮길 때는 `ha note <goal> test-change`로 어느 조건을 어떻게 확인하는지 남기고, 그러면 사용자 확인이 필요 없습니다. 약화하는 변경과 범위 밖의 변경은 지금처럼 확인을 묻습니다.
- 질문하고 답을 기다리는 동안은 `ha note <goal> wait`로 신고하고, 답 없이 계속하면 `resume`을 남깁니다. 신고한 대기만 예산의 경과 시간에서 빠집니다.
- `run-rules/7`까지는 실행 상한을 미리 허락하면 `ha budget <goal> --preapprove`로 기록되고, 그 안에서는 Seal이 이유만으로 상한을 올립니다.
- 도구 설치처럼 실행 조건을 고쳐 오류 한도를 넘은 검사를 다시 볼 수 있으면 `recover`를 남깁니다. 조건·입력마다 두 번이며, 더 허락하려면 `allow-recovery`를 씁니다. 이전 실패는 그대로 남습니다.
- 오래 걸리는 검사는 PLAN의 `## 시간 제한` 절에 2시간까지 제한을 정합니다. 이전 실행이 제한에 걸렸거나 가까웠으면 실행 전에 안내가 나옵니다.

## 검증 보고 형식

Seal 0.1.10부터 도입된 보고 형식은 기존 검토·보고에서 다음 네 가지를 구분합니다.

- 기대 결과의 근거.
- 관측 경계.
- 시험 방법·대체 환경.
- 미확인 범위.

조건 그룹이 같은 설명을 참조할 수 있으며, 실제 수행과 예정은 다릅니다. 현재 배포판 v0.1.9의 Seal 0.1.36에도 포함되며 기존 목표 형식은 유지합니다.

[자세한 보고 경계](OPERATIONS.md#검증-근거-보고)를 참고하세요.

## 배포판과 규칙

현재 배포판 v0.1.9는 ha 0.1.9·spar 0.1.9·spec 0.1.16·seal 0.1.36·spar 플러그인 0.1.9 조합입니다. 기본 규칙은 `/3`이고, Seal은 0.1.16부터 새 목표를 `run-rules/4`로, 0.1.21부터 `run-rules/5`로, 0.1.26부터 `run-rules/6`으로 시작했고, v0.1.9의 Seal 0.1.36은 `run-rules/8`로 시작합니다. 실행 전 거부·횟수 추정, `/3` 결과 분류와 예산 정책이 적용됩니다. 기존 목표는 저장된 규칙을 유지합니다. [버전별 적용 범위](INSTALL.md#배포판과-개발-조합)를 확인합니다.

규칙(`run-rules/N`)은 `ha`가 목표의 기록을 계산하는 방식의 판입니다. 목표는 시작할 때의 규칙으로 끝까지 계산되므로, 새 `ha`가 나와도 진행 중인 목표의 의미는 바뀌지 않습니다.

Seal skill 본문은 약 4,600단어에서 2,200단어 이하로 줄었습니다. Seal 0.1.23은 시작 전 질문 규칙을 더하면서 상한을 2,300단어로 올렸습니다. 이전 Seal로 시작한 목표의 형식은 그런 목표를 이어 갈 때만 읽는 참고 문서(`references/earlier-goals.md`)로 옮겼고, 그 목표의 기록과 문서는 바꾸지 않습니다.
