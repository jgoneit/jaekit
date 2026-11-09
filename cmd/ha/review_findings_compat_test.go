//go:build jaekit_compat_018

package main

// AC-8 of the goal review-findings-report: a run record holding a review
// note with the finding lists reads under the released ha 0.1.8 as the same
// record without them, and 0.1.8 appends to it with the chain whole. The
// check runs an `ha` on PATH whose `--version` is `ha 0.1.8`; it only builds
// with the jaekit_compat_018 tag, so the ordinary test run does not need
// that binary. It checks kept behavior: no such binary, or a setup that
// breaks, fails it.

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// rfrRelease018 is the first ha on PATH that reports version 0.1.8.
func rfrRelease018(t *testing.T) string {
	t.Helper()
	for _, dir := range filepath.SplitList(os.Getenv("PATH")) {
		p := filepath.Join(dir, "ha")
		if info, err := os.Stat(p); err != nil || info.IsDir() || info.Mode().Perm()&0o111 == 0 {
			continue
		}
		if out, err := exec.Command(p, "--version").Output(); err == nil && strings.TrimSpace(string(out)) == "ha 0.1.8" {
			return p
		}
	}
	t.Fatalf("setup: no ha on PATH reports version 0.1.8")
	return ""
}

// rfrOld runs ha 0.1.8 in the fixture's repository.
func rfrOld(t *testing.T, ha string, f *rfrFixture, args ...string) (int, string) {
	t.Helper()
	cmd := exec.Command(ha, args...)
	cmd.Dir = f.dir
	out, err := cmd.CombinedOutput()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return 0, string(out)
	case errors.As(err, &exit):
		return exit.ExitCode(), string(out)
	}
	t.Fatalf("setup: ha 0.1.8 %v: %v", args, err)
	return 0, ""
}

// rfrOldJudgment is the status ha 0.1.8 computes.
func rfrOldJudgment(t *testing.T, ha string, f *rfrFixture) (int, string, []string) {
	t.Helper()
	code, out := rfrOld(t, ha, f, "status", rgGoal, "--format", "json")
	var value map[string]any
	if err := json.Unmarshal([]byte(out), &value); err != nil {
		t.Fatalf("setup: ha 0.1.8 status exit %d gave no status JSON: %v: %s", code, err, out)
	}
	status, _ := value["status"].(string)
	var reasons []string
	list, _ := value["reasons"].([]any)
	for _, item := range list {
		r, _ := item.(map[string]any)
		reasons = append(reasons, fmt.Sprintf("%v/%v/%v", r["criterion"], r["code"], r["class"]))
	}
	return code, status, reasons
}

func TestReviewFindingsReportAC8_Release018ReadsTheNote(t *testing.T) {
	old := rfrRelease018(t)
	deferred, unhandled := rfrLists()
	for _, tc := range []struct {
		name string
		gate func(f *rfrFixture) rgGate
	}{
		{"complete", func(f *rfrFixture) rgGate { return f.chained() }},
		{"blocked", func(f *rfrFixture) rgGate {
			g := f.chained()
			g.state, g.defect = "blocked", 1
			return g
		}},
	} {
		with := rfrKeep(t, "standard")
		with.note(withLists(tc.gate(with).gateValue(), deferred, unhandled, 2))
		without := rfrKeep(t, "standard")
		without.note(tc.gate(without).gateValue())

		wc, ws, wr := rfrOldJudgment(t, old, with)
		oc, os_, or := rfrOldJudgment(t, old, without)
		if wc != oc || ws != os_ || !reflect.DeepEqual(wr, or) {
			t.Errorf("%s: ha 0.1.8 reads the record with the lists as exit %d %s %v, without them as exit %d %s %v",
				tc.name, wc, ws, wr, oc, os_, or)
		}
		for _, r := range wr {
			if strings.Contains(r, "record_invalid") {
				t.Errorf("%s: ha 0.1.8 reads the record with the lists as invalid: %v", tc.name, wr)
			}
		}

		// ha 0.1.8 appends a line; both versions then read a whole chain.
		dw, outW := rfrOld(t, old, with, "done", rgGoal)
		do, _ := rfrOld(t, old, without, "done", rgGoal)
		if dw != do {
			t.Errorf("%s: ha 0.1.8 done exits %d with the lists and %d without them: %s", tc.name, dw, do, outW)
		}
		for _, who := range []string{"ha 0.1.8", "this ha"} {
			var reasons []string
			if who == "ha 0.1.8" {
				_, _, reasons = rfrOldJudgment(t, old, with)
			} else {
				reasons = with.judgment(false).reasons
			}
			for _, r := range reasons {
				if strings.Contains(r, "record_invalid") {
					t.Errorf("%s: after ha 0.1.8 appended, %s reads the record as invalid: %v", tc.name, who, reasons)
				}
			}
		}
	}
}
