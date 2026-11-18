package run

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Environment is the working directory and the environment that ha reads
// and starts child processes with. ha's entry point passes the process's own
// values; tests pass their own, so several invocations can run at once in one
// process without changing its working directory or environment.
//
// Each field left empty stands for the process value, read when it is used:
// Env nil is os.Environ(), Dir "" is the process working directory. The zero
// Environment is therefore exactly the behavior of os/exec and os.Getenv.
type Environment struct {
	// Dir is the working directory: relative PATH entries are found against it.
	Dir string
	// Env holds KEY=VALUE entries as os.Environ returns them.
	Env []string
}

// Environ returns a copy of the entries child processes receive, so callers
// may append to it.
func (e Environment) Environ() []string {
	if e.Env == nil {
		return os.Environ()
	}
	return append([]string(nil), e.Env...)
}

// LookupEnv is os.LookupEnv for this environment. When a name appears more
// than once the last entry counts, as it does for a child process (os/exec
// keeps the last one); on Windows names match regardless of letter case.
func (e Environment) LookupEnv(key string) (string, bool) {
	if e.Env == nil {
		return os.LookupEnv(key)
	}
	for i := len(e.Env) - 1; i >= 0; i-- {
		name, value, ok := splitEntry(e.Env[i])
		if ok && sameName(name, key) {
			return value, true
		}
	}
	return "", false
}

// Getenv is os.Getenv for this environment.
func (e Environment) Getenv(key string) string {
	value, _ := e.LookupEnv(key)
	return value
}

// Command is exec.Command for a child started in dir, with name resolved the
// way exec.Command and exec.Cmd.Start resolve it, but through this
// environment's PATH (and PATHEXT on Windows) and working directory instead
// of the process's. It sets Dir; the caller sets Env, usually from Environ.
// A name that cannot be resolved gives a Cmd whose Start returns the same
// error exec.Command would have stored.
func (e Environment) Command(dir, name string, args ...string) *exec.Cmd {
	if e.Env == nil && e.Dir == "" {
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		return cmd
	}
	path, err := name, error(nil)
	switch {
	case filepath.Base(name) == name:
		var found string
		found, err = e.LookPath(name)
		if found != "" {
			path = found
		}
	case runtime.GOOS == "windows":
		lookup := dir
		if filepath.IsAbs(name) {
			lookup = ""
		}
		var found string
		if found, err = e.lookExtensions(name, lookup); err == nil {
			path = found
		}
	}
	// The resolved path is kept, and Args[0] stays the name, as exec.Command
	// leaves them; Start returns Err before it starts anything.
	cmd := &exec.Cmd{Path: path, Args: append([]string{name}, args...), Err: err}
	cmd.Dir = dir
	return cmd
}

// at names a file for os.Stat: a relative name is relative to Dir.
func (e Environment) at(name string) string {
	if filepath.IsAbs(name) || filepath.VolumeName(name) != "" {
		return name
	}
	if e.Dir == "" {
		return "." + string(filepath.Separator) + name
	}
	return filepath.Join(e.Dir, name)
}

// errDotAllowed reports GODEBUG=execerrdot=0 in this environment, which makes
// exec.LookPath return an executable found relative to the working directory
// instead of exec.ErrDot.
func (e Environment) errDotAllowed() bool {
	allowed := false
	for _, setting := range strings.Split(e.Getenv("GODEBUG"), ",") {
		if value, ok := strings.CutPrefix(strings.TrimSpace(setting), "execerrdot="); ok {
			allowed = value == "0"
		}
	}
	return allowed
}

// splitEntry splits KEY=VALUE as os/exec does: on Windows a key may start
// with one "=" (the per-drive working directories).
func splitEntry(kv string) (string, string, bool) {
	i := strings.Index(kv, "=")
	if i == 0 {
		i = strings.Index(kv[1:], "=") + 1
	}
	if i <= 0 {
		return "", "", false
	}
	return kv[:i], kv[i+1:], true
}

func sameName(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func invalidLookPath(file string) bool {
	return file == "" || file == "." || file == ".."
}
