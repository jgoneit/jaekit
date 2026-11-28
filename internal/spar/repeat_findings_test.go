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

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/reviewgate"
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
	// rptCandidate is the stage text of a candidate packet, with or without
	// dispositions.
	rptCandidate = "The head is a candidate implementation of the criteria. Review the whole change from base to head."
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
		rptFinding("F8", "requirement-question", "AC-2", "claim eight"),
	}
	w.useConfig(w.printer(w.answer(series+"-candidate", c[1], findings...)))
	out := filepath.Join(w.repo, "reviews", series, "01-candidate")
	w.run(w.round("candidate", c[0], c[1], out)...)
	w.triage(out, "cmd-F1", "fix", "--commit", c[2], "--failed-before", "the greeting was wrong", "--passed-after", "it is right", "--note", "고친 메모")
	w.triage(out, "cmd-F2", "reject", "--evidence", rptLongEvidence, "--note", "반박 메모")
	w.triage(out, "cmd-F3", "defer", "--to", "#999", "--user-quote", "미뤄도 된다는 사용자의 말")
	w.triage(out, "cmd-F4", "duplicate", "--of", "cmd-F1")
	w.triage(out, "cmd-F5", "answer", "--user-quote", "사용자의 답 원문")
	w.triage(out, "cmd-F8", "reject", "--evidence", "   ", "--user-quote", "사용자가 거절한 말")
	return out
}

// nextPacket is the packet of the round after review fixes, a candidate dry
// run from the base commit 0 to the new head 2, after the candidate round of
// series from commit 0 to 1.
func (w *rptWorld) nextPacket(series string, extra ...string) string {
	w.t.Helper()
	out := filepath.Join(w.repo, "reviews", series, "02-candidate")
	w.run(append(append(w.round("candidate", w.commits[0], w.commits[2], out), "--dry-run"), extra...)...)
	return w.read(filepath.Join(out, "packet.md"))
}

// fixesRefused fails the test unless a fixes dry run from commit 1 to 2,
// the round that came after review fixes before candidate rounds took its
// place, is refused in series before anything is written.
func (w *rptWorld) fixesRefused(series string) {
	w.t.Helper()
	w.oldStageRefused("fixes", w.commits[1], w.commits[2], filepath.Join(w.repo, "reviews", series, "03-fixes"), "--dry-run")
}

// planDeltaRefused fails the test unless a plan-delta dry run of the plan
// from commit 0 to 2 is refused in series before anything is written.
func (w *rptWorld) planDeltaRefused(series string) {
	w.t.Helper()
	w.oldStageRefused("plan-delta", w.commits[0], w.commits[2], filepath.Join(w.repo, "reviews", series, "02-plan-delta"),
		"--subject", filepath.Join(w.repo, "docs", "PLAN.md"), "--dry-run")
}

// AC-1: each disposition and the automatic deferral show on the finding's
// line of the packet of the candidate round after review fixes. A
// plan-delta round, which showed the dispositions of the plan round, is
// refused.
func TestRepeatFindingsAC1_DispositionShown(t *testing.T) {
	const id = "disposition-not-shown"
	w := newRptWorld(t, false)
	packet := w.nextPacket(filepathSeries(w, "d"))
	for _, want := range []struct{ finding, disposition, detail string }{
		{"cmd-F1", "fix", shortID(w.commits[2])},
		{"cmd-F2", "reject", "근거-시작"},
		{"cmd-F3", "defer", "#999"},
		{"cmd-F4", "duplicate", "cmd-F1"},
		{"cmd-F5", "answer", "사용자의 답 원문"},
		{"cmd-F6", "defer", "auto"},
		{"cmd-F8", "reject", "사용자가 거절한 말"},
	} {
		line := rptEarlierLine(packet, "01-candidate", want.finding)
		if line == "" {
			pdSetup(t, false, "the candidate packet lists no %s", want.finding)
		}
		after := strings.SplitN(line, ": ", 2)
		if len(after) < 2 || !strings.Contains(after[1], want.disposition) || !strings.Contains(after[1], want.detail) {
			t.Errorf("jaekit:violation=%s the line of %s does not show %s with %q: %s", id, want.finding, want.disposition, want.detail, line)
		}
	}

	// The automatic deferral shows the reason spar recorded with it, not only
	// its mark.
	var auto struct {
		Reason string `json:"reason"`
	}
	for _, l := range strings.Split(w.read(filepath.Join(w.repo, "reviews", "d", "01-candidate", "ledger.jsonl")), "\n") {
		if strings.Contains(l, `"finding":"cmd-F6"`) && json.Unmarshal([]byte(l), &auto) == nil {
			break
		}
	}
	if reason := []rune(auto.Reason); len(reason) < 20 {
		pdSetup(t, false, "the candidate's ledger has no reason for the automatic deferral of cmd-F6")
	} else if line := rptEarlierLine(packet, "01-candidate", "cmd-F6"); !strings.Contains(line, string(reason[:20])) {
		t.Errorf("jaekit:violation=%s the automatic deferral does not show its reason %q: %s", id, auto.Reason, line)
	}

	// Those details are all the line shows: not the fix's before and after
	// texts, nor a note recorded with a disposition, nor the user's words
	// recorded with a deferral to a target. A deferral, automatic or not,
	// shows its target or reason, as AC-1 lists for defer.
	for finding, extra := range map[string][]string{
		"cmd-F1": {"the greeting was wrong", "it is right", "고친 메모"},
		"cmd-F2": {"반박 메모"},
		"cmd-F3": {"미뤄도 된다는 사용자의 말"},
	} {
		line := rptEarlierLine(packet, "01-candidate", finding)
		for _, text := range extra {
			if strings.Contains(line, text) {
				t.Errorf("jaekit:violation=%s the line of %s shows %q beyond its disposition: %s", id, finding, text, line)
			}
		}
	}

	// A plan-delta round after a plan round with a disposition is refused.
	c := w.commits
	plan := filepath.Join(w.repo, "docs", "PLAN.md")
	w.useConfig(w.printer(w.answer("plan", c[1], rptFinding("F1", "defect", "AC-1", "the plan misses AC-1"))))
	out := filepath.Join(w.repo, "reviews", "p", "01-plan")
	w.run(append(w.round("plan", c[0], c[1], out), "--subject", plan)...)
	w.triage(out, "cmd-F1", "reject", "--evidence", "계획의 둘째 문단이 AC-1을 다룬다")
	w.planDeltaRefused("p")
}

// filepathSeries runs the candidate round with dispositions in a series of
// the given name and returns that name.
func filepathSeries(w *rptWorld, series string) string {
	w.candidateWithDispositions(series)
	return series
}

// AC-1: a long reject evidence and a long user quote are shortened as an
// earlier finding's claim is. For a text one character over 300 and a text
// of about 2,000 characters with a line break, the disposition's line ends
// with what the claim line shows for the same words, and no later part of
// either text is left among the earlier findings.
func TestRepeatFindingsAC1_LongTextShortened(t *testing.T) {
	const id = "long-disposition-not-shortened"
	w := newRptWorld(t, false)
	c := w.commits
	type pair struct{ claim, disposed, disposition, text string }
	var pairs []pair
	var findings []any
	for i, text := range []string{rptLongEvidence, strings.Repeat("가", 300) + "Ω"} {
		for j, disposition := range []string{"reject", "answer"} {
			class, link := "defect", "AC-1"
			if disposition == "answer" {
				class, link = "requirement-question", "AC-2"
			}
			n := 4*i + 2*j + 1
			findings = append(findings,
				rptFinding(fmt.Sprintf("F%d", n), "defect", "AC-1", text),
				rptFinding(fmt.Sprintf("F%d", n+1), class, link, fmt.Sprintf("claim %d", n+1)))
			pairs = append(pairs, pair{fmt.Sprintf("cmd-F%d", n), fmt.Sprintf("cmd-F%d", n+1), disposition, text})
		}
	}
	w.useConfig(w.printer(w.answer("l-candidate", c[1], findings...)))
	out := filepath.Join(w.repo, "reviews", "l", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], out)...)
	for _, p := range pairs {
		flag := "--evidence"
		if p.disposition == "answer" {
			flag = "--user-quote"
		}
		w.triage(out, p.disposed, p.disposition, flag, p.text)
	}
	packet := w.nextPacket("l")
	for _, p := range pairs {
		claimLine, line := rptEarlierLine(packet, "01-candidate", p.claim), rptEarlierLine(packet, "01-candidate", p.disposed)
		shown := strings.SplitN(claimLine, ": ", 2)
		if line == "" || len(shown) < 2 {
			pdSetup(t, false, "the candidate packet lists no %s or %s", p.claim, p.disposed)
		}
		if !strings.Contains(line, p.disposition) || !strings.HasSuffix(line, shown[1]) {
			t.Errorf("jaekit:violation=%s the %s text of %s (%d characters) is not shortened like the claim %q: %s",
				id, p.disposition, p.disposed, len([]rune(p.text)), shown[1], line)
		}
	}
	section := pdSectionOf(packet, rptEarlierHeading)
	for _, rest := range []string{"근거-끝", "Ω", "\n둘째 줄"} {
		if strings.Contains(section, rest) {
			t.Errorf("jaekit:violation=%s the earlier findings still hold %q of a long text", id, rest)
		}
	}
}

// AC-1: a finding without a recorded disposition keeps the line it had.
func TestRepeatFindingsAC1_UntriagedLineUnchanged(t *testing.T) {
	const id = "untriaged-line-changed"
	w := newRptWorld(t, false)
	packet := w.nextPacket(filepathSeries(w, "u"))
	want := "- 01-candidate cmd-F7 [defect] at head/app/main.py:1: claim seven"
	if line := rptEarlierLine(packet, "01-candidate", "cmd-F7"); line != want {
		t.Errorf("jaekit:violation=%s the line of an open finding is %q, not %q", id, line, want)
	}
}

// AC-1: a finding with two recorded dispositions shows only the last.
func TestRepeatFindingsAC1_LastDispositionShown(t *testing.T) {
	const id = "earlier-disposition-shown"
	w := newRptWorld(t, false)
	c := w.commits
	w.useConfig(w.printer(w.answer("last-candidate", c[1], rptFinding("F1", "defect", "AC-1", "claim one"))))
	out := filepath.Join(w.repo, "reviews", "last", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], out)...)
	w.triage(out, "cmd-F1", "reject", "--evidence", "처음 근거")
	w.triage(out, "cmd-F1", "defer", "--to", "#321", "--user-quote", "나중에 하자는 사용자의 말")
	line := rptEarlierLine(w.nextPacket("last"), "01-candidate", "cmd-F1")
	if line == "" {
		pdSetup(t, false, "the candidate packet lists no cmd-F1")
	}
	if !strings.Contains(line, "defer") || !strings.Contains(line, "#321") || strings.Contains(line, "reject") || strings.Contains(line, "처음 근거") {
		t.Errorf("jaekit:violation=%s the line does not show only the last disposition, a deferral to #321: %s", id, line)
	}
}

// AC-1: an earlier ledger line with a disposition spar does not know, or
// without a finding of its round, is refused before a reviewer starts, as
// contracts/spar-input.md G32 refuses a line it cannot interpret, and no
// round folder is left: here in the candidate round after review fixes,
// from the base commit 0 to the new head 2.
func TestRepeatFindingsAC1_UnknownDispositionRefused(t *testing.T) {
	const id = "unknown-disposition-shown"
	w := newRptWorld(t, false)
	c := w.commits
	for i, x := range []struct{ line, named string }{
		{`{"finding":"cmd-F1","disposition":"bogus"}`, "bogus"},
		{`{"disposition":"reject","evidence":"no finding"}`, `""`},
		{`{"finding":"cmd-F9","disposition":"reject","evidence":"another round's"}`, "cmd-F9"},
	} {
		series := fmt.Sprintf("x%d", i)
		w.useConfig(w.printer(w.answer(series+"-candidate", c[1], rptFinding("F1", "defect", "AC-1", "claim one"))))
		cand := filepath.Join(w.repo, "reviews", series, "01-candidate")
		w.run(w.round("candidate", c[0], c[1], cand)...)
		w.write(filepath.Join(cand, "ledger.jsonl"), x.line+"\n")
		out := filepath.Join(w.repo, "reviews", series, "02-candidate")
		code, stderr := w.spar(append(w.round("candidate", c[0], c[2], out), "--dry-run")...)
		if _, err := os.Stat(out); code != ExitRefused || err == nil || !strings.Contains(stderr, x.named) {
			t.Errorf("jaekit:violation=%s the ledger line %s: exit %d, round folder left %v: %s", id, x.line, code, err == nil, pdFirstLine(stderr))
		}
	}
}

// AC-1: a disposition recorded between the candidate round after review
// fixes, from the base commit 0 to the new head 2, and its retry on the same
// commits makes another packet. The retry is a new counted round, not a
// rerun of the same packet, and spar gate does not call a chain broken. The
// project configuration allows the three counted code rounds.
func TestRepeatFindingsAC1_RerunAfterTriage(t *testing.T) {
	const id = "rerun-ignores-disposition"
	w := newRptWorld(t, false)
	c := w.commits
	w.write(filepath.Join(w.repo, ".jaekit", "spar.toml"), "[caps]\ncode = 3\n")
	w.useConfig(w.printer(w.answer("r-candidate", c[1],
		rptFinding("F1", "defect", "AC-1", "first"), rptFinding("F2", "defect", "AC-1", "second"))))
	cand := filepath.Join(w.repo, "reviews", "r", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	w.triage(cand, "cmd-F1", "reject", "--evidence", "first is wrong")
	w.useConfig(pdCommand("cmd", w.script("fail", "if [ \"$1\" = --version ]; then exit 0; fi\nexit 3\n")))
	first := filepath.Join(w.repo, "reviews", "r", "02-candidate")
	w.run(w.round("candidate", c[0], c[2], first)...)
	w.triage(cand, "cmd-F2", "reject", "--evidence", "second is wrong too")
	w.useConfig(w.printer(w.answer("r-again", c[2])))
	again := filepath.Join(w.repo, "reviews", "r", "03-candidate")
	if code, stderr := w.spar(w.round("candidate", c[0], c[2], again)...); code != ExitOK && code != ExitNoResult {
		t.Errorf("jaekit:violation=%s the retry on the same commits after a new disposition was refused: exit %d: %s", id, code, pdFirstLine(stderr))
		return
	}
	r := w.rawRound(again)
	if rerunOf, _ := r["rerun_of"].([]any); r["counted"] != true || len(rerunOf) > 0 {
		t.Errorf("jaekit:violation=%s after a new disposition the retry is counted %v and a rerun of %v", id, r["counted"], r["rerun_of"])
	}
	gate := filepath.Join(w.dir, "r-gate.json")
	w.main("gate", cand, first, again, "--expect-head", c[2], "--out", gate)
	var g struct {
		Reasons []string `json:"reasons"`
	}
	data, err := os.ReadFile(gate)
	if err != nil || json.Unmarshal(data, &g) != nil {
		pdSetup(t, false, "spar gate wrote no readable gate file: %v", err)
	}
	for _, reason := range g.Reasons {
		if strings.Contains(reason, "chain") {
			t.Errorf("jaekit:violation=%s spar gate calls a chain broken: %s", id, reason)
		}
	}
}

// AC-2: a packet whose earlier findings have a disposition tells reviewers
// to report such a finding again only with a failure scenario the record
// does not answer, in the candidate round after review fixes. A plan-delta
// round, which carried it for the plan round's findings, is refused.
func TestRepeatFindingsAC2_RereportGuidance(t *testing.T) {
	const id = "rereport-guidance-missing"
	w := newRptWorld(t, false)
	if packet := w.nextPacket(filepathSeries(w, "g")); !strings.Contains(packet, rptRereport) {
		t.Errorf("jaekit:violation=%s the candidate packet does not say %q", id, rptRereport)
	}
	c := w.commits
	plan := filepath.Join(w.repo, "docs", "PLAN.md")
	w.useConfig(w.printer(w.answer("plan", c[1], rptFinding("F1", "defect", "AC-1", "the plan misses AC-1"))))
	out := filepath.Join(w.repo, "reviews", "gp", "01-plan")
	w.run(append(w.round("plan", c[0], c[1], out), "--subject", plan)...)
	w.triage(out, "cmd-F1", "reject", "--evidence", "it does not")
	w.planDeltaRefused("gp")
}

// AC-2: the fixes stage line separated findings recorded as fixed from those
// recorded as not fixed. A fixes round is now refused, and the candidate
// round after review fixes keeps the candidate stage line whatever the
// earlier findings' dispositions; the guidance on findings with a recorded
// disposition is the one AC2_RereportGuidance checks.
func TestRepeatFindingsAC2_FixesStageDistinguishes(t *testing.T) {
	w := newRptWorld(t, false)
	packet := w.nextPacket(filepathSeries(w, "s"))
	w.fixesRefused("s")
	var stage string
	for _, line := range strings.Split(pdSectionOf(packet, "## Round"), "\n") {
		if strings.HasPrefix(line, "- Stage: ") {
			stage = line
		}
	}
	if want := "- Stage: `candidate`. " + rptCandidate; stage != want {
		t.Errorf("the stage line of the candidate packet after review fixes is %q, want %q", stage, want)
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

// rptStages runs a round of every stage a new round starts as with the extra
// arguments, such as the two boundary documents, and returns each packet by
// stage: plan as a dry run in 01-plan, candidate in 02-candidate as a real
// round of a command reviewer that reports nothing. A plan-delta and a fixes
// round with the same arguments, as dry runs after them, are refused
// (oldStageRefused).
func (w *rptWorld) rptStages(series string, extra ...string) map[string]string {
	w.t.Helper()
	c := w.commits
	plan := filepath.Join(w.repo, "docs", "PLAN.md")
	w.useConfig(w.printer(w.answer(series, c[1])))
	dry := []string{"--dry-run"}
	runs := []struct {
		stage, base, head string
		extra             []string
	}{
		{"plan", c[0], c[1], append([]string{"--subject", plan}, dry...)},
		{"candidate", c[0], c[1], nil},
	}
	packets := map[string]string{}
	for i, r := range runs {
		out := filepath.Join(w.repo, "reviews", series, fmt.Sprintf("%02d-%s", i+1, r.stage))
		args := append(append(w.round(r.stage, r.base, r.head, out), r.extra...), extra...)
		code, stderr := w.spar(args...)
		if code == ExitUsage && strings.Contains(stderr, "boundary") {
			return nil // spar does not take boundary documents
		}
		if code != ExitOK && code != ExitNoResult {
			pdSetup(w.t, w.keeps, "spar run --stage %s exits %d: %s", r.stage, code, stderr)
		}
		packets[r.stage] = w.read(filepath.Join(out, "packet.md"))
	}
	w.oldStageRefused("plan-delta", c[0], c[1], filepath.Join(w.repo, "reviews", series, "03-plan-delta"),
		append(append([]string{"--subject", plan}, dry...), extra...)...)
	w.oldStageRefused("fixes", c[1], c[2], filepath.Join(w.repo, "reviews", series, "03-fixes"), append(dry, extra...)...)
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
	for _, stage := range StartStages {
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
	for _, token := range []string{"`hardening`", "`check_impact`", "`none`", "name the boundary", "`defect`", "protected"} {
		if !strings.Contains(guidance, token) {
			t.Errorf("jaekit:violation=%s the boundary guidance does not name %s", id, token)
		}
	}
	// Each case is on its own line with its own classification and its fixed
	// qualifier: outside a boundary and needed by nothing is hardening without
	// check impact and names the boundary; an input a requirement needs is a
	// defect whatever a boundary says.
	var outside, required string
	for _, line := range strings.Split(guidance, "\n") {
		switch {
		case strings.Contains(line, "outside") && strings.Contains(line, "`hardening`"):
			outside = line
		case strings.Contains(line, "`defect`"):
			required = line
		}
	}
	if outside == "" || !strings.Contains(outside, "that no criterion, rule, contract or protected regression path requires") ||
		!strings.Contains(outside, "`check_impact` `none`") || !strings.Contains(outside, "name the boundary") || strings.Contains(outside, "`defect`") {
		t.Errorf("jaekit:violation=%s no guidance line pairs a counterexample outside a boundary with `hardening`, `check_impact` `none` and naming the boundary: %q", id, outside)
	}
	if required == "" || !strings.Contains(required, "requires") || !strings.Contains(required, "protected") ||
		!strings.Contains(required, "whatever a boundary says") || strings.Contains(required, "`hardening`") {
		t.Errorf("jaekit:violation=%s no guidance line pairs a counterexample on a required input with `defect`: %q", id, required)
	}
}

// AC-3: round.json of a round of every stage keeps the boundary documents it
// received, by name and digest, in the order given.
func TestRepeatFindingsAC3_BoundaryRecorded(t *testing.T) {
	const id = "boundary-not-recorded"
	w := newRptWorld(t, false)
	one, two := w.rptBoundaries()
	if w.rptStages("br", "--boundary", one, "--boundary", two) == nil {
		t.Errorf("jaekit:violation=%s spar run does not take --boundary", id)
		return
	}
	want := func(path string) string {
		s := sha256.Sum256([]byte(w.read(path)))
		return hex.EncodeToString(s[:])
	}
	for i, stage := range StartStages {
		r := w.rawRound(filepath.Join(w.repo, "reviews", "br", fmt.Sprintf("%02d-%s", i+1, stage)))
		list, _ := r["boundaries"].([]any)
		var names, sums []string
		for _, item := range list {
			m, _ := item.(map[string]any)
			name, _ := m["name"].(string)
			sum, _ := m["sha256"].(string)
			names, sums = append(names, name), append(sums, sum)
		}
		if !reflect.DeepEqual(names, []string{"docs/B1.md", "docs/B2.md"}) || !reflect.DeepEqual(sums, []string{want(one), want(two)}) {
			t.Errorf("jaekit:violation=%s the %s round.json keeps boundaries %v %v", id, stage, names, sums)
		}
	}
}

// AC-3: boundary documents are part of the packet. A retry on the same
// commits whose boundary document changed has another packet digest and is a
// new counted round, not a rerun of the same packet.
func TestRepeatFindingsAC3_BoundaryInPacketIdentity(t *testing.T) {
	const id = "boundary-outside-packet"
	w := newRptWorld(t, false)
	c := w.commits
	one, _ := w.rptBoundaries()
	w.useConfig(pdCommand("cmd", w.script("fail", "if [ \"$1\" = --version ]; then exit 0; fi\nexit 3\n")))
	first := filepath.Join(w.repo, "reviews", "pi", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", c[0], c[1], first), "--boundary", one)...)
	if code == ExitUsage && strings.Contains(stderr, "boundary") {
		t.Errorf("jaekit:violation=%s spar run does not take --boundary", id)
		return
	}
	if code != ExitOK && code != ExitNoResult {
		pdSetup(t, false, "spar run exits %d: %s", code, stderr)
	}
	w.write(one, w.read(one)+"\n바뀐 경계.\n")
	again := filepath.Join(w.repo, "reviews", "pi", "02-candidate")
	w.run(append(w.round("candidate", c[0], c[1], again), "--boundary", one)...)
	a, b := w.rawRound(first), w.rawRound(again)
	if rerunOf, _ := b["rerun_of"].([]any); a["packet_sha256"] == b["packet_sha256"] || b["counted"] != true || len(rerunOf) > 0 {
		t.Errorf("jaekit:violation=%s a retry with a changed boundary document has packet %v (was %v), counted %v, a rerun of %v",
			id, b["packet_sha256"], a["packet_sha256"], b["counted"], b["rerun_of"])
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

// AC-4: --boundary refuses and accepts the documents --criteria does. By
// contracts/spar-input.md G26, --criteria refuses a file it cannot read (a
// missing file, a folder, a file without read permission) with 65 and no
// round folder, and takes any readable file (an empty file, a file outside
// the repository, a file that is not UTF-8). --boundary gives each form that
// outcome. A --criteria outcome off the contract fails without this check's
// violation: it belongs to AC-11.
func TestRepeatFindingsAC4_SameCasesAsCriteria(t *testing.T) {
	const id = "boundary-cases-differ"
	w := newRptWorld(t, false)
	w.useConfig(w.printer(w.answer("cases", w.commits[1])))
	forms := map[string]string{
		"missing":  filepath.Join(w.repo, "docs", "none.md"),
		"folder":   filepath.Join(w.repo, "docs"),
		"empty":    w.write(filepath.Join(w.repo, "docs", "empty.md"), ""),
		"outside":  w.write(filepath.Join(w.dir, "outside", "B.md"), "## 밖의 경계\n"),
		"not-utf8": w.write(filepath.Join(w.repo, "docs", "latin1.md"), "caf\xe9\n"),
	}
	if locked := w.write(filepath.Join(w.repo, "docs", "locked.md"), "locked\n"); os.Geteuid() != 0 && os.Chmod(locked, 0) == nil {
		forms["unreadable"] = locked
		defer os.Chmod(locked, 0o644)
	}
	outcome := func(flag, name, path string) string {
		out := filepath.Join(w.repo, "reviews", strings.TrimPrefix(flag, "--")+"-"+name, "01-candidate")
		code, _ := w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), flag, path, "--dry-run")...)
		_, err := os.Stat(out)
		switch {
		case code == ExitRefused && os.IsNotExist(err):
			return "refused"
		case (code == ExitOK || code == ExitNoResult) && err == nil:
			return "accepted"
		}
		return fmt.Sprintf("exit %d with round folder %v", code, err == nil)
	}
	contract := map[string]string{"missing": "refused", "folder": "refused", "unreadable": "refused",
		"empty": "accepted", "outside": "accepted", "not-utf8": "accepted"}
	for name, path := range forms {
		want := contract[name]
		if got := outcome("--criteria", name, path); got != want {
			t.Errorf("--criteria with a %s document is %s, but contracts/spar-input.md G26 says %s", name, got, want)
		}
		if got := outcome("--boundary", name, path); got != want {
			t.Errorf("jaekit:violation=%s a %s document is %s as --boundary, but %s as --criteria by contracts/spar-input.md G26", id, name, got, want)
		}
	}
}

// AC-4: the input table of the Spar input contract has a row for --boundary
// that names its form, the result outside it and where it goes. The
// contract is required: a missing file fails.
func TestRepeatFindingsAC4_InputTableRow(t *testing.T) {
	const id = "boundary-input-row-missing"
	const path = "../../contracts/spar-input.md"
	data, err := os.ReadFile(path)
	if err != nil {
		pdSetup(t, true, "%v", err)
	}
	var row string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "| G") && strings.Contains(line, "`--boundary`") {
			row = line
		}
	}
	if row == "" {
		t.Fatalf("jaekit:violation=%s %s has no input row for `--boundary`", id, path)
	}
	if cells := strings.Split(row, "|"); len(cells) != 7 {
		t.Errorf("jaekit:violation=%s %s's `--boundary` row does not have the table's five columns", id, path)
	}
	for _, token := range []string{"`round.json`", "(65)"} {
		if !strings.Contains(row, token) {
			t.Errorf("jaekit:violation=%s %s's `--boundary` row does not name %s", id, path, token)
		}
	}
}

// AC-5: in the candidate round after review fixes, from the base commit to
// the new head, with dispositions and boundary documents, every reviewer
// receives the same packet: claude and codex on standard input, agy after
// its command note as an argument and as PACKET.md, native in prompt.md and
// a command reviewer at its {prompt} path.
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
	out := filepath.Join(w.repo, "reviews", "all", "02-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], head, out), "--boundary", one, "--boundary", two)...)
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
// no round folder, and a message that names the reviewer, its limit and the
// packet's measured size, each as a whole number of its own.
func (w *rptWorld) rptRefused(code int, stderr, out string, reviewer string, limit, size int) bool {
	_, err := os.Stat(out)
	numbers := map[string]bool{}
	for _, word := range strings.FieldsFunc(stderr, func(r rune) bool { return r < '0' || r > '9' }) {
		numbers[word] = true
	}
	return code == ExitRefused && os.IsNotExist(err) && size > limit && numbers[strconv.Itoa(size)] &&
		numbers[strconv.Itoa(limit)] && strings.Contains(stderr, reviewer)
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
		if !w.rptRefused(code, stderr, out, "cmd", size-1, size) {
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
	// Only this run's reviewers count: a reviewer left out with --reviewers
	// does not refuse the packet with its smaller limit.
	w.useConfig(pdCommand("cmd", mark) + pdCommand("tiny", mark) + "max_packet_bytes = 100\n")
	out = filepath.Join(w.repo, "reviews", "unselected", "01-candidate")
	if code, stderr := w.spar(append(w.round("candidate", w.commits[0], w.commits[1], out), "--reviewers", "cmd")...); code != ExitOK && code != ExitNoResult {
		t.Errorf("jaekit:violation=%s a reviewer outside the run refused the packet with its limit: exit %d: %s", id, code, pdFirstLine(stderr))
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
	limits, isolation := map[string]int{}, map[string]any{}
	for _, r := range w.rptConfigReviewers(id) {
		name, _ := r["name"].(string)
		limits[name], isolation[name] = rptLimit(r), r["isolation"]
	}
	for name, want := range map[string]int{"claude": 1_200_000, "codex": 1_200_000, "agy": 4_000_000} {
		if limits[name] != want {
			t.Errorf("jaekit:violation=%s %s has the default packet limit %d, want %d", id, name, limits[name], want)
		}
	}
	if want := map[string]any{"claude": "verified", "codex": "verified", "agy": "verified", "native": "host", "cmd": "unverified"}; !reflect.DeepEqual(isolation, want) {
		t.Errorf("preset isolation without a setting is %v, want %v", isolation, want)
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
	w.useConfig("[[reviewers]]\nname = \"native\"\npreset = \"native\"\n\n")
	size := w.rptSize("big-measure", "--criteria", big)
	w.useConfig(w.pdFake("claude", head))
	out := filepath.Join(w.repo, "reviews", "big", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], head, out), "--criteria", big, "--dry-run")...)
	if !w.rptRefused(code, stderr, out, "claude", limits["claude"], size) {
		t.Errorf("jaekit:violation=%s a %d-byte packet over the claude default %d was not refused: exit %d: %s", id, size, limits["claude"], code, pdFirstLine(stderr))
	}
}

// AC-6: the user's setting changes one reviewer's limit, larger or smaller
// than the default.
func TestRepeatFindingsAC6_UserLimit(t *testing.T) {
	const id = "user-limit-ignored"
	w := newRptWorld(t, false)
	head := w.commits[1]
	w.useConfig(w.pdFake("claude", head))
	preset := 0
	for _, r := range w.rptConfigReviewers(id) {
		if r["name"] == "claude" {
			preset = rptLimit(r)
		}
	}
	size := w.rptSize("measure")
	w.useConfig(w.pdFake("claude", head) + fmt.Sprintf("max_packet_bytes = %d\n", size-1))
	out := filepath.Join(w.repo, "reviews", "small", "01-candidate")
	code, stderr := w.spar(append(w.round("candidate", w.commits[0], head, out), "--dry-run")...)
	if !w.rptRefused(code, stderr, out, "claude", size-1, size) {
		t.Errorf("jaekit:violation=%s the user's smaller limit %d did not refuse a %d-byte packet: exit %d: %s", id, size-1, size, code, pdFirstLine(stderr))
	}
	w.useConfig(w.pdFake("claude", head) + "max_packet_bytes = 50000000\n")
	for _, r := range w.rptConfigReviewers(id) {
		if r["name"] == "claude" && rptLimit(r) != 50000000 {
			t.Errorf("jaekit:violation=%s the user's larger limit shows as %d", id, rptLimit(r))
		}
	}
	// The larger limit applies to the run: a packet over the claude default,
	// as spar config shows it without a setting, gets a round.
	if preset <= 0 {
		t.Errorf("jaekit:violation=%s claude shows no default limit for the user's larger limit to exceed", id)
		return
	}
	big := w.write(filepath.Join(w.repo, "docs", "BIG.md"), strings.Repeat("x", preset+1))
	out = filepath.Join(w.repo, "reviews", "large", "01-candidate")
	code, stderr = w.spar(append(w.round("candidate", w.commits[0], head, out), "--criteria", big, "--dry-run")...)
	if _, err := os.Stat(out); (code != ExitOK && code != ExitNoResult) || err != nil {
		t.Errorf("jaekit:violation=%s the user's larger limit did not admit a packet over the default: exit %d: %s", id, code, pdFirstLine(stderr))
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
	w.useConfig(w.pdFake("claude", head), w.pdFake("codex", head)+"max_packet_bytes = 123456\n", w.pdFake("agy", head),
		"[[reviewers]]\nname = \"native\"\npreset = \"native\"\n\n")
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
	if sources["agy"] != "preset" || limits["agy"] <= 0 {
		t.Errorf("jaekit:violation=%s agy's limit shows as %d from %q, want the preset's", id, limits["agy"], sources["agy"])
	}
	if sources["native"] != "" || limits["native"] != 0 {
		t.Errorf("jaekit:violation=%s native shows a limit %d from %q without a setting", id, limits["native"], sources["native"])
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
	// A placeholder such as <goal base commit> is one word.
	quoted, angled, inWord := false, false, false
	for _, r := range block {
		switch {
		case r == '\'':
			quoted, inWord = !quoted, true
		case !quoted && (r == '<' || r == '>'):
			angled = r == '<'
			cur.WriteRune(r)
			inWord = true
		case !quoted && !angled && (r == ' ' || r == '\n' || r == '\t'):
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
// its reject and evidence in the packet of the candidate round after review
// fixes, which took the place of the next fixes round.
func TestRepeatFindingsAC8_TriageThenFixes(t *testing.T) {
	const id = "fixes-packet-without-reject"
	w := newRptWorld(t, false)
	c := w.commits
	w.useConfig(w.printer(w.answer("t-candidate", c[1], rptFinding("F1", "defect", "AC-1", "the greeting changed"))))
	cand := filepath.Join(w.repo, "reviews", "t", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	w.triage(cand, "cmd-F1", "reject", "--evidence", "AC-1이 인사말을 바꾸라고 한다")
	line := rptEarlierLine(w.nextPacket("t"), "01-candidate", "cmd-F1")
	if line == "" {
		pdSetup(t, false, "the candidate packet lists no cmd-F1")
	}
	if !strings.Contains(line, "reject") || !strings.Contains(line, "AC-1이 인사말을 바꾸라고 한다") {
		t.Errorf("jaekit:violation=%s the candidate packet does not show the reject and its evidence: %s", id, line)
	}
}

// AC-10: the packet of the candidate round after review fixes, whose
// earlier finding has no disposition, keeps the earlier line, the candidate
// stage line and no new guidance or section.
func TestRepeatFindingsAC10_OpenFindingPacket(t *testing.T) {
	w := newRptWorld(t, true)
	c := w.commits
	w.useConfig(w.printer(w.answer("o-candidate", c[1], rptFinding("F1", "defect", "AC-1", "the greeting changed"))))
	w.run(w.round("candidate", c[0], c[1], filepath.Join(w.repo, "reviews", "o", "01-candidate"))...)
	packet := w.nextPacket("o")
	if line := rptEarlierLine(packet, "01-candidate", "cmd-F1"); line != "- 01-candidate cmd-F1 [defect] at head/app/main.py:1: the greeting changed" {
		t.Errorf("the line of an open finding changed: %q", line)
	}
	if !strings.Contains(packet, "- Stage: `candidate`. "+rptCandidate+"\n") {
		t.Errorf("the candidate stage line changed")
	}
	for _, s := range []string{rptRereport, rptFixed, rptBoundaryHeading} {
		if strings.Contains(packet, s) {
			t.Errorf("a packet without dispositions or boundaries holds %q", s)
		}
	}
}

// AC-11: a round without boundary documents writes no boundary key, and the
// candidate round after review fixes reads such a round as one without
// boundaries.
func TestRepeatFindingsAC11_RoundWithoutBoundaries(t *testing.T) {
	w := newRptWorld(t, true)
	c := w.commits
	w.useConfig(w.printer(w.answer("n-candidate", c[1])))
	cand := filepath.Join(w.repo, "reviews", "n", "01-candidate")
	w.run(w.round("candidate", c[0], c[1], cand)...)
	if _, ok := w.rawRound(cand)["boundaries"]; ok {
		t.Errorf("a round without boundary documents records a boundaries key")
	}
	if strings.Contains(w.nextPacket("n"), rptBoundaryHeading) {
		t.Errorf("the candidate round after a round without boundaries has a boundary section")
	}
}

// AC-12: Core reads a series with boundary documents and dispositions as it
// reads the same series without them, and a series without any disposition
// differs from one with a disposition only in the ledger entry and the
// gate's judgment of that finding.
func TestRepeatFindingsAC12_CoreReadsNewRecords(t *testing.T) {
	w := newRptWorld(t, true)
	one, two := w.rptBoundaries()
	c := w.commits
	// read runs a candidate round, rejects its finding when triaged, runs the
	// candidate round after review fixes from the base commit to the new
	// head and gates the series. It returns what Core reads from them: the
	// series that ha cost and the review check of ha done read, and the gate
	// that ha note review --gate reads, through both of Core's gate readers.
	// The folder, the round times and the gate file's own digest differ
	// between two series by construction and are cleared.
	read := func(series string, triaged bool, extra ...string) []any {
		w.useConfig(w.printer(w.answer(series, c[1], rptFinding("F1", "defect", "AC-1", "the greeting changed"))))
		cand := filepath.Join(w.repo, "reviews", series, "01-candidate")
		w.run(append(w.round("candidate", c[0], c[1], cand), extra...)...)
		if triaged {
			w.triage(cand, "cmd-F1", "reject", "--evidence", "AC-1이 인사말을 바꾸라고 한다")
		}
		w.useConfig(w.printer(w.answer(series+"-fixes", c[2])))
		fixes := filepath.Join(w.repo, "reviews", series, "02-candidate")
		w.run(append(w.round("candidate", c[0], c[2], fixes), extra...)...)
		s, err := reviewseries.Read(filepath.Join(w.repo, "reviews", series))
		if err != nil {
			t.Fatalf("Core cannot read the series %s: %v", series, err)
		}
		s.Path = ""
		for i := range s.Rounds {
			s.Rounds[i].StartedAt, s.Rounds[i].FinishedAt = "", ""
			for j := range s.Rounds[i].Reviewers {
				s.Rounds[i].Reviewers[j].ElapsedSeconds = nil
			}
		}
		gate := filepath.Join(w.dir, series+"-gate.json")
		code, _, stderr := w.main("gate", cand, fixes, "--expect-head", c[2], "--out", gate, "--repo", w.repo)
		data, err := os.ReadFile(gate)
		if err != nil {
			t.Fatalf("spar gate exits %d and wrote no gate file: %v: %s", code, err, stderr)
		}
		g, err := reviewgate.Read(gate)
		if err != nil {
			t.Fatalf("Core cannot read the gate of %s: %v", series, err)
		}
		p, err := reviewgate.Parse(data)
		if err != nil {
			t.Fatalf("Core cannot parse the gate of %s: %v", series, err)
		}
		g.GateDigest, p.GateDigest = "", ""
		return []any{s, g, p}
	}
	with := read("with", true, "--boundary", one, "--boundary", two)
	without := read("without", true)
	for i, what := range []string{"series", "gate", "parsed gate"} {
		if !reflect.DeepEqual(with[i], without[i]) {
			a, _ := json.Marshal(with[i])
			b, _ := json.Marshal(without[i])
			t.Errorf("Core reads the %s with boundaries and dispositions differently:\n%s\nwithout them:\n%s", what, a, b)
		}
	}

	// The same series without any disposition. In the series, only the
	// candidate round's ledger differs: the reject entry is absent. In the
	// gate, only its reading of that entry differs: the finding is open, so
	// the gate counts one open defect, lists it as unhandled and is blocked,
	// where the gate with the disposition is only degraded (a command
	// reviewer is never a required one). Every round field (counted,
	// rerun_of, findings, reviewers and the rest) and the gate's rounds read
	// the same.
	open := read("open", false)
	reject := []reviewseries.LedgerEntry{{Finding: "cmd-F1", Disposition: "reject"}}
	disposed, undisposed := without[0].(*reviewseries.Series), open[0].(*reviewseries.Series)
	if len(disposed.Rounds) != 2 || len(undisposed.Rounds) != 2 {
		t.Fatalf("Core reads %d and %d rounds, want 2 in each series", len(disposed.Rounds), len(undisposed.Rounds))
	}
	if !reflect.DeepEqual(disposed.Rounds[0].Ledger, reject) || len(undisposed.Rounds[0].Ledger) != 0 ||
		len(disposed.Rounds[1].Ledger) != 0 || len(undisposed.Rounds[1].Ledger) != 0 {
		t.Errorf("Core reads the ledgers %v and %v with the disposition, %v and %v without it",
			disposed.Rounds[0].Ledger, disposed.Rounds[1].Ledger, undisposed.Rounds[0].Ledger, undisposed.Rounds[1].Ledger)
	}
	withoutLedger := func(s *reviewseries.Series) reviewseries.Series {
		cp := *s
		cp.Rounds = append([]reviewseries.Round(nil), s.Rounds...)
		for i := range cp.Rounds {
			cp.Rounds[i].Ledger = nil
		}
		return cp
	}
	if a, b := withoutLedger(disposed), withoutLedger(undisposed); !reflect.DeepEqual(a, b) {
		x, _ := json.Marshal(a)
		y, _ := json.Marshal(b)
		t.Errorf("apart from the ledger, Core reads the series with a disposition differently:\n%s\nwithout one:\n%s", x, y)
	}
	for i, what := range []string{"gate", "parsed gate"} {
		dg, ug := without[i+1].(*record.ReviewData), open[i+1].(*record.ReviewData)
		want := []record.ReviewFinding{{Series: "open", Round: "01-candidate", ID: "cmd-F1", Class: "defect"}}
		dl, err := reviewgate.ReadFindings(dg.Findings)
		if err != nil {
			t.Fatalf("Core cannot read the findings of the %s with a disposition: %v", what, err)
		}
		ul, err := reviewgate.ReadFindings(ug.Findings)
		if err != nil {
			t.Fatalf("Core cannot read the findings of the %s without a disposition: %v", what, err)
		}
		if dg.State != StateDegraded || dg.Open != (record.ReviewOpen{}) || len(dl.Unhandled) != 0 ||
			ug.State != StateBlocked || ug.Open != (record.ReviewOpen{Defect: 1}) || !reflect.DeepEqual(ul.Unhandled, want) ||
			len(dl.Deferred) != 0 || len(ul.Deferred) != 0 || dl.AutoDeferred != 0 || ul.AutoDeferred != 0 {
			t.Errorf("Core reads the %s as %s, open %+v, lists %+v with the disposition; %s, open %+v, lists %+v without it",
				what, dg.State, dg.Open, dl, ug.State, ug.Open, ul)
		}
		a, b := *dg, *ug
		a.State, a.Open, a.Findings = "", record.ReviewOpen{}, nil
		b.State, b.Open, b.Findings = "", record.ReviewOpen{}, nil
		if !reflect.DeepEqual(a, b) {
			x, _ := json.Marshal(a)
			y, _ := json.Marshal(b)
			t.Errorf("apart from its judgment of cmd-F1, Core reads the %s with a disposition differently:\n%s\nwithout one:\n%s", what, x, y)
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
