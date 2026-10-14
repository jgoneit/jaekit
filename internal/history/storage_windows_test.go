package history

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNativeHistoryStorageLifecycle(t *testing.T) {
	t.Run("registration replay and correction", TestRegistrationReplayConflictAndRelations)
	t.Run("concurrent stores and corrections", TestConcurrentAppendsAndConflictingCorrections)
	t.Run("lineage and current", TestCurrentJudgmentsFollowLineageBeforeFilters)
	t.Run("evidence replay preserves observation", TestFileEvidenceObservationBoundaries)
	t.Run("case alias shares storage", func(t *testing.T) {
		common := filepath.Join(t.TempDir(), "History 한글 경로")
		if err := os.Mkdir(common, 0o755); err != nil {
			t.Fatal(err)
		}
		s, _ := Open(common, startHash)
		mustAdd(t, s, facts(runHashes(2, "native")), "first", Decision{Kind: "관측", Summary: "same store"})
		alias, _ := Open(strings.ToUpper(common), startHash)
		if _, events, err := alias.Load(); err != nil || len(events) != 1 {
			t.Fatalf("alias load: %v %d", err, len(events))
		}
		if _, err := add(t, alias, facts(runHashes(2, "native")), "first", Decision{Kind: "관측", Summary: "same store"}); !isReplay(err, "D1") {
			t.Fatalf("alias replay: %v", err)
		}
	})
}

func TestNativeHistoryStorageLocks(t *testing.T) {
	if common := os.Getenv("JAEKIT_HISTORY_LOCK_HELPER"); common != "" {
		s, err := Open(common, startHash)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := s.openStorage(true)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.close()
		unlock, _, err := tx.acquire(false)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		fmt.Println("HISTORY_LOCK_HELD")
		for {
			time.Sleep(time.Minute)
		}
	}
	t.Run("reader waits for writer", TestReadsWaitForRegistration)
	t.Run("first registration retry", TestFirstRegistrationReadWaits)
	t.Run("shared readers exclude writer", func(t *testing.T) {
		s := newStore(t)
		mustAdd(t, s, facts(runHashes(2, "native")), "first", Decision{Kind: "관측", Summary: "first"})
		a, err := s.openStorage(false)
		if err != nil {
			t.Fatal(err)
		}
		defer a.close()
		unlockA, _, err := a.acquire(true)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if unlockA != nil {
				unlockA()
			}
		}()
		alias, _ := Open(strings.ToUpper(s.common), startHash)
		b, err := alias.openStorage(false)
		if err != nil {
			t.Fatal(err)
		}
		defer b.close()
		unlockB, _, err := b.acquire(true)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if unlockB != nil {
				unlockB()
			}
		}()
		done := make(chan error, 1)
		go func() {
			_, err := add(t, alias, facts(runHashes(2, "native")), "second", Decision{Kind: "관측", Summary: "second"})
			done <- err
		}()
		assertHistoryWaiting(t, done)
		unlockA()
		unlockA = nil
		assertHistoryWaiting(t, done)
		unlockB()
		unlockB = nil
		awaitHistoryResult(t, done)
	})
	t.Run("process exit releases ownership", func(t *testing.T) {
		s := newStore(t)
		mustAdd(t, s, facts(runHashes(2, "native")), "first", Decision{Kind: "관측", Summary: "first"})
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, executable, "-test.run=^TestNativeHistoryStorageLocks$")
		cmd.Env = append(os.Environ(), "JAEKIT_HISTORY_LOCK_HELPER="+s.common)
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.Fatal(err)
		}
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		waited := false
		defer func() {
			if !waited {
				cmd.Process.Kill()
				cmd.Wait()
			}
		}()
		ready := make(chan bool, 1)
		go func() {
			scanner := bufio.NewScanner(stdout)
			for scanner.Scan() {
				if scanner.Text() == "HISTORY_LOCK_HELD" {
					ready <- true
					return
				}
			}
			ready <- false
		}()
		select {
		case ok := <-ready:
			if !ok {
				t.Fatal("lock owner failed before acquiring lock")
			}
		case <-ctx.Done():
			t.Fatal("lock owner did not become ready")
		}
		done := make(chan error, 1)
		go func() { _, _, err := s.Load(); done <- err }()
		assertHistoryWaiting(t, done)
		if err := cmd.Process.Kill(); err != nil {
			t.Fatal(err)
		}
		_ = cmd.Wait()
		waited = true
		awaitHistoryResult(t, done)
		// A lingering lock file is not lingering ownership.
		mustAdd(t, s, facts(runHashes(2, "native")), "after-exit", Decision{Kind: "관측", Summary: "owner gone"})
	})
}

func assertHistoryWaiting(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("operation escaped a held history lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
}
func awaitHistoryResult(t *testing.T, done <-chan error) {
	t.Helper()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("history operation did not resume after unlock")
	}
}

func TestNativeHistoryStorageIntegrity(t *testing.T) {
	t.Run("absence damage and unsupported", TestDamageAndUnsupportedAreDistinct)
	t.Run("trailing loss", TestTrailingEventLossIsDamage)
	t.Run("interrupted marker catches up", TestInterruptedRecordCatchesUp)
	t.Run("uncertain append retry", TestUncertainWriteCanBeConfirmedByRequest)
	t.Run("snapshot survives marker replacement", func(t *testing.T) {
		s := newStore(t)
		f := facts(runHashes(2, "native"))
		mustAdd(t, s, f, "first", Decision{Kind: "관측", Summary: "first"})
		tx, err := s.openStorage(false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.close()
		before, err := tx.read("last")
		if err != nil {
			t.Fatal(err)
		}
		// Model a reader that found no lock before the first writer created it,
		// then opened D1's marker before another writer publishes D2.
		snapshot, err := tx.(*windowsStorage).openSnapshot("last")
		if err != nil {
			t.Fatal(err)
		}
		defer snapshot.Close()
		second := mustAdd(t, s, f, "second", Decision{Kind: "관측", Summary: "second"})
		old, err := io.ReadAll(snapshot)
		if err != nil || !bytes.Equal(old, before) {
			t.Fatalf("old snapshot changed: %v %q; want %q", err, old, before)
		}
		last, err := s.readLast(tx)
		if err != nil || last.Seq != second.Seq || last.SHA256 != second.Hash() {
			t.Fatalf("new marker: %v %+v; want D%d %s", err, last, second.Seq, second.Hash())
		}
		if _, events, err := s.Load(); err != nil || len(events) != 2 {
			t.Fatalf("load after snapshot replacement: %v %d", err, len(events))
		}
	})
	t.Run("snapshot opens before rename source closes", func(t *testing.T) {
		s := newStore(t)
		mustAdd(t, s, facts(runHashes(2, "native")), "first", Decision{Kind: "관측", Summary: "first"})
		tx, err := s.openStorage(false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.close()
		native := tx.(*windowsStorage)
		h, err := native.open("last.tmp", false, true, windows.GENERIC_WRITE|windows.DELETE)
		if err != nil {
			t.Fatal(err)
		}
		staged := os.NewFile(uintptr(h), "last.tmp")
		defer staged.Close()
		replacement := []byte(fmt.Sprintf("{\"seq\":2,\"sha256\":%q}\n", startHash))
		if _, err := staged.Write(replacement); err != nil {
			t.Fatal(err)
		}
		if err := staged.Sync(); err != nil {
			t.Fatal(err)
		}
		if err := native.renameLast(h); err != nil {
			t.Fatal(err)
		}
		// The renamed staging handle still requests DELETE. New snapshots must
		// share that access during the interval before replaceLast closes it.
		got, err := tx.read("last")
		if err != nil || !bytes.Equal(got, replacement) {
			t.Fatalf("snapshot before source close: %v %q; want %q", err, got, replacement)
		}
	})
	t.Run("last replacement failure is uncertain", func(t *testing.T) {
		s := newStore(t)
		f := facts(runHashes(2, "native"))
		mustAdd(t, s, f, "first", Decision{Kind: "관측", Summary: "first"})
		path, err := windows.UTF16PtrFromString(s.last())
		if err != nil {
			t.Fatal(err)
		}
		// Deny delete sharing on the old marker. The event can be appended, but
		// atomic marker replacement must fail; this is a real Windows failure.
		h, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if h != windows.InvalidHandle {
				windows.CloseHandle(h)
			}
		}()
		d := Decision{Kind: "관측", Summary: "second"}
		if _, err := add(t, s, f, "second", d); Code(err) != "write_uncertain" {
			t.Fatalf("last replacement failure: %v", err)
		}
		if _, events, err := s.Load(); err != nil || len(events) != 2 {
			t.Fatalf("interrupted append query: %v %d", err, len(events))
		}
		windows.CloseHandle(h)
		h = windows.InvalidHandle
		if _, err := add(t, s, f, "second", d); !isReplay(err, "D2") {
			t.Fatalf("uncertain replay: %v", err)
		}
		data, err := os.ReadFile(s.file())
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.file(), data[:bytes.IndexByte(data, '\n')+1], 0o644); err != nil {
			t.Fatal(err)
		}
		if _, _, err := s.Load(); Code(err) != "history_corrupt" {
			t.Fatalf("caught-up marker must detect lost D2: %v", err)
		}
	})
}

func historyJunction(t *testing.T, link, target string) {
	t.Helper()
	if output, err := exec.Command("cmd.exe", "/c", "mklink", "/J", link, target).CombinedOutput(); err != nil {
		t.Fatalf("create junction: %v: %s", err, output)
	}
	t.Cleanup(func() {
		if err := os.Remove(link); err != nil && !os.IsNotExist(err) {
			t.Errorf("remove junction: %v", err)
		}
	})
}

func TestNativeHistoryStoragePathSafety(t *testing.T) {
	for _, entry := range []string{"common", "ha", "history", "start", "events.jsonl", "lock", "last", "last.tmp"} {
		t.Run("junction "+entry, func(t *testing.T) {
			s := newStore(t)
			f := facts(runHashes(2, "native"))
			mustAdd(t, s, f, "first", Decision{Kind: "관측", Summary: "first"})
			outside := t.TempDir()
			sentinel := filepath.Join(outside, "sentinel")
			original := []byte("outside must remain untouched")
			if err := os.WriteFile(sentinel, original, 0o644); err != nil {
				t.Fatal(err)
			}
			var victim string
			switch entry {
			case "common":
				victim = s.common
			case "ha":
				victim = s.dirs()[0]
			case "history":
				victim = s.dirs()[1]
			case "start":
				victim = s.Dir()
			default:
				victim = filepath.Join(s.Dir(), entry)
			}
			moved := victim + ".original"
			if err := os.Rename(victim, moved); err != nil && !os.IsNotExist(err) {
				t.Fatal(err)
			}
			historyJunction(t, victim, outside)
			// last.tmp is an append-only auxiliary path, so reads need not inspect it.
			if entry != "last.tmp" {
				if _, _, err := s.Load(); Code(err) != "history_unreadable" {
					t.Errorf("junction query: %v", err)
				}
			}
			_, err := add(t, s, f, "second", Decision{Kind: "관측", Summary: "second"})
			want := "history_unreadable"
			if entry == "last.tmp" {
				want = "write_uncertain"
			}
			if Code(err) != want {
				t.Errorf("junction append: %v; want %s", err, want)
			}
			got, err := os.ReadFile(sentinel)
			if err != nil || !bytes.Equal(got, original) {
				t.Fatalf("outside changed: %v %q", err, got)
			}
			entries, err := os.ReadDir(outside)
			if err != nil || len(entries) != 1 {
				t.Fatalf("outside storage created: %v %v", err, entries)
			}
		})
	}
	t.Run("open ancestors and lock cannot be replaced", func(t *testing.T) {
		s := newStore(t)
		mustAdd(t, s, facts(runHashes(2, "native")), "first", Decision{Kind: "관측", Summary: "first"})
		tx, err := s.openStorage(false)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.close()
		unlock, _, err := tx.acquire(true)
		if err != nil {
			t.Fatal(err)
		}
		defer unlock()
		for _, p := range append([]string{s.common}, append(s.dirs(), filepath.Join(s.Dir(), "lock"))...) {
			if err := os.Rename(p, p+".replaced"); err == nil {
				t.Fatalf("pinned path was replaced: %s", p)
			}
		}
		if _, events, _, err := s.load(tx); err != nil || len(events) != 1 {
			t.Fatalf("pinned load: %v %d", err, len(events))
		}
	})
	t.Run("hard-linked files cannot redirect reads or writes", func(t *testing.T) {
		for _, name := range []string{"events.jsonl", "lock", "last", "last.tmp"} {
			t.Run(name, func(t *testing.T) {
				s := newStore(t)
				mustAdd(t, s, facts(runHashes(2, "native")), "first", Decision{Kind: "관측", Summary: "first"})
				outside := filepath.Join(t.TempDir(), "outside")
				original := []byte("outside original")
				if err := os.WriteFile(outside, original, 0o644); err != nil {
					t.Fatal(err)
				}
				victim := filepath.Join(s.Dir(), name)
				if err := os.Remove(victim); err != nil && !os.IsNotExist(err) {
					t.Fatal(err)
				}
				if err := os.Link(outside, victim); err != nil {
					t.Fatal(err)
				}
				if name != "last.tmp" {
					if _, _, err := s.Load(); Code(err) != "history_unreadable" {
						t.Errorf("hard-link load: %v", err)
					}
				}
				_, err := add(t, s, facts(runHashes(2, "native")), "second", Decision{Kind: "관측", Summary: "second"})
				want := "history_unreadable"
				if name == "last.tmp" {
					want = "write_uncertain"
				}
				if Code(err) != want {
					t.Errorf("hard-link append: %v", err)
				}
				got, err := os.ReadFile(outside)
				if err != nil || !bytes.Equal(got, original) {
					t.Fatalf("outside changed: %v %q", err, got)
				}
			})
		}
	})
}
