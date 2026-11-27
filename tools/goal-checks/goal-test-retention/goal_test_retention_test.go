// Package goaltestretention holds the goal checks of the goal-test-retention
// goal. Seal Core links them through the go-test-json adapter declarations
// beside this file; they are deleted with this folder once the goal's
// completion is recorded.
package goaltestretention

import (
	"flag"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

var base = flag.String("base", "", "the goal's start commit, for AC-3")

const root = "../../.."

func TestGoalTestRetentionAC2(t *testing.T) {
	twins := []string{"cmd/ha/check_cost_twin_test.go", "cmd/ha/core_integrity_twin_test.go"}
	out, err := exec.Command("git", append([]string{"-C", root, "ls-files", "--"}, twins...)...).Output()
	if err != nil {
		t.Fatalf("cannot list tracked files: %v", err)
	}
	for _, path := range strings.Fields(string(out)) {
		t.Errorf("jaekit:violation=twin-tests-present %s is still tracked", path)
	}
}

// counted reports whether a changed path counts toward AC-3: a Go test file
// outside the goal checks, or a testdata file the change deleted.
func counted(path string, deleted map[string]bool) bool {
	if strings.HasPrefix(path, "tools/goal-checks/") {
		return false
	}
	return strings.HasSuffix(path, "_test.go") || (strings.Contains("/"+path, "/testdata/") && deleted[path])
}

func TestGoalTestRetentionAC3(t *testing.T) {
	if *base == "" {
		t.Fatal("AC-3 reads the goal's change and needs -base")
	}
	git := func(args ...string) string {
		out, err := exec.Command("git", append([]string{"-C", root}, args...)...).Output()
		if err != nil {
			t.Fatalf("cannot run git %s: %v", strings.Join(args, " "), err)
		}
		return string(out)
	}
	deleted := map[string]bool{}
	for _, path := range strings.Split(git("diff", "--no-renames", "--name-only", "--diff-filter=D", *base, "HEAD"), "\n") {
		if path != "" {
			deleted[path] = true
		}
	}
	added, removed := 0, 0
	for _, line := range strings.Split(strings.TrimSpace(git("diff", "--no-renames", "--numstat", *base, "HEAD")), "\n") {
		fields := strings.SplitN(line, "\t", 3)
		if len(fields) < 3 || fields[0] == "-" || !counted(fields[2], deleted) {
			continue
		}
		a, errA := strconv.Atoi(fields[0])
		d, errD := strconv.Atoi(fields[1])
		if errA != nil || errD != nil {
			t.Fatalf("cannot read numstat line %q", line)
		}
		added += a
		removed += d
	}
	if net := removed - added; net < 6000 {
		t.Errorf("jaekit:violation=goal-tests-short goal tests shrank by %d lines (+%d -%d); it must be 6000 or more", net, added, removed)
	}
}
