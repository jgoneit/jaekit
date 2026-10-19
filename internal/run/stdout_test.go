//go:build !windows

package run

import (
	"bytes"
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
