//go:build !windows

package main

// A check of the goal spar-agent-model-choice: `ha cost` reads a Spar
// review series whose round records name where each reviewer's model and
// effort came from and the agent's values for the run, and one in the form
// spar wrote before those fields existed, and gives both the same review
// and findings summary (contracts/review-round.md, contracts/run-record.md
// §1.7). The test builds spar from this tree, runs a codex reviewer whose
// binary is a /bin/sh script printing the test's answer and a host
// reviewer whose answer it records, and makes the earlier form by removing
// the new fields from round.json. A failure that starts with "setup:"
// observes nothing and is a broken setup.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// amhCost runs ha cost --format json on one series and decodes it.
func amhCost(t *testing.T, series string) map[string]any {
	t.Helper()
	var out, errs bytes.Buffer
	if code := runMain([]string{"cost", "docs/specs/demo", "--series", series, "--format", "json"}, &out, &errs); code != exitOK {
		t.Fatalf("AC-10: ha cost --series %s exits %d: %s", series, code, errs.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(bytes.ReplaceAll(out.Bytes(), []byte(series), []byte("<series>")), &doc); err != nil {
		t.Fatalf("AC-10: ha cost --format json printed no JSON object: %v", err)
	}
	return doc
}

func TestSparAgentModelAC10_HaCostReadsBothForms(t *testing.T) {
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
	env := []string{"HOME=" + tools, "XDG_CONFIG_HOME=" + filepath.Join(tools, "config"),
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
	git("init", "-q", "-b", "main")
	git("add", "-A")
	git("commit", "-qm", "base")
	base := git("rev-parse", "HEAD")
	sfhWrite(t, filepath.Join(repo, "app/greet.py"), "def greet(name):\n    return 'hello, ' + name\n")
	git("commit", "-qam", "head")
	head := git("rev-parse", "HEAD")

	angles := []string{"requirement-counterexample", "side-effects", "design", "premises", "check-validity"}
	checked := []any{}
	for _, a := range angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	finding := map[string]any{"id": "F1", "proposed_class": "defect", "angle": "requirement-counterexample",
		"location": "head/app/greet.py:2", "claim": "synthetic claim", "failure_scenario": "greet('') returns 'hello, '",
		"evidence": "head/app/greet.py:2", "link_kind": "criterion", "link": "AC-1", "introduced_by_change": "yes",
		"regression": "no", "check_impact": "none", "confidence": "high"}
	answer, _ := json.Marshal(map[string]any{"reviewed_head": head, "verdict": "blocking_findings", "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": []any{finding}, "limits": "synthetic reviewer"})
	answerFile := filepath.Join(tools, "answer.json")
	sfhWrite(t, answerFile, string(answer))
	codex := filepath.Join(tools, "codex")
	sfhWrite(t, codex, "#!/bin/sh\nif [ \"$1\" = --version ]; then echo 'codex 0.0.0 (synthetic)'; exit 0; fi\n"+
		"cat >/dev/null\nout=\nfor a in \"$@\"; do if [ \"$prev\" = -o ]; then out=$a; fi; prev=$a; done\n"+
		"cat '"+answerFile+"' > \"$out\"\nprintf '%s\\n' '{\"type\":\"thread.started\"}' '{\"type\":\"turn.completed\",\"usage\":{}}'\n")
	if err := os.Chmod(codex, 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	config := filepath.Join(tools, "spar.toml")
	sfhWrite(t, config, "[[reviewers]]\nname = \"codex\"\npreset = \"codex\"\nbinary = \""+codex+"\"\n\n"+
		"[[reviewers]]\nname = \"host\"\npreset = \"native\"\n")
	env = append(env, "SPAR_CONFIG="+config)

	round := "docs/reviews/new/01-candidate"
	sfhRun(t, repo, env, 0, spar, "run", "--stage", "candidate", "--base", base, "--head", head, "--criteria", "docs/specs/demo/SPEC.md",
		"--out", round, "--repo", ".", "--model", "codex=agent-codex", "--effort", "codex=low")
	sfhRun(t, repo, env, 0, spar, "record", round, "host", "--result", answerFile)
	sfhRun(t, repo, env, 0, spar, "triage", round, "host-F1", "reject", "--evidence", "synthetic evidence")
	var rec map[string]any
	data, err := os.ReadFile(filepath.Join(repo, round, "round.json"))
	if err != nil || json.Unmarshal(data, &rec) != nil {
		t.Fatalf("setup: round.json: %v", err)
	}
	config0, _ := rec["config"].(map[string]any)
	if _, ok := config0["choices"]; !ok {
		t.Fatalf("AC-10: the spar built from this tree records no choices: %s", data)
	}
	// The earlier form: the same series without the fields this goal adds.
	added := []string{"model_source", "effort_source", "default_model", "default_effort", "allowed_models", "allowed_efforts"}
	delete(config0, "choices")
	for _, item := range config0["reviewers"].([]any) {
		for _, k := range added {
			delete(item.(map[string]any), k)
		}
	}
	for _, item := range rec["reviewers"].(map[string]any) {
		for _, k := range added {
			delete(item.(map[string]any), k)
		}
	}
	if err := os.CopyFS(filepath.Join(repo, "docs/reviews/old"), os.DirFS(filepath.Join(repo, "docs/reviews/new"))); err != nil {
		t.Fatalf("setup: %v", err)
	}
	old, _ := json.MarshalIndent(rec, "", "  ")
	sfhWrite(t, filepath.Join(repo, "docs/reviews/old/01-candidate/round.json"), string(old)+"\n")

	t.Chdir(repo)
	withSources := amhCost(t, "docs/reviews/new")
	without := amhCost(t, "docs/reviews/old")
	for _, key := range []string{"review", "findings"} {
		if withSources[key] == nil || !reflect.DeepEqual(withSources[key], without[key]) {
			t.Errorf("AC-10: ha cost %s differs between the series with model sources and without them:\n%v\n%v", key, withSources[key], without[key])
		}
	}
	// The fixture's own outcomes, so a change that alters both forms alike
	// is still seen: one counted candidate round whose two reviewers each
	// completed in one attempt without a fallback, and two defects, the
	// host's rejected and codex's without a disposition.
	var got []string
	review, _ := without["review"].(map[string]any)
	rounds, _ := review["rounds"].([]any)
	for _, r := range rounds {
		r, _ := r.(map[string]any)
		got = append(got, fmt.Sprintf("round %v %v counted=%v", r["round"], r["stage"], r["counted"]))
		reviewers, _ := r["reviewers"].([]any)
		for _, v := range reviewers {
			v, _ := v.(map[string]any)
			got = append(got, fmt.Sprintf("reviewer %v %v attempts=%v fallback=%v", v["name"], v["status"], v["attempts"], v["fallback_used"]))
		}
	}
	findings, _ := without["findings"].(map[string]any)
	classes, _ := findings["classes"].([]any)
	for _, c := range classes {
		if c, _ := c.(map[string]any); c["class"] == "defect" {
			d, _ := c["dispositions"].(map[string]any)
			got = append(got, fmt.Sprintf("defect total=%v reject=%v none=%v", c["total"], d["reject"], d["none"]))
		}
	}
	want := []string{"round 01-candidate candidate counted=true",
		"reviewer codex completed attempts=1 fallback=false", "reviewer host completed attempts=1 fallback=false",
		"defect total=2 reject=1 none=1"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("AC-10: ha cost on the series in the earlier form does not give the fixture's outcomes:\n got  %q\n want %q", got, want)
	}
}
