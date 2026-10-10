package main

// These tests call existing CLI/test boundaries only. Overlaying this file on
// the base compiles; missing product support is a requirement assertion.
import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/goaldocs"
)

func compatibilityCommand(args ...string) (int, string, string) {
	var out, stderr bytes.Buffer
	code := runMain(args, &out, &stderr)
	return code, out.String(), stderr.String()
}

func TestCompatibilityAC7_GoallessReadOnlyQuery(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	t.Setenv("PATH", dir) // No git, shell, model or verification process is available.
	code, out, stderr := compatibilityCommand("capabilities", "--format", "json")
	if code != 0 {
		t.Fatalf("[requirement] goal-free capability query: exit %d: %s", code, stderr)
	}
	var report map[string]any
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("[requirement] capabilities must be JSON: %v", err)
	}
	_, versionOut, _ := compatibilityCommand("--version")
	if report["schema"] != "ha-capabilities/v1" || report["ha_version"] != strings.TrimSpace(strings.TrimPrefix(versionOut, "ha ")) || report["default_rules"] != "run-rules/3" {
		t.Fatalf("[requirement] wrong capability identity/default: %v", report)
	}
	wanted := map[string]any{
		"supported_rules": []any{"run-rules/1", "run-rules/2", "run-rules/3"},
		"formats": map[string]any{
			"criteria": []any{"legacy", "nested/1"}, "check_declaration": []any{"check-declaration/v1"},
			"check_result": []any{"check-result/v1"}, "check_evidence": []any{"check-evidence/v1"},
		},
		"features": []any{"estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1", "budget-change/v1"},
	}
	for key, want := range wanted {
		if !reflect.DeepEqual(report[key], want) {
			t.Errorf("[requirement] %s: got %v, want %v", key, report[key], want)
		}
	}
	code, again, stderr := compatibilityCommand("capabilities")
	if code != 0 || again != out || stderr != "" {
		t.Errorf("[requirement] capability query is not deterministic: %d %s", code, stderr)
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("[requirement] query wrote files without a goal: %v", files)
	}
	for _, args := range [][]string{{"capabilities", "goal"}, {"capabilities", "--format", "md"}, {"capabilities", "--format", ""}, {"capabilities", "--unknown"}} {
		code, out, _ := compatibilityCommand(args...)
		if code != 64 || out != "" {
			t.Errorf("[requirement] invalid query accepted: %v: %d %s", args, code, out)
		}
	}
}

const compatibilityJSON = `{"schema":"ha-capabilities/v1","ha_version":"synthetic","supported_rules":["run-rules/1","run-rules/2","run-rules/3"],"default_rules":"run-rules/3","formats":{"criteria":["legacy","nested/1"],"check_declaration":["check-declaration/v1"],"check_result":["check-result/v1"],"check_evidence":["check-evidence/v1"]},"features":["estimate/v1","dirty-preflight/v1","baseline-safe-copy/v1","structured-change-results/v1","budget-change/v1"]}`

func compatibilityWrite(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

func compatibilityFake(t *testing.T, dir, response string, exit int, mutate bool) string {
	t.Helper()
	path := filepath.Join(dir, "ha")
	// Fixture JSON has no shell quotes; the test command is a query-only stub.
	if strings.Contains(response, "'") {
		t.Fatal("invalid fake-core fixture")
	}
	change := ""
	if mutate {
		change = "printf '\\n# changed\\n' >> \"$0\"\n"
	}
	source := "#!/bin/sh\nif [ \"$*\" != 'capabilities --format json' ]; then exit 99; fi\n" + change +
		"printf '%s\\n' '" + response + "'\nexit " + strconv.Itoa(exit) + "\n"
	compatibilityWrite(t, path, source, 0o755)
	return path
}

func compatibilityMutate(t *testing.T, change func(map[string]any)) string {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal([]byte(compatibilityJSON), &value); err != nil {
		t.Fatal(err)
	}
	change(value)
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestCompatibilityAC8_ConsumerAndActualBinary(t *testing.T) {
	helper, err := filepath.Abs("../../plugins/seal/skills/seal/scripts/check_core.py")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(helper); err != nil {
		if os.IsNotExist(err) {
			t.Fatal("[requirement] Seal has no deterministic consumer of the Core capability contract")
		}
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("test environment requires Python 3 for the optional consumer check")
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	actual := filepath.Join(t.TempDir(), "ha")
	build := exec.Command("go", "build", "-mod=readonly", "-o", actual, "./cmd/ha")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build test Core: %v: %s", err, out)
	}
	call := func(t *testing.T, core, operation, goal, reason string, want int, env []string) map[string]any {
		t.Helper()
		args := []string{helper, "--ha", core, "--operation", operation}
		if goal != "" {
			args = append(args, "--goal", goal)
		}
		cmd := exec.Command(python, args...)
		cmd.Env = append(os.Environ(), append([]string{"PYTHONDONTWRITEBYTECODE=1"}, env...)...)
		out, err := cmd.CombinedOutput()
		code := 0
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok {
				code = exit.ExitCode()
			} else {
				t.Fatal(err)
			}
		}
		var report map[string]any
		if json.Unmarshal(out, &report) != nil {
			t.Fatalf("[requirement] consumer produced no decision JSON: %s", out)
		}
		if code != want || report["reason"] != reason || report["compatible"] != (want == 0) {
			t.Fatalf("[requirement] consumer decision: exit %d, %s; want %d/%s", code, out, want, reason)
		}
		return report
	}
	t.Run("real Core and no goal or writes", func(t *testing.T) {
		dir := t.TempDir()
		t.Chdir(dir)
		report := call(t, actual, "start", "", "compatible", 0, nil)
		resolved, err := filepath.EvalSymlinks(actual)
		if err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(actual)
		if err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256(data)
		if report["executable"] != resolved || report["sha256"] != hex.EncodeToString(hash[:]) || report["rules"] != "run-rules/3" {
			t.Fatalf("[requirement] decision does not bind the actual binary: %v", report)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 0 {
			t.Fatalf("[requirement] compatibility query changed the working directory: %v", entries)
		}
	})
	for _, tc := range []struct {
		name, response, reason string
		exit                   int
	}{
		{"unsupported query", "ha synthetic", "query_unsupported", 64},
		{"query failure", "", "query_failed", 1},
		{"malformed JSON", "{", "invalid_capabilities", 0},
		{"duplicate key", strings.Replace(compatibilityJSON, `"schema":`, `"schema":"ha-capabilities/v1","schema":`, 1), "invalid_capabilities", 0},
		{"unknown schema", compatibilityMutate(t, func(v map[string]any) { v["schema"] = "ha-capabilities/99" }), "unsupported_schema", 0},
		{"missing field", compatibilityMutate(t, func(v map[string]any) { delete(v, "features") }), "invalid_capabilities", 0},
		{"wrong field type", compatibilityMutate(t, func(v map[string]any) { v["features"] = true }), "invalid_capabilities", 0},
		{"missing feature", compatibilityMutate(t, func(v map[string]any) { v["features"] = []string{"estimate/v1"} }), "missing_support", 0},
		{"missing result format", compatibilityMutate(t, func(v map[string]any) { v["formats"].(map[string]any)["check_result"] = []string{} }), "missing_support", 0},
		{"unknown default", compatibilityMutate(t, func(v map[string]any) {
			v["default_rules"] = "run-rules/4"
			v["supported_rules"] = []string{"run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4"}
		}), "unsupported_default_rule", 0},
		{"no legacy fallback", compatibilityMutate(t, func(v map[string]any) { v["default_rules"] = "run-rules/2" }), "unsupported_default_rule", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core := compatibilityFake(t, t.TempDir(), tc.response, tc.exit, false)
			report := call(t, core, "start", "", tc.reason, 2, nil)
			if report["executable"] == nil || report["sha256"] == nil {
				t.Fatalf("[requirement] refusal hides the queried binary: %v", report)
			}
		})
	}
	t.Run("missing Core", func(t *testing.T) {
		call(t, filepath.Join(t.TempDir(), "missing"), "start", "", "core_missing", 2, nil)
	})
	t.Run("changed executable", func(t *testing.T) {
		core := compatibilityFake(t, t.TempDir(), compatibilityJSON, 0, true)
		call(t, core, "start", "", "binary_changed", 2, nil)
	})
	t.Run("PATH precedence and no fallback", func(t *testing.T) {
		badDir, goodDir := t.TempDir(), t.TempDir()
		bad := compatibilityFake(t, badDir, "old Core", 64, false)
		compatibilityFake(t, goodDir, compatibilityJSON, 0, false)
		report := call(t, "ha", "start", "", "query_unsupported", 2, []string{"PATH=" + badDir + string(os.PathListSeparator) + goodDir})
		resolved, err := filepath.EvalSymlinks(bad)
		if err != nil {
			t.Fatal(err)
		}
		if report["executable"] != resolved {
			t.Fatalf("[requirement] checked another executable from PATH: %v", report)
		}
	})
	t.Run("stored legacy rules need no v3 declaration support", func(t *testing.T) {
		response := compatibilityMutate(t, func(v map[string]any) {
			v["default_rules"], v["supported_rules"] = "run-rules/4", []string{"run-rules/1", "run-rules/2", "run-rules/4"}
			v["features"] = []string{"dirty-preflight/v1", "baseline-safe-copy/v1"}
			v["formats"] = map[string]any{"criteria": []string{"legacy"}, "check_declaration": []string{}, "check_result": []string{}, "check_evidence": []string{}}
		})
		core := compatibilityFake(t, t.TempDir(), response, 0, false)
		for _, rules := range []string{"run-rules/1", "run-rules/2"} {
			goal := t.TempDir()
			path := filepath.Join(goal, "runs.jsonl")
			original := `{"schema":"run/v1","kind":"start","rules":"` + rules + `"}` + "\n"
			compatibilityWrite(t, path, original, 0o644)
			compatibilityWrite(t, filepath.Join(goal, "SPEC.md"), "# Synthetic legacy goal\nStatus: Ready\n", 0o644)
			for _, operation := range []string{"resume", "check"} {
				report := call(t, core, operation, goal, "compatible", 0, nil)
				if report["rules"] != rules {
					t.Fatalf("[requirement] legacy rule changed: %v", report)
				}
			}
			call(t, core, "budget", goal, "unsupported_operation", 2, nil)
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if string(data) != original {
				t.Fatal("[requirement] compatibility check rewrote stored rules")
			}
		}
	})
	t.Run("criteria selector agrees with Core boundaries", func(t *testing.T) {
		response := compatibilityMutate(t, func(v map[string]any) {
			v["formats"].(map[string]any)["criteria"] = []string{"legacy"}
		})
		core := compatibilityFake(t, t.TempDir(), response, 0, false)
		for _, tc := range []struct {
			name, before, after, reason string
			formatError                 bool
		}{
			{"legacy", "", "", "compatible", false},
			{"selected nested", "Criteria-Format: nested/1\n", "", "missing_support", false},
			{"nested fenced example", "````markdown\n```\nCriteria-Format: nested/1\n```\n````\n", "", "compatible", false},
			{"fence with trailing text stays open", "~~~markdown\n~~~ still quoted\nCriteria-Format: nested/1\n~~~\n", "", "compatible", false},
			{"indented body example", "", "\n## Explanation\n  Criteria-Format: nested/1\n", "compatible", false},
			{"late selector", "", "\n## Explanation\nCriteria-Format: nested/1\n", "goal_format_invalid", true},
			{"indented metadata", " Criteria-Format: nested/1\n", "", "goal_format_invalid", true},
			{"duplicate selector", "Criteria-Format: nested/1\nCriteria-Format: nested/1\n", "", "goal_format_invalid", true},
			{"unknown selector", "Criteria-Format: nested/99\n", "", "goal_format_invalid", true},
			{"backtick inside fence info is not a fence", "```example`text\nCriteria-Format: nested/1\n", "", "missing_support", false},
		} {
			t.Run(tc.name, func(t *testing.T) {
				root := t.TempDir()
				goal := filepath.Join(root, "goal")
				source := "# Synthetic\nStatus: Ready\n" + tc.before + "\n## Acceptance Criteria\n- AC-1 Synthetic result.\n" + tc.after
				compatibilityWrite(t, filepath.Join(goal, "SPEC.md"), source, 0o644)
				compatibilityWrite(t, filepath.Join(goal, "runs.jsonl"), `{"schema":"run/v1","kind":"start","rules":"run-rules/2"}`+"\n", 0o644)
				loaded, err := goaldocs.Load(root, "goal")
				if err != nil {
					t.Fatal(err)
				}
				formatError := false
				for _, problem := range loaded.Problems {
					formatError = formatError || strings.HasPrefix(problem.Code, "criteria_format")
				}
				if formatError != tc.formatError {
					t.Fatalf("[requirement] Core selector boundary changed: %v", loaded.Problems)
				}
				want := 2
				if tc.reason == "compatible" {
					want = 0
				}
				call(t, core, "resume", goal, tc.reason, want, nil)
			})
		}
	})
}

func TestCompatibilityAC9_LegacyResumeAndOrdinaryChecks(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2"} {
		t.Run(rules, func(t *testing.T) {
			f := newPreflightFixture(t)
			f.rules(rules)
			f.put("tests/greeting.sh", "exit 1\n")
			f.put(scenarioGoal+"/PLAN.md", string(f.read(scenarioGoal+"/PLAN.md"))+"\n## 결과 계약\nLegacy free-form notes, not a declaration table.\n")
			f.git("add", "-A")
			f.git("commit", "-qm", "synthetic legacy input")
			before := f.read(scenarioGoal + "/runs.jsonl")
			report := f.report()
			if report["rules"] != rules || !bytes.Equal(before, f.read(scenarioGoal+"/runs.jsonl")) {
				t.Fatalf("stored legacy query changed meaning: %v", report)
			}
			for _, args := range [][]string{{"note", scenarioGoal, "reopen", "--quote", "Continue this synthetic goal"}, {"check", scenarioGoal, "--baseline", "AC-1"}, {"check", scenarioGoal, "AC-2"}, {"check", scenarioGoal, "T001"}} {
				if code, out := f.ha(args...); code != 0 {
					t.Fatalf("legacy operation %v: %d: %s", args, code, out)
				}
			}
			lines := fixtureRecords(t, f)
			if lines[0]["rules"] != rules || lines[2]["result"] != "fail_as_expected" || lines[3]["result"] != "pass" || lines[4]["result"] != "pass" {
				t.Fatalf("legacy execution or reopen semantics changed: %v", lines)
			}
			if runs := f.report()["budget"].(map[string]any)["runs"]; runs != float64(3) {
				t.Fatalf("legacy use changed: %v", runs)
			}
		})
	}
}

func TestCompatibilityAC9_UndeclaredV3AndManualRemainSupported(t *testing.T) {
	f := newPreflightFixture(t)
	if code, out := f.ha("check", scenarioGoal, "AC-1"); code != 1 {
		t.Fatalf("bare Core must record an unsupported /3 producer: %d: %s", code, out)
	}
	lines := fixtureRecords(t, f)
	if lines[len(lines)-1]["result"] != "unknown" {
		t.Fatalf("undeclared change was promoted or refused before execution: %v", lines)
	}
	plan := string(f.read(scenarioGoal + "/PLAN.md"))
	f.put(scenarioGoal+"/PLAN.md", strings.Replace(plan, "| AC-1 | change | `sh tests/greeting.sh` | tests/greeting.sh | T001 |", "| AC-1 | manual | — synthetic review | — | T001 |", 1))
	for _, args := range [][]string{{"note", scenarioGoal, "confirm", "AC-1", "--quote", "The synthetic output is accepted"}, {"check", scenarioGoal, "AC-2"}, {"check", scenarioGoal, "T001"}} {
		if code, out := f.ha(args...); code != 0 {
			t.Fatalf("ordinary operation %v was forced into the result contract: %d: %s", args, code, out)
		}
	}
}
