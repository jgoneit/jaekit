package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Adding a start option does not enable historical or unknown rules for new
// goals. The refusal must advertise the new option without creating a record.
func TestRules6StartRefusesOtherVersions(t *testing.T) {
	for _, rules := range []string{"run-rules/1", "run-rules/2", "run-rules/7", ""} {
		t.Run(rules, func(t *testing.T) {
			f := rgRepo(t, "")
			code, out := f.ha("start", rgGoal, "--request", "implement the synthetic goal", "--rules", rules)
			if code != exitUsage || !strings.Contains(out, "--rules accepts run-rules/3, run-rules/4, run-rules/5 or run-rules/6") {
				t.Fatalf("start --rules %q: exit %d: %s", rules, code, out)
			}
			if _, err := os.Stat(filepath.Join(f.dir, rgGoal, "runs.jsonl")); !os.IsNotExist(err) {
				t.Fatalf("refused start created a record or could not be inspected: %v", err)
			}
		})
	}
}
