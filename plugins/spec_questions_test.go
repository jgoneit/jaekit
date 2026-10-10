package plugins

import (
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// questionsSection returns the `## Questions` section of the Spec skill body.
func questionsSection(t *testing.T) string {
	t.Helper()
	text := read(t, "spec/skills/spec/SKILL.md")
	_, rest, ok := strings.Cut(text, "\n## Questions\n")
	if !ok {
		t.Fatal("Spec SKILL.md has no ## Questions section")
	}
	section, _, _ := strings.Cut(rest, "\n## ")
	return section
}

func requireInQuestions(t *testing.T, musts ...string) {
	t.Helper()
	section := questionsSection(t)
	for _, s := range musts {
		if !strings.Contains(section, s) {
			t.Errorf("Spec SKILL.md ## Questions lacks %q", s)
		}
	}
}

// a question with options leads with the recommended
// option, its reason, and what each option gives up.
func TestSpecLeadsWithRecommendation(t *testing.T) {
	requireInQuestions(t, "When a question has options, put the option you recommend first, with the reason and what each option gives up.")
}

// with more than one draft, Spec says which it
// recommends and why.
func TestSpecRecommendsAmongDrafts(t *testing.T) {
	requireInQuestions(t, "When you show more than one draft, say which one you recommend and why.")
}

// behavior only the user knows is asked with a concrete
// proposal, and answers outside the options are accepted.
func TestSpecProposesUserOnlyBehavior(t *testing.T) {
	requireInQuestions(t, "For behavior only the user knows, offer a concrete proposal they can adjust, and accept an answer outside the options you offered.")
}

// without grounds for either option, Spec says so
// instead of inventing a preference.
func TestSpecInventsNoPreference(t *testing.T) {
	requireInQuestions(t, "When nothing in the request, the code, or the documents favors one option, say so instead of inventing a preference.")
}

// a recommendation becomes a decision only when the user
// accepts it or leaves the choice to Spec, with a reported proposal as source.
func TestSpecTreatsRecommendationAsProposal(t *testing.T) {
	requireInQuestions(t,
		"A recommendation is not a decision until the user accepts it.",
		"When the user accepts it or leaves the choice to you",
		"it becomes a decision whose source is a proposal stated in your report, and it goes into the list of decisions in your report.",
	)
}

// the questions and goal documents cover the
// outcome-changing decisions of the requested behavior and what it reaches.
// Five views judge omissions without demanding every combination, and the
// skill leaves how to find them to the agent.
func TestSpecCoversOutcomeDecisions(t *testing.T) {
	requireInQuestions(t,
		"Your questions and the goal documents leave out no decision that changes the outcome within the requested behavior and the places the change reaches.",
		"Whether one is missing is judged by holding these views against what the goal touches (people and roles, stored data, states, and outside systems) and each pair that meets:",
		"- How many: one or many, and what belongs to what.",
		"- Life: how each is created, changed, and ended.",
		"- At the same time: what happens when two actions reach the same data at once.",
		"- Values: which values a state or identifier can take, and what must be unique.",
		"- Failure: what a failure leaves behind.",
		"The views test for omissions; they do not demand an answer for every combination.",
		"Leave out combinations that do not change the outcome or lie outside the requested behavior.",
		"A decision that fits none of the views counts the same when it changes the outcome.",
		"How and in what order you find these decisions is up to you.",
	)
	section := questionsSection(t)
	for _, step := range []string{"Before asking", "List what the goal touches"} {
		if strings.Contains(section, step) {
			t.Errorf("Spec SKILL.md ## Questions prescribes a discovery step: %q", step)
		}
	}
}

// each such decision is in the goal documents with its
// source (code shows behavior, not intent), left to the implementer as a
// tuning value or a behavior-neutral detail, or asked or listed as open.
func TestSpecPlacesCoveredDecisions(t *testing.T) {
	requireInQuestions(t,
		"Each such decision ends up in one of three places.",
		"When the user's words, existing goal documents, or the code settle it, it is in the goal documents with its source;",
		"the code shows today's behavior but not whether it was meant, so when the code and documents cannot settle that and the answer changes the result, ask.",
		"A value the implementer can tune inside a decided behavior, and an implementation detail that does not change behavior (names, file layout, internal order), is left to the implementer without asking.",
		"Every other one is asked about or listed under `## Open Decisions`.",
	)
}

// covered decisions never widen the scope, the views are
// not a question list, and the documents record no check per view.
func TestSpecKeepsCoverageInScope(t *testing.T) {
	requireInQuestions(t,
		"Such a decision never adds a feature the user did not ask for to the scope or the criteria.",
		"The views are not a question list, and the goal documents do not record a check per view.",
	)
}

// a small fix covers the decisions the change touches,
// including existing behavior the change affects.
func TestSpecSmallFixCoversAffectedBehavior(t *testing.T) {
	requireInQuestions(t, "For a small fix, cover only the decisions the change touches, including existing behavior the change affects.")
}

// the existing question principles stay, and there is
// no cap on the number of questions.
func TestSpecKeepsQuestionPrinciples(t *testing.T) {
	requireInQuestions(t,
		"Ask only about decisions that change the outcome: externally visible behavior, data and compatibility, permissions and security, irreversible design, and scope.",
		"Never ask what an implementer can decide, such as names, file layout, or internal order.",
		"Group related questions in one message.",
		"When the user leaves a decision open, record it in `## Open Decisions` and keep `Status: Draft`.",
		"Code shows what happens, not what was meant.",
		"Never ask what the code settles.",
	)
	text := read(t, "spec/skills/spec/SKILL.md")
	if !strings.Contains(text, "Find every place the changed behavior reaches: other paths into the same code, saved settings and data, and update or migration paths.") {
		t.Error("Spec SKILL.md lost the procedure for places the change reaches")
	}
	if m := regexp.MustCompile(`(?i)\b(at most|no more than|up to|maximum of)\s+\w+\s+questions?\b`).FindString(text); m != "" {
		t.Errorf("Spec SKILL.md caps the number of questions: %q", m)
	}
}

// the goal document contract §6 holds the same rules.
func TestGoalDocsContractHasQuestionRules(t *testing.T) {
	requireAll(t, "../contracts/goal-docs.md",
		"### 6.5 질문",
		"질문 수 상한과 고정된 질문 목록은 두지 않는다",
		"질문과 목표 문서는 요청된 동작과 그 변경이 닿는 범위에서 결과를 바꾸는 결정을 빠뜨리지 않는다",
		"목표가 건드리는 대상(사람과 역할, 저장 데이터, 상태, 외부 시스템)과 서로 만나는 대상의 쌍에 다음 관점을 대어 판단한다",
		"- 개수와 관계: 하나인가 여럿인가, 누가 무엇을 가지는가",
		"- 생애: 언제 생기고 바뀌고 끝나는가",
		"- 동시에 일어나는 일: 두 행동이 같은 데이터에 닿을 때 어떻게 되는가",
		"- 값의 집합: 상태와 식별자가 가질 수 있는 값, 고유성",
		"- 실패: 실패하면 무엇이 남는가",
		"관점은 누락을 판단하는 기준일 뿐이며 모든 대상·쌍·관점의 조합에 답을 요구하지 않는다",
		"결과를 바꾸지 않거나 요청된 동작과 관계없는 조합은 다루지 않는다",
		"어느 관점에도 맞지 않는 결정도 결과를 바꾸면 같다",
		"그런 결정을 어떤 순서와 방법으로 찾을지는 정하지 않는다",
		"사용자의 말, 기존 목표 문서, 코드로 정해지면 출처와 함께 목표 문서에 있다",
		"지금 동작이 의도인지 실수인지 코드와 문서로 정할 수 없고 그 판단이 결과를 바꾸면 묻는다",
		"이미 정한 동작 안에서 구현자가 조정할 수 있는 값과 동작을 바꾸지 않는 구현 세부(이름, 파일 배치, 내부 순서)는 묻지 않고 구현자에게 맡긴다",
		"나머지는 질문으로 묻거나 `## Open Decisions`에 있다",
		"그런 결정 때문에 요청하지 않은 기능이 범위나 조건에 들어가지 않는다",
		"다섯 관점은 질문 목록이 아니며, 목표 문서에 관점별 점검 결과를 쓰지 않는다",
		"작은 수정에서는 바꾸는 것과 그 변경으로 영향을 받는 기존 동작에 닿는 결정만 다룬다",
		"추천하는 선택지를 맨 앞에 두고, 그 이유와 선택지마다 잃는 것을 함께 적는다",
		"초안을 둘 이상 보이면 어느 쪽을 추천하는지와 이유를 함께 말한다",
		"사용자가 고칠 수 있는 구체적인 제안으로 묻고, 제시한 선택지 밖의 답도 받는다",
		"선호를 지어내지 않고 근거가 없다고 말한다",
		"추천은 사용자가 받아들이기 전에는 결정이 아니다",
		"보고에 적은 Spec의 제안을 출처로 하는 결정이 되고 보고의 결정 목록(§6.4)에 오른다",
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
