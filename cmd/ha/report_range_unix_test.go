//go:build !windows

package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The status computation's read of the base-to-HEAD range fails once and a
// later read would succeed: ha report still shows the uncompared summary and
// exits 70, because that failure is not read again (impl review 03-delta
// codex-F1).
func TestReportRangeFailureIsNotReadAgain(t *testing.T) {
	f := reportFixture(t)
	f.must(0, "start", scenarioGoal, "--request", "synthetic transient range")
	f.put("src/greeting.txt", "hello\n")
	f.git("add", "src/greeting.txt")
	f.git("commit", "-qm", "synthetic change")
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin, mark := t.TempDir(), filepath.Join(t.TempDir(), "failed-once")
	wrapper := "#!/bin/sh\nif [ \"$1 $2\" = \"diff --name-status\" ] && [ ! -e '" + mark + "' ]; then\n" +
		"  : > '" + mark + "'\n  echo 'synthetic transient failure' >&2\n  exit 128\nfi\nexec '" + real + "' \"$@\"\n"
	if err := os.WriteFile(filepath.Join(bin, "git"), []byte(wrapper), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	var out, stderr bytes.Buffer
	code := runMain([]string{"report", scenarioGoal}, &out, &stderr)
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if code != exitInternal || len(lines) != 5 || !strings.HasPrefix(lines[0], "Changed: Git cannot compare base ") ||
		!strings.HasPrefix(lines[1], "Checked: not computed; ") {
		t.Fatalf("exit %d\n%s\nstderr %s", code, out.String(), stderr.String())
	}
	if _, err := os.Stat(mark); err != nil {
		t.Fatal("the wrapper never failed a range read")
	}
	if code, out := f.ha("report", scenarioGoal); code != exitOK || !strings.HasPrefix(out, "Changed: 1 files and 1 commits") {
		t.Fatalf("after the transient failure: exit %d\n%s", code, out)
	}
}
