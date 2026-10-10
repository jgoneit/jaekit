package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/status"
)

// Selecting a new document interpretation must not reuse evidence or a user's
// confirmation of the old interpretation, even when all criterion IDs survive.
func TestCriteriaFormatFreshness(t *testing.T) {
	for _, rules := range []string{status.RulesV1, status.RulesV2} {
		t.Run(rules, func(t *testing.T) {
			s := unsupportedRulesFixture(t)
			st := step{where: t.Name()}
			edit := func(rel, before, after string) {
				t.Helper()
				p := s.file(scenarioGoal + "/" + rel)
				b, err := os.ReadFile(p)
				if err != nil || !strings.Contains(string(b), before) {
					t.Fatalf("edit %s: %v", rel, err)
				}
				if err := os.WriteFile(p, []byte(strings.Replace(string(b), before, after, 1)), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			edit("PLAN.md", "| AC-1 | change | `sh tests/greeting.sh` | tests/greeting.sh | T001 |", "| AC-1 | manual | — review the wording | — | T001 |")
			edit("SPEC.md", "- **AC-2**", "\n  This explanation belongs to the first condition in nested/1.\n\n- **AC-2**")
			s.git("add", "-A")
			s.git("commit", "-qm", "manual fixture")
			run := func(args ...string) {
				t.Helper()
				s.ha(args...)
				if s.lastCode != 0 {
					t.Fatalf("%v: exit %d: %s", args, s.lastCode, s.lastOut)
				}
			}
			run("start", scenarioGoal, "--request", "implement", "--skill-name", "seal", "--skill-version", "0.1.0")
			s.do(step{where: t.Name(), args: []string{"rewrite-record", "1", "rules", `"` + rules + `"`}})
			run("check", scenarioGoal, "AC-2")
			run("note", scenarioGoal, "confirm", "AC-1", "--quote", "The wording is clear")
			s.progress(step{where: t.Name(), args: []string{"progress", "T001", "done"}})
			beforeReport := s.report(st)
			if beforeReport.Status != "complete" {
				t.Fatalf("before selection: %s %v", beforeReport.Status, activeReasons(beforeReport))
			}
			path := s.file(scenarioGoal + "/runs.jsonl")
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			edit("SPEC.md", "Status: Ready", "Status: Ready\nCriteria-Format: nested/1")
			s.git("add", scenarioGoal+"/SPEC.md")
			s.git("commit", "-qm", "select nested criteria")
			rep := s.report(st)
			for _, want := range []string{"spec_changed", "AC-1:manual_unconfirmed", "AC-2:criterion_missing"} {
				if !strings.Contains(" "+strings.Join(activeReasons(rep), " ")+" ", " "+want+" ") {
					t.Errorf("missing %s after selection: %v", want, activeReasons(rep))
				}
			}
			if len(rep.Changes.InvalidConfirmations) != 1 || rep.Changes.InvalidConfirmations[0].Subject != "AC-1" {
				t.Errorf("manual binding reused: %+v", rep.Changes.InvalidConfirmations)
			}
			after, err := os.ReadFile(path)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("selection rewrote records: %v", err)
			}
		})
	}
}
