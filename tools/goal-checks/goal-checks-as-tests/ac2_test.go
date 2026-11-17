//go:build goalchecksastests

package goalchecksastests

import (
	"strings"
	"testing"
)

// TestGoalChecksAC2_SamePullRequest observes that the Goal checks bullet
// retires goal checks in the same pull request and that no separate Goal
// check lifetime bullet remains.
func TestGoalChecksAC2_SamePullRequest(t *testing.T) {
	bullets := sectionBullets(checksSection(t, readAgents(t)))
	goalChecks, ok := findBullet(t, bullets, "Goal checks")

	var problems []string
	if !ok {
		problems = append(problems, `no bullet with the lead "Goal checks"`)
	} else {
		if !strings.Contains(goalChecks, "same pull request") {
			problems = append(problems, `the Goal checks bullet lacks "same pull request"`)
		}
		if strings.Contains(goalChecks, "next change") {
			problems = append(problems, `the Goal checks bullet contains "next change"`)
		}
	}
	if hasLead(bullets, "Goal check lifetime") {
		problems = append(problems, `the section still has a bullet with the lead "Goal check lifetime"`)
	}
	if len(problems) > 0 {
		violation(t, "separate-retirement-remains", problems)
	}
}
