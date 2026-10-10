package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExecOutcomes(t *testing.T) {
	dir := t.TempDir()
	logp := filepath.Join(dir, "out.log")
	cases := []struct {
		argv    []string
		outcome string
		exit    int
	}{
		{[]string{"sh", "-c", "echo ok"}, Pass, 0},
		{[]string{"sh", "-c", "echo bad >&2; exit 3"}, Fail, 3},
		{[]string{"definitely-not-a-command-ha-test"}, Error, -1},
	}
	for _, c := range cases {
		res, err := Exec(dir, c.argv, 10*time.Second, logp)
		if err != nil {
			t.Fatal(err)
		}
		if res.Outcome != c.outcome {
			t.Errorf("%v: outcome %s, want %s", c.argv, res.Outcome, c.outcome)
		}
		if c.exit >= 0 && (res.Exit == nil || *res.Exit != c.exit) {
			t.Errorf("%v: exit %v, want %d", c.argv, res.Exit, c.exit)
		}
		if c.exit < 0 && res.Exit != nil {
			t.Errorf("%v: exit must be nil for a start failure", c.argv)
		}
		if res.Digest == "" {
			t.Errorf("%v: missing digest", c.argv)
		}
	}
	data, _ := os.ReadFile(logp)
	if !strings.Contains(string(data), "could not start") {
		t.Fatalf("start failure must be logged: %q", data)
	}
}

func TestExecTimeoutKillsProcessGroup(t *testing.T) {
	dir := t.TempDir()
	start := time.Now()
	// The child keeps the output open; killing only the parent would hang.
	res, err := Exec(dir, []string{"sh", "-c", "sleep 30 & sleep 30"}, 300*time.Millisecond, filepath.Join(dir, "log"))
	if err != nil {
		t.Fatal(err)
	}
	if res.Outcome != Error || res.Exit != nil {
		t.Fatalf("timeout result = %+v", res)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("timeout did not stop the process group")
	}
}
