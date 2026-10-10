// Package run executes a verification command as argv, without a shell, and
// keeps its combined output in a local log file.
package run

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync/atomic"
	"syscall"
	"time"
)

// Outcomes of a command.
const (
	Pass  = "pass"  // exit 0
	Fail  = "fail"  // exit non-zero
	Error = "error" // could not start, or timed out
)

// Result is the outcome of one command.
type Result struct {
	Outcome  string
	Exit     *int // nil when the command did not start or timed out
	Duration time.Duration
	Digest   string // hex sha256 of the log file
}

// Exec runs argv in dir with stdin closed, writing stdout and stderr to the
// file at logPath. When timeout passes, the whole process group is killed and
// the outcome is Error.
func Exec(dir string, argv []string, timeout time.Duration, logPath string) (Result, error) {
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Result{}, err
	}
	res := Result{}
	start := time.Now()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(log, "ha: could not start %q: %v\n", argv[0], err)
		res.Outcome = Error
	} else {
		var timedOut atomic.Bool
		timer := time.AfterFunc(timeout, func() {
			timedOut.Store(true)
			_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		})
		waitErr := cmd.Wait()
		timer.Stop()
		switch {
		case timedOut.Load():
			fmt.Fprintf(log, "\nha: timed out after %s\n", timeout)
			res.Outcome = Error
		case waitErr == nil:
			code := 0
			res.Exit, res.Outcome = &code, Pass
		default:
			var ee *exec.ExitError
			if !errors.As(waitErr, &ee) {
				fmt.Fprintf(log, "\nha: wait failed: %v\n", waitErr)
				res.Outcome = Error
				break
			}
			code := ee.ExitCode()
			if ws, ok := ee.Sys().(syscall.WaitStatus); ok && ws.Signaled() {
				code = 128 + int(ws.Signal())
			}
			res.Exit, res.Outcome = &code, Fail
		}
	}
	res.Duration = time.Since(start)
	if err := log.Close(); err != nil {
		return Result{}, err
	}
	res.Digest, err = fileDigest(logPath)
	return res, err
}

func fileDigest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
