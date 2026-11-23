package plugins

import (
	"strconv"
	"strings"
	"testing"
)

func forbidAll(t *testing.T, path string, nots ...string) {
	t.Helper()
	text := read(t, path)
	for _, s := range nots {
		if strings.Contains(text, s) {
			t.Errorf("%s still says %q", path, s)
		}
	}
}

// once the writing target is settled, Spec writes or edits the goal
// documents with Status: Draft while a decision is open: that stays a format
// token of the Spec skill.
func TestSpecWritesWithoutSaveAgreement(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Status: Draft")
}

// an answer or a change request edits the same goal documents, which turn
// Status: Ready once no outcome-changing decision is open: that token stays a
// format token of the Spec skill.
func TestSpecRevisesSameDocuments(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Status: Ready")
}

// Draft means an outcome-changing decision is open, Ready means none is.
// Status: Draft and Status: Ready stay format tokens of the Spec skill.
func TestSpecStatusGrantsNothing(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Status: Draft", "Status: Ready")
}

// a continue or implement request sent once the goal documents changed during
// a run confirms that change: the confirm spec note form stays a format token
// of the Seal skill.
func TestSealTakesContinueAsConfirmation(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, `ha note <goal> confirm spec --quote "…"`)
}

// the README links the usage guide, which names the Seal command that starts
// implementation.
func TestGuidesDescribeDirectSave(t *testing.T) {
	requireAll(t, "../README.ko.md", "(guides/USAGE.md)")
	requireAll(t, "../guides/USAGE.md", "$seal:seal docs/specs/<goal>")
}

// a quality case holds one outcome-changing open decision to ask; the public
// case list includes it.
func TestSpecCaseForDirectSave(t *testing.T) {
	path := "../evals/spec-cases/draft-then-ready.md"
	_, rest, ok := strings.Cut(read(t, path), "\n- 물어야 할 것\n")
	if !ok {
		t.Fatal("draft-then-ready.md has no 물어야 할 것 list")
	}
	asks := 0
	for _, line := range strings.Split(rest, "\n") {
		if !strings.HasPrefix(line, "  - ") {
			break
		}
		asks++
	}
	if asks != 1 {
		t.Errorf("draft-then-ready.md lists %d decisions to ask, want 1", asks)
	}
	requireAll(t, "../evals/spec-cases/README.md", "[draft-then-ready](draft-then-ready.md)")
}

// Spec is above 0.1.8 and Seal above 0.1.3, and each
// plugin's skill, both manifests, and the marketplace entry agree.
func TestPluginVersionsRaisedForDirectSave(t *testing.T) {
	var market struct {
		Plugins []struct{ Name, Version string } `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &market)
	for _, c := range []struct {
		name  string
		floor [3]int
	}{{"spec", [3]int{0, 1, 8}}, {"seal", [3]int{0, 1, 3}}} {
		_, version := frontMatter(t, c.name+"/skills/"+c.name+"/SKILL.md")
		var claude, codex manifest
		readJSON(t, c.name+"/.claude-plugin/plugin.json", &claude)
		readJSON(t, c.name+"/.codex-plugin/plugin.json", &codex)
		marketVersion := ""
		for _, p := range market.Plugins {
			if p.Name == c.name {
				marketVersion = p.Version
			}
		}
		if claude.Version != version || codex.Version != version || marketVersion != version {
			t.Errorf("%s versions disagree: skill %q, claude %q, codex %q, marketplace %q", c.name, version, claude.Version, codex.Version, marketVersion)
		}
		parts := strings.Split(version, ".")
		if len(parts) != 3 {
			t.Errorf("%s version %q is not major.minor.patch", c.name, version)
			continue
		}
		var n [3]int
		for i, p := range parts {
			v, err := strconv.Atoi(p)
			if err != nil {
				t.Fatalf("%s version %q: %v", c.name, version, err)
			}
			n[i] = v
		}
		above := false
		for i := range n {
			if n[i] != c.floor[i] {
				above = n[i] > c.floor[i]
				break
			}
		}
		if !above {
			t.Errorf("%s version %s is not above %d.%d.%d", c.name, version, c.floor[0], c.floor[1], c.floor[2])
		}
	}
}

// once the writing target is settled, Spec writes the documents with
// Status: Draft while a content decision is open: that stays a format token
// of the Spec skill.
func TestSpecAsksTargetBeforeWriting(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Status: Draft")
}

// a quality case asks one decision before writing and one after the writing
// target is set; it is listed in the public case list.
func TestSpecCaseForTargetChoice(t *testing.T) {
	path := "../evals/spec-cases/target-then-draft.md"
	text := read(t, path)
	_, rest, ok := strings.Cut(text, "\n- 물어야 할 것\n")
	if !ok {
		t.Fatal("target-then-draft.md has no 물어야 할 것 list")
	}
	before, after := 0, 0
	for _, line := range strings.Split(rest, "\n") {
		if !strings.HasPrefix(line, "  - ") {
			break
		}
		switch {
		case strings.HasPrefix(line, "  - 쓰기 전: "):
			before++
		case strings.HasPrefix(line, "  - 대상이 정해진 뒤: "):
			after++
		}
	}
	if before != 1 || after != 1 {
		t.Errorf("target-then-draft.md asks %d before writing and %d after the target, want 1 and 1", before, after)
	}
	requireAll(t, "../evals/spec-cases/README.md", "[target-then-draft](target-then-draft.md)")
}
