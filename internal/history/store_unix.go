//go:build !windows

package history

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// Directory descriptors keep the storage identity fixed if an ancestor is
// renamed or replaced. Every descendant operation is relative to these handles.
type unixStorage struct{ dirs []int }

const historyDirectoryFlags = historyDirectoryAccess | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

func (s *Store) openStorage(create bool) (storage, error) {
	// Permit upstream system aliases such as macOS /var, but reject a symlink
	// at the common directory itself. Clean away a trailing slash so it cannot
	// bypass the final-component O_NOFOLLOW check.
	root, err := unix.Open(filepath.Clean(s.common), historyDirectoryFlags, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: s.common, Err: err}
	}
	tx := &unixStorage{dirs: []int{root}}
	ok := false
	defer func() {
		if !ok {
			tx.close()
		}
	}()
	for _, name := range []string{"ha", "history", s.start} {
		child, err := unix.Openat(tx.directory(), name, historyDirectoryFlags, 0)
		if create && errors.Is(err, fs.ErrNotExist) {
			if err := unix.Mkdirat(tx.directory(), name, 0o755); err != nil && !errors.Is(err, fs.ErrExist) {
				return nil, &os.PathError{Op: "mkdirat", Path: name, Err: err}
			}
			child, err = unix.Openat(tx.directory(), name, historyDirectoryFlags, 0)
		}
		if err != nil {
			return nil, &os.PathError{Op: "openat", Path: name, Err: err}
		}
		tx.dirs = append(tx.dirs, child)
	}
	ok = true
	return tx, nil
}

func (s *unixStorage) directory() int { return s.dirs[len(s.dirs)-1] }

func (s *unixStorage) close() {
	for i := len(s.dirs) - 1; i >= 0; i-- {
		unix.Close(s.dirs[i])
	}
	s.dirs = nil
}

// Open without truncating, then inspect the returned descriptor before any
// content access. NONBLOCK prevents a substituted FIFO from blocking validation.
func (s *unixStorage) open(name string, flags int) (*os.File, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\x00") {
		return nil, fmt.Errorf("invalid history path component %q", name)
	}
	openFlags := flags | unix.O_NOFOLLOW | unix.O_NONBLOCK | unix.O_NOCTTY | unix.O_CLOEXEC
	fd, err := unix.Openat(s.directory(), name, openFlags, 0o644)
	if flags&unix.O_CREAT != 0 && errors.Is(err, fs.ErrNotExist) {
		// Concurrent first creators on macOS can get ENOENT after another
		// creator wins. Reopen that existing leaf through the same directory,
		// retaining every safety flag and never creating on this second try.
		fd, err = unix.Openat(s.directory(), name, openFlags&^unix.O_CREAT, 0)
	}
	if err != nil {
		return nil, &os.PathError{Op: "openat", Path: name, Err: err}
	}
	if _, err := historyFileInfo(fd, name); err != nil {
		unix.Close(fd)
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func historyFileInfo(fd int, name string) (unix.Stat_t, error) {
	var info unix.Stat_t
	if err := unix.Fstat(fd, &info); err != nil {
		return info, &os.PathError{Op: "fstat", Path: name, Err: err}
	}
	if info.Mode&unix.S_IFMT != unix.S_IFREG {
		return info, fmt.Errorf("%s is not a regular history file", name)
	}
	// A second name could otherwise expose unrelated data to reads or redirect
	// an append or staging-file truncate into an outside file.
	if info.Nlink != 1 {
		return info, fmt.Errorf("%s does not have exactly one hard link", name)
	}
	return info, nil
}

func (s *unixStorage) read(name string) ([]byte, error) {
	f, err := s.open(name, unix.O_RDONLY)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if name == "events.jsonl" {
		return readEventLines(f)
	}
	return io.ReadAll(f)
}

func (s *unixStorage) appendFile() (appendFile, error) {
	return s.open("events.jsonl", unix.O_WRONLY|unix.O_APPEND|unix.O_CREAT)
}

// Queries create nothing. Without a lock file, the caller rechecks its presence
// after reading in case the first registration started during the snapshot.
func (s *unixStorage) acquire(shared bool) (func(), bool, error) {
	flags := unix.O_RDONLY
	if !shared {
		flags = unix.O_RDWR | unix.O_CREAT
	}
	f, err := s.open("lock", flags)
	if shared && errors.Is(err, fs.ErrNotExist) {
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	unlock, err := lockHistoryFile(f, shared)
	if err != nil {
		f.Close()
		return nil, false, err
	}
	return func() { unlock(); f.Close() }, true, nil
}

func (s *unixStorage) lockPresent() (bool, error) {
	f, err := s.open("lock", unix.O_RDONLY)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	f.Close()
	return true, nil
}

// replaceLast replaces a directory entry, never following its destination.
func (s *unixStorage) replaceLast(data []byte) error {
	old, err := s.open("last", unix.O_RDONLY)
	if err == nil {
		old.Close()
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	f, err := s.open("last.tmp", unix.O_WRONLY|unix.O_CREAT)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	// A reused staging file becomes the readable marker after publication.
	// Add only missing owner access, preserving its other mode bits.
	if info.Mode().Perm()&0o600 != 0o600 {
		if err := f.Chmod(info.Mode() | 0o600); err != nil {
			return err
		}
	}
	if err := f.Truncate(0); err != nil {
		return err
	}
	if n, err := f.Write(data); err != nil {
		return err
	} else if n != len(data) {
		return io.ErrShortWrite
	}
	if err := f.Sync(); err != nil {
		return err
	}
	return s.publishLast(f)
}

// renameHistoryMarker separates the rename boundary for replacement fixtures.
var renameHistoryMarker = unix.Renameat

func (s *unixStorage) publishLast(f *os.File) error {
	// Keep the written inode open throughout publication. A pathname rename
	// alone could publish a different file substituted for last.tmp.
	if err := s.checkMarkerIdentity("last.tmp", f); err != nil {
		return err
	}
	if err := renameHistoryMarker(s.directory(), "last.tmp", s.directory(), "last"); err != nil {
		return &os.LinkError{Op: "renameat", Old: "last.tmp", New: "last", Err: err}
	}
	if err := s.checkMarkerIdentity("last", f); err != nil {
		return err
	}
	return f.Close()
}

func (s *unixStorage) checkMarkerIdentity(name string, written *os.File) error {
	info, err := historyFileInfo(int(written.Fd()), "written marker")
	if err != nil {
		return err
	}
	var named unix.Stat_t
	// Fstatat needs only directory search permission, so an existing write-only
	// staging file remains usable. Never follow a substituted symlink.
	if err := unix.Fstatat(s.directory(), name, &named, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return &os.PathError{Op: "fstatat", Path: name, Err: err}
	}
	if named.Mode&unix.S_IFMT != unix.S_IFREG || named.Nlink != 1 || named.Dev != info.Dev || named.Ino != info.Ino {
		return fmt.Errorf("%s changed during marker replacement", name)
	}
	return nil
}

// lockHistoryFile is shared with portable lock/concurrency fixtures.
func lockHistoryFile(f *os.File, shared bool) (func(), error) {
	op := syscall.LOCK_EX
	if shared {
		op = syscall.LOCK_SH
	}
	if err := syscall.Flock(int(f.Fd()), op); err != nil {
		return nil, err
	}
	return func() { syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }, nil
}
