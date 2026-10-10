package bundle

import (
	"strings"
	"testing"
)

func TestOtherBundlesDeclarationRequiresExactLabel(t *testing.T) {
	tests := []struct {
		name        string
		declaration string
		wantInput   bool
		wantInvalid bool
	}{
		{name: "omitted"},
		{name: "exact", declaration: "다른 목표 실행 묶음: `검사 입력`", wantInput: true},
		{name: "explanation", declaration: "다른 목표 실행 묶음: `검사 입력` (다른 목표의 진행 기록을 읽는다)", wantInput: true},
		{name: "suffixed label", declaration: "다른 목표 실행 묶음 여부: `검사 입력`", wantInvalid: true},
		{name: "missing colon", declaration: "다른 목표 실행 묶음 `검사 입력`", wantInvalid: true},
		{name: "space before colon", declaration: "다른 목표 실행 묶음 : `검사 입력`", wantInvalid: true},
		{name: "wrong separator", declaration: "다른 목표 실행 묶음= `검사 입력`", wantInvalid: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			declaration := ""
			if tt.declaration != "" {
				declaration = "- " + tt.declaration + "\n"
			}
			write(t, root, "g/SPEC.md", spec)
			write(t, root, "g/PLAN.md", strings.Replace(plan, "## 범위\n", "## 범위\n"+declaration, 1))
			write(t, root, "g/REVIEW.md", "# REVIEW\n")
			write(t, root, "g/PROGRESS.md", progress)
			write(t, root, "g/tasks/T001.md", task)
			g, b := load(t, root)
			if b.Scope.OtherBundlesInput != tt.wantInput {
				t.Errorf("OtherBundlesInput = %v, want %v", b.Scope.OtherBundlesInput, tt.wantInput)
			}
			invalid := false
			for _, p := range Lint(g, b) {
				if p.Code == "other_bundles_invalid" {
					invalid = true
				} else {
					t.Errorf("unexpected problem: %v", p)
				}
			}
			if invalid != tt.wantInvalid {
				t.Errorf("other_bundles_invalid = %v, want %v", invalid, tt.wantInvalid)
			}
		})
	}
}
