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
// caller the same binary, which no test can change and which outlives the
// test that first asked for it, and fails every asking test when the build
// fails. Each violation's message starts with sharedBinaryViolation; an
// unmarked t.Fatalf is an input failure: the check itself could not run.
//
// Two child processes of this test binary observe what one process with a
// warm cache cannot: a new process has nothing cached, so its first caller
// is the one that builds.
const (
	sharedBinaryTest        = "TestSharedBinaryAC4_SafeSharedBuild"
	sharedBinaryViolation   = "jaekit:violation=shared-binary-unsafe: "
	sharedBinaryUnbuildable = "./cmd/ha-ac4-no-such-package"
	// The child modes.
	sharedBinaryFailingMode = "shared-binary-unbuildable"
	sharedBinaryCleanupMode = "shared-binary-cleanup"
	// Lines the children print; the parent reads them.
	sharedBinaryCalling     = "shared-binary-ac4-child: asking for an unbuildable package in"
	sharedBinaryReturned    = "shared-binary-ac4-child: sharedBinary returned in"
	sharedBinaryFirstAsking = "shared-binary-ac4-child: first caller asking for ./cmd/ha"
	sharedBinaryLaterAsking = "shared-binary-ac4-child: later caller asking for ./cmd/ha"
	sharedBinaryChildSetup  = "shared-binary-ac4-child: not observed: "
)

// The subtests of the children. Every caller of the failing child asks for
// the unbuildable package; the cleanup child's first caller ends before the
// later caller asks.
var (
	sharedBinaryFailingCallers = []string{"first-caller", "second-caller"}
	sharedBinaryCleanupCallers = []string{"first-caller", "later-caller"}
)

func TestSharedBinaryAC4_SafeSharedBuild(t *testing.T) {
	switch sharedBinaryChild() {
	case sharedBinaryFailingMode:
		for _, name := range sharedBinaryFailingCallers {
			t.Run(name, func(t *testing.T) {
				fmt.Println(sharedBinaryCalling, name)
				path := sharedBinary(t, sharedBinaryUnbuildable)
				fmt.Println(sharedBinaryReturned, name, path)
			})
		}
		return
	case sharedBinaryCleanupMode:
		sharedBinaryCleanupChild(t)
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
	// re-execute their own binary: every caller there must fail.
	text, err := sharedBinaryRunChild(t, sharedBinaryFailingMode)
	var exit *exec.ExitError
	switch {
	case !strings.Contains(text, sharedBinaryCalling+" "+sharedBinaryFailingCallers[0]):
		t.Fatalf("child did not reach the unbuildable request: %v: %s", err, text)
	case strings.Contains(text, sharedBinaryReturned):
		t.Fatalf(sharedBinaryViolation+"sharedBinary(%s) returned to a caller instead of failing it:\n%s",
			sharedBinaryUnbuildable, text)
	case sharedBinaryOutcome(text, sharedBinaryFailingCallers, "PASS", "SKIP") != "":
		t.Fatalf(sharedBinaryViolation+"the caller %s asking for %s passed or was skipped:\n%s",
			sharedBinaryOutcome(text, sharedBinaryFailingCallers, "PASS", "SKIP"), sharedBinaryUnbuildable, text)
	case err == nil:
		t.Fatalf(sharedBinaryViolation+"the child asking for %s passed:\n%s", sharedBinaryUnbuildable, text)
	case !errors.As(err, &exit):
		t.Fatalf("child could not run: %v: %s", err, text)
	case !strings.Contains(text, sharedBinaryCalling+" "+sharedBinaryFailingCallers[1]):
		t.Fatalf("child did not reach the second unbuildable request (exit %d): %s", exit.ExitCode(), text)
	case sharedBinaryOutcome(text, sharedBinaryFailingCallers, "FAIL") != strings.Join(sharedBinaryFailingCallers, ","):
		t.Fatalf("child exited %d without a failure line for every caller: %s", exit.ExitCode(), text)
	}

	// The cleanup child's first caller builds ./cmd/ha and ends, so its
	// cleanups and t.TempDir removal run before the later caller asks.
	text, err = sharedBinaryRunChild(t, sharedBinaryCleanupMode)
	switch {
	case !strings.Contains(text, sharedBinaryFirstAsking):
		t.Fatalf("cleanup child did not reach its first request: %v: %s", err, text)
	case sharedBinaryOutcome(text, sharedBinaryCleanupCallers[:1], "PASS") == "":
		t.Fatalf("the first caller in a new process did not get ./cmd/ha: %v: %s", err, text)
	case strings.Contains(text, sharedBinaryChildSetup):
		t.Fatalf("cleanup child could not observe the later caller: %v: %s", err, text)
	case strings.Contains(text, sharedBinaryViolation):
		t.Fatalf(sharedBinaryViolation+"after the first caller ended, the later caller's binary was unsafe:\n%s", text)
	case !strings.Contains(text, sharedBinaryLaterAsking):
		t.Fatalf("cleanup child did not reach its later request: %v: %s", err, text)
	case sharedBinaryOutcome(text, sharedBinaryCleanupCallers[1:], "FAIL", "SKIP") != "":
		t.Fatalf(sharedBinaryViolation+"after the first caller ended, sharedBinary failed or skipped the later caller:\n%s",
			text)
	case err != nil || sharedBinaryOutcome(text, sharedBinaryCleanupCallers[1:], "PASS") == "":
		t.Fatalf("cleanup child did not pass and has no later-caller result: %v: %s", err, text)
	}
}

// sharedBinaryCleanupChild runs in the cleanup child: the first caller records
// the path and content, ends, and the later caller must find the same file.
func sharedBinaryCleanupChild(t *testing.T) {
	var path string
	var digest []byte
	t.Run(sharedBinaryCleanupCallers[0], func(t *testing.T) {
		fmt.Println(sharedBinaryFirstAsking)
		path = sharedBinary(t, "./cmd/ha")
		digest = sharedBinaryDigest(t, path)
	})
	if path == "" || digest == nil {
		t.Fatalf("the first caller did not get ./cmd/ha")
	}
	t.Run(sharedBinaryCleanupCallers[1], func(t *testing.T) {
		fmt.Println(sharedBinaryLaterAsking)
		again := sharedBinary(t, "./cmd/ha")
		if again != path {
			t.Fatalf(sharedBinaryViolation+"after the first caller ended, sharedBinary returned %q, not %q", again, path)
		}
		info, err := os.Stat(again)
		if err != nil {
			t.Fatalf(sharedBinaryViolation+"%q is gone after the first caller ended: %v", again, err)
		}
		if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
			t.Fatalf(sharedBinaryViolation+"%q is writable or not a regular file after the first caller ended (mode %v)",
				again, info.Mode())
		}
		data, err := os.ReadFile(again)
		if err != nil {
			t.Fatalf(sharedBinaryViolation+"%q cannot be read after the first caller ended: %v", again, err)
		}
		if sum := sha256.Sum256(data); !bytes.Equal(sum[:], digest) {
			t.Fatalf(sharedBinaryViolation+"%q changed after the first caller ended", again)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		out, err := exec.CommandContext(ctx, again, "--version").CombinedOutput()
		if ctx.Err() != nil {
			t.Fatalf(sharedBinaryChildSetup+"%q --version did not finish: %v: %s", again, ctx.Err(), out)
		}
		if err != nil || !strings.HasPrefix(string(out), "ha ") {
			t.Fatalf(sharedBinaryViolation+"%q --version is not the ha binary after the first caller ended: %v: %s",
				again, err, out)
		}
	})
}

// sharedBinaryRunChild re-runs this test alone in a new process in the given
// mode and returns its combined output. A child that does not finish is an
// input failure of the asking test.
func sharedBinaryRunChild(t *testing.T, mode string) (string, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^"+sharedBinaryTest+"$", "-test.v", "-test.count=1",
		"--", mode)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("child %s did not finish: %v: %s", mode, ctx.Err(), out)
	}
	return string(out), err
}

// sharedBinaryOutcome returns, comma-separated, the callers whose subtest has
// a `--- <result>:` line for one of the given results in the child output.
func sharedBinaryOutcome(text string, callers []string, results ...string) string {
	var found []string
	for _, caller := range callers {
		for _, result := range results {
			if strings.Contains(text, "--- "+result+": "+sharedBinaryTest+"/"+caller+" ") {
				found = append(found, caller)
				break
			}
		}
	}
	return strings.Join(found, ",")
}

// sharedBinaryChild returns the child mode of this process, or "".
func sharedBinaryChild() string {
	for i, arg := range os.Args {
		if arg == "--" {
			if i+1 < len(os.Args) {
				return os.Args[i+1]
			}
			return ""
		}
	}
	return ""
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
