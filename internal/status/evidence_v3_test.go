package status

import (
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/evidence"
	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/record"
)

func judgmentLine(kind, result string) *record.Line {
	exit := 0
	if result == "fail" || result == "fail_as_expected" {
		exit = 1
	}
	l := &record.Line{Header: record.Header{Kind: kind}, Target: &record.Target{Criterion: "AC-1"}, CriterionKind: bundle.KindChange, Result: result, Exit: &exit}
	if result == "error" {
		l.Exit = nil
	}
	return l
}

func strictJudgeFixture() (*evaluator, func(string, string) *record.Line) {
	digest := strings.Repeat("a", 64)
	d := &evidence.Declaration{Schema: evidence.DeclarationSchema, Producer: "synthetic", ProducerVersion: "1", ProducerPaths: []string{"checks/check.go"}, Targets: []evidence.Target{{ID: "behavior", Violation: "missing"}}}
	e := &evaluator{rules: RulesV3, in: Input{Bundle: &bundle.Bundle{Rows: []bundle.Row{{ID: "AC-1", Kind: bundle.KindChange, Evidence: d, EvidenceDigest: digest}}}}}
	line := func(kind, result string) *record.Line {
		l := judgmentLine(kind, result)
		s := &evidence.Summary{Schema: evidence.SummarySchema, Result: result, DeclarationDigest: digest, ReportDigest: strings.Repeat("b", 64), Invocation: strings.Repeat("c", 64)}
		s.Targets = []evidence.Observation{{Target: "behavior", Attempt: 1, Status: "pass"}}
		switch result {
		case "pass":
			s.Reason = "targets_passed"
		case "fail_as_expected":
			s.Reason = "declared_violation"
			s.Targets[0].Status = "violation"
			s.Targets[0].Violation = "missing"
		case "unexpected_pass":
			s.Reason = "baseline_passed"
		case "fail":
			s.Targets[0].Status = "violation"
			if kind == "baseline" {
				s.Reason = "unexpected_assertion"
				s.Targets[0].Violation = "different"
			} else {
				s.Reason = "assertion_failed"
				s.Targets[0].Violation = "missing"
			}
		case "error":
			s.Reason = "process_start"
			s.Targets = nil
			s.ReportDigest = ""
		case "unknown":
			s.Reason = "report_missing"
			s.Targets = nil
			s.ReportDigest = ""
		}
		l.Evidence = s
		return l
	}
	return e, line
}

func TestJudgmentRequiresPositiveEvidenceAllRules(t *testing.T) {
	for _, rules := range []string{RulesV1, RulesV2, RulesV3} {
		e := &evaluator{rules: rules}
		for _, result := range []string{"", "unsupported", "fail_as_expected"} {
			if got := e.judge("AC-1", bundle.KindMaintain, []*record.Line{judgmentLine("check", result)}, nil); got != "record_invalid" {
				t.Fatalf("%s %q accepted: %s", rules, result, got)
			}
		}
		if got := e.judge("AC-1", bundle.KindMaintain, []*record.Line{judgmentLine("check", "error")}, nil); got != "criterion_error" {
			t.Fatalf("error-only %s: %s", rules, got)
		}
		if got := e.judge("AC-1", bundle.KindMaintain, []*record.Line{judgmentLine("check", "error"), judgmentLine("check", "pass")}, nil); got != "" {
			t.Fatalf("allowed retry %s: %s", rules, got)
		}
		if got := e.judge("AC-1", bundle.KindMaintain, []*record.Line{judgmentLine("check", "error"), judgmentLine("check", "error"), judgmentLine("check", "pass")}, nil); got != "error_limit" {
			t.Fatalf("error cap %s: %s", rules, got)
		}
	}
	for _, rules := range []string{RulesV1, RulesV2} {
		e := &evaluator{rules: rules}
		if got := e.judge("AC-1", bundle.KindChange, []*record.Line{judgmentLine("check", "pass")}, []*record.Line{judgmentLine("baseline", "fail_as_expected")}); got != "" {
			t.Fatalf("legacy proof changed %s: %s", rules, got)
		}
	}
}

func TestOutputFailureAfterExitIsAnErrorUnderNewRules(t *testing.T) {
	zero := 0
	for _, kind := range []string{"", bundle.KindMaintain, bundle.KindChange} {
		l := &record.Line{Header: record.Header{Kind: "check"}, CriterionKind: kind, Result: "error", Exit: &zero}
		if !validResult(l, RulesV3) {
			t.Fatal("attempted output failure became invalid")
		}
		if validResult(l, RulesV2) {
			t.Fatal("legacy error exit interpretation changed")
		}
	}
}

func TestStrictJudgmentPreservesAdverseEvidenceAndErrorCap(t *testing.T) {
	e, line := strictJudgeFixture()
	for _, tc := range []struct {
		name              string
		checks, baselines []string
		want              string
	}{
		{"positive", []string{"pass"}, []string{"fail_as_expected"}, ""},
		{"unknown then pass", []string{"unknown", "pass"}, []string{"fail_as_expected"}, ""},
		{"mixed error cap", []string{"unknown", "error", "pass"}, []string{"fail_as_expected"}, "error_limit"},
		{"baseline mixed cap", []string{"pass"}, []string{"unknown", "error", "fail_as_expected"}, "error_limit"},
		{"baseline cap without current check", nil, []string{"unknown", "unknown"}, "error_limit"},
		{"current failure", []string{"fail", "pass"}, []string{"fail_as_expected"}, "criterion_flaky"},
		{"unexpected violation", []string{"pass"}, []string{"fail", "fail_as_expected"}, "baseline_failed"},
		{"unexpected pass", []string{"pass"}, []string{"unexpected_pass", "fail_as_expected"}, "baseline_unexpected_pass"},
		{"missing report", []string{"unknown"}, []string{"fail_as_expected"}, "criterion_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var C, B []*record.Line
			for _, r := range tc.checks {
				C = append(C, line("check", r))
			}
			for _, r := range tc.baselines {
				B = append(B, line("baseline", r))
			}
			if got := e.judge("AC-1", bundle.KindChange, C, B); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
	current, baseline := line("check", "pass"), line("baseline", "fail_as_expected")
	current.Evidence = nil
	if got := e.judge("AC-1", bundle.KindChange, []*record.Line{current}, []*record.Line{baseline}); got != "evidence_invalid" {
		t.Fatalf("missing proof: %s", got)
	}
	current = line("check", "pass")
	current.Evidence.DeclarationDigest = strings.Repeat("f", 64)
	if got := e.judge("AC-1", bundle.KindChange, []*record.Line{current}, []*record.Line{baseline}); got != "evidence_invalid" {
		t.Fatalf("changed binding: %s", got)
	}
}

func TestInvalidResultPreventsStoredCompletion(t *testing.T) {
	for _, rules := range []string{RulesV1, RulesV2, RulesV3} {
		bad := judgmentLine("check", "unsupported")
		bad.Seq = 4
		e := &evaluator{rules: rules, same: map[string]bool{"code": true}, in: Input{
			Goal: &goaldocs.Goal{Digest: "spec"}, Bundle: &bundle.Bundle{},
			Lines: []record.Line{{Header: record.Header{Kind: "done", Seq: 3}, Status: Complete, SpecDigest: "spec", Commit: "code"}, *bad},
		}}
		if err := e.collectUsage(&Report{}); err != nil {
			t.Fatal(err)
		}
		if len(e.invalid) != 1 {
			t.Fatalf("%s ignored malformed stale record", rules)
		}
		e.clean = true
		got, err := e.completionRecord(&Report{})
		if err != nil || got != nil {
			t.Fatalf("%s reused completed malformed history", rules)
		}
	}
}

func TestNativeRetryExitZeroRemainsAdverseNotMalformed(t *testing.T) {
	e, line := strictJudgeFixture()
	for _, kind := range []string{"check", "baseline"} {
		adverse := line(kind, "fail")
		zero := 0
		adverse.Exit = &zero
		adverse.Evidence.Reason = "inconsistent_attempts"
		adverse.Evidence.Targets = []evidence.Observation{
			{Target: "behavior", Attempt: 1, Status: "violation", Violation: "missing"},
			{Target: "behavior", Attempt: 2, Status: "pass"},
		}
		if !validResult(adverse, RulesV3) {
			t.Fatal("valid native retry became malformed")
		}
		if validResult(adverse, RulesV2) {
			t.Fatal("structured retry exception changed legacy exits")
		}
		C, B := []*record.Line{line("check", "pass")}, []*record.Line{line("baseline", "fail_as_expected")}
		want := "criterion_flaky"
		if kind == "check" {
			C = append([]*record.Line{adverse}, C...)
		} else {
			B = append([]*record.Line{adverse}, B...)
			want = "baseline_failed"
		}
		if got := e.judge("AC-1", bundle.KindChange, C, B); got != want {
			t.Fatalf("native %s got %s, want %s", kind, got, want)
		}
	}
}

func TestMixedEnvironmentCannotEraseAdverseObservation(t *testing.T) {
	e, line := strictJudgeFixture()
	for _, tc := range []struct{ kind, violation, want string }{
		{"check", "missing", "criterion_flaky"},
		{"baseline", "different", "baseline_failed"},
		{"baseline", "missing", ""}, // Expected baseline violation is not adverse.
	} {
		t.Run(tc.kind+tc.violation, func(t *testing.T) {
			mixed := line(tc.kind, "error")
			one := 1
			mixed.Exit = &one
			mixed.Evidence.Reason = "environment_reported"
			mixed.Evidence.ReportDigest = strings.Repeat("b", 64)
			mixed.Evidence.Targets = []evidence.Observation{
				{Target: "behavior", Attempt: 1, Status: "violation", Violation: tc.violation},
				{Target: "behavior", Attempt: 2, Status: "error", Reason: "dependency_missing"},
			}
			C, B := []*record.Line{line("check", "pass")}, []*record.Line{line("baseline", "fail_as_expected")}
			if tc.kind == "check" {
				C = append([]*record.Line{mixed}, C...)
			} else {
				B = append([]*record.Line{mixed}, B...)
			}
			if got := e.judge("AC-1", bundle.KindChange, C, B); got != tc.want {
				t.Fatalf("mixed history got %q want %q", got, tc.want)
			}
		})
	}
}
