package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/status"
)

const pathScenarioDeclaration = "checks/증거 경로/declaration.json"
const pathScenarioProducer = "checks/증거 경로/producer.go"

type evidencePathCase struct {
	ID   string `json:"id"`
	Path string `json:"path"`
}

type evidencePathScenario struct {
	*preflightFixture
	binary  string
	gitDir  string
	outside string
	plan    string
}

// This entrypoint also runs on Unix. Windows-specific cases have a separate
// build-constrained entrypoint, so no runtime skip can masquerade as coverage.
func TestNativeEvidencePathScenarios(t *testing.T) {
	runEvidencePathScenarios(t, "portable")
}

func runEvidencePathScenarios(t *testing.T, group string) {
	t.Helper()
	fixture, err := filepath.Abs(filepath.Join(scenarioRoot, "_native", "windows-evidence-paths"))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(fixture, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var groups map[string][]evidencePathCase
	if err := json.Unmarshal(data, &groups); err != nil || len(groups[group]) == 0 {
		t.Fatalf("missing scenario group %q: %v", group, err)
	}
	s := newEvidencePathScenario(t, fixture)
	t.Run("ordinary-paths-before", func(t *testing.T) { s.assertEvidence(t, 2) })
	for _, item := range groups[group] {
		path := item.Path
		switch path {
		case "$OUTSIDE":
			path = filepath.ToSlash(s.outside)
		case "$DRIVE_RELATIVE":
			volume := filepath.VolumeName(s.outside)
			if len(volume) != 2 || volume[1] != ':' {
				t.Fatalf("Windows drive fixture has no native drive: %q", volume)
			}
			path = volume + "outside.txt"
		}
		for _, location := range []string{"declaration", "producer"} {
			t.Run(item.ID+"/"+location, func(t *testing.T) {
				s.put(scenarioGoal+"/PLAN.md", s.plan)
				s.writeDeclaration(pathScenarioProducer)
				wantReason := "invalid evidence path"
				if location == "declaration" {
					old := "| AC-1 | " + pathScenarioDeclaration + " |"
					s.put(scenarioGoal+"/PLAN.md", strings.Replace(s.plan, old, "| AC-1 | "+path+" |", 1))
				} else {
					s.writeDeclaration(path)
					wantReason = "invalid or duplicate evidence producer path"
				}
				// A committed malformed declaration must fail on path semantics,
				// not on dirty/untracked evidence or generic worktree preflight.
				s.git("add", pathScenarioDeclaration)
				s.git("commit", "-qm", "synthetic unsafe path", "--allow-empty")
				for _, baseline := range []bool{false, true} {
					t.Run(fmt.Sprintf("baseline=%v", baseline), func(t *testing.T) {
						beforeStatus := s.status(t)
						before := s.snapshot(t)
						args := []string{"check", scenarioGoal, "AC-1"}
						if baseline {
							args = append(args, "--baseline")
						}
						code, out := s.command(t, args...)
						if code != 65 || !strings.Contains(out, "evidence_invalid") || !strings.Contains(out, wantReason) {
							t.Fatalf("unsafe %s path %q: exit %d, want evidence_invalid/65 and %q:\n%s", location, path, code, wantReason, out)
						}
						if after := s.snapshot(t); !reflect.DeepEqual(before, after) {
							t.Fatal("refusal changed records, logs, producer marker, source, external data, or Git metadata")
						}
						if afterStatus := s.status(t); !bytes.Equal(beforeStatus, afterStatus) {
							t.Fatalf("refusal changed deterministic status/counters:\nbefore: %s\nafter: %s", beforeStatus, afterStatus)
						}
					})
				}
			})
		}
	}
	// Successful recording after all refusals rules out a blanket rejection
	// or hidden damage to the earlier records and producer inputs.
	s.put(scenarioGoal+"/PLAN.md", s.plan)
	s.writeDeclaration(pathScenarioProducer)
	s.git("add", pathScenarioDeclaration)
	s.git("commit", "-qm", "restore ordinary evidence paths", "--allow-empty")
	t.Run("ordinary-paths-after", func(t *testing.T) {
		s.must(t, 0, "check", scenarioGoal, "AC-1", "--baseline")
		s.must(t, 0, "check", scenarioGoal, "AC-1")
		s.assertEvidence(t, 4)
	})
}

func newEvidencePathScenario(t *testing.T, fixture string) *evidencePathScenario {
	t.Helper()
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	binary := filepath.Join(t.TempDir(), "ha")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", binary, ".").CombinedOutput(); err != nil {
		t.Fatalf("build actual ha command: %v: %s", err, out)
	}
	s := &evidencePathScenario{
		preflightFixture: &preflightFixture{t: t, dir: filepath.Join(root, "프로젝트 space"), marker: filepath.Join(root, "executed")},
		binary:           binary, outside: filepath.Join(root, "outside.txt"),
	}
	copyTree(t, base, s.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	t.Setenv("HA_PATH_SCENARIO_MARKER", s.marker)
	if err := os.WriteFile(s.outside, []byte("outside data must stay unchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	producer, err := os.ReadFile(filepath.Join(fixture, "producer.go"))
	if err != nil {
		t.Fatal(err)
	}
	s.put(".gitattributes", "* -text\n")
	s.put(pathScenarioProducer, string(producer))
	s.writeDeclaration(pathScenarioProducer)
	p := string(s.read(scenarioGoal + "/PLAN.md"))
	p = strings.Replace(p, "`sh tests/greeting.sh` | tests/greeting.sh", "`go run '"+pathScenarioProducer+"'` | "+pathScenarioProducer+", "+pathScenarioDeclaration, 1)
	p = strings.Replace(p, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`", 1)
	p += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | " + pathScenarioDeclaration + " |\n"
	s.plan = p
	s.put(scenarioGoal+"/PLAN.md", p)
	s.git("init", "-q", "-b", "main")
	s.git("add", "-A")
	s.git("commit", "-qm", "synthetic path scenario base")
	s.gitDir = s.git("rev-parse", "--absolute-git-dir")
	s.must(t, 0, "start", scenarioGoal, "--request", "synthetic evidence-path scenario")
	s.must(t, 0, "check", scenarioGoal, "AC-1", "--baseline")
	s.put("src/greeting.txt", "hello\n")
	s.git("add", "src/greeting.txt")
	s.git("commit", "-qm", "synthetic greeting change")
	s.must(t, 0, "check", scenarioGoal, "AC-1")
	return s
}

func (s *evidencePathScenario) writeDeclaration(producer string) {
	s.t.Helper()
	d := map[string]any{
		"schema": "check-declaration/v1", "producer": "synthetic", "producer_version": "1",
		"producer_paths": []string{producer},
		"targets":        []map[string]string{{"id": "greeting", "violation": "missing-greeting"}},
	}
	raw, err := json.Marshal(d)
	if err != nil {
		s.t.Fatal(err)
	}
	s.put(pathScenarioDeclaration, string(raw)+"\n")
}

func (s *evidencePathScenario) command(t *testing.T, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.binary, args...)
	cmd.Dir = s.dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("ha %v timed out: %s", args, out)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatalf("ha %v failed to start: %v", args, err)
	}
	return 0, string(out)
}

func (s *evidencePathScenario) must(t *testing.T, want int, args ...string) {
	t.Helper()
	if code, out := s.command(t, args...); code != want {
		t.Fatalf("ha %v: exit %d, want %d: %s", args, code, want, out)
	}
}

func (s *evidencePathScenario) status(t *testing.T) []byte {
	t.Helper()
	code, out := s.command(t, "status", scenarioGoal, "--format", "json")
	var rep status.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil || code != rep.ExitCode {
		t.Fatalf("status: exit %d: %v: %s", code, err, out)
	}
	return []byte(out)
}

func (s *evidencePathScenario) snapshot(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	for label, root := range map[string]string{"project": s.dir, "ha": filepath.Join(s.gitDir, "ha")} {
		err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == ".git" {
				return filepath.SkipDir
			}
			rel, err := filepath.Rel(root, p)
			if err != nil {
				return err
			}
			key := label + "/" + filepath.ToSlash(rel)
			if d.IsDir() {
				out[key] = "directory"
				return nil
			}
			data, err := os.ReadFile(p)
			out[key] = string(data)
			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for label, name := range map[string]string{
		"outside": s.outside, "producer-marker": s.marker,
		"git-index": filepath.Join(s.gitDir, "index"), "git-config": filepath.Join(s.gitDir, "config"),
		"git-head": filepath.Join(s.gitDir, "HEAD"),
	} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		out[label] = string(data)
	}
	return out
}

func (s *evidencePathScenario) assertEvidence(t *testing.T, runs int) {
	t.Helper()
	var rep status.Report
	if err := json.Unmarshal(s.status(t), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Budget == nil || rep.Budget.Runs != runs || len(rep.Criteria) < 1 {
		t.Fatalf("normal evidence did not consume %d runs: %+v", runs, rep.Budget)
	}
	if a := rep.Criteria[0].Attempts; a.Checks != runs/2 || a.Baselines != runs/2 || a.Errors != 0 || a.Unknowns != 0 || a.BaselineErrors != 0 || a.BaselineUnknowns != 0 {
		t.Fatalf("normal producer was not recorded as baseline and check: %+v", a)
	}
	marker, err := os.ReadFile(s.marker)
	if err != nil || string(marker) != strings.Repeat("executed\n", runs) {
		t.Fatalf("producer execution count differs: %q: %v", marker, err)
	}
	lines := bytes.Split(bytes.TrimSpace(s.read(scenarioGoal+"/runs.jsonl")), []byte("\n"))
	if len(lines) != runs+1 {
		t.Fatalf("expected start and %d evidence records, got %d", runs, len(lines))
	}
	for i, line := range lines[1:] {
		var row struct {
			Kind         string          `json:"kind"`
			Result       string          `json:"result"`
			Evidence     json.RawMessage `json:"evidence"`
			OutputPath   string          `json:"output_path"`
			OutputDigest string          `json:"output_digest"`
		}
		if err := json.Unmarshal(line, &row); err != nil {
			t.Fatal(err)
		}
		wantKind, wantResult := "check", "pass"
		if i%2 == 0 {
			wantKind, wantResult = "baseline", "fail_as_expected"
		}
		if row.Kind != wantKind || row.Result != wantResult || len(row.Evidence) == 0 || string(row.Evidence) == "null" || !strings.HasPrefix(row.OutputPath, ".git/ha/") {
			t.Fatalf("normal structured evidence record is invalid: %s", line)
		}
		data, err := os.ReadFile(filepath.Join(s.gitDir, filepath.FromSlash(strings.TrimPrefix(row.OutputPath, ".git/"))))
		if err != nil {
			t.Fatal(err)
		}
		h := sha256.Sum256(data)
		if row.OutputDigest != hex.EncodeToString(h[:]) || !bytes.Contains(data, []byte("synthetic evidence-path scenario output")) {
			t.Fatal("recorded producer log bytes or digest differ")
		}
	}
}
