//go:build goalchecksastests

package goalchecksastests

import (
	"fmt"
	"strings"
	"testing"
)

// ac1Label names a token in a failure message. The marker prefix is never
// printed literally, so a failure line carries only the declared marker.
func ac1Label(token string) string {
	if token == "jaekit:violation=" {
		return "the violation marker prefix (jaekit:violation followed by =)"
	}
	return fmt.Sprintf("%q", token)
}

// TestGoalChecksAC1_ProductTests observes that the Goal checks bullet states
// the product-test form of goal checks and that the section no longer names a
// build tag or a tools/goal-checks/<goal> location.
func TestGoalChecksAC1_ProductTests(t *testing.T) {
	section := checksSection(t, readAgents(t))
	goalChecks, ok := findBullet(t, sectionBullets(section), "Goal checks")

	var problems []string
	if !ok {
		problems = append(problems, `no bullet with the lead "Goal checks"`)
	} else {
		for _, token := range missingTokens(goalChecks, []string{"go-test-json", "jaekit:violation=", "full verification"}) {
			problems = append(problems, "the Goal checks bullet lacks "+ac1Label(token))
		}
	}
	for _, token := range []string{"build tag", "tools/goal-checks/<"} {
		if strings.Contains(section, token) {
			problems = append(problems, fmt.Sprintf("the section contains %q", token))
		}
	}
	if len(problems) > 0 {
		violation(t, "product-tests-rule-missing", problems)
	}
}
