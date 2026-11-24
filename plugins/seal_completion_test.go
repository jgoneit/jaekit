package plugins

import "testing"

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
