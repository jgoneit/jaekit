//go:build jaekit_compat_018

package main

// AC-1 of the goal check-cost: a Core that does not know run-rules/7 (the
// installed ha 0.1.8, found by ciHa018 of core_integrity_compat_test.go)
// neither computes nor records a goal that this tree's ha started under it,
// and exits 66. The goal's condition table has the baseline column, as a
// run-rules/7 goal's does. This file builds only with the tag
// jaekit_compat_018; ordinary go test and tools/verify.py leave it out.

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
)

func TestCheckCostAC1_OlderCoreRefuses(t *testing.T) {
	const v = "rules-v7-unsupported"
	f := ccRepo(t, ccGoal{conds: []ccCond{ccChange("AC-1", ccMark), ccChange("AC-2", "—"), ccMaintain("AC-3")}, column: true})
	f.start(v)
	f.implement()
	f.must(v, "the current checks", exitOK, "check", rgGoal)
	old := ciHa018(t)
	before := f.records()
	for _, args := range [][]string{{"status", rgGoal, "--format", "json"}, {"check", rgGoal, "AC-1"},
		{"note", rgGoal, "wait", "--reason", "the user decides"}, {"done", rgGoal}} {
		cmd := exec.Command(old, args...)
		cmd.Dir = f.dir
		out, err := cmd.CombinedOutput()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			if code = exit.ExitCode(); code < 0 {
				t.Fatalf("setup: ha 0.1.8 %v ended by a signal: %v", args, err)
			}
		} else if err != nil {
			t.Fatalf("setup: ha 0.1.8 %v: %v", args, err)
		}
		if code != exitRules {
			t.Errorf("jaekit:violation=%s ha 0.1.8 %v on a %s goal exit %d; want 66: %s", v, args, ccRules, code, out)
		}
		if !bytes.Equal(before, f.records()) {
			t.Errorf("jaekit:violation=%s ha 0.1.8 %v changed the run record of a %s goal", v, args, ccRules)
		}
	}
}
