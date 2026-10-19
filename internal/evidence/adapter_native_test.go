package evidence

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jgoneit/jaekit/internal/run"
)

// platformCase is one runner output with the classification it must get on
// every operating system and with either line ending (AC-12).
type platformCase struct {
	Case      string            `json:"case"`
	Adapter   string            `json:"adapter"`
	Exit      int               `json:"exit"`
	Violation string            `json:"violation"`
	Test      map[string]string `json:"test"`
	Output    string            `json:"output"`
	Result    string            `json:"result"`
	Reason    string            `json:"reason"`
	Targets   [][]any           `json:"targets"`
}

func (c platformCase) declaration() *Declaration {
	name := &TestName{Package: c.Test["package"], Classname: c.Test["classname"], Name: c.Test["name"]}
	return &Declaration{Schema: AdapterSchema, Adapter: c.Adapter, Targets: []Target{{ID: "t", Violation: c.Violation, Test: name}}}
}

func (c platformCase) observations() []Observation {
	var out []Observation
	for _, item := range c.Targets {
		if len(item) != 5 {
			return nil
		}
		attempt, _ := item[1].(float64)
		text := func(i int) string { s, _ := item[i].(string); return s }
		out = append(out, Observation{Target: text(0), Attempt: int(attempt), Status: text(2), Violation: text(3), Reason: text(4)})
	}
	return out
}

// TestNativeAdapterClassification classifies fixed runner outputs that a
// child process writes, as written and with CRLF line endings: a go-test-json
// output on standard output and a junit-xml file at the location ha names.
// JAEKIT_ADAPTER_PLATFORM_FIXTURE replaces the fixture file.
func TestNativeAdapterClassification(t *testing.T) {
	path := filepath.Join("testdata", "adapter-platform.json")
	if other := os.Getenv("JAEKIT_ADAPTER_PLATFORM_FIXTURE"); other != "" {
		path = other
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []platformCase `json:"cases"`
	}
	if err := json.Unmarshal(data, &fixture); err != nil || len(fixture.Cases) == 0 {
		t.Fatalf("fixture %s: %v", path, err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range fixture.Cases {
		for _, form := range []string{"lf", "crlf"} {
			t.Run(c.Case+"/"+form, func(t *testing.T) {
				output := c.Output
				if form == "crlf" {
					output = strings.ReplaceAll(output, "\n", "\r\n")
				}
				dir := t.TempDir()
				source := filepath.Join(dir, "output")
				if err := os.WriteFile(source, []byte(output), 0o600); err != nil {
					t.Fatal(err)
				}
				d := c.declaration()
				if err := d.Validate(); err != nil {
					t.Fatalf("fixture declaration: %v", err)
				}
				i, err := PrepareAdapter(dir, d)
				if err != nil {
					t.Fatal(err)
				}
				defer i.Close()
				env := append(i.AdapterEnv(), "JAEKIT_ADAPTER_HELPER="+source, "JAEKIT_ADAPTER_HELPER_EXIT="+strconv.Itoa(c.Exit))
				argv := i.CommandArgv([]string{executable, "-test.run=^TestAdapterOutputHelper$"})
				res, err := run.ExecWithStdout(dir, argv, 30*time.Second, filepath.Join(dir, "log"), env, i.Stdout())
				if err != nil {
					t.Fatal(err)
				}
				s := i.Classify(d, strings.Repeat("a", 64), res, false)
				if s.Result != c.Result || s.Reason != c.Reason || !reflect.DeepEqual(s.Targets, c.observations()) {
					t.Fatalf("got %s/%s %+v, want %s/%s %+v", s.Result, s.Reason, s.Targets, c.Result, c.Reason, c.observations())
				}
			})
		}
	}
}

// TestAdapterOutputHelper is the child process of the classification test:
// it copies its source to standard output, or to the JUnit location, and
// exits with the given status. It does nothing in an ordinary test run.
func TestAdapterOutputHelper(t *testing.T) {
	source := os.Getenv("JAEKIT_ADAPTER_HELPER")
	if source == "" {
		return
	}
	data, err := os.ReadFile(source)
	if err != nil {
		os.Exit(90)
	}
	if location := os.Getenv(JUnitVariable); location != "" {
		err = os.WriteFile(location, data, 0o600)
	} else {
		_, err = os.Stdout.Write(data)
	}
	if err != nil {
		os.Exit(91)
	}
	code, _ := strconv.Atoi(os.Getenv("JAEKIT_ADAPTER_HELPER_EXIT"))
	os.Exit(code)
}
