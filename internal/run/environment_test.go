package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecInvocationEnvironmentAndLegacySignal(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "output.log")
	t.Setenv("HA_TEST_VALUE", "inherited")
	res, err := ExecWithEnv(dir, []string{"sh", "-c", "printf '%s' \"$HA_TEST_VALUE\""}, time.Second, log, []string{"HA_TEST_VALUE=invocation"})
	if err != nil || res.Outcome != Pass || !res.Attempted {
		t.Fatalf("%+v %v", res, err)
	}
	data, err := os.ReadFile(log)
	if err != nil || string(data) != "invocation" {
		t.Fatalf("env output %q: %v", data, err)
	}
	res, err = Exec(dir, []string{"sh", "-c", "kill -TERM $$"}, time.Second, log)
	if err != nil || res.Outcome != Fail || res.Exit == nil || *res.Exit != 143 || res.Signal != "15" {
		t.Fatalf("signal changed legacy outcome: %+v %v", res, err)
	}
}

func TestExecKeepsAttemptOnOutputFailure(t *testing.T) {
	dir := t.TempDir()
	log := filepath.Join(dir, "output.log")
	res, err := ExecWithEnv(dir, []string{"sh", "-c", "rm \"$HA_TEST_LOG\""}, time.Second, log, []string{"HA_TEST_LOG=" + log})
	if err == nil || !res.Attempted || res.Exit == nil || *res.Exit != 0 {
		t.Fatalf("lost attempted process after digest I/O failure: %+v %v", res, err)
	}
	res, err = Exec(dir, []string{"sh", "-c", "exit 0"}, time.Second, filepath.Join(dir, "missing", "out.log"))
	if err == nil || res.Attempted {
		t.Fatalf("log preparation was counted as an attempt: %+v %v", res, err)
	}
	res, err = Exec(dir, []string{"missing-ha-test-command"}, time.Second, log)
	if err != nil || !res.Attempted || res.Reason != "process_start" || res.Exit != nil {
		t.Fatalf("start failure: %+v %v", res, err)
	}
	data, _ := os.ReadFile(log)
	if !strings.Contains(string(data), "could not start") {
		t.Fatal("start diagnostic absent")
	}
}
