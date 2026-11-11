package spar

import (
	"strings"
	"testing"
)

func TestStandpointReportReasonSurvivesOtherInvalidFields(t *testing.T) {
	// Both run and record display only the first five validation problems.
	// Even a reply missing every other field must identify its absent or
	// malformed standpoint report in that displayed reason.
	for _, report := range []any{nil, "not an object", map[string]any{"checked": "yes", "detail": " "}} {
		result := map[string]any{}
		if report != nil {
			result["standpoint_report"] = report
		}
		problems := validateResult(result, "1234567890abcdef", Angles, strPtr("Check record compatibility"))
		if reason := strings.Join(first(problems, 5), "; "); !strings.Contains(reason, "standpoint_report") {
			t.Errorf("standpoint report %v: displayed reason omits standpoint_report: %s", report, reason)
		}
	}
}
