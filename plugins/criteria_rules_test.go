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

// every criterion has an AC-n ID, and optional ones carry the (선택)
// marker: both stay format tokens of the Spec skill.
func TestSpecCriterionIDRules(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "AC-n", "(선택)")
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

// tasks name the work breakdown proposal they follow in the 제안 column, or
// none, and the review explains where the plan differs: the column name, the
// W proposal IDs, the none mark and REVIEW.md stay format tokens of the Seal
// skill.
func TestSealMapsTasksToProposals(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, "제안", "W", "—", "REVIEW.md")
}
