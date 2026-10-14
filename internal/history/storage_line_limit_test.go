package history

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"strings"
	"testing"
)

// This reader never reaches EOF. The assertion also makes a regression fail
// promptly instead of allowing an unbounded allocation to exhaust the test host.
type endlessHistoryLine struct {
	t    *testing.T
	read int
}

func (r *endlessHistoryLine) Read(p []byte) (int, error) {
	if len(p) > MaxLine+1-r.read {
		r.t.Fatalf("oversized line read beyond the detection byte: %d + %d", r.read, len(p))
	}
	for i := range p {
		p[i] = 'x'
	}
	r.read += len(p)
	return len(p), nil
}

type historyReaderFunc func([]byte) (int, error)

func (f historyReaderFunc) Read(p []byte) (int, error) { return f(p) }

// Only load's read boundary is replaced; the real backends are covered below.
type historyReaderStorage struct {
	storage
	source io.Reader
}

func (s historyReaderStorage) read(name string) ([]byte, error) {
	if name == "last" {
		return nil, fs.ErrNotExist
	}
	return readEventLines(s.source)
}

func TestNativeStorageBoundsHistoryLines(t *testing.T) {
	for _, prefix := range []string{"", "short first line\n"} {
		t.Run(fmt.Sprintf("endless line after %d bytes", len(prefix)), func(t *testing.T) {
			endless := &endlessHistoryLine{t: t}
			source := io.MultiReader(strings.NewReader(prefix), endless)
			_, _, _, err := newStore(t).load(historyReaderStorage{source: source})
			if Code(err) != "history_corrupt" || endless.read != MaxLine+1 {
				t.Fatalf("oversized line: read %d bytes, error %v", endless.read, err)
			}
			line := strings.Count(prefix, "\n") + 1
			if !strings.Contains(err.Error(), fmt.Sprintf("line %d:", line)) {
				t.Fatalf("wrong damaged line: %v", err)
			}
		})
	}

	t.Run("exact bound resets at each newline", func(t *testing.T) {
		line := append(bytes.Repeat([]byte("x"), MaxLine), '\n')
		want := bytes.Repeat(line, 3)
		got, err := readEventLines(bytes.NewReader(want))
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("bounded lines changed or refused: %d bytes, %v", len(got), err)
		}
	})

	t.Run("read errors remain unreadable", func(t *testing.T) {
		denied := errors.New("synthetic read failure")
		source := historyReaderFunc(func(p []byte) (int, error) {
			return copy(p, "partial bytes"), denied
		})
		if _, err := readEventLines(source); !errors.Is(err, denied) {
			t.Fatalf("original read error lost: %v", err)
		}
		_, _, _, err := newStore(t).load(historyReaderStorage{source: source})
		if Code(err) != "history_unreadable" || !strings.Contains(err.Error(), denied.Error()) {
			t.Fatalf("read failure misclassified: %v", err)
		}
	})

	t.Run("backend refuses without repairing stored bytes", func(t *testing.T) {
		s := newStore(t)
		f := facts(runHashes(2, "bounded"))
		mustAdd(t, s, f, "first", Decision{Kind: "관측", Summary: "first"})
		before, err := os.ReadFile(s.file())
		if err != nil {
			t.Fatal(err)
		}
		before = append(before, bytes.Repeat([]byte("x"), MaxLine+8192)...)
		before = append(before, '\n')
		if err := os.WriteFile(s.file(), before, 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Load(); Code(err) != "history_corrupt" || !strings.Contains(err.Error(), "line 2:") {
			t.Fatalf("load did not report the oversized stored line: %v", err)
		}
		if _, err := add(t, s, f, "second", Decision{Kind: "관측", Summary: "second"}); Code(err) != "history_corrupt" {
			t.Fatalf("append did not preserve the stored corruption: %v", err)
		}
		after, err := os.ReadFile(s.file())
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("corrupt history was changed: %v", err)
		}
	})

	t.Run("valid history has no total size bound", func(t *testing.T) {
		s := newStore(t)
		f := facts(runHashes(2, "many-lines"))
		const count = 70
		for i := range count {
			mustAdd(t, s, f, fmt.Sprintf("request-%d", i), Decision{
				Kind: "관측", Summary: "normal event", Reason: strings.Repeat("x", MaxReason),
			})
		}
		info, err := os.Stat(s.file())
		if err != nil {
			t.Fatal(err)
		}
		if info.Size() <= MaxLine {
			t.Fatalf("fixture does not exceed one line's bound: %d bytes", info.Size())
		}
		state, events, err := s.Load()
		if err != nil || state != Present || len(events) != count {
			t.Fatalf("valid multiline history refused: %s, %d events, %v", state, len(events), err)
		}
	})

	t.Run("last file reading is unchanged", func(t *testing.T) {
		s := newStore(t)
		tx, err := s.openStorage(true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.close()
		want := bytes.Repeat([]byte("x"), MaxLine+1)
		if err := os.WriteFile(s.last(), want, 0o644); err != nil {
			t.Fatal(err)
		}
		got, err := tx.read("last")
		if err != nil || !bytes.Equal(got, want) {
			t.Fatalf("event-line limit applied to last: %d bytes, %v", len(got), err)
		}
	})
}
