package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Adding a start option does not enable historical or unknown rules for new
// goals. The refusal must advertise the new option without creating a record.
func TestRules6StartRefusesOtherVersions(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/8", ""} {
		t.Run(rules, func(t *testing.T) {
			f := rgRepo(t, "")
			code, out := f.ha("start", rgGoal, "--request", "implement the synthetic goal", "--rules", rules)
			if code != exitUsage || !strings.Contains(out, "--rules accepts run-rules/3, run-rules/4, run-rules/5, run-rules/6 or run-rules/7") {
				t.Fatalf("start --rules %q: exit %d: %s", rules, code, out)
			}
			if _, err := os.Stat(filepath.Join(f.dir, rgGoal, "runs.jsonl")); !os.IsNotExist(err) {
				t.Fatalf("refused start created a record or could not be inspected: %v", err)
			}
		})
	}
}

func TestRules6PostCompletionBudgetOutputs(t *testing.T) {
	for _, rules := range []string{"run-rules/4", "run-rules/5", "run-rules/6"} {
		t.Run(rules, func(t *testing.T) {
			f := rgRepo(t, "light")
			ciMust(f, exitOK, "start", rgGoal, "--request", "implement the synthetic goal", "--rules", rules)
			check := func(want int64) {
				t.Helper()
				budgets := map[string]map[string]any{"status": ciBudget(f, f.status())}
				var cost struct {
					Verification struct {
						Budget map[string]any `json:"budget"`
					} `json:"verification"`
				}
				if err := json.Unmarshal([]byte(ciMarkdown(f, "cost", rgGoal, "--format", "json")), &cost); err != nil {
					t.Fatal(err)
				}
				budgets["cost"] = cost.Verification.Budget
				for command, budget := range budgets {
					value, exists := budget["post_completion_seconds"]
					if rules == "run-rules/6" {
						if !exists || value != float64(want) {
							t.Errorf("%s budget post_completion_seconds = %v (present %t); want %d", command, value, exists, want)
						}
					} else if exists {
						t.Errorf("%s budget under %s includes post_completion_seconds", command, rules)
					}
				}
			}
			check(0)
			f.check()
			ciMustDone(f, "completion")
			ciMust(f, exitOK, "note", rgGoal, "input", "--quote", "merge the synthetic change")
			lines := ciLines(f)
			ciTime(lines[0], 0)
			ciTime(lines[1], time.Second, 0)
			ciTime(lines[2], 2*time.Second)
			ciTime(lines[3], 10*time.Second)
			ciWrite(f, ciRelink(f, lines, 0))
			check(8)
			for lang, text := range map[string]string{"en": "8 seconds after completion", "ko": "완료 뒤 8초"} {
				out := ciMarkdown(f, "cost", rgGoal, "--lang", lang)
				if strings.Contains(out, text) != (rules == "run-rules/6") {
					t.Errorf("cost %s under %s has wrong post-completion output: %s", lang, rules, out)
				}
			}
		})
	}
}
