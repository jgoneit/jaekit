//go:build !windows

package run

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestExecWithStdoutKeepsLogWriteFailure: when the log cannot store a copied
// standard output, the run is an error even though the process exits
// nonzero, which Wait reports before the copy error.
func TestExecWithStdoutKeepsLogWriteFailure(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "output.log")
	if err := syscall.Mkfifo(log, 0o600); err != nil {
		t.Fatal(err)
	}
	// The only reader leaves once ha has opened the log, so every later
	// write to it fails.
	gone := make(chan struct{})
	go func() {
		if r, err := os.OpenFile(log, os.O_RDONLY, 0); err == nil {
			r.Close()
		}
		close(gone)
	}()
	// A digest read of the log waits for a writer; this one ends it.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		for {
			select {
			case <-stop:
				return
			case <-time.After(10 * time.Millisecond):
			}
			if w, err := os.OpenFile(log, os.O_WRONLY|syscall.O_NONBLOCK, 0); err == nil {
				w.Close()
			}
		}
	}()
	starter := func(cmd *exec.Cmd) (processController, error) {
		<-gone
		return startProcess(cmd)
	}
	var copied bytes.Buffer
	res, err := execWithOutput(dir, helperCommand(t), 10*time.Second, log,
		[]string{"HA_NATIVE_HELPER=1", "HA_NATIVE_MODE=arguments", "HA_NATIVE_EXIT=1"}, starter, &copied)
	if err == nil || !res.Attempted || res.Outcome != Fail || res.Digest != "" {
		t.Fatalf("a failed log write produced output evidence: %+v %v", res, err)
	}
}

// TestExecWithStdoutCutShortAfterNonzeroExit: a descendant that keeps the
// copied standard output open past the drain bound makes the run a wait
// failure, even though the process itself exited nonzero, so the copied
// prefix is never classified.
func TestExecWithStdoutCutShortAfterNonzeroExit(t *testing.T) {
	saved := stdoutDrain
	stdoutDrain = 300 * time.Millisecond
	defer func() { stdoutDrain = saved }()
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var copied bytes.Buffer
	res, err := ExecWithStdout(dir, []string{executable, "-test.run=^TestStdoutLingerHelper$"}, 10*time.Second,
		filepath.Join(dir, "output.log"), []string{"HA_STDOUT_HELPER=parent"}, &copied)
	if err != nil || res.Outcome != Error || res.Reason != "process_wait" || res.Exit != nil {
		t.Fatalf("a cut-short standard output was kept as an ordinary exit: %+v %v (copied %q)", res, err, copied.String())
	}
}

// TestStdoutLingerHelper is the child of the test above: it starts a
// descendant that holds standard output for two seconds, prints a line and
// exits 1. It does nothing in an ordinary test run.
func TestStdoutLingerHelper(t *testing.T) {
	switch os.Getenv("HA_STDOUT_HELPER") {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestStdoutLingerHelper$")
		child.Env = append(os.Environ(), "HA_STDOUT_HELPER=linger")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(90)
		}
		fmt.Println("jaekit:violation=greeting-missing before the end")
		os.Exit(1)
	case "killed":
		child := exec.Command(os.Args[0], "-test.run=^TestStdoutLingerHelper$")
		child.Env = append(os.Environ(), "HA_STDOUT_HELPER=linger")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(90)
		}
		fmt.Println("jaekit:violation=greeting-missing before the end")
		syscall.Kill(os.Getpid(), syscall.SIGKILL)
	case "linger":
		time.Sleep(2 * time.Second)
		os.Exit(0)
	}
}

// TestExecWithStdoutSignalKeepsItsReason: a process ended by a signal while a
// descendant still holds the copied standard output keeps the signal, so the
// run is recorded as a signal, not as a cut-short output.
func TestExecWithStdoutSignalKeepsItsReason(t *testing.T) {
	saved := stdoutDrain
	stdoutDrain = 300 * time.Millisecond
	defer func() { stdoutDrain = saved }()
	dir := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	var copied bytes.Buffer
	res, err := ExecWithStdout(dir, []string{executable, "-test.run=^TestStdoutLingerHelper$"}, 10*time.Second,
		filepath.Join(dir, "output.log"), []string{"HA_STDOUT_HELPER=killed"}, &copied)
	if err != nil || res.Signal != "9" || res.Reason == "process_wait" {
		t.Fatalf("the signal was replaced by the cut-short output: %+v %v", res, err)
	}
}
