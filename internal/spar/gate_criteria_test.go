package spar

// A check of the goal ha-review-gate: each round of a review-gate/v1 file
// names the criteria documents it received, by their document name (G26) and
// sha256, so `ha` can tell whether a round reviewed the current goal
// documents. The test drives the spar command line (Main) on a synthetic
// repository with a native reviewer whose answer it records. The opening line
// of an assertion's failure message names the property it observes
// (jaekit:violation=<id>); a failure without it is a broken setup.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type rgcWorld struct {
	t                 *testing.T
	dir, repo, config string
	base, head        string
	replies           int
}

func newRGCWorld(t *testing.T) *rgcWorld {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skipf("git is not available: %v", err)
	}
	w := &rgcWorld{t: t, dir: t.TempDir()}
	w.repo = realish(filepath.Join(w.dir, "repo"))
	w.write(filepath.Join(w.repo, ".gitignore"), "/reviews/\n/docs/\n")
	w.write(filepath.Join(w.repo, "app/main.py"), "print('one')\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app/main.py"), "print('two')\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	w.config = w.write(filepath.Join(w.dir, "spar.toml"), "[[reviewers]]\nname = \"host\"\npreset = \"native\"\n")
	return w
}

func (w *rgcWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	return path
}

func (w *rgcWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid", "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		w.t.Fatalf("setup: git %v: %v %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (w *rgcWorld) spar(args ...string) (int, string) {
	var stdout, stderr bytes.Buffer
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
	return code, stdout.String() + stderr.String()
}

// round runs a round with the given criteria documents and records the
// native reviewer's answer without findings.
func (w *rgcWorld) round(out, stage, head string, criteria ...string) {
	w.t.Helper()
	args := []string{"run", "--stage", stage, "--base", w.base, "--head", head, "--out", out, "--repo", w.repo}
	for _, c := range criteria {
		args = append(args, "--criteria", c)
	}
	if code, msg := w.spar(args...); code != ExitOK && code != 1 {
		w.t.Fatalf("setup: spar run: %d %s", code, msg)
	}
	var checked []any
	for _, a := range Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "checked"})
	}
	data, err := json.Marshal(map[string]any{"reviewed_head": head, "verdict": "no_blocking_findings", "angles_checked": checked,
		"angles_unchecked": []any{}, "findings": []any{}, "limits": ""})
	if err != nil {
		w.t.Fatalf("setup: %v", err)
	}
	w.replies++
	reply := w.write(filepath.Join(w.dir, "replies", fmt.Sprintf("%d.json", w.replies)), string(data)+"\n")
	if code, msg := w.spar("record", out, "host", "--result", reply); code != ExitOK {
		w.t.Fatalf("setup: spar record: %d %s", code, msg)
	}
}

type rgcDoc struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}

func rgcSHA(body string) string {
	sum := sha256.Sum256([]byte(body))
	return hex.EncodeToString(sum[:])
}

// AC-1: each gate round lists the criteria documents of its round by name and
// sha256, and nothing else of them: no source path.
func TestHaReviewGateAC1_SparGateNamesRoundCriteria(t *testing.T) {
	const v = "gate-round-criteria-missing"
	w := newRGCWorld(t)
	specPath := filepath.Join(w.repo, "docs", "specs", "demo", "SPEC.md")
	notes := filepath.Join(w.dir, "outside", "notes.md")
	earlier, current, note := "# Demo\n\n- **AC-1** one\n", "# Demo\n\n- **AC-1** one, changed\n", "outside notes\n"
	w.write(specPath, earlier)
	w.write(notes, note)
	series := filepath.Join(w.repo, "reviews", "demo")
	plan, candidate := filepath.Join(series, "01-plan"), filepath.Join(series, "02-candidate")
	w.round(plan, "plan", w.base, specPath)
	w.write(specPath, current)
	w.round(candidate, "candidate", w.head, specPath, notes)
	gatePath := filepath.Join(w.dir, "gate.json")
	if code, msg := w.spar("gate", plan, candidate, "--expect-head", w.head, "--out", gatePath, "--repo", w.repo); code != ExitOK {
		t.Fatalf("setup: spar gate: %d %s", code, msg)
	}
	data, err := os.ReadFile(gatePath)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var gate struct {
		Rounds []map[string]json.RawMessage `json:"rounds"`
	}
	if err := json.Unmarshal(data, &gate); err != nil || len(gate.Rounds) != 2 {
		t.Fatalf("setup: the gate file has no two rounds: %v", err)
	}
	want := [][]rgcDoc{
		{{"docs/specs/demo/SPEC.md", rgcSHA(earlier)}},
		{{"docs/specs/demo/SPEC.md", rgcSHA(current)}, {"notes.md", rgcSHA(note)}},
	}
	for i, round := range gate.Rounds {
		raw, ok := round["criteria"]
		var entries []map[string]any
		if ok {
			if err := json.Unmarshal(raw, &entries); err != nil {
				t.Errorf("jaekit:violation=%s round %d criteria is not a list of documents: %s", v, i+1, raw)
				continue
			}
		}
		var got []rgcDoc
		for _, e := range entries {
			if len(e) != 2 {
				t.Errorf("jaekit:violation=%s round %d criteria entry %v has other fields than name and sha256", v, i+1, e)
			}
			name, _ := e["name"].(string)
			sum, _ := e["sha256"].(string)
			got = append(got, rgcDoc{name, sum})
		}
		if !ok || !reflect.DeepEqual(got, want[i]) {
			t.Errorf("jaekit:violation=%s round %d names criteria %s; want %v", v, i+1, raw, want[i])
		}
	}
	if bytes.Contains(data, []byte(filepath.Dir(notes))) {
		t.Errorf("jaekit:violation=%s the gate file keeps the source path of a criteria document outside the repository", v)
	}
}
