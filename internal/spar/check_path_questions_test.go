package spar

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// These tests observe how a finding at a check path is classified, recorded
// and gated when it proposes a requirement question. They drive the spar
// command line (Main) on a synthetic repository with a native reviewer, so
// the classification, the automatic deferral and the gate are the ones a
// recorded round uses. The opening line of an assertion's failure message
// names the property it observes (jaekit:violation=<id>); a failure without
// it is a broken setup, not an observation.

// cpqCheck is the check file location the check-path findings point at.
const cpqCheck = "head/checks/test_app.py:2"

const cpqQuote = "synthetic user answer"

type cpqWorld struct {
	t          *testing.T
	dir, repo  string
	base, head string
	criteria   string
	getenv     func(string) string
	series     int
}

// newCPQWorld makes a repository whose head changes a check file, with a
// three-line rule document AGENTS.md, a three-line contract contracts/x.md
// and criteria AC-1 and AC-2. Without git, a check of a change condition is
// skipped and a check of a kept behavior fails, so neither passes.
func newCPQWorld(t *testing.T, keeps bool) *cpqWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		if keeps {
			t.Fatalf("git is not available: %v", err)
		}
		t.Skipf("git is not available: %v", err)
	}
	w := &cpqWorld{t: t, dir: t.TempDir()}
	w.repo = realish(filepath.Join(w.dir, "repo"))
	files := map[string]string{
		".gitignore":         "/reviews/\n",
		"AGENTS.md":          "# Rules\n\nKeep checks honest.\n",
		"contracts/x.md":     "# Contract\n\nOne rule.\n",
		"app/main.py":        "print('one')\n",
		"checks/test_app.py": "import app\nassert True\n",
	}
	for path, body := range files {
		w.write(filepath.Join(w.repo, path), body)
	}
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app/main.py"), "print('two')\n")
	w.write(filepath.Join(w.repo, "checks/test_app.py"), "import app\nassert app\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	w.criteria = w.write(filepath.Join(w.dir, "SPEC.md"), "- **AC-1** one\n- **AC-2** two\n")
	config := w.write(filepath.Join(w.dir, "spar.toml"), "[[reviewers]]\nname = \"host\"\npreset = \"native\"\n")
	values := map[string]string{"SPAR_CONFIG": config, "HOME": w.dir}
	w.getenv = func(k string) string { return values[k] }
	return w
}

func (w *cpqWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatal(err)
	}
	return path
}

func (w *cpqWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// spar runs the spar command line and returns its exit code and messages.
func (w *cpqWorld) spar(args ...string) (int, string) {
	var stdout, stderr bytes.Buffer
	code := Main("test", args, strings.NewReader(""), &stdout, &stderr, w.getenv)
	return code, stdout.String() + stderr.String()
}

// round runs a candidate round in a series of its own, with check paths
// checks/** and the rule document AGENTS.md, and records the native
// reviewer's findings.
func (w *cpqWorld) round(findings ...map[string]any) string {
	w.t.Helper()
	return w.roundWith([]string{"--rules", filepath.Join(w.repo, "AGENTS.md"), "--check-path", "checks/**"}, findings...)
}

// roundWith is round with other rule documents and check paths.
func (w *cpqWorld) roundWith(options []string, findings ...map[string]any) string {
	w.t.Helper()
	w.series++
	out := filepath.Join(w.repo, "reviews", fmt.Sprintf("s%d", w.series), "01-candidate")
	args := append([]string{"run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", out,
		"--criteria", w.criteria, "--repo", w.repo}, options...)
	if code, msg := w.spar(args...); code != ExitOK {
		w.t.Fatalf("spar run: %d %s", code, msg)
	}
	var checked []any
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "checked"})
	}
	list := []any{}
	for _, f := range findings {
		list = append(list, f)
	}
	verdict := "no_blocking_findings"
	if len(list) > 0 {
		verdict = "blocking_findings"
	}
	data, err := json.Marshal(map[string]any{"reviewed_head": w.head, "verdict": verdict, "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": list, "limits": ""})
	if err != nil {
		w.t.Fatal(err)
	}
	reply := w.write(filepath.Join(w.dir, fmt.Sprintf("reply-%d.json", w.series)), string(data)+"\n")
	if code, msg := w.spar("record", out, "host", "--result", reply); code != ExitOK {
		w.t.Fatalf("spar record: %d %s", code, msg)
	}
	return out
}

// findings reads findings.json by the reviewer's own finding ID.
func (w *cpqWorld) findings(out string) map[string]Finding {
	w.t.Helper()
	data, err := os.ReadFile(filepath.Join(out, "findings.json"))
	if err != nil {
		w.t.Fatal(err)
	}
	var list []Finding
	if err := json.Unmarshal(data, &list); err != nil {
		w.t.Fatalf("findings.json: %v", err)
	}
	by := map[string]Finding{}
	for _, f := range list {
		by[f.ReviewerID] = f
	}
	return by
}

// ledger reads the ledger entries of each finding by its round ID. Every
// recorded round has a ledger.jsonl, empty when nothing was deferred, so a
// missing one is a broken setup, never a ledger without entries.
func (w *cpqWorld) ledger(out string) map[string][]LedgerEntry {
	w.t.Helper()
	if info, err := os.Stat(filepath.Join(out, "ledger.jsonl")); err != nil || !info.Mode().IsRegular() {
		w.t.Fatalf("ledger.jsonl cannot be read: %v", err)
	}
	list, err := loadLedger(out)
	if err != nil {
		w.t.Fatalf("ledger.jsonl: %v", err)
	}
	by := map[string][]LedgerEntry{}
	for _, e := range list {
		by[e.Finding] = append(by[e.Finding], e)
	}
	return by
}

// gate writes the gate of a round to its series and reads it back. A gate
// with any reason other than open findings could not read the round or its
// repository, for example a damaged record or a git that failed: that is a
// broken setup, never an observation of how findings are counted.
func (w *cpqWorld) gate(out string) (int, GateRecord) {
	w.t.Helper()
	file := filepath.Join(filepath.Dir(out), "gate.json")
	code, msg := w.spar("gate", out, "--expect-head", w.head, "--out", file, "--repo", w.repo)
	var g GateRecord
	data, err := os.ReadFile(file)
	if err != nil {
		w.t.Fatalf("spar gate wrote no file: %d %s", code, msg)
	}
	if err := json.Unmarshal(data, &g); err != nil {
		w.t.Fatalf("gate.json: %v", err)
	}
	for _, reason := range g.Reasons {
		if !strings.HasSuffix(reason, " open defect or requirement-question finding(s)") {
			w.t.Fatalf("the gate did not read the round cleanly (exit %d): %v", code, g.Reasons)
		}
	}
	if code != ExitOK && code != ExitBlocked {
		w.t.Fatalf("spar gate: exit %d %s", code, msg)
	}
	return code, g
}

func cpqFinding(id string, values map[string]any) map[string]any {
	f := map[string]any{"id": id, "proposed_class": "note", "angle": "design", "location": cpqCheck,
		"claim": "synthetic claim " + id, "failure_scenario": "a synthetic input gives a wrong result",
		"evidence": "synthetic evidence", "link_kind": "none", "link": "", "introduced_by_change": "no",
		"regression": "no", "check_impact": "none", "confidence": "high"}
	for k, v := range values {
		f[k] = v
	}
	return f
}

type cpqCase struct {
	values map[string]any
	class  string
}

// cpqClasses records one round with the cases as findings F1, F2, … and
// returns it with the findings and ledger it recorded.
func (w *cpqWorld) cpqClasses(cases []cpqCase) (string, map[string]Finding, map[string][]LedgerEntry) {
	w.t.Helper()
	var raw []map[string]any
	for i, c := range cases {
		raw = append(raw, cpqFinding(fmt.Sprintf("F%d", i+1), c.values))
	}
	out := w.round(raw...)
	found := w.findings(out)
	if len(found) != len(cases) {
		w.t.Fatalf("recorded %d findings for %d cases", len(found), len(cases))
	}
	return out, found, w.ledger(out)
}

func cpqViolation(t *testing.T, id, format string, args ...any) {
	t.Helper()
	t.Errorf("jaekit:violation=%s: "+format, append([]any{id}, args...)...)
}

// AC-1: a requirement question at a check path whose criterion, rule or
// contract link exists is recorded as a requirement question and is not
// deferred automatically, whatever form the location and the link take. A
// contract link outside the check paths counts, a head file included.
func TestSparQuestionsAC1_LinkedQuestionStaysOpen(t *testing.T) {
	const id = "check-path-question-deferred"
	w := newCPQWorld(t, false)
	question := func(values map[string]any) map[string]any {
		values["proposed_class"] = "requirement-question"
		return values
	}
	cases := []cpqCase{
		{question(map[string]any{"link_kind": "criterion", "link": "AC-1"}), "requirement-question"},
		{question(map[string]any{"link_kind": "criterion", "link": "AC-1, AC-2"}), "requirement-question"},
		{question(map[string]any{"link_kind": "rule", "link": "AGENTS.md:2"}), "requirement-question"},
		{question(map[string]any{"link_kind": "rule", "link": "AGENTS.md:1-3"}), "requirement-question"},
		{question(map[string]any{"link_kind": "rule", "link": "AGENTS.md"}), "requirement-question"},
		{question(map[string]any{"link_kind": "contract", "link": "contracts/x.md:3"}), "requirement-question"},
		{question(map[string]any{"link_kind": "contract", "link": "head/app/main.py:1"}), "requirement-question"},
		{question(map[string]any{"link_kind": "contract", "link": "AGENTS.md:2"}), "requirement-question"},
		{question(map[string]any{"link_kind": "criterion", "link": "AC-2", "location": "head/checks/test_app.py"}), "requirement-question"},
		{question(map[string]any{"link_kind": "criterion", "link": "AC-1", "location": "checks/test_app.py:2"}), "requirement-question"},
		{question(map[string]any{"link_kind": "criterion", "link": "AC-1", "location": "head/checks/"}), "requirement-question"},
		{question(map[string]any{"link_kind": "criterion", "link": "AC-1", "introduced_by_change": "yes", "regression": "yes"}), "requirement-question"},
	}
	_, found, ledger := w.cpqClasses(cases)
	for i, c := range cases {
		f := found[fmt.Sprintf("F%d", i+1)]
		if !f.InCheckPaths || f.LocationUnreadable || f.LinkKind != c.values["link_kind"] {
			t.Errorf("case %d %v: the setup does not give a linked check-path finding: in check paths %v, unreadable %v, link %s",
				i+1, c.values, f.InCheckPaths, f.LocationUnreadable, f.LinkKind)
			continue
		}
		if f.Class != c.class {
			cpqViolation(t, id, "case %d %v: class %s (%s), want %s", i+1, c.values, f.Class, f.ClassReason, c.class)
		}
		if entries := ledger[f.ID]; len(entries) != 0 {
			cpqViolation(t, id, "case %d %v: the round recorded ledger entries %+v", i+1, c.values, entries)
		}
	}
	// A contract outside the repository is never a file in the check
	// paths, even when a check path such as ** would match its absolute
	// path.
	outside := realish(w.write(filepath.Join(w.dir, "policy", "CONTRACT.md"), "# Policy\n\nOne rule.\n"))
	links := []string{"CONTRACT.md:2", filepath.ToSlash(outside) + ":2"}
	var raw []map[string]any
	for i, link := range links {
		raw = append(raw, cpqFinding(fmt.Sprintf("F%d", i+1), map[string]any{"proposed_class": "requirement-question",
			"link_kind": "contract", "link": link}))
	}
	out := w.roundWith([]string{"--rules", outside, "--check-path", "**"}, raw...)
	found, ledger = w.findings(out), w.ledger(out)
	for i, link := range links {
		f := found[fmt.Sprintf("F%d", i+1)]
		if !f.InCheckPaths || f.LinkKind != "contract" {
			t.Errorf("outside contract %s: the setup does not give a linked check-path finding: in check paths %v, link %s",
				link, f.InCheckPaths, f.LinkKind)
			continue
		}
		if f.Class != "requirement-question" || len(ledger[f.ID]) != 0 {
			cpqViolation(t, id, "outside contract %s: class %s (%s), ledger %+v", link, f.Class, f.ClassReason, ledger[f.ID])
		}
	}
}

// AC-2: an open linked question at a check path holds the gate, and each
// disposition that closes it with the user's words clears that reason.
func TestSparQuestionsAC2_QuestionHoldsTheGateUntilAnswered(t *testing.T) {
	const id = "check-path-question-not-gated"
	w := newCPQWorld(t, false)
	closings := [][]string{
		{"answer", "--user-quote", cpqQuote},
		{"reject", "--user-quote", cpqQuote},
		{"defer", "--to", "a later goal", "--user-quote", cpqQuote},
	}
	for _, closing := range closings {
		out := w.round(cpqFinding("F1", map[string]any{"proposed_class": "requirement-question",
			"link_kind": "criterion", "link": "AC-1"}))
		listed := filepath.Base(filepath.Dir(out)) + "/" + filepath.Base(out) + " host-F1"
		code, g := w.gate(out)
		if code != ExitBlocked || g.State != StateBlocked || g.Open["requirement-question"] != 1 ||
			len(g.OpenFindings) != 1 || !strings.HasSuffix(g.OpenFindings[0], listed) {
			t.Fatalf("jaekit:violation=%s: the open question does not hold the gate: exit %d, %s, open %v %v, auto-deferred %d",
				id, code, g.State, g.Open, g.OpenFindings, g.AutoDeferred)
		}
		unquoted := append([]string{"triage", out, "host-F1"}, closing[:len(closing)-2]...)
		if code, msg := w.spar(unquoted...); code != ExitRefused {
			cpqViolation(t, id, "%s without the user's words was accepted: %d %s", closing[0], code, msg)
		}
		// A refusal here may come from a ledger that cannot be read, so it
		// is a broken setup; the gate below observes the closing.
		if code, msg := w.spar(append([]string{"triage", out, "host-F1"}, closing...)...); code != ExitOK {
			t.Fatalf("%v with the user's words was refused: %d %s", closing, code, msg)
		}
		code, g = w.gate(out)
		if code != ExitOK || g.State != StateComplete || g.Open["requirement-question"] != 0 || len(g.OpenFindings) != 0 {
			cpqViolation(t, id, "%s with the user's words left the gate %s (exit %d): open %v, reasons %v",
				closing[0], g.State, code, g.Open, g.Reasons)
		}
	}
}

// AC-3 (kept): a requirement question at a check path without a link, or
// with a link that does not exist, is a hardening item deferred
// automatically. A contract link to a file in the check paths names the
// check code itself and does not count either (the user's decision).
func TestSparQuestionsAC3_UnlinkedQuestionIsDeferred(t *testing.T) {
	const id = "unlinked-question-not-deferred"
	w := newCPQWorld(t, true)
	question := func(kind, link string) map[string]any {
		return map[string]any{"proposed_class": "requirement-question", "link_kind": kind, "link": link}
	}
	cases := []cpqCase{
		{question("none", ""), "hardening"},
		{question("criterion", "AC-99"), "hardening"},
		{question("criterion", "AC-1, AC-99"), "hardening"},
		{question("criterion", "the first criterion"), "hardening"},
		{question("rule", "AGENTS.md:99"), "hardening"},
		{question("rule", "AGENTS.md:0"), "hardening"},
		{question("rule", "AGENTS.md:3-1"), "hardening"},
		{question("rule", "contracts/x.md:1"), "hardening"},
		{question("rule", "AC-1"), "hardening"},
		{question("contract", "contracts/missing.md:1"), "hardening"},
		{question("contract", "checks/test_app.py:2"), "hardening"},
		{question("contract", "head/checks/test_app.py"), "hardening"},
	}
	out, found, ledger := w.cpqClasses(cases)
	for i, c := range cases {
		f := found[fmt.Sprintf("F%d", i+1)]
		if !f.InCheckPaths || f.LocationUnreadable {
			t.Errorf("case %d %v: the setup does not give a check-path finding", i+1, c.values)
			continue
		}
		entries := ledger[f.ID]
		if f.Class != c.class || len(entries) != 1 || entries[0].Disposition != "defer" || !entries[0].Auto {
			cpqViolation(t, id, "case %d %v: class %s, link %s, ledger %+v", i+1, c.values, f.Class, f.LinkKind, entries)
		}
	}
	if code, g := w.gate(out); code != ExitOK || g.State != StateComplete || g.AutoDeferred != len(cases) ||
		g.Open["requirement-question"] != 0 {
		cpqViolation(t, id, "gate %s (exit %d): auto-deferred %d of %d, open %v", g.State, code, g.AutoDeferred, len(cases), g.Open)
	}
}

// AC-4 (kept): a check-path finding that claims a false pass, an inverted
// meaning or a linked contract violation is a defect whatever it proposes.
func TestSparQuestionsAC4_ClaimedCheckFailureIsADefect(t *testing.T) {
	const id = "check-failure-claim-not-defect"
	w := newCPQWorld(t, true)
	cases := []cpqCase{
		{map[string]any{"proposed_class": "requirement-question", "check_impact": "false_pass"}, "defect"},
		{map[string]any{"proposed_class": "requirement-question", "check_impact": "meaning_inverted"}, "defect"},
		{map[string]any{"proposed_class": "requirement-question", "check_impact": "false_pass", "link_kind": "criterion", "link": "AC-1"}, "defect"},
		{map[string]any{"proposed_class": "requirement-question", "check_impact": "contract_violation", "link_kind": "rule", "link": "AGENTS.md:2"}, "defect"},
		{map[string]any{"proposed_class": "requirement-question", "check_impact": "contract_violation", "link_kind": "criterion", "link": "AC-1"}, "defect"},
		{map[string]any{"proposed_class": "note", "check_impact": "false_pass"}, "defect"},
		{map[string]any{"proposed_class": "hardening", "check_impact": "meaning_inverted"}, "defect"},
		{map[string]any{"proposed_class": "defect", "check_impact": "contract_violation", "link_kind": "contract", "link": "contracts/x.md:1"}, "defect"},
		{map[string]any{"proposed_class": "requirement-question", "check_impact": "contract_violation", "link_kind": "contract", "link": "checks/test_app.py:1"}, "defect"},
	}
	out, found, ledger := w.cpqClasses(cases)
	for i, c := range cases {
		f := found[fmt.Sprintf("F%d", i+1)]
		if !f.InCheckPaths || f.LocationUnreadable {
			t.Errorf("case %d %v: the setup does not give a check-path finding", i+1, c.values)
			continue
		}
		if f.Class != c.class || len(ledger[f.ID]) != 0 {
			cpqViolation(t, id, "case %d %v: class %s (%s), ledger %+v", i+1, c.values, f.Class, f.ClassReason, ledger[f.ID])
		}
	}
	if code, g := w.gate(out); code != ExitBlocked || g.Open["defect"] != len(cases) {
		cpqViolation(t, id, "gate %s (exit %d): open %v, want %d defects", g.State, code, g.Open, len(cases))
	}
}

// AC-5 (kept): any other check-path finding is a hardening item deferred
// automatically.
func TestSparQuestionsAC5_OtherCheckPathFindingsAreDeferred(t *testing.T) {
	const id = "check-path-finding-not-deferred"
	w := newCPQWorld(t, true)
	cases := []cpqCase{
		{map[string]any{"proposed_class": "defect", "link_kind": "criterion", "link": "AC-1"}, "hardening"},
		{map[string]any{"proposed_class": "defect"}, "hardening"},
		{map[string]any{"proposed_class": "note"}, "hardening"},
		{map[string]any{"proposed_class": "hardening"}, "hardening"},
		{map[string]any{"proposed_class": "note", "introduced_by_change": "yes", "regression": "yes"}, "hardening"},
		{map[string]any{"proposed_class": "note", "check_impact": "contract_violation", "link_kind": "rule", "link": "AGENTS.md:99"}, "hardening"},
		{map[string]any{"proposed_class": "defect", "check_impact": "contract_violation"}, "hardening"},
	}
	out, found, ledger := w.cpqClasses(cases)
	for i, c := range cases {
		f := found[fmt.Sprintf("F%d", i+1)]
		if !f.InCheckPaths || f.LocationUnreadable {
			t.Errorf("case %d %v: the setup does not give a check-path finding", i+1, c.values)
			continue
		}
		entries := ledger[f.ID]
		if f.Class != c.class || len(entries) != 1 || entries[0].Disposition != "defer" || !entries[0].Auto {
			cpqViolation(t, id, "case %d %v: class %s, ledger %+v", i+1, c.values, f.Class, entries)
		}
	}
	if code, g := w.gate(out); code != ExitOK || g.AutoDeferred != len(cases) {
		cpqViolation(t, id, "gate %s (exit %d): auto-deferred %d of %d", g.State, code, g.AutoDeferred, len(cases))
	}
}

// AC-6 (kept): findings outside the check paths and findings whose location
// cannot be read keep their classes, and neither is deferred automatically.
func TestSparQuestionsAC6_OtherFindingsKeepTheirClasses(t *testing.T) {
	const id = "other-class-changed"
	w := newCPQWorld(t, true)
	outside := func(values map[string]any) map[string]any {
		values["location"] = "head/app/main.py:1"
		return values
	}
	unreadable := func(values map[string]any) map[string]any {
		values["location"] = "head/checks/test_app.py:2:5"
		return values
	}
	cases := []cpqCase{
		{outside(map[string]any{"proposed_class": "defect", "link_kind": "criterion", "link": "AC-1"}), "defect"},
		{outside(map[string]any{"proposed_class": "defect"}), "note"},
		{outside(map[string]any{"proposed_class": "note", "introduced_by_change": "yes", "regression": "yes"}), "defect"},
		{outside(map[string]any{"proposed_class": "note"}), "note"},
		{outside(map[string]any{"proposed_class": "requirement-question"}), "requirement-question"},
		{outside(map[string]any{"proposed_class": "requirement-question", "link_kind": "criterion", "link": "AC-1"}), "requirement-question"},
		{outside(map[string]any{"proposed_class": "requirement-question", "link_kind": "contract", "link": "checks/test_app.py:1"}), "requirement-question"},
		{unreadable(map[string]any{"proposed_class": "defect"}), "defect"},
		{unreadable(map[string]any{"proposed_class": "defect", "check_impact": "false_pass"}), "defect"},
		{unreadable(map[string]any{"proposed_class": "note", "check_impact": "meaning_inverted"}), "defect"},
		{unreadable(map[string]any{"proposed_class": "note", "check_impact": "contract_violation", "link_kind": "rule", "link": "AGENTS.md:2"}), "defect"},
		{unreadable(map[string]any{"proposed_class": "note", "check_impact": "contract_violation"}), "note"},
		{unreadable(map[string]any{"proposed_class": "requirement-question"}), "requirement-question"},
		{unreadable(map[string]any{"proposed_class": "requirement-question", "link_kind": "criterion", "link": "AC-1"}), "requirement-question"},
		{unreadable(map[string]any{"proposed_class": "requirement-question", "link_kind": "contract", "link": "checks/test_app.py:1"}), "requirement-question"},
		{unreadable(map[string]any{"proposed_class": "note", "introduced_by_change": "yes", "regression": "yes"}), "defect"},
		{unreadable(map[string]any{"proposed_class": "hardening"}), "hardening"},
		{unreadable(map[string]any{"proposed_class": "note"}), "note"},
	}
	_, found, ledger := w.cpqClasses(cases)
	for i, c := range cases {
		f := found[fmt.Sprintf("F%d", i+1)]
		wantUnreadable := c.values["location"] != "head/app/main.py:1"
		if f.InCheckPaths || f.LocationUnreadable != wantUnreadable {
			t.Errorf("case %d %v: the setup does not give the location it names: in check paths %v, unreadable %v",
				i+1, c.values, f.InCheckPaths, f.LocationUnreadable)
			continue
		}
		if f.Class != c.class || len(ledger[f.ID]) != 0 {
			cpqViolation(t, id, "case %d %v: class %s (%s), ledger %+v", i+1, c.values, f.Class, f.ClassReason, ledger[f.ID])
		}
	}
}

// AC-7 (kept): a round recorded before this change, where a linked question
// at a check path was recorded as a hardening item and deferred, gives the
// same classes, ledger and gate when the gate is made again.
func TestSparQuestionsAC7_RecordedRoundsKeepTheirGate(t *testing.T) {
	const id = "recorded-round-reclassified"
	w := newCPQWorld(t, true)
	out := w.round(
		cpqFinding("F1", map[string]any{"proposed_class": "requirement-question", "location": "head/app/main.py:1"}),
		cpqFinding("F2", map[string]any{"proposed_class": "requirement-question", "link_kind": "criterion", "link": "AC-1"}))
	// The records as a round made before this change holds them: F2 is a
	// hardening item with its automatic deferral.
	var list []Finding
	data, err := os.ReadFile(filepath.Join(out, "findings.json"))
	if err != nil || json.Unmarshal(data, &list) != nil || len(list) != 2 {
		t.Fatalf("findings.json: %v %s", err, data)
	}
	reason := "check path finding without a false pass, inverted meaning or linked contract violation"
	list[1].Class, list[1].ClassReason = "hardening", reason
	if err := writeJSON(filepath.Join(out, "findings.json"), list); err != nil {
		t.Fatal(err)
	}
	ledgerPath := filepath.Join(out, "ledger.jsonl")
	if err := os.WriteFile(ledgerPath, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	entry := LedgerEntry{Finding: "host-F2", Disposition: "defer", Auto: true, Reason: "hardening: " + reason,
		To: "deferred hardening", RecordedAt: "2026-01-01T00:00:00Z"}
	if err := appendJSONLine(ledgerPath, entry); err != nil {
		t.Fatal(err)
	}
	read := func(name string) []byte {
		data, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return data
	}
	findingsBefore, ledgerBefore := read("findings.json"), read("ledger.jsonl")
	for i := 0; i < 2; i++ {
		code, g := w.gate(out)
		if code != ExitBlocked || g.State != StateBlocked || g.AutoDeferred != 1 || g.Open["defect"] != 0 ||
			g.Open["requirement-question"] != 1 || len(g.OpenFindings) != 1 || !strings.HasSuffix(g.OpenFindings[0], " host-F1") {
			cpqViolation(t, id, "gate %d: %s (exit %d), auto-deferred %d, open %v %v", i+1, g.State, code, g.AutoDeferred,
				g.Open, g.OpenFindings)
		}
	}
	if !bytes.Equal(read("findings.json"), findingsBefore) || !bytes.Equal(read("ledger.jsonl"), ledgerBefore) {
		cpqViolation(t, id, "making the gate changed the recorded findings or ledger")
	}
	if code, msg := w.spar("triage", out, "host-F1", "answer", "--user-quote", cpqQuote); code != ExitOK {
		t.Fatalf("answer: %d %s", code, msg)
	}
	if code, g := w.gate(out); code != ExitOK || g.State != StateComplete || g.AutoDeferred != 1 {
		cpqViolation(t, id, "after the answer: %s (exit %d), auto-deferred %d, reasons %v", g.State, code, g.AutoDeferred, g.Reasons)
	}
	if f := w.findings(out)["F2"]; f.Class != "hardening" {
		cpqViolation(t, id, "the recorded hardening item is now %s", f.Class)
	}
}

// cpqRead reads a repository file; a file that cannot be read is a broken
// setup, not an observation.
func cpqRead(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("%s cannot be read: %v", rel, err)
	}
	return string(data)
}

// AC-8: the public contract states the class rules in this order.
func TestSparQuestionsAC8_ContractStatesTheRules(t *testing.T) {
	const id = "contract-rules-stale"
	text := cpqRead(t, "contracts/review-round.md")
	want := []string{
		"1. 위치가 검사 경로이고 `check_impact`가 `false_pass`나 `meaning_inverted`이면 `defect`",
		"2. 위치가 검사 경로이고 `contract_violation`이며 연결이 확인되면 `defect`",
		"3. 위치가 검사 경로이면, `proposed_class`가 `requirement-question`이고 연결이 확인되며 그 연결이 검사 경로의 파일을 가리키는 `contract` 연결이 아닐 때 `requirement-question`, 그 밖에는 제안 분류와 관계없이 `hardening`",
		"4. `proposed_class`가 `requirement-question`이면 `requirement-question`",
		"5. `introduced_by_change`와 `regression`이 모두 `yes`이면 `defect`",
		"6. `proposed_class`가 `defect`이고 연결이 확인되지 않으면 `note`",
		"7. 그 밖에는 `proposed_class`",
	}
	const heading = "위치를 읽은 지적의 분류 규칙(위에서 처음 맞는 것):\n\n"
	_, rest, ok := strings.Cut(text, heading)
	if !ok {
		t.Fatalf("jaekit:violation=%s: contracts/review-round.md has no %q", id, strings.TrimSpace(heading))
	}
	list, _, _ := strings.Cut(rest, "\n\n")
	if got := strings.Split(list, "\n"); strings.Join(got, "\n") != strings.Join(want, "\n") {
		cpqViolation(t, id, "the class rules are\n%s", list)
	}
	for _, sentence := range []string{
		"규칙 3의 `requirement-question`은 자동 보류하지 않으며, 7절대로 사용자의 말을 인용한 처리로만 닫힌다.",
		"검사 경로의 파일을 가리키는 `contract` 연결은 검사 코드 자신을 가리키므로 규칙 3에서 확인된 연결로 치지 않는다. 규칙 2의 연결 확인에는 이 제한이 없다.",
		"분류는 지적을 기록할 때 정해진다. gate와 요약은 `findings.json`에 기록된 분류를 읽으므로, 이미 기록한 지적은 규칙이 바뀌어도 다시 분류되지 않는다.",
	} {
		if !strings.Contains(text, sentence) {
			cpqViolation(t, id, "contracts/review-round.md does not say %q", sentence)
		}
	}
}

// AC-8: the Spar skill describes the classes by the same rule.
func TestSparQuestionsAC8_SkillStatesTheClasses(t *testing.T) {
	const id = "skill-classes-stale"
	text := cpqRead(t, "plugins/spar/skills/spar/SKILL.md")
	for _, line := range []string{
		"- `requirement-question`: only a change of requirement resolves it. You cannot change a requirement; ask the user. A finding in a check path stays one only when it proposes `requirement-question` and links the criterion, rule or contract it asks about, and that link exists; a `contract` link to a file in the check paths does not count.",
		"- `hardening`: a check-path edge case that nothing requires, including a check-path requirement question without such a link. It is deferred automatically.",
	} {
		if !strings.Contains(text, "\n"+line+"\n") {
			cpqViolation(t, id, "SKILL.md has no line %q", line)
		}
	}
}

// AC-8: the reviewer packet tells reviewers that a requirement question at
// a check path stays a question only with a link that exists.
func TestSparQuestionsAC8_PacketExplainsTheLink(t *testing.T) {
	const id = "packet-guidance-missing"
	w := newCPQWorld(t, false)
	out := filepath.Join(w.repo, "reviews", "dry", "01-candidate")
	if code, msg := w.spar("run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", out,
		"--criteria", w.criteria, "--check-path", "checks/**", "--repo", w.repo, "--dry-run"); code != ExitOK {
		t.Fatalf("spar run --dry-run: %d %s", code, msg)
	}
	data, err := os.ReadFile(filepath.Join(out, "packet.md"))
	if err != nil {
		t.Fatalf("packet.md: %v", err)
	}
	line := "- Check paths: `checks/**`. At these paths a finding that claims `false_pass`, `meaning_inverted` or a linked `contract_violation` is recorded as a `defect`, whatever its `proposed_class`. Any other finding at these paths is recorded as `hardening` and deferred, unless it proposes `requirement-question` and links the criterion, rule or contract it asks about, and that link exists: such a question stays open for the user. A `contract` link to a file in these paths does not keep a question open."
	if !strings.Contains(string(data), "\n"+line+"\n") {
		cpqViolation(t, id, "packet.md has no line %q", line)
	}
}
