package main

// Regressions for two run-rules/5 record cases the review rounds of the goal
// ha-review-gate found: a completion record computed before a review note
// that ended up in front of it, and a stored review summary that lacks a
// field. Both drive the ha command line on the synthetic repository of
// review_gate_test.go and edit runs.jsonl as a concurrent writer or a hand
// edit would leave it.

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// rgLines splits runs.jsonl into its lines.
func (f *rgFixture) rgLines() [][]byte {
	f.t.Helper()
	return bytes.Split(bytes.TrimSuffix(f.records(), []byte("\n")), []byte("\n"))
}

// rgRewrite writes lines back with seq and prev chained again, as an
// append-only writer would have left them.
func (f *rgFixture) rgRewrite(lines [][]byte) {
	f.t.Helper()
	var out bytes.Buffer
	var prev any
	for i, raw := range lines {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		var value map[string]any
		if err := dec.Decode(&value); err != nil {
			f.t.Fatalf("setup: record line %d: %v", i+1, err)
		}
		value["seq"], value["prev"] = i+1, prev
		data, err := json.Marshal(value)
		if err != nil {
			f.t.Fatalf("setup: %v", err)
		}
		out.Write(data)
		out.WriteByte('\n')
		prev = rgSHA(data)
	}
	if err := os.WriteFile(filepath.Join(f.dir, rgGoal, "runs.jsonl"), out.Bytes(), 0o644); err != nil {
		f.t.Fatalf("setup: %v", err)
	}
}

// A done line computed while another process recorded a blocked gate can land
// after that gate. Its completion must not stand, so the blocked gate is
// judged.
func TestHaReviewGateRecord_DoneComputedBeforeAGateDoesNotStand(t *testing.T) {
	f := newRGFixture(t, "unused", "standard")
	f.recordGate("unused", "a covering gate", f.chained())
	f.expectDone("unused", "a covering gate")
	blocked := f.chained()
	blocked.state, blocked.defect = "blocked", 1
	f.recordGate("unused", "a blocked gate", blocked)
	lines := f.rgLines()
	n := len(lines)
	// The done line was computed on the record before the blocked gate, but
	// the gate was appended first.
	lines[n-2], lines[n-1] = lines[n-1], lines[n-2]
	f.rgRewrite(lines)
	if s := f.status(); s.status == "complete" || s.completion != nil || !rgSame(s.review, []string{"review_blocked", "review_open_defect"}) {
		t.Errorf("a completion computed before the blocked gate stands: status %s, completion %v, reasons %v", s.status, s.completion, s.reasons)
	}
}

// A stored review summary with a field removed by hand is invalid record
// data, not a summary with zero open findings.
func TestHaReviewGateRecord_StoredSummaryMissingAFieldIsInvalid(t *testing.T) {
	f := newRGFixture(t, "unused", "standard")
	open := f.chained()
	open.defect = 1
	f.recordGate("unused", "a gate with an open defect", open)
	if s := f.status(); !rgSame(s.review, []string{"review_open_defect"}) {
		t.Fatalf("setup: the recorded gate gives %v", s.reasons)
	}
	original := f.rgLines()
	at := len(original) - 1
	for _, field := range []string{"open", "rounds", "last_reviewed_head"} {
		var value map[string]any
		dec := json.NewDecoder(bytes.NewReader(original[at]))
		dec.UseNumber()
		if err := dec.Decode(&value); err != nil {
			t.Fatalf("setup: %v", err)
		}
		review, _ := value["review"].(map[string]any)
		if value["note"] != "review" || review == nil {
			t.Fatalf("setup: line %d is not a review note", at+1)
		}
		delete(review, field)
		data, err := json.Marshal(value)
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
		lines := append(append([][]byte{}, original[:at]...), data)
		f.rgRewrite(lines)
		s := f.status()
		invalid := false
		for _, r := range s.reasons {
			invalid = invalid || r == "record_invalid"
		}
		if !invalid || s.status == "complete" {
			t.Errorf("a stored review summary without %s: status %s, reasons %v; want record_invalid", field, s.status, s.reasons)
		}
		if code, s := f.done(); code == 0 || s.completion != nil {
			t.Errorf("a stored review summary without %s: ha done exit %d, completion %v; want none", field, code, s.completion)
		}
	}
}
