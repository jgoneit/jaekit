package main

import (
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Evidence keeps its original portable spelling in the event. These names
// cannot denote an ordinary Windows file: reject them before even resolving
// parents, rather than probing a device, pipe or alternate data stream.
func validFileEvidencePath(name string) bool {
	if name == "" || strings.ContainsAny(name, `\:`) || strings.HasPrefix(name, "/") {
		return false
	}
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || !filepath.IsLocal(part) || strings.HasSuffix(part, ".") || strings.HasSuffix(part, " ") {
			return false
		}
	}
	return true
}

// The root is resolved by the caller. Walk from a local volume root so that a
// reparse point introduced into any ancestor cannot redirect this open.
func openFileEvidenceRoot(name string) (*os.File, error) {
	volume := filepath.VolumeName(name)
	if len(volume) != 2 || volume[1] != ':' || !filepath.IsAbs(name) {
		return nil, windows.ERROR_INVALID_NAME
	}
	p, err := windows.UTF16PtrFromString(volume + `\`)
	if err != nil {
		return nil, err
	}
	if windows.GetDriveType(p) == windows.DRIVE_REMOTE {
		return nil, windows.ERROR_INVALID_NAME
	}
	h, err := windows.CreateFile(p, windows.FILE_TRAVERSE|windows.FILE_READ_ATTRIBUTES|windows.SYNCHRONIZE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil, windows.OPEN_EXISTING,
		windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
	if err != nil {
		return nil, err
	}
	f := os.NewFile(uintptr(h), volume+`\`)
	if err := checkEvidenceHandle(h, true); err != nil {
		f.Close()
		return nil, err
	}
	for _, part := range strings.Split(strings.TrimPrefix(name[len(volume):], `\`), `\`) {
		if part == "" {
			continue
		}
		next, err := openEvidenceChild(f, part, true)
		f.Close()
		if err != nil {
			return nil, err
		}
		f = next
	}
	return f, nil
}

func openEvidenceChild(parent *os.File, name string, directory bool) (*os.File, error) {
	if !validFileEvidencePath(name) || strings.Contains(name, "/") {
		return nil, windows.ERROR_INVALID_NAME
	}
	u, err := windows.NewNTUnicodeString(name)
	if err != nil {
		return nil, err
	}
	oa := windows.OBJECT_ATTRIBUTES{RootDirectory: windows.Handle(parent.Fd()), ObjectName: u,
		Attributes: windows.OBJ_CASE_INSENSITIVE | windows.OBJ_DONT_REPARSE}
	oa.Length = uint32(unsafe.Sizeof(oa))
	access := uint32(windows.FILE_READ_ATTRIBUTES | windows.SYNCHRONIZE)
	options := uint32(windows.FILE_OPEN_REPARSE_POINT | windows.FILE_SYNCHRONOUS_IO_NONALERT)
	if directory {
		access |= windows.FILE_TRAVERSE
		options |= windows.FILE_DIRECTORY_FILE
	} else {
		access |= windows.FILE_READ_DATA
		options |= windows.FILE_NON_DIRECTORY_FILE
	}
	var h windows.Handle
	var iosb windows.IO_STATUS_BLOCK
	err = windows.NtCreateFile(&h, access, &oa, &iosb, nil, 0,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
		windows.FILE_OPEN, options, 0, 0)
	if err != nil {
		if status, ok := err.(windows.NTStatus); ok {
			err = status.Errno()
		}
		return nil, err
	}
	f := os.NewFile(uintptr(h), filepath.Join(parent.Name(), name))
	if err := checkEvidenceHandle(h, directory); err != nil {
		f.Close()
		return nil, err
	}
	return f, nil
}

func checkEvidenceHandle(h windows.Handle, directory bool) error {
	kind, err := windows.GetFileType(h)
	if err != nil {
		return err
	}
	var info windows.ByHandleFileInformation
	if err = windows.GetFileInformationByHandle(h, &info); err != nil {
		return err
	}
	if kind != windows.FILE_TYPE_DISK || info.FileAttributes&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 ||
		(info.FileAttributes&windows.FILE_ATTRIBUTE_DIRECTORY != 0) != directory {
		return windows.ERROR_ACCESS_DENIED
	}
	return nil
}

func openConfinedFileEvidence(root *os.File, parent, base string) (*os.File, error) {
	if !filepath.IsLocal(parent) || !validFileEvidencePath(base) || strings.Contains(base, "/") {
		return nil, windows.ERROR_INVALID_NAME
	}
	dir := root
	defer func() {
		if dir != root {
			dir.Close()
		}
	}()
	for _, part := range strings.Split(parent, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		next, err := openEvidenceChild(dir, part, true)
		if err != nil {
			return nil, err
		}
		if dir != root {
			dir.Close()
		}
		dir = next
	}
	return openEvidenceChild(dir, base, false)
}

// Resolve the original spelling again, but never read through that pathname.
// A second anchored walk verifies the root and file identities before hashing;
// new junctions during either walk are rejected by the kernel. Stable parent
// links inside the repository still work, as on Unix.
func openOriginalFileEvidence(root string, pinned *os.File, full string) (*os.File, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return nil, err
	}
	current, err := openFileEvidenceRoot(realRoot)
	if err != nil {
		return nil, err
	}
	defer current.Close()
	before, err := pinned.Stat()
	if err != nil {
		return nil, err
	}
	after, err := current.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(before, after) {
		return nil, windows.ERROR_ACCESS_DENIED
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(full))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(realRoot, parent)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, windows.ERROR_ACCESS_DENIED
	}
	return openConfinedFileEvidence(current, rel, filepath.Base(full))
}
