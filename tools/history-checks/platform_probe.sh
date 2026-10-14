#!/bin/sh
# Drive one decision-history flow with the given ha binary in a fresh
# repository and print a normalized transcript: one "STEP name exit" line per
# command followed by its output. check.py compares the transcripts of every
# platform it can run. Usage: platform_probe.sh <ha> <work directory>
set -u
HA="$1"
WORK="$2"
rm -rf "$WORK"
mkdir -p "$WORK/repo/docs/specs/probe" "$WORK/repo/src"
cd "$WORK/repo" || exit 70
export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1 LC_ALL=C
export GIT_AUTHOR_NAME=synthetic GIT_COMMITTER_NAME=synthetic
export GIT_AUTHOR_EMAIL=synthetic@example.invalid GIT_COMMITTER_EMAIL=synthetic@example.invalid
G=docs/specs/probe
printf 'stable\n' > src/value
printf '# Probe\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## Acceptance Criteria\n\n- **AC-1** The probe stays runnable.\n\n## Open Decisions\n\nNone.\n' > $G/SPEC.md
printf '# PLAN\n\n## 조건표\n\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n| AC-1 | maintain | `true` | — | T001 |\n\n## 범위\n\n- 바꿀 수 있는 경로: `src/**`\n- 테스트 경로: (기본값)\n- 유지할 동작: probe\n\n## Task\n\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n| T001 | Probe | AC-1 | — | — | 예 | — |\n\n## 예산\n\n- 검증 실행: 10\n- 경과 시간: 4시간\n- 비용: 미관측\n' > $G/PLAN.md
printf '# REVIEW\n\n## 결론\n\nready\n' > $G/REVIEW.md
printf '# PROGRESS\n\n## Task 상태\n\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | probe |\n' > $G/PROGRESS.md
git init -q -b main . && git add . && git commit -qm base || exit 70
step() {
  name="$1"
  shift
  out=$("$HA" "$@" 2>&1)
  code=$?
  printf 'STEP %s %s\n%s\n' "$name" "$code" "$out"
}
step query-before-start history $G --format json
step not-started history $G add --kind 관측 --summary early
"$HA" start $G --request probe --skill-name probe --skill-version 1 >/dev/null 2>&1 || exit 70
"$HA" check $G AC-1 >/dev/null 2>&1 || exit 70
step absent history $G --format json
step add-cause history $G add --kind 원인 --status 가설 --summary "PDF first item" --target AC-1 --ref run:2 --request-id cause-1
step add-observation history $G add --kind 관측 --summary "empty input reproduces" --target AC-1 --checked-ref file:src/value --request-id seen-1
step add-decision history $G add --kind 결정 --summary "return zero" --target T001 --builds-on D1 --builds-on D2 --request-id decide-1
step replay history $G add --kind 원인 --status 가설 --summary "PDF first item" --target AC-1 --ref run:2 --request-id cause-1
step request-conflict history $G add --kind 관측 --summary other --request-id cause-1
step bad-status history $G add --kind 결정 --status 확정 --summary x
step bad-relation history $G add --kind 결정 --summary x --builds-on D9
step bad-target history $G add --kind 결정 --summary x --target AC-9
step reject history $G add --kind 원인 --status 기각 --summary "OCR path" --target T001 --supersedes D1 --request-id reject-1
step stale history $G add --kind 원인 --summary again --supersedes D1
step current history $G current --format json
step current-pdf history $G current --keyword PDF --format json
step show history $G show D3 --format json
step page history $G list --limit 2 --format json
step status status $G --format json
H=$(ls -d .git/ha/history/*)
cp "$H/events.jsonl" "$WORK/events.saved"
sed '$d' "$WORK/events.saved" > "$H/events.jsonl"
step tail-removed history $G list --format json
step tail-add history $G add --kind 관측 --summary again --request-id reject-1
cp "$WORK/events.saved" "$H/events.jsonl"
mv .git/ha/history "$WORK/moved-history"
ln -s "$WORK/moved-history" .git/ha/history
step read-link history $G list --format json
rm .git/ha/history
mv "$WORK/moved-history" .git/ha/history
step restored history $G list --limit 2 --format json
