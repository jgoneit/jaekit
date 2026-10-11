package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Each finding decision scenario also runs alone, so one decision can be
// observed without the rest of the scenario suite.
func runNamedScenario(t *testing.T, name string) {
	abs, err := filepath.Abs(scenarioRoot)
	if err != nil {
		t.Fatal(err)
	}
	scenarioRoot = abs
	runScenario(t, name)
}

func TestFindingRetryEquivalentScenario(t *testing.T) {
	runNamedScenario(t, "findings-retry-equivalent")
}

func TestFindingRequiredListsScenario(t *testing.T) {
	runNamedScenario(t, "findings-required-lists")
}

func TestFindingResolutionRefsScenario(t *testing.T) {
	runNamedScenario(t, "findings-resolution-refs")
}

// The stored-reuse regression crosses the real binary, the record bytes and
// the Seal support helper, so it runs as a separate process here.
func TestFindingStoredReuseCLI(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(t.TempDir(), "ha")
	build := exec.Command("go", "build", "-mod=readonly", "-o", binary, "./cmd/ha")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build fixture: %v: %s", err, output)
	}
	command := exec.Command("python3", "tools/findings-checks/test_stored_reuse.py")
	command.Dir = root
	for _, env := range os.Environ() {
		if !strings.HasPrefix(env, "HA_EVIDENCE_") {
			command.Env = append(command.Env, env)
		}
	}
	command.Env = append(command.Env, "PYTHONDONTWRITEBYTECODE=1", "JAEKIT_FINDINGS_BINARY="+binary)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("stored findings reuse: %v: %s", err, output)
	}
}
