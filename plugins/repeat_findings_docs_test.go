package plugins

// Checks of the review-repeat-findings goal on documents. Each test observes
// fixed tokens (flags, field names, setting keys, fixed phrases) in named
// documents; whether the sentences around them say the same thing is for a
// person to read. A document that cannot be read fails the test as a broken
// setup, so that an input failure is never a pass.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rptDoc(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	return string(data)
}

// rptRequire fails with id for every document that lacks one of its tokens.
func rptRequire(t *testing.T, id string, tokens map[string][]string) {
	t.Helper()
	for path, want := range tokens {
		text := rptDoc(t, path)
		for _, token := range want {
			if !strings.Contains(text, token) {
				t.Errorf("jaekit:violation=%s %s does not have %q", id, path, token)
			}
		}
	}
}

const (
	rptSealSkill  = "seal/skills/seal/SKILL.md"
	rptSealBundle = "seal/skills/seal/references/bundle.md"
	rptSealReview = "seal/skills/seal/assets/templates/REVIEW.md"
	rptSealHa     = "seal/skills/seal/references/ha.md"
	rptBundle     = "../contracts/bundle.md"
)

// AC-7: Seal writes the supported forms and check boundaries before the
// first review round of every tier, and asks for the forms of an open-ended
// criterion in the question before any baseline.
func TestRepeatFindingsAC7_FirstRoundBoundaries(t *testing.T) {
	rptRequire(t, "first-round-rule-missing", map[string][]string{
		rptSealSkill:  {"before the earliest review round", "each check's boundary", "ask in the question before any baseline"},
		rptSealBundle: {"before the earliest review round", "check boundaries", "supported input forms of an open-ended criterion", "in the one question before any baseline"},
		rptSealReview: {"before the earliest review round", "each check's boundary"},
		rptBundle:     {"before the earliest review round", "each check's boundary", "supported input forms of an open-ended criterion", "in the one question before any baseline"},
	})
}

// AC-7: a review fix that widens a check widens that check's boundary record
// in the same step.
func TestRepeatFindingsAC7_WidenedCheckRecords(t *testing.T) {
	rptRequire(t, "widened-check-rule-missing", map[string][]string{
		rptSealSkill:  {"widen its boundary record in the same step"},
		rptSealBundle: {"widen its boundary record in the same step"},
		rptSealReview: {"widen its boundary record in the same step"},
		rptBundle:     {"widen its boundary record in the same step"},
	})
}

// AC-7: every round, from the plan round to the cumulative round, gets
// REVIEW.md as a boundary document, and the round example shows it.
func TestRepeatFindingsAC7_BoundaryEveryRound(t *testing.T) {
	const id = "boundary-passing-rule-missing"
	rptRequire(t, id, map[string][]string{
		rptSealSkill:  {"`--boundary`"},
		rptSealBundle: {"as `--boundary` to every review round"},
		rptBundle:     {"as `--boundary` to every review round"},
	})
	text := rptDoc(t, rptSealBundle)
	i := strings.Index(text, "**Decisions for review rounds.**")
	if i < 0 {
		t.Skipf("setup: %s has no example for review rounds", rptSealBundle)
	}
	block := text[i:]
	if j := strings.Index(block, "```bash\n"); j >= 0 {
		block = block[j:]
		if k := strings.Index(block[len("```bash\n"):], "```"); k >= 0 {
			block = block[:len("```bash\n")+k]
		}
	}
	if !strings.Contains(block, "--boundary docs/specs/<goal>/REVIEW.md") {
		t.Errorf("jaekit:violation=%s the round example of %s does not pass --boundary docs/specs/<goal>/REVIEW.md", id, rptSealBundle)
	}
}

// rptSparDocs are the documents that describe Spar's inputs, packet, records
// and settings.
var rptSparDocs = []string{"../contracts/review-round.md", "../contracts/spar-input.md", "../contracts/spar-config.md",
	"../guides/SPAR.md", "../guides/SPAR.en.md", "spar/skills/spar/SKILL.md"}

// AC-9: the documents describe the last disposition at the end of an
// earlier finding's line and the guidance for a finding reported again.
func TestRepeatFindingsAC9_DispositionDocs(t *testing.T) {
	tokens := map[string][]string{
		"../contracts/review-round.md": {"line ends with its last disposition", "a failure scenario that the recorded disposition does not answer"},
		"../guides/SPAR.md":            {"줄 끝에 그 지적의 마지막 처리", "기록된 처리가 답하지 못하는 실패 시나리오"},
		"../contracts/spar-input.md":   {"line ends with its last disposition"},
		"../guides/SPAR.en.md":         {"line ends with its last disposition", "a failure scenario that the recorded disposition does not answer"},
		"spar/skills/spar/SKILL.md":    {"line ends with its last disposition", "a failure scenario that the recorded disposition does not answer"},
	}
	rptRequire(t, "disposition-undocumented", tokens)
}

// AC-9: the documents describe the boundary input, its section and guidance,
// and its record field.
func TestRepeatFindingsAC9_BoundaryDocs(t *testing.T) {
	tokens := map[string][]string{}
	for _, path := range rptSparDocs {
		if strings.Contains(path, "spar-config") {
			continue
		}
		tokens[path] = []string{"`--boundary`"}
	}
	tokens["../contracts/review-round.md"] = append(tokens["../contracts/review-round.md"], "`boundaries`", "`check_impact` `none`")
	for _, path := range []string{"../guides/SPAR.md", "../guides/SPAR.en.md", "spar/skills/spar/SKILL.md"} {
		tokens[path] = append(tokens[path], "`check_impact` `none`")
	}
	rptRequire(t, "boundary-undocumented", tokens)
}

// AC-9: the documents describe the packet size limit, its refusal and its
// setting.
func TestRepeatFindingsAC9_LimitDocs(t *testing.T) {
	tokens := map[string][]string{}
	for _, path := range rptSparDocs {
		tokens[path] = []string{"`max_packet_bytes`"}
	}
	tokens["../contracts/spar-config.md"] = append(tokens["../contracts/spar-config.md"], "`max_packet_bytes_source`")
	rptRequire(t, "limit-undocumented", tokens)
}

// AC-14: the rules apply to goals by the Seal that started them: earlier
// goals' REVIEW.md is not rewritten, the first-round rule applies to goals
// started with this Seal or later, and boundary documents go to every round
// this Seal runs.
func TestRepeatFindingsAC14_EarlierGoals(t *testing.T) {
	const id = "earlier-goal-rule-missing"
	version := rptRuleSeal
	rptRequire(t, id, map[string][]string{
		rptSealSkill: {"in goals started with seal " + version + " or later, before the earliest review round", "(in earlier goals, before any plan round)",
			"Every round gets `REVIEW.md` as `--boundary`"},
		rptSealBundle: {
			"goals started with seal " + version + " or later",
			"goals started earlier write the forms before any plan round",
			"`--boundary` in every round this Seal runs, whichever Seal started the goal",
		},
	})
	text := rptDoc(t, rptSealBundle)
	i := strings.Index(text, "goals started with seal "+version+" or later")
	if i < 0 {
		return
	}
	paragraph := text[i:]
	if j := strings.Index(paragraph, "\n\n"); j >= 0 {
		paragraph = paragraph[:j]
	}
	if !strings.Contains(paragraph, "never rewritten") {
		t.Errorf("jaekit:violation=%s the paragraph of %s that names seal %s does not say earlier goals' REVIEW.md is never rewritten", id, rptSealBundle, version)
	}
}

// rptRuleSeal is the Seal version that brought these rules. A later Seal
// keeps them and their version line, so the checks name this version, not
// the one in the skill's front matter.
const rptRuleSeal = "0.1.31"

// AC-15: the Seal version list has a line for the Seal version that brought
// this change, which names it and says it makes no Core change.
func TestRepeatFindingsAC15_Changelog(t *testing.T) {
	const id = "changelog-entry-missing"
	version := rptRuleSeal
	var line string
	for _, l := range strings.Split(rptDoc(t, rptSealHa), "\n") {
		if strings.HasPrefix(l, "Seal "+version+" ") {
			line = l
		}
	}
	switch {
	case line == "":
		t.Errorf("jaekit:violation=%s %s has no line for Seal %s", id, rptSealHa, version)
	case !strings.Contains(line, "`--boundary`") || !strings.Contains(line, "no Core change"):
		t.Errorf("jaekit:violation=%s the line for Seal %s does not name `--boundary` and no Core change", id, version)
	}
}
