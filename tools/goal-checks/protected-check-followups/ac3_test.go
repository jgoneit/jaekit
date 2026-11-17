//go:build protectedcheckfollowups

package protectedchecks

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const (
	checkInputs = "## Check inputs"
	// A table the producer reads row by row: its heading in references/bundle.md and one row label.
	reviewHeading = "## REVIEW.md"
	reviewRow     = "| 검증 방법과 환경 |"
)

// onceIn puts paragraph under doc's "## Check inputs" and returns the
// criterion's problems that carry label and marker, a part of the paragraph
// the problem quotes.
func onceIn(t *testing.T, doc, criterion, label, marker, paragraph string) []string {
	t.Helper()
	root := docTree(t)
	faithful(t, "authoring", root, criterion)
	insertAfterHeading(t, filepath.Join(root, doc), checkInputs, paragraph)
	return withText(authoring(t, root, criterion).Problems, label, marker)
}

// once reports whether problems is exactly one problem named after doc.
func once(problems []string, doc string) bool {
	return len(problems) == 1 && strings.HasPrefix(problems[0], doc+":")
}

// named returns the problems named after doc.
func named(problems []string, doc string) []string {
	var out []string
	for _, p := range problems {
		if strings.HasPrefix(p, doc+":") {
			out = append(out, p)
		}
	}
	return out
}

// sealLineInSection adds a local path to the line naming Seal 0.1.14 inside
// references/bundle.md's "## Check inputs", which the producer reads both as
// part of the section and as a Seal 0.1.14 line.
func sealLineInSection(t *testing.T) []string {
	t.Helper()
	root := docTree(t)
	faithful(t, "authoring", root, "AC-10")
	path := filepath.Join(root, bundleDoc)
	lines := strings.Split(read(t, path), "\n")
	start := slices.Index(lines, checkInputs)
	if start < 0 {
		t.Fatalf("%s has no %s", bundleDoc, checkInputs)
	}
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "## ") {
			end = i
			break
		}
	}
	i := slices.IndexFunc(lines[start:end], func(line string) bool { return strings.Contains(line, "Seal 0.1.14") })
	if i < 0 {
		t.Fatalf("%s %s has no line naming Seal 0.1.14", bundleDoc, checkInputs)
	}
	lines[start+i] += " Notes live in /Users/example/notes."
	write(t, path, strings.Join(lines, "\n"))
	return withText(authoring(t, root, "AC-10").Problems, localPath, "/Users/e")
}

func TestProtectedAC3_BundleSentenceOnce(t *testing.T) {
	const id = "bundle-problem-not-once"
	got := onceIn(t, bundleDoc, "AC-10", localPath, "/Users/e", "Checks may read /Users/example/notes for context.")
	if !once(got, bundleDoc) {
		t.Errorf("jaekit:violation=%s a local path in %s %s is not reported once under that document: %q", id, bundleDoc, checkInputs, got)
	}
	if got := sealLineInSection(t); !once(got, bundleDoc) {
		t.Errorf("jaekit:violation=%s a local path in the Seal 0.1.14 line of %s %s is not reported once under that document: %q", id, bundleDoc, checkInputs, got)
	}
	// A table row the producer reads, naming Seal 0.1.14, and a separate line
	// of the same document with the same text are two sentences.
	rows := docTree(t)
	path := filepath.Join(rows, bundleDoc)
	lines := strings.Split(read(t, path), "\n")
	start := slices.Index(lines, reviewHeading)
	i := -1
	if start >= 0 {
		i = slices.IndexFunc(lines[start:], func(line string) bool { return strings.HasPrefix(line, reviewRow) })
	}
	if i < 0 {
		t.Fatalf("%s has no %q row under %s", bundleDoc, reviewRow, reviewHeading)
	}
	lines[start+i] += " Seal 0.1.14 notes: /Users/example/rows"
	write(t, path, strings.Join(lines, "\n"))
	appendText(t, path, "## Notes on earlier checks\n\n"+lines[start+i])
	if got := withText(authoring(t, rows, "AC-10").Problems, localPath, "/Users/e"); len(got) != 2 || len(named(got, bundleDoc)) != 2 {
		t.Errorf("jaekit:violation=%s a row naming Seal 0.1.14 and the same line elsewhere in %s are not reported once each: %q", id, bundleDoc, got)
	}
	// Two sentences of one document whose problems read the same are two problems.
	two := docTree(t)
	insertAfterHeading(t, filepath.Join(two, bundleDoc), checkInputs,
		"Checks may read /Users/example/notes for context.\n\nOlder runs kept /Users/example/archive instead.")
	if got := withText(authoring(t, two, "AC-10").Problems, localPath, "/Users/e"); len(got) != 2 || len(named(got, bundleDoc)) != 2 {
		t.Errorf("jaekit:violation=%s two sentences with a local path in %s %s are not reported once each under that document: %q", id, bundleDoc, checkInputs, got)
	}
}

func TestProtectedAC3_SkillSentenceOnce(t *testing.T) {
	const id = "skill-problem-not-once"
	const sentence = "Checks may read /Users/example/notes for context."
	got := onceIn(t, skillDoc, "AC-10", localPath, "/Users/e", sentence)
	if !once(got, skillDoc) {
		t.Errorf("jaekit:violation=%s a local path in %s %s is not reported once under that document: %q", id, skillDoc, checkInputs, got)
	}
	root := docTree(t)
	insertAfterHeading(t, filepath.Join(root, skillDoc), checkInputs, sentence)
	insertAfterHeading(t, filepath.Join(root, bundleDoc), checkInputs, sentence)
	both := withText(authoring(t, root, "AC-10").Problems, localPath, "/Users/e")
	if len(both) != 2 || !once(named(both, skillDoc), skillDoc) || !once(named(both, bundleDoc), bundleDoc) {
		t.Errorf("jaekit:violation=%s the same sentence in %s and %s is not reported once under each: %q", id, skillDoc, bundleDoc, both)
	}
	// A row under the same table heading in SKILL.md is read there and named after it,
	// although references/bundle.md has a row with the same label.
	row := docTree(t)
	appendText(t, filepath.Join(row, skillDoc), reviewHeading+"\n\n| 항목 | 내용 |\n| --- | --- |\n"+reviewRow+" Checks may read /Users/example/notes. |")
	if got := withText(authoring(t, row, "AC-10").Problems, localPath, "/Users/e"); !once(got, skillDoc) {
		t.Errorf("jaekit:violation=%s a row with a local path under %s in %s is not reported once under that document: %q", id, reviewHeading, skillDoc, got)
	}
}

func TestProtectedAC3_UserStepOnce(t *testing.T) {
	const id = "user-step-not-once"
	got := onceIn(t, bundleDoc, "AC-6", userStep, "Ask the user to approve each such check", "Ask the user to approve each such check.")
	if !once(got, bundleDoc) {
		t.Errorf("jaekit:violation=%s a user step in %s %s is not reported once under that document: %q", id, bundleDoc, checkInputs, got)
	}
}
