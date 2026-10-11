package record

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindingStrictInput(t *testing.T) {
	for _, input := range []string{`{"id":"a","id":"b"}`, `{"id":"a","ID":"b"}`, `{"ID":"a"}`, `{"unknown":1}`, `{"completion":{"seq":1,"seq":2}}`, `{"completion":{"seq":1,"SEQ":2}}`, `{"document":{"REF":"x"}}`, "{\"summary\":\"\xff\"}", `{} {}`, `null`, strings.Repeat(" ", 1024*1024+1)} {
		change, err := DecodeFinding([]byte(input))
		if err == nil {
			t.Errorf("accepted %q: %+v", input, change)
		}
	}
}

func TestFindingSchemaAndPersistenceUncertainty(t *testing.T) {
	dir := t.TempDir()
	path, lock := filepath.Join(dir, "runs.jsonl"), filepath.Join(dir, "lock")
	original, err := Append(path, lock, note("old"), t0, nil)
	if err != nil {
		t.Fatal(err)
	}
	event := &Finding{Header: Header{Kind: "finding"}, Finding: FindingChange{ID: "synthetic", RequestID: "request"}}
	line, err := Append(path, lock, event, t0, nil)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, append(append([]byte(nil), original.Raw...), '\n')) || line.Schema != FindingSchema {
		t.Fatal("rewrote history or used silently ignored legacy schema")
	}
	for _, phase := range []string{"partial", "short", "sync", "close"} {
		t.Run(phase, func(t *testing.T) {
			f := &faultFile{phase: phase}
			err := persistLine(f, append(append([]byte(nil), line.Raw...), '\n'))
			var uncertain *UncertainWriteError
			if !errors.As(err, &uncertain) {
				t.Fatalf("finding persistence failure was hidden: %v", err)
			}
			candidate := append(append(append([]byte(nil), original.Raw...), '\n'), f.Bytes()...)
			lines, bad, readErr := Parse(candidate)
			if phase == "sync" || phase == "close" {
				if readErr != nil || bad != nil || len(lines) != 2 || lines[1].Finding.RequestID != "request" {
					t.Fatalf("lost applied request identity: %v %v", bad, readErr)
				}
			} else if bad == nil {
				t.Fatal("partial finding appeared intact")
			}
		})
	}
	legacy := bytes.Replace(data, []byte(FindingSchema), []byte(Schema), 1)
	if _, bad, _ := Parse(legacy); bad == nil {
		t.Fatal("finding accepted under legacy schema")
	}
	aliases := bytes.Replace(data, []byte(`"finding":`), []byte(`"FINDING":`), 1)
	if _, bad, _ := Parse(aliases); bad == nil {
		t.Fatal("case alias accepted in stored extension envelope")
	}
}
