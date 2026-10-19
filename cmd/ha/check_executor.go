package main

import (
	"io"
	"time"

	"github.com/jgoneit/jaekit/internal/run"
)

type checkExecutor func(string, []string, time.Duration, string, []string) (run.Result, error)

// An instance-local dependency lets command tests observe execution failures
// without adding a product flag, environment switch, or mutable global hook.
// Production callers always use the existing executor and timeout unchanged.
// A non-nil stdout also receives the command's standard output (a go-test-json
// run); an injected executor is called without it.
func (c *cli) executeCheckOutput(dir string, argv []string, timeout time.Duration, logPath string, env []string, stdout io.Writer) (run.Result, error) {
	if c.execute != nil {
		return c.execute(dir, argv, timeout, logPath, env)
	}
	if stdout != nil {
		return run.ExecWithStdout(dir, argv, timeout, logPath, env, stdout)
	}
	return run.ExecWithEnv(dir, argv, timeout, logPath, env)
}
