//go:build goalchecksastests

package goalchecksastests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ac6Phrases are the phrases of the separate lifetime rule.
var ac6Phrases = []string{"Goal check lifetime", "goal check lifetime", "deleted in the next change"}

const (
	ac6Template    = ".github/pull_request_template.md"
	ac6ReviewFocus = "## 리뷰 포인트 / Review focus"
)

// ac6FocusTokens name the narrowed records in the template's Review focus
// comment, in English and in Korean.
var ac6FocusTokens = []string{"open-ended", "separate producer", "열린 입력", "생산자"}

// ac6Files lists every *.md directly under guides/ and contracts/, then the
// root README and CLAUDE files and the pull request template. An unreadable or
// empty directory is an input failure: an empty list would pass without
// reading anything.
func ac6Files(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{"guides", "contracts"} {
		entries, err := os.ReadDir(filepath.Join(root, dir))
		if err != nil {
			t.Fatalf("cannot list %s/: %v", dir, err)
		}
		n := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				files = append(files, dir+"/"+e.Name())
				n++
			}
		}
		if n == 0 {
			t.Fatalf("%s/ has no *.md files", dir)
		}
	}
	return append(files, "README.md", "README.ko.md", "README.en.md", "CLAUDE.md", ac6Template)
}

// ac6FocusComment returns the HTML comment that directly follows the one
// Review focus heading of the template. A missing or repeated heading, or a
// heading without a closed comment after it, is corrupt input: the template
// has exactly one of each today.
func ac6FocusComment(t *testing.T, text string) string {
	t.Helper()
	offsets := lineOffsets(text, ac6ReviewFocus)
	if len(offsets) != 1 {
		t.Fatalf("%s: found %d lines equal to %q, want exactly one", ac6Template, len(offsets), ac6ReviewFocus)
	}
	after := strings.TrimLeft(text[offsets[0]+len(ac6ReviewFocus):], " \t\r\n")
	body, ok := strings.CutPrefix(after, "<!--")
	if !ok {
		t.Fatalf("%s: no comment directly after %q", ac6Template, ac6ReviewFocus)
	}
	end := strings.Index(body, "-->")
	if end < 0 {
		t.Fatalf("%s: the comment after %q is not closed", ac6Template, ac6ReviewFocus)
	}
	return body[:end]
}

// TestGoalChecksAC6_NoOtherLifetimeRule keeps the separate lifetime rule out
// of the public guides, contracts, READMEs, CLAUDE.md and the pull request
// template, and keeps the narrowed records in the template's Review focus
// comment.
func TestGoalChecksAC6_NoOtherLifetimeRule(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range ac6Files(t, root) {
		text := readRepoFile(t, root, rel)
		for _, phrase := range ac6Phrases {
			if strings.Contains(text, phrase) {
				t.Errorf("%s contains %q", rel, phrase)
			}
		}
		if rel == ac6Template {
			for _, token := range missingTokens(ac6FocusComment(t, text), ac6FocusTokens) {
				t.Errorf("%s: the %q comment lacks %q", rel, ac6ReviewFocus, token)
			}
		}
	}
}
