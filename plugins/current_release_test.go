package plugins

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// currentReleaseClaim finds sentences that call a version the current
// release, in Korean or English, with optional emphasis, code or a comma
// between the words and the version. It is applied to whole documents, so a
// sentence wrapped across lines matches too.
var currentReleaseClaim = regexp.MustCompile(
	`(?i)(?:현재\s*(?:배포판|릴리스)(?:은|는|인|이|의)?|\bcurrent(?:ly)?\s+release[d]?(?:\s+is)?)[\s*` + "`" + `,]*v?(\d+\.\d+\.\d+)`)

// Public guidance names one current release. A sentence that introduces
// another version as the current release contradicts README and the guides
// even when every installation command is up to date.
func TestPublicGuidanceNamesOnlyTheCurrentRelease(t *testing.T) {
	var marketplace struct {
		Metadata struct {
			Version string `json:"version"`
		} `json:"metadata"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &marketplace)
	current := marketplace.Metadata.Version
	if current == "" {
		t.Fatal("../.claude-plugin/marketplace.json names no product release")
	}
	paths := []string{"../README.md", "../README.en.md"}
	guides, err := filepath.Glob("../guides/*.md")
	if err != nil || len(guides) == 0 {
		t.Fatalf("no public guides: %v", err)
	}
	paths = append(paths, guides...)
	err = filepath.WalkDir("../examples", func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range paths {
		text := read(t, path)
		for _, m := range currentReleaseClaim.FindAllStringSubmatchIndex(text, -1) {
			version := text[m[2]:m[3]]
			// A blank line ends the paragraph, and with it the sentence.
			if version == current || paragraphBreak.MatchString(text[m[0]:m[1]]) {
				continue
			}
			line := strings.Count(text[:m[0]], "\n") + 1
			t.Errorf("%s:%d names v%s as the current release; the current release is v%s",
				filepath.ToSlash(path), line, version, current)
		}
	}
}

var paragraphBreak = regexp.MustCompile(`\n[ \t]*\n`)
