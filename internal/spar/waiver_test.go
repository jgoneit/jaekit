package spar

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A dry run and a reviewer that already holds a waiver are refused, and
// nothing is added to waivers.jsonl.
func TestWaiveRefusesDryRunsAndRepeats(t *testing.T) {
	w := newRWWorld(t, true)
	w.use(rwNative("host"), w.missingAgy())
	dry := filepath.Join(w.newSeries(), "01-dry")
	w.run(dry, "--dry-run")
	if code, msg := w.spar("waive", dry, "agy", "--user-quote", rwQuote); code != ExitRefused || !strings.Contains(msg, "dry run") {
		t.Errorf("a dry run: %d %s", code, msg)
	}
	if _, err := os.Stat(filepath.Join(dry, waiversFile)); err == nil {
		t.Errorf("the refused waiver wrote %s", waiversFile)
	}
	out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
	w.record(out, "host")
	if code, msg := w.spar("waive", out, "agy", "--user-quote", rwQuote); code != ExitOK {
		t.Fatalf("the first waiver: %d %s", code, msg)
	}
	if code, msg := w.spar("waive", out, "agy", "--user-quote", "other words"); code != ExitRefused || !strings.Contains(msg, "already") {
		t.Errorf("a second waiver: %d %s", code, msg)
	}
	waivers, err := loadWaivers(out)
	if err != nil || len(waivers) != 1 || waivers[0].UserQuote != rwQuote {
		t.Errorf("waivers after a refused repeat: %v %+v", err, waivers)
	}
	if code, msg := w.spar("waive", out); code != ExitUsage {
		t.Errorf("waive without a reviewer: %d %s", code, msg)
	}
}

// A waivers.jsonl that cannot be read, or holds a line without a reviewer,
// a preset or the user's words, blocks the gate and refuses a new waiver.
func TestUnreadableWaiversBlock(t *testing.T) {
	w := newRWWorld(t, true)
	w.use(rwNative("host"), w.missingAgy())
	for label, body := range map[string]string{
		"not JSON":       "{\n",
		"no user words":  `{"round": "01-candidate", "reviewer": "agy", "preset": "agy", "status": "unavailable", "user_quote": " ", "recorded_at": "2026-01-01T00:00:00Z"}` + "\n",
		"no reviewer":    `{"round": "01-candidate", "preset": "agy", "status": "unavailable", "user_quote": "go on", "recorded_at": "2026-01-01T00:00:00Z"}` + "\n",
		"not a file":     "",
		"not an object":  "[]\n",
		"no preset name": `{"round": "01-candidate", "reviewer": "agy", "status": "unavailable", "user_quote": "go on", "recorded_at": "2026-01-01T00:00:00Z"}` + "\n",
	} {
		out := w.run(filepath.Join(w.newSeries(), "01-candidate"))
		w.record(out, "host")
		file := filepath.Join(out, waiversFile)
		if label == "not a file" {
			if err := os.MkdirAll(file, 0o755); err != nil {
				t.Fatal(err)
			}
		} else {
			w.write(file, body)
		}
		gate := filepath.Join(filepath.Dir(out), "gate.json")
		code, msg := w.spar("gate", out, "--expect-head", w.head, "--out", gate, "--repo", w.repo)
		var g GateRecord
		if data, err := os.ReadFile(gate); err != nil || json.Unmarshal(data, &g) != nil {
			t.Fatalf("%s: no gate file: %d %s", label, code, msg)
		}
		blocked := false
		for _, reason := range g.Reasons {
			blocked = blocked || strings.Contains(reason, waiversFile)
		}
		if code != ExitBlocked || g.State != StateBlocked || !blocked {
			t.Errorf("%s: the gate is %s (exit %d): %v", label, g.State, code, g.Reasons)
		}
		if code, msg := w.spar("waive", out, "agy", "--user-quote", rwQuote); code != ExitRefused || !strings.Contains(msg, waiversFile) {
			t.Errorf("%s: waive %d %s", label, code, msg)
		}
	}
}

// waivers.jsonl is a round file, so no reviewer may take its name (G20).
func TestWaiversFileNameIsReserved(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	config := writeFile(t, filepath.Join(dir, "spar.toml"), "[[reviewers]]\nname = \"Waivers.jsonl\"\npreset = \"native\"\n")
	if _, err := LoadConfig(repo, env(map[string]string{"SPAR_CONFIG": config, "HOME": dir}), nil); err == nil {
		t.Errorf("a reviewer named after waivers.jsonl was accepted")
	}
}
