package main

// Checks of the goal ha-review-gate: a goal started under run-rules/5 whose
// SPEC asks for independent review (`Assurance: standard` or `strict`)
// completes only with a recorded review gate that covers the last head and the
// current goal documents. The tests drive the ha command line (runMain) on
// synthetic repositories and write review gates in the public review-gate/v1
// form (contracts/review-gate.md), as `spar gate` writes them; they never run
// Spar or a reviewer. The opening line of an assertion's failure message names
// the property it observes (jaekit:violation=<id>); a failure without it is a
// broken setup, not an observation.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

const rgGoal = "docs/specs/demo"

// Texts a gate file carries that the run record must not keep: a finding's
// words in a reason, a finding ID, a waived reviewer's user quote and a round
// folder path.
const (
	rgFindingText = "synthetic finding text that stays in the review records"
	rgFindingID   = "claude-F9"
	rgUserQuote   = "synthetic user words that stay in the review records"
	rgRoundFolder = "docs/reviews/demo-rounds"
)

// rgDoc is a criteria document as a gate round names it.
type rgDoc struct{ name, sha string }

// rgRound is one round of a synthetic gate.
type rgRound struct {
	stage               string
	base, head          string
	docs                []rgDoc
	required, completed int
	waived              bool
	dryRun              bool
}

// rgGate is a synthetic review-gate/v1 file.
type rgGate struct {
	state            string
	expect, last     string
	rounds           []rgRound
	defect, question int
}

type rgFixture struct {
	t                *testing.T
	dir, gates       string
	base, work, head string
	files            int
}

func rgSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func rgSpec(tier string) string {
	line := ""
	if tier != "" {
		line = "Assurance: " + tier + "\n"
	}
	return "# Demo value\n\nStatus: Ready\n" + line + "\n## 목표\n\n`src/value.txt` says ok, as [the value note](note.md) describes.\n\n" +
		"## 범위와 비목표\n\n- 범위: `src/**`\n\n## Acceptance Criteria\n- **AC-1** `src/value.txt` contains exactly `ok`.\n\n## Open Decisions\n\n없음\n"
}

const rgPlan = "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
	"| AC-1 | maintain | `sh tests/value.sh` | — | T001 |\n\n## 범위\n- 바꿀 수 있는 경로: `src/**`, `tests/**` (SPEC: 범위와 비목표)\n" +
	"- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n" +
	"| --- | --- | --- | --- | --- | --- | --- |\n| T001 | the value says ok | AC-1 | — | — | 예 | — |\n\n" +
	"## 예산\n- 검증 실행: 50\n- 경과 시간: 4시간\n- 비용: 미관측\n"

// rgRequireGit fails a check of a kept behavior when git is missing, so a
// missing tool never passes it; a check of a change is skipped instead.
func rgRequireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("setup: git is not available: %v", err)
	}
}

// rgRepo makes a committed repository holding one goal with the given tier
// ("" for a SPEC without the line) and makes it the working directory. Its
// goal documents are SPEC.md and the note SPEC.md links.
func rgRepo(t *testing.T, tier string) *rgFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not available: %v", err)
	}
	f := &rgFixture{t: t, dir: t.TempDir(), gates: t.TempDir()}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	f.put(rgGoal+"/SPEC.md", rgSpec(tier))
	f.put(rgGoal+"/note.md", "# Value note\n\nThe value is one word on one line.\n")
	f.put(rgGoal+"/PLAN.md", rgPlan)
	f.put(rgGoal+"/REVIEW.md", "# REVIEW — demo\n\n## 결론\nready\n")
	f.put(rgGoal+"/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | |\n")
	f.put("src/value.txt", "ok\n")
	f.put("tests/value.sh", "grep -qx ok src/value.txt\n")
	f.git("init", "-q", "-b", "main")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "base")
	t.Chdir(f.dir)
	return f
}

// newRGFixture starts the goal under run-rules/5, adds two code commits (work
// and head) and records a passing AC-1 check at the head. Only a Core without
// run-rules/5 refusing the start is the observed violation; any other failure
// is a broken setup.
func newRGFixture(t *testing.T, violation, tier string) *rgFixture {
	t.Helper()
	f := rgRepo(t, tier)
	f.start(violation, "--rules", "run-rules/5")
	f.put("src/work.txt", "one\n")
	f.work = f.commit("work")
	f.put("src/work.txt", "two\n")
	f.head = f.commit("fix")
	f.check()
	return f
}

func (f *rgFixture) start(violation string, extra ...string) {
	f.t.Helper()
	code, out := f.ha(append([]string{"start", rgGoal, "--request", "implement the synthetic goal"}, extra...)...)
	if code == exitUsage && strings.Contains(out, "--rules") {
		f.t.Fatalf("jaekit:violation=%s ha cannot start a run-rules/5 goal: exit %d: %s", violation, code, out)
	}
	if code != exitOK {
		f.t.Fatalf("setup: ha start exit %d: %s", code, out)
	}
	f.base = f.git("rev-parse", "HEAD")
}

func (f *rgFixture) put(rel, body string) {
	f.t.Helper()
	p := filepath.Join(f.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
}

func (f *rgFixture) read(rel string) []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return data
}

func (f *rgFixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("setup: git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *rgFixture) commit(message string) string {
	f.t.Helper()
	f.git("add", "-A")
	f.git("commit", "-q", "-m", message)
	return f.git("rev-parse", "HEAD")
}

func (f *rgFixture) ha(args ...string) (int, string) {
	var out, errb bytes.Buffer
	code := runMain(args, &out, &errb)
	return code, out.String() + errb.String()
}

// check records a passing AC-1 check at the current head.
func (f *rgFixture) check() {
	f.t.Helper()
	if code, out := f.ha("check", rgGoal, "AC-1"); code != exitOK {
		f.t.Fatalf("setup: ha check AC-1 exit %d: %s", code, out)
	}
}

// docs are the goal documents as they are now, named as a review round in
// this repository names its criteria (repository-relative paths).
func (f *rgFixture) docs() []rgDoc {
	f.t.Helper()
	var out []rgDoc
	for _, name := range []string{rgGoal + "/SPEC.md", rgGoal + "/note.md"} {
		out = append(out, rgDoc{name, rgSHA(f.read(name))})
	}
	return out
}

// covering is a complete gate whose rounds of the given stages each cover the
// current goal documents, with every code round chained up to the head.
func (f *rgFixture) covering(stages ...string) rgGate {
	f.t.Helper()
	g := rgGate{state: "complete", expect: f.head, last: f.head}
	for _, stage := range stages {
		r := rgRound{stage: stage, base: f.base, head: f.head, docs: f.docs(), required: 3, completed: 3}
		if stage == "plan" || stage == "plan-delta" {
			r.head = f.base
		}
		g.rounds = append(g.rounds, r)
	}
	return g
}

// chained is the candidate, fixes and cumulative candidate rounds of a goal
// with the work and fix commits, as Seal runs them.
func (f *rgFixture) chained() rgGate {
	f.t.Helper()
	g := rgGate{state: "complete", expect: f.head, last: f.head}
	for _, r := range []rgRound{{stage: "candidate", base: f.base, head: f.work}, {stage: "fixes", base: f.work, head: f.head},
		{stage: "candidate", base: f.base, head: f.head}} {
		r.docs, r.required, r.completed = f.docs(), 3, 3
		g.rounds = append(g.rounds, r)
	}
	return g
}

func rgCriteriaDigest(docs []rgDoc) string {
	h := sha256.New()
	for _, d := range docs {
		fmt.Fprintf(h, "%s\x00%s\x00", d.name, d.sha)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// gateValue is the gate as spar gate writes it, with every field of the
// file contract.
func (g rgGate) gateValue() map[string]any {
	rounds := []any{}
	openFindings := []any{}
	reasons := []any{}
	for i, r := range g.rounds {
		criteria := []any{}
		for _, d := range r.docs {
			criteria = append(criteria, map[string]any{"name": d.name, "sha256": d.sha})
		}
		path := fmt.Sprintf("%s/%02d-%s", rgRoundFolder, i+1, r.stage)
		round := map[string]any{"path": path, "stage": r.stage, "sequence": i + 1, "base": r.base, "head": r.head,
			"criteria_digest": rgCriteriaDigest(r.docs), "criteria": criteria,
			"packet_digest": rgSHA([]byte(fmt.Sprintf("packet %d", i))), "config_digest": rgSHA([]byte("config")),
			"requested_reviewers": []any{"claude", "codex", "agy"}, "required_reviewers": r.required,
			"completed_required": r.completed, "dry_run": r.dryRun, "counted": true}
		if r.waived {
			round["waived_reviewers"] = []any{map[string]any{"name": "agy", "preset": "agy", "status": "unavailable",
				"reason": "usage limit", "user_quote": rgUserQuote, "recorded_at": "2026-01-01T00:00:00Z"}}
		}
		rounds = append(rounds, round)
	}
	if g.defect+g.question > 0 {
		openFindings = append(openFindings, rgRoundFolder+"/01-candidate "+rgFindingID)
	}
	if g.state != "complete" {
		reasons = append(reasons, rgRoundFolder+"/01-candidate: "+rgFindingText)
	}
	return map[string]any{"schema": "review-gate/v1", "spar_version": "0.1.6-dev", "state": g.state, "reasons": reasons,
		"expect_head": g.expect, "last_reviewed_head": g.last, "previous_gate_digest": nil, "rounds": rounds,
		"open": map[string]any{"defect": g.defect, "requirement-question": g.question}, "open_findings": openFindings,
		"auto_deferred": 0, "growth": map[string]any{"added_lines": map[string]any{"check": 0, "other": 0},
			"over_hardening": false, "rounds": []any{}}, "written_at": "2026-01-01T00:00:00Z"}
}

// write writes a gate file outside the repository and returns its path and
// bytes.
func (f *rgFixture) write(value any) (string, []byte) {
	f.t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return f.writeBytes(append(data, '\n')), append(data, '\n')
}

func (f *rgFixture) writeBytes(data []byte) string {
	f.t.Helper()
	f.files++
	p := filepath.Join(f.gates, fmt.Sprintf("gate-%d.json", f.files))
	if err := os.WriteFile(p, data, 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return p
}

// record records the gate file at path; a refusal is the observed violation.
// An internal error (70), such as a record that cannot be written, is a
// broken setup.
func (f *rgFixture) record(violation, label, path string) {
	f.t.Helper()
	code, out := f.ha("note", rgGoal, "review", "--gate", path)
	if code == exitInternal {
		f.t.Fatalf("setup: %s: ha note review exit %d: %s", label, code, out)
	}
	if code != exitOK {
		f.t.Fatalf("jaekit:violation=%s %s: ha did not record the review gate: exit %d: %s", violation, label, code, out)
	}
}

func (f *rgFixture) recordGate(violation, label string, g rgGate) {
	f.t.Helper()
	path, _ := f.write(g.gateValue())
	f.record(violation, label, path)
}

func (f *rgFixture) records() []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"))
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return data
}

// rgState is what `ha status --format json` says.
type rgState struct {
	code       int
	status     string
	completion any
	reasons    []string // reason codes, criterion reasons as AC-n:code
	review     []string // the review_ reason codes, sorted
	raw        []byte
	value      map[string]any
}

func (f *rgFixture) status() rgState {
	f.t.Helper()
	var out, errb bytes.Buffer
	code := runMain([]string{"status", rgGoal, "--format", "json"}, &out, &errb)
	var value map[string]any
	if err := json.Unmarshal(out.Bytes(), &value); err != nil {
		f.t.Fatalf("setup: ha status exit %d gave no status JSON: %v: %s", code, err, errb.String())
	}
	s := rgState{code: code, raw: out.Bytes(), value: value, completion: value["completion_record"]}
	s.status, _ = value["status"].(string)
	list, _ := value["reasons"].([]any)
	for _, item := range list {
		r, _ := item.(map[string]any)
		c, _ := r["code"].(string)
		if id, _ := r["criterion"].(string); id != "" {
			c = id + ":" + c
		}
		s.reasons = append(s.reasons, c)
		if strings.HasPrefix(c, "review_") {
			s.review = append(s.review, c)
		}
	}
	sort.Strings(s.review)
	return s
}

// done runs `ha done` and returns its exit code and the status after it.
func (f *rgFixture) done() (int, rgState) {
	f.t.Helper()
	code, out := f.ha("done", rgGoal)
	if code == exitInternal {
		f.t.Fatalf("setup: ha done exit %d: %s", code, out)
	}
	return code, f.status()
}

func rgSame(got, want []string) bool {
	if len(got) == 0 && len(want) == 0 {
		return true
	}
	return reflect.DeepEqual(got, want)
}

// expectComplete reports unless the goal is complete with no review reason.
func (f *rgFixture) expectComplete(violation, label string) {
	f.t.Helper()
	if s := f.status(); s.code != 0 || s.status != "complete" || len(s.review) > 0 {
		f.t.Errorf("jaekit:violation=%s %s: status %s (exit %d), reasons %v; want complete", violation, label, s.status, s.code, s.reasons)
	}
}

// expectPending reports unless the goal is not complete, its review reasons
// are exactly want, and `ha done` records no completion.
func (f *rgFixture) expectPending(violation, label string, want ...string) {
	f.t.Helper()
	sort.Strings(want)
	if s := f.status(); s.code == 0 || s.status == "complete" || !rgSame(s.review, want) {
		f.t.Errorf("jaekit:violation=%s %s: status %s (exit %d), reasons %v; want not complete with %v", violation, label, s.status, s.code, s.reasons, want)
	}
	if code, s := f.done(); code == 0 || s.completion != nil {
		f.t.Errorf("jaekit:violation=%s %s: ha done exit %d, completion record %v; want no completion", violation, label, code, s.completion)
	}
}

// expectDone reports unless `ha done` records completion.
func (f *rgFixture) expectDone(violation, label string) {
	f.t.Helper()
	if code, s := f.done(); code != 0 || s.completion == nil {
		f.t.Errorf("jaekit:violation=%s %s: ha done exit %d, status %s, reasons %v; want a completion record", violation, label, code, s.status, s.reasons)
	}
}

// AC-1: a gate file is recorded with its digest and the summary the
// judgment needs, and the record keeps no finding text, quote or round path.
func TestHaReviewGateAC1_RecordsTheGateSummary(t *testing.T) {
	const v = "gate-summary-not-recorded"
	f := newRGFixture(t, v, "standard")
	complete := f.covering("plan", "candidate")
	complete.rounds[0].completed, complete.rounds[0].waived = 2, true
	open := f.chained()
	open.state, open.defect, open.question = "blocked", 2, 1
	open.rounds[1].dryRun = true
	for _, form := range []struct {
		label string
		gate  rgGate
	}{{"a complete gate with a plan round and a waived reviewer", complete}, {"a blocked gate with open findings", open}} {
		path, data := f.write(form.gate.gateValue())
		before := f.records()
		f.record(v, form.label, path)
		after := f.records()
		if !bytes.HasPrefix(after, before) || bytes.Count(after, []byte("\n")) != bytes.Count(before, []byte("\n"))+1 {
			t.Errorf("jaekit:violation=%s %s: recording did not append exactly one line", v, form.label)
			continue
		}
		raw := bytes.TrimSuffix(after[len(before):], []byte("\n"))
		var line struct {
			Kind   string         `json:"kind"`
			Note   string         `json:"note"`
			Review map[string]any `json:"review"`
		}
		if err := json.Unmarshal(raw, &line); err != nil || line.Kind != "note" || line.Note != "review" || line.Review == nil {
			t.Errorf("jaekit:violation=%s %s: the appended line is not a review note with a review object: %s", v, form.label, raw)
			continue
		}
		r := line.Review
		g := form.gate
		if r["gate_digest"] != rgSHA(data) || r["schema"] != "review-gate/v1" || r["state"] != g.state ||
			r["expect_head"] != g.expect || r["last_reviewed_head"] != g.last {
			t.Errorf("jaekit:violation=%s %s: the review object does not keep the file digest and summary: %v", v, form.label, r)
		}
		openCounts, _ := r["open"].(map[string]any)
		if openCounts["defect"] != float64(g.defect) || openCounts["requirement-question"] != float64(g.question) {
			t.Errorf("jaekit:violation=%s %s: open counts %v, want %d defect and %d requirement-question", v, form.label, r["open"], g.defect, g.question)
		}
		rounds, _ := r["rounds"].([]any)
		if len(rounds) != len(g.rounds) {
			t.Errorf("jaekit:violation=%s %s: %d rounds recorded, want %d", v, form.label, len(rounds), len(g.rounds))
			continue
		}
		for i, item := range rounds {
			got, _ := item.(map[string]any)
			want := g.rounds[i]
			var docs []rgDoc
			list, _ := got["criteria"].([]any)
			for _, c := range list {
				m, _ := c.(map[string]any)
				name, _ := m["name"].(string)
				sha, _ := m["sha256"].(string)
				docs = append(docs, rgDoc{name, sha})
			}
			if got["stage"] != want.stage || got["sequence"] != float64(i+1) || got["base"] != want.base || got["head"] != want.head ||
				got["criteria_digest"] != rgCriteriaDigest(want.docs) || !reflect.DeepEqual(docs, want.docs) ||
				got["required_reviewers"] != float64(want.required) || got["completed_required"] != float64(want.completed) ||
				got["dry_run"] != want.dryRun {
				t.Errorf("jaekit:violation=%s %s: round %d recorded as %v, want %+v", v, form.label, i+1, got, want)
			}
		}
		for _, secret := range []string{rgFindingText, rgFindingID, rgUserQuote, rgRoundFolder} {
			if bytes.Contains(raw, []byte(secret)) {
				t.Errorf("jaekit:violation=%s %s: the record keeps %q from the gate file", v, form.label, secret)
			}
		}
	}
	if s := f.status(); s.code == 70 || len(s.reasons) == 0 {
		t.Errorf("jaekit:violation=%s the record with review notes is not judged: exit %d, reasons %v", v, s.code, s.reasons)
	}
}

// AC-2: a standard goal under the new rules completes when the last recorded
// gate is complete, reviewed the head through a code round, covers the
// current goal documents and has no open finding.
func TestHaReviewGateAC2_StandardGoalCompletesWithACoveringGate(t *testing.T) {
	const v = "covering-gate-not-accepted"
	f := newRGFixture(t, v, "standard")
	extra := f.covering("candidate")
	docs := append([]rgDoc{{"AGENTS.md", rgSHA([]byte("rules"))}}, f.docs()...)
	docs[1], docs[2] = docs[2], docs[1]
	extra.rounds[0].docs, extra.rounds[0].completed, extra.rounds[0].waived = docs, 2, true
	for _, form := range []struct {
		label string
		gate  rgGate
	}{
		{"one candidate round", f.covering("candidate")},
		{"candidate, fixes and the cumulative candidate", f.chained()},
		{"a plan round before the candidate round", f.covering("plan", "candidate")},
		{"a waived reviewer and more criteria documents in another order", extra},
	} {
		f.recordGate(v, form.label, form.gate)
		f.expectComplete(v, form.label)
	}
	f.expectDone(v, "the last recorded gate")
}

// AC-3: a strict goal also needs a plan round, and its latest plan round must
// cover the current goal documents.
func TestHaReviewGateAC3_StrictGoalNeedsACoveringPlanRound(t *testing.T) {
	const v = "strict-plan-review-not-required"
	f := newRGFixture(t, v, "strict")
	codeOnly := f.chained()
	f.recordGate(v, "code rounds only", codeOnly)
	f.expectPending(v, "code rounds only", "review_plan_missing")

	stalePlan := f.covering("plan", "candidate")
	stalePlan.rounds[0].docs = []rgDoc{{rgGoal + "/SPEC.md", rgSHA([]byte("an earlier SPEC"))}, f.docs()[1]}
	f.recordGate(v, "a plan round of earlier goal documents", stalePlan)
	f.expectPending(v, "a plan round of earlier goal documents", "review_docs_stale")

	delta := f.covering("plan", "plan-delta", "candidate")
	delta.rounds[0].docs = stalePlan.rounds[0].docs
	f.recordGate(v, "a covering plan-delta round after an earlier plan round", delta)
	f.expectComplete(v, "a covering plan-delta round after an earlier plan round")

	f.recordGate(v, "a covering plan round", f.covering("plan", "candidate"))
	f.expectComplete(v, "a covering plan round")
	f.expectDone(v, "a covering plan round")
}

// AC-4: a gate whose reviewed head is not HEAD leaves completion pending with
// a stale-head reason; a gate recorded for the new head completes it.
func TestHaReviewGateAC4_StaleReviewedHeadKeepsCompletionPending(t *testing.T) {
	const v = "stale-review-head-accepted"
	f := newRGFixture(t, v, "standard")
	f.recordGate(v, "a gate of the head", f.covering("candidate"))
	f.expectComplete(v, "a gate of the head")
	reviewed := f.head
	f.put("src/later.txt", "later\n")
	f.head = f.commit("a commit after the review")
	f.check()
	f.expectPending(v, "a commit after the review", "review_head_stale")

	lastCode := f.covering("candidate")
	lastCode.rounds[0].head = reviewed
	f.recordGate(v, "a gate naming HEAD whose last code round reviewed an earlier head", lastCode)
	f.expectPending(v, "a gate naming HEAD whose last code round reviewed an earlier head", "review_head_stale")

	f.recordGate(v, "a gate of the new head", f.covering("candidate"))
	f.expectComplete(v, "a gate of the new head")
	f.expectDone(v, "a gate of the new head")
}

// AC-5: goal documents changed after the review, or a tier raised to strict,
// leave completion pending with a reason that the review does not cover them.
func TestHaReviewGateAC5_ChangedGoalDocumentsKeepCompletionPending(t *testing.T) {
	const v = "stale-goal-documents-accepted"
	f := newRGFixture(t, v, "standard")
	f.recordGate(v, "a covering gate", f.covering("candidate"))
	f.expectComplete(v, "a covering gate")
	confirm := func() {
		t.Helper()
		if code, out := f.ha("note", rgGoal, "confirm", "spec", "--quote", "goal documents changed by the user"); code != exitOK {
			t.Fatalf("setup: confirm spec exit %d: %s", code, out)
		}
		f.check()
	}

	f.put(rgGoal+"/note.md", "# Value note\n\nThe value is one lowercase word on one line.\n")
	confirm()
	f.expectPending(v, "a linked goal document changed after the review", "review_docs_stale")

	specOnly := f.covering("candidate")
	specOnly.rounds[0].docs = specOnly.rounds[0].docs[:1]
	f.recordGate(v, "a gate whose round left out a linked goal document", specOnly)
	f.expectPending(v, "a gate whose round left out a linked goal document", "review_docs_stale")

	f.recordGate(v, "a gate of the current goal documents", f.covering("candidate"))
	f.expectComplete(v, "a gate of the current goal documents")

	f.put(rgGoal+"/SPEC.md", rgSpec("strict"))
	confirm()
	f.expectPending(v, "the tier raised to strict", "review_docs_stale", "review_plan_missing")

	f.recordGate(v, "a strict gate of the current goal documents", f.covering("plan", "candidate"))
	f.expectComplete(v, "a strict gate of the current goal documents")
	f.expectDone(v, "a strict gate of the current goal documents")
}

// AC-6: a degraded or blocked gate, or one with an open defect or
// requirement-question, leaves completion pending with distinct reasons.
func TestHaReviewGateAC6_IncompleteGateKeepsCompletionPending(t *testing.T) {
	const v = "incomplete-gate-accepted"
	f := newRGFixture(t, v, "standard")
	gate := func(state string, defect, question int) rgGate {
		g := f.chained()
		g.state, g.defect, g.question = state, defect, question
		return g
	}
	for _, form := range []struct {
		label string
		gate  rgGate
		want  []string
	}{
		{"a degraded gate", gate("degraded", 0, 0), []string{"review_degraded"}},
		{"a blocked gate", gate("blocked", 0, 0), []string{"review_blocked"}},
		{"an open defect", gate("complete", 1, 0), []string{"review_open_defect"}},
		{"an open requirement-question", gate("complete", 0, 1), []string{"review_open_question"}},
		{"a blocked gate with both open", gate("blocked", 1, 1), []string{"review_blocked", "review_open_defect", "review_open_question"}},
	} {
		f.recordGate(v, form.label, form.gate)
		f.expectPending(v, form.label, form.want...)
	}
}

// AC-7: without a recorded gate a standard or strict goal stays pending, and
// no other note stands in for the gate; lowering the tier to light by a goal
// document change lets it complete without one.
func TestHaReviewGateAC7_MissingGateKeepsCompletionPending(t *testing.T) {
	const v = "missing-gate-accepted"
	for _, tier := range []string{"strict", "standard"} {
		f := newRGFixture(t, v, tier)
		f.expectPending(v, tier+" without a gate", "review_missing")
		for _, note := range [][]string{{"confirm", "spec", "--quote", "finish it without review"},
			{"input", "--quote", "skip the review"}, {"unblock", "--quote", "go on"}} {
			if code, out := f.ha(append([]string{"note", rgGoal}, note...)...); code != exitOK {
				t.Fatalf("setup: note %v exit %d: %s", note, code, out)
			}
		}
		f.expectPending(v, tier+" with user notes and no gate", "review_missing")
		if tier != "standard" {
			continue
		}
		f.put(rgGoal+"/SPEC.md", rgSpec("light"))
		if code, out := f.ha("note", rgGoal, "confirm", "spec", "--quote", "lower the tier to light"); code != exitOK {
			t.Fatalf("setup: confirm spec exit %d: %s", code, out)
		}
		f.check()
		f.expectComplete(v, "the tier lowered to light")
		f.expectDone(v, "the tier lowered to light")
	}
}

// AC-8: an unusable gate file is refused with its reason and nothing is
// recorded; the goal's judgment stays as it was.
func TestHaReviewGateAC8_UnusableGateFileIsRefused(t *testing.T) {
	const v = "unusable-gate-recorded"
	f := newRGFixture(t, v, "standard")
	mutate := func(change func(m map[string]any)) []byte {
		m := f.covering("candidate").gateValue()
		data, _ := json.Marshal(m)
		var value map[string]any
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatalf("setup: %v", err)
		}
		change(value)
		out, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return out
	}
	round := func(m map[string]any) map[string]any { return m["rounds"].([]any)[0].(map[string]any) }
	valid, _ := json.Marshal(f.covering("candidate").gateValue())
	forms := []struct {
		label string
		path  func() string
	}{
		{"a missing file", func() string { return filepath.Join(f.gates, "missing.json") }},
		{"a folder", func() string { return t.TempDir() }},
		{"an empty file", func() string { return f.writeBytes(nil) }},
		{"text that is not JSON", func() string { return f.writeBytes([]byte("{not json")) }},
		{"a JSON array", func() string { return f.writeBytes([]byte("[]")) }},
		{"JSON with trailing data", func() string { return f.writeBytes(append(append([]byte{}, valid...), []byte(" {}")...)) }},
		{"another schema", func() string { return f.writeBytes(mutate(func(m map[string]any) { m["schema"] = "review-gate/v2" })) }},
		{"no schema", func() string { return f.writeBytes(mutate(func(m map[string]any) { delete(m, "schema") })) }},
		{"an unknown state", func() string { return f.writeBytes(mutate(func(m map[string]any) { m["state"] = "done" })) }},
		{"no last reviewed head", func() string { return f.writeBytes(mutate(func(m map[string]any) { delete(m, "last_reviewed_head") })) }},
		{"no rounds", func() string { return f.writeBytes(mutate(func(m map[string]any) { delete(m, "rounds") })) }},
		{"a round without criteria", func() string {
			return f.writeBytes(mutate(func(m map[string]any) { delete(round(m), "criteria") }))
		}},
		{"a criteria document whose digest is not a sha256", func() string {
			return f.writeBytes(mutate(func(m map[string]any) {
				round(m)["criteria"].([]any)[0].(map[string]any)["sha256"] = "not-a-digest"
			}))
		}},
		{"an unknown stage", func() string { return f.writeBytes(mutate(func(m map[string]any) { round(m)["stage"] = "external" })) }},
		{"a round without its completed count", func() string {
			return f.writeBytes(mutate(func(m map[string]any) { delete(round(m), "completed_required") }))
		}},
		{"no open counts", func() string { return f.writeBytes(mutate(func(m map[string]any) { delete(m, "open") })) }},
		{"an open count that is not a number", func() string {
			return f.writeBytes(mutate(func(m map[string]any) { m["open"].(map[string]any)["defect"] = "0" }))
		}},
	}
	if os.Geteuid() != 0 {
		forms = append(forms, struct {
			label string
			path  func() string
		}{"an unreadable file", func() string {
			p := f.writeBytes(valid)
			if err := os.Chmod(p, 0); err != nil {
				t.Fatalf("setup: %v", err)
			}
			return p
		}})
	}
	refuse := func(phase string) {
		t.Helper()
		for _, form := range forms {
			path := form.path()
			before, state := f.records(), f.status()
			code, out := f.ha("note", rgGoal, "review", "--gate", path)
			first := strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]
			if code != exitRefused || !strings.HasPrefix(first, "ha: review gate ") || !strings.HasSuffix(first, "; nothing recorded") {
				t.Errorf("jaekit:violation=%s %s, %s: exit %d, message %q; want 65 with the gate's reason and nothing recorded", v, phase, form.label, code, first)
			}
			if after := f.records(); !bytes.Equal(after, before) {
				t.Errorf("jaekit:violation=%s %s, %s: the run record changed", v, phase, form.label)
			}
			if now := f.status(); !bytes.Equal(now.raw, state.raw) || now.code != state.code {
				t.Errorf("jaekit:violation=%s %s, %s: the judgment changed from %s %v to %s %v", v, phase, form.label, state.status, state.reasons, now.status, now.reasons)
			}
		}
	}
	refuse("without a recorded gate")
	if s := f.status(); !rgSame(s.review, []string{"review_missing"}) {
		t.Errorf("jaekit:violation=%s refused files left the reasons %v; want review_missing", v, s.reasons)
	}
	f.recordGate(v, "a valid gate", f.covering("candidate"))
	f.expectComplete(v, "a valid gate")
	refuse("after a valid gate")
}

// AC-9 (kept): a light goal under the new rules completes on its other
// conditions alone, with or without a gate.
func TestHaReviewGateAC9_LightGoalCompletesWithoutAGate(t *testing.T) {
	rgRequireGit(t)
	const v = "light-goal-needs-a-gate"
	f := newRGFixture(t, v, "light")
	f.expectComplete(v, "a light goal without a gate")
	f.expectDone(v, "a light goal without a gate")
	g := newRGFixture(t, v, "light")
	degraded := g.chained()
	degraded.state, degraded.defect = "degraded", 1
	g.recordGate(v, "a light goal with a degraded gate", degraded)
	g.expectComplete(v, "a light goal with a degraded gate")
	g.expectDone(v, "a light goal with a degraded gate")
}

// AC-10 (kept): a goal under the new rules without an `Assurance:` line
// completes on its other conditions alone.
func TestHaReviewGateAC10_GoalWithoutTierCompletesWithoutAGate(t *testing.T) {
	rgRequireGit(t)
	const v = "goal-without-tier-needs-a-gate"
	f := newRGFixture(t, v, "")
	f.expectComplete(v, "a goal without the line")
	f.expectDone(v, "a goal without the line")
}

// rgStatusKeys are the status/v1 fields of a goal without run-rules/4
// extras (contracts/run-record.md §4.5).
var rgStatusKeys = []string{"assurance", "budget", "changes", "check_author", "commit", "complete_without_record", "completion_record",
	"criteria", "exit_code", "findings", "goal", "ha_version", "lint", "logs", "reasons", "record_head", "rules", "schema",
	"skill", "spec_digest", "status", "tree_clean", "usage"}

// AC-11 (kept): goals started under run-rules/1 to run-rules/4 are judged as
// before, a standard tier included: they complete without a gate, keep their
// status fields and completion record, and refuse a review note.
func TestHaReviewGateAC11_EarlierRulesKeepTheirJudgment(t *testing.T) {
	rgRequireGit(t)
	const v = "earlier-rules-judgment-changed"
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4"} {
		f := rgRepo(t, "standard")
		if rules == "run-rules/4" {
			f.start(v, "--rules", rules)
		} else {
			f.start(v)
		}
		if rules == "run-rules/1" || rules == "run-rules/2" {
			// A goal started before run-rules/3, as scenarios rebuild one.
			path := filepath.Join(f.dir, rgGoal, "runs.jsonl")
			data := f.records()
			changed := bytes.Replace(data, []byte(`"rules":"run-rules/3"`), []byte(`"rules":"`+rules+`"`), 1)
			if bytes.Equal(changed, data) || bytes.Count(data, []byte("\n")) != 1 {
				t.Fatalf("setup: cannot rebuild a %s start", rules)
			}
			if err := os.WriteFile(path, changed, 0o644); err != nil {
				t.Fatalf("setup: %v", err)
			}
		}
		f.put("src/work.txt", "one\n")
		f.head = f.commit("work")
		f.check()
		s := f.status()
		var keys []string
		for k := range s.value {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		if s.code != 0 || s.status != "complete" || len(s.review) > 0 || s.value["rules"] != rules || !rgSame(keys, rgStatusKeys) {
			t.Errorf("jaekit:violation=%s %s: status %s (exit %d), rules %v, reasons %v, fields %v; want complete with the status/v1 fields",
				v, rules, s.status, s.code, s.value["rules"], s.reasons, keys)
		}
		f.expectDone(v, rules)
		completed := f.status()
		path, _ := f.write(f.covering("candidate").gateValue())
		before := f.records()
		if code, out := f.ha("note", rgGoal, "review", "--gate", path); code != exitRefused || !bytes.Equal(f.records(), before) {
			t.Errorf("jaekit:violation=%s %s: a review note gave exit %d (%s); want 65 and nothing recorded", v, rules, code, strings.TrimSpace(out))
		}
		if now := f.status(); now.status != "complete" || !reflect.DeepEqual(now.completion, completed.completion) {
			t.Errorf("jaekit:violation=%s %s: after a refused review note the status is %s with completion record %v; want %v",
				v, rules, now.status, now.completion, completed.completion)
		}
	}
}

// AC-12 (kept): ha does not build on Spar's packages, and recording and
// judging a gate give the same result for the same input, without reading
// the gate file again.
func TestHaReviewGateAC12_CoreStaysApartFromSparAndDeterministic(t *testing.T) {
	rgRequireGit(t)
	const v = "core-not-apart-or-not-deterministic"
	list := exec.Command("go", "list", "-deps", "github.com/jgoneit/jaekit/cmd/ha")
	out, err := list.Output()
	if err != nil {
		t.Fatalf("setup: go list -deps: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if dep == "github.com/jgoneit/jaekit/internal/spar" || strings.HasPrefix(dep, "github.com/jgoneit/jaekit/internal/spar/") ||
			strings.HasPrefix(dep, "github.com/jgoneit/jaekit/cmd/spar") {
			t.Errorf("jaekit:violation=%s ha depends on %s", v, dep)
		}
	}
	f := newRGFixture(t, v, "standard")
	path, _ := f.write(f.chained().gateValue())
	var reviews []any
	for i := 0; i < 2; i++ {
		before := f.records()
		f.record(v, "the same gate file", path)
		var line map[string]any
		if err := json.Unmarshal(bytes.TrimSpace(f.records()[len(before):]), &line); err != nil {
			t.Fatalf("setup: %v", err)
		}
		reviews = append(reviews, line["review"])
	}
	if !reflect.DeepEqual(reviews[0], reviews[1]) {
		t.Errorf("jaekit:violation=%s the same gate file was recorded as %v and %v", v, reviews[0], reviews[1])
	}
	first := f.status()
	if err := os.Remove(path); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if second := f.status(); !bytes.Equal(first.raw, second.raw) || first.status != "complete" {
		t.Errorf("jaekit:violation=%s the judgment changed (%s, then %s) or read the removed gate file", v, first.status, second.status)
	}
}
