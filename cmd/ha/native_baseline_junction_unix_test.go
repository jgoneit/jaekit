//go:build !windows

package main

import (
	"syscall"
	"testing"
)

// The same command driver can be checked locally with real Unix symlinks.
// This is not evidence of Windows junction behavior.
func TestNativeBaselineSymlinkScenarios(t *testing.T) {
	runBaselineJunctionScenarios(t)
}

func makeEvidenceFIFO(name string) error { return syscall.Mkfifo(name, 0o644) }
