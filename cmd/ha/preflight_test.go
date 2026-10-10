package main

// These tests exercise the public command boundary and use only APIs that
// existed before preflight. This file alone can be overlaid on the base commit.
import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type preflightFixture struct {
	t      *testing.T
	dir    string
	marker string
}

func newPreflightFixture(t *testing.T) *preflightFixture {
	t.Helper()
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	f := &preflightFixture{t: t, dir: t.TempDir(), marker: filepath.Join(t.TempDir(), "executed")}
	copyTree(t, base, f.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	t.Setenv("HA_PREFLIGHT_MARKER", f.marker)
	f.put(".gitignore", "ignored/\n")
	f.put("tests/greeting.sh", "printf 'greeting\\n' >> \"$HA_PREFLIGHT_MARKER\"\n")
	f.put("tests/keep.sh", "printf 'keep\\n' >> \"$HA_PREFLIGHT_MARKER\"\n")
	f.put("docs/specs/other/SPEC.md", "# Other synthetic goal\n")
	f.put("docs/specs/other/PROGRESS.md", "# Other progress\n")
	plan := f.read(scenarioGoal + "/PLAN.md")
	f.put(scenarioGoal+"/PLAN.md", strings.Replace(string(plan), "| 예 | — |", "| 예 | tasks/T001.md |", 1))
	f.put(scenarioGoal+"/tasks/T001.md", "# T001\n\n## 완료 조건\n- `sh tests/keep.sh`\n")
	f.git("init", "-q", "-b", "main")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "synthetic base")
	t.Chdir(f.dir)
	if code, out := f.ha("start", scenarioGoal, "--request", "synthetic verification"); code != 0 {
		t.Fatalf("start: exit %d: %s", code, out)
	}
	return f
}

func (f *preflightFixture) put(name, text string) {
	f.t.Helper()
	p := filepath.Join(f.dir, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
		f.t.Fatal(err)
	}
}

func (f *preflightFixture) read(name string) []byte {
	f.t.Helper()
	out, err := os.ReadFile(filepath.Join(f.dir, filepath.FromSlash(name)))
	if err != nil {
		f.t.Fatal(err)
	}
	return out
}

func (f *preflightFixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = f.dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v: %s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func (f *preflightFixture) ha(args ...string) (int, string) {
	f.t.Helper()
	var out, stderr bytes.Buffer
	code := runMain(args, &out, &stderr)
	return code, out.String() + stderr.String()
}

func (f *preflightFixture) report() map[string]any {
	f.t.Helper()
	_, out := f.ha("status", scenarioGoal, "--format", "json")
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		f.t.Fatalf("status: %v: %s", err, out)
	}
	return report
}

func (f *preflightFixture) rules(rules string) {
	f.t.Helper()
	var start map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(f.read(scenarioGoal+"/runs.jsonl")), &start); err != nil {
		f.t.Fatal(err)
	}
	start["rules"] = rules
	data, err := json.Marshal(start)
	if err != nil {
		f.t.Fatal(err)
	}
	f.put(scenarioGoal+"/runs.jsonl", string(data)+"\n")
}

func (f *preflightFixture) logs() []string {
	f.t.Helper()
	gitDir := f.git("rev-parse", "--absolute-git-dir")
	var logs []string
	err := filepath.WalkDir(gitDir, func(name string, entry os.DirEntry, err error) error {
		if err == nil && !entry.IsDir() && strings.HasSuffix(name, ".log") {
			logs = append(logs, name)
		}
		return err
	})
	if err != nil {
		f.t.Fatal(err)
	}
	return logs
}

func TestCheckPreflightRefusesDirty(t *testing.T) {
	for _, target := range [][]string{{"AC-2"}, {"--baseline", "AC-1"}, {"T001"}} {
		t.Run(strings.Join(target, "-"), func(t *testing.T) {
			f := newPreflightFixture(t)
			f.put("src/keep.txt", "uncommitted\n")
			before := f.read(scenarioGoal + "/runs.jsonl")
			code, out := f.ha(append([]string{"check", scenarioGoal}, target...)...)
			if code != 65 || !strings.Contains(out, "refused before") {
				t.Fatalf("dirty check: exit %d: %s", code, out)
			}
			if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) || len(f.logs()) != 0 {
				t.Fatal("refusal created a check record or output log")
			}
			if _, err := os.Stat(f.marker); !os.IsNotExist(err) {
				t.Fatalf("refused process executed: %v", err)
			}
			if runs := f.report()["budget"].(map[string]any)["runs"]; runs != float64(0) {
				t.Fatalf("refusal consumed runs: %v", runs)
			}
		})
	}
	t.Run("git status failure", func(t *testing.T) {
		f := newPreflightFixture(t)
		realGit, err := exec.LookPath("git")
		if err != nil {
			t.Fatal(err)
		}
		bin := t.TempDir()
		wrapper := "#!/bin/sh\nif [ \"$1\" = status ]; then echo 'synthetic status failure' >&2; exit 81; fi\nexec \"$HA_REAL_GIT\" \"$@\"\n"
		if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755); err != nil {
			t.Fatal(err)
		}
		t.Setenv("HA_REAL_GIT", realGit)
		t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
		before := f.read(scenarioGoal + "/runs.jsonl")
		code, out := f.ha("check", scenarioGoal, "AC-2")
		if code != 70 || !strings.Contains(out, "cannot inspect working tree") {
			t.Fatalf("Git failure: exit %d: %s", code, out)
		}
		if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) || len(f.logs()) != 0 {
			t.Fatal("Git failure created a record or log")
		}
		if _, err := os.Stat(f.marker); !os.IsNotExist(err) {
			t.Fatal("Git failure was treated as permission to execute")
		}
	})
}

func TestDirtyDiagnostics(t *testing.T) {
	f := newPreflightFixture(t)
	outside := t.TempDir()
	link := filepath.Join(f.dir, "tracked-link")
	if err := os.Symlink("before", link); err != nil {
		t.Fatal(err)
	}
	// A local-only submodule fixture exercises Git's own submodule status rules.
	sub := &preflightFixture{t: t, dir: t.TempDir()}
	sub.git("init", "-q", "-b", "main")
	sub.put("file.txt", "base\n")
	sub.git("add", "-A")
	sub.git("commit", "-q", "-m", "submodule base")
	f.git("-c", "protocol.file.allow=always", "submodule", "add", "-q", sub.dir, "modules/local")
	f.git("add", "-A")
	f.git("commit", "-q", "-m", "synthetic links")
	if err := os.Remove(link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(f.dir, "dependencies")); err != nil {
		t.Fatal(err)
	}
	unusual := "local space\n[note].txt"
	f.put(unusual, "local\n")
	f.put("src/keep.txt", "modified\n")
	f.put("modules/local/file.txt", "submodule dirty\n")
	if err := os.Remove(filepath.Join(f.dir, "src/greeting.txt")); err != nil {
		t.Fatal(err)
	}
	f.git("mv", "lib/other.txt", "lib/renamed [one].txt")
	code, out := f.ha("check", scenarioGoal, "AC-2")
	if code != 65 {
		t.Fatalf("diagnostic check: exit %d: %s", code, out)
	}
	for _, want := range []string{strconv.Quote(unusual), `"dependencies" [untracked, symlink`,
		`"tracked-link" [tracked, symlink`, `"modules/local" [tracked, submodule`,
		`"src/greeting.txt" [tracked`, `"src/keep.txt" [tracked`,
		`"lib/renamed [one].txt"`, `(from "lib/other.txt")`, `ignore rules for "dependencies"`} {
		if !strings.Contains(out, want) {
			t.Errorf("diagnostic lacks %q: %s", want, out)
		}
	}
	if strings.Contains(out, outside) {
		t.Fatal("diagnostic exposed a symlink's external target")
	}
	report := f.report()
	found := false
	for _, value := range report["reasons"].([]any) {
		reason := value.(map[string]any)
		if reason["code"] != "worktree_dirty" {
			continue
		}
		found = true
		paths := reason["paths"].([]any)
		if len(paths) != 7 || !strings.Contains(reason["detail"].(string), "untracked, symlink") {
			t.Fatalf("status lost path details: %#v", reason)
		}
	}
	if !found {
		t.Fatal("status omitted worktree_dirty")
	}
	_, markdown := f.ha("status", scenarioGoal)
	if strings.Contains(markdown, unusual) || !strings.Contains(markdown, strconv.Quote(unusual)) {
		t.Fatal("Markdown diagnostic did not quote a newline in a filename")
	}
}

func TestCheckPreflightStopsDirtyBatch(t *testing.T) {
	for _, result := range []string{"0", "1"} {
		t.Run("first exit "+result, func(t *testing.T) {
			f := newPreflightFixture(t)
			// This assertion preserves the legacy exit-code interpretation.
			// The /3 partial-batch case lives in baseline_budget_test.go.
			f.rules("run-rules/2")
			f.put("tests/greeting.sh", "printf 'first\\n' >> \"$HA_PREFLIGHT_MARKER\"\nprintf dirty > became-dirty.txt\nexit "+result+"\n")
			f.git("add", "tests/greeting.sh")
			f.git("commit", "-q", "-m", "synthetic dirty producer")
			code, out := f.ha("check", scenarioGoal, "AC-1", "AC-2")
			if code != 65 || !strings.Contains(out, "1/2 targets executed, 1 not run") {
				t.Fatalf("batch: exit %d: %s", code, out)
			}
			lines := bytes.Split(bytes.TrimSpace(f.read(scenarioGoal+"/runs.jsonl")), []byte("\n"))
			if len(lines) != 2 || len(f.logs()) != 1 {
				t.Fatalf("batch did not preserve exactly the first run: records %d, logs %d", len(lines), len(f.logs()))
			}
			var record map[string]any
			if err := json.Unmarshal(lines[1], &record); err != nil {
				t.Fatal(err)
			}
			want := map[string]string{"0": "pass", "1": "fail"}[result]
			if record["result"] != want || record["tree_clean"] != true {
				t.Fatalf("first execution was reclassified: %#v", record)
			}
			marker, err := os.ReadFile(f.marker)
			if err != nil || string(marker) != "first\n" {
				t.Fatalf("later process executed: %q, %v", marker, err)
			}
			if runs := f.report()["budget"].(map[string]any)["runs"]; runs != float64(1) {
				t.Fatalf("partial batch usage = %v", runs)
			}
		})
	}
}

func TestCheckPreflightPreservesExclusions(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2"} {
		t.Run(rules, func(t *testing.T) {
			f := newPreflightFixture(t)
			f.rules(rules)
			f.put("ignored/local.txt", "ignored\n")
			f.put(scenarioGoal+"/PROGRESS.md", "# Progress\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | todo | local |\n")
			if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
				t.Fatalf("own documents/ignored files blocked: exit %d: %s", code, out)
			}
			f.put("docs/specs/other/PROGRESS.md", "# Changed bookkeeping\n")
			code, out := f.ha("check", scenarioGoal, "AC-2")
			want := map[string]int{"run-rules/1": 65, "run-rules/2": 0}[rules]
			if code != want {
				t.Fatalf("sibling exclusion under %s: exit %d, want %d: %s", rules, code, want, out)
			}
		})
	}
	t.Run("linked worktree", func(t *testing.T) {
		f := newPreflightFixture(t)
		linked := filepath.Join(t.TempDir(), "linked")
		f.git("worktree", "add", "--detach", "-q", linked, "HEAD")
		f.dir = linked
		t.Chdir(linked)
		if code, out := f.ha("start", scenarioGoal, "--request", "linked synthetic goal"); code != 0 {
			t.Fatalf("linked start: exit %d: %s", code, out)
		}
		if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
			t.Fatalf("linked check: exit %d: %s", code, out)
		}
		f.put("src/keep.txt", "dirty\n")
		if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 65 {
			t.Fatalf("linked dirty check: exit %d: %s", code, out)
		}
	})
	for _, rules := range []string{"run-rules/1", "run-rules/2"} {
		t.Run("historical dirty record "+rules, func(t *testing.T) {
			f := newPreflightFixture(t)
			f.rules(rules)
			if code, out := f.ha("check", scenarioGoal, "AC-2"); code != 0 {
				t.Fatalf("synthetic historical run: exit %d: %s", code, out)
			}
			lines := bytes.Split(bytes.TrimSpace(f.read(scenarioGoal+"/runs.jsonl")), []byte("\n"))
			// This is a synthetic legacy fixture, not a claim about a real past run.
			lines[1] = bytes.Replace(lines[1], []byte(`"tree_clean":true`), []byte(`"tree_clean":false`), 1)
			f.put(scenarioGoal+"/runs.jsonl", string(bytes.Join(lines, []byte("\n")))+"\n")
			before := f.read(scenarioGoal + "/runs.jsonl")
			report := f.report()
			if report["budget"].(map[string]any)["runs"] != float64(1) {
				t.Fatal("legacy dirty run disappeared from usage")
			}
			for _, value := range report["criteria"].([]any) {
				criterion := value.(map[string]any)
				if criterion["id"] == "AC-2" && criterion["satisfied"] != false {
					t.Fatal("legacy dirty run became completion evidence")
				}
			}
			f.put("src/keep.txt", "dirty\n")
			if code, _ := f.ha("check", scenarioGoal, "AC-2"); code != 65 {
				t.Fatalf("dirty followup: exit %d", code)
			}
			if !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) {
				t.Fatal("new refusal changed historical evidence")
			}
		})
	}
}
