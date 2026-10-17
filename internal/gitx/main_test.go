package gitx

import (
	"os"
	"testing"

	"github.com/jgoneit/jaekit/internal/gittest"
)

// Fixture repositories must not keep changing after a test ends.
func TestMain(m *testing.M) {
	gittest.DisableAutoMaintenance()
	os.Exit(m.Run())
}
