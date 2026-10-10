package gitx

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorktreeChangesPreservePathsAndKinds(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	put(t, dir, "old name", "tracked\n")
	put(t, dir, "deleted", "gone\n")
	put(t, dir, ".gitignore", "ignored\n")
	if err := os.Symlink("outside-before", filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	git(t, dir, "mv", "old name", "new\nname")
	if err := os.Remove(filepath.Join(dir, "deleted")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(t.TempDir(), filepath.Join(dir, "untracked link")); err != nil {
		t.Fatal(err)
	}
	put(t, dir, "ignored", "ignored\n")
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	changes, err := r.WorktreeChanges(nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 4 {
		t.Fatalf("changes = %#v", changes)
	}
	byPath := map[string]WorktreeChange{}
	for _, c := range changes {
		byPath[c.Path] = c
		if strings.Contains(c.String(), "\n") {
			t.Fatalf("unquoted path in diagnostic: %q", c.String())
		}
	}
	if got := byPath["new\nname"]; got.OriginalPath != "old name" || !got.Tracked {
		t.Fatalf("rename = %#v", got)
	}
	if got := byPath["untracked link"]; got.Tracked || got.Kind != "symlink" {
		t.Fatalf("untracked link = %#v", got)
	}
	if got := byPath["link"]; !got.Tracked || got.Kind != "symlink" {
		t.Fatalf("tracked link = %#v", got)
	}
	if clean, err := r.Clean([]string{"deleted", "link", "new\nname", "old name", "untracked link"}); err != nil || !clean {
		t.Fatalf("excluded/ignored paths changed clean semantics: %t, %v", clean, err)
	}
}

func TestWorktreeStatusParsingRejectsIncompleteOutput(t *testing.T) {
	for _, raw := range []string{"? file", "", "invalid\x00", "1 .M N...\x00", "2 R. N... 100644 100644 100644 a b R100 new\x00", "? ../outside\x00"} {
		if raw == "" {
			continue
		}
		if _, err := parseWorktreeChanges([]byte(raw)); err == nil {
			t.Errorf("accepted malformed status %q", raw)
		}
	}
	conflict := "u UU N... 100644 100644 100644 100644 a b c conflicted name\x00"
	changes, err := parseWorktreeChanges([]byte(conflict))
	if err != nil || len(changes) != 1 || changes[0].Path != "conflicted name" || changes[0].Status != "UU" {
		t.Fatalf("unmerged status = %#v, %v", changes, err)
	}
}
