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
	Attempted bool // a process start was attempted, even if later output I/O failed
	Outcome   string
	Exit      *int   // nil when the command did not start or timed out
	Reason    string // fixed code for process errors; never command output
	Signal    string // signal number, even though legacy Outcome remains Fail
	Duration  time.Duration
	Digest    string // hex sha256 of the log file

	// Limit is the time limit applied to the process. LimitElapsed runs from
	// the limit timer's start, after a successful process start, until the
	// process exit was observed or the timeout was decided; later cleanup is
	// not included. It is nil when the timer never started.
	Limit        time.Duration
	LimitElapsed *time.Duration
	LimitReached bool
	CPU          CPU
}

// CPU states. A value that was not measured is never reported as zero.
const (
	CPUMeasured    = "measured"
	CPUUnavailable = "unavailable" // no process ran, or the system does not report it
	CPUFailed      = "failed"      // the system reports it, but reading it failed
)

// CPU is the processor time the system reported for one command. Scope names
// which processes it covers, Source the interface that reported it.
type CPU struct {
	Status       string
	User, System time.Duration
	Scope        string
	Source       string
}

// Exec runs argv in dir with stdin closed, writing stdout and stderr to the
// file at logPath. When timeout passes, the whole process group is killed and
// the outcome is Error.
func Exec(dir string, argv []string, timeout time.Duration, logPath string) (Result, error) {
	return ExecWithEnv(dir, argv, timeout, logPath, nil)
}

// ExecWithEnv adds invocation-specific environment values without changing the
// legacy exit interpretation used by Exec. Later entries override inherited ones.
type processController interface {
	kill() error
	finish() error
}

type processStarter func(*exec.Cmd) (processController, error)

// A process may have inherited the log handle before admission failed. A
// bounded cleanup failure cannot establish an immutable output digest.
var errUnfinalizedProcess = errors.New("process output finalization is unconfirmed")

func ExecWithEnv(dir string, argv []string, timeout time.Duration, logPath string, extraEnv []string) (Result, error) {
	return execWithStarter(dir, argv, timeout, logPath, extraEnv, startProcess)
}

func execWithStarter(dir string, argv []string, timeout time.Duration, logPath string, extraEnv []string, starter processStarter) (Result, error) {
	log, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return Result{}, err
	}
	res := Result{Limit: timeout, CPU: CPU{Status: CPUUnavailable}}
	var finalizationErr error
	start := time.Now()
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), extraEnv...)
	cmd.Stdout = log
	cmd.Stderr = log
	res.Attempted = true
	group, startErr := starter(cmd)
	if startErr != nil {
		fmt.Fprintf(log, "ha: could not start %q: %v\n", argv[0], startErr)
		res.Outcome, res.Reason = Error, "process_start"
		if errors.Is(startErr, errUnfinalizedProcess) {
			finalizationErr = startErr
		}
	} else {
		waited := make(chan error, 1)
		go func() { waited <- cmd.Wait() }()
		timerStart := time.Now()
		timer := time.NewTimer(timeout)
		var waitErr, controlErr error
		timedOut := false
		select {
		case waitErr = <-waited:
			timer.Stop()
			elapsed := time.Since(timerStart)
			res.LimitElapsed = &elapsed
		case <-timer.C:
			elapsed := time.Since(timerStart)
			res.LimitElapsed, res.LimitReached = &elapsed, true
			timedOut = true
			controlErr = group.kill()
			if controlErr != nil {
				_ = cmd.Process.Kill()
			}
			// A failed termination must not leave the recorder waiting forever.
			select {
			case waitErr = <-waited:
			case <-time.After(5 * time.Second):
				controlErr = errors.Join(controlErr, errors.New("process did not exit after termination"))
			}
		}
		controlErr = errors.Join(controlErr, group.finish())
		res.CPU = processCPU(cmd, group)
		switch {
		case controlErr != nil:
			fmt.Fprintf(log, "\nha: process control failed: %v\n", controlErr)
			res.Outcome, res.Reason = Error, "process_wait"
			finalizationErr = fmt.Errorf("output cannot be finalized after process control failure: %w", controlErr)
		case timedOut:
			fmt.Fprintf(log, "\nha: timed out after %s\n", timeout)
			res.Outcome, res.Reason = Error, "process_timeout"
		case waitErr == nil:
			code := 0
			res.Exit, res.Outcome = &code, Pass
		default:
			var ee *exec.ExitError
			if !errors.As(waitErr, &ee) {
				fmt.Fprintf(log, "\nha: wait failed: %v\n", waitErr)
				res.Outcome, res.Reason = Error, "process_wait"
				break
			}
			code := ee.ExitCode()
			if signalCode, signal := processSignal(ee); signal != "" {
				code, res.Signal = signalCode, signal
			}
			res.Exit, res.Outcome = &code, Fail
		}
	}
	res.Duration = time.Since(start)
	if err := log.Close(); err != nil {
		return res, err
	}
	if finalizationErr != nil {
		return res, finalizationErr
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
