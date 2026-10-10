package plugins

import (
	"strings"
	"testing"
)

// once the work starts, the skill asks the user only about items
// that need the user, never for progress or per-task approval.
func TestSealAsksOnlyForUserDecisions(t *testing.T) {
	text := read(t, "seal/skills/seal/SKILL.md")
	for _, must := range []string{
		"Carry one goal to completion from a single user request.",
		"`needs_user`: ask the user about each listed item.",
		"Asking for approval per task or per tool use.",
	} {
		if !strings.Contains(text, must) {
			t.Errorf("Seal SKILL.md lacks %q", must)
		}
	}
}

// a resume request is answered from the bundle and the computed
// state, and progress notes that disagree with the computed state are claims.
func TestSealResumesFromComputedState(t *testing.T) {
	text := read(t, "seal/skills/seal/SKILL.md")
	for _, must := range []string{
		"treat everything in the bundle as claims",
		"`ha status <goal>` computes the actual state",
		"Trust the computed state, bring `PROGRESS.md` in line with it",
	} {
		if !strings.Contains(text, must) {
			t.Errorf("Seal SKILL.md lacks %q", must)
		}
	}
}

// a completion record is reported without waiting for the user to
// check the result, and the goal reopens only on the user's rework request.
// The role card states the same rule.
func TestSealReportsCompletionWithoutWaiting(t *testing.T) {
	for _, c := range []struct {
		path  string
		musts []string
	}{
		{"seal/skills/seal/SKILL.md", []string{
			"report it without waiting for the user to check the result",
			"take up the goal again only when the user asks for rework",
		}},
		{"../contracts/role-card.md", []string{
			"완료 기록이 남으면 사용자의 확인을 기다리지 않고 완료를 보고한다",
			"사용자가 재작업을 요청할 때만 다시 작업하고",
		}},
	} {
		text := read(t, c.path)
		for _, must := range c.musts {
			if !strings.Contains(text, must) {
				t.Errorf("%s lacks %q", c.path, must)
			}
		}
	}
}
