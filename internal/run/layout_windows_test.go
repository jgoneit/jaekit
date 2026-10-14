package run

import (
	"testing"
	"unsafe"
)

func TestNativeJobLayout(t *testing.T) {
	for _, row := range []struct {
		name      string
		got, want uintptr
	}{
		{"basic limits", unsafe.Sizeof(jobBasicLimits{}), 64},
		{"extended limits", unsafe.Sizeof(jobExtendedLimits{}), 144},
		{"accounting", unsafe.Sizeof(jobAccounting{}), 48},
		{"thread entry", unsafe.Sizeof(threadEntry{}), 28},
	} {
		if row.got != row.want {
			t.Errorf("%s ABI size %d want %d", row.name, row.got, row.want)
		}
	}
}
