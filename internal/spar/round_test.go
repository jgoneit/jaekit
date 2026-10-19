//go:build !windows

package spar

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func gitRun(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

// A round with only a native reviewer named "host" runs no CLI: it waits
// for the host, records the reply under its own name, and the gate follows.
func TestNativeRoundRecordTriageAndGate(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	os.MkdirAll(repo, 0o755)
	gitRun(t, repo, "init", "-q")
	writeFile(t, filepath.Join(repo, ".gitignore"), "/reviews/\n")
	writeFile(t, filepath.Join(repo, "app.txt"), "one\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "base")
	base := gitRun(t, repo, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(repo, "app.txt"), "two\n")
	gitRun(t, repo, "commit", "-qam", "head")
	head := gitRun(t, repo, "rev-parse", "HEAD")
	criteria := writeFile(t, filepath.Join(dir, "SPEC.md"), "- **AC-1** two\n")
	config := writeFile(t, filepath.Join(dir, "spar.toml"), "[[reviewers]]\nname = \"host\"\npreset = \"native\"\n")
	getenv := env(map[string]string{"SPAR_CONFIG": config, "HOME": dir})
	out := filepath.Join(repo, "reviews", "demo", "01-candidate")
	opts := RunOptions{Repo: repo, Stage: "candidate", Base: base, Head: head, Out: out, Criteria: []string{criteria},
		Version: "test", Getenv: getenv}
	if code, err := Run(opts); err != nil || code != ExitOK {
		t.Fatalf("run: %d %v", code, err)
	}
	if _, err := os.Stat(filepath.Join(out, "host", "prompt.md")); err != nil {
		t.Fatalf("native prompt: %v", err)
	}
	reply := map[string]any{"reviewed_head": head, "verdict": "blocking_findings",
		"angles_checked":   []any{},
		"angles_unchecked": []any{},
		"findings":         []any{rawFinding("F1", map[string]any{"link_kind": "criterion", "link": "AC-1", "location": "head/app.txt:1"})},
		"limits":           ""}
	var checked []any
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "ok"})
	}
	reply["angles_checked"] = checked
	data, _ := json.Marshal(reply)
	replyFile := writeFile(t, filepath.Join(dir, "reply.json"), "```json\n"+string(data)+"\n```\n")
	if code, err := Record(RecordOptions{Round: out, Reviewer: "codex", Result: replyFile}); err == nil || code != ExitRefused {
		t.Fatalf("a reviewer that is not in the round was recorded: %d %v", code, err)
	}
	if code, err := Record(RecordOptions{Round: out, Reviewer: "host", Result: replyFile, Model: "m"}); err != nil || code != ExitOK {
		t.Fatalf("record: %d %v", code, err)
	}
	gateOut := filepath.Join(repo, "reviews", "demo", "gate.json")
	gate := func() (int, GateRecord) {
		t.Helper()
		code, err := Gate(GateOptions{Rounds: []string{out}, ExpectHead: head, Out: gateOut, Repo: repo})
		if err != nil {
			t.Fatal(err)
		}
		var g GateRecord
		raw, _ := os.ReadFile(gateOut)
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatal(err)
		}
		return code, g
	}
	if code, g := gate(); code != ExitBlocked || g.Open["defect"] != 1 {
		t.Fatalf("open defect: %d %+v", code, g.Open)
	}
	if err := Triage(TriageOptions{Round: out, Finding: "host-F1", Disposition: "reject", Evidence: "head/app.txt:1 is right"}); err != nil {
		t.Fatal(err)
	}
	if code, g := gate(); code != ExitOK || g.State != StateComplete || g.PreviousGateDigest == nil {
		t.Fatalf("after reject: %d %s %v", code, g.State, g.Reasons)
	}
	// A dry run counts toward nothing, records nothing and blocks a gate.
	dry := filepath.Join(repo, "reviews", "demo", "02-dry")
	opts.Out, opts.DryRun = dry, true
	if code, err := Run(opts); err != nil || code != ExitOK {
		t.Fatalf("dry run: %d %v", code, err)
	}
	if code, err := Record(RecordOptions{Round: dry, Reviewer: "host", Result: replyFile}); err == nil || code != ExitRefused {
		t.Fatalf("a dry run was recorded: %d %v", code, err)
	}
	// The code cap counts the first round only; a tracked record folder is refused.
	opts.DryRun = false
	opts.Out = filepath.Join(repo, "records", "03")
	if _, err := Run(opts); err == nil || !strings.Contains(err.Error(), "not ignored") {
		t.Fatalf("a tracked record folder was accepted: %v", err)
	}
}

// A defect closes through a duplicate only once the finding it duplicates
// is closed, and two duplicates of each other stay open.
func TestDuplicateChains(t *testing.T) {
	final := map[string]LedgerEntry{
		"a": {Finding: "a", Disposition: "duplicate", Of: "b"},
		"b": {Finding: "b", Disposition: "duplicate", Of: "a"},
		"c": {Finding: "c", Disposition: "duplicate", Of: "d"},
		"e": {Finding: "e", Disposition: "duplicate", Of: "f"},
		"f": {Finding: "f", Disposition: "reject", Evidence: "x"},
	}
	for id, want := range map[string]bool{"a": false, "b": false, "c": false, "e": true} {
		if _, done := resolveDuplicate(final, id); done != want {
			t.Errorf("%s: closed=%v", id, done)
		}
	}
	findings := []Finding{{ID: "x", Class: "defect"}, {ID: "y", Class: "note"}, {ID: "z", Class: "defect"}}
	if p, _ := dispositionProblem(findings[0], findings, TriageOptions{Disposition: "duplicate", Of: "y"}, "", ""); p == "" {
		t.Error("a defect closed as a duplicate of a note")
	}
	if p, _ := dispositionProblem(findings[0], findings, TriageOptions{Disposition: "duplicate", Of: "z"}, "", ""); p != "" {
		t.Errorf("a defect duplicating a defect was refused: %s", p)
	}
}

// --reviewers cannot shrink the required reviewers a gate expects.
func TestRequiredOfKeepsConfiguredReviewers(t *testing.T) {
	r := &RoundRecord{RequiredReviewers: []string{"codex"},
		Reviewers: map[string]*ReviewerState{"codex": {Preset: "codex", Required: true}},
		Config:    &Config{ConfiguredRequired: []RequiredRef{{"claude", "claude"}, {"codex", "codex"}}}}
	got := requiredOf(r)
	if len(got) != 2 || got[0].Name != "codex" || got[1].Name != "claude" {
		t.Fatalf("%+v", got)
	}
}

type fixture struct {
	t    *testing.T
	dir  string
	repo string
	revs []string
}

// newFixture makes a repository with three commits and an ignored reviews/.
func newFixture(t *testing.T) *fixture {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	f := &fixture{t: t, dir: t.TempDir()}
	f.repo = realish(filepath.Join(f.dir, "repo"))
	os.MkdirAll(f.repo, 0o755)
	gitRun(t, f.repo, "init", "-q")
	writeFile(t, filepath.Join(f.repo, ".gitignore"), "/reviews/\n")
	for i, body := range []string{"one\n", "one\ntwo\n", "one\ntwo\nthree\nfour\n"} {
		writeFile(t, filepath.Join(f.repo, "checks", "c.txt"), body)
		writeFile(t, filepath.Join(f.repo, "app.txt"), strings.Repeat("x\n", i+1))
		gitRun(t, f.repo, "add", "-A")
		gitRun(t, f.repo, "commit", "-qm", "c")
		f.revs = append(f.revs, gitRun(t, f.repo, "rev-parse", "HEAD"))
	}
	return f
}

func (f *fixture) round(name string, r *RoundRecord, findings []Finding, ledger []LedgerEntry) string {
	f.t.Helper()
	dir := filepath.Join(f.repo, "reviews", "s", name)
	os.MkdirAll(dir, 0o755)
	r.Schema, r.Series = RoundSchema, filepath.Dir(dir)
	if r.Config == nil {
		r.Config = &Config{Caps: Caps{1, 1, 3}, ConfiguredRequired: []RequiredRef{}}
	}
	if r.PacketSHA256 == "" {
		r.PacketSHA256 = "p"
	}
	if findings == nil {
		findings = []Finding{}
	}
	if err := writeJSON(filepath.Join(dir, "round.json"), r); err != nil {
		f.t.Fatal(err)
	}
	if err := writeJSON(filepath.Join(dir, "findings.json"), findings); err != nil {
		f.t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, "ledger.jsonl"), nil, 0o644)
	for _, e := range ledger {
		appendJSONLine(filepath.Join(dir, "ledger.jsonl"), e)
	}
	return dir
}

func (f *fixture) gate(expect string, rounds ...string) GateRecord {
	f.t.Helper()
	out := filepath.Join(f.repo, "reviews", "s", "gate.json")
	if _, err := Gate(GateOptions{Rounds: rounds, ExpectHead: expect, Out: out, Repo: f.repo}); err != nil {
		f.t.Fatal(err)
	}
	var g GateRecord
	data, _ := os.ReadFile(out)
	if err := json.Unmarshal(data, &g); err != nil {
		f.t.Fatal(err)
	}
	return g
}

func reviewer(preset string, required bool, status string) *ReviewerState {
	return &ReviewerState{Preset: preset, Required: required, Status: status}
}

func TestGateRequiredReviewersMissingRecordsAndGrowth(t *testing.T) {
	f := newFixture(t)
	base, head := f.revs[0], f.revs[1]
	both := &Config{Caps: Caps{1, 1, 3}, ConfiguredRequired: []RequiredRef{{"x", "codex"}, {"host", "native"}}}
	a := f.round("01-candidate", &RoundRecord{Stage: "candidate", Base: base, Head: head, Sequence: 1, Counted: true,
		RequiredReviewers: []string{"x", "host"}, Config: both, Reviewers: map[string]*ReviewerState{
			"x": reviewer("codex", true, StatusUnavailable), "host": reviewer("native", true, StatusCompleted)}}, nil, nil)
	// The same name as an unverified command does not complete the codex reviewer.
	b := f.round("02-candidate", &RoundRecord{Stage: "candidate", Base: base, Head: head, Sequence: 2,
		Config:    &Config{Caps: Caps{1, 1, 3}, ConfiguredRequired: []RequiredRef{{"host", "native"}}},
		Reviewers: map[string]*ReviewerState{"x": reviewer("command", false, StatusCompleted)}}, nil, nil)
	if g := f.gate(head, a, b); g.State != StateDegraded {
		t.Fatalf("a command reviewer credited a required codex reviewer: %s %v", g.State, g.Reasons)
	}
	// A rerun of the same reviewer and preset completes the packet.
	c := f.round("03-candidate", &RoundRecord{Stage: "candidate", Base: base, Head: head, Sequence: 3,
		RequiredReviewers: []string{"x"}, Config: both,
		Reviewers: map[string]*ReviewerState{"x": reviewer("codex", true, StatusCompleted)}}, nil, nil)
	if g := f.gate(head, a, c); g.State != StateComplete {
		t.Fatalf("a rerun did not complete the packet: %s %v", g.State, g.Reasons)
	}
	// --reviewers ran only one of the configured required reviewers.
	d := f.round("04-candidate", &RoundRecord{Stage: "candidate", Base: base, Head: head, Sequence: 4, PacketSHA256: "q",
		RequiredReviewers: []string{"host"}, Config: both,
		Reviewers: map[string]*ReviewerState{"host": reviewer("native", true, StatusCompleted)}}, nil, nil)
	if g := f.gate(head, d); g.State != StateDegraded || g.Rounds[0].RequiredReviewers != 2 {
		t.Fatalf("a reduced run looked complete: %s %+v", g.State, g.Rounds)
	}
	// A round without its findings file blocks the gate.
	os.Remove(filepath.Join(c, "findings.json"))
	if g := f.gate(head, a, c); g.State != StateBlocked {
		t.Fatalf("a round without findings.json: %s", g.State)
	}
	// Two fixes rounds of the same commits count their added lines once.
	one := &Config{Caps: Caps{1, 1, 3}, CheckPaths: []string{"checks/**"}, ConfiguredRequired: []RequiredRef{{"host", "native"}}}
	done := map[string]*ReviewerState{"host": reviewer("native", true, StatusCompleted)}
	e := f.round("05-candidate", &RoundRecord{Stage: "candidate", Base: base, Head: head, Sequence: 5, PacketSHA256: "r",
		RequiredReviewers: []string{"host"}, Config: one, Reviewers: done}, nil, nil)
	g1 := f.round("06-fixes", &RoundRecord{Stage: "fixes", Base: head, Head: f.revs[2], Sequence: 6, PacketSHA256: "s",
		RequiredReviewers: []string{"host"}, Config: one, Reviewers: done}, nil, nil)
	g2 := f.round("07-fixes", &RoundRecord{Stage: "fixes", Base: head, Head: f.revs[2], Sequence: 7, PacketSHA256: "s",
		RequiredReviewers: []string{"host"}, Config: one, Reviewers: done}, nil, nil)
	g := f.gate(f.revs[2], e, g1, g2)
	if g.Growth.AddedLines["check"] != 2 || g.Growth.AddedLines["other"] != 1 || !g.Growth.OverHardening {
		t.Fatalf("growth: %+v", g.Growth)
	}
	// Recording into a round whose findings file is gone is refused.
	r, _ := loadRound(c)
	if err := saveRound(c, r, nil, false); err == nil {
		t.Fatal("a round without findings.json was rewritten")
	}
}

func TestAdmitCreditsOnlyFinishedRounds(t *testing.T) {
	cfg := &Config{Caps: Caps{1, 1, 1}}
	running := &RoundRecord{Stage: "candidate", Base: "b", Head: "h", PacketSHA256: "p", Counted: true,
		Reviewers: map[string]*ReviewerState{"x": reviewer("codex", true, "running")}}
	next := &RoundRecord{Stage: "candidate", Base: "b", Head: "h", PacketSHA256: "p", Config: cfg, RequestedReviewers: []string{"x"}}
	if _, _, err := admit([]sibling{{"01", running}}, nil, next); err == nil {
		t.Fatal("a running round let the same packet run again uncounted past the cap")
	}
	running.FinishedAt = "t"
	running.Reviewers["x"].Status = StatusUnavailable
	if counted, rerun, err := admit([]sibling{{"01", running}}, nil, next); err != nil || counted || len(rerun) != 1 {
		t.Fatalf("a rerun of an unfinished reviewer: %v %v %v", counted, rerun, err)
	}
}

func TestExportWritesLinksAsFilesAndRecordsFollowTheirRepository(t *testing.T) {
	f := newFixture(t)
	os.Symlink("../../etc/passwd", filepath.Join(f.repo, "leak"))
	gitRun(t, f.repo, "add", "-A")
	gitRun(t, f.repo, "commit", "-qm", "link")
	ws := filepath.Join(f.dir, "ws")
	if err := exportTree(f.repo, "HEAD", ws); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(filepath.Join(ws, "leak"))
	data, _ := os.ReadFile(filepath.Join(ws, "leak"))
	if err != nil || !info.Mode().IsRegular() || string(data) != "../../etc/passwd" {
		t.Fatalf("link exported as %v %q %v", info, data, err)
	}
	other := filepath.Join(f.dir, "other")
	os.MkdirAll(filepath.Join(other, "tracked"), 0o755)
	gitRun(t, other, "init", "-q")
	writeFile(t, filepath.Join(other, ".gitignore"), "/ignored/\n")
	if err := localRecordPath(f.repo, filepath.Join(other, "tracked", "01")); err == nil {
		t.Fatal("a folder another repository would track was accepted")
	}
	if err := localRecordPath(f.repo, filepath.Join(other, "ignored", "01")); err != nil {
		t.Fatalf("an ignored folder of another repository: %v", err)
	}
	if err := localRecordPath(f.repo, filepath.Join(f.dir, "plain", "01")); err != nil {
		t.Fatalf("a folder outside any repository: %v", err)
	}
}
