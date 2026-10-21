package main

import (
	"encoding/json"

	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/status"
)

// capabilityReport is what this executable supports. `ha capabilities`
// prints it and `ha start --skill` checks a skill's Core requirements against
// it, so the two never disagree (core-capabilities.md §1, §8).
type capabilityReport struct {
	Schema         string              `json:"schema"`
	HaVersion      string              `json:"ha_version"`
	SupportedRules []string            `json:"supported_rules"`
	DefaultRules   string              `json:"default_rules"`
	Formats        map[string][]string `json:"formats"`
	Features       []string            `json:"features"`
}

func supportedCapabilities() capabilityReport {
	return capabilityReport{
		Schema:         "ha-capabilities/v1",
		HaVersion:      version,
		SupportedRules: []string{status.RulesV1, status.RulesV2, status.RulesV3, status.RulesV4},
		DefaultRules:   status.Rules,
		Formats: map[string][]string{
			"finding":           {"run-finding/v1"},
			"decision_history":  {"decision-history/v1"},
			"criteria":          {"legacy", "nested/1"},
			"check_declaration": {"check-declaration/v1", "check-adapter/v1"},
			"check_result":      {"check-result/v1"},
			"check_evidence":    {"check-evidence/v1"},
			"timing":            {"timing/v1"},
			"assurance_tier":    append([]string(nil), goaldocs.AssuranceTiers...),
			"core_requirements": {coreRequirementsSchema},
		},
		Features: []string{"estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1", "budget-change/v1", "post-completion-findings/v1", "decision-history/v1", "run-timing/v1",
			"test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1", "command-time-limits/v1", "runner-adapters/v1", "report/v1", "doctor/v1",
			"auto-budget/v1", "core-requirements/v1"},
	}
}

// capabilities describes this executable, not the contents or state of a goal.
// It deliberately does not open a repository, read configuration, or run tools.
func (c *cli) capabilities(in []string) int {
	a, err := parseArgs(in, []string{"format"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	format, hasFormat := a.vals["format"]
	if len(a.pos) != 0 || (hasFormat && format != "json") {
		return c.fail(exitUsage, "usage: ha capabilities [--format json]")
	}
	if err := json.NewEncoder(c.out).Encode(supportedCapabilities()); err != nil {
		return c.fail(exitInternal, "write capabilities: %v", err)
	}
	return exitOK
}
