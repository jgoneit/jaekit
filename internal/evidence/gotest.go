package evidence

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"unicode/utf8"
)

// goEvent is one line of `go test -json`. Fields the adapter does not use are
// ignored, so later Go versions may add them.
type goEvent struct {
	Action      string  `json:"Action"`
	Package     string  `json:"Package"`
	Test        string  `json:"Test"`
	Output      *string `json:"Output"`
	OutputType  string  `json:"OutputType"`
	FailedBuild string  `json:"FailedBuild"`
}

var goActions = map[string]bool{
	"start": true, "run": true, "pause": true, "cont": true, "pass": true, "bench": true,
	"fail": true, "output": true, "skip": true, "build-output": true, "build-fail": true,
}

type goAttempt struct {
	end    string   // pass, fail or skip; empty when no end record was seen
	errors []string // first lines of the test's own error-typed output
}

type goPackage struct {
	typed, failedBuild, invalid bool
}

// goObservations turns the standard output of `go test -json` into target
// observations. It reads only events; standard error never reaches it. An
// unparsable line makes the whole output corrupt. The second result is an
// early reason; the third lists packages whose failures could not be read
// because the toolchain marked no output type.
func goObservations(d *Declaration, data []byte) ([]Observation, string, []string) {
	linked := map[[2]string]int{}
	packages := map[string]*goPackage{}
	for n, t := range d.Targets {
		linked[[2]string{t.Test.Package, t.Test.Name}] = n
		packages[t.Test.Package] = &goPackage{}
	}
	attempts := make([][]*goAttempt, len(d.Targets))
	open := make([]*goAttempt, len(d.Targets))
	closeOpen := func(pkg string) {
		for n, t := range d.Targets {
			if t.Test.Package == pkg {
				open[n] = nil
			}
		}
	}
	lines := bytes.Split(data, []byte("\n"))
	if len(lines) > 0 && len(lines[len(lines)-1]) == 0 {
		lines = lines[:len(lines)-1]
	}
	for _, line := range lines {
		line = bytes.TrimSuffix(line, []byte("\r"))
		if len(line) == 0 || line[0] != '{' || !utf8.Valid(line) {
			return nil, "output_corrupt", nil
		}
		var e goEvent
		if json.Unmarshal(line, &e) != nil || e.Action == "" {
			return nil, "output_corrupt", nil
		}
		pkg, watched := packages[e.Package]
		if !watched {
			continue
		}
		if e.OutputType != "" {
			pkg.typed = true
		}
		if !goActions[e.Action] {
			pkg.invalid = true
			continue
		}
		if e.Test == "" {
			switch e.Action {
			case "pass", "fail", "skip":
				// The package ended: a linked test still open never ended.
				closeOpen(e.Package)
				if e.FailedBuild != "" {
					pkg.failedBuild = true
				}
			}
			continue
		}
		n, ok := linked[[2]string{e.Package, e.Test}]
		if !ok {
			continue
		}
		switch e.Action {
		case "run":
			open[n] = &goAttempt{}
			attempts[n] = append(attempts[n], open[n])
		case "output":
			if open[n] != nil && e.OutputType == "error" && e.Output != nil {
				open[n].errors = append(open[n].errors, firstLine(*e.Output))
			}
		case "pass", "fail", "skip":
			if open[n] == nil {
				attempts[n] = append(attempts[n], &goAttempt{})
				open[n] = attempts[n][len(attempts[n])-1]
			}
			open[n].end = e.Action
			open[n] = nil
		}
	}
	var observations []Observation
	early := map[string]bool{}
	untyped := map[string]bool{}
	for n, t := range d.Targets {
		pkg := packages[t.Test.Package]
		switch {
		case pkg.invalid:
			early["observation_invalid"] = true
			continue
		case len(attempts[n]) == 0 && pkg.failedBuild:
			observations = append(observations, Observation{Target: t.ID, Attempt: 1, Status: "error", Reason: "compile_error"})
			continue
		case len(attempts[n]) == 0:
			early["target_missing"] = true
			continue
		}
		for a, attempt := range attempts[n] {
			o := Observation{Target: t.ID, Attempt: a + 1}
			switch attempt.end {
			case "pass":
				o.Status = "pass"
			case "skip":
				o.Status, o.Reason = "skip", "skipped"
			case "fail":
				o.Status = "violation"
				if pkg.typed {
					o.Violation = failedViolation(attempt.errors)
				} else {
					o.Violation = UnmarkedViolation
					untyped[t.Test.Package] = true
				}
			default:
				o.Status, o.Reason = "error", "execution_error"
			}
			observations = append(observations, o)
		}
	}
	var notices []string
	for pkg := range untyped {
		notices = append(notices, fmt.Sprintf("%s: package %s printed no OutputType, so its failure messages and markers were not read (go test -json of Go 1.27 or later marks them)", AdapterGoTestJSON, pkg))
	}
	sort.Strings(notices)
	if reason := earlyPrecedence(early); reason != "" {
		return nil, reason, notices
	}
	return observations, "", notices
}
