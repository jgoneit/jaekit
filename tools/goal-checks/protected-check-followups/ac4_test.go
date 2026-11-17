//go:build protectedcheckfollowups

package protectedchecks

import (
	"path/filepath"
	"testing"
)

const guarantee = "execution is incorrectly a correctness guarantee"

// negativeScan adds the guarantee sentence to doc and returns the reporting
// AC-6 problems that name doc.
func negativeScan(t *testing.T, doc string) []string {
	t.Helper()
	root := docTree(t)
	faithful(t, "contract", root, "reporting", "AC-6")
	appendText(t, filepath.Join(root, doc), "Real execution proves correctness.")
	return withText(observe(t, "contract", root, "reporting", "AC-6").Problems, doc+": "+guarantee)
}

func TestProtectedAC4_EarlierGoalsScanned(t *testing.T) {
	const id = "earlier-goals-not-scanned"
	if got := negativeScan(t, earlierDoc); len(got) == 0 {
		t.Errorf("jaekit:violation=%s the guarantee sentence in %s is not reported", id, earlierDoc)
	}
}

// otherSealDocs lists SKILL.md and every reference but
// references/earlier-goals.md, as the checked tree has them.
func otherSealDocs(t *testing.T) []string {
	t.Helper()
	refs, err := filepath.Glob(filepath.Join(repoRoot(t), seal, "references", "*.md"))
	if err != nil || len(refs) == 0 {
		t.Fatalf("no Seal references: %v", err)
	}
	docs := []string{skillDoc}
	for _, ref := range refs {
		if doc := seal + "references/" + filepath.Base(ref); doc != earlierDoc {
			docs = append(docs, doc)
		}
	}
	return docs
}

func TestProtectedAC4_SkillStillScanned(t *testing.T) {
	const id = "skill-not-scanned"
	for _, doc := range otherSealDocs(t) {
		if got := negativeScan(t, doc); len(got) == 0 {
			t.Errorf("jaekit:violation=%s the guarantee sentence in %s is not reported", id, doc)
		}
	}
}
