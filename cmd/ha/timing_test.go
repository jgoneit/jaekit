package main

import (
	"encoding/json"
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
