//go:build goalchecksastests

package goalchecksastests

import (
	"fmt"
	"testing"
)

// TestGoalChecksAC3_SeparateProducers observes that a Separate producers
// bullet keeps the completion-record lifetime for CI, release and install
// producers.
func TestGoalChecksAC3_SeparateProducers(t *testing.T) {
	section := checksSection(t, readAgents(t))
	producers, ok := findBullet(t, sectionBullets(section), "Separate producers")

	var problems []string
	if !ok {
		problems = append(problems, `no bullet with the lead "Separate producers"`)
	} else {
		tokens := []string{
			"CI",
			"release",
			"install",
			"not deleted before that goal's completion record",
			"stands in for the completion record",
		}
		for _, token := range missingTokens(producers, tokens) {
			problems = append(problems, fmt.Sprintf("the Separate producers bullet lacks %q", token))
		}
	}
	if len(problems) > 0 {
		violation(t, "separate-producers-rule-missing", problems)
	}
}
