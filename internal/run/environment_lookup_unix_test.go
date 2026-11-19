//go:build !windows

package run

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const lookPathHelper = "RUN_LOOKPATH_HELPER"

// A relative PATH entry is found from a working directory reached through a
// symlink the way the kernel finds it: ".." leaves the directory the symlink
// points to, not the symlink's own parent. The port gives what exec.LookPath
// gives in a child process started in that directory, with and without
// GODEBUG=execerrdot=0: the same path and error for a tool that is in ../bin
// only physically, and for one that is there only lexically.
func TestEnvironmentLookPathFromSymlinkedDir(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{filepath.Join("real", "work"), filepath.Join("real", "bin"), "bin"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for file, body := range map[string]string{
		filepath.Join("real", "bin", "tool"): "#!/bin/sh\necho physical\n",
		filepath.Join("bin", "other"):        "#!/bin/sh\necho lexical\n",
	} {
		if err := os.WriteFile(filepath.Join(root, file), []byte(body), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(filepath.Join("real", "work"), link); err != nil {
		t.Fatal(err)
	}
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	names := []string{"tool", "other"}
	for _, godebug := range []string{"", "execerrdot=0"} {
		vars := []string{"PATH=../bin", lookPathHelper + "=1"}
		if godebug != "" {
			vars = append(vars, "GODEBUG="+godebug)
		}
		child := exec.Command(self, append([]string{"-test.run=^TestEnvironmentLookPathHelper$", "--"}, names...)...)
		child.Dir, child.Env = link, vars
		out, err := child.Output()
		if err != nil {
			t.Fatalf("setup: exec.LookPath in a child started in %s: %v: %q", link, err, out)
		}
		want := string(out)
		if !strings.HasPrefix(want, fmt.Sprintf("tool: %q ", "../bin/tool")) || !strings.Contains(want, `other: "" `) {
			t.Fatalf("setup: exec.LookPath from %s with PATH=../bin (GODEBUG=%q) gave %q; want ../bin/tool found and other not", link, godebug, want)
		}
		env := Environment{Dir: link, Env: vars}
		if got := lookPathReport(env.LookPath, names...); got != want {
			t.Errorf("Environment{Dir: %s}.LookPath with PATH=../bin (GODEBUG=%q) gave\n%s; exec.LookPath in that directory gives\n%s", link, godebug, got, want)
		}
		if godebug == "" {
			continue
		}
		// Allowed, the command starts the tool ../bin names physically.
		cmd := env.Command(link, "tool")
		cmd.Env = env.Environ()
		if ran, err := cmd.Output(); err != nil || string(ran) != "physical\n" {
			t.Errorf("Command(%s, tool) with PATH=../bin and GODEBUG=execerrdot=0 ran %q, %v; want the tool in real/bin", link, ran, err)
		}
	}
}

// TestEnvironmentLookPathHelper reports exec.LookPath for each argument when
// started as the child of TestEnvironmentLookPathFromSymlinkedDir.
func TestEnvironmentLookPathHelper(t *testing.T) {
	if os.Getenv(lookPathHelper) != "1" {
		return
	}
	fmt.Print(lookPathReport(exec.LookPath, flag.Args()...))
	os.Exit(0)
}

// lookPathReport is one line per name: the path and the error lookPath
// returns for it.
func lookPathReport(lookPath func(string) (string, error), names ...string) string {
	var b strings.Builder
	for _, name := range names {
		path, err := lookPath(name)
		fmt.Fprintf(&b, "%s: %q %v\n", name, path, err)
	}
	return b.String()
}
