# 계약: review-gate/v1

`spar gate`는 라운드 목록과 기대 head로 검토가 끝났는지 판단하고 `review-gate/v1` 파일을 쓴다. 이 파일은 검토 기록이며 완료 근거가 아니다. 완료 규칙은 그 규칙을 가진 쪽(예: Seal의 `ha done`)이 정한다. 아래 예시는 합성이다.

## 1. 명령

```bash
spar gate docs/reviews/example/02-candidate docs/reviews/example/03-fixes \
  --expect-head 3333333 --out docs/reviews/example/gate.json
```

- 라운드는 주어진 순서로 읽는다. `--out`은 저장소 안이면 Git이 무시하는 경로여야 한다.
- 인자가 맞으면 상태와 관계없이 파일을 쓴다. 읽을 수 없는 라운드, 해석할 수 없는 기대 head도 `blocked` 사유로 기록한다.
- 종료 코드: `complete` 0, `degraded` 3, `blocked` 4, 사용법 오류 64, 기록 경로 거부 65, 내부 오류 70.

## 2. 상태

`blocked`가 `degraded`보다, `degraded`가 `complete`보다 앞선다.

`blocked` 사유:

- 상한 초과: 라운드의 series마다, 디스크에 있는 라운드와 주어진 라운드 가운데 `counted`인 것을 단계 묶음별로 세어 마지막 라운드 설정의 상한과 `extensions.jsonl` 연장의 합을 넘음
- 처리되지 않은 지적: 원장에 처리가 없는 `defect`·`requirement-question`. `duplicate`는 가리킨 지적을 따라가 닫는 처리에 닿을 때만 닫히고, 열린 지적에 닿거나 서로를 가리키면 열려 있다
- `fix`로 처리한 지적의 수정 커밋이 기대 head에 포함되지 않음
- 코드 라운드 사슬 끊김: 목록의 fixes 라운드 base가 바로 앞 code 라운드의 head가 아니거나, 앞에 code 라운드가 없음
- 기대 head 미검토: 마지막 code 라운드(없으면 마지막 라운드)의 head가 기대 head가 아님
- dry run 라운드, 읽을 수 없는 라운드, `findings.json`이나 `ledger.jsonl`이 없는 라운드, 저장소에 없는 커밋

`degraded` 사유:

- 필수 리뷰어가 완료하지 않음. 필수 리뷰어는 라운드가 필수로 실행한 리뷰어와 그 설정이 필수로 정한 리뷰어(`config.configured_required`)다. `--reviewers`로 뺀 설정의 필수 리뷰어도 포함된다. 같은 단계·base·head·패킷의 다른 라운드(재실행)에서 같은 이름·preset의 필수 리뷰어로 완료했으면 완료로 본다. 이름이 같아도 preset이 다르면 그 필수 리뷰어는 실행되지 않은 것이다.
- 필수 리뷰어가 없는 라운드(예: `command` 리뷰어만 있는 라운드)

`complete`는 사유가 하나도 없을 때뿐이다.

## 3. 파일

```json
{
  "schema": "review-gate/v1",
  "spar_version": "0.1.6",
  "state": "complete",
  "reasons": [],
  "expect_head": "3333333…",
  "last_reviewed_head": "3333333…",
  "previous_gate_digest": null,
  "rounds": [{
    "path": "docs/reviews/example/02-candidate", "stage": "candidate", "sequence": 2,
    "base": "1111111…", "head": "2222222…", "criteria_digest": "…", "packet_digest": "…", "config_digest": "…",
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
| `rounds[]` | 라운드의 `criteria_digest`, 패킷 digest(`packet_digest`), 설정 digest(`config_digest`), 요청한 리뷰어 이름과 필수로 인정된 리뷰어 수(`required_reviewers`), 그중 완료한 수 |
| `open` | 열린 `defect`와 `requirement-question` 수 |
| `open_findings` | 열린 지적의 라운드와 ID |
| `auto_deferred` | 마지막 처리가 자동 보류인 `hardening` 수 |
| `growth.added_lines` | 목록의 fixes 라운드마다 `git diff --numstat <base> <head>`의 더한 줄을 그 라운드 설정의 검사 경로(`check`)와 그 밖(`other`)으로 나눈 합. 같은 base·head는 한 번만 센다 |
| `growth.over_hardening` | `check`가 `other`보다 크면 `true` |
| `growth.rounds[]` | fixes 라운드별 `check`·`other` |

`growth`는 검토가 수정을 검사 코드 쪽으로 부풀렸는지 읽게 하는 기록이다. `over_hardening`은 판단을 대신하지 않고 상태를 바꾸지 않는다.
