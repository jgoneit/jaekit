package main

// Checks of the goal core-completion-integrity: a new rule version
// run-rules/6 keeps every run-rules/5 rule and changes four judgments (the
// start of the reviewed range, a review note after a completion, a done line
// that did not judge the line before it, and time after a completion). The
// tests drive the ha command line (runMain) on synthetic repositories built
// with the helpers of review_gate_test.go and write review gates in the
// public review-gate/v1 form; they never run Spar, a reviewer or a model.
// Where a judgment depends on time, the record made through the command line
// gets fixed times and its hash links are joined again: status computation
// never reads the current time, so such a record is computed as one written
// at those times. The opening line of an assertion's failure message names
// the property it observes (jaekit:violation=<id>); a failure without it is a
// broken setup, not an observation.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

const ciRequest = "implement the synthetic goal"

// ciStart starts the fixture's goal under run-rules/6. A Core that does not
// know the rule refuses the option (64, naming --rules): that is the observed
// violation. Any other failure is a broken setup.
func ciStart(f *rgFixture, violation string, extra ...string) {
	f.t.Helper()
	code, out := f.ha(append([]string{"start", rgGoal, "--request", ciRequest, "--rules", "run-rules/6"}, extra...)...)
	if code == exitUsage && strings.Contains(out, "--rules") {
		f.t.Fatalf("jaekit:violation=%s ha cannot start a run-rules/6 goal: exit %d: %s", violation, code, strings.TrimSpace(out))
	}
	if code != exitOK {
		f.t.Fatalf("setup: ha start exit %d: %s", code, out)
	}
	f.base = f.git("rev-parse", "HEAD")
}

// ciFixture is a run-rules/6 goal of the given tier with two code commits
// (work and head) and a passing AC-1 check at the head.
func ciFixture(t *testing.T, violation, tier string, prepare ...func(*rgFixture)) *rgFixture {
	t.Helper()
	f := rgRepo(t, tier, prepare...)
	ciStart(f, violation)
	f.put("src/work.txt", "one\n")
	f.work = f.commit("work")
	f.put("src/work.txt", "two\n")
	f.head = f.commit("fix")
	f.check()
	return f
}

// ciElapsed sets the elapsed limit of the fixture's plan.
func ciElapsed(limit string) func(*rgFixture) {
	return func(f *rgFixture) {
		f.put(rgGoal+"/PLAN.md", strings.Replace(rgPlan, "- 경과 시간: 4시간", "- 경과 시간: "+limit, 1))
	}
}

func ciRound(f *rgFixture, stage, base, head string) rgRound {
	return rgRound{stage: stage, base: base, head: head, docs: f.docs(), required: 3, completed: 3}
}

func ciGate(f *rgFixture, rounds ...rgRound) rgGate {
	return rgGate{state: "complete", expect: f.head, last: f.head, rounds: rounds}
}

// ciLine is one run record line: its bytes, its decoded object, the seq it
// had when it was read (its identity) and the identity of the line its
// record_head names (0 for none).
type ciLine struct {
	raw  []byte
	m    map[string]any
	id   int
	head int
}

func ciLines(f *rgFixture) []ciLine {
	f.t.Helper()
	var out []ciLine
	byHash := map[string]int{}
	for _, raw := range bytes.Split(bytes.TrimSuffix(f.records(), []byte("\n")), []byte("\n")) {
		var m map[string]any
		if err := json.Unmarshal(raw, &m); err != nil {
			f.t.Fatalf("setup: run record line: %v", err)
		}
		l := ciLine{raw: append([]byte(nil), raw...), m: m}
		l.id = l.seq()
		if h, ok := m["record_head"].(string); ok && h != "" {
			if l.head = byHash[h]; l.head == 0 {
				f.t.Fatalf("setup: line %d names a record head that is not in the record", l.id)
			}
		}
		byHash[rgSHA(l.raw)] = l.id
		out = append(out, l)
	}
	return out
}

func (l ciLine) seq() int {
	n, _ := l.m["seq"].(float64)
	if i, ok := l.m["seq"].(int); ok {
		return i
	}
	return int(n)
}

func (l ciLine) kind() string {
	k, _ := l.m["kind"].(string)
	return k
}

// ciWrite writes lines as the goal's run record.
func ciWrite(f *rgFixture, lines []ciLine) {
	f.t.Helper()
	var b bytes.Buffer
	for _, l := range lines {
		b.Write(l.raw)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"), b.Bytes(), 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
}

// ciRelink encodes lines[from:] again from their objects and joins the chain:
// seqs follow the line order, each line's prev is the sha256 of the line
// before it, and a done line's record_head names the same line as before, by
// that line's new bytes. Lines before from keep their bytes.
func ciRelink(f *rgFixture, lines []ciLine, from int) []ciLine {
	f.t.Helper()
	out := make([]ciLine, len(lines))
	copy(out, lines)
	raws := map[int][]byte{}
	for i := range out {
		if i >= from {
			m := out[i].m
			m["seq"] = i + 1
			if i == 0 {
				m["prev"] = nil
			} else {
				m["prev"] = rgSHA(out[i-1].raw)
			}
			if h := out[i].head; h != 0 {
				target, ok := raws[h]
				if !ok {
					f.t.Fatalf("setup: line %d names a record head that is not before it", i+1)
				}
				m["record_head"] = rgSHA(target)
			}
			raw, err := json.Marshal(m)
			if err != nil {
				f.t.Fatalf("setup: %v", err)
			}
			out[i].raw = raw
		}
		raws[out[i].id] = out[i].raw
	}
	return out
}

// ciClock is a fixed origin for retimed records.
var ciClock = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func ciAt(d time.Duration) string { return ciClock.Add(d).UTC().Format(time.RFC3339) }

// ciTime sets a line's time and, for a run, its start.
func ciTime(l ciLine, at time.Duration, started ...time.Duration) {
	l.m["at"] = ciAt(at)
	if len(started) > 0 {
		l.m["started_at"] = ciAt(started[0])
	}
}

// ciUnjudged is a copy of the done line d appended right after last: the
// same record head d judged, so it did not judge the line before it, as when
// two ha done runs compute the same record (#151).
func ciUnjudged(f *rgFixture, d, last ciLine) ciLine {
	f.t.Helper()
	m := map[string]any{}
	for k, v := range d.m {
		m[k] = v
	}
	m["seq"] = last.seq() + 1
	m["prev"] = rgSHA(last.raw)
	raw, err := json.Marshal(m)
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return ciLine{raw: raw, m: m, id: -d.id, head: d.head}
}

func ciLastSeq(f *rgFixture) int {
	f.t.Helper()
	lines := ciLines(f)
	return lines[len(lines)-1].seq()
}

func ciMustDone(f *rgFixture, label string) int {
	f.t.Helper()
	code, s := f.done()
	if code != exitOK || s.completion == nil {
		f.t.Fatalf("setup: %s: ha done exit %d, status %s, reasons %v", label, code, s.status, s.reasons)
	}
	return ciLastSeq(f)
}

func ciCompletion(s rgState) int {
	n, ok := s.completion.(float64)
	if !ok {
		return 0
	}
	return int(n)
}

// ciReason returns the status reason with the code, or nil.
func ciReason(s rgState, code string) map[string]any {
	list, _ := s.value["reasons"].([]any)
	for _, item := range list {
		if r, _ := item.(map[string]any); r != nil && r["code"] == code {
			return r
		}
	}
	return nil
}

// ciMarkdown runs ha with the arguments and returns its standard output.
func ciMarkdown(f *rgFixture, args ...string) string {
	f.t.Helper()
	var out, errb bytes.Buffer
	code := runMain(args, &out, &errb)
	if code == exitInternal || out.Len() == 0 {
		f.t.Fatalf("setup: ha %v exit %d: %s", args, code, errb.String())
	}
	return out.String()
}

// ciCompletionLine is the completion record line of `ha status`.
func ciCompletionLine(f *rgFixture) string {
	f.t.Helper()
	for _, line := range strings.Split(ciMarkdown(f, "status", rgGoal), "\n") {
		if strings.HasPrefix(line, "- completion record:") {
			return line
		}
	}
	f.t.Fatalf("setup: ha status printed no completion record line")
	return ""
}

func ciBudget(f *rgFixture, s rgState) map[string]any {
	f.t.Helper()
	b, ok := s.value["budget"].(map[string]any)
	if !ok {
		f.t.Fatalf("setup: ha status JSON has no budget: %s", s.raw)
	}
	return b
}

func ciInt(m map[string]any, key string) (int64, bool) {
	n, ok := m[key].(float64)
	return int64(n), ok
}

// AC-1: run-rules/6 can be chosen at the start and is listed as supported;
// the default start rule stays run-rules/3.
func TestCoreIntegrityAC1_StartAndCapabilities(t *testing.T) {
	const v = "rules-v6-unsupported"
	var out, errb bytes.Buffer
	if code := runMain([]string{"capabilities", "--format", "json"}, &out, &errb); code != exitOK {
		t.Fatalf("setup: ha capabilities exit %d: %s", code, errb.String())
	}
	var caps map[string]any
	if err := json.Unmarshal(out.Bytes(), &caps); err != nil {
		t.Fatalf("setup: ha capabilities: %v", err)
	}
	listed := false
	rules, _ := caps["supported_rules"].([]any)
	for _, r := range rules {
		listed = listed || r == "run-rules/6"
	}
	if !listed {
		t.Errorf("jaekit:violation=%s ha capabilities lists supported_rules %v without run-rules/6", v, rules)
	}
	if caps["default_rules"] != "run-rules/3" {
		t.Errorf("jaekit:violation=%s ha capabilities gives default_rules %v; want run-rules/3", v, caps["default_rules"])
	}

	plain := rgRepo(t, "")
	if code, out := plain.ha("start", rgGoal, "--request", ciRequest); code != exitOK {
		t.Fatalf("setup: ha start without --rules exit %d: %s", code, out)
	}
	if r := ciLines(plain)[0].m["rules"]; r != "run-rules/3" {
		t.Errorf("jaekit:violation=%s a start without --rules records rules %v; want run-rules/3", v, r)
	}

	f := rgRepo(t, "")
	ciStart(f, v)
	if r := ciLines(f)[0].m["rules"]; r != "run-rules/6" {
		t.Errorf("jaekit:violation=%s ha start --rules run-rules/6 records rules %v", v, r)
	}
}

// AC-7: goals started under earlier rules keep their judgments, including
// the run-rules/5 goals in the situations of #150 and #151, and a goal
// before run-rules/5 still refuses review notes.
func TestCoreIntegrityAC7_EarlierRulesKeepTheirJudgments(t *testing.T) {
	rgRequireGit(t)

	// #150 under run-rules/5: a light goal's completion is voided by a
	// later review note.
	light := newRGFixture(t, "earlier-rules-changed", "light")
	first := ciMustDone(light, "run-rules/5 light goal")
	light.recordGate("earlier-rules-changed", "late gate", light.covering("candidate"))
	if s := light.status(); s.completion != nil {
		t.Errorf("run-rules/5: a light goal kept completion record %v after a later review note (seq %d completed)", s.completion, first)
	}

	// #151 under run-rules/5: a done line naming an older record head hides
	// the completion record before it.
	std := newRGFixture(t, "earlier-rules-changed", "standard")
	std.recordGate("earlier-rules-changed", "covering gate", std.covering("candidate"))
	ciMustDone(std, "run-rules/5 standard goal")
	std.put(rgGoal+"/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | merged |\n")
	std.git("add", "-A")
	std.git("commit", "-q", "-m", "bundle documents only")
	ciMustDone(std, "run-rules/5 second done")
	lines := ciLines(std)
	ciWrite(std, append(lines, ciUnjudged(std, lines[len(lines)-1], lines[len(lines)-1])))
	if s := std.status(); s.completion != nil || s.status == "complete" {
		t.Errorf("run-rules/5: the #151 record gives status %s with completion record %v; want no completion record", s.status, s.completion)
	}

	// A run-rules/4 goal still refuses review notes.
	old := rgRepo(t, "standard")
	if code, out := old.ha("start", rgGoal, "--request", ciRequest, "--rules", "run-rules/4"); code != exitOK {
		t.Fatalf("setup: run-rules/4 start exit %d: %s", code, out)
	}
	old.head = old.git("rev-parse", "HEAD")
	path, _ := old.write(ciGate(old, ciRound(old, "candidate", old.head, old.head)).gateValue())
	before := old.records()
	if code, out := old.ha("note", rgGoal, "review", "--gate", path); code != exitRefused || !bytes.Equal(before, old.records()) {
		t.Errorf("run-rules/4: ha note review exit %d (want 65) and the record changed %v: %s", code, !bytes.Equal(before, old.records()), out)
	}
}

// AC-10: ha imports no Spar package, its status computation reads no
// current time, and the same record, bundle and Git give the same status.
func TestCoreIntegrityAC10_CoreStaysDeterministicAndApart(t *testing.T) {
	rgRequireGit(t)
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	list := exec.Command("go", "list", "-deps", "./cmd/ha")
	list.Dir = root
	deps, err := list.Output()
	if err != nil {
		t.Fatalf("setup: go list -deps ./cmd/ha: %v", err)
	}
	for _, dep := range strings.Fields(string(deps)) {
		if dep == "github.com/jgoneit/jaekit/internal/spar" || strings.HasPrefix(dep, "github.com/jgoneit/jaekit/internal/spar/") {
			t.Errorf("ha imports %s", dep)
		}
	}
	sources, err := filepath.Glob(filepath.Join(root, "internal", "status", "*.go"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("setup: internal/status sources: %v", err)
	}
	clock := regexp.MustCompile(`\btime\.(Now|Since|Until)\(`)
	for _, p := range sources {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		if clock.Match(data) {
			t.Errorf("%s reads the current time", filepath.Base(p))
		}
	}

	f := ciFixture(t, "core-not-deterministic", "standard")
	f.recordGate("core-not-deterministic", "gate", ciGate(f, ciRound(f, "candidate", f.work, f.head)))
	for _, args := range [][]string{{"status", rgGoal, "--format", "json"}, {"status", rgGoal}} {
		one, two := ciMarkdown(f, args...), ciMarkdown(f, args...)
		if one != two {
			t.Errorf("ha %v gave different output for the same record:\n%s\n---\n%s", args, one, two)
		}
	}
}

// ciMust fails the setup unless the ha command exits with want.
func ciMust(f *rgFixture, want int, args ...string) string {
	f.t.Helper()
	code, out := f.ha(args...)
	if code != want {
		f.t.Fatalf("setup: ha %v exit %d, want %d: %s", args, code, want, out)
	}
	return out
}

// ciFinding records a candidate post-completion finding about the
// completion record line d.
func ciFinding(f *rgFixture, d ciLine) {
	f.t.Helper()
	criteria := []any{}
	if list, ok := d.m["criteria"].([]any); ok {
		for _, c := range list {
			if m, _ := c.(map[string]any); m != nil {
				criteria = append(criteria, m["id"])
			} else {
				criteria = append(criteria, c)
			}
		}
	}
	input := map[string]any{"id": "F-1", "request_id": "R-1", "revision": 0,
		"completion": map[string]any{"seq": d.seq(), "sha256": rgSHA(d.raw)}, "criteria": criteria,
		"mapping": "unmapped", "status": "candidate", "category": "unknown",
		"summary": "a synthetic report after completion", "source": "synthetic test", "evidence": []any{}}
	if len(criteria) > 0 {
		input["mapping"] = "linked"
	}
	data, err := json.Marshal(input)
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	path := f.writeBytes(data)
	ciMust(f, exitOK, "finding", rgGoal, "--input", path)
}

func ciSeconds(d time.Duration) int64 { return int64(d / time.Second) }
