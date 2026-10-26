# 계약: Spar 입력 형식

`spar`가 리뷰어 결과, 리뷰어 실행, 설정 파일, 명령 인자, 다시 읽는 기록에서 받는 입력 필드는 아래 표 하나가 정한다. 표는 필드마다 받는 형식과 정규화, 형식 밖 입력의 결과, 그 규칙을 쓰는 곳(소비처)을 적는다. 받는 형식 밖의 입력은 표에 적은 결과로 끝나고, 다른 값으로 읽히거나 조용히 받아들여지지 않는다. 한 행의 소비처는 모두 같은 규칙을 쓴다. [검토 라운드](review-round.md), [검토 gate](review-gate.md), [Spar 설정](spar-config.md)은 각 필드의 형식을 이 표의 행으로 가리킨다. 아래 예시의 경로·이름은 합성이다.

## 1. 결과 이름

| 결과 | 뜻 |
| --- | --- |
| `invalid_output` | 그 리뷰어의 결과를 쓰지 않는다([검토 라운드](review-round.md) 4절). 이유에 문제의 키 이름이 들어간다 |
| 읽을 수 없는 위치 | 리뷰어 결과는 그대로 쓰고 그 지적만 4절대로 다룬다 |
| 설정 오류 | 종료 코드 65, `<파일>:<줄>` 메시지. 리뷰어를 띄우지 않고 라운드 폴더를 만들지 않는다 |
| 거부 | 종료 코드 65. 아무것도 쓰지 않는다 |
| 사용법 오류 | 종료 코드 64 |
| 확인되지 않은 연결 | `link_kind`는 `none`, `link`는 빈 문자열이고, 리뷰어가 댄 값은 `claimed_link_kind`·`claimed_link`에 남는다 |

## 2. 입력 형식 표

| 행 | 필드 (주는 쪽) | 받는 형식과 정규화 | 형식 밖의 결과 | 소비처 |
| --- | --- | --- | --- | --- |
| G1 | 결과 객체 (리뷰어) | JSON 객체 하나. 키는 정확히 `reviewed_head`, `verdict`, `angles_checked`, `angles_unchecked`, `findings`, `limits` | 객체가 아님, 키가 빠짐, 모르는 키: `invalid_output`(이유에 키 이름) | 모든 preset의 결과 확인(`spar run`), `spar record --result`·`--transcript`(`spar record`) |
| G2 | `reviewed_head` (리뷰어) | 7자 이상이고 패킷 head 커밋 ID의 앞부분과 글자 그대로 같은 문자열 | `invalid_output` | G1과 같음(`spar run`, `spar record`) |
| G3 | `verdict` (리뷰어) | `no_blocking_findings`, `blocking_findings`, `could_not_review`. `could_not_review`는 `failed`로 기록 | `invalid_output` | G1과 같음(`spar run`, `spar record`) |
| G4 | 각도 항목 (리뷰어) | `angles_checked`의 항목은 `{angle, result}`, `angles_unchecked`의 항목은 `{angle, reason}`, 값은 문자열, `angle`은 다섯 각도 중 하나. 요청한 각도는 두 목록 어디든 한 번 이상 나와야 한다. 같은 각도가 여러 번, 두 목록에 함께 나와도 받고 받은 그대로 기록한다 | 목록이 아님, 항목이 객체가 아님, 키 빠짐·모르는 키, 모르는 각도, 요청 각도 누락: `invalid_output` | G1과 같음(`spar run`, `spar record`), 요약(`summary.md`)의 각도 |
| G5 | 지적 항목 (리뷰어) | 키는 정확히 `id`, `proposed_class`, `angle`, `location`, `claim`, `failure_scenario`, `evidence`, `link_kind`, `link`, `introduced_by_change`, `regression`, `check_impact`, `confidence`이고 모두 문자열이며, 여기에 선택 키 `smallest_fix`(문자열, 빈 문자열 포함)가 더해질 수 있다. `smallest_fix`는 키가 없어도 받고, 빈 문자열과 키가 없는 경우는 모두 방향 없음이다. 열거 값은 [검토 라운드](review-round.md) 3절의 값. `failure_scenario`는 공백만이 아님. `id`·`claim`·`evidence`·`smallest_fix`는 아무 문자열(`id`는 `reviewer_id`로만 남고 라운드의 지적 ID는 spar가 `<리뷰어>-F<n>`으로 매긴다) | 객체가 아님, 키 빠짐(`smallest_fix` 제외), 모르는 키, 문자열 아님(`smallest_fix` 포함), 열거 밖 값, 빈 `failure_scenario`: `invalid_output` | G1과 같음(`spar run`, `spar record`), `findings.json`의 `smallest_fix`, 요약(`summary.md`)의 `## Smallest fixes` |
| G6 | `limits` (리뷰어) | 문자열(빈 문자열 포함) | 문자열이 아님: `invalid_output` | G1과 같음(`spar run`, `spar record`) |
| G7 | 지적 `location` (리뷰어) | 3절의 문법. 접두 `head/`·`base/`·`./` 가운데 앞에 있는 하나만 한 번 뗀다. 대상: 접두 `head/`·`./`는 head 사본, `base/`는 base 사본, 접두 없음은 head 다음 base 사본의 파일이나 폴더(그 아래 추적 파일이 있는 경로, 끝의 `/`는 뗌, 접두만 있으면 저장소 최상위). 접두 없음·`head/`·`./`는 사본에 없으면 라운드 문서의 이름(G26)도 가리킬 수 있다. 줄이 없으면 위치 전체가 대상이면 그것, 아니면 첫 단어. `path`는 대상의 저장소 경로(라운드 문서는 G26의 이름), 저장소 최상위는 빈 경로이고 검사 경로가 아니다. 줄은 기록만 하고 분류에 쓰지 않으며 파일 줄 수와 대조하지 않는다 | 문법 밖이거나 대상이 없음(줄 0, 뒤 줄 < 앞 줄 포함): 읽을 수 없는 위치. 그 지적만 `path`가 빈 값, `in_check_paths`가 `false`, `location_unreadable`이 `true`이고 `class_reason`이 `unreadable location`으로 시작한다. 분류는 4절을 따르고 자동 보류되지 않는다 | 분류(`path`, `in_check_paths`, `class`), 라운드 기록 때의 자동 보류(`ledger.jsonl`), `findings.json`, 요약(`summary.md`의 `(unreadable location)` 표시), gate의 `auto_deferred`, 패킷(`packet.md`)의 보고 방법 문장. `spar run`과 `spar record`가 같은 규칙을 쓴다 |
| G8 | `rule`·`contract` 연결 `link` (리뷰어) | 3절의 문법. 접두 `head/`·`./` 가운데 앞에 있는 하나만 한 번 뗀다(`base/`는 저장소 폴더 이름으로 둔다). 줄이 없으면 연결 전체가 대상 문서면 그것, 아니면 첫 단어. 대상: `rule`은 라운드의 규칙 문서, `contract`는 규칙 문서이거나 head 커밋의 일반 파일(모드 `100644`·`100755`). 규칙 문서가 먼저다: 저장소 상대 경로나 실제 절대 경로가 맞는 문서, 없으면 파일 이름이 맞는 문서(그 이름의 규칙 문서가 하나일 때만). `contract`는 그다음 head 커밋의 일반 파일을 저장소 경로로 본다. 줄을 대면 1 ≤ 앞 줄 ≤ 뒤 줄 ≤ 문서 줄 수(줄 수는 줄바꿈 수, 마지막 줄이 줄바꿈으로 끝나지 않으면 하나 더). `round.json`의 `links.rules[]`가 규칙 문서의 `names`·`file_name`·`lines`를 남긴다 | 문법 밖, 대상 없음, 범위의 어느 한 끝이라도 문서에 없음, 뒤 줄 < 앞 줄, 줄 0, 파일 이름이 겹치는 규칙 문서(head 파일도 없을 때), 심볼릭 링크·submodule·폴더: 확인되지 않은 연결 | 분류의 연결 확인(분류 규칙 2·3·6, `class`), `findings.json`, 패킷(`packet.md`)의 보고 방법 문장. `spar run`과 `spar record`가 같은 규칙을 쓴다 |
| G9 | 기준 ID: `criterion` 연결과 기준 문서 (리뷰어, 호출자) | `AC-<숫자>`로, 바로 앞과 바로 뒤가 ASCII 영문자·숫자가 아닌 것. 한 글에 여러 개 있어도 각각 읽는다(`AC-1,AC-2`). 기준 문서의 ID는 문서 전문에서 같은 규칙으로 찾은 모든 ID. 비교는 글자 그대로(`AC-01`과 `AC-1`은 다르다) | 연결에 ID가 없거나 하나라도 기준 문서에 없음: 확인되지 않은 연결 | 기준 문서 ID 집합(`round.json`의 `links.criteria_ids`), 분류의 `criterion` 연결 확인(`class`), `findings.json`, 패킷(`packet.md`)의 보고 방법 문장 |
| G10 | 사용한 도구와 항목 종류 (리뷰어 실행) | [검토 라운드](review-round.md) 4절의 preset별 목록. agy는 `read_file`을 포함한다 | `tool_violation`, 결과를 쓰지 않음 | claude·codex·agy 실행 감시(`spar run`) |
| G11 | agy 셸 명령 (리뷰어 실행) | [검토 라운드](review-round.md) 4절의 읽기 전용 프로그램과 `sed -n`, 연산자는 `\|`만. `2>/dev/null`과 `2>&1`은 지운 뒤 본다(clean-review 0.2.1과 같다) | 그 밖의 프로그램·연산자·리디렉션·명령 치환, 금지 인자 접두: `tool_violation` | `agy` 실행 감시(`spar run`) |
| G12 | 실제 모델 `observed_model` (리뷰어 실행) | 리뷰어 출력이 밝힌 모델 이름. 출력이 밝히지 않으면 모름이며, `round.json`에는 `null`, 시도 기록(`attempts`)에는 그 필드가 없다 | 해당 없음(`null`이 모름) | `round.json`, 요약(`summary.md`) |
| G13 | 사용량·인증 거절 (리뷰어 실행) | 5절의 거절 문구(사용량·요청 한도, 인증)와 재설정 문구. 5절이 spar가 알아보는 문구를 모두 적는다 | 그 밖의 메시지는 거절이 아니다: 대체 모델을 쓰지 않고 종료 상태대로 기록 | 대체 모델(`fallback_model`) 판단, `attempts`, 요약(`summary.md`) |
| G14 | 실행 파일 찾기: `binary`, preset 이름, `command`의 `argv[0]` (설정) | `/`가 든 값은 그 경로. `/`가 없는 이름은 PATH에서 실행할 수 있는 파일 | `/`가 든 경로가 없음: `unavailable`. 있지만 일반 파일이 아니거나 실행 권한 없음: `failed`. PATH에 실행할 수 있는 파일이 없음(실행 권한 없는 파일만 있어도): `unavailable` | 리뷰어 실행 준비(`spar run`) |
| G15 | native 전사 기록 (호스트) | Claude Code 전사 기록 JSONL. 경고: 허용 밖 도구, 호스트가 더한 도구·맥락, `prompt.md`를 읽지 않음, 그리고 `Read`의 `file_path`·`Grep`·`Glob`의 `path`가 native 작업공간과 `prompt.md` 밖을 가리킴(`the subagent read outside the native workspace with <도구>`). 상대 경로와 경로 없음은 그 항목의 `cwd`로 푼다. 경고는 종류와 도구 이름만 남기고 경로나 내용은 남기지 않는다 | 읽을 수 없는 전사 기록: 거부. `cwd`가 없는 항목의 상대 경로·경로 없음: 작업공간 밖으로 경고 | `spar record --transcript`, `round.json`의 `warnings` |
| G16 | 설정 출처 파일: `SPAR_CONFIG`, `$XDG_CONFIG_HOME/jaekit/spar.toml`, `~/.config/jaekit/spar.toml`, clean-review `config.json`, 프로젝트 `.jaekit/spar.toml` (사용자·저장소) | 없는 경로(깨진 심볼릭 링크와 심볼릭 링크 고리 포함)는 없는 것으로 보고 다음 출처로 넘어간다(사용자 출처는 `source.skipped`에 남고, 프로젝트 설정은 기본값). 있는 경로는 일반 파일이어야 하고 심볼릭 링크는 따라간 대상을 본다. 빈 환경 변수는 없는 것이다. 프로젝트 설정은 커밋이 아니라 작업 트리에서 읽고, 읽은 파일의 경로와 sha256이 적용 기록 `config.files`에 남는다. 설정 `digest`는 적용한 값만 본다 | 있지만 일반 파일이 아님(폴더, FIFO 등)이거나 `stat`이 그 밖의 이유로 실패: 설정 오류 | `spar config`, `spar run` |
| G17 | TOML 문법과 키 (사용자·저장소) | TOML 1.0.0 전체(`go-toml/v2` v2.2.4). 키는 [Spar 설정](spar-config.md)의 것. 프로젝트 설정의 금지 키는 어느 깊이에서도 오류 | 문법 오류, 모르는 키, 금지 키: 설정 오류 | `spar config`, `spar run` |
| G18 | clean-review `config.json` (사용자) | JSON 객체. `reviewers`가 없으면 기본 목록. 있으면 문자열 배열이고, 각 문자열은 앞뒤 공백을 떼고 빈 문자열은 건너뛴다(clean-review와 같다). 남은 이름은 `claude`, `codex`, `agy`, `native` 중 하나, 겹치지 않음. 모두 비면 `native` 하나 | 객체 아님(`null` 포함), `reviewers`가 배열이 아님(`null` 포함), 문자열 아닌 항목, 모르는·겹친 이름: 설정 오류 | `spar config`, `spar run` |
| G19 | `timeout_seconds`, 최상위와 리뷰어별 (사용자) | 양의 정수. 리뷰어별 값이 앞선다. 최상위 값은 그 파일로 정해지는 모든 리뷰어의 기본값이다: `reviewers`가 없어 기본 목록이 될 때, 빈 목록이라 `native`가 될 때, `--reviewers`가 설정에 없는 preset 이름을 고를 때도. 시간 제한은 시도마다 리뷰어의 실행 파일 버전 확인을 포함한 실행 전체에 걸린다(버전 확인 자체는 30초를 넘지 않는다) | 0, 음수, 정수 아님: 설정 오류 | 리뷰어 실행(`spar run`), `spar config`, `round.json`의 `timeout_seconds` |
| G20 | 리뷰어 이름: `name`, `--reviewers`의 이름, `spar record`·`spar waive`의 이름 (사용자, 호출자) | `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`. 생략하면 preset 이름. 한 목록 안에서 대소문자를 무시하고 겹치지 않음. 라운드 폴더에 spar가 쓰는 파일·폴더의 이름(`round.json`, `packet.md`, `schema.json`, `findings.json`, `ledger.jsonl`, `summary.md`, `waivers.jsonl`, `workspace`)과 그 이름에 `.tmp`를 붙인 이름은 대소문자를 무시하고 쓸 수 없음. 그 밖의 비교(필수 리뷰어 대조, `spar record`, `spar waive`)는 글자 그대로 | 문법 밖, 대소문자만 다른 중복, 예약 이름: 설정 오류(`--reviewers`도 같다). `spar record`의 모르는 이름, `spar waive`의 그 라운드 필수 리뷰어가 아닌 이름: 거부 | 사용자 설정(`spar config`, `spar run`), `--reviewers`, 라운드 폴더 배치, 지적 ID `<이름>-F<n>`, `spar gate`의 필수 리뷰어 대조, `spar record`, `spar waive` |
| G21 | `preset`, `argv`, `binary`, `model`, `fallback_model`, `effort`, `allowed_models`, `allowed_efforts` (사용자) | `preset`은 `claude`, `codex`, `agy`, `native`, `command` 중 하나, 필수. `argv`는 `command`에만, 빈 인자 없는 배열, 인자 안 어디서나 `{prompt}`·`{workspace}`·`{schema}`를 바꾸고 그 밖의 `{…}`는 그대로 둔다. `binary`·`model`·`fallback_model`·`effort`는 `claude`·`codex`·`agy`에만(`agy`는 `effort` 없음). `binary`가 빈 문자열이면 preset 이름을 찾는다. `fallback_model`·`effort`가 빈 문자열이면 끈다(`effort`는 CLI에 넘기지 않는다). `allowed_models`(`claude`·`codex`·`agy`)와 `allowed_efforts`(`claude`·`codex`)는 G25의 값 형식을 지키는 문자열의 빈 아닌 배열이다 | 쓰지 않는 preset에 준 키, `argv` 없는 `command`, 빈 `model`, 빈 `argv` 인자, 빈 허용 목록, 문자열이 아니거나 G25의 값 형식을 어기는 항목, 같은 항목의 `model`과 `allowed_models`(또는 `effort`와 `allowed_efforts`): 설정 오류(그 키의 줄) | `spar config`, 리뷰어 실행(`spar run`) |
| G22 | 프로젝트 `caps`, `angles` (저장소) | `caps`: `plan`, `plan_delta`, `code`, 0 이상의 정수. `angles`: 다섯 각도의 겹치지 않는 부분집합, 하나 이상, 순서는 정해진 순서로 정규화 | 음수, 모르는·겹친 각도, 빈 목록: 설정 오류 | 상한(`caps`), 패킷(`packet.md`)의 각도, 각도 확인, `spar config`, `spar run` |
| G23 | 프로젝트 `rules` (저장소) | 저장소 안의 상대 경로. 심볼릭 링크는 따라간 대상이 저장소 안이어야 한다. 그 경로는 읽을 수 있는 일반 파일이어야 한다 | 빈 값, 절대 경로, `..`로 나감, 심볼릭 링크로 저장소 밖, 없는 파일, 폴더, 읽을 수 없는 파일: `spar config`와 `spar run` 모두 설정 오류(`rules` 줄과 항목을 댄다) | `spar config`, `spar run`, 규칙 문서 이름(G26) |
| G24 | 검사 경로 glob: `check_paths`, `--check-path` (저장소, 호출자) | `/`로 나눈 조각. 조각 전체가 `**`이면 0개 이상의 폴더, 그 밖의 조각은 Go `path.Match` 문법(`*`, `?`, `[…]`, `\` 이스케이프)으로 한 이름 안에서 맞춘다(조각 안의 `**`는 `*` 둘). 끝의 `/`는 무시. 저장소 상대 경로만. `--check-path`는 그 실행의 검사 경로에 더해지고 `round.json`의 `config.check_paths`와 설정 `digest`에 반영된다 | 빈 값, 절대 경로, `..` 조각, `path.Match`가 문법 오류로 보는 조각(닫히지 않은 `[`, 끝의 `\`): 프로젝트 설정은 설정 오류(`check_paths` 줄), `--check-path`는 거부. 둘 다 리뷰어를 띄우기 전 | 설정 확인(`spar config`, `spar run`), 분류의 `in_check_paths`, gate `growth`의 나눔, 패킷(`packet.md`)의 검사 경로 목록 |
| G25 | `--reviewers` 목록, `--model 이름=모델`, `--effort 이름=강도` (호출자) | 쉼표로 나눈 이름. 앞뒤 공백을 떼고 빈 항목은 건너뛴다(clean-review와 같다). 모두 비면(`""`, `","`) `native` 하나. 설정에 있는 이름은 그 설정, 없는 이름은 같은 이름의 preset(`command` 제외). `--model`·`--effort`는 첫 `=`에서 이름과 값을 나눈다. 이름은 이번 실행의 리뷰어 이름과 글자 그대로 맞추고, 값은 바꾸지 않고 CLI 인자 하나로 넘긴다. 모델은 사용자 설정에 `model`이 적히지 않은 `claude`·`codex`·`agy`, 강도는 `effort`가 적히지 않은 `claude`·`codex`만 받는다. 허용 목록(`allowed_models`·`allowed_efforts`)이 있으면 그 항목과 대소문자까지 같은 값만 받는다 | G20의 문법·중복, 설정에도 preset에도 없는 이름: 설정 오류. `--model`·`--effort`에 `=`가 없거나 이름이 빔: 사용법 오류(64). 적힌 키, 그 키가 없는 preset, 이번 실행에 없는 이름, 같은 리뷰어의 같은 키 두 번, 목록 밖의 값, 비었거나 `-`로 시작하거나 공백 문자·제어 문자를 품은 값: 거부(65), 리뷰어를 띄우지 않고 라운드 폴더를 만들지 않으며 이름과 값을 `%q`로 보인다 | `spar config`, `spar run`의 `--reviewers`·`--model`·`--effort` |
| G26 | 문서 인자: `--criteria`, `--rules`, `--subject` (호출자) | 읽을 수 있는 파일. 저장소 밖이어도 된다. 문서 이름: 저장소 안이면 저장소 상대 경로, 밖이면 파일 이름. 연결·위치에서 쓰는 이름은 이 이름, 실제 절대 경로, 파일 이름(G8의 겹침 규칙) | 읽을 수 없음: 거부 | 패킷(`packet.md`), 기준 문서 digest(`criteria_digest`), 연결 대상(G8), 위치 대상(G7) |
| G27 | 리비전: `--base`, `--head`, `--expect-head`, `triage --commit` (호출자) | Git이 커밋으로 푸는 리비전. 옵션으로 읽지 않는다 | `spar run`·`spar triage`는 거부, `spar gate`는 `blocked` 사유 | `spar run`, `spar triage`, `spar gate` |
| G28 | 기록 경로: `run --out`, `extend`의 series, `gate --out` (호출자) | 하나의 규칙: 경로가 어떤 Git 저장소 안이면(검토하는 저장소, 경로를 품은 다른 저장소, 무시된 폴더 안의 중첩 저장소 모두) 그 경로를 품은 가장 안쪽 저장소와 바깥 저장소가 모두 무시하고 그 아래 추적 파일이 없어야 한다. 위에 `.git`이 있는데 Git이 판단하지 못하면 안 된다. `run --out`은 아직 없는 폴더. `gate --out`은 주어진 라운드 폴더 안이 아니고, series의 `extensions.jsonl`이 아니며, 이미 있으면 `review-gate/v1` 파일이어야 한다(그 바이트의 sha256이 `previous_gate_digest`) | 거부. 아무것도 쓰지 않는다 | `spar run`, `spar extend`, `spar gate` |
| G29 | gate 라운드 목록 (호출자) | 하나 이상의 라운드 폴더, 주어진 순서로 읽는다. 주어진 라운드의 series에 있는 dry run이 아닌 `review-round/v1` 라운드는 모두 목록에 있어야 한다. 지적을 기록하기 전에 멈춘 라운드(`finished_at`과 `findings.json`이 모두 없음)는 dry run처럼 빠진다(목록에 주면 dry run처럼 `blocked`) | 읽을 수 없는 라운드, 목록에 없는 같은 series의 라운드: `blocked` 사유(`<series>: round <이름> of the series is not in the gate list`) | `spar gate` |
| G30 | 지적 ID와 처리 옵션: `triage`의 지적, `--of`, `--evidence`, `--to`, `--user-quote`, `--failed-before`, `--passed-after`, 그리고 `waive`의 `--user-quote` (호출자) | 지적 ID는 그 라운드 `findings.json`의 `id`와 글자 그대로 같음. 문장 옵션은 앞뒤 공백을 뗀 뒤 비어 있으면 주지 않은 것 | 모르는 지적, 처리 규칙 위반, 사용자의 말이 없는 `waive`: 거부 | `spar triage`, `spar gate`의 `duplicate` 추적, `spar waive` |
| G31 | `--stage`, `extend --stage`, `extend --by` (호출자) | 계약의 단계 이름. `--by`는 1 이상의 정수 | 사용법 오류 | `spar run`, `spar extend` |
| G32 | 다시 읽는 기록: `round.json`, `findings.json`, `ledger.jsonl`, 라운드의 `waivers.jsonl`, series의 `extensions.jsonl` (spar가 쓴 것) | 같은 spar가 쓴 `review-round/v1` 형식. 이 목표 전에 쓴 기록처럼 `location_unreadable`이 없는 지적은 위치를 읽은 지적이고, `smallest_fix`가 없는 지적은 방향 없음이다. `waivers.jsonl`은 `spar waive`와 `spar gate`만 읽는다. 없는 라운드는 결정이 없는 라운드이고, 있으면 줄마다 `reviewer`·`preset`과 앞뒤 공백을 뗀 뒤 비어 있지 않은 `user_quote`가 있어야 한다 | 읽을 수 없음(파일 없음, JSON 아님, schema 다름): `spar run`·`spar extend`·`spar triage`·`spar record`·`spar waive`는 각자 읽는 기록에서 거부, `spar gate`는 `blocked` 사유. `waivers.jsonl`이 JSON이 아니거나 위 항목이 빠진 줄을 가지면 `spar waive`는 거부, `spar gate`는 `blocked` 사유 | `spar run`의 series 읽기, `spar record`, `spar triage`, `spar extend`, `spar waive`, `spar gate` |
| G33 | 동시 실행 (호출자) | series마다 한 번에 하나의 쓰기: `spar run`(상한 판단부터 기록까지), `spar record`, `spar triage`, `spar extend`, `spar waive`가 같은 series 잠금을 쥐고, 다른 쓰기가 쥐고 있으면 표준 오류에 `spar: waiting for another write of <series> to finish`를 쓰고 기다린다. 잠금을 쥔 뒤 기록을 다시 읽는다. `spar gate`는 잠그지 않는다 | 해당 없음(기다린다). `spar gate`가 쓰는 중인 라운드를 읽어 해석하지 못하면 G32의 `blocked` 사유 | `spar run`, `spar record`, `spar triage`, `spar extend`, `spar waive` |

## 3. 위치·연결 문법

G7과 G8이 같은 문법을 쓴다. 다른 것은 떼는 접두와 대상뿐이다.

```text
참조   = 앞뒤 공백을 뗀 글
       = 접두? 경로 (":" 줄 (대시 줄)?)? (끝 나머지)?
접두   = G7: "head/" | "base/" | "./"      G8: "head/" | "./"     (앞에 있는 하나만, 한 번)
줄     = ASCII 숫자 하나 이상
대시   = "-" | "–"
끝     = 공백 | "," | ";"                    (줄 뒤에만. 줄 뒤의 그 밖의 문자는 문법 밖)
나머지 = 아무 글
```

- 공백은 유니코드 공백이다.
- 줄이 있는 참조의 경로는, 처음으로 `:줄` 또는 `:줄대시줄` 뒤에 끝이나 글의 끝이 오는 곳 앞까지다. 경로에 공백이 있어도 되고, 나머지에 다른 위치가 와도 경로는 그대로다.
- 그런 곳이 없으면 줄이 없는 참조다. 참조 전체가 대상이면 그것이 경로이고, 아니면 첫 단어가 경로다.
- 접두는 경로 앞에서 한 번만 뗀다. `head/base/x`는 G7에서 `base/x`, `head/head/x`는 `head/x`다.
- 경로를 정한 뒤 줄을 본다. 줄이 0이거나 범위의 뒤 줄이 앞 줄보다 작으면 G7에서는 읽을 수 없는 위치, G8에서는 확인되지 않은 연결이다. 경로를 읽는 방법은 줄 값과 관계없다.
- 사본의 파일과 폴더는 그 커밋 tree의 blob 항목(일반·실행 파일과 심볼릭 링크)과 그 위 폴더다. submodule(gitlink)은 사본에 없어서 위치 대상도 `contract` 연결 대상도 아니다.
- 위치 대상의 순서: 접두 없음은 head 사본, base 사본, 라운드 문서 순서이고, `head/`·`./`는 head 사본, 라운드 문서 순서이며, `base/`는 base 사본만 본다. 라운드 문서는 G26의 이름과 실제 절대 경로로 맞고, 파일 이름은 그 파일 이름의 라운드 문서가 하나일 때만 맞는다.

## 4. 읽을 수 없는 위치의 분류

위치가 G7의 문법 밖이거나 대상이 없으면 그 지적의 위치를 읽을 수 없다. 리뷰어 결과는 버리지 않는다. 그 리뷰어의 상태는 다른 확인대로 정해지고(위치 때문에 `invalid_output`이 되지 않는다), 그 지적만 다음처럼 다룬다.

- `path`는 빈 값, `in_check_paths`는 `false`, `location_unreadable`은 `true`이고, 요약의 위치 칸에 `(unreadable location)`이 붙는다. `class_reason`은 `unreadable location (<이유>)`로 시작한다.
- 분류는 [검토 라운드](review-round.md) 6절의 분류 규칙 일곱 개 가운데 3(검사 경로의 `requirement-question`·`hardening`)과 6(연결이 확인되지 않은 `defect` 제안의 `note`)을 쓰지 않고, 1·2는 검사 경로 조건 없이 쓴다. 위에서 처음 맞는 것:
  1. `check_impact`가 `false_pass`나 `meaning_inverted`이면 `defect`
  2. `check_impact`가 `contract_violation`이고 연결이 확인되면 `defect`
  3. `proposed_class`가 `requirement-question`이면 `requirement-question`
  4. `introduced_by_change`와 `regression`이 모두 `yes`이면 `defect`
  5. 그 밖에는 `proposed_class`. 연결이 확인되지 않은 `defect` 제안도 `defect`다
- 자동 보류 원장 항목을 쓰지 않는다. 리뷰어가 `hardening`을 제안했으면 `hardening`으로 남되 처리가 없는 상태다. gate를 막지 않고 `auto_deferred`에 세지 않으며, 요약의 처리 칸은 `-`다.
- 실패의 방향: 잘못 읽힌 위치는 사람이 처리할 `defect`를 더 남길 수는 있지만, `defect`를 `hardening`이나 `note`로 숨기지 않는다. 위치가 어떻게 읽혔더라도(검사 경로 안이든 밖이든) `defect`가 됐을 지적은 `defect`다.

## 5. 거절 문구

spar는 리뷰어의 표준 오류와 출력의 오류 메시지에서 아래 문구를 대소문자 없이 찾는다. 이 목록 밖의 메시지는 거절이 아니다(G13).

| 종류 | 문구 | 기록 |
| --- | --- | --- |
| 사용량·요청 한도 | `RESOURCE_EXHAUSTED`, `code 429`, `status 429`(`code`·`status`와 `429` 사이에 기호 넷까지), `429 Too Many Requests`, `too many requests`, `rate limit`, `rate_limit`, `rate-limit`, `ratelimit`, `quota reached`, `quota exceeded`, `quota exhausted`, `insufficient_quota`, `hit your limit`, `hit your usage limit`, `usage limit reached` | `unavailable`, 이유 `quota or rate limit: …`. `fallback_model`이 있으면 대체 모델로 한 번 더 |
| 인증 | `UNAUTHENTICATED`, `code 401`, `status 401`(`code`·`status`와 `401` 사이에 기호 넷까지), `401 Unauthorized`, `not logged in`, `authentication required`, `authentication failed`, `login again`, `log in again`, `sign in again`, `codex login`, `agy login`, `claude login`, `run /login`, `oauth token expired`, `oauth token has expired`, `invalid api key`, `incorrect api key` | `unavailable`, 이유 `authentication: …`. 대체 모델을 쓰지 않는다 |
| 재설정 | `reset`·`resets` 뒤에 `in`·`at`이 오거나 오지 않는 것(`resets in`, `reset at` 등), `try again in`, `try again at`. 그 뒤 공백과 숫자가 든 낱말부터 마침표나 줄 끝까지 | 시도와 리뷰어의 `resets` |

## 6. 대표 형식

받는 형식은 위 표와 문법이 빠짐없이 정한다. 아래는 그 성질을 보이는 대표 예이고, 각 행의 소비처가 모두 같은 결과를 낸다. Go 테스트와 검사가 같은 예를 `internal/spar/testdata/input-forms.json`에서 읽는다.

### G7 위치 (검사 경로 `checks/**`)

| 위치 | 결과 |
| --- | --- |
| `head/app/main.go:10` | `app/main.go`, 검사 경로 아님 |
| `head/checks/t.py:10-20`, `head/checks/t.py:10–20` | `checks/t.py`, 검사 경로 |
| `head/checks/my test.py:3` | `checks/my test.py`(공백 경로) |
| `head/checks/t.py:10 and head/app/main.go:20` | `checks/t.py`(뒤의 위치는 나머지) |
| `head/checks/t.py:10, head/app/main.go:3`, `head/checks/t.py:10; see also` | `checks/t.py` |
| `head/checks/my test.py` (줄 없음, 그 파일이 head에 있음) | `checks/my test.py` |
| `head/checks/t.py (the loop)` (줄 없음) | 전체는 대상이 아님, 첫 단어 `checks/t.py` |
| `head/base/checks/t.py:2` (저장소의 `base` 폴더) | `base/checks/t.py`, 검사 경로 아님 |
| `head/head/x.go:1` (저장소의 `head` 폴더) | `head/x.go` |
| `base/old.go:4` (base에만 있음) | `old.go` |
| `./app/main.go:3`, `app/main.go:3` | `app/main.go` |
| `head/checks/` (폴더) | `checks`, 검사 경로 |
| `head/` | 저장소 최상위, 빈 경로, 검사 경로 아님 |
| `PLAN.md:12`, `head/docs/specs/x/PLAN.md:12` (검토 대상 문서 `docs/specs/x/PLAN.md`) | `docs/specs/x/PLAN.md` |
| `head/checks/t.py:10:5` (열 번호) | 경로 `checks/t.py:10`이 대상이 아님: 읽을 수 없는 위치 |
| `head/checks/t.py:10.` (줄 뒤 마침표) | 문법 밖: 읽을 수 없는 위치 |
| `head/checks/t.py:0` | 줄 0: 읽을 수 없는 위치 |
| `head/checks/t.py:20-10` | 뒤 줄 < 앞 줄: 읽을 수 없는 위치 |
| `In head/checks/t.py:10` (앞에 글) | 경로 `In head/checks/t.py`가 대상이 아님: 읽을 수 없는 위치 |
| `head/checks/missing.py:3` | 대상 없음: 읽을 수 없는 위치 |
| `""` (빈 위치) | 대상 없음: 읽을 수 없는 위치 |

### 읽을 수 없는 위치의 분류 (위치 `head/checks/t.py:10:5`)

| 리뷰어가 낸 값 | 위치가 `checks/t.py`로 읽혔다면 | 읽을 수 없을 때 |
| --- | --- | --- |
| `proposed_class: defect`, `check_impact: none` | `hardening`, 자동 보류 | `defect` |
| `proposed_class: defect`, `check_impact: false_pass` | `defect` | `defect` |
| `proposed_class: note`, `check_impact: meaning_inverted` | `defect` | `defect` |
| `proposed_class: note`, `check_impact: contract_violation`, 확인된 `rule` 연결 | `defect` | `defect` |
| `proposed_class: note`, `check_impact: contract_violation`, 확인되지 않은 연결 | `hardening`, 자동 보류 | `note` |
| `proposed_class: requirement-question`, `check_impact: none` | `hardening`, 자동 보류 | `requirement-question` |
| `proposed_class: note`, `introduced_by_change: yes`, `regression: yes` | `hardening`, 자동 보류 | `defect` |
| `proposed_class: hardening`, `check_impact: none` | `hardening`, 자동 보류 | `hardening`, 처리 없음(자동 보류 아님) |
| `proposed_class: note`, `check_impact: none` | `hardening`, 자동 보류 | `note` |

### G8 연결 (규칙 문서: 두 줄짜리 `AGENTS.md`, 다섯 줄짜리 `rules/API rules.md`, 저장소 최상위 `base/api.md`; head 파일 `contracts/x.md`)

| 연결 | 결과 |
| --- | --- |
| `rule` `AGENTS.md:2`, `head/AGENTS.md:1-2`, `./AGENTS.md:1–2` | 확인됨 |
| `rule` `AGENTS.md:1-9999`, `AGENTS.md:2-3` | 범위의 끝이 문서에 없음: 확인되지 않음 |
| `rule` `AGENTS.md:2-1`, `AGENTS.md:0`, `AGENTS.md:3` | 확인되지 않음 |
| `rule` `rules/API rules.md:4`, `rules/API rules.md` | 확인됨(공백 경로, 줄 없는 전체 이름) |
| `rule` `AGENTS.md Checks and review` | 첫 단어 `AGENTS.md`: 확인됨 |
| `rule` `AGENTS.md:2, AGENTS.md:1` | 확인됨(쉼표 끝) |
| `contract` `base/api.md:3` | 저장소 `base/` 폴더의 규칙 문서: 확인됨 |
| `contract` `contracts/x.md:4` | head 커밋의 일반 파일: 확인됨 |
| `contract` 심볼릭 링크, 폴더, submodule, base에만 있는 파일 | 확인되지 않음 |
| `rule` `AGENTS.md:1` (규칙 문서 `a/AGENTS.md`, `b/AGENTS.md`) | 파일 이름이 겹침: 확인되지 않음 |
| `contract` `review.md:5` (저장소 밖 열 줄짜리 규칙 문서 `review.md`, head에 두 줄짜리 `review.md`) | 규칙 문서가 먼저: 확인됨 |
