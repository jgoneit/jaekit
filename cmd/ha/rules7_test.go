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

func TestRules7ReportRestoredAndAddedBaselineMarks(t *testing.T) {
	f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3")}, column: true})
	f.start("rules7-report")
	before := f.records()
	show := func(removed bool) {
		t.Helper()
		for _, lang := range []string{"ko", "en"} {
			prefix := "baseline mark removed after start: "
			if lang == "ko" {
				prefix = "시작 뒤 baseline 표시를 뺀 조건: "
			}
			review := f.summary("rules7-report", "current marks", lang, 1)
			item, exists := ccItem(review, prefix)
			if exists != removed || (removed && item != "AC-1") {
				t.Errorf("%s removed mark = %q (present %t); want AC-1 present %t", lang, item, exists, removed)
			}
		}
	}
	show(false)
	f.mark("AC-1", "")
	show(true)
	f.mark("AC-1", ccMark)
	show(false)
	f.mark("AC-2", ccMark)
	show(false)
	f.mark("AC-2", "—")
	show(false)
	// A mark belongs in this item only while the condition is still change.
	f.cond("AC-1").kind = "maintain"
	f.mark("AC-1", "")
	show(false)
	if !bytes.Equal(before, f.records()) {
		t.Fatal("reading the reports changed the run records")
	}
}
