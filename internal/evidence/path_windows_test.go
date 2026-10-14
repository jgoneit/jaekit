package evidence

import (
	"bytes"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestNativeWindowsEvidencePaths(t *testing.T) {
	for _, path := range []string{"C:relative", "C:/absolute", "file:stream", "NUL", "checks/CON", "checks/CONIN$", "checks/CONOUT$", "checks/COM1"} {
		if ValidPath(path) {
			t.Errorf("unsafe Windows path accepted: %q", path)
		}
	}
	if !ValidPath("checks/한글 path.json") {
		t.Fatal("ordinary repository path refused")
	}

	// Windows versions differ on device names with an extension. Go 1.26
	// filepath.IsLocal consults this native API for such names:
	// https://github.com/golang/go/blob/go1.26.0/src/internal/filepathlite/path_windows.go
	// Query before opening so a reserved spelling is never opened as a device.
	deviceName := syscall.NewLazyDLL("ntdll.dll").NewProc("RtlIsDosDeviceName_U")
	if err := deviceName.Find(); err != nil {
		t.Fatal(err)
	}
	name, err := syscall.UTF16PtrFromString("CON.txt")
	if err != nil {
		t.Fatal(err)
	}
	reserved, _, _ := deviceName.Call(uintptr(unsafe.Pointer(name)))
	const relative = "checks/CON.txt"
	if reserved != 0 {
		if ValidPath(relative) {
			t.Fatal("actual Windows device name accepted")
		}
		return
	}
	if !ValidPath(relative) {
		t.Fatal("ordinary extension-bearing Windows name refused")
	}
	// Acceptance must refer to a real regular file with the same identity and
	// bytes through both the opened handle and its repository-relative spelling.
	destination := filepath.Join(t.TempDir(), filepath.FromSlash(relative))
	if err := os.Mkdir(filepath.Dir(destination), 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() {
		t.Fatalf("accepted name is not a regular file: %v", err)
	}
	want := []byte("regular Windows file identity\n")
	if _, err := f.Write(want); err != nil {
		t.Fatal(err)
	}
	pathInfo, err := os.Lstat(destination)
	if err != nil || !pathInfo.Mode().IsRegular() || !os.SameFile(info, pathInfo) {
		t.Fatalf("accepted path does not identify the created regular file: %v", err)
	}
	got, err := os.ReadFile(destination)
	if err != nil || !bytes.Equal(got, want) {
		t.Fatalf("accepted regular file bytes differ: %q %v", got, err)
	}
}
