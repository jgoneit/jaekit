//go:build protectedcheckfollowups

package protectedchecks

import (
	"path/filepath"
	"testing"
)

const (
	userStep  = "guidance may add a user step"
	localPath = "local path in new guidance"
)

// versionLine puts one "Seal 0.1.14" guidance line in a new section at the
// end of doc, outside the sections the producer already reads.
func versionLine(t *testing.T, doc, line string) string {
	t.Helper()
	root := docTree(t)
	faithful(t, "authoring", root, "AC-6")
	faithful(t, "authoring", root, "AC-10")
	appendText(t, filepath.Join(root, doc), "## Notes on earlier checks\n\n"+line)
	return root
}

func TestProtectedAC2_VersionLineUserStep(t *testing.T) {
	const id = "version-line-step-missed"
	const line = "Checks written or changed from Seal 0.1.14 on: ask the user to approve each one."
	root := versionLine(t, bundleDoc, line)
	if got := withText(authoring(t, root, "AC-6").Problems, userStep, "ask the user to approve each one"); len(got) == 0 {
		t.Errorf("jaekit:violation=%s the Seal 0.1.14 line in %s that asks for approval is not reported", id, bundleDoc)
	}
	only := versionLine(t, earlierDoc, line)
	if got := withText(authoring(t, only, "AC-6").Problems, userStep, "ask the user to approve each one"); len(got) > 0 {
		t.Errorf("jaekit:violation=%s the Seal 0.1.14 line only in %s, outside the lookup, is reported: %q", id, earlierDoc, got)
	}
}

func TestProtectedAC2_VersionLineLocalPath(t *testing.T) {
	const id = "version-line-path-missed"
	const line = "Checks written or changed from Seal 0.1.14 on read /Users/example/notes first."
	root := versionLine(t, bundleDoc, line)
	if got := withText(authoring(t, root, "AC-10").Problems, localPath, "/Users/e"); len(got) == 0 {
		t.Errorf("jaekit:violation=%s the Seal 0.1.14 line in %s with a local path is not reported", id, bundleDoc)
	}
	only := versionLine(t, earlierDoc, line)
	if got := withText(authoring(t, only, "AC-10").Problems, localPath, "/Users/e"); len(got) > 0 {
		t.Errorf("jaekit:violation=%s the Seal 0.1.14 line only in %s, outside the lookup, is reported: %q", id, earlierDoc, got)
	}
}
