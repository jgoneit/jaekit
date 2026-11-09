package main

// Checks of the goal core-completion-integrity for the elapsed budget after a
// completion under run-rules/6 (AC-5, AC-6). The records are made through the
// command line and then given fixed times (ciTime, ciRelink); helpers are in
// core_integrity_test.go and review_gate_test.go.

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

// ciExpectSeconds reports each budget field that differs from want.
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

// AC-5: the time from a completion record to the next run leaves the elapsed
// budget, so a goal that waited for its merge does not run out after syncing
// with main; status, report and cost use the same elapsed time.
func TestCoreIntegrityAC5_TimeAfterCompletionLeavesElapsed(t *testing.T) {
	const v = "post-completion-time-counted"
	f := rgRepo(t, "standard", ciElapsed("10시간"))
	ciStart(f, v)
	f.put("src/work.txt", "one\n")
	f.work = f.commit("work")
	f.put("src/work.txt", "two\n")
	f.head = f.commit("fix")
	f.check()                                                       // seq 2
	f.recordGate(v, "first gate", f.covering("candidate"))          // seq 3
	ciMustDone(f, "first completion")                               // seq 4
	f.put("src/main.txt", "a change merged from main\n")            // the sync
	f.head = f.commit("merge main")                                 //
	f.check()                                                       // seq 5
	f.recordGate(v, "gate after the sync", f.covering("candidate")) // seq 6
	ciMustDone(f, "completion after the sync")                      // seq 7

	lines := ciLines(f)
	if len(lines) != 7 {
		t.Fatalf("setup: the record has %d lines, want 7", len(lines))
	}
	h := time.Hour
	ciTime(lines[0], 0)
	ciTime(lines[1], 111*time.Minute, 110*time.Minute)
	ciTime(lines[2], 115*time.Minute)
	ciTime(lines[3], 2*h)
	ciTime(lines[4], 16*h+time.Minute, 16*h)
	ciTime(lines[5], 16*h+3*time.Minute)
	ciTime(lines[6], 16*h+4*time.Minute)
	full := ciRelink(f, lines, 0)

	// After re-recording the check, before the new gate: the goal is
	// incomplete for its stale gate, not out of budget.
	ciWrite(f, full[:5])
	s, b := ciBudgetOf(f)
	if ciReason(s, "budget_exceeded") != nil || s.status == "budget_exhausted" || ciReason(s, "review_head_stale") == nil {
		t.Errorf("jaekit:violation=%s after the sync: status %s, reasons %v; want incomplete for the stale gate without budget_exceeded", v, s.status, s.reasons)
	}
	ciExpectSeconds(f, v, "after the sync", b, map[string]int64{"span_seconds": ciSeconds(16*h + time.Minute),
		"reported_wait_seconds": 0, "post_completion_seconds": ciSeconds(14 * h), "elapsed_seconds": ciSeconds(2*h + time.Minute)})
	if runs, _ := ciInt(b, "runs"); runs != 2 {
		t.Errorf("jaekit:violation=%s after the sync: budget runs %d; want the 2 checks", v, runs)
	}
	md := ciMarkdown(f, "status", rgGoal)
	budget := md[strings.Index(md, "### Budget"):]
	if !strings.Contains(budget, strconv.FormatInt(ciSeconds(14*h), 10)) {
		t.Errorf("jaekit:violation=%s after the sync: the Budget section of ha status does not show the %d seconds after the completion record:\n%s", v, ciSeconds(14*h), budget)
	}
	for _, lang := range []string{"en", "ko"} {
		report := ciMarkdown(f, "report", rgGoal, "--lang", lang)
		if strings.Contains(report, "budget exceeded") || strings.Contains(report, "예산 초과") || strings.Contains(report, "budget_exceeded") ||
			strings.Contains(report, "must decide the budget") || strings.Contains(report, "예산 결정이 필요함") {
			t.Errorf("jaekit:violation=%s after the sync: ha report --lang %s still reports the budget as exceeded:\n%s", v, lang, report)
		}
	}
	var cost map[string]any
	if err := json.Unmarshal([]byte(ciMarkdown(f, "cost", rgGoal, "--format", "json")), &cost); err != nil {
		t.Fatalf("setup: ha cost JSON: %v", err)
	}
	verification, _ := cost["verification"].(map[string]any)
	costBudget, _ := verification["budget"].(map[string]any)
	if got, _ := ciInt(costBudget, "elapsed_seconds"); got != ciSeconds(2*h+time.Minute) {
		t.Errorf("jaekit:violation=%s after the sync: ha cost's budget window elapsed is %v; want %d as ha status", v, costBudget["elapsed_seconds"], ciSeconds(2*h+time.Minute))
	}

	// The whole record: complete, with the same accounting.
	ciWrite(f, full)
	s, b = ciBudgetOf(f)
	if s.status != "complete" || ciCompletion(s) != 7 {
		t.Errorf("jaekit:violation=%s the whole record: status %s, completion record %v; want complete at seq 7", v, s.status, s.completion)
	}
	ciExpectSeconds(f, v, "the whole record", b, map[string]int64{"post_completion_seconds": ciSeconds(14 * h),
		"elapsed_seconds": ciSeconds(2*h + 4*time.Minute)})

	// A window with a completion record still refuses a budget change, and a
	// reopen still starts a new window.
	before := f.records()
	if code, out := f.ha("budget", rgGoal, "--runs", "60", "--from", "50", "--window", "1", "--revision", "1",
		"--quote", "60회로 늘려줘", "--context", "the user asked for more runs"); code != exitRefused || string(before) != string(f.records()) {
		t.Errorf("jaekit:violation=%s a budget change in a completed window: exit %d, record changed %v; want 65 and no change: %s", v, code, string(before) != string(f.records()), out)
	}
	ciMust(f, exitOK, "note", rgGoal, "reopen", "--quote", "머지 전에 하나 더 고쳐줘")
	_, b = ciBudgetOf(f)
	if start, _ := ciInt(b, "window_start"); start != 8 {
		t.Errorf("jaekit:violation=%s after reopen: budget window starts at %d; want the reopen at seq 8", v, start)
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
	h := time.Hour

	t.Run("wait_and_finding", func(t *testing.T) {
		f := rgRepo(t, "light")
		ciStart(f, v)
		f.check()                   // seq 2
		ciMustDone(f, "completion") // seq 3
		lines := ciLines(f)         //
		ciTime(lines[0], 0)         // start
		ciTime(lines[1], 51*time.Minute, 50*time.Minute)
		ciTime(lines[2], h)               // T: the completion
		ciWrite(f, ciRelink(f, lines, 0)) //
		done := ciLines(f)[2]
		ciMust(f, exitOK, "note", rgGoal, "wait", "--reason", "the user decides whether to merge") // seq 4
		ciFinding(f, done)                                                                         // seq 5
		f.put("src/main.txt", "a change merged from main\n")
		f.head = f.commit("merge main")
		f.check()                                                     // seq 6
		ciMust(f, exitOK, "note", rgGoal, "input", "--quote", "머지해줘") // seq 7
		lines = ciLines(f)
		if len(lines) != 7 || lines[4].kind() != "finding" {
			t.Fatalf("setup: the record has %d lines and line 5 is %q, want 7 lines with a finding at 5", len(lines), lines[4].kind())
		}
		ciTime(lines[3], 2*h)                  // T+1h: wait
		ciTime(lines[4], 3*h)                  // T+2h: finding
		ciTime(lines[5], 4*h+time.Minute, 4*h) // T+3h: the next run
		ciTime(lines[6], 7*h)                  // T+6h: the answer
		withFinding := ciRelink(f, lines, 3)
		ciWrite(f, withFinding)
		_, b := ciBudgetOf(f)
		// Elapsed: one hour before T and T+3h..T+6h after the run started.
		if got, _ := ciInt(b, "elapsed_seconds"); got != ciSeconds(4*h) {
			t.Errorf("jaekit:violation=%s wait and finding: elapsed %d; want %d (the hour before the completion and the time from the next run on)", v, got, ciSeconds(4*h))
		}
		wait, _ := ciInt(b, "reported_wait_seconds")
		post, _ := ciInt(b, "post_completion_seconds")
		if wait+post != ciSeconds(3*h) {
			t.Errorf("jaekit:violation=%s wait and finding: reported wait %d and post-completion %d leave %d seconds; want the overlap once, %d", v, wait, post, wait+post, ciSeconds(3*h))
		}
		ciExpectSeconds(f, v, "wait and finding", b, map[string]int64{"span_seconds": ciSeconds(7 * h)})

		without := append(append([]ciLine(nil), withFinding[:4]...), withFinding[5:]...)
		ciWrite(f, ciRelink(f, without, 4))
		_, bWithout := ciBudgetOf(f)
		for _, key := range []string{"elapsed_seconds", "span_seconds", "reported_wait_seconds", "post_completion_seconds", "runs", "runs_limit", "window_start"} {
			if !reflect.DeepEqual(b[key], bWithout[key]) {
				t.Errorf("jaekit:violation=%s the finding line changes budget %s: %v with it, %v without it", v, key, b[key], bWithout[key])
			}
		}
	})

	t.Run("done_without_completion", func(t *testing.T) {
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
		ciTime(lines[1], 51*time.Minute, 50*time.Minute)
		ciTime(lines[2], h)                    // T: the completion
		ciTime(lines[3], 3*h)                  // T+2h: done, not a completion
		ciTime(lines[4], 6*h+time.Minute, 6*h) // T+5h: the next run
		ciWrite(f, ciRelink(f, lines, 0))
		_, b := ciBudgetOf(f)
		ciExpectSeconds(f, v, "done without completion", b, map[string]int64{"span_seconds": ciSeconds(6*h + time.Minute),
			"post_completion_seconds": ciSeconds(2 * h), "elapsed_seconds": ciSeconds(4*h + time.Minute)})
	})
}
