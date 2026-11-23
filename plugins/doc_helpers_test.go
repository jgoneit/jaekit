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
