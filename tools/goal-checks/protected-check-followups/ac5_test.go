//go:build protectedcheckfollowups

package protectedchecks

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
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

// The fixed text of tools/verify.py by which the full verification runs the
// protected route's test: the default group "all" runs verify_docs, and one
// loop of verify_docs discovers test_*.py in each listed tools/ directory,
// tools/goal-checks among them.
const (
	groupDefault = `parser.add_argument("group", nargs="?", choices=("all", "go", "docs"), default="all")`
	docsDispatch = "        if args.group in (\"all\", \"docs\"):\n            verify_docs(env)\n"
	goalChecks   = `("Goal check regressions", "goal-checks")`
)

var discoverLoop = regexp.MustCompile(`for label, directory in \(((?:[^()]|\([^()]*\))*)\):\s*` +
	`run\(label, \[sys\.executable, "-m", "unittest", "discover", "-s", f"tools/\{directory\}",\s*"-p", "test_\*\.py"\], env\)`)

// fullVerificationRuns reports whether tools/verify.py, as fixed text, runs
// the tests of tools/goal-checks in its default group.
func fullVerificationRuns(t *testing.T) bool {
	t.Helper()
	verify := read(t, filepath.Join(repoRoot(t), "tools", "verify.py"))
	start := strings.Index(verify, "\ndef verify_docs(")
	if start < 0 || !strings.Contains(verify, groupDefault) || !strings.Contains(verify, docsDispatch) {
		return false
	}
	body := verify[start+1:]
	if end := strings.Index(body, "\ndef "); end >= 0 {
		body = body[:end]
	}
	loop := discoverLoop.FindStringSubmatch(body)
	return loop != nil && strings.Contains(loop[1], goalChecks)
}

func guarded(t *testing.T, id, what, old, new string) {
	t.Helper()
	if !fullVerificationRuns(t) {
		t.Errorf("jaekit:violation=%s the full verification does not run CurrentTree: tools/verify.py's default group has no discover loop over tools/goal-checks test_*.py", id)
	}
	if r := currentTree(t, producerCopy(t, "", "")); !r.ok {
		t.Fatalf("control: CurrentTree fails on the unchanged copy:\n%s", tail(r.output, 30))
	}
	if r := currentTree(t, producerCopy(t, old, new)); !caught(t, r, "CurrentTree with the changed lookup") {
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
