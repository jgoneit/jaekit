package spar

// Checks of the spar-earlier-dispositions goal in the spar package. Every
// test drives spar.Main, the entry cmd/spar calls, with string arguments, so
// the file builds before the change and an option spar does not take yet is
// refused when the test runs. A change test gathers what it observes and
// reports a violation in one failure whose first line starts with
// jaekit:violation=<id>: . A setup that breaks (no git, a file, repository or
// round that cannot be written or read, an exit of spar the test does not
// expect) is an unmarked t.Fatalf before any violation is reported, so an
// input failure is never a pass or a declared violation.

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	edEarlierHeading = "## Findings of earlier rounds"
	// edRereport is the sentence right under that heading when an earlier
	// finding has a recorded disposition.
	edRereport = "Report a finding with a recorded disposition again only with a failure scenario that the recorded disposition does not answer."
	// edTextCheck is the fixed-text check path the rounds declare.
	edTextCheck      = "checks/**"
	edFixedText      = "- Fixed-text checks:"
	edCheckPaths     = "- Check paths:"
	edCheckValidity  = "- `check-validity`: "
	edValidityBefore = edCheckValidity + "For each check the change adds or changes: does it observe the required property at the checked tree, or a proxy? Can it pass on wrong code, or report a tool failure as a violation?"
	// edCheckPathsBefore is the check paths line that
	// TestSparQuestionsAC8_PacketExplainsTheLink pins for --check-path
	// 'checks/**'. That test keeps it in a local variable, so it is repeated
	// here.
	edCheckPathsBefore = "- Check paths: `checks/**`. At these paths a finding that claims `false_pass`, `meaning_inverted` or a linked `contract_violation` is recorded as a `defect`, whatever its `proposed_class`. Any other finding at these paths is recorded as `hardening` and deferred, unless it proposes `requirement-question` and links the criterion, rule or contract it asks about, and that link exists: such a question stays open for the user. A `contract` link to a file in these paths does not keep a question open."
)

// edValiditySentence is the sentence the check-validity line holds in a
// round with fixed-text checks.
const edValiditySentence = "For a check declared as fixed-text only, judge whether it reads the right fixed text in the right file; " +
	"report that fixed text cannot prove the behavior only as a `requirement-question` that names the SPEC criterion, contract or protected path requiring it."

// edViolations gathers what a change test observes against its condition
// and reports it as one marked failure.
type edViolations struct {
	t    *testing.T
	id   string
	list []string
}

func (v *edViolations) add(format string, args ...any) {
	v.list = append(v.list, fmt.Sprintf(format, args...))
}

func (v *edViolations) report() {
	v.t.Helper()
	if len(v.list) > 0 {
		v.t.Errorf("jaekit:violation=%s: %s", v.id, strings.Join(v.list, "\n"))
	}
}

// edLinesWith are the lines of text that start with prefix.
func edLinesWith(text, prefix string) []string {
	var lines []string
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, prefix) {
			lines = append(lines, line)
		}
	}
	return lines
}

// edUnderHeading is the first paragraph of a section, the text between its
// heading and the first blank line or list item after it.
func edUnderHeading(section string) string {
	lines := strings.Split(section, "\n")
	var para []string
	for _, line := range lines[1:] {
		switch {
		case strings.TrimSpace(line) == "" && len(para) == 0:
			continue
		case strings.TrimSpace(line) == "", strings.HasPrefix(line, "- "):
			return strings.Join(para, "\n")
		}
		para = append(para, line)
	}
	return strings.Join(para, "\n")
}

// edFindings reads the findings a round recorded.
func (w *rptWorld) edFindings(dir string) []Finding {
	w.t.Helper()
	var list []Finding
	if err := json.Unmarshal([]byte(w.read(filepath.Join(dir, "findings.json"))), &list); err != nil {
		w.t.Fatalf("setup: findings.json of %s: %v", filepath.Base(dir), err)
	}
	return list
}

// edPriorLines are the earlier findings' lines of round dir in the form a
// fixes packet gives them, each finding's line followed by the disposition
// text given for its ID.
func (w *rptWorld) edPriorLines(dir string, dispositions map[string]string) []string {
	w.t.Helper()
	var lines []string
	for _, f := range w.edFindings(dir) {
		lines = append(lines, fmt.Sprintf("- %s %s [%s] at %s: %s%s", filepath.Base(dir), f.ID, f.Class, f.Location, f.Claim, dispositions[f.ID]))
	}
	return lines
}

// edPlant gives the dry run dry the findings of round from with another
// claim, so that a packet that read the dry run would show that claim.
func (w *rptWorld) edPlant(dry, from, claim string) {
	w.t.Helper()
	list := w.edFindings(from)
	for i := range list {
		list[i].Claim = claim + " " + list[i].ID
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	w.write(filepath.Join(dry, "findings.json"), string(data)+"\n")
}

// edReviewerStatus is the status round.json gives reviewer name, or "".
func (w *rptWorld) edReviewerStatus(dir, name string) string {
	w.t.Helper()
	reviewers, _ := w.rawRound(dir)["reviewers"].(map[string]any)
	state, _ := reviewers[name].(map[string]any)
	status, _ := state["status"].(string)
	return status
}

// edRerun runs, in a series of its own, a candidate round from c0 to c1, a
// fixes round from c1 to c2 with one finding, and then the candidate round's
// identity again with only the reviewer that did not complete it, and adds
// to v what the rerun did not do: get the first round's packet byte for
// byte, without earlier findings, and be recorded as an uncounted rerun of
// it. The series holds two counted code rounds before the rerun, so that a
// rerun counted as a new round is still admitted under the code cap of
// three and recorded, never refused.
func (w *rptWorld) edRerun(v *edViolations) {
	w.t.Helper()
	c := w.commits
	series := filepath.Join(w.repo, "reviews", "ed-rerun")
	// late is a reviewer that never completes, so that its rerun of the same
	// packet is not counted.
	late := pdCommand("late", w.script("late", "if [ \"$1\" = --version ]; then exit 0; fi\nexit 3\n"))
	w.useConfig(w.printer(w.answer("ed-rerun-candidate", c[1], rptFinding("F1", "defect", "AC-1", "rerun claim one"))), late)
	first := filepath.Join(series, "01-candidate")
	w.run(w.round("candidate", c[0], c[1], first)...)
	if s := w.edReviewerStatus(first, "late"); s == "" || s == StatusCompleted {
		w.t.Fatalf("setup: reviewer late of 01-candidate is %q, want a status other than completed", s)
	}
	w.useConfig(w.printer(w.answer("ed-rerun-fixes", c[2], rptFinding("F1", "defect", "AC-1", "rerun fixes claim"))), late)
	fixes := filepath.Join(series, "02-fixes")
	w.run(w.round("fixes", c[1], c[2], fixes)...)
	if n := len(w.edFindings(fixes)); n != 1 {
		w.t.Fatalf("setup: 02-fixes recorded %d findings, want 1", n)
	}
	again := filepath.Join(series, "03-candidate")
	w.run(append(w.round("candidate", c[0], c[1], again), "--reviewers", "late")...)

	packet := w.read(filepath.Join(again, "packet.md"))
	if packet != w.read(filepath.Join(first, "packet.md")) {
		v.add("the run of 01-candidate's stage, base and head after 02-fixes gets another packet than 01-candidate")
	}
	if section := pdSectionOf(packet, edEarlierHeading); section != "" {
		v.add("the run of 01-candidate's stage, base and head after 02-fixes lists earlier findings that 01-candidate did not get: %q",
			edLinesWith(section, "- "))
	}
	r := w.rawRound(again)
	if r["counted"] != false || !reflect.DeepEqual(r["rerun_of"], []any{"01-candidate"}) {
		v.add("the run of 01-candidate's stage, base and head after 02-fixes is recorded with counted %v and rerun_of %v, want false and [01-candidate]",
			r["counted"], r["rerun_of"])
	}
}

// AC-1: a candidate round's packet lists the findings of the series' earlier
// code rounds, each with its last recorded disposition, in the form fixes
// rounds give them, and leaves out plan rounds, dry runs and rounds of its
// own stage, base and head; round.json names the rounds it read. A later run
// of a recorded round's stage, base and head gets only the code rounds
// recorded before that round, so it reruns the same packet.
func TestSparEarlierDispositionsAC1_CandidateGetsEarlierFindings(t *testing.T) {
	v := &edViolations{t: t, id: "candidate-without-earlier-findings"}
	w := newRptWorld(t, true)
	c := w.commits
	series := filepath.Join(w.repo, "reviews", "ed")
	const evidence = "AC-1 asks for the first greeting"
	const planClaim, dryClaim, sameClaim = "plan round claim", "dry run claim", "same identity claim"

	// 01: a plan round from c0 to c1 with one finding.
	w.useConfig(w.printer(w.answer("ed-plan", c[1], rptFinding("F1", "defect", "AC-1", planClaim))))
	plan := filepath.Join(series, "01-plan")
	w.run(append(w.round("plan", c[0], c[1], plan), "--subject", filepath.Join(w.repo, "docs", "PLAN.md"))...)
	if n := len(w.edFindings(plan)); n != 1 {
		t.Fatalf("setup: the plan round recorded %d findings, want 1", n)
	}
	// 02: a candidate round from c0 to c1 with two findings; the first is
	// rejected, the second stays open.
	w.useConfig(w.printer(w.answer("ed-candidate", c[1],
		rptFinding("F1", "defect", "AC-1", "claim one"), rptFinding("F2", "defect", "AC-1", "claim two"))))
	cand := filepath.Join(series, "02-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	w.triage(cand, "cmd-F1", "reject", "--evidence", evidence)
	// 03: a fixes round from c1 to c2 with one open finding.
	w.useConfig(w.printer(w.answer("ed-fixes", c[2], rptFinding("F1", "defect", "AC-1", "fixes claim"))))
	fixes := filepath.Join(series, "03-fixes")
	w.run(w.round("fixes", c[1], c[2], fixes)...)
	// 04: a dry run, given findings of its own.
	dry := filepath.Join(series, "04-candidate")
	w.run(append(w.round("candidate", c[0], c[1], dry), "--dry-run")...)
	w.edPlant(dry, cand, dryClaim)
	// 05: a candidate round of the new round's stage, base and head.
	w.useConfig(w.printer(w.answer("ed-same", c[2], rptFinding("F1", "defect", "AC-1", sameClaim))))
	same := filepath.Join(series, "05-candidate")
	w.run(w.round("candidate", c[0], c[2], same)...)
	// 06: the new candidate round from c0 to c2, whose head differs from the
	// first candidate round's and whose base differs from the fixes round's.
	out := filepath.Join(series, "06-candidate")
	w.run(append(w.round("candidate", c[0], c[2], out), "--dry-run")...)

	want := append(w.edPriorLines(cand, map[string]string{"cmd-F1": " — reject: " + evidence}), w.edPriorLines(fixes, nil)...)
	if len(want) != 3 {
		t.Fatalf("setup: the candidate and fixes rounds recorded %d findings, want 3", len(want))
	}
	// The form above is the one the fixes round's own packet gives the
	// candidate round's findings; a form that differs is a broken premise of
	// this check, not an observation.
	if got := edLinesWith(pdSectionOf(w.read(filepath.Join(fixes, "packet.md")), edEarlierHeading), "- "); !reflect.DeepEqual(got, want[:2]) {
		t.Fatalf("setup: the fixes packet lists the earlier findings as %q, not %q", got, want[:2])
	}

	packet := w.read(filepath.Join(out, "packet.md"))
	if section := pdSectionOf(packet, edEarlierHeading); section == "" {
		v.add("the candidate packet has no %q section", edEarlierHeading)
	} else {
		if under := edUnderHeading(section); !strings.Contains(under, edRereport) {
			v.add("right under %q the packet says %q, not %q", edEarlierHeading, under, edRereport)
		}
		got := edLinesWith(section, "- ")
		for _, line := range want {
			if n := pdCount(got, line); n != 1 {
				v.add("the earlier findings list %q %d times, want once", line, n)
			}
		}
		for _, line := range got {
			if !slices.Contains(want, line) {
				v.add("the earlier findings list %q, which is not a finding of 02-candidate or 03-fixes", line)
			}
		}
		if strings.Contains(section, planClaim) {
			v.add("the earlier findings hold %q of the plan round 01-plan", planClaim)
		}
		for _, claim := range []string{dryClaim, sameClaim} {
			if strings.Contains(section, claim) {
				v.add("the earlier findings hold %q of a dry run or of a round with the same stage, base and head", claim)
			}
		}
	}
	var prior []string
	list, _ := w.rawRound(out)["prior"].([]any)
	for _, p := range list {
		s, _ := p.(string)
		prior = append(prior, s)
	}
	slices.Sort(prior)
	if slices.Contains(prior, "01-plan") {
		v.add("round.json prior names the plan round 01-plan")
	}
	if wantPrior := []string{"02-candidate", "03-fixes"}; !reflect.DeepEqual(prior, wantPrior) {
		v.add("round.json prior is %q, want %q", prior, wantPrior)
	}
	w.edRerun(v)
	v.report()
}

// edDry runs a dry candidate round of the world's base and head into series
// and returns its packet and round.json. A refusal that names --text-check
// is the observation that spar does not take it: it is added to v and ok is
// false. With v nil, or for any other exit, the run is a broken setup.
func (w *cpqWorld) edDry(v *edViolations, series string, extra ...string) (packet string, round map[string]any, ok bool) {
	w.t.Helper()
	out := filepath.Join(w.repo, "reviews", series, "01-candidate")
	args := append([]string{"run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", out,
		"--criteria", w.criteria, "--repo", w.repo, "--dry-run"}, extra...)
	code, msg := w.spar(args...)
	if code != ExitOK {
		if v != nil && (code == ExitUsage || code == ExitRefused) && strings.Contains(msg, "text-check") && slices.Contains(extra, "--text-check") {
			v.add("spar run does not take --text-check (exit %d): %s", code, pdFirstLine(msg))
			return "", nil, false
		}
		w.t.Fatalf("setup: spar run %v: exit %d %s", extra, code, msg)
	}
	data, err := os.ReadFile(filepath.Join(out, "packet.md"))
	if err != nil {
		w.t.Fatalf("setup: packet.md: %v", err)
	}
	raw, err := os.ReadFile(filepath.Join(out, "round.json"))
	if err != nil {
		w.t.Fatalf("setup: round.json: %v", err)
	}
	if err := json.Unmarshal(raw, &round); err != nil {
		w.t.Fatalf("setup: round.json: %v", err)
	}
	return string(data), round, true
}

// edRepoText reads a file of the repository by its path from the root, two
// folders above this package. A file that cannot be read or is not UTF-8 is
// a broken setup.
func edRepoText(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("setup: %s cannot be read: %v", rel, err)
	}
	if !utf8.Valid(data) {
		t.Fatalf("setup: %s is not UTF-8", rel)
	}
	return string(data)
}

// edRefusal runs a candidate round of the world's base and head with extra
// in series, under a command reviewer that writes a file whenever spar
// starts it, and reports spar's exit and messages, whether the reviewer
// started and whether the round folder exists.
func (w *cpqWorld) edRefusal(series string, extra ...string) (code int, msg string, started, written bool) {
	w.t.Helper()
	marker := filepath.Join(w.dir, series+"-started")
	script := w.write(filepath.Join(w.dir, "bin", series), "#!/bin/sh\nprintf started > "+amQuote(marker)+"\nexit 3\n")
	if err := os.Chmod(script, 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	config := w.write(filepath.Join(w.dir, series+".toml"), pdCommand("starts", script))
	out := filepath.Join(w.repo, "reviews", series, "01-candidate")
	args := append([]string{"run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", out,
		"--criteria", w.criteria, "--repo", w.repo}, extra...)
	saved := w.getenv
	w.getenv = func(k string) string {
		if k == "SPAR_CONFIG" {
			return config
		}
		return saved(k)
	}
	code, msg = w.spar(args...)
	w.getenv = saved
	exists := func(path string) bool {
		_, err := os.Lstat(path)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			w.t.Fatalf("setup: %v", err)
		}
		return err == nil
	}
	return code, msg, exists(marker), exists(out)
}

// AC-3: spar run takes --text-check, given any number of times: round.json
// keeps the paths sorted and unique, the packet names them as check paths
// and as fixed-text checks, a check path not declared a fixed-text check is
// listed as before, the same input gives the same packet, a round without
// the option names no fixed-text check, a path outside the repository is
// refused as an invalid --check-path is, and the spar configuration contract
// names the option.
func TestSparEarlierDispositionsAC3_TextCheckDeclared(t *testing.T) {
	v := &edViolations{t: t, id: "text-check-not-declared"}
	for _, rel := range []string{"contracts/spar-config.en.md", "contracts/spar-config.md"} {
		if !strings.Contains(edRepoText(t, rel), "--text-check") {
			v.add("%s does not have %q", rel, "--text-check")
		}
	}
	w := newCPQWorld(t, true)
	packet, round, ok := w.edDry(v, "tc-a", "--text-check", edTextCheck)
	if !ok {
		v.report()
		return
	}
	config, _ := round["config"].(map[string]any)
	declared, _ := config["text_check_paths"].([]any)
	if !slices.Contains(declared, any(edTextCheck)) {
		v.add("round.json config.text_check_paths is %v, without %q", config["text_check_paths"], edTextCheck)
	}
	quoted := "`" + edTextCheck + "`"
	if lines := edLinesWith(packet, edCheckPaths); len(lines) != 1 || !strings.Contains(lines[0], quoted) {
		v.add("the packet's check paths lines %q do not name %s", lines, quoted)
	}
	named := false
	for _, line := range edLinesWith(packet, edFixedText+" ") {
		named = named || strings.Contains(line, quoted)
	}
	if !named {
		v.add("the packet has no line starting with %q that names %s", edFixedText+" ", quoted)
	}
	if again, _, ok := w.edDry(nil, "tc-b", "--text-check", edTextCheck); ok && again != packet {
		v.add("a second run with the same input gives another packet")
	}
	if without, _, _ := w.edDry(nil, "tc-none", "--check-path", edTextCheck); len(edLinesWith(without, edFixedText)) > 0 {
		v.add("a round without --text-check has the lines %q", edLinesWith(without, edFixedText))
	}
	w.edUndeclared(v)
	w.edRepeated(v)
	w.edOutside(v)
	v.report()
}

// edUndeclared: with --check-path 'tests/**' --text-check 'docs_test/**',
// the check paths line is the one a round with both globs given as
// --check-path has today, in either order, and the fixed-text checks line
// names only docs_test/**.
func (w *cpqWorld) edUndeclared(v *edViolations) {
	w.t.Helper()
	const plain, text = "tests/**", "docs_test/**"
	packet, _, ok := w.edDry(v, "tc-undeclared", "--check-path", plain, "--text-check", text)
	if !ok {
		return
	}
	var today []string
	for i, order := range [][]string{{plain, text}, {text, plain}} {
		before, _, _ := w.edDry(nil, fmt.Sprintf("tc-undeclared-before-%d", i), "--check-path", order[0], "--check-path", order[1])
		lines := edLinesWith(before, edCheckPaths)
		if len(lines) != 1 {
			w.t.Fatalf("setup: a round with the check paths %q has the check paths lines %q", order, lines)
		}
		today = append(today, lines[0])
	}
	if lines := edLinesWith(packet, edCheckPaths); len(lines) != 1 || !slices.Contains(today, lines[0]) {
		v.add("with --check-path %s --text-check %s the check paths lines are %q, not one of %q", plain, text, lines, today)
	}
	if lines := edLinesWith(packet, edFixedText); len(lines) != 1 || !strings.Contains(lines[0], "`"+text+"`") || strings.Contains(lines[0], "`"+plain+"`") {
		v.add("with --check-path %s --text-check %s the fixed-text checks lines are %q, not one line that names only `%s`", plain, text, lines, text)
	}
}

// edRepeated: --text-check given for b/**, a/** and b/** again keeps
// ["a/**", "b/**"] in round.json, and the fixed-text checks line names both.
func (w *cpqWorld) edRepeated(v *edViolations) {
	w.t.Helper()
	packet, round, ok := w.edDry(v, "tc-repeated", "--text-check", "b/**", "--text-check", "a/**", "--text-check", "b/**")
	if !ok {
		return
	}
	config, _ := round["config"].(map[string]any)
	if got, want := config["text_check_paths"], []any{"a/**", "b/**"}; !reflect.DeepEqual(got, want) {
		v.add("with --text-check b/** a/** b/** round.json config.text_check_paths is %v, want %v", got, want)
	}
	lines := edLinesWith(packet, edFixedText)
	if len(lines) != 1 || !strings.Contains(lines[0], "`a/**`") || !strings.Contains(lines[0], "`b/**`") {
		v.add("with --text-check b/** a/** b/** the fixed-text checks lines are %q, not one line that names `a/**` and `b/**`", lines)
	}
}

// edOutside: --text-check '../outside' is refused as --check-path
// '../outside' is: with the same exit, before any reviewer starts, and
// without a round folder.
func (w *cpqWorld) edOutside(v *edViolations) {
	w.t.Helper()
	const outside = "../outside"
	want, msg, started, written := w.edRefusal("tc-outside-check-path", "--check-path", outside)
	if want != ExitRefused || started || written {
		w.t.Fatalf("setup: --check-path %s gives exit %d (reviewer started %v, round written %v), not the refusal before anything is written: %s",
			outside, want, started, written, msg)
	}
	code, msg, started, written := w.edRefusal("tc-outside-text-check", "--text-check", outside)
	if code != want || started || written {
		v.add("--text-check %s gives exit %d (reviewer started %v, round written %v), not exit %d before any reviewer starts and without a round as --check-path %s: %s",
			outside, code, started, written, want, outside, pdFirstLine(msg))
	}
}

// edValidityLine is the packet's one check-validity line, or "" with a
// note in v when it has none or several.
func edValidityLine(v *edViolations, label, packet string) string {
	lines := edLinesWith(packet, edCheckValidity)
	if len(lines) != 1 {
		v.add("the packet %s has %d check-validity lines, want 1", label, len(lines))
		return ""
	}
	return lines[0]
}

// AC-4: in a round with fixed-text checks the check-validity line holds the
// one sentence on fixed-text checks: judge whether a text check reads the
// right fixed text in the right file, and report that fixed text cannot
// prove the behavior only as a requirement-question naming what requires
// it; without them it is the earlier text; and the declaration does not
// change how a false_pass finding at that path is classified.
func TestSparEarlierDispositionsAC4_CheckValidityForTextChecks(t *testing.T) {
	v := &edViolations{t: t, id: "text-check-guidance-missing"}
	w := newCPQWorld(t, true)
	with, _, ok := w.edDry(v, "tv-a", "--text-check", edTextCheck)
	if !ok {
		v.report()
		return
	}
	if line := edValidityLine(v, "with --text-check", with); line != "" && !strings.Contains(line, edValiditySentence) {
		v.add("the check-validity line with --text-check does not hold %q: %q", edValiditySentence, line)
	}
	for label, extra := range map[string][]string{
		"without options":        nil,
		"with --check-path only": {"--check-path", edTextCheck},
	} {
		packet, _, _ := w.edDry(nil, "tv-"+strings.ReplaceAll(label, " ", "-"), extra...)
		if line := edValidityLine(v, label, packet); line != "" && line != edValidityBefore {
			v.add("the check-validity line %s is %q, not the earlier %q", label, line, edValidityBefore)
		}
	}

	// The same false_pass findings at checks/test_app.py, whatever class
	// they propose, recorded with the path as a check path, with the path
	// also declared a fixed-text check, and with only the declaration.
	rules := filepath.Join(w.repo, "AGENTS.md")
	var cases []map[string]any
	for i, values := range []map[string]any{
		{"proposed_class": "note"},
		{"proposed_class": "hardening"},
		{"proposed_class": "defect", "link_kind": "criterion", "link": "AC-1"},
		{"proposed_class": "requirement-question", "link_kind": "criterion", "link": "AC-1"},
	} {
		values["check_impact"] = "false_pass"
		cases = append(cases, cpqFinding(fmt.Sprintf("F%d", i+1), values))
	}
	type recorded struct {
		findings map[string]Finding
		ledger   map[string][]LedgerEntry
	}
	record := func(options ...string) recorded {
		out := w.roundWith(append([]string{"--rules", rules}, options...), cases...)
		r := recorded{w.findings(out), w.ledger(out)}
		if len(r.findings) != len(cases) {
			t.Fatalf("setup: recorded %d findings for %d cases", len(r.findings), len(cases))
		}
		return r
	}
	plain := record("--check-path", edTextCheck)
	for id, f := range plain.findings {
		if !f.InCheckPaths {
			t.Fatalf("setup: %s at %s is not in the check paths %s", id, f.Location, edTextCheck)
		}
	}
	dispositions := func(r recorded, f Finding) []string {
		var list []string
		for _, e := range r.ledger[f.ID] {
			list = append(list, fmt.Sprintf("%s auto=%v", e.Disposition, e.Auto))
		}
		return list
	}
	for label, options := range map[string][]string{
		"also declared a fixed-text check": {"--check-path", edTextCheck, "--text-check", edTextCheck},
		"declared only a fixed-text check": {"--text-check", edTextCheck},
	} {
		other := record(options...)
		for id, a := range plain.findings {
			b := other.findings[id]
			if a.Class != b.Class || a.InCheckPaths != b.InCheckPaths || !reflect.DeepEqual(dispositions(plain, a), dispositions(other, b)) {
				v.add("the false_pass finding %s proposing %s is %s (in check paths %v, ledger %v) at a check path, but %s (in check paths %v, ledger %v) when the path is %s",
					id, a.ProposedClass, a.Class, a.InCheckPaths, dispositions(plain, a), b.Class, b.InCheckPaths, dispositions(other, b), label)
			}
		}
	}
	v.report()
}

// AC-6: a round with check paths and without --text-check keeps the check
// paths line of the earlier packet byte for byte.
func TestSparEarlierDispositionsAC6_CheckPathsLineUnchanged(t *testing.T) {
	w := newCPQWorld(t, true)
	packet, _, _ := w.edDry(nil, "keep", "--check-path", edTextCheck)
	if lines := edLinesWith(packet, edCheckPaths); len(lines) != 1 || lines[0] != edCheckPathsBefore {
		t.Errorf("the check paths lines are %q, want only %q", lines, edCheckPathsBefore)
	}
}
