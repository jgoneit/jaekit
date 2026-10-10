// Package gitx calls the user's git CLI with explicit argv. Using the git CLI
// keeps worktree, pathspec, and ignore semantics identical to the user's git.
// Nothing here writes the user's working tree or index.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Repo is a git working tree.
type Repo struct {
	Root   string // absolute top-level directory
	GitDir string // absolute git directory of this working tree
}

// Deleted is the blob value of a path that does not exist at a revision.
const Deleted = "deleted"

func command(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// GIT_OPTIONAL_LOCKS=0 keeps `git status` from refreshing the index.
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	return cmd
}

func run(dir string, args ...string) ([]byte, error) {
	cmd := command(dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return out, fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Open finds the working tree that contains dir.
func Open(dir string) (*Repo, error) {
	out, err := run(dir, "rev-parse", "--show-toplevel", "--absolute-git-dir")
	if err != nil {
		return nil, fmt.Errorf("not inside a git working tree: %w", err)
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "\n")
	if len(parts) != 2 {
		return nil, fmt.Errorf("unexpected git rev-parse output: %q", out)
	}
	return &Repo{Root: filepath.Clean(parts[0]), GitDir: filepath.Clean(parts[1])}, nil
}

func (r *Repo) run(args ...string) ([]byte, error) { return run(r.Root, args...) }

// Head returns the commit id of HEAD.
func (r *Repo) Head() (string, error) {
	out, err := r.run("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", errors.New("the repository has no commit yet; commit before running ha")
	}
	return strings.TrimSpace(string(out)), nil
}

// Pathspec builds `. :(exclude,literal)p ...` for the given excluded paths.
func Pathspec(excludes []string) []string {
	out := []string{"."}
	for _, p := range excludes {
		out = append(out, ":(exclude,literal)"+p)
	}
	return out
}

// Clean reports whether the working tree has no tracked modifications and no
// untracked files outside excludes. Ignored files do not count.
func (r *Repo) Clean(excludes []string) (bool, error) {
	args := append([]string{"status", "--porcelain=v1", "-z", "--untracked-files=all", "--ignore-submodules=none", "--"}, Pathspec(excludes)...)
	out, err := r.run(args...)
	if err != nil {
		return false, err
	}
	return len(out) == 0, nil
}

// Same reports whether commits a and b have no differences outside excludes.
func (r *Repo) Same(a, b string, excludes []string) (bool, error) {
	if a == b {
		return true, nil
	}
	args := append([]string{"diff", "--quiet", "--no-ext-diff", "--no-textconv", a, b, "--"}, Pathspec(excludes)...)
	cmd := command(r.Root, args...)
	err := cmd.Run()
	if err == nil {
		return true, nil
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) && ee.ExitCode() == 1 {
		return false, nil
	}
	return false, fmt.Errorf("git diff --quiet %s %s: %w", a, b, err)
}

// Change is one path that differs between two commits.
type Change struct {
	Status string // A, M, D, or T
	Path   string
}

// Changes lists paths that differ between a and b outside excludes, without
// rename detection (a rename is a delete plus an add).
func (r *Repo) Changes(a, b string, excludes []string) ([]Change, error) {
	args := append([]string{"diff", "--name-status", "-z", "--no-renames", "--no-ext-diff", a, b, "--"}, Pathspec(excludes)...)
	out, err := r.run(args...)
	if err != nil {
		return nil, err
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	var changes []Change
	for i := 0; i+1 < len(fields); i += 2 {
		changes = append(changes, Change{Status: fields[i][:1], Path: fields[i+1]})
	}
	return changes, nil
}

// Blob returns the blob id of path at rev, or Deleted when it does not exist.
func (r *Repo) Blob(rev, path string) (string, error) {
	cmd := command(r.Root, "rev-parse", "--verify", "--quiet", rev+":"+path)
	out, err := cmd.Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return Deleted, nil
		}
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Cat returns the content of path at rev. ok is false when it does not exist.
func (r *Repo) Cat(rev, path string) ([]byte, bool, error) {
	blob, err := r.Blob(rev, path)
	if err != nil || blob == Deleted {
		return nil, false, err
	}
	out, err := r.run("cat-file", "blob", blob)
	if err != nil {
		return nil, false, err
	}
	return out, true, nil
}

// Mode returns the git file mode of path at rev (for example 100755), or ""
// when it does not exist.
func (r *Repo) Mode(rev, path string) (string, error) {
	out, err := r.run("ls-tree", "-z", rev, "--", path)
	if err != nil {
		return "", err
	}
	if mode, _, ok := strings.Cut(string(out), " "); ok {
		return mode, nil
	}
	return "", nil
}

// AddWorktree creates a detached worktree of commit at dir.
func (r *Repo) AddWorktree(dir, commit string) error {
	_, err := r.run("worktree", "add", "--detach", "--quiet", dir, commit)
	return err
}

// RemoveWorktree removes a worktree created by AddWorktree.
func (r *Repo) RemoveWorktree(dir string) error {
	_, err := r.run("worktree", "remove", "--force", dir)
	if err != nil {
		os.RemoveAll(dir)
		_, _ = r.run("worktree", "prune")
	}
	return err
}
