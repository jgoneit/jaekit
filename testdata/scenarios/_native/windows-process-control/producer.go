// A synthetic checker with a real descendant and structured result. The
// descendant has a finite fallback lifetime so a broken cleanup test is bounded.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

func main() {
	mode, marker := os.Args[1], os.Args[2]
	if mode == "writer" {
		deadline := time.Now().Add(15 * time.Second)
		for time.Now().Before(deadline) {
			fmt.Println("descendant stdout")
			fmt.Fprintln(os.Stderr, "descendant stderr")
			file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
			if err != nil {
				panic(err)
			}
			fmt.Fprintln(file, "descendant write")
			file.Close()
			time.Sleep(20 * time.Millisecond)
		}
		return
	}
	if err := os.WriteFile(marker+".producer", []byte("producer executed\n"), 0o600); err != nil {
		panic(err)
	}
	fmt.Println("synthetic process-control producer")
	if strings.HasPrefix(mode, "parent-") || mode == "timeout" {
		child := exec.Command(os.Args[0], "writer", marker)
		child.Stdout, child.Stderr = os.Stdout, os.Stderr
		if err := child.Start(); err != nil {
			panic(err)
		}
		deadline := time.Now().Add(10 * time.Second)
		for {
			data, err := os.ReadFile(marker)
			if err == nil && len(data) > 0 {
				break
			}
			if time.Now().After(deadline) {
				panic("descendant did not write")
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	observation := map[string]any{"target": "greeting", "attempt": 1, "status": "pass"}
	code := 0
	violation := mode == "parent-violation"
	if mode == "parent-greeting" {
		data, err := os.ReadFile("src/greeting.txt")
		if err != nil {
			panic(err)
		}
		violation = string(data) != "hello\n"
	}
	if violation {
		observation["status"], observation["violation"] = "violation", "missing-greeting"
		code = 3
	}
	report := map[string]any{
		"schema": "check-result/v1", "producer": "synthetic-process", "producer_version": "1",
		"invocation": os.Getenv("HA_EVIDENCE_INVOCATION"), "declaration_digest": os.Getenv("HA_EVIDENCE_DECLARATION_DIGEST"),
		"attempts_complete": true, "observations": []any{observation},
	}
	data, err := json.Marshal(report)
	if err != nil {
		panic(err)
	}
	if path := os.Getenv("HA_EVIDENCE_PATH"); path != "" {
		if err := os.WriteFile(path, data, 0o600); err != nil {
			panic(err)
		}
	}
	if mode == "timeout" {
		// The valid pass report above must not override a later timeout.
		for {
			time.Sleep(time.Second)
		}
	}
	os.Exit(code)
}
