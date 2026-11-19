package main

import (
	"bytes"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/jgoneit/jaekit/internal/run"
)

// Tests that run concurrently cannot change the process's environment or
// working directory (t.Setenv, t.Chdir). A fixture instead keeps its own
// environment and directory and passes them to every git and ha command it
// runs, as t.Setenv and t.Chdir would have left them for those commands.

// envWith is base with each KEY=VALUE entry of set in place of base's entry
// of that name (names ignore letter case on Windows), set last, as
// t.Setenv(KEY, VALUE) leaves the environment of a child process.
func envWith(base []string, set ...string) []string {
	replaced := map[string]bool{}
	for _, kv := range set {
		replaced[envName(kv)] = true
	}
	out := make([]string, 0, len(base)+len(set))
	for _, kv := range base {
		if !replaced[envName(kv)] {
			out = append(out, kv)
		}
	}
	return append(out, set...)
}

// envIn is envWith with PWD naming dir, which t.Chdir(dir) also sets outside
// Windows: commands then see the directory they run in as their PWD.
func envIn(dir string, base []string, set ...string) []string {
	if runtime.GOOS != "windows" && runtime.GOOS != "plan9" {
		set = append(append([]string(nil), set...), "PWD="+dir)
	}
	return envWith(base, set...)
}

func envName(kv string) string {
	name, _, _ := strings.Cut(kv, "=")
	if strings.HasPrefix(kv, "=") { // Windows per-drive directories
		name, _, _ = strings.Cut(kv[1:], "=")
		name = "=" + name
	}
	if runtime.GOOS == "windows" {
		return strings.ToUpper(name)
	}
	return name
}

// envValue is os.Getenv for env.
func envValue(env []string, key string) string {
	return run.Environment{Env: env}.Getenv(key)
}

// envCommand is exec.Command for a child started in dir with env: name is
// found through env's PATH, as exec.Command finds it through the process's
// after t.Setenv("PATH", ...).
func envCommand(dir string, env []string, name string, args ...string) *exec.Cmd {
	cmd := run.Environment{Dir: dir, Env: env}.Command(dir, name, args...)
	cmd.Env = env
	return cmd
}

// envLookPath is exec.LookPath through env's PATH.
func envLookPath(env []string, name string) (string, error) {
	dir, _ := os.Getwd()
	return run.Environment{Dir: dir, Env: env}.LookPath(name)
}

// haIn runs ha in dir with env and returns its exit code, standard output
// and standard error, as runMain would after t.Chdir(dir) and t.Setenv.
func haIn(dir string, env []string, args ...string) (int, string, string) {
	var out, errb bytes.Buffer
	code := runWith(dir, env, args, &out, &errb)
	return code, out.String(), errb.String()
}

// gitFixtureEnv is the environment the synthetic repositories commit with:
// no user or system configuration and fixed author and committer names.
func gitFixtureEnv(config, name, email string) []string {
	return []string{"GIT_CONFIG_GLOBAL=" + config, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=" + name, "GIT_COMMITTER_NAME=" + name,
		"GIT_AUTHOR_EMAIL=" + email, "GIT_COMMITTER_EMAIL=" + email}
}
