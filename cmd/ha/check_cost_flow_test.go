package main

// Check of the goal check-cost, AC-12: a synthetic standard goal in the Seal
// bundle format, with one marked change condition, one unmarked change
// condition and one maintain condition, goes through the real ha from start
// to a completion record under run-rules/7: the automatic limit from the
// run-rules/7 minimum, a baseline for the marked condition only, three
// passing checks, a review gate and ha done; a check file fixed after the
// first candidate round, with its reason recorded, shows in ha report. The
// helpers are in check_cost_test.go and check_cost_show_test.go.

import (
	"fmt"
	"strings"
	"testing"
)

func TestCheckCostAC12_SelectiveFlowEndToEnd(t *testing.T) {
	const v = "selective-flow-broken"
	const reason = "claude-F3: AC-2's producer compared the wrong value"
	f := ccRepo(t, ccGoal{tier: "standard", conds: []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3")},
		column: true, runs: "자동"})

	// The automatic limit: minimum 1 × 2 + 1 + 1 = 4, limit max(50, 6).
	f.start(v)
	b := f.startBudget()
	auto, _ := b["auto"].(map[string]any)
	runs, _ := ccNumber(b, "runs")
	minimum, _ := ccNumber(auto, "minimum_runs")
	if runs != 50 || minimum != 4 {
		t.Errorf("jaekit:violation=%s ha start --rules %s with 자동 records budget %v; want 50 runs from minimum 4", v, ccRules, b)
	}

	first := f.implement()
	before := len(ciLines(f.rgFixture))
	f.must(v, "the default baseline run", exitOK, "check", rgGoal, "--baseline")
	f.must(v, "the default checks", exitOK, "check", rgGoal)
	added := ciLines(f.rgFixture)[before:]
	if got := ccTargets(ccRuns(added, "baseline")); !rgSame(got, []string{"AC-1"}) {
		t.Errorf("jaekit:violation=%s the default baseline run recorded baselines of %v; want AC-1 only", v, got)
	}
	checks := ccRuns(added, "check")
	if got := ccTargets(checks); !rgSame(got, []string{"AC-1", "AC-2", "AC-3"}) {
		t.Errorf("jaekit:violation=%s the default checks recorded %v; want AC-1, AC-2 and AC-3", v, got)
	}
	for _, l := range append(ccRuns(added, "baseline"), checks...) {
		if want := map[string]string{"baseline": "fail_as_expected", "check": "pass"}[l.kind()]; l.m["result"] != want {
			t.Errorf("jaekit:violation=%s the %s of %s gave %v; want %s", v, l.kind(), ccTarget(l), l.m["result"], want)
		}
	}

	// The first candidate round reviews the first head; its finding about
	// AC-2's check is fixed and its reason recorded. AC-2 needs no new
	// baseline.
	f.recordGate(v, "the first candidate round", ccGate(f, ccRound(f, "candidate", f.base, first, false)))
	f.revise(ccScript("AC-2"), "fixed after the first candidate round")
	f.head = f.commit("fix AC-2's check")
	seq := f.changeNote(v, ccScript("AC-2"), reason)
	f.must(v, "the checks after the fix", exitOK, "check", rgGoal)
	f.recordGate(v, "the covering gate", ccGate(f, ccRound(f, "candidate", f.base, first, false), ccRound(f, "fixes", first, f.head, false),
		ccRound(f, "candidate", f.base, f.head, false)))

	s := f.state(v, "the covered goal")
	if s.status != "complete" || s.code != exitOK {
		t.Errorf("jaekit:violation=%s the covered goal is %s (exit %d) with reasons %v; want complete", v, s.status, s.code, s.reasons)
	}
	if code, after := f.done(); code != exitOK || after.completion == nil {
		t.Fatalf("jaekit:violation=%s ha done exit %d, completion record %v, reasons %v; want a completion record", v, code, after.completion, after.reasons)
	}
	if got := ccTargets(ccRuns(ciLines(f.rgFixture), "baseline")); !rgSame(got, []string{"AC-1"}) {
		t.Errorf("jaekit:violation=%s the completed goal's record has baselines of %v; want AC-1 only", v, got)
	}

	for _, lang := range []string{"ko", "en"} {
		review := f.summary(v, "the completed goal", lang, 1)
		item, ok := ccItem(review, ccFreezeItems[lang][0])
		want := fmt.Sprintf("%s (seq %d)", ccScript("AC-2"), seq)
		if !ok || !rgSame(ccPaths(item), []string{ccScript("AC-2")}) || !strings.Contains(item, want) {
			t.Errorf("jaekit:violation=%s ha report --lang %s to-review line %q; want %q", v, lang, review, ccFreezeItems[lang][0]+want)
		}
	}
}
