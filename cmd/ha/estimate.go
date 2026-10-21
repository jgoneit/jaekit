package main

import (
	"encoding/json"
	"fmt"

	"github.com/jgoneit/jaekit/internal/bundle"
)

// requiredRunCounts describes a fresh, failure-free run, not work remaining in
// an existing run. Manual conditions do not execute a verification command.
func requiredRunCounts(w *workspace) (changes, maintains int) {
	for _, criterion := range w.goal.Criteria {
		if criterion.Optional {
			continue
		}
		row := w.bundle.Row(criterion.ID)
		if row == nil {
			continue // Callers lint before using an estimate.
		}
		switch row.Kind {
		case bundle.KindChange:
			changes++
		case bundle.KindMaintain:
			maintains++
		}
	}
	return changes, maintains
}

func minimumRequiredRuns(w *workspace) int {
	changes, maintains := requiredRunCounts(w)
	return 2*changes + maintains
}

type estimateSelection struct {
	Mode                 string   `json:"mode"`
	Targets              []string `json:"targets"`
	Runs                 int      `json:"runs"`
	AdditionalRuns       int      `json:"additional_runs"`
	OptionalRuns         int      `json:"optional_runs"`
	TaskRuns             int      `json:"task_runs"`
	RepeatedRequiredRuns int      `json:"repeated_required_runs"`
}

type estimateReport struct {
	Schema                string            `json:"schema"`
	Goal                  string            `json:"goal"`
	BudgetSource          string            `json:"budget_source"`
	RequiredChange        int               `json:"required_change"`
	RequiredMaintain      int               `json:"required_maintain"`
	MinimumRuns           int               `json:"minimum_runs"`
	PlanRunsLimit         int               `json:"plan_runs_limit"`
	Shortfall             int               `json:"shortfall"`
	Headroom              int               `json:"headroom"`
	Auto                  *estimateAuto     `json:"auto,omitempty"`
	DefaultCheckRuns      int               `json:"default_check_runs"`
	DefaultBaselineRuns   int               `json:"default_baseline_runs"`
	DefaultAdditionalRuns int               `json:"default_additional_runs"`
	Selected              estimateSelection `json:"selected"`
	Limitation            string            `json:"limitation"`
}

// estimateAuto is the start limit an automatic PLAN budget would get and
// whether the user must approve it first (run-record.md §1.1).
type estimateAuto struct {
	ComputedRuns    int    `json:"computed_runs"`
	PreapprovedRuns int    `json:"preapproved_runs"`
	Source          string `json:"source"`
	SourcePath      string `json:"source_path,omitempty"`
	ApprovalNeeded  bool   `json:"approval_needed"`
}

// estimateTargets shares execution's selection rules, including repeated
// targets. An empty default selection is useful for estimating a manual-only
// plan, while check itself keeps its existing no-target error.
func (c *cli) estimateTargets(w *workspace, names []string, baseline bool) ([]target, error) {
	if len(names) == 0 {
		hasTarget := false
		for _, row := range w.bundle.Rows {
			criterion := w.goal.Criterion(row.ID)
			if baseline && row.Kind == bundle.KindChange ||
				!baseline && criterion != nil && !criterion.Optional &&
					(row.Kind == bundle.KindChange || row.Kind == bundle.KindMaintain) {
				hasTarget = true
				break
			}
		}
		if !hasTarget {
			return nil, nil
		}
	}
	return c.targets(w, names, baseline)
}

func selectionEstimate(w *workspace, targets []target, baseline bool) estimateSelection {
	selection := estimateSelection{Mode: "check", Targets: []string{}, Runs: len(targets)}
	if baseline {
		selection.Mode = "baseline"
	}
	seenRequired := map[string]bool{}
	for _, target := range targets {
		selection.Targets = append(selection.Targets, target.label())
		criterion := w.goal.Criterion(target.criterion)
		switch {
		case target.task != "":
			selection.TaskRuns++
		case criterion == nil || criterion.Optional:
			selection.OptionalRuns++
		case seenRequired[target.criterion]:
			selection.RepeatedRequiredRuns++
		default:
			seenRequired[target.criterion] = true
		}
	}
	selection.AdditionalRuns = selection.OptionalRuns + selection.TaskRuns + selection.RepeatedRequiredRuns
	return selection
}

func (c *cli) estimate(in []string) int {
	a, err := parseArgs(in, []string{"format"}, []string{"baseline"})
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	format := a.vals["format"]
	if format == "" {
		format = "md"
	}
	if format != "md" && format != "json" {
		return c.fail(exitUsage, "--format must be md or json")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	if c.printLint(w) > 0 {
		return exitProblems
	}
	selected, err := c.estimateTargets(w, a.pos[1:], a.bools["baseline"])
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	checks, err := c.estimateTargets(w, nil, false)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	baselines, err := c.estimateTargets(w, nil, true)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	changes, maintains := requiredRunCounts(w)
	minimum := minimumRequiredRuns(w)
	limit := w.bundle.Budget.Runs
	var auto *autoBudget
	if w.bundle.Budget.Auto {
		computed, err := computeAutoBudget(w)
		if err != nil {
			return c.fail(exitRefused, "%v", err)
		}
		auto, limit = &computed, computed.Computed
	}
	report := estimateReport{
		Schema: "estimate/v1", Goal: w.goal.Dir, BudgetSource: "PLAN.md",
		RequiredChange: changes, RequiredMaintain: maintains,
		MinimumRuns: minimum, PlanRunsLimit: limit,
		Shortfall:        max(0, minimum-limit),
		Headroom:         max(0, limit-minimum),
		DefaultCheckRuns: len(checks), DefaultBaselineRuns: len(baselines),
		DefaultAdditionalRuns: len(checks) + len(baselines) - minimum,
		Selected:              selectionEstimate(w, selected, a.bools["baseline"]),
		Limitation: "Fresh required lower bound only; excludes retries, preparation and manual time. " +
			"Not a total-time estimate or completion guarantee. PLAN limit is not an existing run's " +
			"remaining budget; existing records and budget windows are unchanged.",
	}
	if auto != nil {
		report.Auto = &estimateAuto{ComputedRuns: auto.Computed, PreapprovedRuns: auto.Cap.Runs, Source: auto.Cap.Source,
			ApprovalNeeded: auto.approvalNeeded()}
		if auto.Cap.Source != "default" {
			report.Auto.SourcePath = auto.Cap.File
		}
	}
	if format == "json" {
		encoder := json.NewEncoder(c.out)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(report); err != nil {
			return c.fail(exitInternal, "%v", err)
		}
	} else {
		fmt.Fprintf(c.out, "## ha estimate: %s\n\n", report.Goal)
		fmt.Fprintf(c.out, "- required minimum: %d runs (%d change × 2 + %d maintain; manual: 0)\n",
			report.MinimumRuns, changes, maintains)
		fmt.Fprintf(c.out, "- PLAN run limit: %d; shortfall: %d; headroom above minimum: %d\n",
			report.PlanRunsLimit, report.Shortfall, report.Headroom)
		if auto != nil {
			approval := "no"
			if report.Auto.ApprovalNeeded {
				approval = "yes"
			}
			fmt.Fprintf(c.out, "- auto budget: %d runs (%s); pre-approved limit: %d from %s; user approval needed: %s\n",
				auto.Computed, auto.basis(), auto.Cap.Runs, auto.Cap.shown(), approval)
		}
		fmt.Fprintf(c.out, "- default commands: check %d + baseline %d runs; additional to minimum: %d\n",
			report.DefaultCheckRuns, report.DefaultBaselineRuns, report.DefaultAdditionalRuns)
		fmt.Fprintf(c.out, "- selected %s: %d runs; additional: %d (optional %d, task %d, repeated required %d)\n",
			report.Selected.Mode, report.Selected.Runs, report.Selected.AdditionalRuns,
			report.Selected.OptionalRuns, report.Selected.TaskRuns, report.Selected.RepeatedRequiredRuns)
		for _, label := range report.Selected.Targets {
			fmt.Fprintf(c.out, "  - %s\n", label)
		}
		fmt.Fprintf(c.out, "\n%s\n", report.Limitation)
	}
	if report.Shortfall > 0 || report.Auto != nil && report.Auto.ApprovalNeeded {
		return exitProblems
	}
	return exitOK
}
