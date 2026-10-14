package history

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestFileEvidenceObservationBoundaries(t *testing.T) {
	s := newStore(t)
	runs := runHashes(2, "evidence-observation")
	f := facts(runs)
	version := "sha256:" + hashBytes([]byte("original content"))
	registrationCalls := 0
	f.FileVersion = func(path string) (string, string) {
		registrationCalls++
		switch path {
		case "src/identified":
			return version, "read"
		case "src/unidentified":
			return "", "missing"
		default:
			t.Fatalf("unexpected registration path %q", path)
			return "", "unreadable"
		}
	}
	identified := mustAdd(t, s, f, "identified", Decision{Kind: "관측", Summary: "identified file",
		Evidence: []Evidence{{Type: "file", Path: "src/identified"}}})
	unidentified := mustAdd(t, s, f, "unidentified", Decision{Kind: "관측", Summary: "unidentified file",
		Evidence: []Evidence{{Type: "file", Path: "src/unidentified"}}})
	if registrationCalls != 2 {
		t.Fatalf("registration observations: %d, want 2", registrationCalls)
	}
	before, err := os.ReadFile(s.file())
	if err != nil {
		t.Fatal(err)
	}

	for _, original := range []*Event{identified, unidentified} {
		t.Run("replay "+original.RequestID, func(t *testing.T) {
			calls := 0
			changed := f
			changed.FileVersion = func(string) (string, string) {
				calls++
				return "", "too_large"
			}
			_, err := add(t, s, changed, original.RequestID, original.Decision)
			var replay *Replay
			if !errors.As(err, &replay) || !reflect.DeepEqual(replay.Event, *original) {
				t.Fatalf("replay must return the original event: %v", err)
			}
			if calls != 0 {
				t.Fatalf("replay observed file evidence %d times, want 0", calls)
			}
		})
	}

	state, events, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		query Query
		calls int
		now   string
	}{
		{"current", Query{Mode: "current"}, 0, ""},
		{"list", Query{Mode: "list"}, 0, ""},
		{"show identified ID", Query{Mode: "show", ID: identified.ID}, 1, "unavailable"},
		{"show identified request", Query{Mode: "show", RequestID: identified.RequestID}, 1, "unavailable"},
		{"show unidentified ID", Query{Mode: "show", ID: unidentified.ID}, 0, "unidentified"},
		{"show unidentified request", Query{Mode: "show", RequestID: unidentified.RequestID}, 0, "unidentified"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			v, err := Run(Input{State: state, Events: events, Start: f.Start,
				Chain: Chain{Hashes: runs, Valid: true},
				Now:   Now{Goal: f.Goal, Commit: f.Commit, TreeClean: true, SpecDigest: f.SpecDigest},
				FileVersion: func(path string) (string, string) {
					calls++
					if path != "src/identified" {
						t.Errorf("query must not observe the originally unidentified file %q", path)
					}
					return "", "too_large"
				}}, tc.query)
			if err != nil {
				t.Fatal(err)
			}
			if calls != tc.calls {
				t.Fatalf("query observed file evidence %d times, want %d", calls, tc.calls)
			}
			if tc.now == "" {
				if got := ids(v.Items); got != identified.ID+" "+unidentified.ID {
					t.Fatalf("summary events: %s", got)
				}
				return
			}
			if v.Event == nil || len(v.Event.Evidence) != 1 {
				t.Fatalf("missing detailed evidence: %+v", v)
			}
			want := unidentified.Context.Evidence[0]
			if tc.calls == 1 {
				want = identified.Context.Evidence[0]
			}
			if got := v.Event.Evidence[0]; got.Now != tc.now || got.Recorded != want {
				t.Fatalf("current comparison or original observation changed: %+v, want %s/%+v", got, tc.now, want)
			}
		})
	}
	after, err := os.ReadFile(s.file())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("replay or query changed stored events: %v", err)
	}
}
