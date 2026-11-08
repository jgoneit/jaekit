package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AC-9 of the goal spar-agy-readonly: the round and input contracts (ko,
// en), the Spar guide (ko, en) and the Spar skill state the agy command rule
// and agy's command note with fixed phrases, and the input contract lists
// the G11 representative forms of internal/spar/testdata/input-forms.json.
// The opening line of an assertion's failure message names the property it
// observes (jaekit:violation=<id>); a failure that starts with "setup:" is a
// broken setup, not an observation.

const arDocsViolation = "agy-command-rules-undocumented"

func arRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// arSection is the text from the heading line that starts with start up to
// the next heading of the same or a higher level; "" when there is none.
func arSection(text, start string) string {
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

// arRow is the line of a Markdown table whose first cell is id.
func arRow(text, id string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "| "+id+" |") {
			return line
		}
	}
	return ""
}

type arPlace struct {
	label, text   string
	want, without []string
}

func arCheck(t *testing.T, places []arPlace) {
	t.Helper()
	for _, p := range places {
		if p.text == "" {
			t.Errorf("jaekit:violation=%s %s is missing", arDocsViolation, p.label)
			continue
		}
		for _, w := range p.want {
			if !strings.Contains(p.text, w) {
				t.Errorf("jaekit:violation=%s %s does not say %q", arDocsViolation, p.label, w)
			}
		}
		for _, w := range p.without {
			if strings.Contains(p.text, w) {
				t.Errorf("jaekit:violation=%s %s still says %q", arDocsViolation, p.label, w)
			}
		}
	}
}

func TestSparAgyReadonlyAC9_Documents(t *testing.T) {
	ops := []string{"`&&`", "`||`", "`;`", "`true`"}
	roundKo, roundEn := arRead(t, "../contracts/review-round.md"), arRead(t, "../contracts/review-round.en.md")
	inputKo, inputEn := arRead(t, "../contracts/spar-input.md"), arRead(t, "../contracts/spar-input.en.md")
	arCheck(t, []arPlace{
		{"review-round.md section 2", arSection(roundKo, "## 2."), []string{"패킷 앞에 명령 안내", "`packet.md`는 그대로"}, nil},
		{"review-round.en.md section 2", arSection(roundEn, "## 2."), []string{"command note before the packet", "`packet.md` stays the same"}, nil},
		{"review-round.md section 4", arSection(roundKo, "## 4."), append([]string{"명령 안내", "clean-review 0.2.1과 달리"}, ops...),
			[]string{"이 보호는 clean-review 0.2.1과 같다", "`|` 밖의 셸 연산자"}},
		{"review-round.en.md section 4", arSection(roundEn, "## 4."), append([]string{"command note", "Unlike clean-review 0.2.1"}, ops...),
			[]string{"This protection is the same as clean-review 0.2.1", "shell operators other than `|`"}},
		{"the G11 row of spar-input.md", arRow(inputKo, "G11"), append([]string{"2>/dev/null", "2>&1"}, ops...),
			[]string{"연산자는 `\\|`만", "clean-review 0.2.1과 같다"}},
		{"the G11 row of spar-input.en.md", arRow(inputEn, "G11"), append([]string{"2>/dev/null", "2>&1"}, ops...),
			[]string{"the only operator is `\\|`", "the same as clean-review 0.2.1"}},
		{"guides/SPAR.md 리뷰어 설정", arSection(arRead(t, "../guides/SPAR.md"), "## 리뷰어 설정"), []string{"`&&`", "`||`", "`tool_violation`"}, nil},
		{"guides/SPAR.en.md Configure reviewers", arSection(arRead(t, "../guides/SPAR.en.md"), "## Configure reviewers"), []string{"`&&`", "`||`", "`tool_violation`"}, nil},
		{"the Spar skill's Reviewers section", arSection(arRead(t, "spar/skills/spar/SKILL.md"), "## Reviewers"), []string{"`&&`", "`||`", "command note"}, nil},
	})

	// Section 6 lists each G11 form of the fixture the Go checks read; a line
	// with a newline is stated in prose, since a list item cannot hold it.
	data, err := os.ReadFile(filepath.FromSlash("../internal/spar/testdata/input-forms.json"))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	var doc struct {
		Agy struct {
			Accepted, Kept []string
			Violations     []struct{ Line string }
		} `json:"agy_commands"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("setup: input-forms.json: %v", err)
	}
	lines := append(append([]string{}, doc.Agy.Accepted...), doc.Agy.Kept...)
	for _, v := range doc.Agy.Violations {
		lines = append(lines, v.Line)
	}
	if len(doc.Agy.Accepted) == 0 || len(doc.Agy.Violations) == 0 {
		t.Fatal("setup: input-forms.json has no agy_commands forms")
	}
	for _, c := range []struct{ label, text string }{
		{"spar-input.md section 6 G11", arSection(arSection(inputKo, "## 6."), "### G11")},
		{"spar-input.en.md section 6 G11", arSection(arSection(inputEn, "## 6."), "### G11")},
	} {
		if c.text == "" {
			t.Errorf("jaekit:violation=%s %s is missing", arDocsViolation, c.label)
			continue
		}
		for _, line := range lines {
			if !strings.Contains(line, "\n") && !strings.Contains(c.text, line) {
				t.Errorf("jaekit:violation=%s %s does not list %q", arDocsViolation, c.label, line)
			}
		}
	}
}
