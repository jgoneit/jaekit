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
)

// AC-7: every round, from the plan round to the cumulative round, gets
// REVIEW.md as a boundary document, and the round example shows it.
func TestRepeatFindingsAC7_BoundaryEveryRound(t *testing.T) {
	const id = "boundary-passing-rule-missing"
	rptRequire(t, id, map[string][]string{
		rptSealSkill: {"`--boundary`"},
	})
	block := rptDoc(t, rptSealBundle)
	if i := strings.Index(block, "**Decisions for review rounds.**"); i >= 0 {
		block = block[i:]
	}
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
