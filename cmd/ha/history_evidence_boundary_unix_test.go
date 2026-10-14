//go:build darwin || linux

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestFileEvidenceRejectsReplacedRoot(t *testing.T) {
	const fixtureEnv = "JAEKIT_TEST_REPLACED_EVIDENCE_ROOT"
	if root := os.Getenv(fixtureEnv); root != "" {
		// The repository was a directory when its path was selected, then
		// became a FIFO before file observation. No FIFO writer is started.
		version := fileVersion(root)
		if err := os.Rename(root, root+".checked"); err != nil {
			t.Fatal(err)
		}
		if err := syscall.Mkfifo(root, 0o644); err != nil {
			t.Fatal(err)
		}
		if got, state := version("evidence"); got != "" || state != "unreadable" {
			t.Fatalf("replaced root: version=%q state=%q", got, state)
		}
		return
	}
	root := filepath.Join(t.TempDir(), "repo")
	evidenceWrite(t, filepath.Join(root, "evidence"), "checked\n")
	// A regression must fail within a bound rather than block the suite while
	// opening a FIFO root without an atomic directory-only constraint.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFileEvidenceRejectsReplacedRoot$", "-test.count=1")
	cmd.Env = append(os.Environ(), fixtureEnv+"="+root)
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		t.Fatalf("opening the replaced repository root waited for a FIFO writer: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("replaced root subprocess: %v\n%s", err, out)
	}
}

func TestFileEvidenceBoundsDirectoryDescriptors(t *testing.T) {
	const fixtureEnv = "JAEKIT_TEST_DEEP_EVIDENCE_ROOT"
	const content = "deeply nested evidence\n"
	name := strings.Repeat("d/", 90) + "evidence"
	if root := os.Getenv(fixtureEnv); root != "" {
		var limit syscall.Rlimit
		if err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
			t.Fatal(err)
		}
		limit.Cur = 64
		if err := syscall.Setrlimit(syscall.RLIMIT_NOFILE, &limit); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256([]byte(content))
		want := "sha256:" + hex.EncodeToString(sum[:])
		// More calls than the fd limit also detect a descriptor leaked on each
		// successful walk. Each observation must need constant descriptors.
		for i := 0; i < 70; i++ {
			if version, state := fileVersion(root)(name); state != "read" || version != want {
				t.Fatalf("observation %d: version=%q state=%q, want %q/read", i, version, state, want)
			}
		}
		return
	}
	root := t.TempDir()
	evidenceWrite(t, filepath.Join(root, filepath.FromSlash(name)), content)
	// Lower the limit only in the helper, never in the shared test process.
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestFileEvidenceBoundsDirectoryDescriptors$", "-test.count=1")
	cmd.Env = append(os.Environ(), fixtureEnv+"="+root)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("bounded-descriptor subprocess: %v (context %v)\n%s", err, ctx.Err(), out)
	}
}

func TestFileEvidencePreservesSearchOnlyDirectories(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root bypasses directory permission checks")
	}
	root := t.TempDir()
	first := filepath.Join(root, "search-only")
	second := filepath.Join(first, "nested")
	data := "readable through search-only directories\n"
	evidenceWrite(t, filepath.Join(second, "evidence"), data)
	evidenceLink(t, "search-only/nested", filepath.Join(root, "relative"))
	evidenceLink(t, second, filepath.Join(root, "absolute"))
	// Restore permissions before TempDir cleanup, including the ancestors.
	for _, dir := range []string{second, first, root} {
		if err := os.Chmod(dir, 0o311); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if err := os.Chmod(dir, 0o755); err != nil {
				t.Errorf("restore %s: %v", dir, err)
			}
		})
	}
	sum := sha256.Sum256([]byte(data))
	want := "sha256:" + hex.EncodeToString(sum[:])
	for _, name := range []string{"search-only/nested/evidence", "relative/evidence", "absolute/evidence"} {
		if version, state := fileVersion(root)(name); state != "read" || version != want {
			t.Errorf("%s: version=%q state=%q, want %q/read", name, version, state, want)
		}
	}
}

func makeEvidenceFIFO(name string) error { return syscall.Mkfifo(name, 0o644) }

func evidenceReplacementKinds() []string {
	return []string{"outside symlink", "same inode symlink", "other file", "fifo", "directory", "removed"}
}
