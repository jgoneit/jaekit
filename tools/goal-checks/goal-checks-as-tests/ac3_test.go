//go:build goalchecksastests

package goalchecksastests

import (
	"fmt"
	"testing"
)

// TestGoalChecksAC3_SeparateProducers observes that a Separate producers
// bullet limits separate producers to conditions ordinary tests cannot
// observe (CI, release, install), keeps their completion-record lifetime and
// its regression-or-next-change outcome, and states which goals it covers.
func TestGoalChecksAC3_SeparateProducers(t *testing.T) {
	section := checksSection(t, readAgents(t))
	producers, ok := findBullet(t, sectionBullets(section), "Separate producers")

	var problems []string
	if !ok {
		problems = append(problems, `no bullet with the lead "Separate producers"`)
	} else {
		tokens := []string{
			"cannot observe",
			"CI",
			"release",
			"install",
			"not deleted before that goal's completion record",
			"regression",
			"next change",
			"stands in for the completion record",
			"inside or outside",
			"merged after PR #252",
			"a tracked file or workflow still runs",
		}
		for _, token := range missingTokens(producers, tokens) {
			problems = append(problems, fmt.Sprintf("the Separate producers bullet lacks %q", token))
		}
	}
	if len(problems) > 0 {
		violation(t, "separate-producers-rule-missing", problems)
	}
}
