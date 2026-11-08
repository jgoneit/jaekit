# 계약: review-round/v1

`spar run`은 한 라운드를 실행하고 라운드 폴더에 `review-round/v1` 기록을 남긴다. 리뷰어 결과와 지적 처리는 이 기록에만 쌓이고, 다른 도구(나중에는 `ha`)가 읽을 수 있도록 이 형식을 고정한다. 각 입력 필드의 받는 형식과 형식 밖의 결과는 [입력 형식](spar-input.md)의 표(G1~G33)가 정하고, 이 문서는 그 행을 가리킨다. 아래 예시의 커밋·경로·문장은 합성이다.

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
| `--out` | 새 라운드 폴더. 이미 있으면 거부한다(series를 잠근 뒤 다시 확인한다). 기록 경로 규칙(G28): 경로를 품은 모든 Git 저장소(검토하는 저장소, 다른 저장소, 무시된 폴더 안의 중첩 저장소)가 무시하고 그 아래에 추적 파일이 없어야 하며, 경로 위에 `.git`이 있는데 Git이 그 경로를 판단하지 못하면(소유권 거부, Git 데이터가 없는 연결 worktree, 빈 `.git` 폴더 등) 거부한다 |
| `--criteria` | 기준 문서. 하나 이상 필수. 문서 인자의 이름은 G26 |
| `--rules` | 규칙·계약 문서. 프로젝트 설정의 `rules`(G23)가 더해진다 |
| `--subject` | plan 단계에서 검토할 문서 |
| `--question` | 이번 라운드에 묻는 질문 |
| `--check-path` | 이번 실행의 검사 경로 glob(G24). 잘못된 glob은 리뷰어를 띄우기 전에 거부한다 |
| `--reviewers` | 이번 실행의 리뷰어 이름 목록(G25) |
| `--model` | `이름=모델`. 사용자 설정에 `model`이 적히지 않은 `claude`·`codex`·`agy` 리뷰어의 이번 실행 모델(G25, [설정 계약](spar-config.md) 1절 "모델과 강도를 정하는 순서"). 여러 번 줄 수 있다 |
| `--effort` | `이름=강도`. 사용자 설정에 `effort`가 적히지 않은 `claude`·`codex` 리뷰어의 이번 실행 강도(G25). 여러 번 줄 수 있다 |
| `--repo`, `--label` | 저장소 위치, 기록에 남길 이름 |
| `--dry-run` | 패킷과 작업공간만 만들고 리뷰어를 띄우지 않는다. 작업공간은 라운드 폴더의 `workspace/`에 남는다. 상한에 세지 않고 gate는 받지 않는다 |

라운드 폴더의 부모 폴더가 한 검토의 series다. 같은 series의 다른 `review-round/v1` 라운드에서 상한, 이전 head와 이전 지적을 읽는다. 다른 형식의 기록 폴더는 읽지도 바꾸지도 않는다. series 폴더 자체를 잠가(잠금 파일을 만들지 않는다) series마다 한 번에 한 쓰기만 한다(G33). `spar run`은 상한 판단부터 라운드 기록을 마칠 때까지, `spar record`·`spar triage`·`spar extend`·`spar waive`는 기록을 읽고 쓰는 동안 같은 잠금을 쥔다. 다른 쓰기가 잠금을 쥐고 있으면 표준 오류에 알리고 그것이 끝날 때까지 기다린다. `spar gate`는 잠그지 않는다.

종료 코드: 0 라운드 기록을 썼고 완료했거나 대기 중인 리뷰어가 있음, 1 라운드 기록을 썼지만 완료하거나 대기 중인 리뷰어가 없음, 64 사용법 오류, 65 거부(설정 오류, 추적되는 기록 폴더, 상한 초과, 사슬 끊김, 읽을 수 없는 입력), 70 내부 오류. 거부는 리뷰어를 띄우기 전에 일어나고 라운드 폴더를 만들지 않는다.

## 2. 패킷과 작업공간

- `packet.md`는 모든 리뷰어가 받는 한 텍스트다. 단계 설명, 두 커밋 ID와 tree ID, 공격 각도와 보고 방법, 기준 문서·규칙 문서·검토 대상 문서의 전문, 질문, 같은 series의 앞선 라운드 지적, 두 커밋 사이의 diff가 들어간다. 커밋 메시지, Git 이력, 무시된 파일, 리뷰어 목록은 들어가지 않는다. `packet_sha256`은 이 파일의 sha256이다.
- 앞선 라운드 지적은 plan-delta에서는 plan·plan-delta 라운드, fixes에서는 candidate·fixes 라운드의 것이다. 패킷은 그 지적을 리뷰어 이름이 든 지적 ID(`codex-F1` 등)로 보인다. 단계·base·head가 같은 라운드는 넣지 않아서 같은 입력의 재실행은 같은 패킷을 받는다.
- 리뷰어마다 임시 작업공간 사본을 만든다. `head/`와 `base/`에는 두 커밋의 추적 파일이 blob 그대로 들어가고 `.git`은 없다. symlink는 만들지 않고 그 blob(대상 경로)을 담은 일반 파일로 둔다.
- `schema.json`은 리뷰어 결과의 JSON schema다.
- `claude`는 표준 입력, `codex`는 표준 입력, `agy`는 `-p` 인자로 패킷을 받는다. `agy`의 `-p` 값은 패킷 앞에 명령 안내를 붙인 것이다. 안내는 파일 읽기·목록·검색 도구를 먼저 쓰고, 셸 명령은 4절의 읽기 전용 프로그램과 `sed -n`을 `|`·`&&`·`||`·`;`로만 이으며, 코드·시험·빌드·Git이나 그 밖의 프로그램을 실행하면 검토가 멈추고 결과를 버린다(`tool_violation`)고 말한다. `packet.md`는 그대로이고, 다른 리뷰어는 안내를 받지 않는다. 안내와 패킷을 합친 것이 인자 하나의 한도(Linux 120 KiB, 그 밖 768 KiB)를 넘으면 `agy`는 패킷과 같은 바이트의 `PACKET.md`를 작업공간 최상위에 받고 `-p`는 안내와 그 파일을 읽으라는 짧은 지시다. 시도 기록의 `argv`는 안내 뒤의 패킷을 `<packet.md>`로 보인다. 시도 기록의 `packet_transport`(`stdin`, `argument`, `file`)가 이를 남기고, `PACKET.md`가 바뀌면 `workspace_modified`다. `command`는 `{prompt}` 자리에 `packet.md` 경로를 받는다. 패킷은 각 리뷰어의 제공자에게 전송된다.

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
    "check_impact": "none", "confidence": "high",
    "smallest_fix": "Drop the comma only when the name is empty."
  }],
  "limits": "did not run the code"
}
```

- 결과의 형식은 G1(객체와 최상위 키), G2(`reviewed_head`), G3(`verdict`), G4(각도 항목), G5(지적 항목), G6(`limits`)이다. 모든 preset의 결과에 같은 확인을 한다. 제공자가 schema를 강제하지 않는 `native`·`command` 리뷰어도 같다. 어기면 `invalid_output`이고 이유에 문제의 키 이름이 들어간다.
- 지적의 열거값: `proposed_class` `defect|requirement-question|hardening|note`, `link_kind` `criterion|rule|contract|none`, `introduced_by_change`·`regression` `yes|no|unknown`, `check_impact` `false_pass|meaning_inverted|contract_violation|none`, `confidence` `high|medium|low`. 패킷은 `id`를 보고 순서대로 `F1`, `F2`, …로 붙이라고 하지만 `id`는 schema대로 문자열이면 된다. 라운드의 지적 ID는 spar가 리뷰어 결과의 보고 순서대로 매기는 `<리뷰어>-F<n>`(1부터)이고, 리뷰어가 붙인 `id`는 `reviewer_id`로 남는다. 그래서 리뷰어 이름이나 `id`에 `-`가 있거나 `id`가 겹쳐도 라운드 ID는 겹치지 않는다.
- `smallest_fix`는 리뷰어가 지적마다 적는 가장 작은 수정 방향이다: 실패 시나리오가 맞게 나오게 하는 가장 작은 변경을 패치나 코드가 아닌 문장으로 적고, 말할 수 없으면 빈 문자열로 둔다. schema는 값을 엄격하게 강제하는 제공자를 위해 이 키를 `required`에 두지만, spar의 확인(G5)은 키가 없는 결과도 받는다. 빈 문자열과 키가 없는 경우는 모두 방향 없음이고, 문자열이 아닌 값은 `invalid_output`이다. 방향은 리뷰어의 제안이어서 분류·처리·gate에 쓰지 않는다(6–8절). spar는 수정이 방향을 지켰는지 판정하지 않는다.
- spar가 읽는 방식: `location`은 G7, `rule`·`contract` 연결은 G8, `criterion` 연결은 G9. 위치를 읽을 수 없는 지적도 결과에서 버리지 않는다.
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

`verified` preset의 보호(G10, G11): `claude`는 `-p --safe-mode --restricted --strict-mcp-config --tools Read,Grep,Glob --permission-mode dontAsk --no-session-persistence --disable-slash-commands`로 실행하고 다른 도구나 MCP 서버가 보이면 멈춘다. `codex`는 `exec --ignore-user-config --ignore-rules -s read-only --ephemeral --skip-git-repo-check -c web_search="disabled"`와 `apps`, `browser_use`, `browser_use_external`, `computer_use`, `in_app_browser`, `image_generation`, `memories`, `multi_agent`, `plugins`, `hooks`의 `--disable`로 실행하고 허용 밖의 항목 종류가 보이면 멈춘다. `agy`는 `--sandbox --disable-slash-commands`로 실행하고 파일 읽기·목록·검색 도구와, 셸 명령 중 읽기 전용 프로그램(`find`, `ls`, `grep`, `rg`, `cat`, `head`, `tail`, `wc`, `sort`, `uniq`, `cut`, `stat`, `file`, `diff`, `tree`, `true` 등)과 출력만 하는 `sed -n`을 연산자 `|`, `&&`, `||`, `;`로만 이은 줄을 허용한다. `2>/dev/null`과 `2>&1`은 지운 뒤 보고, 따옴표나 `\` 안의 연산자 글자는 연산자가 아니다. 그 밖의 리디렉션·명령 치환·연산자(`&`, `|&`, `;;`, 괄호 등), 앞이나 뒤나 사이에 명령이 없는 연산자, 목록 밖의 프로그램(`cd`, `git`, `go`, `python3`, `for` 등)은 위반이다. 경로로 부른 프로그램은 `/bin`, `/usr/bin`, `/usr/local/bin`, `/opt/homebrew/bin` 안의 것만 받는다(`head/bin/ls`, `./ls`는 위반). 작은따옴표 밖에서 확장을 시작하는 `$`(이름·숫자·`{`·`[`·특수 매개변수 앞의 `$`, 큰따옴표 밖의 `$'`·`$"`)와 따옴표 밖의 중괄호 확장(단어 `{}` 하나는 제외), 그리고 `-exec`·`-execdir`·`-ok`·`-okdir`·`-delete`·`--pre`·`--compress-program`·`--hostname-bin`으로 시작하는 인자는 위반이다. 닫는 따옴표 앞이나 낱말 끝의 `$`(`"foo$"`, `x$`)는 그대로 받는다. `sed`는 `-n`(또는 `--quiet`, `--silent`)과 함께, 모든 스크립트(`-e`, `--expression=`, 또는 `-e`가 없을 때 첫 피연산자)가 출력만 하는 형식일 때만 받는다. 옵션은 첫 피연산자 앞에만 올 수 있고(BSD `sed`는 첫 피연산자에서 옵션 읽기를 멈춘다), `--` 뒤는 모두 피연산자다. `-E`·`-r` 밖의 `sed` 옵션(`-f`, `-i` 등)은 위반이다. 위반의 이유는 명령줄과 첫 문제를 보인다. 첫 문제는 줄 어디에든 있는 셸 구문(백틱, `$(`, `<(`, `>(`, 줄바꿈)이고, 그것이 없으면 왼쪽부터 읽어 처음 규칙을 어긴 부분이다. clean-review 0.2.1과 달리 `&&`·`||`·`;`와 `true`를 받고, 셸 확장과 출력만 하지 않는 `sed`는 받지 않는다. 이은 부분은 하나하나 clean-review 0.2.1도 받는 명령이다. 파일 읽기 도구는 0.2.1과 같다(0.2.0의 도구에 agy 1.4.3이 쓰는 `read_file`을 더한 것). `agy`는 이 규칙을 2절의 명령 안내로 받는다. 그 밖에 허용 프로그램이 파일을 쓰는 옵션(예: `sort -o`)은 따로 막지 않는다. 파일 쓰기는 `agy`의 `--sandbox`에 맡기고, `head/`·`base/`·`PACKET.md`가 바뀌면 `workspace_modified`다.

대체 모델: 지정 모델이 사용량·요청 한도로 거절되고(거절 문구: G13, [입력 형식](spar-input.md) 5절) `fallback_model`이 있으면 같은 패킷과 새 작업공간 사본으로 한 번 더 실행한다. 인증 거절이나 다른 실패에는 쓰지 않는다. 에이전트가 `--model`로 고른 모델의 대체 모델은 사용자가 적은 `fallback_model`뿐이다([설정 계약](spar-config.md) 1절). 대체 실행은 지정 모델의 실행과 같은 강도를 쓴다. `attempts`에 시도마다 모델·상태·원인·재설정 문구가 남고 `fallback_used`가 `true`가 된다.

## 5. 라운드 폴더

| 파일 | 내용 |
| --- | --- |
| `round.json` | 아래 필드 |
| `packet.md`, `schema.json` | 패킷과 결과 schema |
| `findings.json` | 분류된 지적 배열. 지적마다 리뷰어가 준 `smallest_fix`를 그대로 남기고, 없으면 빈 문자열이다 |
| `ledger.jsonl` | 처리 원장. 한 줄에 한 처리, 덧붙이기만 한다 |
| `waivers.jsonl` | 필수 리뷰어를 빼는 사용자 결정(10절). 한 줄에 한 결정, 덧붙이기만 한다. 결정이 없는 라운드에는 없다 |
| `summary.md` | 사람이 읽는 요약. 방향이 있는 지적은 지적 표 뒤 `## Smallest fixes` 절에 `- <지적 ID>: <방향>`으로 보인다(앞뒤 공백을 뗀 `smallest_fix`, 그 안의 줄바꿈 뒤 줄은 두 칸 들여 쓴다). 방향이 있는 지적이 없으면 그 절이 없다. 리뷰어 줄은 요청 모델과 그 출처(`model <모델> [<출처>]`)를, `claude`·`codex`는 강도와 그 출처(`effort <강도> [<출처>]`, 사용자가 비워 둔 강도는 `effort none [user]`)를 보인다. 출처가 없는 예전 기록은 모델만 출처 없이 보이고 강도는 보이지 않는다 |
| `<리뷰어>/` | 그 리뷰어의 표준 출력·오류와 결과. 대체 모델은 `<리뷰어>/fallback/` |
| `workspace/` | dry run에서만: 리뷰어가 받을 작업공간(`head/`, `base/`) |
| `<native 리뷰어>/` | `prompt.md`, 작업공간, 기록된 결과. 기본 이름이면 `native/` |

`round.json`의 주요 필드: `schema`(`review-round/v1`), `spar_version`, `stage`, `label`, `series`, `sequence`, `repo`, `base`, `head`, `base_tree`, `head_tree`, `diff_sha256`, `criteria`·`rules`·`subjects`(`name`, `source`, `sha256`), `criteria_digest`(기준 문서 이름과 sha256의 sha256), `questions`, `prior`, `packet_sha256`, `config`([설정 계약](spar-config.md)의 적용 기록), `links`(연결 확인에 쓴 `criteria_ids`와 규칙 문서의 `rules[]`: `names`, `file_name`, `lines`), `requested_reviewers`, `required_reviewers`, `dry_run`, `counted`, `rerun_of`, `started_at`, `finished_at`, `reviewers`.

`reviewers.<이름>`: `name`, `preset`, `isolation`(`verified`, `host`, `unverified`), `required`, `status`, `reason`, `requested_model`, `model_source`(`claude`·`codex`·`agy`만: `user`, `agent`, `preset`), `observed_model`, `fallback_model`, `fallback_used`, `effort`, `effort_source`(`claude`·`codex`만, 같은 세 값), `elapsed_seconds`, `timeout_seconds`(G19), `resets`, `attempts`, `findings`(완료한 경우의 지적 수, 아니면 `null`), `verdict`, `angles_checked`, `angles_unchecked`, `limits`, `warnings`, `agent`, `recorded_from`, `workspace_digest`(native 작업공간을 준비할 때의 digest).

## 6. 분류

`findings.json`의 항목은 리뷰어 결과의 필드에 `id`(`<리뷰어>-F<n>`, 보고 순서), `reviewer`, `reviewer_id`(리뷰어가 붙인 `id`), `class`, `class_reason`, `path`, `in_check_paths`, `location_unreadable`, `claimed_link_kind`, `claimed_link`을 더한다. `path`와 `in_check_paths`는 위치를 읽은 결과다(G7). `link_kind`와 `link`는 확인된 연결(G8, G9)만 남고, 확인되지 않으면 `none`과 빈 문자열이다. 이 목표 전에 쓴 기록처럼 `location_unreadable`이 없는 지적은 위치를 읽은 지적이다. 리뷰어가 `smallest_fix`를 빼면 빈 문자열로 남고, `smallest_fix`가 없는 예전 기록의 지적은 방향 없음이다. 아래 규칙은 `smallest_fix`를 읽지 않는다.

위치를 읽은 지적의 분류 규칙(위에서 처음 맞는 것):

1. 위치가 검사 경로이고 `check_impact`가 `false_pass`나 `meaning_inverted`이면 `defect`
2. 위치가 검사 경로이고 `contract_violation`이며 연결이 확인되면 `defect`
3. 위치가 검사 경로이면, `proposed_class`가 `requirement-question`이고 연결이 확인되며 그 연결이 검사 경로의 파일을 가리키는 `contract` 연결이 아닐 때 `requirement-question`, 그 밖에는 제안 분류와 관계없이 `hardening`
4. `proposed_class`가 `requirement-question`이면 `requirement-question`
5. `introduced_by_change`와 `regression`이 모두 `yes`이면 `defect`
6. `proposed_class`가 `defect`이고 연결이 확인되지 않으면 `note`
7. 그 밖에는 `proposed_class`

위치를 읽은 `hardening`은 라운드를 기록할 때 `{"disposition": "defer", "auto": true}` 원장 항목으로 자동 보류된다. 규칙 3의 `requirement-question`은 자동 보류하지 않으며, 7절대로 사용자의 말을 인용한 처리로만 닫힌다. 그 연결은 질문이 묻는 기준·규칙·계약이어야 하고(리뷰어 패킷이 알린다), spar는 연결한 대상이 있는지와, `contract` 연결이면 그 대상(G8이 찾은 규칙 문서나 head 파일의 저장소 경로)이 검사 경로 밖인지를 확인한다. 저장소 밖의 규칙 문서는 저장소 경로가 없으므로 늘 검사 경로 밖이다. 검사 경로의 파일을 가리키는 `contract` 연결은 검사 코드 자신을 가리키므로 규칙 3에서 확인된 연결로 치지 않는다. 규칙 2의 연결 확인에는 이 제한이 없다. 위치를 읽을 수 없는 지적은 [입력 형식](spar-input.md) 4절대로 분류하고 자동 보류하지 않는다: 규칙 3과 6을 쓰지 않고 1과 2는 검사 경로 조건 없이 쓴다.

분류는 지적을 기록할 때 정해진다. gate와 요약은 `findings.json`에 기록된 분류를 읽으므로, 이미 기록한 지적은 규칙이 바뀌어도 다시 분류되지 않는다.

## 7. 처리

`spar triage <라운드> <지적> <처리> [옵션]`이 원장에 한 줄을 더한다. 규칙에 맞지 않으면 아무것도 쓰지 않고 65로 끝난다. 마지막 항목이 그 지적의 처리다.

| 처리 | 요구 |
| --- | --- |
| `fix` | `--commit`(라운드 head 뒤의 커밋), `--failed-before`, `--passed-after` |
| `reject` | `--evidence`(`requirement-question`은 대신 `--user-quote`) |
| `defer` | `--to`. `defect`는 `--user-quote`도 |
| `duplicate` | `--of` 같은 라운드의 다른 지적. `defect`와 `requirement-question`은 같은 분류의 지적만 가리키고, 가리킨 지적이 닫혀야 닫힌다(gate) |
| `answer` | `requirement-question`에만 |

`requirement-question`을 닫는 `fix`·`reject`·`defer`·`answer`와, `hardening`의 `fix`(자동 보류된 것과 위치를 읽을 수 없는 것 모두)는 사용자의 말을 인용한 `--user-quote`가 필요하다. 지적 ID와 옵션의 형식: G30. `note`는 처리하지 않아도 된다.

원장 항목: `finding`, `disposition`, `auto`, `reason`, `commit`, `failed_before`, `passed_after`, `evidence`, `of`, `to`, `user_quote`, `note`, `recorded_at`.

## 8. 상한·재실행·사슬

- 단계 묶음은 `plan`, `plan_delta`, `code`(candidate와 fixes)다. series에서 `counted`가 `true`인 같은 묶음의 라운드 수가 상한과 연장의 합 이상이면 새 실행을 거부한다.
- 같은 단계·base·head·패킷 digest의 앞선 실행이 있고, 이번 실행의 리뷰어가 모두 그 실행들에서 완료하지 못한 리뷰어(이름과 preset이 같은 리뷰어)이면 상한에 세지 않는다(`counted: false`, `rerun_of`). 라운드는 series마다 하나씩 실행되므로 `finished_at`이 없는 앞선 라운드는 도중에 멈춘 것이고, 그 리뷰어는 완료하지 못한 것이다. 그 밖의 실행은 모두 센다.
- fixes 라운드의 base는 같은 series의 마지막 code 라운드의 head여야 한다. 아니면 거부한다. 이번 실행과 단계·base·head·패킷 digest가 모두 같은 앞선 라운드(같은 패킷의 재실행)만 건너뛴다. 패킷이 다르면 base·head가 같아도 그 라운드가 사슬의 마지막이다.
- 연장은 `spar extend <series> --stage plan|plan-delta|code --by N [--cause <원인>] [--reason "…"] --user-quote "…"`로 series의 `extensions.jsonl`에 `{"stage", "by", "cause", "reason", "user_quote", "recorded_at"}`을 더한다. `cause`와 `reason`은 있을 때만 쓴다. 사용자의 말이 없으면 거부한다. series 경로는 `--out`과 같은 기록 경로 규칙(G28)을 따르고(검토하는 저장소를 찾지 못해도 그 경로가 속한 저장소로 확인한다), `--repo`가 Git 저장소가 아니면 거부한다.
- code 연장은 상한을 다 쓴 원인을 `--cause`로 받는다. 값은 `churn`, `recurring`, `unrelated`, `hardening`, `other` 다섯 가운데 하나이고(대소문자 구분), 이 다섯이 전부다. 없거나 목록 밖이면 사용법 오류(64)다. `--reason`은 앞뒤 공백을 떼고 기록하며, 뗀 뒤 비면 이유가 없는 것이다. `--by`가 2 이상이거나 `--cause other`이면 이유가 필요하고, 없으면 거부(65)한다. plan·plan-delta 연장은 `--cause`·`--reason`을 받지 않아 사용법 오류(64)로 끝나며, 그 줄은 이전처럼 `{"stage", "by", "user_quote", "recorded_at"}`이다. 어느 오류와 거부도 아무것도 쓰지 않는다. 원인은 실행자가 11절의 보고를 보고 고르며, spar는 고르지 않는다.
- 상한 계산, 재실행 판단, gate는 `cause`·`reason`을 읽지 않는다. 원인이 없는 줄(이 기능 전에 기록한 줄)도 지금처럼 `stage`와 `by`로 센다.
- code 상한 거부 메시지는 위의 글 뒤에 ``; read why with `spar cap-report <series>` and record a code extension with --cause``를 붙인다. plan·plan-delta 상한 거부 메시지는 그대로다.

## 9. native 리뷰어

`native` preset 리뷰어는 `pending`으로 시작하고 자기 이름의 폴더에 `prompt.md`(지시와 패킷 전문)와 `workspace/`를 남긴다(기본 이름이면 `native/prompt.md`). 호스트의 clean-context 하위 에이전트가 답하면 `spar record <라운드> <이름> --result <파일|->`나 `--transcript <전사 기록>`으로 기록한다. dry run 라운드와 이미 완료한 리뷰어는 기록하지 않는다. 기록할 때 작업공간이 준비 때와 같은지, 결과가 schema·head·각도 요구를 만족하는지, 라운드의 `findings.json`·`ledger.jsonl`이 있는지 확인한다(없으면 거부). 전사 기록(G15)에서 `Read`·`Grep`·`Glob` 밖의 도구 사용, 호스트가 제공한 그 밖의 도구, 호스트가 더한 맥락(Git 상태, 지시 파일, hook 맥락), 프롬프트를 읽지 않은 것, 그리고 `Read`·`Grep`·`Glob`이 native 작업공간과 `prompt.md` 밖을 읽은 것은 `warnings`에 남는다. 맥락의 내용과 읽은 경로는 기록하지 않고 종류와 도구 이름만 남긴다. 격리 등급은 `host`이고 필수 리뷰어에 든다.

## 10. 필수 리뷰어를 빼는 결정

필수 리뷰어가 라운드를 끝내지 못했고 사용자가 그 리뷰어 없이 진행하라고 했으면, 그 말을 그 라운드에 기록한다.

```bash
spar waive docs/reviews/example/02-candidate agy --user-quote "agy 없이 진행"
```

- `spar waive <라운드> <리뷰어> --user-quote "…"`는 그 라운드 폴더의 `waivers.jsonl`에 `{"round", "reviewer", "preset", "status", "reason", "user_quote", "recorded_at"}` 한 줄을 더한다. `round`는 라운드 폴더 이름, `preset`은 그 필수 리뷰어의 preset, `status`와 `reason`은 기록할 때 `round.json`에 있는 그 리뷰어의 상태와 원인이다. 라운드가 그 이름과 preset의 리뷰어를 실행하지 않았으면 `status`는 `not_run`이고 `reason`은 없다.
- 사용자의 말이 없으면 기록하지 않는다.
- 다음 경우에는 아무것도 쓰지 않고 이유와 함께 65로 끝난다: `--user-quote`가 없거나 앞뒤 공백을 뗀 뒤 비어 있음, 라운드를 읽을 수 없음(`round.json`·`findings.json`·`ledger.jsonl`이 없거나 해석되지 않음, G32), dry run 라운드, 그 리뷰어가 그 라운드의 필수 리뷰어([검토 gate](review-gate.md) 2절)가 아님, 그 리뷰어가 그 라운드나 같은 series에서 단계·base·head·패킷이 같은 다른 라운드(재실행)에서 같은 preset의 필수 리뷰어로 완료함, 그 라운드에 그 리뷰어의 결정이 이미 있음, `waivers.jsonl`을 읽을 수 없음, 이 기능 전에 기록한 라운드에 이름이 `waivers.jsonl`(대소문자 무시)인 리뷰어가 있어 그 폴더가 파일 자리를 차지함.
- 결정이 기록된 리뷰어는 그 라운드의 `degraded` 사유가 되지 않고 gate 파일에 따로 보인다. 완료한 필수 리뷰어가 하나도 없는 라운드는 결정이 있어도 `degraded`다([검토 gate](review-gate.md) 2절).
- 결정은 그 라운드에만 적용된다. 같은 패킷의 재실행도 다른 라운드이므로, 그 라운드에서도 그 리뷰어가 끝내지 못하면 다시 기록한다.

## 11. 상한 보고

`spar cap-report`는 series의 code 라운드 기록과 Git을 읽어, 상한을 쓴 모습과 상한 안에서 끝나지 못한 이유를 볼 신호를 보인다. 원인을 고르거나 연장을 권하지 않는다. 원인은 실행자가 고르고, 연장은 사용자가 정한다(8절).

```bash
spar cap-report docs/reviews/example --format json --repo .
```

- **읽는 것.** series의 라운드(`round.json`, `findings.json`, `ledger.jsonl`), `extensions.jsonl`, 그리고 `--repo`(없으면 현재 폴더)를 품은 Git 저장소만 읽는다. 설정 파일을 읽지 않고, series를 잠그지 않으며(`spar gate`와 같다), 아무것도 쓰지 않는다. 부분 clone에 없는 Git 객체를 가져오지 않도록 Git에 `GIT_NO_LAZY_FETCH=1`을 준다(Git 2.44부터 따른다). 현재 시각을 쓰지 않으므로 같은 기록과 Git이면 출력이 바이트까지 같다.
- **형식.** `--format md`(기본)는 사람이 읽는 Markdown이고, `--format json`은 `spar-cap-report/v1` JSON 객체다. 둘은 같은 값을 담는다.
- **종료 코드.** 0 보고, 64 사용법 오류(series 없음, 모르는 옵션, `md`·`json` 밖의 형식), 65 거부. 거부하면 표준 출력에 아무것도 쓰지 않는다.

| 키 | 값 |
| --- | --- |
| `schema` | `spar-cap-report/v1` |
| `series` | 준 series 경로 그대로 |
| `cap` | `code`: series에서 `sequence`가 가장 큰 라운드(단계와 관계없이)의 설정 code 상한, 라운드가 없으면 `null`. `extensions`: code 연장 줄의 `by` 합계(원인이 없는 줄도 센다). `allowed`: `code + extensions`, `code`가 `null`이면 `null`. `counted`: `counted`가 참이고 dry run이 아닌 code 라운드 수. `spar run`의 상한 판단과 같으므로 지적을 기록하기 전에 멈춘 라운드도 센다 |
| `rounds` | `candidate`·`fixes` 라운드 가운데 dry run이 아니고 지적을 기록하기 전에 멈추지 않은 것, sequence 순(같으면 폴더 이름 순). 라운드마다 `round`(폴더 이름), `stage`, `sequence`, `base`, `head`(전체 SHA), `counted`, `classes`(`defect`·`requirement-question`·`hardening`·`note`별 지적 수). 같은 패킷의 재실행은 `counted: false`로 든다 |
| `skipped` | 목록과 신호에서 뺀 code 라운드: `{round, reason}`. `reason`은 dry run이면 `dry run`, 지적을 기록하기 전에 멈춘 라운드(gate처럼 `finished_at`도 `findings.json`도 없음)면 `stopped before recording findings` |
| `open` | gate와 같은 기준([검토 gate](review-gate.md))으로 열린 `defect`·`requirement-question`: 마지막 처리가 없거나 `duplicate` 사슬이 열린 지적에서 끝난다. `{round, finding, class}` |
| `churn` | 첫 counted `candidate` 라운드 뒤의 라운드마다 `{round, count, findings}`. 대상은 `defect`·`requirement-question` 가운데 위치를 읽을 수 있고, 경로가 그 첫 라운드의 head와 그 라운드의 head 사이에서 바뀐 파일(`git diff --name-only --no-renames`)인 지적이다 |
| `recurring` | 같은 연결(`link_kind`, `link`)이 패킷(단계·base·head·패킷 digest)이 다른 라운드 둘 이상에서 `defect`·`requirement-question`으로 나온 것: `{link_kind, link, rounds}`. 같은 패킷의 재실행은 한 라운드라서, `rounds`에는 패킷마다 목록 순으로 처음 나온 라운드만 든다. 연결 순으로 정렬한다 |
| `unrelated` | `introduced_by_change`가 `no`나 `unknown`인 `defect`: `{round, finding}`. 그 수는 목록의 길이다 |
| `check_growth` | gate `growth`와 같은 계산: 목록에 든 `fixes` 라운드가 더한 줄(`git diff --numstat -M`)을 그 라운드 설정의 검사 경로 안(`check`)과 밖(`other`)으로 나눈 합. `over_hardening`은 `check`가 `other`보다 클 때 참 |
| `limits` | 세 지적 신호의 경계 문장(아래) |

세 지적 신호(`churn`, `recurring`, `unrelated`)는 마지막 처리가 `reject`나 `duplicate`인 지적을 세지 않는다. 열린 지적과 class 수는 처리와 관계없이 기록을 그대로 센다. 신호의 경계는 `limits`에 고정 문장으로 함께 나온다.

- `churn`: "Counted by file: a finding in a file the fixes changed counts even when it is about another line." 파일 단위이므로, 바뀐 파일의 무관한 줄에 대한 지적도 센다.
- `recurring`: "Counted by link: the same problem reported under different links is not grouped." 같은 문제를 다른 연결로 낸 지적은 묶이지 않는다.
- `unrelated`: "Uses each reviewer's introduced_by_change as reported." 리뷰어가 낸 판단을 그대로 쓴다.

Markdown은 JSON의 `schema`(형식 이름)를 뺀 모든 값을 담는다. 순서는 다음과 같다. 빈 목록은 `- none`이다.
1. `# Cap report: <series>`
2. `- code rounds counted: <counted> of <allowed> (cap <code>, extensions <n>)`. 상한을 모르면 `of unknown (cap unknown, extensions <n>)`.
3. `- open: defect <n>, requirement-question <n>`
4. `## Rounds` 표(열 `Round | Stage | Sequence | Counted | Base | Head | defect | requirement-question | hardening | note`, 커밋은 전체 ID). 건너뛴 라운드가 있으면 `- skipped <round>: <reason>`.
5. `## Open findings`: `- <round> <finding> (<class>)`
6. `## Signals`:
   - `### Churn`: `- <round>: <count> (<ID>, …)`, 0이면 `- <round>: 0`.
   - `### Recurring links`: `- <link_kind> <link>: <round>, …`
   - `### Unrelated defects`: 먼저 `- count: <n>`, 이어서 `- <round> <finding>`. 0이면 `- count: 0` 줄만 있다.
   - `### Check growth`: `- added lines: check <n>, other <n>; over hardening: yes|no`
7. `## Limits`: `- churn: …`, `- recurring: …`, `- unrelated: …`

다음 경우에는 표준 오류에 실패한 경로나 커밋을 밝히고 65로 끝나며 보고를 내지 않는다. 깨진 입력을 신호가 없는 보고로 바꾸지 않는다.
- series가 없거나 폴더가 아니다.
- `round.json`을 열 수 없거나 JSON이 아니다. `round.json`이 없거나 다른 형식의 JSON(`schema`가 `review-round/v1`인 객체가 아닌 것, 목록 포함)인 폴더는 다른 도구의 것이라 건너뛴다.
- `round.json`의 `stage`가 계약의 네 단계 밖이다.
- 보고가 읽는 `round.json` 필드가 없거나 `null`이거나 spar가 쓰는 JSON 타입이 아니다: `stage`·`base`·`head`·`packet_sha256`(글), `sequence`(정수), `dry_run`·`counted`(참거짓), `config`(객체)의 `caps.code`(정수)와 `check_paths`(글의 목록, `null`은 목록 없음), 그리고 있을 때의 `finished_at`(글).
- 끝난 라운드(`finished_at`이 있음)에 `findings.json`이 없거나, `findings.json`이 있는데 `ledger.jsonl`이 없다.
- 목록 라운드의 `findings.json`이 지적의 목록이 아니거나(`null` 포함), 지적에 `id`가 없거나, `class`가 네 class 밖이거나, 신호가 읽는 `introduced_by_change`가 `yes`·`no`·`unknown` 밖이거나 `link_kind`가 `criterion`·`rule`·`contract`·`none` 밖이다. 또는 지적의 `id`·`class`·`introduced_by_change`·`link_kind`·`link`·`path`가 없거나 `null`이거나 글이 아니거나, 있는 `location_unreadable`이 참거짓이 아니다.
- 목록 라운드의 `ledger.jsonl` 줄이 해석되지 않거나, `finding`이 없거나, `disposition`이 다섯 처리 밖이다.
- `extensions.jsonl` 줄이 해석되지 않거나 형식 밖이다: `stage`가 `plan`·`plan-delta`·`code` 밖, `by`가 1 미만, `user_quote`가 비었음, `cause`가 code 줄이 아닌 곳에 있거나 다섯 값 밖, `cause`가 있는 줄의 `by`가 2 이상이거나 `cause`가 `other`인데 `reason`이 없거나 비었음. 원인이 없는 code 줄(이 기능 전에 기록한 줄)에는 이유를 요구하지 않는다.
- `--repo`가 Git 저장소가 아니거나 Git을 시작할 수 없다.
- 실행하는 Git이 2.44보다 오래되었고(`GIT_NO_LAZY_FETCH`를 모른다) 저장소가 부분 clone이다(`extensions.partialClone`이나 참인 `remote.<이름>.promisor`가 있음). 객체를 읽기 전에 거부하며, 이유에 Git 버전을 밝힌다.
- 목록 라운드의 base나 head가 저장소에 없다(얕은 clone 포함), 또는 보고에 필요한 객체가 부분 clone에 없다.
