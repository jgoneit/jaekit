package record

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 3, 1, 2, 3, 0, time.UTC)

func note(q string) *Note {
	return &Note{Header: Header{Kind: "note", HaVersion: "test"}, Note: "input", Quote: q}
}

func TestAppendChainsLines(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "runs.jsonl")
	lockp := filepath.Join(dir, "lock")
	a, err := Append(p, lockp, note("one"), t0, nil)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Append(p, lockp, note("two <&>"), t0, nil)
	if err != nil {
		t.Fatal(err)
	}
	if a.Seq != 1 || a.Prev != nil || b.Seq != 2 || b.Prev == nil || *b.Prev != a.Hash() {
		t.Fatalf("chain wrong: %+v %+v", a.Header, b.Header)
	}
	if !strings.HasPrefix(string(a.Raw), `{"schema":"run/v1","seq":1,"prev":null,"kind":"note","at":"2026-10-03T01:02:03Z","ha_version":"test","note":"input"`) {
		t.Fatalf("field order or format changed: %s", a.Raw)
	}
	if !bytes.Contains(b.Raw, []byte("<&>")) {
		t.Fatalf("HTML characters must not be escaped: %s", b.Raw)
	}
	lines, bad, err := Read(p)
	if err != nil || bad != nil || len(lines) != 2 {
		t.Fatalf("read = %d lines, %v, %v", len(lines), bad, err)
	}
}

func TestDeletedLineBreaksChainAndBlocksAppend(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "runs.jsonl")
	lockp := filepath.Join(dir, "lock")
	for _, q := range []string{"a", "b", "c"} {
		if _, err := Append(p, lockp, note(q), t0, nil); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(p)
	parts := strings.SplitAfter(string(data), "\n")
	if err := os.WriteFile(p, []byte(parts[0]+parts[2]), 0o644); err != nil {
		t.Fatal(err)
	}
	_, bad, err := Read(p)
	if err != nil || bad == nil || bad.Line != 2 {
		t.Fatalf("want integrity error at line 2, got %v %v", bad, err)
	}
	if _, err := Append(p, lockp, note("d"), t0, nil); !errors.Is(err, ErrBroken) {
		t.Fatalf("append on broken chain: %v", err)
	}
}

func TestConcurrentAppendsKeepChain(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "runs.jsonl")
	lockp := filepath.Join(dir, "lock")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := Append(p, lockp, note("x"), t0, nil); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	lines, bad, err := Read(p)
	if err != nil || bad != nil || len(lines) != 20 {
		t.Fatalf("got %d lines, %v, %v", len(lines), bad, err)
	}
}
