package plugins

// Property: shared document helpers
// Helpers that more than one property file or a staying test reads documents with.

import (
	"os"
	"testing"
)

func visualRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
