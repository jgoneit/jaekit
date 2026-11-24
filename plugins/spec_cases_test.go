package plugins

import (
	"strings"
	"testing"
)

// the quality case index lists the long-session case.
func TestSpecLongSessionCase(t *testing.T) {
	if !strings.Contains(read(t, "../evals/spec-cases/README.md"), "[long-session-release](long-session-release.md)") {
		t.Error("evals/spec-cases/README.md does not list long-session-release")
	}
}
