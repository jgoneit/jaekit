//go:build jaekit_compat_018

package main

// AC-1 of the goal core-completion-integrity: a Core that does not know
// run-rules/6 (the installed ha 0.1.8) neither computes nor records a goal
// that this tree's ha started under it, and exits 66. This file builds only
// with the tag jaekit_compat_018; ordinary go test and tools/verify.py leave
// it out. The ha 0.1.8 executable is the first `ha` on PATH whose --version
// prints "ha 0.1.8"; without one the setup fails.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ciHa018 finds the installed ha 0.1.8.
func ciHa018(t *testing.T) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "ha")
		if info, err := os.Stat(p); err != nil || info.IsDir() {
			continue
		}
		out, err := exec.Command(p, "--version").Output()
		if err == nil && strings.TrimSpace(string(out)) == "ha 0.1.8" {
			return p
		}
	}
	t.Fatalf("setup: no ha 0.1.8 on PATH")
	return ""
}

func TestCoreIntegrityAC1_OlderCoreRefuses(t *testing.T) {
	const v = "rules-v6-unsupported"
	old := ciHa018(t)
	f := ciFixture(t, v, "")
	before := f.records()
	for _, args := range [][]string{{"status", rgGoal, "--format", "json"}, {"check", rgGoal, "AC-1"},
		{"note", rgGoal, "wait", "--reason", "the user decides"}, {"done", rgGoal}} {
		cmd := exec.Command(old, args...)
		cmd.Dir = f.dir
		out, err := cmd.CombinedOutput()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatalf("setup: ha 0.1.8 %v: %v", args, err)
		}
		if code != exitRules {
			t.Errorf("jaekit:violation=%s ha 0.1.8 %v on a run-rules/6 goal exit %d; want 66: %s", v, args, code, out)
		}
		if !bytes.Equal(before, f.records()) {
			t.Errorf("jaekit:violation=%s ha 0.1.8 %v changed the run record of a run-rules/6 goal", v, args)
		}
	}
}
