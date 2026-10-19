package bundle

import (
	"reflect"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/goaldocs"
)

// Only directories holding a SPEC.md are goals; each contributes its
// amendments/ directory, and the bundle file list stays as it was.
func TestOtherGoalAmendments(t *testing.T) {
	root := t.TempDir()
	write(t, root, "specs/g/SPEC.md", spec)
	write(t, root, "specs/other/SPEC.md", spec)
	write(t, root, "specs/other/amendments/wording.md", "# 수정안\n")
	write(t, root, "specs/notes/README.md", "notes\n")
	write(t, root, "specs/z/SPEC.md", spec)
	g := &goaldocs.Goal{Dir: "specs/g"}
	got, err := OtherGoalAmendments(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"specs/other/amendments", "specs/z/amendments"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("OtherGoalAmendments = %#v, want %#v", got, want)
	}
	bundles, err := OtherGoalBundles(root, g)
	if err != nil {
		t.Fatal(err)
	}
	if len(bundles) != 2*len(goaldocs.BundleNames) {
		t.Fatalf("OtherGoalBundles = %#v", bundles)
	}
	for _, p := range bundles {
		if strings.Contains(p, goaldocs.AmendmentDir) || strings.HasPrefix(p, "specs/notes/") {
			t.Fatalf("OtherGoalBundles lists %s", p)
		}
	}
}
