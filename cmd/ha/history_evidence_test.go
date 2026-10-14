package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A file reference is missing only when the path does not exist; a parent
// that cannot be traversed or resolved leaves its existence unknown.
func TestFileEvidenceSeparatesMissingFromUnreadable(t *testing.T) {
	root := t.TempDir()
	write := func(p, s string) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(s), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("docs/a.md", "a\n")
	write("locked/sub/b.md", "b\n")
	if err := os.Symlink("loop", filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	version := fileVersion(root)
	want := map[string]string{
		"docs/a.md":       "read",
		"docs/none.md":    "missing",
		"none/deeper.md":  "missing",
		"docs/a.md/x.md":  "missing",
		"loop/c.md":       "unreadable",
		"locked/sub/b.md": "unreadable",
	}
	if os.Geteuid() == 0 {
		delete(want, "locked/sub/b.md")
	}
	if err := os.Chmod(filepath.Join(root, "locked"), 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(filepath.Join(root, "locked"), 0o755)
	for p, state := range want {
		if _, got := version(p); got != state {
			t.Errorf("%s: %s, want %s", p, got, state)
		}
	}
}
