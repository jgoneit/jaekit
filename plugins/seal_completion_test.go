package plugins

import (
	"strings"
	"testing"
)

// once the work starts, the skill asks the user about the items of the
// needs_user state.
func TestSealAsksOnlyForUserDecisions(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "needs_user")
}

// a resume request is answered from the computed state of ha status, and
// PROGRESS.md is brought in line with it.
func TestSealResumesFromComputedState(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "ha status <goal>", "PROGRESS.md")
}

// the role card states that a completion record is reported without waiting
// for the user to check the result, and that the goal reopens only on the
// user's rework request.
func TestSealReportsCompletionWithoutWaiting(t *testing.T) {
	const path = "../contracts/role-card.md"
	text := read(t, path)
	for _, must := range []string{
		"Once a completion record exists, report completion without waiting for the user's confirmation",
		"work again only when the user asks for rework",
	} {
		if !strings.Contains(text, must) {
			t.Errorf("%s lacks %q", path, must)
		}
	}
}
