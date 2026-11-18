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
// error exec.Command and exec.Cmd.Start would have returned.
func (e Environment) Command(dir, name string, args ...string) *exec.Cmd {
	lookup := commandLookup(runtime.GOOS, name, filepath.Base)
	if lookup == lookupNone || (e.Env == nil && e.Dir == "") {
		// exec.Command's own Cmd. For a name os/exec looks nothing up for
		// (a name with a slash on Unix, an empty name), os.StartProcess or
		// Start itself reports a missing or unusable file, so the error is
		// os/exec's by construction rather than a copy of it.
		cmd := exec.Command(name, args...)
		cmd.Dir = dir
		return cmd
	}
	var path string
	var err error
	if lookup == lookupPath {
		path, err = e.LookPath(name)
	} else {
		// Start would add the extension relative to dir; an absolute name
		// has a volume and is looked up as it is, as in os/exec.
		path, err = e.lookExtensions(name, dir)
	}
	if path == "" {
		path = name
	}
	// The resolved path is kept, and Args[0] stays the name, as exec.Command
	// leaves them; Start returns Err before it starts anything.
	cmd := &exec.Cmd{Path: path, Args: append([]string{name}, args...), Err: err}
	cmd.Dir = dir
	return cmd
}

// nameLookup is what os/exec resolves for a command name before it starts the
// process: exec.Command searches PATH for a name without a path separator,
// and on Windows exec.Cmd.Start adds a %PATHEXT% extension to any other
// non-empty name, relative to Cmd.Dir. Nothing is looked up for the rest:
// os.StartProcess opens such a name as it is, and Start refuses an empty
// one ("exec: no command").
type nameLookup int

const (
	lookupNone nameLookup = iota
	lookupPath
	lookupExtensions
)

// commandLookup is the lookup os/exec on goos makes for name, where base is
// that system's filepath.Base.
func commandLookup(goos, name string, base func(string) string) nameLookup {
	switch {
	case base(name) == name:
		return lookupPath
	case goos == "windows" && name != "":
		return lookupExtensions
	}
	return lookupNone
}

// at names a file for os.Stat: the file the OS opens for name in a process
// whose working directory is Dir.
func (e Environment) at(name string) string {
	if e.Dir == "" && filepath.VolumeName(name) == "" && (name == "" || !os.IsPathSeparator(name[0])) {
		// The process's own working directory: "." keeps a lookup from
		// searching PATH.
		return "." + string(filepath.Separator) + name
	}
	return resolveIn(e.Dir, name, filepath.VolumeName, os.IsPathSeparator, filepath.Join)
}

// resolveIn is the path the OS opens for name in a process whose working
// directory is dir ("" is the process's own), under one system's path
// syntax: volume is filepath.VolumeName, isSep is os.IsPathSeparator and
// join is filepath.Join. A name rooted without a drive (\x on Windows) is on
// dir's volume; a drive-relative name (C:x) is in dir only when dir is on
// that drive, and otherwise left for the drive's own current directory; an
// absolute, UNC or device path is unchanged; any other name is in dir.
func resolveIn(dir, name string, volume func(string) string, isSep func(byte) bool, join func(...string) string) string {
	vol := volume(name)
	rest := name[len(vol):]
	rooted := rest != "" && isSep(rest[0])
	switch {
	case vol != "" && (rooted || isSep(vol[0])):
		return name
	case vol != "":
		if dir != "" && strings.EqualFold(vol, volume(dir)) {
			return join(dir, rest)
		}
		return name
	case rooted:
		return volume(dir) + name
	}
	return join(dir, name)
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
