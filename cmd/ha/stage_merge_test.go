package main

// Checks of the spar-stage-merge goal in Seal Core: `ha status` and `ha done`
// judge a recorded review gate of plan and candidate rounds only, and keep
// judging a gate of the earlier stages plan-delta and fixes as they did
// before the change. The tests drive the ha command line (runWith) on
// synthetic repositories and write review gates in the public review-gate/v1
// form, as `spar gate` writes them; they never run Spar or a reviewer.
// Helpers are in review_gate_test.go and core_integrity_test.go. Both checks
// are kept behaviors: a failure carries no marker, and a broken setup fails
// with "setup:".

import (
	"fmt"
	"testing"
)

// smRules are the run rules whose goals judge a review gate.
var smRules = []string{"run-rules/5", "run-rules/6", "run-rules/7"}

// smFixture starts a goal of the tier under rules, adds two code commits
// (work and head) and records a passing AC-1 check at the head.
func smFixture(t *testing.T, rules, tier string) *rgFixture {
	t.Helper()
	rgRequireGit(t)
	f := rgRepo(t, tier)
	if code, out := f.ha("start", rgGoal, "--request", "implement the synthetic goal", "--rules", rules); code != exitOK {
		t.Fatalf("setup: ha start --rules %s exit %d: %s", rules, code, out)
	}
	f.base = f.git("rev-parse", "HEAD")
	f.put("src/work.txt", "one\n")
	f.work = f.commit("work")
	f.put("src/work.txt", "two\n")
	f.head = f.commit("fix")
	f.check()
	return f
}

// smGateValue is g as spar gate writes it: a fixes round also adds its lines
// to the gate's growth.
func smGateValue(g rgGate) map[string]any {
	value := g.gateValue()
	growth := value["growth"].(map[string]any)
	added := growth["added_lines"].(map[string]any)
	rounds := []any{}
	for i, r := range g.rounds {
		if r.stage != "fixes" || r.dryRun {
			continue
		}
		rounds = append(rounds, map[string]any{"path": fmt.Sprintf("%s/%02d-%s", rgRoundFolder, i+1, r.stage),
			"base": r.base, "head": r.head, "check": 0, "other": 1})
		added["other"] = added["other"].(int) + 1
	}
	growth["rounds"] = rounds
	return value
}

// smRecord records a complete gate of the rounds, which expects and last
// reviewed HEAD.
func smRecord(f *rgFixture, label string, rounds ...rgRound) {
	f.t.Helper()
	path, _ := f.write(smGateValue(rgGate{state: "complete", expect: f.head, last: f.head, rounds: rounds}))
	if code, out := f.ha("note", rgGoal, "review", "--gate", path); code != exitOK {
		f.t.Fatalf("setup: %s: ha note review exit %d: %s", label, code, out)
	}
}

// smComplete reports unless the goal is complete without any reason and
// `ha done` records completion.
func smComplete(f *rgFixture, label string) {
	f.t.Helper()
	if s := f.status(); s.code != 0 || s.status != "complete" || len(s.reasons) > 0 {
		f.t.Errorf("%s: status %s (exit %d), reasons %v; want complete without reasons", label, s.status, s.code, s.reasons)
	}
	if code, s := f.done(); code != exitOK || s.completion == nil {
		f.t.Errorf("%s: ha done exit %d, status %s, reasons %v; want a completion record", label, code, s.status, s.reasons)
	}
}

// AC-6: a gate of plan (strict only) and candidate rounds, the last candidate
// round from the goal's start base to HEAD, completes a standard and a strict
// goal.
func TestSparStageMergeAC6_HaDoneWithPlanAndCandidateGate(t *testing.T) {
	t.Parallel()
	for _, rules := range smRules {
		for _, tier := range []string{"standard", "strict"} {
			label := fmt.Sprintf("%s %s goal with a gate of plan and candidate rounds", rules, tier)
			f := smFixture(t, rules, tier)
			var rounds []rgRound
			if tier == "strict" {
				rounds = append(rounds, ciRound(f, "plan", f.base, f.base))
			}
			rounds = append(rounds, ciRound(f, "candidate", f.base, f.work), ciRound(f, "candidate", f.base, f.head))
			smRecord(f, label, rounds...)
			smComplete(f, label)
		}
	}
}

// AC-7: a gate recorded with the earlier stages is judged as before the
// change: plan-delta counts as the plan round of a strict goal, a fixes
// round continues the code rounds, and a gate whose code rounds are fixes
// rounds only gets review_base_mismatch under run-rules/6 and /7.
func TestSparStageMergeAC7_HaReadsLegacyGate(t *testing.T) {
	t.Parallel()
	for _, rules := range smRules {
		for _, tier := range []string{"standard", "strict"} {
			label := fmt.Sprintf("%s %s goal with a gate of plan, plan-delta, candidate, fixes and candidate rounds", rules, tier)
			f := smFixture(t, rules, tier)
			smRecord(f, label, ciRound(f, "plan", f.base, f.base), ciRound(f, "plan-delta", f.base, f.base),
				ciRound(f, "candidate", f.base, f.work), ciRound(f, "fixes", f.work, f.head), ciRound(f, "candidate", f.base, f.head))
			smComplete(f, label)
		}

		label := rules + " strict goal with a gate of a plan-delta round and no plan round"
		f := smFixture(t, rules, "strict")
		smRecord(f, label, ciRound(f, "plan-delta", f.base, f.base), ciRound(f, "candidate", f.base, f.head))
		smComplete(f, label)

		label = rules + " standard goal with a gate of a fixes round only"
		f = smFixture(t, rules, "standard")
		smRecord(f, label, ciRound(f, "fixes", f.work, f.head))
		if rules == "run-rules/5" {
			// run-rules/5 does not compare the review with the start base.
			smComplete(f, label)
			continue
		}
		if s := f.status(); s.code == 0 || s.status == "complete" || !rgSame(s.reasons, []string{ciBaseReason}) {
			t.Errorf("%s: status %s (exit %d), reasons %v; want pending with exactly %s", label, s.status, s.code, s.reasons, ciBaseReason)
		}
		if code, s := f.done(); code == exitOK || s.completion != nil {
			t.Errorf("%s: ha done exit %d, completion record %v; want no completion", label, code, s.completion)
		}
	}
}
