package status

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
)

// budgetReasons reports malformed persisted changes as invalid local evidence,
// rather than guessing a limit or hiding the report behind an internal error.
func (e *evaluator) budgetReasons(rep *Report) ([]Reason, error) {
	bud, err := e.budget()
	if err != nil {
		var invalid *BudgetError
		if !errors.As(err, &invalid) {
			return nil, err
		}
		e.invalid = append(e.invalid, invalid.Error())
		return []Reason{{Code: "record_invalid", Class: ClassUser, Detail: invalid.Error()}}, nil
	}
	rep.Budget = bud
	if !bud.Exceeded {
		return nil, nil
	}
	return []Reason{{Code: "budget_exceeded", Class: ClassBudget, Detail: fmt.Sprintf("%d/%d runs, %ds/%ds in the window from seq %d", bud.Runs, bud.RunsLimit, bud.ElapsedSeconds, bud.ElapsedLimitSeconds, bud.WindowStart)}}, nil
}

// BudgetEvent exposes the complete history without replacing earlier limits.
type BudgetEvent struct {
	Seq    int                     `json:"seq"`
	At     string                  `json:"at"`
	Change record.BudgetChangeData `json:"change"`
}

// BudgetState is a deterministic fold of stored lines. Checks belong to the
// window in which they are appended, regardless of when their process began.
type BudgetState struct {
	Goal, Rules         string
	WindowStart         int
	Revision            int
	RunsLimit           int
	Runs                int
	ElapsedLimitSeconds int64
	ElapsedSeconds      int64
	Completed           bool
	History             []BudgetEvent
}

// BudgetChangeRequest states the budget revision the caller intended to edit.
// Ordinary intervening checks do not change that revision.
type BudgetChangeRequest struct {
	WindowStart, Revision, PreviousRuns, Runs int
	Quote, Context, Interpretation, Reason    string
}

// BudgetError is a refused change or an invalid persisted budget event.
type BudgetError struct{ Detail string }

func (e *BudgetError) Error() string { return e.Detail }

func budgetError(format string, args ...any) error {
	return &BudgetError{Detail: fmt.Sprintf(format, args...)}
}

func validateBudgetChange(state *BudgetState, d *record.BudgetChangeData) error {
	if state.Rules != RulesV3 {
		return budgetError("budget changes require %s; goal uses %s", RulesV3, state.Rules)
	}
	if state.Completed {
		return budgetError("window %d has a completion record; quote a rework request with reopen first", state.WindowStart)
	}
	if d == nil {
		return budgetError("budget_change has no budget_change data")
	}
	if d.Goal != state.Goal || d.WindowStart != state.WindowStart || d.Revision != state.Revision || d.PreviousRuns != state.RunsLimit {
		return budgetError("budget conflict: current goal %s, window %d, revision %d, runs limit %d", state.Goal, state.WindowStart, state.Revision, state.RunsLimit)
	}
	if d.Runs <= 0 {
		return budgetError("run limit must be a positive integer")
	}
	if d.Runs == d.PreviousRuns {
		return budgetError("run limit is unchanged (%d)", d.Runs)
	}
	if d.Runs > d.PreviousRuns && (strings.TrimSpace(d.Quote) == "" || strings.TrimSpace(d.Context) == "") {
		return budgetError("increasing the run limit requires the user's quote and its request context")
	}
	if d.Runs < d.PreviousRuns && strings.TrimSpace(d.Reason) == "" {
		return budgetError("reducing the run limit requires a reason")
	}
	return nil
}

// FoldBudget checks budget event semantics while retaining /1 and /2 window
// accounting. Callers separately validate the record hash chain.
func FoldBudget(lines []record.Line) (*BudgetState, error) {
	lines = record.ExecutionLines(lines)
	var state *BudgetState
	var originalRuns int
	var from string
	for _, l := range lines {
		switch l.Kind {
		case "start":
			if state != nil {
				return nil, budgetError("record seq %d: more than one start", l.Seq)
			}
			if !Supported(l.Rules) {
				return nil, &UnsupportedRulesError{Rules: l.Rules}
			}
			state = &BudgetState{Goal: l.Goal, Rules: l.Rules, WindowStart: l.Seq, Revision: l.Seq, History: []BudgetEvent{}}
			if l.Budget != nil {
				state.RunsLimit, state.ElapsedLimitSeconds = l.Budget.Runs, l.Budget.ElapsedSeconds
			}
			if l.Rules == RulesV3 && (state.RunsLimit <= 0 || state.ElapsedLimitSeconds <= 0) {
				return nil, budgetError("record seq %d: invalid start budget", l.Seq)
			}
			originalRuns, from = state.RunsLimit, l.At
		case "budget_change":
			if state == nil {
				return nil, budgetError("record seq %d: budget change before start", l.Seq)
			}
			if err := validateBudgetChange(state, l.BudgetChange); err != nil {
				return nil, budgetError("record seq %d: %v", l.Seq, err)
			}
			state.RunsLimit, state.Revision = l.BudgetChange.Runs, l.Seq
			state.History = append(state.History, BudgetEvent{Seq: l.Seq, At: l.At, Change: *l.BudgetChange})
		case "note":
			if state != nil && l.Note == "reopen" {
				state.WindowStart, state.Revision = l.Seq, l.Seq
				state.Runs, state.Completed, from = 0, false, l.At
				if state.Rules != RulesV3 {
					state.RunsLimit = originalRuns
				}
			}
		case "check", "baseline":
			if state != nil {
				state.Runs++
			}
		case "done":
			if state != nil && l.Status == Complete {
				state.Completed = true
			}
		}
		if l.Kind != "budget_change" && l.BudgetChange != nil {
			return nil, budgetError("record seq %d: budget data on a %s record", l.Seq, l.Kind)
		}
	}
	if state == nil {
		return nil, budgetError("goal has not started")
	}
	start, err := time.Parse(time.RFC3339, from)
	if err != nil {
		return nil, budgetError("bad budget window time %q", from)
	}
	end, err := time.Parse(time.RFC3339, lines[len(lines)-1].At)
	if err != nil {
		return nil, budgetError("bad last record time %q", lines[len(lines)-1].At)
	}
	state.ElapsedSeconds = int64(end.Sub(start) / time.Second)
	return state, nil
}

// NewBudgetChange validates intent against a freshly read chain. Use this
// inside AppendChecked's callback so another change/reopen cannot race it.
func NewBudgetChange(lines []record.Line, req BudgetChangeRequest) (*record.BudgetChange, error) {
	state, err := FoldBudget(lines)
	if err != nil {
		return nil, err
	}
	d := record.BudgetChangeData{Goal: state.Goal, WindowStart: req.WindowStart, Revision: req.Revision,
		PreviousRuns: req.PreviousRuns, Runs: req.Runs, Quote: req.Quote, Context: req.Context,
		Interpretation: req.Interpretation, Reason: req.Reason}
	if err := validateBudgetChange(state, &d); err != nil {
		return nil, err
	}
	return &record.BudgetChange{Header: record.Header{Kind: "budget_change"}, Change: d}, nil
}
