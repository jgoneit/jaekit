package plugins

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/goaldocs"
)

func TestSpecBoundaryContractExamples(t *testing.T) {
	command := exec.Command("python3", "tools/contract-checks/test_spec_check.py")
	command.Dir = ".."
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "HA_EVIDENCE_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "PYTHONDONTWRITEBYTECODE=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("boundary examples: %v\n%s", err, output)
	}
	pattern := regexp.MustCompile("(?s)## 목표 예시: ([a-z-]+)\\n\\n```markdown\\n(.*?)\\n```")
	matches := pattern.FindAllStringSubmatch(read(t, "../evals/spec-cases/decision-boundaries.md"), -1)
	if len(matches) != 4 {
		t.Fatalf("expected four complete synthetic goals, got %d", len(matches))
	}
	for _, match := range matches {
		t.Run(match[1], func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "goal")
			if err := os.Mkdir(dir, 0755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte(match[2]+"\n"), 0644); err != nil {
				t.Fatal(err)
			}
			goal, err := goaldocs.Load(root, "goal")
			if err != nil {
				t.Fatal(err)
			}
			expected := 2
			if match[1] == "allowlist-ready" {
				expected = 1
			}
			if len(goal.Problems) != 0 || len(goal.Criteria) != expected {
				t.Fatalf("synthetic goal not loadable: criteria=%v problems=%v", goal.Criteria, goal.Problems)
			}
			wantStatus := "Ready"
			if match[1] == "observation-draft" {
				wantStatus = "Draft"
			}
			if goal.Status != wantStatus {
				t.Fatalf("status %s, want %s", goal.Status, wantStatus)
			}
		})
	}
}
