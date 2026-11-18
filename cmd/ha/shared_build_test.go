package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jgoneit/jaekit/internal/run"
)

// The working directory and environment of the test process as it started,
// before any test changed them: shared builds run with these, so a test that
// narrowed PATH or set GOFLAGS does not change what every test gets.
var (
	sharedStartDir, sharedStartDirErr = os.Getwd()
	sharedStartEnv                    = os.Environ()
)

// sharedBuilds are the binaries this test process built, each in its own
// folder under one process-level temporary folder that TestMain removes.
var sharedBuilds = struct {
	sync.Mutex
	folder string
	err    error
	made   bool
	byPkg  map[string]*sharedBuild
}{byPkg: map[string]*sharedBuild{}}

type sharedBuild struct {
	once sync.Once
	path string
	err  error
}

// sharedBinary builds the Go package pkg (a module path or a path relative to
// the module root, such as ./cmd/spar) once per test process and returns the
// read-only binary. Concurrent callers wait for the one build; when it
// failed, every caller fails with its reason. The binary outlives the test
// that first asked for it.
func sharedBinary(t testing.TB, pkg string) string {
	t.Helper()
	sharedBuilds.Lock()
	build := sharedBuilds.byPkg[pkg]
	if build == nil {
		build = &sharedBuild{}
		sharedBuilds.byPkg[pkg] = build
	}
	sharedBuilds.Unlock()
	build.once.Do(func() { build.path, build.err = buildShared(pkg) })
	if build.err != nil {
		t.Fatalf("setup: shared build of %s: %v", pkg, build.err)
	}
	return build.path
}

func buildShared(pkg string) (string, error) {
	folder, err := sharedBuildFolder()
	if err != nil {
		return "", err
	}
	root, err := sharedModuleRoot()
	if err != nil {
		return "", err
	}
	// Its own folder keeps the binary's usual name (ha, spar).
	dir, err := os.MkdirTemp(folder, "pkg-")
	if err != nil {
		return "", err
	}
	name := filepath.Base(filepath.FromSlash(pkg))
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(dir, name)
	env := run.Environment{Dir: sharedStartDir, Env: sharedStartEnv}
	cmd := env.Command(root, "go", "build", "-mod=readonly", "-o", binary, pkg)
	cmd.Env = env.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("go build -mod=readonly %s: %v: %s", pkg, err, strings.TrimSpace(string(out)))
	}
	if err := os.Chmod(binary, 0o555); err != nil {
		return "", err
	}
	return binary, nil
}

// sharedBuildFolder makes the process-level folder on first use.
func sharedBuildFolder() (string, error) {
	sharedBuilds.Lock()
	defer sharedBuilds.Unlock()
	if !sharedBuilds.made {
		sharedBuilds.made = true
		sharedBuilds.folder, sharedBuilds.err = os.MkdirTemp("", "ha-test-binaries-")
	}
	return sharedBuilds.folder, sharedBuilds.err
}

// sharedModuleRoot is the folder holding go.mod above the package directory
// the test process started in.
func sharedModuleRoot() (string, error) {
	if sharedStartDirErr != nil {
		return "", sharedStartDirErr
	}
	for dir := sharedStartDir; ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil && info.Mode().IsRegular() {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			return "", fmt.Errorf("no go.mod above %s", sharedStartDir)
		}
	}
}

// removeSharedBinaries deletes the process-level folder after every test
// ended. The read-only binaries are made writable first, as Windows needs.
func removeSharedBinaries() {
	sharedBuilds.Lock()
	defer sharedBuilds.Unlock()
	if sharedBuilds.folder == "" {
		return
	}
	_ = filepath.WalkDir(sharedBuilds.folder, func(path string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(sharedBuilds.folder)
	sharedBuilds.folder = ""
}
