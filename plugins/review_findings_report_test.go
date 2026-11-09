package plugins

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AC-12 of the goal review-findings-report: the contracts and guides, each in
// Korean and English, describe the gate's finding lists, what the review note
// keeps and refuses, the report items and the round summary's whole claims.
// The check looks for fixed phrases in named sections. A document that
// cannot be read observes nothing and skips, which the adapter reads as
// inconclusive. A failure names the property on its first line
// (jaekit:violation=<id>).

func rfrDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Skipf("setup: %v", err)
	}
	return string(data)
}

// rfrSection is the text from the heading line that starts with start up to
// the next heading of the same or a higher level; "" when there is none.
func rfrSection(text, start string) string {
	level := strings.Index(start, " ")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, start) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if n := strings.Index(lines[j], " "); n > 0 && n <= level && strings.Trim(lines[j][:n], "#") == "" {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

func TestReviewFindingsReportAC12_Documents(t *testing.T) {
	const id = "finding-lists-undocumented"
	// The report items, the same tokens in both languages of the contract.
	items := []string{
		"`review findings deferred: <series>/<round> <id> [<class>] → <to>`",
		"`auto-deferred hardening findings: <n>`",
		"`review findings without a disposition: <series>/<round> <id> [<class>]`",
		"`the review gate's finding lists were not recorded (review note seq <n>)`",
		"`the recorded review finding lists cannot be read (review note seq <n>)`",
		"`보류한 검토 지적: <series>/<round> <id> [<class>] → <to>`",
		"`자동 보류한 hardening 지적: <n>개`",
		"`처리 없는 검토 지적: <series>/<round> <id> [<class>]`",
		"`검토 gate의 지적 목록이 기록되지 않음(review note seq <n>)`",
		"`기록된 검토 지적 목록을 읽을 수 없음(review note seq <n>)`",
	}
	cases := []struct {
		path, section string
		phrases       []string
	}{
		{"../contracts/review-gate.md", "## 3. 파일", []string{"| `deferred_findings` |", "| `unhandled_findings` |", "`series`", "`round`", "`to`"}},
		{"../contracts/review-gate.en.md", "## 3. File", []string{"| `deferred_findings` |", "| `unhandled_findings` |", "`series`", "`round`", "`to`"}},
		{"../contracts/run-record.md", "### 1.3 ", items},
		{"../contracts/run-record.en.md", "### 1.3 ", items},
		{"../contracts/run-record.md", "### 4.8 ", []string{"| `findings` |", "`deferred_findings`", "`unhandled_findings`", "`auto_deferred`"}},
		{"../contracts/run-record.en.md", "### 4.8 ", []string{"| `findings` |", "`deferred_findings`", "`unhandled_findings`", "`auto_deferred`"}},
		{"../contracts/review-round.md", "## 5. ", []string{"지적 표의 `Claim` 칸은 주장 전체를"}},
		{"../contracts/review-round.en.md", "## 5. ", []string{"the `Claim` column of the finding table holds the whole claim"}},
		{"../guides/USAGE.md", "", []string{"보류하거나 처리 없이 남은 검토 지적"}},
		{"../guides/USAGE.en.md", "", []string{"review findings that were deferred or left without a disposition"}},
		{"../guides/SPAR.md", "", []string{"`summary.md`의 지적 표는 주장을 자르지 않고"}},
		{"../guides/SPAR.en.md", "", []string{"The finding table in `summary.md` shows each claim whole"}},
	}
	for _, tc := range cases {
		text := rfrDoc(t, tc.path)
		if tc.section != "" {
			text = rfrSection(text, tc.section)
		}
		for _, p := range tc.phrases {
			if !strings.Contains(text, p) {
				where := tc.path
				if tc.section != "" {
					where += " " + tc.section
				}
				t.Errorf("jaekit:violation=%s %s does not say %s", id, where, p)
			}
		}
	}
}
