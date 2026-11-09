package main

// Checks of the goal core-completion-integrity for the review gate and the
// completion record under run-rules/6 (AC-2, AC-3, AC-4). Helpers are in
// core_integrity_test.go and review_gate_test.go.

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

const ciBaseReason = "review_base_mismatch"

// ciExpectBaseReason reports unless the goal is pending with exactly the
// base reason among its review reasons, classed as the executor's, naming
// the gate's seq, the last candidate round's base (when there is one) and
// the start base, and `ha done` records no completion.
func ciExpectBaseReason(f *rgFixture, violation, label string, gateSeq int, roundBase string) {
	f.t.Helper()
	s := f.status()
	r := ciReason(s, ciBaseReason)
	if r == nil || s.status == "complete" || s.code == 0 || !rgSame(s.review, []string{ciBaseReason}) {
		f.t.Errorf("jaekit:violation=%s %s: status %s (exit %d), review reasons %v; want pending with exactly %s", violation, label, s.status, s.code, s.review, ciBaseReason)
	} else {
		detail, _ := r["detail"].(string)
		want := []string{fmt.Sprintf("seq %d", gateSeq), f.base[:12]}
		if roundBase != "" {
			want = append(want, roundBase[:12])
		}
		var missing []string
		for _, w := range want {
			if !strings.Contains(detail, w) {
				missing = append(missing, w)
			}
		}
		if r["class"] != "executor" || len(missing) > 0 {
			f.t.Errorf("jaekit:violation=%s %s: reason %s has class %v and detail %q missing %v", violation, label, ciBaseReason, r["class"], detail, missing)
		}
	}
	if code, after := f.done(); code == 0 || after.completion != nil {
		f.t.Errorf("jaekit:violation=%s %s: ha done exit %d, completion record %v; want no completion", violation, label, code, after.completion)
	}
}

// The forms of each check run one after another in the linked test: the
// runner adapter reads only the linked test's own failure messages, so a
// subtest would hide them (contracts/check-result.md §5).

// AC-2: under run-rules/6 a standard or strict goal completes only when the
// last candidate round of the last gate (dry runs left out) starts at the
// start record's base commit; fixes rounds after it stay Spar's chain.
func TestCoreIntegrityAC2_ReviewStartsAtTheBase(t *testing.T) {
	const v = "review-base-unchecked"
	type form struct {
		label, tier string
		rounds      func(f *rgFixture) []rgRound
		ok          bool
		roundBase   func(f *rgFixture) string
	}
	work := func(f *rgFixture) string { return f.work }
	forms := []form{
		{"candidate W..H only", "standard", func(f *rgFixture) []rgRound {
			return []rgRound{ciRound(f, "candidate", f.work, f.head)}
		}, false, work},
		{"candidate B..W then candidate W..H", "standard", func(f *rgFixture) []rgRound {
			return []rgRound{ciRound(f, "candidate", f.base, f.work), ciRound(f, "candidate", f.work, f.head)}
		}, false, work},
		{"candidate B..H", "standard", func(f *rgFixture) []rgRound {
			return []rgRound{ciRound(f, "candidate", f.base, f.head)}
		}, true, nil},
		{"candidate B..W then fixes W..H", "standard", func(f *rgFixture) []rgRound {
			return []rgRound{ciRound(f, "candidate", f.base, f.work), ciRound(f, "fixes", f.work, f.head)}
		}, true, nil},
		{"candidate B..H then a dry-run candidate W..H", "standard", func(f *rgFixture) []rgRound {
			dry := ciRound(f, "candidate", f.work, f.head)
			dry.dryRun = true
			return []rgRound{ciRound(f, "candidate", f.base, f.head), dry}
		}, true, nil},
		{"fixes W..H without a candidate round", "standard", func(f *rgFixture) []rgRound {
			return []rgRound{ciRound(f, "fixes", f.work, f.head)}
		}, false, nil},
		{"strict: plan, then candidate W..H", "strict", func(f *rgFixture) []rgRound {
			return []rgRound{ciRound(f, "plan", f.base, f.base), ciRound(f, "candidate", f.work, f.head)}
		}, false, work},
		{"strict: plan, then candidate B..H", "strict", func(f *rgFixture) []rgRound {
			return []rgRound{ciRound(f, "plan", f.base, f.base), ciRound(f, "candidate", f.base, f.head)}
		}, true, nil},
	}
	for _, fm := range forms {
		func() {
			f := ciFixture(t, v, fm.tier)
			f.recordGate(v, fm.label, ciGate(f, fm.rounds(f)...))
			seq := ciLastSeq(f)
			if fm.ok {
				if s := f.status(); s.code != 0 || s.status != "complete" || ciReason(s, ciBaseReason) != nil || len(s.review) > 0 {
					t.Errorf("jaekit:violation=%s %s: status %s (exit %d), reasons %v; want complete", v, fm.label, s.status, s.code, s.reasons)
				}
				f.expectDone(v, fm.label)
				return
			}
			base := ""
			if fm.roundBase != nil {
				base = fm.roundBase(f)
			}
			ciExpectBaseReason(f, v, fm.label, seq, base)
		}()
	}

	// After a reopen the comparison is still with the start record's base.
	func() {
		f := ciFixture(t, v, "standard")
		f.recordGate(v, "first gate", ciGate(f, ciRound(f, "candidate", f.base, f.head)))
		ciMustDone(f, "first completion")
		ciMust(f, exitOK, "note", rgGoal, "reopen", "--quote", "리뷰 반영해서 다시 고쳐줘")
		old := f.head
		f.put("src/work.txt", "three\n")
		f.head = f.commit("rework")
		f.check()
		f.recordGate(v, "candidate from the old head", ciGate(f, ciRound(f, "candidate", old, f.head)))
		ciExpectBaseReason(f, v, "after reopen, candidate H..H2", ciLastSeq(f), old)
		f.recordGate(v, "candidate from the start base", ciGate(f, ciRound(f, "candidate", f.base, f.head)))
		f.expectComplete(v, "after reopen, candidate B..H2")
		f.expectDone(v, "after reopen, candidate B..H2")
	}()
}

// AC-3: under run-rules/6 a review note after a completion record voids it
// only when the current tier judges the gate (standard, strict).
func TestCoreIntegrityAC3_LateReviewNoteByTier(t *testing.T) {
	const v = "late-review-note-voids-unreviewed"
	for _, tier := range []string{"light", ""} {
		label := "tier " + tier
		if tier == "" {
			label = "no tier line"
		}
		func() {
			f := ciFixture(t, v, tier)
			done := ciMustDone(f, label)
			f.recordGate(v, label+": late gate", ciGate(f, ciRound(f, "candidate", f.work, f.head)))
			if s := f.status(); s.code != 0 || s.status != "complete" || ciCompletion(s) != done {
				t.Errorf("jaekit:violation=%s %s: after a later review note status %s (exit %d), completion record %v; want complete with seq %d", v, label, s.status, s.code, s.completion, done)
			}
			if line := ciCompletionLine(f); line != fmt.Sprintf("- completion record: seq %d", done) {
				t.Errorf("jaekit:violation=%s %s: ha status prints %q; want the completion record seq %d", v, label, line, done)
			}
		}()
	}
	for _, tier := range []string{"standard", "strict"} {
		func() {
			f := ciFixture(t, v, tier)
			stages := []string{"candidate"}
			if tier == "strict" {
				stages = []string{"plan", "candidate"}
			}
			f.recordGate(v, tier+": covering gate", f.covering(stages...))
			done := ciMustDone(f, tier)
			blocked := f.covering(stages...)
			blocked.state, blocked.defect = "blocked", 1
			f.recordGate(v, tier+": blocked gate", blocked)
			s := f.status()
			review := append([]string(nil), s.review...)
			sort.Strings(review)
			if s.completion != nil || s.status == "complete" || !rgSame(review, []string{"review_blocked", "review_open_defect"}) {
				t.Errorf("jaekit:violation=%s %s: after a later blocked gate status %s, completion record %v, review reasons %v; want the completion voided and the new gate judged", v, tier, s.status, s.completion, s.review)
			}
			f.recordGate(v, tier+": covering gate again", f.covering(stages...))
			if code, after := f.done(); code != 0 || after.completion == nil || ciCompletion(after) == done {
				t.Errorf("jaekit:violation=%s %s: ha done after a new covering gate exit %d, completion record %v; want a new completion record", v, tier, code, after.completion)
			}
		}()
	}
}

// AC-4: under run-rules/6 a done line whose record head is not the line
// before it is not a completion record, and the computation keeps looking at
// the done lines before it.
func TestCoreIntegrityAC4_UnjudgedDoneLineIsSkipped(t *testing.T) {
	const v = "unjudged-done-hides-completion"
	bundleOnly := func(f *rgFixture, note string) {
		f.put(rgGoal+"/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | "+note+" |\n")
		f.git("add", "-A")
		f.git("commit", "-q", "-m", "bundle documents only: "+note)
	}

	// The example of #151: D1 at H with a covering gate, a commit of bundle
	// documents only, D2 judged, D3 computed against D1 as well.
	func() {
		f := ciFixture(t, v, "standard")
		f.recordGate(v, "covering gate", f.covering("candidate"))
		ciMustDone(f, "D1")
		bundleOnly(f, "merged")
		d2 := ciMustDone(f, "D2")
		lines := ciLines(f)
		last := lines[len(lines)-1]
		ciWrite(f, append(lines, ciUnjudged(f, last, last)))
		s := f.status()
		if s.code != 0 || s.status != "complete" || ciCompletion(s) != d2 {
			t.Errorf("jaekit:violation=%s #151: status %s (exit %d), completion record %v, reasons %v; want complete with D2 (seq %d)", v, s.status, s.code, s.completion, s.reasons, d2)
		}
		if line := ciCompletionLine(f); line != fmt.Sprintf("- completion record: seq %d", d2) {
			t.Errorf("jaekit:violation=%s #151: ha status prints %q; want the completion record D2 (seq %d) only", v, line, d2)
		}
	}()

	// A review note after the judged line still voids it under standard.
	func() {
		f := ciFixture(t, v, "standard")
		f.recordGate(v, "covering gate", f.covering("candidate"))
		ciMustDone(f, "D1")
		bundleOnly(f, "merged")
		ciMustDone(f, "D2")
		f.recordGate(v, "later gate", f.covering("candidate"))
		lines := ciLines(f)
		d2 := lines[len(lines)-2]
		ciWrite(f, append(lines, ciUnjudged(f, d2, lines[len(lines)-1])))
		if s := f.status(); s.completion != nil {
			t.Errorf("jaekit:violation=%s a review note after D2: completion record %v; want none", v, s.completion)
		}
	}()

	// With no valid completion before it, the line changes nothing.
	func() {
		f := ciFixture(t, v, "light")
		ciMustDone(f, "D1")
		f.put("src/work.txt", "changed after completion\n")
		f.head = f.commit("code change")
		without := f.records()
		sWithout, lineWithout := f.status(), ciCompletionLine(f)
		lines := ciLines(f)
		last := lines[len(lines)-1]
		ciWrite(f, append(lines, ciUnjudged(f, last, last)))
		s, line := f.status(), ciCompletionLine(f)
		if s.code != sWithout.code || s.status != sWithout.status || !rgSame(s.reasons, sWithout.reasons) || (s.completion == nil) != (sWithout.completion == nil) || line != lineWithout {
			t.Errorf("jaekit:violation=%s no earlier completion: with the unjudged line status %s (exit %d) reasons %v completion %v, %q; without it %s (exit %d) reasons %v completion %v, %q (%d bytes)",
				v, s.status, s.code, s.reasons, s.completion, line, sWithout.status, sWithout.code, sWithout.reasons, sWithout.completion, lineWithout, len(without))
		}
	}()
}
