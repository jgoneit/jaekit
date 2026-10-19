package evidence

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
	"unicode/utf8"
)

// A check-adapter/v1 declaration links each target to one existing test. ha
// reads the runner's own output instead of a producer report: the standard
// output of `go test -json`, or a JUnit XML file at a location ha names.
const (
	AdapterSchema     = "check-adapter/v1"
	AdapterGoTestJSON = "go-test-json"
	AdapterJUnitXML   = "junit-xml"
	// UnmarkedViolation is the observed violation of a linked test that failed
	// without the same marker on every failure message's first line. Declared
	// violations cannot start with the reserved prefix, so it never equals one.
	UnmarkedViolation = "jaekit:unmarked-failure"
	reservedPrefix    = "jaekit:"
	markerPrefix      = "jaekit:violation="
	// JUnitPlaceholder in a command argument becomes the output location of a
	// junit-xml condition; JUnitVariable carries the same location.
	JUnitPlaceholder = "{HA_JUNIT_XML}"
	JUnitVariable    = "HA_JUNIT_XML"
	maxGoOutput      = 64 << 20
	maxJUnitOutput   = 16 << 20
	maxTestName      = 1024
)

// TestName is the exact name of a linked test: an import path and the full
// test name for go-test-json, a testcase classname and name for junit-xml. An
// empty Classname matches a testcase without one.
type TestName struct {
	Package   string
	Classname string
	Name      string
}

type adapterTestJSON struct {
	Package   *string `json:"package"`
	Classname *string `json:"classname"`
	Name      *string `json:"name"`
}

type adapterTargetJSON struct {
	ID        string           `json:"id"`
	Violation string           `json:"violation"`
	Test      *adapterTestJSON `json:"test"`
}

type adapterDeclarationJSON struct {
	Schema  string              `json:"schema"`
	Adapter string              `json:"adapter"`
	Targets []adapterTargetJSON `json:"targets"`
}

func adapterSchema(data []byte) bool {
	var probe struct {
		Schema string `json:"schema"`
	}
	return json.Unmarshal(data, &probe) == nil && probe.Schema == AdapterSchema
}

// loadAdapter binds the declaration bytes and the adapter's classification
// meaning version, so a changed meaning never reuses earlier evidence.
func loadAdapter(p string, data []byte) (*Declaration, string, error) {
	var raw adapterDeclarationJSON
	if err := strictJSON(data, &raw); err != nil {
		return nil, "", errors.New("invalid adapter declaration JSON")
	}
	if _, ok := adapterSemantics[raw.Adapter]; !ok {
		return nil, "", fmt.Errorf("unknown runner adapter %q", raw.Adapter)
	}
	d := &Declaration{Schema: raw.Schema, Adapter: raw.Adapter}
	for i, t := range raw.Targets {
		target := Target{ID: t.ID, Violation: t.Violation}
		if t.Test == nil {
			return nil, "", fmt.Errorf("adapter target %d has no test", i+1)
		}
		if t.Test.Name == nil {
			return nil, "", fmt.Errorf("adapter target %d has no test name", i+1)
		}
		name := &TestName{Name: *t.Test.Name}
		switch raw.Adapter {
		case AdapterGoTestJSON:
			if t.Test.Package == nil || t.Test.Classname != nil {
				return nil, "", fmt.Errorf("adapter target %d: a go-test-json test has a package and a name", i+1)
			}
			name.Package = *t.Test.Package
		case AdapterJUnitXML:
			if t.Test.Classname == nil || t.Test.Package != nil {
				return nil, "", fmt.Errorf("adapter target %d: a junit-xml test has a classname and a name", i+1)
			}
			name.Classname = *t.Test.Classname
		}
		target.Test = name
		d.Targets = append(d.Targets, target)
	}
	if err := d.Validate(); err != nil {
		return nil, "", err
	}
	var binding strings.Builder
	fmt.Fprintf(&binding, "%s\x00%s\n", p, digest(data))
	fmt.Fprintf(&binding, "\x00adapter\x00%s\x00%s\n", d.Adapter, adapterSemantics[d.Adapter])
	return d, digest([]byte(binding.String())), nil
}

func (d *Declaration) validateAdapter() error {
	if _, ok := adapterSemantics[d.Adapter]; !ok {
		return fmt.Errorf("unknown runner adapter %q", d.Adapter)
	}
	if d.Producer != "" || d.ProducerVersion != "" || len(d.ProducerPaths) != 0 {
		return errors.New("an adapter declaration names no producer")
	}
	if len(d.Targets) == 0 || len(d.Targets) > 1024 {
		return errors.New("adapter declaration needs one to 1024 targets")
	}
	ids, tests := map[string]bool{}, map[string]bool{}
	for i, t := range d.Targets {
		if !identifier.MatchString(t.ID) || ids[t.ID] {
			return fmt.Errorf("adapter target %d has an invalid or duplicate id", i+1)
		}
		ids[t.ID] = true
		if !identifier.MatchString(t.Violation) {
			return fmt.Errorf("adapter target %q has an invalid violation", t.ID)
		}
		if strings.HasPrefix(t.Violation, reservedPrefix) {
			return fmt.Errorf("adapter target %q: violations starting with %q are reserved", t.ID, reservedPrefix)
		}
		if t.Test == nil {
			return fmt.Errorf("adapter target %q has no test", t.ID)
		}
		if err := t.Test.validate(d.Adapter); err != nil {
			return fmt.Errorf("adapter target %q: %v", t.ID, err)
		}
		key := t.Test.key()
		if tests[key] {
			return fmt.Errorf("adapter target %q links a test another target links", t.ID)
		}
		tests[key] = true
	}
	return nil
}

func (n *TestName) key() string { return n.Package + "\x00" + n.Classname + "\x00" + n.Name }

func (n *TestName) validate(adapter string) error {
	if n.Name == "" {
		return errors.New("empty test name")
	}
	for _, part := range []string{n.Package, n.Classname, n.Name} {
		if len(part) > maxTestName {
			return fmt.Errorf("test name over %d bytes", maxTestName)
		}
		if hasControl(part) {
			return errors.New("test name with a control character")
		}
	}
	switch adapter {
	case AdapterGoTestJSON:
		if n.Classname != "" {
			return errors.New("a go-test-json test has no classname")
		}
		if n.Package == "" || strings.IndexFunc(n.Package, unicode.IsSpace) >= 0 {
			return errors.New("invalid Go package path")
		}
		if !goTestName(n.Name) {
			return errors.New("not a Go test name (Test, Fuzz or Example followed by a non-lowercase letter, subtests after /)")
		}
	case AdapterJUnitXML:
		if n.Package != "" {
			return errors.New("a junit-xml test has no package")
		}
	}
	return nil
}

func hasControl(s string) bool {
	for _, r := range s {
		if r < 0x20 || (r >= 0x7f && r <= 0x9f) {
			return true
		}
	}
	return false
}

// goTestName accepts the names go test -json reports: a top-level test,
// fuzz test or example function, then subtest names after slashes. Go
// replaces spaces in subtest names, so a reported name never has one.
func goTestName(name string) bool {
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if part == "" || strings.IndexFunc(part, unicode.IsSpace) >= 0 {
			return false
		}
	}
	first := parts[0]
	for i, r := range first {
		if !(unicode.IsLetter(r) || r == '_' || (i > 0 && unicode.IsDigit(r))) {
			return false
		}
	}
	for _, prefix := range []string{"Test", "Fuzz", "Example"} {
		if rest, ok := strings.CutPrefix(first, prefix); ok {
			r, _ := utf8.DecodeRuneInString(rest)
			return rest == "" || !unicode.IsLower(r)
		}
	}
	return false
}

// lineMarkers returns the identifier of every marker on one line. A marker is
// the longest run of identifier characters right after markerPrefix; a run
// that is empty, longer than 128 characters or does not start with an ASCII
// letter or digit is not a marker.
func lineMarkers(line string) []string {
	var ids []string
	for rest := line; ; {
		i := strings.Index(rest, markerPrefix)
		if i < 0 {
			return ids
		}
		rest = rest[i+len(markerPrefix):]
		n := 0
		for n < len(rest) && markerByte(rest[n]) {
			n++
		}
		if n >= 1 && n <= 128 && alnum(rest[0]) {
			ids = append(ids, rest[:n])
		}
		rest = rest[n:]
	}
}

func markerByte(b byte) bool {
	return alnum(b) || b == '_' || b == '.' || b == ':' || b == '/' || b == '-'
}

func alnum(b byte) bool { return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' }

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSuffix(s, "\r")
}

// failedViolation is the observed violation of a failed test from the first
// lines of its failure messages: the marker identifier when every line has a
// marker and all markers name the same identifier, otherwise unmarked.
func failedViolation(firstLines []string) string {
	if len(firstLines) == 0 {
		return UnmarkedViolation
	}
	seen := ""
	for _, line := range firstLines {
		ids := lineMarkers(line)
		if len(ids) == 0 {
			return UnmarkedViolation
		}
		for _, id := range ids {
			if seen != "" && id != seen {
				return UnmarkedViolation
			}
			seen = id
		}
	}
	return seen
}

// capture keeps up to limit bytes of a process's standard output. It accepts
// every write so the process never sees a write error; over records that the
// output did not fit.
type capture struct {
	limit int
	data  []byte
	over  bool
}

func (c *capture) Write(p []byte) (int, error) {
	if !c.over && len(c.data)+len(p) <= c.limit {
		c.data = append(c.data, p...)
	} else {
		c.over, c.data = true, nil
	}
	return len(p), nil
}

// PrepareAdapter reserves a fresh private location for one adapter run. A
// junit-xml run's file goes into a new empty directory there; a go-test-json
// run's standard output is kept in memory.
func PrepareAdapter(parent string, d *Declaration) (*Invocation, error) {
	if d == nil || d.Adapter == "" {
		return nil, errors.New("not an adapter declaration")
	}
	dir, err := os.MkdirTemp(parent, ".evidence-")
	if err != nil {
		return nil, err
	}
	var nonce [32]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	i := &Invocation{ID: hex.EncodeToString(nonce[:]), dir: dir, adapter: d.Adapter}
	switch d.Adapter {
	case AdapterGoTestJSON:
		i.stdout = &capture{limit: maxGoOutput}
	case AdapterJUnitXML:
		i.Path = filepath.Join(dir, "junit.xml")
	}
	return i, nil
}

// AdapterEnv names the output location of a junit-xml run.
func (i *Invocation) AdapterEnv() []string {
	if i.adapter == AdapterJUnitXML {
		return []string{JUnitVariable + "=" + i.Path}
	}
	return nil
}

// CommandArgv is the argv ha runs. Only a junit-xml run replaces the
// placeholder, also inside a longer argument; the record keeps the plan's argv.
func (i *Invocation) CommandArgv(argv []string) []string {
	out := append([]string(nil), argv...)
	if i.adapter == AdapterJUnitXML {
		for n, arg := range out {
			out[n] = strings.ReplaceAll(arg, JUnitPlaceholder, i.Path)
		}
	}
	return out
}

// Stdout receives a copy of a go-test-json run's standard output.
func (i *Invocation) Stdout() io.Writer {
	if i.stdout == nil {
		return nil
	}
	return i.stdout
}

// Notices are fixed explanations ha prints after the result line. They name
// declared values only, never runner output.
func (i *Invocation) Notices() []string { return i.notices }

// adapterOutput reads the runner output, or returns the reason it is absent
// or too large. Nothing is judged from a cut prefix.
func (i *Invocation) adapterOutput() ([]byte, string) {
	switch i.adapter {
	case AdapterGoTestJSON:
		if i.stdout == nil {
			return nil, "output_missing"
		}
		if i.stdout.over {
			return nil, "output_too_large"
		}
		if len(i.stdout.data) == 0 {
			return nil, "output_missing"
		}
		return i.stdout.data, ""
	case AdapterJUnitXML:
		info, err := os.Lstat(i.Path)
		if err != nil || !info.Mode().IsRegular() {
			return nil, "output_missing"
		}
		if info.Size() > maxJUnitOutput {
			return nil, "output_too_large"
		}
		f, err := os.Open(i.Path)
		if err != nil {
			return nil, "output_missing"
		}
		defer f.Close()
		data, err := io.ReadAll(io.LimitReader(f, maxJUnitOutput+1))
		if err != nil {
			return nil, "output_missing"
		}
		if len(data) > maxJUnitOutput {
			return nil, "output_too_large"
		}
		return data, ""
	}
	return nil, "output_missing"
}

// earlyPrecedence picks one per-target reason when targets disagree.
func earlyPrecedence(reasons map[string]bool) string {
	for _, reason := range []string{"attempt_history_missing", "observation_invalid", "target_missing"} {
		if reasons[reason] {
			return reason
		}
	}
	return ""
}
