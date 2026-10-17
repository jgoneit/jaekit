package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"math/bits"
	"strings"

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/run"
)

// The time view reads a goal's record and prints each run's process
// measurement (run-record.md §1.2). It runs nothing and writes nothing: no
// record, log, lock or working tree file changes.

const timingSchema = "timing/v1"

// Why a measured value is absent from the view.
const (
	timingNotRecorded = "not_recorded" // the line has no such value, as lines written before it existed
	timingNotMeasured = "not_measured" // recorded as null: the limit timer never started
	timingInvalid     = "invalid"      // present but of the wrong type or value
)

// Fixed codes the record may name; anything else is shown as invalid, so the
// view never prints a free-form value from the record.
var (
	cpuScopes  = map[string]bool{"process-and-waited-descendants": true, "job-process-tree": true}
	cpuSources = map[string]bool{"wait4-rusage": true, "job-object-accounting": true}
)

type timingCPU struct {
	Status   string `json:"status"`
	UserMS   *int64 `json:"user_ms,omitempty"`
	SystemMS *int64 `json:"system_ms,omitempty"`
	Scope    string `json:"scope,omitempty"`
	Source   string `json:"source,omitempty"`
}

type timingRun struct {
	Seq              int               `json:"seq"`
	Target           record.Target     `json:"target"`
	Kind             string            `json:"kind"`
	Result           string            `json:"result"`
	DurationMS       int64             `json:"duration_ms"`
	LimitMS          *int64            `json:"limit_ms"`
	LimitElapsedMS   *int64            `json:"limit_elapsed_ms"`
	LimitUsePermille *int64            `json:"limit_use_permille"`
	LimitReached     *bool             `json:"limit_reached"`
	CPU              *timingCPU        `json:"cpu"`
	Unavailable      map[string]string `json:"unavailable"`
}

type timingReport struct {
	Schema          string      `json:"schema"`
	Goal            string      `json:"goal"`
	Runs            []timingRun `json:"runs"`
	TotalDurationMS int64       `json:"total_duration_ms"`
	TotalMeaning    string      `json:"total_meaning"`
}

const totalMeaning = "sum of the listed runs' duration_ms; runs can overlap, and this is not the goal's elapsed time or budget"

func (c *cli) timing(in []string) int {
	a, err := parseArgs(in, []string{"format"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	format, hasFormat := a.vals["format"]
	if len(a.pos) != 1 || (hasFormat && format != "json" && format != "md") {
		return c.fail(exitUsage, "usage: ha timing <goal> [--format md|json]")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "read record: %v", err)
	}
	if bad != nil {
		return c.fail(exitRefused, "%v; the time view reads only an intact record", bad)
	}
	// The repository-relative goal directory, never the argument as given (no absolute paths).
	rep := timingReport{Schema: timingSchema, Goal: w.goal.Dir, Runs: []timingRun{}, TotalMeaning: totalMeaning}
	for _, l := range lines {
		if l.Kind != "check" && l.Kind != "baseline" {
			continue
		}
		r := timingOf(l)
		rep.TotalDurationMS += r.DurationMS
		rep.Runs = append(rep.Runs, r)
	}
	if format == "json" {
		b, err := json.Marshal(rep)
		if err != nil {
			return c.fail(exitInternal, "write time view: %v", err)
		}
		_, _ = c.out.Write(append(b, '\n'))
		return exitOK
	}
	_, _ = io.WriteString(c.out, timingMarkdown(rep))
	return exitOK
}

// timingOf reads one run's measurement. Each value is checked on its own, so
// a damaged value hides only itself.
func timingOf(l record.Line) timingRun {
	r := timingRun{Seq: l.Seq, Kind: l.Kind, Result: l.Result, DurationMS: l.DurationMS, Unavailable: map[string]string{}}
	if l.Target != nil {
		r.Target = *l.Target
	}
	raw := bytes.TrimSpace(l.Timing)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		for _, name := range []string{"limit_ms", "limit_elapsed_ms", "limit_use_permille", "limit_reached", "cpu"} {
			r.Unavailable[name] = timingNotRecorded
		}
		return r
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		for _, name := range []string{"limit_ms", "limit_elapsed_ms", "limit_use_permille", "limit_reached", "cpu"} {
			r.Unavailable[name] = timingInvalid
		}
		return r
	}
	if v, ok := fields["limit_ms"]; !ok {
		r.Unavailable["limit_ms"] = timingNotRecorded
	} else if n, ok := wholeMS(v); ok && n > 0 {
		r.LimitMS = &n
	} else {
		r.Unavailable["limit_ms"] = timingInvalid
	}
	if v, ok := fields["limit_elapsed_ms"]; !ok {
		r.Unavailable["limit_elapsed_ms"] = timingNotRecorded
	} else if bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
		r.Unavailable["limit_elapsed_ms"] = timingNotMeasured
	} else if n, ok := wholeMS(v); ok {
		r.LimitElapsedMS = &n
	} else {
		r.Unavailable["limit_elapsed_ms"] = timingInvalid
	}
	if v, ok := fields["limit_reached"]; !ok {
		r.Unavailable["limit_reached"] = timingNotRecorded
	} else {
		var reached bool
		if json.Unmarshal(v, &reached) == nil && !bytes.Equal(bytes.TrimSpace(v), []byte("null")) {
			r.LimitReached = &reached
		} else {
			r.Unavailable["limit_reached"] = timingInvalid
		}
	}
	if r.LimitMS != nil && r.LimitElapsedMS != nil {
		if use, ok := permille(*r.LimitElapsedMS, *r.LimitMS); ok {
			r.LimitUsePermille = &use
		} else {
			r.Unavailable["limit_use_permille"] = timingInvalid
		}
	} else {
		r.Unavailable["limit_use_permille"] = timingNotRecorded
		if r.Unavailable["limit_ms"] == timingInvalid || r.Unavailable["limit_elapsed_ms"] == timingInvalid {
			r.Unavailable["limit_use_permille"] = timingInvalid
		} else if r.Unavailable["limit_elapsed_ms"] == timingNotMeasured {
			r.Unavailable["limit_use_permille"] = timingNotMeasured
		}
	}
	if v, ok := fields["cpu"]; !ok {
		r.Unavailable["cpu"] = timingNotRecorded
	} else if cpu, damaged, ok := cpuOf(v); ok {
		r.CPU = cpu
		for _, name := range damaged {
			r.Unavailable["cpu."+name] = timingInvalid
		}
	} else {
		r.Unavailable["cpu"] = timingInvalid
	}
	return r
}

// wholeMS reads a non-negative whole number of milliseconds written as a JSON
// number. json.Number would also take a quoted number, which is not one.
func wholeMS(v json.RawMessage) (int64, bool) {
	v = bytes.TrimSpace(v)
	if len(v) == 0 || v[0] < '0' || v[0] > '9' {
		return 0, false
	}
	dec := json.NewDecoder(bytes.NewReader(v))
	dec.UseNumber()
	var n json.Number
	if err := dec.Decode(&n); err != nil {
		return 0, false
	}
	ms, err := n.Int64()
	return ms, err == nil && ms >= 0
}

// permille is elapsed*1000/limit, exact in 128 bits; false when it does not
// fit a non-negative int64 or the limit is zero.
func permille(elapsed, limit int64) (int64, bool) {
	if elapsed < 0 || limit <= 0 {
		return 0, false
	}
	hi, lo := bits.Mul64(uint64(elapsed), 1000)
	if hi >= uint64(limit) {
		return 0, false
	}
	q, _ := bits.Div64(hi, lo, uint64(limit))
	if q > math.MaxInt64 {
		return 0, false
	}
	return int64(q), true
}

// cpuOf reads a CPU value. A measured value whose user or system time alone is
// damaged keeps the rest, and names the damaged times (run-record.md §1.2);
// any other damage makes the whole value unreadable.
func cpuOf(v json.RawMessage) (*timingCPU, []string, bool) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(v, &fields); err != nil || fields == nil {
		return nil, nil, false
	}
	var cpu timingCPU
	if json.Unmarshal(fields["status"], &cpu.Status) != nil {
		return nil, nil, false
	}
	for name, target := range map[string]*string{"scope": &cpu.Scope, "source": &cpu.Source} {
		if raw, ok := fields[name]; ok && json.Unmarshal(raw, target) != nil {
			return nil, nil, false
		}
	}
	if (cpu.Scope != "" && !cpuScopes[cpu.Scope]) || (cpu.Source != "" && !cpuSources[cpu.Source]) {
		return nil, nil, false
	}
	var damaged []string
	switch cpu.Status {
	case run.CPUMeasured:
		if cpu.Scope == "" || cpu.Source == "" {
			return nil, nil, false
		}
		for _, item := range []struct {
			name   string
			target **int64
		}{{"user_ms", &cpu.UserMS}, {"system_ms", &cpu.SystemMS}} {
			if ms, ok := wholeMS(fields[item.name]); ok {
				*item.target = &ms
			} else {
				damaged = append(damaged, item.name)
			}
		}
		if len(damaged) == 2 {
			return nil, nil, false
		}
	case run.CPUUnavailable, run.CPUFailed:
		if _, ok := fields["user_ms"]; ok {
			return nil, nil, false
		}
		if _, ok := fields["system_ms"]; ok {
			return nil, nil, false
		}
	default:
		return nil, nil, false
	}
	return &cpu, damaged, true
}

func timingMarkdown(rep timingReport) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Time view for %s (%s). These are measurements only: status, freshness, usage and budget do not use them.\n\n", rep.Goal, rep.Schema)
	if len(rep.Runs) == 0 {
		b.WriteString("No check or baseline runs are recorded.\n")
		return b.String()
	}
	b.WriteString("| seq | target | kind | result | duration | limit | limit elapsed | limit use | limit reached | CPU user / system |\n")
	b.WriteString("| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, r := range rep.Runs {
		target := r.Target.Criterion
		if r.Target.Task != "" {
			target = fmt.Sprintf("%s command %d", r.Target.Task, r.Target.Index)
		}
		limit := absent(r.Unavailable["limit_ms"])
		if r.LimitMS != nil {
			limit = seconds(*r.LimitMS)
		}
		elapsed := absent(r.Unavailable["limit_elapsed_ms"])
		if r.LimitElapsedMS != nil {
			elapsed = seconds(*r.LimitElapsedMS)
		}
		use := absent(r.Unavailable["limit_use_permille"])
		if r.LimitUsePermille != nil {
			use = fmt.Sprintf("%d.%d%%", *r.LimitUsePermille/10, *r.LimitUsePermille%10)
		}
		reached := absent(r.Unavailable["limit_reached"])
		if r.LimitReached != nil {
			reached = map[bool]string{true: "yes", false: "no"}[*r.LimitReached]
		}
		cpu := absent(r.Unavailable["cpu"])
		if r.CPU != nil {
			switch r.CPU.Status {
			case run.CPUMeasured:
				shown := func(ms *int64, name string) string {
					if ms == nil {
						return absent(r.Unavailable["cpu."+name])
					}
					return seconds(*ms)
				}
				cpu = fmt.Sprintf("%s / %s (%s, %s)", shown(r.CPU.UserMS, "user_ms"), shown(r.CPU.SystemMS, "system_ms"), r.CPU.Scope, r.CPU.Source)
			case run.CPUFailed:
				cpu = "measurement failed"
			default:
				cpu = "not available"
			}
		}
		fmt.Fprintf(&b, "| %d | %s | %s | %s | %s | %s | %s | %s | %s | %s |\n",
			r.Seq, target, r.Kind, r.Result, seconds(r.DurationMS), limit, elapsed, use, reached, cpu)
	}
	fmt.Fprintf(&b, "\nSum of run durations: %s. Runs can overlap; this is not the goal's elapsed time or budget.\n", seconds(rep.TotalDurationMS))
	b.WriteString("Limit elapsed runs from the limit timer's start to the observed exit or the timeout decision; duration also covers process start and cleanup.\n")
	return b.String()
}

func absent(reason string) string {
	switch reason {
	case timingNotMeasured:
		return "not measured"
	case timingInvalid:
		return "unavailable (invalid)"
	}
	return "not recorded"
}

func seconds(ms int64) string {
	if ms%1000 == 0 {
		return fmt.Sprintf("%d s", ms/1000)
	}
	return fmt.Sprintf("%d.%03d s", ms/1000, ms%1000)
}
