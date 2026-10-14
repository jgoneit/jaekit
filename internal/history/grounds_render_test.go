package history

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// groundsFixture includes corrections before, within, and after a dependent
// judgment, as well as corrections outside the selected run lineage.
func groundsFixture(t *testing.T) (*Store, []string, string) {
	t.Helper()
	s := newStore(t)
	runs := runHashes(4, "grounds")
	diverged := append(append([]string{}, runs[:2]...), runHashes(3, "other")[2:]...)
	appendAt := func(request string, chain []string, d Decision, recorded int64) {
		t.Helper()
		if _, err := s.Append(time.Unix(recorded, 0), "test", func(events []Event) (*Event, error) {
			return Prepare(d, request, events, facts(chain))
		}); err != nil {
			t.Fatalf("append %s: %v", request, err)
		}
	}
	appendAt("ground", runs[:2], Decision{Kind: "원인", Summary: "original ground"}, 1700000000) // D1
	appendAt("before", runs[:3], Decision{Kind: "원인", Summary: "earlier correction", EventAt: "2030-01-01T00:00:00Z",
		Supersedes: []Supersede{{ID: "D1"}}}, 1700000030) // D2
	since := query(t, s, runs[:3], Query{Mode: "current"}).Cursor
	appendAt("dependent", runs[:3], Decision{Kind: "결정", Summary: "chosen dependency", Targets: []string{"T001"},
		BuildsOn: []string{"D1"}, EventAt: "2020-01-01T00:00:00Z"}, 1700000020) // D3
	appendAt("self-correcting", runs[:3], Decision{Kind: "결정", Summary: "chosen revision", Targets: []string{"T001"},
		BuildsOn: []string{"D1"}, Supersedes: []Supersede{{ID: "D1", After: "D2"}}}, 1700000040) // D4
	appendAt("after", runs, Decision{Kind: "원인", Summary: "later correction", EventAt: "2010-01-01T00:00:00Z",
		Supersedes: []Supersede{{ID: "D1", After: "D4"}}}, 1700000010) // D5
	appendAt("different", diverged, Decision{Kind: "원인", Summary: "another lineage",
		Supersedes: []Supersede{{ID: "D1", After: "D5"}}}, 1700000050) // D6
	appendAt("unverifiable", runHashes(5, "grounds"), Decision{Kind: "원인", Summary: "beyond selected run head",
		Supersedes: []Supersede{{ID: "D1", After: "D6"}}}, 1700000060) // D7
	return s, runs, since
}

func assertGrounds(t *testing.T, v *View, id string, by ...string) {
	t.Helper()
	var item *Item
	if v.Event != nil && v.Event.ID == id {
		item = &v.Event.Item
	} else {
		for i := range v.Items {
			if v.Items[i].ID == id {
				item = &v.Items[i]
			}
		}
	}
	if item == nil {
		t.Fatalf("%s is absent from %s: %+v", id, v.Mode, v)
	}
	want := []Grounds{{ID: "D1", By: by}}
	if !reflect.DeepEqual(item.GroundsCorrected, want) || !item.Current {
		t.Fatalf("%s grounds/current: %+v, want %+v and current", id, item, want)
	}
	marker := "linked grounds D1 corrected by " + strings.Join(by, ", ")
	md := v.Markdown()
	if !strings.Contains(md, marker) || strings.Contains(md, "later corrected") {
		t.Fatalf("%s must use a time-neutral marker %q:\n%s", id, marker, md)
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var wire struct {
		Items []struct {
			ID     string          `json:"id"`
			Ground json.RawMessage `json:"grounds_corrected"`
		} `json:"items"`
		Event *struct {
			ID     string          `json:"id"`
			Ground json.RawMessage `json:"grounds_corrected"`
		} `json:"event"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		t.Fatal(err)
	}
	var raw json.RawMessage
	if wire.Event != nil && wire.Event.ID == id {
		raw = wire.Event.Ground
	} else {
		for _, it := range wire.Items {
			if it.ID == id {
				raw = it.Ground
			}
		}
	}
	wantJSON := `[{"id":"D1","by":["` + strings.Join(by, `","`) + `"]}]`
	if string(raw) != wantJSON {
		t.Fatalf("%s JSON grounds: %s, want %s", id, raw, wantJSON)
	}
}

func TestGroundsCorrectionsPreserveEarlierSameAndLaterEvents(t *testing.T) {
	s, runs, since := groundsFixture(t)
	before, err := os.ReadFile(s.file())
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		q    Query
		ids  []string
	}{
		{"current", Query{Mode: "current", Targets: []string{"T001"}, Keywords: []string{"chosen"}}, []string{"D3", "D4"}},
		{"list", Query{Mode: "list", Targets: []string{"T001"}, Keywords: []string{"chosen"}}, []string{"D3", "D4"}},
		{"since", Query{Mode: "current", Targets: []string{"T001"}, Keywords: []string{"chosen"}, Since: since}, []string{"D3", "D4"}},
		{"show ID", Query{Mode: "show", ID: "D3"}, []string{"D3"}},
		{"show request", Query{Mode: "show", RequestID: "dependent"}, []string{"D3"}},
		{"show same-event correction", Query{Mode: "show", ID: "D4"}, []string{"D4"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			v := query(t, s, runs, tc.q)
			if v.Event == nil && ids(v.Items) != strings.Join(tc.ids, " ") {
				t.Fatalf("query changed the selected judgments: %s", ids(v.Items))
			}
			for _, id := range tc.ids {
				assertGrounds(t, v, id, "D2", "D4", "D5")
			}
		})
	}
	after, err := os.ReadFile(s.file())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("query changed stored events: %v", err)
	}
}

func TestGroundsCorrectionsRespectSelectedLineage(t *testing.T) {
	s, runs, _ := groundsFixture(t)
	assertGrounds(t, query(t, s, runs, Query{Mode: "show", ID: "D3"}), "D3", "D2", "D4", "D5")
	// Advancing the selected run head confirms D7 without including D6 from
	// the other chain. Corrections remain in registration order.
	assertGrounds(t, query(t, s, runHashes(5, "grounds"), Query{Mode: "show", ID: "D3"}), "D3", "D2", "D4", "D5", "D7")
}

func TestGroundsCorrectionPagesKeepOriginalBoundary(t *testing.T) {
	s, runs, _ := groundsFixture(t)
	q := Query{Mode: "current", Targets: []string{"T001"}, Keywords: []string{"chosen"}, Limit: 1}
	first := query(t, s, runs, q)
	if first.Next == nil || ids(first.Items) != "D3" {
		t.Fatalf("first page: %+v", first)
	}
	assertGrounds(t, first, "D3", "D2", "D4", "D5")
	mustAdd(t, s, facts(runs), "new-correction", Decision{Kind: "원인", Summary: "new correction",
		Supersedes: []Supersede{{ID: "D1", After: "D7"}}}) // D8
	q.Page = *first.Next
	second := query(t, s, runs, q)
	if second.Next != nil || second.Events != first.Events || ids(second.Items) != "D4" {
		t.Fatalf("second page changed the pinned selection: %+v", second)
	}
	assertGrounds(t, second, "D4", "D2", "D4", "D5")
	q.Page, q.Limit = "", 20
	fresh := query(t, s, runs, q)
	if ids(fresh.Items) != "D3 D4" {
		t.Fatalf("new ground correction changed current judgments: %s", ids(fresh.Items))
	}
	for _, id := range []string{"D3", "D4"} {
		assertGrounds(t, fresh, id, "D2", "D4", "D5", "D8")
	}
}
