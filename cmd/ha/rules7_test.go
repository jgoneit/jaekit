package main

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestRules7EstimateRulesAndStartMarks(t *testing.T) {
	f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3")}, column: true})
	for _, tc := range []struct {
		rules   string
		minimum int
	}{{"", 5}, {"run-rules/6", 5}, {"run-rules/7", 4}} {
		args := []string{}
		if tc.rules != "" {
			args = append(args, "--rules", tc.rules)
		}
		est := f.estimate("rules7-estimate", "before start", args...)
		if n, ok := ccNumber(est, "minimum_runs"); !ok || n != tc.minimum {
			t.Fatalf("estimate %s minimum = %v; want %d", tc.rules, est["minimum_runs"], tc.minimum)
		}
	}
	f.start("rules7-start")
	start := ciLines(f.rgFixture)[0].m
	if want := []any{"AC-1"}; !reflect.DeepEqual(start["baseline_marked"], want) {
		t.Fatalf("baseline_marked = %v; want %v", start["baseline_marked"], want)
	}
	before := f.records()
	code, out := f.ha("estimate", rgGoal, "--rules", "run-rules/6")
	if code != exitUsage || !bytes.Equal(before, f.records()) {
		t.Fatalf("mismatched estimate: exit %d, record changed %t: %s", code, !bytes.Equal(before, f.records()), out)
	}
	f.estimate("rules7-estimate", "recorded rules match", "--rules", "run-rules/7")
	code, out = f.ha("estimate", rgGoal)
	if code != exitOK || !strings.Contains(out, "4 runs (1 marked change × 2 + 1 unmarked change + 1 maintain; manual: 0)") {
		t.Fatalf("human estimate: exit %d: %s", code, out)
	}
}

func TestRules7EmptyStartMarksAreRecorded(t *testing.T) {
	f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ""), ccMaintain("AC-2")}})
	f.start("rules7-start")
	marks, exists := ciLines(f.rgFixture)[0].m["baseline_marked"]
	if !exists || !reflect.DeepEqual(marks, []any{}) {
		t.Fatalf("baseline_marked = %#v (exists %t); want []", marks, exists)
	}
}
