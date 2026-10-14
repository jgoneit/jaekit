package main

import (
	"time"

	"github.com/jgoneit/jaekit/internal/run"
)

type checkExecutor func(string, []string, time.Duration, string, []string) (run.Result, error)

// An instance-local dependency lets command tests observe execution failures
// without adding a product flag, environment switch, or mutable global hook.
// Production callers always use the existing executor and timeout unchanged.
func (c *cli) executeCheck(dir string, argv []string, timeout time.Duration, logPath string, env []string) (run.Result, error) {
	if c.execute != nil {
		return c.execute(dir, argv, timeout, logPath, env)
	}
	return run.ExecWithEnv(dir, argv, timeout, logPath, env)
}
