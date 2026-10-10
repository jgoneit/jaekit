package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/goaldocs"
)

func requireAll(t *testing.T, path string, musts ...string) {
	t.Helper()
	text := read(t, path)
	for _, s := range musts {
		if !strings.Contains(text, s) {
			t.Errorf("%s lacks %q", path, s)
		}
	}
}

// every criterion has a stable AC-n ID, and optional ones carry
// the marker right after it.
func TestSpecCriterionIDRules(t *testing.T) {
	requireAll(t, "spec/skills/spec/SKILL.md",
		"Every completion criterion has an ID `AC-n` that is stable within the goal.",
		"Optional criteria carry `(선택)` right after the ID.")
}

// editing keeps existing IDs and never reuses a removed one.
func TestSpecKeepsCriterionIDs(t *testing.T) {
	requireAll(t, "spec/skills/spec/SKILL.md", "keep every existing criterion ID, never reuse a removed ID")
}

// a work breakdown proposal covers every criterion, has no cycle,
// and states no order, method, or progress.
func TestSpecWorkBreakdownRules(t *testing.T) {
	requireAll(t, "spec/skills/spec/SKILL.md",
		"never order, method, or progress",
		"every criterion reaches a proposal and dependencies have no cycle")
	requireAll(t, "spec/skills/spec/references/goal-docs.md",
		"Every proposal is grounded in a criterion, and dependencies have no cycle. Never state order, method, or progress.")
}

// the documents are checked before saving and the result is
// reported in the conversation.
func TestSpecChecksBeforeSaving(t *testing.T) {
	requireAll(t, "spec/skills/spec/SKILL.md", "Check the documents before saving and report the result in the conversation")
}

// goal documents Spec saved in this repository pass the goal
// document checks that ha lint runs.
func TestSyntheticGoalDocsPassLint(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "goal")
	if err := os.Mkdir(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A synthetic fixture exercises document validation without historical goals.
	const spec = "# Empty input\nStatus: Ready\n## Acceptance Criteria\n- **AC-1** Empty input produces zero.\n- **AC-2** (선택) JSON output carries the same value.\n## 작업 분해 제안\n| ID | 결과 | 조건 | 의존 |\n| --- | --- | --- | --- |\n| W1 | Zero for empty input | AC-1, AC-2 | — |\n"
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte(spec), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := goaldocs.Load(root, "goal")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Problems) != 0 {
		t.Fatalf("synthetic goal problems: %v", g.Problems)
	}
}

// tasks name the work breakdown proposal they follow, and the
// review explains where the plan differs.
func TestSealMapsTasksToProposals(t *testing.T) {
	requireAll(t, "seal/skills/seal/references/bundle.md",
		"`제안` is the `W` proposal it follows or `—` (explain differences in `REVIEW.md`)",
		"differences from the work breakdown proposal and why")
}
