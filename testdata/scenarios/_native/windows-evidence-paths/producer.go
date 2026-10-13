package main

// A real, shell-independent producer for the command scenario. The marker is
// outside the repository so an attempted process cannot be hidden by cleanup.
import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func main() {
	marker, err := os.OpenFile(os.Getenv("HA_PATH_SCENARIO_MARKER"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		panic(err)
	}
	if _, err := marker.WriteString("executed\n"); err != nil {
		panic(err)
	}
	if err := marker.Close(); err != nil {
		panic(err)
	}
	fmt.Println("synthetic evidence-path scenario output")
	data, err := os.ReadFile("src/greeting.txt")
	if err != nil {
		panic(err)
	}
	passed := strings.TrimSpace(string(data)) == "hello"
	observation := map[string]any{"target": "greeting", "attempt": 1, "status": "pass"}
	if !passed {
		observation["status"] = "violation"
		observation["violation"] = "missing-greeting"
	}
	report := map[string]any{
		"schema": "check-result/v1", "producer": "synthetic", "producer_version": "1",
		"invocation":         os.Getenv("HA_EVIDENCE_INVOCATION"),
		"declaration_digest": os.Getenv("HA_EVIDENCE_DECLARATION_DIGEST"),
		"attempts_complete":  true, "observations": []any{observation},
	}
	raw, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(os.Getenv("HA_EVIDENCE_PATH"), raw, 0o600); err != nil {
		panic(err)
	}
	if !passed {
		os.Exit(1)
	}
}
