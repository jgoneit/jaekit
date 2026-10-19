package spar

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// git runs the user's git CLI in a repository and returns its standard output.
func git(repo string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-c", "core.quotepath=off", "-C", repo}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return out, nil
}

func gitOK(repo string, args ...string) bool {
	_, err := git(repo, args...)
	return err == nil
}

// repoRoot returns the top folder of the repository holding dir.
func repoRoot(dir string) (string, error) {
	out, err := git(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// resolveCommit returns the full commit ID a revision names.
func resolveCommit(repo, rev string) (string, error) {
	if strings.HasPrefix(rev, "-") {
		return "", fmt.Errorf("%q is not a revision", rev)
	}
	out, err := git(repo, "rev-parse", "--verify", "--quiet", "--end-of-options", rev+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%q does not name a commit in %s", rev, repo)
	}
	return strings.TrimSpace(string(out)), nil
}

func treeOf(repo, commit string) (string, error) {
	out, err := git(repo, "rev-parse", "--verify", commit+"^{tree}")
	return strings.TrimSpace(string(out)), err
}

func isAncestor(repo, older, newer string) bool {
	return gitOK(repo, "merge-base", "--is-ancestor", older, newer)
}

// diff is the change between two commits without commit messages.
func diff(repo, base, head string) ([]byte, error) {
	return git(repo, "diff", "--no-color", "--no-ext-diff", "--no-textconv", "-M", base, head, "--")
}

// numstat returns the lines each path gained between two commits.
func numstat(repo, base, head string) (map[string]int, error) {
	out, err := git(repo, "diff", "--numstat", "-z", "--no-renames", base, head, "--")
	if err != nil {
		return nil, err
	}
	added := map[string]int{}
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		fields := strings.SplitN(rec, "\t", 3)
		if len(fields) != 3 {
			continue
		}
		n, err := strconv.Atoi(fields[0])
		if err != nil {
			n = 0 // "-" for binary files
		}
		added[fields[2]] += n
	}
	return added, nil
}

// fileAt reports whether a commit holds a file at a repository path.
func fileAt(repo, commit, path string) ([]byte, bool) {
	out, err := git(repo, "cat-file", "blob", commit+":"+path)
	return out, err == nil
}

type treeEntry struct {
	mode, kind, object, path string
}

func listTree(repo, commit string) ([]treeEntry, error) {
	out, err := git(repo, "ls-tree", "-r", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	var entries []treeEntry
	for _, rec := range strings.Split(string(out), "\x00") {
		if rec == "" {
			continue
		}
		meta, path, ok := strings.Cut(rec, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			return nil, fmt.Errorf("git ls-tree: unexpected line %q", rec)
		}
		entries = append(entries, treeEntry{fields[0], fields[1], fields[2], path})
	}
	return entries, nil
}

// exportTree writes the tracked files of a commit under dest, without
// history, ignored files or attribute filters: each file is its blob.
func exportTree(repo, commit, dest string) error {
	entries, err := listTree(repo, commit)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	cmd := exec.Command("git", "-C", repo, "cat-file", "--batch")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		return err
	}
	var blobs []treeEntry
	for _, e := range entries {
		if e.kind == "blob" {
			blobs = append(blobs, e)
		}
	}
	go func() {
		w := bufio.NewWriter(stdin)
		for _, e := range blobs {
			fmt.Fprintln(w, e.object)
		}
		w.Flush()
		stdin.Close()
	}()
	r := bufio.NewReader(stdout)
	var werr error
	for _, e := range blobs {
		header, err := r.ReadString('\n')
		if err != nil {
			werr = fmt.Errorf("git cat-file: %v", err)
			break
		}
		fields := strings.Fields(header)
		if len(fields) != 3 || fields[1] != "blob" {
			werr = fmt.Errorf("git cat-file: unexpected header %q", strings.TrimSpace(header))
			break
		}
		size, err := strconv.ParseInt(fields[2], 10, 64)
		if err != nil {
			werr = err
			break
		}
		data := make([]byte, size)
		if _, err := io.ReadFull(r, data); err != nil {
			werr = err
			break
		}
		if _, err := r.Discard(1); err != nil {
			werr = err
			break
		}
		if werr = writeEntry(dest, e, data); werr != nil {
			break
		}
	}
	if werr != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return werr
	}
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("git cat-file: %s", strings.TrimSpace(stderr.String()))
	}
	return nil
}

func writeEntry(dest string, e treeEntry, data []byte) error {
	rel := filepath.FromSlash(e.path)
	if !filepath.IsLocal(rel) {
		return fmt.Errorf("tracked path %q leaves the workspace", e.path)
	}
	path := filepath.Join(dest, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	switch e.mode {
	case "120000":
		// A symbolic link becomes a file holding its target, the blob
		// itself: links, alone or chained, could lead outside the workspace.
		return os.WriteFile(path, data, 0o644)
	case "100755":
		return os.WriteFile(path, data, 0o755)
	default:
		return os.WriteFile(path, data, 0o644)
	}
}

// ignoredPath reports whether Git ignores a path inside the repository and
// tracks nothing under it.
func ignoredPath(repo, rel string) (bool, error) {
	rel = filepath.ToSlash(rel)
	out, err := git(repo, "ls-files", "-z", "--", rel)
	if err != nil {
		return false, err
	}
	if len(out) > 0 {
		return false, nil
	}
	for _, probe := range []string{rel, rel + "/"} {
		cmd := exec.Command("git", "-C", repo, "check-ignore", "-q", "--", probe)
		err := cmd.Run()
		if err == nil {
			return true, nil
		}
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return false, fmt.Errorf("git check-ignore %s: %v", probe, err)
		}
	}
	return false, nil
}
