package status

import (
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
