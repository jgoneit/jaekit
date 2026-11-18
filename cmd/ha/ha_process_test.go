package main

import (
	"bytes"
	"errors"
	"os/exec"
	"testing"
)

// haProcess runs the shared ha binary as its own process in dir with env as
// its whole environment and returns its exit code, standard output and
// standard error. Unlike haIn, what ha does through its process's own
// working directory and environment then lands in dir and env too: a file
// it writes relative to the working directory, or a program it finds
// through the process's PATH. A binary that cannot start fails the test as
// a broken setup.
func haProcess(t testing.TB, dir string, env []string, args ...string) (int, string, string) {
	t.Helper()
	cmd := exec.Command(sharedBinary(t, "./cmd/ha"), args...)
	cmd.Dir, cmd.Env = dir, env
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("setup: start ha %v in %s: %v", args, dir, err)
		}
	}
	return cmd.ProcessState.ExitCode(), out.String(), errb.String()
}
