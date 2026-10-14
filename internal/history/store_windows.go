package history

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Every directory is kept open without FILE_SHARE_DELETE until the operation
// finishes. Descendants are opened relative to those handles, never by joining
// an unchecked path. This prevents a parent or lock replacement from changing
// the storage identity between validation, locking, and I/O.
type windowsStorage struct{ dirs []windows.Handle }

func (s *Store) openStorage(create bool) (storage, error) {
	absolute, err := filepath.Abs(s.common)
	if err != nil {
		return nil, err
	}
	volume := filepath.VolumeName(absolute)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(absolute) {
		return nil, fmt.Errorf("history requires a local drive path")
	}
	rootPath, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil {
		return nil, err
	}
	if windows.GetDriveType(rootPath) == windows.DRIVE_REMOTE {
		return nil, fmt.Errorf("network history storage is unsupported")
	}
	rootName, err := windows.UTF16PtrFromString(`\\?\` + volume + `\`)
	if err != nil {
		return nil, err
	}
	root, err := windows.CreateFile(rootName, windows.FILE_READ_ATTRIBUTES|windows.FILE_TRAVERSE|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	tx := &windowsStorage{dirs: []windows.Handle{root}}
	ok := false
	defer func() {
		if !ok {
			tx.close()
		}
	}()
	if err := plainHandle(root, true); err != nil {
		return nil, err
	}
	parts := strings.Split(strings.TrimPrefix(absolute[len(volume):], `\`), `\`)
	if len(parts) == 1 && parts[0] == "" {
		parts = nil
	}
	for _, part := range parts {
		child, err := tx.open(part, true, false, 0)
		if err != nil {
			return nil, err
		}
		tx.dirs = append(tx.dirs, child)
	}
	for _, part := range []string{"ha", "history", s.start} {
		child, err := tx.open(part, true, create, 0)
		if err != nil {
			return nil, err
		}
		tx.dirs = append(tx.dirs, child)
	}
	ok = true
	return tx, nil
}

func (s *windowsStorage) close() {
	for i := len(s.dirs) - 1; i >= 0; i-- {
		windows.CloseHandle(s.dirs[i])
	}
	s.dirs = nil
}

func (s *windowsStorage) directory() windows.Handle { return s.dirs[len(s.dirs)-1] }

// open does not truncate or follow a reparse point, even when creating a file.
// Inspect the exact handle before any content read, write, or truncate.
func (s *windowsStorage) open(name string, directory, create bool, access uint32) (windows.Handle, error) {
	return s.openWithShare(name, directory, create, access, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE)
}

func (s *windowsStorage) openWithShare(name string, directory, create bool, access, share uint32) (windows.Handle, error) {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `\/:`) || strings.TrimRight(name, " .") != name {
		return 0, fmt.Errorf("invalid history path component %q", name)
	}
	objectName, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return 0, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: s.directory(), ObjectName: objectName,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	disposition := uint32(windows.FILE_OPEN)
	if create {
		disposition = windows.FILE_OPEN_IF
	}
	options := uint32(windows.FILE_SYNCHRONOUS_IO_NONALERT | windows.FILE_OPEN_REPARSE_POINT)
	if directory {
		options |= windows.FILE_DIRECTORY_FILE
		access = windows.FILE_TRAVERSE | windows.FILE_READ_ATTRIBUTES
	} else {
		options |= windows.FILE_NON_DIRECTORY_FILE
	}
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, access|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE, &oa, &iosb, nil, windows.FILE_ATTRIBUTE_NORMAL,
		share, disposition, options, 0, 0)
	if err != nil {
		var status windows.NTStatus
		if errors.As(err, &status) {
			err = status.Errno()
		}
		return 0, err
	}
	if err := plainHandle(h, directory); err != nil {
		windows.CloseHandle(h)
		return 0, err
	}
	return h, nil
}

func plainHandle(h windows.Handle, directory bool) error {
	kind, err := windows.GetFileType(h)
	if err != nil {
		return err
	}
	if kind != windows.FILE_TYPE_DISK {
		return fmt.Errorf("history storage is not a disk file")
	}
	var info windows.ByHandleFileInformation
	if err := windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
		return fmt.Errorf("history storage is a reparse point")
	}
	if (info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return fmt.Errorf("unexpected history storage file type")
	}
	// A second hard-link name could otherwise redirect writes into an unrelated
	// file even though no path component is a reparse point.
	if !directory && info.NumberOfLinks != 1 {
		return fmt.Errorf("history storage has multiple hard links")
	}
	return nil
}

// Snapshots keep reading the validated file even if its name is replaced. Only
// these read handles share deletion; directories, locks, and writers stay pinned.
func (s *windowsStorage) openSnapshot(name string) (*os.File, error) {
	h, err := s.openWithShare(name, false, false, windows.GENERIC_READ,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), name), nil
}

func (s *windowsStorage) read(name string) ([]byte, error) {
	f, err := s.openSnapshot(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func (s *windowsStorage) appendFile() (appendFile, error) {
	// Match Go's Windows O_APPEND access mask: generic write rights except
	// FILE_WRITE_DATA, so WriteFile appends and File.Sync can flush the handle.
	access := uint32(windows.FILE_GENERIC_WRITE &^ windows.FILE_WRITE_DATA)
	h, err := s.open("events.jsonl", false, true, access)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(h), "events.jsonl"), nil
}

func (s *windowsStorage) acquire(shared bool) (func(), bool, error) {
	access := uint32(windows.GENERIC_READ)
	if !shared {
		access |= windows.GENERIC_WRITE
	}
	h, err := s.open("lock", false, !shared, access)
	if shared && errors.Is(err, fs.ErrNotExist) {
		return func() {}, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	f := os.NewFile(uintptr(h), "lock")
	unlock, err := lockHistoryFile(f, shared)
	if err != nil {
		f.Close()
		return nil, false, err
	}
	return func() { unlock(); f.Close() }, true, nil
}

func lockHistoryFile(f *os.File, shared bool) (func(), error) {
	var overlapped windows.Overlapped
	flags := uint32(0)
	if !shared {
		flags = windows.LOCKFILE_EXCLUSIVE_LOCK
	}
	if err := windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, &overlapped); err != nil {
		return nil, err
	}
	return func() { windows.UnlockFileEx(windows.Handle(f.Fd()), 0, 1, 0, &overlapped) }, nil
}

func (s *windowsStorage) lockPresent() (bool, error) {
	h, err := s.open("lock", false, false, windows.FILE_READ_ATTRIBUTES)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	windows.CloseHandle(h)
	return true, nil
}

func (s *windowsStorage) replaceLast(data []byte) error {
	// Refuse an existing unsafe last entry. The replacement below is relative
	// to the pinned directory and replaces an entry; it never follows it.
	old, err := s.open("last", false, false, windows.FILE_READ_ATTRIBUTES)
	if err == nil {
		windows.CloseHandle(old)
	} else if !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	h, err := s.open("last.tmp", false, true, windows.GENERIC_WRITE|windows.DELETE)
	if err != nil {
		return err
	}
	f := os.NewFile(uintptr(h), "last.tmp")
	defer f.Close()
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
	if err := s.renameLast(h); err != nil {
		return err
	}
	return f.Close()
}

func (s *windowsStorage) renameLast(h windows.Handle) error {
	// The Ex form starts with ULONG Flags; the classic form reads its first byte
	// as ReplaceIfExists. Both then use pointer-aligned RootDirectory, a byte
	// length, and a counted UTF-16 name.
	type renameInfo struct {
		Flags  uint32
		Root   windows.Handle
		Length uint32
		Name   [1]uint16
	}
	name, _ := windows.UTF16FromString("last")
	name = name[:len(name)-1]
	size := int(unsafe.Offsetof(renameInfo{}.Name)) + len(name)*2
	buf := make([]byte, size)
	info := (*renameInfo)(unsafe.Pointer(&buf[0]))
	info.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS | windows.FILE_RENAME_POSIX_SEMANTICS
	info.Root = s.directory()
	info.Length = uint32(len(name) * 2)
	copy(unsafe.Slice(&info.Name[0], len(name)), name)
	var iosb windows.IO_STATUS_BLOCK
	// An initial unlocked reader can retain the old marker while a writer
	// replaces it. POSIX semantics preserve that reader's snapshot handle.
	const fileRenameInformationEx = 65
	if err := windows.NtSetInformationFile(h, &iosb, &buf[0], uint32(size), fileRenameInformationEx); err == nil {
		return nil
	}
	// Match Go's Renameat fallback for filesystems without the Ex form or POSIX
	// semantics. Such filesystems still reject replacement of an open marker.
	info.Flags = windows.FILE_RENAME_REPLACE_IF_EXISTS
	return windows.NtSetInformationFile(h, &iosb, &buf[0], uint32(size), windows.FileRenameInformation)
}
