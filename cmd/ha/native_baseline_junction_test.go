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

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/status"
)

type baselineJunctionScenario struct {
	*preflightFixture
	binary, shim, realGit, gitDir, outside, ledger, mode string
}

type junctionFileSnapshot struct {
	Bytes string
	Mode  fs.FileMode
}

type junctionObservation struct {
	Tree    string `json:"tree"`
	Ordinal int    `json:"ordinal"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
	OS      string `json:"os"`
}

func runBaselineJunctionScenarios(t *testing.T) {
	t.Helper()
	fixture, err := filepath.Abs(filepath.Join(scenarioRoot, "_native", "windows-baseline-junction"))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(fixture, "cases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []string
	wanted := []string{"regular-before", "parent-junction", "leaf-junction", "partial-batch", "regular-after"}
	if err := json.Unmarshal(raw, &cases); err != nil || !reflect.DeepEqual(cases, wanted) {
		t.Fatalf("native baseline cases differ: %v: %v", cases, err)
	}
	s := newBaselineJunctionScenario(t, fixture)
	for _, name := range cases {
		t.Run(name, func(t *testing.T) {
			beforeStatus := s.status(t)
			before := s.snapshot(t)
			beforeRecords := s.read(scenarioGoal + "/runs.jsonl")
			beforeMarker := s.markerBytes(t)
			mode, expected := "regular", []string{"regular"}
			args := []string{"check", scenarioGoal, "--baseline", "AC-1"}
			switch name {
			case "parent-junction":
				mode, expected = "parent", []string{"parent"}
			case "leaf-junction":
				mode, expected = "leaf", []string{"leaf"}
			case "partial-batch":
				mode, expected = "batch", []string{"regular", "parent"}
				args = append(args, "AC-1", "AC-1")
			}
			code, out := s.invoke(t, mode, args...)
			s.assertWorktrees(t, expected)
			if mode == "regular" {
				if code != 0 || !strings.Contains(out, "baseline fail_as_expected") {
					t.Fatalf("safe baseline: exit %d: %s", code, out)
				}
				log := s.assertOneAttempt(t, beforeRecords, beforeMarker, beforeStatus)
				s.assertPriorFiles(t, before, log)
				return
			}
			progress := "0/1 targets executed, 1 not run"
			if mode == "batch" {
				progress = "1/3 targets executed, 2 not run"
			}
			component := "overlay"
			if mode == "leaf" {
				component = "overlay/data.txt"
			}
			diagnostic := fmt.Sprintf("check path %q: baseline component %q is a symlink or irregular entry", "overlay/data.txt", component)
			for _, reason := range []string{"before execution: baseline preparation for AC-1", diagnostic, progress} {
				if code != 65 || !strings.Contains(out, reason) {
					t.Fatalf("actual baseline junction refusal lacks %q: exit %d: %s", reason, code, out)
				}
			}
			if mode == "batch" {
				log := s.assertOneAttempt(t, beforeRecords, beforeMarker, beforeStatus)
				s.assertPriorFiles(t, before, log)
			} else {
				if after := s.snapshot(t); !reflect.DeepEqual(before, after) {
					t.Fatal("junction refusal changed source/index, external bytes/permissions, records, logs, or producer marker")
				}
				if after := s.status(t); !reflect.DeepEqual(beforeStatus, after) {
					t.Fatalf("unattempted junction changed deterministic status: before %+v after %+v", beforeStatus, after)
				}
			}
		})
	}
}

func newBaselineJunctionScenario(t *testing.T, fixture string) *baselineJunctionScenario {
	t.Helper()
	realGit, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	realGit, err = filepath.Abs(realGit)
	if err != nil {
		t.Fatal(err)
	}
	root, bin := t.TempDir(), t.TempDir()
	suffix := ""
	if runtime.GOOS == "windows" {
		suffix = ".exe"
	}
	binary, helper := filepath.Join(bin, "ha"+suffix), filepath.Join(bin, "fixture"+suffix)
	for _, build := range [][2]string{{binary, "."}, {helper, filepath.Join(fixture, "fixture.go")}} {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		out, err := exec.CommandContext(ctx, "go", "build", "-o", build[0], build[1]).CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("build actual command/fixture: %v: %s", err, out)
		}
	}
	shim := filepath.Join(bin, "git"+suffix)
	data, err := os.ReadFile(helper)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shim, data, 0o700); err != nil {
		t.Fatal(err)
	}
	s := &baselineJunctionScenario{
		preflightFixture: &preflightFixture{t: t, dir: filepath.Join(root, "프로젝트 space"), marker: filepath.Join(root, "producer.jsonl")},
		binary:           binary, shim: bin, realGit: realGit, outside: filepath.Join(root, "외부 target"), ledger: filepath.Join(root, "worktrees.jsonl"),
	}
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
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
	if err := os.Mkdir(s.outside, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.outside, "data.txt"), []byte("external sentinel must remain unchanged\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.put(".gitattributes", "* -text\n")
	data, err = os.ReadFile(filepath.Join(fixture, "fixture.go"))
	if err != nil {
		t.Fatal(err)
	}
	s.put("checks/fixture.go", string(data))
	s.put("checks/declaration.json", `{"schema":"check-declaration/v1","producer":"synthetic-junction","producer_version":"1","producer_paths":["checks/fixture.go"],"targets":[{"id":"greeting","violation":"missing-greeting"}]}`)
	s.put("overlay/data.txt", "base overlay\n")
	command := fmt.Sprintf("'%s' produce", filepath.ToSlash(helper))
	plan := string(s.read(scenarioGoal + "/PLAN.md"))
	plan = strings.Replace(plan, "`sh tests/greeting.sh` | tests/greeting.sh", "`"+command+"` | checks/fixture.go, checks/declaration.json, overlay/data.txt", 1)
	plan = strings.Replace(plan, "`src/**`, `tests/**`", "`src/**`, `tests/**`, `checks/**`, `overlay/**`", 1)
	plan += "\n## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/declaration.json |\n"
	s.put(scenarioGoal+"/PLAN.md", plan)
	s.git("init", "-q", "-b", "main")
	s.git("add", "-A")
	s.git("commit", "-qm", "synthetic baseline junction base")
	s.gitDir = s.git("rev-parse", "--absolute-git-dir")
	s.must(t, "regular", "start", scenarioGoal, "--request", "synthetic native baseline junction scenario")
	s.put("src/greeting.txt", "hello\n")
	s.put("overlay/data.txt", "head overlay\n")
	s.git("add", "-A")
	s.git("commit", "-qm", "synthetic current greeting and overlay")
	s.must(t, "regular", "check", scenarioGoal, "--baseline", "AC-1")
	s.must(t, "regular", "check", scenarioGoal, "AC-1")
	rep := s.status(t)
	if rep.Usage.Total != 2 || rep.Budget == nil || rep.Budget.Runs != 2 || !rep.Criteria[0].Satisfied {
		t.Fatalf("fixture lacks usable baseline plus current evidence: %+v", rep)
	}
	return s
}

func (s *baselineJunctionScenario) command(t *testing.T, args ...string) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, s.binary, args...)
	cmd.Dir = s.dir
	cmd.Env = append(os.Environ(), "PATH="+s.shim+string(os.PathListSeparator)+os.Getenv("PATH"),
		"HA_JUNCTION_REAL_GIT="+s.realGit, "HA_JUNCTION_WORKTREES="+filepath.Join(s.gitDir, "ha", "worktrees"),
		"HA_JUNCTION_OUTSIDE="+s.outside, "HA_JUNCTION_LEDGER="+s.ledger,
		"HA_JUNCTION_MARKER="+s.marker, "HA_JUNCTION_MODE="+s.mode)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("actual ha timed out: %v: %s", args, out)
	}
	if exit, ok := err.(*exec.ExitError); ok {
		return exit.ExitCode(), string(out)
	}
	if err != nil {
		t.Fatalf("actual ha failed to start: %v: %s", err, out)
	}
	return 0, string(out)
}

func (s *baselineJunctionScenario) invoke(t *testing.T, mode string, args ...string) (int, string) {
	t.Helper()
	s.mode = mode
	if err := os.WriteFile(s.ledger, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return s.command(t, args...)
}

func (s *baselineJunctionScenario) must(t *testing.T, mode string, args ...string) {
	t.Helper()
	if code, out := s.invoke(t, mode, args...); code != 0 {
		t.Fatalf("fixture actual ha %v: exit %d: %s", args, code, out)
	}
}

func (s *baselineJunctionScenario) status(t *testing.T) status.Report {
	t.Helper()
	code, out := s.command(t, "status", scenarioGoal, "--format", "json")
	var rep status.Report
	if err := json.Unmarshal([]byte(out), &rep); err != nil || code != rep.ExitCode || len(rep.Criteria) != 2 {
		t.Fatalf("invalid actual status: exit %d: %v: %s", code, err, out)
	}
	return rep
}

func (s *baselineJunctionScenario) markerBytes(t *testing.T) []byte {
	t.Helper()
	data, err := os.ReadFile(s.marker)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func (s *baselineJunctionScenario) assertWorktrees(t *testing.T, expected []string) {
	t.Helper()
	data, err := os.ReadFile(s.ledger)
	if err != nil {
		t.Fatal(err)
	}
	lines := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(lines) != len(expected) {
		t.Fatalf("actual baseline checkout count %d, want %d: %s", len(lines), len(expected), data)
	}
	for i, line := range lines {
		var row junctionObservation
		if err := json.Unmarshal(line, &row); err != nil || row.Ordinal != i+1 || row.Kind != expected[i] || row.OS != runtime.GOOS {
			t.Fatalf("fixture did not reach intended actual checkout boundary: %s: %v", line, err)
		}
		if filepath.Dir(filepath.Dir(row.Tree)) != filepath.Join(s.gitDir, "ha", "worktrees") {
			t.Fatalf("observed caller tree instead of disposable baseline: %s", row.Tree)
		}
		if _, err := os.Lstat(row.Tree); !os.IsNotExist(err) {
			t.Fatalf("baseline cleanup left its disposable tree: %v", err)
		}
	}
	entries, err := os.ReadDir(filepath.Join(s.gitDir, "ha", "worktrees"))
	if err != nil || len(entries) != 0 {
		t.Fatalf("baseline holder cleanup incomplete: %v %v", entries, err)
	}
	if count := strings.Count(s.git("worktree", "list", "--porcelain"), "worktree "); count != 1 {
		t.Fatalf("baseline cleanup left a registered worktree: %d", count)
	}
}

func (s *baselineJunctionScenario) snapshot(t *testing.T) map[string]junctionFileSnapshot {
	t.Helper()
	out := map[string]junctionFileSnapshot{}
	for label, root := range map[string]string{"project": s.dir, "outside": s.outside, "ha": filepath.Join(s.gitDir, "ha")} {
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
			info, err := d.Info()
			if err != nil {
				return err
			}
			var data []byte
			if !d.IsDir() {
				data, err = os.ReadFile(p)
				if err != nil {
					return err
				}
			}
			out[label+"/"+filepath.ToSlash(rel)] = junctionFileSnapshot{string(data), info.Mode()}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	for label, p := range map[string]string{"marker": s.marker, "index": filepath.Join(s.gitDir, "index"), "config": filepath.Join(s.gitDir, "config"), "head": filepath.Join(s.gitDir, "HEAD")} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		info, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		out[label] = junctionFileSnapshot{string(data), info.Mode()}
	}
	if runtime.GOOS == "windows" {
		// FileMode alone does not describe Windows permissions. The built-in
		// read-only listing also covers the external directory and file DACLs.
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		data, err := exec.CommandContext(ctx, "icacls.exe", s.outside, "/T").CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("cannot observe external Windows permissions: %v: %s", err, data)
		}
		out["outside-permissions"] = junctionFileSnapshot{Bytes: string(data)}
	}
	return out
}

func (s *baselineJunctionScenario) assertPriorFiles(t *testing.T, before map[string]junctionFileSnapshot, newLog string) {
	t.Helper()
	after := s.snapshot(t)
	for name, value := range before {
		if name == "marker" || name == "project/"+scenarioGoal+"/runs.jsonl" {
			continue
		}
		if after[name] != value {
			t.Fatalf("actual attempt changed prior source/index, outside permissions/data, or existing log: %s", name)
		}
	}
	for name := range after {
		if _, exists := before[name]; !exists && name != "ha/"+strings.TrimPrefix(newLog, ".git/ha/") {
			t.Fatalf("actual attempt left an unexpected artifact: %s", name)
		}
	}
}

func (s *baselineJunctionScenario) assertOneAttempt(t *testing.T, before, marker []byte, beforeStatus status.Report) string {
	t.Helper()
	old, bad, err := record.Parse(before)
	if err != nil || bad != nil {
		t.Fatal("fixture history is invalid")
	}
	data := s.read(scenarioGoal + "/runs.jsonl")
	rows, bad, err := record.Parse(data)
	if err != nil || bad != nil || len(rows) != len(old)+1 || !bytes.HasPrefix(data, before) {
		t.Fatalf("prior records changed or unexecuted targets added records: %v %v", bad, err)
	}
	row := rows[len(rows)-1]
	if row.Kind != "baseline" || row.Result != "fail_as_expected" || row.Evidence == nil || row.Evidence.Reason != "declared_violation" || row.Exit == nil || *row.Exit != 1 || row.Target == nil || row.Target.Criterion != "AC-1" || len(row.Overlay) != 3 {
		t.Fatalf("safe baseline result lost its existing meaning: %+v", row)
	}
	if got := s.markerBytes(t); !bytes.HasPrefix(got, marker) || bytes.Count(got, []byte("\n")) != bytes.Count(marker, []byte("\n"))+1 {
		t.Fatal("refused or later target started a checker, or safe checker never ran")
	}
	if !strings.HasPrefix(row.OutputPath, ".git/ha/") {
		t.Fatalf("attempt has no stored raw output: %s", row.OutputPath)
	}
	log := filepath.Join(s.gitDir, filepath.FromSlash(strings.TrimPrefix(row.OutputPath, ".git/")))
	output, err := os.ReadFile(log)
	digest := sha256.Sum256(output)
	if err != nil || !bytes.Contains(output, []byte("synthetic baseline junction producer")) || row.OutputDigest != hex.EncodeToString(digest[:]) {
		t.Fatalf("record does not identify the actual finalized producer output: %v", err)
	}
	after := s.status(t)
	if after.Budget == nil || after.Budget.Runs != beforeStatus.Budget.Runs+1 || after.Usage.Total != beforeStatus.Usage.Total+1 || after.Usage.Baselines != beforeStatus.Usage.Baselines+1 || after.Usage.Checks != beforeStatus.Usage.Checks || after.RecordHead == nil || after.RecordHead.Seq != row.Seq || after.RecordHead.SHA256 != row.Hash() {
		t.Fatal("actual attempt lost its record head/usage, or unexecuted targets were charged")
	}
	if !after.Criteria[0].Satisfied || after.Criteria[0].Attempts.Baselines != beforeStatus.Criteria[0].Attempts.Baselines+1 || !reflect.DeepEqual(after.Criteria[1], beforeStatus.Criteria[1]) {
		t.Fatal("safe evidence became unusable or unrelated condition state changed")
	}
	return row.OutputPath
}
