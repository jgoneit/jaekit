# 계약: review-gate/v1

`spar gate`는 라운드 목록과 기대 head로 검토가 끝났는지 판단하고 `review-gate/v1` 파일을 쓴다. 이 파일은 검토 기록이며 완료 근거가 아니다. 완료 규칙은 그 규칙을 가진 쪽(예: Seal의 `ha done`)이 정한다. 아래 예시는 합성이다.

## 1. 명령

```bash
spar gate docs/reviews/example/02-candidate docs/reviews/example/03-fixes \
  --expect-head 3333333 --out docs/reviews/example/gate.json
```

- 라운드는 주어진 순서로 읽는다(G29). 주어진 라운드의 series에 있는 dry run이 아닌 라운드는 모두 목록에 있어야 한다. 지적을 기록하기 전에 멈춘 라운드(`finished_at`과 `findings.json`이 모두 없음)는 dry run처럼 빠진다.
- `--out`은 [입력 형식](spar-input.md)의 기록 경로 규칙(G28)을 따른다. 그 밖에 주어진 라운드 폴더 안이나 series의 `extensions.jsonl`이면 거부하고, 이미 있는 파일은 `review-gate/v1` 파일일 때만 덮는다. 거부하면 아무것도 쓰지 않는다.
- 인자가 맞으면 상태와 관계없이 파일을 쓴다. 읽을 수 없는 라운드, 해석할 수 없는 기대 head도 `blocked` 사유로 기록한다.
- 종료 코드: `complete` 0, `degraded` 3, `blocked` 4, 사용법 오류 64, 기록 경로 거부 65, 내부 오류 70.

## 2. 상태

`blocked`가 `degraded`보다, `degraded`가 `complete`보다 앞선다.

`blocked` 사유:

- 상한 초과: 라운드의 series마다, 디스크에 있는 라운드와 주어진 라운드 가운데 `counted`인 것을 단계 묶음별로 세어 그 series에서 `sequence`가 가장 큰 라운드 설정의 상한과 `extensions.jsonl` 연장의 합을 넘음. 주어진 라운드가 그보다 앞선 라운드여도 같다
- 처리되지 않은 지적: 원장에 처리가 없는 `defect`·`requirement-question`. `duplicate`는 가리킨 지적을 따라가 닫는 처리에 닿을 때만 닫히고, 열린 지적에 닿거나 서로를 가리키면 열려 있다
- `fix`로 처리한 지적의 수정 커밋이 기대 head에 포함되지 않음. 분류와 관계없이(`hardening`·`note` 포함) 모든 `fix`를 확인한다
- 코드 라운드 사슬 끊김: 목록의 fixes 라운드 base가 바로 앞 code 라운드의 head가 아니거나, 앞에 code 라운드가 없음. 바로 앞 code 라운드와 단계·base·head·패킷 digest가 모두 같은 라운드(같은 패킷의 재실행)만 이 확인에서 빠진다. 패킷이 다르면 base·head가 같아도 새 fixes 라운드다
- 기대 head 미검토: 마지막 code 라운드(없으면 마지막 라운드)의 head가 기대 head가 아님
- dry run 라운드, 읽을 수 없는 라운드, `findings.json`이나 `ledger.jsonl`이 없는 라운드, `waivers.jsonl`을 읽을 수 없는 라운드, 저장소에 없는 커밋
- 목록에 없는 같은 series의 라운드(G29): `<series>: round <이름> of the series is not in the gate list`. 앞선 라운드를 빼고 gate를 만들어 열린 지적을 지나치지 않게 한다

`degraded` 사유:

- 필수 리뷰어가 완료하지 않음. 필수 리뷰어는 라운드가 필수로 실행한 리뷰어와 그 설정이 필수로 정한 리뷰어(`config.configured_required`)다. `--reviewers`로 뺀 설정의 필수 리뷰어도 포함된다. 같은 단계·base·head·패킷의 다른 라운드(재실행)에서 같은 이름·preset의 필수 리뷰어로 완료했으면 완료로 본다. 이름이 같아도 preset이 다르면 그 필수 리뷰어는 실행되지 않은 것이다. 그 라운드의 `waivers.jsonl`에 사용자의 결정(`spar waive`, [검토 라운드](review-round.md) 10절)이 있는 리뷰어는 이 사유가 되지 않고 그 라운드의 `waived_reviewers`에 보인다.
- 필수 리뷰어가 없는 라운드(예: `command` 리뷰어만 있는 라운드)
- 결정이 적용된 리뷰어가 있는데 완료한 필수 리뷰어가 하나도 없는 라운드: `<라운드>: no required reviewer completed this round`. 결정은 독립 검토가 없는 라운드를 `complete`로 만들지 않는다. 완료한 필수 리뷰어는 `completed_required`로 세는 리뷰어이며, 같은 단계·base·head·패킷의 재실행에서 완료한 리뷰어를 포함한다.

결정은 사용자의 말을 인용해서만 기록되고, 그 라운드 폴더의 `waivers.jsonl`에서만 읽어 그 라운드에만 적용한다. 다른 라운드의 결정은 같은 패킷의 재실행이라도 적용하지 않는다. 결정은 이름과 preset이 같은 필수 리뷰어에 적용되고, 결정이 있어도 그 리뷰어가 완료했으면(같은 패킷의 재실행 포함) 완료로 세며 `waived_reviewers`에 넣지 않는다. `waivers.jsonl`이 없는 라운드(이 기능 전에 기록한 라운드 포함)와, 이 기능 전에 이름이 `waivers.jsonl`(대소문자 무시)인 리뷰어를 실행해 그 폴더가 그 자리에 있는 라운드는 결정이 없는 라운드이고, 그 gate의 상태·사유·필드는 결정 기능이 없을 때와 같다.

`complete`는 사유가 하나도 없을 때뿐이다.

## 3. 파일

```json
{
  "schema": "review-gate/v1",
  "spar_version": "0.1.8",
  "state": "complete",
  "reasons": [],
  "expect_head": "3333333…",
  "last_reviewed_head": "3333333…",
  "previous_gate_digest": null,
  "rounds": [{
    "path": "docs/reviews/example/02-candidate", "stage": "candidate", "sequence": 2,
    "base": "1111111…", "head": "2222222…", "criteria_digest": "…",
    "criteria": [{"name": "docs/specs/example/SPEC.md", "sha256": "…"}], "packet_digest": "…", "config_digest": "…",
    "requested_reviewers": ["claude", "codex", "agy"], "required_reviewers": 3, "completed_required": 3,
    "dry_run": false, "counted": true
  }],
  "open": {"defect": 0, "requirement-question": 0},
  "open_findings": [],
  "auto_deferred": 2,
  "growth": {"added_lines": {"check": 40, "other": 12}, "over_hardening": true, "rounds": []},
  "written_at": "2026-01-01T00:00:00Z"
}
```

| 필드 | 뜻 |
| --- | --- |
| `state`, `reasons` | 상태와 사유. `blocked` 사유가 먼저 온다 |
| `expect_head` | 기대 head의 커밋 ID |
| `last_reviewed_head` | 마지막으로 검토된 head |
| `previous_gate_digest` | 같은 `--out` 경로에 있던 이전 gate 파일 바이트의 sha256, 없으면 `null` |
| `rounds[]` | 라운드의 `criteria_digest`, 패킷 digest(`packet_digest`), 설정 digest(`config_digest`), 요청한 리뷰어 이름과 필수로 인정된 리뷰어 수(`required_reviewers`), 그중 완료한 수(`completed_required`, 같은 단계·base·head·패킷의 재실행에서 완료한 것 포함) |
| `rounds[].criteria` | 라운드가 받은 기준 문서마다 `round.json`의 순서대로 `name`(저장소 안이면 저장소 상대 경로, 밖이면 파일 이름; 심볼릭 링크를 푼 이름)과 내용의 `sha256`. 원본 경로는 넣지 않는다. `ha`의 `run-rules/5`가 목표 문서와의 대응을 이것으로 판단한다([run-record.md](run-record.md) §4.8) |
| `rounds[].waived_reviewers` | 결정이 적용된 필수 리뷰어마다 `name`, `preset`, `status`(`round.json`의 상태, 라운드가 그 이름과 preset의 리뷰어를 실행하지 않았으면 `not_run`), `reason`(`round.json`의 원인, 없으면 빠짐), `user_quote`, `recorded_at`(결정의 기록 시각). `completed_required`에 세지 않는다. 적용된 결정이 없는 라운드에는 이 필드가 없다 |
| `open` | 열린 `defect`와 `requirement-question` 수 |
| `open_findings` | 열린 지적의 라운드와 ID |
| `auto_deferred` | 마지막 처리가 자동 보류인 `hardening` 수. 위치를 읽을 수 없는 `hardening`은 원장 항목이 없어 세지 않는다 |
| `growth.added_lines` | 목록의 fixes 라운드마다 `git diff --numstat -M <base> <head>`의 더한 줄(이름을 바꾼 파일은 새 경로에서 더한 줄만)을 그 라운드 설정의 검사 경로(`check`)와 그 밖(`other`)으로 나눈 합. 같은 base·head는 한 번만 센다 |
| `growth.over_hardening` | `check`가 `other`보다 크면 `true` |
| `growth.rounds[]` | fixes 라운드별 `check`·`other` |

`growth`는 검토가 수정을 검사 코드 쪽으로 부풀렸는지 읽게 하는 기록이다. `over_hardening`은 판단을 대신하지 않고 상태를 바꾸지 않는다.
