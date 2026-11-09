package main

// Check of the goal core-completion-integrity, AC-8: outside the four
// judgments AC-2 to AC-6 change, a goal started under run-rules/6 is judged
// and commanded like the same record under run-rules/5. The test builds two
// synthetic repositories that differ only in the start rule (the twins),
// runs the same steps on both and, after every step, compares the exit code,
// how the run record changed, and `ha status --format json` without the
// rule, times and line hashes. Fixed commit dates give both twins the same
// commit IDs, and the record stays out of the commits.
//
// The steps keep away from the changed judgments: every gate's last
// candidate round starts at the goal's base commit, the only completion
// record is the last line, and the done line appended after another line
// has no completion record before it. Error recovery is outside this check.
//
// A refused run-rules/6 start and any difference between the twins are the
// observed violation (jaekit:violation=...). Every other failure, including a
// run-rules/5 twin that does not reach the state a step names, is a broken
// setup.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const (
	ctViolation = "rules-v6-differs-from-v5"
	// ctDate is every commit's author and committer date.
	ctDate      = "2026-01-02T03:04:05Z"
	ctRequest   = "implement the synthetic goal"
	ctAuthority = "the request lets the executor change the goal's tests"
	ctQuote     = "synthetic user words for this step"
	ctContext   = "the synthetic goal's current window"
	// ctPending is a want for `ha done` on a goal that is not complete: any
	// nonzero exit.
	ctPending = -1
)

// ctRules are the twins' start rules: the earlier rule and the new one.
var ctRules = [2]string{"run-rules/5", "run-rules/6"}

// ctTimeKeys are status fields that hold a time or a length of time; they
// follow when each command ran, not the judgment.
var ctTimeKeys = map[string]bool{"at": true, "started_at": true, "elapsed_seconds": true, "span_seconds": true,
	"reported_wait_seconds": true, "post_completion_seconds": true}

// ctTwins are the two repositories, the run-rules/5 one first.
type ctTwins struct {
	t    *testing.T
	pair [2]*rgFixture
	// saved and doneLine hold each twin's record and the done line ha
	// computed from it, for the done line that lands after another line.
	saved, doneLine [2][]byte
}

// ctObs is what follows one step in one twin.
type ctObs struct {
	code   int
	out    string
	record string // how the run record changed
	state  rgState
	norm   any // the status JSON without the rule, times and line hashes
}

// ctAct is one step's action on a twin; it returns an exit code and output.
type ctAct func(i int, f *rgFixture) (int, string)

func ctNothing(int, *rgFixture) (int, string) { return exitOK, "" }

func ctHa(args ...string) ctAct {
	return func(_ int, f *rgFixture) (int, string) { return f.ha(args...) }
}

func ctFirst(out string) string {
	return strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]
}

// newCTTwins makes the twins: the same committed standard goal in two
// repositories whose base commits have the same ID.
func newCTTwins(t *testing.T) *ctTwins {
	t.Helper()
	rgRequireGit(t)
	t.Setenv("GIT_AUTHOR_DATE", ctDate)
	t.Setenv("GIT_COMMITTER_DATE", ctDate)
	tw := &ctTwins{t: t}
	for i := range tw.pair {
		tw.pair[i] = rgRepo(t, "standard", func(f *rgFixture) {
			f.put(".gitignore", "/"+rgGoal+"/runs.jsonl\n")
		})
	}
	tw.sameHead("the base commit")
	return tw
}

// use makes twin i's repository the working directory, as ha reads the goal
// path from it.
func (tw *ctTwins) use(i int) *rgFixture {
	tw.t.Helper()
	f := tw.pair[i]
	if err := os.Chdir(f.dir); err != nil {
		tw.t.Fatalf("setup: %v", err)
	}
	if err := os.Setenv("PWD", f.dir); err != nil {
		tw.t.Fatalf("setup: %v", err)
	}
	return f
}

func (tw *ctTwins) sameHead(label string) {
	tw.t.Helper()
	a, b := tw.pair[0].git("rev-parse", "HEAD"), tw.pair[1].git("rev-parse", "HEAD")
	if a != b {
		tw.t.Fatalf("setup: %s: the twins' commits differ (%s, %s)", label, a, b)
	}
}

func ctRecords(f *rgFixture) []byte {
	f.t.Helper()
	data, err := os.ReadFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return data
}

// ctChange names how the record changed: unchanged, rewritten, or the kind
// (with the note, status or result) of each appended line.
func ctChange(before, after []byte) string {
	if bytes.Equal(before, after) {
		return "unchanged"
	}
	if !bytes.HasPrefix(after, before) {
		return "rewritten"
	}
	var parts []string
	for _, raw := range bytes.Split(bytes.TrimSuffix(after[len(before):], []byte("\n")), []byte("\n")) {
		var l struct{ Kind, Note, Status, Result string }
		if err := json.Unmarshal(raw, &l); err != nil {
			parts = append(parts, "a line that is not JSON")
			continue
		}
		name := l.Kind
		for _, s := range []string{l.Note, l.Status, l.Result} {
			if s != "" {
				name += "/" + s
			}
		}
		parts = append(parts, name)
	}
	return "appended " + strings.Join(parts, ", ")
}

// ctStrip copies a decoded JSON value without the time fields and without
// the line hash of every record head (`record_head.sha256`), which follow
// when each line was written.
func ctStrip(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, val := range x {
			if !ctTimeKeys[k] && (key != "record_head" || k != "sha256") {
				m[k] = ctStrip(val, k)
			}
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = ctStrip(val, key)
		}
		return out
	}
	return v
}

// ctNormal is the status document without the start rule, the time fields
// and the line hashes.
func ctNormal(value map[string]any) any {
	m := ctStrip(value, "").(map[string]any)
	delete(m, "rules")
	return m
}

func ctShow(v any, present bool) string {
	if !present {
		return "(absent)"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

// ctDiff returns the first path where a and b differ, with both values, or
// an empty path when they are the same.
func ctDiff(path string, a, b any) (string, string, string) {
	switch x := a.(type) {
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok {
			break
		}
		keys := map[string]bool{}
		for k := range x {
			keys[k] = true
		}
		for k := range y {
			keys[k] = true
		}
		var names []string
		for k := range keys {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, k := range names {
			va, oka := x[k]
			vb, okb := y[k]
			if !oka || !okb {
				return path + "." + k, ctShow(va, oka), ctShow(vb, okb)
			}
			if p, s, u := ctDiff(path+"."+k, va, vb); p != "" {
				return p, s, u
			}
		}
		return "", "", ""
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			break
		}
		for i := range x {
			if p, s, u := ctDiff(fmt.Sprintf("%s[%d]", path, i), x[i], y[i]); p != "" {
				return p, s, u
			}
		}
		return "", "", ""
	default:
		if reflect.DeepEqual(a, b) {
			return "", "", ""
		}
	}
	return path, ctShow(a, true), ctShow(b, true)
}

// run does one step on each twin in turn. want is the exit code the step
// gives under run-rules/5 (ctPending: any nonzero exit); another one means
// the fixture did not build the state the step names. An internal error
// (70) of either twin is a broken setup too.
func (tw *ctTwins) run(label string, want int, act ctAct) [2]ctObs {
	tw.t.Helper()
	var obs [2]ctObs
	for i := range tw.pair {
		f := tw.use(i)
		before := ctRecords(f)
		code, out := act(i, f)
		if code == exitInternal {
			tw.t.Fatalf("setup: %s: ha internal error under %s: %s", label, ctRules[i], ctFirst(out))
		}
		obs[i] = ctObs{code: code, out: out, record: ctChange(before, ctRecords(f)), state: f.status()}
		obs[i].norm = ctNormal(obs[i].state.value)
	}
	if got := obs[0].code; (want == ctPending && got == exitOK) || (want != ctPending && got != want) {
		tw.t.Fatalf("setup: %s: exit %d under %s, want %d: %s", label, got, ctRules[0], want, ctFirst(obs[0].out))
	}
	return obs
}

// compare reports the first difference between the twins after a step.
func (tw *ctTwins) compare(label string, obs [2]ctObs) {
	tw.t.Helper()
	old, now := obs[0], obs[1]
	switch {
	case now.code != old.code:
		tw.t.Fatalf("jaekit:violation=%s %s: exit %d under %s, %d under %s: %s", ctViolation, label,
			now.code, ctRules[1], old.code, ctRules[0], ctFirst(now.out))
	case now.record != old.record:
		tw.t.Fatalf("jaekit:violation=%s %s: the run record %s under %s, %s under %s", ctViolation, label,
			now.record, ctRules[1], old.record, ctRules[0])
	case now.state.code != old.state.code:
		tw.t.Fatalf("jaekit:violation=%s %s: ha status exit %d (%s %v) under %s, %d (%s %v) under %s", ctViolation, label,
			now.state.code, now.state.status, now.state.reasons, ctRules[1], old.state.code, old.state.status, old.state.reasons, ctRules[0])
	}
	if where, a, b := ctDiff("status", old.norm, now.norm); where != "" {
		tw.t.Fatalf("jaekit:violation=%s %s: the status JSON differs at %s: %s under %s, %s under %s", ctViolation, label,
			where, b, ctRules[1], a, ctRules[0])
	}
}

func (tw *ctTwins) step(label string, want int, act ctAct) [2]ctObs {
	tw.t.Helper()
	obs := tw.run(label, want, act)
	tw.compare(label, obs)
	return obs
}

// review fails the setup unless the run-rules/5 twin's review reasons are
// exactly want.
func (tw *ctTwins) review(label string, obs [2]ctObs, want ...string) {
	tw.t.Helper()
	sort.Strings(want)
	if !rgSame(obs[0].state.review, want) {
		tw.t.Fatalf("setup: %s: review reasons %v under %s, want %v", label, obs[0].state.reasons, ctRules[0], want)
	}
}

// has fails the setup unless the run-rules/5 twin's reasons include code,
// or leave it out when want is false. A code ending in ":" stands for any
// reason of that condition (AC-n:).
func (tw *ctTwins) has(label string, obs [2]ctObs, code string, want bool) {
	tw.t.Helper()
	found := false
	for _, r := range obs[0].state.reasons {
		found = found || r == code || (strings.HasSuffix(code, ":") && strings.HasPrefix(r, code))
	}
	if found != want {
		tw.t.Fatalf("setup: %s: reasons %v under %s; want %s present: %v", label, obs[0].state.reasons, ctRules[0], code, want)
	}
}

// commit commits the same files in both twins.
func (tw *ctTwins) commit(label string, files ...string) [2]ctObs {
	tw.t.Helper()
	for _, f := range tw.pair {
		for j := 0; j+1 < len(files); j += 2 {
			f.put(files[j], files[j+1])
		}
		f.head = f.commit(label)
	}
	tw.sameHead(label)
	return tw.step(label, exitOK, ctNothing)
}

// edit changes goal documents in both twins without a commit.
func (tw *ctTwins) edit(label string, files ...string) [2]ctObs {
	tw.t.Helper()
	return tw.step(label, exitOK, func(_ int, f *rgFixture) (int, string) {
		for j := 0; j+1 < len(files); j += 2 {
			f.put(files[j], files[j+1])
		}
		return exitOK, ""
	})
}

// start starts both goals; a run-rules/6 start refused for its rule is the
// observed violation.
func (tw *ctTwins) start() {
	tw.t.Helper()
	obs := tw.run("the start", exitOK, func(i int, f *rgFixture) (int, string) {
		code, out := f.ha("start", rgGoal, "--request", ctRequest, "--rules", ctRules[i], "--test-authority", ctAuthority)
		if i == 1 && code == exitUsage && strings.Contains(out, "--rules") {
			tw.t.Fatalf("jaekit:violation=%s ha refuses to start a goal under %s: exit %d: %s", ctViolation, ctRules[i], code, ctFirst(out))
		}
		if code != exitOK {
			tw.t.Fatalf("setup: ha start under %s exit %d: %s", ctRules[i], code, ctFirst(out))
		}
		f.base = f.git("rev-parse", "HEAD")
		return code, out
	})
	for i, o := range obs {
		if got := o.state.value["rules"]; got != ctRules[i] {
			if i == 0 {
				tw.t.Fatalf("setup: the start under %s computes under %v", ctRules[i], got)
			}
			tw.t.Fatalf("jaekit:violation=%s ha start --rules %s started a goal that computes under %v", ctViolation, ctRules[i], got)
		}
	}
	tw.compare("the start", obs)
}

// gate records the gate build makes of each twin, as spar gate writes it.
func (tw *ctTwins) gate(label string, build func(f *rgFixture) rgGate) [2]ctObs {
	tw.t.Helper()
	return tw.step(label, exitOK, func(_ int, f *rgFixture) (int, string) {
		path, _ := f.write(build(f).gateValue())
		return f.ha("note", rgGoal, "review", "--gate", path)
	})
}

// done runs `ha done` on a goal that is not complete.
func (tw *ctTwins) done(label string) {
	tw.t.Helper()
	obs := tw.step(label, ctPending, ctHa("done", rgGoal))
	if obs[0].state.completion != nil {
		tw.t.Fatalf("setup: %s: a completion record under %s", label, ctRules[0])
	}
}

// ctBudget is a budget command's arguments for the twin's current window,
// revision and limit, read from its status.
func ctBudget(f *rgFixture, extra ...string) []string {
	f.t.Helper()
	b, _ := f.status().value["budget"].(map[string]any)
	window, ok1 := b["window_start"].(float64)
	revision, ok2 := b["revision"].(float64)
	limit, ok3 := b["runs_limit"].(float64)
	if !ok1 || !ok2 || !ok3 {
		f.t.Fatalf("setup: the status budget has no window, revision or limit: %v", b)
	}
	args := []string{"budget", rgGoal, "--window", fmt.Sprint(int(window)), "--revision", fmt.Sprint(int(revision))}
	for _, a := range extra {
		if a == "{limit}" {
			a = fmt.Sprint(int(limit))
		}
		args = append(args, a)
	}
	return args
}

// ctMutate is a covering gate's bytes after change.
func ctMutate(f *rgFixture, change func(m map[string]any)) []byte {
	f.t.Helper()
	data, err := json.Marshal(f.covering("candidate").gateValue())
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	var value map[string]any
	if err := json.Unmarshal(data, &value); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	change(value)
	out, err := json.Marshal(value)
	if err != nil {
		f.t.Fatalf("setup: %v", err)
	}
	return out
}

func ctRound(m map[string]any) map[string]any { return m["rounds"].([]any)[0].(map[string]any) }

// The header fields a line copy changes.
var (
	ctSeqField  = regexp.MustCompile(`"seq":[0-9]+`)
	ctPrevField = regexp.MustCompile(`"prev":("[0-9a-f]{64}"|null)`)
	ctAtField   = regexp.MustCompile(`"at":"[^"]*"`)
)

func ctReplaceFirst(t *testing.T, re *regexp.Regexp, line []byte, value string) []byte {
	t.Helper()
	loc := re.FindIndex(line)
	if loc == nil {
		t.Fatalf("setup: the done line has no %s field: %s", re, line)
	}
	return append(append(append([]byte{}, line[:loc[0]]...), value...), line[loc[1]:]...)
}

func TestCoreIntegrityAC8_SameAsRulesV5Elsewhere(t *testing.T) {
	tw := newCTTwins(t)
	tw.start()

	// Checks, and review reasons without a gate.
	for _, f := range tw.pair {
		f.put("src/work.txt", "one\n")
	}
	tw.commit("a code commit")
	for _, f := range tw.pair {
		f.work = f.head
	}
	tw.step("AC-1 checked", exitOK, ctHa("check", rgGoal, "AC-1"))
	obs := tw.step("the status without a gate", exitOK, ctNothing)
	tw.review("the status without a gate", obs, "review_missing")
	tw.done("ha done without a gate")

	// Gate files that cannot be recorded leave the record as it was.
	valid := func(f *rgFixture) []byte {
		data, err := json.Marshal(f.covering("candidate").gateValue())
		if err != nil {
			f.t.Fatalf("setup: %v", err)
		}
		return data
	}
	for _, form := range []struct {
		label string
		path  func(f *rgFixture) string
	}{
		{"a missing gate file", func(f *rgFixture) string { return filepath.Join(f.gates, "missing.json") }},
		{"an empty gate file", func(f *rgFixture) string { return f.writeBytes(nil) }},
		{"a gate file that is not JSON", func(f *rgFixture) string { return f.writeBytes([]byte("{not json")) }},
		{"a gate file with trailing data", func(f *rgFixture) string { return f.writeBytes(append(valid(f), " {}"...)) }},
		{"a gate of another schema", func(f *rgFixture) string {
			return f.writeBytes(ctMutate(f, func(m map[string]any) { m["schema"] = "review-gate/v2" }))
		}},
		{"a gate with an unknown state", func(f *rgFixture) string {
			return f.writeBytes(ctMutate(f, func(m map[string]any) { m["state"] = "done" }))
		}},
		{"a gate without rounds", func(f *rgFixture) string {
			return f.writeBytes(ctMutate(f, func(m map[string]any) { delete(m, "rounds") }))
		}},
		{"a gate round with an unknown stage", func(f *rgFixture) string {
			return f.writeBytes(ctMutate(f, func(m map[string]any) { ctRound(m)["stage"] = "external" }))
		}},
		{"a gate criteria digest that is not a sha256", func(f *rgFixture) string {
			return f.writeBytes(ctMutate(f, func(m map[string]any) {
				ctRound(m)["criteria"].([]any)[0].(map[string]any)["sha256"] = "not-a-digest"
			}))
		}},
	} {
		obs := tw.run(form.label, exitRefused, func(_ int, f *rgFixture) (int, string) {
			return f.ha("note", rgGoal, "review", "--gate", form.path(f))
		})
		if obs[0].record != "unchanged" {
			t.Fatalf("setup: %s: the run record %s under %s", form.label, obs[0].record, ctRules[0])
		}
		tw.compare(form.label, obs)
		tw.review(form.label, obs, "review_missing")
	}

	// A changed existing test under the recorded authority, explained by a
	// test-change note.
	for _, f := range tw.pair {
		f.put("tests/value.sh", "grep -qx ok src/value.txt\n# the check reads one line\n")
	}
	obs = tw.commit("an existing test changed")
	tw.has("an existing test changed", obs, "test_definition_changed", true)
	obs = tw.step("the test change explained", exitOK,
		ctHa("note", rgGoal, "test-change", "tests:tests/value.sh", "--criteria", "AC-1", "--how", "it still reads src/value.txt"))
	tw.has("the test change explained", obs, "test_definition_changed", false)
	tw.step("AC-1 checked after the test change", exitOK, ctHa("check", rgGoal, "AC-1"))

	// A reported wait.
	tw.step("a wait", exitOK, ctHa("note", rgGoal, "wait", "--reason", "a synthetic decision asked of the user"))
	tw.step("the resume", exitOK, ctHa("note", rgGoal, "resume"))

	// Budget changes: a pre-approved limit, a raise inside it with a reason,
	// a raise past it with only a reason, and a raise with the user's words.
	budget := func(extra ...string) ctAct {
		return func(_ int, f *rgFixture) (int, string) { return f.ha(ctBudget(f, extra...)...) }
	}
	tw.step("a pre-approved limit", exitOK, budget("--preapprove", "80", "--quote", ctQuote, "--context", ctContext))
	tw.step("a raise inside the pre-approved limit", exitOK, budget("--runs", "60", "--from", "{limit}", "--reason", "more checks of AC-1"))
	obs = tw.run("a raise past the pre-approved limit with only a reason", exitRefused,
		budget("--runs", "90", "--from", "{limit}", "--reason", "more checks of AC-1"))
	if obs[0].record != "unchanged" {
		t.Fatalf("setup: a refused budget change: the run record %s under %s", obs[0].record, ctRules[0])
	}
	tw.compare("a raise past the pre-approved limit with only a reason", obs)
	tw.step("a raise with the user's words", exitOK, budget("--runs", "100", "--from", "{limit}", "--quote", ctQuote, "--context", ctContext))

	// Per-command time limits: a valid entry, then one over the maximum.
	limited := rgPlan + "\n## 시간 제한\n- AC-1: 5분\n"
	obs = tw.edit("a time limit for AC-1", rgGoal+"/PLAN.md", limited)
	tw.has("a time limit for AC-1", obs, "AC-1:", true)
	tw.step("AC-1 checked under its time limit", exitOK, ctHa("check", rgGoal, "AC-1"))
	obs = tw.edit("a time limit over the maximum", rgGoal+"/PLAN.md", rgPlan+"\n## 시간 제한\n- AC-1: 3시간\n")
	tw.has("a time limit over the maximum", obs, "lint_error", true)
	obs = tw.run("a check under a time limit over the maximum", exitRefused, ctHa("check", rgGoal, "AC-1"))
	if obs[0].record != "unchanged" {
		t.Fatalf("setup: a check refused before running: the run record %s under %s", obs[0].record, ctRules[0])
	}
	tw.compare("a check under a time limit over the maximum", obs)
	obs = tw.edit("the valid time limit again", rgGoal+"/PLAN.md", limited)
	tw.has("the valid time limit again", obs, "lint_error", false)

	// Gates that leave completion pending, each with its reasons.
	state := func(s string, defect, question int) func(f *rgFixture) rgGate {
		return func(f *rgFixture) rgGate {
			g := f.chained()
			g.state, g.defect, g.question = s, defect, question
			return g
		}
	}
	for _, form := range []struct {
		label string
		build func(f *rgFixture) rgGate
		want  []string
	}{
		{"a degraded gate", state("degraded", 0, 0), []string{"review_degraded"}},
		{"a blocked gate", state("blocked", 0, 0), []string{"review_blocked"}},
		{"a gate with an open defect", state("complete", 1, 0), []string{"review_open_defect"}},
		{"a gate with an open requirement-question", state("complete", 0, 1), []string{"review_open_question"}},
		{"a blocked gate with both open", state("blocked", 1, 1), []string{"review_blocked", "review_open_defect", "review_open_question"}},
		{"a gate with plan rounds and no code round", func(f *rgFixture) rgGate { return f.covering("plan", "plan-delta") },
			[]string{"review_code_missing"}},
		{"a gate whose round left out a goal document", func(f *rgFixture) rgGate {
			g := f.covering("candidate")
			g.rounds[0].docs = g.rounds[0].docs[:1]
			return g
		}, []string{"review_docs_stale"}},
		{"a gate whose last code round reviewed an earlier head", func(f *rgFixture) rgGate {
			g := f.covering("candidate")
			g.rounds[0].head = f.work
			return g
		}, []string{"review_head_stale"}},
	} {
		obs := tw.gate(form.label, form.build)
		tw.review(form.label, obs, form.want...)
		tw.done("ha done after " + form.label)
	}

	// A covering gate, then a commit after it and goal documents changed
	// after it.
	covering := func(stages ...string) func(f *rgFixture) rgGate {
		return func(f *rgFixture) rgGate { return f.covering(stages...) }
	}
	obs = tw.gate("a covering gate", covering("candidate"))
	tw.review("a covering gate", obs)
	for _, f := range tw.pair {
		f.put("src/later.txt", "later\n")
	}
	tw.commit("a commit after the review")
	obs = tw.step("AC-1 checked after the review", exitOK, ctHa("check", rgGoal, "AC-1"))
	tw.review("AC-1 checked after the review", obs, "review_head_stale")
	tw.done("ha done with a stale reviewed head")
	obs = tw.gate("a gate of the new head", covering("candidate"))
	tw.review("a gate of the new head", obs)
	tw.edit("a linked goal document changed", rgGoal+"/note.md", "# Value note\n\nThe value is one lowercase word on one line.\n")
	tw.step("the changed goal documents confirmed", exitOK, ctHa("note", rgGoal, "confirm", "spec", "--quote", ctQuote))
	obs = tw.step("AC-1 checked after the goal documents changed", exitOK, ctHa("check", rgGoal, "AC-1"))
	tw.review("AC-1 checked after the goal documents changed", obs, "review_docs_stale")
	tw.done("ha done with stale goal documents")
	obs = tw.gate("a gate of the current goal documents", covering("candidate"))
	tw.review("a gate of the current goal documents", obs)

	// A strict goal without a plan round.
	tw.edit("the tier raised to strict", rgGoal+"/SPEC.md", rgSpec("strict"))
	tw.step("the strict tier confirmed", exitOK, ctHa("note", rgGoal, "confirm", "spec", "--quote", ctQuote))
	obs = tw.step("AC-1 checked under the strict tier", exitOK, ctHa("check", rgGoal, "AC-1"))
	tw.review("AC-1 checked under the strict tier", obs, "review_docs_stale", "review_plan_missing")
	obs = tw.gate("a strict goal's gate without a plan round", covering("candidate"))
	tw.review("a strict goal's gate without a plan round", obs, "review_plan_missing")
	tw.done("ha done without a plan round")
	obs = tw.gate("a strict goal's covering gate", covering("plan", "candidate"))
	tw.review("a strict goal's covering gate", obs)
	if obs[0].state.status != "complete" || obs[0].state.completion != nil {
		t.Fatalf("setup: a strict goal's covering gate: status %s, completion record %v under %s; want complete without a record",
			obs[0].state.status, obs[0].state.completion, ctRules[0])
	}

	// A done line that lands after a line appended while ha done computed:
	// ha done computes a completion from the record as it stands, an input
	// note is appended meanwhile, and the done line follows it with the
	// record head it judged. No completion record comes before it.
	obs = tw.step("ha done computes a completion", exitOK, func(i int, f *rgFixture) (int, string) {
		tw.saved[i] = ctRecords(f)
		return f.ha("done", rgGoal)
	})
	if obs[0].state.completion == nil {
		t.Fatalf("setup: ha done computes a completion: no completion record under %s", ctRules[0])
	}
	tw.step("the done line not yet written", exitOK, func(i int, f *rgFixture) (int, string) {
		after := ctRecords(f)
		if !bytes.HasPrefix(after, tw.saved[i]) || bytes.Count(after[len(tw.saved[i]):], []byte("\n")) != 1 {
			t.Fatalf("setup: ha done under %s did not append one line", ctRules[i])
		}
		tw.doneLine[i] = bytes.TrimSuffix(after[len(tw.saved[i]):], []byte("\n"))
		if err := os.WriteFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"), tw.saved[i], 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		return exitOK, ""
	})
	tw.step("an input note appended while ha done computes", exitOK, ctHa("note", rgGoal, "input", "--quote", ctQuote))
	late := "the done line after the input note"
	obs = tw.run(late, exitOK, func(i int, f *rgFixture) (int, string) {
		records := ctRecords(f)
		lines := bytes.Split(bytes.TrimSuffix(records, []byte("\n")), []byte("\n"))
		note := lines[len(lines)-1]
		var header struct {
			Seq int    `json:"seq"`
			At  string `json:"at"`
		}
		if err := json.Unmarshal(note, &header); err != nil || header.At == "" {
			t.Fatalf("setup: the input note line: %v", err)
		}
		line := ctReplaceFirst(t, ctSeqField, tw.doneLine[i], fmt.Sprintf(`"seq":%d`, header.Seq+1))
		line = ctReplaceFirst(t, ctPrevField, line, fmt.Sprintf(`"prev":"%s"`, rgSHA(note)))
		line = ctReplaceFirst(t, ctAtField, line, fmt.Sprintf(`"at":"%s"`, header.At))
		if err := os.WriteFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"), append(append(records, line...), '\n'), 0o644); err != nil {
			t.Fatalf("setup: %v", err)
		}
		return exitOK, ""
	})
	if obs[0].state.completion != nil {
		t.Fatalf("setup: %s: completion record %v under %s", late, obs[0].state.completion, ctRules[0])
	}
	if c := obs[1].state.completion; c != nil {
		t.Fatalf("jaekit:violation=%s %s: the done line that did not judge the line before it is completion record %v under %s",
			ctViolation, late, c, ctRules[1])
	}
	tw.compare(late, obs)
	obs = tw.step("ha done after the late done line", exitOK, ctHa("done", rgGoal))
	if obs[0].state.completion == nil {
		t.Fatalf("setup: ha done after the late done line: no completion record under %s", ctRules[0])
	}
}
