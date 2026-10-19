package run

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// cleanupEnds is a process controller whose finish ends every process left in
// the group before the real finish, as the Windows job cleanup does.
type cleanupEnds struct{ processController }

func (c cleanupEnds) finish() error {
	_ = c.kill()
	return c.processController.finish()
}

// TestNativeStdoutDrainBeforeCleanup: a descendant that writes the copied
// standard output shortly after the process exited is read to the end before
// the process group is finished, so cleanup never cuts the stream short.
func TestNativeStdoutDrainBeforeCleanup(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	starters := map[string]processStarter{
		"cleanup_ends_descendants": func(cmd *exec.Cmd) (processController, error) {
			group, err := startProcess(cmd)
			if err != nil {
				return nil, err
			}
			return cleanupEnds{group}, nil
		},
		"native_cleanup": startProcess,
	}
	for name, starter := range starters {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			var copied bytes.Buffer
			res, err := execWithOutput(dir, []string{executable, "-test.run=^TestStdoutLateWriterHelper$"}, 20*time.Second,
				filepath.Join(dir, "output.log"), []string{"HA_LATE_WRITER=parent"}, starter, &copied)
			if err != nil || res.Outcome != Pass || !strings.Contains(copied.String(), "late line") {
				t.Fatalf("the descendant's output was cut short: %+v %v (copied %q)", res, err, copied.String())
			}
		})
	}
}

// TestStdoutLateWriterHelper is the child of the test above: it starts a
// descendant that writes to standard output half a second later, prints a
// line and exits 0. It does nothing in an ordinary test run.
func TestStdoutLateWriterHelper(t *testing.T) {
	switch os.Getenv("HA_LATE_WRITER") {
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=^TestStdoutLateWriterHelper$")
		child.Env = append(os.Environ(), "HA_LATE_WRITER=child")
		child.Stdout = os.Stdout
		if err := child.Start(); err != nil {
			os.Exit(90)
		}
		fmt.Println("early line")
		os.Exit(0)
	case "child":
		time.Sleep(500 * time.Millisecond)
		fmt.Println("late line")
		os.Exit(0)
	}
}
