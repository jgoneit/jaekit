package plugins

import (
	"strings"
	"testing"
)

// The README's completion report example follows the Seal that the release
// ships: since Seal 0.1.20 the report is `ha report` output pasted after
// `ha done`, never `ha status` or `ha done` output. The check reads fixed
// tokens between the section heading and the next one in each README.
func TestReadmeCompletionReportUsesHaReport(t *testing.T) {
	for _, c := range []struct {
		path, heading, next string
		labels              []string
	}{
		{"../README.md", "### The completion report\n", "\n### The other files\n",
			[]string{"Changed: ", "Checked: ", "To review: ", "Remaining limits: ", "Next step: ", "### Evidence per condition", "### Output of unmet conditions"}},
		{"../README.ko.md", "### 완료 보고\n", "\n### 다른 파일\n",
			[]string{"바뀐 것: ", "확인한 것: ", "직접 볼 것: ", "남은 한계: ", "다음 행동: ", "### 조건별 근거", "### 미충족 조건의 원문 위치"}},
	} {
		text := visualRead(t, c.path)
		start := strings.Index(text, c.heading)
		if start < 0 {
			t.Fatalf("%s lacks %q", c.path, strings.TrimSpace(c.heading))
		}
		end := strings.Index(text[start:], c.next)
		if end < 0 {
			t.Fatalf("%s lacks %q after the completion report", c.path, strings.TrimSpace(c.next))
		}
		section := text[start : start+end]
		for _, want := range append([]string{"`ha report`"}, c.labels...) {
			if !strings.Contains(section, want) {
				t.Errorf("%s completion report lacks %q", c.path, want)
			}
		}
		for _, stale := range []string{"ha status: docs/specs/", "### Unmet", "skill: seal 0.1.17", "ha: 0.1.6"} {
			if strings.Contains(section, stale) {
				t.Errorf("%s completion report still shows %q", c.path, stale)
			}
		}
	}
}
