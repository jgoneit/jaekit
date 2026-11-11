package main

// Checks of the goal check-cost that compare twins: synthetic repositories
// that hold the same goal and differ only in the start rule or in the PLAN
// condition table's optional `baseline` column (the mark). Each test runs the
// same steps on every twin in turn and, after each step, compares the exit
// code, how the run record changed and `ha status --format json` with the
// first twin (the reference); AC-13 also compares the human-readable status
// and both reports. The goal directory is ignored by Git, as Jaekit's docs/
// is, and every commit has a fixed date, so twins whose PLANs differ still
// have the same commit IDs.
//
// AC-13: a goal started under each earlier rule (run-rules/1 to /6) gives
// the outputs this tree's ha gave before this goal (cwKept), and is judged,
// estimated, shown and completed the same whether its PLAN has the mark
// column or not; the default start rule stays run-rules/3. AC-14: outside the
// judgments the goal changes, a run-rules/7 goal whose every change condition
// is marked is judged and commanded like the same record under run-rules/6.
//
// The checks use only the ha command line (runMain) and the helpers of
// review_gate_test.go and core_integrity_test.go; they never run Spar, a
// reviewer or a model, and never read a commit, ref or record outside the
// repositories they build. A difference between the twins, a lint problem
// the mark brings to an earlier rule's goal, an earlier rule's output that
// differs from the expected one, and a refused run-rules/7 start are the
// observed violation (jaekit:violation=...). Every other failure, including
// a reference twin that does not reach the state a step names, is a broken
// setup (setup:).

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
	"slices"
	"sort"
	"strings"
	"testing"
)

const (
	cwDate      = "2026-01-02T03:04:05Z"
	cwRequest   = "implement the synthetic goal"
	cwAuthority = "the request lets the executor change the goal's tests"
	cwQuote     = "synthetic user words for this step"
	cwContext   = "the synthetic goal's current window"
	// cwPending is a want for a step whose exit is any nonzero code.
	cwPending = -1
	// cwMissingProgram is a command that cannot start, so its runs record
	// an error.
	cwMissingProgram = "cw-no-such-program-for-this-check"
	// cwReq is the mark of a change condition that requires a baseline.
	cwReq = "요구"
)

// cwProducer is the synthetic producer of both change conditions: target $1
// passes when file $2 holds exactly $3 and is the declared violation $4
// otherwise, written as check-result/v1 to HA_EVIDENCE_PATH.
const cwProducer = `# Synthetic producer of the check-cost twins.
if [ -f "$2" ] && [ "$(cat "$2")" = "$3" ]; then
	obs='{"target":"'"$1"'","attempt":1,"status":"pass"}'
	code=0
else
	obs='{"target":"'"$1"'","attempt":1,"status":"violation","violation":"'"$4"'"}'
	code=1
fi
printf '{"schema":"check-result/v1","invocation":"%s","declaration_digest":"%s","producer":"cw-synthetic","producer_version":"1","attempts_complete":true,"observations":[%s]}\n' "$HA_EVIDENCE_INVOCATION" "$HA_EVIDENCE_DECLARATION_DIGEST" "$obs" > "$HA_EVIDENCE_PATH"
exit $code
`

func cwDeclaration(target string) string {
	return `{"schema":"check-declaration/v1","producer":"cw-synthetic","producer_version":"1","producer_paths":["checks/produce.sh"],` +
		`"targets":[{"id":"` + target + `","violation":"` + target + `-not-ok"}]}` + "\n"
}

func cwSpec(tier string) string {
	line := ""
	if tier != "" {
		line = "Assurance: " + tier + "\n"
	}
	return "# Demo values\n\nStatus: Ready\n" + line + "\n## 목표\n\n`src/one.txt` and `src/two.txt` say ok, as [the value note](note.md) describes.\n\n" +
		"## 범위와 비목표\n\n- 범위: `src/**`\n\n## Acceptance Criteria\n" +
		"- **AC-1** `src/one.txt` contains exactly `ok`.\n- **AC-2** `src/two.txt` contains exactly `ok`.\n" +
		"- **AC-3** `src/value.txt` still contains exactly `ok`.\n\n## Open Decisions\n\n없음\n"
}

// cwCommand is a change condition's default command.
func cwCommand(target string) string {
	return "sh checks/produce.sh " + target + " src/" + target + ".txt ok " + target + "-not-ok"
}

// cwPlan is the goal's PLAN. marks holds the baseline column cell of AC-1,
// AC-2 and AC-3; nil leaves the column out. commands replaces a condition's
// command, and extra is appended (such as a `## 시간 제한` section).
type cwPlan struct {
	marks    []string
	commands map[string]string
	extra    string
}

func (p cwPlan) text() string {
	cmd := map[string]string{"AC-1": cwCommand("one"), "AC-2": cwCommand("two"), "AC-3": "sh tests/value.sh"}
	for id, c := range p.commands {
		cmd[id] = c
	}
	rows := [][]string{
		{"AC-1", "change", "`" + cmd["AC-1"] + "`", "checks/produce.sh, checks/one.json", "T001"},
		{"AC-2", "change", "`" + cmd["AC-2"] + "`", "checks/produce.sh, checks/two.json", "T001"},
		{"AC-3", "maintain", "`" + cmd["AC-3"] + "`", "—", "T001"},
	}
	head := []string{"ID", "종류", "검증 명령", "검사 경로", "Task"}
	if p.marks != nil {
		head = append(head, "baseline")
		for i := range rows {
			rows[i] = append(rows[i], p.marks[i])
		}
	}
	line := func(cells []string) string { return "| " + strings.Join(cells, " | ") + " |\n" }
	var b strings.Builder
	b.WriteString("# PLAN — demo\n\n## 조건표\n")
	b.WriteString(line(head))
	b.WriteString(strings.Repeat("| --- ", len(head)) + "|\n")
	for _, r := range rows {
		b.WriteString(line(r))
	}
	b.WriteString("\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/one.json |\n| AC-2 | checks/two.json |\n")
	b.WriteString("\n## 범위\n- 바꿀 수 있는 경로: `src/**`, `tests/**`, `checks/**` (SPEC: 범위와 비목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n")
	b.WriteString("\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n")
	b.WriteString("| T001 | the values say ok | AC-1, AC-2, AC-3 | — | — | 예 | — |\n")
	b.WriteString("\n## 예산\n- 검증 실행: 50\n- 경과 시간: 4시간\n- 비용: 미관측\n")
	b.WriteString(p.extra)
	return b.String()
}

// cwRepo makes the committed repository of the synthetic goal with the tier
// ("" for a SPEC without the line) and the PLAN. Only the producer, the
// declarations, the maintained value and its test are committed.
func cwRepo(t *testing.T, tier string, plan cwPlan) *rgFixture {
	t.Helper()
	return rgRepo(t, tier, func(f *rgFixture) {
		f.put(".gitignore", "/"+rgGoal+"/\n")
		f.put(rgGoal+"/SPEC.md", cwSpec(tier))
		f.put(rgGoal+"/PLAN.md", plan.text())
		f.put("checks/produce.sh", cwProducer)
		f.put("checks/one.json", cwDeclaration("one"))
		f.put("checks/two.json", cwDeclaration("two"))
	})
}

// cwTwin is one repository of a twin set.
type cwTwin struct {
	f     *rgFixture
	name  string
	rules string
	plan  cwPlan
}

// cwTwins are the repositories, the reference first.
type cwTwins struct {
	t         *testing.T
	violation string
	twins     []*cwTwin
	// drop names status fields left out of the comparison.
	drop map[string]bool
	// human also compares the human-readable status and reports.
	human bool
	// kept names the earlier rule whose expected outputs (cwKept) the only
	// twin's steps and views are compared with; "" in a set of twins. With
	// it, every deviation from those outputs is the observed violation, so
	// the steps do not check the reference twin's state as setup.
	kept string
	// steps are the step lines of a set with kept outputs so far.
	steps []string
}

// cwObs is what follows one step in one twin.
type cwObs struct {
	code   int
	out    string
	record string
	state  rgState
	norm   any
	human  map[string]string
}

type cwAct func(tw *cwTwin) (int, string)

func cwNothing(*cwTwin) (int, string) { return exitOK, "" }

func cwHa(args ...string) cwAct {
	return func(tw *cwTwin) (int, string) { return tw.f.ha(args...) }
}

func cwFirst(out string) string {
	return strings.SplitN(strings.TrimSpace(out), "\n", 2)[0]
}

// newCWTwins makes one repository per twin, each with its own PLAN, and
// fails the setup unless their base commits are the same.
func newCWTwins(t *testing.T, violation, tier string, twins ...*cwTwin) *cwTwins {
	t.Helper()
	rgRequireGit(t)
	t.Setenv("GIT_AUTHOR_DATE", cwDate)
	t.Setenv("GIT_COMMITTER_DATE", cwDate)
	set := &cwTwins{t: t, violation: violation, twins: twins, drop: map[string]bool{}}
	for _, tw := range twins {
		tw.f = cwRepo(t, tier, tw.plan)
	}
	set.sameHead("the base commit")
	return set
}

func (s *cwTwins) use(tw *cwTwin) *rgFixture {
	s.t.Helper()
	if err := os.Chdir(tw.f.dir); err != nil {
		s.t.Fatalf("setup: %v", err)
	}
	if err := os.Setenv("PWD", tw.f.dir); err != nil {
		s.t.Fatalf("setup: %v", err)
	}
	return tw.f
}

func (s *cwTwins) sameHead(label string) {
	s.t.Helper()
	want := s.twins[0].f.git("rev-parse", "HEAD")
	for _, tw := range s.twins[1:] {
		if got := tw.f.git("rev-parse", "HEAD"); got != want {
			s.t.Fatalf("setup: %s: the commits of %s and %s differ (%s, %s)", label, s.twins[0].name, tw.name, want, got)
		}
	}
}

func cwRecords(f *rgFixture) []byte {
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

// cwChange names how the record changed: unchanged, rewritten, or the kind
// (with the note, status or result) of each appended line.
func cwChange(before, after []byte) string {
	if bytes.Equal(before, after) {
		return "unchanged"
	}
	if !bytes.HasPrefix(after, before) {
		return "rewritten"
	}
	var parts []string
	for _, raw := range bytes.Split(bytes.TrimSuffix(after[len(before):], []byte("\n")), []byte("\n")) {
		var l struct {
			Kind, Note, Status, Result string
			Target                     struct{ Criterion, Task string }
		}
		if err := json.Unmarshal(raw, &l); err != nil {
			parts = append(parts, "a line that is not JSON")
			continue
		}
		name := l.Kind
		for _, s := range []string{l.Target.Criterion, l.Target.Task, l.Note, l.Status, l.Result} {
			if s != "" {
				name += "/" + s
			}
		}
		parts = append(parts, name)
	}
	return "appended " + strings.Join(parts, ", ")
}

// cwTimeKeys are status fields that hold a time or a length of time; they
// follow when each command ran, not the judgment.
var cwTimeKeys = map[string]bool{"at": true, "started_at": true, "elapsed_seconds": true, "span_seconds": true,
	"reported_wait_seconds": true, "post_completion_seconds": true}

// cwStrip copies a decoded JSON value without the time fields, the line
// hash of every record head and the dropped fields.
func cwStrip(v any, key string, drop map[string]bool) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, val := range x {
			if !cwTimeKeys[k] && !drop[k] && (key != "record_head" || k != "sha256") {
				m[k] = cwStrip(val, k, drop)
			}
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = cwStrip(val, key, drop)
		}
		return out
	}
	return v
}

func cwShow(v any, present bool) string {
	if !present {
		return "(absent)"
	}
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	return string(data)
}

// cwDiff returns the first path where a and b differ, with both values, or
// an empty path when they are the same.
func cwDiff(path string, a, b any) (string, string, string) {
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
				return path + "." + k, cwShow(va, oka), cwShow(vb, okb)
			}
			if p, s, u := cwDiff(path+"."+k, va, vb); p != "" {
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
			if p, s, u := cwDiff(fmt.Sprintf("%s[%d]", path, i), x[i], y[i]); p != "" {
				return p, s, u
			}
		}
		return "", "", ""
	default:
		if reflect.DeepEqual(a, b) {
			return "", "", ""
		}
	}
	return path, cwShow(a, true), cwShow(b, true)
}

// Values in human-readable output that follow when each command ran or the
// bytes of each record line: line hashes, times and lengths of time.
var (
	cwHashText    = regexp.MustCompile(`\b[0-9a-f]{64}\b`)
	cwHeadText    = regexp.MustCompile(`seq (\d+) \([0-9a-f]{7,64}\)`)
	cwTimeText    = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(\.\d+)?(Z|[+-]\d{2}:\d{2})`)
	cwElapsedText = regexp.MustCompile(`\b\d+/(\d+) seconds\b`)
	cwSecondsText = regexp.MustCompile(`\b\d+(\.\d+)? ?(ms|s|seconds?|초)\b`)
)

func cwHumanNormal(text string) string {
	text = cwHashText.ReplaceAllString(text, "<hash>")
	text = cwHeadText.ReplaceAllString(text, "seq $1 (<hash>)")
	text = cwTimeText.ReplaceAllString(text, "<time>")
	text = cwElapsedText.ReplaceAllString(text, "<n>/$1 seconds")
	return cwSecondsText.ReplaceAllString(text, "<n> $2")
}

// cwFirstDiff returns the first line number (from 1) where a and b differ
// with both lines ("(none)" past the end), or 0 when they are the same.
func cwFirstDiff(a, b string) (int, string, string) {
	if a == b {
		return 0, "", ""
	}
	x, y := strings.Split(a, "\n"), strings.Split(b, "\n")
	for i := 0; ; i++ {
		la, lb := "(none)", "(none)"
		if i < len(x) {
			la = x[i]
		}
		if i < len(y) {
			lb = y[i]
		}
		if la != lb {
			return i + 1, la, lb
		}
	}
}

// cwHumanViews are the human-readable outputs AC-13 compares.
var cwHumanViews = [][]string{{"status", rgGoal, "--format", "md"}, {"report", rgGoal, "--lang", "ko"}, {"report", rgGoal, "--lang", "en"}}

func (s *cwTwins) views(f *rgFixture) map[string]string {
	s.t.Helper()
	out := map[string]string{}
	for _, args := range cwHumanViews {
		code, text := f.ha(args...)
		if code == exitInternal || code == exitUsage {
			s.t.Fatalf("setup: ha %v exit %d: %s", args, code, cwFirst(text))
		}
		out[strings.Join(args, " ")] = fmt.Sprintf("exit %d\n%s", code, cwHumanNormal(text))
	}
	return out
}

// run does one step on each twin in turn. want is the reference twin's exit
// (cwPending: any nonzero exit); another one means the fixture did not build
// the state the step names. An internal error (70) of any twin is a broken
// setup too. In a set with kept outputs the step's line (cwStepLine) is
// compared with the expected one instead.
func (s *cwTwins) run(label string, want int, act cwAct) []cwObs {
	s.t.Helper()
	obs := make([]cwObs, len(s.twins))
	for i, tw := range s.twins {
		f := s.use(tw)
		before := cwRecords(f)
		code, out := act(tw)
		if code == exitInternal {
			s.t.Fatalf("setup: %s: ha internal error in %s: %s", label, tw.name, cwFirst(out))
		}
		obs[i] = cwObs{code: code, out: out, record: cwChange(before, cwRecords(f))}
		if _, err := os.Stat(filepath.Join(f.dir, rgGoal, "runs.jsonl")); err == nil {
			obs[i].state = f.status()
			obs[i].norm = cwStrip(obs[i].state.value, "", s.drop)
			if s.human {
				obs[i].human = s.views(f)
			}
		}
	}
	if s.kept != "" {
		s.steps = append(s.steps, cwStepLine(label, obs[0]))
		s.keep("steps", func() []string { return s.steps })
		return obs
	}
	if got := obs[0].code; (want == cwPending && got == exitOK) || (want != cwPending && got != want) {
		s.t.Fatalf("setup: %s: exit %d in %s, want %d: %s", label, got, s.twins[0].name, want, cwFirst(obs[0].out))
	}
	return obs
}

// cwStepLine is what follows one step: the exit, how the record changed,
// and the status and reasons after it ("none" before the start).
func cwStepLine(label string, o cwObs) string {
	state := "none"
	if o.state.value != nil {
		state = fmt.Sprintf("%s (exit %d) %v", o.state.status, o.state.code, o.state.reasons)
	}
	return fmt.Sprintf("%s: exit %d; record %s; status %s", label, o.code, o.record, state)
}

// compare reports the first difference between a twin and the reference
// after a step, and a rewritten record in any twin.
func (s *cwTwins) compare(label string, obs []cwObs) {
	s.t.Helper()
	ref, v := obs[0], s.violation
	for i, o := range obs {
		if o.record == "rewritten" {
			s.t.Fatalf("jaekit:violation=%s %s: the run record of %s was rewritten", v, label, s.twins[i].name)
		}
	}
	for i := 1; i < len(obs); i++ {
		now, name, refName := obs[i], s.twins[i].name, s.twins[0].name
		switch {
		case now.code != ref.code:
			s.t.Fatalf("jaekit:violation=%s %s: exit %d in %s, %d in %s: %s", v, label, now.code, name, ref.code, refName, cwFirst(now.out))
		case now.record != ref.record:
			s.t.Fatalf("jaekit:violation=%s %s: the run record %s in %s, %s in %s", v, label, now.record, name, ref.record, refName)
		case now.state.code != ref.state.code:
			s.t.Fatalf("jaekit:violation=%s %s: ha status exit %d (%s %v) in %s, %d (%s %v) in %s", v, label,
				now.state.code, now.state.status, now.state.reasons, name, ref.state.code, ref.state.status, ref.state.reasons, refName)
		}
		if where, a, b := cwDiff("status", ref.norm, now.norm); where != "" {
			s.t.Fatalf("jaekit:violation=%s %s: the status JSON differs at %s: %s in %s, %s in %s", v, label, where, b, name, a, refName)
		}
		for _, args := range cwHumanViews {
			key := strings.Join(args, " ")
			if n, a, b := cwFirstDiff(ref.human[key], now.human[key]); n > 0 {
				s.t.Fatalf("jaekit:violation=%s %s: `ha %s` line %d is %q in %s, %q in %s", v, label, key, n, b, name, a, refName)
			}
		}
	}
}

func (s *cwTwins) step(label string, want int, act cwAct) []cwObs {
	s.t.Helper()
	obs := s.run(label, want, act)
	s.compare(label, obs)
	return obs
}

// has fails the setup unless the reference twin's reasons include code, or
// leave it out when want is false. A code ending in ":" stands for any
// reason of that condition (AC-n:). A set with kept outputs observes the
// reasons in its step lines instead.
func (s *cwTwins) has(label string, obs []cwObs, code string, want bool) {
	s.t.Helper()
	if s.kept != "" {
		return
	}
	found := false
	for _, r := range obs[0].state.reasons {
		found = found || r == code || (strings.HasSuffix(code, ":") && strings.HasPrefix(r, code))
	}
	if found != want {
		s.t.Fatalf("setup: %s: reasons %v in %s; want %s present: %v", label, obs[0].state.reasons, s.twins[0].name, code, want)
	}
}

// review fails the setup unless the reference twin's review reasons are
// exactly want.
func (s *cwTwins) review(label string, obs []cwObs, want ...string) {
	s.t.Helper()
	sort.Strings(want)
	if !rgSame(obs[0].state.review, want) {
		s.t.Fatalf("setup: %s: review reasons %v in %s, want %v", label, obs[0].state.reasons, s.twins[0].name, want)
	}
}

// unchanged fails the setup unless the step left the reference twin's
// record as it was.
func (s *cwTwins) unchanged(label string, obs []cwObs) {
	s.t.Helper()
	if obs[0].record != "unchanged" {
		s.t.Fatalf("setup: %s: the run record %s in %s", label, obs[0].record, s.twins[0].name)
	}
}

// commit writes the same files (path, body pairs; a nil body removes the
// path) in every twin and commits them.
func (s *cwTwins) commit(label string, files ...any) []cwObs {
	s.t.Helper()
	for _, tw := range s.twins {
		for j := 0; j+1 < len(files); j += 2 {
			rel := files[j].(string)
			// Remove the path first, so that a symbolic link there is
			// replaced instead of written through.
			if err := os.RemoveAll(filepath.Join(tw.f.dir, filepath.FromSlash(rel))); err != nil {
				s.t.Fatalf("setup: %v", err)
			}
			if body, ok := files[j+1].(string); ok {
				tw.f.put(rel, body)
			}
		}
		tw.f.head = tw.f.commit(label)
	}
	s.sameHead(label)
	return s.step(label, exitOK, cwNothing)
}

// edit rewrites each twin's PLAN after change changes its plan.
func (s *cwTwins) edit(label string, change func(p *cwPlan)) []cwObs {
	s.t.Helper()
	return s.step(label, exitOK, func(tw *cwTwin) (int, string) {
		change(&tw.plan)
		tw.f.put(rgGoal+"/PLAN.md", tw.plan.text())
		return exitOK, ""
	})
}

// cwSaved reports whether a goal under rules is a saved-record fixture:
// this tree's ha starts run-rules/3 to /7 with --rules, and no longer starts
// run-rules/1 or /2.
func cwSaved(rules string) bool { return rules == "run-rules/1" || rules == "run-rules/2" }

// cwSaveRules turns the goal this tree's ha just started under run-rules/3
// into a saved goal of rules: its start line, still the record's only line,
// names rules. No later line links to it yet, so no hash link is joined
// again (estimate_test.go makes its earlier-rule goals the same way).
func cwSaveRules(f *rgFixture, rules string) {
	f.t.Helper()
	lines := ciLines(f)
	if len(lines) != 1 || lines[0].kind() != "start" {
		f.t.Fatalf("setup: the record after ha start is not one start line (%d lines)", len(lines))
	}
	lines[0].m["rules"] = rules
	ciWrite(f, ciRelink(f, lines, 0))
}

// start starts every twin under its rule; a saved-record fixture's twin
// starts under run-rules/3 and is saved under its rule. A refused run-rules/7
// start is the observed violation, and so is a start that records another
// rule; in a set with kept outputs so is any refused start.
func (s *cwTwins) start(label string, extra ...string) {
	s.t.Helper()
	obs := s.run(label, exitOK, func(tw *cwTwin) (int, string) {
		rules := tw.rules
		if cwSaved(rules) {
			rules = "run-rules/3"
		}
		code, out := tw.f.ha(append([]string{"start", rgGoal, "--request", cwRequest, "--rules", rules}, extra...)...)
		if code != exitOK && (tw.rules == "run-rules/7" || tw != s.twins[0] || s.kept != "") {
			s.t.Fatalf("jaekit:violation=%s ha refuses to start %s under %s: exit %d: %s", s.violation, tw.name, rules, code, cwFirst(out))
		}
		if code != exitOK {
			s.t.Fatalf("setup: ha start of %s under %s exit %d: %s", tw.name, rules, code, cwFirst(out))
		}
		if cwSaved(tw.rules) {
			cwSaveRules(tw.f, tw.rules)
		}
		tw.f.base = tw.f.git("rev-parse", "HEAD")
		return code, out
	})
	for i, o := range obs {
		if got := o.state.value["rules"]; got != s.twins[i].rules {
			if i == 0 && s.kept == "" {
				s.t.Fatalf("setup: %s: %s computes under %v", label, s.twins[i].name, got)
			}
			s.t.Fatalf("jaekit:violation=%s %s: the start of %s, which computes under %v", s.violation, label, s.twins[i].name, got)
		}
	}
	s.compare(label, obs)
}

// gate records the gate build makes of each twin, as spar gate writes it.
func (s *cwTwins) gate(label string, build func(f *rgFixture) rgGate) []cwObs {
	s.t.Helper()
	return s.step(label, exitOK, func(tw *cwTwin) (int, string) {
		path, _ := tw.f.write(build(tw.f).gateValue())
		return tw.f.ha("note", rgGoal, "review", "--gate", path)
	})
}

// done runs `ha done` on a goal that is not complete.
func (s *cwTwins) done(label string) {
	s.t.Helper()
	obs := s.step(label, cwPending, cwHa("done", rgGoal))
	if obs[0].state.completion != nil {
		s.t.Fatalf("setup: %s: a completion record in %s", label, s.twins[0].name)
	}
}

// refused runs a step the reference refuses with 65 and no record.
func (s *cwTwins) refused(label string, act cwAct) []cwObs {
	s.t.Helper()
	obs := s.run(label, exitRefused, act)
	s.unchanged(label, obs)
	s.compare(label, obs)
	return obs
}

// recoveries fails the setup unless the reference twin shows the input of
// the condition with the used and allowed recoveries.
func (s *cwTwins) recoveries(label string, obs []cwObs, criterion, input string, used, allowed int) {
	s.t.Helper()
	list, _ := obs[0].state.value["recoveries"].([]any)
	for _, item := range list {
		r, _ := item.(map[string]any)
		if r["criterion"] == criterion && r["input"] == input && r["used"] == float64(used) && r["allowed"] == float64(allowed) {
			return
		}
	}
	s.t.Fatalf("setup: %s: recoveries %v in %s, want %s %s with %d used of %d", label, obs[0].state.value["recoveries"],
		s.twins[0].name, criterion, input, used, allowed)
}

// cwBudgetArgs is a budget command's arguments for the twin's current
// window, revision and limit, read from its status.
func cwBudgetArgs(f *rgFixture, extra ...string) []string {
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

// cwMutate is a covering gate's bytes after change.
func cwMutate(f *rgFixture, change func(m map[string]any)) []byte {
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

// output reports a difference between a twin's normalized command output
// and the reference's.
func (s *cwTwins) output(label string, obs []cwObs) {
	s.t.Helper()
	for i := 1; i < len(obs); i++ {
		if n, a, b := cwFirstDiff(cwHumanNormal(obs[0].out), cwHumanNormal(obs[i].out)); n > 0 {
			s.t.Fatalf("jaekit:violation=%s %s: output line %d is %q in %s, %q in %s", s.violation, label, n, b, s.twins[i].name, a, s.twins[0].name)
		}
	}
}

// noMarkFields reports an earlier rule's status that carries a field only a
// run-rules/7 goal shows (criteria[].baseline_required, check_freeze).
func (s *cwTwins) noMarkFields(label string, obs []cwObs) {
	s.t.Helper()
	for i, o := range obs {
		if o.state.value == nil {
			continue
		}
		tw := s.twins[i]
		if _, ok := o.state.value["check_freeze"]; ok {
			s.t.Fatalf("jaekit:violation=%s %s: the %s status of %s has check_freeze", s.violation, label, tw.rules, tw.name)
		}
		list, _ := o.state.value["criteria"].([]any)
		for _, item := range list {
			c, _ := item.(map[string]any)
			if _, ok := c["baseline_required"]; ok {
				s.t.Fatalf("jaekit:violation=%s %s: the %s status of %s has criteria[].baseline_required (%v)", s.violation, label, tw.rules, tw.name, c["id"])
			}
		}
	}
}

// query runs a command that only reads the goal (ha lint, ha estimate) on
// each twin and compares its exit, normalized output and record change with
// the reference's. A set of twins leaves out the status and views, which
// such a command leaves as they were unless it changes the record; a set
// with kept outputs runs it as any step.
func (s *cwTwins) query(label string, act cwAct) []cwObs {
	s.t.Helper()
	if s.kept != "" {
		return s.same(label, exitOK, act)
	}
	obs := make([]cwObs, len(s.twins))
	for i, tw := range s.twins {
		f := s.use(tw)
		before := cwRecords(f)
		code, out := act(tw)
		if code == exitInternal {
			s.t.Fatalf("setup: %s: ha internal error in %s: %s", label, tw.name, cwFirst(out))
		}
		obs[i] = cwObs{code: code, out: out, record: cwChange(before, cwRecords(f))}
	}
	if obs[0].code != exitOK {
		s.t.Fatalf("setup: %s: exit %d in %s, want 0: %s", label, obs[0].code, s.twins[0].name, cwFirst(obs[0].out))
	}
	s.compare(label, obs)
	s.output(label, obs)
	return obs
}

// same runs a step, compares it, its output and the fields an earlier
// rule's status must not have.
func (s *cwTwins) same(label string, want int, act cwAct) []cwObs {
	s.t.Helper()
	obs := s.step(label, want, act)
	s.output(label, obs)
	s.noMarkFields(label, obs)
	return obs
}

// cwEstimates are the estimate views AC-13 compares: the minimum with the
// default check selection, and the default baseline selection, as JSON and
// human-readable; cwEstimateViews names them in the expected outputs.
var (
	cwEstimates = [][]string{{"estimate", rgGoal, "--format", "json"}, {"estimate", rgGoal, "--baseline", "--format", "json"},
		{"estimate", rgGoal}, {"estimate", rgGoal, "--baseline"}}
	cwEstimateViews = []string{"estimate json", "estimate baseline json", "estimate text", "estimate baseline text"}
)

// cwEarlierRules are the rule versions AC-13 keeps.
var cwEarlierRules = []string{"run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4", "run-rules/5", "run-rules/6"}

// cwNames are the values of one repository that the expected outputs name
// instead of their bytes: its commits, by their subjects, and its temporary
// folders.
type cwNames struct {
	commits map[string]string
	paths   [][2]string
}

func cwNamesOf(f *rgFixture) cwNames {
	f.t.Helper()
	n := cwNames{commits: map[string]string{}}
	for _, line := range strings.Split(f.git("log", "--all", "--format=%H %s"), "\n") {
		if id, subject, ok := strings.Cut(line, " "); ok {
			n.commits[id] = "<commit " + subject + ">"
		}
	}
	for _, p := range [][2]string{{f.dir, "<repo>"}, {f.gates, "<gates>"}} {
		if real, err := filepath.EvalSymlinks(p[0]); err == nil && real != p[0] {
			n.paths = append(n.paths, [2]string{real, p[1]})
		}
		n.paths = append(n.paths, p)
	}
	sort.SliceStable(n.paths, func(i, j int) bool { return len(n.paths[i][0]) > len(n.paths[j][0]) })
	return n
}

// What the expected outputs leave out of human-readable text, besides
// commits and temporary folders: times (cwTimeText), the line hash after a
// record seq, and lengths of time. A run limit (the number after "/") stays.
var (
	cwKeptHead   = regexp.MustCompile(`seq (\d+) \([0-9a-f]{7,64}\)`)
	cwKeptWindow = regexp.MustCompile(`\b\d+/(\d+) seconds\b`)
	cwKeptSpan   = regexp.MustCompile(`\b(leaves out|window span) \d+ seconds\b`)
	cwKeptHex    = regexp.MustCompile(`\b[0-9a-f]{7,40}\b`)
)

func (n cwNames) text(s string) string {
	for _, p := range n.paths {
		s = strings.ReplaceAll(s, p[0], p[1])
	}
	s = cwTimeText.ReplaceAllString(s, "<time>")
	s = cwKeptHead.ReplaceAllString(s, "seq $1 (<line hash>)")
	s = cwKeptWindow.ReplaceAllString(s, "<seconds>/$1 seconds")
	s = cwKeptSpan.ReplaceAllString(s, "$1 <seconds> seconds")
	return cwKeptHex.ReplaceAllStringFunc(s, func(h string) string {
		for id, name := range n.commits {
			if strings.HasPrefix(id, h) {
				return name
			}
		}
		return h
	})
}

// value normalizes a decoded JSON value: times, lengths of time and the line
// hash of every record head become placeholders, and strings are normalized
// as text.
func (n cwNames) value(v any, key string) any {
	switch x := v.(type) {
	case map[string]any:
		m := map[string]any{}
		for k, val := range x {
			s, isText := val.(string)
			_, isNumber := val.(float64)
			switch {
			case (k == "at" || k == "started_at") && isText && s != "":
				m[k] = "<time>"
			case cwTimeKeys[k] && isNumber:
				m[k] = "<seconds>"
			case key == "record_head" && k == "sha256" && isText:
				m[k] = "<line hash>"
			default:
				m[k] = n.value(val, k)
			}
		}
		return m
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = n.value(val, key)
		}
		return out
	case string:
		return n.text(x)
	}
	return v
}

func cwCompact(v any) string {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return fmt.Sprint(v)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

// json renders a normalized JSON document as lines: one per field (only the
// given ones, when keys are given), and one per item of a list that is not
// empty.
func (n cwNames) json(doc map[string]any, keys ...string) []string {
	if len(keys) == 0 {
		for k := range doc {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var lines []string
	for _, k := range keys {
		val, ok := doc[k]
		if !ok {
			lines = append(lines, k+": (absent)")
			continue
		}
		val = n.value(val, k)
		if list, isList := val.([]any); isList && len(list) > 0 {
			for i, item := range list {
				lines = append(lines, fmt.Sprintf("%s[%d]: %s", k, i, cwCompact(item)))
			}
			continue
		}
		lines = append(lines, k+": "+cwCompact(val))
	}
	return lines
}

// output renders a command's exit code and normalized output as lines.
func (n cwNames) output(code int, out string) []string {
	return append([]string{fmt.Sprintf("exit %d", code)}, strings.Split(strings.TrimRight(n.text(out), "\n"), "\n")...)
}

// cwKeptFor is the expected output of rules: the lines of view that carry no
// rule tag, and those whose tag ("@4,5,6 ") names its number.
func cwKeptFor(view []string, rules string) []string {
	number := strings.TrimPrefix(rules, "run-rules/")
	var out []string
	for _, line := range view {
		if tag, rest, ok := strings.Cut(line, " "); ok && strings.HasPrefix(tag, "@") {
			if !slices.Contains(strings.Split(tag[1:], ","), number) {
				continue
			}
			line = rest
		}
		out = append(out, line)
	}
	return out
}

// keep compares a view of the only twin of a set with kept outputs with the
// expected lines of its rule. A view the check has no expected lines for is
// a broken check.
func (s *cwTwins) keep(view string, render func() []string) {
	s.t.Helper()
	if s.kept == "" {
		return
	}
	expected, ok := cwKept[view]
	if !ok {
		s.t.Fatalf("setup: no expected output %q", view)
	}
	want, got := cwKeptFor(expected, s.kept), render()
	if view == "steps" {
		// The steps so far; a flow with more steps than the expected list
		// is a changed check, not a changed ha.
		if len(got) > len(want) {
			s.t.Fatalf("setup: step %d %q has no expected line", len(got), got[len(got)-1])
		}
		want = want[:len(got)]
	}
	if n, a, b := cwFirstDiff(strings.Join(want, "\n"), strings.Join(got, "\n")); n > 0 {
		s.t.Fatalf("jaekit:violation=%s %s, %s: line %d is %q, want %q as this tree's ha gave it before this goal",
			s.violation, s.twins[0].name, view, n, b, a)
	}
}

// keepState compares the status JSON, the human-readable status and the
// Korean and English reports of the only twin with the expected ones at a
// fixed point.
func (s *cwTwins) keepState(when string, obs []cwObs) {
	s.t.Helper()
	if s.kept == "" {
		return
	}
	f := s.twins[0].f
	n := cwNamesOf(f)
	s.keep("status json "+when, func() []string { return n.json(obs[0].state.value) })
	for _, view := range [][]string{{"status", rgGoal, "--format", "md"}, {"report", rgGoal, "--lang", "ko"}, {"report", rgGoal, "--lang", "en"}} {
		s.keep(view[0]+" "+view[len(view)-1]+" "+when, func() []string {
			code, out := f.ha(view...)
			if code == exitInternal || code == exitUsage {
				s.t.Fatalf("setup: ha %v exit %d: %s", view, code, cwFirst(out))
			}
			return n.output(code, out)
		})
	}
}

// keepEstimate compares one estimate view (cwEstimates[i]) of the only twin
// with the expected one: every field of a JSON view, or the exit and every
// line of a human-readable one.
func (s *cwTwins) keepEstimate(i int, when string, obs []cwObs) {
	s.t.Helper()
	view := cwEstimateViews[i] + " " + when
	s.keep(view, func() []string {
		n := cwNamesOf(s.twins[0].f)
		if args := cwEstimates[i]; args[len(args)-1] != "json" {
			return n.output(obs[0].code, obs[0].out)
		}
		var doc map[string]any
		if err := json.Unmarshal([]byte(obs[0].out), &doc); err != nil {
			s.t.Fatalf("setup: %s: ha estimate printed no JSON: %v: %s", view, err, cwFirst(obs[0].out))
		}
		return n.json(doc)
	})
}

// cwEarlierSteps are AC-13's steps on a set of one earlier rule: the start,
// the checks, the default baseline selection and ha done, then, when later
// is one of the twins, the mark column added to its completed goal. A set
// with kept outputs compares its views at fixed points; a set of twins
// compares the twins after every step.
func cwEarlierSteps(s *cwTwins, later *cwTwin) {
	s.t.Helper()
	ref := s.twins[0]
	// estimates runs every estimate view; a set with kept outputs compares
	// each with the expected one at this point.
	estimates := func(when string) {
		s.t.Helper()
		for i, args := range cwEstimates {
			obs := s.query(when+": ha "+strings.Join(args, " "), cwHa(args...))
			s.keepEstimate(i, when, obs)
		}
	}
	lint := func(label string) {
		s.t.Helper()
		obs := s.query(label, cwHa("lint", rgGoal))
		s.keep(label, func() []string { return cwNamesOf(ref.f).output(obs[0].code, obs[0].out) })
	}
	lint("ha lint before the start")
	estimates("before the start")
	s.start("the start")
	lint("ha lint after the start")
	estimates("after the start")

	s.commit("the code commit", "src/one.txt", "ok\n", "src/two.txt", "ok\n")
	obs := s.same("every required condition checked", exitOK, cwHa("check", rgGoal))
	s.keepState("with the baselines missing", obs)
	s.has("every required condition checked", obs, "AC-1:baseline_missing", true)
	s.has("every required condition checked", obs, "AC-2:baseline_missing", true)
	obs = s.same("AC-1's baseline", exitOK, cwHa("check", rgGoal, "--baseline", "AC-1"))
	s.has("AC-1's baseline", obs, "AC-1:", false)
	s.has("AC-1's baseline", obs, "AC-2:baseline_missing", true)
	s.same("ha done with AC-2's baseline missing", cwPending, cwHa("done", rgGoal))
	estimates("with AC-2's baseline missing")

	label := "the default baseline selection"
	obs = s.same(label, exitOK, cwHa("check", rgGoal, "--baseline"))
	if want := "appended baseline/AC-1/fail_as_expected, baseline/AC-2/fail_as_expected"; s.kept == "" && obs[0].record != want {
		s.t.Fatalf("setup: %s: the run record %s in %s, want %s", label, obs[0].record, ref.name, want)
	}
	obs = s.same("ha done", exitOK, cwHa("done", rgGoal))
	s.keepState("after ha done", obs)
	if s.kept == "" && obs[0].state.completion == nil {
		s.t.Fatalf("setup: ha done: no completion record in %s", ref.name)
	}
	if s.kept != "" {
		if len(s.steps) != len(cwKeptFor(cwKept["steps"], s.kept)) {
			s.t.Fatalf("setup: %d steps, but the expected output has %d", len(s.steps), len(cwKeptFor(cwKept["steps"], s.kept)))
		}
		return
	}

	// The column added to the completed goal changes no judgment, view or
	// record.
	label = "the baseline column added after the completion"
	obs = s.same(label, exitOK, func(tw *cwTwin) (int, string) {
		if tw == later {
			tw.plan.marks = []string{cwReq, cwReq, "—"}
			tw.f.put(rgGoal+"/PLAN.md", tw.plan.text())
		}
		return exitOK, ""
	})
	if obs[0].state.status != "complete" {
		s.t.Fatalf("setup: %s: status %s in %s", label, obs[0].state.status, ref.name)
	}
	s.query("ha lint after the completion", cwHa("lint", rgGoal))
	estimates("after the completion")
	s.same("ha done again", exitOK, cwHa("done", rgGoal))
}

// AC-13: for each earlier rule (run-rules/1 to /6), a goal gives the steps'
// exits, record changes, statuses and reasons, the lint result, every field
// of `ha estimate --format json` and every line of the human-readable `ha
// estimate`, each with and without --baseline, before and after the start
// and with AC-2's baseline missing, and at two points (the baselines
// missing, and after ha done) the `ha status` JSON and human output and the
// Korean and English `ha report` that this tree's ha gave before this goal
// (cwKept). The same goal whose PLAN has the baseline column, from its start
// or added after its completion, gives the same lint result, estimates,
// judgment, reasons, completion, `ha status` JSON and human output and `ha
// report` as the goal without the column; no command rewrites its record;
// and a start without --rules still uses run-rules/3. This tree's ha starts
// run-rules/3 to /6; a run-rules/1 or /2 goal is a saved-record fixture
// (cwSaveRules). The kept outputs of every rule are compared before any
// twins, so a base where only the column differs fails on the twins.
func TestCheckCostAC13_EarlierRulesIgnoreMark(t *testing.T) {
	const v = "earlier-rules-change-with-mark"
	for _, rules := range cwEarlierRules {
		func() {
			plain := &cwTwin{name: "the " + rules + " goal without the baseline column", rules: rules}
			s := newCWTwins(t, v, "", plain)
			s.kept = rules
			cwEarlierSteps(s, nil)
		}()
	}
	for _, rules := range cwEarlierRules {
		func() {
			plain := &cwTwin{name: "the " + rules + " goal without the baseline column", rules: rules}
			marked := &cwTwin{name: "the " + rules + " goal with the baseline column", rules: rules,
				plan: cwPlan{marks: []string{cwReq, "", "—"}}}
			later := &cwTwin{name: "the " + rules + " goal that gains the baseline column after its completion", rules: rules}
			s := newCWTwins(t, v, "", plain, marked, later)
			s.human = true
			cwEarlierSteps(s, later)
		}()
	}

	// The default start rule.
	fresh := cwRepo(t, "", cwPlan{})
	code, out := fresh.ha("capabilities", "--format", "json")
	var info struct {
		Default string `json:"default_rules"`
	}
	if err := json.Unmarshal([]byte(out), &info); code != exitOK || err != nil {
		t.Fatalf("setup: ha capabilities exit %d: %v: %s", code, err, cwFirst(out))
	}
	if info.Default != "run-rules/3" {
		t.Fatalf("jaekit:violation=%s ha capabilities names the default start rule %q, want run-rules/3", v, info.Default)
	}
	if code, out := fresh.ha("start", rgGoal, "--request", cwRequest); code != exitOK {
		t.Fatalf("setup: ha start without --rules exit %d: %s", code, cwFirst(out))
	}
	if got := fresh.status().value["rules"]; got != "run-rules/3" {
		t.Fatalf("jaekit:violation=%s a start without --rules computes under %v, want run-rules/3", v, got)
	}
}

// AC-14: a run-rules/7 goal whose every change condition is marked runs the
// same steps as the same goal under run-rules/6 with the same exit codes, run
// record changes and status JSON (without the rule, times, line hashes,
// criteria[].baseline_required and check_freeze). The steps cover the marked
// conditions' baselines (default selection, freshness after a check file
// changes, an unexpected pass, the error limit and recovery of the baseline
// input), current violations, test change authority, reported waits,
// pre-approved limits, time limits, the dirty refusal, the baseline copy
// refusal, out-of-scope changes and review gate states.
func TestCheckCostAC14_SameAsRulesV6Elsewhere(t *testing.T) {
	const v = "rules-v7-differs-from-v6"
	old := &cwTwin{name: "the run-rules/6 goal", rules: "run-rules/6"}
	now := &cwTwin{name: "the run-rules/7 goal with every change condition marked", rules: "run-rules/7",
		plan: cwPlan{marks: []string{cwReq, cwReq, "—"}}}
	s := newCWTwins(t, v, "standard", old, now)
	for _, k := range []string{"rules", "baseline_required", "check_freeze"} {
		s.drop[k] = true
	}
	s.start("the start", "--test-authority", cwAuthority)
	checkAll := cwHa("check", rgGoal)

	// The marked conditions' checks and baselines, and review reasons
	// without a gate.
	s.commit("a code commit", "src/one.txt", "ok\n", "src/two.txt", "ok\n")
	for _, tw := range s.twins {
		tw.f.work = tw.f.head
	}
	obs := s.step("every required condition checked", exitOK, checkAll)
	s.has("every required condition checked", obs, "AC-1:baseline_missing", true)
	s.has("every required condition checked", obs, "AC-2:baseline_missing", true)
	obs = s.step("the default baseline selection", exitOK, cwHa("check", rgGoal, "--baseline"))
	if want := "appended baseline/AC-1/fail_as_expected, baseline/AC-2/fail_as_expected"; obs[0].record != want {
		t.Fatalf("setup: the default baseline selection: the run record %s in %s, want %s", obs[0].record, old.name, want)
	}
	s.has("the default baseline selection", obs, "AC-1:", false)
	s.has("the default baseline selection", obs, "AC-2:", false)
	s.review("the default baseline selection", obs, "review_missing")
	s.done("ha done without a gate")

	// Baseline freshness: a changed check file leaves both baselines stale
	// until each is recorded again.
	obs = s.commit("the producer changed", "checks/produce.sh", cwProducer+"# the producer reads one file\n")
	s.has("the producer changed", obs, "AC-1:", true)
	s.has("the producer changed", obs, "AC-2:", true)
	obs = s.step("every required condition checked after the producer changed", exitOK, checkAll)
	s.has("every required condition checked after the producer changed", obs, "AC-1:baseline_missing", true)
	obs = s.step("AC-1's baseline recorded again", exitOK, cwHa("check", rgGoal, "--baseline", "AC-1"))
	s.has("AC-1's baseline recorded again", obs, "AC-1:", false)
	s.has("AC-1's baseline recorded again", obs, "AC-2:baseline_missing", true)
	obs = s.step("AC-2's baseline recorded again", exitOK, cwHa("check", rgGoal, "--baseline", "AC-2"))
	s.has("AC-2's baseline recorded again", obs, "AC-2:", false)

	// Structured results: a current violation, then the fixed value.
	s.commit("AC-1's value broken", "src/one.txt", "no\n")
	obs = s.step("AC-1 checked with its violation", exitProblems, cwHa("check", rgGoal, "AC-1"))
	s.has("AC-1 checked with its violation", obs, "AC-1:", true)
	s.commit("AC-1's value fixed", "src/one.txt", "ok\n")
	obs = s.step("every required condition checked after the fix", exitOK, checkAll)
	s.has("every required condition checked after the fix", obs, "AC-1:", false)

	// An unexpected baseline pass: AC-1's command observes a value the base
	// commit already has.
	vacuous := func(p *cwPlan) {
		p.commands = map[string]string{"AC-1": "sh checks/produce.sh one src/value.txt ok one-not-ok"}
	}
	s.edit("AC-1's command observes a value the base already has", vacuous)
	obs = s.step("AC-1's baseline passes at the base", cwPending, cwHa("check", rgGoal, "--baseline", "AC-1"))
	if want := "appended baseline/AC-1/unexpected_pass"; obs[0].record != want {
		t.Fatalf("setup: AC-1's baseline passes at the base: the run record %s in %s, want %s", obs[0].record, old.name, want)
	}
	s.has("AC-1's baseline passes at the base", obs, "AC-1:", true)
	obs = s.edit("AC-1's command restored", func(p *cwPlan) { p.commands = nil })
	s.has("AC-1's command restored", obs, "AC-1:", false)

	// A changed existing test under the recorded authority, explained by a
	// test-change note.
	obs = s.commit("an existing test changed", "tests/value.sh", "grep -qx ok src/value.txt\n# the check reads one line\n")
	s.has("an existing test changed", obs, "test_definition_changed", true)
	obs = s.step("the test change explained", exitOK,
		cwHa("note", rgGoal, "test-change", "tests:tests/value.sh", "--criteria", "AC-3", "--how", "it still reads src/value.txt"))
	s.has("the test change explained", obs, "test_definition_changed", false)
	obs = s.step("every required condition checked after the test change", exitOK, checkAll)
	s.has("every required condition checked after the test change", obs, "AC-1:", false)

	// A reported wait.
	s.step("a wait", exitOK, cwHa("note", rgGoal, "wait", "--reason", "a synthetic decision asked of the user"))
	s.step("the resume", exitOK, cwHa("note", rgGoal, "resume"))

	// Budget changes: a pre-approved limit, a raise inside it with a reason,
	// a raise past it with only a reason, and a raise with the user's words.
	budget := func(extra ...string) cwAct {
		return func(tw *cwTwin) (int, string) { return tw.f.ha(cwBudgetArgs(tw.f, extra...)...) }
	}
	s.step("a pre-approved limit", exitOK, budget("--preapprove", "80", "--quote", cwQuote, "--context", cwContext))
	s.step("a raise inside the pre-approved limit", exitOK, budget("--runs", "60", "--from", "{limit}", "--reason", "more checks"))
	s.refused("a raise past the pre-approved limit with only a reason", budget("--runs", "90", "--from", "{limit}", "--reason", "more checks"))
	s.step("a raise with the user's words", exitOK, budget("--runs", "100", "--from", "{limit}", "--quote", cwQuote, "--context", cwContext))

	// Per-command time limits: a valid entry, then one over the maximum.
	obs = s.edit("a time limit for AC-3", func(p *cwPlan) { p.extra = "\n## 시간 제한\n- AC-3: 5분\n" })
	s.has("a time limit for AC-3", obs, "AC-3:", true)
	obs = s.step("AC-3 checked under its time limit", exitOK, cwHa("check", rgGoal, "AC-3"))
	s.has("AC-3 checked under its time limit", obs, "AC-3:", false)
	obs = s.edit("a time limit over the maximum", func(p *cwPlan) { p.extra = "\n## 시간 제한\n- AC-3: 3시간\n" })
	s.has("a time limit over the maximum", obs, "lint_error", true)
	s.refused("a check under a time limit over the maximum", cwHa("check", rgGoal, "AC-3"))
	obs = s.edit("the valid time limit again", func(p *cwPlan) { p.extra = "\n## 시간 제한\n- AC-3: 5분\n" })
	s.has("the valid time limit again", obs, "lint_error", false)

	// The error limit and recovery of AC-2's baseline input: its command
	// names a program that does not exist, so each baseline records an
	// error; the second fresh error passes the limit of one rerun. The
	// executor recovers twice, a third recovery is refused, the user allows
	// one more, and the restored command's baseline counts again.
	s.edit("AC-2's command replaced by a program that does not exist", func(p *cwPlan) {
		p.commands = map[string]string{"AC-2": cwMissingProgram}
	})
	baseline := cwHa("check", rgGoal, "--baseline", "AC-2")
	recover := cwHa("note", rgGoal, "recover", "AC-2", "--input", "baseline", "--change", "the command names a program that exists")
	errorLimit := func(label string) {
		t.Helper()
		obs := s.step(label+": the first error", exitProblems, baseline)
		s.has(label+": the first error", obs, "AC-2:error_limit", false)
		obs = s.step(label+": the second error", exitProblems, baseline)
		s.has(label+": the second error", obs, "AC-2:error_limit", true)
	}
	recovered := func(label string, used, allowed int) {
		t.Helper()
		obs := s.step(label, exitOK, recover)
		s.has(label, obs, "AC-2:error_limit", false)
		s.recoveries(label, obs, "AC-2", "baseline", used, allowed)
	}
	obs = s.step("an AC-2 baseline that cannot start", exitProblems, baseline)
	s.has("an AC-2 baseline that cannot start", obs, "AC-2:error_limit", false)
	s.refused("a recovery before the error limit", recover)
	obs = s.step("a second AC-2 baseline that cannot start", exitProblems, baseline)
	s.has("a second AC-2 baseline that cannot start", obs, "AC-2:error_limit", true)
	recovered("the first recovery", 1, 2)
	errorLimit("after the first recovery")
	recovered("the second recovery", 2, 2)
	errorLimit("after the second recovery")
	s.refused("a third recovery without the user's allowance", recover)
	obs = s.step("the user allows one more recovery", exitOK,
		cwHa("note", rgGoal, "allow-recovery", "AC-2", "--input", "baseline", "--count", "1", "--quote", cwQuote))
	s.has("the user allows one more recovery", obs, "AC-2:error_limit", true)
	s.recoveries("the user allows one more recovery", obs, "AC-2", "baseline", 2, 3)
	recovered("the allowed recovery", 3, 3)
	obs = s.edit("AC-2's command restored", func(p *cwPlan) { p.commands = nil })
	s.has("AC-2's command restored", obs, "AC-2:", false)

	// The dirty refusal: an untracked file refuses a check and records
	// nothing.
	obs = s.step("an untracked file", exitOK, func(tw *cwTwin) (int, string) {
		tw.f.put("src/untracked.txt", "untracked\n")
		return exitOK, ""
	})
	s.has("an untracked file", obs, "worktree_dirty", true)
	s.refused("a check of a dirty tree", cwHa("check", rgGoal, "AC-1"))
	s.refused("a baseline of a dirty tree", cwHa("check", rgGoal, "--baseline", "AC-1"))
	obs = s.step("the untracked file removed", exitOK, func(tw *cwTwin) (int, string) {
		if err := os.Remove(filepath.Join(tw.f.dir, "src", "untracked.txt")); err != nil {
			t.Fatalf("setup: %v", err)
		}
		return exitOK, ""
	})
	s.has("the untracked file removed", obs, "worktree_dirty", false)

	// The baseline copy refusal: a check path that is a symbolic link at
	// HEAD is never copied onto the base commit.
	for _, tw := range s.twins {
		f := tw.f
		f.put("checks/produce-real.sh", cwProducer+"# the producer reads one file\n")
		link := filepath.Join(f.dir, "checks", "produce.sh")
		if err := os.Remove(link); err != nil {
			t.Fatalf("setup: %v", err)
		}
		if err := os.Symlink("produce-real.sh", link); err != nil {
			t.Fatalf("setup: %v", err)
		}
		f.head = f.commit("the producer as a symbolic link")
	}
	s.sameHead("the producer as a symbolic link")
	s.step("the producer as a symbolic link", exitOK, cwNothing)
	s.refused("a baseline with a symbolic link among its check paths", cwHa("check", rgGoal, "--baseline", "AC-1"))
	obs = s.commit("the producer as a regular file again", "checks/produce.sh", cwProducer+"# the producer reads one file\n",
		"checks/produce-real.sh", nil)
	obs = s.step("every required condition checked after the producer was restored", exitOK, checkAll)
	s.has("every required condition checked after the producer was restored", obs, "AC-1:", false)
	s.has("every required condition checked after the producer was restored", obs, "AC-2:", false)

	// An out-of-scope change, then its removal.
	obs = s.commit("a file outside the scope", "notes/outside.txt", "outside\n")
	s.has("a file outside the scope", obs, "out_of_scope", true)
	obs = s.commit("the file outside the scope removed", "notes/outside.txt", nil)
	s.has("the file outside the scope removed", obs, "out_of_scope", false)
	s.has("the file outside the scope removed", obs, "AC-1:", false)

	// Gates that leave completion pending, each with its reasons.
	for _, tw := range s.twins {
		tw.f.work = tw.f.head
	}
	state := func(st string, defect, question int) func(f *rgFixture) rgGate {
		return func(f *rgFixture) rgGate {
			g := f.covering("candidate")
			g.state, g.defect, g.question = st, defect, question
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
	} {
		obs := s.gate(form.label, form.build)
		s.review(form.label, obs, form.want...)
		s.done("ha done after " + form.label)
	}

	// Gate files that cannot be recorded leave the record as it was.
	for _, form := range []struct {
		label string
		path  func(f *rgFixture) string
	}{
		{"a missing gate file", func(f *rgFixture) string { return filepath.Join(f.gates, "missing.json") }},
		{"a gate file that is not JSON", func(f *rgFixture) string { return f.writeBytes([]byte("{not json")) }},
		{"a gate of another schema", func(f *rgFixture) string {
			return f.writeBytes(cwMutate(f, func(m map[string]any) { m["schema"] = "review-gate/v2" }))
		}},
	} {
		s.refused(form.label, func(tw *cwTwin) (int, string) {
			return tw.f.ha("note", rgGoal, "review", "--gate", form.path(tw.f))
		})
	}

	// A covering gate, then a commit after it and goal documents changed
	// after it.
	covering := func(stages ...string) func(f *rgFixture) rgGate {
		return func(f *rgFixture) rgGate { return f.covering(stages...) }
	}
	obs = s.gate("a covering gate", covering("candidate"))
	s.review("a covering gate", obs)
	s.commit("a commit after the review", "src/later.txt", "later\n")
	obs = s.step("every required condition checked after the review", exitOK, checkAll)
	s.review("every required condition checked after the review", obs, "review_head_stale")
	s.done("ha done with a stale reviewed head")
	obs = s.gate("a gate of the new head", covering("candidate"))
	s.review("a gate of the new head", obs)
	s.step("a linked goal document changed", exitOK, func(tw *cwTwin) (int, string) {
		tw.f.put(rgGoal+"/note.md", "# Value note\n\nEach value is one lowercase word on one line.\n")
		return exitOK, ""
	})
	s.step("the changed goal documents confirmed", exitOK, cwHa("note", rgGoal, "confirm", "spec", "--quote", cwQuote))
	s.step("every required condition checked after the goal documents changed", exitOK, checkAll)
	obs = s.step("the baselines after the goal documents changed", exitOK, cwHa("check", rgGoal, "--baseline"))
	s.review("the baselines after the goal documents changed", obs, "review_docs_stale")
	s.done("ha done with stale goal documents")
	obs = s.gate("a gate of the current goal documents", covering("candidate"))
	s.review("a gate of the current goal documents", obs)

	// A strict goal without a plan round, then with one.
	s.step("the tier raised to strict", exitOK, func(tw *cwTwin) (int, string) {
		tw.f.put(rgGoal+"/SPEC.md", cwSpec("strict"))
		return exitOK, ""
	})
	s.step("the strict tier confirmed", exitOK, cwHa("note", rgGoal, "confirm", "spec", "--quote", cwQuote))
	s.step("every required condition checked under the strict tier", exitOK, checkAll)
	obs = s.step("the baselines under the strict tier", exitOK, cwHa("check", rgGoal, "--baseline"))
	s.review("the baselines under the strict tier", obs, "review_docs_stale", "review_plan_missing")
	obs = s.gate("a strict goal's gate without a plan round", covering("candidate"))
	s.review("a strict goal's gate without a plan round", obs, "review_plan_missing")
	s.done("ha done without a plan round")
	obs = s.gate("a strict goal's covering gate", covering("plan", "candidate"))
	s.review("a strict goal's covering gate", obs)
	if obs[0].state.status != "complete" {
		t.Fatalf("setup: a strict goal's covering gate: status %s, reasons %v in %s; want complete", obs[0].state.status, obs[0].state.reasons, old.name)
	}
	obs = s.step("ha done", exitOK, cwHa("done", rgGoal))
	if obs[0].state.completion == nil {
		t.Fatalf("setup: ha done: no completion record in %s", old.name)
	}
	obs = s.step("a reopen after the completion record", exitOK, cwHa("note", rgGoal, "reopen", "--quote", cwQuote))
	if obs[0].state.completion != nil {
		t.Fatalf("setup: a reopen after the completion record: completion record %v in %s", obs[0].state.completion, old.name)
	}
}

// cwKept holds, for each view AC-13 compares, the normalized output this
// tree's ha gave before this goal under run-rules/1 to /6. A line that
// starts with "@" and rule numbers ("@4,5,6 ") is in the output of those
// rules only; every other line is in all six.
var cwKept = map[string][]string{
	"steps": {
		`ha lint before the start: exit 0; record unchanged; status none`,
		`before the start: ha estimate docs/specs/demo --format json: exit 0; record unchanged; status none`,
		`before the start: ha estimate docs/specs/demo --baseline --format json: exit 0; record unchanged; status none`,
		`before the start: ha estimate docs/specs/demo: exit 0; record unchanged; status none`,
		`before the start: ha estimate docs/specs/demo --baseline: exit 0; record unchanged; status none`,
		`the start: exit 0; record appended start; status incomplete (exit 1) [AC-1:criterion_missing AC-2:criterion_missing AC-3:criterion_missing]`,
		`ha lint after the start: exit 0; record unchanged; status incomplete (exit 1) [AC-1:criterion_missing AC-2:criterion_missing AC-3:criterion_missing]`,
		`after the start: ha estimate docs/specs/demo --format json: exit 0; record unchanged; status incomplete (exit 1) [AC-1:criterion_missing AC-2:criterion_missing AC-3:criterion_missing]`,
		`after the start: ha estimate docs/specs/demo --baseline --format json: exit 0; record unchanged; status incomplete (exit 1) [AC-1:criterion_missing AC-2:criterion_missing AC-3:criterion_missing]`,
		`after the start: ha estimate docs/specs/demo: exit 0; record unchanged; status incomplete (exit 1) [AC-1:criterion_missing AC-2:criterion_missing AC-3:criterion_missing]`,
		`after the start: ha estimate docs/specs/demo --baseline: exit 0; record unchanged; status incomplete (exit 1) [AC-1:criterion_missing AC-2:criterion_missing AC-3:criterion_missing]`,
		`the code commit: exit 0; record unchanged; status incomplete (exit 1) [AC-1:criterion_missing AC-2:criterion_missing AC-3:criterion_missing]`,
		`every required condition checked: exit 0; record appended check/AC-1/pass, check/AC-2/pass, check/AC-3/pass; status incomplete (exit 1) [AC-1:baseline_missing AC-2:baseline_missing]`,
		`AC-1's baseline: exit 0; record appended baseline/AC-1/fail_as_expected; status incomplete (exit 1) [AC-2:baseline_missing]`,
		`ha done with AC-2's baseline missing: exit 1; record appended done/incomplete; status incomplete (exit 1) [AC-2:baseline_missing]`,
		`with AC-2's baseline missing: ha estimate docs/specs/demo --format json: exit 0; record unchanged; status incomplete (exit 1) [AC-2:baseline_missing]`,
		`with AC-2's baseline missing: ha estimate docs/specs/demo --baseline --format json: exit 0; record unchanged; status incomplete (exit 1) [AC-2:baseline_missing]`,
		`with AC-2's baseline missing: ha estimate docs/specs/demo: exit 0; record unchanged; status incomplete (exit 1) [AC-2:baseline_missing]`,
		`with AC-2's baseline missing: ha estimate docs/specs/demo --baseline: exit 0; record unchanged; status incomplete (exit 1) [AC-2:baseline_missing]`,
		`the default baseline selection: exit 0; record appended baseline/AC-1/fail_as_expected, baseline/AC-2/fail_as_expected; status complete (exit 0) []`,
		`ha done: exit 0; record appended done/complete; status complete (exit 0) []`,
	},
	"ha lint before the start": {
		`exit 0`,
		`ok: no lint problems`,
	},
	"estimate json before the start": {
		`budget_source: "PLAN.md"`,
		`default_additional_runs: 0`,
		`default_baseline_runs: 2`,
		`default_check_runs: 3`,
		`goal: "docs/specs/demo"`,
		`headroom: 45`,
		`limitation: "Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged."`,
		`minimum_runs: 5`,
		`plan_runs_limit: 50`,
		`required_change: 2`,
		`required_maintain: 1`,
		`schema: "estimate/v1"`,
		`selected: {"additional_runs":0,"mode":"check","optional_runs":0,"repeated_required_runs":0,"runs":3,"targets":["AC-1","AC-2","AC-3"],"task_runs":0}`,
		`shortfall: 0`,
	},
	"estimate baseline json before the start": {
		`budget_source: "PLAN.md"`,
		`default_additional_runs: 0`,
		`default_baseline_runs: 2`,
		`default_check_runs: 3`,
		`goal: "docs/specs/demo"`,
		`headroom: 45`,
		`limitation: "Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged."`,
		`minimum_runs: 5`,
		`plan_runs_limit: 50`,
		`required_change: 2`,
		`required_maintain: 1`,
		`schema: "estimate/v1"`,
		`selected: {"additional_runs":0,"mode":"baseline","optional_runs":0,"repeated_required_runs":0,"runs":2,"targets":["AC-1","AC-2"],"task_runs":0}`,
		`shortfall: 0`,
	},
	"estimate text before the start": {
		`exit 0`,
		`## ha estimate: docs/specs/demo`,
		"",
		`- required minimum: 5 runs (2 change × 2 + 1 maintain; manual: 0)`,
		`- PLAN run limit: 50; shortfall: 0; headroom above minimum: 45`,
		`- default commands: check 3 + baseline 2 runs; additional to minimum: 0`,
		`- selected check: 3 runs; additional: 0 (optional 0, task 0, repeated required 0)`,
		`  - AC-1`,
		`  - AC-2`,
		`  - AC-3`,
		"",
		`Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged.`,
	},
	"estimate baseline text before the start": {
		`exit 0`,
		`## ha estimate: docs/specs/demo`,
		"",
		`- required minimum: 5 runs (2 change × 2 + 1 maintain; manual: 0)`,
		`- PLAN run limit: 50; shortfall: 0; headroom above minimum: 45`,
		`- default commands: check 3 + baseline 2 runs; additional to minimum: 0`,
		`- selected baseline: 2 runs; additional: 0 (optional 0, task 0, repeated required 0)`,
		`  - AC-1`,
		`  - AC-2`,
		"",
		`Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged.`,
	},
	"ha lint after the start": {
		`exit 0`,
		`ok: no lint problems`,
	},
	"estimate json after the start": {
		`budget_source: "PLAN.md"`,
		`default_additional_runs: 0`,
		`default_baseline_runs: 2`,
		`default_check_runs: 3`,
		`goal: "docs/specs/demo"`,
		`headroom: 45`,
		`limitation: "Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged."`,
		`minimum_runs: 5`,
		`plan_runs_limit: 50`,
		`required_change: 2`,
		`required_maintain: 1`,
		`schema: "estimate/v1"`,
		`selected: {"additional_runs":0,"mode":"check","optional_runs":0,"repeated_required_runs":0,"runs":3,"targets":["AC-1","AC-2","AC-3"],"task_runs":0}`,
		`shortfall: 0`,
	},
	"estimate baseline json after the start": {
		`budget_source: "PLAN.md"`,
		`default_additional_runs: 0`,
		`default_baseline_runs: 2`,
		`default_check_runs: 3`,
		`goal: "docs/specs/demo"`,
		`headroom: 45`,
		`limitation: "Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged."`,
		`minimum_runs: 5`,
		`plan_runs_limit: 50`,
		`required_change: 2`,
		`required_maintain: 1`,
		`schema: "estimate/v1"`,
		`selected: {"additional_runs":0,"mode":"baseline","optional_runs":0,"repeated_required_runs":0,"runs":2,"targets":["AC-1","AC-2"],"task_runs":0}`,
		`shortfall: 0`,
	},
	"estimate text after the start": {
		`exit 0`,
		`## ha estimate: docs/specs/demo`,
		"",
		`- required minimum: 5 runs (2 change × 2 + 1 maintain; manual: 0)`,
		`- PLAN run limit: 50; shortfall: 0; headroom above minimum: 45`,
		`- default commands: check 3 + baseline 2 runs; additional to minimum: 0`,
		`- selected check: 3 runs; additional: 0 (optional 0, task 0, repeated required 0)`,
		`  - AC-1`,
		`  - AC-2`,
		`  - AC-3`,
		"",
		`Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged.`,
	},
	"estimate baseline text after the start": {
		`exit 0`,
		`## ha estimate: docs/specs/demo`,
		"",
		`- required minimum: 5 runs (2 change × 2 + 1 maintain; manual: 0)`,
		`- PLAN run limit: 50; shortfall: 0; headroom above minimum: 45`,
		`- default commands: check 3 + baseline 2 runs; additional to minimum: 0`,
		`- selected baseline: 2 runs; additional: 0 (optional 0, task 0, repeated required 0)`,
		`  - AC-1`,
		`  - AC-2`,
		"",
		`Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged.`,
	},
	"status json with the baselines missing": {
		`assurance: "local"`,
		`@1,2,3 budget: {"elapsed_limit_seconds":14400,"elapsed_seconds":"<seconds>","exceeded":false,"excess_runs":0,"history":[],"reached":false,"remaining_runs":47,"revision":1,"runs":3,"runs_limit":50,"window_start":1}`,
		`@4,5 budget: {"elapsed_limit_seconds":14400,"elapsed_seconds":"<seconds>","exceeded":false,"excess_runs":0,"history":[],"reached":false,"remaining_runs":47,"reported_wait_seconds":"<seconds>","revision":1,"runs":3,"runs_limit":50,"span_seconds":"<seconds>","wait_basis":"executor_reported","window_start":1}`,
		`@6 budget: {"elapsed_limit_seconds":14400,"elapsed_seconds":"<seconds>","exceeded":false,"excess_runs":0,"history":[],"post_completion_seconds":"<seconds>","reached":false,"remaining_runs":47,"reported_wait_seconds":"<seconds>","revision":1,"runs":3,"runs_limit":50,"span_seconds":"<seconds>","wait_basis":"executor_reported","window_start":1}`,
		`changes: {"command_changed":[],"ha_version_changed":[],"invalid_confirmations":[],"kind_changed":[],"optional":[]}`,
		`check_author: "executor"`,
		`commit: "<commit the code commit>"`,
		`complete_without_record: false`,
		`completion_record: null`,
		`criteria[0]: {"attempts":{"baseline_errors":0,"baseline_fails":0,"baseline_unknowns":0,"baselines":0,"checks":1,"errors":0,"fail_commits":[],"fails":0,"stale":0,"unexpected_passes":0,"unknowns":0},"id":"AC-1","kind":"change","reason":"baseline_missing","records":[],"required":true,"satisfied":false}`,
		`criteria[1]: {"attempts":{"baseline_errors":0,"baseline_fails":0,"baseline_unknowns":0,"baselines":0,"checks":1,"errors":0,"fail_commits":[],"fails":0,"stale":0,"unexpected_passes":0,"unknowns":0},"id":"AC-2","kind":"change","reason":"baseline_missing","records":[],"required":true,"satisfied":false}`,
		`criteria[2]: {"attempts":{"baseline_errors":0,"baseline_fails":0,"baseline_unknowns":0,"baselines":0,"checks":1,"errors":0,"fail_commits":[],"fails":0,"stale":0,"unexpected_passes":0,"unknowns":0},"id":"AC-3","kind":"maintain","reason":"","records":[4],"required":true,"satisfied":true}`,
		`exit_code: 1`,
		`findings: {"candidate":0,"confirmed":0,"confirmed_existing":0,"findings":[],"goal":"docs/specs/demo","record_head":{"seq":4,"sha256":"<line hash>"},"schema":"findings/v1","unresolved":0}`,
		`goal: "docs/specs/demo"`,
		`ha_version: "0.1.8-dev"`,
		`lint: null`,
		`logs: []`,
		`reasons[0]: {"class":"executor","code":"baseline_missing","criterion":"AC-1"}`,
		`reasons[1]: {"class":"executor","code":"baseline_missing","criterion":"AC-2"}`,
		`record_head: {"seq":4,"sha256":"<line hash>"}`,
		`@1 rules: "run-rules/1"`,
		`@2 rules: "run-rules/2"`,
		`@3 rules: "run-rules/3"`,
		`@4 rules: "run-rules/4"`,
		`@5 rules: "run-rules/5"`,
		`@6 rules: "run-rules/6"`,
		`schema: "status/v1"`,
		`skill: {"digest":"","name":"","version":""}`,
		`spec_digest: "cc7b0b169f30b8975d9bd8f7d60fdcadd05989a0bdec7efce7fdee6a82169604"`,
		`status: "incomplete"`,
		`tree_clean: true`,
		`usage: {"baselines":0,"checks":3,"current_inputs":3,"environment_errors":0,"stale_inputs":0,"total":3,"unknowns":0,"valid_evidence":3}`,
	},
	"status md with the baselines missing": {
		`exit 1`,
		`## ha status: docs/specs/demo`,
		"",
		`- status: **incomplete** (exit 1)`,
		`- completion record: none`,
		`- assurance: local (an agent with the same user permissions can change checks and records)`,
		`- check author: executor`,
		`- record head: seq 4 (<line hash>)`,
		`@1 - rules: run-rules/1; skill: not recorded; ha: 0.1.8-dev`,
		`@2 - rules: run-rules/2; skill: not recorded; ha: 0.1.8-dev`,
		`@3 - rules: run-rules/3; skill: not recorded; ha: 0.1.8-dev`,
		`@4 - rules: run-rules/4; skill: not recorded; ha: 0.1.8-dev`,
		`@5 - rules: run-rules/5; skill: not recorded; ha: 0.1.8-dev`,
		`@6 - rules: run-rules/6; skill: not recorded; ha: 0.1.8-dev`,
		`- code: <commit the code commit>, tree clean: yes; goal digest: cc7b0b169f30`,
		`- attempts: 3 total; types: 3 check, 0 baseline; outcomes: 0 environment errors, 0 unknown; inputs: 3 current, 0 stale; valid positive evidence: 3 (overlapping axes)`,
		"",
		`### Unmet`,
		"",
		"- AC-1 `baseline_missing` (executor): no fresh record",
		"- AC-2 `baseline_missing` (executor): no fresh record",
		"",
		`### Criteria`,
		"",
		`| ID | kind | required | satisfied | reason | records | attempts |`,
		`| --- | --- | --- | --- | --- | --- | --- |`,
		`| AC-1 | change | yes | no | baseline_missing | — | checks 1 (fail 0, error 0), baselines 0 (unexpected pass 0, error 0), stale 0 |`,
		`| AC-2 | change | yes | no | baseline_missing | — | checks 1 (fail 0, error 0), baselines 0 (unexpected pass 0, error 0), stale 0 |`,
		`| AC-3 | maintain | yes | yes | — | 4 | checks 1 (fail 0, error 0), baselines 0 (unexpected pass 0, error 0), stale 0 |`,
		"",
		`### Reasons`,
		"",
		"- `baseline_missing` (executor) AC-1",
		"- `baseline_missing` (executor) AC-2",
		"",
		`### Budget`,
		"",
		`- window from seq 1: 3/50 runs, <seconds>/14400 seconds`,
		`- budget revision: 1; remaining: 47 runs; excess: 0 runs; run cap reached: no`,
		`@4,5,6 - elapsed leaves out <seconds> seconds of waits the executor reported (window span <seconds> seconds)`,
		`@6 - elapsed also leaves out <seconds> seconds after completion, excluding reported waits and runs`,
	},
	"report ko with the baselines missing": {
		`exit 0`,
		`바뀐 것: base <commit base> 이후 실행 묶음 문서 밖에서 파일 2개, commit 1개`,
		`확인한 것: 필수 조건 3개 중 1개 충족(검사 기록 1개, 사용자 확인 0개); change 조건 2개 중 0개에 신선한 baseline`,
		`직접 볼 것: 없음`,
		`남은 한계: 보증 수준 local; 검사 작성자: 실행자; 지금 HEAD를 덮는 완료 기록이 없음`,
		"다음 행동: AC-1 `baseline_missing` 해결 (executor)",
		"",
		"### `## 현재` 도장",
		"",
		`- 기록 head: seq 4 (<line hash>), 기록 시각 <time>; 상태: incomplete`,
		"",
		`### 조건별 근거`,
		"",
		`근거 출처: 지금 상태`,
		`| ID | 종류 | 필수 | 충족 | 근거 기록 |`,
		`| --- | --- | --- | --- | --- |`,
		`| AC-1 | change | 예 | 아니오 | — |`,
		`| AC-2 | change | 예 | 아니오 | — |`,
		`| AC-3 | maintain | 예 | 예 | 4 |`,
		"",
		`### 완료 범위`,
		"",
		`- 완료 기록: none`,
		"",
		`### 미충족 조건의 원문 위치`,
		"",
		"- AC-1 `baseline_missing` (executor): 신선한 기록 없음",
		"- AC-2 `baseline_missing` (executor): 신선한 기록 없음",
	},
	"report en with the baselines missing": {
		`exit 0`,
		`Changed: 2 files and 1 commits outside the bundle documents since base <commit base>`,
		`Checked: 1 of 3 required conditions satisfied (1 by check records, 0 by user confirmation); 0 of 2 change conditions have a fresh baseline`,
		`To review: none`,
		`Remaining limits: assurance local; check author: executor; HEAD is not covered by a completion record`,
		"Next step: resolve AC-1 `baseline_missing` (executor)",
		"",
		"### Stamp for `## 현재`",
		"",
		`- record head: seq 4 (<line hash>), recorded at <time>; status: incomplete`,
		"",
		`### Evidence per condition`,
		"",
		`Source: current state`,
		`| ID | kind | required | satisfied | records |`,
		`| --- | --- | --- | --- | --- |`,
		`| AC-1 | change | yes | no | — |`,
		`| AC-2 | change | yes | no | — |`,
		`| AC-3 | maintain | yes | yes | 4 |`,
		"",
		`### Completion coverage`,
		"",
		`- completion record: none`,
		"",
		`### Output of unmet conditions`,
		"",
		"- AC-1 `baseline_missing` (executor): no fresh record",
		"- AC-2 `baseline_missing` (executor): no fresh record",
	},
	"estimate json with AC-2's baseline missing": {
		`budget_source: "PLAN.md"`,
		`default_additional_runs: 0`,
		`default_baseline_runs: 2`,
		`default_check_runs: 3`,
		`goal: "docs/specs/demo"`,
		`headroom: 45`,
		`limitation: "Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged."`,
		`minimum_runs: 5`,
		`plan_runs_limit: 50`,
		`required_change: 2`,
		`required_maintain: 1`,
		`schema: "estimate/v1"`,
		`selected: {"additional_runs":0,"mode":"check","optional_runs":0,"repeated_required_runs":0,"runs":3,"targets":["AC-1","AC-2","AC-3"],"task_runs":0}`,
		`shortfall: 0`,
	},
	"estimate baseline json with AC-2's baseline missing": {
		`budget_source: "PLAN.md"`,
		`default_additional_runs: 0`,
		`default_baseline_runs: 2`,
		`default_check_runs: 3`,
		`goal: "docs/specs/demo"`,
		`headroom: 45`,
		`limitation: "Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged."`,
		`minimum_runs: 5`,
		`plan_runs_limit: 50`,
		`required_change: 2`,
		`required_maintain: 1`,
		`schema: "estimate/v1"`,
		`selected: {"additional_runs":0,"mode":"baseline","optional_runs":0,"repeated_required_runs":0,"runs":2,"targets":["AC-1","AC-2"],"task_runs":0}`,
		`shortfall: 0`,
	},
	"estimate text with AC-2's baseline missing": {
		`exit 0`,
		`## ha estimate: docs/specs/demo`,
		"",
		`- required minimum: 5 runs (2 change × 2 + 1 maintain; manual: 0)`,
		`- PLAN run limit: 50; shortfall: 0; headroom above minimum: 45`,
		`- default commands: check 3 + baseline 2 runs; additional to minimum: 0`,
		`- selected check: 3 runs; additional: 0 (optional 0, task 0, repeated required 0)`,
		`  - AC-1`,
		`  - AC-2`,
		`  - AC-3`,
		"",
		`Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged.`,
	},
	"estimate baseline text with AC-2's baseline missing": {
		`exit 0`,
		`## ha estimate: docs/specs/demo`,
		"",
		`- required minimum: 5 runs (2 change × 2 + 1 maintain; manual: 0)`,
		`- PLAN run limit: 50; shortfall: 0; headroom above minimum: 45`,
		`- default commands: check 3 + baseline 2 runs; additional to minimum: 0`,
		`- selected baseline: 2 runs; additional: 0 (optional 0, task 0, repeated required 0)`,
		`  - AC-1`,
		`  - AC-2`,
		"",
		`Fresh required lower bound only; excludes retries, preparation and manual time. Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's remaining budget; existing records and budget windows are unchanged.`,
	},
	"status json after ha done": {
		`assurance: "local"`,
		`@1,2,3 budget: {"elapsed_limit_seconds":14400,"elapsed_seconds":"<seconds>","exceeded":false,"excess_runs":0,"history":[],"reached":false,"remaining_runs":44,"revision":1,"runs":6,"runs_limit":50,"window_start":1}`,
		`@4,5 budget: {"elapsed_limit_seconds":14400,"elapsed_seconds":"<seconds>","exceeded":false,"excess_runs":0,"history":[],"reached":false,"remaining_runs":44,"reported_wait_seconds":"<seconds>","revision":1,"runs":6,"runs_limit":50,"span_seconds":"<seconds>","wait_basis":"executor_reported","window_start":1}`,
		`@6 budget: {"elapsed_limit_seconds":14400,"elapsed_seconds":"<seconds>","exceeded":false,"excess_runs":0,"history":[],"post_completion_seconds":"<seconds>","reached":false,"remaining_runs":44,"reported_wait_seconds":"<seconds>","revision":1,"runs":6,"runs_limit":50,"span_seconds":"<seconds>","wait_basis":"executor_reported","window_start":1}`,
		`changes: {"command_changed":[],"ha_version_changed":[],"invalid_confirmations":[],"kind_changed":[],"optional":[]}`,
		`check_author: "executor"`,
		`commit: "<commit the code commit>"`,
		`complete_without_record: false`,
		`completion_record: 9`,
		`criteria[0]: {"attempts":{"baseline_errors":0,"baseline_fails":0,"baseline_unknowns":0,"baselines":2,"checks":1,"errors":0,"fail_commits":[],"fails":0,"stale":0,"unexpected_passes":0,"unknowns":0},"id":"AC-1","kind":"change","reason":"","records":[2,5,7],"required":true,"satisfied":true}`,
		`criteria[1]: {"attempts":{"baseline_errors":0,"baseline_fails":0,"baseline_unknowns":0,"baselines":1,"checks":1,"errors":0,"fail_commits":[],"fails":0,"stale":0,"unexpected_passes":0,"unknowns":0},"id":"AC-2","kind":"change","reason":"","records":[3,8],"required":true,"satisfied":true}`,
		`criteria[2]: {"attempts":{"baseline_errors":0,"baseline_fails":0,"baseline_unknowns":0,"baselines":0,"checks":1,"errors":0,"fail_commits":[],"fails":0,"stale":0,"unexpected_passes":0,"unknowns":0},"id":"AC-3","kind":"maintain","reason":"","records":[4],"required":true,"satisfied":true}`,
		`exit_code: 0`,
		`findings: {"candidate":0,"confirmed":0,"confirmed_existing":0,"findings":[],"goal":"docs/specs/demo","record_head":{"seq":9,"sha256":"<line hash>"},"schema":"findings/v1","unresolved":0}`,
		`goal: "docs/specs/demo"`,
		`ha_version: "0.1.8-dev"`,
		`lint: null`,
		`logs: []`,
		`reasons: []`,
		`record_head: {"seq":9,"sha256":"<line hash>"}`,
		`@1 rules: "run-rules/1"`,
		`@2 rules: "run-rules/2"`,
		`@3 rules: "run-rules/3"`,
		`@4 rules: "run-rules/4"`,
		`@5 rules: "run-rules/5"`,
		`@6 rules: "run-rules/6"`,
		`schema: "status/v1"`,
		`skill: {"digest":"","name":"","version":""}`,
		`spec_digest: "cc7b0b169f30b8975d9bd8f7d60fdcadd05989a0bdec7efce7fdee6a82169604"`,
		`status: "complete"`,
		`tree_clean: true`,
		`usage: {"baselines":3,"checks":3,"current_inputs":6,"environment_errors":0,"stale_inputs":0,"total":6,"unknowns":0,"valid_evidence":6}`,
	},
	"status md after ha done": {
		`exit 0`,
		`## ha status: docs/specs/demo`,
		"",
		`- status: **complete** (exit 0)`,
		`- completion record: seq 9`,
		`- assurance: local (an agent with the same user permissions can change checks and records)`,
		`- check author: executor`,
		`- record head: seq 9 (<line hash>)`,
		`@1 - rules: run-rules/1; skill: not recorded; ha: 0.1.8-dev`,
		`@2 - rules: run-rules/2; skill: not recorded; ha: 0.1.8-dev`,
		`@3 - rules: run-rules/3; skill: not recorded; ha: 0.1.8-dev`,
		`@4 - rules: run-rules/4; skill: not recorded; ha: 0.1.8-dev`,
		`@5 - rules: run-rules/5; skill: not recorded; ha: 0.1.8-dev`,
		`@6 - rules: run-rules/6; skill: not recorded; ha: 0.1.8-dev`,
		`- code: <commit the code commit>, tree clean: yes; goal digest: cc7b0b169f30`,
		`- attempts: 6 total; types: 3 check, 3 baseline; outcomes: 0 environment errors, 0 unknown; inputs: 6 current, 0 stale; valid positive evidence: 6 (overlapping axes)`,
		"",
		`### Criteria`,
		"",
		`| ID | kind | required | satisfied | reason | records | attempts |`,
		`| --- | --- | --- | --- | --- | --- | --- |`,
		`| AC-1 | change | yes | yes | — | 2, 5, 7 | checks 1 (fail 0, error 0), baselines 2 (unexpected pass 0, error 0), stale 0 |`,
		`| AC-2 | change | yes | yes | — | 3, 8 | checks 1 (fail 0, error 0), baselines 1 (unexpected pass 0, error 0), stale 0 |`,
		`| AC-3 | maintain | yes | yes | — | 4 | checks 1 (fail 0, error 0), baselines 0 (unexpected pass 0, error 0), stale 0 |`,
		"",
		`### Reasons`,
		"",
		`None.`,
		"",
		`### Budget`,
		"",
		`- window from seq 1: 6/50 runs, <seconds>/14400 seconds`,
		`- budget revision: 1; remaining: 44 runs; excess: 0 runs; run cap reached: no`,
		`@4,5,6 - elapsed leaves out <seconds> seconds of waits the executor reported (window span <seconds> seconds)`,
		`@6 - elapsed also leaves out <seconds> seconds after completion, excluding reported waits and runs`,
	},
	"report ko after ha done": {
		`exit 0`,
		`바뀐 것: base <commit base> 이후 실행 묶음 문서 밖에서 파일 2개, commit 1개`,
		`확인한 것: 필수 조건 3개 중 3개 충족(검사 기록 3개, 사용자 확인 0개); change 조건 2개 중 2개에 신선한 baseline`,
		`직접 볼 것: 없음`,
		`남은 한계: 보증 수준 local; 검사 작성자: 실행자`,
		`다음 행동: 없음; 완료 기록 seq 9이 유효하고 남은 할 일이 없음`,
		"",
		"### `## 현재` 도장",
		"",
		`- 기록 head: seq 9 (<line hash>), 기록 시각 <time>; 상태: complete; 완료 기록: seq 9`,
		"",
		`### 조건별 근거`,
		"",
		`근거 출처: 완료 기록 seq 9`,
		`| ID | 종류 | 필수 | 충족 | 근거 기록 |`,
		`| --- | --- | --- | --- | --- |`,
		`| AC-1 | change | 예 | 예 | 2, 5, 7 |`,
		`| AC-2 | change | 예 | 예 | 3, 8 |`,
		`| AC-3 | maintain | 예 | 예 | 4 |`,
		"",
		`### 완료 범위`,
		"",
		`- 완료 기록 seq 9, commit <commit the code commit>; 지금 HEAD <commit the code commit>를 덮음`,
		"",
		`### 미충족 조건의 원문 위치`,
		"",
		`- 없음`,
	},
	"report en after ha done": {
		`exit 0`,
		`Changed: 2 files and 1 commits outside the bundle documents since base <commit base>`,
		`Checked: 3 of 3 required conditions satisfied (3 by check records, 0 by user confirmation); 2 of 2 change conditions have a fresh baseline`,
		`To review: none`,
		`Remaining limits: assurance local; check author: executor`,
		`Next step: none; completion record seq 9 holds and nothing is left to do`,
		"",
		"### Stamp for `## 현재`",
		"",
		`- record head: seq 9 (<line hash>), recorded at <time>; status: complete; completion record: seq 9`,
		"",
		`### Evidence per condition`,
		"",
		`Source: completion record seq 9`,
		`| ID | kind | required | satisfied | records |`,
		`| --- | --- | --- | --- | --- |`,
		`| AC-1 | change | yes | yes | 2, 5, 7 |`,
		`| AC-2 | change | yes | yes | 3, 8 |`,
		`| AC-3 | maintain | yes | yes | 4 |`,
		"",
		`### Completion coverage`,
		"",
		`- completion record seq 9, commit <commit the code commit>; HEAD <commit the code commit> is covered`,
		"",
		`### Output of unmet conditions`,
		"",
		`- none`,
	},
}
