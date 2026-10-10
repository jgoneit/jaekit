package main

import (
	"encoding/json"

	"github.com/jgoneit/jaekit/internal/status"
)

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
	report := struct {
		Schema         string              `json:"schema"`
		HaVersion      string              `json:"ha_version"`
		SupportedRules []string            `json:"supported_rules"`
		DefaultRules   string              `json:"default_rules"`
		Formats        map[string][]string `json:"formats"`
		Features       []string            `json:"features"`
	}{
		Schema:         "ha-capabilities/v1",
		HaVersion:      version,
		SupportedRules: []string{status.RulesV1, status.RulesV2, status.RulesV3},
		DefaultRules:   status.Rules,
		Formats: map[string][]string{
			"criteria":          {"legacy", "nested/1"},
			"check_declaration": {"check-declaration/v1"},
			"check_result":      {"check-result/v1"},
			"check_evidence":    {"check-evidence/v1"},
		},
		Features: []string{"estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1", "structured-change-results/v1", "budget-change/v1"},
	}
	if err := json.NewEncoder(c.out).Encode(report); err != nil {
		return c.fail(exitInternal, "write capabilities: %v", err)
	}
	return exitOK
}
