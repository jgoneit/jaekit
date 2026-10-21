package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/run"
	"github.com/jgoneit/jaekit/internal/status"
)

// A start failure of ha's own process control is not the tool's result:
// doctor ends with 70 and prints no line for that tool (run-record.md §1.4).
func TestDoctorProcessControlFailure(t *testing.T) {
	doctorFixture(t, "- 도구: `synthetic-tool`\n")
	var out, stderr bytes.Buffer
	c := &cli{out: &out, err: &stderr}
	c.execute = func(dir string, argv []string, timeout time.Duration, log string, env []string) (run.Result, error) {
		return run.Result{Attempted: true, Outcome: run.Error, Reason: "process_start",
			StartError: fmt.Errorf("CreateJobObjectW: %w", run.ErrProcessControl)}, nil
	}
	if code := c.doctor([]string{scenarioGoal}); code != exitInternal || strings.Contains(out.String(), doctorUnavailable) {
		t.Fatalf("ha doctor: exit %d: %s%s", code, out.String(), stderr.String())
	}
}

func TestReportSummaryParts(t *testing.T) {
	if got := oneLine.Replace("docs/specs/two\nlines\r"); got != `docs/specs/two\nlines\r` {
		t.Fatalf("oneLine: %q", got)
	}
	reasons := []status.Reason{
		{Code: "criterion_missing", Criterion: "AC-1"},
		{Code: "out_of_scope", Paths: []string{"a.txt", "b/c.txt"}},
		{Code: "spec_changed"},
	}
	if got := reasonList(reasons); got != "AC-1 `criterion_missing`, `out_of_scope` (a.txt, b/c.txt), `spec_changed`" {
		t.Fatalf("reasonList: %s", got)
	}
}
