package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNativeBaselineRejectsJunction(t *testing.T) {
	parent := t.TempDir()
	outside := filepath.Join(parent, "외부 target")
	workspace := filepath.Join(parent, "baseline space")
	for _, dir := range []string{outside, workspace} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	sentinel := filepath.Join(outside, "observer.txt")
	if err := os.WriteFile(sentinel, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	// mklink is an explicitly invoked test fixture command, not a checker
	// command interpretation path. Directory junctions need no developer mode.
	cmd := exec.Command("cmd.exe", "/c", "mklink", "/J", filepath.Join(workspace, "checks"), outside)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, out)
	}
	root, err := os.OpenRoot(workspace)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	if err := applyBaselineFile(root, baselineFile{path: "checks/observer.txt", data: []byte("changed"), mode: 0o644}); err == nil {
		t.Fatal("junction destination was accepted")
	}
	data, err := os.ReadFile(sentinel)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("outside target changed: %q %v", data, err)
	}
}
