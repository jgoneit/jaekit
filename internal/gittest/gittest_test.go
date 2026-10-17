package gittest

import (
	"os/exec"
	"strings"
	"testing"
)

func TestDisableAutoMaintenanceKeepsInheritedSettings(t *testing.T) {
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "synthetic.kept")
	t.Setenv("GIT_CONFIG_VALUE_0", "yes")
	// Restored after the test, like the entries the helper appends.
	for _, key := range []string{"GIT_CONFIG_KEY_1", "GIT_CONFIG_VALUE_1", "GIT_CONFIG_KEY_2", "GIT_CONFIG_VALUE_2"} {
		t.Setenv(key, "")
	}
	// An inherited `git -c` setting outranks GIT_CONFIG_COUNT entries.
	t.Setenv("GIT_CONFIG_PARAMETERS", "'maintenance.auto'='true' 'gc.auto'='100'")
	DisableAutoMaintenance()
	for key, want := range map[string]string{"synthetic.kept": "yes", "maintenance.auto": "false", "gc.auto": "0"} {
		out, err := exec.Command("git", "config", "--get", key).Output()
		if err != nil || strings.TrimSpace(string(out)) != want {
			t.Fatalf("git config %s = %q, %v; want %q", key, out, err, want)
		}
	}
}
