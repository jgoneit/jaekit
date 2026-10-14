package main

import "testing"

// Windows Job finalization also terminates descendants of a checker which
// already exited. Unix process-group finalization does not promise this extra
// behavior, so this entrypoint is selected by the real Windows build only.
func TestNativeWindowsProcessControlScenarios(t *testing.T) {
	runProcessScenarios(t, "parent")
}
