package run

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// The process's own directory and environment resolve every name as
// os/exec does.
func TestEnvironmentLookPathMatchesProcess(t *testing.T) {
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	env := Environment{Dir: dir, Env: os.Environ()}
	for _, name := range []string{"git", "go", "sh", "cmd", "run-no-such-program", "", ".", ".."} {
		want, wantErr := exec.LookPath(name)
		got, gotErr := env.LookPath(name)
		if got != want || (gotErr == nil) != (wantErr == nil) || (gotErr != nil && gotErr.Error() != wantErr.Error()) {
			t.Errorf("LookPath(%q) = %q, %v; exec.LookPath gives %q, %v", name, got, gotErr, want, wantErr)
		}
	}
}

// A passed PATH, not the process's, finds the executable, and a command
// built from the environment starts it.
func TestEnvironmentCommandUsesItsPath(t *testing.T) {
	bin, other := t.TempDir(), t.TempDir()
	name, body := "run-env-tool", "#!/bin/sh\necho from-env-path\n"
	file := name
	if runtime.GOOS == "windows" {
		file, body = name+".bat", "@echo from-env-path\r\n"
	}
	if err := os.WriteFile(filepath.Join(bin, file), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	base := []string{"PATH=" + other}
	if runtime.GOOS == "windows" {
		base = append(base, "SystemRoot="+os.Getenv("SystemRoot"), "PATHEXT=.COM;.EXE;.BAT;.CMD")
	}
	// The last entry of a repeated name counts, as for a child process.
	env := Environment{Dir: other, Env: append(append([]string(nil), base...), "PATH="+bin)}
	got, err := env.LookPath(name)
	if err != nil || got != filepath.Join(bin, file) {
		t.Fatalf("LookPath(%q) = %q, %v; want %q", name, got, err, filepath.Join(bin, file))
	}
	cmd := env.Command(other, name)
	cmd.Env = env.Environ()
	out, err := cmd.Output()
	if err != nil || string(out) == "" {
		t.Fatalf("command from the environment's PATH: %v: %q", err, out)
	}
	// Without the folder on PATH, the name is not found, as exec reports it.
	missing := Environment{Dir: other, Env: base}
	if _, err := missing.LookPath(name); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("LookPath without the folder: %v, want exec.ErrNotFound", err)
	}
	if err := missing.Command(other, name).Run(); !errors.Is(err, exec.ErrNotFound) {
		t.Fatalf("command without the folder: %v, want exec.ErrNotFound", err)
	}
	if v, ok := env.LookupEnv("PATH"); !ok || v != bin {
		t.Fatalf("LookupEnv(PATH) = %q, %v; want the last entry %q", v, ok, bin)
	}
	if _, ok := env.LookupEnv("RUN_ENV_UNSET"); ok {
		t.Fatal("LookupEnv found a name the environment does not have")
	}
}
