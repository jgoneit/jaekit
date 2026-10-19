# 계약: review-round/v1

`spar run`은 한 라운드를 실행하고 라운드 폴더에 `review-round/v1` 기록을 남긴다. 리뷰어 결과와 지적 처리는 이 기록에만 쌓이고, 다른 도구(나중에는 `ha`)가 읽을 수 있도록 이 형식을 고정한다. 아래 예시의 커밋·경로·문장은 합성이다.

## 1. 명령

```bash
spar run --stage candidate --base 1111111 --head 2222222 \
  --criteria docs/specs/example/SPEC.md --rules AGENTS.md \
  --out docs/reviews/example/02-candidate
```

| 인자 | 뜻 |
| --- | --- |
| `--stage` | `plan`, `plan-delta`, `candidate`, `fixes` |
| `--base`, `--head` | 비교할 두 커밋. `--head` 기본값은 `HEAD` |
| `--out` | 새 라운드 폴더. 이미 있으면 거부한다(series를 잠근 뒤 다시 확인한다). 검토하는 저장소나 다른 Git 저장소 안이면 그 저장소가 무시하는 경로여야 하고 그 아래에 추적 파일이 없어야 한다. 경로 위에 `.git`이 있는데 Git이 그 경로를 판단하지 못하면(소유권 거부, Git 데이터가 없는 연결 worktree, 빈 `.git` 폴더 등) 거부한다 |
| `--criteria` | 기준 문서. 하나 이상 필수 |
| `--rules` | 규칙·계약 문서. 프로젝트 설정의 `rules`가 더해진다 |
| `--subject` | plan 단계에서 검토할 문서 |
| `--question` | 이번 라운드에 묻는 질문 |
| `--check-path` | 이번 실행의 검사 경로 glob |
| `--reviewers` | 이번 실행의 리뷰어 이름 목록 |
| `--repo`, `--label` | 저장소 위치, 기록에 남길 이름 |
| `--dry-run` | 패킷과 작업공간만 만들고 리뷰어를 띄우지 않는다. 상한에 세지 않고 gate는 받지 않는다 |

라운드 폴더의 부모 폴더가 한 검토의 series다. 같은 series의 다른 `review-round/v1` 라운드에서 상한, 이전 head와 이전 지적을 읽는다. 다른 형식의 기록 폴더는 읽지도 바꾸지도 않는다. series의 `.spar-lock`을 잠가 series마다 한 번에 한 라운드만, 상한 판단부터 라운드 기록을 마칠 때까지 실행한다. 다른 실행이 잠금을 쥐고 있으면 표준 오류에 알리고 그 라운드가 끝날 때까지 기다린다.

종료 코드: 0 라운드 기록을 썼고 완료했거나 대기 중인 리뷰어가 있음, 1 라운드 기록을 썼지만 완료하거나 대기 중인 리뷰어가 없음, 64 사용법 오류, 65 거부(설정 오류, 추적되는 기록 폴더, 상한 초과, 사슬 끊김, 읽을 수 없는 입력), 70 내부 오류. 거부는 리뷰어를 띄우기 전에 일어나고 라운드 폴더를 만들지 않는다.

## 2. 패킷과 작업공간

- `packet.md`는 모든 리뷰어가 받는 한 텍스트다. 단계 설명, 두 커밋 ID와 tree ID, 공격 각도와 보고 방법, 기준 문서·규칙 문서·검토 대상 문서의 전문, 질문, 같은 series의 앞선 라운드 지적, 두 커밋 사이의 diff가 들어간다. 커밋 메시지, Git 이력, 무시된 파일, 리뷰어 목록은 들어가지 않는다. `packet_sha256`은 이 파일의 sha256이다.
- 앞선 라운드 지적은 plan-delta에서는 plan·plan-delta 라운드, fixes에서는 candidate·fixes 라운드의 것이다. 단계·base·head가 같은 라운드는 넣지 않아서 같은 입력의 재실행은 같은 패킷을 받는다.
- 리뷰어마다 임시 작업공간 사본을 만든다. `head/`와 `base/`에는 두 커밋의 추적 파일이 blob 그대로 들어가고 `.git`은 없다. symlink는 만들지 않고 그 blob(대상 경로)을 담은 일반 파일로 둔다.
- `schema.json`은 리뷰어 결과의 JSON schema다.
- `claude`는 표준 입력, `codex`는 표준 입력, `agy`는 `-p` 인자로 패킷을 받는다. 패킷이 인자 하나의 한도(Linux 120 KiB, 그 밖 768 KiB)를 넘으면 `agy`는 같은 바이트의 `PACKET.md`를 작업공간 최상위에 받고 `-p`는 그 파일을 읽으라는 짧은 지시다. 시도 기록의 `packet_transport`(`stdin`, `argument`, `file`)가 이를 남기고, `PACKET.md`가 바뀌면 `workspace_modified`다. `command`는 `{prompt}` 자리에 `packet.md` 경로를 받는다. 패킷은 각 리뷰어의 제공자에게 전송된다.

## 3. 리뷰어 결과

리뷰어는 JSON 객체 하나를 낸다.

```json
{
  "reviewed_head": "2222222",
  "verdict": "blocking_findings",
  "angles_checked": [{"angle": "requirement-counterexample", "result": "checked the empty-name input"}],
  "angles_unchecked": [{"angle": "design", "reason": "out of time"}],
  "findings": [{
    "id": "F1", "proposed_class": "defect", "angle": "requirement-counterexample",
    "location": "head/app/greet.py:3", "claim": "An empty name gives a trailing comma.",
    "failure_scenario": "greet('') returns 'hello, '", "evidence": "head/app/greet.py:3",
    "link_kind": "criterion", "link": "AC-1", "introduced_by_change": "yes", "regression": "no",
    "check_impact": "none", "confidence": "high"
  }],
  "limits": "did not run the code"
}
```

- `reviewed_head`는 패킷 head의 앞 7자 이상이어야 한다. `verdict`는 `no_blocking_findings`, `blocking_findings`, `could_not_review`. 지적의 `angle`을 포함한 열거형 값은 schema의 값 중 하나여야 한다. 제공자가 schema를 강제하지 않는 `native`·`command` 리뷰어의 결과에도 같은 검사를 한다.
- 요청한 각도는 모두 `angles_checked`나 `angles_unchecked`에 있어야 한다. 둘 다 없거나 하나라도 빠지면 `invalid_output`이다.
- 지적의 열거값: `proposed_class` `defect|requirement-question|hardening|note`, `link_kind` `criterion|rule|contract|none`, `introduced_by_change`·`regression` `yes|no|unknown`, `check_impact` `false_pass|meaning_inverted|contract_violation|none`, `confidence` `high|medium|low`. `id`는 리뷰어 안에서 겹치지 않는다.
- `could_not_review`는 완료가 아니라 `failed`로 기록한다.

## 4. 리뷰어 상태

| 상태 | 뜻 |
| --- | --- |
| `completed` | 결과가 schema와 head, 각도 요구를 만족했다 |
| `pending` | `native`가 호스트 하위 에이전트의 답을 기다린다 |
| `unavailable` | CLI가 없거나, 사용량·요청 한도나 인증으로 거절했다 |
| `timed_out` | 시간 제한 안에 결과가 없었다 |
| `invalid_output` | 결과가 없거나 JSON이 아니거나 schema·head·각도 요구를 어겼다 |
| `failed` | 실행 파일을 실행할 수 없거나, 결과 없이 0이 아닌 종료, 또는 `could_not_review` |
| `tool_violation` | 허용 밖의 도구나 명령을 썼다. 결과는 쓰지 않는다 |
| `workspace_modified` | 작업공간 사본의 `head/`·`base/`가 바뀌었다. 결과는 쓰지 않는다 |
| `not_run` | `--dry-run` |

완료하지 않은 리뷰어의 `findings`는 `null`이고 요약에 상태와 원인이 나온다. 0건으로 바뀌지 않는다.

`verified` preset의 보호: `claude`는 `-p --safe-mode --restricted --strict-mcp-config --tools Read,Grep,Glob --permission-mode dontAsk --no-session-persistence --disable-slash-commands`로 실행하고 다른 도구나 MCP 서버가 보이면 멈춘다. `codex`는 `exec --ignore-user-config --ignore-rules -s read-only --ephemeral --skip-git-repo-check -c web_search="disabled"`와 `apps`, `browser_use`, `browser_use_external`, `computer_use`, `in_app_browser`, `image_generation`, `memories`, `multi_agent`, `plugins`, `hooks`의 `--disable`로 실행하고 허용 밖의 항목 종류가 보이면 멈춘다. `agy`는 `--sandbox --disable-slash-commands`로 실행하고 파일 읽기·목록·검색 도구와 셸 명령 중 읽기 전용 프로그램(`find`, `ls`, `grep`, `rg`, `cat`, `head`, `tail`, `wc`, `sort`, `uniq`, `cut`, `stat`, `file`, `diff`, `tree` 등)과 출력만 하는 `sed -n`만 허용한다. 리디렉션·명령 치환·`|` 밖의 셸 연산자, 그리고 `-exec`·`-execdir`·`-ok`·`-okdir`·`-delete`·`--pre`·`--compress-program`으로 시작하는 인자는 위반이다. 이 보호는 clean-review 0.2.0과 같다. 그 밖에 허용 프로그램이 파일을 쓰는 옵션(예: `sort -o`)은 따로 막지 않는다. 파일 쓰기는 `agy`의 `--sandbox`에 맡기고, `head/`·`base/`·`PACKET.md`가 바뀌면 `workspace_modified`다.

대체 모델: 지정 모델이 사용량·요청 한도로 거절되고 `fallback_model`이 있으면 같은 패킷과 새 작업공간 사본으로 한 번 더 실행한다. 인증 거절이나 다른 실패에는 쓰지 않는다. `attempts`에 시도마다 모델·상태·원인·재설정 문구가 남고 `fallback_used`가 `true`가 된다.

## 5. 라운드 폴더

| 파일 | 내용 |
| --- | --- |
| `round.json` | 아래 필드 |
| `packet.md`, `schema.json` | 패킷과 결과 schema |
| `findings.json` | 분류된 지적 배열 |
| `ledger.jsonl` | 처리 원장. 한 줄에 한 처리, 덧붙이기만 한다 |
| `summary.md` | 사람이 읽는 요약 |
| `<리뷰어>/` | 그 리뷰어의 표준 출력·오류와 결과. 대체 모델은 `<리뷰어>/fallback/` |
| `<native 리뷰어>/` | `prompt.md`, 작업공간, 기록된 결과. 기본 이름이면 `native/` |

`round.json`의 주요 필드: `schema`(`review-round/v1`), `spar_version`, `stage`, `label`, `series`, `sequence`, `repo`, `base`, `head`, `base_tree`, `head_tree`, `diff_sha256`, `criteria`·`rules`·`subjects`(`name`, `source`, `sha256`), `criteria_digest`(기준 문서 이름과 sha256의 sha256), `questions`, `prior`, `packet_sha256`, `config`([설정 계약](spar-config.md)의 적용 기록), `links`(연결 확인에 쓴 `AC-n`과 규칙 문서 줄 수), `requested_reviewers`, `required_reviewers`, `dry_run`, `counted`, `rerun_of`, `started_at`, `finished_at`, `reviewers`.

`reviewers.<이름>`: `name`, `preset`, `isolation`(`verified`, `host`, `unverified`), `required`, `status`, `reason`, `requested_model`, `observed_model`, `fallback_model`, `fallback_used`, `effort`, `elapsed_seconds`, `timeout_seconds`, `resets`, `attempts`, `findings`(완료한 경우의 지적 수, 아니면 `null`), `verdict`, `angles_checked`, `angles_unchecked`, `limits`, `warnings`, `agent`, `recorded_from`, `workspace_digest`(native 작업공간을 준비할 때의 digest).

## 6. 분류

`findings.json`의 항목은 리뷰어 결과의 필드에 `id`(`<리뷰어>-<리뷰어 ID>`), `reviewer`, `reviewer_id`, `class`, `class_reason`, `path`, `in_check_paths`, `claimed_link_kind`, `claimed_link`을 더한다. `link_kind`와 `link`는 확인된 연결만 남고, 확인되지 않으면 `none`과 빈 문자열이다.

연결 확인:

- `criterion`: 연결에 나오는 모든 `AC-n`이 기준 문서에 있어야 한다.
- `rule`: 라운드의 규칙 문서를 이름·경로로 대야 하고, 줄 번호를 대면 그 문서에 그 줄이 있어야 한다(1부터).
- `contract`: 규칙 문서이거나 head 커밋의 파일이어야 하고 줄 번호 조건은 같다.

분류 규칙(위에서 처음 맞는 것):

1. 위치가 검사 경로이고 `check_impact`가 `false_pass`나 `meaning_inverted`이면 `defect`
2. 위치가 검사 경로이고 `contract_violation`이며 연결이 확인되면 `defect`
3. 위치가 검사 경로이면 제안 분류와 관계없이 `hardening`
4. `proposed_class`가 `requirement-question`이면 `requirement-question`
5. `introduced_by_change`와 `regression`이 모두 `yes`이면 `defect`
6. `proposed_class`가 `defect`이고 연결이 확인되지 않으면 `note`
7. 그 밖에는 `proposed_class`

`hardening`은 라운드를 기록할 때 `{"disposition": "defer", "auto": true}` 원장 항목으로 자동 보류된다.

## 7. 처리

`spar triage <라운드> <지적> <처리> [옵션]`이 원장에 한 줄을 더한다. 규칙에 맞지 않으면 아무것도 쓰지 않고 65로 끝난다. 마지막 항목이 그 지적의 처리다.

| 처리 | 요구 |
| --- | --- |
| `fix` | `--commit`(라운드 head 뒤의 커밋), `--failed-before`, `--passed-after` |
| `reject` | `--evidence`(`requirement-question`은 대신 `--user-quote`) |
| `defer` | `--to`. `defect`는 `--user-quote`도 |
| `duplicate` | `--of` 같은 라운드의 다른 지적. `defect`와 `requirement-question`은 같은 분류의 지적만 가리키고, 가리킨 지적이 닫혀야 닫힌다(gate) |
| `answer` | `requirement-question`에만 |

`requirement-question`을 닫는 `fix`·`reject`·`defer`·`answer`와, 자동 보류된 `hardening`의 `fix`는 사용자의 말을 인용한 `--user-quote`가 필요하다. `note`는 처리하지 않아도 된다.

원장 항목: `finding`, `disposition`, `auto`, `reason`, `commit`, `failed_before`, `passed_after`, `evidence`, `of`, `to`, `user_quote`, `note`, `recorded_at`.

## 8. 상한·재실행·사슬

- 단계 묶음은 `plan`, `plan_delta`, `code`(candidate와 fixes)다. series에서 `counted`가 `true`인 같은 묶음의 라운드 수가 상한과 연장의 합 이상이면 새 실행을 거부한다.
- 같은 단계·base·head·패킷 digest의 앞선 실행이 있고, 이번 실행의 리뷰어가 모두 그 실행들에서 완료하지 못한 리뷰어이면 상한에 세지 않는다(`counted: false`, `rerun_of`). 라운드는 series마다 하나씩 실행되므로 `finished_at`이 없는 앞선 라운드는 도중에 멈춘 것이고, 그 리뷰어는 완료하지 못한 것이다. 그 밖의 실행은 모두 센다.
- fixes 라운드의 base는 같은 series에서 단계·base·head가 다른 마지막 code 라운드의 head여야 한다. 아니면 거부한다.
- 연장은 `spar extend <series> --stage plan|plan-delta|code --by N --user-quote "…"`로 series의 `extensions.jsonl`에 `{"stage", "by", "user_quote", "recorded_at"}`을 더한다. 사용자의 말이 없으면 거부한다. series 경로는 `--out`과 같은 기록 경로 규칙을 따르고(검토하는 저장소를 찾지 못해도 그 경로가 속한 저장소로 확인한다), `--repo`가 Git 저장소가 아니면 거부한다.

## 9. native 리뷰어

`native` preset 리뷰어는 `pending`으로 시작하고 자기 이름의 폴더에 `prompt.md`(지시와 패킷 전문)와 `workspace/`를 남긴다(기본 이름이면 `native/prompt.md`). 호스트의 clean-context 하위 에이전트가 답하면 `spar record <라운드> <이름> --result <파일|->`나 `--transcript <전사 기록>`으로 기록한다. dry run 라운드와 이미 완료한 리뷰어는 기록하지 않는다. 기록할 때 작업공간이 준비 때와 같은지, 결과가 schema·head·각도 요구를 만족하는지, 라운드의 `findings.json`·`ledger.jsonl`이 있는지 확인한다(없으면 거부). 전사 기록에서 `Read`·`Grep`·`Glob` 밖의 도구 사용, 호스트가 제공한 그 밖의 도구, 호스트가 더한 맥락(Git 상태, 지시 파일, hook 맥락)과 프롬프트를 읽지 않은 것은 `warnings`에 남는다. 맥락의 내용은 기록하지 않고 종류만 남긴다. 격리 등급은 `host`이고 필수 리뷰어에 든다.
