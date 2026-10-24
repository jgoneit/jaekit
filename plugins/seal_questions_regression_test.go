package plugins

// Regression checks for review fixes of the seal-questions-up-front goal.
// They use the fixed-phrase helpers of seal_questions_up_front_test.go and
// run with the package's other tests.

import (
	"strings"
	"testing"
)

const sqContract = "../contracts/bundle.md"

// AC-6: an answer given up front is recorded as the confirmation once the
// change it allows is committed, because a confirmation binds to the content
// as it is when recorded. That change is not asked about again, but a later
// change to the confirmed content is new and is asked again; the earlier
// quote is never re-recorded for it.
func TestSealQuestionsRegression_EarlyAnswerConfirmedAfterChange(t *testing.T) {
	const v = "early-answer-confirmation-unstated"
	sqRequire(t, v, sqBundle, sqQuestions,
		"A confirmation binds to the content it names as that content is when recorded",
		"record an answer given up front as that confirmation once the change it allows is committed, quoting the answer, and do not ask about that change again.",
		"A later commit that changes the confirmed content is a new change: ask the user again, never re-recording the earlier quote for content they did not answer about.")
	sqRequire(t, v, sqUsage, "## Seal의 시작 전 확인과 검토 순서",
		"미리 받은 답은 그 답이 허락한 변경을 commit한 뒤 그 답을 인용해 확인으로 기록하고, 그 변경을 다시 묻지 않습니다.",
		"확인한 내용이 그 뒤 다시 바뀌면 새 변경이라 다시 묻습니다.")
	sqRequire(t, v, sqUsageEn, "## Seal's pre-start checks and review order",
		"Seal records an answer given up front as the confirmation, quoting it, once the change it allows is committed, and does not ask about that change again.",
		"When the confirmed content changes later, that is a new change and Seal asks again.")
	for _, path := range []string{sqUsage, sqUsageEn} {
		sqForbid(t, v, path, "확인을 기록한 뒤에 합니다", "before the confirmation is recorded")
	}
}

// sqForbid reports, with the violation marker, each phrase the file still
// contains.
func sqForbid(t *testing.T, v, path string, phrases ...string) {
	t.Helper()
	text := sqRead(t, path)
	for _, p := range phrases {
		if strings.Contains(text, p) {
			t.Errorf("jaekit:violation=%s %s still says %q", v, path, p)
		}
	}
}

// The bundle contract states the Seal 0.1.23 question and scope rules next
// to the earlier per-version rules (AGENTS.md: the contract and the
// implementation change together).
func TestSealQuestionsRegression_ContractStatesRules(t *testing.T) {
	const v = "contract-questions-missing"
	sqRequire(t, v, sqContract, "### 2.2 범위 (`## 범위`)",
		"Seal 0.1.23 이상은 계획이 새로 만들 시험·검사 파일과 그 도우미 파일을 처음부터 여기에 넣어, 그 파일을 더해도 범위 확인을 묻지 않는다.",
		"목표 문서가 바꾸지 말라고 한 경로는 넣지 않는다.")
	sqRequire(t, v, sqContract, "",
		"Seal 0.1.23 이상은 baseline을 기록하기 전에, 목표 문서·계획·프로젝트 지침·코드에서 미리 알 수 있고 사용자의 말이 필요한 결정을 쓸 수 없는 도구와 승인이 필요한 예산까지 모아 한 메시지로 묻는다.",
		"사용자 확인으로만 충족할 조건이 있으면 그 조건과 끝에 확인해 달라고 할 것을 같은 메시지에서 알린다.",
		"물을 것이 없으면 아무것도 묻지 않는다.")
}
