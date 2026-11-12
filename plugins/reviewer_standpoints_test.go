package plugins

// These checks observe fixed tokens in named sections of this tree and ha's
// public-format readers in a synthetic repository. They do not read history or
// the real repository's docs/. Token presence does not judge prose meaning or
// whether an agent follows the skill. Input failures are setup failures, never
// marked violations or skips; each document test emits one collected violation.

import (
	rpbytes "bytes"
	rpsha256 "crypto/sha256"
	rpjson "encoding/json"
	rperrors "errors"
	rpfmt "fmt"
	rpos "os"
	rpexec "os/exec"
	rppath "path/filepath"
	rpruntime "runtime"
	rpstrings "strings"
	rptesting "testing"
)

// Document implementers copy these tokens verbatim into the sections selected
// below. Whitespace is folded, so wrapping a sentence does not change the check.
// Each pair of language lists states the same requirement; format names remain
// unchanged. This is the complete token block for AC-13 and AC-14.
var (
	rpSkillPath       = "spar/skills/spar/SKILL.md"         // SPEC AC-13, AC-14: Spar skill.
	rpRunSection      = "## When to run each stage"         // SPEC AC-13, AC-14: round instructions.
	rpReviewerSection = "## Reviewers"                      // SPEC AC-14: delivery, result and records.
	rpSkillViolation  = "skill-standpoint-guidance-missing" // SPEC AC-13: declared violation.
	rpDocsViolation   = "standpoint-docs-missing"           // SPEC AC-14: declared violation.
	rpSkillTokens     = []string{
		"In plan and candidate rounds, including the cumulative round, pick the change's main risks.",                                 // SPEC AC-13: stages and risks.
		"Give each reviewer a different standpoint, one each; when there are fewer risks than reviewers, leave the rest without one.", // SPEC AC-13: allocation.
		"In fixes and plan-delta rounds, you may give the earlier standpoints again or give none.",                                    // SPEC AC-13: optional reuse.
		"A standpoint names the risk to attack, where to look first and the position from which to see it.",                           // SPEC AC-13: standpoint contents.
		"Do not claim that the fix is right or explain the solution.",                                                                 // SPEC AC-13: prohibited reasoning.
		"a changed contract boundary",                 // SPEC AC-13: example ground 1.
		"a stored format and its earlier records",     // SPEC AC-13: example ground 2.
		"the meaning of a check's result",             // SPEC AC-13: example ground 3.
		"another place that keeps the same invariant", // SPEC AC-13: example ground 4.
		"A standpoint does not replace angles.",       // SPEC AC-13: unchanged angles.
		"When rerunning only reviewers that did not finish, give the same standpoints so the rerun is not counted.", // SPEC AC-13: rerun guidance.
		"When reporting the gate, name standpoints left without a completed reviewer.",                              // SPEC AC-13: uncovered risks.
	}
	rpArgumentEn = []string{
		"`--standpoint NAME=TEXT`", // SPEC AC-14, AC-4: argument.
		"The first `=` separates the reviewer name from the text; the option may be repeated.",                                                                                             // SPEC AC-14, AC-4: syntax.
		"Refused (65): a name outside this run's configured reviewers and `--reviewers`, more than one standpoint for a reviewer, text empty after trimming whitespace, or invalid UTF-8.", // SPEC AC-14, AC-4: exhaustive refusal list.
		"Names are compared exactly and case-sensitively, as in G20.",                                                                                                                      // SPEC AC-14, AC-4: name comparison.
		"A missing `=` or an empty name is a usage error (64).",                                                                                                                            // SPEC AC-14, AC-4: usage forms.
		"Both refusals happen before any reviewer starts or any round folder is created; the message names the offending name or value.",                                                   // SPEC AC-14, AC-4: side effects and diagnosis.
		"Other text is kept unchanged.", // SPEC AC-14, AC-4: no additional refusal.
	}
	rpArgumentKo = []string{
		"`--standpoint NAME=TEXT`", // SPEC AC-14, AC-4: argument.
		"첫 `=`가 리뷰어 이름과 글을 나누며, 옵션을 여러 번 쓸 수 있다.",                                                   // SPEC AC-14, AC-4: syntax.
		"거부(65): 이번 실행의 설정과 `--reviewers`에 없는 이름, 한 리뷰어에 관점 둘 이상, 앞뒤 공백을 뗀 뒤 빈 글, 올바른 UTF-8이 아닌 글.", // SPEC AC-14, AC-4: exhaustive refusal list.
		"이름은 G20처럼 대소문자를 구분해 정확히 비교한다.",                                                             // SPEC AC-14, AC-4: name comparison.
		"`=`가 없거나 이름이 비면 사용법 오류(64)다.",                                                              // SPEC AC-14, AC-4: usage forms.
		"두 거부 모두 리뷰어를 띄우거나 라운드 폴더를 만들기 전에 일어나며, 메시지에 문제의 이름이나 값을 보인다.",                              // SPEC AC-14, AC-4: side effects and diagnosis.
		"그 밖의 글은 받은 그대로 쓴다.",                                                                        // SPEC AC-14, AC-4: no additional refusal.
	}
	rpDeliveryEn = []string{
		"The shared packet's `## Standpoints for this round` section contains all standpoints and their reviewer assignments.",                                                                                     // SPEC AC-14, AC-1: shared section.
		"Each reviewer also receives an instruction identifying its own standpoint and asking it to attack that risk while still checking every angle; an unassigned reviewer is told to review the whole change.", // SPEC AC-14, AC-1: individual instructions.
		"`claude` and `codex` receive the instruction and packet on standard input.",                                                                                                                               // SPEC AC-14, AC-1: stdin routes.
		"`agy` receives the command note, instruction and packet in `-p`; for a large packet, `PACKET.md` holds the packet and `-p` holds the command note, instruction and file reference.",                       // SPEC AC-14, AC-1: agy routes.
		"`command` receives the reviewer's `input.md` through `{prompt}`; `native` receives the instruction after its fixed instructions in `prompt.md`.",                                                          // SPEC AC-14, AC-1: command and native routes.
		"For a round with standpoints, both a real run and `--dry-run` leave in `<round>/<reviewer>/input.md` exactly the input that reviewer receives.",                                                           // SPEC AC-14, AC-1: persisted input (03-candidate codex-F2).
		"For `agy` with a large packet, `input.md` holds its `-p` text and the packet is in `PACKET.md`.",                                                                                                          // SPEC AC-14, AC-1: persisted agy file route.
	}
	rpDeliveryKo = []string{
		"공유 패킷의 `## Standpoints for this round` 절에는 모든 관점과 리뷰어 배정이 들어간다.",                               // SPEC AC-14, AC-1: shared section.
		"리뷰어마다 자기 관점을 알리고 모든 각도를 확인하면서 그 위험을 공격하라는 지시도 받으며, 관점이 없는 리뷰어는 변경 전체를 검토하라는 지시를 받는다.",          // SPEC AC-14, AC-1: individual instructions.
		"`claude`와 `codex`는 지시와 패킷을 표준 입력으로 받는다.",                                                       // SPEC AC-14, AC-1: stdin routes.
		"`agy`는 명령 안내, 지시, 패킷을 `-p`로 받으며, 큰 패킷이면 `PACKET.md`에 패킷을 두고 `-p`에는 명령 안내, 지시, 파일 안내를 둔다.",      // SPEC AC-14, AC-1: agy routes.
		"`command`는 `{prompt}`로 리뷰어의 `input.md`를 받고, `native`는 `prompt.md`의 고정 지시 뒤에 관점 지시를 받는다.",       // SPEC AC-14, AC-1: command and native routes.
		"관점이 있는 라운드는 실제 실행과 `--dry-run` 모두 리뷰어마다 그 리뷰어가 받는 입력을 그대로 `<round>/<reviewer>/input.md`에 남긴다.", // SPEC AC-14, AC-1: persisted input (03-candidate codex-F2).
		"큰 패킷을 받는 `agy`의 `input.md`는 `-p` 글이고, 패킷은 `PACKET.md`에 있다.",                                    // SPEC AC-14, AC-1: persisted agy file route.
	}
	rpResultEn = []string{
		"`standpoint_report`", // SPEC AC-14, AC-3: result key.
		"`checked` is a boolean and `detail` is nonblank text: what was checked when true, or why it could not be checked when false.",                                           // SPEC AC-14, AC-3: report form and meaning.
		"For an assigned reviewer, a missing, null or malformed report is `invalid_output`, with `standpoint_report` in the reason; an unassigned reviewer need not report one.", // SPEC AC-14, AC-3: required report.
		"Every preset uses the same check in `spar run` and `spar record --result` or `--transcript`.",                                                                           // SPEC AC-14, AC-3: shared validation.
	}
	rpResultKo = []string{
		"`standpoint_report`", // SPEC AC-14, AC-3: result key.
		"`checked`는 불리언이고 `detail`은 공백뿐이 아닌 글이다. 참이면 확인한 것, 거짓이면 확인하지 못한 이유다.",                                              // SPEC AC-14, AC-3: report form and meaning.
		"관점을 받은 리뷰어의 보고가 없거나 null이거나 형식이 틀리면 `invalid_output`이고 이유에 `standpoint_report`가 들어가며, 관점이 없는 리뷰어에게는 보고를 요구하지 않는다.", // SPEC AC-14, AC-3: required report.
		"모든 preset에 `spar run`과 `spar record --result` 또는 `--transcript`가 같은 확인을 한다.",                                       // SPEC AC-14, AC-3: shared validation.
	}
	rpRecordEn = []string{
		"`round.json` keeps `standpoints` as reviewer/text pairs, `standpoints_uncovered` as reviewer names without a completed reviewer, and each reviewer's `standpoint` and `standpoint_report` as text/report or null.", // SPEC AC-14, AC-2, AC-7: fields.
		"`summary.md` shows each reviewer's standpoint and report, and standpoints without a completed reviewer, including waived reviewers.",                                                                               // SPEC AC-14, AC-2, AC-7: summary lines.
		"The schema remains `review-round/v1`; records without standpoint fields mean no standpoints.",                                                                                                                      // SPEC AC-14, AC-2: older records.
	}
	rpRecordKo = []string{
		"`round.json`은 `standpoints`에 reviewer/text 쌍, `standpoints_uncovered`에 완료한 리뷰어가 없는 관점의 리뷰어 이름, 리뷰어마다 `standpoint`와 `standpoint_report`에 글과 보고 또는 null을 남긴다.", // SPEC AC-14, AC-2, AC-7: fields.
		"`summary.md`는 리뷰어마다 관점과 보고를 보이고, waive된 리뷰어를 포함해 완료한 리뷰어가 없는 관점을 보인다.",                                                                                       // SPEC AC-14, AC-2, AC-7: summary lines.
		"형식은 `review-round/v1` 그대로이며, 관점 필드가 없는 기록은 관점 없음으로 읽는다.",                                                                                                     // SPEC AC-14, AC-2: older records.
	}
	rpIdentityEn = []string{
		"Standpoint text and assignments are in the packet, so `packet_sha256` identifies them as part of the same input; changing either makes a new input.", // SPEC AC-14, AC-6: identity.
		"A rerun of only unfinished reviewers is not counted when stage, base, head and `packet_sha256` are unchanged, including the same standpoints.",       // SPEC AC-14, AC-6: reruns.
	}
	rpIdentityKo = []string{
		"관점의 글과 배정은 패킷 안에 있으므로 `packet_sha256`이 같은 입력의 일부로 식별하며, 어느 하나라도 바꾸면 새 입력이다.",      // SPEC AC-14, AC-6: identity.
		"끝내지 못한 리뷰어만 다시 돌리고 단계, base, head와 `packet_sha256`이 같은 관점을 포함해 그대로이면 상한에 세지 않는다.", // SPEC AC-14, AC-6: reruns.
	}
	rpUnchangedEn = []string{
		"Standpoints do not change angles, classification, dispositions, caps or the gate's state and reasons.", // SPEC AC-14, AC-9: unchanged rules.
	}
	rpUnchangedKo = []string{
		"관점은 각도, 분류, 처리, 상한, gate의 상태와 사유를 바꾸지 않는다.", // SPEC AC-14, AC-9: unchanged rules.
	}
	rpExclusionEn = "Commit messages, Git history, ignored files and the reviewer list are not in it."                                 // SPEC AC-14: existing packet exclusion.
	rpExclusionKo = "커밋 메시지, Git 이력, 무시된 파일, 리뷰어 목록은 들어가지 않는다."                                                                        // SPEC AC-14: existing packet exclusion.
	rpExceptionEn = "The exception is `## Standpoints for this round`, which names the assigned reviewers when standpoints are given." // SPEC AC-14, AC-1: qualified exclusion.
	rpExceptionKo = "예외는 관점이 있을 때 배정된 리뷰어 이름을 넣는 `## Standpoints for this round` 절이다."                                                 // SPEC AC-14, AC-1: qualified exclusion.
	rpDocuments   = []struct {
		rpPath            string
		rpHeadings        []string // Arguments, delivery, result, record, identity, unchanged.
		rpKorean          bool
		rpPacketExclusion bool
	}{
		{"../contracts/review-round.md", []string{"## 1. 명령", "## 2. 패킷과 작업공간", "## 3. 리뷰어 결과", "## 5. 라운드 폴더", "## 8. 상한·재실행·사슬", "## 8. 상한·재실행·사슬"}, true, true},                                                                        // SPEC AC-14: round contract.
		{"../contracts/review-round.en.md", []string{"## 1. Command", "## 2. Packet and workspaces", "## 3. Reviewer result", "## 5. Round folder", "## 8. Caps, reruns and the chain", "## 8. Caps, reruns and the chain"}, false, true}, // SPEC AC-14: English round contract.
		{"../contracts/spar-input.md", []string{"## 2. 입력 형식 표", "## 2. 입력 형식 표", "## 2. 입력 형식 표", "## 2. 입력 형식 표", "## 2. 입력 형식 표", "## 2. 입력 형식 표"}, true, false},                                                                       // SPEC AC-14: input contract.
		{"../contracts/spar-input.en.md", []string{"## 2. Input format table", "## 2. Input format table", "## 2. Input format table", "## 2. Input format table", "## 2. Input format table", "## 2. Input format table"}, false, false}, // SPEC AC-14: English input contract.
		{"../guides/SPAR.md", []string{"## 검토 실행", "## 컴퓨터 밖으로 나가는 것", "## 검토 실행", "## 검토 실행", "## 검토 실행", "## 검토 실행"}, true, false},                                                                                                      // SPEC AC-14: Korean guide.
		{"../guides/SPAR.en.md", []string{"## Run a review", "## What leaves your computer", "## Run a review", "## Run a review", "## Run a review", "## Run a review"}, false, false},                                                   // SPEC AC-14: English guide.
		{rpSkillPath, []string{rpRunSection, rpReviewerSection, rpReviewerSection, rpReviewerSection, rpRunSection, rpRunSection}, false, false},                                                                                          // SPEC AC-14: Spar skill.
	}
)

func rpRead(rpT *rptesting.T, rpPath string) string {
	rpT.Helper()
	rpData, rpErr := rpos.ReadFile(rpPath)
	if rpErr != nil {
		rpT.Fatalf("setup: read %s: %v", rpPath, rpErr)
	}
	return string(rpData)
}

// Use the existing review-summary helpers: they bound a section by the next
// same-or-higher heading outside code fences and fold whitespace only.
func rpMissing(rpText, rpPath, rpHeading string, rpTokens []string) []string {
	rpSection, rpOK := rsmSection(rpText, rpHeading)
	var rpMissingTokens []string
	if !rpOK {
		rpMissingTokens = append(rpMissingTokens, rpfmt.Sprintf("%s: missing section %q", rpPath, rpHeading))
	}
	for _, rpToken := range rpTokens {
		if !rpstrings.Contains(rsmNorm(rpSection), rsmNorm(rpToken)) {
			rpMissingTokens = append(rpMissingTokens, rpfmt.Sprintf("%s [%s]: missing %q", rpPath, rpHeading, rpToken))
		}
	}
	return rpMissingTokens
}

func rpFailMissing(rpT *rptesting.T, rpViolation string, rpMissingTokens []string) {
	rpT.Helper()
	if len(rpMissingTokens) > 0 {
		rpT.Errorf("jaekit:violation=%s %d missing document requirements\n%s", rpViolation, len(rpMissingTokens), rpstrings.Join(rpMissingTokens, "\n"))
	}
}

func TestReviewerStandpointsAC13_Skill(rpT *rptesting.T) {
	rpFailMissing(rpT, rpSkillViolation, rpMissing(rpRead(rpT, rpSkillPath), rpSkillPath, rpRunSection, rpSkillTokens))
}

func TestReviewerStandpointsAC14_Documents(rpT *rptesting.T) {
	var rpMissingTokens []string
	for _, rpDocument := range rpDocuments {
		rpText := rpRead(rpT, rpDocument.rpPath)
		rpGroups := [][]string{rpArgumentEn, rpDeliveryEn, rpResultEn, rpRecordEn, rpIdentityEn, rpUnchangedEn}
		rpExclusion, rpException := rpExclusionEn, rpExceptionEn
		if rpDocument.rpKorean {
			rpGroups = [][]string{rpArgumentKo, rpDeliveryKo, rpResultKo, rpRecordKo, rpIdentityKo, rpUnchangedKo}
			rpExclusion, rpException = rpExclusionKo, rpExceptionKo
		}
		for rpIndex, rpTokens := range rpGroups {
			rpMissingTokens = append(rpMissingTokens, rpMissing(rpText, rpDocument.rpPath, rpDocument.rpHeadings[rpIndex], rpTokens)...)
		}
		if rpDocument.rpPacketExclusion {
			rpSection, _ := rsmSection(rpText, rpDocument.rpHeadings[1])
			// If the old sentence survives, its own paragraph must name the
			// exception; mentioning standpoints elsewhere is insufficient.
			for _, rpParagraph := range rpstrings.Split(rpstrings.ReplaceAll(rpSection, "\r\n", "\n"), "\n\n") {
				if rpstrings.Contains(rsmNorm(rpParagraph), rpExclusion) && !rpstrings.Contains(rsmNorm(rpParagraph), rpException) {
					rpMissingTokens = append(rpMissingTokens, rpfmt.Sprintf("%s [%s]: packet exclusion paragraph needs %q", rpDocument.rpPath, rpDocument.rpHeadings[1], rpException))
				}
			}
		}
	}
	rpFailMissing(rpT, rpDocsViolation, rpMissingTokens)
}

// Like rsmRun, this runs argv directly in the named repository. Separate
// streams and exit codes make equality observable; a launch error or signal
// is a setup failure, not two apparently equal failed observations.
type rpCommandResult struct {
	rpOut, rpErr string
	rpCode       int
}

func rpRun(rpT *rptesting.T, rpDir string, rpEnv []string, rpName string, rpArgs ...string) rpCommandResult {
	rpT.Helper()
	rpCmd := rpexec.Command(rpName, rpArgs...)
	rpCmd.Dir, rpCmd.Env = rpDir, rpEnv
	var rpOut, rpErrOut rpbytes.Buffer
	rpCmd.Stdout, rpCmd.Stderr = &rpOut, &rpErrOut
	rpErr := rpCmd.Run()
	rpCode := 0
	if rpErr != nil {
		var rpExit *rpexec.ExitError
		if !rperrors.As(rpErr, &rpExit) || rpExit.ExitCode() < 0 {
			rpT.Fatalf("setup: %s %v: %v\n%s", rpName, rpArgs, rpErr, rpErrOut.String())
		}
		rpCode = rpExit.ExitCode()
	}
	return rpCommandResult{rpOut.String(), rpErrOut.String(), rpCode}
}

func rpMust(rpT *rptesting.T, rpDir string, rpEnv []string, rpName string, rpArgs ...string) string {
	rpT.Helper()
	rpResult := rpRun(rpT, rpDir, rpEnv, rpName, rpArgs...)
	if rpResult.rpCode != 0 {
		rpT.Fatalf("setup: %s %v: exit %d\n%s%s", rpName, rpArgs, rpResult.rpCode, rpResult.rpOut, rpResult.rpErr)
	}
	return rpResult.rpOut
}

func rpPut(rpT *rptesting.T, rpDir, rpName, rpBody string) {
	rpT.Helper()
	rpPath := rppath.Join(rpDir, rppath.FromSlash(rpName))
	if rpErr := rpos.MkdirAll(rppath.Dir(rpPath), 0o755); rpErr != nil {
		rpT.Fatalf("setup: mkdir: %v", rpErr)
	}
	if rpErr := rpos.WriteFile(rpPath, []byte(rpBody), 0o644); rpErr != nil {
		rpT.Fatalf("setup: write %s: %v", rpName, rpErr)
	}
}

func rpPutJSON(rpT *rptesting.T, rpDir, rpName string, rpValue any) {
	rpT.Helper()
	rpData, rpErr := rpjson.MarshalIndent(rpValue, "", "  ")
	if rpErr != nil {
		rpT.Fatalf("setup: encode %s: %v", rpName, rpErr)
	}
	rpPut(rpT, rpDir, rpName, string(rpData)+"\n")
}

func TestReviewerStandpointsAC11_HaReaders(rpT *rptesting.T) {
	rpRoot, rpErr := rppath.Abs("..")
	if rpErr != nil {
		rpT.Fatalf("setup: repository root: %v", rpErr)
	}
	rpGo := rppath.Join(rpruntime.GOROOT(), "bin", "go")
	rpHa := rppath.Join(rpT.TempDir(), "ha")
	if rpruntime.GOOS == "windows" {
		rpGo += ".exe"
		rpHa += ".exe"
	}
	rpDeps := rpMust(rpT, rpRoot, rpos.Environ(), rpGo, "list", "-deps", "./cmd/ha")
	for _, rpDependency := range rpstrings.Fields(rpDeps) {
		if rpDependency == "github.com/jgoneit/jaekit/internal/spar" {
			rpT.Fatal("ha imports the Spar package")
		}
	}
	rpMust(rpT, rpRoot, rpos.Environ(), rpGo, "build", "-o", rpHa, "./cmd/ha")
	rpDir := rpT.TempDir()
	rpEnv := append(rpos.Environ(),
		"XDG_CONFIG_HOME="+rpT.TempDir(), "GIT_CONFIG_GLOBAL="+rpos.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid",
		"GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
	rpGoal, rpSeries := "docs/specs/demo", "docs/reviews/demo"
	// Minimal goal documents as in the existing plugins reader checks; no
	// execution history is needed to read a review series with ha cost.
	rpPut(rpT, rpDir, ".gitignore", "/docs/reviews/\n")
	rpPut(rpT, rpDir, "src/value.txt", "ok\n")
	rpPut(rpT, rpDir, rpGoal+"/SPEC.md", "# Demo\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## 목표\n\nRead synthetic review records.\n\n## Acceptance Criteria\n- **AC-1** `src/value.txt` contains `ok`.\n\n## Open Decisions\n\n없음\n")
	rpPut(rpT, rpDir, rpGoal+"/PLAN.md", "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n| AC-1 | maintain | `git --version` | — | T001 |\n\n## 범위\n- 바꿀 수 있는 경로: `src/**` (SPEC: 목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n| T001 | read records | AC-1 | — | — | 예 | — |\n\n## 예산\n- 검증 실행: 10\n- 경과 시간: 1시간\n- 비용: 미관측\n")
	rpPut(rpT, rpDir, rpGoal+"/REVIEW.md", "# REVIEW — demo\n\n## 결론\nready\n")
	rpPut(rpT, rpDir, rpGoal+"/PROGRESS.md", "# PROGRESS — demo\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | todo | |\n")
	rpMust(rpT, rpDir, rpEnv, "git", "init", "-q")
	rpMust(rpT, rpDir, rpEnv, "git", "add", "-A")
	rpMust(rpT, rpDir, rpEnv, "git", "-c", "commit.gpgsign=false", "commit", "-qm", "synthetic base")
	rpHead := rpstrings.TrimSpace(rpMust(rpT, rpDir, rpEnv, "git", "rev-parse", "HEAD"))
	rpSpecHash := rpfmt.Sprintf("%x", rpsha256.Sum256([]byte(rpRead(rpT, rppath.Join(rpDir, rpGoal, "SPEC.md")))))
	rpDigest := rpfmt.Sprintf("%x", rpsha256.Sum256([]byte("synthetic packet and configuration")))
	rpRounds := []map[string]any{}
	rpPaths := []string{"01-plan", "02-candidate"}
	rpGateRounds := []any{}
	for rpIndex, rpStage := range []string{"plan", "candidate"} {
		// Shape copied from gcRound/gcReviewer in the ha cost checks.
		rpReviewers := map[string]any{}
		for _, rpName := range []string{"claude", "codex", "agy"} {
			rpStatus := "completed"
			if rpName == "agy" {
				rpStatus = "unavailable"
			}
			rpReviewers[rpName] = map[string]any{"name": rpName, "preset": rpName, "isolation": "verified", "required": true,
				"status": rpStatus, "elapsed_seconds": 12.5, "fallback_used": false,
				"attempts": []any{map[string]any{"model": "synthetic", "status": rpStatus, "elapsed_seconds": 12.5}}, "findings": nil}
			if rpStatus == "completed" {
				rpReviewers[rpName].(map[string]any)["findings"] = 0
			}
		}
		rpRound := map[string]any{"schema": "review-round/v1", "spar_version": "0.1.7", "stage": rpStage, "sequence": rpIndex + 1,
			"base": rpHead, "head": rpHead, "dry_run": false, "counted": true, "rerun_of": []string{},
			"started_at": "2026-01-01T00:00:00Z", "finished_at": "2026-01-01T00:01:00Z", "reviewers": rpReviewers}
		rpRounds = append(rpRounds, rpRound)
		rpRoundPath := rpSeries + "/" + rpPaths[rpIndex]
		rpPutJSON(rpT, rpDir, rpRoundPath+"/round.json", rpRound)
		rpPutJSON(rpT, rpDir, rpRoundPath+"/findings.json", []any{})
		rpPut(rpT, rpDir, rpRoundPath+"/ledger.jsonl", "")
		rpGateRounds = append(rpGateRounds, map[string]any{"path": rpRoundPath, "stage": rpStage, "sequence": rpIndex + 1,
			"criteria_digest": rpDigest, "criteria": []any{map[string]any{"name": rpGoal + "/SPEC.md", "sha256": rpSpecHash}},
			"packet_digest": rpDigest, "config_digest": rpDigest,
			"base": rpHead, "head": rpHead, "requested_reviewers": []string{"claude", "codex", "agy"}, "required_reviewers": 3,
			"completed_required": 2, "dry_run": false, "counted": true})
	}
	rpPutJSON(rpT, rpDir, rpSeries+"/gate.json", map[string]any{"schema": "review-gate/v1", "spar_version": "0.1.7", "state": "degraded",
		"reasons": []string{"required reviewer agy did not complete"}, "expect_head": rpHead, "last_reviewed_head": rpHead,
		"previous_gate_digest": nil, "rounds": rpGateRounds, "open": map[string]any{"defect": 0, "requirement-question": 0},
		"open_findings": []any{}, "auto_deferred": 0, "deferred_findings": []any{}, "unhandled_findings": []any{},
		"growth":     map[string]any{"added_lines": map[string]any{"check": 0, "other": 0}, "over_hardening": false, "rounds": []any{}},
		"written_at": "2026-01-01T00:02:00Z"})
	rpBefore := map[string]rpCommandResult{}
	for _, rpFormat := range []string{"json", "md"} {
		rpBefore[rpFormat] = rpRun(rpT, rpDir, rpEnv, rpHa, "cost", rpGoal, "--series", rpSeries, "--format", rpFormat)
		if rpBefore[rpFormat].rpCode != 0 {
			rpT.Fatalf("ha cost %s without standpoint fields failed: %+v", rpFormat, rpBefore[rpFormat])
		}
	}
	// Guard against equal empty outputs or a series that was never read.
	var rpCost map[string]any
	if rpErr := rpjson.Unmarshal([]byte(rpBefore["json"].rpOut), &rpCost); rpErr != nil {
		rpT.Fatalf("ha cost returned invalid JSON: %v", rpErr)
	}
	rpReview, rpOK := rpCost["review"].(map[string]any)
	if !rpOK {
		rpT.Fatalf("ha cost did not read the review series: %s", rpBefore["json"].rpOut)
	}
	rpObserved, rpOK := rpReview["rounds"].([]any)
	if !rpOK || len(rpObserved) != len(rpRounds) {
		rpT.Fatalf("ha cost did not read both rounds: %v", rpReview)
	}
	rpSeriesList, rpOK := rpReview["series"].([]any)
	if !rpOK || len(rpSeriesList) != 1 {
		rpT.Fatalf("ha cost did not read exactly one series: %v", rpReview)
	}
	rpSeriesRead, rpOK := rpSeriesList[0].(map[string]any)
	if !rpOK {
		rpT.Fatalf("ha cost returned a malformed series: %v", rpSeriesList)
	}
	rpGates, rpOK := rpSeriesRead["gates"].([]any)
	if !rpOK || len(rpGates) != 1 {
		rpT.Fatalf("ha cost did not read the gate: %v", rpSeriesRead)
	}
	for _, rpPath := range rpPaths {
		if !rpstrings.Contains(rpBefore["md"].rpOut, rpPath) {
			rpT.Fatalf("ha cost Markdown did not include %s", rpPath)
		}
	}
	for rpIndex, rpRound := range rpRounds {
		// REVIEW A6: a completed assigned reviewer, an unassigned reviewer
		// and an uncovered assigned reviewer exercise non-null and null forms.
		rpRound["standpoints"] = []any{map[string]any{"reviewer": "claude", "text": "Attack the stored format from the reader's position."}, map[string]any{"reviewer": "agy", "text": "Attack the contract boundary from the caller's position."}}
		rpRound["standpoints_uncovered"] = []string{"agy"}
		for rpName, rpValue := range rpRound["reviewers"].(map[string]any) {
			rpReviewer := rpValue.(map[string]any)
			rpReviewer["standpoint"], rpReviewer["standpoint_report"] = nil, nil
			if rpName == "claude" {
				rpReviewer["standpoint"] = "Attack the stored format from the reader's position."
				rpReviewer["standpoint_report"] = map[string]any{"checked": true, "detail": "Checked earlier records in the synthetic fixture."}
			} else if rpName == "agy" {
				rpReviewer["standpoint"] = "Attack the contract boundary from the caller's position."
			}
		}
		rpPutJSON(rpT, rpDir, rpSeries+"/"+rpPaths[rpIndex]+"/round.json", rpRound)
	}
	for _, rpFormat := range []string{"json", "md"} {
		rpAfter := rpRun(rpT, rpDir, rpEnv, rpHa, "cost", rpGoal, "--series", rpSeries, "--format", rpFormat)
		if rpAfter != rpBefore[rpFormat] {
			rpT.Errorf("ha cost %s changed after adding standpoint fields\nbefore: %+v\nafter: %+v", rpFormat, rpBefore[rpFormat], rpAfter)
		}
	}
}
