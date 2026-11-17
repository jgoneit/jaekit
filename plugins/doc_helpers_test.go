package plugins

// Property: shared document helpers
// Helpers that more than one property file or a staying test reads documents with.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func visualRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

var (
	mdFence   = regexp.MustCompile("^\\s*(`{3,}|~{3,})")
	mdComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	mdItem    = regexp.MustCompile(`^\s*(?:[-*+]|\d+[.)])\s`)
)

// mdUnits splits the Markdown a reader sees, outside fenced code and HTML
// comments, into units: a list item together with its indented continuation
// lines is one unit, and every other non-blank line is its own unit. A nested
// list item starts a unit of its own.
func mdUnits(text string) []string {
	var outside []string
	fence := ""
	for _, line := range strings.Split(text, "\n") {
		m := mdFence.FindStringSubmatch(line)
		switch {
		case fence == "" && m != nil:
			fence = m[1]
		case fence != "":
			if m != nil && m[1][0] == fence[0] && len(m[1]) >= len(fence) {
				fence = ""
			}
		default:
			outside = append(outside, line)
			continue
		}
		outside = append(outside, "")
	}
	var units []string
	item := false
	for _, line := range strings.Split(mdComment.ReplaceAllString(strings.Join(outside, "\n"), ""), "\n") {
		switch {
		case strings.TrimSpace(line) == "":
		case mdItem.MatchString(line):
			units, item = append(units, line), true
		case item && (line[0] == ' ' || line[0] == '\t'):
			units[len(units)-1] += "\n" + line
		default:
			units, item = append(units, line), false
		}
	}
	return units
}

func visualSection(t *testing.T, path, heading string) string {
	t.Helper()
	text := visualRead(t, path)
	start := strings.Index(text, heading+"\n")
	if start < 0 {
		t.Fatalf("%s lacks %s", path, heading)
	}
	level := strings.IndexByte(heading, ' ')
	for _, m := range regexp.MustCompile(`(?m)^#{1,6} `).FindAllStringIndex(text[start+len(heading)+1:], -1) {
		if m[1]-m[0]-1 <= level {
			return text[start : start+len(heading)+1+m[0]]
		}
	}
	return text[start:]
}
