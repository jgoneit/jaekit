//go:build !windows

package run

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

type processGroup struct{ pid int }

func startProcess(cmd *exec.Cmd) (processController, error) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return &processGroup{pid: cmd.Process.Pid}, nil
}

func (g *processGroup) kill() error {
	err := syscall.Kill(-g.pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
func (g *processGroup) finish() error { return nil }

func processSignal(err *exec.ExitError) (int, string) {
	if status, ok := err.Sys().(syscall.WaitStatus); ok && status.Signaled() {
		return 128 + int(status.Signal()), fmt.Sprint(int(status.Signal()))
	}
	return 0, ""
}
