package status

import (
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/bundle"
)

func TestRules6Inheritance(t *testing.T) {
	for _, tc := range []struct {
		rules                                                  string
		supported, structured, autonomous, reviewed, integrity bool
	}{
		{RulesV1, true, false, false, false, false},
		{RulesV2, true, false, false, false, false},
		{RulesV3, true, true, false, false, false},
		{RulesV4, true, true, true, false, false},
		{RulesV5, true, true, true, true, false},
		{RulesV6, true, true, true, true, true},
		{"run-rules/7", false, false, false, false, false},
		{"", false, false, false, false, false},
	} {
		t.Run(tc.rules, func(t *testing.T) {
			got := [5]bool{Supported(tc.rules), Structured(tc.rules), Autonomous(tc.rules), Reviewed(tc.rules), CompletionIntegrity(tc.rules)}
			want := [5]bool{tc.supported, tc.structured, tc.autonomous, tc.reviewed, tc.integrity}
			if got != want {
				t.Errorf("supported, structured, autonomous, reviewed, completion integrity = %v; want %v", got, want)
			}
		})
	}
	if Rules != RulesV3 {
		t.Errorf("default rules = %s; want %s", Rules, RulesV3)
	}
	if message := (&UnsupportedRulesError{Rules: "run-rules/7"}).Error(); !strings.Contains(message, `"run-rules/6"`) {
		t.Errorf("unsupported-rule diagnostic omits run-rules/6: %s", message)
	}
}

// Bundle loading and status must agree on the inherited digest semantics:
// changing a result contract or command limit makes earlier evidence stale.
func TestRules6CommandDigestInheritance(t *testing.T) {
	row := bundle.Row{Kind: bundle.KindChange, Command: "go test ./...",
		EvidencePath: "checks/result.json", EvidenceDigest: "first-contract", Limit: time.Minute}
	initial := row.CommandDigestFor(RulesV6)
	if want := row.CommandDigestFor(RulesV5); initial != want {
		t.Fatalf("run-rules/6 command digest = %s; run-rules/5 = %s", initial, want)
	}
	for _, tc := range []struct {
		name   string
		change func(*bundle.Row)
	}{
		{"contract content", func(r *bundle.Row) { r.EvidenceDigest = "second-contract" }},
		{"contract path", func(r *bundle.Row) { r.EvidencePath = "checks/other.json" }},
		{"command limit", func(r *bundle.Row) { r.Limit = 2 * time.Minute }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := row
			tc.change(&changed)
			got := changed.CommandDigestFor(RulesV6)
			if got == initial || got != changed.CommandDigestFor(RulesV5) {
				t.Errorf("changed %s must invalidate evidence under both rule versions", tc.name)
			}
		})
	}
}
