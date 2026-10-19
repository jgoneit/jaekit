package goaldocs

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const assuranceBody = "\n## Acceptance Criteria\n- AC-1 first\n\n## Open Decisions\nNone\n"

func assuranceDoc(header ...string) string {
	return "# Synthetic goal\n" + strings.Join(header, "\n") + "\n" + assuranceBody
}

func problemCodes(g *Goal) map[string][]int {
	out := map[string][]int{}
	for _, p := range g.Problems {
		out[p.Code] = append(out[p.Code], p.Line)
	}
	return out
}

func TestAssuranceLineIsRead(t *testing.T) {
	for _, tier := range AssuranceTiers {
		g := loadNestedFixture(t, assuranceDoc("Status: Ready", "Criteria-Format: nested/1", "Assurance: "+tier))
		if len(g.Problems) != 0 || g.Assurance != tier {
			t.Fatalf("%s: assurance=%q problems=%v", tier, g.Assurance, g.Problems)
		}
	}
	cases := map[string]string{
		"trailing-space": assuranceDoc("Status: Ready", "Assurance: strict \t "),
		"crlf":           strings.ReplaceAll(assuranceDoc("Status: Ready", "Assurance: strict"), "\n", "\r\n"),
		"fenced-lines": assuranceDoc("Status: Ready", "Assurance: light", "", "````markdown", "Assurance: Strict", "```",
			"Assurance: unknown", "```` not a close", "Assurance: high", "````", "", "~~~", "  Assurance: strict", "~~~"),
	}
	want := map[string]string{"trailing-space": "strict", "crlf": "strict", "fenced-lines": "light"}
	for name, source := range cases {
		g := loadNestedFixture(t, source)
		if len(g.Problems) != 0 || g.Assurance != want[name] {
			t.Fatalf("%s: assurance=%q problems=%v", name, g.Assurance, g.Problems)
		}
	}
	g := loadNestedFixture(t, assuranceDoc("Status: Ready"))
	if len(g.Problems) != 0 || g.Assurance != "" {
		t.Fatalf("absent line: assurance=%q problems=%v", g.Assurance, g.Problems)
	}
}

func TestAssuranceLineDiagnostics(t *testing.T) {
	cases := []struct {
		name   string
		source string
		code   string
		line   int
	}{
		{"capitalized", assuranceDoc("Status: Ready", "Assurance: Light"), "assurance_format", 3},
		{"empty", assuranceDoc("Status: Ready", "Assurance:"), "assurance_format", 3},
		{"unknown", assuranceDoc("Status: Ready", "Assurance: high"), "assurance_format", 3},
		{"comment", assuranceDoc("Status: Ready", "Assurance: light # comment"), "assurance_format", 3},
		{"space-indent", assuranceDoc("Status: Ready", "  Assurance: light"), "assurance_format", 3},
		{"tab-indent", assuranceDoc("Status: Ready", "\tAssurance: light"), "assurance_format", 3},
		{"duplicate", assuranceDoc("Status: Ready", "Assurance: light", "Assurance: light"), "assurance_format_duplicate", 4},
		{"after-section", "# Synthetic goal\nStatus: Ready\n\n## 목표\nAssurance: strict\n" + assuranceBody, "assurance_format", 5},
	}
	for _, tc := range cases {
		g := loadNestedFixture(t, tc.source)
		codes := problemCodes(g)
		lines, ok := codes[tc.code]
		if !ok || lines[0] != tc.line || g.Assurance != "" {
			t.Fatalf("%s: assurance=%q problems=%v, want %s at line %d", tc.name, g.Assurance, g.Problems, tc.code, tc.line)
		}
	}
	g := loadNestedFixture(t, assuranceDoc("Status: Ready", "Criteria-Format: nested/2", "Assurance: Light"))
	codes := problemCodes(g)
	if len(codes["criteria_format"]) != 1 || len(codes["assurance_format"]) != 1 {
		t.Fatalf("both header errors must be reported: %v", g.Problems)
	}
}

func TestAssuranceLineIsDeterministic(t *testing.T) {
	source := assuranceDoc("Status: Ready", "Assurance: standard")
	first := loadNestedFixture(t, source)
	for _, env := range [][2]string{{"TZ", "Asia/Seoul"}, {"LC_ALL", "ko_KR.UTF-8"}, {"LANG", "C"}} {
		t.Setenv(env[0], env[1])
		g := loadNestedFixture(t, source)
		if g.Assurance != first.Assurance || g.Digest != first.Digest || len(g.Problems) != 0 {
			t.Fatalf("%s=%s changed the reading: %q %v", env[0], env[1], g.Assurance, g.Problems)
		}
	}
}

func TestAmendmentsStayOutsideGoalSet(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "goal")
	if err := os.MkdirAll(filepath.Join(dir, AmendmentDir), 0o755); err != nil {
		t.Fatal(err)
	}
	source := assuranceDoc("Status: Ready") + "\n수정안: [wording](amendments/wording.md), [old](./amendments/../amendments/old.md)\n"
	if err := os.WriteFile(filepath.Join(dir, "SPEC.md"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := Load(root, "goal")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, AmendmentDir, "wording.md"), []byte("# 수정안\n\n## 계기\n\nF1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	after, err := Load(root, "goal")
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Problems) != 0 || len(after.Docs) != 1 || after.Docs[0] != "goal/SPEC.md" || after.Digest != before.Digest {
		t.Fatalf("amendments joined the goal document set: docs=%v problems=%v", after.Docs, after.Problems)
	}
}
