package main

// This disposable fixture is a real producer, or a Git argv proxy. Only the
// proxy mutates a synthetic checkout, after real git worktree add succeeds.
import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "produce" {
		produce()
		return
	}
	args := os.Args[1:]
	cmd := exec.Command(os.Getenv("HA_JUNCTION_REAL_GIT"), args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		if exit, ok := err.(*exec.ExitError); ok {
			os.Exit(exit.ExitCode())
		}
		panic(err)
	}
	if len(args) < 2 || args[0] != "worktree" || args[1] != "add" {
		return
	}
	if len(args) != 6 || args[2] != "--detach" || args[3] != "--quiet" {
		panic("unexpected worktree add arguments")
	}
	tree := filepath.Clean(args[4])
	parent := filepath.Clean(os.Getenv("HA_JUNCTION_WORKTREES"))
	rel, err := filepath.Rel(parent, tree)
	if err != nil || !filepath.IsAbs(tree) || !filepath.IsLocal(rel) ||
		filepath.Base(tree) != "tree" || filepath.Dir(filepath.Dir(tree)) != parent ||
		!strings.HasPrefix(filepath.Base(filepath.Dir(tree)), "base-") {
		panic("refuse to modify anything outside the fixture baseline worktrees")
	}
	ledger := os.Getenv("HA_JUNCTION_LEDGER")
	previous, err := os.ReadFile(ledger)
	if err != nil && !os.IsNotExist(err) {
		panic(err)
	}
	ordinal := strings.Count(string(previous), "\n") + 1
	mode := os.Getenv("HA_JUNCTION_MODE")
	kind := "regular"
	if mode == "parent" || mode == "leaf" || (mode == "batch" && ordinal == 2) {
		kind = "parent"
		if mode == "leaf" {
			kind = "leaf"
		}
	}
	name := "overlay"
	if kind == "leaf" {
		name = "overlay/data.txt"
	}
	target := filepath.Join(tree, filepath.FromSlash(name))
	if kind != "regular" {
		if err := os.RemoveAll(target); err != nil {
			panic(err)
		}
		outside := os.Getenv("HA_JUNCTION_OUTSIDE")
		if runtime.GOOS == "windows" {
			out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", target, outside).CombinedOutput()
			if err != nil {
				panic(fmt.Sprintf("junction fixture setup: %v: %s", err, out))
			}
		} else if err := os.Symlink(outside, target); err != nil {
			panic(err)
		}
		info, err := os.Lstat(target)
		if err != nil || info.Mode()&(os.ModeSymlink|os.ModeIrregular) == 0 {
			panic("fixture did not create an actual link or irregular entry")
		}
	}
	appendJSON(ledger, map[string]any{"tree": tree, "ordinal": ordinal, "kind": kind, "path": name, "os": runtime.GOOS})
}

func appendJSON(name string, value any) {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(name, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	if _, err := f.Write(append(data, '\n')); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
}

func produce() {
	data, err := os.ReadFile("overlay/data.txt")
	if err != nil || string(data) != "head overlay\n" {
		panic("actual checker did not receive the committed HEAD overlay")
	}
	appendJSON(os.Getenv("HA_JUNCTION_MARKER"), map[string]any{"executed": true})
	fmt.Println("synthetic baseline junction producer")
	data, err = os.ReadFile("src/greeting.txt")
	if err != nil {
		panic(err)
	}
	passed := strings.TrimSpace(string(data)) == "hello"
	observation := map[string]any{"target": "greeting", "attempt": 1, "status": "pass"}
	if !passed {
		observation["status"], observation["violation"] = "violation", "missing-greeting"
	}
	report := map[string]any{
		"schema": "check-result/v1", "producer": "synthetic-junction", "producer_version": "1",
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
