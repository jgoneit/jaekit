package plugins

// Property: document links and language pairs
// Relative links and anchors in the READMEs, guides, issue templates and
// image notes reach real files and headings, and each Korean/English pair
// links the same destinations and the other language's document.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"unicode"
)

// dlReading is one way of reading a document's links and headings. The two
// readings below differ in which lines open a code block, whether &amp; in a
// target reads as &, and which lines count as headings.
type dlReading struct {
	fence   func(line string) bool
	amp     bool
	anchors func(text string) map[string]bool
}

// dlStrict counts only headings outside code blocks; a fence is a line that
// starts with ``` after spaces.
var dlStrict = dlReading{
	fence:   func(line string) bool { return strings.HasPrefix(strings.TrimLeft(line, " "), "```") },
	amp:     true,
	anchors: dlAnchorsOutsideFences,
}

// dlAnyLine counts every heading line, inside code blocks too; a fence is a
// line that starts with ``` after any white space.
var dlAnyLine = dlReading{
	fence:   func(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "```") },
	amp:     false,
	anchors: dlAnchorsAnyLine,
}

var (
	dlLinkRe    = regexp.MustCompile(`\]\(([^)\s]+)\)|(?:href|src)="([^"]+)"`)
	dlSchemeRe  = regexp.MustCompile(`^[a-z]+:`)
	dlHeadingRe = regexp.MustCompile(`^(#{1,6}) +(.*)$`)
	dlAnyLineRe = regexp.MustCompile(`(?m)^#{1,6} (.+)$`)
)

// dlSlug is GitHub's heading anchor: lower case; letters, digits, marks,
// hyphens and underscores kept; spaces become hyphens.
func dlSlug(heading string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(heading) {
		switch {
		case unicode.IsLetter(r), unicode.IsDigit(r), unicode.IsMark(r), r == '-', r == '_':
			b.WriteRune(r)
		case r == ' ':
			b.WriteRune('-')
		}
	}
	return b.String()
}

// dlAnchorSet numbers repeated slugs the way GitHub does.
func dlAnchorSet(titles []string) map[string]bool {
	seen := map[string]int{}
	out := map[string]bool{}
	for _, h := range titles {
		s := dlSlug(h)
		if n := seen[s]; n > 0 {
			out[s+"-"+strconv.Itoa(n)] = true
		} else {
			out[s] = true
		}
		seen[s]++
	}
	return out
}

// dlAnchorsOutsideFences returns the anchors of the headings outside code
// blocks.
func dlAnchorsOutsideFences(text string) map[string]bool {
	var titles []string
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " "), "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if m := dlHeadingRe.FindStringSubmatch(line); m != nil {
			titles = append(titles, m[2])
		}
	}
	return dlAnchorSet(titles)
}

// dlAnchorsAnyLine returns the anchors of every heading line, code blocks
// included.
func dlAnchorsAnyLine(text string) map[string]bool {
	var titles []string
	for _, m := range dlAnyLineRe.FindAllStringSubmatch(text, -1) {
		titles = append(titles, m[1])
	}
	return dlAnchorSet(titles)
}

// dlTargets returns the link targets of text outside code blocks.
func dlTargets(text string, r dlReading) []string {
	var kept, out []string
	fenced := false
	for _, line := range strings.Split(text, "\n") {
		if r.fence(line) {
			fenced = !fenced
			continue
		}
		if !fenced {
			kept = append(kept, line)
		}
	}
	for _, m := range dlLinkRe.FindAllStringSubmatch(strings.Join(kept, "\n"), -1) {
		target := m[1]
		if target == "" {
			target = m[2]
		}
		if r.amp {
			target = strings.ReplaceAll(target, "&amp;", "&")
		}
		out = append(out, target)
	}
	return out
}

// dlCheckLinks reports each relative link of path whose file does not exist
// or whose anchor names no heading there.
func dlCheckLinks(t *testing.T, path string, r dlReading) {
	t.Helper()
	for _, target := range dlTargets(read(t, path), r) {
		if dlSchemeRe.MatchString(target) {
			continue
		}
		file, anchor, _ := strings.Cut(target, "#")
		dest := path
		if file != "" {
			dest = filepath.Join(filepath.Dir(path), file)
		}
		info, err := os.Stat(dest)
		if err != nil {
			t.Errorf("%s links %q, which does not exist", path, target)
			continue
		}
		if anchor != "" && !info.IsDir() && !r.anchors(read(t, dest))[anchor] {
			t.Errorf("%s links %q, but %s has no such heading", path, target, dest)
		}
	}
}

// Relative links in the READMEs, the install, usage and operations guides
// in both languages and the issue templates reach files that exist and
// headings outside code blocks there.
func TestDocLinksReachTargets(t *testing.T) {
	files := []string{"../README.md", "../README.ko.md", "../README.en.md", "../guides/INSTALL.md", "../guides/INSTALL.en.md",
		"../guides/USAGE.md", "../guides/USAGE.en.md", "../guides/OPERATIONS.md", "../guides/OPERATIONS.en.md"}
	forms, _ := filepath.Glob("../.github/ISSUE_TEMPLATE/*.yml")
	if len(forms) == 0 {
		t.Error("../.github/ISSUE_TEMPLATE has no issue templates")
	}
	for _, path := range append(files, forms...) {
		dlCheckLinks(t, path, dlStrict)
	}
}

// Relative links in each README, its install and operations guides, its
// usage guide when the README links it, and the image notes reach files that
// exist and heading lines there. This reading also counts heading lines
// inside code blocks.
func TestDocLinksReachTargetsAnyHeadingLine(t *testing.T) {
	for _, l := range []struct{ readme, install, operations, usage, notes string }{
		{"../README.ko.md", "../guides/INSTALL.md", "../guides/OPERATIONS.md", "../guides/USAGE.md", "../assets/readme/SOURCES.md"},
		{"../README.md", "../guides/INSTALL.en.md", "../guides/OPERATIONS.en.md", "../guides/USAGE.en.md", "../assets/readme/SOURCES.en.md"},
	} {
		for _, path := range []string{l.readme, l.install, l.operations, l.notes} {
			dlCheckLinks(t, path, dlAnyLine)
		}
		if strings.Contains(read(t, l.readme), strings.TrimPrefix(l.usage, "../")) {
			dlCheckLinks(t, l.usage, dlAnyLine)
		}
	}
}

// dlNormLinks returns the set of link files with anchors dropped and the
// language suffixes of documents and images removed.
func dlNormLinks(links []string) []string {
	set := map[string]bool{}
	for _, l := range links {
		file, _, _ := strings.Cut(l, "#")
		file = strings.Replace(file, ".en.md", ".md", 1)
		file = strings.Replace(file, ".ko.md", ".md", 1)
		for _, ext := range []string{".svg", ".png"} {
			file = strings.Replace(file, ".en"+ext, ext, 1)
			file = strings.Replace(file, ".ko"+ext, ext, 1)
		}
		set[file] = true
	}
	var out []string
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// The Korean and English READMEs, and the Korean and English install
// guides, link the same destinations apart from language suffixes. Each
// README links its install guide, and each install guide links the other
// language's.
func TestDocLinksLanguagePairsAgree(t *testing.T) {
	for _, pair := range []struct{ ko, en, sep string }{
		{"../README.ko.md", "../README.md", "\n"},
		{"../guides/INSTALL.md", "../guides/INSTALL.en.md", " "},
	} {
		ko := dlNormLinks(dlTargets(read(t, pair.ko), dlStrict))
		en := dlNormLinks(dlTargets(read(t, pair.en), dlStrict))
		if strings.Join(ko, pair.sep) != strings.Join(en, pair.sep) {
			t.Errorf("link destinations differ:\n%s: %v\n%s: %v", pair.ko, ko, pair.en, en)
		}
	}
	requireAll(t, "../README.ko.md", "(guides/INSTALL.md)")
	requireAll(t, "../README.md", "(guides/INSTALL.en.md)")
	requireAll(t, "../guides/INSTALL.md", "(INSTALL.en.md)")
	requireAll(t, "../guides/INSTALL.en.md", "(INSTALL.md)")
}
