package spar

// Checks of the review-repeat-findings goal. Every change test observes the
// behavior through spar.Main, an entry that already existed before the goal,
// so the file builds on the commit before the change and a missing behavior
// fails an assertion marked jaekit:violation=<id>. A setup problem skips a
// change test (pdSetup) and fails a maintain test, so that an input failure is
// never a pass or a violation.

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/reviewseries"
)

// rptBoundaryHeading opens the packet section that holds the boundary
// documents; rptEarlierHeading opens the earlier findings.
const (
	rptBoundaryHeading = "## Check boundaries"
	rptEarlierHeading  = "## Findings of earlier rounds"
	// rptRereport and rptFixed, rptNotFixed are the guidance that a packet with
	// a recorded disposition carries.
	rptRereport = "only with a failure scenario that the recorded disposition does not answer"
	rptFixed    = "recorded as fixed"
	rptNotFixed = "recorded as rejected, deferred, duplicate or answered"
	// rptOldFixes is the fixes stage line of a packet without dispositions.
	rptOldFixes = "The head fixes findings of earlier rounds. Review the fixes from base to head, and whether they fix those findings without breaking anything else."
)

type rptWorld struct{ *pdWorld }

func newRptWorld(t *testing.T, keeps bool) *rptWorld { return &rptWorld{newPDWorld(t, keeps)} }

// main runs one spar command with the world's configuration.
func (w *rptWorld) main(args ...string) (int, string, string) {
	var stdout, stderr strings.Builder
	getenv := func(k string) string {
		switch k {
		case "SPAR_CONFIG":
			return w.config
		case "HOME":
			return w.dir
		}
		return ""
	}
	code := Main("test", args, strings.NewReader(""), &stdout, &stderr, getenv)
	return code, stdout.String(), stderr.String()
}

// run is a round that must be written: spar run exits 0, or 1 when no
// reviewer completed.
func (w *rptWorld) run(args ...string) {
	w.t.Helper()
	code, stderr := w.spar(args...)
	if code != ExitOK && code != ExitNoResult {
		pdSetup(w.t, w.keeps, "spar run exits %d: %s", code, stderr)
	}
}

func (w *rptWorld) triage(out, finding, disposition string, flags ...string) {
	w.t.Helper()
	code, _, stderr := w.main(append([]string{"triage", out, finding, disposition}, flags...)...)
	if code != ExitOK {
		pdSetup(w.t, w.keeps, "spar triage %s %s %s exits %d: %s", filepath.Base(out), finding, disposition, code, stderr)
	}
}

// earlierLine is the line of the packet's earlier findings for finding id of
// round dir, or "" when there is none.
func rptEarlierLine(packet, dir, id string) string {
	for _, line := range strings.Split(pdSectionOf(packet, rptEarlierHeading), "\n") {
		if strings.HasPrefix(line, "- "+dir+" "+id+" [") {
			return line
		}
	}
	return ""
}

func rptFinding(id, class, link, claim string) map[string]any {
	values := map[string]any{"proposed_class": class, "claim": claim, "location": "head/app/main.py:1"}
	if link != "" {
		values["link_kind"], values["link"] = "criterion", link
	}
	return rawFinding(id, values)
}

// rptLongEvidence is longer than an earlier finding's claim may be, with a
// beginning, a backtick, a line break and an end that a shortened line
// cannot hold.
var rptLongEvidence = "근거-시작 `code` 처음 줄\n둘째 줄 " + strings.Repeat("반박 근거가 이어진다. ", 120) + "근거-끝"

// candidateWithDispositions runs a candidate round from commit 0 to 1 whose
// command reviewer reports one finding per disposition, one hardening item
// that spar defers on its own and one finding left open, and records the
// dispositions. It returns the round folder.
func (w *rptWorld) candidateWithDispositions(series string) string {
	w.t.Helper()
	c := w.commits
	findings := []any{
		rptFinding("F1", "defect", "AC-1", "claim one"),
		rptFinding("F2", "defect", "AC-1", "claim two"),
		rptFinding("F3", "defect", "AC-1", "claim three"),
		rptFinding("F4", "defect", "AC-1", "claim four"),
		rptFinding("F5", "requirement-question", "AC-2", "claim five"),
		rptFinding("F6", "hardening", "", "claim six"),
		rptFinding("F7", "defect", "AC-1", "claim seven"),
	}
	w.useConfig(w.printer(w.answer(series+"-candidate", c[1], findings...)))
	out := filepath.Join(w.repo, "reviews", series, "01-candidate")
	w.run(w.round("candidate", c[0], c[1], out)...)
	w.triage(out, "cmd-F1", "fix", "--commit", c[2], "--failed-before", "the greeting was wrong", "--passed-after", "it is right")
	w.triage(out, "cmd-F2", "reject", "--evidence", rptLongEvidence)
	w.triage(out, "cmd-F3", "defer", "--to", "#999", "--user-quote", "미뤄도 된다는 사용자의 말")
	w.triage(out, "cmd-F4", "duplicate", "--of", "cmd-F1")
	w.triage(out, "cmd-F5", "answer", "--user-quote", "사용자의 답 원문")
	return out
}

// fixesPacket is the packet of a fixes dry run from commit 1 to 2 after the
// candidate round of series.
func (w *rptWorld) fixesPacket(series string, extra ...string) string {
	w.t.Helper()
	out := filepath.Join(w.repo, "reviews", series, "02-fixes")
	w.run(append(append(w.round("fixes", w.commits[1], w.commits[2], out), "--dry-run"), extra...)...)
	return w.read(filepath.Join(out, "packet.md"))
}

// AC-1: each disposition and the automatic deferral show on the finding's
// line of a fixes packet, and of a plan-delta packet.
func TestRepeatFindingsAC1_DispositionShown(t *testing.T) {
	const id = "disposition-not-shown"
	w := newRptWorld(t, false)
	packet := w.fixesPacket(filepathSeries(w, "d"))
	for _, want := range []struct{ finding, disposition, detail string }{
		{"cmd-F1", "fix", shortID(w.commits[2])},
		{"cmd-F2", "reject", "근거-시작"},
		{"cmd-F3", "defer", "#999"},
		{"cmd-F4", "duplicate", "cmd-F1"},
		{"cmd-F5", "answer", "사용자의 답 원문"},
		{"cmd-F6", "defer", "auto"},
	} {
		line := rptEarlierLine(packet, "01-candidate", want.finding)
		if line == "" {
			pdSetup(t, false, "the fixes packet lists no %s", want.finding)
		}
		after := strings.SplitN(line, ": ", 2)
		if len(after) < 2 || !strings.Contains(after[1], want.disposition) || !strings.Contains(after[1], want.detail) {
			t.Errorf("jaekit:violation=%s the line of %s does not show %s with %q: %s", id, want.finding, want.disposition, want.detail, line)
		}
	}

	// A plan-delta round shows the dispositions of the plan round.
	c := w.commits
	plan := filepath.Join(w.repo, "docs", "PLAN.md")
	w.useConfig(w.printer(w.answer("plan", c[1], rptFinding("F1", "defect", "AC-1", "the plan misses AC-1"))))
	out := filepath.Join(w.repo, "reviews", "p", "01-plan")
	w.run(append(w.round("plan", c[0], c[1], out), "--subject", plan)...)
	w.triage(out, "cmd-F1", "reject", "--evidence", "계획의 둘째 문단이 AC-1을 다룬다")
	delta := filepath.Join(w.repo, "reviews", "p", "02-plan-delta")
	w.run(append(w.round("plan-delta", c[0], c[2], delta), "--subject", plan, "--dry-run")...)
	line := rptEarlierLine(w.read(filepath.Join(delta, "packet.md")), "01-plan", "cmd-F1")
	if line == "" {
		pdSetup(t, false, "the plan-delta packet lists no cmd-F1")
	}
	if !strings.Contains(line, "reject") || !strings.Contains(line, "계획의 둘째 문단이 AC-1을 다룬다") {
		t.Errorf("jaekit:violation=%s the plan-delta line does not show the reject and its evidence: %s", id, line)
	}
}

// filepathSeries runs the candidate round with dispositions in a series of
// the given name and returns that name.
func filepathSeries(w *rptWorld, series string) string {
	w.candidateWithDispositions(series)
	return series
}

// AC-1: a long reject evidence is shortened like a claim: its beginning is
// on one line, its end is not.
func TestRepeatFindingsAC1_LongTextShortened(t *testing.T) {
	const id = "long-disposition-not-shortened"
	w := newRptWorld(t, false)
	packet := w.fixesPacket(filepathSeries(w, "l"))
	line := rptEarlierLine(packet, "01-candidate", "cmd-F2")
	switch {
	case line == "":
		pdSetup(t, false, "the fixes packet lists no cmd-F2")
	case !strings.Contains(line, "근거-시작"):
		t.Errorf("jaekit:violation=%s the reject evidence is not on the line: %s", id, line)
	case strings.Contains(line, "근거-끝") || len([]rune(line)) > 1000:
		t.Errorf("jaekit:violation=%s the reject evidence is not shortened (%d characters)", id, len([]rune(line)))
	}
}

// AC-1: a finding without a recorded disposition keeps the line it had.
func TestRepeatFindingsAC1_UntriagedLineUnchanged(t *testing.T) {
	const id = "untriaged-line-changed"
	w := newRptWorld(t, false)
	packet := w.fixesPacket(filepathSeries(w, "u"))
	want := "- 01-candidate cmd-F7 [defect] at head/app/main.py:1: claim seven"
	if line := rptEarlierLine(packet, "01-candidate", "cmd-F7"); line != want {
		t.Errorf("jaekit:violation=%s the line of an open finding is %q, not %q", id, line, want)
	}
}

// AC-1: a disposition recorded between a round and its rerun makes another
// packet, so the run is not a rerun of the same packet.
func TestRepeatFindingsAC1_RerunAfterTriage(t *testing.T) {
	const id = "rerun-ignores-disposition"
	w := newRptWorld(t, false)
	c := w.commits
	w.useConfig(w.printer(w.answer("r-candidate", c[1],
		rptFinding("F1", "defect", "AC-1", "first"), rptFinding("F2", "defect", "AC-1", "second"))))
	cand := filepath.Join(w.repo, "reviews", "r", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	w.triage(cand, "cmd-F1", "reject", "--evidence", "first is wrong")
	w.useConfig(pdCommand("cmd", w.script("fail", "if [ \"$1\" = --version ]; then exit 0; fi\nexit 3\n")))
	w.run(w.round("fixes", c[1], c[2], filepath.Join(w.repo, "reviews", "r", "02-fixes"))...)
	w.triage(cand, "cmd-F2", "reject", "--evidence", "second is wrong too")
	again := filepath.Join(w.repo, "reviews", "r", "03-fixes")
	code, stderr := w.spar(append(w.round("fixes", c[1], c[2], again), "--reviewers", "cmd")...)
	if code != ExitOK && code != ExitNoResult {
		if _, err := os.Stat(again); err == nil {
			pdSetup(t, false, "spar run exits %d and wrote %s: %s", code, again, stderr)
		}
		return // refused without a round: not a rerun of the same packet
	}
	r := w.rawRound(again)
	if r["counted"] == false && !reflect.DeepEqual(r["rerun_of"], []any{}) && r["rerun_of"] != nil {
		t.Errorf("jaekit:violation=%s after a new disposition the run is a rerun of %v, not counted", id, r["rerun_of"])
	}
}

// AC-2: a packet whose earlier findings have a disposition tells reviewers
// to report such a finding again only with a failure scenario the record
// does not answer, in a fixes and a plan-delta round.
func TestRepeatFindingsAC2_RereportGuidance(t *testing.T) {
	const id = "rereport-guidance-missing"
	w := newRptWorld(t, false)
	if packet := w.fixesPacket(filepathSeries(w, "g")); !strings.Contains(packet, rptRereport) {
		t.Errorf("jaekit:violation=%s the fixes packet does not say %q", id, rptRereport)
	}
	c := w.commits
	plan := filepath.Join(w.repo, "docs", "PLAN.md")
	w.useConfig(w.printer(w.answer("plan", c[1], rptFinding("F1", "defect", "AC-1", "the plan misses AC-1"))))
	out := filepath.Join(w.repo, "reviews", "gp", "01-plan")
	w.run(append(w.round("plan", c[0], c[1], out), "--subject", plan)...)
	w.triage(out, "cmd-F1", "reject", "--evidence", "it does not")
	delta := filepath.Join(w.repo, "reviews", "gp", "02-plan-delta")
	w.run(append(w.round("plan-delta", c[0], c[2], delta), "--subject", plan, "--dry-run")...)
	if !strings.Contains(w.read(filepath.Join(delta, "packet.md")), rptRereport) {
		t.Errorf("jaekit:violation=%s the plan-delta packet does not say %q", id, rptRereport)
	}
}

// AC-2: the fixes stage line of such a packet separates findings recorded as
// fixed from those recorded as not fixed.
func TestRepeatFindingsAC2_FixesStageDistinguishes(t *testing.T) {
	const id = "fixes-stage-not-distinguished"
	w := newRptWorld(t, false)
	packet := w.fixesPacket(filepathSeries(w, "s"))
	if !strings.Contains(packet, rptFixed) || !strings.Contains(packet, rptNotFixed) {
		t.Errorf("jaekit:violation=%s the fixes stage line does not separate %q from %q", id, rptFixed, rptNotFixed)
	}
}

// rptBoundaries writes two boundary documents with a table, a code fence and
// a heading of their own, and returns their paths.
func (w *rptWorld) rptBoundaries() (string, string) {
	one := w.write(filepath.Join(w.repo, "docs", "B1.md"),
		"## 경계 하나\n\n| 검사 | 보는 것 |\n| --- | --- |\n| AC-1 | 첫 줄 |\n\n```text\n코드 블록\n```\n")
	two := w.write(filepath.Join(w.repo, "docs", "B2.md"), "## 경계 둘\n\n둘째 문서의 내용.\n")
	return one, two
}

// rptStages runs a dry round of every stage with the two boundary documents
// and returns each packet by stage.
func (w *rptWorld) rptStages(series string, extra ...string) map[string]string {
	w.t.Helper()
	c := w.commits
	plan := filepath.Join(w.repo, "docs", "PLAN.md")
	w.useConfig(w.printer(w.answer(series, c[1])))
	runs := []struct {
		stage, base, head string
		extra             []string
	}{
		{"plan", c[0], c[1], []string{"--subject", plan}},
		{"plan-delta", c[0], c[1], []string{"--subject", plan}},
		{"candidate", c[0], c[1], nil},
		{"fixes", c[1], c[2], nil},
	}
	packets := map[string]string{}
	for i, r := range runs {
		out := filepath.Join(w.repo, "reviews", series, fmt.Sprintf("%02d-%s", i+1, r.stage))
		args := append(append(append(w.round(r.stage, r.base, r.head, out), r.extra...), "--dry-run"), extra...)
		code, stderr := w.spar(args...)
		if code == ExitUsage && strings.Contains(stderr, "boundary") {
			return nil // spar does not take boundary documents
		}
		if code != ExitOK && code != ExitNoResult {
			pdSetup(w.t, w.keeps, "spar run --stage %s exits %d: %s", r.stage, code, stderr)
		}
		packets[r.stage] = w.read(filepath.Join(out, "packet.md"))
	}
	return packets
}

// AC-3: a round of any stage that receives boundary documents has one
// section with each document's name and text in the order given.
func TestRepeatFindingsAC3_BoundarySection(t *testing.T) {
	const id = "boundary-section-missing"
	w := newRptWorld(t, false)
	one, two := w.rptBoundaries()
	packets := w.rptStages("b", "--boundary", one, "--boundary", two)
	if packets == nil {
		t.Errorf("jaekit:violation=%s spar run does not take --boundary", id)
		return
	}
	for _, stage := range Stages {
		packet := packets[stage]
		if n := pdCount(pdHeadings(packet), rptBoundaryHeading); n != 1 {
			t.Errorf("jaekit:violation=%s the %s packet has %d %q sections, want 1", id, stage, n, rptBoundaryHeading)
			continue
		}
		section := pdSectionOf(packet, rptBoundaryHeading)
		i, j := strings.Index(section, "### docs/B1.md"), strings.Index(section, "### docs/B2.md")
		if i < 0 || j < 0 || i > j || !strings.Contains(section, w.read(one)) || !strings.Contains(section, w.read(two)) {
			t.Errorf("jaekit:violation=%s the %s packet's boundary section does not hold docs/B1.md and docs/B2.md in order", id, stage)
		}
	}
}

// AC-3: the boundary section tells reviewers how to report a counterexample
// outside a boundary and one on an input a requirement needs.
func TestRepeatFindingsAC3_BoundaryGuidance(t *testing.T) {
	const id = "boundary-guidance-missing"
	w := newRptWorld(t, false)
	one, two := w.rptBoundaries()
	packets := w.rptStages("bg", "--boundary", one, "--boundary", two)
	if packets == nil {
		t.Errorf("jaekit:violation=%s spar run does not take --boundary", id)
		return
	}
	section := pdSectionOf(packets["candidate"], rptBoundaryHeading)
	guidance := section
	if k := strings.Index(section, "### "); k >= 0 {
		guidance = section[:k]
	}
	for _, token := range []string{"`hardening`", "`check_impact`", "`none`", "`defect`", "protected"} {
		if !strings.Contains(guidance, token) {
			t.Errorf("jaekit:violation=%s the boundary guidance does not name %s", id, token)
		}
	}
}

// AC-3: round.json keeps the boundary documents the round received.
func TestRepeatFindingsAC3_BoundaryRecorded(t *testing.T) {
	const id = "boundary-not-recorded"
	w := newRptWorld(t, false)
	one, two := w.rptBoundaries()
	if w.rptStages("br", "--boundary", one, "--boundary", two) == nil {
		t.Errorf("jaekit:violation=%s spar run does not take --boundary", id)
		return
	}
	r := w.rawRound(filepath.Join(w.repo, "reviews", "br", "03-candidate"))
	list, _ := r["boundaries"].([]any)
	var names, sums []string
	for _, item := range list {
		m, _ := item.(map[string]any)
		name, _ := m["name"].(string)
		sum, _ := m["sha256"].(string)
		names, sums = append(names, name), append(sums, sum)
	}
	want := func(path string) string {
		s := sha256.Sum256([]byte(w.read(path)))
		return hex.EncodeToString(s[:])
	}
	if !reflect.DeepEqual(names, []string{"docs/B1.md", "docs/B2.md"}) || !reflect.DeepEqual(sums, []string{want(one), want(two)}) {
		t.Errorf("jaekit:violation=%s round.json keeps boundaries %v %v", id, names, sums)
	}
}

// AC-4: a boundary document that cannot be read is refused with 65 before a
// reviewer starts, and no round folder is left. The forms are those
// --criteria refuses: a missing file, a folder and a file without read
// permission.
func TestRepeatFindingsAC4_UnreadableBoundaryRefused(t *testing.T) {
	const id = "unreadable-boundary-accepted"
	w := newRptWorld(t, false)
	started := filepath.Join(w.dir, "started")
	w.useConfig(pdCommand("cmd", w.script("mark", "if [ \"$1\" = --version ]; then exit 0; fi\ntouch "+amQuote(started)+"\nexit 3\n")))
	locked := w.write(filepath.Join(w.repo, "docs", "locked.md"), "locked\n")
	forms := map[string]string{"missing": filepath.Join(w.repo, "docs", "none.md"), "folder": filepath.Join(w.repo, "docs")}
	if os.Geteuid() != 0 && os.Chmod(locked, 0) == nil {
		forms["unreadable"] = locked
		defer os.Chmod(locked, 0o644)
	}
	for name, path := range forms {
		out := filepath.Join(w.repo, "reviews", "u-"+name, "01-candidate")
		code, stderr := w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), "--boundary", path)...)
		_, statErr := os.Stat(out)
		if code != ExitRefused || !strings.Contains(stderr, "boundary") || statErr == nil {
			t.Errorf("jaekit:violation=%s a %s boundary document: exit %d, round folder left %v: %s", id, name, code, statErr == nil, pdFirstLine(stderr))
		}
	}
	if _, err := os.Stat(started); err == nil {
		t.Errorf("jaekit:violation=%s a reviewer started for a refused boundary document", id)
	}
}

// AC-4: the input table of the Spar input contract, in both languages, has a
// row for --boundary that names its form, the result outside it and where it
// goes.
func TestRepeatFindingsAC4_InputTableRow(t *testing.T) {
	const id = "boundary-input-row-missing"
	for path, tokens := range map[string][]string{
		"../../contracts/spar-input.md":    {"패킷", "`round.json`"},
		"../../contracts/spar-input.en.md": {"packet", "`round.json`"},
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			pdSetup(t, false, "%v", err)
		}
		var row string
		for _, line := range strings.Split(string(data), "\n") {
			if strings.HasPrefix(line, "| G") && strings.Contains(line, "`--boundary`") {
				row = line
			}
		}
		if row == "" {
			t.Errorf("jaekit:violation=%s %s has no input row for `--boundary`", id, path)
			continue
		}
		if cells := strings.Split(row, "|"); len(cells) < 6 {
			t.Errorf("jaekit:violation=%s %s's `--boundary` row does not have the four columns", id, path)
		}
		for _, token := range tokens {
			if !strings.Contains(row, token) {
				t.Errorf("jaekit:violation=%s %s's `--boundary` row does not name %s", id, path, token)
			}
		}
	}
}

// AC-5: in a fixes round with dispositions and boundary documents, every
// reviewer receives the same packet: claude and codex on standard input, agy
// after its command note as an argument and as PACKET.md, native in
// prompt.md and a command reviewer at its {prompt} path.
func TestRepeatFindingsAC5_EveryReviewerGetsSamePacket(t *testing.T) {
	const id = "reviewer-packet-differs"
	w := newRptWorld(t, false)
	w.candidateWithDispositions("all")
	one, two := w.rptBoundaries()
	head := w.commits[2]
	cmd := w.script("command", "if [ \"$1\" = --version ]; then exit 0; fi\ncat \"$1\" > "+
		amQuote(filepath.Join(w.dir, "got", "command"))+"\ncat "+amQuote(w.answer("command", head))+"\n")
	w.useConfig(w.pdFake("claude", head), w.pdFake("codex", head), w.pdFake("agy", head),
		"[[reviewers]]\nname = \"native\"\npreset = \"native\"\n\n", pdCommand("command", cmd, "{prompt}"))
	out := filepath.Join(w.repo, "reviews", "all", "02-fixes")
	code, stderr := w.spar(append(w.round("fixes", w.commits[1], head, out), "--boundary", one, "--boundary", two)...)
	if code == ExitUsage && strings.Contains(stderr, "boundary") {
		t.Errorf("jaekit:violation=%s spar run does not take --boundary", id)
		return
	}
	if code != ExitOK && code != ExitNoResult {
		pdSetup(t, false, "spar run exits %d: %s", code, stderr)
	}
	packet := w.read(filepath.Join(out, "packet.md"))
	if !strings.Contains(packet, rptBoundaryHeading) || !strings.Contains(packet, rptRereport) {
		t.Errorf("jaekit:violation=%s packet.md has no boundary section or disposition guidance", id)
	}
	for _, name := range []string{"claude", "codex", "command"} {
		if w.got(name) != packet {
			t.Errorf("jaekit:violation=%s %s did not receive packet.md as it is", id, name)
		}
	}
	if w.promptArg() != agyCommandNote+packet {
		t.Errorf("jaekit:violation=%s agy's -p value is not its command note followed by packet.md", id)
	}
	if prompt := w.read(filepath.Join(out, "native", "prompt.md")); !strings.HasSuffix(prompt, packet) {
		t.Errorf("jaekit:violation=%s native/prompt.md does not end with packet.md", id)
	}

	// A packet over the argument limit reaches agy as PACKET.md.
	if err := os.Remove(filepath.Join(w.dir, "got", "agy")); err != nil && !os.IsNotExist(err) {
		pdSetup(t, false, "%v", err)
	}
	big := w.write(filepath.Join(w.repo, "docs", "BIG.md"), strings.Repeat("- a line that makes the packet large\n", argumentLimit()/36+64))
	w.useConfig(w.pdFake("agy", head))
	out = filepath.Join(w.repo, "reviews", "big", "01-candidate")
	code, stderr = w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), "--boundary", one, "--boundary", big)...)
	if code != ExitOK && code != ExitNoResult {
		pdSetup(t, false, "spar run exits %d: %s", code, stderr)
	}
	packet = w.read(filepath.Join(out, "packet.md"))
	if p := w.promptArg(); !strings.HasPrefix(p, agyCommandNote) || strings.Contains(p, rptBoundaryHeading) {
		pdSetup(t, false, "the large packet did not go to agy through %s", packetFile)
	}
	if !strings.Contains(packet, rptBoundaryHeading) || w.got("agy") != packet {
		t.Errorf("jaekit:violation=%s the PACKET.md agy received is not packet.md with its boundary section", id)
	}
}

// rptConfigReviewers is the reviewers list that spar config prints. A
// configuration that spar config refuses for its max_packet_bytes setting is
// the observation that spar does not take the setting: it fails with id and
// returns nil.
func (w *rptWorld) rptConfigReviewers(id string) []map[string]any {
	w.t.Helper()
	code, stdout, stderr := w.main("config", "--repo", w.repo)
	if code != ExitOK && strings.Contains(stderr, "max_packet_bytes") {
		w.t.Errorf("jaekit:violation=%s spar config does not take max_packet_bytes: %s", id, pdFirstLine(stderr))
		return nil
	}
	if code != ExitOK {
		pdSetup(w.t, w.keeps, "spar config exits %d: %s", code, stderr)
	}
	var cfg struct {
		Reviewers []map[string]any `json:"reviewers"`
	}
	if err := json.Unmarshal([]byte(stdout), &cfg); err != nil {
		pdSetup(w.t, w.keeps, "spar config output: %v", err)
	}
	return cfg.Reviewers
}

func rptLimit(r map[string]any) int {
	v, _ := r["max_packet_bytes"].(float64)
	return int(v)
}

// rptSize is the byte size of the packet of a dry candidate round, written
// in a series of its own with the current configuration and extra
// arguments.
func (w *rptWorld) rptSize(series string, extra ...string) int {
	w.t.Helper()
	out := filepath.Join(w.repo, "reviews", series, "01-candidate")
	w.run(append(append(w.round("candidate", w.commits[0], w.commits[1], out), "--dry-run"), extra...)...)
	return len(w.read(filepath.Join(out, "packet.md")))
}

// rptRefused reports whether a run was refused for the packet size: exit 65,
// no round folder, and a message that names the reviewer, its limit and a
// packet size over that limit.
func (w *rptWorld) rptRefused(code int, stderr, out string, reviewer string, limit int) bool {
	_, err := os.Stat(out)
	over := false
	for _, word := range strings.FieldsFunc(stderr, func(r rune) bool { return r < '0' || r > '9' }) {
		if n, err := strconv.Atoi(word); err == nil && n > limit {
			over = true
		}
	}
	return code == ExitRefused && os.IsNotExist(err) && over &&
		strings.Contains(stderr, reviewer) && strings.Contains(stderr, strconv.Itoa(limit))
}

// AC-6: a packet over a reviewer's limit is refused with 65 before any
// reviewer starts, in a dry run and in a real round, and leaves no round
// folder, so the code round count stays; at the limit the round runs.
func TestRepeatFindingsAC6_OversizeRefused(t *testing.T) {
	const id = "oversize-packet-admitted"
	w := newRptWorld(t, false)
	started := filepath.Join(w.dir, "started")
	mark := w.script("mark", "if [ \"$1\" = --version ]; then exit 0; fi\ntouch "+amQuote(started)+"\nexit 3\n")
	w.useConfig(pdCommand("cmd", mark))
	size := w.rptSize("measure")
	w.useConfig(pdCommand("cmd", mark) + fmt.Sprintf("max_packet_bytes = %d\n", size-1))
	for _, dry := range []bool{true, false} {
		out := filepath.Join(w.repo, "reviews", "o"+strconv.FormatBool(dry), "01-candidate")
		args := w.round("candidate", w.commits[0], w.commits[1], out)
		if dry {
			args = append(args, "--dry-run")
		}
		code, stderr := w.spar(args...)
		if !w.rptRefused(code, stderr, out, "cmd", size-1) {
			t.Errorf("jaekit:violation=%s a %d-byte packet over the limit %d (dry run %v): exit %d: %s", id, size, size-1, dry, code, pdFirstLine(stderr))
		}
	}
	if _, err := os.Stat(started); err == nil {
		t.Errorf("jaekit:violation=%s a reviewer started for a packet over its limit", id)
	}
	w.useConfig(pdCommand("cmd", mark) + fmt.Sprintf("max_packet_bytes = %d\n", size))
	out := filepath.Join(w.repo, "reviews", "at", "01-candidate")
	if code, stderr := w.spar(w.round("candidate", w.commits[0], w.commits[1], out)...); code != ExitOK && code != ExitNoResult {
		t.Errorf("jaekit:violation=%s a packet at its limit was not run: exit %d: %s", id, code, pdFirstLine(stderr))
	}
}

// AC-6: claude, codex and agy have a limit without any setting; native and
// command have none; a packet over the claude default is refused.
func TestRepeatFindingsAC6_PresetDefaults(t *testing.T) {
	const id = "preset-limit-missing"
	w := newRptWorld(t, false)
	head := w.commits[1]
	w.useConfig(w.pdFake("claude", head), w.pdFake("codex", head), w.pdFake("agy", head),
		"[[reviewers]]\nname = \"native\"\npreset = \"native\"\n\n", pdCommand("cmd", w.script("ok", "exit 0\n")))
	limits := map[string]int{}
	for _, r := range w.rptConfigReviewers(id) {
		name, _ := r["name"].(string)
		limits[name] = rptLimit(r)
	}
	for _, name := range []string{"claude", "codex", "agy"} {
		if limits[name] <= 0 {
			t.Errorf("jaekit:violation=%s %s has no default packet limit", id, name)
		}
	}
	for _, name := range []string{"native", "cmd"} {
		if limits[name] != 0 {
			t.Errorf("jaekit:violation=%s %s has a packet limit %d without a setting", id, name, limits[name])
		}
	}
	if limits["claude"] <= 0 {
		return
	}
	big := w.write(filepath.Join(w.repo, "docs", "BIG.md"), strings.Repeat("x", limits["claude"]+1))
	w.useConfig(w.pdFake("claude", head))
	out := filepath.Join(w.repo, "reviews", "big", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], head, out), "--criteria", big, "--dry-run")...)
	if _, err := os.Stat(out); code != ExitRefused || err == nil || !strings.Contains(stderr, strconv.Itoa(limits["claude"])) {
		t.Errorf("jaekit:violation=%s a packet over the claude default %d was not refused: exit %d: %s", id, limits["claude"], code, pdFirstLine(stderr))
	}
}

// AC-6: the user's setting changes one reviewer's limit, larger or smaller
// than the default.
func TestRepeatFindingsAC6_UserLimit(t *testing.T) {
	const id = "user-limit-ignored"
	w := newRptWorld(t, false)
	head := w.commits[1]
	w.useConfig(w.pdFake("claude", head))
	size := w.rptSize("measure")
	w.useConfig(w.pdFake("claude", head) + fmt.Sprintf("max_packet_bytes = %d\n", size-1))
	out := filepath.Join(w.repo, "reviews", "small", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], head, out), "--dry-run")...)
	if !w.rptRefused(code, stderr, out, "claude", size-1) {
		t.Errorf("jaekit:violation=%s the user's smaller limit %d did not refuse a %d-byte packet: exit %d: %s", id, size-1, size, code, pdFirstLine(stderr))
	}
	w.useConfig(w.pdFake("claude", head) + "max_packet_bytes = 50000000\n")
	for _, r := range w.rptConfigReviewers(id) {
		if r["name"] == "claude" && rptLimit(r) != 50000000 {
			t.Errorf("jaekit:violation=%s the user's larger limit shows as %d", id, rptLimit(r))
		}
	}
}

// AC-6: a limit that is not a positive integer is a configuration error
// that says so, not one that only names an unknown setting.
func TestRepeatFindingsAC6_BadLimitRefused(t *testing.T) {
	const id = "bad-limit-accepted"
	w := newRptWorld(t, false)
	for _, value := range []string{"0", "-1", "\"many\"", "1.5"} {
		w.useConfig(pdCommand("cmd", w.script("ok", "exit 0\n")) + "max_packet_bytes = " + value + "\n")
		code, _, stderr := w.main("config", "--repo", w.repo)
		if code == ExitOK || !strings.Contains(stderr, "max_packet_bytes") || !strings.Contains(stderr, "positive") {
			t.Errorf("jaekit:violation=%s max_packet_bytes = %s: spar config exits %d: %s", id, value, code, pdFirstLine(stderr))
		}
	}
}

// AC-6: spar config shows each reviewer's limit and where it comes from.
func TestRepeatFindingsAC6_ConfigShowsLimit(t *testing.T) {
	const id = "config-limit-not-shown"
	w := newRptWorld(t, false)
	head := w.commits[1]
	w.useConfig(w.pdFake("claude", head), w.pdFake("codex", head)+"max_packet_bytes = 123456\n")
	sources := map[string]string{}
	limits := map[string]int{}
	reviewers := w.rptConfigReviewers(id)
	if reviewers == nil {
		return
	}
	for _, r := range reviewers {
		name, _ := r["name"].(string)
		sources[name], _ = r["max_packet_bytes_source"].(string)
		limits[name] = rptLimit(r)
	}
	if sources["claude"] != "preset" || limits["claude"] <= 0 {
		t.Errorf("jaekit:violation=%s claude's limit shows as %d from %q, want the preset's", id, limits["claude"], sources["claude"])
	}
	if sources["codex"] != "user" || limits["codex"] != 123456 {
		t.Errorf("jaekit:violation=%s codex's limit shows as %d from %q, want 123456 from the user", id, limits["codex"], sources["codex"])
	}
}

// rptSealExample is the spar run command of the Seal reference's example for
// review rounds, split into arguments as a shell reads them.
func rptSealExample(t *testing.T) []string {
	data, err := os.ReadFile("../../plugins/seal/skills/seal/references/bundle.md")
	if err != nil {
		pdSetup(t, false, "%v", err)
	}
	text := string(data)
	i := strings.Index(text, "**Decisions for review rounds.**")
	if i < 0 {
		pdSetup(t, false, "the Seal reference has no example for review rounds")
	}
	j := strings.Index(text[i:], "```bash\n")
	if j < 0 {
		pdSetup(t, false, "the example for review rounds has no bash block")
	}
	block := text[i+j+len("```bash\n"):]
	block = block[:strings.Index(block, "```")]
	block = strings.ReplaceAll(block, "\\\n", " ")
	var args []string
	var cur strings.Builder
	quoted, inWord := false, false
	for _, r := range block {
		switch {
		case r == '\'':
			quoted, inWord = !quoted, true
		case !quoted && (r == ' ' || r == '\n' || r == '\t'):
			if inWord {
				args = append(args, cur.String())
				cur.Reset()
				inWord = false
			}
		default:
			cur.WriteRune(r)
			inWord = true
		}
	}
	if inWord {
		args = append(args, cur.String())
	}
	if len(args) < 2 || args[0] != "spar" || args[1] != "run" {
		pdSetup(t, false, "the example does not start with spar run: %q", args)
	}
	return args[2:]
}

// AC-8: the round example of the Seal reference, given to spar as it is with
// its placeholders filled, makes a round whose packet has the boundary
// section with the goal's REVIEW.md.
func TestRepeatFindingsAC8_SealExampleRuns(t *testing.T) {
	const id = "seal-example-without-boundary"
	w := newRptWorld(t, false)
	goal := filepath.Join(w.repo, "docs", "specs", "demo")
	w.write(filepath.Join(goal, "SPEC.md"), "# Demo\n\n- **AC-1** one\n")
	review := w.write(filepath.Join(goal, "REVIEW.md"), "# REVIEW\n\n## 독립 검토\n\n| 검사 | 경계 |\n| --- | --- |\n| AC-1 | 첫 줄만 |\n")
	w.useConfig(w.printer(w.answer("example", w.commits[1])))
	var args []string
	out := ""
	for _, a := range rptSealExample(t) {
		switch {
		case a == "<goal base commit>":
			a = w.commits[0]
		case a == "<head>":
			a = w.commits[1]
		case strings.HasPrefix(a, "docs/specs/<goal>/"):
			a = filepath.Join(goal, strings.TrimPrefix(a, "docs/specs/<goal>/"))
		case strings.HasPrefix(a, "docs/reviews/<goal>/"):
			a = filepath.Join(w.repo, "reviews", "demo", strings.ReplaceAll(strings.TrimPrefix(a, "docs/reviews/<goal>/"), "<NN>", "01"))
			out = a
		case a == "AGENTS.md":
			a = filepath.Join(w.repo, "AGENTS.md")
		}
		args = append(args, a)
	}
	if out == "" {
		pdSetup(t, false, "the example has no --out under docs/reviews/<goal>/")
	}
	code, stderr := w.spar(append(args, "--repo", w.repo, "--dry-run")...)
	if code != ExitOK && code != ExitNoResult {
		t.Errorf("jaekit:violation=%s the Seal example was refused: exit %d: %s", id, code, pdFirstLine(stderr))
		return
	}
	section := pdSectionOf(w.read(filepath.Join(out, "packet.md")), rptBoundaryHeading)
	if section == "" || !strings.Contains(section, w.read(review)) {
		t.Errorf("jaekit:violation=%s the Seal example's packet has no boundary section with the goal's REVIEW.md", id)
	}
}

// AC-8: a finding rejected with spar triage after a candidate round shows
// its reject and evidence in the next fixes round's packet.
func TestRepeatFindingsAC8_TriageThenFixes(t *testing.T) {
	const id = "fixes-packet-without-reject"
	w := newRptWorld(t, false)
	c := w.commits
	w.useConfig(w.printer(w.answer("t-candidate", c[1], rptFinding("F1", "defect", "AC-1", "the greeting changed"))))
	cand := filepath.Join(w.repo, "reviews", "t", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	w.triage(cand, "cmd-F1", "reject", "--evidence", "AC-1이 인사말을 바꾸라고 한다")
	line := rptEarlierLine(w.fixesPacket("t"), "01-candidate", "cmd-F1")
	if line == "" {
		pdSetup(t, false, "the fixes packet lists no cmd-F1")
	}
	if !strings.Contains(line, "reject") || !strings.Contains(line, "AC-1이 인사말을 바꾸라고 한다") {
		t.Errorf("jaekit:violation=%s the fixes packet does not show the reject and its evidence: %s", id, line)
	}
}

// AC-10: packets without boundary documents or dispositions stay as the
// earlier goals' fixed packets have them.
func TestRepeatFindingsAC10_FixedPackets(t *testing.T) {
	t.Run("decisions", TestSparPacketDecisionsAC6_NoDecisionPacketUnchanged)
	t.Run("standpoints", TestReviewerStandpointsAC8_NoStandpointUnchanged)
}

// AC-10: a fixes packet whose earlier finding has no disposition keeps the
// earlier line, the earlier stage line and no new guidance or section.
func TestRepeatFindingsAC10_OpenFindingPacket(t *testing.T) {
	w := newRptWorld(t, true)
	c := w.commits
	w.useConfig(w.printer(w.answer("o-candidate", c[1], rptFinding("F1", "defect", "AC-1", "the greeting changed"))))
	w.run(w.round("candidate", c[0], c[1], filepath.Join(w.repo, "reviews", "o", "01-candidate"))...)
	packet := w.fixesPacket("o")
	if line := rptEarlierLine(packet, "01-candidate", "cmd-F1"); line != "- 01-candidate cmd-F1 [defect] at head/app/main.py:1: the greeting changed" {
		t.Errorf("the line of an open finding changed: %q", line)
	}
	if !strings.Contains(packet, rptOldFixes) {
		t.Errorf("the fixes stage line changed")
	}
	for _, s := range []string{rptRereport, rptFixed, rptBoundaryHeading} {
		if strings.Contains(packet, s) {
			t.Errorf("a packet without dispositions or boundaries holds %q", s)
		}
	}
}

// AC-11: a round without boundary documents writes no boundary key, and a
// fixes round reads such a round as one without boundaries.
func TestRepeatFindingsAC11_RoundWithoutBoundaries(t *testing.T) {
	w := newRptWorld(t, true)
	c := w.commits
	w.useConfig(w.printer(w.answer("n-candidate", c[1])))
	cand := filepath.Join(w.repo, "reviews", "n", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	if _, ok := w.rawRound(cand)["boundaries"]; ok {
		t.Errorf("a round without boundary documents records a boundaries key")
	}
	if strings.Contains(w.fixesPacket("n"), rptBoundaryHeading) {
		t.Errorf("a fixes round after a round without boundaries has a boundary section")
	}
}

// AC-12: Core reads a series with boundary documents and dispositions as it
// reads the same series without them.
func TestRepeatFindingsAC12_CoreReadsNewRecords(t *testing.T) {
	w := newRptWorld(t, true)
	one, two := w.rptBoundaries()
	c := w.commits
	read := func(series string, extra ...string) *reviewseries.Series {
		w.useConfig(w.printer(w.answer(series, c[1], rptFinding("F1", "defect", "AC-1", "the greeting changed"))))
		cand := filepath.Join(w.repo, "reviews", series, "01-candidate")
		w.run(append(w.round("candidate", c[0], c[1], cand), extra...)...)
		w.triage(cand, "cmd-F1", "reject", "--evidence", "AC-1이 인사말을 바꾸라고 한다")
		w.useConfig(w.printer(w.answer(series+"-fixes", c[2])))
		w.run(append(w.round("fixes", c[1], c[2], filepath.Join(w.repo, "reviews", series, "02-fixes")), extra...)...)
		s, err := reviewseries.Read(filepath.Join(w.repo, "reviews", series))
		if err != nil {
			t.Fatalf("Core cannot read the series %s: %v", series, err)
		}
		return s
	}
	with := read("with", "--boundary", one, "--boundary", two)
	without := read("without")
	if len(with.Rounds) != len(without.Rounds) || len(with.Skipped) != len(without.Skipped) {
		t.Fatalf("Core reads %d rounds and skips %d with boundaries, %d and %d without",
			len(with.Rounds), len(with.Skipped), len(without.Rounds), len(without.Skipped))
	}
	for i := range with.Rounds {
		a, b := with.Rounds[i], without.Rounds[i]
		if a.Stage != b.Stage || a.Sequence != b.Sequence || !reflect.DeepEqual(a.Counted, b.Counted) ||
			a.HasFindings != b.HasFindings || len(a.Reviewers) != len(b.Reviewers) {
			t.Errorf("round %s reads differently with boundaries: %+v, without: %+v", a.Name, a, b)
		}
	}
}

// AC-12: ha depends on no Spar package.
func TestRepeatFindingsAC12_HaApartFromSpar(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("go is not available: %v", err)
	}
	out, err := exec.Command("go", "list", "-deps", "github.com/jgoneit/jaekit/cmd/ha").Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "github.com/jgoneit/jaekit/internal/spar" || strings.HasPrefix(pkg, "github.com/jgoneit/jaekit/internal/spar/") ||
			pkg == "github.com/jgoneit/jaekit/cmd/spar" {
			t.Errorf("ha depends on %s", pkg)
		}
	}
}
