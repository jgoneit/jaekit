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

// ac6Files lists every *.md directly under guides/ and contracts/, then the
// root README and CLAUDE files. An unreadable or empty directory is an input
// failure: an empty list would pass without reading anything.
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
	return append(files, "README.md", "README.ko.md", "README.en.md", "CLAUDE.md")
}

// TestGoalChecksAC6_NoOtherLifetimeRule keeps the separate lifetime rule out
// of the public guides, contracts, READMEs and CLAUDE.md.
func TestGoalChecksAC6_NoOtherLifetimeRule(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range ac6Files(t, root) {
		text := readRepoFile(t, root, rel)
		for _, phrase := range ac6Phrases {
			if strings.Contains(text, phrase) {
				t.Errorf("%s contains %q", rel, phrase)
			}
		}
	}
}
