package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func evidenceWrite(t *testing.T, name, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(name, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func evidenceLink(t *testing.T, target, name string) {
	t.Helper()
	if err := os.Symlink(target, name); err != nil {
		t.Fatal(err)
	}
}

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

func TestFileEvidenceRejectsReplacedLeaf(t *testing.T) {
	for _, kind := range []string{"outside symlink", "same inode symlink", "other file", "fifo", "directory", "removed"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			name := filepath.Join(root, "evidence")
			original := filepath.Join(root, "original")
			outside := filepath.Join(t.TempDir(), "outside")
			evidenceWrite(t, name, "checked\n")
			evidenceWrite(t, outside, "outside\n")
			version, state := observeFileEvidence(root, "evidence", func() {
				if err := os.Rename(name, original); err != nil {
					t.Fatal(err)
				}
				switch kind {
				case "outside symlink":
					evidenceLink(t, outside, name)
				case "same inode symlink":
					// Identity alone cannot reject a link to the checked file.
					evidenceLink(t, "original", name)
				case "other file":
					evidenceWrite(t, name, "replacement\n")
				case "fifo":
					if err := syscall.Mkfifo(name, 0o644); err != nil {
						t.Fatal(err)
					}
				case "directory":
					if err := os.Mkdir(name, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			})
			want := "unreadable"
			if kind == "removed" {
				want = "missing"
			}
			if version != "" || state != want {
				t.Fatalf("replaced leaf: version=%q state=%q, want empty/%s", version, state, want)
			}
		})
	}
}

func TestFileEvidenceRejectsReplacedParent(t *testing.T) {
	for _, kind := range []string{"outside", "inside"} {
		t.Run(kind, func(t *testing.T) {
			root := t.TempDir()
			parent := filepath.Join(root, "docs")
			other := filepath.Join(root, "other")
			if kind == "outside" {
				other = t.TempDir()
			}
			evidenceWrite(t, filepath.Join(parent, "evidence"), "checked\n")
			evidenceWrite(t, filepath.Join(other, "evidence"), "replacement\n")
			version, state := observeFileEvidence(root, "docs/evidence", func() {
				if err := os.Rename(parent, filepath.Join(root, "original")); err != nil {
					t.Fatal(err)
				}
				evidenceLink(t, other, parent)
			})
			if version != "" || state != "unreadable" {
				t.Fatalf("replaced parent: version=%q state=%q", version, state)
			}
		})
	}
}

func TestFileEvidenceConfinedWalkRejectsLinks(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	evidenceWrite(t, filepath.Join(root, "docs", "evidence"), "checked\n")
	evidenceWrite(t, filepath.Join(outside, "evidence"), "outside\n")
	pinned, err := openFileEvidenceRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer pinned.Close()
	// Simulate a link introduced after parent canonicalization but before
	// descriptor traversal, including a link whose target remains inside.
	if err := os.Rename(filepath.Join(root, "docs"), filepath.Join(root, "original")); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{outside, "original"} {
		link := filepath.Join(root, "docs")
		evidenceLink(t, target, link)
		if f, err := openConfinedFileEvidence(pinned, "docs", "evidence"); err == nil {
			f.Close()
			t.Fatalf("confined traversal followed new parent symlink %q", target)
		}
		if err := os.Remove(link); err != nil {
			t.Fatal(err)
		}
	}
	evidenceLink(t, "original/evidence", filepath.Join(root, "leaf"))
	for _, path := range [][2]string{{".", "leaf"}, {"../outside", "evidence"}, {".", "/"}, {".", "../evidence"}} {
		if f, err := openConfinedFileEvidence(pinned, path[0], path[1]); err == nil {
			f.Close()
			t.Fatalf("confined traversal accepted %q/%q", path[0], path[1])
		}
	}
}

func TestFileEvidencePreservesStablePathStates(t *testing.T) {
	root := t.TempDir()
	data := "checked\n"
	evidenceWrite(t, filepath.Join(root, "docs", "evidence"), data)
	evidenceLink(t, "docs", filepath.Join(root, "relative"))
	evidenceLink(t, filepath.Join(root, "docs"), filepath.Join(root, "absolute"))
	evidenceLink(t, "docs/evidence", filepath.Join(root, "leaf"))
	evidenceLink(t, "missing", filepath.Join(root, "dangling"))
	evidenceLink(t, t.TempDir(), filepath.Join(root, "outside"))
	sum := sha256.Sum256([]byte(data))
	for name, state := range map[string]string{
		"docs/evidence": "read", "relative/evidence": "read", "absolute/evidence": "read",
		"leaf": "not_regular", "dangling": "not_regular", "docs": "not_regular",
		"outside/evidence": "outside", "docs/missing": "missing",
	} {
		version, got := fileVersion(root)(name)
		wantVersion := ""
		if state == "read" {
			wantVersion = "sha256:" + hex.EncodeToString(sum[:])
		}
		if got != state || version != wantVersion {
			t.Errorf("%s: version=%q state=%q, want %q/%s", name, version, got, wantVersion, state)
		}
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

// growingEvidenceReader grows the same file after the first content read. It
// makes the read/size boundary deterministic without a scheduling race.
type growingEvidenceReader struct {
	file *os.File
	grow func()
	n    int64
}

func (r *growingEvidenceReader) Read(p []byte) (int, error) {
	n, err := r.file.Read(p)
	r.n += int64(n)
	if r.grow != nil {
		r.grow()
		r.grow = nil
	}
	return n, err
}

func TestFileEvidenceBoundsGrowth(t *testing.T) {
	t.Run("initial size boundary", func(t *testing.T) {
		root := t.TempDir()
		name := filepath.Join(root, "evidence")
		evidenceWrite(t, name, "")
		for _, size := range []int64{fileEvidenceLimit - 1, fileEvidenceLimit, fileEvidenceLimit + 1} {
			if err := os.Truncate(name, size); err != nil {
				t.Fatal(err)
			}
			version, state := fileVersion(root)("evidence")
			if size <= fileEvidenceLimit {
				if state != "read" || len(version) != len("sha256:")+64 {
					t.Fatalf("size %d: version=%q state=%q", size, version, state)
				}
			} else if state != "too_large" || version != "" {
				t.Fatalf("oversized: version=%q state=%q", version, state)
			}
		}
	})
	t.Run("growth after check", func(t *testing.T) {
		root := t.TempDir()
		name := filepath.Join(root, "evidence")
		evidenceWrite(t, name, "x")
		version, state := observeFileEvidence(root, "evidence", func() {
			if err := os.Truncate(name, fileEvidenceLimit+1); err != nil {
				t.Fatal(err)
			}
		})
		if version != "" || state != "too_large" {
			t.Fatalf("growth after check: version=%q state=%q", version, state)
		}
	})
	t.Run("growth during read", func(t *testing.T) {
		name := filepath.Join(t.TempDir(), "evidence")
		evidenceWrite(t, name, "x")
		f, err := os.Open(name)
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		r := &growingEvidenceReader{file: f, grow: func() {
			if err := os.Truncate(name, fileEvidenceLimit*2); err != nil {
				t.Fatal(err)
			}
		}}
		version, state := hashFileEvidence(r)
		if version != "" || state != "too_large" || r.n != fileEvidenceLimit+1 {
			t.Fatalf("growth during read: version=%q state=%q bytes=%d", version, state, r.n)
		}
		// The rest was not consumed by hashing.
		if pos, err := f.Seek(0, io.SeekCurrent); err != nil || pos != fileEvidenceLimit+1 {
			t.Fatalf("read position=%d, error=%v", pos, err)
		}
	})
}
