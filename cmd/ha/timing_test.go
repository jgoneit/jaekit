package main

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/record"
)

func timingLine(timing string) record.Line {
	l := record.Line{Header: record.Header{Seq: 7, Kind: "check"}, Target: &record.Target{Task: "T001", Index: 2},
		Result: "pass", DurationMS: 1500}
	if timing != "" {
		l.Timing = json.RawMessage(timing)
	}
	return l
}

// Lines written before the timing field existed, null values and damaged
// values each hide only the value concerned (run-record.md §1.2).
func TestTimingViewReadsEachValueOnItsOwn(t *testing.T) {
	cases := map[string]struct {
		timing string
		want   map[string]string
	}{
		"line without timing": {"", map[string]string{"limit_ms": timingNotRecorded, "limit_elapsed_ms": timingNotRecorded,
			"limit_reached": timingNotRecorded, "cpu": timingNotRecorded}},
		"timer never started": {`{"limit_ms":900000,"limit_elapsed_ms":null,"limit_reached":false,"cpu":{"status":"unavailable"}}`,
			map[string]string{"limit_elapsed_ms": timingNotMeasured, "limit_use_permille": timingNotMeasured}},
		"timing is not an object": {`[1,2]`, map[string]string{"limit_ms": timingInvalid, "limit_elapsed_ms": timingInvalid,
			"limit_reached": timingInvalid, "cpu": timingInvalid}},
		"quoted and negative numbers": {`{"limit_ms":"900000","limit_elapsed_ms":-1,"limit_reached":"no","cpu":{"status":"measured","user_ms":1,"system_ms":2,"scope":"process-and-waited-descendants","source":"wait4-rusage"}}`,
			map[string]string{"limit_ms": timingInvalid, "limit_elapsed_ms": timingInvalid, "limit_reached": timingInvalid,
				"limit_use_permille": timingInvalid}},
		"free-form scope": {`{"limit_ms":1000,"limit_elapsed_ms":500,"limit_reached":false,"cpu":{"status":"measured","user_ms":1,"system_ms":2,"scope":"/home/user/secret","source":"wait4-rusage"}}`,
			map[string]string{"cpu": timingInvalid}},
		"unavailable cpu with values": {`{"limit_ms":1000,"limit_elapsed_ms":500,"limit_reached":false,"cpu":{"status":"failed","user_ms":0}}`,
			map[string]string{"cpu": timingInvalid}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			r := timingOf(timingLine(c.timing))
			if len(r.Unavailable) != len(c.want) {
				t.Fatalf("unavailable %v, want %v", r.Unavailable, c.want)
			}
			for field, reason := range c.want {
				if r.Unavailable[field] != reason {
					t.Fatalf("unavailable %v, want %v", r.Unavailable, c.want)
				}
			}
			if r.DurationMS != 1500 || r.Target.Task != "T001" || r.Target.Index != 2 {
				t.Fatalf("recorded fields lost: %+v", r)
			}
		})
	}
}

func TestTimingViewShowsLimitUseAndNoFreeFormValues(t *testing.T) {
	r := timingOf(timingLine(`{"limit_ms":900000,"limit_elapsed_ms":612345,"limit_reached":false,"cpu":{"status":"measured","user_ms":1200,"system_ms":300,"scope":"job-process-tree","source":"job-object-accounting"}}`))
	if r.LimitUsePermille == nil || *r.LimitUsePermille != 680 || len(r.Unavailable) != 0 {
		t.Fatalf("limit use: %+v", r)
	}
	md := timingMarkdown(timingReport{Schema: timingSchema, Goal: "docs/specs/example", Runs: []timingRun{r}, TotalDurationMS: r.DurationMS})
	for _, want := range []string{"| 7 | T001 command 2 | check | pass | 1.500 s | 900 s | 612.345 s | 68.0% | no | 1.200 s / 0.300 s (job-process-tree, job-object-accounting) |",
		"Sum of run durations: 1.500 s. Runs can overlap; this is not the goal's elapsed time or budget."} {
		if !strings.Contains(md, want) {
			t.Fatalf("missing %q in\n%s", want, md)
		}
	}
	damaged := timingOf(timingLine(`{"limit_ms":1000,"limit_elapsed_ms":5,"limit_reached":false,"cpu":{"status":"measured","user_ms":1,"system_ms":2,"scope":"/home/user/secret","source":"wait4-rusage"}}`))
	md = timingMarkdown(timingReport{Schema: timingSchema, Goal: "docs/specs/example", Runs: []timingRun{damaged}})
	if strings.Contains(md, "/home/user/secret") || !strings.Contains(md, "unavailable (invalid)") {
		t.Fatalf("a free-form record value reached the view:\n%s", md)
	}
}

// A damaged CPU time hides only that time; the rest of the CPU value stays
// visible (run-record.md §1.2).
func TestTimingViewHidesOnlyTheDamagedCPUTime(t *testing.T) {
	for field, other := range map[string]string{"user_ms": "system_ms", "system_ms": "user_ms"} {
		timing := `{"limit_ms":1000,"limit_elapsed_ms":500,"limit_reached":false,"cpu":{"status":"measured","user_ms":200,"system_ms":300,"scope":"process-and-waited-descendants","source":"wait4-rusage"}}`
		timing = strings.Replace(timing, `"`+field+`":`, `"`+field+`":-`, 1)
		r := timingOf(timingLine(timing))
		if r.CPU == nil || r.CPU.Scope != "process-and-waited-descendants" || r.CPU.Source != "wait4-rusage" {
			t.Fatalf("%s damaged: cpu %+v", field, r.CPU)
		}
		values := map[string]*int64{"user_ms": r.CPU.UserMS, "system_ms": r.CPU.SystemMS}
		if values[field] != nil || values[other] == nil {
			t.Fatalf("%s damaged: user %v, system %v", field, r.CPU.UserMS, r.CPU.SystemMS)
		}
		if len(r.Unavailable) != 1 || r.Unavailable["cpu."+field] != timingInvalid {
			t.Fatalf("%s damaged: unavailable %v", field, r.Unavailable)
		}
		md := timingMarkdown(timingReport{Schema: timingSchema, Goal: "docs/specs/g", Runs: []timingRun{r}})
		if !strings.Contains(md, "0.300 s") && !strings.Contains(md, "0.200 s") || !strings.Contains(md, absent(timingInvalid)) {
			t.Fatalf("%s damaged: Markdown %s", field, md)
		}
	}
}

// Proximity is the exact quotient, and invalid only when it does not fit
// (run-record.md §1.2).
func TestTimingViewLimitUseDoesNotOverflow(t *testing.T) {
	r := timingOf(timingLine(`{"limit_ms":9223372036854775807,"limit_elapsed_ms":9223372036854775806,"limit_reached":false,"cpu":{"status":"unavailable"}}`))
	if r.LimitUsePermille == nil || *r.LimitUsePermille != 999 {
		t.Fatalf("near-maximum values: use %v, unavailable %v", r.LimitUsePermille, r.Unavailable)
	}
	r = timingOf(timingLine(`{"limit_ms":1,"limit_elapsed_ms":9223372036854775807,"limit_reached":true,"cpu":{"status":"unavailable"}}`))
	if r.LimitUsePermille != nil || r.Unavailable["limit_use_permille"] != timingInvalid {
		t.Fatalf("a quotient beyond int64: use %v, unavailable %v", r.LimitUsePermille, r.Unavailable)
	}
}

// The view names the goal by its repository-relative directory, however the
// goal was given (run-record.md §1.2: no absolute paths).
func TestTimingViewNamesTheGoalWithoutAnAbsolutePath(t *testing.T) {
	root := estimateTestFixture(t, []estimateTestRow{{id: "AC-1", kind: "maintain"}}, 50)
	if code, text := estimateTestCLI("start", estimateTestGoal, "--request", "synthetic", "--skill-name", "seal", "--skill-version", "0.1.0"); code != 0 {
		t.Fatalf("start exit %d: %s", code, text)
	}
	for _, format := range []string{"json", "md"} {
		code, text := estimateTestCLI("timing", filepath.Join(root, estimateTestGoal), "--format", format)
		if code != 0 || strings.Contains(text, root) || !strings.Contains(text, estimateTestGoal) {
			t.Fatalf("%s view of an absolute goal path (exit %d): %s", format, code, text)
		}
	}
}
