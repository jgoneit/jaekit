package plugins

// These checks observe the checked tree and synthetic temporary repositories,
// never repository history or private goal documents. The fixed prose tokens
// below are an authoring contract, not a semantic or real-use evaluation.
// Only property assertions carry a violation marker; input failures are setup.

import (
	brJSON "encoding/json"
	brFmt "fmt"
	brFS "io/fs"
	brOS "os"
	brExec "os/exec"
	brPath "path/filepath"
	brReflect "reflect"
	brRegexp "regexp"
	brStrings "strings"
	brTesting "testing"

	brGoalDocs "github.com/jgoneit/jaekit/internal/goaldocs"
	brStatus "github.com/jgoneit/jaekit/internal/status"
)

// All observed tokens and their section selectors are declared here. Whitespace
// may wrap in prose tokens. Template headings and decision-line prefixes are
// structural and must appear on their own lines in the indicated section.
var (
	// AC-1, AC-2, AC-3: REVIEW locations and the decision subheadings.
	brReviewTemplate = "seal/skills/seal/assets/templates/REVIEW.md"
	brReviewHeading  = "## 중요한 미결정"
	brUserHeading    = "### 사용자의 답"
	brAssumeHeading  = "### 실행자 가정"
	brDecisionRow    = "중요한 미결정"
	brReviewDocs     = []brDoc{
		{"seal/skills/seal/references/bundle.md", "## REVIEW.md", "en"},
		{"../contracts/bundle.md", "## 4. `REVIEW.md`", "ko"},
		{"../contracts/bundle.en.md", "## 4. `REVIEW.md`", "en"},
	}
	// AC-1: the user's answer and the assumption each name their contents.
	brUserItems = map[string][]string{
		"en": {"the decision, the user's quoted answer, and where that answer is recorded"},
		"ko": {"결정, 사용자의 답 인용, 그 답이 기록된 곳"},
	}
	brAssumeItems = map[string][]string{
		"en": {"the assumption and its reason", "mark an assumption that reads a SPEC sentence"},
		"ko": {"가정과 그 이유", "SPEC 문장을 해석한 가정에는 그렇다는 표시를 붙인다"},
	}
	// AC-1: the empty-section and empty-side cases, and the order in prose.
	brEmptyDecisions = map[string][]string{
		"en": {"With no decision at all, write only one line `없음`", "Otherwise keep both subheadings and write `없음` under an empty one"},
		"ko": {"결정이 하나도 없으면 `없음` 한 줄만 둔다", "그렇지 않으면 두 소제목을 모두 두고 빈 쪽에 `없음`을 쓴다"},
	}
	brDecisionOrder = "`### 사용자의 답`, `### 실행자 가정`"
	// AC-1: retain today's decision scope and the report's repeated assumptions.
	brDecisionScope = [][]string{
		{"Open decisions in the goal documents and new outcome-changing ones", "an explicit assumption that the report repeats"},
		{"목표 문서의 `## Open Decisions`에 남은 결정, 그리고 계획 중 새로 드러난 결과를 바꾸는 결정", "가정은 보고서에 다시 나온다"},
		{"Decisions left in the goal documents' `## Open Decisions`, and outcome-changing decisions that came up newly during planning", "Assumptions appear again in the report"},
	}
	// SPEC AC-2: version boundary, earlier forms and the fixed no-rewrite sentence.
	brReviewBoundary = map[string][]string{
		"en": {
			"The split REVIEW form applies to goals started with seal 0.1.29 or later",
			"A goal started with an earlier Seal keeps the REVIEW form it started with",
			"An existing `REVIEW.md` is never rewritten into the split form",
		},
		"ko": {
			"나뉜 REVIEW 형식은 seal 0.1.29 이상으로 시작한 목표에 쓴다",
			"그 전 Seal로 시작한 목표의 REVIEW는 시작할 때의 형식을 유지한다",
			"이미 있는 `REVIEW.md`는 나뉜 형식으로 고쳐 쓰지 않는다",
		},
	}
	// AC-3: today's complete level-two heading list, in order.
	brReviewHeadings = []string{"## 요구사항 대응", "## 불필요한 task", "## 문서 충돌", "## 중요한 미결정", "## 검증 방법과 환경", "## 독립 검토", "## 결론"}
	// AC-3: today's exact scope/trust-boundary phrases, in template/doc order.
	brTrustPhrases = []string{
		"a reading that keeps a component's declared scope or trust boundary is such an assumption, not a question",
		"A reading that keeps a component's declared scope or trust boundary is such an assumption, not a question",
		"구성 요소가 선언한 범위나 신뢰 경계를 지키는 해석은 질문이 아니라 이런 가정이다",
		"A reading that keeps a component's declared scope or trust boundary is not a question but such an assumption",
	}
	// AC-4, AC-5: decision rules are read only in these named sections.
	brSpecSkill    = "spec/skills/spec/SKILL.md"
	brSpecLanguage = "## Language"
	brSpecDocs     = []brDoc{
		{brSpecSkill, "## Output contract", "en"},
		{"spec/skills/spec/references/goal-docs.md", "## SPEC.md", "en"},
		{"../contracts/goal-docs.md", "### 6.2 결정의 출처", "ko"},
		{"../contracts/goal-docs.en.md", "### 6.2 Sources of decisions", "en"},
	}
	// AC-4: marker placement, source priority, trailing source and changes.
	brMarkerRules = map[string][]string{
		"en": {
			"In new SPEC documents, put one marker at the very start of each decision sentence",
			"`[사용자]` when the source includes the user's words",
			"`[제안]` when there are no user's words and the source includes a Spec proposal stated in the report",
			"`[기존]` otherwise: only a repository file, an existing goal document, or a review finding identifier",
			"keep the detailed source at the end",
			"when the source changes, the marker changes with it",
			"the markers are contract tokens in every language",
		},
		"ko": {
			"새 SPEC 문서에서는 결정 문장 맨 앞에 표시 하나를 둔다",
			"출처에 사용자의 말이 있으면 `[사용자]`",
			"사용자의 말 없이 보고에 적은 Spec의 제안이 있으면 `[제안]`",
			"그 밖에 저장소 파일, 기존 목표 문서, 검토 지적의 식별자만 있으면 `[기존]`",
			"자세한 출처는 끝에 그대로 둔다",
			"출처가 바뀌면 표시도 함께 바뀐다",
			"표시는 언어와 관계없는 계약 토큰이다",
		},
	}
	// AC-4, AC-6: exact template decision prefix; each real marker is exercised.
	brMarkers           = []string{"[사용자]", "[제안]", "[기존]"}
	brMarkerPlaceholder = "<[사용자], [제안] or [기존]>"
	brSourceLabel       = "Source:"
	brDecisionHeading   = "## Decisions and reasons"
	brSpecTemplates     = []string{
		"spec/skills/spec/assets/templates/backend-feature.md",
		"spec/skills/spec/assets/templates/cross-module-feature.md",
		"spec/skills/spec/assets/templates/product.md",
	}
	// AC-5: existing decisions, user-selected additions and existing labels.
	brExistingRules = map[string][]string{
		"en": {"Decisions without a marker remain valid", "When editing an existing document, add no marker unless the user decides", "Existing source labels stay unchanged"},
		"ko": {"표시 없는 결정도 그대로 유효하다", "기존 문서를 고칠 때 사용자가 정하지 않으면 표시를 더하지 않는다", "이미 있는 출처 표기는 바꾸지 않는다"},
	}
	// AC-5: the exact forbidden patterns in TestSpecPluginKnowsNothingOfSeal.
	brForbidden = []*brRegexp.Regexp{
		brRegexp.MustCompile(`(?i)\bseal\b`),
		brRegexp.MustCompile(`\bha (lint|start|check|note|status|done|log)\b`),
		brRegexp.MustCompile(`PLAN\.md|REVIEW\.md|PROGRESS\.md|runs\.jsonl`),
		brRegexp.MustCompile(`\b(needs_user|budget_exhausted|incomplete)\b`),
	}
	// AC-6: template headers, criteria and the actual CLI's lint success output.
	brStatusKey       = "Status:"
	brReady           = "Ready"
	brFormatKey       = "Criteria-Format:"
	brAssuranceKey    = "Assurance:"
	brCriteriaHeading = "## Acceptance Criteria"
	brOpenHeading     = "## Open Decisions"
	brRequiredIDs     = []string{"AC-1", "AC-2"}
	brCriterionID     = brRegexp.MustCompile(`(?m)^- \*\*(AC-[0-9]+)\*\* `)
	brPlaceholder     = brRegexp.MustCompile(`<[^<>\n]+>`)
	brLintOK          = "ok: no lint problems"
	brTierOutput      = "assurance tier: "
	brMaintainKind    = "maintain"
	// AC-7, AC-8: PR rules belong to the review-summary sections only.
	brSummaryDocs = []brDoc{
		{"seal/skills/seal/references/bundle.md", "## Review summary", "en"},
		{"../contracts/bundle.md", "### 5.4 검수 요약", "ko"},
		{"../contracts/bundle.en.md", "### 5.4 Review summary", "en"},
	}
	// AC-7: source, mandatory contents, project additions and private omissions.
	brPRContents = map[string][]string{
		"en": {
			"When writing or changing a completed goal's PR description, build it from that goal's `## 검수 요약`",
			"Include the completion record's commit, the changed behavior and files, and an evidence summary",
			"every item of `### 직접 볼 것`, including deferred findings with their issues and findings without a disposition",
			"include `### 보지 않은 것`",
			"Add the items the project instructions require in a PR description",
			"Leave out paths and links to goal documents, execution bundles and review records that Git does not track",
			"leave out the raw user conversation",
			"instead, describe the content briefly or say it is in local records",
		},
		"ko": {
			"완료한 목표의 PR 설명을 쓰거나 고칠 때 그 목표의 `## 검수 요약`에서 만든다",
			"완료 기록의 commit, 바뀐 동작과 파일, 근거 요약을 넣는다",
			"`### 직접 볼 것`의 모든 항목을 넣고, 보류한 지적과 그 이슈 및 처리 없는 지적도 포함한다",
			"`### 보지 않은 것`을 넣는다",
			"프로젝트 지침이 PR 설명에 요구하는 항목도 넣는다",
			"Git이 추적하지 않는 목표 문서, 실행 묶음, 검토 기록의 경로와 링크는 넣지 않는다",
			"사용자 대화의 원문은 넣지 않는다",
			"대신 내용을 짧게 적거나 로컬 기록에 있다고 적는다",
		},
	}
	// AC-8: user-request boundary, no additional authority, no erased findings.
	brPRLimits = map[string][]string{
		"en": {"Only when the user asked for a PR", "This guidance adds no authority to create, change or push a PR, or answer a review", "Never drop a `### 직접 볼 것` item or rewrite it as resolved"},
		"ko": {"사용자가 PR을 요청했을 때만 적용한다", "PR을 만들거나 고치거나 올리거나 리뷰에 답할 권한을 더하지 않는다", "`### 직접 볼 것` 항목을 빼거나 해결된 것처럼 바꿔 쓰지 않는다"},
	}
	// AC-9: both plugin versions and the Seal change paragraph's Core boundary.
	brVersions     = []brVersion{{"seal", "0.1.29"}, {"spec", "0.1.14"}}
	brVersionKey   = "version"
	brPluginsKey   = "plugins"
	brPluginName   = "name"
	brMarketplace  = "../.claude-plugin/marketplace.json"
	brHaReference  = "seal/skills/seal/references/ha.md"
	brHaHeading    = "# ha — Seal Core commands"
	brSealChange   = "Seal 0.1.29"
	brNoCoreChange = "no Core change"
	brFrontVersion = brRegexp.MustCompile(`^\s*version:\s*["']?([^"'\s]+)["']?\s*$`)
)

type brDoc struct{ brPath, brHeading, brLang string }
type brVersion struct{ brName, brWant string }

func brRead(brT *brTesting.T, brFile string) string {
	brT.Helper()
	brData, brErr := brOS.ReadFile(brFile)
	if brErr != nil {
		brT.Fatalf("setup: cannot read %s: %v", brFile, brErr)
	}
	return string(brData)
}

func brNorm(brText string) string { return brStrings.Join(brStrings.Fields(brText), " ") }

func brLevel(brLine string) int {
	brN := len(brLine) - len(brStrings.TrimLeft(brLine, "#"))
	if brN == 0 || brN > 6 || brN >= len(brLine) || brLine[brN] != ' ' {
		return 0
	}
	return brN
}

// Fenced examples never supply a document's structural heading.
func brFence(brOpen, brLine string) (string, bool) {
	brTrim := brStrings.TrimSpace(brLine)
	if len(brTrim) >= 3 && (brTrim[0] == '`' || brTrim[0] == '~') {
		brN := len(brTrim) - len(brStrings.TrimLeft(brTrim, brTrim[:1]))
		if brN >= 3 {
			if brOpen == "" {
				return brTrim[:brN], true
			}
			if brTrim[0] == brOpen[0] && brN >= len(brOpen) && brStrings.TrimSpace(brTrim[brN:]) == "" {
				return "", true
			}
		}
	}
	return brOpen, brOpen != ""
}

// The returned section includes its heading, ending at the next same/higher
// heading. Callers can replace a synthetic section without changing neighbors.
func brSection(brText, brHeading string) (string, bool) {
	brLines := brStrings.Split(brText, "\n")
	brStart, brOpen := -1, ""
	for brI, brLine := range brLines {
		var brFenced bool
		brOpen, brFenced = brFence(brOpen, brLine)
		if brFenced {
			continue
		}
		if brStart >= 0 && brLevel(brLine) > 0 && brLevel(brLine) <= brLevel(brHeading) {
			return brStrings.Join(brLines[brStart:brI], "\n"), true
		}
		if brStart < 0 && brStrings.TrimRight(brLine, " \t\r") == brHeading {
			brStart = brI
		}
	}
	if brStart < 0 {
		return "", false
	}
	return brStrings.Join(brLines[brStart:], "\n"), true
}

// Each heading must occur exactly once as an unfenced whole line.
func brHeadingOrder(brText, brFirst, brSecond string) bool {
	brFirstLine, brSecondLine, brOpen := -1, -1, ""
	for brI, brLine := range brStrings.Split(brText, "\n") {
		var brFenced bool
		brOpen, brFenced = brFence(brOpen, brLine)
		if brFenced {
			continue
		}
		switch brStrings.TrimRight(brLine, " \t\r") {
		case brFirst:
			if brFirstLine >= 0 {
				return false
			}
			brFirstLine = brI
		case brSecond:
			if brSecondLine >= 0 {
				return false
			}
			brSecondLine = brI
		}
	}
	return brFirstLine >= 0 && brSecondLine > brFirstLine
}

func brNeed(brMissing *[]string, brWhere, brText string, brTokens ...string) {
	for _, brToken := range brTokens {
		if !brStrings.Contains(brNorm(brText), brNorm(brToken)) {
			*brMissing = append(*brMissing, brFmt.Sprintf("%s lacks %q", brWhere, brToken))
		}
	}
}

func brDocSection(brT *brTesting.T, brMissing *[]string, brD brDoc) string {
	brT.Helper()
	brText, brOK := brSection(brRead(brT, brD.brPath), brD.brHeading)
	if !brOK {
		*brMissing = append(*brMissing, brFmt.Sprintf("%s lacks section %q", brD.brPath, brD.brHeading))
	}
	return brText
}

func brNeedDocs(brT *brTesting.T, brMissing *[]string, brDocs []brDoc, brTokens map[string][]string) {
	brT.Helper()
	for _, brD := range brDocs {
		brText := brDocSection(brT, brMissing, brD)
		brNeed(brMissing, brD.brPath+" "+brD.brHeading, brText, brTokens[brD.brLang]...)
	}
}

// One property assertion per test collects all missing tokens and differences.
func brFail(brT *brTesting.T, brViolation string, brMissing []string) {
	brT.Helper()
	if len(brMissing) > 0 {
		brT.Errorf("jaekit:violation=%s %s", brViolation, brStrings.Join(brMissing, "; "))
	}
}

func brRow(brText string) string {
	for _, brLine := range brStrings.Split(brText, "\n") {
		brCells := brStrings.Split(brLine, "|")
		if len(brCells) >= 3 && brStrings.Contains(brCells[1], brDecisionRow) {
			return brLine
		}
	}
	return ""
}

func TestBundleReadabilityAC1_ReviewDecisionsSplit(brT *brTesting.T) {
	var brMissing []string
	brSectionText := brDocSection(brT, &brMissing, brDoc{brReviewTemplate, brReviewHeading, "en"})
	brUser, _ := brSection(brSectionText, brUserHeading)
	brAssume, _ := brSection(brSectionText, brAssumeHeading)
	if !brHeadingOrder(brSectionText, brUserHeading, brAssumeHeading) {
		brMissing = append(brMissing, brReviewTemplate+" must have each subheading once, with the user's answer before the assumption inside "+brReviewHeading)
	}
	brNeed(&brMissing, brReviewTemplate+" "+brUserHeading, brUser, brUserItems["en"]...)
	brNeed(&brMissing, brReviewTemplate+" "+brAssumeHeading, brAssume, brAssumeItems["en"]...)
	brNeed(&brMissing, brReviewTemplate+" "+brReviewHeading, brSectionText, brEmptyDecisions["en"]...)
	for brI, brD := range brReviewDocs {
		brText := brDocSection(brT, &brMissing, brD)
		brWhere := brD.brPath + " " + brD.brHeading
		brNeed(&brMissing, brWhere, brText, brDecisionOrder)
		brNeed(&brMissing, brWhere, brText, brUserItems[brD.brLang]...)
		brNeed(&brMissing, brWhere, brText, brAssumeItems[brD.brLang]...)
		brNeed(&brMissing, brWhere, brText, brEmptyDecisions[brD.brLang]...)
		brNeed(&brMissing, brWhere+" decision row", brRow(brText), brDecisionScope[brI]...)
	}
	brFail(brT, "review-decisions-not-split", brMissing)
}

func TestBundleReadabilityAC2_ReviewFormatBoundary(brT *brTesting.T) {
	var brMissing []string
	for _, brD := range brReviewDocs {
		brText := brDocSection(brT, &brMissing, brD)
		brNeed(&brMissing, brD.brPath+" "+brD.brHeading, brText, brReviewBoundary[brD.brLang]...)
	}
	brFail(brT, "review-format-boundary-missing", brMissing)
}

func TestBundleReadabilityAC3_ReviewKept(brT *brTesting.T) {
	brText := brRead(brT, brReviewTemplate)
	var brHeadings, brMissing []string
	brOpen := ""
	for _, brLine := range brStrings.Split(brText, "\n") {
		var brFenced bool
		brOpen, brFenced = brFence(brOpen, brLine)
		if !brFenced && brLevel(brLine) == 2 {
			brHeadings = append(brHeadings, brStrings.TrimRight(brLine, " \t\r"))
		}
	}
	if !brReflect.DeepEqual(brHeadings, brReviewHeadings) {
		brMissing = append(brMissing, brFmt.Sprintf("REVIEW headings = %q, want %q", brHeadings, brReviewHeadings))
	}
	brSec, brOK := brSection(brText, brReviewHeading)
	if !brOK {
		brMissing = append(brMissing, brReviewTemplate+" lacks "+brReviewHeading)
	}
	brNeed(&brMissing, brReviewTemplate+" "+brReviewHeading, brSec, brTrustPhrases[0])
	for brI, brD := range brReviewDocs {
		brText = brDocSection(brT, &brMissing, brD)
		brNeed(&brMissing, brD.brPath+" decision row", brRow(brText), brTrustPhrases[brI+1])
	}
	if len(brMissing) > 0 {
		brT.Errorf("%s", brStrings.Join(brMissing, "; "))
	}
}

// Return the sole decision placeholder line, not a Source: in another section.
func brDecisionLine(brText string) (string, bool) {
	brSec, brOK := brSection(brText, brDecisionHeading)
	if !brOK {
		return "", false
	}
	brFound := ""
	for _, brLine := range brStrings.Split(brSec, "\n") {
		if brStrings.Contains(brLine, brSourceLabel) {
			if brFound != "" {
				return "", false
			}
			brFound = brLine
		}
	}
	brPrefix := "- " + brMarkerPlaceholder + " "
	if !brStrings.HasPrefix(brFound, brPrefix) || brStrings.Count(brFound, brMarkerPlaceholder) != 1 {
		return brFound, false
	}
	brTail := brStrings.TrimSpace(brStrings.TrimPrefix(brFound, brPrefix))
	brSlots := brPlaceholder.FindAllStringIndex(brTail, -1)
	if len(brSlots) != 2 || brSlots[0][0] != 0 {
		return brFound, false
	}
	return brFound, brStrings.TrimSpace(brTail[brSlots[0][1]:brSlots[1][0]]) == brSourceLabel &&
		brStrings.TrimSpace(brTail[brSlots[1][1]:]) == ""
}

func TestBundleReadabilityAC4_DecisionMarkers(brT *brTesting.T) {
	var brMissing []string
	brNeedDocs(brT, &brMissing, brSpecDocs, brMarkerRules)
	brText := brDocSection(brT, &brMissing, brDoc{brSpecSkill, brSpecLanguage, "en"})
	brNeed(&brMissing, brSpecSkill+" "+brSpecLanguage, brText, brMarkers...)
	for _, brFile := range brSpecTemplates {
		if _, brOK := brDecisionLine(brRead(brT, brFile)); !brOK {
			brMissing = append(brMissing, brFmt.Sprintf("%s %s needs one decision line starting %q, then a decision placeholder, then %q and its trailing placeholder", brFile, brDecisionHeading, "- "+brMarkerPlaceholder+" ", brSourceLabel))
		}
	}
	brFail(brT, "decision-marker-missing", brMissing)
}

func TestBundleReadabilityAC5_ExistingDocsKept(brT *brTesting.T) {
	var brMissing []string
	brNeedDocs(brT, &brMissing, brSpecDocs, brExistingRules)
	brErr := brPath.WalkDir("spec", func(brFile string, brEntry brFS.DirEntry, brErr error) error {
		if brErr != nil {
			return brErr
		}
		if brEntry.IsDir() {
			return nil
		}
		brData, brErr := brOS.ReadFile(brFile)
		if brErr != nil {
			return brErr
		}
		for _, brPattern := range brForbidden {
			if brMatch := brPattern.FindString(string(brData)); brMatch != "" {
				brMissing = append(brMissing, brFmt.Sprintf("%s mentions forbidden %q", brFile, brMatch))
			}
		}
		return nil
	})
	if brErr != nil {
		brT.Fatalf("setup: cannot read Spec files: %v", brErr)
	}
	brFail(brT, "existing-docs-rule-missing", brMissing)
}

func brTempDir(brT *brTesting.T) string {
	brT.Helper()
	brDir, brErr := brOS.MkdirTemp("", "bundle-readability-")
	if brErr != nil {
		brT.Fatalf("setup: temporary directory: %v", brErr)
	}
	brT.Cleanup(func() {
		if brErr := brOS.RemoveAll(brDir); brErr != nil {
			brT.Errorf("setup: removing temporary directory: %v", brErr)
		}
	})
	return brDir
}

// Normal command exits are returned so lint/status can judge their output;
// inability to start, or termination by a signal (negative exit), is setup.
func brRun(brT *brTesting.T, brDir string, brEnv []string, brName string, brArgs ...string) (string, int) {
	brT.Helper()
	brCmd := brExec.Command(brName, brArgs...)
	brCmd.Dir, brCmd.Env = brDir, brEnv
	brOut, brErr := brCmd.CombinedOutput()
	if brErr == nil {
		return string(brOut), 0
	}
	if brExit, brOK := brErr.(*brExec.ExitError); brOK && brExit.ExitCode() >= 0 {
		return string(brOut), brExit.ExitCode()
	}
	brT.Fatalf("setup: %s %v did not run to a normal exit: %v\n%s", brName, brArgs, brErr, brOut)
	return "", -1
}

func brMustRun(brT *brTesting.T, brDir string, brEnv []string, brName string, brArgs ...string) string {
	brT.Helper()
	brOut, brCode := brRun(brT, brDir, brEnv, brName, brArgs...)
	if brCode != 0 {
		brT.Fatalf("setup: %s %v exited %d\n%s", brName, brArgs, brCode, brOut)
	}
	return brOut
}

func brWrite(brT *brTesting.T, brFile, brText string) {
	brT.Helper()
	if brErr := brOS.MkdirAll(brPath.Dir(brFile), 0o755); brErr != nil {
		brT.Fatalf("setup: cannot create fixture directory: %v", brErr)
	}
	if brErr := brOS.WriteFile(brFile, []byte(brText), 0o644); brErr != nil {
		brT.Fatalf("setup: cannot write fixture %s: %v", brFile, brErr)
	}
}

func brHeader(brText, brKey string) string {
	for _, brLine := range brStrings.Split(brText, "\n") {
		if brLevel(brLine) == 2 {
			break
		}
		if brStrings.HasPrefix(brLine, brKey) {
			return brStrings.TrimSpace(brStrings.TrimPrefix(brLine, brKey))
		}
	}
	return ""
}

func brTemplateHeaders(brMissing *[]string, brWhere, brText string) bool {
	brOK := true
	for _, brKey := range []string{brStatusKey, brFormatKey, brAssuranceKey} {
		if brHeader(brText, brKey) == "" {
			*brMissing = append(*brMissing, brWhere+" lacks its "+brKey+" header")
			brOK = false
		}
	}
	return brOK
}

func brSyntheticSpec(brText, brLine, brMarker string) string {
	// Replace the marker before filling other angle-bracket placeholders.
	brText = brStrings.Replace(brText, brLine, brStrings.Replace(brLine, brMarkerPlaceholder, brMarker, 1), 1)
	brText = brPlaceholder.ReplaceAllString(brText, "synthetic")
	brLines := brStrings.Split(brText, "\n")
	for brI, brLine := range brLines {
		if brLevel(brLine) == 2 {
			break
		}
		if brStrings.HasPrefix(brLine, brStatusKey) {
			brLines[brI] = brStatusKey + " " + brReady
		}
	}
	brText = brStrings.Join(brLines, "\n")
	if brOpen, brOK := brSection(brText, brOpenHeading); brOK {
		brText = brStrings.Replace(brText, brOpen, brOpenHeading+"\n\n없음\n", 1)
	}
	return brText
}

type brReading struct {
	brLint     string
	brLintCode int
	brReport   brStatus.Report
	brGoal     *brGoalDocs.Goal
	brFormat   string
}

// The minimal PLAN uses the existing condition/task table forms, with one
// maintain row per actual template criterion. REVIEW/PROGRESS are the public
// empty-input example's forms. No verification or recording command is run.
func brReadFixture(brT *brTesting.T, brHa, brSpec string, brIDs []string) brReading {
	brT.Helper()
	brDir := brTempDir(brT)
	brGoalDir := "goal"
	var brEnv []string
	for _, brValue := range brOS.Environ() {
		if !brStrings.HasPrefix(brValue, "GIT_") && !brStrings.HasPrefix(brValue, "XDG_CONFIG_HOME=") {
			brEnv = append(brEnv, brValue)
		}
	}
	brEnv = append(brEnv, "XDG_CONFIG_HOME="+brTempDir(brT), "GIT_CONFIG_GLOBAL="+brOS.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid", "GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
	brPlan := "# PLAN — synthetic\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n"
	for _, brID := range brIDs {
		brPlan += brFmt.Sprintf("| %s | %s | `git --version` | — | T001 |\n", brID, brMaintainKind)
	}
	brPlan += "\n## 범위\n- 바꿀 수 있는 경로: `src/**` (SPEC: Goal)\n- 테스트 경로: (기본값)\n- 유지할 동작: synthetic\n\n" +
		"## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
		"| T001 | synthetic | " + brStrings.Join(brIDs, ", ") + " | — | — | 예 | — |\n\n" +
		"## 예산\n- 검증 실행: 10\n- 경과 시간: 1시간\n- 비용: 미관측\n\n## 실행 환경\n- git: `git --version`\n"
	brWrite(brT, brPath.Join(brDir, brGoalDir, "SPEC.md"), brSpec)
	brWrite(brT, brPath.Join(brDir, brGoalDir, "PLAN.md"), brPlan)
	for _, brName := range []string{"REVIEW.md", "PROGRESS.md"} {
		brWrite(brT, brPath.Join(brDir, brGoalDir, brName), brRead(brT, brPath.Join("..", "examples", "empty-input", brName)))
	}
	brWrite(brT, brPath.Join(brDir, "src", "synthetic.txt"), "synthetic\n")
	for _, brArgs := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "commit.gpgsign=false", "-c", "core.hooksPath=" + brOS.DevNull, "commit", "-qm", "synthetic fixture"}} {
		brMustRun(brT, brDir, brEnv, "git", brArgs...)
	}
	var brResult brReading
	brResult.brLint, brResult.brLintCode = brRun(brT, brDir, brEnv, brHa, "lint", brGoalDir)
	if brResult.brLintCode != 0 && brResult.brLintCode != 1 {
		brT.Fatalf("setup: ha lint exited %d\n%s", brResult.brLintCode, brResult.brLint)
	}
	brOut, brCode := brRun(brT, brDir, brEnv, brHa, "status", brGoalDir, "--format", "json")
	if brCode < 0 || brCode > 4 {
		brT.Fatalf("setup: ha status exited %d\n%s", brCode, brOut)
	}
	if brErr := brJSON.Unmarshal([]byte(brOut), &brResult.brReport); brErr != nil {
		brT.Fatalf("setup: ha status is not JSON: %v\n%s", brErr, brOut)
	}
	if brResult.brReport.Schema == "" || brResult.brReport.ExitCode != brCode {
		brT.Fatalf("setup: ha status output does not identify its schema and exit code\n%s", brOut)
	}
	var brErr error
	brResult.brGoal, brErr = brGoalDocs.Load(brDir, brGoalDir)
	if brErr != nil {
		brT.Fatalf("setup: cannot read the synthetic goal: %v", brErr)
	}
	brResult.brFormat = brHeader(brSpec, brFormatKey)
	return brResult
}

func brCompareReads(brMissing *[]string, brWhere, brSource string, brMarked, brPlain brReading, brIDs []string) {
	if brMarked.brLintCode != 0 || !brStrings.Contains(brMarked.brLint, brLintOK) || brMarked.brLint != brPlain.brLint || brMarked.brLintCode != brPlain.brLintCode {
		*brMissing = append(*brMissing, brFmt.Sprintf("%s lint differs or reports problems: marked %d %q; plain %d %q", brWhere, brMarked.brLintCode, brMarked.brLint, brPlain.brLintCode, brPlain.brLint))
	}
	// status/v1 deliberately does not expose SPEC Status, assurance tier or
	// criteria-format selection. Lint supplies the tier; the same production
	// document loader supplies Status and interpreted criteria. Preserve and
	// compare the format header, and compare its parsed bodies/IDs as well.
	brTier := brHeader(brSource, brAssuranceKey)
	if brMarked.brGoal.Status != brReady || brPlain.brGoal.Status != brReady ||
		brMarked.brGoal.Assurance != brTier || brPlain.brGoal.Assurance != brTier ||
		!brStrings.Contains(brMarked.brLint, brTierOutput+brTier) ||
		brMarked.brFormat != brHeader(brSource, brFormatKey) || brMarked.brFormat != brPlain.brFormat ||
		!brReflect.DeepEqual(brMarked.brGoal.Criteria, brPlain.brGoal.Criteria) {
		*brMissing = append(*brMissing, brWhere+" changed Status, assurance tier, criteria format or interpreted criteria")
	}
	if len(brMarked.brGoal.Problems) != 0 || len(brPlain.brGoal.Problems) != 0 || len(brMarked.brReport.Lint) != 0 || len(brPlain.brReport.Lint) != 0 {
		*brMissing = append(*brMissing, brWhere+" has document or bundle lint problems")
	}
	if brMarked.brReport.Status != brPlain.brReport.Status || brMarked.brReport.Assurance != brPlain.brReport.Assurance ||
		!brReflect.DeepEqual(brMarked.brReport.Criteria, brPlain.brReport.Criteria) {
		*brMissing = append(*brMissing, brWhere+" status or criterion readings differ between marked and unmarked SPEC")
	}
	for _, brRead := range []brReading{brMarked, brPlain} {
		var brGot []string
		for _, brCriterion := range brRead.brReport.Criteria {
			brGot = append(brGot, brCriterion.ID)
			if brCriterion.Kind != brMaintainKind {
				*brMissing = append(*brMissing, brFmt.Sprintf("%s %s kind = %q, want %s", brWhere, brCriterion.ID, brCriterion.Kind, brMaintainKind))
			}
		}
		if !brReflect.DeepEqual(brGot, brIDs) {
			*brMissing = append(*brMissing, brFmt.Sprintf("%s status IDs = %q, want %q", brWhere, brGot, brIDs))
		}
	}
}

func TestBundleReadabilityAC6_MarkedTemplatesRead(brT *brTesting.T) {
	brHa := brPath.Join(brTempDir(brT), "ha")
	brMustRun(brT, "..", brOS.Environ(), "go", "build", "-o", brHa, "./cmd/ha")
	var brMissing []string
	for brI, brFile := range brSpecTemplates {
		brSource := brRead(brT, brFile)
		if !brTemplateHeaders(&brMissing, brFile, brSource) {
			continue
		}
		brLine, brOK := brDecisionLine(brSource)
		if !brOK {
			brMissing = append(brMissing, brFile+" lacks the decision-line marker placeholder "+brMarkerPlaceholder)
			continue
		}
		brMarked := brSyntheticSpec(brSource, brLine, brMarkers[brI])
		// Remove only the selected real marker on that same decision line.
		brPlain := brStrings.Replace(brMarked, "- "+brMarkers[brI]+" ", "- ", 1)
		brCriteria, brOK := brSection(brMarked, brCriteriaHeading)
		var brIDs []string
		for _, brMatch := range brCriterionID.FindAllStringSubmatch(brCriteria, -1) {
			brIDs = append(brIDs, brMatch[1])
		}
		if !brOK || len(brIDs) < 2 || brIDs[0] != brRequiredIDs[0] || brIDs[1] != brRequiredIDs[1] {
			brT.Fatalf("setup: %s cannot supply the synthetic AC-1 and AC-2 criteria", brFile)
		}
		brMarkedRead := brReadFixture(brT, brHa, brMarked, brIDs)
		brPlainRead := brReadFixture(brT, brHa, brPlain, brIDs)
		brCompareReads(&brMissing, brFile, brSource, brMarkedRead, brPlainRead, brIDs)
	}
	brFail(brT, "marked-template-not-read", brMissing)
}

func TestBundleReadabilityAC7_PRDescription(brT *brTesting.T) {
	var brMissing []string
	brNeedDocs(brT, &brMissing, brSummaryDocs, brPRContents)
	brFail(brT, "pr-description-guidance-missing", brMissing)
}

func TestBundleReadabilityAC8_PRLimits(brT *brTesting.T) {
	var brMissing []string
	brNeedDocs(brT, &brMissing, brSummaryDocs, brPRLimits)
	brFail(brT, "pr-description-limits-missing", brMissing)
}

func TestBundleReadabilityAC9_Versions(brT *brTesting.T) {
	var brMissing []string
	var brMarket map[string]brJSON.RawMessage
	if brErr := brJSON.Unmarshal([]byte(brRead(brT, brMarketplace)), &brMarket); brErr != nil {
		brT.Fatalf("setup: marketplace JSON: %v", brErr)
	}
	var brPlugins []map[string]brJSON.RawMessage
	if brErr := brJSON.Unmarshal(brMarket[brPluginsKey], &brPlugins); brErr != nil {
		brT.Fatalf("setup: marketplace plugins JSON: %v", brErr)
	}
	for _, brVersion := range brVersions {
		for _, brHost := range []string{".claude-plugin", ".codex-plugin"} {
			brFile := brPath.Join(brVersion.brName, brHost, "plugin.json")
			var brManifest map[string]brJSON.RawMessage
			if brErr := brJSON.Unmarshal([]byte(brRead(brT, brFile)), &brManifest); brErr != nil {
				brT.Fatalf("setup: %s manifest JSON: %v", brFile, brErr)
			}
			brGot := brJSONString(brT, brFile, brManifest, brVersionKey)
			if brGot != brVersion.brWant {
				brMissing = append(brMissing, brFmt.Sprintf("%s version = %q, want %s", brFile, brGot, brVersion.brWant))
			}
		}
		brFound := 0
		for _, brPlugin := range brPlugins {
			if brJSONString(brT, brMarketplace, brPlugin, brPluginName) == brVersion.brName {
				brFound++
				brGot := brJSONString(brT, brMarketplace, brPlugin, brVersionKey)
				if brGot != brVersion.brWant {
					brMissing = append(brMissing, brFmt.Sprintf("marketplace %s version = %q, want %s", brVersion.brName, brGot, brVersion.brWant))
				}
			}
		}
		if brFound != 1 {
			brMissing = append(brMissing, brFmt.Sprintf("marketplace has %d entries for %s, want one", brFound, brVersion.brName))
		}
		brFile := brPath.Join(brVersion.brName, "skills", brVersion.brName, "SKILL.md")
		brParts := brStrings.SplitN(brStrings.ReplaceAll(brRead(brT, brFile), "\r\n", "\n"), "---\n", 3)
		if len(brParts) != 3 || brParts[0] != "" {
			brT.Fatalf("setup: %s has no readable front matter", brFile)
		}
		var brFoundVersions []string
		for _, brLine := range brStrings.Split(brParts[1], "\n") {
			if brMatch := brFrontVersion.FindStringSubmatch(brLine); brMatch != nil {
				brFoundVersions = append(brFoundVersions, brMatch[1])
			}
		}
		if len(brFoundVersions) != 1 || brFoundVersions[0] != brVersion.brWant {
			brMissing = append(brMissing, brFmt.Sprintf("%s front matter versions = %q, want %s", brFile, brFoundVersions, brVersion.brWant))
		}
	}
	brText := brDocSection(brT, &brMissing, brDoc{brHaReference, brHaHeading, "en"})
	brFound := false
	for _, brParagraph := range brStrings.Split(brText, "\n\n") {
		if brStrings.HasPrefix(brStrings.TrimSpace(brParagraph), brSealChange+" ") {
			brFound = true
			brNeed(&brMissing, brHaReference+" "+brSealChange+" paragraph", brParagraph, brNoCoreChange)
		}
	}
	if !brFound {
		brMissing = append(brMissing, brHaReference+" lacks a paragraph starting "+brSealChange)
	}
	brFail(brT, "versions-not-raised", brMissing)
}

func brJSONString(brT *brTesting.T, brFile string, brObject map[string]brJSON.RawMessage, brKey string) string {
	brT.Helper()
	brRaw, brOK := brObject[brKey]
	if !brOK {
		return ""
	}
	var brValue string
	if brErr := brJSON.Unmarshal(brRaw, &brValue); brErr != nil {
		brT.Fatalf("setup: %s %s is not a JSON string: %v", brFile, brKey, brErr)
	}
	return brValue
}

func TestBundleReadabilityHelpersHeadingOrder(brT *brTesting.T) {
	brOrdered := brUserHeading + "\n\n" + brAssumeHeading
	brReversed := brAssumeHeading + "\n\n" + brUserHeading
	brMention := "Use `" + brUserHeading + "`, then `" + brAssumeHeading + "`."
	for _, brCase := range []struct {
		brName, brBody string
		brWant         bool
	}{
		{"ordered headings", brOrdered, true},
		{"prose mentions only", brMention, false},
		{"prose before reversed headings", brMention + "\n" + brReversed, false},
		{"duplicate user heading", brOrdered + "\n" + brUserHeading, false},
		{"duplicate assumption heading", brOrdered + "\n" + brAssumeHeading, false},
		{"missing user heading", brAssumeHeading, false},
		{"missing assumption heading", brUserHeading, false},
		{"indented mentions", " " + brUserHeading + "\n " + brAssumeHeading, false},
		{"fenced headings only", "```md\n" + brOrdered + "\n```", false},
		{"fenced reverse before headings", "~~~md\n" + brReversed + "\n~~~\n" + brOrdered, true},
	} {
		brT.Run(brCase.brName, func(brT *brTesting.T) {
			brText := brReviewHeading + "\n\n" + brCase.brBody
			if brGot := brHeadingOrder(brText, brUserHeading, brAssumeHeading); brGot != brCase.brWant {
				brT.Errorf("heading order = %t, want %t", brGot, brCase.brWant)
			}
		})
	}
}

func TestBundleReadabilityHelpersDecisionLine(brT *brTesting.T) {
	brPrefix := "- " + brMarkerPlaceholder + " "
	for _, brCase := range []struct {
		brName, brLine string
		brWant         bool
	}{
		{"marker decision source", brPrefix + "<decision and reason> Source: <source>", true},
		{"source before decision", brPrefix + "Source: <source> <decision and reason>", false},
		{"marker after decision", "- <decision and reason> " + brMarkerPlaceholder + " Source: <source>", false},
		{"indented marker", " " + brPrefix + "<decision and reason> Source: <source>", false},
		{"missing decision placeholder", brPrefix + "decision Source: <source>", false},
		{"missing source placeholder", brPrefix + "<decision and reason> Source: source", false},
		{"content after source", brPrefix + "<decision and reason> Source: <source> trailing decision", false},
		{"duplicate marker", brPrefix + brMarkerPlaceholder + " <decision and reason> Source: <source>", false},
	} {
		brT.Run(brCase.brName, func(brT *brTesting.T) {
			brText := brDecisionHeading + "\n\n" + brCase.brLine
			brLine, brGot := brDecisionLine(brText)
			if brGot != brCase.brWant {
				brT.Errorf("decision line = %t, want %t for %q", brGot, brCase.brWant, brLine)
			}
		})
	}
}

func TestBundleReadabilityHelpersTemplateHeaders(brT *brTesting.T) {
	brLine := "- " + brMarkerPlaceholder + " <decision and reason> Source: <source>"
	for _, brCase := range []struct {
		brName, brHeaders string
		brWant            bool
	}{
		{"standard", "Status: Draft\nCriteria-Format: nested/1\nAssurance: standard", true},
		{"strict", "Status: Draft\nCriteria-Format: nested/1\nAssurance: strict", true},
		{"missing status", "Criteria-Format: nested/1\nAssurance: strict", false},
		{"missing format", "Status: Draft\nAssurance: strict", false},
		{"missing assurance", "Status: Draft\nCriteria-Format: nested/1", false},
		{"empty assurance", "Status: Draft\nCriteria-Format: nested/1\nAssurance:", false},
	} {
		brT.Run(brCase.brName, func(brT *brTesting.T) {
			brText := "# synthetic\n\n" + brCase.brHeaders + "\n\n" + brDecisionHeading + "\n\n" + brLine
			var brMissing []string
			if brGot := brTemplateHeaders(&brMissing, brCase.brName, brText); brGot != brCase.brWant || (len(brMissing) == 0) != brCase.brWant {
				brT.Errorf("template headers = %t, want %t; missing = %q", brGot, brCase.brWant, brMissing)
			}
			brSpec := brSyntheticSpec(brText, brLine, brMarkers[0])
			for _, brKey := range []string{brFormatKey, brAssuranceKey} {
				if brGot, brWant := brHeader(brSpec, brKey), brHeader(brText, brKey); brGot != brWant {
					brT.Errorf("synthetic %s = %q, want original %q", brKey, brGot, brWant)
				}
			}
			brWantStatus := brHeader(brText, brStatusKey)
			if brWantStatus != "" {
				brWantStatus = brReady
			}
			if brGot := brHeader(brSpec, brStatusKey); brGot != brWantStatus {
				brT.Errorf("synthetic Status = %q, want %q", brGot, brWantStatus)
			}
		})
	}
}
