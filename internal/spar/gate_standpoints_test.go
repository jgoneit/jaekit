package spar

// Checks of the gate's uncovered_standpoints (contracts/review-gate.md §3):
// a standpoint is listed when its reviewer completed none of the given rounds
// with the same packet identity. They reuse the reviewer-standpoints fixtures
// (rsNew, rsFake, rsRun, rsGate): synthetic repositories and /bin/sh
// reviewers, nothing from the checkout's history. A broken fixture fails with
// "setup:"; added helpers have the gs prefix.

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/reviewgate"
)

const (
	gsStored  = "Attack stored records"
	gsReaders = "Attack contract readers"
)

// gsWorld configures claude and codex, both answering for head 1 with a
// standpoint report; claude fails while the fail-claude file exists.
func gsWorld(t *testing.T, failClaude bool) *rsWorld {
	t.Helper()
	w := rsNew(t, &rsChecks{})
	w.rsUse(w.rsFake("claude", "claude", w.rsAnswer("claude", 1, rsReport(true, "checked stored records"), true), ""),
		w.rsFake("codex", "codex", w.rsAnswer("codex", 1, rsReport(true, "checked contract readers"), true), ""))
	if failClaude {
		w.rsWrite(filepath.Join(w.rsDir, "fail-claude"), "intentional")
	}
	return w
}

// gsRun runs one round and fails the test as setup when spar refuses it.
func gsRun(w *rsWorld, out, stage string, base, head int, extra ...string) map[string]any {
	w.rsT.Helper()
	w.rsRun("", out, stage, base, head, extra...)
	return w.rsRaw(out)
}

// gsEntry is one uncovered_standpoints item as the gate file holds it.
func gsEntry(reviewer, text, series string, rounds ...string) any {
	names := []any{}
	for _, r := range rounds {
		names = append(names, map[string]any{"series": series, "round": r})
	}
	return map[string]any{"reviewer": reviewer, "text": text, "rounds": names}
}

func gsStatus(t *testing.T, r map[string]any, name, want string) {
	t.Helper()
	if got := rsState(r, name)["status"]; got != want {
		t.Fatalf("setup: reviewer %s is %v, want %s", name, got, want)
	}
}

// gsRerunOf fails as setup unless b is an uncounted rerun of a's packet.
func gsRerunOf(t *testing.T, a, b map[string]any, first string) {
	t.Helper()
	if b["counted"] != false || !reflect.DeepEqual(b["rerun_of"], []any{first}) || b["packet_sha256"] != a["packet_sha256"] {
		t.Fatalf("setup: not a rerun of %s: counted=%v rerun_of=%v", first, b["counted"], b["rerun_of"])
	}
}

func TestGateCountsAStandpointCompletedInARerunOfThePacket(t *testing.T) {
	w := gsWorld(t, true)
	standpoints := []string{"--standpoint", "claude=" + gsStored, "--standpoint", "codex=" + gsReaders}
	first := w.rsOut("rerun", "01-candidate")
	a := gsRun(w, first, "candidate", 0, 1, standpoints...)
	gsStatus(t, a, "claude", StatusFailed)
	gsStatus(t, a, "codex", StatusCompleted)
	w.rsRemove(filepath.Join(w.rsDir, "fail-claude"))
	again := w.rsOut("rerun", "02-candidate")
	b := gsRun(w, again, "candidate", 0, 1, append([]string{"--reviewers", "claude"}, standpoints...)...)
	gsRerunOf(t, a, b, "01-candidate")
	gsStatus(t, b, "claude", StatusCompleted)
	// Each round keeps its own reading: the first lacks claude, the partial
	// rerun did not run codex.
	if !reflect.DeepEqual(a["standpoints_uncovered"], []any{"claude"}) || !reflect.DeepEqual(b["standpoints_uncovered"], []any{"codex"}) {
		t.Fatalf("setup: per-round coverage is %v and %v", a["standpoints_uncovered"], b["standpoints_uncovered"])
	}
	g := w.rsGate("rerun", 1, first, again)
	got, present := g["uncovered_standpoints"]
	if !present || !reflect.DeepEqual(got, []any{}) {
		t.Errorf("both reviewers completed the packet, but the gate lists uncovered standpoints %v (present %t)", got, present)
	}
	if g["state"] != StateComplete || len(rsList(g["reasons"])) != 0 {
		t.Errorf("gate is %v with reasons %v, want complete", g["state"], g["reasons"])
	}
}

func TestGateListsAStandpointWhoseReviewerCompletedNoRoundOfThePacket(t *testing.T) {
	w := gsWorld(t, true)
	standpoints := []string{"--standpoint", "claude=" + gsStored, "--standpoint", "codex=" + gsReaders}
	first := w.rsOut("failing", "01-candidate")
	a := gsRun(w, first, "candidate", 0, 1, standpoints...)
	again := w.rsOut("failing", "02-candidate")
	b := gsRun(w, again, "candidate", 0, 1, append([]string{"--reviewers", "claude"}, standpoints...)...)
	gsRerunOf(t, a, b, "01-candidate")
	gsStatus(t, b, "claude", StatusFailed)
	want := []any{gsEntry("claude", gsStored, "failing", "01-candidate", "02-candidate")}
	g := w.rsGate("failing", 1, first, again)
	if got := g["uncovered_standpoints"]; !reflect.DeepEqual(got, want) {
		t.Errorf("uncovered_standpoints = %v, want claude's standpoint with both rounds of its packet: %v", got, want)
	}
	if g["state"] != StateDegraded {
		t.Errorf("gate is %v, want degraded for the failed required reviewer", g["state"])
	}
}

// A waiver is the user's decision to go on without a reviewer, not a
// completed review: the standpoint stays listed, as round.json and
// summary.md list it, while the waiver clears the degraded reason.
func TestGateKeepsAWaivedReviewersStandpointUncovered(t *testing.T) {
	w := gsWorld(t, true)
	out := w.rsOut("waived", "01-candidate")
	gsRun(w, out, "candidate", 0, 1, "--standpoint", "claude="+gsStored)
	w.rsMust(0, "waive", out, "claude", "--user-quote", rsQuote)
	g := w.rsGate("waived", 1, out)
	want := []any{gsEntry("claude", gsStored, "waived", "01-candidate")}
	if got := g["uncovered_standpoints"]; !reflect.DeepEqual(got, want) {
		t.Errorf("uncovered_standpoints = %v, want the waived reviewer's standpoint: %v", got, want)
	}
	rounds := rsList(g["rounds"])
	if len(rounds) != 1 || len(rsList(rsMap(rounds[0])["waived_reviewers"])) != 1 || g["state"] != StateComplete {
		t.Errorf("setup: the waiver did not apply: state %v, rounds %v", g["state"], rounds)
	}
}

func TestGateDoesNotLetAnotherPacketCoverAStandpoint(t *testing.T) {
	w := gsWorld(t, true)
	first := w.rsOut("other", "01-candidate")
	a := gsRun(w, first, "candidate", 0, 1, "--standpoint", "claude="+gsStored)
	gsStatus(t, a, "claude", StatusFailed)
	w.rsRemove(filepath.Join(w.rsDir, "fail-claude"))
	// Changed text on the same commits is another packet.
	changed := w.rsOut("other", "02-candidate")
	b := gsRun(w, changed, "candidate", 0, 1, "--standpoint", "claude="+gsStored+" differently")
	// The same text in a later stage and range is another packet too.
	w.rsAnswer("claude", 2, rsReport(true, "checked stored records"), true)
	w.rsAnswer("codex", 2, nil, false)
	fixes := w.rsOut("other", "03-fixes")
	c := gsRun(w, fixes, "fixes", 1, 2, "--standpoint", "claude="+gsStored)
	for _, r := range []map[string]any{b, c} {
		gsStatus(t, r, "claude", StatusCompleted)
		if r["counted"] != true || r["packet_sha256"] == a["packet_sha256"] {
			t.Fatalf("setup: %v is not a new packet", r["stage"])
		}
	}
	g := w.rsGate("other", 2, first, changed, fixes)
	want := []any{gsEntry("claude", gsStored, "other", "01-candidate")}
	if got := g["uncovered_standpoints"]; !reflect.DeepEqual(got, want) {
		t.Errorf("uncovered_standpoints = %v, want only the first packet's standpoint: %v", got, want)
	}
}

var gsWrittenAt = regexp.MustCompile(`"written_at": "[^"]*"`)

func TestGateWithoutStandpointsHasNoUncoveredStandpointsField(t *testing.T) {
	w := gsWorld(t, false)
	plain := w.rsOut("plain", "01-candidate")
	gsRun(w, plain, "candidate", 0, 1)
	g := w.rsGate("plain", 1, plain)
	keys := []string{}
	for k := range g {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if !reflect.DeepEqual(keys, rwGateKeys) {
		t.Errorf("a gate without standpoints has fields %v, want the earlier fields %v", keys, rwGateKeys)
	}
	// The field is the only difference standpoints make: removing them from
	// round.json gives the same bytes without its line.
	out := w.rsOut("covered", "01-candidate")
	gsRun(w, out, "candidate", 0, 1, "--standpoint", "claude="+gsStored, "--standpoint", "codex="+gsReaders)
	w.rsGate("with", 1, out)
	with := w.rsRead(w.rsOut("gates", "with.json"))
	w.rsStrip(out)
	w.rsGate("without", 1, out)
	without := w.rsRead(w.rsOut("gates", "without.json"))
	const line = "  \"uncovered_standpoints\": [],\n"
	if strings.Count(with, line) != 1 {
		t.Fatalf("setup: a gate with covered standpoints lacks %q:\n%s", line, with)
	}
	if strings.Contains(without, "uncovered_standpoints") {
		t.Errorf("a gate over a round without standpoints carries uncovered_standpoints:\n%s", without)
	}
	got := gsWrittenAt.ReplaceAllString(strings.Replace(with, line, "", 1), `"written_at": ""`)
	if want := gsWrittenAt.ReplaceAllString(without, `"written_at": ""`); got != want {
		t.Errorf("standpoints changed the gate beyond uncovered_standpoints:\nwith (field removed):\n%s\nwithout:\n%s", got, want)
	}
}

// Core (internal/reviewgate) allows unknown fields: a gate that lists an
// uncovered standpoint reads as the same gate without standpoints, apart
// from the digest of its own bytes.
func TestCoreReadsAGateWithUncoveredStandpointsAsWithout(t *testing.T) {
	w := gsWorld(t, true)
	out := w.rsOut("core", "01-candidate")
	gsRun(w, out, "candidate", 0, 1, "--standpoint", "claude="+gsStored)
	if g := w.rsGate("with", 1, out); len(rsList(g["uncovered_standpoints"])) != 1 {
		t.Fatalf("setup: the gate lists %v", g["uncovered_standpoints"])
	}
	w.rsStrip(out)
	w.rsGate("without", 1, out)
	withPath, withoutPath := w.rsOut("gates", "with.json"), w.rsOut("gates", "without.json")
	with, err := reviewgate.Read(withPath)
	if err != nil {
		t.Fatalf("Core refused a gate with uncovered_standpoints: %v", err)
	}
	without, err := reviewgate.Read(withoutPath)
	if err != nil {
		t.Fatalf("setup: Core refused the gate without standpoints: %v", err)
	}
	data, err := os.ReadFile(withPath)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	sum := sha256.Sum256(data)
	if with.GateDigest != hex.EncodeToString(sum[:]) {
		t.Errorf("Core's gate digest %s is not the sha256 of the file it read", with.GateDigest)
	}
	with.GateDigest, without.GateDigest = "", ""
	if !reflect.DeepEqual(with, without) {
		t.Errorf("Core reads the gate with uncovered_standpoints differently:\n%+v\n%+v", with, without)
	}
}
