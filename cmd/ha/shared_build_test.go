package main

import (
	"crypto/sha256"
	"fmt"
	"io"
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

// sharedChangedViolation starts the message of every caller, and the line
// TestMain prints, for a shared binary that is no longer the file built.
const sharedChangedViolation = "jaekit:violation=shared-binary-changed: "

// sharedBuilds are the binaries this test process built, each in its own
// folder under one process-level temporary folder that TestMain removes.
// built lists the builds that succeeded, for TestMain to check.
var sharedBuilds = struct {
	sync.Mutex
	folder string
	err    error
	made   bool
	byPkg  map[string]*sharedBuild
	built  []*sharedBuild
}{byPkg: map[string]*sharedBuild{}}

type sharedBuild struct {
	once sync.Once
	pkg  string
	path string
	// info and digest are the binary right after the build, made read-only:
	// every call, and TestMain, checks that the file is still this one.
	info   os.FileInfo
	digest [sha256.Size]byte
	err    error
}

// sharedBinary builds the Go package pkg (a module path or a path relative to
// the module root, such as ./cmd/spar) once per test process and returns the
// read-only binary in its read-only folder. Concurrent callers wait for the
// one build; when it failed, every caller fails with its reason. The binary
// outlives the test that first asked for it, and no test may change it:
// every call checks that it is still the file built, with the same mode and
// SHA-256, and fails the caller with a shared-binary-changed violation when
// it is not.
func sharedBinary(t testing.TB, pkg string) string {
	t.Helper()
	sharedBuilds.Lock()
	build := sharedBuilds.byPkg[pkg]
	if build == nil {
		build = &sharedBuild{pkg: pkg}
		sharedBuilds.byPkg[pkg] = build
	}
	sharedBuilds.Unlock()
	build.once.Do(func() { build.err = build.make() })
	if build.err != nil {
		t.Fatalf("setup: shared build of %s: %v", pkg, build.err)
	}
	reason, err := build.changed()
	if err != nil {
		t.Fatalf("setup: shared binary of %s could not be checked: %v", pkg, err)
	}
	if reason != "" {
		t.Fatalf(sharedChangedViolation+"%s: %s", pkg, reason)
	}
	return build.path
}

// make builds the package into its own folder, makes the binary and then the
// folder read-only, and records the binary's identity, mode and SHA-256.
func (b *sharedBuild) make() error {
	folder, err := sharedBuildFolder()
	if err != nil {
		return err
	}
	root, err := sharedModuleRoot()
	if err != nil {
		return err
	}
	// Its own folder keeps the binary's usual name (ha, spar).
	dir, err := os.MkdirTemp(folder, "pkg-")
	if err != nil {
		return err
	}
	name := filepath.Base(filepath.FromSlash(b.pkg))
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(dir, name)
	env := run.Environment{Dir: sharedStartDir, Env: sharedStartEnv}
	cmd := env.Command(root, "go", "build", "-mod=readonly", "-o", binary, b.pkg)
	cmd.Env = env.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("go build -mod=readonly %s: %v: %s", b.pkg, err, strings.TrimSpace(string(out)))
	}
	if err := os.Chmod(binary, 0o555); err != nil {
		return err
	}
	info, err := os.Lstat(binary)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode().Perm()&0o222 != 0 {
		return fmt.Errorf("%s is not a read-only file after the build (mode %v)", binary, info.Mode())
	}
	// On Windows the file identity is read when first compared: read it now,
	// from the file built, not later from whatever has the name then.
	os.SameFile(info, info)
	digest, err := sharedFileDigest(binary)
	if err != nil {
		return err
	}
	if err := os.Chmod(dir, 0o555); err != nil {
		return err
	}
	b.path, b.info, b.digest = binary, info, digest
	sharedBuilds.Lock()
	sharedBuilds.built = append(sharedBuilds.built, b)
	sharedBuilds.Unlock()
	return nil
}

// changed returns why the binary is no longer the file built, or "" when it
// still is: the same file, with the read-only mode it was given and the
// SHA-256 recorded after the build. err is a file that is the same and keeps
// its mode but could not be read, which shows no change.
func (b *sharedBuild) changed() (string, error) {
	info, err := os.Lstat(b.path)
	switch {
	case err != nil:
		return fmt.Sprintf("%s is gone: %v", b.path, err), nil
	case !os.SameFile(info, b.info):
		return fmt.Sprintf("%s is not the file built: it was replaced", b.path), nil
	case info.Mode() != b.info.Mode():
		return fmt.Sprintf("%s has mode %v, not the read-only %v it was built with", b.path, info.Mode(), b.info.Mode()), nil
	}
	digest, err := sharedFileDigest(b.path)
	if err != nil {
		return "", err
	}
	if digest != b.digest {
		return fmt.Sprintf("%s has SHA-256 %x, not the %x it was built with", b.path, digest, b.digest), nil
	}
	return "", nil
}

// checkSharedBinaries checks, after every test ended, that each binary built
// in this process is still the file built. It writes a shared-binary-changed
// violation line for each one that is not, and a setup line for each it could
// not read, and reports whether every binary was found unchanged.
func checkSharedBinaries(w io.Writer) bool {
	sharedBuilds.Lock()
	built := append([]*sharedBuild(nil), sharedBuilds.built...)
	sharedBuilds.Unlock()
	unchanged := true
	for _, b := range built {
		reason, err := b.changed()
		switch {
		case err != nil:
			fmt.Fprintf(w, "setup: shared binary of %s could not be checked: %v\n", b.pkg, err)
			unchanged = false
		case reason != "":
			fmt.Fprintf(w, "%s%s: %s\n", sharedChangedViolation, b.pkg, reason)
			unchanged = false
		}
	}
	return unchanged
}

func sharedFileDigest(path string) ([sha256.Size]byte, error) {
	var digest [sha256.Size]byte
	f, err := os.Open(path)
	if err != nil {
		return digest, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return digest, err
	}
	copy(digest[:], h.Sum(nil))
	return digest, nil
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
// ended. The read-only folders and binaries are made writable first, as
// removal from a folder needs and Windows needs for the files.
func removeSharedBinaries() {
	sharedBuilds.Lock()
	defer sharedBuilds.Unlock()
	if sharedBuilds.folder == "" {
		return
	}
	// WalkDir reads a folder after visiting it, so a folder is writable and
	// readable again before its entries are read.
	_ = filepath.WalkDir(sharedBuilds.folder, func(path string, d os.DirEntry, err error) error {
		if err == nil {
			_ = os.Chmod(path, 0o700)
		}
		return nil
	})
	_ = os.RemoveAll(sharedBuilds.folder)
	sharedBuilds.folder = ""
}
