package status

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/record"
)

// A baseline counts as fresh for ha report only when status accepts it as
// evidence: the count never says more than the judgment (PLAN A8, I3).
func TestFreshBaselineNeedsValidEvidence(t *testing.T) {
	cases := []struct {
		name   string
		damage func(*record.Line)
		fresh  bool
		reason string
	}{
		{"valid", func(*record.Line) {}, true, "criterion_missing"},
		{"missing summary", func(l *record.Line) { l.Evidence = nil }, false, "evidence_invalid"},
		{"changed binding", func(l *record.Line) { l.Evidence.DeclarationDigest = strings.Repeat("f", 64) }, false, "evidence_invalid"},
		{"exit zero", func(l *record.Line) { zero := 0; l.Exit = &zero }, false, "record_invalid"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e, line := strictJudgeFixture()
			e.in.Goal = &goaldocs.Goal{Criteria: []goaldocs.Criterion{{ID: "AC-1"}}, Digest: "spec"}
			e.start = &record.Line{BaseCommit: "base"}
			e.clean = true
			e.in.Bundle.Rows[0].CheckPaths = []string{"checks/check.go"}
			e.overlay = map[string]string{"checks/check.go": "d1"}
			b := line("baseline", "fail_as_expected")
			b.BaseCommit, b.TreeClean, b.SpecDigest, b.CommandDigest = "base", true, "spec", e.currentDigest("AC-1")
			b.Overlay = []record.Overlay{{Path: "checks/check.go", Digest: "d1"}}
			tc.damage(b)
			e.in.Lines = []record.Line{*b}
			rep := &Report{commandDigests: map[string]string{}, judged: map[string]JudgedRecord{},
				freshBaselines: map[string]bool{}, confirmations: map[string]int{}}
			if err := e.criteria(rep); err != nil {
				t.Fatal(err)
			}
			if got := rep.FreshBaseline("AC-1"); got != tc.fresh {
				t.Errorf("fresh baseline = %v, want %v", got, tc.fresh)
			}
			if got := rep.Criteria[0].Reason; got != tc.reason {
				t.Errorf("reason = %s, want %s", got, tc.reason)
			}
		})
	}
}

// The record ### Unmet points to for evidence_invalid is the line the
// judgment rejected, under every structured rule (run-record.md §4.4).
func TestJudgedLineNamesInvalidEvidence(t *testing.T) {
	for _, rules := range []string{RulesV3, RulesV4} {
		t.Run(rules, func(t *testing.T) {
			e, line := strictJudgeFixture()
			e.rules = rules
			e.in.Goal = &goaldocs.Goal{Criteria: []goaldocs.Criterion{{ID: "AC-1"}}, Digest: "spec"}
			e.start = &record.Line{BaseCommit: "base"}
			e.clean = true
			e.same = map[string]bool{"c1": true}
			e.in.Bundle.Rows[0].CheckPaths = []string{"checks/check.go"}
			e.overlay = map[string]string{"checks/check.go": "d1"}
			b := line("baseline", "fail_as_expected")
			b.Seq, b.BaseCommit, b.TreeClean, b.SpecDigest, b.CommandDigest = 1, "base", true, "spec", e.currentDigest("AC-1")
			b.Overlay = []record.Overlay{{Path: "checks/check.go", Digest: "d1"}}
			c := line("check", "pass")
			c.Seq, c.Commit, c.TreeClean, c.SpecDigest, c.CommandDigest = 2, "c1", true, "spec", e.currentDigest("AC-1")
			c.Evidence = nil
			e.in.Lines = []record.Line{*b, *c}
			rep := &Report{commandDigests: map[string]string{}, judged: map[string]JudgedRecord{},
				freshBaselines: map[string]bool{}, confirmations: map[string]int{}}
			if err := e.criteria(rep); err != nil {
				t.Fatal(err)
			}
			if got := rep.Criteria[0].Reason; got != "evidence_invalid" {
				t.Fatalf("reason = %s, want evidence_invalid", got)
			}
			if got, ok := rep.judged["AC-1"]; !ok || got.Seq != 2 {
				t.Fatalf("judged record = %+v (%v), want seq 2", got, ok)
			}
		})
	}
}

// judge and validEvidence each keep their own doc comment.
func TestJudgmentDocComments(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "status.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || (fn.Name.Name != "judge" && fn.Name.Name != "validEvidence") {
			continue
		}
		if fn.Doc == nil || !strings.HasPrefix(fn.Doc.Text(), fn.Name.Name+" ") {
			t.Errorf("%s: doc comment does not describe it: %q", fn.Name.Name, fn.Doc.Text())
		}
	}
}
