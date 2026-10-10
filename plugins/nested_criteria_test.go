package plugins

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/goaldocs"
)

func readNestedExample(t *testing.T, source string) *goaldocs.Goal {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "goal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "goal", "SPEC.md"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := goaldocs.Load(root, "goal")
	if err != nil {
		t.Fatal(err)
	}
	if len(g.Problems) != 0 {
		t.Fatalf("producer example cannot be read: %v", g.Problems)
	}
	return g
}

// Parse the actual distributed templates and format examples. This checks the
// producer/consumer boundary without requiring a model or an installed CLI.
// It uses only the existing Load API so this file also works as a baseline overlay.
func TestNestedCriteriaTemplates(t *testing.T) {
	for name, count := range map[string]int{
		"small-change": 2, "backend-feature": 3, "cross-module-feature": 4, "product": 4,
	} {
		t.Run(name, func(t *testing.T) {
			source := read(t, "spec/skills/spec/assets/templates/"+name+".md")
			prologue, _, _ := strings.Cut(source, "\n## ")
			if !strings.Contains(prologue, "\nCriteria-Format: nested/1\n") {
				t.Fatal("new document template must select nested/1 in its prologue")
			}
			g := readNestedExample(t, source)
			if len(g.Criteria) != count {
				t.Fatalf("criteria = %d, want %d", len(g.Criteria), count)
			}
			for i, criterion := range g.Criteria {
				if criterion.ID != fmt.Sprintf("AC-%d", i+1) || criterion.Optional {
					t.Fatalf("template criterion identity/requiredness changed: %#v", criterion)
				}
			}
			parts := strings.Split(g.Criteria[0].Text, "\n")
			if len(parts) != 2 || !strings.HasPrefix(parts[1], "- ") {
				t.Fatalf("template's supporting detail must stay inside AC-1: %q", g.Criteria[0].Text)
			}
		})
	}
	markdownExample := regexp.MustCompile("(?s)```markdown\\n(.*?)\\n```")
	for _, file := range []string{"../contracts/goal-docs.md", "spec/skills/spec/references/goal-docs.md"} {
		t.Run(file, func(t *testing.T) {
			var example string
			for _, match := range markdownExample.FindAllStringSubmatch(read(t, file), -1) {
				if strings.Contains(match[1], "## Acceptance Criteria") {
					example = match[1]
					break
				}
			}
			if !strings.HasPrefix(example, "Criteria-Format: nested/1\n") {
				t.Fatal("criteria example must show explicit format selection")
			}
			g := readNestedExample(t, "# Synthetic goal\nStatus: Ready\n"+example+"\n")
			if len(g.Criteria) != 3 || g.Criteria[0].Optional || g.Criteria[1].Optional || !g.Criteria[2].Optional {
				t.Fatalf("example criterion count/requiredness changed: %#v", g.Criteria)
			}
			for i, criterion := range g.Criteria {
				if criterion.ID != fmt.Sprintf("AC-%d", i+1) {
					t.Fatalf("example criterion ID changed: %#v", criterion)
				}
			}
			parts := strings.Split(g.Criteria[0].Text, "\n")
			if len(parts) != 4 || !strings.HasPrefix(parts[1], "- ") || parts[2] != "" || parts[3] == "" {
				t.Fatalf("example child list and continued paragraph were not preserved: %q", g.Criteria[0].Text)
			}
		})
	}
}
