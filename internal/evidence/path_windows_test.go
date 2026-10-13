package evidence

import "testing"

func TestNativeWindowsEvidencePaths(t *testing.T) {
	for _, path := range []string{"C:relative", "C:/absolute", "file:stream", "NUL", "checks/CON.txt", "checks/COM1"} {
		if ValidPath(path) {
			t.Errorf("unsafe Windows path accepted: %q", path)
		}
	}
	if !ValidPath("checks/한글 path.json") {
		t.Fatal("ordinary repository path refused")
	}
}
