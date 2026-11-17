//go:build protectedcheckfollowups

package protectedchecks

import (
	"path/filepath"
	"strconv"
	"testing"
)

// The lines of tools/contract-checks/check.py the two representative lookup
// changes replace. A change to these lines needs this check updated.
const (
	earlierGoalsLine = `EARLIER_GOALS_ONLY = {SEAL + "references/earlier-goals.md"}`
	clauseJoinLine   = `    return "\n".join((root / doc).read_text(encoding="utf-8") for doc in names if (root / doc).exists())`
)

// producerCopy copies the documents, the contract producers and the
// protected route's test into a new tree, and changes check.py there.
func producerCopy(t *testing.T, old, new string) string {
	t.Helper()
	root := repoRoot(t)
	tree := docTree(t)
	copyDir(t, filepath.Join(root, "tools", "contract-checks"), filepath.Join(tree, "tools", "contract-checks"))
	copyFile(t, filepath.Join(root, "tools", "goal-checks", "test_authoring_check.py"),
		filepath.Join(tree, "tools", "goal-checks", "test_authoring_check.py"))
	if old != "" {
		replaceOnce(t, filepath.Join(tree, "tools", "contract-checks", "check.py"), old, new)
	}
	return tree
}

// currentTree runs the protected route's CurrentTree tests of the copy.
func currentTree(t *testing.T, tree string) unittestResult {
	t.Helper()
	return runUnittest(t, filepath.Join(tree, "tools", "goal-checks"), nil, "test_authoring_check.CurrentTree")
}

func guarded(t *testing.T, id, what, old, new string) {
	t.Helper()
	if r := currentTree(t, producerCopy(t, "", "")); !r.ok {
		t.Fatalf("control: CurrentTree fails on the unchanged copy:\n%s", tail(r.output, 30))
	}
	if r := currentTree(t, producerCopy(t, old, new)); r.ok {
		t.Errorf("jaekit:violation=%s CurrentTree passes when the lookup %s", id, what)
	}
}

func TestProtectedAC5_EarlierGoalsLookupGuarded(t *testing.T) {
	guarded(t, "earlier-goals-lookup-unguarded", "reads references/earlier-goals.md for every clause",
		earlierGoalsLine, `EARLIER_GOALS_ONLY = set()`)
}

func TestProtectedAC5_CheckedTreeLookupGuarded(t *testing.T) {
	checkout := strconv.Quote(repoRoot(t))
	mixed := "    checkout = Path(" + checkout + ")\n" +
		`    return "\n".join(((root if doc == name else checkout) / doc).read_text(encoding="utf-8") ` +
		`for doc in names if ((root if doc == name else checkout) / doc).exists())`
	guarded(t, "checked-tree-lookup-unguarded",
		"reads the clause's own document from the checked tree and the skill's other documents from the checkout",
		clauseJoinLine, mixed)
}
