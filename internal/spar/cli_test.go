package spar

import (
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The usage text's spar run synopsis names every option spar run takes and
// no other, and lists --boundary in the form of --rules and --subject.
func TestUsageRunSynopsisNamesEveryOption(t *testing.T) {
	start, end := strings.Index(usageText, "  spar run "), strings.Index(usageText, "  spar record ")
	if start < 0 || end < start {
		t.Fatalf("the usage text has no spar run synopsis before spar record:\n%s", usageText)
	}
	synopsis := usageText[start:end]
	listed := map[string]bool{}
	for _, m := range regexp.MustCompile(`--([a-z][a-z-]*)`).FindAllStringSubmatch(synopsis, -1) {
		listed[m[1]] = true
	}
	taken := append(append([]string{}, runValued...), runBare...)
	for _, name := range taken {
		if !listed[name] {
			t.Errorf("the spar run synopsis does not list --%s", name)
		}
	}
	for name := range listed {
		if !slices.Contains(taken, name) {
			t.Errorf("the spar run synopsis lists --%s, which spar run does not take", name)
		}
	}
	if !strings.Contains(synopsis, " [--boundary FILE]... ") {
		t.Errorf("the spar run synopsis does not list [--boundary FILE]...:\n%s", synopsis)
	}
}
