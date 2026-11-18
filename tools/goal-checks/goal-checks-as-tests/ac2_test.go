//go:build goalchecksastests

package goalchecksastests

import (
	"fmt"
	"strings"
	"testing"
)

// ac2ProducersLead is the one bullet that may name the next change.
const ac2ProducersLead = "Separate producers"

// ac2Preamble returns the section's text before its first bullet. The bullets
// of sectionBullets run without gaps from the first lead line to the end of
// the section, so the preamble is what their lengths leave at the start.
func ac2Preamble(section string, bullets []bullet) string {
	n := len(section)
	for _, b := range bullets {
		n -= len(b.text)
	}
	return section[:n]
}

// TestGoalChecksAC2_SamePullRequest observes that the Goal checks bullet
// retires goal checks in the same pull request, that no separate Goal check
// lifetime bullet remains, and that "next change" appears in the section only
// inside the Separate producers bullet.
func TestGoalChecksAC2_SamePullRequest(t *testing.T) {
	section := checksSection(t, readAgents(t))
	bullets := sectionBullets(section)
	goalChecks, ok := findBullet(t, bullets, "Goal checks")
	// findBullet fails on two Separate producers bullets, so the loop below
	// skips at most one bullet.
	findBullet(t, bullets, ac2ProducersLead)

	var problems []string
	if !ok {
		problems = append(problems, `no bullet with the lead "Goal checks"`)
	} else if !strings.Contains(goalChecks, "same pull request") {
		problems = append(problems, `the Goal checks bullet lacks "same pull request"`)
	}
	if hasLead(bullets, "Goal check lifetime") {
		problems = append(problems, `the section still has a bullet with the lead "Goal check lifetime"`)
	}
	if strings.Contains(ac2Preamble(section, bullets), "next change") {
		problems = append(problems, `the section contains "next change" before its first bullet`)
	}
	for _, b := range bullets {
		if b.lead != ac2ProducersLead && strings.Contains(b.text, "next change") {
			problems = append(problems, fmt.Sprintf("the %s bullet contains %q", b.lead, "next change"))
		}
	}
	if len(problems) > 0 {
		violation(t, "separate-retirement-remains", problems)
	}
}
