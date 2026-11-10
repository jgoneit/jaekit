package main

// Checks of the goal core-completion-integrity for the elapsed budget after a
// completion under run-rules/6 (AC-5, AC-6). The records are made through the
// command line and then given fixed times (ciTime, ciRelink); helpers are in
// core_integrity_test.go and review_gate_test.go. The forms of a check run one
// after another in the linked test, without subtests.

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"
)

// ciBudgetOf decodes the budget the status of the current record shows.
func ciBudgetOf(f *rgFixture) (rgState, map[string]any) {
	f.t.Helper()
	s := f.status()
	return s, ciBudget(f, s)
}

// ciExpectSeconds reports each budget field that differs from want, and an
// elapsed time that is not the span less the reported wait and the time
// after a completion.
func ciExpectSeconds(f *rgFixture, violation, label string, b map[string]any, want map[string]int64) {
	f.t.Helper()
	for key, w := range want {
		if got, ok := ciInt(b, key); !ok || got != w {
			f.t.Errorf("jaekit:violation=%s %s: budget %s is %v; want %d (budget %v)", violation, label, key, b[key], w, b)
		}
	}
	span, _ := ciInt(b, "span_seconds")
	wait, _ := ciInt(b, "reported_wait_seconds")
	post, _ := ciInt(b, "post_completion_seconds")
	if elapsed, _ := ciInt(b, "elapsed_seconds"); elapsed != span-wait-post {
		f.t.Errorf("jaekit:violation=%s %s: elapsed %d is not span %d less reported wait %d and post-completion %d", violation, label, elapsed, span, wait, post)
	}
}

// ciSync is the record of #124: a goal completes two hours after its start,
// takes the user's answer and a resume while it waits for its merge, syncs
// with main fourteen hours after the completion and checks again; with gates
// it records a new covering gate and completes again. The goal has started;
// the record gets fixed times. It returns the record up to the check after
// the sync and the whole record.
func ciSync(f *rgFixture, violation string, gates bool) (prefix, whole []ciLine) {
	f.t.Helper()
	roles := []string{"start"}
	f.put("src/work.txt", "one\n")
	f.work = f.commit("work")
	f.put("src/work.txt", "two\n")
	f.head = f.commit("fix")
	f.check()
	roles = append(roles, "check")
	if gates {
		f.recordGate(violation, "first gate", f.covering("candidate"))
		roles = append(roles, "gate")
	}
	ciMustDone(f, "first completion")
	ciMust(f, exitOK, "note", rgGoal, "input", "--quote", "머지해줘")
	ciMust(f, exitOK, "note", rgGoal, "resume")
	roles = append(roles, "done", "input", "resume")
	f.put("src/main.txt", "a change merged from main\n")
	f.head = f.commit("merge main")
	f.check()
	roles = append(roles, "check again")
	cut := len(roles)
	if gates {
		f.recordGate(violation, "gate after the sync", f.covering("candidate"))
		ciMustDone(f, "completion after the sync")
		roles = append(roles, "gate again", "done again")
	}
	lines := ciLines(f)
	if len(lines) != len(roles) {
		f.t.Fatalf("setup: the record has %d lines, want %d", len(lines), len(roles))
	}
	h, m := time.Hour, time.Minute
	for i, role := range roles {
		switch role {
		case "start":
			ciTime(lines[i], 0)
		case "check":
			ciTime(lines[i], 111*m, 110*m)
		case "gate":
			ciTime(lines[i], 115*m)
		case "done":
			ciTime(lines[i], 2*h)
		case "input":
			ciTime(lines[i], 10*h)
		case "resume":
			ciTime(lines[i], 12*h)
		case "check again":
			ciTime(lines[i], 16*h+m, 16*h)
		case "gate again":
			ciTime(lines[i], 16*h+3*m)
		case "done again":
			ciTime(lines[i], 16*h+4*m)
		}
	}
	whole = ciRelink(f, lines, 0)
	return whole[:cut], whole
}

// AC-5: the time from a completion record to the next run leaves the elapsed
// budget, notes inside it do not end it, so a goal that waited for its merge
// does not run out after syncing with main; status, report and cost use the
// same elapsed time.
func TestCoreIntegrityAC5_TimeAfterCompletionLeavesElapsed(t *testing.T) {
	const v = "post-completion-time-counted"
	h, m := time.Hour, time.Minute
	f := rgRepo(t, "standard", ciElapsed("10시간"))
	ciStart(f, v)
	prefix, whole := ciSync(f, v, true)

	// After re-recording the check, before the new gate: the goal is
	// incomplete for its stale gate, not out of budget. The input and resume
	// notes after the completion do not end the time after it.
	ciWrite(f, prefix)
	s, b := ciBudgetOf(f)
	if ciReason(s, "budget_exceeded") != nil || s.status == "budget_exhausted" || ciReason(s, "review_head_stale") == nil {
		t.Errorf("jaekit:violation=%s after the sync: status %s, reasons %v; want incomplete for the stale gate without budget_exceeded", v, s.status, s.reasons)
	}
	ciExpectSeconds(f, v, "after the sync", b, map[string]int64{"span_seconds": ciSeconds(16*h + m),
		"reported_wait_seconds": 0, "post_completion_seconds": ciSeconds(14 * h), "elapsed_seconds": ciSeconds(2*h + m)})
	if runs, _ := ciInt(b, "runs"); runs != 2 {
		t.Errorf("jaekit:violation=%s after the sync: budget runs %d; want the 2 checks", v, runs)
	}
	md := ciMarkdown(f, "status", rgGoal)
	post := strconv.FormatInt(ciSeconds(14*h), 10)
	shown := false
	if at := strings.Index(md, "### Budget"); at >= 0 {
		for _, line := range strings.Split(md[at:], "\n") {
			waits := strings.Contains(line, "waits the executor reported")
			if waits && strings.Contains(line, post) {
				t.Errorf("jaekit:violation=%s after the sync: ha status shows the time after the completion as reported waits: %q", v, line)
			}
			shown = shown || (!waits && strings.Contains(line, post))
		}
	}
	if !shown {
		t.Errorf("jaekit:violation=%s after the sync: the Budget section of ha status has no line of its own for the %s seconds after the completion record:\n%s", v, post, md)
	}
	for _, lang := range []string{"en", "ko"} {
		report := ciMarkdown(f, "report", rgGoal, "--lang", lang)
		for _, word := range []string{"budget exceeded", "예산 초과", "budget_exceeded", "must decide the budget", "예산 결정이 필요함"} {
			if strings.Contains(report, word) {
				t.Errorf("jaekit:violation=%s after the sync: ha report --lang %s still says %q:\n%s", v, lang, word, report)
			}
		}
	}
	var cost map[string]any
	if err := json.Unmarshal([]byte(ciMarkdown(f, "cost", rgGoal, "--format", "json")), &cost); err != nil {
		t.Fatalf("setup: ha cost JSON: %v", err)
	}
	verification, _ := cost["verification"].(map[string]any)
	costBudget, _ := verification["budget"].(map[string]any)
	if got, _ := ciInt(costBudget, "elapsed_seconds"); got != ciSeconds(2*h+m) {
		t.Errorf("jaekit:violation=%s after the sync: ha cost's budget window elapsed is %v; want %d as ha status", v, costBudget["elapsed_seconds"], ciSeconds(2*h+m))
	}

	// The whole record: complete, with the same accounting.
	ciWrite(f, whole)
	s, b = ciBudgetOf(f)
	if s.status != "complete" || ciCompletion(s) != len(whole) {
		t.Errorf("jaekit:violation=%s the whole record: status %s, completion record %v; want complete at seq %d", v, s.status, s.completion, len(whole))
	}
	ciExpectSeconds(f, v, "the whole record", b, map[string]int64{"post_completion_seconds": ciSeconds(14 * h),
		"elapsed_seconds": ciSeconds(2*h + 4*m)})

	// A window with a completion record still refuses a budget change, and a
	// reopen still starts a new window.
	before := f.records()
	if code, out := f.ha("budget", rgGoal, "--runs", "60", "--from", "50", "--window", "1", "--revision", "1",
		"--quote", "60회로 늘려줘", "--context", "the user asked for more runs"); code != exitRefused || string(before) != string(f.records()) {
		t.Errorf("jaekit:violation=%s a budget change in a completed window: exit %d, record changed %v; want 65 and no change: %s", v, code, string(before) != string(f.records()), out)
	}
	ciMust(f, exitOK, "note", rgGoal, "reopen", "--quote", "머지 전에 하나 더 고쳐줘")
	_, b = ciBudgetOf(f)
	if start, _ := ciInt(b, "window_start"); start != int64(len(whole)+1) {
		t.Errorf("jaekit:violation=%s after reopen: budget window starts at %d; want the reopen at seq %d", v, start, len(whole)+1)
	}
	if runs, _ := ciInt(b, "runs"); runs != 0 {
		t.Errorf("jaekit:violation=%s after reopen: budget runs %d; want 0 in the new window", v, runs)
	}
}

// AC-6: the time after the next run counts again, a reported wait that
// overlaps the time after a completion leaves the elapsed time once, a
// finding line neither ends that time nor changes the budget, and a done
// line that records no completion ends it.
func TestCoreIntegrityAC6_PostCompletionBoundaries(t *testing.T) {
	const v = "post-completion-boundary-wrong"
	h, m := time.Hour, time.Minute

	// A finding half an hour after the completion, a wait from T+1h that the
	// run at T+3h ends, and the answer at T+6h.
	func() {
		f := rgRepo(t, "light")
		ciStart(f, v)
		f.check()                   // seq 2
		ciMustDone(f, "completion") // seq 3
		lines := ciLines(f)
		ciTime(lines[0], 0)
		ciTime(lines[1], 51*m, 50*m)
		ciTime(lines[2], h) // T: the completion
		ciWrite(f, ciRelink(f, lines, 0))
		ciFinding(f, ciLines(f)[2])                                                                // seq 4
		ciMust(f, exitOK, "note", rgGoal, "wait", "--reason", "the user decides whether to merge") // seq 5
		f.put("src/main.txt", "a change merged from main\n")
		f.head = f.commit("merge main")
		f.check()                                                     // seq 6
		ciMust(f, exitOK, "note", rgGoal, "input", "--quote", "머지해줘") // seq 7
		lines = ciLines(f)
		if len(lines) != 7 || lines[3].kind() != "finding" {
			t.Fatalf("setup: the record has %d lines and line 4 is %q, want 7 lines with a finding at 4", len(lines), lines[3].kind())
		}
		ciTime(lines[3], h+30*m)     // T+30m: finding
		ciTime(lines[4], 2*h)        // T+1h: wait
		ciTime(lines[5], 4*h+m, 4*h) // T+3h: the next run
		ciTime(lines[6], 7*h)        // T+6h: the answer
		withFinding := ciRelink(f, lines, 3)
		ciWrite(f, withFinding)
		_, b := ciBudgetOf(f)
		// The wait runs from the second after its note to the run; the time
		// after the completion is the rest of T..T+3h. Elapsed is the hour
		// before T and T+3h..T+6h.
		ciExpectSeconds(f, v, "finding, wait and answer", b, map[string]int64{"span_seconds": ciSeconds(7 * h),
			"reported_wait_seconds": ciSeconds(2*h) - 1, "post_completion_seconds": ciSeconds(h) + 1, "elapsed_seconds": ciSeconds(4 * h)})

		without := append(append([]ciLine(nil), withFinding[:3]...), withFinding[4:]...)
		ciWrite(f, ciRelink(f, without, 3))
		_, bWithout := ciBudgetOf(f)
		for _, key := range []string{"elapsed_seconds", "span_seconds", "reported_wait_seconds", "post_completion_seconds", "runs", "runs_limit", "window_start"} {
			if !reflect.DeepEqual(b[key], bWithout[key]) {
				t.Errorf("jaekit:violation=%s the finding line changes budget %s: %v with it, %v without it", v, key, b[key], bWithout[key])
			}
		}
	}()

	// A run that started before the completion and was recorded after it
	// does not end the time after the completion; its own run time counts.
	func() {
		f := rgRepo(t, "light", ciElapsed("10시간"))
		ciStart(f, v)
		f.check()                                                     // seq 2
		ciMustDone(f, "completion")                                   // seq 3
		f.check()                                                     // seq 4: started before seq 3
		ciMust(f, exitOK, "note", rgGoal, "input", "--quote", "머지해줘") // seq 5
		f.check()                                                     // seq 6
		lines := ciLines(f)
		if len(lines) != 6 {
			t.Fatalf("setup: the record has %d lines, want 6", len(lines))
		}
		ciTime(lines[0], 0)
		ciTime(lines[1], 111*m, 110*m)
		ciTime(lines[2], 2*h)            // T: the completion
		ciTime(lines[3], 2*h+5*m, 118*m) // a run from T-2m to T+5m
		ciTime(lines[4], 10*h)           // the user's answer
		ciTime(lines[5], 18*h+m, 18*h)   // the first run that started after T
		ciWrite(f, ciRelink(f, lines, 0))
		_, b := ciBudgetOf(f)
		ciExpectSeconds(f, v, "a run that started before the completion", b, map[string]int64{"span_seconds": ciSeconds(18*h + m),
			"post_completion_seconds": ciSeconds(16*h - 5*m), "elapsed_seconds": ciSeconds(2*h + 6*m)})
	}()

	// A finding appended while ha done computes neither changes the budget
	// nor which line is the completion record: the record reads as the one
	// without the finding line.
	func() {
		f := rgRepo(t, "light", ciElapsed("10시간"))
		ciStart(f, v)
		f.check()                     // seq 2
		ciMustDone(f, "a completion") // seq 3: D1
		f.put("src/main.txt", "a change merged from main\n")
		f.head = f.commit("merge main")
		f.check() // seq 4
		lines := ciLines(f)
		ciTime(lines[0], 0)
		ciTime(lines[1], 41*m, 40*m)
		ciTime(lines[2], h)
		ciTime(lines[3], 111*m, 110*m)
		ciWrite(f, ciRelink(f, lines, 0))
		before := ciLines(f)
		ciMustDone(f, "the completion ha done computed") // D2, computed with seq 4 as its record head
		d2 := ciLines(f)[len(before)]
		ciWrite(f, before)
		ciFinding(f, before[2]) // seq 5: a finding about D1, appended while D2 was computed
		lines = ciLines(f)
		lines = append(lines, ciUnjudged(f, d2, lines[len(lines)-1])) // seq 6: D2 after the finding
		ciWrite(f, lines)
		ciMust(f, exitOK, "note", rgGoal, "input", "--quote", "머지해줘") // seq 7
		lines = ciLines(f)
		if len(lines) != 7 || lines[4].kind() != "finding" || lines[5].kind() != "done" {
			t.Fatalf("setup: the record has %d lines; want a finding at 5 and a done line at 6 of 7", len(lines))
		}
		ciTime(lines[4], 115*m)
		ciTime(lines[5], 2*h)
		ciTime(lines[6], 16*h)
		with := ciRelink(f, lines, 4)
		ciWrite(f, with)
		s := f.status()
		b := ciBudget(f, s)
		ciWrite(f, ciRelink(f, append(append([]ciLine(nil), with[:4]...), with[5:]...), 4))
		sWithout := f.status()
		bWithout := ciBudget(f, sWithout)
		if s.status != sWithout.status || (s.completion == nil) != (sWithout.completion == nil) {
			t.Errorf("jaekit:violation=%s a finding appended while ha done computed: status %s, completion record %v; without the finding %s, %v",
				v, s.status, s.completion, sWithout.status, sWithout.completion)
		}
		for _, key := range []string{"elapsed_seconds", "span_seconds", "reported_wait_seconds", "post_completion_seconds", "exceeded"} {
			if !reflect.DeepEqual(b[key], bWithout[key]) {
				t.Errorf("jaekit:violation=%s a finding appended while ha done computed changes budget %s: %v with it, %v without it", v, key, b[key], bWithout[key])
			}
		}
	}()

	// A done line that records no completion ends the time after it.
	func() {
		f := rgRepo(t, "light")
		ciStart(f, v)
		f.check()                   // seq 2
		ciMustDone(f, "completion") // seq 3
		f.put("src/main.txt", "a change merged from main\n")
		f.head = f.commit("merge main")
		if code, s := f.done(); code == exitOK || s.completion != nil { // seq 4: not a completion
			t.Fatalf("setup: ha done after a code change exit %d, completion %v; want no completion", code, s.completion)
		}
		f.check() // seq 5
		lines := ciLines(f)
		if len(lines) != 5 || lines[3].kind() != "done" || lines[3].m["status"] == "complete" {
			t.Fatalf("setup: the record has %d lines; want a done line that is not a completion at 4", len(lines))
		}
		ciTime(lines[0], 0)
		ciTime(lines[1], 51*m, 50*m)
		ciTime(lines[2], h)          // T: the completion
		ciTime(lines[3], 3*h)        // T+2h: done, not a completion
		ciTime(lines[4], 6*h+m, 6*h) // T+5h: the next run
		ciWrite(f, ciRelink(f, lines, 0))
		_, b := ciBudgetOf(f)
		ciExpectSeconds(f, v, "done without completion", b, map[string]int64{"span_seconds": ciSeconds(6*h + m),
			"post_completion_seconds": ciSeconds(2 * h), "elapsed_seconds": ciSeconds(4*h + m)})
	}()
}
