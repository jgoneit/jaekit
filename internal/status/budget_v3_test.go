package status

import (
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
)

func storedBudgetFixture(rules string, used int) []record.Line {
	lines := []record.Line{{Header: record.Header{Kind: "start", Seq: 1, At: "2026-01-01T00:00:00Z"},
		Goal: "docs/specs/synthetic", Rules: rules, Budget: &record.Budget{Runs: 50, ElapsedSeconds: 14400, Cost: "unobserved"}}}
	for i := 0; i < used; i++ {
		lines = append(lines, record.Line{Header: record.Header{Kind: "check", Seq: i + 2, At: "2026-01-01T00:01:00Z"}})
	}
	return lines
}

func storeBudgetChange(t *testing.T, lines []record.Line, req BudgetChangeRequest) []record.Line {
	t.Helper()
	event, err := NewBudgetChange(lines, req)
	if err != nil {
		t.Fatal(err)
	}
	return append(lines, record.Line{Header: record.Header{Kind: "budget_change", Seq: len(lines) + 1, At: "2026-01-01T00:02:00Z"}, BudgetChange: &event.Change})
}

func TestBudgetV3TotalsAndReopen(t *testing.T) {
	lines := storedBudgetFixture(RulesV3, 48)
	lines = storeBudgetChange(t, lines, BudgetChangeRequest{WindowStart: 1, Revision: 1, PreviousRuns: 50, Runs: 100, Quote: "yes", Context: "Raise this window's total cap to 100"})
	state, err := FoldBudget(lines)
	if err != nil {
		t.Fatal(err)
	}
	if state.Runs != 48 || state.RunsLimit != 100 || state.WindowStart != 1 || state.ElapsedSeconds != 120 || state.ElapsedLimitSeconds != 14400 {
		t.Fatalf("change reset usage or time: %+v", state)
	}
	bud, err := (&evaluator{rules: RulesV3, in: Input{Lines: lines}}).budget()
	if err != nil || bud.RemainingRuns != 52 || bud.ExcessRuns != 0 || bud.Exceeded || bud.Reached {
		t.Fatalf("budget: %+v %v", bud, err)
	}
	lines = storeBudgetChange(t, lines, BudgetChangeRequest{WindowStart: 1, Revision: state.Revision, PreviousRuns: 100, Runs: 40, Reason: "Stop additional spending"})
	bud, err = (&evaluator{rules: RulesV3, in: Input{Lines: lines}}).budget()
	if err != nil || bud.Runs != 48 || bud.RemainingRuns != 0 || bud.ExcessRuns != 8 || !bud.Exceeded {
		t.Fatalf("reduction: %+v %v", bud, err)
	}
	seq := len(lines) + 1
	lines = append(lines, record.Line{Header: record.Header{Kind: "note", Seq: seq, At: "2026-01-01T01:00:00Z"}, Note: "reopen", Quote: "Rework this goal"})
	// A check started earlier but appended after reopen belongs to the new window.
	lines = append(lines, record.Line{Header: record.Header{Kind: "check", Seq: seq + 1, At: "2026-01-01T01:01:00Z"}})
	state, err = FoldBudget(lines)
	if err != nil || state.WindowStart != seq || state.Revision != seq || state.RunsLimit != 40 || state.Runs != 1 || state.ElapsedSeconds != 60 || len(state.History) != 2 {
		t.Fatalf("reopen lost cap/history or check: %+v %v", state, err)
	}
	bud = &Budget{Runs: 40, RunsLimit: 40}
	finishBudget(bud)
	if !bud.Reached || bud.Exceeded || bud.RemainingRuns != 0 || bud.ExcessRuns != 0 {
		t.Fatalf("equal cap: %+v", bud)
	}
}

func TestBudgetV3RejectsInvalidAndStaleChanges(t *testing.T) {
	base := BudgetChangeRequest{WindowStart: 1, Revision: 1, PreviousRuns: 50, Runs: 100, Quote: "$seal:seal", Context: "Apply the offered total of 100"}
	for _, tc := range []struct {
		name string
		edit func(*BudgetChangeRequest)
	}{
		{"window", func(r *BudgetChangeRequest) { r.WindowStart = 2 }},
		{"revision", func(r *BudgetChangeRequest) { r.Revision = 2 }},
		{"old limit", func(r *BudgetChangeRequest) { r.PreviousRuns = 40 }},
		{"zero", func(r *BudgetChangeRequest) { r.Runs = 0 }},
		{"negative", func(r *BudgetChangeRequest) { r.Runs = -1 }},
		{"same", func(r *BudgetChangeRequest) { r.Runs = 50 }},
		{"quote", func(r *BudgetChangeRequest) { r.Quote = " " }},
		{"context", func(r *BudgetChangeRequest) { r.Context = "" }},
		{"reduction reason", func(r *BudgetChangeRequest) { r.Runs = 40; r.Reason = "" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := base
			tc.edit(&req)
			if _, err := NewBudgetChange(storedBudgetFixture(RulesV3, 48), req); err == nil {
				t.Fatal("accepted invalid intent")
			}
		})
	}
	for _, rules := range []string{RulesV1, RulesV2} {
		if _, err := NewBudgetChange(storedBudgetFixture(rules, 48), base); err == nil {
			t.Fatalf("changed legacy %s", rules)
		}
	}
	lines := storedBudgetFixture(RulesV3, 48)
	lines = append(lines, record.Line{Header: record.Header{Kind: "done", Seq: 50, At: "2026-01-01T00:01:00Z"}, Status: Complete})
	if _, err := NewBudgetChange(lines, base); err == nil {
		t.Fatal("changed completed window without reopen")
	}
	lines = storedBudgetFixture(RulesV3, 48)
	lines = storeBudgetChange(t, lines, base)
	if _, err := NewBudgetChange(lines, base); err == nil {
		t.Fatal("reapplied stale intent")
	}
	lines[len(lines)-1].BudgetChange.PreviousRuns = 40
	if _, err := FoldBudget(lines); err == nil {
		t.Fatal("accepted inconsistent persisted previous cap")
	}
}

func TestBudgetV3ConcurrentChangesAndReplay(t *testing.T) {
	dir := t.TempDir()
	path, lock := filepath.Join(dir, "runs.jsonl"), filepath.Join(dir, "lock")
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	start := &record.Start{Header: record.Header{Kind: "start"}, Goal: "docs/specs/synthetic", Rules: RulesV3, Budget: record.Budget{Runs: 50, ElapsedSeconds: 14400}}
	if _, err := record.Append(path, lock, start, now, nil); err != nil {
		t.Fatal(err)
	}
	// Intervening inspection/check records do not invalidate the budget revision.
	if _, err := record.Append(path, lock, &record.Check{Header: record.Header{Kind: "check"}}, now, nil); err != nil {
		t.Fatal(err)
	}
	var accepted atomic.Int32
	var wg sync.WaitGroup
	for _, cap := range []int{80, 100} {
		wg.Add(1)
		go func(cap int) {
			defer wg.Done()
			entry := &record.BudgetChange{}
			_, err := record.AppendChecked(path, lock, entry, now, func(lines []record.Line) error {
				change, err := NewBudgetChange(lines, BudgetChangeRequest{WindowStart: 1, Revision: 1, PreviousRuns: 50, Runs: cap, Quote: "yes", Context: fmt.Sprintf("Raise total to %d", cap)})
				if err == nil {
					*entry = *change
				}
				return err
			}, nil)
			if err == nil {
				accepted.Add(1)
				return
			}
			var conflict *BudgetError
			if !errors.As(err, &conflict) {
				t.Error(err)
			}
		}(cap)
	}
	wg.Wait()
	lines, bad, err := record.Read(path)
	if err != nil || bad != nil || len(lines) != 3 || accepted.Load() != 1 {
		t.Fatalf("concurrency: %d %d %v %v", len(lines), accepted.Load(), bad, err)
	}
	state, err := FoldBudget(lines)
	if err != nil || state.Runs != 1 || state.Revision != 3 || len(state.History) != 1 || (state.RunsLimit != 80 && state.RunsLimit != 100) {
		t.Fatalf("replay: %+v %v", state, err)
	}
}

func TestBudgetLegacyWindowsRemainUnchanged(t *testing.T) {
	for _, rules := range []string{RulesV1, RulesV2} {
		lines := storedBudgetFixture(rules, 51)
		state, err := FoldBudget(lines)
		if err != nil || state.Runs != 51 || state.RunsLimit != 50 {
			t.Fatalf("legacy: %+v %v", state, err)
		}
		lines = append(lines, record.Line{Header: record.Header{Kind: "note", Seq: 53, At: "2026-01-01T00:03:00Z"}, Note: "reopen", Quote: "continue"})
		state, err = FoldBudget(lines)
		if err != nil || state.Runs != 0 || state.RunsLimit != 50 || state.ElapsedSeconds != 0 {
			t.Fatalf("legacy reopen: %+v %v", state, err)
		}
	}
}

func TestBudgetInvalidStoredChangeProducesReportReason(t *testing.T) {
	lines := storeBudgetChange(t, storedBudgetFixture(RulesV3, 48), BudgetChangeRequest{WindowStart: 1, Revision: 1, PreviousRuns: 50, Runs: 100, Quote: "yes", Context: "raise total to 100"})
	lines[len(lines)-1].BudgetChange.PreviousRuns = 49
	e := &evaluator{rules: RulesV3, in: Input{Lines: lines}}
	rep := &Report{}
	reasons, err := e.budgetReasons(rep)
	if err != nil || len(reasons) != 1 || reasons[0].Code != "record_invalid" || reasons[0].Class != ClassUser || len(e.invalid) != 1 || rep.Budget != nil {
		t.Fatalf("malformed event must fail closed in a usable report: %+v %+v %v", reasons, rep.Budget, err)
	}
}
