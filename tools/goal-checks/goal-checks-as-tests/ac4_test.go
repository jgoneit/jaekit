//go:build goalchecksastests

package goalchecksastests

import (
	"strings"
	"testing"
)

// TestGoalChecksAC4_NarrowedRecords observes that the Input failures bullet
// is narrowed to separate producers and the Observation boundary bullet to
// open-ended inputs, and that neither keeps its old, unnarrowed opening. Both
// bullets exist today, so a missing one is corrupt input, not the violation.
func TestGoalChecksAC4_NarrowedRecords(t *testing.T) {
	bullets := sectionBullets(checksSection(t, readAgents(t)))
	inputFailures, ok := findBullet(t, bullets, "Input failures")
	if !ok {
		t.Fatalf(`%s: no bullet with the lead "Input failures"`, agentsFile)
	}
	boundary, ok := findBullet(t, bullets, "Observation boundary")
	if !ok {
		t.Fatalf(`%s: no bullet with the lead "Observation boundary"`, agentsFile)
	}

	var problems []string
	if !strings.Contains(inputFailures, "separate producer") {
		problems = append(problems, `the Input failures bullet lacks "separate producer"`)
	}
	if !strings.Contains(boundary, "open-ended") {
		problems = append(problems, `the Observation boundary bullet lacks "open-ended"`)
	}
	if strings.Contains(boundary, "For each check a goal adds or changes") {
		problems = append(problems, `the Observation boundary bullet contains "For each check a goal adds or changes"`)
	}
	if strings.Contains(inputFailures, "For each producer of a change condition") {
		problems = append(problems, `the Input failures bullet contains "For each producer of a change condition"`)
	}
	if len(problems) > 0 {
		violation(t, "records-not-narrowed", problems)
	}
}
