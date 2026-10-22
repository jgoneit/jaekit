// Command spar runs independent review rounds on pinned commits and keeps
// the records a gate decides from. It is a separate executable from ha:
// Seal Core never imports it.
package main

import (
	"os"

	"github.com/jgoneit/jaekit/internal/spar"
)

// version is set at build time with -ldflags "-X main.version=...".
var version = "0.1.7-dev"

func main() {
	os.Exit(spar.Main(version, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Getenv))
}
