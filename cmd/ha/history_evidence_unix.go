//go:build darwin || linux

package main

import (
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// Search-only directory handles preserve access through directories without
// listing permission. O_DIRECTORY rejects a replaced FIFO before opening it;
// O_NOFOLLOW rejects links introduced after path canonicalization.
const fileEvidenceDirectoryFlags = fileEvidenceDirectoryAccess | unix.O_DIRECTORY | unix.O_NOFOLLOW | unix.O_CLOEXEC

func openFileEvidenceRoot(name string) (*os.File, error) {
	fd, err := unix.Open(name, fileEvidenceDirectoryFlags, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(fd), name), nil
}

func openConfinedFileEvidence(root *os.File, parent, base string) (*os.File, error) {
	if !filepath.IsLocal(parent) || !filepath.IsLocal(base) || base == "." || filepath.Base(base) != base {
		return nil, unix.EINVAL
	}
	rootFD := int(root.Fd())
	fd := rootFD
	defer func() {
		if fd != rootFD {
			unix.Close(fd)
		}
	}()
	for _, part := range strings.Split(parent, string(filepath.Separator)) {
		if part == "." {
			continue
		}
		if part == "" || part == ".." {
			return nil, unix.EINVAL
		}
		next, err := unix.Openat(fd, part, fileEvidenceDirectoryFlags, 0)
		if err != nil {
			return nil, err
		}
		// The child descriptor pins the next directory before its parent is
		// released. Keep only the root and current directory, independent of
		// path depth; the deferred close also covers a later open failure.
		if fd != rootFD {
			unix.Close(fd)
		}
		fd = next
	}
	file, err := unix.Openat(fd, base, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(file), filepath.Join(root.Name(), parent, base)), nil
}
