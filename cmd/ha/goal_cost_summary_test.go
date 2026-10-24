//go:build !windows

package main

// Checks of the goal goal-cost-summary: `ha cost` summarizes what one goal
// cost in verification and review from its run record, its goal documents,
// Git and the Spar review series folders a person names
// (contracts/run-record.md §1.7). The tests drive the ha command line
// (runMain) on synthetic repositories and write review series in the public
// review-round/v1 and review-gate/v1 forms (contracts/review-round.md §5,
// contracts/review-gate.md §3), as spar writes them; only AC-10 runs a real
// spar binary, with a host reviewer whose answers the test writes. Each
// expected value is computed by the test from those records, not taken from
// the summary.
//
// The opening line of an assertion's failure message names the property it
// observes (jaekit:violation=<id>); a failure that starts with "setup:"
// observes nothing and is a broken setup.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	gcGoal   = "docs/specs/demo"
	gcSeries = "docs/reviews/demo"
)

// Synthetic words the records hold.
const (
	gcRequest      = "implement the synthetic goal"
	gcWaitOne      = "synthetic decision one asked of the user"
	gcWaitTwo      = "synthetic decision two asked of the user"
	gcAnswer       = "synthetic answer to decision one"
	gcSpecQuote    = "synthetic confirmation of the goal documents"
	gcQuestionText = "synthetic answer to the requirement question"
	gcExtendQuote  = "synthetic words that extend the code cap"
	gcWaiverQuote  = "synthetic words that waive agy"
)

type gcFixture struct {
	t    *testing.T
	dir  string
	base string
	head string
	// seqs names the record lines the fixture made.
	seqs map[string]int
}

func gcSpec(tier string) string {
	line := ""
	if tier != "" {
		line = "Assurance: " + tier + "\n"
	}
	return "# Demo value\n\nStatus: Ready\n" + line + "\n## 목표\n\n`src/value.txt` says ok.\n\n## 범위와 비목표\n\n- 범위: `src/**`\n\n" +
		"## Acceptance Criteria\n- **AC-1** `src/value.txt` contains exactly `ok`.\n- **AC-2** `src/keep.txt` keeps `keep`.\n" +
		"- **AC-3** (선택) `src/extra.txt` contains `extra`.\n\n## Open Decisions\n\n없음\n"
}

const gcPlan = "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
	"| AC-1 | change | `sh tests/junit.sh {HA_JUNIT_XML}` | checks/AC-1.json, tests/junit.sh | T001 |\n" +
	"| AC-2 | maintain | `sh tests/keep.sh` | — | T001 |\n" +
	"| AC-3 | maintain | `sh tests/extra.sh` | — | T001 |\n\n" +
	"## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/AC-1.json |\n\n" +
	"## 범위\n- 바꿀 수 있는 경로: `src/**`, `tests/**`, `checks/**` (SPEC: 범위와 비목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n" +
	"## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
	"| T001 | the values hold | AC-1, AC-2, AC-3 | — | — | 예 | — |\n\n## 예산\n- 검증 실행: 50\n- 경과 시간: 4시간\n- 비용: 미관측\n"

const gcRunner = `out="${1:-$HA_JUNIT_XML}"
if grep -qx ok src/value.txt; then
	printf '<testsuites><testcase classname="demo" name="value"/></testsuites>\n' > "$out"
	exit 0
fi
printf '<testsuites><testcase classname="demo" name="value"><failure message="jaekit:violation=value-wrong value">x</failure></testcase></testsuites>\n' > "$out"
exit 1
`

const gcDeclaration = `{"schema": "check-adapter/v1", "adapter": "junit-xml", "targets": [{"id": "value", "violation": "value-wrong", "test": {"classname": "demo", "name": "value"}}]}`

func gcSHA(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// gcRepo makes a committed repository holding one goal and makes it the
// working directory.
func gcRepo(t *testing.T, tier string) *gcFixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not available: %v", err)
	}
	f := &gcFixture{t: t, dir: t.TempDir(), seqs: map[string]int{}}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	f.put(".gitignore", "/docs/reviews/\n")
	f.put(gcGoal+"/SPEC.md", gcSpec(tier))
	f.put(gcGoal+"/PLAN.md", gcPlan)
	f.put(gcGoal+"/REVIEW.md", "# REVIEW — demo\n\n## 결론\nready\n")
	f.put(gcGoal+"/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | |\n")
	f.put("src/value.txt", "no\n")
	f.put("src/keep.txt", "keep\n")
	f.put("tests/junit.sh", gcRunner)
	f.put("tests/keep.sh", "grep -qx keep src/keep.txt\n")
	f.put("tests/extra.sh", "grep -qx extra src/extra.txt\n")
	f.put("checks/AC-1.json", gcDeclaration)
	f.git("init", "-q", "-b", "main")
	f.commit("base")
	t.Chdir(f.dir)
	return f
}

func (f *gcFixture) put(rel, body string) {
	f.t.Helper()
	p := filepath.Join(f.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
}

func (f *gcFixture) putJSON(rel string, value any) {
	f.t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	f.put(rel, string(data)+"\n")
}

// putLines writes a JSON lines file, one object per line.
func (f *gcFixture) putLines(rel string, values ...any) {
	f.t.Helper()
	var b strings.Builder
	for _, v := range values {
		data, err := json.Marshal(v)
		if err != nil {
			f.t.Fatalf("setup: %v", err)
		}
		b.Write(data)
		b.WriteByte('\n')
	}
	f.put(rel, b.String())
}

func (f *gcFixture) read(rel string) []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(rel)))
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return data
}

func (f *gcFixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("setup: git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *gcFixture) commit(message string) string {
	f.t.Helper()
	f.git("add", "-A")
	f.git("commit", "-q", "-m", message)
	f.head = f.git("rev-parse", "HEAD")
	return f.head
}

func gcHa(args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runMain(args, &out, &errb)
	return code, out.String(), errb.String()
}

// must runs an ha command that exists before this goal; any other exit is a
// broken setup. It returns the seq the command printed, if any.
func (f *gcFixture) must(name string, want int, args ...string) {
	f.t.Helper()
	code, out, errs := gcHa(args...)
	if code != want {
		f.t.Fatalf("setup: ha %v: exit %d, want %d: %s%s", args, code, want, out, errs)
	}
	if name != "" {
		f.seqs[name] = gcLastSeq(f.t, f.read(gcGoal+"/runs.jsonl"))
	}
}

func gcLastSeq(t *testing.T, records []byte) int {
	t.Helper()
	lines := gcLines(t, records)
	return int(lines[len(lines)-1]["seq"].(float64))
}

// gcLines decodes the run record, one object per line.
func gcLines(t *testing.T, records []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, raw := range bytes.Split(bytes.TrimSuffix(records, []byte("\n")), []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			t.Fatalf("setup: run record line: %v", err)
		}
		out = append(out, m)
	}
	return out
}

// retime moves every line's time to a synthetic minute (and a run's start
// five seconds before it) so waits have lengths, and links the chain again.
// It runs before any done line, whose record head names the line before it.
func (f *gcFixture) retime() {
	f.t.Helper()
	path := filepath.Join(f.dir, gcGoal, "runs.jsonl")
	lines := gcLines(f.t, f.read(gcGoal+"/runs.jsonl"))
	origin := time.Now().UTC().Add(-3 * time.Hour).Truncate(time.Second)
	var b bytes.Buffer
	var prev *string
	for _, l := range lines {
		seq := int(l["seq"].(float64))
		at := origin.Add(time.Duration(seq) * time.Minute)
		l["at"] = at.Format(time.RFC3339)
		if _, ok := l["started_at"]; ok {
			l["started_at"] = at.Add(-5 * time.Second).Format(time.RFC3339)
		}
		if prev == nil {
			l["prev"] = nil
		} else {
			l["prev"] = *prev
		}
		data, err := json.Marshal(l)
		if err != nil {
			f.t.Fatalf("setup: %v", err)
		}
		h := gcSHA(data)
		prev = &h
		b.Write(data)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
}

// gcBuild makes the goal every check reads: a run-rules/5 standard goal
// with two baselines whose overlaid check file changed (with the executor's
// test-change note), a check that failed
// and passed after a code change, a check recorded twice on the same inputs,
// two reported waits, input and confirm notes, a review series and a review
// gate that a completion record follows.
func gcBuild(t *testing.T) *gcFixture {
	t.Helper()
	f := gcRepo(t, "standard")
	f.base = f.head
	f.must("start", exitOK, "start", gcGoal, "--request", gcRequest, "--rules", "run-rules/5", "--test-authority", "the request lets the executor change the goal's tests")
	f.must("baseline1", exitOK, "check", gcGoal, "--baseline", "AC-1")
	f.must("wait1", exitOK, "note", gcGoal, "wait", "--reason", gcWaitOne)
	f.must("input", exitOK, "note", gcGoal, "input", "--quote", gcAnswer)
	f.put("tests/junit.sh", gcRunner+"# the runner reads one value\n")
	f.commit("check: comment")
	f.must("testchange", exitOK, "note", gcGoal, "test-change", "tests:tests/junit.sh", "--criteria", "AC-1", "--how", "the runner still reads src/value.txt")
	f.must("baseline2", exitOK, "check", gcGoal, "--baseline", "AC-1")
	f.put("src/value.txt", "ok\n")
	f.commit("work")
	f.must("ac1", exitOK, "check", gcGoal, "AC-1")
	f.must("ac3fail", exitProblems, "check", gcGoal, "AC-3")
	f.put("src/extra.txt", "extra\n")
	f.commit("extra")
	f.must("ac3pass", exitOK, "check", gcGoal, "AC-3")
	f.must("ac2a", exitOK, "check", gcGoal, "AC-2")
	f.must("ac2b", exitOK, "check", gcGoal, "AC-2")
	f.must("ac1final", exitOK, "check", gcGoal, "AC-1")
	f.must("confirm", exitOK, "note", gcGoal, "confirm", "spec", "--quote", gcSpecQuote)
	f.must("wait2", exitOK, "note", gcGoal, "wait", "--reason", gcWaitTwo)
	f.must("resume", exitOK, "note", gcGoal, "resume")
	f.retime()
	f.series(gcSeries)
	f.must("review", exitOK, "note", gcGoal, "review", "--gate", gcSeries+"/gate.json")
	f.must("done", exitOK, "done", gcGoal)
	return f
}

func gcBool(b bool) *bool { return &b }

// gcReviewer is a reviewer entry of round.json as spar writes it.
func gcReviewer(status string, elapsed float64, attempts int, fallback bool) map[string]any {
	list := []any{}
	for i := 0; i < attempts; i++ {
		list = append(list, map[string]any{"model": fmt.Sprintf("model-%d", i), "status": status, "elapsed_seconds": elapsed / float64(attempts)})
	}
	return map[string]any{"name": "", "preset": "", "isolation": "verified", "required": true, "status": status,
		"fallback_used": fallback, "elapsed_seconds": elapsed, "attempts": list, "findings": nil}
}

func gcRound(stage string, sequence int, counted bool, rerunOf []string, dryRun bool, base, head string, started, finished string,
	reviewers map[string]map[string]any) map[string]any {
	rs := map[string]any{}
	for name, r := range reviewers {
		r["name"], r["preset"] = name, name
		rs[name] = r
	}
	r := map[string]any{"schema": "review-round/v1", "spar_version": "0.1.7", "stage": stage, "sequence": sequence,
		"base": base, "head": head, "dry_run": dryRun, "counted": counted, "rerun_of": rerunOf, "started_at": started,
		"reviewers": rs}
	if finished != "" {
		r["finished_at"] = finished
	}
	if rerunOf == nil {
		r["rerun_of"] = []any{}
	}
	return r
}

func gcFinding(id, class string, inCheckPaths, unreadable bool) map[string]any {
	return map[string]any{"id": id, "reviewer": strings.SplitN(id, "-", 2)[0], "reviewer_id": "F1", "class": class,
		"class_reason": "synthetic", "proposed_class": class, "location": "head/src/value.txt:1", "path": "src/value.txt",
		"in_check_paths": inCheckPaths, "location_unreadable": unreadable, "claim": "synthetic claim"}
}

// series writes a review series: a counted candidate round with five
// findings and a waiver, a counted fixes round, its rerun, a dry run, an
// extension, another tool's folder, a stray file and a review gate that
// covers HEAD and the current goal documents.
func (f *gcFixture) series(dir string) {
	f.t.Helper()
	f.putJSON(dir+"/01-candidate/round.json", gcRound("candidate", 1, true, nil, false, f.base, f.head,
		"2026-01-01T00:00:00Z", "2026-01-01T00:10:00Z", map[string]map[string]any{
			"claude": gcReviewer("completed", 100.125, 1, false), "codex": gcReviewer("completed", 250.5, 1, false),
			"agy": gcReviewer("unavailable", 3.25, 2, true)}))
	f.putJSON(dir+"/01-candidate/findings.json", []any{
		gcFinding("claude-F1", "defect", true, false), gcFinding("codex-F1", "defect", false, false),
		gcFinding("codex-F2", "hardening", true, false), gcFinding("claude-F2", "requirement-question", false, false),
		gcFinding("claude-F3", "note", false, true)})
	f.putLines(dir+"/01-candidate/ledger.jsonl",
		map[string]any{"finding": "codex-F2", "disposition": "defer", "auto": true, "to": "deferred hardening", "recorded_at": "2026-01-01T00:10:00Z"},
		map[string]any{"finding": "claude-F1", "disposition": "fix", "auto": false, "commit": f.head, "recorded_at": "2026-01-01T00:11:00Z"},
		map[string]any{"finding": "codex-F1", "disposition": "reject", "auto": false, "evidence": "synthetic", "recorded_at": "2026-01-01T00:12:00Z"},
		map[string]any{"finding": "claude-F2", "disposition": "answer", "auto": false, "user_quote": gcQuestionText, "recorded_at": "2026-01-01T00:13:00Z"})
	f.putLines(dir+"/01-candidate/waivers.jsonl",
		map[string]any{"round": "01-candidate", "reviewer": "agy", "preset": "agy", "status": "unavailable", "user_quote": gcWaiverQuote, "recorded_at": "2026-01-01T00:14:00Z"})
	f.putJSON(dir+"/02-fixes/round.json", gcRound("fixes", 2, true, nil, false, f.head, f.head,
		"2026-01-01T01:00:00Z", "2026-01-01T01:05:30Z", map[string]map[string]any{
			"claude": gcReviewer("completed", 40, 1, false), "codex": gcReviewer("timed_out", 600, 1, false)}))
	f.putJSON(dir+"/02-fixes/findings.json", []any{gcFinding("claude-F1", "defect", false, false)})
	f.putLines(dir+"/02-fixes/ledger.jsonl",
		map[string]any{"finding": "claude-F1", "disposition": "duplicate", "auto": false, "of": "claude-F1", "recorded_at": "2026-01-01T01:06:00Z"})
	f.putJSON(dir+"/02b-fixes/round.json", gcRound("fixes", 3, false, []string{"02-fixes"}, false, f.head, f.head,
		"2026-01-01T01:10:00Z", "2026-01-01T01:12:00Z", map[string]map[string]any{"codex": gcReviewer("completed", 110.75, 1, false)}))
	f.putJSON(dir+"/02b-fixes/findings.json", []any{gcFinding("codex-F1", "hardening", false, false)})
	f.put(dir+"/02b-fixes/ledger.jsonl", "")
	f.putJSON(dir+"/04-dry/round.json", gcRound("candidate", 4, false, nil, true, f.base, f.head,
		"2026-01-01T02:00:00Z", "2026-01-01T02:00:01Z", map[string]map[string]any{"claude": gcReviewer("not_run", 0, 0, false)}))
	f.putLines(dir+"/extensions.jsonl", map[string]any{"stage": "code", "by": 1, "user_quote": gcExtendQuote, "recorded_at": "2026-01-01T00:30:00Z"})
	f.putJSON(dir+"/other-tool/round.json", map[string]any{"schema": "other-review/v1"})
	f.put(dir+"/notes.txt", "a person's own notes\n")
	docs := []any{map[string]any{"name": gcGoal + "/SPEC.md", "sha256": gcSHA(f.read(gcGoal + "/SPEC.md"))}}
	round := func(path, stage string, seq int, base string) map[string]any {
		return map[string]any{"path": dir + "/" + path, "stage": stage, "sequence": seq, "base": base, "head": f.head,
			"criteria_digest": gcSHA([]byte("criteria")), "criteria": docs, "packet_digest": gcSHA([]byte(path)),
			"config_digest": gcSHA([]byte("config")), "requested_reviewers": []any{"claude", "codex"}, "required_reviewers": 2,
			"completed_required": 2, "dry_run": false, "counted": true}
	}
	f.putJSON(dir+"/gate.json", map[string]any{"schema": "review-gate/v1", "spar_version": "0.1.7", "state": "complete",
		"reasons": []any{}, "expect_head": f.head, "last_reviewed_head": f.head, "previous_gate_digest": nil,
		"rounds":        []any{round("01-candidate", "candidate", 1, f.base), round("02-fixes", "fixes", 2, f.head), round("02b-fixes", "fixes", 3, f.head)},
		"open":          map[string]any{"defect": 0, "requirement-question": 0},
		"open_findings": []any{}, "auto_deferred": 1,
		"growth": map[string]any{"added_lines": map[string]any{"check": 7, "other": 3}, "over_hardening": true,
			"rounds": []any{map[string]any{"path": dir + "/02-fixes", "base": f.head, "head": f.head, "check": 7, "other": 3}}},
		"written_at": "2026-01-01T03:00:00Z"})
}

// cost runs ha cost and requires it to print a summary: a Core without the
// command, or one that refuses the input, is the observed violation.
func gcCost(t *testing.T, violation string, args ...string) string {
	t.Helper()
	code, out, errs := gcHa(append([]string{"cost"}, args...)...)
	if code != exitOK {
		t.Fatalf("jaekit:violation=%s ha cost %v gave exit %d, want 0: %s", violation, args, code, strings.TrimSpace(errs))
	}
	return out
}

// costJSON runs ha cost --format json and decodes it.
func gcCostJSON(t *testing.T, violation string, args ...string) map[string]any {
	t.Helper()
	out := gcCost(t, violation, append(args, "--format", "json")...)
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("jaekit:violation=%s ha cost --format json printed no JSON object: %v", violation, err)
	}
	return doc
}

// gcAt walks a decoded JSON value by keys and list indexes.
func gcAt(v any, path ...any) any {
	for _, p := range path {
		switch k := p.(type) {
		case string:
			m, ok := v.(map[string]any)
			if !ok {
				return nil
			}
			v = m[k]
		case int:
			l, ok := v.([]any)
			if !ok || k >= len(l) {
				return nil
			}
			v = l[k]
		}
	}
	return v
}

// gcSame compares a decoded JSON value with a Go value through JSON.
func gcSame(got, want any) bool {
	a, _ := json.Marshal(got)
	var normal any
	b, _ := json.Marshal(want)
	_ = json.Unmarshal(b, &normal)
	c, _ := json.Marshal(normal)
	return bytes.Equal(a, c)
}

func gcExpect(t *testing.T, violation, what string, got, want any) {
	t.Helper()
	if !gcSame(got, want) {
		g, _ := json.Marshal(got)
		w, _ := json.Marshal(want)
		t.Errorf("jaekit:violation=%s %s is %s, want %s", violation, what, g, w)
	}
}

func gcContains(t *testing.T, violation, label, text string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("jaekit:violation=%s %s does not show %q:\n%s", violation, label, w, text)
		}
	}
}

func gcStatus(t *testing.T) map[string]any {
	t.Helper()
	_, out, _ := gcHa("status", gcGoal, "--format", "json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("setup: ha status --format json: %v: %s", err, out)
	}
	return doc
}

// AC-1: the goal's tier, rules version, criteria counts, the files and
// commits ha report's Changed counts, and the valid completion record.
func TestGoalCostSummaryAC1_KindAndSize(t *testing.T) {
	const v = "cost-kind-missing"
	f := gcBuild(t)
	doc := gcCostJSON(t, v, gcGoal)
	_, report, _ := gcHa("report", gcGoal)
	changed := regexp.MustCompile(`^Changed: (\d+) files and (\d+) commits`).FindStringSubmatch(report)
	if changed == nil {
		t.Fatalf("setup: ha report printed no Changed counts:\n%s", report)
	}
	files, _ := strconv.Atoi(changed[1])
	commits, _ := strconv.Atoi(changed[2])
	gcExpect(t, v, "kind", doc["kind"], map[string]any{"assurance": "standard", "rules": "run-rules/5", "required_criteria": 2,
		"optional_criteria": 1, "base": f.base, "changed_files": files, "changed_commits": commits, "completion_record": f.seqs["done"]})
	text := gcCost(t, v, gcGoal)
	gcContains(t, v, "the Markdown summary", text, "- assurance tier: standard", "- rules: run-rules/5", "- criteria: 2 required, 1 optional",
		fmt.Sprintf("- changed: %d files and %d commits outside the bundle documents since base %s", files, commits, f.base[:12]),
		fmt.Sprintf("- completion record: seq %d", f.seqs["done"]))
	// A goal whose SPEC has no Assurance line shows none, and one without a
	// valid completion record shows none.
	g := gcRepo(t, "")
	g.must("", exitOK, "start", gcGoal, "--request", gcRequest, "--rules", "run-rules/5")
	doc = gcCostJSON(t, v, gcGoal)
	if gcAt(doc, "kind", "assurance") != nil || gcAt(doc, "unavailable", "kind.assurance") != "none" || gcAt(doc, "kind", "completion_record") != nil {
		t.Errorf("jaekit:violation=%s a SPEC without an Assurance line and a goal without completion give kind %v", v, doc["kind"])
	}
	gcContains(t, v, "the Markdown summary", gcCost(t, v, gcGoal), "- assurance tier: none", "- completion record: none")
}

// gcRecordCounts reads the run record the way the summary must count it.
type gcCounts struct {
	runs      map[string]map[string]int
	check     int64
	baseline  int64
	perTarget map[string][2]int
	first     map[string]any
	last      map[string]any
}

func gcCount(t *testing.T, records []byte) gcCounts {
	t.Helper()
	c := gcCounts{runs: map[string]map[string]int{"check": {}, "baseline": {}}, perTarget: map[string][2]int{}}
	lines := gcLines(t, records)
	c.first, c.last = lines[0], lines[len(lines)-1]
	for _, l := range lines {
		kind, _ := l["kind"].(string)
		if kind != "check" && kind != "baseline" {
			continue
		}
		c.runs[kind][l["result"].(string)]++
		ms := int64(l["duration_ms"].(float64))
		target := gcAt(l, "target", "criterion").(string)
		pt := c.perTarget[target]
		if kind == "check" {
			c.check += ms
			pt[0]++
		} else {
			c.baseline += ms
			pt[1]++
		}
		c.perTarget[target] = pt
	}
	return c
}

// AC-2: runs by kind and result with their time, runs per condition, each
// re-recorded run with the inputs that changed or the previous result, the
// note counts, the record span and the budget values ha status shows.
func TestGoalCostSummaryAC2_VerificationCost(t *testing.T) {
	const v = "cost-verification-missing"
	f := gcBuild(t)
	doc := gcCostJSON(t, v, gcGoal)
	c := gcCount(t, f.read(gcGoal+"/runs.jsonl"))
	vr := doc["verification"]
	gcExpect(t, v, "runs", gcAt(vr, "runs"), c.runs)
	gcExpect(t, v, "duration_ms", gcAt(vr, "duration_ms"), map[string]any{"check": c.check, "baseline": c.baseline, "total": c.check + c.baseline})
	targets := []any{}
	for _, id := range []string{"AC-1", "AC-3", "AC-2"} {
		targets = append(targets, map[string]any{"target": id, "checks": c.perTarget[id][0], "baselines": c.perTarget[id][1]})
	}
	gcExpect(t, v, "targets", gcAt(vr, "targets"), targets)
	s := f.seqs
	gcExpect(t, v, "rerecords", gcAt(vr, "rerecords"), []any{
		map[string]any{"target": "AC-1", "kind": "baseline", "seq": s["baseline2"], "previous_seq": s["baseline1"], "changed": []string{"overlay"}, "previous_result": "fail_as_expected"},
		map[string]any{"target": "AC-3", "kind": "check", "seq": s["ac3pass"], "previous_seq": s["ac3fail"], "changed": []string{"code"}, "previous_result": "fail"},
		map[string]any{"target": "AC-2", "kind": "check", "seq": s["ac2b"], "previous_seq": s["ac2a"], "changed": []string{}, "previous_result": "pass"},
		map[string]any{"target": "AC-1", "kind": "check", "seq": s["ac1final"], "previous_seq": s["ac1"], "changed": []string{"code"}, "previous_result": "pass"},
	})
	gcExpect(t, v, "notes", gcAt(vr, "notes"), map[string]any{"test_change": 1, "recover": 0, "allow_recovery": 0, "reopen": 0})
	from, _ := time.Parse(time.RFC3339, c.first["at"].(string))
	to, _ := time.Parse(time.RFC3339, c.last["at"].(string))
	gcExpect(t, v, "span", gcAt(vr, "span"), map[string]any{"first_seq": 1, "first_at": c.first["at"], "last_seq": c.last["seq"],
		"last_at": c.last["at"], "seconds": int64(to.Sub(from) / time.Second)})
	budget := func(doc, status map[string]any) {
		t.Helper()
		bd := gcStatusBudget(status)
		gcExpect(t, v, "budget", gcAt(doc, "verification", "budget"), bd)
	}
	budget(doc, gcStatus(t))
	text := gcCost(t, v, gcGoal)
	gcContains(t, v, "the Markdown summary", text,
		fmt.Sprintf("  - AC-1 baseline seq %d after seq %d: overlaid check files changed", s["baseline2"], s["baseline1"]),
		fmt.Sprintf("  - AC-3 check seq %d after seq %d: code changed", s["ac3pass"], s["ac3fail"]),
		fmt.Sprintf("  - AC-2 check seq %d after seq %d: no input changed; previous result pass", s["ac2b"], s["ac2a"]),
		fmt.Sprintf("- run time: %d ms in all (check %d ms, baseline %d ms)", c.check+c.baseline, c.check, c.baseline))
	// A reopen opens a new budget window; the summary follows ha status.
	f.must("reopen", exitOK, "note", gcGoal, "reopen", "--quote", "synthetic rework request")
	doc = gcCostJSON(t, v, gcGoal)
	gcExpect(t, v, "notes after a reopen", gcAt(doc, "verification", "notes", "reopen"), 1)
	budget(doc, gcStatus(t))
	if gcAt(doc, "kind", "completion_record") != nil {
		t.Errorf("jaekit:violation=%s after a reopen the summary still names completion record %v", v, gcAt(doc, "kind", "completion_record"))
	}
}

// gcStatusBudget is the budget part of ha status the summary repeats.
func gcStatusBudget(status map[string]any) map[string]any {
	b, _ := status["budget"].(map[string]any)
	return map[string]any{"window_start": b["window_start"], "span_seconds": b["span_seconds"], "reported_wait_seconds": b["reported_wait_seconds"],
		"elapsed_seconds": b["elapsed_seconds"], "elapsed_limit_seconds": b["elapsed_limit_seconds"], "runs": b["runs"], "runs_limit": b["runs_limit"]}
}

// AC-3: each reported wait with its decision and its length as ha's budget
// computes it, the input and confirm notes, and with a series the
// requirement questions with their answers, the cap extensions and the
// waivers, counted by source and listed.
func TestGoalCostSummaryAC3_QuestionsAndWaits(t *testing.T) {
	const v = "cost-questions-missing"
	f := gcBuild(t)
	doc := gcCostJSON(t, v, gcGoal, "--series", gcSeries)
	// Each synthetic line is a minute after the one before it, and a wait
	// runs from the second after its note to the line that ends it.
	gcExpect(t, v, "waits", gcAt(doc, "questions", "waits"), []any{
		map[string]any{"seq": f.seqs["wait1"], "reason": gcWaitOne, "seconds": 59},
		map[string]any{"seq": f.seqs["wait2"], "reason": gcWaitTwo, "seconds": 59},
	})
	gcExpect(t, v, "reported_wait_seconds", gcAt(doc, "questions", "reported_wait_seconds"), gcAt(gcStatus(t), "budget", "reported_wait_seconds"))
	gcExpect(t, v, "input and confirm notes", []any{gcAt(doc, "questions", "input_notes"), gcAt(doc, "questions", "confirm_notes")}, []int{1, 1})
	gcExpect(t, v, "questions by source", gcAt(doc, "questions", "by_source"),
		map[string]any{"wait": 2, "requirement_question": 1, "extension": 1, "waiver": 1})
	gcExpect(t, v, "requirement questions", gcAt(doc, "questions", "requirement_questions"), []any{map[string]any{"series": gcSeries,
		"round": "01-candidate", "finding": "claude-F2", "disposition": "answer", "user_quote": gcQuestionText}})
	gcExpect(t, v, "extensions", gcAt(doc, "questions", "extensions"), []any{map[string]any{"series": gcSeries, "stage": "code", "by": 1, "user_quote": gcExtendQuote}})
	gcExpect(t, v, "waivers", gcAt(doc, "questions", "waivers"), []any{map[string]any{"series": gcSeries, "round": "01-candidate", "reviewer": "agy", "user_quote": gcWaiverQuote}})
	text := gcCost(t, v, gcGoal, "--series", gcSeries)
	gcContains(t, v, "the Markdown summary", text,
		"- questions by source: waits 2, requirement-questions 1, cap extensions 1, reviewer waivers 1",
		fmt.Sprintf("  - seq %d, 59 seconds: %s", f.seqs["wait1"], gcWaitOne), fmt.Sprintf("  - seq %d, 59 seconds: %s", f.seqs["wait2"], gcWaitTwo),
		"- input notes 1; confirm notes 1", "  - "+gcSeries+" 01-candidate claude-F2: answer: "+gcQuestionText,
		"  - "+gcSeries+": code +1: "+gcExtendQuote, "  - "+gcSeries+" 01-candidate agy: "+gcWaiverQuote)
	// Without a series the Spar sources are not given, never zero.
	doc = gcCostJSON(t, v, gcGoal)
	for _, key := range []string{"requirement_question", "extension", "waiver"} {
		if gcAt(doc, "questions", "by_source", key) != nil || gcAt(doc, "unavailable", "questions.by_source."+key) != "not_given" {
			t.Errorf("jaekit:violation=%s without a series the %s source is %v", v, key, gcAt(doc, "questions", "by_source", key))
		}
	}
	gcContains(t, v, "the Markdown summary", gcCost(t, v, gcGoal), "requirement-questions, cap extensions and reviewer waivers not given")
}

// AC-4: rounds by stage, counted rounds apart from reruns, dry runs left
// out, each round's recorded span and each reviewer's elapsed seconds,
// status, attempts and fallback, and each reviewer's total; without a series
// the review part is not given and shows no zero.
func TestGoalCostSummaryAC4_ReviewRounds(t *testing.T) {
	const v = "cost-review-missing"
	gcBuild(t)
	doc := gcCostJSON(t, v, gcGoal, "--series", gcSeries)
	r := doc["review"]
	gcExpect(t, v, "stages", gcAt(r, "stages"), []any{
		map[string]any{"stage": "plan", "counted": 0, "reruns": 0, "counted_not_recorded": 0},
		map[string]any{"stage": "plan-delta", "counted": 0, "reruns": 0, "counted_not_recorded": 0},
		map[string]any{"stage": "candidate", "counted": 1, "reruns": 0, "counted_not_recorded": 0},
		map[string]any{"stage": "fixes", "counted": 1, "reruns": 1, "counted_not_recorded": 0},
	})
	gcExpect(t, v, "series", gcAt(r, "series", 0, "rounds"), 3)
	gcExpect(t, v, "dry runs", gcAt(r, "series", 0, "dry_runs"), 1)
	rounds, _ := gcAt(r, "rounds").([]any)
	var names []string
	for _, rd := range rounds {
		names = append(names, gcAt(rd, "round").(string))
	}
	gcExpect(t, v, "round names", names, []string{"01-candidate", "02-fixes", "02b-fixes"})
	gcExpect(t, v, "01-candidate seconds", gcAt(r, "rounds", 0, "seconds"), 600)
	gcExpect(t, v, "02-fixes seconds", gcAt(r, "rounds", 1, "seconds"), 330)
	gcExpect(t, v, "02b-fixes rerun", []any{gcAt(r, "rounds", 2, "counted"), gcAt(r, "rounds", 2, "rerun_of")}, []any{false, []string{"02-fixes"}})
	gcExpect(t, v, "01-candidate reviewers", gcAt(r, "rounds", 0, "reviewers"), []any{
		map[string]any{"name": "agy", "status": "unavailable", "elapsed_seconds": 3.25, "attempts": 2, "fallback_used": true},
		map[string]any{"name": "claude", "status": "completed", "elapsed_seconds": 100.125, "attempts": 1, "fallback_used": false},
		map[string]any{"name": "codex", "status": "completed", "elapsed_seconds": 250.5, "attempts": 1, "fallback_used": false},
	})
	gcExpect(t, v, "reviewer seconds", gcAt(r, "reviewer_seconds"), []any{
		map[string]any{"reviewer": "agy", "seconds": 3.25, "rounds": 1, "not_recorded": 0},
		map[string]any{"reviewer": "claude", "seconds": 140.125, "rounds": 2, "not_recorded": 0},
		map[string]any{"reviewer": "codex", "seconds": 961.25, "rounds": 3, "not_recorded": 0},
	})
	text := gcCost(t, v, gcGoal, "--series", gcSeries)
	gcContains(t, v, "the Markdown summary", text,
		"- rounds by stage: plan 0 counted, 0 reruns; plan-delta 0 counted, 0 reruns; candidate 1 counted, 0 reruns; fixes 1 counted, 1 reruns",
		"  - "+gcSeries+" 01-candidate (candidate, sequence 1, counted): 600s, 2026-01-01T00:00:00Z – 2026-01-01T00:10:00Z",
		"    - agy: unavailable, 3.25s, attempts 2, fallback used", "  - "+gcSeries+" 02b-fixes (fixes, sequence 3, rerun of 02-fixes): 120s",
		"  - codex: 961.25 seconds in 3 rounds")
	if strings.Contains(text, "04-dry") {
		t.Errorf("jaekit:violation=%s the dry run is listed among the rounds:\n%s", v, text)
	}
	// Without a series the review part says it was not given.
	doc = gcCostJSON(t, v, gcGoal)
	if doc["review"] != nil || gcAt(doc, "unavailable", "review") != "not_given" {
		t.Errorf("jaekit:violation=%s without a series the review part is %v", v, doc["review"])
	}
	text = gcCost(t, v, gcGoal)
	section := text[strings.Index(text, "## Review rounds"):strings.Index(text, "## Findings and fixes")]
	if !strings.Contains(section, "- not given: no review series was named with --series") || regexp.MustCompile(`\b0\b`).MatchString(section) {
		t.Errorf("jaekit:violation=%s without a series the review part shows:\n%s", v, section)
	}
}

// AC-5: findings by class and location, their last dispositions by class,
// automatic deferrals apart, and the lines fixes rounds added as the gate's
// growth records them; 0 without a fixes round.
func TestGoalCostSummaryAC5_FindingsAndGrowth(t *testing.T) {
	const v = "cost-findings-missing"
	f := gcBuild(t)
	doc := gcCostJSON(t, v, gcGoal, "--series", gcSeries)
	d := func(fix, reject, deferred, auto, duplicate, answer, none int) map[string]any {
		return map[string]any{"fix": fix, "reject": reject, "defer": deferred, "auto_defer": auto, "duplicate": duplicate, "answer": answer, "none": none}
	}
	gcExpect(t, v, "classes", gcAt(doc, "findings", "classes"), []any{
		map[string]any{"class": "defect", "total": 3, "check_paths": 1, "other": 2, "unreadable": 0, "location_not_recorded": 0, "dispositions": d(1, 1, 0, 0, 1, 0, 0)},
		map[string]any{"class": "requirement-question", "total": 1, "check_paths": 0, "other": 1, "unreadable": 0, "location_not_recorded": 0, "dispositions": d(0, 0, 0, 0, 0, 1, 0)},
		map[string]any{"class": "hardening", "total": 2, "check_paths": 1, "other": 1, "unreadable": 0, "location_not_recorded": 0, "dispositions": d(0, 0, 0, 1, 0, 0, 1)},
		map[string]any{"class": "note", "total": 1, "check_paths": 0, "other": 0, "unreadable": 1, "location_not_recorded": 0, "dispositions": d(0, 0, 0, 0, 0, 0, 1)},
	})
	gcExpect(t, v, "growth", gcAt(doc, "findings", "growth"), []any{map[string]any{"series": gcSeries, "lines": map[string]any{"check": 7, "other": 3}, "source": "gate", "gate": "gate.json"}})
	gcExpect(t, v, "growth total", gcAt(doc, "findings", "growth_total"), map[string]any{"check": 7, "other": 3})
	text := gcCost(t, v, gcGoal, "--series", gcSeries)
	gcContains(t, v, "the Markdown summary", text,
		"- defect: 3 (check paths 1, other 2, unreadable location 0, location not recorded 0)",
		"  - fix 1, reject 1, defer 0, automatic defer 0, duplicate 1, answer 0, no disposition 0",
		"- lines added by fixes rounds: check 7, other 3 in all", "  - "+gcSeries+": check 7, other 3 (gate.json)")
	// A series without a fixes round added 0 lines; one with a fixes round
	// but no gate file has not recorded them.
	f.series("docs/reviews/nofixes")
	if err := os.RemoveAll(filepath.Join(f.dir, "docs/reviews/nofixes/02-fixes")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(f.dir, "docs/reviews/nofixes/02b-fixes")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	f.series("docs/reviews/nogate")
	if err := os.Remove(filepath.Join(f.dir, "docs/reviews/nogate/gate.json")); err != nil {
		t.Fatalf("setup: %v", err)
	}
	doc = gcCostJSON(t, v, gcGoal, "--series", "docs/reviews/nofixes", "--series", "docs/reviews/nogate")
	gcExpect(t, v, "growth of two series", gcAt(doc, "findings", "growth"), []any{
		map[string]any{"series": "docs/reviews/nofixes", "lines": map[string]any{"check": 0, "other": 0}, "source": "no_fixes_round", "gate": nil},
		map[string]any{"series": "docs/reviews/nogate", "lines": nil, "source": "no_gate_file", "gate": nil},
	})
	if gcAt(doc, "findings", "growth_total") != nil || gcAt(doc, "unavailable", "findings.growth_total") != "not_recorded" {
		t.Errorf("jaekit:violation=%s a series without a gate file still gives a growth total %v", v, gcAt(doc, "findings", "growth_total"))
	}
}

// AC-6 (manual; this test runs with the full verification): tokens are not
// recorded anywhere, so the summary shows them as not recorded, never as 0.
func TestGoalCostSummaryAC6_TokensNotRecorded(t *testing.T) {
	const v = "cost-tokens-shown"
	gcBuild(t)
	doc := gcCostJSON(t, v, gcGoal, "--series", gcSeries)
	gcExpect(t, v, "tokens", doc["tokens"], map[string]any{"input": nil, "cached_input": nil, "output": nil})
	for _, key := range []string{"input", "cached_input", "output"} {
		if gcAt(doc, "unavailable", "tokens."+key) != "not_recorded" {
			t.Errorf("jaekit:violation=%s tokens.%s is not marked not recorded", v, key)
		}
	}
	for lang, want := range map[string]string{"en": "- new input, cached input and output tokens: not recorded", "ko": "- 새 입력·캐시 입력·출력 토큰: 기록 없음"} {
		text := gcCost(t, v, gcGoal, "--series", gcSeries, "--lang", lang)
		section := text[strings.LastIndex(text, "\n## "):]
		if !strings.Contains(section, want) || regexp.MustCompile(`\d`).MatchString(section) {
			t.Errorf("jaekit:violation=%s the %s token part is:\n%s", v, lang, section)
		}
	}
}

// gcNumbers lists the numbers a text holds, in order of size, so two
// languages that place them differently compare equal.
func gcNumbers(text string) []string {
	out := regexp.MustCompile(`\d+(\.\d+)?`).FindAllString(text, -1)
	sort.Strings(out)
	return out
}

// AC-7: the Markdown and JSON forms show the same values, the language
// changes only labels and sentences, and no sentence compares the goal with
// another or claims an improvement.
func TestGoalCostSummaryAC7_OutputForms(t *testing.T) {
	const v = "cost-output-forms-differ"
	f := gcBuild(t)
	en := gcCost(t, v, gcGoal, "--series", gcSeries)
	ko := gcCost(t, v, gcGoal, "--series", gcSeries, "--lang", "ko")
	if def := gcCost(t, v, gcGoal, "--series", gcSeries, "--lang", "en"); def != en {
		t.Errorf("jaekit:violation=%s --lang en differs from the default", v)
	}
	if en == ko {
		t.Errorf("jaekit:violation=%s --lang ko prints the English summary", v)
	}
	if a, b := gcNumbers(en), gcNumbers(ko); !gcSame(a, b) {
		t.Errorf("jaekit:violation=%s the two languages show different numbers:\n%v\n%v", v, a, b)
	}
	for _, token := range []string{"AC-1", "AC-2", "AC-3", "fail_as_expected", "pass", "01-candidate", "02b-fixes", "claude-F2", "unavailable",
		"defect", "requirement-question", "hardening", "gate.json", "notes.txt", "other-tool/", gcWaitOne, gcQuestionText, gcWaiverQuote, gcExtendQuote, f.base[:12]} {
		if strings.Count(en, token) != strings.Count(ko, token) || !strings.Contains(en, token) {
			t.Errorf("jaekit:violation=%s %q appears %d times in English and %d in Korean", v, token, strings.Count(en, token), strings.Count(ko, token))
		}
	}
	jsonOut := gcCost(t, v, gcGoal, "--series", gcSeries, "--format", "json")
	var doc map[string]any
	if err := json.Unmarshal([]byte(jsonOut), &doc); err != nil {
		t.Fatalf("jaekit:violation=%s no JSON: %v", v, err)
	}
	if ko2 := gcCost(t, v, gcGoal, "--series", gcSeries, "--format", "json", "--lang", "ko"); ko2 != jsonOut {
		t.Errorf("jaekit:violation=%s the JSON form changes with --lang", v)
	}
	// Every number of the JSON form, other than the digests, commits and
	// times, is in the Markdown form, and the Markdown form holds no other.
	var want []string
	var walk func(key string, x any)
	walk = func(key string, x any) {
		switch val := x.(type) {
		case map[string]any:
			for k, item := range val {
				walk(k, item)
			}
		case []any:
			for _, item := range val {
				walk(key, item)
			}
		case float64:
			want = append(want, strconv.FormatFloat(val, 'f', -1, 64))
		}
	}
	walk("", doc)
	sort.Strings(want)
	numbers := map[string]bool{}
	for _, n := range gcNumbers(en) {
		numbers[n] = true
	}
	for _, n := range want {
		if !numbers[n] {
			t.Errorf("jaekit:violation=%s the JSON value %s is not in the Markdown form", v, n)
		}
	}
	for _, word := range []string{"improv", "better", "worse", "faster", "slower", "cheaper", "costlier", "개선", "효과", "나아", "줄었", "늘었", "빨라", "느려"} {
		for lang, text := range map[string]string{"en": en, "ko": ko} {
			if strings.Contains(strings.ToLower(text), word) {
				t.Errorf("jaekit:violation=%s the %s summary says %q", v, lang, word)
			}
		}
	}
}

// gcTree reads every file under dir, .git included, with its mode.
func gcTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	out := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		if d.IsDir() {
			out[rel] = "dir " + info.Mode().String()
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = info.Mode().String() + " " + gcSHA(data) + " " + info.ModTime().String()
		return nil
	})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return out
}

// AC-8: the summary writes nothing (records, lock, working tree, index,
// refs, Spar records), gives the same output for the same input a second
// later, starts no reviewer or model CLI, and leaves the other commands'
// output and exit codes as they were.
func TestGoalCostSummaryAC8_ReadOnly(t *testing.T) {
	const v = "cost-not-read-only"
	f := gcBuild(t)
	// Reviewer and model CLIs on PATH record that they ran.
	bin, ran := t.TempDir(), filepath.Join(t.TempDir(), "ran")
	for _, name := range []string{"claude", "codex", "agy", "spar", "gemini"} {
		script := "#!/bin/sh\necho " + name + " >> '" + ran + "'\nexit 0\n"
		if err := os.WriteFile(filepath.Join(bin, name), []byte(script), 0o755); err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	others := func() []string {
		var out []string
		for _, args := range [][]string{{"status", gcGoal, "--format", "json"}, {"status", gcGoal}, {"report", gcGoal}, {"report", gcGoal, "--lang", "ko"},
			{"timing", gcGoal, "--format", "json"}, {"lint", gcGoal}} {
			code, stdout, stderr := gcHa(args...)
			out = append(out, fmt.Sprintf("%v exit %d\n%s%s", args, code, stdout, stderr))
		}
		return out
	}
	beforeOthers := others()
	beforeTree := gcTree(t, f.dir)
	var outputs []string
	for _, args := range [][]string{{gcGoal}, {gcGoal, "--series", gcSeries}, {gcGoal, "--series", gcSeries, "--format", "json"}, {gcGoal, "--series", gcSeries, "--lang", "ko"}} {
		outputs = append(outputs, gcCost(t, v, args...))
	}
	time.Sleep(1100 * time.Millisecond)
	for i, args := range [][]string{{gcGoal}, {gcGoal, "--series", gcSeries}, {gcGoal, "--series", gcSeries, "--format", "json"}, {gcGoal, "--series", gcSeries, "--lang", "ko"}} {
		if again := gcCost(t, v, args...); again != outputs[i] {
			t.Errorf("jaekit:violation=%s ha cost %v printed a different summary a second later", v, args)
		}
	}
	afterTree := gcTree(t, f.dir)
	for path, before := range beforeTree {
		if afterTree[path] != before {
			t.Errorf("jaekit:violation=%s ha cost changed %s", v, path)
		}
	}
	for path := range afterTree {
		if _, ok := beforeTree[path]; !ok {
			t.Errorf("jaekit:violation=%s ha cost created %s", v, path)
		}
	}
	if data, err := os.ReadFile(ran); err == nil {
		t.Errorf("jaekit:violation=%s ha cost started %s", v, strings.TrimSpace(string(data)))
	}
	if afterOthers := others(); !gcSame(afterOthers, beforeOthers) {
		t.Errorf("jaekit:violation=%s the other commands changed after ha cost:\n%v\n%v", v, beforeOthers, afterOthers)
	}
}

// AC-9: input problems. A goal without a start record shows only what its
// documents give and says there is no run record; a missing or unreadable
// series, a broken round or a broken run record ends with an error naming
// the file and no summary; other tools' folders and files are skipped and
// named; fields an older ha or spar did not write are not recorded while
// the rest is counted; a clone without the base cannot compare the changed
// files and commits but still shows what the record gives.
func TestGoalCostSummaryAC9_InputProblems(t *testing.T) {
	const v = "cost-input-problem-wrong"
	refused := func(label, want string, args ...string) {
		t.Helper()
		code, out, errs := gcHa(append([]string{"cost"}, args...)...)
		if code != exitRefused || out != "" || !strings.Contains(errs, want) {
			t.Errorf("jaekit:violation=%s %s: exit %d, stdout %q, stderr %q; want 65, no summary and %q named", v, label, code, out, errs, want)
		}
	}
	// Not started.
	gcRepo(t, "standard")
	doc := gcCostJSON(t, v, gcGoal)
	if doc["verification"] != nil || gcAt(doc, "unavailable", "verification") != "not_started" || gcAt(doc, "kind", "required_criteria") != 2.0 ||
		gcAt(doc, "kind", "assurance") != "standard" || gcAt(doc, "kind", "rules") != nil {
		t.Errorf("jaekit:violation=%s a goal without a start record gives %v", v, doc)
	}
	gcContains(t, v, "a goal without a start record", gcCost(t, v, gcGoal), "- no run record: the goal has not started", "- criteria: 2 required, 1 optional")

	f := gcBuild(t)
	// Skipped folders and files are named.
	doc = gcCostJSON(t, v, gcGoal, "--series", gcSeries)
	gcExpect(t, v, "skipped", gcAt(doc, "review", "series", 0, "skipped"), []any{
		map[string]any{"name": "notes.txt", "why": "not a review-gate/v1 file"},
		map[string]any{"name": "other-tool/", "why": "round.json is not a review-round/v1 record"},
	})
	// A missing series and a series that is a file.
	refused("a missing series", "docs/reviews/missing", gcGoal, "--series", "docs/reviews/missing")
	refused("a series that is a file", gcSeries+"/notes.txt", gcGoal, "--series", gcSeries+"/notes.txt")
	// Broken round files, each in its own copy of the series.
	broken := map[string]func(dir string){
		"round.json":    func(dir string) { f.put(dir+"/02-fixes/round.json", `{"schema": "review-round/v1", "stage": `) },
		"findings.json": func(dir string) { f.put(dir+"/02-fixes/findings.json", `[{"id": "claude-F1", "class": `) },
		"ledger.jsonl":  func(dir string) { f.put(dir+"/01-candidate/ledger.jsonl", "{\"finding\": \"codex-F2\"\n") },
		"waivers.jsonl": func(dir string) { f.put(dir+"/01-candidate/waivers.jsonl", "not json\n") },
		"extensions":    func(dir string) { f.put(dir+"/extensions.jsonl", "{\"stage\": 1}\n") },
		"lost findings": func(dir string) { os.Remove(filepath.Join(f.dir, dir, "02-fixes", "findings.json")) },
	}
	for name, breakIt := range broken {
		dir := "docs/reviews/broken-" + strings.ReplaceAll(name, " ", "-")
		f.series(dir)
		breakIt(dir)
		want := map[string]string{"round.json": "02-fixes", "findings.json": "02-fixes", "ledger.jsonl": "01-candidate/ledger.jsonl",
			"waivers.jsonl": "01-candidate/waivers.jsonl", "extensions": "extensions.jsonl", "lost findings": "findings.json"}[name]
		refused("a broken "+name, want, gcGoal, "--series", dir)
	}
	// Fields an older spar did not write.
	f.series("docs/reviews/older")
	older := map[string]any{"schema": "review-round/v1", "stage": "candidate", "sequence": 1, "dry_run": false,
		"started_at": "2026-01-01T00:00:00Z", "finished_at": "2026-01-01T00:01:00Z",
		"reviewers": map[string]any{"claude": map[string]any{"status": "completed"}}}
	f.putJSON("docs/reviews/older/01-candidate/round.json", older)
	f.putJSON("docs/reviews/older/01-candidate/findings.json", []any{map[string]any{"id": "claude-F1", "class": "defect"}})
	doc = gcCostJSON(t, v, gcGoal, "--series", "docs/reviews/older")
	gcExpect(t, v, "an older round", gcAt(doc, "review", "rounds", 0), map[string]any{"series": "docs/reviews/older", "round": "01-candidate",
		"stage": "candidate", "sequence": 1, "counted": nil, "rerun_of": nil, "started_at": "2026-01-01T00:00:00Z", "finished_at": "2026-01-01T00:01:00Z",
		"seconds": 60, "reviewers": []any{map[string]any{"name": "claude", "status": "completed", "elapsed_seconds": nil, "attempts": nil, "fallback_used": nil}}})
	gcExpect(t, v, "the candidate stage of an older round", gcAt(doc, "review", "stages", 2), map[string]any{"stage": "candidate", "counted": 0, "reruns": 0, "counted_not_recorded": 1})
	gcExpect(t, v, "an older finding", gcAt(doc, "findings", "classes", 0, "location_not_recorded"), 1)
	gcExpect(t, v, "an older reviewer's time", gcAt(doc, "review", "reviewer_seconds", 0), map[string]any{"reviewer": "claude", "seconds": 40, "rounds": 2, "not_recorded": 1})
	gcContains(t, v, "an older round", gcCost(t, v, gcGoal, "--series", "docs/reviews/older"),
		"    - claude: completed, not recorded, not recorded, not recorded", "counting not recorded")
	// A shallow clone: Git cannot compare the base with HEAD.
	records := f.read(gcGoal + "/runs.jsonl")
	clone := filepath.Join(t.TempDir(), "shallow")
	f.git("clone", "-q", "--depth", "1", "file://"+filepath.ToSlash(f.dir), clone)
	if err := os.WriteFile(filepath.Join(clone, gcGoal, "runs.jsonl"), records, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Chdir(clone)
	doc = gcCostJSON(t, v, gcGoal)
	if gcAt(doc, "kind", "changed_files") != nil || gcAt(doc, "unavailable", "kind.changed_files") != "cannot_compare" ||
		gcAt(doc, "unavailable", "kind.changed_commits") != "cannot_compare" || gcAt(doc, "verification", "runs", "check", "pass") == nil ||
		gcAt(doc, "kind", "rules") != "run-rules/5" {
		t.Errorf("jaekit:violation=%s a shallow clone gives kind %v, verification %v", v, doc["kind"], doc["verification"])
	}
	gcContains(t, v, "a shallow clone", gcCost(t, v, gcGoal), "- changed: Git cannot compare base "+f.base[:12]+" with HEAD")
	t.Chdir(f.dir)
	// A broken run record.
	path := filepath.Join(f.dir, gcGoal, "runs.jsonl")
	edited := bytes.Replace(records, []byte(gcWaitOne), []byte("edited by hand"), 1)
	if err := os.WriteFile(path, edited, 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	refused("a broken run record", "runs.jsonl", gcGoal)
}

// AC-11: the contract defines the command, its items, their sources and its
// input problems, and names the Spar formats it reads with their versions;
// the guides say how to read it, to compare goals of the same tier and
// similar size, and that a before-and-after comparison of a few goals is not
// a causal effect.
func TestGoalCostSummaryAC11_ContractAndGuides(t *testing.T) {
	const v = "cost-docs-missing"
	read := func(path string) string {
		t.Helper()
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		return string(data)
	}
	contract := read("../../contracts/run-record.md")
	gcContains(t, v, "contracts/run-record.md", contract,
		"### 1.7 비용 요약 (`ha cost`)", "`ha cost <goal> [--series <folder>]... [--format md|json] [--lang ko|en]`",
		"`review-round/v1`", "`review-gate/v1`", "`cost/v1`", "| `ha cost <goal>",
		"**항목과 출처.**", "**입력 문제.**", "토큰", "https://github.com/jgoneit/jaekit/issues/174")
	for path, wants := range map[string][]string{
		"../../guides/USAGE.md":    {"## 목표 하나의 비용 보기", "ha cost docs/specs/<goal> --series docs/reviews/<goal>", "같은 등급", "비슷한 크기", "인과 효과가 아닙니다"},
		"../../guides/USAGE.en.md": {"## Reading one goal's cost", "ha cost docs/specs/<goal> --series docs/reviews/<goal>", "same tier", "similar size", "not a causal effect"},
	} {
		gcContains(t, v, path, read(path), wants...)
	}
}

// gcSpar runs the spar binary built from this tree in the fixture
// repository; any exit but want is a broken setup.
func (f *gcFixture) spar(bin string, want int, args ...string) string {
	f.t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = f.dir
	out, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		f.t.Fatalf("setup: spar %v: %v", args, err)
	}
	if code != want {
		f.t.Fatalf("setup: spar %v: exit %d, want %d: %s", args, code, want, out)
	}
	return string(out)
}

// gcAnswer records the host reviewer's answer for a round: it checks every
// angle the round asks for and reports the given findings.
func (f *gcFixture) answer(bin, round, head string, findings ...map[string]any) {
	f.t.Helper()
	var rec map[string]any
	if err := json.Unmarshal(f.read(round+"/round.json"), &rec); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	checked := []any{}
	for _, angle := range gcAt(rec, "config", "angles").([]any) {
		checked = append(checked, map[string]any{"angle": angle, "result": "synthetic check"})
	}
	list := []any{}
	for _, item := range findings {
		list = append(list, item)
	}
	reply := map[string]any{"reviewed_head": head, "verdict": "blocking_findings", "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": list, "limits": "synthetic reviewer"}
	if len(findings) == 0 {
		reply["verdict"] = "no_blocking_findings"
	}
	path := filepath.Join(f.t.TempDir(), "reply.json")
	data, _ := json.Marshal(reply)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	f.spar(bin, 0, "record", round, "host", "--result", path)
}

func gcRaw(id, class, location, linkKind, link, introduced string) map[string]any {
	return map[string]any{"id": id, "proposed_class": class, "angle": "requirement-counterexample", "location": location,
		"claim": "synthetic claim " + id, "failure_scenario": "synthetic scenario", "evidence": location, "link_kind": linkKind,
		"link": link, "introduced_by_change": introduced, "regression": introduced, "check_impact": "none", "confidence": "high"}
}

// AC-10: a goal carried with ha whose review series a real spar wrote,
// with a fixes round, an automatically deferred finding and a disposition
// quoting the user: the summary's numbers are the ones both tools recorded.
func TestGoalCostSummaryAC10_RealSparRecords(t *testing.T) {
	const v = "cost-not-matching-records"
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("go is not available: %v", err)
	}
	tools := t.TempDir()
	spar := filepath.Join(tools, "spar")
	build := exec.Command("go", "build", "-o", spar, "./cmd/spar")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("setup: go build ./cmd/spar: %v: %s", err, out)
	}
	config := filepath.Join(tools, "spar.toml")
	if err := os.WriteFile(config, []byte("[[reviewers]]\nname = \"host\"\npreset = \"native\"\n"), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
	t.Setenv("SPAR_CONFIG", config)
	t.Setenv("HOME", tools)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(tools, "config"))
	f := gcRepo(t, "standard")
	f.base = f.head
	f.must("start", exitOK, "start", gcGoal, "--request", gcRequest, "--rules", "run-rules/5")
	f.must("baseline", exitOK, "check", gcGoal, "--baseline", "AC-1")
	f.put("src/value.txt", "ok\n")
	work := f.commit("work")
	f.must("", exitOK, "check", gcGoal, "AC-1", "AC-2")
	series := gcSeries
	run := func(stage, out, base string) {
		f.spar(spar, 0, "run", "--stage", stage, "--base", base, "--head", f.head, "--criteria", gcGoal+"/SPEC.md",
			"--check-path", "tests/**", "--out", series+"/"+out, "--repo", ".")
	}
	run("candidate", "01-candidate", f.base)
	f.answer(spar, series+"/01-candidate", f.head,
		gcRaw("F1", "defect", "head/src/value.txt:1", "criterion", "AC-1", "yes"),
		gcRaw("F2", "defect", "head/tests/junit.sh:1", "none", "", "no"),
		gcRaw("F3", "requirement-question", "head/src/keep.txt:1", "criterion", "AC-2", "no"))
	f.must("wait", exitOK, "note", gcGoal, "wait", "--reason", gcWaitOne)
	f.must("input", exitOK, "note", gcGoal, "input", "--quote", gcQuestionText)
	f.put("tests/added.sh", "# a synthetic check line\ngrep -qx ok src/value.txt\n")
	f.put("src/fixed.txt", "fixed\n")
	fix := f.commit("fix")
	f.spar(spar, 0, "triage", series+"/01-candidate", "host-F1", "fix", "--commit", fix, "--failed-before", "synthetic before", "--passed-after", "synthetic after")
	f.spar(spar, 0, "triage", series+"/01-candidate", "host-F3", "answer", "--user-quote", gcQuestionText)
	run("fixes", "02-fixes", work)
	f.answer(spar, series+"/02-fixes", f.head)
	run("candidate", "03-candidate", f.base)
	f.answer(spar, series+"/03-candidate", f.head)
	f.spar(spar, 0, "gate", series+"/01-candidate", series+"/02-fixes", series+"/03-candidate", "--expect-head", f.head,
		"--out", series+"/gate.json", "--repo", ".")
	f.must("", exitOK, "check", gcGoal, "AC-1", "AC-2")
	f.must("review", exitOK, "note", gcGoal, "review", "--gate", series+"/gate.json")
	f.must("done", exitOK, "done", gcGoal)

	// What the two tools recorded, read by this test.
	type recordedRound struct {
		stage   string
		counted bool
		seconds map[string]any
		classes map[string]int
		final   map[string]map[string]any
	}
	var rounds []recordedRound
	for _, name := range []string{"01-candidate", "02-fixes", "03-candidate"} {
		var rec map[string]any
		if err := json.Unmarshal(f.read(series+"/"+name+"/round.json"), &rec); err != nil {
			t.Fatalf("setup: %v", err)
		}
		rr := recordedRound{stage: rec["stage"].(string), counted: rec["counted"].(bool), seconds: map[string]any{},
			classes: map[string]int{}, final: map[string]map[string]any{}}
		for reviewer, state := range rec["reviewers"].(map[string]any) {
			rr.seconds[reviewer] = gcAt(state, "elapsed_seconds")
		}
		var findings []map[string]any
		if err := json.Unmarshal(f.read(series+"/"+name+"/findings.json"), &findings); err != nil {
			t.Fatalf("setup: %v", err)
		}
		for _, fd := range findings {
			rr.classes[fd["class"].(string)]++
		}
		for _, l := range gcLines(t, f.read(series+"/"+name+"/ledger.jsonl")) {
			rr.final[l["finding"].(string)] = l
		}
		rounds = append(rounds, rr)
	}
	var gate map[string]any
	if err := json.Unmarshal(f.read(series+"/gate.json"), &gate); err != nil {
		t.Fatalf("setup: %v", err)
	}
	first := rounds[0]
	if rounds[1].stage != "fixes" || gcAt(first.final["host-F2"], "auto") != true || gcAt(first.final["host-F3"], "user_quote") != gcQuestionText ||
		gcAt(gate, "growth", "added_lines", "check") == 0.0 || gate["state"] != "complete" {
		t.Fatalf("setup: the spar records lack a fixes round, an automatic deferral, a quoted disposition, check-path growth or a complete gate: %v %v %v",
			first.final, gate["growth"], gate["state"])
	}

	doc := gcCostJSON(t, v, gcGoal, "--series", series)
	c := gcCount(t, f.read(gcGoal+"/runs.jsonl"))
	gcExpect(t, v, "verification runs", gcAt(doc, "verification", "runs"), c.runs)
	gcExpect(t, v, "verification time", gcAt(doc, "verification", "duration_ms", "total"), c.check+c.baseline)
	gcExpect(t, v, "completion record", gcAt(doc, "kind", "completion_record"), f.seqs["done"])
	gcExpect(t, v, "gate", gcAt(doc, "review", "series", 0, "gates", 0, "recorded_seq"), f.seqs["review"])
	stages := map[string]int{}
	for _, rr := range rounds {
		if rr.counted {
			stages[rr.stage]++
		}
	}
	gcExpect(t, v, "candidate rounds", gcAt(doc, "review", "stages", 2, "counted"), stages["candidate"])
	gcExpect(t, v, "fixes rounds", gcAt(doc, "review", "stages", 3, "counted"), stages["fixes"])
	for i, rr := range rounds {
		gcExpect(t, v, fmt.Sprintf("round %d host seconds", i+1), gcAt(doc, "review", "rounds", i, "reviewers", 0, "elapsed_seconds"), rr.seconds["host"])
	}
	classes := map[string]int{}
	auto, answered := 0, 0
	for _, rr := range rounds {
		for class, n := range rr.classes {
			classes[class] += n
		}
		for _, e := range rr.final {
			if e["auto"] == true {
				auto++
			}
			if e["disposition"] == "answer" {
				answered++
			}
		}
	}
	for i, class := range []string{"defect", "requirement-question", "hardening", "note"} {
		gcExpect(t, v, class+" findings", gcAt(doc, "findings", "classes", i, "total"), classes[class])
	}
	gcExpect(t, v, "automatic deferrals", gcAt(doc, "findings", "classes", 2, "dispositions", "auto_defer"), auto)
	gcExpect(t, v, "answers", gcAt(doc, "findings", "classes", 1, "dispositions", "answer"), answered)
	gcExpect(t, v, "growth", gcAt(doc, "findings", "growth_total"), gcAt(gate, "growth", "added_lines"))
	gcExpect(t, v, "requirement question", gcAt(doc, "questions", "requirement_questions", 0, "user_quote"), gcQuestionText)
	gcExpect(t, v, "waits", gcAt(doc, "questions", "by_source", "wait"), 1)
	gcExpect(t, v, "budget", gcAt(doc, "verification", "budget"), gcStatusBudget(gcStatus(t)))
}
