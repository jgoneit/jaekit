package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

// The Seal consumer (check_core.py) asks for the run-rules/4 features only
// where a run-rules/4 goal uses them, and never for a run-rules/3 goal
// (core-capabilities.md §6).
func TestCompatibilityRunRules4Goals(t *testing.T) {
	helper, err := filepath.Abs("../../plugins/seal/skills/seal/scripts/check_core.py")
	if err != nil {
		t.Fatal(err)
	}
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Fatal("test environment requires Python 3 for the optional consumer check")
	}
	decide := func(t *testing.T, core, operation, goal string) (int, map[string]any) {
		t.Helper()
		cmd := exec.Command(python, helper, "--ha", core, "--operation", operation, "--goal", goal)
		cmd.Env = append(os.Environ(), "PYTHONDONTWRITEBYTECODE=1")
		out, err := cmd.CombinedOutput()
		code := 0
		if exit, ok := err.(*exec.ExitError); ok {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		var report map[string]any
		if json.Unmarshal(out, &report) != nil {
			t.Fatalf("consumer produced no decision JSON: %s", out)
		}
		return code, report
	}
	goal := func(t *testing.T, rules string) string {
		dir := t.TempDir()
		compatibilityWrite(t, filepath.Join(dir, "SPEC.md"), "# Synthetic goal\nStatus: Ready\n", 0o644)
		compatibilityWrite(t, filepath.Join(dir, "runs.jsonl"), `{"schema":"run/v1","kind":"start","rules":"`+rules+`"}`+"\n", 0o644)
		return dir
	}
	features := []string{"test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1", "command-time-limits/v1"}
	without := func(t *testing.T, feature string) string {
		return compatibilityMutate(t, func(v map[string]any) {
			var kept []string
			for _, f := range v["features"].([]any) {
				if f != feature {
					kept = append(kept, f.(string))
				}
			}
			v["features"] = kept
		})
	}
	v4 := goal(t, "run-rules/4")
	for _, operation := range []string{"resume", "check", "budget"} {
		core := compatibilityFake(t, t.TempDir(), compatibilityJSON, 0, false)
		if code, report := decide(t, core, operation, v4); code != 0 || report["rules"] != "run-rules/4" {
			t.Fatalf("%s of a run-rules/4 goal: exit %d, %v", operation, code, report)
		}
		for _, feature := range features {
			core := compatibilityFake(t, t.TempDir(), without(t, feature), 0, false)
			code, report := decide(t, core, operation, v4)
			missing, _ := report["missing"].([]any)
			if code != 2 || report["reason"] != "missing_support" || !slices.Contains(missing, any("features:"+feature)) {
				t.Fatalf("%s of a run-rules/4 goal without %s: exit %d, %v", operation, feature, code, report)
			}
		}
	}
	v3 := goal(t, "run-rules/3")
	old := compatibilityMutate(t, func(v map[string]any) {
		v["supported_rules"] = []string{"run-rules/1", "run-rules/2", "run-rules/3"}
		v["features"] = []string{"estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1", "budget-change/v1"}
	})
	for _, operation := range []string{"resume", "check", "budget"} {
		core := compatibilityFake(t, t.TempDir(), old, 0, false)
		if code, report := decide(t, core, operation, v3); code != 0 || report["rules"] != "run-rules/3" {
			t.Fatalf("%s of a run-rules/3 goal on a Core without run-rules/4: exit %d, %v", operation, code, report)
		}
		if code, report := decide(t, core, operation, v4); code != 2 || report["reason"] != "unsupported_stored_rule" {
			t.Fatalf("%s of a run-rules/4 goal on a Core without it: exit %d, %v", operation, code, report)
		}
	}
}
