//go:build !windows

package spar

// AC-7 of the readme-first-run goal: when spar refuses a record folder that
// Git does not ignore, its message points to the Spar guide's setup section,
// and the refusal stays as it was: exit 65 before any reviewer starts, with
// nothing written. The test runs the spar command line on a synthetic
// repository that ignores nothing, as a project that has never run Spar.
//
// Failure messages about that property open with the violation marker that
// ha's go-test-json adapter reads. A git that cannot run, a usage error and
// any other failure to build the repository fail without the marker.

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadmeFirstRunAC7_RefusalPointsToSetup(t *testing.T) {
	const v = "refusal-without-setup"
	const guide = "https://github.com/jgoneit/jaekit/blob/main/guides/SPAR.en.md#set-up-a-project"
	if _, err := exec.LookPath("git"); err != nil {
		t.Fatalf("setup: git is not installed: %v", err)
	}
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	gitRun(t, dir, "init", "-q", repo)
	writeFile(t, filepath.Join(repo, "app.txt"), "one\n")
	gitRun(t, repo, "add", "-A")
	gitRun(t, repo, "commit", "-qm", "base")
	base := gitRun(t, repo, "rev-parse", "HEAD")
	writeFile(t, filepath.Join(repo, "app.txt"), "two\n")
	gitRun(t, repo, "commit", "-qam", "head")
	head := gitRun(t, repo, "rev-parse", "HEAD")
	criteria := writeFile(t, filepath.Join(dir, "SPEC.md"), "- **AC-1** two\n")
	config := writeFile(t, filepath.Join(dir, "spar.toml"), "[[reviewers]]\nname = \"host\"\npreset = \"native\"\n")
	out := filepath.Join(repo, "docs", "reviews", "demo", "01-candidate")

	var stdout, stderr bytes.Buffer
	code := Main("test", []string{"run", "--repo", repo, "--stage", "candidate", "--base", base, "--head", head,
		"--out", out, "--criteria", criteria}, strings.NewReader(""), &stdout, &stderr,
		env(map[string]string{"SPAR_CONFIG": config, "HOME": dir}))
	if code == ExitUsage {
		t.Fatalf("setup: spar run took the arguments as a usage error: %s", stderr.String())
	}
	if code != ExitRefused {
		t.Errorf("jaekit:violation=%s spar run into a folder Git does not ignore exited %d, want %d: %s", v, code, ExitRefused, stderr.String())
	}
	message := stderr.String()
	if !strings.Contains(message, "is not ignored by the Git repository") {
		t.Errorf("jaekit:violation=%s spar run did not refuse the folder as one Git does not ignore: %s", v, message)
	}
	if !strings.Contains(message, guide) {
		t.Errorf("jaekit:violation=%s the refusal does not point to %s: %s", v, guide, message)
	}
	if _, err := os.Stat(filepath.Join(repo, "docs")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("jaekit:violation=%s the refused run left %s (%v)", v, filepath.Join(repo, "docs"), err)
	}
	if strings.Contains(message, "host/prompt.md") || stdout.Len() > 0 {
		t.Errorf("jaekit:violation=%s the refused run reported work: %q %q", v, stdout.String(), message)
	}
}
