package plugins

import (
	"regexp"
	"testing"
)

// The examples index links both examples and names the release that README
// and the guides call current.
func TestExamplesIndexDescribesCurrentRelease(t *testing.T) {
	const path = "../examples/README.md"
	text := read(t, path)
	requireIn(t, path, text, "(empty-input/)", "(check-result/)", rel011Tag)
	for _, m := range regexp.MustCompile(`v\d+\.\d+\.\d+`).FindAllString(text, -1) {
		if m != rel011Tag {
			t.Errorf("%s names %s instead of the current release %s", path, m, rel011Tag)
		}
	}
}
