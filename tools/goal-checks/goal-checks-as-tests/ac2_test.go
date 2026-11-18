//go:build goalchecksastests

package goalchecksastests

import (
	"fmt"
	"strings"
	"testing"
)

// ac2ProducersLead is the one bullet that may name the next change.
const ac2ProducersLead = "Separate producers"

// ac2OutsideBullets returns the section's text that is in no bullet: the text
// before the first bullet, between bullets and after the last one. The pieces
// are joined at line starts, so a token without a newline never spans two.
func ac2OutsideBullets(section string, bullets []bullet) string {
	var b strings.Builder
	prev := 0
	for _, bl := range bullets {
		b.WriteString(section[prev:bl.off])
		prev = bl.off + len(bl.text)
	}
	b.WriteString(section[prev:])
	return b.String()
}

// TestGoalChecksAC2_SamePullRequest observes that the Goal checks bullet
// retires goal checks in the same pull request, that no separate Goal check
// lifetime bullet remains, and that "next change" appears in the section only
// inside the Separate producers bullet's own text: not in the preamble, another
// bullet or text after the list.
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
	if strings.Contains(ac2OutsideBullets(section, bullets), "next change") {
		problems = append(problems, `the section contains "next change" outside its bullets`)
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
