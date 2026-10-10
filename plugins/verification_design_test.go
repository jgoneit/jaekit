package plugins

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Public examples and adversarial mutations are exercised through the same
// suite as the condition producer. This proves documentation, not host behavior.
func TestVerificationDesignContracts(t *testing.T) {
	command := exec.Command("python3", "tools/contract-checks/test_design_check.py")
	command.Dir = ".."
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "HA_EVIDENCE_") {
			command.Env = append(command.Env, value)
		}
	}
	command.Env = append(command.Env, "PYTHONDONTWRITEBYTECODE=1")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("assurance contracts: %v\n%s", err, output)
	}
}
