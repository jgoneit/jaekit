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
// documents without waiting for a save agreement, and an open decision is
// listed under ## Open Decisions with Status: Draft. Both stay format tokens of
// the Spec skill, and the skill holds none of the old save-agreement wording.
func TestSpecWritesWithoutSaveAgreement(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Open Decisions", "Status: Draft")
	forbidSkillText(t, specSkillDir, "Save only after the user agrees", "that agreement covers those decisions")
}

// after writing, Spec reports what it wrote instead of pasting the full
// draft: the Spec skill holds none of the old wording that showed the full
// draft or every changed section.
func TestSpecReportsWhatItWrote(t *testing.T) {
	forbidSkillText(t, specSkillDir, "show the full draft", "show every changed section in full")
}

// the source category is a proposal stated in Spec's report everywhere Spec
// reads it, and it becomes the user's decision on acceptance or an
// implementation request, existing labels staying. The Spec skill holds none
// of the old wording of a proposal the user agreed to when saving, the SPEC
// templates name the new category, and the goal document contract says the
// same.
func TestSpecProposalBecomesDecisionOnRequest(t *testing.T) {
	forbidSkillText(t, specSkillDir,
		"a proposal of yours that the user agreed to when saving",
		"the user agreed to when saving",
		"the list of decisions shown before saving",
	)
	for _, name := range []string{"backend-feature", "cross-module-feature", "product"} {
		path := "spec/skills/spec/assets/templates/" + name + ".md"
		requireAll(t, path, "or a proposal stated in your report>")
		forbidAll(t, path, "agreed to when saving")
	}
	requireAll(t, "../contracts/goal-docs.md",
		"- 보고에 적은 Spec의 제안\n",
		"보고에 적은 Spec의 제안은 사용자가 받아들이거나 그 목표 문서로 구현을 요청하면 사용자의 결정이 된다.",
		"이미 적힌 \"Spec 제안(저장 때 동의)\" 표기는 그대로 둔다.",
	)
	forbidAll(t, "../contracts/goal-docs.md", "- 저장 때 동의를 받은 Spec의 제안")
}

// an answer or a change request edits the same goal documents, which turn
// Status: Ready once no outcome-changing decision is open: that token stays a
// format token of the Spec skill.
func TestSpecRevisesSameDocuments(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Status: Ready")
}

// after writing or editing, Spec stops: while a content decision is open
// implementation starts with a new request, and otherwise it asks the
// implement-now question. The Spec skill holds none of the old wording that
// reported after saving or took a save agreement, and the Codex manifest
// describes the stop.
func TestSpecStopsAfterWriting(t *testing.T) {
	forbidSkillText(t, specSkillDir, "After saving, report", "the reply that agrees to save")
	requireAll(t, "spec/.codex-plugin/plugin.json",
		"After writing and reporting it stops. While a decision is open, implementation starts with a new request;",
		"with nothing left to decide it asks whether to implement now")
}

// Draft means an outcome-changing decision is open, Ready means none is, and
// neither authorizes implementation. Status: Draft and Status: Ready stay
// format tokens of the Spec skill, and the goal document contract says the
// same.
func TestSpecStatusGrantsNothing(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "Status: Draft", "Status: Ready")
	requireAll(t, "../contracts/goal-docs.md",
		"`Status: Ready`는 실행 권한이 아니다.",
		"`Draft`는 결과를 바꾸는 미결정이 남았다는 뜻이고 `Ready`는 남지 않았다는 뜻이다. 문서의 설명이며 어느 쪽도 실행 권한이 아니다.",
	)
}

// Seal never takes a message asking for goal documents as an implementation
// request: its skill holds none of the old save-agreement wording.
func TestSealIgnoresDocumentRequests(t *testing.T) {
	forbidSkillText(t, sealSkillDir, "agrees to save", "save its draft", "after the goal documents are saved")
}

// a continue or implement request sent once the goal documents changed during
// a run confirms that change: the confirm spec note form stays a format token
// of the Seal skill, and the role card contract says the same.
func TestSealTakesContinueAsConfirmation(t *testing.T) {
	requireSkillTokens(t, sealSkillDir, `ha note <goal> confirm spec --quote "…"`)
	requireAll(t, "../contracts/role-card.md",
		"실행을 시작한 뒤 목표 문서가 바뀌고 사용자가 계속이나 구현을 요청하면, 그 요청이 바뀐 목표 문서의 확인이다.",
		"그 요청을 인용해 `ha note confirm spec`으로 남기고, 보고에 바뀐 조건을 밝힌다.",
	)
}

// the goal document contract §3 and §6 say the same as
// the skill body, and §6.4 defines direct save, the report, and choosing an
// unsettled writing target before writing.
func TestGoalDocsContractWritesDirectly(t *testing.T) {
	path := "../contracts/goal-docs.md"
	requireAll(t, path,
		"Spec을 부른 요청이나 질문에 답하거나 수정을 요청하는 답이 구현까지 요청해도 Spec은 목표 문서를 쓴 뒤 멈춘다.",
		"옮긴 항목은 보고의 결정 목록(§6.4)에 적고,",
		"목표 문서를 쓰기 전에 요청이 무엇인지 정하고, 보고(§6.4)에서 근거와 함께 알린다.",
		"기존 목표 문서 수정: 기존 조건 ID를 유지한다(§2.1). 어느 문서를 고칠지 요청과 대화로 정해지지 않았으면 쓰기 전에 고른다(§6.4).",
		"새 목표로 쓸지 그 문서를 고칠지 근거와 함께 제안하고 사용자의 결정대로 한다. 요청과 대화로 정해지지 않았으면 쓰기 전에 묻는다(§6.4).",
		"### 6.4 저장과 보고",
		"**작성 대상.** 같은 동작을 다루는 목표 문서가 `docs/specs/`에 있는데 요청과 대화가 작성 대상(새 목표인지 그 문서 수정인지, 또는 어느 목표 문서를 고칠지)을 정하지 않았으면, Spec은 어떤 파일도 쓰거나 고치기 전에 그 선택을 묻는다.",
		"추천하는 대상을 맨 앞에 두고 근거와 선택지마다 잃는 것을 함께 적으며(§6.5 추천 먼저), 사용자의 선택을 따른다.",
		"사용자가 대상을 지목했거나 대화에서 이미 골랐으면 다시 묻지 않는다.",
		"쓰기 전에 받는 것은 작성 대상뿐이다. 대상이 정해진 뒤 내용의 미결정은 아래처럼 `Status: Draft`로 쓰고 묻는다.",
		"**바로 저장.** 작성 대상이 정해지면 Spec은 사용자의 저장 동의를 기다리지 않고 목표 문서를 바로 쓰거나 고친다.",
		"결과를 바꾸는 미결정이 남으면 `Status: Draft`로 쓰고 그 결정을 `## Open Decisions`에 두며, 같은 보고에서 묻는다.",
		"초안 전문을 대화에 붙이지 않는다.",
		"- 요청 종류와 작은 수정인지(§6.1)\n",
		"- 쓰거나 고친 파일 경로와 바뀐 절\n",
		"- 사용자가 말하지 않았는데 Spec이 정한 결정과 출처(§6.2)\n",
		"- 완료 뒤 평가 항목으로 옮긴 항목(§3)\n",
		"- 열린 질문\n",
		"- 정합성 확인 결과(§3)\n",
		"사용자가 질문에 답하거나 수정을 요청하면 같은 목표 문서를 고치고 다시 보고한다.",
		"보고에서 구현은 새 요청으로 시작한다고 알리고 멈춘다.",
		"보고에 적은 Spec의 제안을 출처로 하는 결정이 되고 보고의 결정 목록(§6.4)에 오른다.",
	)
	forbidAll(t, path,
		"### 6.4 저장 확인",
		"저장 동의를 구할 때",
		"저장 동의는 그 결정들에 대한 동의다",
		"저장 전에 보이는 결정 목록",
		"초안을 보여 주기 전에",
		"저장 전 결정 목록",
	)
}

// the README links the usage guide, which names the goal document path
// Spec saves to and the Seal command that starts implementation, and the
// README no longer describes saving after showing a draft.
func TestGuidesDescribeDirectSave(t *testing.T) {
	requireAll(t, "../README.ko.md", "(guides/USAGE.md)")
	requireAll(t, "../guides/USAGE.md", "`docs/specs/<goal>/SPEC.md`", "$seal:seal docs/specs/<goal>")
	forbidAll(t, "../README.ko.md", "초안 전문을 보여 준 뒤 동의하면")
}

// Synthetic cases require direct save and count outside changes until Spec stops.
func TestSpecCasesUseDirectSave(t *testing.T) {
	requireAll(t, "../evals/spec-cases/same-request-handoff.md",
		"Spec이 멈출 때까지 목표 문서 밖의 변경이 0이다",
		"구현은 새 요청으로 시작한다고 알린 뒤 멈춘다",
	)
	requireAll(t, "../evals/spec-cases/long-session-release.md",
		"목표 문서를 바로 쓰고, 요청 종류, 쓴 파일 경로, Spec이 정한 결정과 출처를 보고한다",
		"Spec이 멈출 때까지 목표 문서 밖의 변경이 0이다",
	)
	requireAll(t, "../evals/spec-cases/evaluation-items.md", "보고의 결정 목록에 그 두 항목을 평가 항목으로 옮겼다")
	requireAll(t, "../evals/spec-cases/admin-order-search.md", "보고에 적은 Spec의 제안을 출처로")
	for _, name := range []string{"same-request-handoff", "long-session-release", "evaluation-items", "admin-order-search"} {
		forbidAll(t, "../evals/spec-cases/"+name+".md", "저장 동의", "저장 때 동의", "초안 전문")
	}
	requireAll(t, "../evals/spec-cases/README.md",
		"| 목표 문서 밖의 변경 | Spec이 멈출 때까지 목표 문서 밖에서 바꾸거나 실행한 것",
		"목표 문서를 쓰기 전에 저장해도 되는지 물은 것도 하나로 센다",
	)
	forbidAll(t, "../evals/spec-cases/README.md", "저장 동의를 기다리", "저장 동의 전에", "저장 전 결정 목록", "목표 문서 저장까지")

}

// a quality case holds one outcome-changing open
// decision and expects a Draft goal document with the question in the first
// reply, the same document turned Ready after the answer, and no save
// agreement; the public case list includes it.
func TestSpecCaseForDirectSave(t *testing.T) {
	path := "../evals/spec-cases/draft-then-ready.md"
	requireAll(t, path,
		"## 요청", "## 사용자 답", "## 기대 결과",
		"첫 응답에서 목표 문서를 `Status: Draft`로 쓰고",
		"`## Open Decisions`에 두며, 같은 보고에서 묻는다",
		"사용자가 답하면 같은 목표 문서를 고쳐 `Status: Ready`로 바꾸고",
		"저장 동의를 묻거나 기다리지 않는다",
		"목표 문서를 저장해도 되는지(저장 동의)",
		"구현은 새 요청으로 시작한다고 알린 뒤 멈춘다",
		"질문 상한",
	)
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
	requireAll(t, "../evals/spec-cases/README.md", "| [draft-then-ready](draft-then-ready.md) | 바로 저장 |")
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

// when goal documents in docs/specs/ cover the same behavior and the writing
// target is open, Spec asks for it before writing, and once it is settled it
// writes the documents with content decisions under ## Open Decisions and
// Status: Draft. Those stay format tokens of the Spec skill, and the skill
// holds none of the old propose-then-wait wording.
func TestSpecAsksTargetBeforeWriting(t *testing.T) {
	requireSkillTokens(t, specSkillDir, "docs/specs/", "Open Decisions", "Status: Draft")
	forbidSkillText(t, specSkillDir, "If one does, propose either a new goal or an edit to that document")
}

// a quality case has a goal document covering the same
// behavior, a request that names no writing target, and one content decision
// left once the target is set. It expects no goal document written before the
// target is chosen, only that document written as Draft with the question
// once it is, and the same document turned Ready after the answer; it is
// listed in the public case list.
func TestSpecCaseForTargetChoice(t *testing.T) {
	path := "../evals/spec-cases/target-then-draft.md"
	text := read(t, path)
	requireAll(t, path,
		"## 요청", "## 저장소 맥락", "## 사용자 답", "## 기대 결과",
		"같은 동작을 다루는 목표 문서는 `docs/specs/order-cancel/SPEC.md`",
		"- 작성 대상: ",
		"첫 응답에서 어떤 목표 문서도 쓰거나 고치지 않고 작성 대상을 묻는다",
		"`docs/specs/order-cancel/SPEC.md`만 고치고 새 목표 디렉토리를 만들지 않는다",
		"`## Open Decisions`에 두고 `Status: Draft`로 쓰며, 같은 보고에서 묻는다",
		"사용자가 답하면 같은 목표 문서를 고쳐 `Status: Ready`로 바꾸고",
		"작성 대상을 받은 뒤 다시 작성 대상을 묻는 것",
		"저장 동의를 묻거나 기다리지 않는다",
		"질문 상한",
	)
	_, request, _ := strings.Cut(text, "## 요청\n")
	request, _, _ = strings.Cut(request, "\n## ")
	for _, s := range []string{"docs/specs", "목표 문서", "새 목표", "order-cancel"} {
		if strings.Contains(request, s) {
			t.Errorf("target-then-draft.md request names the writing target: %q", s)
		}
	}
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
	requireAll(t, "../evals/spec-cases/README.md", "| [target-then-draft](target-then-draft.md) | 작성 대상 선택 |")
}
