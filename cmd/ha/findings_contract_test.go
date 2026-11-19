package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The shared regression exercises an actual binary and independent file/Git
// boundaries. The goal's recorded check additionally passes a fixed legacy
// binary; CI itself does not require a previously installed release.
func TestFindingsPublicCLI(t *testing.T) {
	t.Parallel()
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	binary := sharedBinary(t, "./cmd/ha")
	command := exec.Command("python3", "tools/findings-checks/check.py", "--binary", binary, "--criterion", "all")
	command.Dir = root
	// This nested public regression is not the parent goal's result producer.
	// Its fixture must not overwrite a caller's evidence artifact: the
	// environment it starts from holds a parent's evidence variables.
	marker := filepath.Join(t.TempDir(), "outer-evidence.json")
	parent := envWith(os.Environ(), "HA_EVIDENCE_PATH="+marker, "HA_EVIDENCE_INVOCATION=synthetic-parent",
		"HA_EVIDENCE_DECLARATION_DIGEST=synthetic-parent")
	for _, env := range parent {
		if !strings.HasPrefix(env, "HA_EVIDENCE_") {
			command.Env = append(command.Env, env)
		}
	}
	command.Env = append(command.Env, "PYTHONDONTWRITEBYTECODE=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("findings CLI contract: %v: %s", err, output)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("nested regression touched parent evidence: %v", err)
	}
}
