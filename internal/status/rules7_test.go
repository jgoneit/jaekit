package status

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/bundle"
	"github.com/jgoneit/jaekit/internal/goaldocs"
	"github.com/jgoneit/jaekit/internal/record"
)

func TestRules7InheritsRules6(t *testing.T) {
	if !Supported(RulesV7) || !Structured(RulesV7) || !Autonomous(RulesV7) ||
		!Reviewed(RulesV7) || !CompletionIntegrity(RulesV7) || !SelectiveBaseline(RulesV7) {
		t.Fatal("run-rules/7 must retain every run-rules/6 judgment")
	}
	for _, rules := range []string{RulesV1, RulesV2, RulesV3, RulesV4, RulesV5, RulesV6, "run-rules/8", ""} {
		if SelectiveBaseline(rules) {
			t.Errorf("selective baselines enabled under %q", rules)
		}
	}
	if Rules != RulesV3 {
		t.Fatalf("default rules changed to %s", Rules)
	}
}

func TestRules7BaselineRequirementDisplay(t *testing.T) {
	for _, rules := range []string{RulesV1, RulesV2, RulesV3, RulesV4, RulesV5, RulesV6, RulesV7} {
		t.Run(rules, func(t *testing.T) {
			in := rules6ComputationInput(t, rules)
			in.Goal.Criteria = []goaldocs.Criterion{{ID: "AC-1"}, {ID: "AC-2", Optional: true}, {ID: "AC-3"}, {ID: "AC-4"}}
			in.Bundle.Rows = []bundle.Row{
				{ID: "AC-1", Kind: bundle.KindChange, Command: "true"},
				{ID: "AC-2", Kind: bundle.KindChange, Command: "true"},
				{ID: "AC-3", Kind: bundle.KindMaintain, Command: "true"},
				{ID: "AC-4", Kind: bundle.KindManual},
			}
			var previousJSON []byte
			var previousMarkdown string
			for _, marked := range []string{"AC-1", "AC-2", ""} {
				for i := range in.Bundle.Rows {
					in.Bundle.Rows[i].BaselineRequired = in.Bundle.Rows[i].ID == marked
				}
				rep := rules6Compute(t, in)
				data, err := rep.JSON()
				if err != nil {
					t.Fatal(err)
				}
				var output struct {
					Criteria []map[string]any `json:"criteria"`
				}
				if err := json.Unmarshal(data, &output); err != nil {
					t.Fatal(err)
				}
				for _, c := range output.Criteria {
					value, exists := c["baseline_required"]
					if rules == RulesV7 && c["kind"] == "change" {
						want := c["id"] == marked
						if got, ok := value.(bool); !ok || got != want {
							t.Errorf("%s baseline_required = %#v; want %t", c["id"], value, want)
						}
					} else if exists {
						t.Errorf("%s unexpectedly has baseline_required = %#v", c["id"], value)
					}
				}
				markdown := rep.Markdown(in.Goal.Dir)
				if rules == RulesV7 {
					ids := marked
					if ids == "" {
						ids = "none"
					}
					if want := "- baseline required: " + ids + "\n"; !strings.Contains(markdown, want) {
						t.Errorf("status lacks %q: %s", want, markdown)
					}
				} else {
					if strings.Contains(markdown, "- baseline required:") {
						t.Errorf("%s unexpectedly shows baseline requirements", rules)
					}
					if previousJSON != nil && (!bytes.Equal(data, previousJSON) || markdown != previousMarkdown) {
						t.Errorf("%s output changed with the PLAN baseline marks", rules)
					}
				}
				previousJSON, previousMarkdown = data, markdown
			}
		})
	}
}

func TestRules7OptionalBaselineRetainsAdverseResults(t *testing.T) {
	for _, tc := range []struct {
		name      string
		marked    bool
		checks    []string
		baselines []string
		want      string
	}{
		{"marked missing", true, []string{"pass"}, nil, "baseline_missing"},
		{"unmarked missing", false, []string{"pass"}, nil, ""},
		{"current required", false, nil, nil, "criterion_missing"},
		{"current inconclusive", false, []string{"unknown"}, nil, "criterion_error"},
		{"marked positive", true, []string{"pass"}, []string{"fail_as_expected"}, ""},
		{"unmarked positive", false, []string{"pass"}, []string{"fail_as_expected"}, ""},
		{"unexpected pass persists", false, []string{"pass"}, []string{"unexpected_pass", "fail_as_expected"}, "baseline_unexpected_pass"},
		{"unintended violation persists", false, []string{"pass"}, []string{"fail", "fail_as_expected"}, "baseline_failed"},
		{"baseline inconclusive", false, []string{"pass"}, []string{"unknown"}, "baseline_error"},
		{"baseline retry succeeds", false, []string{"pass"}, []string{"error", "fail_as_expected"}, ""},
		{"baseline error cap", false, []string{"pass"}, []string{"unknown", "error", "fail_as_expected"}, "error_limit"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, line := strictJudgeFixture()
			e.rules = RulesV7
			e.in.Bundle.Rows[0].BaselineRequired = tc.marked
			var checks, baselines []*record.Line
			for _, result := range tc.checks {
				checks = append(checks, line("check", result))
			}
			for _, result := range tc.baselines {
				baselines = append(baselines, line("baseline", result))
			}
			if got := e.judge("AC-1", bundle.KindChange, checks, baselines); got != tc.want {
				t.Fatalf("reason = %q; want %q", got, tc.want)
			}
		})
	}
	// A current pass still needs its structured proof even without a baseline.
	e, line := strictJudgeFixture()
	e.rules = RulesV7
	check := line("check", "pass")
	check.Evidence = nil
	if got := e.judge("AC-1", bundle.KindChange, []*record.Line{check}, nil); got != "evidence_invalid" {
		t.Fatalf("unproved pass = %q; want evidence_invalid", got)
	}
}
