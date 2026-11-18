package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// at returns "file:line" of the first line of src that holds marker.
func at(t *testing.T, file, src, marker string) string {
	t.Helper()
	for i, line := range strings.Split(src, "\n") {
		if strings.Contains(line, marker) {
			return file + ":" + strconv.Itoa(i+1)
		}
	}
	t.Fatalf("%s: no line holds %q", file, marker)
	return ""
}

// observe writes a synthetic package to a new directory and checks it.
func observe(t *testing.T, files map[string]string) (dir string, code int, stdout, stderr string) {
	t.Helper()
	dir = t.TempDir()
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var out, errs bytes.Buffer
	code = check([]string{dir}, &out, &errs)
	return dir, code, out.String(), errs.String()
}

// expect checks the exit code, an empty stderr and that stdout holds every
// line of want.
func expect(t *testing.T, code int, stdout, stderr string, wantCode int, want ...string) {
	t.Helper()
	if code != wantCode || stderr != "" {
		t.Fatalf("exit %d want %d\nstdout:\n%s\nstderr:\n%s", code, wantCode, stdout, stderr)
	}
	for _, line := range want {
		if !strings.Contains(stdout, line) {
			t.Errorf("stdout lacks %q:\n%s", line, stdout)
		}
	}
}

const pointerReceiver = `package fx

import (
	"os"
	"testing"
)

type fixture struct {
	t   *testing.T
	dir string
}

func (f *fixture) ha(args ...string) { // @ha
	f.t.Helper()
	if err := os.Chdir(f.dir); err != nil { // @chdir
		f.t.Fatal(err)
	}
}

func TestPointer(t *testing.T) {
	t.Run("sub", func(t *testing.T) { // @lit
		t.Parallel() // @parallel
		f := &fixture{t: t, dir: t.TempDir()}
		f.ha("status")
	})
}
`

func TestPointerReceiverMethod(t *testing.T) {
	t.Parallel()
	dir, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": pointerReceiver})
	src := pointerReceiver
	want := "parallel test reaches os.Chdir after t.Parallel() at " + at(t, "fx_test.go", src, "@parallel") + ":\n" +
		"    func literal in TestPointer (" + at(t, "fx_test.go", src, "@lit") + ") -> " +
		"(*fixture).ha (" + at(t, "fx_test.go", src, "@ha") + ") -> " +
		"os.Chdir (" + at(t, "fx_test.go", src, "@chdir") + ")\n" +
		"parallelstate: violation: 1 finding(s) in 1 parallel function(s) of " + dir + "\n"
	if code != 1 || stdout != want || stderr != "" {
		t.Fatalf("exit %d\nstdout:\n%s\nwant:\n%s\nstderr:\n%s", code, stdout, want, stderr)
	}
}

const valueServer = `package fx

import "os"

type server struct{ dir string }

func (s server) do(cmd string) error { // @do
	return os.Chdir(s.dir) // @chdir
}
`

const valueTest = `package fx

import "testing"

func TestValue(t *testing.T) { // @test
	t.Parallel()
	var s server
	s.dir = t.TempDir()
	if err := s.do("status"); err != nil {
		t.Fatal(err)
	}
}
`

func TestValueReceiverMethod(t *testing.T) {
	t.Parallel()
	_, code, stdout, stderr := observe(t, map[string]string{"server.go": valueServer, "fx_test.go": valueTest})
	expect(t, code, stdout, stderr, 1,
		"    TestValue ("+at(t, "fx_test.go", valueTest, "@test")+") -> "+
			"server.do ("+at(t, "server.go", valueServer, "@do")+") -> "+
			"os.Chdir ("+at(t, "server.go", valueServer, "@chdir")+")")
}

const fieldMethod = `package fx

import (
	"os"
	"testing"
)

type fixture struct{ dir string }

func (f *fixture) ha() { _ = os.Chdir(f.dir) } // @ha

type suite struct {
	name string
	f    *fixture
}

func newSuite(dir string) *suite { return &suite{f: &fixture{dir: dir}} }

func TestField(t *testing.T) { // @field
	t.Parallel()
	s := &suite{f: &fixture{dir: t.TempDir()}}
	s.f.ha()
}

func TestResult(t *testing.T) { // @result
	t.Parallel()
	newSuite(t.TempDir()).f.ha()
}
`

func TestFieldMethod(t *testing.T) {
	t.Parallel()
	_, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": fieldMethod})
	tail := " -> (*fixture).ha (" + at(t, "fx_test.go", fieldMethod, "@ha") + ") -> os.Chdir (" + at(t, "fx_test.go", fieldMethod, "@ha") + ")"
	expect(t, code, stdout, stderr, 1,
		"    TestField ("+at(t, "fx_test.go", fieldMethod, "@field")+")"+tail,
		"    TestResult ("+at(t, "fx_test.go", fieldMethod, "@result")+")"+tail,
		"parallelstate: violation: 2 finding(s) in 2 parallel function(s) of ")
}

const unresolved = `package fx

import (
	"os"
	"testing"

	"example.com/other"
)

type fixture struct{ dir string }

func (f *fixture) ha() error { // @ha
	return os.Chdir(f.dir) // @chdir
}

type quiet struct{}

func (quiet) ha() error { return nil }

func TestUnknown(t *testing.T) { // @test
	t.Parallel()
	x := other.Open(t.TempDir())
	_ = x.ha()
}
`

func TestUnresolvableReceiver(t *testing.T) {
	t.Parallel()
	_, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": unresolved})
	expect(t, code, stdout, stderr, 1,
		"    TestUnknown ("+at(t, "fx_test.go", unresolved, "@test")+") -> "+
			"(*fixture).ha by name ("+at(t, "fx_test.go", unresolved, "@ha")+") -> "+
			"os.Chdir ("+at(t, "fx_test.go", unresolved, "@chdir")+")",
		"parallelstate: violation: 1 finding(s) in 1 parallel function(s) of ")
}

const quietMethods = `package fx

import (
	"os"
	"strings"
	"testing"
)

type fixture struct {
	t   *testing.T
	dir string
}

func (f *fixture) put(name, text string) {
	f.t.Helper()
	if err := os.WriteFile(f.path(name), []byte(text), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f fixture) path(name string) string { return f.dir + "/" + name }

func (f *fixture) read(name string) string {
	data, err := os.ReadFile(f.path(name))
	if err != nil {
		f.t.Fatal(err)
	}
	return strings.TrimSpace(string(data))
}

func TestQuiet(t *testing.T) {
	t.Parallel()
	f := &fixture{t: t, dir: t.TempDir()}
	f.put("a.txt", "hello\n")
	if got := f.read("a.txt"); got != "hello" {
		t.Fatalf("read %q", got)
	}
}
`

func TestMethodsThatChangeNothingPass(t *testing.T) {
	t.Parallel()
	dir, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": quietMethods})
	want := "parallelstate: pass: 1 parallel function(s) in 1 file(s) of " + dir + "; none reaches a process-state change\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("exit %d\nstdout:\n%s\nwant:\n%s\nstderr:\n%s", code, stdout, want, stderr)
	}
}

// A known receiver type follows only its own method: not a same-named
// method of another type, not a method of package testing, and not a field
// holding a function.
const resolvedOnly = `package fx

import (
	"os"
	"testing"
)

type quiet struct{ dir string }

func (q quiet) ha() string { return q.dir }

type loud struct{ dir string }

func (l *loud) ha() string { _ = os.Chdir(l.dir); return l.dir }

func (l *loud) Cleanup(func()) { os.Setenv("HOME", l.dir) }

func (l *loud) run() { os.Clearenv() }

type job struct{ run func() }

func TestPrecise(t *testing.T) {
	t.Parallel()
	q := quiet{dir: t.TempDir()}
	_ = q.ha()
	t.Cleanup(func() {})
	j := job{run: func() {}}
	j.run()
}
`

func TestResolvedReceiverFollowsOnlyItsMethod(t *testing.T) {
	t.Parallel()
	_, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": resolvedOnly})
	expect(t, code, stdout, stderr, 0, "parallelstate: pass: 1 parallel function(s) in 1 file(s) of ")
}

// Each build has its own declaration of enter; go/types resolves the call to
// the first one, and every declaration is followed.
const buildsTest = `package fx

import "testing"

type fixture struct{ dir string }

func TestBuilds(t *testing.T) { // @test
	t.Parallel()
	f := &fixture{dir: t.TempDir()}
	f.enter()
}
`

const buildsDarwin = `//go:build darwin

package fx

func (f *fixture) enter() {}
`

const buildsLinux = `//go:build linux

package fx

import "os"

func (f *fixture) enter() { _ = os.Chdir(f.dir) } // @enter
`

func TestEveryBuildsMethodIsFollowed(t *testing.T) {
	t.Parallel()
	_, code, stdout, stderr := observe(t, map[string]string{
		"fx_test.go":           buildsTest,
		"enter_darwin_test.go": buildsDarwin,
		"enter_linux_test.go":  buildsLinux,
	})
	enter := at(t, "enter_linux_test.go", buildsLinux, "@enter")
	expect(t, code, stdout, stderr, 1,
		"    TestBuilds ("+at(t, "fx_test.go", buildsTest, "@test")+") -> (*fixture).enter ("+enter+") -> os.Chdir ("+enter+")")
}

const otherSelections = `package fx

import (
	"os"
	"testing"
)

type fixture struct{ dir string }

func (f *fixture) ha() { _ = os.Chdir(f.dir) } // @ha

func (f *fixture) env(t *testing.T) { t.Setenv("HOME", f.dir) } // @env

func (f *fixture) setup(t *testing.T) { f.env(t); reset(f) } // @setup

func reset(f *fixture) { os.Unsetenv("PWD") } // @reset

type outer struct{ *fixture }

func each(fn func()) { fn() }

func TestPromoted(t *testing.T) { // @promoted
	t.Parallel()
	o := outer{&fixture{dir: t.TempDir()}}
	o.ha()
}

func TestMethodValue(t *testing.T) { // @value
	t.Parallel()
	f := &fixture{dir: t.TempDir()}
	each(f.ha)
}

func TestMethodExpression(t *testing.T) { // @expression
	t.Parallel()
	(*fixture).ha(&fixture{dir: t.TempDir()})
}

func TestChain(t *testing.T) { // @chain
	t.Parallel()
	f := &fixture{dir: t.TempDir()}
	f.setup(t)
}
`

func TestPromotedValuesExpressionsAndChains(t *testing.T) {
	t.Parallel()
	_, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": otherSelections})
	src := otherSelections
	line := func(marker string) string { return at(t, "fx_test.go", src, marker) }
	ha := " -> (*fixture).ha (" + line("@ha") + ") -> os.Chdir (" + line("@ha") + ")"
	setup := " -> (*fixture).setup (" + line("@setup") + ")"
	expect(t, code, stdout, stderr, 1,
		"    TestPromoted ("+line("@promoted")+")"+ha,
		"    TestMethodValue ("+line("@value")+")"+ha,
		"    TestMethodExpression ("+line("@expression")+")"+ha,
		"    TestChain ("+line("@chain")+")"+setup+" -> (*fixture).env ("+line("@env")+") -> t.Setenv ("+line("@env")+")",
		"    TestChain ("+line("@chain")+")"+setup+" -> reset ("+line("@reset")+") -> os.Unsetenv ("+line("@reset")+")",
		"parallelstate: violation: 5 finding(s) in 4 parallel function(s) of ")
}

func TestInputFailuresExitTwo(t *testing.T) {
	t.Parallel()
	for name, files := range map[string]map[string]string{
		"parse error": {"fx_test.go": quietMethods, "bad_test.go": "package fx\n\nfunc (f *fixture) ha( {\n"},
		"no go file":  {"README": "synthetic\n"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			_, code, stdout, stderr := observe(t, files)
			if code != 2 || stdout != "" || !strings.HasPrefix(stderr, "parallelstate: not observed: ") {
				t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, stdout, stderr)
			}
		})
	}
	t.Run("missing directory", func(t *testing.T) {
		t.Parallel()
		var out, errs bytes.Buffer
		code := check([]string{filepath.Join(t.TempDir(), "missing")}, &out, &errs)
		if code != 2 || out.Len() != 0 || !strings.HasPrefix(errs.String(), "parallelstate: not observed: ") {
			t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errs.String())
		}
	})
	t.Run("no argument", func(t *testing.T) {
		t.Parallel()
		var out, errs bytes.Buffer
		if code := check(nil, &out, &errs); code != 2 || out.Len() != 0 || !strings.HasPrefix(errs.String(), "usage: parallelstate DIR") {
			t.Fatalf("exit %d\nstdout:\n%s\nstderr:\n%s", code, out.String(), errs.String())
		}
	})
}
