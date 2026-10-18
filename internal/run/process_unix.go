//go:build !windows

package run

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"time"
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

// processCPU reads the user and system time wait4 reported for the process:
// its own use and that of the descendants it waited for. Descendants that
// were killed with the process group without being waited for are not counted.
func processCPU(cmd *exec.Cmd, _ processController) CPU {
	cpu := CPU{Status: CPUFailed, Scope: "process-and-waited-descendants", Source: "wait4-rusage"}
	if cmd.ProcessState == nil {
		return cpu
	}
	usage, ok := cmd.ProcessState.SysUsage().(*syscall.Rusage)
	if !ok || usage == nil {
		return cpu
	}
	cpu.Status = CPUMeasured
	cpu.User = time.Duration(usage.Utime.Nano())
	cpu.System = time.Duration(usage.Stime.Nano())
	return cpu
}
