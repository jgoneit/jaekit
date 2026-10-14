//go:build !windows && !darwin && !linux

package history

import "golang.org/x/sys/unix"

const historyDirectoryAccess = unix.O_RDONLY
