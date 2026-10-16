// Package gittest holds Git settings for test processes that create
// repositories.
package gittest

import (
	"os"
	"strconv"
)

// DisableAutoMaintenance keeps every Git command started by the current test
// process, including the ones ha starts, from running automatic maintenance.
// Git detaches that maintenance by default, so after a fixture commit it can
// keep writing to .git while a test reads the directory or while t.TempDir
// removes it. Settings already passed through GIT_CONFIG_COUNT are kept.
func DisableAutoMaintenance() {
	n, err := strconv.Atoi(os.Getenv("GIT_CONFIG_COUNT"))
	if err != nil || n < 0 {
		n = 0
	}
	for _, setting := range [][2]string{{"maintenance.auto", "false"}, {"gc.auto", "0"}} {
		os.Setenv("GIT_CONFIG_KEY_"+strconv.Itoa(n), setting[0])
		os.Setenv("GIT_CONFIG_VALUE_"+strconv.Itoa(n), setting[1])
		n++
	}
	os.Setenv("GIT_CONFIG_COUNT", strconv.Itoa(n))
}
