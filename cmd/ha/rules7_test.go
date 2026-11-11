package main

import (
	"bytes"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestRules7CheckChangeBlobBinding(t *testing.T) {
	const v = "rules7-check-change"
	const helper = "checks/helper.txt"
	cond := ccChange("AC-1", "")
	cond.paths = []string{ccScript("AC-1"), ccDecl("AC-1"), helper}
	f := ccRepo(t, ccGoal{tier: "standard", conds: []ccCond{cond}})
	f.start(v)
	f.recordGate(v, "freeze", ccGate(f, ccRound(f, "candidate", f.base, f.base, false)))

	f.revise(helper, "changed after review")
	before := f.records()
	code, out := f.ha("note", rgGoal, "check-change", "checks:"+helper, "--reason", "F1: repair check input")
	if code != exitRefused || !bytes.Equal(before, f.records()) {
		t.Fatalf("dirty check-change: exit %d, record changed %t: %s", code, !bytes.Equal(before, f.records()), out)
	}
	f.head = f.commit("repair check input")
	seq := f.changeNote(v, helper, "F1: repair check input")
	lines := ciLines(f.rgFixture)
	data, _ := lines[len(lines)-1].m["check_change"].(map[string]any)
	if want := f.git("rev-parse", "HEAD:"+helper); data["blob"] != want {
		t.Fatalf("reason bound to %v; want HEAD blob %s", data, want)
	}
	_, files := f.freeze(v, "current reason", f.state(v, "current reason"))
	if got := files[helper]; got.seq != seq || got.state != "changed" {
		t.Fatalf("current reason = %+v", got)
	}

	f.revise(helper, "later change invalidates the reason")
	f.head = f.commit("revise input again")
	_, files = f.freeze(v, "stale reason", f.state(v, "stale reason"))
	if got := files[helper]; !got.nullReason {
		t.Fatalf("reason survived a later blob change: %+v", got)
	}

	f.git("rm", "--", helper)
	f.setPaths("AC-1", ccScript("AC-1"), ccDecl("AC-1"))
	f.head = f.commit("remove obsolete check input")
	seq = f.changeNote(v, helper, "F2: input no longer used")
	lines = ciLines(f.rgFixture)
	data, _ = lines[len(lines)-1].m["check_change"].(map[string]any)
	if blob, present := data["blob"]; !present || blob != nil {
		t.Fatalf("absent path binding = %v; want explicit null blob", data)
	}
	_, files = f.freeze(v, "removed reason", f.state(v, "removed reason"))
	if got := files[helper]; got.seq != seq || got.state != "removed" {
		t.Fatalf("removed path reason = %+v", got)
	}
	f.put(helper, "returned after deletion\n")
	f.head = f.commit("restore check input")
	_, files = f.freeze(v, "stale deletion reason", f.state(v, "stale deletion reason"))
	if got := files[helper]; !got.nullReason {
		t.Fatalf("null binding survived re-adding the file: %+v", got)
	}
}

func TestRules7ReviewTableSnapshotAndNames(t *testing.T) {
	const v = "rules7-review-table"
	const helper = "checks/helper.txt"
	cond := ccChange("AC-1", "")
	cond.paths = []string{ccScript("AC-1"), ccDecl("AC-1"), helper}
	f := ccRepo(t, ccGoal{tier: "standard", conds: []ccCond{cond, ccMaintain("AC-2")}})
	f.start(v)
	if _, ok := ciLines(f.rgFixture)[0].m["check_paths"]; ok {
		t.Fatal("start record must not keep the freeze table")
	}
	gate := ccGate(f, ccRound(f, "candidate", f.base, f.base, false))
	seq := cwGateSeq(f, v, "first freeze note", gate)
	lines := ciLines(f.rgFixture)
	paths, _ := lines[len(lines)-1].m["check_paths"].(map[string]any)
	if len(paths) != 1 || !reflect.DeepEqual(paths["AC-1"], []any{ccScript("AC-1"), ccDecl("AC-1"), helper}) {
		t.Fatalf("review check_paths = %v; want the change condition paths", paths)
	}
	f.setPaths("AC-1", ccScript("AC-1"), ccDecl("AC-1"))
	f.recordGate(v, "same gate with a later condition table", gate)
	fz, files := f.freeze(v, "earliest snapshot retained", f.state(v, "earliest snapshot retained"))
	if fz["series"] != "demo-rounds" || fz["round"] != "01-candidate" || fz["table_from"] != float64(seq) || files[helper].state != "removed" {
		t.Fatalf("freeze identity and original table = %v, files %v", fz, files)
	}
}

func TestRules7CheckChangeRejectedByEarlierRules(t *testing.T) {
	for n := 1; n <= 6; n++ {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", "")}})
			rules := fmt.Sprintf("run-rules/%d", n)
			startRules := rules
			if cwSaved(rules) {
				startRules = "run-rules/3"
			}
			f.rgFixture.start("earlier-rule-check-change", "--rules", startRules)
			if cwSaved(rules) {
				cwSaveRules(f.rgFixture, rules)
			}
			before := f.records()
			code, out := f.ha("note", rgGoal, "check-change", "checks:"+ccScript("AC-1"), "--reason", "F1: repair check")
			if code != exitUsage || !bytes.Equal(before, f.records()) {
				t.Fatalf("earlier-rule note exit %d, record changed %t: %s", code, !bytes.Equal(before, f.records()), out)
			}
			if n >= 5 {
				f.recordGate("earlier-rule-review", "earlier-rule review", ccGate(f, ccRound(f, "candidate", f.base, f.base, false)))
				lines := ciLines(f.rgFixture)
				last := lines[len(lines)-1].m
				if _, ok := last["check_paths"]; ok {
					t.Fatal("earlier review acquired a condition table")
				}
				review := last["review"].(map[string]any)
				round := review["rounds"].([]any)[0].(map[string]any)
				if _, series := round["series"]; series {
					t.Fatal("earlier review acquired round display names")
				}
			}
		})
	}
}

func TestRules7EmptyFreezeTableIsObserved(t *testing.T) {
	const v = "rules7-empty-freeze-table"
	f := ccRepo(t, ccGoal{tier: "standard", conds: []ccCond{ccMaintain("AC-1")}})
	f.start(v)
	seq := cwGateSeq(f, v, "empty freeze table", ccGate(f, ccRound(f, "candidate", f.base, f.base, false)))
	lines := ciLines(f.rgFixture)
	paths, ok := lines[len(lines)-1].m["check_paths"].(map[string]any)
	if !ok || len(paths) != 0 {
		t.Fatalf("check_paths = %v; want a recorded empty object", paths)
	}
	fz, files := f.freeze(v, "empty freeze table", f.state(v, "empty freeze table"))
	if fz["table_from"] != float64(seq) || fz["removed_observed"] != true || len(files) != 0 {
		t.Fatalf("empty table was not observed: %v, files %v", fz, files)
	}
}

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
