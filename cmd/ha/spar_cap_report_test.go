//go:build !windows

package main

// Checks of the goal spar-cap-report: `ha cost` shows the cause and reason
// that `spar extend` records with a code cap extension, and says that an
// extension line without a cause, as spar wrote it before, has none
// (contracts/run-record.md §1.7). The AC-9 test builds spar from this tree
// and records the extensions with it, so the file ha reads is the one spar
// writes. The opening line of an assertion's failure message names the
// property it observes (jaekit:violation=<id>); a failure that starts with
// "setup:" is a broken setup, not an observation.

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func crhWrite(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("setup: %v", err)
	}
}

// crhRepo makes a repository with the goal docs/specs/demo whose review
// records go in the ignored docs/reviews/, and moves into it.
func crhRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("setup: git is not available: %v", err)
	}
	repo := t.TempDir()
	crhWrite(t, filepath.Join(repo, ".gitignore"), "/docs/reviews/\n")
	crhWrite(t, filepath.Join(repo, "docs/specs/demo/SPEC.md"), "# Demo\n\nStatus: Ready\nAssurance: standard\n\n## 목표\n\nA greeting.\n\n"+
		"## Acceptance Criteria\n- **AC-1** greet('') returns 'hello'.\n\n## Open Decisions\n\n없음\n")
	crhWrite(t, filepath.Join(repo, "app/greet.py"), "def greet(name):\n    return 'hello ' + name\n")
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"add", "-A"}, {"commit", "-qm", "base"}} {
		cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=synthetic",
			"GIT_AUTHOR_EMAIL=synthetic@example.invalid", "GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("setup: git %v: %v %s", args, err, out)
		}
	}
	t.Chdir(repo)
	return repo
}

// crhCost runs ha cost on the demo goal with one series.
func crhCost(t *testing.T, series string, args ...string) string {
	t.Helper()
	var out, errs bytes.Buffer
	if code := runMain(append([]string{"cost", "docs/specs/demo", "--series", series}, args...), &out, &errs); code != exitOK {
		t.Fatalf("setup: ha cost --series %s %v exits %d: %s", series, args, code, errs.String())
	}
	return out.String()
}

// crhQuestions decodes the questions section of ha cost --format json.
func crhQuestions(t *testing.T, series string) map[string]any {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal([]byte(crhCost(t, series, "--format", "json")), &doc); err != nil {
		t.Fatalf("setup: ha cost --format json printed no JSON object: %v", err)
	}
	q, _ := doc["questions"].(map[string]any)
	return q
}

// crhFollowed reports whether a line equal to first is directly followed by
// a line equal to next.
func crhFollowed(text, first, next string) bool {
	lines := strings.Split(text, "\n")
	for i := 0; i+1 < len(lines); i++ {
		if lines[i] == first && lines[i+1] == next {
			return true
		}
	}
	return false
}

func crhExpect(t *testing.T, v, what string, got, want any) {
	t.Helper()
	g, _ := json.Marshal(got)
	w, _ := json.Marshal(want)
	if !bytes.Equal(g, w) {
		t.Errorf("jaekit:violation=%s %s is %s, want %s", v, what, g, w)
	}
}

// AC-9: the cause and reason that this tree's spar extend records appear in
// ha cost's list of cap extensions, in English and Korean Markdown and in
// JSON; the count by source stays one per extension line.
func TestSparCapReportAC9_HaCostShowsCause(t *testing.T) {
	const v = "cost-cause-missing"
	if _, err := exec.LookPath("go"); err != nil {
		t.Fatalf("setup: go is not available: %v", err)
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
	repo := crhRepo(t)
	series := "docs/reviews/cause"
	for _, args := range [][]string{
		{"--by", "2", "--cause", "churn", "--reason", "two fixes touched greet.py", "--user-quote", "synthetic words one"},
		{"--by", "1", "--cause", "other", "--reason", "the reviewer quota ran out", "--user-quote", "synthetic words two"},
	} {
		cmd := exec.Command(spar, append(append([]string{"extend", series, "--stage", "code"}, args...), "--repo", ".")...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(), "HOME="+tools, "XDG_CONFIG_HOME="+filepath.Join(tools, "config"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("jaekit:violation=%s this tree's spar extend %v does not record the cause: %v %s", v, args, err, out)
		}
	}
	q := crhQuestions(t, series)
	crhExpect(t, v, "the cap extensions in JSON", q["extensions"], []any{
		map[string]any{"series": series, "stage": "code", "by": 2, "cause": "churn", "reason": "two fixes touched greet.py",
			"user_quote": "synthetic words one"},
		map[string]any{"series": series, "stage": "code", "by": 1, "cause": "other", "reason": "the reviewer quota ran out",
			"user_quote": "synthetic words two"},
	})
	if by, _ := q["by_source"].(map[string]any); by == nil || by["extension"] != float64(2) {
		t.Errorf("jaekit:violation=%s the extensions by source are %v, want 2", v, q["by_source"])
	}
	for _, c := range []struct{ lang, cause, reason string }{{"en", "cause", "reason"}, {"ko", "원인", "이유"}} {
		text := crhCost(t, series, "--lang", c.lang)
		for _, pair := range [][2]string{
			{"  - " + series + ": code +2: synthetic words one", "    - " + c.cause + ": churn; " + c.reason + ": two fixes touched greet.py"},
			{"  - " + series + ": code +1: synthetic words two", "    - " + c.cause + ": other; " + c.reason + ": the reviewer quota ran out"},
		} {
			if !crhFollowed(text, pair[0], pair[1]) {
				t.Errorf("jaekit:violation=%s ha cost --lang %s does not show %q under %q:\n%s", v, c.lang, pair[1], pair[0], text)
			}
		}
	}
}

// AC-10 (ha cost): extension lines without a cause, as spar wrote them
// before, keep their JSON form and say in Markdown that no cause was
// recorded.
func TestSparCapReportAC10_LinesWithoutCauseInHaCost(t *testing.T) {
	const v = "extension-compat-broken"
	crhRepo(t)
	series := "docs/reviews/old"
	crhWrite(t, filepath.Join(series, "extensions.jsonl"),
		`{"stage":"code","by":1,"user_quote":"synthetic words one","recorded_at":"2026-01-01T00:00:00Z"}`+"\n"+
			`{"stage":"plan","by":1,"user_quote":"synthetic words two","recorded_at":"2026-01-01T00:01:00Z"}`+"\n")
	q := crhQuestions(t, series)
	crhExpect(t, v, "extension lines without a cause in JSON", q["extensions"], []any{
		map[string]any{"series": series, "stage": "code", "by": 1, "user_quote": "synthetic words one"},
		map[string]any{"series": series, "stage": "plan", "by": 1, "user_quote": "synthetic words two"},
	})
	if by, _ := q["by_source"].(map[string]any); by == nil || by["extension"] != float64(2) {
		t.Errorf("jaekit:violation=%s the extensions by source are %v, want 2", v, q["by_source"])
	}
	for _, c := range []struct{ lang, none string }{{"en", "    - cause: not recorded"}, {"ko", "    - 원인: 기록 없음"}} {
		text := crhCost(t, series, "--lang", c.lang)
		for _, line := range []string{"  - " + series + ": code +1: synthetic words one", "  - " + series + ": plan +1: synthetic words two"} {
			if !crhFollowed(text, line, c.none) {
				t.Errorf("jaekit:violation=%s ha cost --lang %s does not show %q under %q:\n%s", v, c.lang, c.none, line, text)
			}
		}
	}
}
