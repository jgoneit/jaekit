//go:build goalcheckcleanup

// Package cleanupchecks holds the acceptance checks of the goal that removed
// finished goals' check folders from tools/goal-checks and the plugins tests
// named after goals. Under the "Goal check lifetime" rule of AGENTS.md this
// folder is deleted in the change after that goal merges.
//
// Every check judges the checked tree, a temporary copy of it, or the inputs
// its condition names. A check that cannot observe its property fails with a
// "setup:" message, never with a declared violation and never as a pass.
package cleanupchecks

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// removedFolders are the subfolders of tools/goal-checks that AC-1 removes.
// Only the names are listed; paths are assembled when a check runs.
var removedFolders = []string{
	"autonomous-recovery-gates", "bundle-readability", "check-cost", "core-completion-integrity",
	"final-candidate-evidence", "goal-cost-summary", "ha-report-doctor", "ha-review-gate",
	"ha-start-budget-check", "readme-demo-user-steps", "readme-first-run", "readme-flow-visuals",
	"release-0-1-7", "release-0-1-8", "review-findings-report", "review-roundtrip-rules",
	"review-summary", "review-summary-followups", "reviewer-standpoints", "run-timing-records",
	"runner-adapters", "seal-questions-up-front", "seal-slim-review-tiers", "spar",
	"spar-agent-model-choice", "spar-agy-readonly", "spar-cap-report", "spar-check-path-questions",
	"spar-input-hardening", "spar-onboarding", "spar-packet-decisions", "spar-reviewer-waiver",
	"spar-smallest-fix", "spec-agent-handoff", "status-delta-first", "verify-feedback-loop",
	"verify-nested-independence", "windows-ci-parallel-shells",
}

// keptFolders are the only subfolders of tools/goal-checks that remain.
var keptFolders = []string{
	"pages-site", "release-0-1-3-followups", "release-0-1-3-review-fixes", "review-findings",
	"windows-entry-name-refusal",
}

// goalCheckRoot is tools/goal-checks as a slash path.
var goalCheckRoot = strings.Join([]string{"tools", "goal-checks"}, "/")

// repoRoot is the repository root; go test runs a package in its directory.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("setup: repository root: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
		t.Fatalf("setup: no go.mod at the repository root %s: %v", dir, err)
	}
	return dir
}

// gitCommand runs git in dir with a fixed locale and no optional locks.
func gitCommand(dir string, args ...string) *exec.Cmd {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "LC_ALL=C")
	return cmd
}

// gitOutput runs git and fails the test as a setup failure when git fails.
func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := gitCommand(dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("setup: git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return string(out)
}

// gitStatus runs git and returns its exit code; 0 and 1 are answers, any
// other result is a setup failure.
func gitStatus(t *testing.T, dir string, args ...string) int {
	t.Helper()
	cmd := gitCommand(dir, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) && exit.ExitCode() == 1 {
		return 1
	}
	t.Fatalf("setup: git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	return -1
}

// trackedFile is one entry of the index of the checked tree.
type trackedFile struct {
	path string
	mode string // 100644, 100755 or 120000
}

// trackedFiles lists the tracked files of the checked tree. ha checks only a
// clean tree, so the index equals the tree being checked.
func trackedFiles(t *testing.T, root string) []trackedFile {
	t.Helper()
	out := gitOutput(t, root, "ls-files", "-s", "-z")
	var files []trackedFile
	for _, entry := range strings.Split(out, "\x00") {
		if entry == "" {
			continue
		}
		meta, path, ok := strings.Cut(entry, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			t.Fatalf("setup: unreadable index entry %q", entry)
		}
		files = append(files, trackedFile{path: path, mode: fields[0]})
	}
	if len(files) == 0 {
		t.Fatalf("setup: the checked tree has no tracked files")
	}
	return files
}

// readFile reads a repository file; a missing or unreadable file is a setup
// failure.
func readFile(t *testing.T, root, rel string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("setup: read %s: %v", rel, err)
	}
	return data
}

// digest is the hex sha256 of data.
func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// compareFiles compares tracked files under a prefix (or exactly the listed
// files when prefix is empty) with expected content and executable bits.
func compareFiles(t *testing.T, root string, files []trackedFile, keep func(string) bool,
	expected map[string]expectedFile) []string {
	t.Helper()
	var problems []string
	seen := map[string]bool{}
	for _, file := range files {
		if !keep(file.path) {
			continue
		}
		seen[file.path] = true
		want, ok := expected[file.path]
		if !ok {
			problems = append(problems, "unexpected file "+file.path)
			continue
		}
		if file.mode == "120000" {
			problems = append(problems, file.path+" is a symbolic link")
			continue
		}
		if got := digest(readFile(t, root, file.path)); got != want.sha256 {
			problems = append(problems, file.path+" content changed")
		}
		if (file.mode == "100755") != want.executable {
			problems = append(problems, file.path+" mode changed to "+file.mode)
		}
	}
	for path := range expected {
		if !seen[path] {
			problems = append(problems, "missing file "+path)
		}
	}
	return problems
}

// section returns the Markdown section that starts with the exact heading
// line, up to the next heading of the same or a higher level.
func section(text, heading string) (string, bool) {
	level := len(heading) - len(strings.TrimLeft(heading, "#"))
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, " \r") != heading {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			trimmed := strings.TrimLeft(lines[j], "#")
			hashes := len(lines[j]) - len(trimmed)
			if hashes > 0 && hashes <= level && strings.HasPrefix(trimmed, " ") {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n"), true
	}
	return "", false
}

// joinLines reports problems after the opening line of a failure message.
func joinLines(first string, problems []string) string {
	return first + "\n" + strings.Join(problems, "\n")
}
