package status

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
)

const findingGoal = "docs/specs/synthetic"

type findingFixture struct {
	path, lock string
	lines      []record.Line
	change     record.FindingChange
	at         time.Time
}

func newFindingFixture(t *testing.T, rules string) *findingFixture {
	t.Helper()
	dir := t.TempDir()
	f := &findingFixture{path: filepath.Join(dir, "runs.jsonl"), lock: filepath.Join(dir, "lock"), at: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	entries := []record.Entry{
		&record.Start{Header: record.Header{Kind: "start"}, Goal: findingGoal, Rules: rules, Budget: record.Budget{Runs: 10, ElapsedSeconds: 14400}},
		&record.Check{Header: record.Header{Kind: "check"}, Target: record.Target{Criterion: "AC-1"}, CriterionKind: "maintain"},
		&record.Done{Header: record.Header{Kind: "done"}, Status: Complete, Commit: "synthetic-commit", SpecDigest: "synthetic-spec", Criteria: []record.CriterionResult{{ID: "AC-1", Satisfied: true}}},
	}
	for i, e := range entries {
		if _, err := record.Append(f.path, f.lock, e, f.at.Add(time.Duration(i)*time.Second), nil); err != nil {
			t.Fatal(err)
		}
	}
	f.read(t)
	d := f.lines[2]
	f.change = record.FindingChange{ID: "one", RequestID: "request-one", Completion: record.RecordRef{Seq: 3, SHA256: d.Hash()}, Criteria: []string{"AC-1"}, Mapping: "linked", Status: "candidate", Category: "existing_condition", Summary: "Synthetic", Source: "synthetic:review", Evidence: []string{}}
	return f
}
func (f *findingFixture) read(t *testing.T) {
	t.Helper()
	lines, bad, err := record.Read(f.path)
	if err != nil || bad != nil {
		t.Fatalf("read: %v %v", bad, err)
	}
	f.lines = lines
}
func (f *findingFixture) append(c record.FindingChange) (record.Line, error) {
	event := &record.Finding{}
	return record.AppendChecked(f.path, f.lock, event, f.at.Add(100000*time.Hour), func(lines []record.Line) error {
		e, err := NewFinding(lines, findingGoal, c)
		if err != nil {
			return err
		}
		*event = *e
		return nil
	}, nil)
}

func TestFindingsBudgetAllSavedRules(t *testing.T) {
	for _, rules := range []string{RulesV1, RulesV2, RulesV3} {
		t.Run(rules, func(t *testing.T) {
			f := newFindingFixture(t, rules)
			before, err := (&evaluator{rules: rules, in: Input{Lines: f.lines}, start: &f.lines[0]}).budget()
			if err != nil {
				t.Fatal(err)
			}
			if _, err = f.append(f.change); err != nil {
				t.Fatal(err)
			}
			f.read(t)
			after, err := (&evaluator{rules: rules, in: Input{Lines: f.lines}, start: &f.lines[0]}).budget()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("finding changed %s budget: %#v %#v %v", rules, before, after, err)
			}
			if _, err = record.Append(f.path, f.lock, &record.Note{Header: record.Header{Kind: "note"}, Note: "reopen", Quote: "synthetic rework"}, f.at.Add(200000*time.Hour), nil); err != nil {
				t.Fatal(err)
			}
			f.read(t)
			before, err = (&evaluator{rules: rules, in: Input{Lines: f.lines}, start: &f.lines[0]}).budget()
			if err != nil {
				t.Fatal(err)
			}
			c := f.change
			c.RequestID = "confirm"
			c.Revision = 4
			c.Status = "confirmed"
			c.Reason = "reproduced"
			c.Evidence = []string{"synthetic:reproduction"}
			if _, err = f.append(c); err != nil {
				t.Fatal(err)
			}
			f.read(t)
			after, err = (&evaluator{rules: rules, in: Input{Lines: f.lines}, start: &f.lines[0]}).budget()
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("finding changed reopened budget: %#v %#v %v", before, after, err)
			}
		})
	}
}

func TestFindingsConcurrentCASAndOtherEvents(t *testing.T) {
	f := newFindingFixture(t, RulesV3)
	if _, err := f.append(f.change); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c := f.change
			c.Revision = 4
			c.RequestID = fmt.Sprintf("judgment-%d", i)
			c.Status = "confirmed"
			c.Evidence = []string{"synthetic:reproduction"}
			c.Reason = "review"
			if _, err := f.append(c); err == nil {
				accepted.Add(1)
			}
		}(i)
	}
	// A real check append and explicit reopen share the record lock, but neither
	// resets a finding revision nor overwrites one of its competing judgments.
	wg.Add(2)
	go func() {
		defer wg.Done()
		_, err := record.Append(f.path, f.lock, &record.Check{Header: record.Header{Kind: "check"}}, f.at.Add(time.Hour), nil)
		if err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		_, err := record.Append(f.path, f.lock, &record.Note{Header: record.Header{Kind: "note"}, Note: "reopen", Quote: "synthetic explicit rework"}, f.at.Add(2*time.Hour), nil)
		if err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	f.read(t)
	report, err := FoldFindings(f.lines, findingGoal)
	if err != nil || accepted.Load() != 1 || len(report.Findings[0].History) != 2 || len(f.lines) != 7 {
		t.Fatalf("CAS/other events: accepted=%d records=%d report=%+v err=%v", accepted.Load(), len(f.lines), report, err)
	}
}

func TestFindingsReferencesAndReplay(t *testing.T) {
	f := newFindingFixture(t, RulesV3)
	if _, err := NewFinding(f.lines, "docs/specs/other", f.change); err == nil {
		t.Fatal("accepted another goal")
	}
	if _, err := f.append(f.change); err != nil {
		t.Fatal(err)
	}
	f.read(t)
	// An uncertain reply is safely resolved by the same request, with no append.
	_, err := NewFinding(f.lines, findingGoal, f.change)
	var replay *FindingReplay
	if !errors.As(err, &replay) || replay.Seq != 4 || replay.SHA256 != f.lines[3].Hash() {
		t.Fatalf("missing original receipt: %v", err)
	}
	c := f.change
	c.RequestID = "new-claim"
	c.Revision = 4
	c.Reason = "correction"
	c.Completion.SHA256 = "wrong"
	if _, err = NewFinding(f.lines, findingGoal, c); err == nil {
		t.Fatal("rebound completion")
	}
	a, err := FoldFindings(f.lines, findingGoal)
	if err != nil {
		t.Fatal(err)
	}
	ab, _ := a.JSON()
	b, _ := FoldFindings(f.lines, findingGoal)
	bb, _ := b.JSON()
	if !bytes.Equal(ab, bb) {
		t.Fatal("nondeterministic findings")
	}
}
