package run

import (
	"os"
	"path"
	"path/filepath"
	"strings"
	"testing"
)

// Windows path syntax for resolveIn on any system: separators, a drive
// letter or a \\host\share (or \\?\C:) volume, and a join that adds one
// separator. The cases avoid names that filepath.Join would also clean.
func windowsSep(c byte) bool { return c == '\\' || c == '/' }

func windowsVolume(p string) string {
	switch {
	case len(p) >= 2 && p[1] == ':':
		return p[:2]
	case len(p) >= 2 && windowsSep(p[0]) && windowsSep(p[1]):
		seps := 0
		for i := 2; i < len(p); i++ {
			if windowsSep(p[i]) {
				if seps++; seps == 2 {
					return p[:i]
				}
			}
		}
		return p
	}
	return ""
}

func windowsJoin(elem ...string) string {
	out := ""
	for _, e := range elem {
		switch {
		case e == "":
		case out == "":
			out = e
		default:
			out = strings.TrimRight(out, `\/`) + `\` + e
		}
	}
	return out
}

func TestResolveInWindowsForms(t *testing.T) {
	for _, c := range []struct{ form, dir, name, want string }{
		{"rooted without a drive", `C:\repo`, `\bin\tool.exe`, `C:\bin\tool.exe`},
		{"rooted with slashes", `C:\repo`, `/bin/tool`, `C:/bin/tool`},
		{"a rooted PATH entry", `C:\repo`, `\tools\x.exe`, `C:\tools\x.exe`},
		{"rooted on a share", `\\host\share\repo`, `\bin\tool`, `\\host\share\bin\tool`},
		{"rooted, process directory", ``, `\bin\tool`, `\bin\tool`},
		{"drive-relative, same drive", `C:\repo`, `C:bin\tool`, `C:\repo\bin\tool`},
		{"drive-relative, same drive in lower case", `C:\repo`, `c:bin\tool`, `C:\repo\bin\tool`},
		{"the drive alone, same drive", `C:\repo`, `C:`, `C:\repo`},
		{"drive-relative, other drive", `C:\repo`, `D:bin\tool`, `D:bin\tool`},
		{"drive-relative, share directory", `\\host\share\repo`, `C:bin\tool`, `C:bin\tool`},
		{"drive-relative, process directory", ``, `C:bin\tool`, `C:bin\tool`},
		{"absolute, same drive", `C:\repo`, `C:\bin\tool`, `C:\bin\tool`},
		{"absolute, other drive", `C:\repo`, `D:\bin\tool`, `D:\bin\tool`},
		{"UNC", `C:\repo`, `\\host\share\bin\tool`, `\\host\share\bin\tool`},
		{"device path", `C:\repo`, `\\?\C:\bin\tool`, `\\?\C:\bin\tool`},
		{"plain relative", `C:\repo`, `bin\tool`, `C:\repo\bin\tool`},
		{"plain relative on a share", `\\host\share\repo`, `tool.exe`, `\\host\share\repo\tool.exe`},
	} {
		if got := resolveIn(c.dir, c.name, windowsVolume, windowsSep, windowsJoin); got != c.want {
			t.Errorf("%s: resolveIn(%q, %q) = %q, want %q", c.form, c.dir, c.name, got, c.want)
		}
	}
}

func TestResolveInUnixForms(t *testing.T) {
	none := func(string) string { return "" }
	slash := func(c byte) bool { return c == '/' }
	for _, c := range []struct{ dir, name, want string }{
		{"/repo", "/bin/tool", "/bin/tool"},
		{"/repo", "bin/tool", "/repo/bin/tool"},
		{"/repo", `\bin\tool`, `/repo/\bin\tool`},
		{"/repo", "C:tool", "/repo/C:tool"},
	} {
		if got := resolveIn(c.dir, c.name, none, slash, path.Join); got != c.want {
			t.Errorf("resolveIn(%q, %q) = %q, want %q", c.dir, c.name, got, c.want)
		}
	}
}

// at uses this system's own path syntax.
func TestEnvironmentAtNativeForms(t *testing.T) {
	dir := t.TempDir()
	sep := string(filepath.Separator)
	rooted := sep + "bin" + sep + "tool"
	for _, c := range []struct {
		dir, name, want string
	}{
		{dir, rooted, filepath.VolumeName(dir) + rooted},
		{dir, "tool", filepath.Join(dir, "tool")},
		{dir, filepath.Join(dir, "tool"), filepath.Join(dir, "tool")},
		{"", rooted, rooted},
		{"", "tool", "." + sep + "tool"},
	} {
		if got := (Environment{Dir: c.dir, Env: os.Environ()}).at(c.name); got != c.want {
			t.Errorf("Environment{Dir: %q}.at(%q) = %q, want %q", c.dir, c.name, got, c.want)
		}
	}
}
