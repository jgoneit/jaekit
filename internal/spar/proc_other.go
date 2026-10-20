//go:build windows

package spar

import "os/exec"

// Spar is not built or shipped for Windows; this keeps the package compiling.
func ownGroup(cmd *exec.Cmd) {}

func lockFile(path string, waiting func()) (func(), error) { return func() {}, nil }

func signalGroup(cmd *exec.Cmd, kill bool) {
	if cmd.Process != nil {
		cmd.Process.Kill()
	}
}
