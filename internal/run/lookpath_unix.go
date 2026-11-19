//go:build !windows

package run

import (
	"os/exec"
	"path/filepath"
	"strings"
)

// LookPath is exec.LookPath for this environment: a name without a slash is
// searched in its PATH, and a relative PATH entry ("" is ".") against its
// working directory. An executable found through a relative entry is
// exec.ErrDot unless the environment sets GODEBUG=execerrdot=0.
func (e Environment) LookPath(file string) (string, error) {
	if e.Env == nil && e.Dir == "" {
		return exec.LookPath(file)
	}
	if invalidLookPath(file) {
		return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
	}
	if strings.Contains(file, "/") {
		if err := e.executable(file); err != nil {
			return "", &exec.Error{Name: file, Err: err}
		}
		return file, nil
	}
	for _, dir := range filepath.SplitList(e.Getenv("PATH")) {
		if dir == "" {
			dir = "." // Unix shell semantics
		}
		path := filepath.Join(dir, file)
		if e.executable(path) == nil {
			if !filepath.IsAbs(path) && !e.errDotAllowed() {
				return path, &exec.Error{Name: file, Err: exec.ErrDot}
			}
			return path, nil
		}
	}
	return "", &exec.Error{Name: file, Err: exec.ErrNotFound}
}

// executable is exec's check that a file can be run: the standard library's
// own check, given a name with a slash so that it searches nothing.
func (e Environment) executable(name string) error {
	_, err := exec.LookPath(e.at(name))
	if failure, ok := err.(*exec.Error); ok {
		return failure.Err
	}
	return err
}

// lookExtensions resolves nothing outside Windows, as in os/exec.
func (e Environment) lookExtensions(path, _ string) (string, error) { return path, nil }
