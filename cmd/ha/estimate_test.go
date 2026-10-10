package main

// These tests exercise the CLI through runMain and use no estimate-only Go
// symbols, so overlaying this file onto an older checkout gives behavioral
// failures rather than a compiler error.

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const estimateTestGoal = "docs/specs/estimate-example"

type estimateTestRow struct {
	id, kind string
	optional bool
}

func estimateTestWrite(t *testing.T, root, name, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func estimateTestGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false"}, args...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func estimateTestFixture(t *testing.T, rows []estimateTestRow, limit int) string {
	t.Helper()
	root := t.TempDir()
	cfg := filepath.Join(t.TempDir(), "gitconfig")
	if err := os.WriteFile(cfg, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", cfg)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "estimate fixture")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "estimate@example.invalid")
	}
	var spec, plan strings.Builder
	spec.WriteString("# Synthetic estimate goal\n\nStatus: Ready\n\n## Acceptance Criteria\n")
	plan.WriteString("# PLAN\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n")
	var ids []string
	for _, row := range rows {
		if strings.HasPrefix(row.id, "AC-") {
			optional := ""
			if row.optional {
				optional = " (선택)"
			}
			fmt.Fprintf(&spec, "- **%s**%s A synthetic observable result.\n", row.id, optional)
		}
		command, paths := "`sh tests/observe.sh`", "—"
		if row.kind == "change" {
			paths = "tests/observe.sh"
		}
		if row.kind == "manual" {
			command = "— a synthetic user decision"
		}
		fmt.Fprintf(&plan, "| %s | %s | %s | %s | T001 |\n", row.id, row.kind, command, paths)
		ids = append(ids, row.id)
	}
	spec.WriteString("\n## Open Decisions\n없음\n")
	fmt.Fprintf(&plan, "\n## 범위\n- 바꿀 수 있는 경로: `src/**`, `tests/**`\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n| T001 | synthetic behavior | %s | — | — | 예 | tasks/T001.md |\n\n## 예산\n- 검증 실행: %d\n- 경과 시간: 4시간\n", strings.Join(ids, ", "), limit)
	estimateTestWrite(t, root, estimateTestGoal+"/SPEC.md", spec.String())
	estimateTestWrite(t, root, estimateTestGoal+"/PLAN.md", plan.String())
	estimateTestWrite(t, root, estimateTestGoal+"/REVIEW.md", "# REVIEW\n\n## 결론\nready\n")
	estimateTestWrite(t, root, estimateTestGoal+"/PROGRESS.md", "# PROGRESS\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | todo | |\n")
	estimateTestWrite(t, root, estimateTestGoal+"/tasks/T001.md", "# Task\n\n## 완료 조건\n- `sh tests/observe.sh`\n- `sh tests/observe.sh`\n")
	estimateTestWrite(t, root, "tests/observe.sh", "printf 'executed\\n' >> events\nexit 0\n")
	estimateTestWrite(t, root, ".gitignore", "/events\n")
	estimateTestWrite(t, root, "src/value.txt", "base\n")
	estimateTestGit(t, root, "init", "-q", "-b", "main")
	estimateTestGit(t, root, "add", "-A")
	estimateTestGit(t, root, "commit", "-q", "-m", "synthetic base")
	t.Chdir(root)
	return root
}

func estimateTestCLI(args ...string) (int, string) {
	var out, stderr bytes.Buffer
	code := runMain(args, &out, &stderr)
	return code, out.String() + stderr.String()
}

func estimateTestJSON(t *testing.T, wantCode int, args ...string) map[string]any {
	t.Helper()
	code, text := estimateTestCLI(append([]string{"estimate", estimateTestGoal}, append(args, "--format", "json")...)...)
	if code != wantCode {
		t.Fatalf("estimate exit %d, want %d: %s", code, wantCode, text)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(text), &report); err != nil {
		t.Fatalf("estimate did not return JSON: %v\n%s", err, text)
	}
	return report
}

func estimateTestNumbers(t *testing.T, report map[string]any, want map[string]int) {
	t.Helper()
	for key, number := range want {
		if got, ok := report[key].(float64); !ok || got != float64(number) {
			t.Errorf("%s = %v, want %d", key, report[key], number)
		}
	}
}

func estimateTestSnapshot(t *testing.T, root string) map[string][32]byte {
	t.Helper()
	files := map[string][32]byte{}
	if err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, err := os.ReadFile(path)
		if err == nil {
			rel, _ := filepath.Rel(root, path)
			files[rel] = sha256.Sum256(data)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

func TestEstimateAC8(t *testing.T) {
	t.Run("34 required changes", func(t *testing.T) {
		var rows []estimateTestRow
		for i := 1; i <= 34; i++ {
			rows = append(rows, estimateTestRow{fmt.Sprintf("AC-%d", i), "change", false})
		}
		estimateTestFixture(t, rows, 68)
		report := estimateTestJSON(t, 0)
		estimateTestNumbers(t, report, map[string]int{"minimum_runs": 68, "required_change": 34, "required_maintain": 0, "headroom": 0, "shortfall": 0, "default_check_runs": 34, "default_baseline_runs": 34})
	})
	t.Run("default and explicit selection match actual records", func(t *testing.T) {
		rows := []estimateTestRow{{"AC-1", "change", false}, {"AC-2", "maintain", false}, {"AC-3", "manual", false}, {"AC-4", "change", true}, {"AC-5", "maintain", true}, {"EX-1", "change", true}}
		root := estimateTestFixture(t, rows, 30)
		report := estimateTestJSON(t, 0)
		estimateTestNumbers(t, report, map[string]int{"minimum_runs": 3, "default_check_runs": 2, "default_baseline_runs": 3, "default_additional_runs": 2})
		if code, text := estimateTestCLI("start", estimateTestGoal, "--request", "Run this synthetic goal"); code != 0 {
			t.Fatalf("start: %d %s", code, text)
		}
		for _, args := range [][]string{nil, {"--baseline"}, {"AC-1", "AC-1", "AC-4", "AC-5", "EX-1", "T001", "T001"}, {"--baseline", "AC-1", "AC-1", "AC-4", "EX-1"}} {
			report := estimateTestJSON(t, 0, args...)
			selected := report["selected"].(map[string]any)
			before, err := os.ReadFile(filepath.Join(root, estimateTestGoal, "runs.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			code, text := estimateTestCLI(append([]string{"check", estimateTestGoal}, args...)...)
			if code != 0 && code != 1 { // Baselines observe unexpected pass, but still execute.
				t.Fatalf("check: %d %s", code, text)
			}
			after, err := os.ReadFile(filepath.Join(root, estimateTestGoal, "runs.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			actual := bytes.Count(after, []byte("\n")) - bytes.Count(before, []byte("\n"))
			estimateTestNumbers(t, selected, map[string]int{"runs": actual})
			if len(args) == 7 {
				estimateTestNumbers(t, selected, map[string]int{"runs": 9, "optional_runs": 3, "task_runs": 4, "repeated_required_runs": 1, "additional_runs": 8})
			}
		}
	})
	t.Run("manual only and invalid selections", func(t *testing.T) {
		estimateTestFixture(t, []estimateTestRow{{"AC-1", "manual", false}}, 1)
		for _, args := range [][]string{nil, {"--baseline"}} {
			report := estimateTestJSON(t, 0, args...)
			estimateTestNumbers(t, report, map[string]int{"minimum_runs": 0, "default_check_runs": 0, "default_baseline_runs": 0})
			estimateTestNumbers(t, report["selected"].(map[string]any), map[string]int{"runs": 0})
		}
		for _, args := range [][]string{{"AC-1"}, {"AC-99"}, {"--baseline", "T001"}, {"--format", "other"}} {
			code, text := estimateTestCLI(append([]string{"estimate", estimateTestGoal}, args...)...)
			if code != 64 {
				t.Fatalf("invalid selection %v: %d %s", args, code, text)
			}
		}
	})
}

func TestStartBudgetAC9(t *testing.T) {
	for _, limit := range []int{2, 3, 4} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			root := estimateTestFixture(t, []estimateTestRow{{"AC-1", "change", false}, {"AC-2", "maintain", false}}, limit)
			wantEstimate := 0
			if limit < 3 {
				wantEstimate = 1
			}
			report := estimateTestJSON(t, wantEstimate)
			estimateTestNumbers(t, report, map[string]int{"minimum_runs": 3, "plan_runs_limit": limit, "shortfall": max(0, 3-limit)})
			before := estimateTestSnapshot(t, root)
			code, text := estimateTestCLI("start", estimateTestGoal, "--request", "Run this synthetic goal")
			if limit < 3 {
				if code != 65 {
					t.Fatalf("insufficient budget start exit %d, want 65: %s", code, text)
				}
				for _, word := range []string{"minimum", "limit", "shortfall"} {
					if !strings.Contains(text, word) {
						t.Errorf("refusal does not explain %s: %s", word, text)
					}
				}
				if !reflect.DeepEqual(before, estimateTestSnapshot(t, root)) {
					t.Fatal("refused start changed files, records, or Git metadata")
				}
			} else if code != 0 {
				t.Fatalf("sufficient budget start exit %d: %s", code, text)
			}
		})
	}
}

func TestEstimateAC10(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2"} {
		t.Run(rules, func(t *testing.T) {
			root := estimateTestFixture(t, []estimateTestRow{{"AC-1", "change", false}}, 10)
			before := estimateTestSnapshot(t, root)
			estimateTestJSON(t, 0, "T001")
			if !reflect.DeepEqual(before, estimateTestSnapshot(t, root)) {
				t.Fatal("pre-start estimate changed files or executed a command")
			}
			if code, text := estimateTestCLI("start", estimateTestGoal, "--request", "Run this synthetic goal"); code != 0 {
				t.Fatalf("start: %d %s", code, text)
			}
			// Synthetic compatibility fixture: only the initial record exists,
			// so changing its selected rules leaves no following hashes to repair.
			recordPath := filepath.Join(root, estimateTestGoal, "runs.jsonl")
			data, err := os.ReadFile(recordPath)
			if err != nil {
				t.Fatal(err)
			}
			var start map[string]any
			if err := json.Unmarshal(data, &start); err != nil {
				t.Fatal(err)
			}
			start["rules"] = rules
			data, err = json.Marshal(start)
			if err != nil {
				t.Fatal(err)
			}
			estimateTestWrite(t, root, estimateTestGoal+"/runs.jsonl", string(data)+"\n")
			if code, text := estimateTestCLI("check", estimateTestGoal, "--baseline"); code != 1 {
				t.Fatalf("synthetic baseline: %d %s", code, text)
			}
			planPath := filepath.Join(root, estimateTestGoal, "PLAN.md")
			plan, err := os.ReadFile(planPath)
			if err != nil {
				t.Fatal(err)
			}
			changed := strings.ReplaceAll(string(plan), "sh tests/observe.sh", "missing-estimate-test-command")
			changed = strings.ReplaceAll(changed, "검증 실행: 10", "검증 실행: 1")
			estimateTestWrite(t, root, estimateTestGoal+"/PLAN.md", changed)
			if code, text := estimateTestCLI("check", estimateTestGoal); code != 1 {
				t.Fatalf("synthetic execution error: %d %s", code, text)
			}
			if code, text := estimateTestCLI("note", estimateTestGoal, "reopen", "--quote", "Continue the synthetic goal"); code != 0 {
				t.Fatalf("reopen: %d %s", code, text)
			}
			_, statusBefore := estimateTestCLI("status", estimateTestGoal, "--format", "json")
			before = estimateTestSnapshot(t, root)
			report := estimateTestJSON(t, 1)
			if report["budget_source"] != "PLAN.md" || !strings.Contains(report["limitation"].(string), "existing records and budget windows are unchanged") {
				t.Fatal("estimate confuses the PLAN limit with the recorded budget")
			}
			if !reflect.DeepEqual(before, estimateTestSnapshot(t, root)) {
				t.Fatal("estimate changed existing records, logs, or workspace files")
			}
			_, statusAfter := estimateTestCLI("status", estimateTestGoal, "--format", "json")
			if statusBefore != statusAfter {
				t.Fatal("estimate changed the existing status, attempts, or budget window")
			}
		})
	}
}
