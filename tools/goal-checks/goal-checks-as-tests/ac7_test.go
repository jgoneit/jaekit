//go:build goalchecksastests

package goalchecksastests

import (
	"fmt"
	"strings"
	"testing"
)

// ac7WordLimit is one more than the most words the rewritten section may have.
// The section had 448 whitespace-separated words before the change.
const ac7WordLimit = 448

// TestGoalChecksAC7_Shorter observes that the section has fewer
// whitespace-separated words than before the change.
func TestGoalChecksAC7_Shorter(t *testing.T) {
	section := checksSection(t, readAgents(t))
	if words := len(strings.Fields(section)); words >= ac7WordLimit {
		violation(t, "section-not-shorter", []string{
			fmt.Sprintf("the section has %d whitespace-separated words, want fewer than %d", words, ac7WordLimit),
		})
	}
}
