package evidence

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeCleanCRLFStillRequiresCommittedBytes(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	root := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=synthetic", "-c", "user.email=synthetic@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + filepath.Join(root, "no-hooks")}, args...)...)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q")
	git("config", "core.autocrlf", "true")
	if err := os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("producer.txt text\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(root, "producer.txt")
	lf := []byte("committed\ncontent\n")
	if err := os.WriteFile(p, lf, 0o600); err != nil {
		t.Fatal(err)
	}
	git("add", ".gitattributes", "producer.txt")
	git("commit", "-qm", "synthetic line endings")
	crlf := bytes.ReplaceAll(lf, []byte("\n"), []byte("\r\n"))
	if err := os.WriteFile(p, crlf, 0o600); err != nil {
		t.Fatal(err)
	}
	// Refresh the index from the converted worktree bytes. Git's clean filter
	// retains the same committed LF blob although the file now contains CRLF.
	git("add", "producer.txt")
	if out := git("status", "--porcelain"); out != "" {
		t.Fatalf("fixture must be Git-clean: %s", out)
	}
	before := git("config", "--local", "--list")
	if _, err := ReadInput(root, "producer.txt"); err == nil || !strings.Contains(err.Error(), "CRLF") {
		t.Fatalf("line-ending mismatch was accepted or unexplained: %v", err)
	}
	after, err := os.ReadFile(p)
	if err != nil || !bytes.Equal(after, crlf) || git("config", "--local", "--list") != before {
		t.Fatalf("read mutated bytes/config: %v", err)
	}
	if err := os.WriteFile(p, lf, 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := ReadInput(root, "producer.txt"); err != nil || !bytes.Equal(data, lf) {
		t.Fatalf("committed bytes refused: %v", err)
	}
}
