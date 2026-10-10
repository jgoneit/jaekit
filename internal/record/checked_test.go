package record

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestBaselineBudgetAC12_AppendRefusalHasNoEntry(t *testing.T) {
	dir := t.TempDir()
	path, lockPath := filepath.Join(dir, "runs.jsonl"), filepath.Join(dir, "lock")
	if _, err := Append(path, lockPath, note("first"), t0, nil); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	want := errors.New("stale budget revision")
	prepared := false
	_, err := AppendChecked(path, lockPath, note("rejected"), t0, func(lines []Line) error {
		if len(lines) != 1 || lines[0].Quote != "first" {
			t.Fatalf("validation did not see current chain: %+v", lines)
		}
		return want
	}, func(int) error { prepared = true; return nil })
	after, _ := os.ReadFile(path)
	if !errors.Is(err, want) || prepared || !bytes.Equal(before, after) {
		t.Fatalf("refusal mutated record/prepared: %v, %v", err, prepared)
	}
	var uncertain *UncertainWriteError
	if errors.As(err, &uncertain) {
		t.Fatal("pre-write rejection must not be uncertain")
	}
}

type faultFile struct {
	bytes.Buffer
	phase  string
	closed bool
}

var injectedWriteError = errors.New("injected persistence fault")

func (f *faultFile) Write(data []byte) (int, error) {
	if f.phase == "partial" || f.phase == "short" {
		n, _ := f.Buffer.Write(data[:len(data)/2])
		if f.phase == "short" {
			return n, nil
		}
		return n, injectedWriteError
	}
	return f.Buffer.Write(data)
}
func (f *faultFile) Sync() error {
	if f.phase == "sync" {
		return injectedWriteError
	}
	return nil
}
func (f *faultFile) Close() error {
	f.closed = true
	if f.phase == "close" {
		return injectedWriteError
	}
	return nil
}

func TestBaselineBudgetAC12_PersistenceFaultsAreUncertain(t *testing.T) {
	for _, phase := range []string{"partial", "short", "sync", "close"} {
		t.Run(phase, func(t *testing.T) {
			f := &faultFile{phase: phase}
			err := persistLine(f, []byte("stored entry\n"))
			var uncertain *UncertainWriteError
			if !errors.As(err, &uncertain) || !f.closed || f.Len() == 0 {
				t.Fatalf("write may have applied but was not reported: %v", err)
			}
			want := injectedWriteError
			if phase == "short" {
				want = io.ErrShortWrite
			}
			if !errors.Is(err, want) {
				t.Fatalf("underlying error lost: %v", err)
			}
			if (phase == "sync" || phase == "close") && f.String() != "stored entry\n" {
				t.Fatal("fixture must establish full-write ambiguity")
			}
		})
	}
}

func TestAppendCheckedSerializesValidation(t *testing.T) {
	dir := t.TempDir()
	path, lockPath := filepath.Join(dir, "runs.jsonl"), filepath.Join(dir, "lock")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			entry := note("pending")
			_, err := AppendChecked(path, lockPath, entry, t0, func(lines []Line) error {
				entry.Quote = "validated"
				for _, l := range lines {
					if l.Quote != "validated" {
						return errors.New("unvalidated predecessor")
					}
				}
				return nil
			}, nil)
			if err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	lines, bad, err := Read(path)
	if err != nil || bad != nil || len(lines) != 8 {
		t.Fatalf("bad chain: %v %v %d", err, bad, len(lines))
	}
}
