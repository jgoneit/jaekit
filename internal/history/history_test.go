package history

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const startHash = "0000000000000000000000000000000000000000000000000000000000000001"

func runHashes(n int, salt string) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = hashBytes([]byte(fmt.Sprintf("%s-%d", salt, i+1)))
	}
	out[0] = startHash
	return out
}

func facts(runs []string) Facts {
	return Facts{Start: Ref{Seq: 1, SHA256: startHash}, RunHashes: runs, Goal: "docs/specs/g", Commit: "c1", TreeClean: true,
		SpecDigest: "s1", Targets: map[string]bool{"AC-1": true, "AC-2": true, "T001": true},
		FileVersion:   func(string) (string, string) { return "", "missing" },
		ResolveCommit: func(string) (string, bool) { return "", false }}
}

func newStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir(), startHash)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func add(t *testing.T, s *Store, f Facts, request string, d Decision) (*Event, error) {
	t.Helper()
	return s.Append(time.Unix(1700000000, 0), "test", func(events []Event) (*Event, error) {
		return Prepare(d, request, events, f)
	})
}

func mustAdd(t *testing.T, s *Store, f Facts, request string, d Decision) *Event {
	t.Helper()
	e, err := add(t, s, f, request, d)
	if err != nil {
		t.Fatalf("add %s: %v", request, err)
	}
	return e
}

func TestDecisionForm(t *testing.T) {
	ok := []Decision{
		{Kind: "관측", Summary: "seen"},
		{Kind: "원인", Status: "가설", Summary: "maybe"},
		{Kind: "조치", Status: "반영", Summary: "done"},
		{Kind: "검증", Status: "실패", Summary: "failed", Reason: "line one\nline two"},
		{Kind: "재개", Summary: "resume", Evidence: []Evidence{{Type: "run", Seq: 1}, {Type: "file", Path: "a/b.md", Section: "현재"},
			{Type: "commit", Commit: "abcdef0"}, {Type: "external", Ref: "https://example.invalid/1", Version: "rev 2", Checked: true}}},
	}
	for _, d := range ok {
		if err := Check(d); err != nil {
			t.Errorf("%+v: %v", d, err)
		}
	}
	bad := map[string]Decision{
		"invalid_input/unknown kind":     {Kind: "plan", Summary: "x"},
		"invalid_input/status on 결정":     {Kind: "결정", Status: "확정", Summary: "x"},
		"invalid_input/wrong state":      {Kind: "원인", Status: "통과", Summary: "x"},
		"invalid_input/empty summary":    {Kind: "관측"},
		"invalid_input/two-line summary": {Kind: "관측", Summary: "a\nb"},
		"limit_exceeded/summary":         {Kind: "관측", Summary: strings.Repeat("x", MaxSummary+1)},
		"limit_exceeded/reason":          {Kind: "관측", Summary: "x", Reason: strings.Repeat("x", MaxReason+1)},
		"limit_exceeded/targets":         {Kind: "관측", Summary: "x", Targets: manyTargets(MaxList + 1)},
		"invalid_input/target":           {Kind: "관측", Summary: "x", Targets: []string{"ac-1"}},
		"invalid_input/repeated target":  {Kind: "관측", Summary: "x", Targets: []string{"AC-1", "AC-1"}},
		"invalid_input/builds_on":        {Kind: "관측", Summary: "x", BuildsOn: []string{"D0"}},
		"invalid_input/supersedes after": {Kind: "관측", Summary: "x", Supersedes: []Supersede{{ID: "D1", After: "x"}}},
		"invalid_input/event time":       {Kind: "관측", Summary: "x", EventAt: "yesterday"},
		"invalid_input/path escape":      {Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "file", Path: "../secret"}}},
		"invalid_input/absolute path":    {Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "file", Path: "/etc/passwd"}}},
		"invalid_input/run fields":       {Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "run", Seq: 1, Path: "x"}}},
		"invalid_input/commit":           {Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "commit", Commit: "-n"}}},
		"invalid_input/type":             {Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "url", Ref: "x"}}},
		"invalid_input/repeated ref":     {Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "run", Seq: 1}, {Type: "run", Seq: 1, Checked: true}}},
	}
	for name, d := range bad {
		want := strings.SplitN(name, "/", 2)[0]
		if got := Code(Check(d)); got != want {
			t.Errorf("%s: code %q, want %q", name, got, want)
		}
	}
}

func manyTargets(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("AC-%d", i+1)
	}
	return out
}

func TestStrictInput(t *testing.T) {
	good := `{"request_id":"r1","decision":{"kind":"관측","summary":"x","evidence":[{"type":"run","seq":1}]}}`
	d, id, err := DecodeInput([]byte(good))
	if err != nil || id != "r1" || d.Evidence[0].Seq != 1 {
		t.Fatalf("good input: %v %q %+v", err, id, d)
	}
	for _, raw := range []string{
		`{"decision":{"kind":"관측","summary":"x"},"decision":{}}`,
		`{"Decision":{"kind":"관측","summary":"x"}}`,
		`{"decision":{"KIND":"관측","summary":"x"}}`,
		`{"decision":{"kind":"관측","summary":"x","evidence":[{"TYPE":"run","seq":1}]}}`,
		`{"decision":{"kind":"관측","summary":"x"},"extra":1}`,
		`{"decision":{"kind":"관측","summary":"x"}} {}`,
		"{\"decision\":{\"kind\":\"\xff\",\"summary\":\"x\"}}",
		`[]`,
		`{"request_id":"r1"}`,
	} {
		if _, _, err := DecodeInput([]byte(raw)); Code(err) != "invalid_input" {
			t.Errorf("%s: %v", raw, err)
		}
	}
	if _, _, err := DecodeInput(bytes.Repeat([]byte(" "), MaxInput+1)); Code(err) != "limit_exceeded" {
		t.Errorf("oversized input: %v", err)
	}
}

func TestRegistrationReplayConflictAndRelations(t *testing.T) {
	s := newStore(t)
	f := facts(runHashes(3, "a"))
	d1 := mustAdd(t, s, f, "r1", Decision{Kind: "원인", Status: "가설", Summary: "hypothesis", Targets: []string{"AC-1"}, Evidence: []Evidence{{Type: "run", Seq: 2}}})
	if d1.ID != "D1" || d1.Context.Evidence[0].Version != f.RunHashes[1] || d1.Context.RunHead.Seq != 3 {
		t.Fatalf("stored event: %+v", d1)
	}
	// The same request and content replays; different content is refused.
	f2 := f
	f2.Commit = "c2"
	if _, err := add(t, s, f2, "r1", d1.Decision); !isReplay(err, "D1") {
		t.Fatalf("replay: %v", err)
	}
	if _, err := add(t, s, f, "r1", Decision{Kind: "관측", Summary: "other"}); Code(err) != "request_conflict" {
		t.Fatalf("request conflict: %v", err)
	}
	for name, d := range map[string]Decision{
		"invalid_relation":  {Kind: "결정", Summary: "x", BuildsOn: []string{"D2"}},
		"invalid_reference": {Kind: "결정", Summary: "x", Targets: []string{"AC-9"}},
	} {
		if _, err := add(t, s, f, "bad-"+name, d); Code(err) != name {
			t.Errorf("%s: %v", name, err)
		}
	}
	if _, err := add(t, s, f, "bad-run", Decision{Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "run", Seq: 9}}}); Code(err) != "invalid_reference" {
		t.Errorf("missing run: %v", err)
	}
	if _, err := add(t, s, f, "bad-hash", Decision{Kind: "관측", Summary: "x", Evidence: []Evidence{{Type: "run", Seq: 2, SHA256: startHash}}}); Code(err) != "invalid_reference" {
		t.Errorf("wrong run hash: %v", err)
	}
	mustAdd(t, s, f, "r2", Decision{Kind: "원인", Status: "기각", Summary: "rejected", Supersedes: []Supersede{{ID: "D1"}}})
	if _, err := add(t, s, f, "r3", Decision{Kind: "원인", Summary: "again", Supersedes: []Supersede{{ID: "D1"}}}); Code(err) != "correction_conflict" {
		t.Fatalf("stale correction: %v", err)
	}
	mustAdd(t, s, f, "r3", Decision{Kind: "원인", Summary: "again", Supersedes: []Supersede{{ID: "D1", After: "D2"}}})
	_, events, err := s.Load()
	if err != nil || len(events) != 3 || events[0].Decision.Summary != "hypothesis" {
		t.Fatalf("history after corrections: %v %d", err, len(events))
	}
}

func isReplay(err error, id string) bool {
	var r *Replay
	return errors.As(err, &r) && r.Event.ID == id
}

func TestDamageAndUnsupportedAreDistinct(t *testing.T) {
	s := newStore(t)
	f := facts(runHashes(2, "a"))
	mustAdd(t, s, f, "r1", Decision{Kind: "관측", Summary: "one"})
	mustAdd(t, s, f, "r2", Decision{Kind: "결정", Summary: "two", BuildsOn: []string{"D1"}})
	original, _ := os.ReadFile(s.file())
	cases := map[string][]byte{
		"history_corrupt/truncated":   original[:len(original)-5],
		"history_corrupt/edited":      bytes.Replace(original, []byte(`"one"`), []byte(`"uno"`), 1),
		"history_corrupt/deleted":     original[bytes.IndexByte(original, '\n')+1:],
		"history_corrupt/not json":    append(append([]byte(nil), original...), []byte("{oops\n")...),
		"history_unsupported/version": append(append([]byte(nil), original...), []byte(`{"schema":"decision-history/v2"}`+"\n")...),
	}
	for name, data := range cases {
		if err := os.WriteFile(s.file(), data, 0o644); err != nil {
			t.Fatal(err)
		}
		want := strings.SplitN(name, "/", 2)[0]
		if _, _, err := s.Load(); Code(err) != want {
			t.Errorf("%s: %v", name, err)
		}
		// A damaged or unsupported history refuses appends and is left as is.
		if _, err := add(t, s, f, "after", Decision{Kind: "관측", Summary: "x"}); Code(err) != want {
			t.Errorf("%s append: %v", name, err)
		}
		if now, _ := os.ReadFile(s.file()); !bytes.Equal(now, data) {
			t.Errorf("%s: history changed", name)
		}
	}
	empty := newStore(t)
	if state, events, err := empty.Load(); state != Absent || events != nil || err != nil {
		t.Errorf("absent history: %v %v %v", state, events, err)
	}
	if _, err := os.Stat(empty.Dir()); !errors.Is(err, os.ErrNotExist) {
		t.Error("loading an absent history created files")
	}
}

type failingFile struct {
	file      *os.File
	failWrite bool
}

func (f failingFile) Write(b []byte) (int, error) {
	if f.failWrite {
		return 0, errors.New("injected write failure")
	}
	return f.file.Write(b)
}
func (f failingFile) Sync() error  { return errors.New("injected sync failure") }
func (f failingFile) Close() error { return f.file.Close() }

func TestUncertainWriteCanBeConfirmedByRequest(t *testing.T) {
	s := newStore(t)
	f := facts(runHashes(2, "a"))
	d := Decision{Kind: "검증", Status: "통과", Summary: "checked"}
	defer func() { persist = persistLine }()
	// The bytes reach the file but sync fails: the write may have applied.
	persist = func(file appendFile, data []byte) error {
		return persistLine(failingFile{file: file.(*os.File)}, data)
	}
	if _, err := add(t, s, f, "uncertain", d); Code(err) != "write_uncertain" {
		t.Fatalf("sync failure: %v", err)
	}
	persist = persistLine
	if _, err := add(t, s, f, "uncertain", d); !isReplay(err, "D1") {
		t.Fatalf("retry after an applied uncertain write must replay: %v", err)
	}
	// Nothing reached the file: a retry with the same request applies once.
	persist = func(file appendFile, data []byte) error {
		return persistLine(failingFile{file: file.(*os.File), failWrite: true}, data)
	}
	if _, err := add(t, s, f, "second", d); Code(err) != "write_uncertain" {
		t.Fatalf("write failure: %v", err)
	}
	persist = persistLine
	e := mustAdd(t, s, f, "second", d)
	if e.ID != "D2" {
		t.Fatalf("retry stored %s", e.ID)
	}
	_, events, err := s.Load()
	if err != nil || len(events) != 2 {
		t.Fatalf("events after retries: %v %d", err, len(events))
	}
}

func TestConcurrentAppendsAndConflictingCorrections(t *testing.T) {
	dir := t.TempDir()
	f := facts(runHashes(2, "a"))
	open := func() *Store {
		s, err := Open(dir, startHash)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	var wg sync.WaitGroup
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := add(t, open(), f, fmt.Sprintf("c%d", i), Decision{Kind: "관측", Summary: fmt.Sprintf("independent %d", i)}); err != nil {
				t.Errorf("independent %d: %v", i, err)
			}
		}(i)
	}
	wg.Wait()
	_, events, err := open().Load()
	if err != nil || len(events) != 16 {
		t.Fatalf("independent appends: %v %d", err, len(events))
	}
	results := make(chan error, 2)
	for i := 0; i < 2; i++ {
		go func(i int) {
			_, err := add(t, open(), f, fmt.Sprintf("fix%d", i), Decision{Kind: "원인", Summary: "fix", Supersedes: []Supersede{{ID: "D1"}}})
			results <- err
		}(i)
	}
	var ok, conflict int
	for i := 0; i < 2; i++ {
		switch err := <-results; {
		case err == nil:
			ok++
		case Code(err) == "correction_conflict":
			conflict++
		default:
			t.Errorf("unexpected: %v", err)
		}
	}
	if ok != 1 || conflict != 1 {
		t.Fatalf("conflicting corrections: %d stored, %d conflicts", ok, conflict)
	}
}

func TestSymlinkedStorageIsRefused(t *testing.T) {
	common, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(common, "ha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(common, "ha", "history")); err != nil {
		t.Fatal(err)
	}
	s, _ := Open(common, startHash)
	if _, err := add(t, s, facts(runHashes(2, "a")), "r1", Decision{Kind: "관측", Summary: "x"}); Code(err) != "history_unreadable" {
		t.Fatalf("symlinked history directory: %v", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote through a symlink: %v", entries)
	}
}

// lineageFixture stores events recorded at different run heads: the shared
// prefix, the current chain, and a diverged chain.
func lineageFixture(t *testing.T) (*Store, []string, []string) {
	s := newStore(t)
	current := runHashes(6, "current")
	diverged := append(append([]string{}, current[:3]...), runHashes(5, "diverged")[3:]...)
	at := func(runs []string, n int) Facts { return facts(runs[:n]) }
	mustAdd(t, s, at(current, 3), "a", Decision{Kind: "원인", Status: "가설", Summary: "PDF parser assumes a first item", Targets: []string{"T001"}})                            // D1 shared prefix
	mustAdd(t, s, at(current, 5), "b", Decision{Kind: "관측", Summary: "empty input reproduces", Targets: []string{"AC-1"}})                                                   // D2 current
	mustAdd(t, s, at(current, 5), "c", Decision{Kind: "결정", Summary: "return zero for empty input", Targets: []string{"T001"}, BuildsOn: []string{"D1", "D2"}})              // D3
	mustAdd(t, s, at(diverged, 5), "d", Decision{Kind: "결정", Summary: "diverged branch plan", Targets: []string{"T001"}, Supersedes: []Supersede{{ID: "D3"}}})               // D4 other lineage
	mustAdd(t, s, at(current, 6), "e", Decision{Kind: "원인", Status: "기각", Summary: "OCR path is the cause", Targets: []string{"T001"}, Supersedes: []Supersede{{ID: "D1"}}}) // D5
	return s, current, diverged
}

func query(t *testing.T, s *Store, runs []string, q Query) *View {
	t.Helper()
	state, events, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	v, err := Run(Input{State: state, Events: events, Start: Ref{Seq: 1, SHA256: startHash}, Chain: Chain{Hashes: runs, Valid: true},
		Now: Now{Goal: "docs/specs/g", Commit: "c1", TreeClean: true, SpecDigest: "s1"}}, q)
	if err != nil {
		t.Fatalf("query %+v: %v", q, err)
	}
	return v
}

func ids(items []Item) string {
	var out []string
	for _, it := range items {
		if it.Type == "replacement" {
			out = append(out, it.ID+">"+it.By)
		} else {
			out = append(out, it.ID)
		}
	}
	return strings.Join(out, " ")
}

func TestCurrentJudgmentsFollowLineageBeforeFilters(t *testing.T) {
	s, current, diverged := lineageFixture(t)
	v := query(t, s, current, Query{Mode: "current"})
	if got := ids(v.Items); got != "D2 D3 D5" {
		t.Fatalf("current set: %s", got)
	}
	if v.OtherLineage.Count != 1 || v.OtherLineage.IDs[0].ID != "D4" || v.OtherLineage.IDs[0].Lineage != LineageDifferent {
		t.Fatalf("other lineage: %+v", v.OtherLineage)
	}
	d3 := v.Items[1]
	if len(d3.GroundsCorrected) != 1 || d3.GroundsCorrected[0].ID != "D1" || d3.GroundsCorrected[0].By[0] != "D5" || !d3.Current {
		t.Fatalf("grounds correction must show without rejecting D3: %+v", d3)
	}
	if len(d3.OtherCorrections) != 1 || d3.OtherCorrections[0] != "D4" {
		t.Fatalf("a correction from another lineage is shown, not applied: %+v", d3)
	}
	// D1 was replaced by D5, which does not match the keyword; it stays replaced.
	if got := ids(query(t, s, current, Query{Mode: "current", Keywords: []string{"pdf"}}).Items); got != "" {
		t.Fatalf("filtered current must not resurrect D1: %s", got)
	}
	// The diverged chain keeps the shared-prefix D1 and its own D4; D2 and D3
	// name a different seq 5, and D5 at seq 6 cannot be checked there.
	v = query(t, s, diverged, Query{Mode: "current"})
	if got := ids(v.Items); got != "D1 D4" {
		t.Fatalf("diverged current set: %s", got)
	}
	// Head advance alone keeps earlier judgments in the same lineage.
	longer := append(append([]string{}, current...), runHashes(9, "current")[6:]...)
	if got := ids(query(t, s, longer, Query{Mode: "current"}).Items); got != "D2 D3 D5" {
		t.Fatalf("head advance changed the lineage: %s", got)
	}
	// A broken run chain makes every lineage unverifiable instead of current.
	state, events, _ := s.Load()
	broken, err := Run(Input{State: state, Events: events, Start: Ref{Seq: 1, SHA256: startHash}, Chain: Chain{Hashes: current, Valid: false}}, Query{Mode: "current"})
	if err != nil || len(broken.Items) != 0 || broken.OtherLineage.Count != 5 || broken.RunChain != "broken" {
		t.Fatalf("broken chain: %v %+v", err, broken)
	}
}

func TestChangesAnnounceReplacementsOutsideFilters(t *testing.T) {
	s := newStore(t)
	runs := runHashes(3, "a")
	f := facts(runs)
	mustAdd(t, s, f, "a", Decision{Kind: "원인", Status: "가설", Summary: "PDF parser", Targets: []string{"T001", "AC-2"}})
	v := query(t, s, runs, Query{Mode: "current", Targets: []string{"AC-2"}})
	cursor := v.Cursor
	mustAdd(t, s, f, "b", Decision{Kind: "원인", Status: "확정", Summary: "OCR path", Targets: []string{"T001"}, Supersedes: []Supersede{{ID: "D1"}}})
	v = query(t, s, runs, Query{Mode: "current", Targets: []string{"AC-2"}, Since: cursor})
	if got := ids(v.Items); got != "D1>D2" {
		t.Fatalf("changes must carry the replacement notice: %s", got)
	}
	if got := ids(query(t, s, runs, Query{Mode: "current", Targets: []string{"AC-2"}}).Items); got != "" {
		t.Fatalf("plain current view is a normal empty result: %s", got)
	}
	if got := ids(query(t, s, runs, Query{Mode: "list", Since: cursor}).Items); got != "D2 D1>D2" {
		t.Fatalf("list changes: %s", got)
	}
}

func TestPagesKeepTheirBoundary(t *testing.T) {
	s := newStore(t)
	runs := runHashes(2, "a")
	f := facts(runs)
	for i := 0; i < 5; i++ {
		mustAdd(t, s, f, fmt.Sprintf("p%d", i), Decision{Kind: "관측", Summary: fmt.Sprintf("item %d", i)})
	}
	first := query(t, s, runs, Query{Mode: "current", Limit: 2})
	if !first.Truncated || first.Next == nil || ids(first.Items) != "D1 D2" {
		t.Fatalf("first page: %+v", first)
	}
	// A correction stored between pages does not shift the original query.
	mustAdd(t, s, f, "fix", Decision{Kind: "관측", Summary: "replaces D3", Supersedes: []Supersede{{ID: "D3"}}})
	second := query(t, s, runs, Query{Mode: "current", Limit: 2, Page: *first.Next})
	third := query(t, s, runs, Query{Mode: "current", Limit: 2, Page: *second.Next})
	if got := ids(second.Items) + " " + ids(third.Items); got != "D3 D4 D5" || third.Next != nil {
		t.Fatalf("later pages: %s", got)
	}
	state, events, _ := s.Load()
	in := Input{State: state, Events: events, Start: Ref{Seq: 1, SHA256: startHash}, Chain: Chain{Hashes: runs, Valid: true}}
	for code, q := range map[string]Query{
		"cursor_mismatch": {Mode: "current", Limit: 2, Page: *first.Next, Keywords: []string{"x"}},
		"cursor_invalid":  {Mode: "current", Page: strings.Replace(*first.Next, startHash[:16], strings.Repeat("f", 16), 1)},
	} {
		if _, err := Run(in, q); Code(err) != code {
			t.Errorf("%s: %v", code, err)
		}
	}
	in.Chain = Chain{Hashes: runHashes(2, "other"), Valid: true}
	if _, err := Run(in, Query{Mode: "current", Page: *first.Next}); Code(err) != "cursor_stale" {
		t.Errorf("stale lineage: %v", err)
	}
}
