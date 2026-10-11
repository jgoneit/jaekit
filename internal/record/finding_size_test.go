package record

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// findingLineBound is the stored finding line bound of contracts/findings.md.
const findingLineBound = 2*1024*1024 + 64*1024

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
