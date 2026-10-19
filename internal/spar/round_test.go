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
