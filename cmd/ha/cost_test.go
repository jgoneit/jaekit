//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Regressions of ha cost found in review (01-candidate). They use the
// synthetic goal and series of goal_cost_summary_test.go.

// A goal started before run-rules/4 cannot report waits or record
// test-change and recovery notes: those values are not recorded, not zero.
func TestCostWaitsBeforeRunRules4AreNotRecorded(t *testing.T) {
	f := gcRepo(t, "standard")
	f.must("", exitOK, "start", gcGoal, "--request", gcRequest)
	doc := gcCostJSON(t, "x", gcGoal)
	for _, key := range []string{"questions.waits", "questions.reported_wait_seconds", "questions.by_source.wait",
		"verification.notes.test_change", "verification.notes.recover", "verification.notes.allow_recovery"} {
		if got := gcAt(doc, "unavailable", key); got != "not_recorded" {
			t.Errorf("%s is %v in unavailable, want not_recorded", key, got)
		}
	}
	if gcAt(doc, "questions", "waits") != nil || gcAt(doc, "questions", "by_source", "wait") != nil ||
		gcAt(doc, "verification", "notes", "test_change") != nil || gcAt(doc, "verification", "notes", "reopen") != 0.0 {
		t.Errorf("a run-rules/3 goal gives questions %v, notes %v", doc["questions"], gcAt(doc, "verification", "notes"))
	}
	text := gcCost(t, "x", gcGoal)
	for _, want := range []string{"waits not recorded", "- waits: not recorded before run-rules/4", "- test changes not recorded;"} {
		if !strings.Contains(text, want) {
			t.Errorf("the summary does not say %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "waits 0") || strings.Contains(text, "- waits: none") {
		t.Errorf("the summary shows waits as zero:\n%s", text)
	}
}

// A goal without a start record and with a series shows its wait source as
// not started in both forms, never as 0.
func TestCostUnstartedGoalWithSeriesShowsNoZeroWaits(t *testing.T) {
	f := gcRepo(t, "standard")
	f.base = f.head
	f.series(gcSeries)
	doc := gcCostJSON(t, "x", gcGoal, "--series", gcSeries)
	if gcAt(doc, "questions", "by_source", "wait") != nil || gcAt(doc, "unavailable", "questions.by_source.wait") != "not_started" {
		t.Fatalf("by_source.wait is %v", gcAt(doc, "questions", "by_source", "wait"))
	}
	for _, lang := range []string{"en", "ko"} {
		text := gcCost(t, "x", gcGoal, "--series", gcSeries, "--lang", lang)
		line := ""
		for _, l := range strings.Split(text, "\n") {
			if strings.Contains(l, "requirement-question") && strings.HasPrefix(l, "- ") {
				line = l
				break
			}
		}
		if strings.Contains(line, " 0") || (!strings.Contains(line, "not started") && !strings.Contains(line, "시작 전")) {
			t.Errorf("%s: the by-source line is %q", lang, line)
		}
	}
}

// The lines fixes rounds added come from a gate only when that gate lists
// every fixes round of the series; a fixes round run after the gate leaves
// them not recorded.
func TestCostGrowthNeedsAGateListingEveryFixesRound(t *testing.T) {
	f := gcRepo(t, "standard")
	f.base = f.head
	f.must("", exitOK, "start", gcGoal, "--request", gcRequest, "--rules", "run-rules/5")
	f.series(gcSeries)
	doc := gcCostJSON(t, "x", gcGoal, "--series", gcSeries)
	if gcAt(doc, "findings", "growth", 0, "source") != "gate" {
		t.Fatalf("setup: the covering gate is not used: %v", gcAt(doc, "findings", "growth"))
	}
	for _, name := range []string{"round.json", "findings.json", "ledger.jsonl"} {
		f.put(gcSeries+"/03-fixes/"+name, string(f.read(gcSeries+"/02-fixes/"+name)))
	}
	doc = gcCostJSON(t, "x", gcGoal, "--series", gcSeries)
	if gcAt(doc, "findings", "growth", 0, "lines") != nil || gcAt(doc, "findings", "growth", 0, "source") != "gate_misses_rounds" ||
		gcAt(doc, "findings", "growth_total") != nil {
		t.Errorf("a gate that misses 03-fixes gives growth %v, total %v", gcAt(doc, "findings", "growth"), gcAt(doc, "findings", "growth_total"))
	}
	if text := gcCost(t, "x", gcGoal, "--series", gcSeries); !strings.Contains(text, "not recorded: gate.json does not list every fixes round") {
		t.Errorf("the summary does not say the gate misses a fixes round:\n%s", text)
	}
}

// A round time that is not an RFC 3339 time is a broken round.
func TestCostMalformedRoundTimeIsRefused(t *testing.T) {
	f := gcRepo(t, "standard")
	f.base = f.head
	f.must("", exitOK, "start", gcGoal, "--request", gcRequest, "--rules", "run-rules/5")
	f.series(gcSeries)
	path := filepath.Join(f.dir, gcSeries, "01-candidate", "round.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.Replace(string(data), "2026-01-01T00:00:00Z", "not-a-time", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	code, out, errs := gcHa("cost", gcGoal, "--series", gcSeries)
	if code != exitRefused || out != "" || !strings.Contains(errs, "01-candidate: round.json cannot be read") {
		t.Errorf("exit %d, stdout %q, stderr %q; want 65 naming the round", code, out, errs)
	}
}
