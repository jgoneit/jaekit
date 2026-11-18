package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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
// Four child processes of this test binary observe what one process with a
// warm cache cannot: a new process has nothing cached, so its first caller
// is the one that builds.
//
//   - The unbuildable child starts in a module this test writes, whose
//     package ./unbuildable has a syntax error. Every caller must fail, and
//     each caller's own output must name the build failure: the package's
//     import path and the compiler's error. A caller that fails without
//     both (for example because `go` could not start) is an input failure.
//   - The cleanup child's first caller builds ./cmd/ha and ends; the later
//     caller must get the same, unchanged binary.
//   - The changed child's first caller builds ./cmd/ha, one test then
//     changes one byte of that file and puts back its mode and modification
//     time, and the later caller asks again. The later caller must fail
//     naming the binary's path and `changed`, and the child must report
//     shared-binary-changed (TestMain) and exit nonzero.
//   - The changed-last child's first caller builds ./cmd/ha and one test
//     then changes that file the same way, with no caller after it. Every
//     test must pass, and the child must still exit nonzero with TestMain's
//     own shared-binary-changed line: a line that starts with the marker
//     after the testing package's final PASS line, where no test's output
//     can be. In the changed child the later caller's failure carries the
//     same marker, so only this child shows that TestMain checks.
const (
	sharedBinaryTest      = "TestSharedBinaryAC4_SafeSharedBuild"
	sharedBinaryViolation = "jaekit:violation=shared-binary-unsafe: "
	// The unbuildable fixture module and what its build failure prints.
	sharedBinaryFixtureModule     = "example.com/shared-binary-ac4"
	sharedBinaryUnbuildable       = "./unbuildable"
	sharedBinaryUnbuildableImport = sharedBinaryFixtureModule + "/unbuildable"
	sharedBinaryUnbuildableSource = "package main\n\nthis is not Go\n"
	sharedBinaryCompilerError     = "syntax error: non-declaration statement outside function body"
	// The child modes.
	sharedBinaryFailingMode     = "shared-binary-unbuildable"
	sharedBinaryCleanupMode     = "shared-binary-cleanup"
	sharedBinaryChangedMode     = "shared-binary-changed"
	sharedBinaryChangedLastMode = "shared-binary-changed-last"
	// Lines the children print; the parent reads them.
	sharedBinaryCalling     = "shared-binary-ac4-child: asking for an unbuildable package in"
	sharedBinaryReturned    = "shared-binary-ac4-child: sharedBinary returned in"
	sharedBinaryFirstAsking = "shared-binary-ac4-child: first caller asking for ./cmd/ha"
	sharedBinaryLaterAsking = "shared-binary-ac4-child: later caller asking for ./cmd/ha"
	sharedBinaryChanged     = "shared-binary-ac4-child: a test changed the shared binary "
	sharedBinaryChildSetup  = "shared-binary-ac4-child: not observed: "
	// What TestMain prints when a shared binary changed during the run. The
	// parent's messages name it without the marker prefix, so that a failure
	// line never carries two different markers.
	sharedBinaryChangedName   = "shared-binary-changed"
	sharedBinaryChangedReport = "jaekit:violation=" + sharedBinaryChangedName
)

// The subtests of the children. Every caller of the failing child asks for
// the unbuildable package; the cleanup child's first caller ends before the
// later caller asks; the changed child's middle test changes the binary
// between its two callers; the changed-last child's last test changes it
// after its only caller.
var (
	sharedBinaryFailingCallers     = []string{"first-caller", "second-caller"}
	sharedBinaryCleanupCallers     = []string{"first-caller", "later-caller"}
	sharedBinaryChangedCallers     = []string{"first-caller", "changing-test", "later-caller"}
	sharedBinaryChangedLastCallers = sharedBinaryChangedCallers[:2:2]
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
	case sharedBinaryChangedMode:
		sharedBinaryChangedChild(t, sharedBinaryChangedCallers)
		return
	case sharedBinaryChangedLastMode:
		sharedBinaryChangedChild(t, sharedBinaryChangedLastCallers)
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

	// The build failure, the first caller's end and a changing test are each
	// observed in a child process, as other cmd/ha tests re-execute their
	// own binary.
	sharedBinaryCheckUnbuildable(t)
	t.Logf("unbuildable: every caller failed naming %s and %q", sharedBinaryUnbuildableImport, sharedBinaryCompilerError)
	sharedBinaryCheckCleanup(t)
	t.Logf("cleanup: the later caller got the same unchanged ./cmd/ha binary")
	sharedBinaryCheckChanged(t)
	t.Logf("changed: the later caller failed naming the change, and the child reported %s", sharedBinaryChangedName)
	sharedBinaryCheckChangedLast(t)
	t.Logf("changed-last: every test passed, and TestMain reported %s and failed the child", sharedBinaryChangedName)
}

// sharedBinaryCheckUnbuildable writes a module whose ./unbuildable package
// has a syntax error and runs the failing child in it: every caller there
// must fail, and for the build failure of that package.
func sharedBinaryCheckUnbuildable(t *testing.T) {
	t.Helper()
	module := t.TempDir()
	if err := os.WriteFile(filepath.Join(module, "go.mod"),
		[]byte("module "+sharedBinaryFixtureModule+"\n\ngo 1.21\n"), 0o644); err != nil {
		t.Fatalf("write the unbuildable fixture module: %v", err)
	}
	if err := os.Mkdir(filepath.Join(module, "unbuildable"), 0o755); err != nil {
		t.Fatalf("write the unbuildable fixture module: %v", err)
	}
	if err := os.WriteFile(filepath.Join(module, "unbuildable", "main.go"),
		[]byte(sharedBinaryUnbuildableSource), 0o644); err != nil {
		t.Fatalf("write the unbuildable fixture module: %v", err)
	}

	text, err := sharedBinaryRunChild(t, sharedBinaryFailingMode, module)
	var exit *exec.ExitError
	switch {
	case !strings.Contains(text, sharedBinaryCalling+" "+sharedBinaryFailingCallers[0]):
		t.Fatalf("child did not reach the unbuildable request: %v:\n%s", err, text)
	case strings.Contains(text, sharedBinaryReturned):
		t.Fatalf(sharedBinaryViolation+"sharedBinary(%s) returned to a caller instead of failing it:\n%s",
			sharedBinaryUnbuildable, text)
	case sharedBinaryOutcome(text, sharedBinaryFailingCallers, "PASS", "SKIP") != "":
		t.Fatalf(sharedBinaryViolation+"the caller %s asking for %s passed or was skipped:\n%s",
			sharedBinaryOutcome(text, sharedBinaryFailingCallers, "PASS", "SKIP"), sharedBinaryUnbuildable, text)
	case err == nil:
		t.Fatalf(sharedBinaryViolation+"the child asking for %s passed:\n%s", sharedBinaryUnbuildable, text)
	case !errors.As(err, &exit):
		t.Fatalf("child could not run: %v:\n%s", err, text)
	case !strings.Contains(text, sharedBinaryCalling+" "+sharedBinaryFailingCallers[1]):
		t.Fatalf("child did not reach the second unbuildable request (exit %d):\n%s", exit.ExitCode(), text)
	case sharedBinaryOutcome(text, sharedBinaryFailingCallers, "FAIL") != strings.Join(sharedBinaryFailingCallers, ","):
		t.Fatalf("child exited %d without a failure line for every caller:\n%s", exit.ExitCode(), text)
	}
	// A caller that failed for another reason, such as a `go` that could not
	// start, does not show that the build failure fails it.
	for _, caller := range sharedBinaryFailingCallers {
		own := sharedBinaryCallerOutput(text, caller)
		if !strings.Contains(own, sharedBinaryUnbuildableImport) || !strings.Contains(own, sharedBinaryCompilerError) {
			t.Fatalf("not observed: the caller %s failed without naming the build failure of %s (%q):\n%s",
				caller, sharedBinaryUnbuildableImport, sharedBinaryCompilerError, text)
		}
	}
}

// sharedBinaryCheckCleanup runs the cleanup child: its first caller builds
// ./cmd/ha and ends, so its cleanups and t.TempDir removal run before the
// later caller asks.
func sharedBinaryCheckCleanup(t *testing.T) {
	t.Helper()
	text, err := sharedBinaryRunChild(t, sharedBinaryCleanupMode, "")
	switch {
	case !strings.Contains(text, sharedBinaryFirstAsking):
		t.Fatalf("cleanup child did not reach its first request: %v:\n%s", err, text)
	case sharedBinaryOutcome(text, sharedBinaryCleanupCallers[:1], "PASS") == "":
		t.Fatalf("the first caller in a new process did not get ./cmd/ha: %v:\n%s", err, text)
	case strings.Contains(text, sharedBinaryChildSetup):
		t.Fatalf("cleanup child could not observe the later caller: %v:\n%s", err, text)
	case strings.Contains(text, sharedBinaryViolation):
		t.Fatalf(sharedBinaryViolation+"after the first caller ended, the later caller's binary was unsafe:\n%s", text)
	case !strings.Contains(text, sharedBinaryLaterAsking):
		t.Fatalf("cleanup child did not reach its later request: %v:\n%s", err, text)
	case sharedBinaryOutcome(text, sharedBinaryCleanupCallers[1:], "FAIL", "SKIP") != "":
		t.Fatalf(sharedBinaryViolation+"after the first caller ended, sharedBinary failed or skipped the later caller:\n%s",
			text)
	case err != nil || sharedBinaryOutcome(text, sharedBinaryCleanupCallers[1:], "PASS") == "":
		t.Fatalf("cleanup child did not pass and has no later-caller result: %v:\n%s", err, text)
	}
}

// sharedBinaryCheckChanged runs the changed child: after one of its tests
// changed the shared binary, a later caller must fail naming the change, and
// the child must report shared-binary-changed and exit nonzero.
func sharedBinaryCheckChanged(t *testing.T) {
	t.Helper()
	text, err := sharedBinaryRunChild(t, sharedBinaryChangedMode, "")
	path := sharedBinaryLine(text, sharedBinaryChanged)
	later := sharedBinaryChangedCallers[2:]
	own := sharedBinaryCallerOutput(text, later[0])
	var exit *exec.ExitError
	switch {
	case strings.Contains(text, sharedBinaryChildSetup) || path == "":
		t.Fatalf("changed child could not change its shared binary: %v:\n%s", err, text)
	case !strings.Contains(text, sharedBinaryLaterAsking):
		t.Fatalf("changed child did not reach its later request: %v:\n%s", err, text)
	case strings.Contains(text, sharedBinaryReturned):
		t.Fatalf(sharedBinaryViolation+"after a test changed %s, sharedBinary(./cmd/ha) returned it to a later caller:\n%s",
			path, text)
	case sharedBinaryOutcome(text, later, "PASS", "SKIP") != "":
		t.Fatalf(sharedBinaryViolation+"after a test changed %s, the later caller passed or was skipped:\n%s", path, text)
	case err == nil:
		t.Fatalf(sharedBinaryViolation+"the child in which a test changed %s passed:\n%s", path, text)
	case !errors.As(err, &exit):
		t.Fatalf("changed child could not run: %v:\n%s", err, text)
	case !strings.Contains(text, sharedBinaryChangedReport):
		t.Fatalf(sharedBinaryViolation+"the child in which a test changed %s exited %d without reporting %s:\n%s",
			path, exit.ExitCode(), sharedBinaryChangedName, text)
	case sharedBinaryOutcome(text, later, "FAIL") == "":
		t.Fatalf("changed child exited %d without a failure line for the later caller:\n%s", exit.ExitCode(), text)
	case !strings.Contains(own, "changed") ||
		!strings.Contains(own, path) && !strings.Contains(own, strconv.Quote(path)):
		t.Fatalf("not observed: the later caller failed without naming the change of %s:\n%s", path, text)
	}
}

// sharedBinaryCheckChangedLast runs the changed-last child: a test changes
// the shared binary after its only caller, so no caller can fail for it.
// Every test there must pass, and the child must still exit nonzero with
// TestMain's shared-binary-changed line for that binary.
func sharedBinaryCheckChangedLast(t *testing.T) {
	t.Helper()
	text, err := sharedBinaryRunChild(t, sharedBinaryChangedLastMode, "")
	path := sharedBinaryLine(text, sharedBinaryChanged)
	callers := sharedBinaryChangedLastCallers
	summary, report := sharedBinaryMainReport(text, path)
	var exit *exec.ExitError
	switch {
	case strings.Contains(text, sharedBinaryChildSetup) || path == "":
		t.Fatalf("changed-last child could not change its shared binary: %v:\n%s", err, text)
	case summary != "PASS" || sharedBinaryOutcome(text, callers, "PASS") != strings.Join(callers, ","):
		t.Fatalf("not observed: a test of the changed-last child did not pass (final line %q), so TestMain's "+
			"check is not observed alone: %v:\n%s", summary, err, text)
	case err == nil:
		t.Fatalf(sharedBinaryViolation+"a test changed %s after its last caller, and the child passed: "+
			"TestMain did not check the shared binaries:\n%s", path, text)
	case !errors.As(err, &exit):
		t.Fatalf("changed-last child could not run: %v:\n%s", err, text)
	case report == "":
		t.Fatalf(sharedBinaryViolation+"a test changed %s after its last caller, and the child exited %d without "+
			"TestMain's %s line for it after the final PASS line:\n%s", path, exit.ExitCode(), sharedBinaryChangedName, text)
	}
}

// sharedBinaryMainReport returns the testing package's final summary line
// in a child's -test.v output (PASS or FAIL, "" when it has none) and the
// first line after it that starts with TestMain's shared-binary-changed
// marker and names path. m.Run prints the summary as it returns, so a line
// after it comes from TestMain, not from a test; a test's own failure lines
// are indented and come before it.
func sharedBinaryMainReport(text, path string) (summary, report string) {
	lines := strings.Split(text, "\n")
	last := -1
	for i, line := range lines {
		if line := strings.TrimRight(line, "\r"); line == "PASS" || line == "FAIL" {
			summary, last = line, i
		}
	}
	if last < 0 || path == "" {
		return summary, ""
	}
	for _, line := range lines[last+1:] {
		line = strings.TrimRight(line, "\r")
		if strings.HasPrefix(line, sharedBinaryChangedReport+": ") && strings.Contains(line, path) {
			return summary, line
		}
	}
	return summary, ""
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

// sharedBinaryChangedChild runs in the changed and changed-last children:
// the first caller gets ./cmd/ha and the changing test changes that file.
// With a third subtest name (the changed child), the later caller asks again
// and only it may fail; without one (the changed-last child), no test asks
// again and every test passes. TestMain reports the change in both.
func sharedBinaryChangedChild(t *testing.T, callers []string) {
	var path string
	t.Run(callers[0], func(t *testing.T) {
		fmt.Println(sharedBinaryFirstAsking)
		path = sharedBinary(t, "./cmd/ha")
	})
	if path == "" {
		t.Fatalf(sharedBinaryChildSetup + "the first caller did not get ./cmd/ha")
	}
	t.Run(callers[1], func(t *testing.T) {
		if err := sharedBinaryChange(path); err != nil {
			t.Fatalf(sharedBinaryChildSetup+"cannot change %q: %v", path, err)
		}
		fmt.Println(sharedBinaryChanged + path)
	})
	if len(callers) < 3 {
		return
	}
	t.Run(callers[2], func(t *testing.T) {
		fmt.Println(sharedBinaryLaterAsking)
		again := sharedBinary(t, "./cmd/ha")
		fmt.Println(sharedBinaryReturned, callers[2], again)
	})
}

// sharedBinaryChange flips one byte of the file in place, then puts back its
// mode and modification time, so that only its content shows the change.
func sharedBinaryChange(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.Size() == 0 {
		return fmt.Errorf("%s is empty", path)
	}
	if err := os.Chmod(path, 0o700); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	b := make([]byte, 1)
	offset := info.Size() / 2
	_, err = file.ReadAt(b, offset)
	if err == nil {
		b[0] ^= 0xff
		_, err = file.WriteAt(b, offset)
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	if err := os.Chmod(path, info.Mode().Perm()); err != nil {
		return err
	}
	return os.Chtimes(path, info.ModTime(), info.ModTime())
}

// sharedBinaryRunChild re-runs this test alone in a new process in the given
// mode, in dir when it is not empty, and returns its combined output. A child
// that does not finish is an input failure of the asking test.
func sharedBinaryRunChild(t *testing.T, mode, dir string) (string, error) {
	t.Helper()
	self, err := os.Executable()
	if err != nil {
		t.Fatalf("find this test binary: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, self, "-test.run=^"+sharedBinaryTest+"$", "-test.v", "-test.count=1",
		"--", mode)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("child %s did not finish: %v:\n%s", mode, ctx.Err(), out)
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

// sharedBinaryCallerOutput returns what the child printed while one caller's
// subtest ran: the lines after its `=== RUN` line up to the next `===` or
// `---` line, or "" when it has no `=== RUN` line. The children's subtests
// run one at a time, and `-test.v` prints their `--- <result>:` lines only
// when the parent test ends, so the next header ends the caller's output.
func sharedBinaryCallerOutput(text, caller string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimRight(line, "\r") != "=== RUN   "+sharedBinaryTest+"/"+caller {
			continue
		}
		var own []string
		for _, next := range lines[i+1:] {
			header := strings.TrimLeft(next, " ")
			if strings.HasPrefix(header, "=== ") || strings.HasPrefix(header, "--- ") {
				break
			}
			own = append(own, next)
		}
		return strings.Join(own, "\n")
	}
	return ""
}

// sharedBinaryLine returns the rest of the first line of text that starts
// with prefix, or "".
func sharedBinaryLine(text, prefix string) string {
	for _, line := range strings.Split(text, "\n") {
		if rest, ok := strings.CutPrefix(strings.TrimRight(line, "\r"), prefix); ok {
			return rest
		}
	}
	return ""
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
