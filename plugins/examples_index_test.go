package plugins

import (
	"regexp"
	"strings"
	"testing"
)

// The examples index introduces both examples and describes the /3 linkage
// with the release that README and the guides call current, not as a
// development-only feature of an older release.
func TestExamplesIndexDescribesCurrentRelease(t *testing.T) {
	const path = "../examples/README.md"
	text := read(t, path)
	requireIn(t, path, text, "(empty-input/)", "(check-result/)", rel011Tag, "`/3`",
		"예제만으로 완료를 실행할 수 없습니다")
	for _, stale := range []string{"개발 Core", "개발용 Core", "development Core"} {
		if strings.Contains(text, stale) {
			t.Errorf("%s still describes the released /3 linkage as development-only: %q", path, stale)
		}
	}
	for _, m := range regexp.MustCompile(`v\d+\.\d+\.\d+`).FindAllString(text, -1) {
		if m != rel011Tag {
			t.Errorf("%s names %s instead of the current release %s", path, m, rel011Tag)
		}
	}
}
