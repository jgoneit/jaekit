package evidence

import (
	"bytes"
	"encoding/xml"
	"io"
	"strings"
	"unicode/utf8"
)

// junitCase is what the adapter keeps of one linked testcase.
type junitCase struct {
	failures []string // first line of each failure's message attribute or body
	errors   int
	skipped  int
	retry    bool // rerunFailure, rerunError, flakyFailure or flakyError
	invalid  bool // a child or combination outside the supported forms
}

var junitRetries = map[string]bool{"rerunFailure": true, "rerunError": true, "flakyFailure": true, "flakyError": true}
var junitIgnored = map[string]bool{"system-out": true, "system-err": true, "properties": true}

// junitClassname reports whether a testcase's classname attribute matches a
// declared classname. An empty declared classname matches a testcase without
// the attribute; otherwise the values must be equal.
func junitClassname(declared string, attr *string) bool {
	if attr == nil {
		return declared == ""
	}
	return *attr == declared
}

// junitObservations reads one JUnit XML document. The document must be UTF-8
// with a single testsuites or testsuite root and no DTD; testcases may sit at
// any depth. Only linked testcases are judged.
func junitObservations(d *Declaration, data []byte) ([]Observation, string) {
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
	if !utf8.Valid(data) {
		return nil, "output_corrupt"
	}
	dec := xml.NewDecoder(bytes.NewReader(data))
	dec.Strict = true
	occurrences := make([]int, len(d.Targets))
	cases := make([]*junitCase, len(d.Targets))
	type frame struct {
		name    string
		target  int // the linked target of a testcase frame, or -1
		message *string
		body    strings.Builder
	}
	var stack []*frame
	rootSeen, rootDone := false, false
	// current is the innermost linked testcase being read, if any.
	current := func() (*junitCase, int) {
		for depth := len(stack) - 1; depth >= 0; depth-- {
			if stack[depth].name == "testcase" {
				if stack[depth].target < 0 {
					return nil, depth
				}
				return cases[stack[depth].target], depth
			}
		}
		return nil, -1
	}
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, "output_corrupt"
		}
		switch t := token.(type) {
		case xml.StartElement:
			seen := map[xml.Name]bool{}
			for _, attr := range t.Attr {
				if seen[attr.Name] {
					return nil, "output_corrupt"
				}
				seen[attr.Name] = true
			}
			name := t.Name.Local
			if len(stack) == 0 {
				if rootDone || t.Name.Space != "" || (name != "testsuites" && name != "testsuite") {
					return nil, "output_corrupt"
				}
				rootSeen = true
			}
			f := &frame{name: name, target: -1}
			if tc, depth := current(); tc != nil {
				// A direct child of a linked testcase, or deeper content.
				switch {
				case depth != len(stack)-1:
					// Content of system-out, system-err and properties is not read.
					if !junitIgnored[stack[depth+1].name] {
						tc.invalid = true
					}
				case name == "failure" || name == "error":
					for _, attr := range t.Attr {
						if attr.Name.Space == "" && attr.Name.Local == "message" {
							value := attr.Value
							f.message = &value
						}
					}
					if name == "error" {
						tc.errors++
					}
				case name == "skipped":
					tc.skipped++
				case junitRetries[name]:
					tc.retry = true
				case junitIgnored[name]:
				default:
					tc.invalid = true
				}
			} else if name == "testcase" {
				var classname *string
				var testName string
				for _, attr := range t.Attr {
					if attr.Name.Space != "" {
						continue
					}
					switch attr.Name.Local {
					case "classname":
						value := attr.Value
						classname = &value
					case "name":
						testName = attr.Value
					}
				}
				for n, target := range d.Targets {
					if target.Test.Name == testName && junitClassname(target.Test.Classname, classname) {
						occurrences[n]++
						if cases[n] == nil {
							cases[n] = &junitCase{}
						}
						f.target = n
						break
					}
				}
			}
			stack = append(stack, f)
		case xml.EndElement:
			f := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if tc, depth := current(); tc != nil && depth == len(stack)-1 && f.name == "failure" {
				if f.message != nil {
					tc.failures = append(tc.failures, firstLine(*f.message))
				} else {
					tc.failures = append(tc.failures, firstLine(f.body.String()))
				}
			}
			if len(stack) == 0 {
				rootDone = true
			}
		case xml.CharData:
			if len(stack) == 0 {
				if len(bytes.TrimSpace(t)) != 0 {
					return nil, "output_corrupt"
				}
				continue
			}
			stack[len(stack)-1].body.Write(t)
		case xml.Directive:
			return nil, "output_corrupt"
		}
	}
	if !rootSeen || !rootDone {
		return nil, "output_corrupt"
	}
	var observations []Observation
	early := map[string]bool{}
	for n, t := range d.Targets {
		tc := cases[n]
		switch {
		case occurrences[n] == 0:
			early["target_missing"] = true
			continue
		case occurrences[n] > 1 || tc.retry:
			early["attempt_history_missing"] = true
			continue
		case tc.invalid || (tc.skipped > 0 && (len(tc.failures) > 0 || tc.errors > 0)):
			early["observation_invalid"] = true
			continue
		}
		o := Observation{Target: t.ID, Attempt: 1}
		switch {
		case tc.errors > 0:
			o.Status, o.Reason = "error", "execution_error"
		case len(tc.failures) > 0:
			o.Status, o.Violation = "violation", failedViolation(tc.failures)
		case tc.skipped > 0:
			o.Status, o.Reason = "skip", "skipped"
		default:
			o.Status = "pass"
		}
		observations = append(observations, o)
	}
	if reason := earlyPrecedence(early); reason != "" {
		return nil, reason
	}
	return observations, ""
}
