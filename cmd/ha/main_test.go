package main

import (
	"os"
	"testing"

	"github.com/jgoneit/jaekit/internal/gittest"
)

// Fixture repositories must not keep changing after a test ends. Binaries
// that tests share (sharedBinary) are removed once every test has ended.
func TestMain(m *testing.M) {
	gittest.DisableAutoMaintenance()
	code := m.Run()
	removeSharedBinaries()
	os.Exit(code)
}
