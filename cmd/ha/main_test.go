package main

import (
	"os"
	"testing"

	"github.com/jgoneit/jaekit/internal/gittest"
)

// Fixture repositories must not keep changing after a test ends. Binaries
// that tests share (sharedBinary) are removed once every test has ended,
// after a check that no test changed them: a changed one prints a
// shared-binary-changed violation and fails the run even when every test
// passed.
func TestMain(m *testing.M) {
	gittest.DisableAutoMaintenance()
	code := m.Run()
	if !checkSharedBinaries(os.Stdout) && code == 0 {
		code = 1
	}
	removeSharedBinaries()
	os.Exit(code)
}
