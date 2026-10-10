package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/status"
)

// budget changes a total limit in the named window, never its usage. The
// caller names the budget revision it actually considered; check records can
// arrive meanwhile without overwriting either usage or another budget choice.
func (c *cli) budget(in []string) int {
	a, err := parseArgs(in, []string{"runs", "from", "window", "revision", "quote", "context", "reason", "interpretation"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	if len(a.pos) != 1 {
		return c.fail(exitUsage, "usage: ha budget <goal> --runs <total> --from <old-total> --window <seq> --revision <seq> ...")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	if start := startLine(lines); start != nil && start.Rules != status.RulesV3 {
		return c.fail(exitRules, "budget changes require %s; this goal keeps %s", status.RulesV3, start.Rules)
	}
	if bad != nil {
		return c.fail(exitRefused, "%v", bad)
	}
	values := map[string]int{}
	for _, name := range []string{"runs", "from", "window", "revision"} {
		v, err := strconv.Atoi(a.vals[name])
		if err != nil || v <= 0 {
			return c.fail(exitUsage, "--%s must be a positive integer", name)
		}
		values[name] = v
	}
	req := status.BudgetChangeRequest{
		WindowStart: values["window"], Revision: values["revision"],
		PreviousRuns: values["from"], Runs: values["runs"],
		Quote: a.vals["quote"], Context: a.vals["context"],
		Reason: a.vals["reason"], Interpretation: a.vals["interpretation"],
	}
	entry := &record.BudgetChange{}
	var rejected error
	validate := func(current []record.Line) error {
		change, err := status.NewBudgetChange(current, req)
		if err != nil {
			rejected = err
			return err
		}
		*entry = *change
		entry.Header = record.Header{Kind: "budget_change", HaVersion: version}
		return nil
	}
	l, err := record.AppendChecked(w.records, w.lock, entry, time.Now(), validate, nil)
	if err != nil {
		if rejected != nil || errors.Is(err, record.ErrBroken) {
			return c.fail(exitRefused, "%v; read ha status before deciding a new change", err)
		}
		return c.fail(exitInternal, "%v", err)
	}
	fmt.Fprintf(c.out, "budget changed: %d -> %d total runs, window %d (seq %d); usage and time limit preserved\n", req.PreviousRuns, req.Runs, req.WindowStart, l.Seq)
	return exitOK
}
