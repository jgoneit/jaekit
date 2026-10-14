package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

func makeEvidenceFIFO(string) error { return windows.ERROR_NOT_SUPPORTED }

func evidenceReplacementKinds() []string {
	return []string{"outside symlink", "same inode symlink", "other file", "directory", "removed"}
}

func evidenceJunction(t *testing.T, target, name string) {
	t.Helper()
	out, err := exec.Command("cmd.exe", "/c", "mklink", "/J", name, target).CombinedOutput()
	if err != nil {
		t.Fatalf("create native junction: %v: %s", err, out)
	}
}

func TestNativeHistoryWindowsFileBoundaries(t *testing.T) {
	t.Run("special-references", func(t *testing.T) {
		f := newNativeHistoryFixture(t, false)
		// ADS and drive-relative spellings pass the unchanged event grammar;
		// no Win32 name interpretation may turn them into normal file evidence.
		f.put("ordinary", "base content")
		if err := os.WriteFile(filepath.Join(f.dir, "ordinary:stream"), []byte("stream secret"), 0o644); err != nil {
			t.Fatal(err)
		}
		names := []string{"NUL", "CON", "CONIN$", "CONOUT$", "COM1", "LPT1", "COM¹", "ordinary:stream", "C:ordinary", "dir/NUL"}
		argv := []string{"history", scenarioGoal, "add", "--kind", "관측", "--summary", "special names are not opened", "--request-id", "windows-special"}
		for _, name := range names {
			argv = append(argv, "--ref", "file:"+name)
		}
		nativeHistoryCommand(t, f.dir, 0, argv...)
		v := nativeHistoryView(t, f, "show", "D1")
		if v.Event == nil || len(v.Event.Evidence) != len(names) {
			t.Fatalf("special reference event lost: %+v", v.Event)
		}
		for i, e := range v.Event.Evidence {
			if e.Path != names[i] || e.Recorded.State != "unreadable" || e.Recorded.Version != "" {
				t.Fatalf("special reference %q: %+v", names[i], e)
			}
		}
		for _, name := range []string{`\\.\pipe\never-connect`, `\\?\GLOBALROOT\Device\NamedPipe\never-connect`, `//./pipe/never-connect`} {
			if validFileEvidencePath(name) {
				t.Fatalf("native pipe namespace accepted: %q", name)
			}
			if version, state := fileVersion(f.dir)(name); version != "" || state != "unreadable" {
				t.Fatalf("pipe namespace observation: %q/%s", version, state)
			}
		}
	})
	t.Run("stable-parent-and-outside", func(t *testing.T) {
		root := t.TempDir()
		evidenceWrite(t, filepath.Join(root, "actual", "evidence"), "inside")
		evidenceJunction(t, filepath.Join(root, "actual"), filepath.Join(root, "parent"))
		outside := t.TempDir()
		evidenceWrite(t, filepath.Join(outside, "evidence"), "outside")
		evidenceJunction(t, outside, filepath.Join(root, "outside"))
		for name, state := range map[string]string{"parent/evidence": "read", "outside/evidence": "outside", "parent": "not_regular", "missing/x": "missing"} {
			version, got := fileVersion(root)(name)
			if got != state || (state != "read" && version != "") {
				t.Fatalf("%s: %q/%s, want %s", name, version, got, state)
			}
		}
		// Windows drive and path casing identify the same anchored file.
		original, state := fileVersion(root)("actual/evidence")
		if v, s := fileVersion(strings.ToUpper(root))("ACTUAL/EVIDENCE"); state != "read" || s != "read" || v != original {
			t.Fatalf("case changed repository identity: %q/%s vs %q/%s", original, state, v, s)
		}
	})
	for _, component := range []string{"parent", "root"} {
		t.Run("junction-race-"+component, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), "repo")
			evidenceWrite(t, filepath.Join(root, "docs", "evidence"), "inside")
			outside := t.TempDir()
			evidenceWrite(t, filepath.Join(outside, "docs", "evidence"), "outside")
			evidenceWrite(t, filepath.Join(outside, "evidence"), "outside")
			version, state := observeFileEvidence(root, "docs/evidence", func() {
				name := root
				if component == "parent" {
					name = filepath.Join(root, "docs")
				}
				if err := os.Rename(name, name+".original"); err != nil {
					t.Fatal(err)
				}
				evidenceJunction(t, outside, name)
			})
			if version != "" || state != "unreadable" {
				t.Fatalf("replaced %s read redirected bytes: %q/%s", component, version, state)
			}
		})
	}
	t.Run("confined-reparse", func(t *testing.T) {
		root := t.TempDir()
		evidenceWrite(t, filepath.Join(root, "docs", "evidence"), "inside")
		pinned, err := openFileEvidenceRoot(root)
		if err != nil {
			t.Fatal(err)
		}
		defer pinned.Close()
		if err := os.Rename(filepath.Join(root, "docs"), filepath.Join(root, "original")); err != nil {
			t.Fatal(err)
		}
		evidenceJunction(t, filepath.Join(root, "original"), filepath.Join(root, "docs"))
		if f, err := openConfinedFileEvidence(pinned, "docs", "evidence"); err == nil {
			f.Close()
			t.Fatal("confined walk followed a newly introduced inside junction")
		}
		evidenceJunction(t, filepath.Join(root, "original"), filepath.Join(root, "leaf"))
		if f, err := openConfinedFileEvidence(pinned, ".", "leaf"); err == nil {
			f.Close()
			t.Fatal("confined walk opened a final reparse point")
		}
	})
	t.Run("sharing-denial", func(t *testing.T) {
		root := t.TempDir()
		name := filepath.Join(root, "evidence")
		evidenceWrite(t, name, "private")
		p, err := windows.UTF16PtrFromString(name)
		if err != nil {
			t.Fatal(err)
		}
		h, err := windows.CreateFile(p, windows.GENERIC_READ, 0, nil, windows.OPEN_EXISTING, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(h)
		if version, state := fileVersion(root)("evidence"); version != "" || state != "unreadable" {
			t.Fatalf("sharing denial became normal/missing: %q/%s", version, state)
		}
	})
}
