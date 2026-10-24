package status

import (
	"errors"

	"github.com/jgoneit/jaekit/internal/record"
)

// The cost summary (`ha cost`, run-record.md §1.7) shows the budget and the
// reported waits from the record alone, with the same computations ha status
// uses, so its values are the ones ha status shows.

// RecordBudget computes the budget block of ha status from the record's
// lines only: it needs no Git, bundle or goal documents.
func RecordBudget(lines []record.Line) (*Budget, error) {
	e := &evaluator{in: Input{Lines: lines}, rules: Rules}
	for i := range lines {
		if lines[i].Kind == "start" {
			e.start = &lines[i]
			e.rules = lines[i].Rules
			break
		}
	}
	if e.start == nil {
		return nil, errors.New("goal has not started")
	}
	if !Supported(e.rules) {
		return nil, &UnsupportedRulesError{Rules: e.rules}
	}
	return e.budget()
}

// WaitSpan is one reported wait (run-rules/4 and later): the seq of its wait
// note, the decision it asked and the seconds it counts by itself, computed
// as the budget computes waits (run-record.md §4.7) inside its own window.
// Overlapping waits each keep their own seconds; the budget counts the
// overlap once.
type WaitSpan struct {
	Seq     int    `json:"seq"`
	Reason  string `json:"reason"`
	Seconds int64  `json:"seconds"`
}

// WaitSpans returns every wait note of the record in order, with its
// seconds. A wait note that ends in the second it began counts zero.
func WaitSpans(lines []record.Line) []WaitSpan {
	lines = record.ExecutionLines(lines)
	out := []WaitSpan{}
	from := -1
	flush := func(to int) {
		if from < 0 {
			return
		}
		window := lines[from:to]
		counted := map[int]int64{}
		if waits, runs, begin, end, ok := waitIntervals(window); ok {
			runs = union(runs)
			for _, w := range waits {
				counted[w.line.Seq] = clipped(subtract([]interval{w.interval}, runs), begin, end)
			}
		}
		for _, l := range window {
			if l.Kind == "note" && l.Note == "wait" {
				out = append(out, WaitSpan{Seq: l.Seq, Reason: l.Reason, Seconds: counted[l.Seq]})
			}
		}
	}
	for i, l := range lines {
		if l.Kind == "start" || (l.Kind == "note" && l.Note == "reopen") {
			flush(i)
			from = i
		}
	}
	flush(len(lines))
	return out
}

// ReportedWaitSeconds is the reported wait of every window of the record,
// each counted once as the budget counts the current window's.
func ReportedWaitSeconds(lines []record.Line) int64 {
	lines = record.ExecutionLines(lines)
	var total int64
	from := -1
	for i, l := range lines {
		if l.Kind == "start" || (l.Kind == "note" && l.Note == "reopen") {
			if from >= 0 {
				total += reportedWait(lines[from:i])
			}
			from = i
		}
	}
	if from >= 0 {
		total += reportedWait(lines[from:])
	}
	return total
}
