package run

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

// TestNativeRunTiming observes the limit timer and CPU accounting of real
// processes on the current system (run-record.md §2.2).
func TestNativeRunTiming(t *testing.T) {
	dir := t.TempDir()
	t.Run("exit", func(t *testing.T) {
		res, err := ExecWithEnv(dir, helperCommand(t), 10*time.Second, filepath.Join(dir, "exit.log"),
			[]string{"HA_NATIVE_HELPER=1", "HA_NATIVE_MODE=arguments", "HA_NATIVE_EXIT=0"})
		if err != nil || res.Outcome != Pass {
			t.Fatalf("run: %+v %v", res, err)
		}
		if res.Limit != 10*time.Second || res.LimitReached || res.LimitElapsed == nil {
			t.Fatalf("limit: %+v", res)
		}
		if *res.LimitElapsed < 0 || *res.LimitElapsed > res.Duration {
			t.Fatalf("limit elapsed %v outside the run's duration %v", *res.LimitElapsed, res.Duration)
		}
		if res.CPU.Status != CPUMeasured || res.CPU.User < 0 || res.CPU.System < 0 || res.CPU.Scope == "" || res.CPU.Source == "" {
			t.Fatalf("cpu: %+v", res.CPU)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		limit := 300 * time.Millisecond
		res, err := ExecWithEnv(dir, helperCommand(t), limit, filepath.Join(dir, "timeout.log"),
			[]string{"HA_NATIVE_HELPER=1", "HA_NATIVE_MODE=parent", "HA_NATIVE_MARKER=" + filepath.Join(dir, "heartbeat")})
		if err != nil || res.Outcome != Error || res.Reason != "process_timeout" {
			t.Fatalf("run: %+v %v", res, err)
		}
		if !res.LimitReached || res.LimitElapsed == nil || *res.LimitElapsed < limit {
			t.Fatalf("limit: %+v", res)
		}
		// The limit interval ends at the timeout decision; cleanup is outside it.
		if *res.LimitElapsed > res.Duration {
			t.Fatalf("limit elapsed %v exceeds the run's duration %v", *res.LimitElapsed, res.Duration)
		}
		if res.CPU.Status == "" || res.CPU.Status == CPUUnavailable {
			t.Fatalf("cpu after a started process: %+v", res.CPU)
		}
	})
	t.Run("start failure", func(t *testing.T) {
		res, err := Exec(dir, []string{"ha-nonexistent-timing-checker"}, time.Second, filepath.Join(dir, "missing.log"))
		if err != nil || res.Reason != "process_start" {
			t.Fatalf("run: %+v %v", res, err)
		}
		if res.LimitElapsed != nil || res.LimitReached || res.CPU.Status != CPUUnavailable {
			t.Fatalf("a process that never started has no limit interval or CPU: %+v", res)
		}
	})
}

// TestCPUCollectionFailureKeepsTheRun makes CPU collection fail for a process
// that ran. The result, exit code, output digest, log and limit values are the
// same as with a working collection; only the CPU state says it failed.
func TestCPUCollectionFailureKeepsTheRun(t *testing.T) {
	dir := t.TempDir()
	env := []string{"HA_NATIVE_HELPER=1", "HA_NATIVE_MODE=arguments", "HA_NATIVE_EXIT=3"}
	measured, err := ExecWithEnv(dir, helperCommand(t), 10*time.Second, filepath.Join(dir, "measured.log"), env)
	if err != nil || measured.CPU.Status != CPUMeasured {
		t.Fatalf("measured run: %+v %v", measured, err)
	}
	original := collectCPU
	t.Cleanup(func() { collectCPU = original })
	collectCPU = func(*exec.Cmd, processController) CPU {
		return CPU{Status: CPUFailed, Scope: measured.CPU.Scope, Source: measured.CPU.Source}
	}
	failed, err := ExecWithEnv(dir, helperCommand(t), 10*time.Second, filepath.Join(dir, "failed.log"), env)
	if err != nil {
		t.Fatalf("a CPU collection failure ended the run with an error: %v", err)
	}
	if failed.CPU.Status != CPUFailed || failed.CPU.User != 0 || failed.CPU.System != 0 {
		t.Fatalf("cpu after a collection failure: %+v", failed.CPU)
	}
	same := failed.Attempted == measured.Attempted && failed.Outcome == measured.Outcome &&
		failed.Reason == measured.Reason && failed.Signal == measured.Signal && failed.Digest == measured.Digest &&
		failed.Limit == measured.Limit && failed.LimitReached == measured.LimitReached &&
		(failed.LimitElapsed == nil) == (measured.LimitElapsed == nil) &&
		failed.Exit != nil && measured.Exit != nil && *failed.Exit == *measured.Exit
	if !same {
		t.Fatalf("the run changed with a CPU collection failure:\nmeasured %+v\nfailed   %+v", measured, failed)
	}
	logs := [2][]byte{}
	for i, name := range []string{"measured.log", "failed.log"} {
		if logs[i], err = os.ReadFile(filepath.Join(dir, name)); err != nil {
			t.Fatal(err)
		}
	}
	if string(logs[0]) != string(logs[1]) {
		t.Fatalf("the output changed with a CPU collection failure: %q %q", logs[0], logs[1])
	}
}
