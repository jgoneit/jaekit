package main

import "testing"

// The filename selects the real Windows build. There is no skip or fake path
// classifier; the same built ha subprocess sees the host's actual OS semantics.
func TestNativeWindowsEvidencePathScenarios(t *testing.T) {
	runEvidencePathScenarios(t, "windows")
}
