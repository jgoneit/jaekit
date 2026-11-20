package plugins

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// the Spec skill leaves how to find outcome-changing decisions to the agent:
// no document of it prescribes a discovery step.
func TestSpecCoversOutcomeDecisions(t *testing.T) {
	forbidSkillText(t, specSkillDir, "Before asking", "List what the goal touches")
}

// a decision neither settled nor left to the implementer is asked about or
// listed under ## Open Decisions: that section name stays a format token of
// the Spec skill.
func TestSpecPlacesCoveredDecisions(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Open Decisions")
}

// a decision the user leaves open is recorded under ## Open Decisions with
// Status: Draft, both format tokens of the Spec skill, and no document of the
// skill caps the number of questions.
func TestSpecKeepsQuestionPrinciples(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Open Decisions", "Status: Draft")
	questionCap := regexp.MustCompile(`(?i)\b(at most|no more than|up to|maximum of)\s+\w+\s+questions?\b`)
	for _, doc := range skillDocs(t, specSkillDir) {
		if m := questionCap.FindString(doc.text); m != "" {
			t.Errorf("%s caps the number of questions: %q", doc.path, m)
		}
	}
}

// the goal document contract §6 holds the same rules.
func TestGoalDocsContractHasQuestionRules(t *testing.T) {
	requireAll(t, "../contracts/goal-docs.md",
		"### 6.5 Questions",
		"There is no cap on the number of questions and no fixed question list",
		"The questions and the goal documents do not miss outcome-changing decisions within the requested behavior and the scope its change reaches",
		"is judged by applying the following perspectives to the things the goal touches (people and roles, stored data, states, external systems) and to the pairs of those things that meet",
		"- Count and relations: one or many, who owns what",
		"- Lifecycle: when it is created, changes and ends",
		"- Things that happen at once: what happens when two actions touch the same data",
		"- Value sets: the values that states and identifiers can take, uniqueness",
		"- Failure: what remains when it fails",
		"The perspectives are only a standard for judging omissions; they do not require an answer for every combination of thing, pair and perspective",
		"Combinations that do not change the outcome or are unrelated to the requested behavior are not covered",
		"A decision that fits no perspective is treated the same if it changes the outcome",
		"The order and method for finding such decisions are not specified",
		"If the user's words, existing goal documents or the code settle it, it is in the goal documents with its source",
		"Spec asks when code and documents cannot settle whether today's behavior is intended or a mistake and that judgment changes the outcome",
		"Values the implementer can adjust within already decided behavior, and implementation details that do not change behavior (names, file layout, internal order), are left to the implementer without asking",
		"The rest is asked as a question or is in `## Open Decisions`",
		"Such decisions do not bring unrequested features into the scope or the criteria",
		"The five perspectives are not a question list, and per-perspective check results are not written in goal documents",
		"In a small fix, only decisions that touch what changes and the existing behavior that change affects are covered",
		"puts the recommended option first, with the reason and what each option loses",
		"When showing two or more drafts, Spec says which one it recommends and why",
		"is asked about as a concrete proposal the user can correct, and answers outside the offered options are accepted too",
		"Spec does not invent a preference; it says there are no grounds",
		"A recommendation is not a decision until the user accepts it",
		"it becomes a decision whose source is the Spec proposal stated in the report, and it goes into the report's decision list (§6.4)",
	)
}

// a quality case holds two unstated decisions (a
// concurrent action on the same data, and how many of one relate to another).
func TestSpecCaseForDecisionFinding(t *testing.T) {
	requireAll(t, "../evals/spec-cases/meeting-room-booking.md",
		"같은 회의실의 같은 시간에 두 사람이 동시에 예약하려 할 때 어떻게 할지",
		"한 예약이 회의실 하나만 잡는지, 여럿을 함께 잡을 수 있는지",
		"묻지 않았다면 `## Open Decisions`에 있어야 한다",
		"묻지 않아야 할 것",
		"구현자가 정할 세부",
		"들어가면 안 되는 것",
		"질문 상한",
	)
}

// a quality case expects the recommended option first,
// with its reason and what each option gives up.
func TestSpecCaseForRecommendation(t *testing.T) {
	requireAll(t, "../evals/spec-cases/admin-order-search.md",
		"사용자에게 보이는 차이가 있는 선택이다",
		"추천하는 선택지를 맨 앞에 두고 이유를 함께 적는다",
		"선택지마다 잃는 것을 함께 적는다",
		"질문 상한",
	)
}

// Every public quality case is listed and has enough synthetic context to run it.
func TestSyntheticSpecCasesHaveContext(t *testing.T) {
	for _, name := range []string{
		"ambiguous-current-behavior", "ambiguous-export", "bug-report-after-implementation",
		"evaluation-items", "existing-discount-expiry", "login-message-scope",
		"long-session-release", "profile-nickname-save", "readme-typo", "same-request-handoff",
		"meeting-room-booking", "admin-order-search", "draft-then-ready", "target-then-draft",
	} {
		t.Run(name, func(t *testing.T) {
			requireAll(t, "../evals/spec-cases/README.md", "["+name+"]("+name+".md)")
			requireAll(t, "../evals/spec-cases/"+name+".md",
				"평가용 가상 시나리오", "실제 사용자 대화나 실행 기록이 아닙니다",
				"## 요청", "## 저장소 맥락", "## 사용자 답", "## 기대 결과", "질문 상한")
		})
	}
}

// the Spec plugin version is above 0.1.7, and the skill,
// both plugin manifests, and the marketplace entry agree on it.
func TestSpecPluginVersionRaised(t *testing.T) {
	_, version := frontMatter(t, "spec/skills/spec/SKILL.md")
	var claude, codex manifest
	readJSON(t, "spec/.claude-plugin/plugin.json", &claude)
	readJSON(t, "spec/.codex-plugin/plugin.json", &codex)
	var market struct {
		Plugins []struct{ Name, Version string } `json:"plugins"`
	}
	readJSON(t, "../.claude-plugin/marketplace.json", &market)
	marketVersion := ""
	for _, p := range market.Plugins {
		if p.Name == "spec" {
			marketVersion = p.Version
		}
	}
	if claude.Version != version || codex.Version != version || marketVersion != version {
		t.Errorf("Spec versions disagree: skill %q, claude %q, codex %q, marketplace %q", version, claude.Version, codex.Version, marketVersion)
	}
	parts := strings.Split(version, ".")
	if len(parts) != 3 {
		t.Fatalf("Spec version %q is not major.minor.patch", version)
	}
	var n [3]int
	for i, p := range parts {
		v, err := strconv.Atoi(p)
		if err != nil {
			t.Fatalf("Spec version %q: %v", version, err)
		}
		n[i] = v
	}
	if n[0] == 0 && (n[1] < 1 || n[1] == 1 && n[2] <= 7) {
		t.Errorf("Spec version %s is not above 0.1.7", version)
	}
}
