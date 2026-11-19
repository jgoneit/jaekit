package run

import (
	"path"
	"strings"
	"testing"
)

// windowsBase is filepath.Base under Windows path syntax, on any system.
func windowsBase(p string) string {
	if p == "" {
		return "."
	}
	for len(p) > 0 && windowsSep(p[len(p)-1]) {
		p = p[:len(p)-1]
	}
	p = p[len(windowsVolume(p)):]
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		p = p[i+1:]
	}
	if p == "" {
		return `\`
	}
	return p
}

// Command resolves a name exactly when os/exec does, on either system:
// exec.Command searches PATH for a name that is its own base name, exec.Cmd.
// Start on Windows adds a %PATHEXT% extension to any other non-empty name,
// and nothing else is looked up before os.StartProcess, so an empty name is
// Start's "exec: no command" on Windows too.
func TestCommandLookupMatchesOSExec(t *testing.T) {
	for _, c := range []struct {
		goos string
		base func(string) string
		name string
		want nameLookup
	}{
		{"linux", path.Base, "tool", lookupPath},
		{"linux", path.Base, ".", lookupPath},
		{"linux", path.Base, "..", lookupPath},
		{"linux", path.Base, `.\tool`, lookupPath},
		{"linux", path.Base, "./missing-tool", lookupNone},
		{"linux", path.Base, "sub/tool", lookupNone},
		{"linux", path.Base, "/bin/tool", lookupNone},
		{"linux", path.Base, "", lookupNone},
		{"darwin", path.Base, "./tool", lookupNone},
		{"windows", windowsBase, "tool", lookupPath},
		{"windows", windowsBase, "tool.exe", lookupPath},
		{"windows", windowsBase, ".", lookupPath},
		{"windows", windowsBase, "", lookupNone},
		{"windows", windowsBase, "./missing-tool", lookupExtensions},
		{"windows", windowsBase, `.\tool`, lookupExtensions},
		{"windows", windowsBase, `sub\tool`, lookupExtensions},
		{"windows", windowsBase, `C:\bin\tool`, lookupExtensions},
		{"windows", windowsBase, `C:tool`, lookupExtensions},
		{"windows", windowsBase, `\bin\tool`, lookupExtensions},
		{"windows", windowsBase, `\\host\share\tool`, lookupExtensions},
	} {
		if got := commandLookup(c.goos, c.name, c.base); got != c.want {
			t.Errorf("commandLookup(%s, %q) = %d, want %d", c.goos, c.name, got, c.want)
		}
	}
}
