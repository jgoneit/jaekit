# Seal Core scenarios

Each directory is one scenario. `cmd/ha/scenario_test.go` builds a git repository from `_base/` plus the scenario's `files/`, commits it as the base, and runs `steps` line by line. Shared step sequences are in `_steps/`. `finding-input <path> <json>` writes a finding request in which `$COMPLETION` names the latest completion record. Verification commands are small shell scripts run as argv (`sh tests/x.sh`), never as shell strings.

| Scenario | Source | Expected |
| --- | --- | --- |
| `unverified-criterion` | EVALUATION §3 조건 미검증 | `incomplete`, `AC-2:criterion_missing` |
| `defect-caught` | EVALUATION §3 검사가 잡는 결함 | `incomplete`, `AC-1:criterion_failed` |
| `flaky-rerun` | EVALUATION §3 Flaky 재실행 | `incomplete`, `AC-1:criterion_flaky` |
| `weakened-command` | EVALUATION §3 검증 명령 약화 | not detected; listed in `changes.command_changed` |
| `maintain-broken` | EVALUATION §3 유지 조건 파괴 | `incomplete`, `AC-2:criterion_failed` |
| `vacuous-check` | EVALUATION §3 공허한 검사 | `incomplete`, `AC-1:baseline_unexpected_pass` |
| `stale-result` | EVALUATION §3 오래된 결과 | `incomplete`, stale criteria |
| `dirty-check` | 실행 전 작업 상태 확인 | exit 65 before process; no new records or usage |
| `dirty-after-verify` | EVALUATION §3 검증 뒤 미커밋 변경 | `incomplete`, `worktree_dirty` |
| `goal-dir-check-changed` | EVALUATION §3 Goal 디렉토리 안 검사 변경 | `incomplete`, stale criteria |
| `error-limit` | EVALUATION §3 오류 반복 숨김 | `needs_user`, `AC-2:error_limit` |
| `confirm-reuse` | EVALUATION §3 확인 재사용 | `needs_user`, `out_of_scope`, invalid confirmation listed |
| `output-leak` | EVALUATION §3 검증 출력 유출 | secret absent from `runs.jsonl` and ha output; visible in `ha log` |
| `out-of-scope` | EVALUATION §3 범위 밖 변경 | `needs_user`, `out_of_scope` |
| `test-weakened` | EVALUATION §3 테스트 약화 | `needs_user`, `test_definition_changed`; cleared by `confirm tests:` |
| `criterion-id-missing` | EVALUATION §3 조건 ID 누락 | `incomplete`, `lint_error`; checks refused |
| `spec-sneak-edit` | EVALUATION §3 SPEC 몰래 수정 | stale, then `needs_user`, `spec_changed` |
| `aux-doc-sneak-edit` | EVALUATION §3 보조 문서 몰래 수정 | stale, then `needs_user`, `spec_changed` |
| `false-progress` | EVALUATION §3 거짓 진행 기록 | `incomplete` despite PROGRESS.md saying done |
| `failed-record-deleted` | EVALUATION §3 실패 기록 삭제 | `needs_user`, `record_integrity`; appends refused |
| `baseline-error-retry` | EVALUATION §3.1 | `complete` |
| `baseline-safety-refusal` | baseline preparation safety | missing overlay refused before execution under `/2`; no new check record or usage |
| `maintain-error-retry` | EVALUATION §3.1 | `complete`, one error shown in attempts |
| `manual-confirmed` | EVALUATION §3.1 | `complete` after a quoted confirmation |
| `bundle-docs-only` | EVALUATION §3.1 | records stay fresh |
| `time-after-done` | EVALUATION §3.1 | still `complete`; deterministic output |
| `findings-contract` | findings.md | actual JSON and human findings queries preserve the historical completion and deterministic state |
| `findings-retry-equivalent` | findings.md 이력·충돌·저장 실패 | a retry that only writes out or omits an empty optional list returns the original receipt; different content stays refused |
| `findings-required-lists` | findings.md 입력과 조회 | omitted, null or mistyped required lists are refused with exit 64 and no record; empty arrays stay arrays |
| `findings-resolution-refs` | findings.md 입력과 조회 | current `resolution_refs` only on `resolved`; earlier references stay in history |
| `reopen-after-done` | EVALUATION §3.1 | completion record voided, new budget window |
| `complete-happy-path` | ROADMAP P1 | `complete` with a completion record; tree untouched |
| `budget-runs` | INV-11 | under `run-rules/1`: `budget_exhausted`; quoted reopen opens a new window |
| `budget-only-complete` | run-rules-2 AC-1 | `run-rules/2`: only `budget_exceeded` left → `complete` and a completion record; the budget still shows as exceeded |
| `budget-with-other-reasons` | run-rules-2 AC-2 | `budget_exhausted` with both reasons and no completion; `complete` once the other reason is confirmed; `blocked` wins |
| `budget-with-failure` | run-rules-2 AC-2 | a failed condition stays; `budget_exhausted`, no completion |
| `sibling-bundle-only` | run-rules-2 AC-4 | another goal committing only its bundle files keeps results fresh and in scope |
| `sibling-bundle-dirty` | run-rules-2 AC-5 | uncommitted bundle files of another goal leave the tree clean, also in new check records |
| `sibling-bundle-after-done` | run-rules-2 AC-6 | the completion record stays valid |
| `sibling-code-changes` | run-rules-2 AC-7 | another goal's SPEC.md, linked documents, `checks/`, a directory without SPEC.md, and `examples/` stay code |
| `sibling-declared` | run-rules-2 AC-8 | a PLAN declaration makes other goals' bundle files code; the current PLAN decides |
| `sibling-declaration-invalid` | run-rules-2 AC-8 | an unreadable declaration is `other_bundles_invalid` |
| `sibling-declaration-label-invalid` | run-rules-2 AC-8 | a malformed declaration label is `other_bundles_invalid`; only the exact key enables the input declaration |
| `rules-v2-start` | run-rules-2 AC-9 | an existing goal keeps `run-rules/2` |
| `rules-v3-start` | structured baseline classification | a new start records `/3`; an unsupported script failure stays unknown |
| `budget-v3-change` | same-window budget change | total limit changes preserve usage; stale revisions are refused; reopen inherits the latest limit |
| `rules-v1-kept` | run-rules-2 AC-10 | historical `run-rules/1` budget semantics remain; a future dirty check is refused |
| `blocked` | bundle.md §5 | `blocked` until unblock |
| `not-started` | run-record.md §4.2 | `incomplete`, `not_started`; checks refused |
| `start-budget-minimum` | run-record.md §1.1 | estimate and start report shortfall without a record; an equal cap is accepted |
| `nested-criteria` | goal-docs.md | explicit nested format passes lint and the full verification/completion path |
| `start-refused` | run-record.md §1 | second start refused |
| `start-lint-refused` | run-record.md §1 | start refused on lint problems and without a request |
| `rules-unsupported` | INV-21 | exit 66 |
| `rules-unsupported-record` | run-rules-2 AC-12 | exit 66 for check, baseline, note, and done; nothing recorded |
| `rules-unsupported-corrupt` | run-rules-2 AC-12 | exit 66 for status, done, check, baseline, and note despite later record corruption; nothing recorded |
| `ha-version-change` | run-record.md §4.4 | listed in `changes.ha_version_changed` |
| `kind-changed` | T18 | listed in `changes.kind_changed` |
| `task-check` | bundle.md §3 | task command recorded with a task target |
| `lint-ok` | bundle.md §8 | `ha lint` passes on a valid bundle |
| `output-path-checkout` | run-record.md §2.2 | `output_path` is `.git/ha/runs/<goal-key>/<seq>.log`; no local path anywhere |
| `output-path-worktree` | run-record.md §2.2 | same `output_path` in a linked worktree; log lists in status and done carry it |
| `worktree-log` | run-record.md §1 | `ha log` and `--tail` work in a linked worktree |
| `legacy-output-path` | run-record.md §2.2 | an older absolute `output_path` stays as written; `complete` after it |

EVALUATION §3 rows whose P1 expectation is "탐지 기대 없음" other than the command change (weak checks, special-casing, unrelated changes inside scope, tests that change sources) have no scenario: P1 does not claim to detect them.

Budget scenarios explicitly reconstruct a small-cap historical start after admission, so they continue to test stored `/1` and `/2` budget semantics independently of the new minimum for future starts.

## 구조화 근거와 예산 변경

- `rules-v3-start`: 새 시작의 /3 선택과 미지원 스크립트 실패의 unknown 보존.
- `budget-v3-change`: 같은 창의 횟수 상한 변경, 오래된 개정 거부, 재개 시 최신 상한 계승.

기존 `_steps/start`를 쓰는 사례는 /2 시작 기록을 명시해 구규칙 호환성을 확인한다. /3 사례는 새 시작을 직접 호출한다. 구조화 결과의 실제 프로세스·기록·완료 연결과 오류 경계는 `cmd/ha/baseline_budget_test.go`에서도 검증한다.
