package main

// Checks of the goal check-cost, AC-5 and AC-10: a run-rules/7 goal's status
// and report show which change conditions require a baseline and which marks
// were removed after the start (AC-5), and, for a standard or strict goal
// with a recorded gate, the check files changed after the freeze round with
// the reasons recorded for them (AC-10). The helpers are in
// check_cost_test.go; see there for how a violation and a broken setup are
// told apart.

import (
	"fmt"
	"sort"
	"strings"
	"testing"
)

// ccRound is a gate round of the fixture's current goal documents.
func ccRound(f *ccFixture, stage, base, head string, dry bool) rgRound {
	return rgRound{stage: stage, base: base, head: head, docs: f.docs(), required: 3, completed: 3, dryRun: dry}
}

// ccGate is a complete gate with no open finding whose expected and last
// reviewed head are the fixture's head.
func ccGate(f *ccFixture, rounds ...rgRound) rgGate {
	return rgGate{state: "complete", expect: f.head, last: f.head, rounds: rounds}
}

// changeNote records `ha note <goal> check-change checks:<path> --reason`
// and returns its seq; a refusal is the observed violation.
func (f *ccFixture) changeNote(violation, path, reason string) int {
	f.t.Helper()
	f.must(violation, "a check-change reason for "+path, exitOK, "note", rgGoal, "check-change", "checks:"+path, "--reason", reason)
	return ciLines(f.rgFixture)[len(ciLines(f.rgFixture))-1].seq()
}

// ccFrozen is one file of the status JSON check_freeze: its state and its
// reason's seq and text (seq 0 for a null reason).
type ccFrozen struct {
	state      string
	seq        int
	reason     string
	nullReason bool
}

// freeze reads check_freeze from the status; a missing or malformed field is
// the observed violation.
func (f *ccFixture) freeze(violation, label string, s rgState) (map[string]any, map[string]ccFrozen) {
	f.t.Helper()
	fz, ok := s.value["check_freeze"].(map[string]any)
	if !ok {
		f.t.Fatalf("jaekit:violation=%s %s: the status JSON has check_freeze %v; want the freeze", violation, label, s.value["check_freeze"])
	}
	list, ok := fz["files"].([]any)
	if !ok {
		f.t.Fatalf("jaekit:violation=%s %s: check_freeze has files %v; want a list", violation, label, fz["files"])
	}
	files := map[string]ccFrozen{}
	for _, item := range list {
		m, _ := item.(map[string]any)
		path, _ := m["path"].(string)
		state, _ := m["state"].(string)
		entry := ccFrozen{state: state, nullReason: m["reason"] == nil}
		if r, ok := m["reason"].(map[string]any); ok {
			entry.seq, _ = ccNumber(r, "seq")
			entry.reason, _ = r["reason"].(string)
		}
		if _, dup := files[path]; dup || path == "" {
			f.t.Errorf("jaekit:violation=%s %s: check_freeze lists %q more than once or without a path", violation, label, path)
		}
		files[path] = entry
	}
	return fz, files
}

// ccFreezeItems are the report's to-review items for frozen check files: the
// paths with a reason (each as `<path> (seq n: <reason text>)`) and the paths
// without one.
var ccFreezeItems = map[string][2]string{
	"ko": {"동결 뒤 바뀐 검사 파일(까닭 있음): ", "동결 뒤 까닭 없이 바뀐 검사 파일: "},
	"en": {"check files changed after the freeze with a reason: ", "check files changed after the freeze without a reason: "},
}

// ccWithReason is a report item of a frozen check file with its reason.
func ccWithReason(path string, seq int, reason string) string {
	return fmt.Sprintf("%s (seq %d: %s)", path, seq, reason)
}

// ccNotRecorded is what the human status ("en") and each report language say
// when no review note kept the freeze-time condition table, so check paths
// removed after the freeze are not observed.
var ccNotRecorded = map[string]string{
	"en": "freeze-time condition table was not recorded",
	"ko": "동결 때의 조건표가 기록되지 않음",
}

// notRecorded reports whether the human status and both report languages say
// that the freeze-time condition table was not recorded, against want.
func (f *ccFixture) notRecorded(violation, label string, want bool) {
	f.t.Helper()
	human := f.output(violation, label, "status", rgGoal)
	if got := strings.Contains(human, ccNotRecorded["en"]); got != want {
		f.t.Errorf("jaekit:violation=%s %s: ha status says %q: %v; want %v:\n%s", violation, label, ccNotRecorded["en"], got, want, human)
	}
	for _, lang := range []string{"ko", "en"} {
		review := f.summary(violation, label, lang, 1)
		if got := strings.Contains(review, ccNotRecorded[lang]); got != want {
			f.t.Errorf("jaekit:violation=%s %s: ha report --lang %s to-review line %q says %q: %v; want %v", violation, label, lang, review, ccNotRecorded[lang], got, want)
		}
	}
}

// ccPaths are the paths of a report list item, each item's first word.
func ccPaths(item string) []string {
	var out []string
	for _, entry := range strings.Split(item, ",") {
		if words := strings.Fields(entry); len(words) > 0 {
			out = append(out, words[0])
		}
	}
	sort.Strings(out)
	return out
}

// noFreeze reports a freeze display in the status JSON, the human status or
// either report language.
func (f *ccFixture) noFreeze(violation, label string) {
	f.t.Helper()
	s := f.state(violation, label)
	if fz, ok := s.value["check_freeze"]; ok {
		f.t.Errorf("jaekit:violation=%s %s: the status JSON has check_freeze %v; want none", violation, label, fz)
	}
	human := f.output(violation, label, "status", rgGoal)
	if line, ok := ccLine(human, "- check freeze:"); ok {
		f.t.Errorf("jaekit:violation=%s %s: ha status prints %q; want no freeze line", violation, label, line)
	}
	if strings.Contains(human, ccNotRecorded["en"]) {
		f.t.Errorf("jaekit:violation=%s %s: ha status says %q; want no freeze display", violation, label, ccNotRecorded["en"])
	}
	for _, lang := range []string{"ko", "en"} {
		review := f.summary(violation, label, lang, 1)
		items := ccFreezeItems[lang]
		for _, prefix := range append(items[:], ccNotRecorded[lang]) {
			if strings.Contains(review, prefix) {
				f.t.Errorf("jaekit:violation=%s %s: ha report --lang %s lists %q; want no freeze item", violation, label, lang, strings.TrimSpace(prefix))
			}
		}
	}
}

// AC-5: the status and report of a run-rules/7 goal show which change
// conditions require a baseline, how many of them have a fresh one, and the
// marks removed after the start.
func TestCheckCostAC5_StatusAndReportShowMark(t *testing.T) {
	const v = "baseline-requirement-hidden"
	f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3"), ccChange("AC-4", ccMark)}, column: true})
	f.start(v)
	f.implement()
	f.must(v, "the current checks", exitOK, "check", rgGoal)
	f.must(v, "the baseline of the marked AC-1", exitOK, "check", rgGoal, "AC-1", "--baseline")
	f.must(v, "the baseline of the unmarked AC-2", exitOK, "check", rgGoal, "AC-2", "--baseline")

	// show checks every display for the current marks: required are the
	// change conditions the PLAN marks now, fresh how many of them have a
	// fresh baseline, removed the marks of the start that are gone.
	show := func(label string, required []string, fresh int, removed []string) {
		t.Helper()
		s := f.state(v, label)
		for _, id := range []string{"AC-1", "AC-2", "AC-4"} {
			want := false
			for _, r := range required {
				want = want || r == id
			}
			c := ccCriterion(s, id)
			if got, ok := c["baseline_required"].(bool); !ok || got != want {
				t.Errorf("jaekit:violation=%s %s: status JSON %s baseline_required %v; want %v", v, label, id, c["baseline_required"], want)
			}
		}
		const prefix = "- baseline required: "
		line, ok := ccLine(f.output(v, label, "status", rgGoal), prefix)
		got := strings.TrimPrefix(line, prefix)
		want := strings.Join(required, ", ")
		if len(required) == 0 {
			want = "none"
		}
		if !ok || (len(required) == 0 && got != "none") || (len(required) > 0 && !rgSame(ccList(got), required)) {
			t.Errorf("jaekit:violation=%s %s: ha status prints %q; want %q", v, label, line, prefix+want)
		}

		change := 3
		counts := map[string]string{
			"ko": fmt.Sprintf("change 조건 %d개 중 %d개가 baseline을 요구하고 그중 %d개에 신선한 baseline", change, len(required), fresh),
			"en": fmt.Sprintf("%d of %d change conditions require a baseline; %d of them have a fresh baseline", len(required), change, fresh),
		}
		removedItem := map[string]string{"ko": "시작 뒤 baseline 표시를 뺀 조건: ", "en": "baseline mark removed after start: "}
		for _, lang := range []string{"ko", "en"} {
			if checked := f.summary(v, label, lang, 0); !strings.Contains(checked, counts[lang]) {
				t.Errorf("jaekit:violation=%s %s: ha report --lang %s checked line %q; want %q", v, label, lang, checked, counts[lang])
			}
			review := f.summary(v, label, lang, 1)
			item, ok := ccItem(review, removedItem[lang])
			if len(removed) == 0 && ok {
				t.Errorf("jaekit:violation=%s %s: ha report --lang %s lists %q; want no removed mark", v, label, lang, removedItem[lang]+item)
			}
			if len(removed) > 0 && (!ok || !rgSame(ccList(item), removed)) {
				t.Errorf("jaekit:violation=%s %s: ha report --lang %s to-review line %q; want %q", v, label, lang, review, removedItem[lang]+strings.Join(removed, ", "))
			}
		}
	}

	// The marked AC-1 and AC-4; only AC-1 has a baseline among them (the
	// unmarked AC-2's baseline does not count).
	show("the marks of the start", []string{"AC-1", "AC-4"}, 1, nil)
	f.mark("AC-4", "—")
	show("AC-4's mark removed after the start", []string{"AC-1"}, 1, []string{"AC-4"})
	f.mark("AC-1", "")
	show("both marks of the start removed", nil, 0, []string{"AC-1", "AC-4"})
	f.mark("AC-2", ccMark)
	show("both marks removed and the baselined AC-2 marked", []string{"AC-2"}, 1, []string{"AC-1", "AC-4"})
}

// AC-10: a run-rules/7 standard or strict goal shows the check files changed
// after the freeze round of its last recorded gate, with the reasons that
// still hold for them, in its status and report. The check paths of the
// freeze-time condition table come from the earliest review note whose gate
// ends with the freeze round; without one, removed check paths are not
// observed and the status and report say so. The display never blocks
// completion and is absent for other goals.
func TestCheckCostAC10_FreezeShown(t *testing.T) {
	const v = "check-freeze-hidden"
	const (
		reasonOne = "claude-F1: the producer named the wrong value file"
		reasonTwo = "codex-F2: AC-2's producer read stale output"
	)
	// AC-2's 검사 경로 at the start: its producer, its declaration and two
	// check inputs.
	conds := func() []ccCond {
		extra := ccChange("AC-2", "—")
		extra.paths = []string{ccScript("AC-2"), ccDecl("AC-2"), "checks/ac-2-extra.txt", "checks/ac-2-early.txt"}
		return []ccCond{ccChange("AC-1", ccMark), extra, ccMaintain("AC-3")}
	}
	short := func(line, head string) bool {
		i := strings.LastIndex(line, " at ")
		token := ""
		if i >= 0 {
			token = strings.TrimSpace(line[i+len(" at "):])
		}
		return len(token) >= 7 && strings.HasPrefix(head, token)
	}
	observed := func(label string, fz map[string]any, want bool) {
		t.Helper()
		if got, ok := fz["removed_observed"].(bool); !ok || got != want {
			t.Errorf("jaekit:violation=%s %s: check_freeze removed_observed %v; want %v", v, label, fz["removed_observed"], want)
		}
	}
	rounds := func(list []rgRound, more ...rgRound) []rgRound { return append(append([]rgRound{}, list...), more...) }

	// A standard goal: the freeze is the first candidate round of the last
	// recorded gate that is not a dry run, and a gate recorded right after
	// that round keeps the freeze-time table.
	func() {
		f := ccRepo(t, ccGoal{tier: "standard", conds: conds(), column: true})
		f.start(v)
		// An earlier gate, recorded while the table still had the check
		// paths of the start; its only candidate round reviewed the base.
		f.recordGate(v, "an earlier gate", ccGate(f, ccRound(f, "candidate", f.base, f.base, false)))
		// Before the freeze: a check file changed without a reason, a check
		// path added to the table after the start and one of the start
		// removed from it.
		f.revise(ccScript("AC-2"), "settled before the first candidate round")
		f.put("checks/ac-2-late.txt", "a check input added after the start\n")
		f.setPaths("AC-2", ccScript("AC-2"), ccDecl("AC-2"), "checks/ac-2-extra.txt", "checks/ac-2-late.txt")
		frozen := f.implement()
		f.must(v, "the current checks at the freeze head", exitOK, "check", rgGoal)
		f.must(v, "the marked baseline at the freeze head", exitOK, "check", rgGoal, "--baseline")

		// The gate recorded right after the freeze round: a plan round and a
		// dry-run candidate round before the first counted candidate round.
		freezeRounds := []rgRound{ccRound(f, "plan", f.base, f.base, false), ccRound(f, "candidate", f.base, f.base, true),
			ccRound(f, "candidate", f.base, frozen, false)}
		f.recordGate(v, "the gate recorded right after the freeze round", ccGate(f, freezeRounds...))
		label := "the freeze head is HEAD"
		fz, files := f.freeze(v, label, f.state(v, label))
		if fz["stage"] != "candidate" || fz["head"] != frozen || len(files) > 0 {
			t.Errorf("jaekit:violation=%s %s: check_freeze stage %v head %v files %v; want candidate at %s with no file", v, label, fz["stage"], fz["head"], files, frozen)
		}
		observed(label, fz, true)
		f.notRecorded(v, label, false)

		// After the freeze: AC-1's producer changed with a reason that
		// holds; AC-2's producer changed with a reason, then changed again;
		// a check path added to the table, and both check inputs of the
		// freeze-time table removed from it: one the start had, one added
		// after the start.
		f.revise(ccScript("AC-1"), "fix after the first candidate round")
		f.revise(ccScript("AC-2"), "fix after the first candidate round")
		f.head = f.commit("fix the checks after the review")
		one := f.changeNote(v, ccScript("AC-1"), reasonOne)
		f.changeNote(v, ccScript("AC-2"), reasonTwo)
		f.revise(ccScript("AC-2"), "changed again after its reason")
		f.put("checks/ac-2-helper.txt", "a check input added after the freeze\n")
		f.setPaths("AC-2", ccScript("AC-2"), ccDecl("AC-2"), "checks/ac-2-helper.txt")
		fixes := f.head
		f.head = f.commit("change the checks again")
		f.must(v, "the current checks at the last head", exitOK, "check", rgGoal)
		f.must(v, "the marked baseline at the last head", exitOK, "check", rgGoal, "--baseline")
		f.recordGate(v, "the last gate", ccGate(f, rounds(freezeRounds, ccRound(f, "fixes", frozen, fixes, false),
			ccRound(f, "fixes", fixes, f.head, false), ccRound(f, "candidate", f.base, f.head, false))...))

		label = "check files changed after the freeze"
		s := f.state(v, label)
		fz, files = f.freeze(v, label, s)
		want := map[string]ccFrozen{
			ccScript("AC-1"):         {state: "changed", seq: one, reason: reasonOne},
			ccScript("AC-2"):         {state: "changed", nullReason: true},
			"checks/ac-2-helper.txt": {state: "added", nullReason: true},
			"checks/ac-2-extra.txt":  {state: "removed", nullReason: true},
			"checks/ac-2-late.txt":   {state: "removed", nullReason: true},
		}
		if fz["stage"] != "candidate" || fz["head"] != frozen {
			t.Errorf("jaekit:violation=%s %s: check_freeze stage %v head %v; want candidate at %s", v, label, fz["stage"], fz["head"], frozen)
		}
		if got, ok := files["checks/ac-2-early.txt"]; ok {
			t.Errorf("jaekit:violation=%s %s: check_freeze lists checks/ac-2-early.txt, which the table lost before the freeze, as %+v", v, label, got)
		}
		if !ccSameFrozen(files, want) {
			t.Errorf("jaekit:violation=%s %s: check_freeze files %+v; want %+v", v, label, files, want)
		}
		observed(label, fz, true)
		f.notRecorded(v, label, false)

		human := f.output(v, label, "status", rgGoal)
		if line, ok := ccLine(human, "- check freeze: candidate round "); !ok || !short(line, frozen) {
			t.Errorf("jaekit:violation=%s %s: ha status freeze line %q; want `- check freeze: candidate round <series>/<round> at %s`", v, label, line, frozen[:12])
		}
		for path, w := range want {
			prefix := fmt.Sprintf("  - %s %s: ", path, w.state)
			line, ok := ccLine(human, prefix)
			if !ok || (w.nullReason && line != prefix+"no reason") || (!w.nullReason && !strings.Contains(line, w.reason)) {
				t.Errorf("jaekit:violation=%s %s: ha status file line %q; want %q followed by its reason or `no reason`", v, label, line, prefix)
			}
		}

		withItem := ccWithReason(ccScript("AC-1"), one, reasonOne)
		wantWithout := []string{"checks/ac-2-extra.txt", "checks/ac-2-helper.txt", "checks/ac-2-late.txt", ccScript("AC-2")}
		sort.Strings(wantWithout)
		for _, lang := range []string{"ko", "en"} {
			review := f.summary(v, label, lang, 1)
			with, okWith := ccItem(review, ccFreezeItems[lang][0])
			without, okWithout := ccItem(review, ccFreezeItems[lang][1])
			if !okWith || !rgSame(ccPaths(with), []string{ccScript("AC-1")}) || !strings.Contains(with, withItem) {
				t.Errorf("jaekit:violation=%s %s: ha report --lang %s to-review line %q; want %q", v, label, lang, review, ccFreezeItems[lang][0]+withItem)
			}
			if !okWithout || !rgSame(ccPaths(without), wantWithout) {
				t.Errorf("jaekit:violation=%s %s: ha report --lang %s to-review line %q; want %q", v, label, lang, review,
					ccFreezeItems[lang][1]+strings.Join(wantWithout, ", "))
			}
		}

		// The display never blocks completion.
		if s.status != "complete" || s.code != exitOK {
			t.Errorf("jaekit:violation=%s %s: status %s (exit %d), reasons %v; want complete", v, label, s.status, s.code, s.reasons)
		}
		if code, after := f.done(); code != exitOK || after.completion == nil {
			t.Errorf("jaekit:violation=%s %s: ha done exit %d, completion record %v, reasons %v; want a completion record", v, label, code, after.completion, after.reasons)
		}
	}()

	// A strict goal: the freeze is the first plan round that is not a dry
	// run, not a candidate round. The gate recorded right after it keeps the
	// freeze-time table, so a check path removed after it is listed.
	func() {
		f := ccRepo(t, ccGoal{tier: "strict", conds: conds(), column: true})
		f.start(v)
		f.revise(ccScript("AC-2"), "settled before the plan round")
		planned := f.commit("the checks the plan round reviews")
		f.head = planned
		planRounds := []rgRound{ccRound(f, "plan", f.base, f.base, true), ccRound(f, "plan", f.base, planned, false)}
		f.recordGate(v, "the gate recorded right after the plan round", ccGate(f, planRounds...))
		f.revise(ccScript("AC-1"), "fix after the plan round")
		f.setPaths("AC-2", ccScript("AC-2"), ccDecl("AC-2"), "checks/ac-2-early.txt")
		f.implement()
		f.recordGate(v, "a strict gate", ccGate(f, rounds(planRounds, ccRound(f, "candidate", f.base, f.head, false))...))
		label := "a strict goal with a check file changed and a check path removed after its plan round"
		fz, files := f.freeze(v, label, f.state(v, label))
		want := map[string]ccFrozen{ccScript("AC-1"): {state: "changed", nullReason: true}, "checks/ac-2-extra.txt": {state: "removed", nullReason: true}}
		if fz["stage"] != "plan" || fz["head"] != planned || !ccSameFrozen(files, want) {
			t.Errorf("jaekit:violation=%s %s: check_freeze stage %v head %v files %+v; want plan at %s with files %+v", v, label, fz["stage"], fz["head"], files, planned, want)
		}
		observed(label, fz, true)
		if line, ok := ccLine(f.output(v, label, "status", rgGoal), "- check freeze: plan round "); !ok || !short(line, planned) {
			t.Errorf("jaekit:violation=%s %s: ha status freeze line %q; want `- check freeze: plan round <series>/<round> at %s`", v, label, line, planned[:12])
		}
	}()

	// No review note right after the freeze round, only a gate recorded after
	// later rounds: the freeze-time table is not known. The changed check
	// file is still listed; the check path removed after the freeze is not
	// observed, and the status and report say that the freeze-time table was
	// not recorded.
	func() {
		f := ccRepo(t, ccGoal{tier: "standard", conds: conds(), column: true})
		f.start(v)
		first := f.implement()
		f.revise(ccScript("AC-1"), "fix after the first candidate round")
		f.setPaths("AC-2", ccScript("AC-2"), ccDecl("AC-2"), "checks/ac-2-early.txt")
		f.head = f.commit("fix the checks after the review")
		f.recordGate(v, "a gate recorded only after the fixes round", ccGate(f, ccRound(f, "candidate", f.base, first, false),
			ccRound(f, "fixes", first, f.head, false), ccRound(f, "candidate", f.base, f.head, false)))
		label := "a standard goal without a review note right after its freeze round"
		fz, files := f.freeze(v, label, f.state(v, label))
		want := map[string]ccFrozen{ccScript("AC-1"): {state: "changed", nullReason: true}}
		if fz["stage"] != "candidate" || fz["head"] != first || !ccSameFrozen(files, want) {
			t.Errorf("jaekit:violation=%s %s: check_freeze stage %v head %v files %+v; want candidate at %s with files %+v and no removed path",
				v, label, fz["stage"], fz["head"], files, first, want)
		}
		observed(label, fz, false)
		f.notRecorded(v, label, true)
	}()

	// No display: light, no tier line, no gate, and gates without the
	// freeze round. Each goal changes a check file after its rounds.
	for _, tier := range []string{"light", ""} {
		f := ccRepo(t, ccGoal{tier: tier, conds: conds(), column: true})
		f.start(v)
		f.implement()
		f.recordGate(v, "a gate of a goal without review", ccGate(f, ccRound(f, "plan", f.base, f.base, false), ccRound(f, "candidate", f.base, f.head, false)))
		f.revise(ccScript("AC-1"), "changed after the candidate round")
		f.head = f.commit("change a check")
		f.noFreeze(v, fmt.Sprintf("a goal with tier %q and a gate", tier))
	}
	func() {
		f := ccRepo(t, ccGoal{tier: "standard", conds: conds(), column: true})
		f.start(v)
		f.implement()
		f.revise(ccScript("AC-1"), "changed without a gate")
		f.head = f.commit("change a check")
		f.noFreeze(v, "a standard goal without a gate")
		f.recordGate(v, "a candidate gate", ccGate(f, ccRound(f, "candidate", f.base, f.base, false)))
		f.recordGate(v, "a last gate with only a dry-run candidate round", ccGate(f, ccRound(f, "plan", f.base, f.base, false),
			ccRound(f, "candidate", f.base, f.head, true)))
		f.noFreeze(v, "a standard goal whose last gate has no counted candidate round")
	}()
	func() {
		f := ccRepo(t, ccGoal{tier: "strict", conds: conds(), column: true})
		f.start(v)
		f.implement()
		f.recordGate(v, "a strict gate without a plan round", ccGate(f, ccRound(f, "candidate", f.base, f.head, false)))
		f.revise(ccScript("AC-1"), "changed after the candidate round")
		f.head = f.commit("change a check")
		f.noFreeze(v, "a strict goal whose gate has no plan round")
		f.recordGate(v, "a strict gate with only a dry-run plan round", ccGate(f, ccRound(f, "plan", f.base, f.base, true),
			ccRound(f, "candidate", f.base, f.head, false)))
		f.noFreeze(v, "a strict goal whose gate has only a dry-run plan round")
	}()
}

// ccSameFrozen reports whether two check_freeze file sets are equal.
func ccSameFrozen(got, want map[string]ccFrozen) bool {
	if len(got) != len(want) {
		return false
	}
	for path, w := range want {
		if g, ok := got[path]; !ok || g != w {
			return false
		}
	}
	return true
}
