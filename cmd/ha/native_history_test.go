package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/history"
)

// Use a new OS process for each public command. The existing native driver
// delegates directly to runMain and propagates the actual exit code.
func nativeHistoryCommand(t *testing.T, dir string, want int, args ...string) string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	argv := append([]string{"-test.run=^TestProcessScenarioDriver$", "--", "native"}, args...)
	cmd := exec.CommandContext(ctx, os.Args[0], argv...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			t.Fatalf("ha %v did not finish: %v: %s", args, err, out)
		}
		code = exit.ExitCode()
	}
	if ctx.Err() != nil || code != want {
		t.Fatalf("ha %v: exit=%d want=%d context=%v: %s", args, code, want, ctx.Err(), out)
	}
	return string(out)
}

func newNativeHistoryFixture(t *testing.T, linked bool) *preflightFixture {
	t.Helper()
	base, err := filepath.Abs(filepath.Join(scenarioRoot, "_base"))
	if err != nil {
		t.Fatal(err)
	}
	f := &preflightFixture{t: t, dir: filepath.Join(t.TempDir(), "판단 history")}
	copyTree(t, base, f.dir)
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	for _, key := range []string{"GIT_AUTHOR_NAME", "GIT_COMMITTER_NAME"} {
		t.Setenv(key, "synthetic")
	}
	for _, key := range []string{"GIT_AUTHOR_EMAIL", "GIT_COMMITTER_EMAIL"} {
		t.Setenv(key, "synthetic@example.invalid")
	}
	f.put(".gitattributes", "* -text\n")
	f.put("근거 space/내용.txt", "registered evidence\n")
	f.git("init", "-q", "-b", "main")
	f.git("add", "-A")
	f.git("commit", "-qm", "synthetic history base")
	if linked {
		dir := filepath.Join(t.TempDir(), "연결 worktree")
		f.git("worktree", "add", "--detach", "-q", dir, "HEAD")
		f.dir = dir
	}
	nativeHistoryCommand(t, f.dir, 0, "start", scenarioGoal, "--request", "synthetic native history")
	return f
}

func nativeHistoryView(t *testing.T, f *preflightFixture, args ...string) history.View {
	t.Helper()
	argv := append([]string{"history", scenarioGoal}, args...)
	argv = append(argv, "--format", "json")
	out := nativeHistoryCommand(t, f.dir, 0, argv...)
	var v history.View
	if err := json.Unmarshal([]byte(out), &v); err != nil {
		t.Fatalf("history view: %v: %s", err, out)
	}
	return v
}

func nativeHistoryStore(t *testing.T, f *preflightFixture) string {
	t.Helper()
	line := bytes.Split(f.read(scenarioGoal+"/runs.jsonl"), []byte("\n"))[0]
	h := sha256.Sum256(line)
	common := f.git("rev-parse", "--path-format=absolute", "--git-common-dir")
	return filepath.Join(common, "ha", "history", hex.EncodeToString(h[:]))
}

func TestNativeHistoryGitLifecycle(t *testing.T) {
	for _, linked := range []bool{false, true} {
		t.Run(fmt.Sprintf("linked=%v", linked), func(t *testing.T) {
			f := newNativeHistoryFixture(t, linked)
			runs := f.read(scenarioGoal + "/runs.jsonl")
			store := nativeHistoryStore(t, f)
			if v := nativeHistoryView(t, f); v.History != "absent" || len(v.Items) != 0 {
				t.Fatalf("absent history: %+v", v)
			}
			if _, err := os.Stat(store); !os.IsNotExist(err) {
				t.Fatalf("read created absent history: %v", err)
			}
			add := []string{"history", scenarioGoal, "add", "--kind", "관측", "--summary", "native original", "--target", "AC-1", "--checked-ref", "file:근거 space/내용.txt", "--request-id", "native-original"}
			first := nativeHistoryCommand(t, f.dir, 0, add...)
			var receipt map[string]any
			if err := json.Unmarshal([]byte(first), &receipt); err != nil || receipt["id"] != "D1" || receipt["replayed"] != false {
				t.Fatalf("first receipt: %v %s", err, first)
			}
			before := nativeHistoryView(t, f, "show", "D1")
			if before.Event == nil || len(before.Event.Evidence) != 1 || before.Event.Evidence[0].Recorded.State != "read" || before.Event.Evidence[0].Now != "unchanged" {
				t.Fatalf("registered file observation: %+v", before.Event)
			}
			if other := nativeHistoryView(t, f, "show", "--request-id", "native-original"); !reflect.DeepEqual(before.Event, other.Event) {
				t.Fatal("event and request detail differ")
			}
			events, err := os.ReadFile(filepath.Join(store, "events.jsonl"))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(f.dir, "근거 space", "내용.txt")); err != nil {
				t.Fatal(err)
			}
			replayed := nativeHistoryCommand(t, f.dir, 0, add...)
			var replay map[string]any
			if err := json.Unmarshal([]byte(replayed), &replay); err != nil || replay["id"] != receipt["id"] || replay["sha256"] != receipt["sha256"] || replay["replayed"] != true {
				t.Fatalf("replay changed receipt: %v %s", err, replayed)
			}
			if after := nativeHistoryView(t, f, "show", "D1"); !reflect.DeepEqual(before.Event.Evidence[0].Recorded, after.Event.Evidence[0].Recorded) || after.Event.Evidence[0].Now != "unavailable" {
				t.Fatalf("detail changed original observation: %+v", after.Event)
			}
			for _, mode := range []string{"current", "list"} {
				if v := nativeHistoryView(t, f, mode); len(v.Items) != 1 || v.Items[0].ID != "D1" {
					t.Fatalf("%s lost event: %+v", mode, v)
				}
			}
			if after, err := os.ReadFile(filepath.Join(store, "events.jsonl")); err != nil || !bytes.Equal(events, after) {
				t.Fatal("replay/query rewrote stored evidence")
			}
			nativeHistoryCommand(t, f.dir, 0, "history", scenarioGoal, "add", "--kind", "결정", "--summary", "native correction", "--supersedes", "D1", "--request-id", "native-correction")
			nativeHistoryCommand(t, f.dir, 0, "history", scenarioGoal, "add", "--kind", "결정", "--summary", "uses corrected evidence", "--builds-on", "D1", "--request-id", "native-dependent")
			if out := nativeHistoryCommand(t, f.dir, 65, "history", scenarioGoal, "add", "--kind", "결정", "--summary", "stale correction", "--supersedes", "D1", "--request-id", "native-conflict"); !strings.Contains(out, "correction_conflict") {
				t.Fatalf("correction conflict: %s", out)
			}
			v := nativeHistoryView(t, f)
			if len(v.Items) != 2 || v.Items[0].ID == "D1" || v.Items[1].ID == "D1" {
				t.Fatalf("current retained superseded event: %+v", v)
			}
			detail := nativeHistoryView(t, f, "show", "D3")
			if len(detail.Event.GroundsCorrected) != 1 || !reflect.DeepEqual(detail.Event.GroundsCorrected[0].By, []string{"D2"}) {
				t.Fatalf("correction before dependent lost: %+v", detail.Event)
			}
			md := nativeHistoryCommand(t, f.dir, 0, "history", scenarioGoal, "show", "D3")
			if !strings.Contains(md, "corrected") || strings.Contains(md, "later corrected") {
				t.Fatalf("Markdown correction meaning: %s", md)
			}
			if filtered := nativeHistoryView(t, f, "current", "--target", "AC-1"); len(filtered.Items) != 0 {
				t.Fatal("filter resurrected the judgment corrected outside the filter")
			}
			if filtered := nativeHistoryView(t, f, "list", "--keyword", "native original"); len(filtered.Items) != 1 || filtered.Items[0].ID != "D1" {
				t.Fatalf("keyword filter: %+v", filtered)
			}
			page := nativeHistoryView(t, f, "list", "--limit", "1")
			seen := map[string]bool{}
			for {
				for _, item := range page.Items {
					if seen[item.ID] {
						t.Fatal("page duplicated an event")
					}
					seen[item.ID] = true
				}
				if page.Next == nil {
					break
				}
				page = nativeHistoryView(t, f, "list", "--limit", "1", "--page", *page.Next)
			}
			if len(seen) != 3 {
				t.Fatalf("page lost events: %v", seen)
			}
			for _, failure := range []struct {
				code   int
				marker string
				args   []string
			}{
				{64, "invalid_input", []string{"list", "--limit", "0"}},
				{65, "cursor_invalid", []string{"list", "--page", "broken"}},
				{65, "unknown_event", []string{"show", "D99"}},
				{65, "invalid_relation", []string{"add", "--kind", "관측", "--summary", "bad relation", "--builds-on", "D99", "--request-id", "native-bad"}},
				{65, "request_conflict", []string{"add", "--kind", "관측", "--summary", "different content", "--request-id", "native-original"}},
			} {
				args := append([]string{"history", scenarioGoal}, failure.args...)
				if out := nativeHistoryCommand(t, f.dir, failure.code, args...); !strings.Contains(out, failure.marker) {
					t.Fatalf("public error lost %s: %s", failure.marker, out)
				}
			}
			if !bytes.Equal(runs, f.read(scenarioGoal+"/runs.jsonl")) {
				t.Fatal("history changed execution records")
			}

			// A second worktree shares the start and store, then its run chain
			// diverges. The new judgment is visible but never a current judgment
			// in the first worktree's execution lineage.
			dir := filepath.Join(t.TempDir(), "다른 worktree")
			f.git("worktree", "add", "--detach", "-q", dir, "HEAD")
			other := &preflightFixture{t: t, dir: dir}
			other.put(scenarioGoal+"/runs.jsonl", string(runs))
			if nativeHistoryStore(t, other) != store || len(nativeHistoryView(t, other, "list").Items) != 3 {
				t.Fatal("linked worktree did not share history")
			}
			nativeHistoryCommand(t, f.dir, 0, "note", scenarioGoal, "input", "--quote", "first lineage")
			nativeHistoryCommand(t, other.dir, 0, "note", scenarioGoal, "input", "--quote", "second lineage")
			nativeHistoryCommand(t, other.dir, 0, "history", scenarioGoal, "add", "--kind", "관측", "--summary", "other lineage", "--request-id", "native-other")
			v = nativeHistoryView(t, f)
			if v.OtherLineage == nil || v.OtherLineage.Count != 1 || v.OtherLineage.IDs[0].Lineage != history.LineageDifferent || len(v.Items) != 2 {
				t.Fatalf("different run lineage merged: %+v", v)
			}
			// Exercise the public error classification over real disk bytes.
			path := filepath.Join(store, "events.jsonl")
			original, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			for _, failure := range []struct {
				code   int
				marker string
				bytes  []byte
			}{
				{66, "history_unsupported", bytes.Replace(original, []byte(`"schema":"decision-history/v1"`), []byte(`"schema":"decision-history/future"`), 1)},
				{65, "history_corrupt", append(append([]byte{}, original...), []byte(`{"partial":`)...)},
			} {
				if err := os.WriteFile(path, failure.bytes, 0o600); err != nil {
					t.Fatal(err)
				}
				if out := nativeHistoryCommand(t, f.dir, failure.code, "history", scenarioGoal, "list"); !strings.Contains(out, failure.marker) {
					t.Fatalf("disk error lost %s: %s", failure.marker, out)
				}
			}
			if err := os.WriteFile(path, original, 0o600); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestNativeHistoryFileEvidence(t *testing.T) {
	f := newNativeHistoryFixture(t, false)
	f.put("empty", "")
	if err := os.Truncate(filepath.Join(f.dir, "empty"), fileEvidenceLimit+1); err != nil {
		t.Fatal(err)
	}
	nativeHistoryCommand(t, f.dir, 0, "history", scenarioGoal, "add", "--kind", "관측", "--summary", "bounded evidence states", "--request-id", "native-states",
		"--ref", "file:근거 space/내용.txt", "--ref", "file:missing", "--ref", "file:src", "--ref", "file:empty")
	v := nativeHistoryView(t, f, "show", "D1")
	want := []string{"read", "missing", "not_regular", "too_large"}
	if v.Event == nil || len(v.Event.Evidence) != len(want) {
		t.Fatalf("evidence states: %+v", v.Event)
	}
	for i, state := range want {
		if e := v.Event.Evidence[i]; e.Recorded.State != state || (state != "read" && e.Recorded.Version != "") {
			t.Fatalf("evidence %d: %+v, want %s", i, e, state)
		}
	}
	// Shared reader checks include the exact 64 MiB boundary and growth both
	// after the identity check and while the bounded reader is consuming bytes.
	TestFileEvidenceBoundsGrowth(t)
	for _, kind := range []string{"file", "directory", "removed"} {
		t.Run("replace-"+kind, func(t *testing.T) {
			root := t.TempDir()
			name := filepath.Join(root, "evidence")
			evidenceWrite(t, name, "original")
			version, state := observeFileEvidence(root, "evidence", func() {
				if err := os.Rename(name, name+".original"); err != nil {
					t.Fatal(err)
				}
				if kind == "file" {
					evidenceWrite(t, name, "other")
				} else if kind == "directory" {
					if err := os.Mkdir(name, 0o755); err != nil {
						t.Fatal(err)
					}
				}
			})
			want := "unreadable"
			if kind == "removed" {
				want = "missing"
			}
			if version != "" || state != want {
				t.Fatalf("replaced evidence accepted: %q/%s", version, state)
			}
		})
	}
}
