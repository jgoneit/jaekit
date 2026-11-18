package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// cmd-ha-test-speed AC-4 (tools/goal-checks/cmd-ha-test-speed/AC-4.json).
// sharedBinary(t, pkg) builds pkg once per test process and hands every
// caller the same binary, which no test can change, and fails the asking
// test when the build fails. Each violation's message starts with
// sharedBinaryViolation; an unmarked t.Fatalf is an input failure: the check
// itself could not run.
const (
	sharedBinaryViolation   = "jaekit:violation=shared-binary-unsafe: "
	sharedBinaryChildMode   = "shared-binary-unbuildable"
	sharedBinaryUnbuildable = "./cmd/ha-ac4-no-such-package"
	sharedBinaryCalling     = "shared-binary-ac4-child: asking for an unbuildable package"
	sharedBinaryReturned    = "shared-binary-ac4-child: sharedBinary returned"
)

func TestSharedBinaryAC4_SafeSharedBuild(t *testing.T) {
	if sharedBinaryChild() {
		// A new process has nothing cached: the build is attempted here.
		fmt.Println(sharedBinaryCalling)
		path := sharedBinary(t, sharedBinaryUnbuildable)
		fmt.Println(sharedBinaryReturned, path)
		return
	}
	t.Parallel()

	first := sharedBinary(t, "./cmd/ha")
	before, err := os.Stat(first)
	if err != nil {
		t.Fatalf(sharedBinaryViolation+"sharedBinary(./cmd/ha) returned %q, which is not a file: %v", first, err)
	}
	second := sharedBinary(t, "./cmd/ha")
	if second != first {
		t.Fatalf(sharedBinaryViolation+"two calls for ./cmd/ha returned %q and %q", first, second)
	}
	after, err := os.Stat(second)
	if err != nil {
		t.Fatalf(sharedBinaryViolation+"%q disappeared after the second call: %v", second, err)
	}
	if !os.SameFile(before, after) || !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size() {
		t.Fatalf(sharedBinaryViolation+"the second call for ./cmd/ha rebuilt or replaced %q", first)
	}
	if !after.Mode().IsRegular() || after.Mode().Perm()&0o222 != 0 {
		t.Fatalf(sharedBinaryViolation+"%q is writable or not a regular file (mode %v)", first, after.Mode())
	}

	digest := sharedBinaryDigest(t, first)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out, err := exec.CommandContext(ctx, first, "--version").CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("%q --version did not finish: %v: %s", first, ctx.Err(), out)
	}
	if err != nil || !strings.HasPrefix(string(out), "ha ") {
		t.Fatalf(sharedBinaryViolation+"%q --version is not the ha binary: %v: %s", first, err, out)
	}
	if again := sharedBinaryDigest(t, first); !bytes.Equal(again, digest) {
		t.Fatalf(sharedBinaryViolation+"running %q --version changed its content", first)
	}

	// The build failure is observed in a child process, as other cmd/ha tests
	// re-execute their own binary.
	child, cancelChild := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancelChild()
	cmd := exec.CommandContext(child, os.Args[0], "-test.run=^TestSharedBinaryAC4_SafeSharedBuild$", "-test.v",
		"-test.count=1", "--", sharedBinaryChildMode)
	out, err = cmd.CombinedOutput()
	text := string(out)
	var exit *exec.ExitError
	switch {
	case child.Err() != nil:
		t.Fatalf("child for %s did not finish: %v: %s", sharedBinaryUnbuildable, child.Err(), out)
	case !strings.Contains(text, sharedBinaryCalling):
		t.Fatalf("child did not reach the unbuildable request: %v: %s", err, out)
	case strings.Contains(text, sharedBinaryReturned):
		t.Fatalf(sharedBinaryViolation+"sharedBinary(%s) returned instead of failing the test:\n%s",
			sharedBinaryUnbuildable, out)
	case err == nil, strings.Contains(text, "--- SKIP: TestSharedBinaryAC4_SafeSharedBuild"):
		t.Fatalf(sharedBinaryViolation+"the test asking for %s passed or was skipped:\n%s", sharedBinaryUnbuildable, out)
	case !errors.As(err, &exit):
		t.Fatalf("child could not run: %v: %s", err, out)
	case !strings.Contains(text, "--- FAIL: TestSharedBinaryAC4_SafeSharedBuild"):
		t.Fatalf("child exited %d without a test failure line: %s", exit.ExitCode(), out)
	}
}

// sharedBinaryChild reports whether this process is the AC-4 child.
func sharedBinaryChild() bool {
	for i, arg := range os.Args {
		if arg == "--" {
			return i+1 < len(os.Args) && os.Args[i+1] == sharedBinaryChildMode
		}
	}
	return false
}

func sharedBinaryDigest(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	sum := sha256.Sum256(data)
	return sum[:]
}
