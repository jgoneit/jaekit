package plugins

// Regression checks for review fixes of the seal-questions-up-front goal.
// They use the fixed-phrase helpers of seal_questions_up_front_test.go and
// run with the package's other tests.

import "testing"

const sqContract = "../contracts/bundle.md"

// AC-6: an answer given up front is recorded as the confirmation once the
// change it allows is committed, because a confirmation binds to the content
// as it is when recorded; the user is not asked the same thing again.
func TestSealQuestionsRegression_EarlyAnswerConfirmedAfterChange(t *testing.T) {
	const v = "early-answer-confirmation-unstated"
	sqRequire(t, v, sqBundle, sqQuestions,
		"A confirmation binds to the content it names as that content is when recorded",
		"record an answer given up front as that confirmation once the change it allows is committed, quoting the answer, and never ask the user the same thing again")
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
