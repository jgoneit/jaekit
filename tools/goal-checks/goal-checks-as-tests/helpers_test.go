//go:build goalchecksastests

// Package goalchecksastests holds the goal-checks-as-tests goal's checks.
// Each check reads the checked tree's AGENTS.md (AC-6 also reads the listed
// public documents) and judges fixed tokens in it. No check reads Git, a base
// commit or history.
//
// A change condition's failure that observes its declared violation has a
// first line starting with "jaekit:violation=<id>: ". An input failure (an
// unreadable file, a file that is not valid UTF-8, a missing or repeated
// heading, a missing bullet that exists today, two bullets with the lead a
// check looks up) fails with t.Fatalf and no marker, so the go-test-json
// adapter never reads it as a pass or as the declared violation.
package goalchecksastests

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	agentsFile        = "AGENTS.md"
	sectionHeading    = "## Checks and review"
	reviewHeading     = "### Review findings"
	validationHeading = "## Validation"
)

// repoRoot walks up from the test's working directory to the directory that
// holds go.mod.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("cannot read the working directory: %v", err)
	}
	for {
		mod := filepath.Join(dir, "go.mod")
		info, err := os.Stat(mod)
		switch {
		case err == nil && info.Mode().IsRegular():
			return dir
		case err != nil && !errors.Is(err, fs.ErrNotExist):
			t.Fatalf("cannot inspect %s: %v", mod, err)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no go.mod in the working directory or above it")
		}
		dir = parent
	}
}

// readRepoFile reads a file named relative to the repository root. Every file
// a check reads comes through here. Bytes that are not valid UTF-8 are corrupt
// input: searched as text, they could hide a token and let a check pass.
func readRepoFile(t *testing.T, root, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("cannot read %s: %v", rel, err)
	}
	if !utf8.Valid(data) {
		t.Fatalf("%s is not valid UTF-8", rel)
	}
	return string(data)
}

// readAgents reads the checked tree's AGENTS.md.
func readAgents(t *testing.T) string {
	t.Helper()
	return readRepoFile(t, repoRoot(t), agentsFile)
}

// lineOffsets returns the byte offset of every line in text that equals line
// exactly. Lines end at "\n"; the last line may have no newline.
func lineOffsets(text, line string) []int {
	var offsets []int
	for off := 0; off < len(text); {
		end := len(text)
		if i := strings.IndexByte(text[off:], '\n'); i >= 0 {
			end = off + i
		}
		if text[off:end] == line {
			offsets = append(offsets, off)
		}
		off = end + 1
	}
	return offsets
}

// uniqueLine returns the offset of the one line in AGENTS.md that equals line.
// A missing or repeated line is corrupt input.
func uniqueLine(t *testing.T, text, line string) int {
	t.Helper()
	offsets := lineOffsets(text, line)
	if len(offsets) != 1 {
		t.Fatalf("%s: found %d lines equal to %q, want exactly one", agentsFile, len(offsets), line)
	}
	return offsets[0]
}

// checksSection returns the text from the "## Checks and review" line up to,
// not including, the "### Review findings" line.
func checksSection(t *testing.T, text string) string {
	t.Helper()
	start := uniqueLine(t, text, sectionHeading)
	end := uniqueLine(t, text, reviewHeading)
	if end < start {
		t.Fatalf("%s: %q comes before %q", agentsFile, reviewHeading, sectionHeading)
	}
	return text[start:end]
}

// reviewBlock returns the text from the "### Review findings" line up to, not
// including, the "## Validation" line.
func reviewBlock(t *testing.T, text string) string {
	t.Helper()
	start := uniqueLine(t, text, reviewHeading)
	end := uniqueLine(t, text, validationHeading)
	if end < start {
		t.Fatalf("%s: %q comes before %q", agentsFile, validationHeading, reviewHeading)
	}
	return text[start:end]
}

// bullet is one lead bullet of a section: its lead, the byte offset of its
// lead line in the section, and its text from that line through its last
// continuation line, newline included.
type bullet struct {
	lead string
	off  int
	text string
}

// bulletLead reports the lead of a line that starts with "- **<Lead>.**".
func bulletLead(line string) (string, bool) {
	rest, ok := strings.CutPrefix(line, "- **")
	if !ok {
		return "", false
	}
	i := strings.Index(rest, ".**")
	if i <= 0 {
		return "", false
	}
	return rest[:i], true
}

// sectionBullets returns a section's lead bullets in order. A bullet starts at
// a line that starts with "- **<Lead>.**" and takes in each following line
// that is indented and not blank. It ends before the first blank line, the
// first line without indentation (another bullet, a paragraph, a heading) or
// the end of the section, so text after a list is never part of its last
// bullet.
func sectionBullets(section string) []bullet {
	var out []bullet
	open := false
	for off := 0; off < len(section); {
		end, next := len(section), len(section)
		if i := strings.IndexByte(section[off:], '\n'); i >= 0 {
			end, next = off+i, off+i+1
		}
		line := section[off:end]
		if lead, ok := bulletLead(line); ok {
			out = append(out, bullet{lead: lead, off: off, text: section[off:next]})
			open = true
		} else if open && isContinuation(line) {
			b := &out[len(out)-1]
			b.text = section[b.off:next]
		} else {
			open = false
		}
		off = next
	}
	return out
}

// isContinuation reports whether line continues the bullet above it: it starts
// with a space or a tab and has other text.
func isContinuation(line string) bool {
	return (strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")) && strings.TrimSpace(line) != ""
}

// findBullet returns the one bullet with the given lead. Two bullets with the
// same lead make the lookup ambiguous, which is corrupt input.
func findBullet(t *testing.T, bullets []bullet, lead string) (string, bool) {
	t.Helper()
	found, n := "", 0
	for _, b := range bullets {
		if b.lead == lead {
			found, n = b.text, n+1
		}
	}
	if n > 1 {
		t.Fatalf("%s: %d bullets have the lead %q, want at most one", agentsFile, n, lead)
	}
	return found, n == 1
}

// hasLead reports whether any bullet has the given lead.
func hasLead(bullets []bullet, lead string) bool {
	for _, b := range bullets {
		if b.lead == lead {
			return true
		}
	}
	return false
}

// missingTokens returns the tokens that text does not contain, in order.
func missingTokens(text string, tokens []string) []string {
	var missing []string
	for _, token := range tokens {
		if !strings.Contains(text, token) {
			missing = append(missing, token)
		}
	}
	return missing
}

// violation fails the test with the declared violation marker on the first
// line. Call it once per test, with every observed problem in one message, so
// every failure message of the test carries the same marker.
func violation(t *testing.T, id string, problems []string) {
	t.Helper()
	t.Fatalf("jaekit:violation=%s: %s", id, strings.Join(problems, "; "))
}
