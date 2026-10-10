package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"testing"
)

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func put(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, rel)
	os.MkdirAll(filepath.Dir(p), 0o755)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCleanSameChangesAndWorktree(t *testing.T) {
	dir := t.TempDir()
	git(t, dir, "init", "-q", "-b", "main")
	put(t, dir, ".gitignore", "ignored.txt\n")
	put(t, dir, "src/a.txt", "a\n")
	put(t, dir, "g/PLAN.md", "plan\n")
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "base")
	r, err := Open(filepath.Join(dir, "src"))
	if err != nil {
		t.Fatal(err)
	}
	base, _ := r.Head()
	excl := []string{"g/PLAN.md"}

	put(t, dir, "ignored.txt", "x")
	put(t, dir, "g/PLAN.md", "changed\n")
	if ok, _ := r.Clean(excl); !ok {
		t.Fatal("ignored files and excluded bundle docs must not make the tree dirty")
	}
	put(t, dir, "src/new.txt", "n\n")
	if ok, _ := r.Clean(excl); ok {
		t.Fatal("an untracked file must make the tree dirty")
	}
	git(t, dir, "add", "-A")
	git(t, dir, "commit", "-q", "-m", "next")
	head, _ := r.Head()
	if same, _ := r.Same(base, head, excl); same {
		t.Fatal("a new source file is a difference")
	}
	ch, err := r.Changes(base, head, excl)
	if err != nil || !reflect.DeepEqual(ch, []Change{{Status: "A", Path: "src/new.txt"}}) {
		t.Fatalf("changes = %#v, %v", ch, err)
	}
	if b, _ := r.Blob(base, "src/new.txt"); b != Deleted {
		t.Fatalf("blob at base = %q", b)
	}
	wt := filepath.Join(r.GitDir, "ha", "wt-test")
	if err := r.AddWorktree(wt, base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(wt, "src", "new.txt")); !os.IsNotExist(err) {
		t.Fatal("the base worktree must not contain later files")
	}
	if err := r.RemoveWorktree(wt); err != nil {
		t.Fatal(err)
	}
	if out := git(t, dir, "status", "--porcelain"); out != "" {
		t.Fatalf("worktree handling changed the user's tree: %q", out)
	}
}
