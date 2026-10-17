package run

import (
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
