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
// AC-13: a goal started under run-rules/6 is judged, estimated, shown and
// completed the same whether its PLAN has the mark column or not, and the
// default start rule stays run-rules/3. AC-14: outside the judgments the goal
// changes, a run-rules/7 goal whose every change condition is marked is
// judged and commanded like the same record under run-rules/6.
//
// The checks use only the ha command line (runMain) and the helpers of
// review_gate_test.go and core_integrity_test.go; they never run Spar, a
// reviewer or a model, and never read a commit, ref or record outside the
// repositories they build. A difference between the twins, a lint problem
// the mark brings to a run-rules/6 goal, and a refused run-rules/7 start are
// the observed violation (jaekit:violation=...). Every other failure,
// including a reference twin that does not reach the state a step names, is
// a broken setup (setup:).

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
// setup too.
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
	if got := obs[0].code; (want == cwPending && got == exitOK) || (want != cwPending && got != want) {
		s.t.Fatalf("setup: %s: exit %d in %s, want %d: %s", label, got, s.twins[0].name, want, cwFirst(obs[0].out))
	}
	return obs
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
// reason of that condition (AC-n:).
func (s *cwTwins) has(label string, obs []cwObs, code string, want bool) {
	s.t.Helper()
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

// start starts every twin under its rule. A refused run-rules/7 start is the
// observed violation, and so is a start that records another rule.
func (s *cwTwins) start(label string, extra ...string) {
	s.t.Helper()
	obs := s.run(label, exitOK, func(tw *cwTwin) (int, string) {
		code, out := tw.f.ha(append([]string{"start", rgGoal, "--request", cwRequest, "--rules", tw.rules}, extra...)...)
		if code != exitOK && tw.rules == "run-rules/7" {
			s.t.Fatalf("jaekit:violation=%s ha refuses to start %s under %s: exit %d: %s", s.violation, tw.name, tw.rules, code, cwFirst(out))
		}
		if code != exitOK && tw != s.twins[0] {
			s.t.Fatalf("jaekit:violation=%s ha refuses to start %s under %s: exit %d: %s", s.violation, tw.name, tw.rules, code, cwFirst(out))
		}
		if code != exitOK {
			s.t.Fatalf("setup: ha start of %s under %s exit %d: %s", tw.name, tw.rules, code, cwFirst(out))
		}
		tw.f.base = tw.f.git("rev-parse", "HEAD")
		return code, out
	})
	for i, o := range obs {
		if got := o.state.value["rules"]; got != s.twins[i].rules {
			if i == 0 {
				s.t.Fatalf("setup: %s: %s computes under %v", label, s.twins[i].name, got)
			}
			s.t.Fatalf("jaekit:violation=%s %s: ha start --rules %s started %s, which computes under %v", s.violation, label, s.twins[i].rules, s.twins[i].name, got)
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

// noMarkFields reports a run-rules/6 status that carries a field only a
// run-rules/7 goal shows (criteria[].baseline_required, check_freeze).
func (s *cwTwins) noMarkFields(label string, obs []cwObs) {
	s.t.Helper()
	for i, o := range obs {
		if o.state.value == nil {
			continue
		}
		if _, ok := o.state.value["check_freeze"]; ok {
			s.t.Fatalf("jaekit:violation=%s %s: the run-rules/6 status of %s has check_freeze", s.violation, label, s.twins[i].name)
		}
		list, _ := o.state.value["criteria"].([]any)
		for _, item := range list {
			c, _ := item.(map[string]any)
			if _, ok := c["baseline_required"]; ok {
				s.t.Fatalf("jaekit:violation=%s %s: the run-rules/6 status of %s has criteria[].baseline_required (%v)", s.violation, label, s.twins[i].name, c["id"])
			}
		}
	}
}

// same runs a step, compares it, its output and the fields a run-rules/6
// status must not have.
func (s *cwTwins) same(label string, want int, act cwAct) []cwObs {
	s.t.Helper()
	obs := s.step(label, want, act)
	s.output(label, obs)
	s.noMarkFields(label, obs)
	return obs
}

// cwEstimates are the estimate views AC-13 compares: the minimum with the
// default check selection, and the default baseline selection.
var cwEstimates = [][]string{{"estimate", rgGoal, "--format", "json"}, {"estimate", rgGoal, "--baseline", "--format", "json"},
	{"estimate", rgGoal}, {"estimate", rgGoal, "--baseline"}}

// AC-13: a run-rules/6 goal whose PLAN has the baseline column, from its
// start or added after its completion, gives the same lint result, estimate
// (minimum and default baseline selection), judgment, reasons, completion,
// `ha status` JSON and human output and `ha report` as the same goal without
// the column; no command rewrites its record; and a start without --rules
// still uses run-rules/3.
func TestCheckCostAC13_EarlierRulesIgnoreMark(t *testing.T) {
	const v = "earlier-rules-change-with-mark"
	plain := &cwTwin{name: "the run-rules/6 goal without the baseline column", rules: "run-rules/6"}
	marked := &cwTwin{name: "the run-rules/6 goal with the baseline column", rules: "run-rules/6",
		plan: cwPlan{marks: []string{cwReq, "", "—"}}}
	later := &cwTwin{name: "the run-rules/6 goal that gains the baseline column after its completion", rules: "run-rules/6"}
	s := newCWTwins(t, v, "", plain, marked, later)
	s.human = true

	estimates := func(when string) {
		t.Helper()
		for _, args := range cwEstimates {
			s.same(when+": ha "+strings.Join(args, " "), exitOK, cwHa(args...))
		}
	}
	s.same("ha lint before the start", exitOK, cwHa("lint", rgGoal))
	estimates("before the start")
	s.start("the start")
	s.same("ha lint after the start", exitOK, cwHa("lint", rgGoal))
	estimates("after the start")

	s.commit("the code commit", "src/one.txt", "ok\n", "src/two.txt", "ok\n")
	obs := s.same("every required condition checked", exitOK, cwHa("check", rgGoal))
	s.has("every required condition checked", obs, "AC-1:baseline_missing", true)
	s.has("every required condition checked", obs, "AC-2:baseline_missing", true)
	obs = s.same("AC-1's baseline", exitOK, cwHa("check", rgGoal, "--baseline", "AC-1"))
	s.has("AC-1's baseline", obs, "AC-1:", false)
	s.has("AC-1's baseline", obs, "AC-2:baseline_missing", true)
	s.same("ha done with AC-2's baseline missing", cwPending, cwHa("done", rgGoal))
	estimates("with AC-2's baseline missing")

	label := "the default baseline selection"
	obs = s.same(label, exitOK, cwHa("check", rgGoal, "--baseline"))
	if want := "appended baseline/AC-1/fail_as_expected, baseline/AC-2/fail_as_expected"; obs[0].record != want {
		t.Fatalf("setup: %s: the run record %s in %s, want %s", label, obs[0].record, plain.name, want)
	}
	obs = s.same("ha done", exitOK, cwHa("done", rgGoal))
	if obs[0].state.completion == nil {
		t.Fatalf("setup: ha done: no completion record in %s", plain.name)
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
		t.Fatalf("setup: %s: status %s in %s", label, obs[0].state.status, plain.name)
	}
	s.same("ha lint after the completion", exitOK, cwHa("lint", rgGoal))
	estimates("after the completion")
	s.same("ha done again", exitOK, cwHa("done", rgGoal))

	// The default start rule.
	caps := plain.f
	code, out := caps.ha("capabilities", "--format", "json")
	var info struct {
		Default string `json:"default_rules"`
	}
	if err := json.Unmarshal([]byte(out), &info); code != exitOK || err != nil {
		t.Fatalf("setup: ha capabilities exit %d: %v: %s", code, err, cwFirst(out))
	}
	if info.Default != "run-rules/3" {
		t.Fatalf("jaekit:violation=%s ha capabilities names the default start rule %q, want run-rules/3", v, info.Default)
	}
	fresh := cwRepo(t, "", cwPlan{})
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
