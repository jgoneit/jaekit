package main

// Checks of the goal check-cost: a new rule version run-rules/7 keeps every
// run-rules/6 judgment and changes which change conditions need a baseline
// (the optional last `baseline` column of PLAN `## 조건표`, where `요구` on a
// change row asks for one), the default baseline selection, the minimum
// runs, the status and report display of that requirement, and a display of
// the check files changed after the first review round that received them
// (the check freeze).
//
// The tests drive the ha command line (runMain) on synthetic repositories
// built with the helpers of review_gate_test.go. Every change condition there
// has its own check-result/v1 producer (a POSIX sh script reading one value
// file) and declaration, so a baseline overlays only that condition's check
// files; maintain conditions run a plain sh script. Review gates are written
// in the public review-gate/v1 form. The tests never run Spar, a reviewer or
// a model, and read no commit, ref or goal bundle of this repository.
//
// A Core without run-rules/7 refuses `ha start --rules run-rules/7`. That
// refusal, and any other refusal or wrong answer of ha for such a goal, is
// the observed violation: the first line of the failure message starts with
// jaekit:violation=<id>. A broken setup (git or sh missing, temporary files,
// JSON decoding, an internal ha error) fails with `setup:` and never carries
// the marker. No test uses t.Run.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const (
	// ccRules is the new rule version.
	ccRules = "run-rules/7"
	// ccMark is the baseline cell of a change condition that requires one.
	ccMark    = "요구"
	ccRequest = "implement the synthetic goal"
)

// ccCond is one condition of a synthetic check-cost goal.
type ccCond struct {
	id   string // AC-n
	kind string // "change" or "maintain"
	// mark is the baseline cell: ccMark, "—" or "" (an empty cell).
	mark string
	// atBase makes a change condition's value already right at the base
	// commit, so its baseline passes unexpectedly.
	atBase bool
	// paths are the 검사 경로 when they are not the condition's producer and
	// declaration; files other than those two get synthetic content.
	paths []string
	// violation is the identifier a change condition's producer reports
	// while the value is wrong; "" is the declared ccViolation.
	violation string
	// runner makes a change condition's command its ccRunner program instead
	// of `sh <producer>`. The test adds that program after the start, so it
	// is not a check file and a baseline cannot start it in the base tree.
	runner bool
}

// ccViolation is the violation every change condition's declaration names.
const ccViolation = "value-wrong"

func ccChange(id, mark string) ccCond { return ccCond{id: id, kind: "change", mark: mark} }

func ccMaintain(id string) ccCond { return ccCond{id: id, kind: "maintain"} }

// ccGoal is a synthetic goal: its tier ("" for no Assurance line), its
// conditions in table order, whether the 조건표 has the baseline column, and
// the `검증 실행` value (a number or 자동; "" means 50).
type ccGoal struct {
	tier   string
	conds  []ccCond
	column bool
	runs   string
}

func ccStem(id string) string { return strings.ToLower(id) }

// ccValue is the file a change condition's producer reads; it says ok once
// the goal is implemented.
func ccValue(id string) string { return "src/" + ccStem(id) + ".txt" }

// ccScript is a change condition's producer and command.
func ccScript(id string) string { return "checks/" + ccStem(id) + ".sh" }

// ccDecl is a change condition's result declaration.
func ccDecl(id string) string { return "checks/" + ccStem(id) + ".json" }

// ccMaintainScript is a maintain condition's command.
func ccMaintainScript(id string) string { return "tests/" + ccStem(id) + ".sh" }

// ccRunner is the program that runs a change condition's producer when its
// command is not `sh <producer>`: an executable file outside its 검사 경로.
func ccRunner(id string) string { return "src/" + ccStem(id) + "-run" }

// ccAddRunner writes the condition's ccRunner program, executable, into the
// working tree; the caller commits it.
func (f *ccFixture) ccAddRunner(id string) {
	f.t.Helper()
	f.put(ccRunner(id), "#!/bin/sh\nexec sh "+ccScript(id)+"\n")
	if err := os.Chmod(filepath.Join(f.dir, filepath.FromSlash(ccRunner(id))), 0o755); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
}

func (c ccCond) checkPaths() []string {
	if c.paths != nil {
		return c.paths
	}
	return []string{ccScript(c.id), ccDecl(c.id)}
}

// command is the 검증 명령 of a condition.
func (c ccCond) command() string {
	switch {
	case c.kind != "change":
		return "sh " + ccMaintainScript(c.id)
	case c.runner:
		return ccRunner(c.id)
	}
	return "sh " + ccScript(c.id)
}

// ccProducer is a check-result/v1 producer for one change condition: its one
// target passes when the condition's value file says ok and reports the
// condition's violation (the declared one unless it names another)
// otherwise.
func ccProducer(c ccCond) string {
	v, violation := ccValue(c.id), c.violation
	if violation == "" {
		violation = ccViolation
	}
	return "# Synthetic check-result/v1 producer of " + c.id + ": " + v + " says ok.\n" +
		"if [ -f " + v + " ] && [ \"$(cat " + v + ")\" = ok ]; then\n" +
		"  obs='{\"target\":\"value\",\"attempt\":1,\"status\":\"pass\"}'; code=0\n" +
		"else\n" +
		"  obs='{\"target\":\"value\",\"attempt\":1,\"status\":\"violation\",\"violation\":\"" + violation + "\"}'; code=1\n" +
		"fi\n" +
		"printf '{\"schema\":\"check-result/v1\",\"invocation\":\"%s\",\"declaration_digest\":\"%s\",\"producer\":\"cc-synthetic\"," +
		"\"producer_version\":\"1\",\"attempts_complete\":true,\"observations\":[%s]}\\n' " +
		"\"$HA_EVIDENCE_INVOCATION\" \"$HA_EVIDENCE_DECLARATION_DIGEST\" \"$obs\" > \"$HA_EVIDENCE_PATH\"\n" +
		"exit $code\n"
}

func ccDeclaration(id string) string {
	return `{"schema":"check-declaration/v1","producer":"cc-synthetic","producer_version":"1","producer_paths":["` + ccScript(id) +
		`"],"targets":[{"id":"value","violation":"` + ccViolation + `"}]}` + "\n"
}

// ccSpec is the goal's SPEC.md. It links note.md, so its goal documents are
// the two files rgFixture.docs names.
func ccSpec(g ccGoal) string {
	var b strings.Builder
	b.WriteString("# Demo values\n\nStatus: Ready\n")
	if g.tier != "" {
		b.WriteString("Assurance: " + g.tier + "\n")
	}
	b.WriteString("\n## 목표\n\nThe values say ok, as [the value note](note.md) describes.\n\n" +
		"## 범위와 비목표\n\n- 범위: `src/**`, `checks/**`\n\n## Acceptance Criteria\n")
	for _, c := range g.conds {
		if c.kind == "change" {
			fmt.Fprintf(&b, "- **%s** `%s` says ok.\n", c.id, ccValue(c.id))
		} else {
			fmt.Fprintf(&b, "- **%s** `src/value.txt` keeps saying ok.\n", c.id)
		}
	}
	b.WriteString("\n## Open Decisions\n\n없음\n")
	return b.String()
}

// ccPlan is the goal's PLAN.md in the Seal bundle format, with the baseline
// column last when the goal has it.
func ccPlan(g ccGoal) string {
	var b strings.Builder
	b.WriteString("# PLAN — demo\n\n## 조건표\n")
	if g.column {
		b.WriteString("| ID | 종류 | 검증 명령 | 검사 경로 | Task | baseline |\n| --- | --- | --- | --- | --- | --- |\n")
	} else {
		b.WriteString("| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n")
	}
	var ids, changes []string
	for _, c := range g.conds {
		ids = append(ids, c.id)
		paths := "—"
		if c.kind == "change" {
			paths = strings.Join(c.checkPaths(), ", ")
			changes = append(changes, c.id)
		}
		fmt.Fprintf(&b, "| %s | %s | `%s` | %s | T001 |", c.id, c.kind, c.command(), paths)
		if g.column {
			fmt.Fprintf(&b, " %s |", c.mark)
		}
		b.WriteString("\n")
	}
	if len(changes) > 0 {
		b.WriteString("\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n")
		for _, id := range changes {
			fmt.Fprintf(&b, "| %s | %s |\n", id, ccDecl(id))
		}
	}
	runs := g.runs
	if runs == "" {
		runs = "50"
	}
	b.WriteString("\n## 범위\n- 바꿀 수 있는 경로: `src/**`, `tests/**`, `checks/**` (SPEC: 범위와 비목표)\n" +
		"- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n")
	fmt.Fprintf(&b, "## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n"+
		"| T001 | the values say ok | %s | — | — | 예 | — |\n\n", strings.Join(ids, ", "))
	fmt.Fprintf(&b, "## 예산\n- 검증 실행: %s\n- 경과 시간: 4시간\n- 비용: 미관측\n", runs)
	return b.String()
}

// ccFixture is a synthetic check-cost goal in its repository.
type ccFixture struct {
	*rgFixture
	g ccGoal
}

// ccRepo makes a committed repository holding the goal g and makes it the
// working directory. Every change condition's value says old at base (ok
// with atBase). The pre-approved run limit is the default: the user
// configuration folder is an empty temporary one.
func ccRepo(t *testing.T, g ccGoal) *ccFixture {
	t.Helper()
	rgRequireGit(t)
	if _, err := exec.LookPath("sh"); err != nil {
		t.Fatalf("setup: sh is not available: %v", err)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	g.conds = append([]ccCond(nil), g.conds...)
	f := &ccFixture{g: g}
	f.rgFixture = rgRepo(t, g.tier, func(r *rgFixture) {
		r.put(rgGoal+"/SPEC.md", ccSpec(g))
		r.put(rgGoal+"/PLAN.md", ccPlan(g))
		for _, c := range g.conds {
			if c.kind != "change" {
				r.put(ccMaintainScript(c.id), "grep -qx ok src/value.txt\n")
				continue
			}
			value := "old\n"
			if c.atBase {
				value = "ok\n"
			}
			r.put(ccValue(c.id), value)
			r.put(ccScript(c.id), ccProducer(c))
			r.put(ccDecl(c.id), ccDeclaration(c.id))
			for _, p := range c.checkPaths() {
				if p != ccScript(c.id) && p != ccDecl(c.id) {
					r.put(p, "synthetic check input of "+c.id+"\n")
				}
			}
		}
	})
	return f
}

func (f *ccFixture) cond(id string) *ccCond {
	f.t.Helper()
	for i := range f.g.conds {
		if f.g.conds[i].id == id {
			return &f.g.conds[i]
		}
	}
	f.t.Fatalf("setup: %s is not a condition of the fixture", id)
	return nil
}

// writePlan writes PLAN.md from the fixture's goal; it stays uncommitted,
// as a bundle document.
func (f *ccFixture) writePlan() { f.put(rgGoal+"/PLAN.md", ccPlan(f.g)) }

// mark sets a condition's baseline cell in PLAN.md.
func (f *ccFixture) mark(id, mark string) {
	f.t.Helper()
	f.cond(id).mark = mark
	f.writePlan()
}

// setPaths sets a change condition's 검사 경로 in PLAN.md.
func (f *ccFixture) setPaths(id string, paths ...string) {
	f.t.Helper()
	f.cond(id).paths = append([]string{}, paths...)
	f.writePlan()
}

// revise changes a check file's content (a comment line) without changing
// what it checks.
func (f *ccFixture) revise(path, note string) {
	f.t.Helper()
	f.put(path, string(f.read(path))+"# "+note+"\n")
}

// implement makes every change condition's value say ok and commits it.
func (f *ccFixture) implement() string {
	f.t.Helper()
	for _, c := range f.g.conds {
		if c.kind == "change" {
			f.put(ccValue(c.id), "ok\n")
		}
	}
	f.head = f.commit("implement the values")
	f.work = f.head
	return f.head
}

// tryStart runs `ha start --rules run-rules/7` for the goal and returns its
// exit code and output; after a start the base is HEAD. An internal error is
// a broken setup.
func (f *ccFixture) tryStart(extra ...string) (int, string) {
	f.t.Helper()
	code, out := f.ha(append([]string{"start", rgGoal, "--request", ccRequest, "--rules", ccRules}, extra...)...)
	if code == exitInternal {
		f.t.Fatalf("setup: ha start exit %d: %s", code, out)
	}
	if code == exitOK {
		f.base = f.git("rev-parse", "HEAD")
		f.head = f.base
	}
	return code, out
}

// start starts the goal under run-rules/7; a refusal is the observed
// violation.
func (f *ccFixture) start(violation string, extra ...string) string {
	f.t.Helper()
	code, out := f.tryStart(extra...)
	if code != exitOK {
		f.t.Fatalf("jaekit:violation=%s ha start --rules %s refused the synthetic goal: exit %d: %s", violation, ccRules, code, strings.TrimSpace(out))
	}
	return out
}

// must runs ha and returns its output. Another exit code than want is the
// observed violation; an internal error is a broken setup.
func (f *ccFixture) must(violation, label string, want int, args ...string) string {
	f.t.Helper()
	code, out := f.ha(args...)
	if code == exitInternal && want != exitInternal {
		f.t.Fatalf("setup: %s: ha %v exit %d: %s", label, args, code, out)
	}
	if code != want {
		f.t.Fatalf("jaekit:violation=%s %s: ha %v exit %d, want %d: %s", violation, label, args, code, want, strings.TrimSpace(out))
	}
	return out
}

// state is `ha status --format json`. No status JSON for the goal is the
// observed violation; an internal error or undecodable JSON is a broken
// setup.
func (f *ccFixture) state(violation, label string) rgState {
	f.t.Helper()
	var out, errb bytes.Buffer
	code := runMain([]string{"status", rgGoal, "--format", "json"}, &out, &errb)
	if code == exitInternal {
		f.t.Fatalf("setup: %s: ha status exit %d: %s", label, code, errb.String())
	}
	if out.Len() == 0 {
		f.t.Fatalf("jaekit:violation=%s %s: ha status gave no status JSON: exit %d: %s", violation, label, code, strings.TrimSpace(errb.String()))
	}
	var value map[string]any
	if err := json.Unmarshal(out.Bytes(), &value); err != nil {
		f.t.Fatalf("setup: %s: ha status JSON: %v", label, err)
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

// output runs ha and returns its standard output. No output is the observed
// violation; an internal error is a broken setup.
func (f *ccFixture) output(violation, label string, args ...string) string {
	f.t.Helper()
	var out, errb bytes.Buffer
	code := runMain(args, &out, &errb)
	if code == exitInternal {
		f.t.Fatalf("setup: %s: ha %v exit %d: %s", label, args, code, errb.String())
	}
	if out.Len() == 0 {
		f.t.Fatalf("jaekit:violation=%s %s: ha %v printed nothing: exit %d: %s", violation, label, args, code, strings.TrimSpace(errb.String()))
	}
	return out.String()
}

// ccReportLabels are the summary labels of `ha report` by language: the
// checked line and the to-review line.
var ccReportLabels = map[string][2]string{"ko": {"확인한 것", "직접 볼 것"}, "en": {"Checked", "To review"}}

// summary is the text of a `ha report --lang <lang>` summary line: which 0
// is the checked line, 1 the to-review line. A report without the line is
// the observed violation.
func (f *ccFixture) summary(violation, label, lang string, which int) string {
	f.t.Helper()
	text := f.output(violation, label, "report", rgGoal, "--lang", lang)
	name := ccReportLabels[lang][which] + ": "
	if line, ok := ccLine(text, name); ok {
		return strings.TrimPrefix(line, name)
	}
	f.t.Fatalf("jaekit:violation=%s %s: ha report --lang %s has no %q line:\n%s", violation, label, lang, name, text)
	return ""
}

// ccLine is the first line of text that starts with prefix.
func ccLine(text, prefix string) (string, bool) {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			return line, true
		}
	}
	return "", false
}

// ccItem is the item of a report summary line that starts with prefix,
// without the prefix: the text after it up to the next "; " item separator.
func ccItem(line, prefix string) (string, bool) {
	i := strings.Index(line, prefix)
	if i < 0 {
		return "", false
	}
	rest := line[i+len(prefix):]
	if j := strings.Index(rest, "; "); j >= 0 {
		rest = rest[:j]
	}
	return rest, true
}

// ccList splits a comma-separated list and sorts it.
func ccList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if p := strings.TrimSpace(part); p != "" {
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// ccCriterion is the status JSON criterion with the ID, or nil.
func ccCriterion(s rgState, id string) map[string]any {
	list, _ := s.value["criteria"].([]any)
	for _, item := range list {
		if m, _ := item.(map[string]any); m != nil && m["id"] == id {
			return m
		}
	}
	return nil
}

// ccHasReason reports whether the status names the reason code for the
// criterion (id "" for a goal-level reason).
func ccHasReason(s rgState, id, code string) bool {
	want := code
	if id != "" {
		want = id + ":" + code
	}
	for _, r := range s.reasons {
		if r == want {
			return true
		}
	}
	return false
}

// ccTarget is the condition a run record line names.
func ccTarget(l ciLine) string {
	t, _ := l.m["target"].(map[string]any)
	id, _ := t["criterion"].(string)
	return id
}

// ccRuns are the lines of the kind ("check" or "baseline") among lines.
func ccRuns(lines []ciLine, kind string) []ciLine {
	var out []ciLine
	for _, l := range lines {
		if l.kind() == kind {
			out = append(out, l)
		}
	}
	return out
}

// ccTargets are the conditions the lines name, in record order.
func ccTargets(lines []ciLine) []string {
	out := []string{}
	for _, l := range lines {
		out = append(out, ccTarget(l))
	}
	return out
}

// ccEstimate runs `ha estimate ... --format json`. Another exit code than 0
// is the observed violation; an internal error or undecodable JSON is a
// broken setup.
func (f *ccFixture) estimate(violation, label string, args ...string) map[string]any {
	f.t.Helper()
	full := append(append([]string{"estimate", rgGoal}, args...), "--format", "json")
	code, out := f.ha(full...)
	if code == exitInternal {
		f.t.Fatalf("setup: %s: ha %v exit %d: %s", label, full, code, out)
	}
	if code != exitOK {
		f.t.Fatalf("jaekit:violation=%s %s: ha %v exit %d: %s", violation, label, full, code, strings.TrimSpace(out))
	}
	var value map[string]any
	if err := json.Unmarshal([]byte(out), &value); err != nil {
		f.t.Fatalf("setup: %s: ha estimate JSON: %v: %s", label, err, out)
	}
	return value
}

// ccNumber is a JSON number field, and whether it is one.
func ccNumber(m map[string]any, key string) (int, bool) {
	n, ok := m[key].(float64)
	return int(n), ok
}

// startBudget is the start record's budget object.
func (f *ccFixture) startBudget() map[string]any {
	f.t.Helper()
	b, ok := ciLines(f.rgFixture)[0].m["budget"].(map[string]any)
	if !ok {
		f.t.Fatalf("setup: the start record has no budget")
	}
	return b
}

// AC-1: run-rules/7 can be chosen at the start and is listed as supported;
// the default start rule stays run-rules/3.
func TestCheckCostAC1_StartAndCapabilities(t *testing.T) {
	const v = "rules-v7-unsupported"
	var out, errb bytes.Buffer
	code := runMain([]string{"capabilities", "--format", "json"}, &out, &errb)
	if code == exitInternal {
		t.Fatalf("setup: ha capabilities exit %d: %s", code, errb.String())
	}
	if code != exitOK {
		t.Fatalf("jaekit:violation=%s ha capabilities exit %d: %s", v, code, errb.String())
	}
	var caps map[string]any
	if err := json.Unmarshal(out.Bytes(), &caps); err != nil {
		t.Fatalf("setup: ha capabilities JSON: %v", err)
	}
	listed := false
	rules, _ := caps["supported_rules"].([]any)
	for _, r := range rules {
		listed = listed || r == ccRules
	}
	if !listed {
		t.Errorf("jaekit:violation=%s ha capabilities lists supported_rules %v without %s", v, rules, ccRules)
	}
	if caps["default_rules"] != "run-rules/3" {
		t.Errorf("jaekit:violation=%s ha capabilities gives default_rules %v; want run-rules/3", v, caps["default_rules"])
	}

	conds := []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3")}

	// A start without --rules keeps the default rule.
	func() {
		f := ccRepo(t, ccGoal{conds: conds})
		code, out := f.ha("start", rgGoal, "--request", ccRequest)
		if code == exitInternal {
			t.Fatalf("setup: ha start without --rules exit %d: %s", code, out)
		}
		if code != exitOK {
			t.Errorf("jaekit:violation=%s ha start without --rules exit %d: %s", v, code, strings.TrimSpace(out))
			return
		}
		if r := ciLines(f.rgFixture)[0].m["rules"]; r != "run-rules/3" {
			t.Errorf("jaekit:violation=%s a start without --rules records rules %v; want run-rules/3", v, r)
		}
	}()

	// A start with --rules run-rules/7 records it, for a condition table
	// without and with the baseline column.
	for _, form := range []struct {
		label  string
		column bool
	}{{"a condition table without the baseline column", false}, {"a condition table with the baseline column", true}} {
		f := ccRepo(t, ccGoal{conds: conds, column: form.column})
		code, out := f.tryStart()
		if code != exitOK {
			t.Errorf("jaekit:violation=%s %s: ha start --rules %s exit %d: %s", v, form.label, ccRules, code, strings.TrimSpace(out))
			continue
		}
		if r := ciLines(f.rgFixture)[0].m["rules"]; r != ccRules {
			t.Errorf("jaekit:violation=%s %s: ha start --rules %s records rules %v", v, form.label, ccRules, r)
		}
	}
}

// AC-2: under run-rules/7 a required change condition needs a baseline only
// when the current PLAN marks it; an adverse baseline recorded for an
// unmarked one (unexpected_pass, a failure other than the intended one,
// errors past the limit) still blocks it.
func TestCheckCostAC2_SatisfactionFollowsMark(t *testing.T) {
	const v = "baseline-requirement-wrong"
	expect := func(label string, s rgState, id string, satisfied bool, reason string) {
		t.Helper()
		c := ccCriterion(s, id)
		if c == nil {
			t.Errorf("jaekit:violation=%s %s: the status has no criterion %s", v, label, id)
			return
		}
		got, _ := c["reason"].(string)
		if c["satisfied"] != satisfied || got != reason {
			t.Errorf("jaekit:violation=%s %s: %s satisfied %v with reason %q; want satisfied %v with reason %q", v, label, id, c["satisfied"], got, satisfied, reason)
		}
		if reason != "baseline_missing" && ccHasReason(s, id, "baseline_missing") {
			t.Errorf("jaekit:violation=%s %s: the status names %s baseline_missing: %v", v, label, id, s.reasons)
		}
	}

	// A marked condition, an unmarked one with `—`, a maintain condition and
	// an unmarked one with an empty cell whose value already holds at base.
	func() {
		atBase := ccChange("AC-4", "")
		atBase.atBase = true
		f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3"), atBase}, column: true})
		f.start(v)
		f.implement()
		f.must(v, "the current checks", exitOK, "check", rgGoal)

		label := "passing current checks and no baseline"
		s := f.state(v, label)
		expect(label, s, "AC-1", false, "baseline_missing")
		expect(label, s, "AC-2", true, "")
		expect(label, s, "AC-4", true, "")
		for _, l := range ccRuns(ciLines(f.rgFixture), "check") {
			if id := ccTarget(l); id == "AC-2" || id == "AC-4" {
				if e, _ := l.m["evidence"].(map[string]any); e == nil || e["result"] != "pass" {
					t.Errorf("jaekit:violation=%s the current check of the unmarked %s is not judged through its result contract: evidence %v", v, id, l.m["evidence"])
				}
			}
		}

		// The current PLAN decides: a mark removed and a mark added after
		// the start.
		f.mark("AC-1", "—")
		f.mark("AC-2", ccMark)
		label = "the marks of AC-1 and AC-2 swapped after the start"
		s = f.state(v, label)
		expect(label, s, "AC-1", true, "")
		expect(label, s, "AC-2", false, "baseline_missing")
		f.mark("AC-1", ccMark)
		f.mark("AC-2", "—")

		f.must(v, "the baseline of the marked AC-1", exitOK, "check", rgGoal, "AC-1", "--baseline")
		label = "a fresh fail_as_expected baseline of the marked condition"
		s = f.state(v, label)
		expect(label, s, "AC-1", true, "")
		if s.status != "complete" || s.code != exitOK {
			t.Errorf("jaekit:violation=%s %s: status %s (exit %d), reasons %v; want complete", v, label, s.status, s.code, s.reasons)
		}

		// A baseline recorded for an unmarked condition counts with its
		// adverse result.
		f.must(v, "the baseline of the unmarked AC-4", exitProblems, "check", rgGoal, "AC-4", "--baseline")
		label = "an unexpected_pass baseline of the unmarked AC-4"
		s = f.state(v, label)
		expect(label, s, "AC-4", false, "baseline_unexpected_pass")
		if s.status == "complete" {
			t.Errorf("jaekit:violation=%s %s: the goal is complete; want the baseline to block AC-4", v, label)
		}
	}()

	// The other two adverse baselines of unmarked conditions. At base AC-2's
	// producer reports a violation its declaration does not name (a failure
	// other than the intended one). AC-3's command is its runner program,
	// which the goal adds after the start: the current checks run it, and a
	// baseline in the base tree cannot start it, so two baselines record two
	// errors past the limit of one rerun. Each blocks its condition.
	func() {
		unintended := ccChange("AC-2", "—")
		unintended.violation = "value-other"
		unstartable := ccChange("AC-3", "")
		unstartable.runner = true
		f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ccMark), unintended, unstartable, ccMaintain("AC-4")}, column: true})
		f.start(v)
		f.ccAddRunner("AC-3")
		f.implement()
		f.must(v, "the current checks", exitOK, "check", rgGoal)
		f.must(v, "the baseline of the marked AC-1", exitOK, "check", rgGoal, "AC-1", "--baseline")
		label := "passing current checks of the unmarked AC-2 and AC-3 without a baseline"
		s := f.state(v, label)
		expect(label, s, "AC-2", true, "")
		expect(label, s, "AC-3", true, "")
		if s.status != "complete" || s.code != exitOK {
			t.Errorf("jaekit:violation=%s %s: status %s (exit %d), reasons %v; want complete", v, label, s.status, s.code, s.reasons)
		}

		f.must(v, "the baseline of the unmarked AC-2", exitProblems, "check", rgGoal, "AC-2", "--baseline")
		label = "a baseline of the unmarked AC-2 with a violation its declaration does not name"
		s = f.state(v, label)
		expect(label, s, "AC-2", false, "baseline_failed")
		expect(label, s, "AC-3", true, "")
		if s.status == "complete" {
			t.Errorf("jaekit:violation=%s %s: the goal is complete; want the baseline to block AC-2", v, label)
		}

		f.must(v, "the first baseline of the unmarked AC-3", exitProblems, "check", rgGoal, "AC-3", "--baseline")
		f.must(v, "the second baseline of the unmarked AC-3", exitProblems, "check", rgGoal, "AC-3", "--baseline")
		// The premise of this form: both baselines failed to start.
		var starts []string
		for _, l := range ccRuns(ciLines(f.rgFixture), "baseline") {
			if ccTarget(l) == "AC-3" {
				starts = append(starts, fmt.Sprint(l.m["result"]))
			}
		}
		if !rgSame(starts, []string{"error", "error"}) {
			t.Fatalf("setup: the AC-3 baselines recorded %v; the form needs two errors of a command that cannot start", starts)
		}
		label = "two baselines of the unmarked AC-3 whose command cannot start"
		s = f.state(v, label)
		expect(label, s, "AC-3", false, "error_limit")
		if s.status == "complete" {
			t.Errorf("jaekit:violation=%s %s: the goal is complete; want the errors to block AC-3", v, label)
		}
	}()

	// A condition table without marks reads as all unmarked.
	for _, form := range []struct {
		label  string
		column bool
		mark   string
	}{{"a condition table without the baseline column", false, ""}, {"a baseline column without any mark", true, "—"}} {
		f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", form.mark), ccChange("AC-2", ""), ccMaintain("AC-3")}, column: form.column})
		f.start(v)
		f.implement()
		f.must(v, form.label+": the current checks", exitOK, "check", rgGoal)
		s := f.state(v, form.label)
		expect(form.label, s, "AC-1", true, "")
		expect(form.label, s, "AC-2", true, "")
		if s.status != "complete" || s.code != exitOK {
			t.Errorf("jaekit:violation=%s %s: status %s (exit %d), reasons %v; want complete without baselines", v, form.label, s.status, s.code, s.reasons)
		}
	}
}

// AC-3: under run-rules/7 a baseline run without targets selects only the
// marked change conditions; a named unmarked one still runs and counts; with
// no mark nothing is selected and nothing recorded. ha estimate selects the
// same.
func TestCheckCostAC3_DefaultSelectionFollowsMark(t *testing.T) {
	const v = "baseline-selection-wrong"
	selected := func(est map[string]any) []string {
		sel, _ := est["selected"].(map[string]any)
		list, _ := sel["targets"].([]any)
		out := []string{}
		for _, item := range list {
			s, _ := item.(string)
			out = append(out, s)
		}
		return out
	}

	func() {
		f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3"),
			ccChange("AC-4", ccMark), ccChange("AC-5", "")}, column: true})
		f.start(v)
		f.implement()

		est := f.estimate(v, "the started goal")
		if n, ok := ccNumber(est, "default_baseline_runs"); !ok || n != 2 {
			t.Errorf("jaekit:violation=%s ha estimate gives default_baseline_runs %v; want 2 (the marked AC-1 and AC-4)", v, est["default_baseline_runs"])
		}
		est = f.estimate(v, "the started goal's baseline selection", "--baseline")
		if got := selected(est); !rgSame(got, []string{"AC-1", "AC-4"}) {
			t.Errorf("jaekit:violation=%s ha estimate --baseline selects %v; want [AC-1 AC-4]", v, got)
		}

		before := len(ciLines(f.rgFixture))
		f.must(v, "a baseline run without targets", exitOK, "check", rgGoal, "--baseline")
		added := ciLines(f.rgFixture)[before:]
		base := ccRuns(added, "baseline")
		if len(base) != len(added) || !rgSame(ccTargets(base), []string{"AC-1", "AC-4"}) {
			t.Errorf("jaekit:violation=%s ha check --baseline without targets recorded %d lines with baselines of %v; want baselines of [AC-1 AC-4] only",
				v, len(added), ccTargets(base))
		}
		for _, l := range base {
			if l.m["result"] != "fail_as_expected" {
				t.Errorf("jaekit:violation=%s the selected baseline of %s gave %v; want fail_as_expected", v, ccTarget(l), l.m["result"])
			}
		}

		label := "a named baseline of the unmarked AC-2"
		s0 := f.state(v, label)
		runs0, _ := ciInt(ciBudget(f.rgFixture, s0), "runs")
		usage0, _ := s0.value["usage"].(map[string]any)
		baselines0, _ := ccNumber(usage0, "baselines")
		before = len(ciLines(f.rgFixture))
		f.must(v, label, exitOK, "check", rgGoal, "AC-2", "--baseline")
		added = ciLines(f.rgFixture)[before:]
		if len(added) != 1 || added[0].kind() != "baseline" || ccTarget(added[0]) != "AC-2" || added[0].m["result"] != "fail_as_expected" {
			t.Errorf("jaekit:violation=%s %s: recorded %v; want one fail_as_expected baseline of AC-2", v, label, ccTargets(added))
		}
		s1 := f.state(v, label)
		runs1, _ := ciInt(ciBudget(f.rgFixture, s1), "runs")
		usage1, _ := s1.value["usage"].(map[string]any)
		baselines1, _ := ccNumber(usage1, "baselines")
		if runs1 != runs0+1 || baselines1 != baselines0+1 {
			t.Errorf("jaekit:violation=%s %s: budget runs %d → %d and usage baselines %d → %d; want each one more", v, label, runs0, runs1, baselines0, baselines1)
		}
	}()

	// No marked condition: nothing to select.
	func() {
		f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", "—"), ccChange("AC-2", ""), ccMaintain("AC-3")}, column: true})
		f.start(v)
		f.implement()
		est := f.estimate(v, "a goal without marks")
		if n, ok := ccNumber(est, "default_baseline_runs"); !ok || n != 0 {
			t.Errorf("jaekit:violation=%s without marks ha estimate gives default_baseline_runs %v; want 0", v, est["default_baseline_runs"])
		}
		est = f.estimate(v, "a goal without marks, baseline selection", "--baseline")
		if got := selected(est); len(got) != 0 {
			t.Errorf("jaekit:violation=%s without marks ha estimate --baseline selects %v; want none", v, got)
		}
		before := f.records()
		code, out := f.ha("check", rgGoal, "--baseline")
		if code == exitInternal {
			t.Fatalf("setup: ha check --baseline exit %d: %s", code, out)
		}
		if code != exitUsage || !bytes.Equal(f.records(), before) {
			t.Errorf("jaekit:violation=%s without marks ha check --baseline exit %d (want 64) and the record changed %v: %s",
				v, code, !bytes.Equal(f.records(), before), strings.TrimSpace(out))
		}
	}()
}

// AC-4: the run-rules/7 minimum is marked required change × 2 + unmarked
// required change + required maintain; ha start's limit check and automatic
// budget and ha estimate use it.
func TestCheckCostAC4_MinimumFollowsMark(t *testing.T) {
	const v = "baseline-minimum-wrong"
	// The SPEC example: one marked change, four unmarked changes and three
	// maintain conditions give minimum 9 (run-rules/6 gives 13).
	conds := []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccChange("AC-3", ""), ccChange("AC-4", "—"), ccChange("AC-5", ""),
		ccMaintain("AC-6"), ccMaintain("AC-7"), ccMaintain("AC-8")}
	const minimum, auto = 9, 50

	func() {
		f := ccRepo(t, ccGoal{conds: conds, column: true, runs: "자동"})
		est := f.estimate(v, "before the start", "--rules", ccRules)
		a, _ := est["auto"].(map[string]any)
		if n, _ := ccNumber(est, "minimum_runs"); n != minimum {
			t.Errorf("jaekit:violation=%s ha estimate --rules %s before the start gives minimum_runs %v; want %d", v, ccRules, est["minimum_runs"], minimum)
		}
		if n, _ := ccNumber(a, "computed_runs"); n != auto {
			t.Errorf("jaekit:violation=%s ha estimate --rules %s before the start gives auto %v; want computed_runs %d", v, ccRules, est["auto"], auto)
		}

		f.start(v)
		b := f.startBudget()
		ab, _ := b["auto"].(map[string]any)
		runs, _ := ccNumber(b, "runs")
		least, _ := ccNumber(ab, "minimum_runs")
		computed, _ := ccNumber(ab, "computed_runs")
		if runs != auto || least != minimum || computed != auto {
			t.Errorf("jaekit:violation=%s ha start --rules %s with 자동 records budget %v; want %d runs from minimum %d", v, ccRules, b, auto, minimum)
		}

		est = f.estimate(v, "the started goal")
		a, _ = est["auto"].(map[string]any)
		if n, _ := ccNumber(est, "minimum_runs"); n != minimum {
			t.Errorf("jaekit:violation=%s ha estimate on the started goal gives minimum_runs %v; want %d", v, est["minimum_runs"], minimum)
		}
		if n, _ := ccNumber(a, "computed_runs"); n != auto {
			t.Errorf("jaekit:violation=%s ha estimate on the started goal gives auto %v; want computed_runs %d", v, est["auto"], auto)
		}
	}()

	// The start limit check: a PLAN limit of the minimum starts, one less is
	// refused with nothing recorded.
	func() {
		f := ccRepo(t, ccGoal{conds: conds, column: true, runs: fmt.Sprint(minimum)})
		f.start(v)
		if runs, _ := ccNumber(f.startBudget(), "runs"); runs != minimum {
			t.Errorf("jaekit:violation=%s a PLAN limit of %d starts with budget runs %d", v, minimum, runs)
		}
	}()
	func() {
		f := ccRepo(t, ccGoal{conds: conds, column: true, runs: fmt.Sprint(minimum - 1)})
		code, out := f.tryStart()
		data, err := os.ReadFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"))
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("setup: %v", err)
		}
		if code != exitRefused || len(data) > 0 {
			t.Errorf("jaekit:violation=%s a PLAN limit of %d below the minimum %d gave ha start exit %d (want 65) and %d record bytes: %s",
				v, minimum-1, minimum, code, len(data), strings.TrimSpace(out))
		}
	}()
}
