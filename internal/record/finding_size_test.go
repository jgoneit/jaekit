package record

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findingLineBound is the stored finding line bound of contracts/findings.md.
const findingLineBound = 2*1024*1024 + 64*1024

func TestParseFindingLineBound(t *testing.T) {
	for _, delta := range []int{-1, 0, 1} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			event := &Finding{Header: Header{Schema: FindingSchema, Seq: 1, Kind: "finding"}}
			raw, err := marshal(event)
			if err != nil {
				t.Fatal(err)
			}
			padding := findingLineBound + delta - len(raw)
			event.Finding.Summary = strings.Repeat("가", padding/3) + strings.Repeat("x", padding%3)
			raw, err = marshal(event)
			if err != nil || len(raw) != findingLineBound+delta {
				t.Fatalf("boundary fixture: bytes=%d, error=%v", len(raw), err)
			}
			lines, bad, err := Parse(append(raw, '\n'))
			if err != nil || len(lines) != 1 {
				t.Fatalf("parse: lines=%d, error=%v", len(lines), err)
			}
			if delta <= 0 && bad != nil {
				t.Fatalf("supported finding refused: %v", bad)
			}
			if delta > 0 && (bad == nil || bad.Line != 1 || !strings.Contains(bad.Detail, "finding line exceeds")) {
				t.Fatalf("oversized finding not identified: %v", bad)
			}
		})
	}
}

func TestParseLegacyLineKeepsExistingSizeSupport(t *testing.T) {
	event := note(strings.Repeat("x", findingLineBound+1))
	event.Schema, event.Seq = Schema, 1
	raw, err := marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	if _, bad, err := Parse(append(raw, '\n')); bad != nil || err != nil {
		t.Fatalf("finding limit applied to legacy record: %v %v", bad, err)
	}
}

// Support checks accept stored finding lines up to the documented bound. The
// Core refuses to write a line past it instead of producing one they reject.
func TestFindingLineBound(t *testing.T) {
	dir := t.TempDir()
	path, lock := filepath.Join(dir, "runs.jsonl"), filepath.Join(dir, "lock")
	if _, err := Append(path, lock, note("old"), t0, nil); err != nil {
		t.Fatal(err)
	}
	// U+2028 is written as a six-byte escape, twice the bytes of its input.
	within := &Finding{Header: Header{Kind: "finding"}, Finding: FindingChange{ID: "within", RequestID: "within", Summary: strings.Repeat("\u2028", 1024*1024/3)}}
	line, err := Append(path, lock, within, t0, nil)
	if err != nil || len(line.Raw) > findingLineBound || len(line.Raw) <= 2*1024*1024-64*1024 {
		t.Fatalf("a finding from a 1 MiB input was not stored within the bound: %d bytes, %v", len(line.Raw), err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	past := &Finding{Header: Header{Kind: "finding"}, Finding: FindingChange{ID: "past", RequestID: "past", Summary: strings.Repeat("\u2028", findingLineBound/6+1)}}
	if _, err := Append(path, lock, past, t0, nil); err == nil || !strings.Contains(err.Error(), "finding line exceeds") {
		t.Fatalf("a finding line past the bound was not refused: %v", err)
	}
	after, err := os.ReadFile(path)
	if err != nil || string(before) != string(after) {
		t.Fatalf("a refused finding changed the record: %v", err)
	}
}
