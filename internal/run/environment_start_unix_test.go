//go:build !windows

package run

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"
)

// With the process's own working directory and environment, as ha's entry
// point passes them, a command that cannot start fails with exactly the
// error exec.Command gives for the same name started in the same folder: a
// missing ./name, a missing bare name and an existing ./name that cannot be
// executed. Through the exec path ha check uses (ExecIn), the outcome, the
// error and the "could not start" line in the log are those of the zero
// Environment, which is exec.Command as on the start commit.
func TestEnvironmentStartErrorsMatchExecCommand(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	process := Environment{Dir: wd, Env: os.Environ()}
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "tool"), []byte("#!/bin/sh\nexit 0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		name  string
		wraps error
	}{
		{"./missing-tool", fs.ErrNotExist},
		{fmt.Sprintf("run-missing-tool-%d", os.Getpid()), exec.ErrNotFound},
		{"./tool", fs.ErrPermission},
	} {
		cmd := exec.Command(c.name)
		cmd.Dir = dir
		want := startError(cmd)
		if !errors.Is(want, c.wraps) {
			t.Fatalf("setup: exec.Command(%q) in %s: start error %v, want one wrapping %v", c.name, dir, want, c.wraps)
		}
		if got := startError(process.Command(dir, c.name)); !reflect.DeepEqual(got, want) {
			t.Errorf("Command(%q) start error %T %v; exec.Command gives %T %v", c.name, got, got, want, want)
		}

		before, beforeLog := execStart(t, Environment{}, dir, c.name)
		after, afterLog := execStart(t, process, dir, c.name)
		if before.Outcome != Error || before.Reason != "process_start" || !reflect.DeepEqual(before.StartError, want) {
			t.Fatalf("setup: the zero Environment ran %q as %s/%s, %v", c.name, before.Outcome, before.Reason, before.StartError)
		}
		if after.Outcome != before.Outcome || after.Reason != before.Reason || !reflect.DeepEqual(after.StartError, before.StartError) {
			t.Errorf("ExecIn(%q) = %s/%s, %T %v; exec.Command gives %s/%s, %T %v", c.name,
				after.Outcome, after.Reason, after.StartError, after.StartError,
				before.Outcome, before.Reason, before.StartError, before.StartError)
		}
		if afterLog != beforeLog {
			t.Errorf("ExecIn(%q) logged %q; exec.Command gives %q", c.name, afterLog, beforeLog)
		}
	}
}

// An empty Env is an empty environment: Environ gives a non-nil empty copy,
// so a child started with it, directly or through ExecIn, receives none of
// the process's variables, as exec.Cmd gives none for an empty, non-nil Env.
func TestEnvironmentEmptyEnvGivesChildNoVariables(t *testing.T) {
	const parentOnly = "RUN_PARENT_ONLY_VARIABLE=parent"
	t.Setenv("RUN_PARENT_ONLY_VARIABLE", "parent")
	process := os.Environ()
	if !slices.Contains(process, parentOnly) {
		t.Fatalf("setup: %s is not in the process environment", parentOnly)
	}
	dir := t.TempDir()
	env := Environment{Dir: dir, Env: []string{}}
	if got := env.Environ(); got == nil || len(got) != 0 {
		t.Fatalf("Environ() of an empty Env = %#v; want a non-nil empty slice", got)
	}
	cmd := env.Command(dir, "/usr/bin/env")
	cmd.Env = env.Environ()
	direct, err := cmd.Output()
	if err != nil {
		t.Fatalf("/usr/bin/env with the empty environment: %v", err)
	}
	res, logged := execStart(t, env, dir, "/usr/bin/env")
	if res.Outcome != Pass {
		t.Fatalf("ExecIn(/usr/bin/env) with the empty environment = %s/%s, %v; log %q", res.Outcome, res.Reason, res.StartError, logged)
	}
	for _, child := range []struct{ via, out string }{{"Command", string(direct)}, {"ExecIn", logged}} {
		for _, entry := range process {
			if strings.Contains(child.out, entry) {
				t.Errorf("a child started through %s with the empty environment received the process entry %q", child.via, entry)
			}
		}
	}
}

// startError starts cmd and returns Start's error; a command that started is
// waited for.
func startError(cmd *exec.Cmd) error {
	err := cmd.Start()
	if err == nil {
		_ = cmd.Wait()
	}
	return err
}

// execStart runs name through ExecIn in env and returns the result and the log.
func execStart(t *testing.T, env Environment, dir, name string) (Result, string) {
	t.Helper()
	logPath := filepath.Join(t.TempDir(), "output.log")
	res, err := ExecIn(env, dir, []string{name}, time.Minute, logPath, nil, nil)
	if err != nil {
		t.Fatalf("setup: ExecIn(%q): %v", name, err)
	}
	log, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("setup: read the log of %q: %v", name, err)
	}
	return res, string(log)
}
