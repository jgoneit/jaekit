package plugins

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// the Spec plugin carries its guidance, references, and templates
// itself, and no document in it links outside the plugin.
func TestSpecPluginSelfContained(t *testing.T) {
	root, err := filepath.Abs("spec")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		"spec/skills/spec/SKILL.md",
		"spec/skills/spec/references/goal-docs.md",
		"spec/skills/spec/assets/templates/small-change.md",
		"spec/skills/spec/assets/templates/backend-feature.md",
		"spec/skills/spec/assets/templates/cross-module-feature.md",
		"spec/skills/spec/assets/templates/product.md",
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("Spec plugin lacks %s", p)
		}
	}
	link := regexp.MustCompile(`\]\(([^)#\s]+)`)
	err = filepath.WalkDir("spec", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" {
			return err
		}
		for _, m := range link.FindAllStringSubmatch(read(t, p), -1) {
			if strings.Contains(m[1], "://") {
				continue
			}
			target, err := filepath.Abs(filepath.Join(filepath.Dir(p), m[1]))
			if err != nil {
				return err
			}
			if !strings.HasPrefix(target, root+string(filepath.Separator)) {
				t.Errorf("%s links outside the Spec plugin: %s", p, m[1])
			} else if _, err := os.Stat(target); err != nil {
				t.Errorf("%s links to a missing file: %s", p, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

// the quality case results record omissions, distortions, added
// scope, unneeded questions, and changes outside the goal documents per case,
// together with the host and the invocation.

// the quality case index lists the maintenance cases.
func TestSpecCasesCoverMaintenance(t *testing.T) {
	index := read(t, "../evals/spec-cases/README.md")
	for _, name := range []string{"existing-discount-expiry", "ambiguous-current-behavior", "bug-report-after-implementation"} {
		if !strings.Contains(index, "["+name+"]("+name+".md)") {
			t.Errorf("evals/spec-cases/README.md does not list %s", name)
		}
	}
}

// the quality case index lists the evaluation-items case.
func TestSpecCaseForEvaluationItems(t *testing.T) {
	if !strings.Contains(read(t, "../evals/spec-cases/README.md"), "[evaluation-items](evaluation-items.md)") {
		t.Error("evals/spec-cases/README.md does not list evaluation-items")
	}
}
