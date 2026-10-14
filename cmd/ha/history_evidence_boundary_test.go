package main

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"
	"testing"
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

func TestFileEvidenceRejectsReplacedLeaf(t *testing.T) {
	for _, kind := range evidenceReplacementKinds() {
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
					if err := makeEvidenceFIFO(name); err != nil {
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
