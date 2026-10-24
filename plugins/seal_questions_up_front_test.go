package plugins

// Checks of the seal-questions-up-front goal: the Seal skill, its bundle and
// ha references and the usage guides say that Seal asks the user decisions it
// can foresee in one message before any baseline, names the conditions only
// the user can confirm, lists the test and check files a plan adds in its
// scope from the start, asks later questions when they arise, asks nothing
// when there is nothing to ask, and keeps its authority rules.
//
// Each test observes fixed phrases in named sections of fixed files of the
// current tree, never an open category of sentences, and opens every failure
// message about its property with the same violation marker, which ha's
// go-test-json adapter reads. A missing section is that violation: the
// criterion requires the section to say something. A file the test cannot
// read fails with a "setup:" message and no marker: that is neither the
// declared violation nor a pass.

import (
	"os"
	"strings"
	"testing"
)

const (
	sqSkill   = "seal/skills/seal/SKILL.md"
	sqBundle  = "seal/skills/seal/references/bundle.md"
	sqHa      = "seal/skills/seal/references/ha.md"
	sqUsage   = "../guides/USAGE.md"
	sqUsageEn = "../guides/USAGE.en.md"

	sqStarting  = "## Starting"
	sqQuestions = "## Questions for the user"
	sqScope     = "### `## 범위`"
)

// sqRead returns a file of the tree under test. Any read error is a setup
// failure, never the violation.
func sqRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("setup: read %s: %v", path, err)
	}
	return string(b)
}

// sqSection returns the text from heading to the next heading of the same or
// a higher level, skipping fenced code, and whether the heading exists.
func sqSection(text, heading string) (string, bool) {
	lines := strings.Split(text, "\n")
	level := strings.Index(heading, " ")
	start, fenced := -1, false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}
		if start < 0 {
			if line == heading {
				start = i
			}
			continue
		}
		if n := strings.Index(line, " "); n > 0 && n <= level && strings.Trim(line[:n], "#") == "" {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

// sqRequire reports, with the violation marker, each phrase the section of
// the file does not contain, or the missing section itself.
func sqRequire(t *testing.T, v, path, heading string, phrases ...string) {
	t.Helper()
	text := sqRead(t, path)
	where := path
	if heading != "" {
		section, ok := sqSection(text, heading)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks the section %q", v, path, heading)
			return
		}
		text, where = section, path+" "+heading
	}
	for _, p := range phrases {
		if !strings.Contains(text, p) {
			t.Errorf("jaekit:violation=%s %s does not say %q", v, where, p)
		}
	}
}

// AC-1: before any baseline, Seal asks in one message the decisions it can
// foresee from the goal documents, plan, project instructions and code that
// need the user's words, with options, the recommendation leading and what
// the answer changes. The examples are typical, not a complete list.
func TestSealQuestionsAC1_OneMessageBeforeBaseline(t *testing.T) {
	const v = "prestart-questions-missing"
	sqRequire(t, v, sqSkill, sqStarting,
		"Before any baseline, ask in one message every decision needing the user's words that the goal documents, plan, project instructions and code already show",
		"each with options, the recommended one leading, and what the answer changes",
		"(references/bundle.md#questions-for-the-user)")
	sqRequire(t, v, sqBundle, sqQuestions,
		"Before any baseline",
		"gather every decision that the goal documents, the plan, the project instructions and the code already show and that the current rules leave to the user's words, and ask them all in one message",
		"Typical decisions, not a complete list:",
		// The four representative examples of SPEC AC-1.
		"a change that project instructions reserve for the user's words, such as a plan that changes a file on a protected path",
		"a change to an existing test that the request does not authorize",
		"a place where the goal documents and the code seem to disagree and the user must choose",
		"unusable tools that `ha doctor` reports, and a computed budget that needs approval",
		"Any other decision of that kind goes in the same message.",
		"Give each question its options, the recommended one leading, and what each answer changes.")
}

// AC-2: the same message names the conditions only the user's confirmation
// can meet and what the user will be asked to confirm at the end.
func TestSealQuestionsAC2_UserOnlyConditionsNamed(t *testing.T) {
	const v = "user-confirmation-unannounced"
	sqRequire(t, v, sqBundle, sqQuestions,
		"When a condition can be met only by the user's confirmation",
		"the same message names it and what you will ask them to confirm at the end")
}

// AC-3: the test and check files a plan adds, with their helpers, are in the
// changeable paths from the start, never a path the goal documents keep
// unchanged.
func TestSealQuestionsAC3_NewCheckFilesInScope(t *testing.T) {
	const v = "new-check-files-out-of-scope"
	sqRequire(t, v, sqSkill, sqStarting,
		"List the test and check files the plan adds, with their helpers, in `바꿀 수 있는 경로` from the start.")
	sqRequire(t, v, sqBundle, sqScope,
		"List from the start every test and check file the plan will add, with its helper files, so adding them needs no scope confirmation",
		"never list a path the goal documents say must not change")
}

// AC-4: later questions are asked when they arise, the ones pending at the
// same moment in one message, and the confirmations completion needs in one
// message.
func TestSealQuestionsAC4_LaterQuestionsTogether(t *testing.T) {
	const v = "later-questions-scattered"
	sqRequire(t, v, sqSkill, sqStarting,
		"Ask a later question when it arises, those pending at once in one message.")
	sqRequire(t, v, sqBundle, sqQuestions,
		"ask each question when it arises",
		"questions pending at the same moment go in one message",
		"Ask for the confirmations completion needs in one message, once the results they confirm exist.")
}

// AC-5: with nothing to foresee, Seal asks nothing, and it never asks whether
// anything else needs asking.
func TestSealQuestionsAC5_NothingToAskAsksNothing(t *testing.T) {
	const v = "empty-question-asked"
	sqRequire(t, v, sqSkill, sqStarting, "with none, ask nothing.")
	sqRequire(t, v, sqBundle, sqQuestions,
		"With no such decision or condition, ask nothing and start the work",
		"never ask whether anything else needs asking")
}

// AC-6: the authority rules stay as they were, and asking early never stands
// in for a needed confirmation.
func TestSealQuestionsAC6_AuthorityKept(t *testing.T) {
	const v = "authority-rule-lost"
	// The rules as the base states them, letter for letter.
	sqRequire(t, v, sqSkill, "## Authority",
		"Work on a goal only when the user asks in this conversation to implement, finish, or continue it.",
		"belong to the user. Read them; never edit them.")
	sqRequire(t, v, sqSkill, "## Seal Core",
		"a weakened check or a change outside it needs the user's confirmation")
	sqRequire(t, v, sqSkill, sqStarting,
		"if the computed limit needs approval, ask once with both limits and write the approved number, never asking again for that PLAN")
	sqRequire(t, v, sqHa, "## run-rules/4",
		"When you ask the user a question the remaining work depends on, record `ha note <goal> wait --reason \"<the decision asked>\"`",
		"keep working on independent work while you wait")
	// How the questions asked up front keep those rules.
	sqRequire(t, v, sqBundle, sqQuestions,
		"An answer settles only what it says.",
		"A change outside the recorded authority or one that weakens a check still needs the user's confirmation, recorded with `ha note <goal> confirm`; asking early never replaces it.",
		"While you wait, record the wait and keep doing independent work",
		"the budget is still asked once per PLAN")
}

// AC-7: both usage guides' pre-start sections say what Seal asks in one
// message before the start and what it asks after the start.
func TestSealQuestionsAC7_UsageGuides(t *testing.T) {
	const v = "guide-questions-missing"
	sqRequire(t, v, sqUsage, "## Seal의 시작 전 확인과 검토 순서",
		"**시작 전 질문은 한 메시지로.**",
		"미리 알 수 있고 사용자의 말이 필요한 결정을 모아 한 메시지로 묻습니다",
		"물을 것이 없으면 아무것도 묻지 않고",
		"**시작 뒤 질문은 생길 때.**",
		"미리 알 수 없던 질문은 생겼을 때 묻고, 같은 때 기다리는 질문은 한 메시지로 묻습니다",
		"끝에 필요한 사용자 확인도 결과가 생긴 뒤 한 메시지로 모아 묻습니다")
	sqRequire(t, v, sqUsageEn, "## Seal's pre-start checks and review order",
		"**Questions before the start, in one message.**",
		"that need your words, and asks them in one message",
		"When there is nothing to ask, Seal asks nothing",
		"**Later questions when they arise.**",
		"is asked when it arises; questions pending at the same moment go in one message",
		"The confirmations needed at the end are also asked together in one message")
}
