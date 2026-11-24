package plugins

// Property: Spec skill rules
// Checks the header lines of the Spec skill's SPEC templates.

import (
	"strings"
	"testing"
)

const (
	spkDecisionHead = "## Decisions and reasons"
	spkMarkerSlot   = "<[사용자], [제안] or [기존]>"
	spkStatusKey    = "Status:"
	spkFormatKey    = "Criteria-Format:"
	spkAssuranceKey = "Assurance:"
)

var spkTemplates = []string{
	"spec/skills/spec/assets/templates/backend-feature.md",
	"spec/skills/spec/assets/templates/cross-module-feature.md",
	"spec/skills/spec/assets/templates/product.md",
}

func spkLevel(line string) int {
	n := len(line) - len(strings.TrimLeft(line, "#"))
	if n == 0 || n > 6 || n >= len(line) || line[n] != ' ' {
		return 0
	}
	return n
}

func spkReport(t *testing.T, missing []string) {
	t.Helper()
	for _, m := range missing {
		t.Error(m)
	}
}

// spkHeader returns the value of a header line before the first level-two
// heading, or "".
func spkHeader(text, key string) string {
	for _, line := range strings.Split(text, "\n") {
		if spkLevel(line) == 2 {
			break
		}
		if strings.HasPrefix(line, key) {
			return strings.TrimSpace(strings.TrimPrefix(line, key))
		}
	}
	return ""
}

func spkTemplateHeaders(missing *[]string, where, text string) bool {
	ok := true
	for _, key := range []string{spkStatusKey, spkFormatKey, spkAssuranceKey} {
		if spkHeader(text, key) == "" {
			*missing = append(*missing, where+" lacks its "+key+" header")
			ok = false
		}
	}
	return ok
}

// Each marked SPEC template carries the Status:, Criteria-Format: and
// Assurance: header lines.
func TestSpecSkillTemplateHeaders(t *testing.T) {
	var missing []string
	for _, file := range spkTemplates {
		spkTemplateHeaders(&missing, file, read(t, file))
	}
	spkReport(t, missing)
}

func TestSpecSkillHelperTemplateHeaders(t *testing.T) {
	line := "- " + spkMarkerSlot + " <decision and reason> Source: <source>"
	for _, c := range []struct {
		name, headers string
		want          bool
	}{
		{"standard", "Status: Draft\nCriteria-Format: nested/1\nAssurance: standard", true},
		{"strict", "Status: Draft\nCriteria-Format: nested/1\nAssurance: strict", true},
		{"missing status", "Criteria-Format: nested/1\nAssurance: strict", false},
		{"missing format", "Status: Draft\nAssurance: strict", false},
		{"missing assurance", "Status: Draft\nCriteria-Format: nested/1", false},
		{"empty assurance", "Status: Draft\nCriteria-Format: nested/1\nAssurance:", false},
	} {
		t.Run(c.name, func(t *testing.T) {
			text := "# synthetic\n\n" + c.headers + "\n\n" + spkDecisionHead + "\n\n" + line
			var missing []string
			if got := spkTemplateHeaders(&missing, c.name, text); got != c.want || (len(missing) == 0) != c.want {
				t.Errorf("template headers = %t, want %t; missing = %q", got, c.want, missing)
			}
		})
	}
}
