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

const mutexWrite = `package fx

import (
	"sync"
	"testing"
)

var (
	mu      sync.Mutex
	current string
)

func setCurrent(name string) { // @set
	mu.Lock()
	defer mu.Unlock()
	current = name // @write
}

func TestSet(t *testing.T) { // @test
	t.Parallel() // @parallel
	setCurrent(t.Name())
}

func TestRead(t *testing.T) {
	t.Parallel()
	mu.Lock()
	defer mu.Unlock()
	_ = current
}
`

// A helper that writes a package-level variable under a mutex is a finding,
// and the mutex's own Lock and Unlock are not.
func TestWriteThroughHelperUnderMutex(t *testing.T) {
	t.Parallel()
	dir, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": mutexWrite})
	line := func(marker string) string { return at(t, "fx_test.go", mutexWrite, marker) }
	want := "parallel test reaches write of current after t.Parallel() at " + line("@parallel") + ":\n" +
		"    TestSet (" + line("@test") + ") -> setCurrent (" + line("@set") + ") -> write of current (" + line("@write") + ")\n" +
		"parallelstate: violation: 1 finding(s) in 2 parallel function(s) of " + dir + "\n"
	if code != 1 || stdout != want || stderr != "" {
		t.Fatalf("exit %d\nstdout:\n%s\nwant:\n%s\nstderr:\n%s", code, stdout, want, stderr)
	}
}

const writeForms = `package fx

import (
	"encoding/json"
	"testing"
)

type config struct {
	dir    string
	limits map[string]int
}

var (
	settings config
	cache    = map[string]string{}
	table    [4]int
	counts   []int
	ptr      *config
	hits     int
)

func TestField(t *testing.T) { // @field
	t.Parallel()
	settings.dir = t.TempDir() // @fieldwrite
}

func TestMapIndex(t *testing.T) { // @map
	t.Parallel()
	cache["k"] = "v" // @mapwrite
}

func TestOtherForms(t *testing.T) { // @forms
	t.Parallel()
	settings.limits["n"]++    // @incdec
	table[1] += 2             // @opassign
	delete(cache, "k")        // @delete
	clear(counts)             // @clear
	copy(counts[1:], []int{}) // @copy
	(*ptr).dir = "x"          // @deref
	for hits = range counts { // @range
	}
	_ = json.Unmarshal(nil, &settings) // @address
}
`

func TestFieldIndexAndOtherWrites(t *testing.T) {
	t.Parallel()
	_, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": writeForms})
	line := func(marker string) string { return at(t, "fx_test.go", writeForms, marker) }
	forms := "    TestOtherForms (" + line("@forms") + ") -> write of "
	expect(t, code, stdout, stderr, 1,
		"    TestField ("+line("@field")+") -> write of settings ("+line("@fieldwrite")+")",
		"    TestMapIndex ("+line("@map")+") -> write of cache ("+line("@mapwrite")+")",
		forms+"settings ("+line("@incdec")+")",
		forms+"table ("+line("@opassign")+")",
		forms+"cache ("+line("@delete")+")",
		forms+"counts ("+line("@clear")+")",
		forms+"counts ("+line("@copy")+")",
		forms+"ptr ("+line("@deref")+")",
		forms+"hits ("+line("@range")+")",
		forms+"settings ("+line("@address")+")",
		"parallelstate: violation: 10 finding(s) in 3 parallel function(s) of ")
}

// Reads of package-level variables, and writes of local variables, fields
// of local copies and struct keys, are no finding: go/types tells a local
// variable from the package-level one it shadows.
const readOnly = `package fx

import "testing"

type config struct{ dir string }

var (
	defaults = map[string]string{"a": "b"}
	limit    = 3
	names    []string
	settings config
)

func lookup(key string) string { return defaults[key] }

func TestReadOnly(t *testing.T) {
	t.Parallel()
	if lookup("a") == "" || len(names) > limit {
		t.Fatal("unexpected")
	}
	local := settings
	local.dir = t.TempDir()
	copied := config{dir: settings.dir}
	copied.dir += "/x"
	m := map[string]int{}
	m["x"]++
	delete(m, "x")
	for _, name := range names {
		_ = name
	}
	limit := 4
	limit++
	_ = &local
	_ = limit
}
`

func TestReadOnlyUsePasses(t *testing.T) {
	t.Parallel()
	dir, code, stdout, stderr := observe(t, map[string]string{"fx_test.go": readOnly})
	want := "parallelstate: pass: 1 parallel function(s) in 1 file(s) of " + dir + "; none reaches a process-state change\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("exit %d\nstdout:\n%s\nwant:\n%s\nstderr:\n%s", code, stdout, want, stderr)
	}
}

// sharedHelper is a synthetic shared-build helper with the names that
// sharedBuildWriters lists.
const sharedHelper = `package fx

import (
	"os"
	"sync"
	"testing"
)

var sharedBuilds = struct {
	sync.Mutex
	folder string
	byPkg  map[string]*sharedBuild
}{byPkg: map[string]*sharedBuild{}}

type sharedBuild struct {
	once sync.Once
	path string
	err  error
}

func sharedBinary(t testing.TB, pkg string) string { // @binary
	sharedBuilds.Lock()
	build := sharedBuilds.byPkg[pkg]
	if build == nil {
		build = &sharedBuild{}
		sharedBuilds.byPkg[pkg] = build // @index
	}
	sharedBuilds.Unlock()
	build.once.Do(func() { build.err = build.make(pkg) })
	if build.err != nil {
		t.Fatal(build.err)
	}
	trace(pkg)
	return build.path
}

func (b *sharedBuild) make(pkg string) error { // @make
	folder, err := sharedBuildFolder()
	if err != nil {
		return err
	}
	b.path = folder + "/" + pkg
	return nil
}

func sharedBuildFolder() (string, error) { // @folder
	sharedBuilds.Lock()
	defer sharedBuilds.Unlock()
	if sharedBuilds.folder == "" {
		sharedBuilds.folder, _ = os.MkdirTemp("", "bin-") // @folderwrite
	}
	return sharedBuilds.folder, nil
}
`

const sharedTest = `package fx

import "testing"

func TestShared(t *testing.T) { // @test
	t.Parallel()
	_ = sharedBinary(t, "./cmd/ha")
}
`

const quietTrace = `package fx

func trace(string) {}
`

// Writes in the listed functions of shared_build_test.go, entered through
// sharedBinary, are allowed.
func TestAllowlistedSharedBuildWritesPass(t *testing.T) {
	t.Parallel()
	dir, code, stdout, stderr := observe(t, map[string]string{
		"shared_build_test.go": sharedHelper,
		"trace_test.go":        quietTrace,
		"fx_test.go":           sharedTest,
	})
	want := "parallelstate: pass: 1 parallel function(s) in 3 file(s) of " + dir + "; none reaches a process-state change\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("exit %d\nstdout:\n%s\nwant:\n%s\nstderr:\n%s", code, stdout, want, stderr)
	}
}

const loudTrace = `package fx

var lastBuilt string

func trace(pkg string) { lastBuilt = pkg } // @trace
`

const directFolder = `package fx

import "testing"

func TestFolder(t *testing.T) { // @direct
	t.Parallel()
	_, _ = sharedBuildFolder()
}
`

// The allowlist hides no other write: a function outside it that
// sharedBinary calls, a listed function reached without sharedBinary, and
// the same functions in a file of another name are findings.
func TestAllowlistHidesNoOtherWrite(t *testing.T) {
	t.Parallel()
	line := func(file, src, marker string) string { return at(t, file, src, marker) }
	t.Run("outside the list or entered elsewhere", func(t *testing.T) {
		t.Parallel()
		_, code, stdout, stderr := observe(t, map[string]string{
			"shared_build_test.go": sharedHelper,
			"trace_test.go":        loudTrace,
			"fx_test.go":           sharedTest,
			"direct_test.go":       directFolder,
		})
		shared := func(marker string) string { return line("shared_build_test.go", sharedHelper, marker) }
		expect(t, code, stdout, stderr, 1,
			"    TestShared ("+line("fx_test.go", sharedTest, "@test")+") -> sharedBinary ("+shared("@binary")+") -> "+
				"trace ("+line("trace_test.go", loudTrace, "@trace")+") -> write of lastBuilt ("+line("trace_test.go", loudTrace, "@trace")+")",
			"    TestFolder ("+line("direct_test.go", directFolder, "@direct")+") -> sharedBuildFolder ("+shared("@folder")+") -> "+
				"write of sharedBuilds ("+shared("@folderwrite")+")",
			"parallelstate: violation: 2 finding(s) in 2 parallel function(s) of ")
	})
	t.Run("another file", func(t *testing.T) {
		t.Parallel()
		_, code, stdout, stderr := observe(t, map[string]string{
			"helpers_test.go": sharedHelper,
			"trace_test.go":   quietTrace,
			"fx_test.go":      sharedTest,
		})
		helper := func(marker string) string { return line("helpers_test.go", sharedHelper, marker) }
		binary := "    TestShared (" + line("fx_test.go", sharedTest, "@test") + ") -> sharedBinary (" + helper("@binary") + ") -> "
		expect(t, code, stdout, stderr, 1,
			binary+"write of sharedBuilds ("+helper("@index")+")",
			binary+"(*sharedBuild).make ("+helper("@make")+") -> sharedBuildFolder ("+helper("@folder")+") -> write of sharedBuilds ("+helper("@folderwrite")+")",
			"parallelstate: violation: 2 finding(s) in 1 parallel function(s) of ")
	})
}

const otherPackageWrite = `package fx

import (
	"testing"

	"example.com/other"
)

func TestOther(t *testing.T) {
	t.Parallel()
	other.Value = 1
	other.Table["k"] = 2
	other.Count++
	delete(other.Table, "k")
	_ = &other.Value
	other.Set(t.Name())
}
`

const globalVar = `package fx

var Global int
`

const externalTest = `package fx_test

import (
	"testing"

	"example.com/fx"
)

func TestExternal(t *testing.T) {
	t.Parallel()
	fx.Global = 2
	fx.Global++
}
`

// Variables of other packages are not in scope: an imported package's, and,
// from the external test package, the package under test's.
func TestWriteInAnotherPackageIsNotReported(t *testing.T) {
	t.Parallel()
	dir, code, stdout, stderr := observe(t, map[string]string{
		"fx_test.go":     otherPackageWrite,
		"global.go":      globalVar,
		"export_test.go": externalTest,
	})
	want := "parallelstate: pass: 2 parallel function(s) in 3 file(s) of " + dir + "; none reaches a process-state change\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Fatalf("exit %d\nstdout:\n%s\nwant:\n%s\nstderr:\n%s", code, stdout, want, stderr)
	}
}
