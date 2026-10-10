package status

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The computation must not depend on the current time (run-record.md §4).
func TestComputationNeverReadsTheClock(t *testing.T) {
	files, _ := filepath.Glob("*.go")
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(src), "time.Now") || strings.Contains(string(src), "time.Since") {
			t.Errorf("%s reads the current time", f)
		}
	}
}

func TestDecideOrder(t *testing.T) {
	r := func(code string) Reason { return Reason{Code: code, Class: classOf[code]} }
	cases := []struct {
		reasons []Reason
		want    string
	}{
		{nil, Complete},
		{[]Reason{r("spec_changed"), r("blocked"), r("budget_exceeded")}, Blocked},
		{[]Reason{r("criterion_missing"), r("budget_exceeded")}, BudgetExhausted},
		{[]Reason{r("out_of_scope"), r("worktree_dirty")}, Incomplete},
		{[]Reason{r("out_of_scope"), r("manual_unconfirmed")}, NeedsUser},
	}
	for _, c := range cases {
		if got := decide(c.reasons); got != c.want {
			t.Errorf("decide(%v) = %s, want %s", c.reasons, got, c.want)
		}
	}
	for code, class := range classOf {
		if class == "" {
			t.Errorf("%s has no class", code)
		}
	}
}

// documentedFields reads the field names of the table that follows heading
// in the run-record contract.
func documentedFields(t *testing.T, heading string) []string {
	t.Helper()
	src, err := os.ReadFile(filepath.Join("..", "..", "contracts", "run-record.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(src)
	i := strings.Index(text, heading)
	if i < 0 {
		t.Fatalf("contract has no %q", heading)
	}
	rest := text[i+len(heading):]
	if j := strings.Index(rest, "\n#"); j >= 0 {
		rest = rest[:j]
	}
	var fields []string
	row := regexp.MustCompile("(?m)^\\| `([a-z_]+)` \\|")
	for _, m := range row.FindAllStringSubmatch(rest, -1) {
		fields = append(fields, m[1])
	}
	sort.Strings(fields)
	return fields
}

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// AC-19: the JSON output follows the fields documented in the contract.
func TestJSONMatchesContract(t *testing.T) {
	seq := 3
	rep := &Report{CompletionRecord: &seq, RecordHead: &Head{}, Budget: &Budget{}}
	cases := []struct {
		heading string
		value   any
	}{
		{"#### 상태 문서 (`status/v1`)", rep},
		{"#### 조건 (`criteria[]`)", Criterion{}},
		{"#### 시도 (`attempts`)", Attempts{}},
		{"#### 이유 (`reasons[]`)", Reason{Criterion: "AC-1", Paths: []string{"x"}, Detail: "d"}},
		{"#### 변화 (`changes`)", Changes{}},
		{"#### 예산 (`budget`)", Budget{}},
	}
	for _, c := range cases {
		doc := documentedFields(t, c.heading)
		got := jsonKeys(t, c.value)
		if strings.Join(doc, ",") != strings.Join(got, ",") {
			t.Errorf("%s\n contract: %v\n output:   %v", c.heading, doc, got)
		}
	}
}
