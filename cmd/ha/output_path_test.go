package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The run-record contract states what output_path holds: a repository-relative
// path without absolute paths, the same form in a linked worktree with .git
// standing for that working tree's git directory, and older absolute paths
// read as written.
func TestRunRecordStatesOutputPath(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Join("..", "..", "contracts", "run-record.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		"local output location (a repository-relative path, for example `.git/ha/runs/<goal-key>/7.log`)",
		"`output_path` never holds an absolute path.",
		"The format is the same in a linked worktree, where `.git` means that working tree's git directory.",
		"Absolute paths left by an earlier `ha` are read as they are, not rewritten.",
	} {
		if !strings.Contains(string(doc), s) {
			t.Errorf("run-record.md lacks %q", s)
		}
	}
}
