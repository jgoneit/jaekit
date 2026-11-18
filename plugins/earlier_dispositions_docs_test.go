package plugins

// Checks of the spar-earlier-dispositions goal on documents. Each test
// observes fixed tokens and one fixed rule text in named documents, and the
// absence of fixed old phrases in the representative documents below;
// whether the sentences around them say the same thing is for a person to
// read. Paths are relative to the repository root, one folder above this
// package. A file or folder that cannot be read, or a file that is not UTF-8,
// is an unmarked t.Fatalf before anything is judged, so an input failure is
// never a pass or a declared violation. A violation is reported as one
// failure whose first line starts with jaekit:violation=<id>: .

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"
)

const (
	edSkill         = "plugins/spar/skills/spar/SKILL.md"
	edRounds        = "plugins/spar/skills/spar/references/rounds.md"
	edAgents        = "AGENTS.md"
	edCumulative    = "receives the earlier code rounds' findings with their last dispositions"
	edOldCumulative = "does not take earlier findings"
	// edRepeatedPremises is the one rule on repeated premises, from its bold
	// name to the end of its last sentence.
	edRepeatedPremises = "**Repeated premises.** This rule applies to local rounds and to remote pull request reviews alike. " +
		"After a premise was rejected with evidence or answered by the user, do not fix a finding on the same premise that brings no failure scenario the recorded disposition does not answer: " +
		"record it as a duplicate of the earlier finding, or reject it citing that disposition. A finding with a new failure scenario is a new finding. " +
		"When the same premise comes back that way twice, ask the user which reading of the criterion is intended, and wait for the answer."
	edPremisesName = "**Repeated premises.**"
)

// edOldPremiseRules are the phrases of the two rules the one rule replaces.
var edOldPremiseRules = []string{"two rounds in a row", "more than three remote review rounds"}

// edRead reads a file by its path from the repository root.
func edRead(t *testing.T, rel string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("setup: %s cannot be read: %v", rel, err)
	}
	if !utf8.Valid(data) {
		t.Fatalf("setup: %s is not UTF-8", rel)
	}
	return string(data)
}

// edRepresentative reads the representative documents: every *.md under
// plugins/, contracts/ and guides/, README.md, README.ko.md and
// README.en.md when present, and AGENTS.md. It returns their paths in order
// and their texts.
func edRepresentative(t *testing.T) ([]string, map[string]string) {
	t.Helper()
	texts := map[string]string{}
	for _, dir := range []string{"plugins", "contracts", "guides"} {
		root := filepath.Join("..", dir)
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			t.Fatalf("setup: %s/ is not a folder: %v", dir, err)
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
				return nil
			}
			rel, err := filepath.Rel("..", path)
			if err != nil {
				return err
			}
			rel = filepath.ToSlash(rel)
			texts[rel] = edRead(t, rel)
			return nil
		})
		if err != nil {
			t.Fatalf("setup: reading %s/: %v", dir, err)
		}
	}
	for _, rel := range []string{"README.md", "README.ko.md", "README.en.md"} {
		if _, err := os.Lstat(filepath.Join("..", rel)); errors.Is(err, fs.ErrNotExist) {
			continue
		} else if err != nil {
			t.Fatalf("setup: %s: %v", rel, err)
		}
		texts[rel] = edRead(t, rel)
	}
	texts[edAgents] = edRead(t, edAgents)
	var paths []string
	for p := range texts {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, texts
}

// edReport fails with id and the problems found, in one failure.
func edReport(t *testing.T, id string, problems []string) {
	t.Helper()
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=%s: %s", id, strings.Join(problems, "\n"))
	}
}

// AC-2: the Spar skill, its rounds reference and the review-round and Spar
// input contracts say that a candidate round (the cumulative round included)
// receives the earlier code rounds' findings with their last dispositions,
// and no representative document still says that it does not.
func TestSparEarlierDispositionsAC2_CumulativeTakesDispositions(t *testing.T) {
	required := []struct{ path, token string }{
		{edSkill, edCumulative},
		{edRounds, edCumulative},
		{"contracts/review-round.en.md", "for candidate and fixes rounds"},
		{"contracts/review-round.md", "candidate와 fixes에서는"},
		{"contracts/spar-input.en.md", "In a candidate, fixes or plan-delta round"},
		{"contracts/spar-input.md", "candidate·fixes·plan-delta 라운드의"},
	}
	texts := map[string]string{}
	for _, r := range required {
		texts[r.path] = edRead(t, r.path)
	}
	paths, representative := edRepresentative(t)
	var problems []string
	for _, r := range required {
		if !strings.Contains(texts[r.path], r.token) {
			problems = append(problems, fmt.Sprintf("%s does not have %q", r.path, r.token))
		}
	}
	for _, p := range paths {
		if strings.Contains(representative[p], edOldCumulative) {
			problems = append(problems, fmt.Sprintf("%s still says %q", p, edOldCumulative))
		}
	}
	edReport(t, "cumulative-without-dispositions", problems)
}

// AC-5: the Spar skill and AGENTS.md give the one rule on repeated premises
// byte for byte, wherever its bold name appears, and no representative
// document keeps the phrases of the two rules it replaces.
func TestSparEarlierDispositionsAC5_OneRepeatedPremiseRule(t *testing.T) {
	texts := map[string]string{edSkill: edRead(t, edSkill), edAgents: edRead(t, edAgents)}
	paths, representative := edRepresentative(t)
	var problems []string
	for _, p := range []string{edSkill, edAgents} {
		text := texts[p]
		if !strings.Contains(text, edPremisesName) {
			problems = append(problems, fmt.Sprintf("%s has no %s rule", p, edPremisesName))
			continue
		}
		// From each bold name to the end of the rule; in AGENTS.md the rule is
		// a list item, so the "- " before the name is not compared.
		for rest := text; ; {
			i := strings.Index(rest, edPremisesName)
			if i < 0 {
				break
			}
			rest = rest[i:]
			if !strings.HasPrefix(rest, edRepeatedPremises) {
				line, _, _ := strings.Cut(rest, "\n")
				problems = append(problems, fmt.Sprintf("%s's rule differs from the one rule: %q", p, line))
			}
			rest = rest[len(edPremisesName):]
		}
	}
	for _, p := range paths {
		for _, old := range edOldPremiseRules {
			if strings.Contains(representative[p], old) {
				problems = append(problems, fmt.Sprintf("%s still says %q", p, old))
			}
		}
	}
	edReport(t, "repeated-premise-rules-differ", problems)
}
