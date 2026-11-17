//go:build protectedcheckfollowups

package protectedchecks

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const sentinelTest = "integration_evidence_boundaries.SourceIdentityEvidence.test_git_environment_cannot_redirect_snapshot_writes"

// The start of source_identity.git's Git call; the injected faults go right before it.
const gitCall = "    try:\n        result = subprocess.run([\"git\", \"-c\", \"core.hooksPath=/dev/null\", \"-c\", \"core.fsmonitor=false\", *arguments],"

// fakeGit puts fake-git.sh first on PATH as git. It returns the environment
// and the fake's path: a Go command resolves its name with the parent's PATH,
// so the emulation control calls the fake by path; Python's subprocess
// resolves git with the PATH of the environment it is given.
func fakeGit(t *testing.T) ([]string, string) {
	t.Helper()
	dir := t.TempDir()
	fake := filepath.Join(dir, "git")
	copyFile(t, filepath.Join(repoRoot(t), "tools", "goal-checks", "protected-check-followups", "fake-git.sh"), fake)
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatalf("fake git: %v", err)
	}
	state := filepath.Join(t.TempDir(), "pending")
	write(t, state, "")
	return []string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"JAEKIT_REAL_GIT=" + tool(t, "git"),
		"JAEKIT_FAKE_GIT_STATE=" + state,
	}, fake
}

func gitIn(t *testing.T, git, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command(git, args...)
	cmd.Dir, cmd.Env = dir, childEnv(env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// emulationWorks fails as an input failure unless the fake Git writes the lock
// after a commit and leaves a repository with maintenance.auto false alone.
func emulationWorks(t *testing.T) {
	t.Helper()
	for _, auto := range []string{"", "false"} {
		env, git := fakeGit(t)
		repo := t.TempDir()
		gitIn(t, git, repo, env, "init", "-q")
		gitIn(t, git, repo, env, "config", "user.name", "Synthetic")
		gitIn(t, git, repo, env, "config", "user.email", "synthetic@example.invalid")
		if auto != "" {
			gitIn(t, git, repo, env, "config", "maintenance.auto", auto)
		}
		gitIn(t, git, repo, env, "commit", "--allow-empty", "-qm", "synthetic")
		gitIn(t, git, repo, env, "status", "--short")
		_, err := os.Stat(filepath.Join(repo, ".git", "objects", "maintenance.lock"))
		if (auto == "") != (err == nil) {
			t.Fatalf("control: the emulated maintenance with maintenance.auto %q left the lock: %v", auto, err == nil)
		}
	}
}

func releaseChecks(root string) string { return filepath.Join(root, "tools", "release-checks") }

// toolsCopy copies the checked tree's tools/ into a new tree and injects a
// fault right before source_identity.git's Git call.
func toolsCopy(t *testing.T, fault string) string {
	t.Helper()
	tree := t.TempDir()
	copyDir(t, filepath.Join(repoRoot(t), "tools"), filepath.Join(tree, "tools"))
	if fault != "" {
		replaceOnce(t, filepath.Join(releaseChecks(tree), "source_identity.py"), gitCall, fault+gitCall)
	}
	return tree
}

func TestProtectedAC6_MaintenanceLockIgnored(t *testing.T) {
	const id = "maintenance-lock-fails-sentinel"
	emulationWorks(t)
	env, _ := fakeGit(t)
	r := runUnittest(t, releaseChecks(repoRoot(t)), env, sentinelTest)
	switch {
	case r.ok:
	case strings.Contains(r.output, "maintenance.lock"):
		t.Errorf("jaekit:violation=%s the sentinel comparison fails on the maintenance lock its own setup started:\n%s", id, tail(r.output, 12))
	default:
		t.Fatalf("the sentinel test fails for another reason:\n%s", tail(r.output, 30))
	}
}

func snapshotWriteCaught(t *testing.T, id, what, fault string) {
	t.Helper()
	if r := runUnittest(t, releaseChecks(toolsCopy(t, "")), nil, sentinelTest); !r.ok {
		t.Fatalf("control: the sentinel test fails on the unchanged copy:\n%s", tail(r.output, 30))
	}
	if r := runUnittest(t, releaseChecks(toolsCopy(t, fault)), nil, sentinelTest); r.ok {
		t.Errorf("jaekit:violation=%s the sentinel test passes when the snapshot %s", id, what)
	}
}

func TestProtectedAC6_SentinelWriteCaught(t *testing.T) {
	snapshotWriteCaught(t, "sentinel-write-not-caught", "writes objects into the sentinel through GIT_OBJECT_DIRECTORY",
		"    env.update({key: value for key, value in os.environ.items() if key == \"GIT_OBJECT_DIRECTORY\"})\n")
}

func TestProtectedAC6_OriginalWriteCaught(t *testing.T) {
	snapshotWriteCaught(t, "original-write-not-caught", "writes a file into the checked repository's .git",
		"    if os.path.isdir(os.path.join(root, \".git\")):\n        open(os.path.join(root, \".git\", \"jaekit-probe\"), \"a\").close()\n")
}
