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

// the quality cases cover AC-19 to AC-22.
func TestSpecCasesCoverMaintenance(t *testing.T) {
	index := read(t, "../evals/spec-cases/README.md")
	for _, c := range []struct {
		name  string
		musts []string
	}{
		{"existing-discount-expiry", []string{"**유지**", "근거 파일 경로와 함께 사실로 적혀 있다", "해당 없음으로 본 이유"}},
		{"ambiguous-current-behavior", []string{"묻지 않아야 할 것", "근거 파일 경로와 함께 사실로 적혀 있다"}},
		{"bug-report-after-implementation", []string{"고치자고 근거와 함께 제안하고, 사용자가 정한 대로 한다", "기존 조건 ID(AC-1~AC-3)는 그대로"}},
	} {
		if !strings.Contains(index, "["+c.name+"]("+c.name+".md)") {
			t.Errorf("evals/spec-cases/README.md does not list %s", c.name)
		}
		text := read(t, "../evals/spec-cases/"+c.name+".md")
		for _, must := range c.musts {
			if !strings.Contains(text, must) {
				t.Errorf("%s.md lacks %q", c.name, must)
			}
		}
	}
}

// a criterion met only by a person's confirmation or by work outside the goal
// becomes an evaluation item after completion, and stays a criterion only
// when the user asks: the goal document contract says so.
func TestSpecMovesHumanChecksToEvaluation(t *testing.T) {
	requireAll(t, "../contracts/goal-docs.md",
		"사람의 확인이나 판단 자체가 충족 조건인 것",
		"완료 뒤 평가 항목으로 관련 맥락에 확인할 곳과 함께 남기고",
		"사용자가 완료 조건으로 두라고 하면 그 말을 출처로 따른다",
	)
}

// a quality case checks AC-28 with a request that holds both a
// person's approval and an observation after completion.
func TestSpecCaseForEvaluationItems(t *testing.T) {
	text := read(t, "../evals/spec-cases/evaluation-items.md")
	for _, must := range []string{
		"디자이너가 화면을 승인", "베타 사용자", "완료 조건에 들어가면 안 되는 것",
		"완료 뒤 평가 항목으로 관련 맥락에", "결정 목록에 그 두 항목을 평가 항목으로 옮겼다", "질문 상한",
	} {
		if !strings.Contains(text, must) {
			t.Errorf("evaluation-items.md lacks %q", must)
		}
	}
	if !strings.Contains(read(t, "../evals/spec-cases/README.md"), "[evaluation-items](evaluation-items.md)") {
		t.Error("evals/spec-cases/README.md does not list evaluation-items")
	}
}
