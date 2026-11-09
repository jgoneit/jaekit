package main

// Checks of the goal review-findings-report on the Seal side: a review note
// keeps the review gate's lists of deferred findings and findings without a
// disposition (AC-2), `ha report` shows them on its `To review` line (AC-3,
// AC-4), says so when a gate had no lists (AC-5) or a stored list cannot be
// read (AC-6), and refuses a gate whose lists are malformed (AC-6). The
// tests drive the ha command line (runMain) on synthetic repositories and
// write review gates in the public review-gate/v1 form, as `spar gate`
// writes them, reusing the pure gate helpers of review_gate_test.go; they
// never run Spar or a reviewer. A change check names the property it
// observes on the first line of each failure (jaekit:violation=<id>). A
// setup that breaks (no git, a file, repository, command or record that
// cannot be made or read, an ha command the setup needs that fails) skips a
// change check, which the adapter reads as inconclusive; it fails a check of
// kept behavior (AC-4, AC-11), so a broken setup never passes one.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const (
	rfrSeries = "demo-rounds"
	rfrIssue  = "https://github.com/example/demo/issues/9"
	rfrTarget = "later\nsecond line"
)

// rfrFixture is a run-rules/5 goal like the one of review_gate_test.go,
// whose setup failures stop the check through stop.
type rfrFixture struct {
	t                *testing.T
	stop             func(format string, args ...any)
	dir, gates       string
	base, work, head string
	files            int
}

// rfrChange is a fixture for a change check: a broken setup skips.
func rfrChange(t *testing.T, tier string) *rfrFixture {
	t.Helper()
	return newRFR(t, tier, func(format string, args ...any) { t.Helper(); t.Skipf("setup: "+format, args...) })
}

// rfrKeep is a fixture for a check of kept behavior: a broken setup fails.
func rfrKeep(t *testing.T, tier string) *rfrFixture {
	t.Helper()
	return newRFR(t, tier, func(format string, args ...any) { t.Helper(); t.Fatalf("setup: "+format, args...) })
}

func newRFR(t *testing.T, tier string, stop func(string, ...any)) *rfrFixture {
	t.Helper()
	f := &rfrFixture{t: t, stop: stop}
	if _, err := exec.LookPath("git"); err != nil {
		stop("git is not available: %v", err)
	}
	f.dir, f.gates = t.TempDir(), t.TempDir()
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
	f.commit("base")
	t.Chdir(f.dir)
	if code, out := f.ha("start", rgGoal, "--request", "implement the synthetic goal", "--rules", "run-rules/5"); code != exitOK {
		stop("ha start exit %d: %s", code, out)
	}
	f.base = f.git("rev-parse", "HEAD")
	f.put("src/work.txt", "one\n")
	f.work = f.commit("work")
	f.put("src/work.txt", "two\n")
	f.head = f.commit("fix")
	if code, out := f.ha("check", rgGoal, "AC-1"); code != exitOK {
		stop("ha check AC-1 exit %d: %s", code, out)
	}
	return f
}

func (f *rfrFixture) put(rel, body string) {
	f.t.Helper()
	p := filepath.Join(f.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.stop("%v", err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		f.stop("%v", err)
	}
}

func (f *rfrFixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.stop("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *rfrFixture) commit(message string) string {
	f.t.Helper()
	f.git("add", "-A")
	f.git("commit", "-q", "-m", message)
	return f.git("rev-parse", "HEAD")
}

// ha runs the ha command line in the fixture's repository; a test may hold
// two fixtures.
func (f *rfrFixture) ha(args ...string) (int, string) {
	f.t.Helper()
	if err := os.Chdir(f.dir); err != nil {
		f.stop("%v", err)
	}
	var out, errb bytes.Buffer
	code := runMain(args, &out, &errb)
	return code, out.String() + errb.String()
}

func (f *rfrFixture) docs() []rgDoc {
	f.t.Helper()
	var out []rgDoc
	for _, name := range []string{rgGoal + "/SPEC.md", rgGoal + "/note.md"} {
		data, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(name)))
		if err != nil {
			f.stop("%v", err)
		}
		out = append(out, rgDoc{name, rgSHA(data)})
	}
	return out
}

// chained is a complete gate of the candidate, fixes and cumulative rounds
// up to the head, as review_gate_test.go makes it.
func (f *rfrFixture) chained() rgGate {
	f.t.Helper()
	g := rgGate{state: "complete", expect: f.head, last: f.head}
	for _, r := range []rgRound{{stage: "candidate", base: f.base, head: f.work}, {stage: "fixes", base: f.work, head: f.head},
		{stage: "candidate", base: f.base, head: f.head}} {
		r.docs, r.required, r.completed = f.docs(), 3, 3
		g.rounds = append(g.rounds, r)
	}
	return g
}

// absolute moves a gate's round paths and open findings under a folder
// outside the repository, as spar gate writes them when it is given
// absolute round folders.
func (f *rfrFixture) absolute(v map[string]any) map[string]any {
	prefix := filepath.ToSlash(filepath.Join(f.gates, rfrSeries))
	for _, r := range v["rounds"].([]any) {
		round := r.(map[string]any)
		round["path"] = strings.Replace(round["path"].(string), rgRoundFolder, prefix, 1)
	}
	var open []any
	for _, o := range v["open_findings"].([]any) {
		open = append(open, strings.Replace(o.(string), rgRoundFolder, prefix, 1))
	}
	if open != nil {
		v["open_findings"] = open
	}
	return v
}

func rfrItem(round, id, class string, to ...string) map[string]any {
	m := map[string]any{"series": rfrSeries, "round": round, "id": id, "class": class}
	if len(to) > 0 {
		m["to"] = to[0]
	}
	return m
}

// withLists is the gate value with the two lists and the automatic
// deferral count.
func withLists(v map[string]any, deferred, unhandled []any, auto int) map[string]any {
	v["deferred_findings"] = deferred
	v["unhandled_findings"] = unhandled
	v["auto_deferred"] = auto
	return v
}

func (f *rfrFixture) writeGate(value any) string {
	f.t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		f.stop("%v", err)
	}
	return f.writeBytes(append(data, '\n'))
}

func (f *rfrFixture) writeBytes(data []byte) string {
	f.t.Helper()
	f.files++
	p := filepath.Join(f.gates, fmt.Sprintf("gate-%d.json", f.files))
	if err := os.WriteFile(p, data, 0o644); err != nil {
		f.stop("%v", err)
	}
	return p
}

// note records a gate file the setup needs.
func (f *rfrFixture) note(value any) {
	f.t.Helper()
	path := f.writeGate(value)
	if code, out := f.ha("note", rgGoal, "review", "--gate", path); code != exitOK {
		f.stop("ha note review exit %d: %s", code, out)
	}
}

func (f *rfrFixture) records() []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"))
	if err != nil {
		f.stop("%v", err)
	}
	return data
}

func (f *rfrFixture) lastLine() (int, map[string]json.RawMessage) {
	f.t.Helper()
	lines := bytes.Split(bytes.TrimSuffix(f.records(), []byte("\n")), []byte("\n"))
	var line map[string]json.RawMessage
	if err := json.Unmarshal(lines[len(lines)-1], &line); err != nil {
		f.stop("the last record line: %v", err)
	}
	return len(lines), line
}

// rfrJudgment is what `ha status --format json` and `ha done` decide.
type rfrJudgment struct {
	statusCode int
	status     string
	reasons    []string // code/class
	doneCode   int
	completed  bool
}

func (f *rfrFixture) status() (int, map[string]any) {
	f.t.Helper()
	code, out := f.ha("status", rgGoal, "--format", "json")
	var value map[string]any
	if err := json.Unmarshal([]byte(out), &value); err != nil {
		f.stop("ha status exit %d gave no status JSON: %v: %s", code, err, out)
	}
	return code, value
}

func (f *rfrFixture) judgment(done bool) rfrJudgment {
	f.t.Helper()
	code, value := f.status()
	j := rfrJudgment{statusCode: code}
	j.status, _ = value["status"].(string)
	list, _ := value["reasons"].([]any)
	for _, item := range list {
		r, _ := item.(map[string]any)
		j.reasons = append(j.reasons, fmt.Sprintf("%v/%v/%v", r["criterion"], r["code"], r["class"]))
	}
	if done {
		j.doneCode, _ = f.ha("done", rgGoal)
		if j.doneCode == exitInternal {
			f.stop("ha done exit %d", j.doneCode)
		}
		_, after := f.status()
		j.completed = after["completion_record"] != nil
	}
	return j
}

// toReview is the `To review` line of `ha report` in a language.
func (f *rfrFixture) toReview(lang string) string {
	f.t.Helper()
	code, out := f.ha("report", rgGoal, "--lang", lang)
	if code != exitOK {
		f.stop("ha report --lang %s exit %d: %s", lang, code, out)
	}
	label := map[string]string{"en": "To review: ", "ko": "직접 볼 것: "}[lang]
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, label) {
			return strings.TrimPrefix(line, label)
		}
	}
	f.stop("ha report --lang %s has no %q line: %s", lang, label, out)
	return ""
}

// The lists of the AC-2 and AC-3 gates.
func rfrLists() (deferred, unhandled []any) {
	deferred = []any{rfrItem("01-candidate", "claude-F1", "defect", rfrIssue), rfrItem("03-candidate", "codex-F4", "note", rfrTarget)}
	// A field Spar does not write stays out of the note.
	withText := rfrItem("01-candidate", "codex-F2", "note")
	withText["claim"] = rgFindingText
	unhandled = []any{withText, rfrItem("02-fixes", "agy-F3", "hardening")}
	return deferred, unhandled
}

// AC-2: the review note keeps the two lists and the automatic deferral count
// with no finding text and no absolute path, and the judgment of the same
// record is the judgment of a gate without the lists.
func TestReviewFindingsReportAC2_NoteKeepsTheLists(t *testing.T) {
	const id = "note-drops-finding-lists"
	deferred, unhandled := rfrLists()
	for _, tc := range []struct {
		name string
		gate func(f *rfrFixture) rgGate
	}{
		{"complete", func(f *rfrFixture) rgGate { return f.chained() }},
		{"blocked", func(f *rfrFixture) rgGate {
			g := f.chained()
			g.state, g.defect = "blocked", 1
			return g
		}},
	} {
		with := rfrChange(t, "standard")
		with.note(with.absolute(withLists(tc.gate(with).gateValue(), deferred, unhandled, 2)))
		_, line := with.lastLine()
		var review map[string]json.RawMessage
		if err := json.Unmarshal(line["review"], &review); err != nil {
			rfrSetupf(t, "the review note: %v", err)
		}
		var got any
		if raw, ok := review["findings"]; !ok || json.Unmarshal(raw, &got) != nil {
			t.Errorf("jaekit:violation=%s %s: the review note has no finding lists: %s", id, tc.name, line["review"])
		} else {
			want := map[string]any{
				"deferred":      []any{rfrItem("01-candidate", "claude-F1", "defect", rfrIssue), rfrItem("03-candidate", "codex-F4", "note", rfrTarget)},
				"unhandled":     []any{rfrItem("01-candidate", "codex-F2", "note"), rfrItem("02-fixes", "agy-F3", "hardening")},
				"auto_deferred": float64(2)}
			if !reflect.DeepEqual(roundTrip(got), roundTrip(want)) {
				t.Errorf("jaekit:violation=%s %s: the review note keeps %v, want %v", id, tc.name, got, want)
			}
		}
		records := with.records()
		for _, absent := range []string{rgFindingText, filepath.ToSlash(with.gates), with.gates} {
			if bytes.Contains(records, []byte(absent)) {
				t.Errorf("jaekit:violation=%s %s: the record keeps %q", id, tc.name, absent)
			}
		}

		without := rfrChange(t, "standard")
		without.note(without.absolute(tc.gate(without).gateValue()))
		if a, b := with.judgment(true), without.judgment(true); !reflect.DeepEqual(a, b) {
			t.Errorf("jaekit:violation=%s %s: the judgment with the lists is %+v, without them %+v", id, tc.name, a, b)
		}
	}
}

func roundTrip(v any) any {
	data, _ := json.Marshal(v)
	var out any
	_ = json.Unmarshal(data, &out)
	return out
}

func rfrSetupf(t *testing.T, format string, args ...any) {
	t.Helper()
	t.Skipf("setup: "+format, args...)
}

// The report items of the AC-3 lists, in each language.
var rfrItems = map[string][3]string{
	"en": {
		"review findings deferred: demo-rounds/01-candidate claude-F1 [defect] → " + rfrIssue + ", demo-rounds/03-candidate codex-F4 [note] → later\\nsecond line",
		"auto-deferred hardening findings: 2",
		"review findings without a disposition: demo-rounds/01-candidate codex-F2 [note], demo-rounds/02-fixes agy-F3 [hardening]"},
	"ko": {
		"보류한 검토 지적: demo-rounds/01-candidate claude-F1 [defect] → " + rfrIssue + ", demo-rounds/03-candidate codex-F4 [note] → later\\nsecond line",
		"자동 보류한 hardening 지적: 2개",
		"처리 없는 검토 지적: demo-rounds/01-candidate codex-F2 [note], demo-rounds/02-fixes agy-F3 [hardening]"},
}

// rfrUserItem is the item an open requirement-question puts first.
var rfrUserItem = map[string]string{"en": "user decisions: `review_open_question`", "ko": "사용자 결정: `review_open_question`"}

// AC-3: the `To review` line shows the last review note's deferred findings,
// automatic deferral count and findings without a disposition after the
// existing items, in both languages, and any one of the three is not
// `none`.
func TestReviewFindingsReportAC3_ReportShowsTheLists(t *testing.T) {
	const id = "report-hides-finding-lists"
	deferred, unhandled := rfrLists()
	f := rfrChange(t, "standard")
	g := f.chained()
	g.state, g.question = "blocked", 1
	f.note(withLists(g.gateValue(), deferred, unhandled, 2))
	for _, lang := range []string{"en", "ko"} {
		items := rfrItems[lang]
		want := strings.Join([]string{rfrUserItem[lang], items[0], items[1], items[2]}, "; ")
		if got := f.toReview(lang); got != want {
			t.Errorf("jaekit:violation=%s --lang %s: the line is %q, want %q", id, lang, got, want)
		}
	}

	// Each of the three alone, then nothing: the last recorded review note
	// decides the whole line.
	none := map[string]string{"en": "none", "ko": "없음"}
	for i, lists := range []struct {
		deferred, unhandled []any
		auto                int
	}{{deferred: deferred[:1], unhandled: []any{}}, {deferred: []any{}, unhandled: []any{}, auto: 2},
		{deferred: []any{}, unhandled: unhandled[1:]}, {deferred: []any{}, unhandled: []any{}}} {
		f.note(withLists(f.chained().gateValue(), lists.deferred, lists.unhandled, lists.auto))
		for _, lang := range []string{"en", "ko"} {
			want := none[lang]
			switch i {
			case 0:
				want = strings.SplitN(rfrItems[lang][0], ", ", 2)[0]
			case 1:
				want = rfrItems[lang][1]
			case 2:
				want = strings.Replace(rfrItems[lang][2], "demo-rounds/01-candidate codex-F2 [note], ", "", 1)
			}
			if got := f.toReview(lang); got != want {
				t.Errorf("jaekit:violation=%s --lang %s: after note %d the line is %q, want %q", id, lang, i+1, got, want)
			}
		}
	}
}

// AC-4: a gate whose lists are both empty leaves the `To review` line as it
// was before this change.
func TestReviewFindingsReportAC4_EmptyListsKeepTheLine(t *testing.T) {
	f := rfrKeep(t, "standard")
	f.note(withLists(f.chained().gateValue(), []any{}, []any{}, 0))
	want := map[string]string{"en": "none", "ko": "없음"}
	for _, lang := range []string{"en", "ko"} {
		if got := f.toReview(lang); got != want[lang] {
			t.Errorf("empty lists, --lang %s: the line is %q, want %q", lang, got, want[lang])
		}
	}
	g := f.chained()
	g.state, g.question = "blocked", 1
	f.note(withLists(g.gateValue(), []any{}, []any{}, 0))
	for _, lang := range []string{"en", "ko"} {
		if got := f.toReview(lang); got != rfrUserItem[lang] {
			t.Errorf("empty lists with an open question, --lang %s: the line is %q, want %q", lang, got, rfrUserItem[lang])
		}
	}
}

// AC-5: a gate from a Spar without the lists is recorded as before, and the
// `To review` line says its lists were not recorded, with the note's seq.
func TestReviewFindingsReportAC5_OldGateSaysNotRecorded(t *testing.T) {
	const id = "old-gate-reads-as-none"
	f := rfrChange(t, "standard")
	path := f.writeGate(f.chained().gateValue())
	before := f.records()
	if code, out := f.ha("note", rgGoal, "review", "--gate", path); code != exitOK {
		if code == exitInternal {
			rfrSetupf(t, "ha note review exit %d: %s", code, out)
		}
		t.Fatalf("jaekit:violation=%s a gate without the lists is not recorded: exit %d: %s", id, code, out)
	}
	seq, _ := f.lastLine()
	if after := f.records(); !bytes.HasPrefix(after, before) || len(after) == len(before) {
		t.Fatalf("jaekit:violation=%s the gate without the lists did not add one record line", id)
	}
	want := map[string]string{
		"en": fmt.Sprintf("the review gate's finding lists were not recorded (review note seq %d)", seq),
		"ko": fmt.Sprintf("검토 gate의 지적 목록이 기록되지 않음(review note seq %d)", seq)}
	for _, lang := range []string{"en", "ko"} {
		if got := f.toReview(lang); got != want[lang] {
			t.Errorf("jaekit:violation=%s --lang %s: the line is %q, want %q", id, lang, got, want[lang])
		}
	}
}

// AC-6: a gate whose lists are malformed is refused with 65 and nothing
// recorded; a stored list that cannot be read leaves the judgment as it is,
// and the `To review` line says it cannot be read, with the note's seq.
func TestReviewFindingsReportAC6_MalformedLists(t *testing.T) {
	const id = "malformed-lists-accepted"
	f := rfrChange(t, "standard")
	deferred, unhandled := rfrLists()
	item := func(drop string, set map[string]any) []any {
		m := rfrItem("01-candidate", "claude-F1", "defect", rfrIssue)
		delete(m, drop)
		for k, v := range set {
			m[k] = v
		}
		return []any{m}
	}
	cases := []struct {
		name  string
		apply func(v map[string]any)
	}{
		{"deferred_findings is not a list", func(v map[string]any) { v["deferred_findings"] = map[string]any{} }},
		{"unhandled_findings is null", func(v map[string]any) { v["unhandled_findings"] = nil }},
		{"an item is not an object", func(v map[string]any) { v["unhandled_findings"] = []any{"claude-F1"} }},
		{"an empty finding ID", func(v map[string]any) { v["deferred_findings"] = item("", map[string]any{"id": ""}) }},
		{"a class outside the four", func(v map[string]any) { v["deferred_findings"] = item("", map[string]any{"class": "blocker"}) }},
		{"a deferral without to", func(v map[string]any) { v["deferred_findings"] = item("to", nil) }},
		{"a deferral with an empty to", func(v map[string]any) { v["deferred_findings"] = item("", map[string]any{"to": ""}) }},
		{"a round that is a path", func(v map[string]any) {
			v["deferred_findings"] = item("", map[string]any{"round": rgRoundFolder + "/01-candidate"})
		}},
		{"a series that is missing", func(v map[string]any) { v["deferred_findings"] = item("series", nil) }},
		{"a round that is not a string", func(v map[string]any) { v["deferred_findings"] = item("", map[string]any{"round": 1}) }},
		{"only one list", func(v map[string]any) { delete(v, "unhandled_findings") }},
		{"auto_deferred is not a count", func(v map[string]any) { v["auto_deferred"] = "2" }},
		{"auto_deferred is negative", func(v map[string]any) { v["auto_deferred"] = -1 }},
	}
	for _, tc := range cases {
		v := withLists(f.chained().gateValue(), deferred, unhandled, 2)
		tc.apply(v)
		path := f.writeGate(v)
		before := f.records()
		code, out := f.ha("note", rgGoal, "review", "--gate", path)
		if code == exitInternal {
			rfrSetupf(t, "%s: ha note review exit %d: %s", tc.name, code, out)
		}
		prefix := "ha: review gate " + path + ": "
		if code != exitRefused || !strings.Contains(out, prefix) || !strings.HasSuffix(strings.TrimSpace(out), "; nothing recorded") ||
			!bytes.Equal(f.records(), before) {
			t.Errorf("jaekit:violation=%s %s: exit %d, records unchanged %v: %s; want exit 65, %q…; nothing recorded",
				id, tc.name, code, bytes.Equal(f.records(), before), out, prefix)
		}
	}

	// A stored list a hand edit made unreadable.
	f.note(withLists(f.chained().gateValue(), deferred, unhandled, 2))
	seq, _ := f.lastLine()
	judged := f.judgment(false)
	stored := []string{`"not lists"`, `null`, `{"deferred":"x","unhandled":[],"auto_deferred":0}`,
		`{"deferred":[{"series":"s","round":"r","id":"","class":"defect","to":"t"}],"unhandled":[],"auto_deferred":0}`,
		`{"deferred":[],"unhandled":[],"auto_deferred":"two"}`, `{"deferred":[],"unhandled":[]}`}
	want := map[string]string{
		"en": fmt.Sprintf("the recorded review finding lists cannot be read (review note seq %d)", seq),
		"ko": fmt.Sprintf("기록된 검토 지적 목록을 읽을 수 없음(review note seq %d)", seq)}
	for _, raw := range stored {
		f.storeFindings(raw)
		if got := f.judgment(false); !reflect.DeepEqual(got, judged) {
			t.Errorf("jaekit:violation=%s stored %s: the judgment is %+v, want %+v", id, raw, got, judged)
		}
		for _, lang := range []string{"en", "ko"} {
			if got := f.toReview(lang); got != want[lang] {
				t.Errorf("jaekit:violation=%s stored %s, --lang %s: the line is %q, want %q", id, raw, lang, got, want[lang])
			}
		}
	}
	code, out := f.ha("done", rgGoal)
	if code == exitInternal {
		rfrSetupf(t, "ha done exit %d: %s", code, out)
	}
	if code != exitOK {
		t.Errorf("jaekit:violation=%s with an unreadable stored list ha done exits %d: %s", id, code, out)
	}
}

// storeFindings replaces the stored finding lists of the last record line,
// the review note, as a hand edit would; it is the last line, so the chain
// stays whole.
func (f *rfrFixture) storeFindings(raw string) {
	f.t.Helper()
	data := f.records()
	lines := bytes.Split(bytes.TrimSuffix(data, []byte("\n")), []byte("\n"))
	var line, review map[string]json.RawMessage
	if json.Unmarshal(lines[len(lines)-1], &line) != nil || json.Unmarshal(line["review"], &review) != nil {
		f.stop("the last record line is not a review note")
	}
	review["findings"] = json.RawMessage(raw)
	var err error
	if line["review"], err = json.Marshal(review); err != nil {
		f.stop("%v", err)
	}
	if lines[len(lines)-1], err = json.Marshal(line); err != nil {
		f.stop("%v", err)
	}
	if err := os.WriteFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"), append(bytes.Join(lines, []byte("\n")), '\n'), 0o644); err != nil {
		f.stop("%v", err)
	}
}

// AC-11: ha and its packages do not import Spar's, and the same record,
// bundle, Git and --lang give the same report.
func TestReviewFindingsReportAC11_ApartAndDeterministic(t *testing.T) {
	goTool, err := exec.LookPath("go")
	if err != nil {
		t.Fatalf("setup: go is not available: %v", err)
	}
	cmd := exec.Command(goTool, "list", "-deps", "github.com/jgoneit/jaekit/cmd/ha")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("setup: go list -deps: %v", err)
	}
	for _, dep := range strings.Fields(string(out)) {
		if strings.HasPrefix(dep, "github.com/jgoneit/jaekit/internal/spar") {
			t.Errorf("cmd/ha imports %s", dep)
		}
	}
	f := rfrKeep(t, "standard")
	deferred, unhandled := rfrLists()
	f.note(withLists(f.chained().gateValue(), deferred, unhandled, 2))
	for _, lang := range []string{"en", "ko"} {
		_, first := f.ha("report", rgGoal, "--lang", lang)
		_, second := f.ha("report", rgGoal, "--lang", lang)
		if first != second {
			t.Errorf("--lang %s: two reports of the same record differ:\n%s\n---\n%s", lang, first, second)
		}
	}
}
