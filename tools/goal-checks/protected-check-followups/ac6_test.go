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

// The assertions of the sentinel test that compare each repository's
// contents; a failed comparison prints its line in the traceback.
const (
	sentinelCompare = "self.assertEqual(contents(sentinel), other)"
	originalCompare = "self.assertEqual(contents(self.root), original)"
	// The temporary folder name of the sentinel repository.
	sentinelPrefix = "jaekit-git-sentinel-"
)

// The start of source_identity.git's Git call; the injected faults go right before it.
const gitCall = "    try:\n        result = subprocess.run([\"git\", \"-c\", \"core.hooksPath=/dev/null\", \"-c\", \"core.fsmonitor=false\", *arguments],"

// isolatedGit keeps the host's system and global Git configuration out of a
// run, so a host maintenance.auto setting cannot decide it.
func isolatedGit() []string {
	return []string{"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=" + os.DevNull}
}

// fakeGit puts fake-git.sh first on PATH as git. It returns the environment,
// the fake's path and the log of the Git directories it wrote a lock into. A
// Go command resolves its name with the parent's PATH, so the emulation
// control calls the fake by path; Python's subprocess resolves git with the
// PATH of the environment it is given.
func fakeGit(t *testing.T) (env []string, fake, log string) {
	t.Helper()
	dir := t.TempDir()
	fake = filepath.Join(dir, "git")
	copyFile(t, filepath.Join(repoRoot(t), "tools", "goal-checks", "protected-check-followups", "fake-git.sh"), fake)
	if err := os.Chmod(fake, 0o755); err != nil {
		t.Fatalf("fake git: %v", err)
	}
	state := filepath.Join(t.TempDir(), "pending")
	write(t, state, "")
	write(t, state+".log", "")
	env = append([]string{
		"PATH=" + dir + string(os.PathListSeparator) + os.Getenv("PATH"),
		"JAEKIT_REAL_GIT=" + tool(t, "git"),
		"JAEKIT_FAKE_GIT_STATE=" + state,
	}, isolatedGit()...)
	return env, fake, state + ".log"
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
		env, git, _ := fakeGit(t)
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
	env, _, log := fakeGit(t)
	r := runUnittest(t, releaseChecks(repoRoot(t)), env, sentinelTest)
	if r.ok {
		return
	}
	if !caught(t, r, "the sentinel test under the emulated maintenance") || !strings.Contains(r.output, sentinelCompare) ||
		!strings.Contains(read(t, log), sentinelPrefix) {
		t.Fatalf("the sentinel test fails for another reason:\n%s", tail(r.output, 30))
	}
	t.Errorf("jaekit:violation=%s the sentinel comparison fails on the maintenance lock its own commit started:\n%s", id, tail(r.output, 12))
}

// writeCaught runs the sentinel test on an unchanged copy (it must pass) and
// on a copy with fault injected; the faulted run must fail the comparison
// named by compare, not end in an error.
func writeCaught(t *testing.T, fault, compare string) bool {
	t.Helper()
	if r := runUnittest(t, releaseChecks(toolsCopy(t, "")), isolatedGit(), sentinelTest); !r.ok {
		t.Fatalf("control: the sentinel test fails on the unchanged copy:\n%s", tail(r.output, 30))
	}
	r := runUnittest(t, releaseChecks(toolsCopy(t, fault)), isolatedGit(), sentinelTest)
	if !caught(t, r, "the sentinel test with an injected write") {
		return false
	}
	if !strings.Contains(r.output, compare) {
		t.Fatalf("the injected write fails another assertion than %q:\n%s", compare, tail(r.output, 30))
	}
	return true
}

// A write the snapshot makes into the sentinel's Git directory while the
// test's GIT_DIR points there. Nothing the snapshot reads changes.
func sentinelWrite(parts string) string {
	return "    if os.environ.get(\"GIT_DIR\"):\n        open(os.path.join(os.environ[\"GIT_DIR\"], " + parts + "), \"a\").close()\n"
}

func TestProtectedAC6_SentinelWriteCaught(t *testing.T) {
	const id = "sentinel-write-not-caught"
	for _, c := range []struct{ what, parts string }{
		{"writes objects/maintenance.lock into the sentinel's Git directory", `"objects", "maintenance.lock"`},
		{"writes another file into the sentinel's Git directory", `"jaekit-probe"`},
	} {
		if !writeCaught(t, sentinelWrite(c.parts), sentinelCompare) {
			t.Errorf("jaekit:violation=%s the sentinel test passes when the snapshot %s", id, c.what)
		}
	}
}

func TestProtectedAC6_OriginalWriteCaught(t *testing.T) {
	const id = "original-write-not-caught"
	fault := "    if os.path.isdir(os.path.join(root, \".git\")):\n        open(os.path.join(root, \".git\", \"jaekit-probe\"), \"a\").close()\n"
	if !writeCaught(t, fault, originalCompare) {
		t.Errorf("jaekit:violation=%s the sentinel test passes when the snapshot writes a file into the checked repository's .git", id)
	}
}
