package record

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	lockFileEx   = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")
	unlockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("UnlockFileEx")
)

// Lock one byte through a synchronous handle. Windows releases the lock when
// its handle/process ends; the existence of the lock file grants no ownership.
func lockFile(f *os.File) (func(), error) {
	var overlap syscall.Overlapped
	ok, _, err := lockFileEx.Call(f.Fd(), 2, 0, 1, 0, uintptr(unsafe.Pointer(&overlap)))
	if ok == 0 {
		return nil, fmt.Errorf("LockFileEx: %w", err)
	}
	return func() {
		unlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlap)))
	}, nil
}
