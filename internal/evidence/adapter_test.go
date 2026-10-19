package evidence

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/run"
)

const testPackage = "example.com/demo/forms"

func goDeclaration(targets ...[3]string) *Declaration {
	d := &Declaration{Schema: AdapterSchema, Adapter: AdapterGoTestJSON}
	for _, t := range targets {
		d.Targets = append(d.Targets, Target{ID: t[0], Violation: t[1], Test: &TestName{Package: testPackage, Name: t[2]}})
	}
	return d
}

func junitDeclaration(classname, name string) *Declaration {
	return &Declaration{Schema: AdapterSchema, Adapter: AdapterJUnitXML,
		Targets: []Target{{ID: "t", Violation: "greeting-missing", Test: &TestName{Classname: classname, Name: name}}}}
}

func committedRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v %s", err, out)
		}
	}
	git("init", "-q")
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("commit", "-qm", "fixture")
	return root
}

func TestAdapterDeclarationLint(t *testing.T) {
	goTest := `{"package": "example.com/demo", "name": "TestX"}`
	target := func(test string) string {
		return `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "t", "violation": "v", "test": ` + test + `}]}`
	}
	bad := map[string]string{
		"unknown adapter":          `{"schema": "check-adapter/v1", "adapter": "pytest-json", "targets": [{"id": "t", "violation": "v", "test": ` + goTest + `}]}`,
		"no test":                  `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "t", "violation": "v"}]}`,
		"no name":                  target(`{"package": "example.com/demo"}`),
		"empty name":               target(`{"package": "example.com/demo", "name": ""}`),
		"control character":        target(`{"package": "example.com/demo", "name": "TestX\u0001"}`),
		"C1 control character":     target(`{"package": "example.com/demo", "name": "TestX\u0085"}`),
		"JUnit name in Go":         target(`{"classname": "a", "name": "TestX"}`),
		"Go name in JUnit":         `{"schema": "check-adapter/v1", "adapter": "junit-xml", "targets": [{"id": "t", "violation": "v", "test": ` + goTest + `}]}`,
		"not a Go test":            target(`{"package": "example.com/demo", "name": "helper"}`),
		"lowercase after Test":     target(`{"package": "example.com/demo", "name": "Testx"}`),
		"empty subtest":            target(`{"package": "example.com/demo", "name": "TestX//y"}`),
		"space in subtest":         target(`{"package": "example.com/demo", "name": "TestX/a b"}`),
		"space in package":         target(`{"package": "example.com/a b", "name": "TestX"}`),
		"long name":                target(`{"package": "example.com/demo", "name": "TestX` + strings.Repeat("a", 1024) + `"}`),
		"reserved violation":       `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "t", "violation": "jaekit:unmarked-failure", "test": ` + goTest + `}]}`,
		"one test, two targets":    `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "a", "violation": "v", "test": ` + goTest + `}, {"id": "b", "violation": "w", "test": ` + goTest + `}]}`,
		"producer field":           `{"schema": "check-adapter/v1", "adapter": "go-test-json", "producer": "x", "targets": [{"id": "t", "violation": "v", "test": ` + goTest + `}]}`,
		"unknown test field":       target(`{"package": "example.com/demo", "name": "TestX", "file": "x"}`),
		"duplicate key":            `{"schema": "check-adapter/v1", "adapter": "go-test-json", "adapter": "go-test-json", "targets": [{"id": "t", "violation": "v", "test": ` + goTest + `}]}`,
		"no targets":               `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": []}`,
		"adapter in a v1 producer": `{"schema": "check-declaration/v1", "adapter": "go-test-json", "producer": "p", "producer_version": "1", "producer_paths": ["p.py"], "targets": [{"id": "t", "violation": "v"}]}`,
	}
	files := map[string]string{"p.py": "x\n"}
	for name, content := range bad {
		files[strings.ReplaceAll(name, " ", "-")+".json"] = content
	}
	good := map[string]string{
		"go.json":        target(goTest),
		"fuzz.json":      target(`{"package": "example.com/demo", "name": "Fuzz_X/sub/case"}`),
		"example.json":   target(`{"package": "example.com/demo", "name": "Example"}`),
		"junit.json":     `{"schema": "check-adapter/v1", "adapter": "junit-xml", "targets": [{"id": "t", "violation": "v", "test": {"classname": "", "name": "greets with hello"}}]}`,
		"unicode.json":   target(`{"package": "example.com/demo", "name": "TestÜber/한글"}`),
		"two-junit.json": `{"schema": "check-adapter/v1", "adapter": "junit-xml", "targets": [{"id": "a", "violation": "v", "test": {"classname": "x", "name": "n"}}, {"id": "b", "violation": "v", "test": {"classname": "y", "name": "n"}}]}`,
	}
	for name, content := range good {
		files[name] = content
	}
	root := committedRepo(t, files)
	for name := range bad {
		if _, _, err := Load(root, strings.ReplaceAll(name, " ", "-")+".json"); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for name := range good {
		d, binding, err := Load(root, name)
		if err != nil || d.Adapter == "" || !digestPattern.MatchString(binding) {
			t.Errorf("%s: %v", name, err)
		}
	}
	// The adapter's classification meaning version is part of the binding.
	_, before, _ := Load(root, "go.json")
	_, junitBefore, _ := Load(root, "junit.json")
	saved := adapterSemantics[AdapterGoTestJSON]
	adapterSemantics[AdapterGoTestJSON] = "2"
	_, after, _ := Load(root, "go.json")
	_, junitAfter, _ := Load(root, "junit.json")
	adapterSemantics[AdapterGoTestJSON] = saved
	if before == after || junitBefore != junitAfter {
		t.Fatalf("binding does not follow the adapter's meaning version: %s %s %s %s", before, after, junitBefore, junitAfter)
	}
}

func TestMarkerReading(t *testing.T) {
	long := strings.Repeat("a", 129)
	cases := []struct {
		lines []string
		want  string
	}{
		{[]string{"x jaekit:violation=greeting-missing broke"}, "greeting-missing"},
		{[]string{"jaekit:violation=a.b/c:d-e_f"}, "a.b/c:d-e_f"},
		{[]string{"jaekit:violation=" + long}, UnmarkedViolation},
		{[]string{"jaekit:violation=" + long[:128]}, long[:128]},
		{[]string{"jaekit:violation=_x"}, UnmarkedViolation},
		{[]string{"jaekit:violation= x"}, UnmarkedViolation},
		{[]string{"jaekit:violation=a", "jaekit:violation=a again"}, "a"},
		{[]string{"jaekit:violation=a", "plain"}, UnmarkedViolation},
		{[]string{"jaekit:violation=a jaekit:violation=b"}, UnmarkedViolation},
		{[]string{"jaekit:violation=a", "jaekit:violation=b"}, UnmarkedViolation},
		{[]string{"JAEKIT:violation=a"}, UnmarkedViolation},
		{nil, UnmarkedViolation},
		{[]string{""}, UnmarkedViolation},
	}
	for _, c := range cases {
		if got := failedViolation(c.lines); got != c.want {
			t.Errorf("%q: got %q, want %q", c.lines, got, c.want)
		}
	}
	if got := firstLine("a\r\nb"); got != "a" {
		t.Errorf("first line of CRLF text: %q", got)
	}
}

type goStream struct{ lines []string }

func (s *goStream) event(action, test, output, kind string, extra ...string) *goStream {
	e := map[string]any{"Action": action, "Package": testPackage}
	if test != "" {
		e["Test"] = test
	}
	if output != "" {
		e["Output"] = output
	}
	if kind != "" {
		e["OutputType"] = kind
	}
	for i := 0; i+1 < len(extra); i += 2 {
		e[extra[i]] = extra[i+1]
	}
	data, _ := json.Marshal(e)
	s.lines = append(s.lines, string(data))
	return s
}

func (s *goStream) bytes() []byte { return []byte(strings.Join(s.lines, "\n") + "\n") }

func ran(test string, outputs [][2]string, end string) *goStream {
	s := (&goStream{}).event("start", "", "", "").event("run", test, "", "").event("output", test, "=== RUN   "+test+"\n", "frame")
	for _, o := range outputs {
		s.event("output", test, o[0], o[1])
	}
	if end != "" {
		s.event(end, test, "", "")
	}
	return s
}

func TestGoTestJSONObservations(t *testing.T) {
	mark := "    x_test.go:3: jaekit:violation=greeting-missing broke\n"
	d := goDeclaration([3]string{"t", "greeting-missing", "TestMarked"})
	obs := func(status, violation, reason string, attempt int) Observation {
		return Observation{Target: "t", Attempt: attempt, Status: status, Violation: violation, Reason: reason}
	}
	twice := ran("TestMarked", nil, "pass")
	twice.event("run", "TestMarked", "", "").event("output", "TestMarked", mark, "error").event("fail", "TestMarked", "", "")
	cases := []struct {
		name    string
		data    []byte
		want    []Observation
		early   string
		notices int
	}{
		{"G1 marker in a log of a passing test", ran("TestMarked", [][2]string{{"    jaekit:violation=greeting-missing\n", ""}}, "pass").bytes(), []Observation{obs("pass", "", "", 1)}, "", 0},
		{"G2 marked error", ran("TestMarked", [][2]string{{mark, "error"}}, "fail").bytes(), []Observation{obs("violation", "greeting-missing", "", 1)}, "", 0},
		{"G3 logged marker, unmarked error", ran("TestMarked", [][2]string{{mark, ""}, {"plain\n", "error"}}, "fail").bytes(), []Observation{obs("violation", UnmarkedViolation, "", 1)}, "", 0},
		{"G5 marker on a continuation line", ran("TestMarked", [][2]string{{"first\n", "error"}, {mark, "error-continue"}}, "fail").bytes(), []Observation{obs("violation", UnmarkedViolation, "", 1)}, "", 0},
		{"G6 failure without messages", ran("TestMarked", [][2]string{{"panic: jaekit:violation=greeting-missing\n", ""}}, "fail").bytes(), []Observation{obs("violation", UnmarkedViolation, "", 1)}, "", 0},
		{"G10 no end record", ran("TestMarked", [][2]string{{mark, "error"}}, "").event("fail", "", "", "").bytes(), []Observation{obs("error", "", "execution_error", 1)}, "", 0},
		{"no end and no package end", ran("TestMarked", nil, "").bytes(), []Observation{obs("error", "", "execution_error", 1)}, "", 0},
		{"G11 attempts in order", twice.bytes(), []Observation{obs("pass", "", "", 1), obs("violation", "greeting-missing", "", 2)}, "", 0},
		{"skip", ran("TestMarked", nil, "skip").bytes(), []Observation{obs("skip", "", "skipped", 1)}, "", 0},
		{"G9 build failure", (&goStream{}).event("start", "", "", "").event("fail", "", "", "", "FailedBuild", testPackage).bytes(), []Observation{obs("error", "", "compile_error", 1)}, "", 0},
		{"not run", ran("TestOther", nil, "fail").bytes(), nil, "target_missing", 0},
		{"G15 untyped toolchain", (&goStream{}).event("run", "TestMarked", "", "").event("output", "TestMarked", mark, "").event("fail", "TestMarked", "", "").bytes(), []Observation{obs("violation", UnmarkedViolation, "", 1)}, "", 1},
		{"unknown action on the linked package", ran("TestMarked", nil, "pass").event("teleport", "", "", "").bytes(), nil, "observation_invalid", 0},
		{"G13 not JSON", []byte("{\"Action\":\"start\"}\nnot json\n"), nil, "output_corrupt", 0},
		{"blank line", []byte("{\"Action\":\"start\"}\n\n"), nil, "output_corrupt", 0},
		{"no action", []byte("{\"Package\":\"x\"}\n"), nil, "output_corrupt", 0},
		{"JSON null", []byte("null\n"), nil, "output_corrupt", 0},
		{"invalid UTF-8", []byte("{\"Action\":\"start\",\"Package\":\"\xff\"}\n"), nil, "output_corrupt", 0},
		{"truncated", []byte("{\"Action\":\"start\"}\n{\"Action\":\"ru"), nil, "output_corrupt", 0},
	}
	for _, c := range cases {
		got, early, notices := goObservations(d, c.data)
		if !reflect.DeepEqual(got, c.want) || early != c.early || len(notices) != c.notices {
			t.Errorf("%s: got %+v %q %q, want %+v %q", c.name, got, early, notices, c.want, c.early)
		}
		crlf := []byte(strings.ReplaceAll(string(c.data), "\n", "\r\n"))
		if again, againEarly, _ := goObservations(d, crlf); !reflect.DeepEqual(again, got) || againEarly != early {
			t.Errorf("%s: CRLF differs: %+v %q", c.name, again, againEarly)
		}
	}
	// G12: the same test name in another package is not the linked test.
	other := ran("TestMarked", nil, "pass")
	other.lines = append(other.lines, `{"Action":"run","Package":"example.com/demo/other","Test":"TestMarked"}`,
		`{"Action":"output","Package":"example.com/demo/other","Test":"TestMarked","Output":"jaekit:violation=greeting-missing\n","OutputType":"error"}`,
		`{"Action":"fail","Package":"example.com/demo/other","Test":"TestMarked"}`, `{"Action":"teleport","Package":"example.com/demo/other"}`)
	if got, early, _ := goObservations(d, other.bytes()); early != "" || !reflect.DeepEqual(got, []Observation{obs("pass", "", "", 1)}) {
		t.Errorf("G12: %+v %q", got, early)
	}
	// G7, G8: a parent sees only its own messages; a linked subtest sees its own.
	family := ran("TestParent", nil, "")
	family.event("run", "TestParent/child", "", "").event("output", "TestParent/child", mark, "error").event("fail", "TestParent/child", "", "").event("fail", "TestParent", "", "")
	parent := goDeclaration([3]string{"p", "greeting-missing", "TestParent"}, [3]string{"c", "greeting-missing", "TestParent/child"})
	got, early, _ := goObservations(parent, family.bytes())
	want := []Observation{{Target: "p", Attempt: 1, Status: "violation", Violation: UnmarkedViolation}, {Target: "c", Attempt: 1, Status: "violation", Violation: "greeting-missing"}}
	if early != "" || !reflect.DeepEqual(got, want) {
		t.Errorf("G7/G8: %+v %q", got, early)
	}
}

func junitDocument(cases string) []byte {
	return []byte(`<?xml version="1.0" encoding="utf-8"?>` + "\n<testsuites><testsuite name=\"s\">\n" + cases + "</testsuite></testsuites>\n")
}

func TestJUnitObservations(t *testing.T) {
	mark := "jaekit:violation=greeting-missing"
	d := junitDeclaration("tests.test_greet", "test_greets")
	one := func(inner string) []byte {
		return junitDocument(`<testcase classname="tests.test_greet" name="test_greets">` + inner + "</testcase>\n")
	}
	obs := func(status, violation, reason string) []Observation {
		return []Observation{{Target: "t", Attempt: 1, Status: status, Violation: violation, Reason: reason}}
	}
	cases := []struct {
		name  string
		data  []byte
		want  []Observation
		early string
	}{
		{"J1 pass", one(""), obs("pass", "", ""), ""},
		{"J2 message attribute", one(`<failure message="AssertionError: ` + mark + ` wrong&#10;assert x">trace ` + mark + `</failure>`), obs("violation", "greeting-missing", ""), ""},
		{"J3 marker only in the body", one(`<failure message="assert x">` + mark + `</failure>`), obs("violation", UnmarkedViolation, ""), ""},
		{"J5 body first line", one(`<failure><![CDATA[Error: ` + mark + "\n    at x]]></failure>"), obs("violation", "greeting-missing", ""), ""},
		{"blank first body line", one("<failure>\n   Error: " + mark + "</failure>"), obs("violation", UnmarkedViolation, ""), ""},
		{"empty message attribute", one(`<failure message="">` + mark + `</failure>`), obs("violation", UnmarkedViolation, ""), ""},
		{"J10 marker in system-out", one(`<failure message="x">y</failure><system-out>` + mark + `</system-out>`), obs("violation", UnmarkedViolation, ""), ""},
		{"two marked failures", one(`<failure message="` + mark + ` a"/><failure message="` + mark + ` b"/>`), obs("violation", "greeting-missing", ""), ""},
		{"J6 error", one(`<error message="setup">x</error>`), obs("error", "", "execution_error"), ""},
		{"error and failure", one(`<failure message="` + mark + `"/><error message="x"/>`), obs("error", "", "execution_error"), ""},
		{"J7 skipped", one(`<skipped message="no"/>`), obs("skip", "", "skipped"), ""},
		{"properties", one(`<properties><property name="a" value="b"/></properties>`), obs("pass", "", ""), ""},
		{"J8 nothing collected", []byte(`<?xml version="1.0"?><testsuites><testsuite tests="0"/></testsuites>`), nil, "target_missing"},
		{"J9 duplicate", junitDocument(`<testcase classname="tests.test_greet" name="test_greets"/><testcase classname="tests.test_greet" name="test_greets"/>`), nil, "attempt_history_missing"},
		{"rerunFailure", one(`<rerunFailure message="x"/>`), nil, "attempt_history_missing"},
		{"rerunError", one(`<rerunError message="x"/>`), nil, "attempt_history_missing"},
		{"flakyFailure", one(`<flakyFailure message="x"/>`), nil, "attempt_history_missing"},
		{"flakyError", one(`<flakyError message="x"/>`), nil, "attempt_history_missing"},
		{"unknown child", one(`<surprise/>`), nil, "observation_invalid"},
		{"skipped with failure", one(`<skipped/><failure message="` + mark + `"/>`), nil, "observation_invalid"},
		{"element inside a failure", one(`<failure><b>` + mark + `</b></failure>`), nil, "observation_invalid"},
		{"J11 DTD", []byte(`<?xml version="1.0"?><!DOCTYPE testsuites [<!ENTITY m "x">]><testsuites/>`), nil, "output_corrupt"},
		{"J11 truncated", one("")[:40], nil, "output_corrupt"},
		{"syntax error", []byte(`<testsuites><testsuite></testsuit></testsuites>`), nil, "output_corrupt"},
		{"two roots", append(one(""), []byte("<testsuites/>")...), nil, "output_corrupt"},
		{"text after the root", append(one(""), []byte("x")...), nil, "output_corrupt"},
		{"not JUnit", []byte(`<html><testcase classname="tests.test_greet" name="test_greets"/></html>`), nil, "output_corrupt"},
		{"declared Latin-1", []byte(`<?xml version="1.0" encoding="ISO-8859-1"?><testsuites/>`), nil, "output_corrupt"},
		{"invalid UTF-8", []byte("<testsuites>\xff</testsuites>"), nil, "output_corrupt"},
		{"duplicate attribute", []byte(`<testsuites><testcase classname="a" classname="b" name="test_greets"/></testsuites>`), nil, "output_corrupt"},
		{"empty file", []byte{}, nil, "output_corrupt"},
		{"undefined entity", one(`<failure message="&m;"/>`), nil, "output_corrupt"},
		{"byte order mark", append([]byte("\xef\xbb\xbf"), one("")...), obs("pass", "", ""), ""},
		{"testsuite root and deep testcase", []byte(`<testsuite><x><testcase classname="tests.test_greet" name="test_greets"/></x></testsuite>`), obs("pass", "", ""), ""},
	}
	for _, c := range cases {
		got, early := junitObservations(d, c.data)
		if !reflect.DeepEqual(got, c.want) || early != c.early {
			t.Errorf("%s: got %+v %q, want %+v %q", c.name, got, early, c.want, c.early)
		}
		crlf := []byte(strings.ReplaceAll(string(c.data), "\n", "\r\n"))
		if again, againEarly := junitObservations(d, crlf); !reflect.DeepEqual(again, got) || againEarly != early {
			t.Errorf("%s: CRLF differs: %+v %q", c.name, again, againEarly)
		}
	}
	// Exact classname and name: an empty declared classname matches a testcase without one.
	blank := junitDeclaration("", "greets")
	for doc, want := range map[string]string{
		`<testcase name="greets"/>`:                    "",
		`<testcase classname="" name="greets"/>`:       "",
		`<testcase classname="x" name="greets"/>`:      "target_missing",
		`<testcase classname="" name="greets again"/>`: "target_missing",
		`<testcase classname="tests" name="Greets"/>`:  "target_missing",
	} {
		if _, early := junitObservations(blank, junitDocument(doc)); early != want {
			t.Errorf("%s: %q, want %q", doc, early, want)
		}
	}
}

func TestAdapterClassifyAndStored(t *testing.T) {
	binding := strings.Repeat("b", 64)
	d := goDeclaration([3]string{"t", "greeting-missing", "TestMarked"})
	prepare := func(d *Declaration) *Invocation {
		i, err := PrepareAdapter(t.TempDir(), d)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { i.Close() })
		return i
	}
	i := prepare(d)
	i.Stdout().Write(ran("TestMarked", [][2]string{{"jaekit:violation=greeting-missing\n", "error"}}, "fail").bytes())
	base := i.Classify(d, binding, processResult(1), true)
	if base.Result != "fail_as_expected" || base.Reason != "declared_violation" || !digestPattern.MatchString(base.ReportDigest) {
		t.Fatalf("baseline: %+v", base)
	}
	if err := ValidateStored(&base, d, binding, true, base.Result, intPtr(1)); err != nil {
		t.Fatalf("stored baseline: %v", err)
	}
	if err := ValidateStored(&base, d, strings.Repeat("c", 64), true, base.Result, intPtr(1)); err == nil {
		t.Fatal("stored evidence accepted under another binding")
	}
	if s := prepare(d).Classify(d, binding, processResult(1), false); s.Result != "unknown" || s.Reason != "output_missing" {
		t.Fatalf("no output: %+v", s)
	}
	big := prepare(d)
	big.Stdout().Write(make([]byte, maxGoOutput+1))
	if s := big.Classify(d, binding, processResult(1), false); s.Reason != "output_too_large" {
		t.Fatalf("over the limit: %+v", s)
	}
	for _, reason := range []string{"output_missing", "output_corrupt", "output_too_large"} {
		s := Summary{Schema: SummarySchema, Result: "unknown", Reason: reason, DeclarationDigest: binding, Invocation: strings.Repeat("a", 64)}
		if err := ValidateStored(&s, d, binding, false, "unknown", intPtr(1)); err != nil {
			t.Errorf("%s: %v", reason, err)
		}
		if err := ValidateStored(&s, testDeclaration(), binding, false, "unknown", intPtr(1)); err == nil {
			t.Errorf("%s accepted for a producer declaration", reason)
		}
	}
	missing := Summary{Schema: SummarySchema, Result: "unknown", Reason: "report_missing", DeclarationDigest: binding, Invocation: strings.Repeat("a", 64)}
	if err := ValidateStored(&missing, d, binding, false, "unknown", intPtr(1)); err == nil {
		t.Error("a producer report reason accepted for an adapter")
	}
	odd := Summary{Schema: SummarySchema, Result: "error", Reason: "environment_reported", DeclarationDigest: binding, Invocation: strings.Repeat("a", 64),
		ReportDigest: strings.Repeat("d", 64), Targets: []Observation{{Target: "t", Attempt: 1, Status: "error", Reason: "import_error"}}}
	if err := ValidateStored(&odd, d, binding, false, "error", intPtr(1)); err == nil {
		t.Error("an environment reason no adapter produces was accepted")
	}

	j := junitDeclaration("tests.test_greet", "test_greets")
	ji := prepare(j)
	if env := ji.AdapterEnv(); len(env) != 1 || env[0] != JUnitVariable+"="+ji.Path {
		t.Fatalf("environment: %v", env)
	}
	argv := ji.CommandArgv([]string{"runner", JUnitPlaceholder, "--out=" + JUnitPlaceholder})
	if argv[1] != ji.Path || argv[2] != "--out="+ji.Path {
		t.Fatalf("argv: %v", argv)
	}
	if entries, err := os.ReadDir(filepath.Dir(ji.Path)); err != nil || len(entries) != 0 {
		t.Fatalf("location is not new and empty: %v %v", entries, err)
	}
	if s := ji.Classify(j, binding, processResult(1), false); s.Reason != "output_missing" {
		t.Fatalf("no file: %+v", s)
	}
	if err := os.Mkdir(ji.Path, 0o700); err != nil {
		t.Fatal(err)
	}
	if s := ji.Classify(j, binding, processResult(0), false); s.Reason != "output_missing" {
		t.Fatalf("directory: %+v", s)
	}
	os.Remove(ji.Path)
	if err := os.WriteFile(ji.Path, junitDocument(`<testcase classname="tests.test_greet" name="test_greets"><error message="x"/></testcase>`), 0o600); err != nil {
		t.Fatal(err)
	}
	if s := ji.Classify(j, binding, processResult(0), false); s.Result != "unknown" || s.Reason != "report_exit_conflict" || len(s.Targets) != 1 {
		t.Fatalf("error element with exit 0: %+v", s)
	}
	if s := ji.Classify(j, binding, run.Result{Attempted: true, Outcome: run.Error, Reason: "process_start"}, false); s.Reason != "process_start" {
		t.Fatalf("process failure: %+v", s)
	}
	if s := ji.Classify(d, binding, processResult(0), false); s.Reason != "unsupported_declaration" {
		t.Fatalf("another adapter's declaration: %+v", s)
	}
	plain := ji.CommandArgv(nil)
	if gi := prepare(d); gi.AdapterEnv() != nil || !reflect.DeepEqual(gi.CommandArgv([]string{JUnitPlaceholder}), []string{JUnitPlaceholder}) || plain != nil {
		t.Fatal("a go-test-json run got a JUnit location")
	}
}

func intPtr(v int) *int { return &v }
