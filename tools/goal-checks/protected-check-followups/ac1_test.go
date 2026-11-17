//go:build protectedcheckfollowups

package protectedchecks

import (
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The user-resolved reasons table of the Seal reference: the rows AC-6 of
// the authoring producer compares with its seven reasons.
var userRow = regexp.MustCompile("^\\| `[a-z_]+` \\| user \\|")

const (
	reasonsChanged = "user-resolved reasons changed"
	removedRow     = "| `record_invalid` | user |"
	addedRow       = "| `invented_reason` | user | synthetic |"
)

// reasonsTable returns the table in text that holds the user rows, and the
// text without it.
func reasonsTable(t *testing.T, text, doc string) (table, rest string) {
	t.Helper()
	lines := strings.Split(text, "\n")
	first := slices.IndexFunc(lines, userRow.MatchString)
	if first < 0 {
		t.Fatalf("%s has no user-resolved reasons table", doc)
	}
	start, end := first, first
	for start > 0 && strings.HasPrefix(lines[start-1], "|") {
		start--
	}
	for end+1 < len(lines) && strings.HasPrefix(lines[end+1], "|") {
		end++
	}
	table = strings.Join(lines[start:end+1], "\n")
	rest = strings.Join(append(slices.Clone(lines[:start]), lines[end+1:]...), "\n")
	return table, rest
}

// moveTable moves the table, rows unchanged, from references/ha.md to the end of doc.
func moveTable(t *testing.T, root, doc string) {
	t.Helper()
	table, rest := reasonsTable(t, read(t, filepath.Join(root, haDoc)), haDoc)
	write(t, filepath.Join(root, haDoc), rest)
	appendText(t, filepath.Join(root, doc), table)
}

// changeTable removes the record_invalid row, adds an unknown reason row, or
// removes the whole table in doc.
func changeTable(t *testing.T, root, doc, change string) {
	t.Helper()
	path := filepath.Join(root, doc)
	table, _ := reasonsTable(t, read(t, path), doc)
	rows := strings.Split(table, "\n")
	var changed string
	switch change {
	case "row removed":
		i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, removedRow) })
		if i < 0 {
			t.Fatalf("%s has no row %q", doc, removedRow)
		}
		changed = strings.Join(slices.Delete(rows, i, i+1), "\n")
	case "row added":
		changed = table + "\n" + addedRow
	case "no table":
	default:
		t.Fatalf("unknown change %q", change)
	}
	replaceOnce(t, path, table, changed)
}

// splitTable moves the record_invalid row out of the table in
// references/ha.md into a table of its own, with the same header, at the end
// of doc: the reasons are then split across two documents.
func splitTable(t *testing.T, root, doc string) {
	t.Helper()
	path := filepath.Join(root, haDoc)
	table, _ := reasonsTable(t, read(t, path), haDoc)
	rows := strings.Split(table, "\n")
	i := slices.IndexFunc(rows, func(row string) bool { return strings.HasPrefix(row, removedRow) })
	if i < 2 || !strings.Contains(rows[1], "---") {
		t.Fatalf("%s: the reasons table has no header above %q", haDoc, removedRow)
	}
	moved := rows[0] + "\n" + rows[1] + "\n" + rows[i]
	replaceOnce(t, path, table, strings.Join(slices.Delete(rows, i, i+1), "\n"))
	appendText(t, filepath.Join(root, doc), moved)
}

func reasonProblems(t *testing.T, root string) []string {
	t.Helper()
	return withText(authoring(t, root, "AC-6").Problems, reasonsChanged)
}

func TestProtectedAC1_MovedReasonsTable(t *testing.T) {
	const id = "reasons-table-not-found"
	root := docTree(t)
	faithful(t, "authoring", root, "AC-6")
	before := reasonProblems(t, root)
	moveTable(t, root, bundleDoc)
	if after := reasonProblems(t, root); !slices.Equal(before, after) {
		t.Errorf("jaekit:violation=%s moving the table unchanged to %s changes the reason result from %q to %q", id, bundleDoc, before, after)
	}
	split := docTree(t)
	splitTable(t, split, bundleDoc)
	if after := reasonProblems(t, split); !slices.Equal(before, after) {
		t.Errorf("jaekit:violation=%s splitting the rows between %s and %s changes the reason result from %q to %q", id, haDoc, bundleDoc, before, after)
	}
}

func TestProtectedAC1_ReasonSetExact(t *testing.T) {
	const id = "reason-set-not-exact"
	faithful(t, "authoring", docTree(t), "AC-6")
	for _, c := range []struct{ change, token string }{
		{"row removed", "record_invalid"},
		{"row added", "invented_reason"},
		{"no table", reasonsChanged},
	} {
		inPlace := docTree(t)
		changeTable(t, inPlace, haDoc, c.change)
		moved := docTree(t)
		moveTable(t, moved, bundleDoc)
		changeTable(t, moved, bundleDoc, c.change)
		here, there := reasonProblems(t, inPlace), reasonProblems(t, moved)
		if len(withText(here, c.token)) == 0 {
			t.Errorf("jaekit:violation=%s %s in %s: the reason result %q does not follow the change", id, c.change, haDoc, here)
		}
		if !slices.Equal(here, there) {
			t.Errorf("jaekit:violation=%s %s: the reason result is %q with the table in %s and %q in %s", id, c.change, here, haDoc, there, bundleDoc)
		}
	}
}

func TestProtectedAC1_EarlierGoalsOnlyTable(t *testing.T) {
	const id = "earlier-goals-table-accepted"
	faithful(t, "authoring", docTree(t), "AC-6")
	only := docTree(t)
	moveTable(t, only, earlierDoc)
	none := docTree(t)
	changeTable(t, none, haDoc, "no table")
	if got, want := reasonProblems(t, only), reasonProblems(t, none); len(got) == 0 || !slices.Equal(got, want) {
		t.Errorf("jaekit:violation=%s a table only in %s gives %q, not the result with no table %q", id, earlierDoc, got, want)
	}
}
