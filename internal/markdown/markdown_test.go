package markdown

import (
	"reflect"
	"testing"
)

const sample = "# Title\n\n## Acceptance Criteria\n- **AC-1** first\n  continued\n- **AC-2** (선택) second\n\n```\n## Not a heading\n- not an item\n```\n\n## 조건표\n| ID | 명령 |\n| --- | --- |\n| AC-1 | `a \\| b` |\n| AC-2 | — |\n\n### Sub\n## Next\nsee [doc](aux.md#part) and [web](https://x.y) and [top](#t)\n"

func TestSectionStopsAtSameLevelAndSkipsFences(t *testing.T) {
	d := Parse(sample)
	lines, ok := d.Section(2, "Acceptance Criteria")
	if !ok {
		t.Fatal("section not found")
	}
	items := Items(lines)
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2: %#v", len(items), items)
	}
	if items[0].Text != "**AC-1** first" || !reflect.DeepEqual(items[0].Body, []string{"continued"}) {
		t.Fatalf("item 0 = %#v", items[0])
	}
	if _, ok := d.Section(2, "Not a heading"); ok {
		t.Fatal("heading inside a fence must be ignored")
	}
	sub, _ := d.Section(2, "조건표")
	if len(Tables(sub)) != 1 {
		t.Fatal("table inside section not found")
	}
}

func TestTablesUnescapePipes(t *testing.T) {
	d := Parse(sample)
	lines, _ := d.Section(2, "조건표")
	tb := Tables(lines)[0]
	if !tb.HeaderIs("ID", "명령") {
		t.Fatalf("header = %#v", tb.Header)
	}
	if got := CodeSpans(tb.Rows[0].Cell(1)); !reflect.DeepEqual(got, []string{"a | b"}) {
		t.Fatalf("code span = %#v", got)
	}
	if !IsNone(tb.Rows[1].Cell(1)) {
		t.Fatal("em dash must read as none")
	}
}

func TestRelativeLinks(t *testing.T) {
	got := Parse(sample).RelativeLinks()
	if !reflect.DeepEqual(got, []string{"aux.md"}) {
		t.Fatalf("links = %#v", got)
	}
}

func TestCodeSpansDoubleBacktick(t *testing.T) {
	got := CodeSpans("run `` a `b` `` and `c`")
	if !reflect.DeepEqual(got, []string{"a `b`", "c"}) {
		t.Fatalf("spans = %#v", got)
	}
}
