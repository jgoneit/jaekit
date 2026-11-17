//go:build protectedcheckfollowups

// Package protectedchecks holds the goal checks of protected-check-followups.
// They copy the checked tree's documents or tools into temporary trees,
// change them, and run the checked tree's own producers and tests on them.
// They read no Git history. Input failures (a missing file, unreadable
// output, a tool that cannot start) end in t.Fatalf without a violation
// marker, so the adapter never reads them as a violation or a pass.
package protectedchecks

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	seal       = "plugins/seal/skills/seal/"
	skillDoc   = seal + "SKILL.md"
	bundleDoc  = seal + "references/bundle.md"
	haDoc      = seal + "references/ha.md"
	earlierDoc = seal + "references/earlier-goals.md"
)

// repoRoot is the checked tree: go test runs in this package's directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatalf("checked tree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("checked tree has no go.mod: %v", err)
	}
	return root
}

// childEnv is this process's environment without ha's evidence variables, so
// no producer a check starts writes a result for the check itself, plus extra.
func childEnv(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "HA_EVIDENCE_") {
			env = append(env, kv)
		}
	}
	return append(env, extra...)
}

func tool(t *testing.T, name string) string {
	t.Helper()
	path, err := exec.LookPath(name)
	if err != nil {
		t.Fatalf("%s cannot start: %v", name, err)
	}
	return path
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
	if err := os.WriteFile(dst, data, 0o644); err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
}

// copyDir copies regular files and symbolic links under src; caches are left out.
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case d.IsDir() && d.Name() == "__pycache__":
			return filepath.SkipDir
		case d.IsDir():
			return os.MkdirAll(target, 0o755)
		case d.Type()&fs.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		case d.Type().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.WriteFile(target, data, info.Mode().Perm())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
}

// docTree copies the documents the contract producers read into a new
// temporary tree.
func docTree(t *testing.T) string {
	t.Helper()
	root, dst := repoRoot(t), t.TempDir()
	for _, dir := range []string{"plugins", "contracts", "guides", ".claude-plugin"} {
		copyDir(t, filepath.Join(root, dir), filepath.Join(dst, dir))
	}
	for _, name := range []string{"internal/status/status.go", "cmd/ha/baseline.go", "README.md", "README.ko.md", "README.en.md", "AGENTS.md"} {
		copyFile(t, filepath.Join(root, name), filepath.Join(dst, name))
	}
	return dst
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// replaceOnce replaces the single occurrence of old in the file; anything
// else is an input failure, since the change would not be made.
func replaceOnce(t *testing.T, path, old, new string) {
	t.Helper()
	text := read(t, path)
	if n := strings.Count(text, old); n != 1 {
		t.Fatalf("%s: expected one %q, found %d", path, old, n)
	}
	write(t, path, strings.Replace(text, old, new, 1))
}

func appendText(t *testing.T, path, text string) {
	t.Helper()
	write(t, path, strings.TrimRight(read(t, path), "\n")+"\n\n"+text+"\n")
}

// insertAfterHeading puts a paragraph right under the first line that is
// exactly heading.
func insertAfterHeading(t *testing.T, path, heading, paragraph string) {
	t.Helper()
	lines := strings.Split(read(t, path), "\n")
	for i, line := range lines {
		if line == heading {
			out := append(append(append([]string{}, lines[:i+1]...), "", paragraph), lines[i+1:]...)
			write(t, path, strings.Join(out, "\n"))
			return
		}
	}
	t.Fatalf("%s has no heading %q", path, heading)
}

type observation struct {
	Status   string   `json:"status"`
	Problems []string `json:"problems"`
	Text     *string  `json:"text"`
}

// observe runs observe.py of this directory, which imports the checked tree's
// contract producers and observes the given tree.
func observe(t *testing.T, args ...string) observation {
	t.Helper()
	cmd := exec.Command(tool(t, "python3"), append([]string{"-I", "observe.py"}, args...)...)
	cmd.Env = childEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("observe.py %v: %v\n%s", args, err, stderr.String())
	}
	var obs observation
	if err := json.Unmarshal(stdout.Bytes(), &obs); err != nil {
		t.Fatalf("observe.py %v printed no observation: %v\n%s", args, err, stdout.String())
	}
	return obs
}

// authoring observes one criterion of the Seal check-authoring producer.
func authoring(t *testing.T, root, criterion string) observation {
	t.Helper()
	return observe(t, "authoring", root, criterion)
}

func withText(problems []string, parts ...string) []string {
	var out []string
	for _, p := range problems {
		hit := true
		for _, part := range parts {
			hit = hit && strings.Contains(p, part)
		}
		if hit {
			out = append(out, p)
		}
	}
	return out
}

// faithful fails the test as an input failure unless the unchanged copy gives
// the checked tree's own observation (args after the root, for observe): a
// copy that misses a document the producer reads would make every later
// observation meaningless.
func faithful(t *testing.T, mode, root string, args ...string) {
	t.Helper()
	got := observe(t, append([]string{mode, root}, args...)...)
	want := observe(t, append([]string{mode, repoRoot(t)}, args...)...)
	if got.Status != want.Status || !slices.Equal(got.Problems, want.Problems) {
		t.Fatalf("control: %s %v on the unchanged copy is %s %q, on the checked tree %s %q",
			mode, args, got.Status, got.Problems, want.Status, want.Problems)
	}
}

type unittestResult struct {
	ran    int
	ok     bool
	output string
}

var ranLine = regexp.MustCompile(`(?m)^Ran (\d+) tests? in `)

// runUnittest runs python3 -m unittest with the given arguments in dir. A
// result it cannot read, or a module that did not load (unittest's
// _FailedTest), is an input failure.
func runUnittest(t *testing.T, dir string, env []string, args ...string) unittestResult {
	t.Helper()
	cmd := exec.Command(tool(t, "python3"), append([]string{"-m", "unittest", "-v"}, args...)...)
	cmd.Dir = dir
	cmd.Env = childEnv(env...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if exit, ok := err.(*exec.ExitError); err != nil && (!ok || exit.ExitCode() < 0) {
		t.Fatalf("unittest %v did not finish: %v\n%s", args, err, text)
	}
	m := ranLine.FindStringSubmatch(text)
	if m == nil || strings.Contains(text, "_FailedTest") {
		t.Fatalf("unittest %v gave no readable result:\n%s", args, text)
	}
	ran, _ := strconv.Atoi(m[1])
	lines := strings.Split(strings.TrimSpace(text), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	ok := err == nil && (last == "OK" || strings.HasPrefix(last, "OK ("))
	if !ok && !strings.HasPrefix(last, "FAILED (") {
		t.Fatalf("unittest %v ended with an unknown line %q:\n%s", args, last, text)
	}
	if ran == 0 {
		t.Fatalf("unittest %v ran no test:\n%s", args, text)
	}
	return unittestResult{ran: ran, ok: ok, output: text}
}

func tail(text string, n int) string {
	lines := strings.Split(strings.TrimRight(text, "\n"), "\n")
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	return strings.Join(lines, "\n")
}
