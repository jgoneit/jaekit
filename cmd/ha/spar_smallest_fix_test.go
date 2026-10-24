//go:build !windows

package main

// A check of the goal spar-smallest-fix: `ha cost` reads a Spar review
// series whose findings carry the reviewer's smallest fix direction
// (smallest_fix) and one written before that field existed, and gives both
// the same review and findings summary (contracts/review-round.md §5,
// contracts/run-record.md §1.7). The test builds spar from this tree, writes
// the series with a host reviewer whose answer it gives, and makes the older
// form by removing smallest_fix from findings.json. A failure that starts
// with "setup:" observes nothing and is a broken setup.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// sfhRun runs a program in dir; any exit but 0 or want is a broken setup.
func sfhRun(t *testing.T, dir string, env []string, want int, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	code := 0
	if exit, ok := err.(*exec.ExitError); ok {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("setup: %s %v: %v", name, args, err)
	}
	if code != 0 && code != want {
		t.Fatalf("setup: %s %v: exit %d: %s", name, args, code, out)
	}
}

func sfhWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// sfhCost runs ha cost --format json on one series and decodes it.
func sfhCost(t *testing.T, series string) map[string]any {
	t.Helper()
	var out, errs bytes.Buffer
	if code := runMain([]string{"cost", "docs/specs/demo", "--series", series, "--format", "json"}, &out, &errs); code != exitOK {
		t.Fatalf("AC-6: ha cost --series %s exits %d: %s", series, code, errs.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(bytes.ReplaceAll(out.Bytes(), []byte(series), []byte("<series>")), &doc); err != nil {
		t.Fatalf("AC-6: ha cost --format json printed no JSON object: %v", err)
	}
	return doc
}

// AC-6 (ha cost): a series with smallest_fix and the same series without it
// give the same review and findings summary, and the class counts are the
// ones findings.json holds.
func TestSparSmallestFixAC6_HaCostReadsBothForms(t *testing.T) {
	for _, tool := range []string{"git", "go"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Fatalf("setup: %s is not available: %v", tool, err)
		}
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	tools := t.TempDir()
	spar := filepath.Join(tools, "spar")
	build := exec.Command("go", "build", "-o", spar, "./cmd/spar")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("setup: go build ./cmd/spar: %v: %s", err, out)
	}
	config := filepath.Join(tools, "spar.toml")
	sfhWrite(t, config, "[[reviewers]]\nname = \"host\"\npreset = \"native\"\n")
	env := []string{"SPAR_CONFIG=" + config, "HOME=" + tools, "XDG_CONFIG_HOME=" + filepath.Join(tools, "config"),
		"GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=synthetic",
		"GIT_AUTHOR_EMAIL=synthetic@example.invalid", "GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid"}
	repo := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), env...)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("setup: git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	sfhWrite(t, filepath.Join(repo, ".gitignore"), "/docs/reviews/\n")
	sfhWrite(t, filepath.Join(repo, "docs/specs/demo/SPEC.md"), "# Demo\n\nStatus: Ready\nAssurance: standard\n\n## 목표\n\nA greeting.\n\n"+
		"## Acceptance Criteria\n- **AC-1** greet('') returns 'hello'.\n\n## Open Decisions\n\n없음\n")
	sfhWrite(t, filepath.Join(repo, "app/greet.py"), "def greet(name):\n    return 'hello ' + name\n")
	sfhWrite(t, filepath.Join(repo, "checks/test_greet.py"), "def test_greet():\n    assert True\n")
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-qm", "base")
	base := git("rev-parse", "HEAD")
	sfhWrite(t, filepath.Join(repo, "app/greet.py"), "def greet(name):\n    return 'hello, ' + name\n")
	git("commit", "-qam", "head")
	head := git("rev-parse", "HEAD")

	round := "docs/reviews/new/01-candidate"
	sfhRun(t, repo, env, 1, spar, "run", "--stage", "candidate", "--base", base, "--head", head, "--criteria", "docs/specs/demo/SPEC.md",
		"--check-path", "checks/**", "--out", round, "--repo", ".")
	var rec map[string]any
	data, err := os.ReadFile(filepath.Join(repo, round, "round.json"))
	if err != nil || json.Unmarshal(data, &rec) != nil {
		t.Fatalf("setup: round.json: %v", err)
	}
	checked := []any{}
	for _, angle := range rec["config"].(map[string]any)["angles"].([]any) {
		checked = append(checked, map[string]any{"angle": angle, "result": "synthetic check"})
	}
	finding := func(id, location, class, link, fix string) map[string]any {
		return map[string]any{"id": id, "proposed_class": class, "angle": "requirement-counterexample", "location": location,
			"claim": "synthetic claim " + id, "failure_scenario": "greet('') returns 'hello, '", "evidence": location,
			"link_kind": map[bool]string{true: "criterion", false: "none"}[link != ""], "link": link,
			"introduced_by_change": "yes", "regression": "no", "check_impact": "none", "confidence": "high", "smallest_fix": fix}
	}
	reply, _ := json.Marshal(map[string]any{"reviewed_head": head, "verdict": "blocking_findings", "angles_checked": checked,
		"angles_unchecked": []any{}, "limits": "synthetic reviewer", "findings": []any{
			finding("F1", "head/app/greet.py:2", "defect", "AC-1", "Drop the comma only when the name is empty."),
			finding("F2", "head/checks/test_greet.py:1", "hardening", "", ""),
			finding("F3", "head/app/greet.py:2", "defect", "", "Name the criterion."),
			finding("F4", "head/app/greet.py:2", "requirement-question", "AC-1", "Ask which greeting is right."),
		}})
	replyFile := filepath.Join(tools, "reply.json")
	sfhWrite(t, replyFile, string(reply))
	sfhRun(t, repo, env, 0, spar, "record", round, "host", "--result", replyFile)
	sfhRun(t, repo, env, 0, spar, "triage", round, "host-F1", "reject", "--evidence", "synthetic evidence")
	sfhRun(t, repo, env, 0, spar, "triage", round, "host-F4", "answer", "--user-quote", "synthetic answer")

	// The older form: the same series without smallest_fix.
	var findings []map[string]any
	data, err = os.ReadFile(filepath.Join(repo, round, "findings.json"))
	if err != nil || json.Unmarshal(data, &findings) != nil {
		t.Fatalf("setup: findings.json: %v", err)
	}
	directions := 0
	classes := map[string]float64{}
	for _, f := range findings {
		if s, _ := f["smallest_fix"].(string); s != "" {
			directions++
		}
		delete(f, "smallest_fix")
		classes[f["class"].(string)]++
	}
	if directions != 3 {
		t.Fatalf("setup: the spar built from this tree kept %d directions, want 3", directions)
	}
	if err := os.CopyFS(filepath.Join(repo, "docs/reviews/old"), os.DirFS(filepath.Join(repo, "docs/reviews/new"))); err != nil {
		t.Fatalf("setup: %v", err)
	}
	old, _ := json.MarshalIndent(findings, "", "  ")
	sfhWrite(t, filepath.Join(repo, "docs/reviews/old/01-candidate/findings.json"), string(old)+"\n")

	t.Chdir(repo)
	withDirections := sfhCost(t, "docs/reviews/new")
	without := sfhCost(t, "docs/reviews/old")
	for _, key := range []string{"review", "findings"} {
		if !reflect.DeepEqual(withDirections[key], without[key]) {
			t.Errorf("AC-6: ha cost %s differs between the series with smallest_fix and without it:\n%v\n%v", key, withDirections[key], without[key])
		}
	}
	got := map[string]float64{}
	if section, ok := withDirections["findings"].(map[string]any); ok {
		for _, item := range section["classes"].([]any) {
			c := item.(map[string]any)
			if total := c["total"].(float64); total > 0 {
				got[c["class"].(string)] = total
			}
		}
	}
	if !reflect.DeepEqual(got, classes) {
		t.Errorf("AC-6: ha cost counts the classes %v, findings.json holds %v", got, classes)
	}
}
