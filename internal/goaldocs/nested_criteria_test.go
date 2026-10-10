package goaldocs

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// These tests use the existing Load API so the test file can also be overlaid
// onto the previous reader without copying the implementation under test.
func loadNestedFixture(t *testing.T, source string) *Goal {
	t.Helper()
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "goal"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "goal", "SPEC.md"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := Load(root, "goal")
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const nestedHeader = "# Synthetic goal\nStatus: Ready\nCriteria-Format: nested/1\n\n## Acceptance Criteria\n"

func TestNestedCriteriaParsing(t *testing.T) {
	for rootIndent := 0; rootIndent <= 3; rootIndent++ {
		for childIndent := 2; childIndent <= 4; childIndent++ {
			t.Run(fmt.Sprintf("root%d-child%d", rootIndent, childIndent), func(t *testing.T) {
				root := strings.Repeat(" ", rootIndent)
				child := strings.Repeat(" ", rootIndent+childIndent)
				source := nestedHeader + root + "- **AC-1** first\n" +
					child + "- detail\n" + child + "continued\n\n\n" + child + "paragraph\n" +
					child + "- **AC-99** quoted identifier\n\n" + root + "* AC-2 (선택) second\n\n## Open Decisions\nNone\n"
				g := loadNestedFixture(t, source)
				if len(g.Problems) != 0 {
					t.Fatalf("problems: %v", g.Problems)
				}
				want := []Criterion{
					{ID: "AC-1", Text: "first\n- detail\ncontinued\n\nparagraph\n- **AC-99** quoted identifier", Line: 6},
					{ID: "AC-2", Optional: true, Text: "second", Line: 14},
				}
				if !reflect.DeepEqual(g.Criteria, want) {
					t.Fatalf("criteria = %#v, want %#v", g.Criteria, want)
				}
			})
		}
	}
	t.Run("paragraph-boundaries-and-fences", func(t *testing.T) {
		g := loadNestedFixture(t, nestedHeader+"- AC-1 first\n\n  next paragraph\n\n```\n- AC-90 ignored\n```\n\n  final paragraph\n\n- AC-2 second\n\n")
		if len(g.Problems) != 0 || len(g.Criteria) != 2 || g.Criteria[0].Text != "first\n\nnext paragraph\n\nfinal paragraph" || g.Criteria[1].Text != "second" {
			t.Fatalf("criteria=%#v problems=%v", g.Criteria, g.Problems)
		}
	})
	t.Run("marker-padding-sets-content-column", func(t *testing.T) {
		g := loadNestedFixture(t, nestedHeader+"-   AC-1 first\n    + child\n- AC-2 second\n")
		if len(g.Problems) != 0 || len(g.Criteria) != 2 || g.Criteria[0].Text != "first\n+ child" {
			t.Fatalf("criteria=%#v problems=%v", g.Criteria, g.Problems)
		}
	})
}

func TestNestedCriteriaDiagnostics(t *testing.T) {
	cases := []struct {
		name, source, code string
		line               int
	}{
		{"missing-id", nestedHeader + "- AC-1 first\n- missing ID\n", "criterion_without_id", 7},
		{"duplicate-id", nestedHeader + "- AC-1 first\n- **AC-1** duplicate\n", "criterion_duplicate_id", 7},
		{"ambiguous-list-indent", nestedHeader + "- AC-1 first\n - child\n", "criterion_indentation", 7},
		{"ambiguous-continuation", nestedHeader + "- AC-1 first\n continuation\n", "criterion_indentation", 7},
		{"before-root-column", nestedHeader + "  - AC-1 first\n- AC-2 second\n", "criterion_indentation", 7},
		{"marker-padding", nestedHeader + "-   AC-1 first\n  - child\n", "criterion_indentation", 7},
		{"tab-indent", nestedHeader + "- AC-1 first\n\tchild\n", "criterion_indentation", 7},
		{"tab-after-marker", nestedHeader + "-\tAC-1 first\n", "criterion_indentation", 6},
		{"orphan-body", nestedHeader + "  no parent\n- AC-1 first\n", "criterion_indentation", 6},
		{"root-too-deep", nestedHeader + "    - AC-1 first\n", "criterion_indentation", 6},
		{"unknown-format", strings.Replace(nestedHeader, "nested/1", "nested/99", 1) + "- AC-1 first\n", "criteria_format", 3},
		{"empty-format", strings.Replace(nestedHeader, "nested/1", "", 1) + "- AC-1 first\n", "criteria_format", 3},
		{"one-space-format", strings.Replace(nestedHeader, "Criteria-Format:", " Criteria-Format:", 1) + "- AC-1 first\n\n  paragraph\n", "criteria_format", 3},
		{"three-space-format", strings.Replace(nestedHeader, "Criteria-Format:", "   Criteria-Format:", 1) + "- AC-1 first\n\n  paragraph\n", "criteria_format", 3},
		{"tab-format", strings.Replace(nestedHeader, "Criteria-Format:", "\tCriteria-Format:", 1) + "- AC-1 first\n\n  paragraph\n", "criteria_format", 3},
		{"duplicate-format", strings.Replace(nestedHeader, "\n\n##", "\nCriteria-Format: nested/1\n\n##", 1) + "- AC-1 first\n", "criteria_format_duplicate", 4},
		{"misplaced-format", "# Goal\nStatus: Ready\n## Acceptance Criteria\nCriteria-Format: nested/1\n- AC-1 first\n", "criteria_format", 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := loadNestedFixture(t, tc.source)
			for _, p := range g.Problems {
				if p.Code == tc.code && p.Line == tc.line && p.File == "goal/SPEC.md" && p.Detail != "" {
					return
				}
			}
			t.Fatalf("missing %s on line %d: %v", tc.code, tc.line, g.Problems)
		})
	}
}

func TestNestedCriteriaCompatibility(t *testing.T) {
	for _, prefix := range []string{" ", "   ", "\t"} {
		source := "# Goal\nStatus: Ready\n## Acceptance Criteria\n- AC-1 first\n" + prefix + "Criteria-Format: nested/1\n- AC-2 second\n"
		quoted := loadNestedFixture(t, source)
		want := []Criterion{{ID: "AC-1", Text: "first\nCriteria-Format: nested/1", Line: 4}, {ID: "AC-2", Text: "second", Line: 6}}
		if len(quoted.Problems) != 0 || !reflect.DeepEqual(quoted.Criteria, want) {
			t.Fatalf("legacy body quote with prefix %q: criteria=%#v problems=%v", prefix, quoted.Criteria, quoted.Problems)
		}
	}
	legacy := "# Goal\nStatus: Ready\n## Acceptance Criteria\n- AC-1 first\n continued\n\tand tab\n    - existing body\n\n    ignored after blank\n  - AC-2 (선택) second\n"
	g := loadNestedFixture(t, legacy)
	if len(g.Problems) != 0 || len(g.Criteria) != 2 {
		t.Fatalf("legacy criteria=%#v problems=%v", g.Criteria, g.Problems)
	}
	want := []Criterion{{ID: "AC-1", Text: "first\ncontinued\nand tab\n- existing body", Line: 4},
		{ID: "AC-2", Optional: true, Text: "second", Line: 10}}
	if !reflect.DeepEqual(g.Criteria, want) {
		t.Fatalf("legacy criteria = %#v, want %#v", g.Criteria, want)
	}
	for i, c := range g.Criteria {
		h := sha256.Sum256([]byte(want[i].Text))
		if c.TextSHA256() != hex.EncodeToString(h[:]) {
			t.Fatalf("legacy confirmation text changed for %s", c.ID)
		}
	}
	// A fenced example must not opt an existing document into the new contract.
	fenced := loadNestedFixture(t, strings.Replace(legacy, "## Acceptance Criteria", "```\nCriteria-Format: nested/1\n```\n## Acceptance Criteria", 1))
	for i := range fenced.Criteria {
		if fenced.Criteria[i].TextSHA256() != g.Criteria[i].TextSHA256() {
			t.Fatal("a fenced format example changed criterion text")
		}
	}
	// The format is part of the user's document bytes, not ambient parser state.
	// Explicitly changing it changes the same digest used by existing evidence.
	oldSource := "# Goal\nStatus: Ready\n## Acceptance Criteria\n- AC-1 first\n  - AC-2 second\n"
	old := loadNestedFixture(t, oldSource)
	selectedSource := strings.Replace(oldSource, "Status: Ready\n", "Status: Ready\nCriteria-Format: nested/1\n", 1)
	selected := loadNestedFixture(t, selectedSource)
	if len(old.Problems) != 0 || len(selected.Problems) != 0 || len(old.Criteria) != 2 || len(selected.Criteria) != 1 {
		t.Fatalf("old=%#v selected=%#v problems=%v", old.Criteria, selected.Criteria, selected.Problems)
	}
	if selected.Criteria[0].Text != "first\n- AC-2 second" || selected.Digest == old.Digest || selected.Criteria[0].TextSHA256() == old.Criteria[0].TextSHA256() {
		t.Fatal("explicit format selection must bind the new meaning to changed document/text digests")
	}
	if again := loadNestedFixture(t, oldSource); again.Digest != old.Digest || !reflect.DeepEqual(again.Criteria, old.Criteria) {
		t.Fatal("reading a selected document must not change later legacy reads")
	}
}

func TestNestedCriteriaFenceBoundaries(t *testing.T) {
	preamble := "# Goal\nStatus: Ready\n"
	body := "## Acceptance Criteria\n- AC-1 first\n  - AC-2 (선택) second\n"
	legacy := loadNestedFixture(t, preamble+body)
	cases := []struct{ name, example string }{
		{"shorter-backticks", "````markdown\n```\nCriteria-Format: nested/1\n```\n````\n"},
		{"shorter-tildes", "~~~~markdown\n~~~\nCriteria-Format: nested/1\n~~~\n~~~~\n"},
		{"closing-backticks-with-text", "```markdown\n``` still inside\nCriteria-Format: nested/1\n``` also inside\n```\n"},
		{"closing-tildes-with-text", "~~~markdown\n~~~ still inside\nCriteria-Format: nested/1\n~~~ also inside\n~~~\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			g := loadNestedFixture(t, preamble+tc.example+body)
			if len(g.Problems) != 0 || len(g.Criteria) != len(legacy.Criteria) {
				t.Fatalf("fenced selector changed legacy criteria: %#v problems=%v", g.Criteria, g.Problems)
			}
			for i, got := range g.Criteria {
				want := legacy.Criteria[i]
				if got.ID != want.ID || got.Optional != want.Optional || got.Text != want.Text || got.TextSHA256() != want.TextSHA256() {
					t.Fatalf("fenced selector changed criterion %s: %#v, want %#v", got.ID, got, want)
				}
			}
			// A real selector after the example still opts in. The quoted value
			// may even be unsupported without becoming active metadata.
			example := strings.ReplaceAll(tc.example, "nested/1", "nested/99")
			selected := loadNestedFixture(t, preamble+example+"Criteria-Format: nested/1\n"+body)
			if len(selected.Problems) != 0 || len(selected.Criteria) != 1 || selected.Criteria[0].Text != "first\n- AC-2 (선택) second" {
				t.Fatalf("real selector after fence was not selected: %#v problems=%v", selected.Criteria, selected.Problems)
			}
		})
	}
}
