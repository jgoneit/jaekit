package status

import (
	"fmt"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/evidence"
	"github.com/jgoneit/jaekit/internal/record"
)

// Usage separates attempt type, outcome, and input freshness. These axes
// overlap; adding their columns does not give the total number of attempts.
type Usage struct {
	Total             int `json:"total"`
	Checks            int `json:"checks"`
	Baselines         int `json:"baselines"`
	EnvironmentErrors int `json:"environment_errors"`
	Unknowns          int `json:"unknowns"`
	CurrentInputs     int `json:"current_inputs"`
	StaleInputs       int `json:"stale_inputs"`
	ValidEvidence     int `json:"valid_evidence"`
}

func validResult(l *record.Line, rules string) bool {
	if l.Kind == "check" {
		switch l.Result {
		case "pass":
			return l.Exit != nil && *l.Exit == 0
		case "fail":
			return validFailureExit(l, rules)
		case "error":
			return rules == RulesV3 || l.Exit == nil
		case "unknown":
			return rules == RulesV3 && l.CriterionKind == bundle.KindChange
		}
	}
	if l.Kind == "baseline" {
		switch l.Result {
		case "fail_as_expected":
			return l.Exit != nil && *l.Exit != 0
		case "unexpected_pass":
			return l.Exit != nil && *l.Exit == 0
		case "error":
			return rules == RulesV3 || l.Exit == nil
		case "unknown":
			return rules == RulesV3 && l.CriterionKind == bundle.KindChange
		case "fail":
			return rules == RulesV3 && l.CriterionKind == bundle.KindChange && validFailureExit(l, rules)
		}
	}
	return false
}

func validFailureExit(l *record.Line, rules string) bool {
	if l.Exit == nil {
		return false
	}
	if *l.Exit != 0 {
		return true
	}
	// A native runner can exit zero after retrying a failed assertion. Its
	// complete history remains a failure; fresh evidence is revalidated by judge.
	return rules == RulesV3 && l.CriterionKind == bundle.KindChange && l.Evidence != nil &&
		l.Evidence.Schema == evidence.SummarySchema && l.Evidence.Result == "fail" && l.Evidence.Reason == "inconsistent_attempts"
}

func finishBudget(b *Budget) {
	b.RemainingRuns = max(0, b.RunsLimit-b.Runs)
	b.ExcessRuns = max(0, b.Runs-b.RunsLimit)
	b.Reached = b.RunsLimit > 0 && b.Runs == b.RunsLimit
	b.Exceeded = (b.RunsLimit > 0 && b.Runs > b.RunsLimit) || (b.ElapsedLimitSeconds > 0 && b.ElapsedSeconds > b.ElapsedLimitSeconds)
}

func (e *evaluator) collectUsage(rep *Report) error {
	for i := range e.in.Lines {
		l := &e.in.Lines[i]
		if l.Kind == "budget_change" && e.rules != RulesV3 {
			e.invalid = append(e.invalid, fmt.Sprintf("seq %d: budget_change is unsupported under %s", l.Seq, e.rules))
		}
		if l.Kind != "check" && l.Kind != "baseline" {
			continue
		}
		u := &rep.Usage
		u.Total++
		if l.Kind == "check" {
			u.Checks++
		} else {
			u.Baselines++
		}
		if l.Result == "error" {
			u.EnvironmentErrors++
		}
		if l.Result == "unknown" {
			u.Unknowns++
		}
		valid := validResult(l, e.rules)
		if !valid {
			e.invalid = append(e.invalid, fmt.Sprintf("seq %d: invalid %s result or exit", l.Seq, l.Kind))
		}
		var fresh bool
		var err error
		if l.Kind == "baseline" {
			fresh, err = e.freshBaseline(l)
		} else if l.Target != nil && l.Target.Task != "" {
			fresh, err = e.freshTask(l)
		} else {
			fresh, err = e.freshCheck(l)
		}
		if err != nil {
			return err
		}
		if fresh {
			u.CurrentInputs++
		} else {
			u.StaleInputs++
		}
		if !fresh || !valid || (l.Result != "pass" && l.Result != "fail_as_expected") {
			continue
		}
		if e.rules == RulesV3 && l.CriterionKind == bundle.KindChange {
			if l.Target == nil {
				continue
			}
			r := e.in.Bundle.Row(l.Target.Criterion)
			if r == nil || evidence.ValidateStored(l.Evidence, r.Evidence, r.EvidenceDigest, l.Kind == "baseline", l.Result, l.Exit) != nil {
				continue
			}
		}
		u.ValidEvidence++
	}
	return nil
}

func (e *evaluator) freshTask(l *record.Line) (bool, error) {
	if !e.clean || !l.TreeClean || l.SpecDigest != e.in.Goal.Digest {
		return false, nil
	}
	t := e.in.Bundle.Task(l.Target.Task)
	if t == nil || l.Target.Index < 1 || l.Target.Index > len(t.Commands) || l.CommandDigest != bundle.Digest(t.Commands[l.Target.Index-1]) {
		return false, nil
	}
	return e.sameAsHead(l.Commit)
}
