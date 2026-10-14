//go:build !windows

package history

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestUnanchoredRegistrationIsReported(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permissions do not restrict root")
	}
	s := newStore(t)
	f := facts(runHashes(2, "a"))
	mustAdd(t, s, f, "r1", Decision{Kind: "관측", Summary: "first"})
	d := Decision{Kind: "관측", Summary: "second"}
	// The events file stays writable; the record of the last event cannot be replaced.
	os.Chmod(s.Dir(), 0o555)
	_, err := add(t, s, f, "r2", d)
	os.Chmod(s.Dir(), 0o755)
	if Code(err) != "write_uncertain" || !strings.Contains(err.Error(), "D2 was stored") {
		t.Fatalf("an unrecorded last event must be reported: %v", err)
	}
	if _, events, err := s.Load(); err != nil || len(events) != 2 {
		t.Fatalf("the stored event reads: %v %d", err, len(events))
	}
	if _, err := add(t, s, f, "r2", d); !isReplay(err, "D2") {
		t.Fatalf("the same request must replay once the record catches up: %v", err)
	}
	data, _ := os.ReadFile(s.file())
	os.WriteFile(s.file(), data[:bytes.IndexByte(data, '\n')+1], 0o644)
	if _, _, err := s.Load(); Code(err) != "history_corrupt" {
		t.Fatalf("after catching up, losing D2 is damage: %v", err)
	}
}

func TestSymlinkedStorageIsRefused(t *testing.T) {
	common, outside := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(common, "ha"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(common, "ha", "history")); err != nil {
		t.Fatal(err)
	}
	s, _ := Open(common, startHash)
	if _, err := add(t, s, facts(runHashes(2, "a")), "r1", Decision{Kind: "관측", Summary: "x"}); Code(err) != "history_unreadable" {
		t.Fatalf("symlinked history directory: %v", err)
	}
	if entries, _ := os.ReadDir(outside); len(entries) != 0 {
		t.Fatalf("wrote through a symlink: %v", entries)
	}
}

func TestReadsRefuseSymlinkedStorage(t *testing.T) {
	for _, level := range []int{0, 1, 2} {
		s := newStore(t)
		mustAdd(t, s, facts(runHashes(2, "a")), "r1", Decision{Kind: "관측", Summary: "real"})
		dir := s.dirs()[level]
		moved := filepath.Join(t.TempDir(), "moved")
		if err := os.Rename(dir, moved); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(moved, dir); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Load(); Code(err) != "history_unreadable" {
			t.Errorf("symlinked %s: %v", filepath.Base(dir), err)
		}
	}
}

func TestStoragePinsDirectoryHierarchy(t *testing.T) {
	for _, level := range []string{"common", "ha", "history", "start"} {
		t.Run(level, func(t *testing.T) {
			s, outside := newStore(t), newStore(t)
			f := facts(runHashes(2, "pinned"))
			mustAdd(t, s, f, "original", Decision{Kind: "관측", Summary: "original storage"})
			mustAdd(t, outside, f, "outside", Decision{Kind: "관측", Summary: "outside storage"})
			original, untouched := historyStorageBytes(t, s.Dir()), historyStorageBytes(t, outside.Dir())
			tx, err := s.openStorage(false)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.close()
			victim, target := s.common, outside.common
			for i, name := range []string{"ha", "history", "start"} {
				if level == name {
					victim, target = s.dirs()[i], outside.dirs()[i]
				}
			}
			rel, err := filepath.Rel(victim, s.Dir())
			if err != nil {
				t.Fatal(err)
			}
			moved := filepath.Join(t.TempDir(), "original")
			if err := os.Rename(victim, moved); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(target, victim); err != nil {
				t.Fatal(err)
			}
			pinned := filepath.Join(moved, rel)
			unlock, held, err := tx.acquire(false)
			if err != nil || !held {
				t.Fatalf("acquire pinned lock: %v %t", err, held)
			}
			defer unlock()
			for _, check := range []struct {
				dir    string
				locked bool
			}{{pinned, true}, {outside.Dir(), false}} {
				file, err := os.OpenFile(filepath.Join(check.dir, "lock"), os.O_RDWR, 0)
				if err != nil {
					t.Fatal(err)
				}
				err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
				if (err != nil) != check.locked {
					t.Errorf("lock redirected: %s: %v; locked=%t", check.dir, err, check.locked)
				}
				if err == nil {
					syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				}
				file.Close()
			}
			if present, err := tx.lockPresent(); err != nil || !present {
				t.Fatalf("pinned lock presence: %v %t", err, present)
			}
			for _, name := range []string{"events.jsonl", "last"} {
				got, err := tx.read(name)
				if err != nil || !bytes.Equal(got, original[name]) {
					t.Errorf("read redirected for %s: %v %q", name, err, got)
				}
			}
			file, err := tx.appendFile()
			if err != nil {
				t.Fatal(err)
			}
			appended := []byte("pinned append\n")
			if err := persistLine(file, appended); err != nil {
				t.Fatal(err)
			}
			marker := []byte("pinned marker\n")
			if err := tx.replaceLast(marker); err != nil {
				t.Fatal(err)
			}
			got := historyStorageBytes(t, pinned)
			if !bytes.Equal(got["events.jsonl"], append(original["events.jsonl"], appended...)) || !bytes.Equal(got["last"], marker) {
				t.Errorf("writes left the pinned directory: %q", got)
			}
			for name, data := range historyStorageBytes(t, outside.Dir()) {
				if !bytes.Equal(data, untouched[name]) {
					t.Errorf("outside %s changed: %q", name, data)
				}
			}
		})
	}
	t.Run("upstream symlink permitted", func(t *testing.T) {
		parent := t.TempDir()
		if err := os.Mkdir(filepath.Join(parent, "common"), 0o755); err != nil {
			t.Fatal(err)
		}
		alias := filepath.Join(t.TempDir(), "parent")
		if err := os.Symlink(parent, alias); err != nil {
			t.Fatal(err)
		}
		s, _ := Open(filepath.Join(alias, "common"), startHash)
		mustAdd(t, s, facts(runHashes(2, "alias")), "first", Decision{Kind: "관측", Summary: "upstream alias"})
		if _, events, err := s.Load(); err != nil || len(events) != 1 {
			t.Fatalf("upstream alias: %v %d", err, len(events))
		}
	})
	t.Run("common symlink rejected", func(t *testing.T) {
		s := newStore(t)
		mustAdd(t, s, facts(runHashes(2, "alias")), "first", Decision{Kind: "관측", Summary: "original"})
		alias := filepath.Join(t.TempDir(), "common")
		if err := os.Symlink(s.common, alias); err != nil {
			t.Fatal(err)
		}
		for _, suffix := range []string{"", string(filepath.Separator)} {
			linked, _ := Open(alias+suffix, startHash)
			if _, _, err := linked.Load(); Code(err) != "history_unreadable" {
				t.Errorf("common alias %q: %v", suffix, err)
			}
		}
	})
	t.Run("search only directories", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permissions do not restrict root")
		}
		s := newStore(t)
		f := facts(runHashes(2, "search"))
		mustAdd(t, s, f, "first", Decision{Kind: "관측", Summary: "first"})
		for _, dir := range append([]string{s.common}, s.dirs()...) {
			mode := os.FileMode(0o111)
			if dir == s.Dir() {
				mode = 0o333
			}
			if err := os.Chmod(dir, mode); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { os.Chmod(dir, 0o755) })
		}
		if _, events, err := s.Load(); err != nil || len(events) != 1 {
			t.Fatalf("search-only read: %v %d", err, len(events))
		}
		mustAdd(t, s, f, "second", Decision{Kind: "관측", Summary: "second"})
	})
	t.Run("write only staging file", func(t *testing.T) {
		if os.Geteuid() == 0 {
			t.Skip("permissions do not restrict root")
		}
		s := newStore(t)
		f := facts(runHashes(2, "write-only"))
		mustAdd(t, s, f, "first", Decision{Kind: "관측", Summary: "first"})
		if err := os.WriteFile(filepath.Join(s.Dir(), "last.tmp"), []byte("old staging bytes"), 0o200); err != nil {
			t.Fatal(err)
		}
		mustAdd(t, s, f, "second", Decision{Kind: "관측", Summary: "second"})
		info, err := os.Stat(s.last())
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Errorf("published marker permissions: %o; want 600", info.Mode().Perm())
		}
		if state, events, err := s.Load(); err != nil || state != Present || len(events) != 2 {
			t.Fatalf("load after write-only staging: %v %s %d", err, state, len(events))
		}
		mustAdd(t, s, f, "third", Decision{Kind: "관측", Summary: "third"})
		if state, events, err := s.Load(); err != nil || state != Present || len(events) != 3 {
			t.Fatalf("next registration after write-only staging: %v %s %d", err, state, len(events))
		}
	})
}

func historyStorageBytes(t *testing.T, dir string) map[string][]byte {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 3 {
		t.Fatalf("unexpected storage entries in %s: %v", dir, entries)
	}
	result := make(map[string][]byte)
	for _, name := range []string{"events.jsonl", "last", "lock"} {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		result[name] = data
	}
	return result
}

func TestStorageRejectsReplacedLeaf(t *testing.T) {
	for _, name := range []string{"events.jsonl", "lock", "last", "last.tmp"} {
		for _, kind := range []string{"symlink", "hardlink", "fifo", "directory"} {
			t.Run(name+"/"+kind, func(t *testing.T) {
				s := newStore(t)
				mustAdd(t, s, facts(runHashes(2, "leaf")), "first", Decision{Kind: "관측", Summary: "first"})
				tx, err := s.openStorage(false)
				if err != nil {
					t.Fatal(err)
				}
				defer tx.close()
				outside := filepath.Join(t.TempDir(), "outside")
				original := []byte("outside must remain unchanged")
				if err := os.WriteFile(outside, original, 0o644); err != nil {
					t.Fatal(err)
				}
				victim := filepath.Join(s.Dir(), name)
				if err := os.Remove(victim); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				switch kind {
				case "symlink":
					err = os.Symlink(outside, victim)
				case "hardlink":
					err = os.Link(outside, victim)
				case "fifo":
					err = syscall.Mkfifo(victim, 0o600)
				case "directory":
					err = os.Mkdir(victim, 0o755)
				}
				if err != nil {
					t.Fatal(err)
				}
				refuse := func(operation func() error) {
					t.Helper()
					done := make(chan error, 1)
					go func() { done <- operation() }()
					select {
					case err := <-done:
						if err == nil {
							t.Error("unsafe leaf was accepted")
						}
					case <-time.After(2 * time.Second):
						t.Fatal("nonregular leaf blocked storage access")
					}
				}
				if name != "last.tmp" {
					refuse(func() error { _, err := tx.read(name); return err })
				}
				switch name {
				case "events.jsonl":
					refuse(func() error {
						file, err := tx.appendFile()
						if file != nil {
							file.Close()
						}
						return err
					})
				case "lock":
					for _, shared := range []bool{true, false} {
						refuse(func() error {
							unlock, _, err := tx.acquire(shared)
							if unlock != nil {
								unlock()
							}
							return err
						})
					}
					refuse(func() error { _, err := tx.lockPresent(); return err })
				case "last", "last.tmp":
					refuse(func() error { return tx.replaceLast([]byte("replacement\n")) })
				}
				got, err := os.ReadFile(outside)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatalf("outside changed: %v %q", err, got)
				}
			})
		}
	}
	t.Run("staging replaced before publication", func(t *testing.T) {
		s := newStore(t)
		mustAdd(t, s, facts(runHashes(2, "stage")), "first", Decision{Kind: "관측", Summary: "first"})
		tx, err := s.openStorage(false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.close()
		native := tx.(*unixStorage)
		before, err := tx.read("last")
		if err != nil {
			t.Fatal(err)
		}
		staged, err := native.open("last.tmp", unix.O_WRONLY|unix.O_CREAT)
		if err != nil {
			t.Fatal(err)
		}
		defer staged.Close()
		if _, err := staged.Write([]byte("written marker\n")); err != nil {
			t.Fatal(err)
		}
		if err := staged.Sync(); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(s.Dir(), "last.tmp")
		// Retain the written inode under a different name: it still has exactly
		// one link, so the source identity check must catch the replacement.
		if err := os.Rename(path, path+".written"); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("substituted marker\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := native.publishLast(staged); err == nil {
			t.Error("substituted staging file was published")
		}
		got, err := tx.read("last")
		if err != nil || !bytes.Equal(got, before) {
			t.Fatalf("marker changed before source validation: %v %q", err, got)
		}
	})
	for _, point := range []string{"staging replacement during rename", "marker replacement after rename"} {
		t.Run(point, func(t *testing.T) {
			s := newStore(t)
			f := facts(runHashes(2, "stage"))
			mustAdd(t, s, f, "first", Decision{Kind: "관측", Summary: "first"})
			rename := renameHistoryMarker
			defer func() { renameHistoryMarker = rename }()
			injected := false
			substitute := func(name string) error {
				path := filepath.Join(s.Dir(), name)
				if err := os.Rename(path, path+".written"); err != nil {
					return err
				}
				return os.WriteFile(path, []byte("substituted marker\n"), 0o644)
			}
			renameHistoryMarker = func(oldFD int, oldName string, newFD int, newName string) error {
				injected = true
				if point == "staging replacement during rename" {
					if err := substitute(oldName); err != nil {
						return err
					}
				}
				if err := rename(oldFD, oldName, newFD, newName); err != nil {
					return err
				}
				if point == "marker replacement after rename" {
					return substitute(newName)
				}
				return nil
			}
			_, err := add(t, s, f, "second", Decision{Kind: "관측", Summary: "second"})
			if !injected {
				t.Fatal("replacement fixture did not reach the rename boundary")
			}
			if Code(err) != "write_uncertain" {
				t.Errorf("substituted marker was acknowledged: %v", err)
			}
			data, err := os.ReadFile(s.file())
			if err != nil {
				t.Fatal(err)
			}
			if events, err := Parse(data, startHash); err != nil || len(events) != 2 {
				t.Fatalf("appended event must remain after uncertain marker: %v %d", err, len(events))
			}
		})
	}
}
