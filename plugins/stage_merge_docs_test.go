package plugins

// Checks of the spar-stage-merge goal on documents. Each test observes fixed
// new tokens in named documents and the absence of fixed old tokens in a fixed
// set of documents; whether the sentences around them say the same thing is
// for a person to read. Tokens are exact substrings of a file's text. Paths
// are relative to the repository root, one folder above this package. Every
// file a test reads is read before anything is judged: a file that cannot be
// read, or that is not UTF-8, is an unmarked t.Fatalf, so an input failure is
// never a pass or a declared violation. A violation is reported as one failure
// whose first line starts with jaekit:violation=<id>: and lists every problem
// found, one per line.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// smToken is one fixed token and the id the goal documents give it.
type smToken struct{ id, text string }

const (
	smSealSkill = "plugins/seal/skills/seal/SKILL.md"
	smSparSkill = "plugins/spar/skills/spar/SKILL.md"
)

// smSkills are the two skill bodies that carry the new review guidance.
var smSkills = []string{smSealSkill, smSparSkill}

// smSkillSet is the skill set: the Seal and Spar skill files that may not keep
// the old fixes and plan-delta guidance.
var smSkillSet = []string{
	smSealSkill,
	"plugins/seal/skills/seal/references/bundle.md",
	"plugins/seal/skills/seal/references/ha.md",
	"plugins/seal/skills/seal/assets/templates/REVIEW.md",
	smSparSkill,
	"plugins/spar/skills/spar/references/rounds.md",
}

// smDocSet is the contract and guide set: each named document in its Korean
// (X.md) and English (X.en.md) version.
var smDocSet = smBothLanguages(
	"contracts/review-round",
	"contracts/review-gate",
	"contracts/spar-input",
	"contracts/spar-config",
	"contracts/bundle",
	"contracts/role-card",
	"guides/SPAR",
	"guides/USAGE",
	"guides/OPERATIONS",
)

// AC-1 tokens: one candidate round after review fixes.
var (
	smAC1New = []smToken{
		{"N1", "After review fixes, run one `candidate` round from the goal's base commit to the new head"},
		{"N2", "The last such round that ends with no open `defect` or `requirement-question` is the cumulative round"},
	}
	smAC1Old = []smToken{
		{"O1", "a `fixes` round after review-fix commits"},
		{"O2", "A `fixes` round never replaces it"},
		{"O3", "`spar run` with `--stage fixes`"},
		{"O4", "a `fixes` round sees only the fixes"},
		{"O5", "when fixes followed, once more as the cumulative round"},
		// O6: the REVIEW template's 독립 검토 placeholder lists the fixes round
		// among the planned review stages.
		{"O6", "candidate, fixes and the cumulative round"},
		// O7: the bundle reference's 독립 검토 row lists fixes as a review stage.
		{"O7", "(plan, candidate, fixes, cumulative round)"},
		{"O8", "a candidate round with no later fixes is one"},
		{"O9", "three code rounds (candidate and fixes together)"},
		{"O10", "When no fixes followed the first candidate round, that round is the cumulative round"},
		{"O11", "a fixes round run on the same base and head after a disposition changed"},
		{"O12", "After committing fixes for findings; its base is the head of the previous code round"},
		{"O13", "and the cumulative round to the last head"},
		{"O14", "narrow checks pass and the cumulative round"},
		{"O15", "(plan, candidate, cumulative round)"},
		{"O16", "after review fixes and the cumulative round"},
	}
)

// AC-2 tokens: a strict goal's plan round, then candidate rounds.
var (
	smAC2New = []smToken{
		{"N3", "A `strict` goal takes one `plan` round before implementation"},
		{"N4", "the next `candidate` round reviews the current goal documents"},
	}
	smAC2Old = []smToken{
		{"P1", "`--stage plan-delta`"},
		{"P2", "At most once, only when the plan round changed the plan or its checks"},
		{"P3", "one plan-delta round"},
		{"P4", "--stage plan|plan-delta"},
		{"P5", "In fixes and plan-delta rounds, you may give the earlier standpoints again or give none"},
		{"P6", "In candidate, fixes and plan-delta packets"},
		{"P7", "no plan-delta round is required"},
		{"P8", "no plan-delta is required"},
	}
)

// AC-9 tokens: the contracts and guides name the two stages of new rounds.
var (
	smAC9New = []struct {
		path  string
		token smToken
	}{
		{"contracts/review-round.en.md", smToken{"R1", "New rounds use the stages `plan` and `candidate`"}},
		{"contracts/review-round.md", smToken{"R2", "새 라운드의 단계는 `plan`과 `candidate`다"}},
		{"contracts/review-round.en.md", smToken{"R3", "recorded `plan-delta` and `fixes` rounds are still read"}},
		{"contracts/review-round.md", smToken{"R4", "기록된 `plan-delta`·`fixes` 라운드는 그대로 읽는다"}},
		{"contracts/spar-input.en.md", smToken{"R5", "a new round with `plan-delta` or `fixes` is a usage error that names `candidate`"}},
		{"contracts/spar-input.md", smToken{"R6", "새 라운드를 `plan-delta`나 `fixes`로 시작하면 `candidate`를 알리는 사용법 오류다"}},
		{"contracts/spar-config.en.md", smToken{"R7", "`code` 2"}},
		{"contracts/spar-config.en.md", smToken{"R7", "`plan_delta` is still read, and no new round uses it"}},
		{"contracts/spar-config.md", smToken{"R8", "`code` 2"}},
		{"contracts/spar-config.md", smToken{"R8", "`plan_delta`는 읽지만 새 라운드에는 쓰이지 않는다"}},
		{"contracts/review-gate.en.md", smToken{"R9", "With no recorded fixes round, `growth.rounds` is empty: growth was not measured"}},
		{"contracts/review-gate.md", smToken{"R10", "기록된 fixes 라운드가 없으면 `growth.rounds`는 비어 있다: growth를 재지 않았다"}},
		{"guides/SPAR.en.md", smToken{"R11", "One series allows one plan round and two code rounds"}},
		{"guides/SPAR.md", smToken{"R12", "한 series는 plan 1회, 코드 라운드 2회까지입니다"}},
		{"contracts/review-round.en.md", smToken{"R13", "A recorded series that gets a new round is counted against the caps that new round records"}},
		{"contracts/review-round.md", smToken{"R14", "기록된 series에 새 라운드를 더하면 그 새 라운드가 기록한 상한으로 센다"}},
	}
	smAC9Old = []smToken{
		{"Q1", "One series allows one plan round, one plan-delta round and three code rounds"},
		{"Q2", "한 series는 plan 1회, plan delta 1회, 코드 라운드 3회까지입니다"},
		{"Q3", "`code` 3 (candidate and fixes together)"},
		{"Q4", "`code` 3(candidate와 fixes 합계)"},
		{"Q5", "spar run --stage fixes"},
		{"Q6", "a fixes stage after the review-fix commits"},
		{"Q7", "검토 수정 commit 뒤 fixes 단계"},
		{"Q8", "a fixes stage after review fixes"},
		{"Q9", "검토 수정 뒤 fixes 단계"},
		{"Q10", "takes the candidate, fixes and cumulative rounds"},
		{"Q11", "a fixes round after review fixes"},
		{"Q12", "검토 수정 뒤 fixes 라운드"},
		{"Q13", "at most once, when the plan round changed the plan"},
		{"Q14", "plan 라운드로 계획이 바뀌었을 때 최대 한 번"},
		{"Q15", "after fixing findings; its base is the previous code round's head"},
		{"Q16", "지적을 고친 뒤. base는 이전 코드 라운드의 head"},
		{"Q17", "`standard`는 최종 후보·수정·누적 라운드"},
		{"Q18", "| `--stage` | `plan`, `plan-delta`, `candidate`, `fixes` |"},
		{"Q19", "those of the plan and plan-delta rounds for plan-delta"},
		{"Q20", "plan-delta에서는 plan·plan-delta 라운드"},
		{"Q21", "The stage description of such a fixes round becomes"},
		{"Q22", "그런 fixes 라운드의 단계 설명은"},
		{"Q23", "A `fixes` or `plan-delta` run"},
		{"Q24", "`fixes`나 `plan-delta` 실행"},
		{"Q25", "The base of a fixes round must be the head of the last code round of the same series"},
		{"Q26", "fixes 라운드의 base는 같은 series의 마지막 code 라운드의 head여야 한다"},
		{"Q27", "The plan and plan-delta cap refusals are unchanged"},
		{"Q28", "plan·plan-delta 상한 거부 메시지는 그대로다"},
		{"Q29", "docs/reviews/example/03-fixes"},
		{"Q30", "In a candidate, fixes or plan-delta round"},
		{"Q31", "candidate·fixes·plan-delta 라운드의"},
		{"Q32", "plan_delta = 1"},
		{"Q33", "(plan, candidate, fixes, cumulative round)"},
		{"Q34", "계획한 검토 단계(plan, candidate, fixes, 누적 라운드)"},
		{"Q35", "A fixes round that saw only the fixes cannot replace it"},
		{"Q36", "수정분만 본 fixes 라운드는 이를 대신하지 못"},
		{"Q37", "The packet of a candidate, fixes or plan-delta round"},
		{"Q38", "A fixes round also asks them"},
		{"Q39", "fixes 라운드에서는 고쳤다고 기록한 지적이"},
		{"Q40", "a fixes round run on the same base and head after a disposition changed"},
		{"Q41", "같은 base·head로 돌린 fixes 라운드는 새 라운드로"},
		{"Q42", "In fixes and plan-delta rounds the earlier standpoints"},
		{"Q43", "fixes와 plan-delta에서는 앞선 관점을"},
		{"Q44", "Fixes may follow a candidate"},
		{"Q45", "candidate 뒤에 fixes가 이어져도 되고"},
		{"Q46", "requires a plan-delta"},
		{"Q47", "plan-delta를 요구하지 않습니다"},
		{"Q48", "for candidate and fixes rounds"},
		{"Q49", "candidate와 fixes에서는"},
		{"Q50", "and a cumulative round before `ha done`"},
		{"Q51", "`ha done` 전에 누적 라운드를 받는다"},
		{"Q52", "`ha done` 전 누적 라운드"},
		{"Q53", "after review fixes and the cumulative round"},
		{"Q54", "candidate 라운드와 누적 라운드를 받고"},
		{"Q55", "(plan, candidate, cumulative round)"},
		{"Q56", "(plan, candidate, 누적 라운드)"},
		{"Q57", "검토 수정 뒤의 candidate 라운드와 누적 라운드"},
	}
)

// smBothLanguages returns X.md and X.en.md for each name, in order.
func smBothLanguages(names ...string) []string {
	var paths []string
	for _, name := range names {
		paths = append(paths, name+".md", name+".en.md")
	}
	return paths
}

// smRead reads a file by its path from the repository root. A file that
// cannot be read or is not UTF-8 stops the test without a marker.
func smRead(t *testing.T, rel string) string {
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

// smReadAll reads every file of every list before anything is judged.
func smReadAll(t *testing.T, lists ...[]string) map[string]string {
	t.Helper()
	texts := map[string]string{}
	for _, list := range lists {
		for _, rel := range list {
			if _, ok := texts[rel]; !ok {
				texts[rel] = smRead(t, rel)
			}
		}
	}
	return texts
}

// smRequire adds a problem when path's text does not have the token.
func smRequire(problems *[]string, texts map[string]string, path string, token smToken) {
	if !strings.Contains(texts[path], token.text) {
		*problems = append(*problems, fmt.Sprintf("%s: %s does not have %q", token.id, path, token.text))
	}
}

// smForbid adds one problem for every occurrence of an old token in a file of
// paths, with the line it starts on.
func smForbid(problems *[]string, texts map[string]string, paths []string, tokens []smToken) {
	for _, path := range paths {
		text := texts[path]
		for _, token := range tokens {
			for offset := 0; ; {
				i := strings.Index(text[offset:], token.text)
				if i < 0 {
					break
				}
				at := offset + i
				line := 1 + strings.Count(text[:at], "\n")
				*problems = append(*problems, fmt.Sprintf("%s: %s:%d still has %q", token.id, path, line, token.text))
				offset = at + len(token.text)
			}
		}
	}
}

// smReport fails with id and every problem found, in one failure.
func smReport(t *testing.T, id string, problems []string) {
	t.Helper()
	if len(problems) > 0 {
		t.Errorf("jaekit:violation=%s: %s", id, strings.Join(problems, "\n"))
	}
}

// AC-1: both skill bodies tell the user to run one candidate round from the
// goal's base commit to the new head after review fixes and name the last such
// round with no open defect or requirement question the cumulative round, and
// no file of the skill set keeps the old fixes-round guidance.
func TestSparStageMergeAC1_OneCandidateAfterFixes(t *testing.T) {
	texts := smReadAll(t, smSkills, smSkillSet)
	var problems []string
	for _, path := range smSkills {
		for _, token := range smAC1New {
			smRequire(&problems, texts, path, token)
		}
	}
	smForbid(&problems, texts, smSkillSet, smAC1Old)
	smReport(t, "fixes-round-guidance", problems)
}

// AC-2: both skill bodies say a strict goal takes one plan round before
// implementation and the next candidate round reviews the current goal
// documents, and no file of the skill set keeps the old plan-delta guidance.
func TestSparStageMergeAC2_PlanThenCandidate(t *testing.T) {
	texts := smReadAll(t, smSkills, smSkillSet)
	var problems []string
	for _, path := range smSkills {
		for _, token := range smAC2New {
			smRequire(&problems, texts, path, token)
		}
	}
	smForbid(&problems, texts, smSkillSet, smAC2Old)
	smReport(t, "plan-delta-guidance", problems)
}

// AC-9: the review-round, Spar input, Spar config and review gate contracts
// and the Spar guide name plan and candidate as the stages of new rounds, the
// code-round cap of two and how recorded plan-delta and fixes rounds are read,
// and no file of the contract and guide set keeps an old stage instruction.
func TestSparStageMergeAC9_ContractsNameTwoStages(t *testing.T) {
	var required []string
	for _, r := range smAC9New {
		required = append(required, r.path)
	}
	texts := smReadAll(t, required, smDocSet)
	var problems []string
	for _, r := range smAC9New {
		smRequire(&problems, texts, r.path, r.token)
	}
	smForbid(&problems, texts, smDocSet, smAC9Old)
	smReport(t, "old-stage-instruction", problems)
}
