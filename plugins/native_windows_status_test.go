package plugins

// Property: native Windows status in the root READMEs
// Each root README marks native Windows as not supported with one fixed status
// label and a link to its Windows guide, together on one line or in one list
// item. Only that marker is required; the wording around it stays free.

import (
	"regexp"
	"strings"
	"testing"
)

// The English README and its Korean translation each carry the native Windows
// marker outside fenced code and HTML comments. A link to the guide may add a
// heading anchor. README.en.md is only a pointer to the other two and carries
// no platform status.
func TestReadmesMarkNativeWindowsUnsupported(t *testing.T) {
	for _, c := range []struct{ readme, label, guide string }{
		{"../README.md", "**Not supported**", "guides/WINDOWS.en.md"},
		{"../README.ko.md", "**지원하지 않음**", "guides/WINDOWS.md"},
	} {
		link := regexp.MustCompile(`\]\(` + regexp.QuoteMeta(c.guide) + `(?:#[^)\s]*)?\)`)
		found := false
		for _, unit := range mdUnits(read(t, c.readme)) {
			if strings.Contains(unit, c.label) && link.MatchString(unit) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("%s has no line or list item with both the label %s and a link to %s", c.readme, c.label, c.guide)
		}
	}
}
