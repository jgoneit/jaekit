//go:build protectedcheckfollowups

package protectedchecks

import (
	"path/filepath"
	"strings"
	"testing"
)

const checkInputs = "## Check inputs"

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

func TestProtectedAC3_BundleSentenceOnce(t *testing.T) {
	const id = "bundle-problem-not-once"
	got := onceIn(t, bundleDoc, "AC-10", localPath, "/Users/e", "Checks may read /Users/example/notes for context.")
	if !once(got, bundleDoc) {
		t.Errorf("jaekit:violation=%s a local path in %s %s is not reported once under that document: %q", id, bundleDoc, checkInputs, got)
	}
}

func TestProtectedAC3_SkillSentenceOnce(t *testing.T) {
	const id = "skill-problem-not-once"
	got := onceIn(t, skillDoc, "AC-10", localPath, "/Users/e", "Checks may read /Users/example/notes for context.")
	if !once(got, skillDoc) {
		t.Errorf("jaekit:violation=%s a local path in %s %s is not reported once under that document: %q", id, skillDoc, checkInputs, got)
	}
}

func TestProtectedAC3_UserStepOnce(t *testing.T) {
	const id = "user-step-not-once"
	got := onceIn(t, bundleDoc, "AC-6", userStep, "Ask the user to approve each such check", "Ask the user to approve each such check.")
	if !once(got, bundleDoc) {
		t.Errorf("jaekit:violation=%s a user step in %s %s is not reported once under that document: %q", id, bundleDoc, checkInputs, got)
	}
}
