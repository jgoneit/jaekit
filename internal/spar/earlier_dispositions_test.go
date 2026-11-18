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
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
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

// edValidityGuidance is what the check-validity line says in a round with
// fixed-text checks.
var edValidityGuidance = []string{"right fixed text in the right file", "requirement-question", "SPEC criterion, contract or protected path"}

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

// AC-1: a candidate round's packet lists the findings of the series' earlier
// code rounds, each with its last recorded disposition, in the form fixes
// rounds give them, and leaves out dry runs and rounds of its own stage,
// base and head; round.json names the rounds it read.
func TestSparEarlierDispositionsAC1_CandidateGetsEarlierFindings(t *testing.T) {
	v := &edViolations{t: t, id: "candidate-without-earlier-findings"}
	w := newRptWorld(t, true)
	c := w.commits
	series := filepath.Join(w.repo, "reviews", "ed")
	const evidence = "AC-1 asks for the first greeting"
	const dryClaim, sameClaim = "dry run claim", "same identity claim"

	// 01: a candidate round from c0 to c1 with two findings; the first is
	// rejected, the second stays open.
	w.useConfig(w.printer(w.answer("ed-candidate", c[1],
		rptFinding("F1", "defect", "AC-1", "claim one"), rptFinding("F2", "defect", "AC-1", "claim two"))))
	cand := filepath.Join(series, "01-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	w.triage(cand, "cmd-F1", "reject", "--evidence", evidence)
	// 02: a fixes round from c1 to c2 with one open finding.
	w.useConfig(w.printer(w.answer("ed-fixes", c[2], rptFinding("F1", "defect", "AC-1", "fixes claim"))))
	fixes := filepath.Join(series, "02-fixes")
	w.run(w.round("fixes", c[1], c[2], fixes)...)
	// 03: a dry run, given findings of its own.
	dry := filepath.Join(series, "03-candidate")
	w.run(append(w.round("candidate", c[0], c[1], dry), "--dry-run")...)
	w.edPlant(dry, cand, dryClaim)
	// 04: a candidate round of the new round's stage, base and head.
	w.useConfig(w.printer(w.answer("ed-same", c[2], rptFinding("F1", "defect", "AC-1", sameClaim))))
	same := filepath.Join(series, "04-candidate")
	w.run(w.round("candidate", c[0], c[2], same)...)
	// 05: the new candidate round from c0 to c2, whose head differs from the
	// first candidate round's and whose base differs from the fixes round's.
	out := filepath.Join(series, "05-candidate")
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
				v.add("the earlier findings list %q, which is not a finding of 01-candidate or 02-fixes", line)
			}
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
	if wantPrior := []string{"01-candidate", "02-fixes"}; !reflect.DeepEqual(prior, wantPrior) {
		v.add("round.json prior is %q, want %q", prior, wantPrior)
	}
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

// AC-3: spar run takes --text-check: round.json keeps the path, the packet
// names it as a check path and as a fixed-text check, the same input gives
// the same packet, and a round without the option names no fixed-text
// check.
func TestSparEarlierDispositionsAC3_TextCheckDeclared(t *testing.T) {
	v := &edViolations{t: t, id: "text-check-not-declared"}
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
	v.report()
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

// AC-4: in a round with fixed-text checks the check-validity question asks
// whether a text check observes the right fixed text in the right file and
// sends a requirement reading to a requirement-question; without them it is
// the earlier text; and the declaration does not change how a false_pass
// finding at that path is classified.
func TestSparEarlierDispositionsAC4_CheckValidityForTextChecks(t *testing.T) {
	v := &edViolations{t: t, id: "text-check-guidance-missing"}
	w := newCPQWorld(t, true)
	with, _, ok := w.edDry(v, "tv-a", "--text-check", edTextCheck)
	if !ok {
		v.report()
		return
	}
	if line := edValidityLine(v, "with --text-check", with); line != "" {
		for _, token := range edValidityGuidance {
			if !strings.Contains(line, token) {
				v.add("the check-validity line with --text-check does not say %q: %q", token, line)
			}
		}
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
