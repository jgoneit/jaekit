package spar

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// These CLI checks use only synthetic repositories and /bin/sh reviewers.
// A declared-property failure starts with jaekit:violation; a broken fixture,
// unreadable JSON, or unavailable tool starts with setup. No test reads the
// checkout's history or private goal documents. Test names are the adapter's
// entry points; all added helpers and their identifiers have the rs prefix.
//
// Regenerate the baseline ONLY on the goal's pre-standpoint tree:
// RS_WRITE_BASE=1 go test -run TestReviewerStandpointsAC8_ ./internal/spar
// The generator refuses a tree which already emits standpoints. The fixture
// pins actual CLI inputs, packet/schema/summary bytes and legacy records.
const rsFixture = "testdata/reviewer-standpoints/base.json"
const rsSection = "## Standpoints for this round"
const rsNoInstruction = "You have no standpoint this round. Review the whole change as the packet asks; the standpoints below belong to other reviewers."
const rsQuote = "synthetic user decision: continue without this reviewer"

var rsNames = []string{"claude", "codex", "agy", "command", "native"}
var rsAngles = []string{"requirement-counterexample", "side-effects", "design", "premises", "check-validity"}

type rsWorld struct {
	rsT                     *testing.T
	rsChecks                *rsChecks
	rsDir, rsRepo, rsConfig string
	rsCommits, rsTrees      []string
	rsReplacements          map[string]string
}

func rsNew(rsT *testing.T, rsC *rsChecks) *rsWorld {
	rsT.Helper()
	for _, rsTool := range []string{"git", "/bin/sh", "cat", "mkdir", "wc", "tr", "ls"} {
		if _, rsErr := exec.LookPath(rsTool); rsErr != nil {
			rsT.Fatalf("setup: tool %s: %v", rsTool, rsErr)
		}
	}
	rsT.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	rsT.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	rsT.Setenv("GIT_CONFIG_COUNT", "0")
	rsDir, rsErr := filepath.EvalSymlinks(rsT.TempDir())
	if rsErr != nil {
		rsT.Fatalf("setup: temp directory: %v", rsErr)
	}
	rsW := &rsWorld{rsT: rsT, rsChecks: rsC, rsDir: rsDir, rsRepo: filepath.Join(rsDir, "repo"), rsReplacements: map[string]string{}}
	rsW.rsWrite(filepath.Join(rsW.rsRepo, ".gitignore"), "/reviews/\n/.jaekit/\n")
	rsW.rsWrite(filepath.Join(rsW.rsRepo, "AGENTS.md"), "# Rules\n\nKeep reviews honest.\n")
	rsW.rsWrite(filepath.Join(rsW.rsRepo, "SPEC.md"), "# Synthetic goal\n\n- **AC-1** one\n- **AC-2** two\n")
	rsW.rsWrite(filepath.Join(rsW.rsRepo, "PLAN.md"), "# Plan\n\nDo one thing.\n")
	rsW.rsWrite(filepath.Join(rsW.rsRepo, "checks/test_app.py"), "assert True\n")
	rsW.rsGit("init", "-q")
	for rsI, rsBody := range []string{"print('one')\n", "print('two')\n", "print('three')\n"} {
		rsW.rsWrite(filepath.Join(rsW.rsRepo, "app/main.py"), rsBody)
		rsW.rsGit("add", "-A")
		rsW.rsGit("commit", "-qm", fmt.Sprintf("synthetic-%d", rsI))
		rsW.rsCommits = append(rsW.rsCommits, rsW.rsGit("rev-parse", "HEAD"))
		rsW.rsTrees = append(rsW.rsTrees, rsW.rsGit("rev-parse", "HEAD^{tree}"))
	}
	rsW.rsConfig = filepath.Join(rsDir, "spar.toml")
	return rsW
}
func (rsW *rsWorld) rsWrite(rsPath, rsBody string) string {
	rsW.rsT.Helper()
	if rsErr := os.MkdirAll(filepath.Dir(rsPath), 0755); rsErr != nil {
		rsW.rsT.Fatalf("setup: mkdir: %v", rsErr)
	}
	if rsErr := os.WriteFile(rsPath, []byte(rsBody), 0644); rsErr != nil {
		rsW.rsT.Fatalf("setup: write: %v", rsErr)
	}
	return rsPath
}
func (rsW *rsWorld) rsRead(rsPath string) string {
	rsW.rsT.Helper()
	rsData, rsErr := os.ReadFile(rsPath)
	if rsErr != nil {
		rsW.rsT.Fatalf("setup: read %s: %v", rsPath, rsErr)
	}
	return string(rsData)
}
func (rsW *rsWorld) rsGit(rsArgs ...string) string {
	rsW.rsT.Helper()
	rsCmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null", "-c", "init.defaultBranch=main"}, rsArgs...)...)
	rsCmd.Dir = rsW.rsRepo
	rsCmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=test@example.invalid", "GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=test@example.invalid", "GIT_AUTHOR_DATE=2026-01-01T00:00:00Z", "GIT_COMMITTER_DATE=2026-01-01T00:00:00Z")
	rsOut, rsErr := rsCmd.CombinedOutput()
	if rsErr != nil {
		rsW.rsT.Fatalf("setup: git %v: %v %s", rsArgs, rsErr, rsOut)
	}
	return strings.TrimSpace(string(rsOut))
}
func (rsW *rsWorld) rsJSON(rsText string) any {
	rsW.rsT.Helper()
	var rsV any
	if rsErr := json.Unmarshal([]byte(rsText), &rsV); rsErr != nil {
		rsW.rsT.Fatalf("setup: invalid JSON: %v", rsErr)
	}
	return rsV
}
func rsMap(rsV any) map[string]any { rsM, _ := rsV.(map[string]any); return rsM }
func rsList(rsV any) []any         { rsL, _ := rsV.([]any); return rsL }
func rsString(rsV any) string      { rsS, _ := rsV.(string); return rsS }
func (rsW *rsWorld) rsEncode(rsV any) string {
	rsW.rsT.Helper()
	rsB, rsErr := json.MarshalIndent(rsV, "", "  ")
	if rsErr != nil {
		rsW.rsT.Fatalf("setup: marshal: %v", rsErr)
	}
	return string(rsB) + "\n"
}
func (rsW *rsWorld) rsRaw(rsOut string) map[string]any {
	return rsMap(rsW.rsJSON(rsW.rsRead(filepath.Join(rsOut, "round.json"))))
}
func rsState(rsR map[string]any, rsName string) map[string]any {
	return rsMap(rsMap(rsR["reviewers"])[rsName])
}
func rsQ(rsS string) string { return "'" + strings.ReplaceAll(rsS, "'", "'\"'\"'") + "'" }
func rsInstruction(rsName string, rsHas bool) string {
	if !rsHas {
		return rsNoInstruction
	}
	return "Your standpoint for this round is the one under `### " + rsName + "` in `## Standpoints for this round`. Attack that risk first and hardest, and still check every angle as the packet asks. Report what you checked from it in `standpoint_report`."
}

// Only the declared parent emits property failures. A subtest may still fail
// for setup; preserve that as an unmarked parent failure even when another
// subtest has already collected a property problem.
type rsChecks struct {
	rsProblems []string
}

func rsCollect(rsT *testing.T, rsID string) *rsChecks {
	rsC := &rsChecks{}
	rsT.Cleanup(func() {
		if rsT.Failed() {
			rsT.Errorf("setup: parent or subtest failed before property reporting")
			return
		}
		if len(rsC.rsProblems) == 0 {
			return
		}
		rsPrefix := ""
		if rsID != "" {
			rsPrefix = "jaekit:violation=" + rsID + " "
		}
		rsT.Errorf("%s%s", rsPrefix, strings.Join(rsC.rsProblems, "\n"))
	})
	return rsC
}

func rsAssert(rsC *rsChecks, rsOK bool, rsFormat string, rsArgs ...any) {
	if !rsOK {
		rsC.rsProblems = append(rsC.rsProblems, fmt.Sprintf(rsFormat, rsArgs...))
	}
}
func (rsW *rsWorld) rsCLI(rsArgs ...string) (int, string, string) {
	rsW.rsT.Helper()
	var rsOut, rsErr strings.Builder
	rsGetenv := func(rsKey string) string {
		switch rsKey {
		case "SPAR_CONFIG":
			return rsW.rsConfig
		case "HOME":
			return rsW.rsDir
		case "XDG_CONFIG_HOME":
			return filepath.Join(rsW.rsDir, "xdg")
		}
		return ""
	}
	rsCode := Main("test", rsArgs, strings.NewReader(""), &rsOut, &rsErr, rsGetenv)
	return rsCode, rsOut.String(), rsErr.String()
}
func (rsW *rsWorld) rsMust(rsWant int, rsArgs ...string) string {
	rsW.rsT.Helper()
	rsCode, rsOut, rsErr := rsW.rsCLI(rsArgs...)
	if rsCode != rsWant {
		rsW.rsT.Fatalf("setup: spar %s exit %d, want %d: %s%s", rsArgs[0], rsCode, rsWant, rsOut, rsErr)
	}
	return rsOut
}
func (rsW *rsWorld) rsArgs(rsOut, rsStage string, rsBase, rsHead int, rsExtra ...string) []string {
	return append([]string{"run", "--repo", rsW.rsRepo, "--stage", rsStage, "--base", rsW.rsCommits[rsBase], "--head", rsW.rsCommits[rsHead], "--out", rsOut, "--criteria", filepath.Join(rsW.rsRepo, "SPEC.md"), "--rules", filepath.Join(rsW.rsRepo, "AGENTS.md"), "--check-path", "checks/**"}, rsExtra...)
}
func (rsW *rsWorld) rsRun(rsID, rsOut, rsStage string, rsBase, rsHead int, rsExtra ...string) bool {
	rsW.rsT.Helper()
	rsCode, _, rsErr := rsW.rsCLI(rsW.rsArgs(rsOut, rsStage, rsBase, rsHead, rsExtra...)...)
	if rsCode == 0 || rsCode == 1 {
		rsW.rsCheckProcesses(rsOut)
		return true
	}
	if rsID != "" && (rsCode == 64 || rsCode == 65) && strings.Contains(rsErr, "standpoint") {
		rsAssert(rsW.rsChecks, false, "spar run rejected standpoints (exit %d): %s", rsCode, strings.Split(rsErr, "\n")[0])
		return false
	}
	rsW.rsT.Fatalf("setup: spar run exit %d: %s", rsCode, rsErr)
	return false
}

// Unexpected process startup failures/signals are inconclusive, never violations.
func (rsW *rsWorld) rsCheckProcesses(rsOut string) {
	rsW.rsT.Helper()
	for rsName, rsValue := range rsMap(rsW.rsRaw(rsOut)["reviewers"]) {
		rsSt := rsMap(rsValue)
		rsReason := rsString(rsSt["reason"])
		if rsSt["status"] == "unavailable" && !strings.Contains(strings.ToLower(rsReason), "quota") {
			rsW.rsT.Fatalf("setup: fake %s unavailable: %s", rsName, rsReason)
		}
		if rsSt["status"] == "failed" && !strings.Contains(rsReason, "synthetic intentional failure") {
			rsW.rsT.Fatalf("setup: fake %s failed: %s", rsName, rsReason)
		}
		for _, rsAttempt := range rsList(rsSt["attempts"]) {
			rsA := rsMap(rsAttempt)
			if rsCode, rsOK := rsA["exit_code"].(float64); rsOK && rsCode < 0 {
				rsW.rsT.Fatalf("setup: fake %s terminated by signal: %v", rsName, rsA)
			}
		}
	}
}
func (rsW *rsWorld) rsOut(rsSeries, rsName string) string {
	return filepath.Join(rsW.rsRepo, "reviews", rsSeries, rsName)
}
func (rsW *rsWorld) rsUse(rsEntries ...string) {
	rsW.rsWrite(rsW.rsConfig, strings.Join(rsEntries, "\n"))
}
func rsNative(rsName string) string {
	return fmt.Sprintf("[[reviewers]]\nname=%q\npreset=\"native\"\n", rsName)
}
func rsReport(rsChecked bool, rsDetail string) any {
	return map[string]any{"checked": rsChecked, "detail": rsDetail}
}
func rsFinding(rsID string, rsValues map[string]any) map[string]any {
	rsF := map[string]any{"id": rsID, "proposed_class": "defect", "angle": "design", "location": "head/app/main.py:1", "claim": "synthetic changed behavior", "failure_scenario": "synthetic example", "evidence": "synthetic evidence", "link_kind": "criterion", "link": "AC-1", "introduced_by_change": "yes", "regression": "no", "check_impact": "none", "confidence": "high"}
	for rsK, rsV := range rsValues {
		rsF[rsK] = rsV
	}
	return rsF
}
func (rsW *rsWorld) rsAnswer(rsName string, rsHead int, rsReportValue any, rsInclude bool, rsFindings ...any) string {
	rsChecked := []any{}
	for _, rsAngle := range rsAngles {
		rsChecked = append(rsChecked, map[string]any{"angle": rsAngle, "result": "synthetic check"})
	}
	rsVerdict := "no_blocking_findings"
	if len(rsFindings) > 0 {
		rsVerdict = "blocking_findings"
	}
	rsR := map[string]any{"reviewed_head": rsW.rsCommits[rsHead], "verdict": rsVerdict, "angles_checked": rsChecked, "angles_unchecked": []any{}, "findings": append([]any{}, rsFindings...), "limits": "synthetic reviewer"}
	if rsInclude {
		rsR["standpoint_report"] = rsReportValue
	}
	rsBytes, rsErr := json.Marshal(rsR)
	if rsErr != nil {
		rsW.rsT.Fatalf("setup: answer JSON: %v", rsErr)
	}
	return rsW.rsWrite(filepath.Join(rsW.rsDir, "answers", rsName+".json"), string(rsBytes))
}

// Fakes save argv (NUL separated), stdin and both file transports before
// answering. A separate directory per call also observes fallback attempts.
func (rsW *rsWorld) rsFake(rsName, rsPreset, rsAnswer, rsExtra string) string {
	rsCalls := filepath.Join(rsW.rsDir, "calls", rsName)
	rsW.rsWrite(filepath.Join(rsCalls, ".keep"), "")
	rsBody := "#!/bin/sh\nset -eu\nif [ \"${1-}\" = --version ]; then echo 'synthetic 0.0.0'; exit 0; fi\n" +
		"if [ \"${1-}\" = --preflight ]; then cat " + rsQ(rsAnswer) + " >/dev/null; ls " + rsQ(rsCalls) + " | wc -l | tr -d ' ' >/dev/null; exit 0; fi\n" +
		"rs_n=$(ls " + rsQ(rsCalls) + " | wc -l | tr -d ' ')\nrs_call=" + rsQ(rsCalls) + "/$rs_n\nmkdir \"$rs_call\"\nprintf '%s\\0' \"$@\" > \"$rs_call/argv\"\ncat > \"$rs_call/stdin\"\n" +
		"if [ -f PACKET.md ]; then cat PACKET.md > \"$rs_call/PACKET.md\"; fi\n"
	if rsPreset == "command" {
		rsBody += "cat \"$1\" > \"$rs_call/prompt\"\n"
	}
	rsBody += "rs_prev=\nrs_model=\nrs_out=\nfor rs_arg in \"$@\"; do\ncase \"$rs_prev\" in --model|-m) rs_model=$rs_arg;; -o) rs_out=$rs_arg;; esac\nrs_prev=$rs_arg\ndone\n" +
		"if [ -f " + rsQ(filepath.Join(rsW.rsDir, "fail-"+rsName)) + " ]; then echo 'synthetic intentional failure' >&2; exit 3; fi\n" +
		"case \"$rs_model\" in quota-*) echo 'RESOURCE_EXHAUSTED (code 429): quota exceeded' >&2; exit 1;; esac\n"
	switch rsPreset {
	case "claude":
		rsBody += "printf '%s\\n' '{\"type\":\"system\",\"subtype\":\"init\",\"model\":\"synthetic\",\"tools\":[\"Read\",\"Grep\",\"Glob\"],\"mcp_servers\":[]}'\nprintf '%s' '{\"type\":\"result\",\"subtype\":\"success\",\"is_error\":false,\"structured_output\":'\ncat " + rsQ(rsAnswer) + "\nprintf '%s\\n' '}'\n"
	case "codex":
		rsBody += "cat " + rsQ(rsAnswer) + " > \"$rs_out\"\nprintf '%s\\n' '{\"type\":\"thread.started\"}' '{\"type\":\"turn.completed\",\"usage\":{}}'\n"
	case "agy":
		rsBody += "printf '%s\\n' '{\"init\":{\"model\":\"synthetic\"}}'\nprintf '%s' '{\"result\":{\"status\":\"success\",\"structured_output\":'\ncat " + rsQ(rsAnswer) + "\nprintf '%s\\n' '}}'\n"
	case "command":
		rsBody += "cat " + rsQ(rsAnswer) + "\n"
	}
	rsPath := rsW.rsWrite(filepath.Join(rsW.rsDir, "bin", rsName), rsBody)
	if rsErr := os.Chmod(rsPath, 0755); rsErr != nil {
		rsW.rsT.Fatalf("setup: chmod: %v", rsErr)
	}
	for _, rsArg := range []string{"--version", "--preflight"} {
		if rsOutput, rsErr := exec.Command(rsPath, rsArg).CombinedOutput(); rsErr != nil {
			rsW.rsT.Fatalf("setup: fake preflight: %v %s", rsErr, rsOutput)
		}
	}
	if rsPreset == "command" {
		return fmt.Sprintf("[[reviewers]]\nname=%q\npreset=\"command\"\nargv=[%q,\"{prompt}\"]\n%s\n", rsName, rsPath, rsExtra)
	}
	return fmt.Sprintf("[[reviewers]]\nname=%q\npreset=%q\nbinary=%q\n%s\n", rsName, rsPreset, rsPath, rsExtra)
}
func (rsW *rsWorld) rsAll(rsHead int, rsStandpoints map[string]string) {
	rsEntries := []string{}
	for _, rsName := range rsNames {
		if rsName == "native" {
			rsEntries = append(rsEntries, rsNative(rsName))
			continue
		}
		_, rsHas := rsStandpoints[rsName]
		rsAnswer := rsW.rsAnswer(rsName, rsHead, rsReport(true, "checked "+rsName), rsHas)
		rsEntries = append(rsEntries, rsW.rsFake(rsName, rsName, rsAnswer, ""))
	}
	rsW.rsUse(rsEntries...)
}
func (rsW *rsWorld) rsCalls(rsName string) []string {
	rsW.rsT.Helper()
	rsEntries, rsErr := os.ReadDir(filepath.Join(rsW.rsDir, "calls", rsName))
	if rsErr != nil {
		rsW.rsT.Fatalf("setup: calls: %v", rsErr)
	}
	rsResult := []string{}
	for _, rsE := range rsEntries {
		if rsE.IsDir() {
			rsResult = append(rsResult, filepath.Join(rsW.rsDir, "calls", rsName, rsE.Name()))
		}
	}
	sort.Strings(rsResult)
	return rsResult
}
func (rsW *rsWorld) rsLast(rsName, rsFile string) string {
	rsW.rsT.Helper()
	rsCalls := rsW.rsCalls(rsName)
	if len(rsCalls) == 0 {
		rsW.rsT.Fatalf("setup: fake %s never captured input", rsName)
	}
	return rsW.rsRead(filepath.Join(rsCalls[len(rsCalls)-1], rsFile))
}
func rsArg(rsArgs, rsKey string) string {
	rsA := strings.Split(rsArgs, "\x00")
	for rsI := 0; rsI+1 < len(rsA); rsI++ {
		if rsA[rsI] == rsKey {
			return rsA[rsI+1]
		}
	}
	return ""
}
func (rsW *rsWorld) rsRecord(rsOut, rsName, rsAnswer, rsMode string) (int, string) {
	rsPath := rsAnswer
	if rsMode == "transcript" {
		rsEntry := map[string]any{"type": "assistant", "cwd": filepath.Join(rsOut, rsName, "workspace"), "message": map[string]any{"model": "synthetic", "content": []any{map[string]any{"type": "tool_use", "name": "Read", "input": map[string]any{"file_path": filepath.Join(rsOut, rsName, "prompt.md")}}, map[string]any{"type": "text", "text": rsW.rsRead(rsAnswer)}}}}
		rsB, rsErr := json.Marshal(rsEntry)
		if rsErr != nil {
			rsW.rsT.Fatalf("setup: transcript: %v", rsErr)
		}
		rsPath = rsW.rsWrite(filepath.Join(rsW.rsDir, "reply.jsonl"), string(rsB)+"\n")
	}
	rsCode, _, rsErr := rsW.rsCLI("record", rsOut, rsName, "--"+rsMode, rsPath)
	if rsCode != 0 && rsCode != 1 {
		rsW.rsT.Fatalf("setup: record exit %d: %s", rsCode, rsErr)
	}
	return rsCode, rsErr
}
func (rsW *rsWorld) rsGate(rsName string, rsHead int, rsRounds ...string) map[string]any {
	rsW.rsT.Helper()
	rsPath := rsW.rsOut("gates", rsName+".json")
	rsArgs := append([]string{"gate"}, rsRounds...)
	rsArgs = append(rsArgs, "--expect-head", rsW.rsCommits[rsHead], "--out", rsPath, "--repo", rsW.rsRepo)
	rsCode, _, rsErr := rsW.rsCLI(rsArgs...)
	if rsCode != 0 && rsCode != 3 && rsCode != 4 {
		rsW.rsT.Fatalf("setup: gate exit %d: %s", rsCode, rsErr)
	}
	return rsMap(rsW.rsJSON(rsW.rsRead(rsPath)))
}
func rsGateJudgment(rsG map[string]any) map[string]any {
	rsM := map[string]any{}
	for _, rsK := range []string{"state", "reasons", "open", "auto_deferred", "open_findings"} {
		rsM[rsK] = rsG[rsK]
	}
	return rsM
}
func (rsW *rsWorld) rsCap(rsSeries string) map[string]any {
	return rsMap(rsW.rsJSON(rsW.rsMust(0, "cap-report", rsSeries, "--format", "json", "--repo", rsW.rsRepo)))
}
func (rsW *rsWorld) rsRemove(rsPath string) {
	rsW.rsT.Helper()
	if rsErr := os.Remove(rsPath); rsErr != nil {
		rsW.rsT.Fatalf("setup: remove: %v", rsErr)
	}
}
func (rsW *rsWorld) rsStrip(rsOut string) {
	rsR := rsW.rsRaw(rsOut)
	delete(rsR, "standpoints")
	delete(rsR, "standpoints_uncovered")
	for _, rsV := range rsMap(rsR["reviewers"]) {
		delete(rsMap(rsV), "standpoint")
		delete(rsMap(rsV), "standpoint_report")
	}
	rsW.rsWrite(filepath.Join(rsOut, "round.json"), rsW.rsEncode(rsR))
}

func (rsW *rsWorld) rsPropertyFile(rsID, rsPath string) string {
	rsW.rsT.Helper()
	rsData, rsErr := os.ReadFile(rsPath)
	if os.IsNotExist(rsErr) {
		rsAssert(rsW.rsChecks, false, "required file %s is missing", filepath.Base(rsPath))
		return ""
	}
	if rsErr != nil {
		rsW.rsT.Fatalf("setup: read %s: %v", rsPath, rsErr)
	}
	return string(rsData)
}

// Keep the large input identical across hosts, and above both supported
// argument limits. Its complete bytes are part of the no-standpoint fixture.
func (rsW *rsWorld) rsLargeCriteria() string {
	return rsW.rsWrite(filepath.Join(rsW.rsDir, "BIG.md"), strings.Repeat("large synthetic criterion\n", 34000))
}

func (rsW *rsWorld) rsReceived(rsOut, rsName string) string {
	switch rsName {
	case "claude", "codex":
		return rsW.rsLast(rsName, "stdin")
	case "agy":
		return rsArg(rsW.rsLast(rsName, "argv"), "-p")
	case "native":
		return rsW.rsRead(filepath.Join(rsOut, rsName, "prompt.md"))
	default:
		return rsW.rsLast(rsName, "prompt")
	}
}

// With these plain synthetic standpoint texts, the next level-two heading
// ends exactly the one added section. Preserve every byte outside it.
func rsWithoutSection(rsC *rsChecks, rsPacket string) string {
	rsHeader := rsSection + "\n"
	rsStart := strings.Index(rsPacket, rsHeader)
	rsAssert(rsC, strings.Count(rsPacket, rsHeader) == 1, "packet must have exactly one standpoint section")
	if rsStart < 0 {
		return rsPacket
	}
	rsEnd := strings.Index(rsPacket[rsStart+len(rsHeader):], "\n## ")
	if rsEnd < 0 {
		rsAssert(rsC, false, "standpoint section has no following shared section")
		return rsPacket
	}
	rsEnd += rsStart + len(rsHeader) + 1
	return rsPacket[:rsStart] + rsPacket[rsEnd:]
}

func TestReviewerStandpointsAC1_EveryRouteGetsItsStandpoint(rsT *testing.T) {
	const rsID = "standpoint-not-delivered"
	rsC := rsCollect(rsT, rsID)
	for _, rsLarge := range []bool{false, true} {
		rsT.Run(fmt.Sprintf("agy-file-%t", rsLarge), func(rsT *testing.T) {
			rsW := rsNew(rsT, rsC)
			// A real earlier round supplies the same nonempty prior findings to
			// each fixes run, alongside the documents, decision and question.
			rsW.rsUse(rsNative("native"))
			rsSeed := rsW.rsOut("delivery", "01-candidate")
			rsW.rsRun(rsID, rsSeed, "candidate", 0, 1)
			rsW.rsRecord(rsSeed, "native", rsW.rsAnswer("prior", 1, nil, false, rsFinding("F1", map[string]any{"claim": "Keep this earlier finding in every shared packet"})), "result")
			rsNamesHere := append(append([]string{}, rsNames...), "whole")
			rsTexts := map[string]string{
				"claude":  "Attack stored record compatibility.",
				"codex":   "Attack the contract refusal boundary.",
				"agy":     "Attack both packet transport forms.",
				"command": "Attack command prompt delivery.",
				"native":  "Attack native prompt delivery.",
			}
			rsUse := func(rsAssigned bool) {
				rsEntries := []string{}
				for _, rsName := range rsNamesHere {
					if rsName == "native" {
						rsEntries = append(rsEntries, rsNative(rsName))
						continue
					}
					rsPreset := rsName
					if rsName == "whole" {
						rsPreset = "command"
					}
					_, rsHas := rsTexts[rsName]
					rsEntries = append(rsEntries, rsW.rsFake(rsName, rsPreset, rsW.rsAnswer(rsName, 2, rsReport(true, "checked "+rsName), rsAssigned && rsHas), ""))
				}
				rsW.rsUse(rsEntries...)
			}
			rsOut := rsW.rsOut("delivery", "02-fixes")
			rsCommon := []string{"--decision", "Keep the existing classification", "--decision-quote", "Keep it", "--question", "Are AC-1 and the shared rules covered?", "--subject", filepath.Join(rsW.rsRepo, "PLAN.md")}
			if rsLarge {
				rsCommon = append(rsCommon, "--criteria", rsW.rsLargeCriteria())
			}
			rsUse(false)
			if !rsW.rsRun(rsID, rsOut, "fixes", 1, 2, rsCommon...) {
				return
			}
			rsPlainPacket := rsW.rsRead(filepath.Join(rsOut, "packet.md"))
			rsAssert(rsC, strings.Contains(rsPlainPacket, "Keep this earlier finding in every shared packet"), "control packet omitted the seeded prior finding")
			rsPlainInputs := map[string]string{}
			for _, rsName := range rsNamesHere {
				rsPlainInputs[rsName] = rsW.rsReceived(rsOut, rsName)
			}
			rsPlainFile := ""
			if rsLarge {
				rsPlainFile = rsW.rsLast("agy", "PACKET.md")
			}
			rsClear := func() {
				if rsErr := os.RemoveAll(rsOut); rsErr != nil {
					rsT.Fatalf("setup: remove temporary round: %v", rsErr)
				}
			}
			rsClear()
			rsUse(true)
			rsOptions := append([]string{}, rsCommon...)
			for _, rsName := range rsNames {
				rsOptions = append(rsOptions, "--standpoint", rsName+"="+rsTexts[rsName])
			}
			// Reuse the exact output path so even native's fixed path-bearing
			// instructions must match without normalizing any prompt bytes.
			if !rsW.rsRun(rsID, rsOut, "fixes", 1, 2, append(append([]string{}, rsOptions...), "--dry-run")...) {
				return
			}
			rsDryInputs := map[string]string{}
			for _, rsName := range rsNamesHere {
				rsDryInputs[rsName] = rsW.rsPropertyFile(rsID, filepath.Join(rsOut, rsName, "input.md"))
			}
			rsDryFile := ""
			if rsLarge {
				rsDryFile = rsW.rsRead(filepath.Join(rsOut, "packet.md"))
			}
			rsClear()
			if !rsW.rsRun(rsID, rsOut, "fixes", 1, 2, rsOptions...) {
				return
			}
			rsPacket := rsW.rsRead(filepath.Join(rsOut, "packet.md"))
			rsAssert(rsC, rsWithoutSection(rsC, rsPacket) == rsPlainPacket, "standpoint packet changed shared documents, decisions, questions, prior findings, diff or instructions")
			for _, rsName := range rsNamesHere {
				_, rsHas := rsTexts[rsName]
				rsPrefix := ""
				if rsName == "agy" {
					rsPrefix = agyCommandNote
				} else if rsName == "native" {
					rsPrefix = fmt.Sprintf(nativeInstructions, filepath.Join(rsOut, rsName, "workspace"), filepath.Join(rsOut, "schema.json"), rsOut, rsName)
				}
				rsInput := rsW.rsReceived(rsOut, rsName)
				rsInstructionText := rsInstruction(rsName, rsHas) + "\n\n"
				rsAssert(rsC, strings.HasPrefix(rsInput, rsPrefix+rsInstructionText), "%s input lost its own standpoint instruction or fixed prefix", rsName)
				rsShared := strings.TrimPrefix(rsInput, rsPrefix+rsInstructionText)
				if rsName == "agy" && rsLarge {
					rsAssert(rsC, rsPrefix+rsShared == rsPlainInputs[rsName] && strings.Contains(rsShared, "PACKET.md"), "large agy pointer changed beyond the standpoint instruction")
					rsFile := rsW.rsLast("agy", "PACKET.md")
					rsAssert(rsC, rsFile == rsPacket && rsWithoutSection(rsC, rsFile) == rsPlainFile, "large agy PACKET.md changed the independently observed shared content")
					rsAssert(rsC, rsDryFile == rsFile, "dry-run packet.md differs from agy's normal-run PACKET.md delivery")
				} else {
					rsAssert(rsC, rsShared == rsPacket && rsPrefix+rsWithoutSection(rsC, rsShared) == rsPlainInputs[rsName], "%s changed shared content relative to its no-standpoint control", rsName)
				}
				if rsHas {
					_, rsAssignment, rsFound := strings.Cut(rsPacket, "\n### "+rsName+"\n")
					rsAssignment = strings.SplitN(strings.SplitN(rsAssignment, "\n### ", 2)[0], "\n## ", 2)[0]
					rsAssert(rsC, rsFound && strings.Contains(rsAssignment, rsTexts[rsName]), "%s shared input omitted its own assigned standpoint text", rsName)
				}
				rsAssert(rsC, rsW.rsPropertyFile(rsID, filepath.Join(rsOut, rsName, "input.md")) == rsInput, "%s input.md differs from actual preset delivery", rsName)
				rsAssert(rsC, rsDryInputs[rsName] == rsInput, "%s dry-run input.md differs byte for byte from normal-run delivery", rsName)
				if rsName == "command" || rsName == "whole" {
					rsAssert(rsC, strings.Split(rsW.rsLast(rsName, "argv"), "\x00")[0] == filepath.Join(rsOut, rsName, "input.md"), "%s did not receive its input.md path", rsName)
				}
			}
		})
	}
}

func TestReviewerStandpointsAC2_RecordShowsStandpoints(rsT *testing.T) {
	const rsID = "standpoint-not-recorded"
	rsC := rsCollect(rsT, rsID)
	rsW := rsNew(rsT, rsC)
	rsText := "저장 형식을 읽는 쪽\nsecond line"
	rsW.rsUse(rsW.rsFake("claude", "claude", rsW.rsAnswer("claude", 1, rsReport(true, "checked record readers\nmore detail"), true), ""), rsW.rsFake("codex", "codex", rsW.rsAnswer("codex", 1, rsReport(false, "could not inspect downstream\nmore detail"), true), ""), rsNative("native"))
	rsOut := rsW.rsOut("record", "01-candidate")
	if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, "--standpoint", "claude="+rsText, "--standpoint", "codex=Attack downstream readers") {
		return
	}
	rsW.rsRecord(rsOut, "native", rsW.rsAnswer("native", 1, nil, false), "result")
	rsR := rsW.rsRaw(rsOut)
	rsAssert(rsC, reflect.DeepEqual(rsR["standpoints"], []any{map[string]any{"reviewer": "claude", "text": rsText}, map[string]any{"reviewer": "codex", "text": "Attack downstream readers"}}), "top-level standpoints did not preserve assignments and text: %v", rsR["standpoints"])
	for _, rsName := range []string{"claude", "codex", "native"} {
		rsSt := rsState(rsR, rsName)
		rsS, rsHasS := rsSt["standpoint"]
		rsReportValue, rsHasR := rsSt["standpoint_report"]
		if rsName == "native" {
			rsAssert(rsC, rsHasS && rsHasR && rsS == nil && rsReportValue == nil, "unassigned reviewer fields must both be explicit null")
			continue
		}
		rsWant := rsText
		rsWantReport := rsReport(true, "checked record readers\nmore detail")
		if rsName == "codex" {
			rsWant = "Attack downstream readers"
			rsWantReport = rsReport(false, "could not inspect downstream\nmore detail")
		}
		rsAssert(rsC, rsS == rsWant && reflect.DeepEqual(rsReportValue, rsWantReport), "%s standpoint or report not recorded: %v", rsName, rsSt)
		rsAssert(rsC, strings.Contains(rsW.rsPropertyFile(rsID, filepath.Join(rsOut, rsName, "input.md")), rsWant), "%s input omitted its text", rsName)
	}
	rsSummary := rsW.rsRead(filepath.Join(rsOut, "summary.md"))
	for _, rsLine := range []string{"- Standpoints: 2 for claude, codex", "  - standpoint: 저장 형식을 읽는 쪽", "  - standpoint report: checked: checked record readers", "  - standpoint report: unchecked: could not inspect downstream"} {
		rsAssert(rsC, strings.Contains(rsSummary, rsLine+"\n"), "summary omitted %q", rsLine)
	}
	// Read an actual old-format round: removing only the additive fields must
	// not alter the gate, and spar summary still reads the old summary bytes.
	rsBefore := rsW.rsGate("record-before", 1, rsOut)
	rsW.rsStrip(rsOut)
	rsAfter := rsW.rsGate("record-after", 1, rsOut)
	rsAssert(rsC, reflect.DeepEqual(rsGateJudgment(rsBefore), rsGateJudgment(rsAfter)), "old-format round changes gate judgment")
	rsOld := rsW.rsOut("old", "01-candidate")
	rsW.rsUse(rsNative("native"))
	rsW.rsRun(rsID, rsOld, "candidate", 0, 1)
	rsW.rsRecord(rsOld, "native", rsW.rsAnswer("old", 1, nil, false), "result")
	rsW.rsStrip(rsOld)
	rsOldSummary := rsW.rsMust(0, "summary", rsOld)
	rsAssert(rsC, !strings.Contains(strings.ToLower(rsOldSummary), "standpoint") && strings.Contains(rsOldSummary, "native: completed"), "old-format summary was not read as a round without standpoints")
	rsAssert(rsC, rsW.rsGate("old", 1, rsOld)["state"] != "blocked", "old-format round was blocked")
}

func TestReviewerStandpointsAC3_ReportRequired(rsT *testing.T) {
	const rsID = "standpoint-report-not-required"
	rsC := rsCollect(rsT, rsID)
	rsCases := []struct {
		rsName    string
		rsValue   any
		rsInclude bool
	}{
		{"missing", nil, false}, {"null", nil, true}, {"missing-checked", map[string]any{"detail": "checked"}, true},
		{"wrong-checked", map[string]any{"checked": "yes", "detail": "checked"}, true}, {"blank-detail", rsReport(true, " \n\t"), true},
	}
	for _, rsCase := range rsCases {
		for _, rsRoute := range []string{"run", "result", "transcript"} {
			rsT.Run(rsCase.rsName+"/"+rsRoute, func(rsT *testing.T) {
				rsW := rsNew(rsT, rsC)
				rsAnswer := rsW.rsAnswer("bad", 1, rsCase.rsValue, rsCase.rsInclude)
				rsEntry := rsNative("assigned")
				if rsRoute == "run" {
					rsEntry = rsW.rsFake("assigned", "command", rsAnswer, "")
				}
				rsW.rsUse(rsEntry, rsW.rsFake("whole", "command", rsW.rsAnswer("whole", 1, nil, false), ""))
				rsOut := rsW.rsOut("reports", "01-candidate")
				if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, "--standpoint", "assigned=Attack report validation") {
					return
				}
				if rsRoute != "run" {
					rsW.rsRecord(rsOut, "assigned", rsAnswer, rsRoute)
				}
				rsR := rsW.rsRaw(rsOut)
				rsSt := rsState(rsR, "assigned")
				rsAssert(rsC, rsSt["status"] == "invalid_output" && strings.Contains(rsString(rsSt["reason"]), "standpoint_report"), "%s %s was not invalid_output with a standpoint_report reason: %v", rsCase.rsName, rsRoute, rsSt)
				rsAssert(rsC, rsState(rsR, "whole")["status"] == "completed", "unassigned reviewer could not omit the report")
				rsSchema := rsMap(rsW.rsJSON(rsW.rsRead(filepath.Join(rsOut, "schema.json"))))
				rsProp := rsMap(rsMap(rsSchema["properties"])["standpoint_report"])
				rsAssert(rsC, reflect.DeepEqual(rsProp["type"], []any{"object", "null"}), "schema does not declare object or null for standpoint_report")
				rsRequired := false
				for _, rsKey := range rsList(rsSchema["required"]) {
					rsRequired = rsRequired || rsKey == "standpoint_report"
				}
				rsAssert(rsC, rsRequired, "schema does not require standpoint_report")
			})
		}
	}
	// Both checked values are valid, and null is permitted for an unassigned
	// reviewer (the missing-key case above is a separate observation).
	for _, rsChecked := range []bool{true, false} {
		rsT.Run(fmt.Sprintf("valid-%t", rsChecked), func(rsT *testing.T) {
			rsW := rsNew(rsT, rsC)
			rsW.rsUse(rsNative("assigned"), rsW.rsFake("whole", "command", rsW.rsAnswer("whole", 1, nil, true), ""), rsW.rsFake("claude", "claude", rsW.rsAnswer("claude", 1, rsReport(rsChecked, "specific schema observation"), true), ""))
			rsOut := rsW.rsOut("valid", "01-candidate")
			if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, "--standpoint", "assigned=Attack record acceptance", "--standpoint", "claude=Attack delivered schema") {
				return
			}
			rsDelivered := rsArg(rsW.rsLast("claude", "argv"), "--json-schema")
			rsAssert(rsC, rsDelivered == rsW.rsRead(filepath.Join(rsOut, "schema.json")), "claude --json-schema differs from the round's schema.json")
			rsAssert(rsC, rsDelivered != "", "claude did not receive --json-schema")
			if rsDelivered != "" {
				rsSchema := rsMap(rsW.rsJSON(rsDelivered))
				rsProp := rsMap(rsMap(rsSchema["properties"])["standpoint_report"])
				rsAssert(rsC, reflect.DeepEqual(rsProp["type"], []any{"object", "null"}), "delivered claude schema does not allow an object or null standpoint_report")
				rsProps := rsMap(rsProp["properties"])
				rsAssert(rsC, rsMap(rsProps["checked"])["type"] == "boolean" && rsMap(rsProps["detail"])["type"] == "string", "delivered claude schema does not permit the A5 checked/detail report")
				for _, rsRequired := range []struct {
					rsSchema map[string]any
					rsKey    string
				}{{rsSchema, "standpoint_report"}, {rsProp, "checked"}, {rsProp, "detail"}} {
					rsFound := false
					for _, rsKey := range rsList(rsRequired.rsSchema["required"]) {
						rsFound = rsFound || rsKey == rsRequired.rsKey
					}
					rsAssert(rsC, rsFound, "delivered claude schema does not require %s", rsRequired.rsKey)
				}
			}
			rsW.rsRecord(rsOut, "assigned", rsW.rsAnswer("assigned", 1, rsReport(rsChecked, "specific observation"), true), "result")
			rsR := rsW.rsRaw(rsOut)
			rsAssert(rsC, rsState(rsR, "assigned")["status"] == "completed" && rsState(rsR, "whole")["status"] == "completed" && rsState(rsR, "claude")["status"] == "completed", "valid report or unassigned null was rejected")
		})
	}
}

func TestReviewerStandpointsAC4_Refusals(rsT *testing.T) {
	const rsID = "standpoint-input-not-refused"
	rsC := rsCollect(rsT, rsID)
	rsCases := []struct {
		rsName    string
		rsArgs    []string
		rsCode    int
		rsMention string
	}{
		{"unknown", []string{"--standpoint", "unknown=attack"}, 65, "unknown"},
		{"case-sensitive", []string{"--standpoint", "Cmd=attack"}, 65, "Cmd"},
		{"excluded", []string{"--reviewers", "other", "--standpoint", "cmd=attack"}, 65, "cmd"},
		{"duplicate", []string{"--standpoint", "cmd=first", "--standpoint", "cmd=second"}, 65, "cmd"},
		{"blank", []string{"--standpoint", "cmd= \n\t"}, 65, "cmd"},
		{"invalid-utf8", []string{"--standpoint", "cmd=bad\xfftext"}, 65, "cmd"},
		{"no-equals", []string{"--standpoint", "cmd"}, 64, ""},
		{"empty-name", []string{"--standpoint", "=attack"}, 64, ""},
	}
	for _, rsCase := range rsCases {
		rsT.Run(rsCase.rsName, func(rsT *testing.T) {
			rsW := rsNew(rsT, rsC)
			rsW.rsUse(rsW.rsFake("cmd", "command", rsW.rsAnswer("cmd", 1, nil, false), ""), rsW.rsFake("other", "command", rsW.rsAnswer("other", 1, nil, false), ""))
			rsOut := rsW.rsOut("refusal", "01-candidate")
			rsCode, _, rsErr := rsW.rsCLI(rsW.rsArgs(rsOut, "candidate", 0, 1, rsCase.rsArgs...)...)
			rsAssert(rsC, rsCode == rsCase.rsCode, "%s exits %d, want %d: %s", rsCase.rsName, rsCode, rsCase.rsCode, strings.Split(rsErr, "\n")[0])
			if rsCode == rsCase.rsCode && rsCase.rsMention != "" {
				rsAssert(rsC, strings.Contains(rsErr, rsCase.rsMention), "refusal omitted the offending name or value: %s", rsErr)
			}
			rsAssert(rsC, len(rsW.rsCalls("cmd")) == 0 && len(rsW.rsCalls("other")) == 0, "a refused input started a reviewer")
			_, rsStat := os.Stat(rsOut)
			if rsStat != nil && !os.IsNotExist(rsStat) {
				rsT.Fatalf("setup: stat: %v", rsStat)
			}
			rsAssert(rsC, os.IsNotExist(rsStat), "refused input created a round folder")
			if rsCase.rsCode == 64 {
				_, rsStat = os.Stat(filepath.Dir(rsOut))
				rsAssert(rsC, os.IsNotExist(rsStat), "usage error wrote a series")
			}
		})
	}
	for rsI, rsText := range []string{"first line\nsecond line", "  inner   spaces = retained  "} {
		rsT.Run(fmt.Sprintf("accepted-%d", rsI), func(rsT *testing.T) {
			rsW := rsNew(rsT, rsC)
			rsW.rsUse(rsW.rsFake("cmd", "command", rsW.rsAnswer("cmd", 1, rsReport(true, "checked"), true), ""))
			rsOut := rsW.rsOut("accepted", "01-candidate")
			if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, "--standpoint", "cmd="+rsText) {
				return
			}
			rsAssert(rsC, rsState(rsW.rsRaw(rsOut), "cmd")["status"] == "completed" && strings.Contains(rsW.rsLast("cmd", "prompt"), rsText), "text outside the refusal list was not accepted verbatim")
		})
	}
}

// Level-two Markdown headings outside fenced code; fences in a standpoint
// must not close its containing fence or turn quoted text into a new section.
func rsHeadings(rsText string) []string {
	rsHeads := []string{}
	rsFenceLen := 0
	var rsFenceChar byte
	for _, rsLine := range strings.Split(rsText, "\n") {
		rsS := strings.TrimLeft(rsLine, " ")
		if len(rsLine)-len(rsS) > 3 {
			continue
		}
		rsN := 0
		for rsN < len(rsS) && (rsS[rsN] == '`' || rsS[rsN] == '~') && rsS[rsN] == rsS[0] {
			rsN++
		}
		switch {
		case rsFenceLen > 0:
			if rsN >= rsFenceLen && rsS[0] == rsFenceChar && strings.TrimSpace(rsS[rsN:]) == "" {
				rsFenceLen = 0
			}
		case rsN >= 3 && !(rsS[0] == '`' && strings.Contains(rsS[rsN:], "`")):
			rsFenceChar, rsFenceLen = rsS[0], rsN
		default:
			if strings.HasPrefix(rsS, "## ") {
				rsHeads = append(rsHeads, strings.TrimSpace(rsS))
			}
		}
	}
	return rsHeads
}
func TestReviewerStandpointsAC5_TextVerbatim(rsT *testing.T) {
	const rsID = "standpoint-text-not-verbatim"
	rsC := rsCollect(rsT, rsID)
	rsTexts := []string{"line one\nline two\n", "```go\n## forged\n```\n````\nend", "# a heading\n## another heading", "저장 형식의 이전 소비자를 확인하라", "  한글\n```go\n## forged\n```\n````\n# heading\ninner  spaces = 그대로\n"}
	for rsI, rsText := range rsTexts {
		rsT.Run(strconv.Itoa(rsI), func(rsT *testing.T) {
			rsW := rsNew(rsT, rsC)
			rsW.rsUse(rsW.rsFake("cmd", "command", rsW.rsAnswer("cmd", 1, rsReport(true, "checked"), true), ""))
			rsCommon := []string{"--decision", "keep shared review", "--decision-quote", "yes", "--question", "Is the boundary covered?"}
			rsPlain := rsW.rsOut("plain", "01-candidate")
			rsW.rsRun(rsID, rsPlain, "candidate", 0, 1, append(rsCommon, "--dry-run")...)
			rsOut := rsW.rsOut("text", "01-candidate")
			if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, append(rsCommon, "--standpoint", "cmd="+rsText)...) {
				return
			}
			rsAssert(rsC, strings.Contains(rsW.rsLast("cmd", "prompt"), rsText), "reviewer input did not retain the standpoint bytes")
			rsBefore := rsHeadings(rsW.rsRead(filepath.Join(rsPlain, "packet.md")))
			rsWant := []string{}
			for _, rsHeading := range rsBefore {
				if rsHeading == "## Questions for this round" {
					rsWant = append(rsWant, rsSection)
				}
				rsWant = append(rsWant, rsHeading)
			}
			rsPacket := rsW.rsRead(filepath.Join(rsOut, "packet.md"))
			rsAssert(rsC, reflect.DeepEqual(rsHeadings(rsPacket), rsWant), "standpoint text changed section headings or their order: %q", rsHeadings(rsPacket))
			rsAssert(rsC, strings.Contains(rsPacket, "Each standpoint below belongs to one reviewer. The reviewer it names attacks that risk first; every reviewer still checks every angle."), "standpoint section omitted its shared instruction")
		})
	}
}

func TestReviewerStandpointsAC6_SameInput(rsT *testing.T) {
	const rsID = "standpoint-input-identity-wrong"
	rsC := rsCollect(rsT, rsID)
	rsW := rsNew(rsT, rsC)
	rsW.rsUse(rsW.rsFake("claude", "claude", rsW.rsAnswer("claude", 1, rsReport(true, "checked"), true), ""), rsW.rsFake("codex", "codex", rsW.rsAnswer("codex", 1, nil, false), ""))
	rsW.rsWrite(filepath.Join(rsW.rsDir, "fail-claude"), "intentional")
	rsFirst := rsW.rsOut("identity", "01-candidate")
	if !rsW.rsRun(rsID, rsFirst, "candidate", 0, 1, "--standpoint", "claude=Attack stored records") {
		return
	}
	rsW.rsRemove(filepath.Join(rsW.rsDir, "fail-claude"))
	rsSame := rsW.rsOut("identity", "02-candidate")
	if !rsW.rsRun(rsID, rsSame, "candidate", 0, 1, "--reviewers", "claude", "--standpoint", "claude=Attack stored records") {
		return
	}
	rsA, rsB := rsW.rsRaw(rsFirst), rsW.rsRaw(rsSame)
	rsAssert(rsC, rsB["counted"] == false && reflect.DeepEqual(rsB["rerun_of"], []any{"01-candidate"}) && rsB["packet_sha256"] == rsA["packet_sha256"], "same standpoint rerun changed identity: counted=%v rerun_of=%v", rsB["counted"], rsB["rerun_of"])
	rsGate := rsW.rsGate("identity", 1, rsFirst, rsSame)
	for _, rsGR := range rsList(rsGate["rounds"]) {
		rsAssert(rsC, rsMap(rsGR)["completed_required"] == rsMap(rsGR)["required_reviewers"], "rerun did not complete the original packet's required reviewers: %v", rsGR)
	}
	rsCode, _, rsErr := rsW.rsCLI("waive", rsFirst, "claude", "--user-quote", rsQuote)
	rsAssert(rsC, rsCode == 65 && strings.Contains(rsErr, "same packet"), "waive did not recognize completion of the same packet: exit %d %s", rsCode, rsErr)
	rsChanged := rsW.rsOut("identity", "03-candidate")
	if !rsW.rsRun(rsID, rsChanged, "candidate", 0, 1, "--standpoint", "claude=Attack stored records differently") {
		return
	}
	rsChangedRecord := rsW.rsRaw(rsChanged)
	rsAssert(rsC, rsChangedRecord["counted"] == true && len(rsList(rsChangedRecord["rerun_of"])) == 0 && rsChangedRecord["packet_sha256"] != rsA["packet_sha256"], "changed text was treated as the old packet")
	// A changed assignment is also a new packet, even with identical text.
	rsW.rsAll(1, map[string]string{"codex": "Attack stored records"})
	rsAssigned := rsW.rsOut("identity", "04-candidate")
	if !rsW.rsRun(rsID, rsAssigned, "candidate", 0, 1, "--reviewers", "claude,codex", "--standpoint", "codex=Attack stored records") {
		return
	}
	rsD := rsW.rsRaw(rsAssigned)
	rsAssert(rsC, rsD["counted"] == true && len(rsList(rsD["rerun_of"])) == 0 && rsD["packet_sha256"] != rsA["packet_sha256"], "changed assignment did not count as a new packet")
	rsCap := rsW.rsCap(filepath.Dir(rsFirst))
	rsAssert(rsC, rsMap(rsCap["cap"])["counted"] == float64(3), "cap report does not count the three distinct packets: %v", rsCap["cap"])
	// Completion under a DIFFERENT standpoint must not prevent waiving the
	// failed original, or give the original completion credit in the gate.
	rsW.rsUse(rsW.rsFake("claude", "claude", rsW.rsAnswer("claude", 1, rsReport(true, "checked"), true), ""))
	rsW.rsWrite(filepath.Join(rsW.rsDir, "fail-claude"), "intentional")
	rsFailed := rsW.rsOut("different", "01-candidate")
	if !rsW.rsRun(rsID, rsFailed, "candidate", 0, 1, "--standpoint", "claude=original") {
		return
	}
	rsW.rsRemove(filepath.Join(rsW.rsDir, "fail-claude"))
	rsOther := rsW.rsOut("different", "02-candidate")
	if !rsW.rsRun(rsID, rsOther, "candidate", 0, 1, "--standpoint", "claude=changed") {
		return
	}
	rsG := rsW.rsGate("different", 1, rsFailed, rsOther)
	rsGRs := rsList(rsG["rounds"])
	rsAssert(rsC, len(rsGRs) == 2 && rsMap(rsGRs[0])["completed_required"] == float64(0), "different packet incorrectly completed the original reviewer")
	rsCode, _, rsErr = rsW.rsCLI("waive", rsFailed, "claude", "--user-quote", rsQuote)
	rsAssert(rsC, rsCode == 0, "different packet prevented waiver: exit %d %s", rsCode, rsErr)
	// Fixes reruns share their base only for exactly the same packet. Changing
	// a standpoint on that rerun must trigger the ordinary chain refusal.
	rsSeed := rsW.rsOut("chain", "01-candidate")
	rsW.rsRun(rsID, rsSeed, "candidate", 0, 1, "--standpoint", "claude=chain")
	rsW.rsAnswer("claude", 2, rsReport(true, "checked fix"), true)
	rsW.rsWrite(filepath.Join(rsW.rsDir, "fail-claude"), "intentional")
	rsFix := rsW.rsOut("chain", "02-fixes")
	if !rsW.rsRun(rsID, rsFix, "fixes", 1, 2, "--standpoint", "claude=chain") {
		return
	}
	rsW.rsRemove(filepath.Join(rsW.rsDir, "fail-claude"))
	rsFixAgain := rsW.rsOut("chain", "03-fixes")
	if !rsW.rsRun(rsID, rsFixAgain, "fixes", 1, 2, "--standpoint", "claude=chain") {
		return
	}
	rsAssert(rsC, rsW.rsRaw(rsFixAgain)["counted"] == false && reflect.DeepEqual(rsW.rsRaw(rsFixAgain)["rerun_of"], []any{"02-fixes"}), "same fixes packet did not rerun outside the cap")
	rsFixGate := rsW.rsGate("chain", 2, rsSeed, rsFix, rsFixAgain)
	rsAssert(rsC, rsFixGate["state"] != "blocked", "same fixes rerun broke gate chain: %v", rsFixGate["reasons"])
	rsCode, _, rsErr = rsW.rsCLI(rsW.rsArgs(rsW.rsOut("chain", "04-fixes"), "fixes", 1, 2, "--standpoint", "claude=changed chain")...)
	rsAssert(rsC, rsCode == 65 && strings.Contains(rsErr, "chain"), "changed fixes packet reused the old base: exit %d %s", rsCode, rsErr)
	// Recurring findings group reruns of one packet once, but split text and
	// assignment changes. Native pending reviewers make genuine reruns possible.
	rsW.rsUse(rsNative("a"), rsNative("b"))
	rsRecurring := []string{}
	for rsI, rsSP := range []string{"a=risk", "a=risk", "a=other risk", "b=risk"} {
		rsOut := rsW.rsOut("recurring", fmt.Sprintf("%02d-candidate", rsI+1))
		rsOptions := []string{"--standpoint", rsSP}
		if rsI == 1 {
			rsOptions = append(rsOptions, "--reviewers", "a")
		}
		// The assigned reviewer a is pending in the first round and is the
		// only reviewer selected for its same-packet rerun.
		if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, rsOptions...) {
			return
		}
		rsWho := "a"
		if rsI == 0 || rsI == 3 {
			rsWho = "b"
		}
		rsW.rsRecord(rsOut, rsWho, rsW.rsAnswer("recurring", 1, rsReport(true, "checked"), strings.HasPrefix(rsSP, rsWho+"="), rsFinding("F1", nil)), "result")
		rsRecurring = append(rsRecurring, rsOut)
	}
	rsRC := rsList(rsW.rsCap(filepath.Dir(rsRecurring[0]))["recurring"])
	rsAssert(rsC, len(rsRC) == 1 && reflect.DeepEqual(rsMap(rsRC[0])["rounds"], []any{"01-candidate", "03-candidate", "04-candidate"}), "cap-report did not split exactly the three standpoint packets: %v", rsRC)
	// Quota fallback must receive exactly the designated attempt's input.
	for _, rsPreset := range []string{"codex", "agy"} {
		rsW.rsUse(rsW.rsFake(rsPreset, rsPreset, rsW.rsAnswer(rsPreset, 1, rsReport(true, "checked fallback"), true), "fallback_model=\"synthetic-fallback\""))
		rsBefore := len(rsW.rsCalls(rsPreset))
		rsOut := rsW.rsOut("fallback-"+rsPreset, "01-candidate")
		if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, "--standpoint", rsPreset+"=Attack fallback", "--model", rsPreset+"=quota-primary") {
			return
		}
		rsCalls := rsW.rsCalls(rsPreset)[rsBefore:]
		rsSt := rsState(rsW.rsRaw(rsOut), rsPreset)
		rsAssert(rsC, len(rsCalls) == 2 && len(rsList(rsSt["attempts"])) == 2 && rsSt["fallback_used"] == true && rsSt["status"] == "completed", "%s did not make exactly one fallback attempt: %v", rsPreset, rsSt)
		if len(rsCalls) == 2 {
			rsFirstInput := rsW.rsRead(filepath.Join(rsCalls[0], "stdin"))
			rsSecondInput := rsW.rsRead(filepath.Join(rsCalls[1], "stdin"))
			if rsPreset == "agy" {
				rsFirstInput = rsArg(rsW.rsRead(filepath.Join(rsCalls[0], "argv")), "-p")
				rsSecondInput = rsArg(rsW.rsRead(filepath.Join(rsCalls[1], "argv")), "-p")
			}
			rsAssert(rsC, rsFirstInput == rsSecondInput && strings.Contains(rsFirstInput, rsInstruction(rsPreset, true)), "%s fallback input differs from designated input", rsPreset)
		}
	}
}

func TestReviewerStandpointsAC7_Uncovered(rsT *testing.T) {
	const rsID = "uncovered-standpoint-hidden"
	rsC := rsCollect(rsT, rsID)
	for _, rsMode := range []string{"result", "transcript"} {
		rsT.Run("native-"+rsMode, func(rsT *testing.T) {
			rsW := rsNew(rsT, rsC)
			rsW.rsUse(rsNative("native"))
			rsOut := rsW.rsOut("native-uncovered", "01-candidate")
			if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, "--standpoint", "native=Attack recorded completion") {
				return
			}
			rsBefore := rsW.rsRaw(rsOut)
			rsAssert(rsC, rsState(rsBefore, "native")["status"] == "pending" && reflect.DeepEqual(rsBefore["standpoints_uncovered"], []any{"native"}), "pending native standpoint must be uncovered")
			rsAssert(rsC, strings.Contains(rsW.rsRead(filepath.Join(rsOut, "summary.md")), "- Standpoints without a completed reviewer: native\n"), "pending native standpoint is missing from the summary")
			rsW.rsRecord(rsOut, "native", rsW.rsAnswer("native", 1, rsReport(true, "checked recorded completion"), true), rsMode)
			rsAfter := rsW.rsRaw(rsOut)
			rsAssert(rsC, rsState(rsAfter, "native")["status"] == "completed" && reflect.DeepEqual(rsAfter["standpoints_uncovered"], []any{}), "record did not remove the completed native reviewer from standpoints_uncovered: %v", rsAfter["standpoints_uncovered"])
			rsAssert(rsC, !strings.Contains(rsW.rsRead(filepath.Join(rsOut, "summary.md")), "Standpoints without a completed reviewer"), "record left the completed native standpoint uncovered in summary.md")
		})
	}
	for _, rsWaived := range []bool{false, true} {
		rsT.Run(fmt.Sprintf("waived-%t", rsWaived), func(rsT *testing.T) {
			rsW := rsNew(rsT, rsC)
			rsW.rsUse(rsW.rsFake("claude", "claude", rsW.rsAnswer("claude", 1, rsReport(true, "checked"), true), ""), rsW.rsFake("codex", "codex", rsW.rsAnswer("codex", 1, nil, false), ""))
			rsW.rsWrite(filepath.Join(rsW.rsDir, "fail-claude"), "intentional")
			rsOut := rsW.rsOut("uncovered", "01-candidate")
			if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, "--standpoint", "claude=Attack unread records") {
				return
			}
			if rsWaived {
				rsW.rsMust(0, "waive", rsOut, "claude", "--user-quote", rsQuote)
			}
			rsR := rsW.rsRaw(rsOut)
			rsAssert(rsC, reflect.DeepEqual(rsR["standpoints_uncovered"], []any{"claude"}), "failed or waived standpoint was not uncovered: %v", rsR["standpoints_uncovered"])
			rsSummary := rsW.rsMust(0, "summary", rsOut)
			rsAssert(rsC, strings.Contains(rsSummary, "- Standpoints without a completed reviewer: claude\n") && strings.Contains(rsSummary, "  - standpoint report: none\n"), "summary hides uncovered standpoint or absent report")
			rsBefore := rsW.rsGate("with", 1, rsOut)
			rsW.rsStrip(rsOut)
			rsAfter := rsW.rsGate("without", 1, rsOut)
			rsAssert(rsC, reflect.DeepEqual(rsGateJudgment(rsBefore), rsGateJudgment(rsAfter)), "standpoint fields changed gate state or reasons: %v versus %v", rsGateJudgment(rsBefore), rsGateJudgment(rsAfter))
		})
	}
}

// Normalize only world identities, times and execution durations. Packet
// digests are checked against the RAW bytes before replacing them with the
// digest of normalized bytes, so a stale/wrong digest cannot be normalized
// into success. No source code, git refs or previous checkout is consulted.
func rsHash(rsText string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(rsText))) }
func (rsW *rsWorld) rsNormalize(rsText string) string {
	rsPairs := []string{rsW.rsDir, "<world>"}
	for rsI, rsCommit := range rsW.rsCommits {
		rsPairs = append(rsPairs, rsCommit, fmt.Sprintf("<commit-%d>", rsI), rsCommit[:12], fmt.Sprintf("<short-commit-%d>", rsI), rsW.rsTrees[rsI], fmt.Sprintf("<tree-%d>", rsI))
	}
	rsKeys := []string{}
	for rsK := range rsW.rsReplacements {
		rsKeys = append(rsKeys, rsK)
	}
	sort.Slice(rsKeys, func(rsI, rsJ int) bool { return len(rsKeys[rsI]) > len(rsKeys[rsJ]) })
	for _, rsK := range rsKeys {
		rsPairs = append(rsPairs, rsK, rsW.rsReplacements[rsK])
	}
	// Apply hashes before IDs: their hexadecimal prefixes must not get
	// confused with a commit's display form.
	rsText = strings.NewReplacer(rsPairs...).Replace(rsText)
	rsText = regexp.MustCompile(regexp.QuoteMeta(filepath.Join(os.TempDir(), "spar-workspace-"))+`[0-9]+`).ReplaceAllString(rsText, "<scratch>")
	rsText = regexp.MustCompile(`\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?Z`).ReplaceAllString(rsText, "<time>")
	rsText = regexp.MustCompile(`("elapsed_seconds"\s*:\s*)[0-9.eE+-]+`).ReplaceAllStringFunc(rsText, func(rsField string) string {
		rsKey, rsValue, _ := strings.Cut(rsField, ":")
		rsNumber, rsErr := strconv.ParseFloat(strings.TrimSpace(rsValue), 64)
		if rsErr != nil {
			rsW.rsT.Fatalf("setup: elapsed seconds: %v", rsErr)
		}
		if rsNumber > 0 {
			return rsKey + ": 1"
		}
		return rsKey + ": 0"
	})
	rsText = regexp.MustCompile(`, [0-9]+\.[0-9]+s\)`).ReplaceAllString(rsText, ", <elapsed>s)")
	return rsText
}
func (rsW *rsWorld) rsExpand(rsText string) string {
	rsPairs := []string{"<world>", rsW.rsDir, "<time>", "2026-01-01T00:00:00Z"}
	for rsI, rsCommit := range rsW.rsCommits {
		rsPairs = append(rsPairs, fmt.Sprintf("<commit-%d>", rsI), rsCommit, fmt.Sprintf("<short-commit-%d>", rsI), rsCommit[:12], fmt.Sprintf("<tree-%d>", rsI), rsW.rsTrees[rsI])
	}
	return strings.NewReplacer(rsPairs...).Replace(rsText)
}
func (rsW *rsWorld) rsPinDigests(rsOut string) {
	rsR := rsW.rsRaw(rsOut)
	rsPacket := rsW.rsRead(filepath.Join(rsOut, "packet.md"))
	rsActual := rsString(rsR["packet_sha256"])
	if rsActual != rsHash(rsPacket) {
		rsAssert(rsW.rsChecks, false, "packet_sha256 is not the digest of packet.md")
	} else {
		rsW.rsReplacements[rsActual] = rsHash(rsW.rsNormalize(rsPacket))
	}
	rsConfig := rsMap(rsR["config"])
	rsConfigBody := rsW.rsRead(rsW.rsConfig)
	rsW.rsReplacements[rsHash(rsConfigBody)] = rsHash(rsW.rsNormalize(rsConfigBody))
	rsDigest := rsString(rsConfig["digest"])
	if len(rsDigest) >= 12 {
		rsW.rsReplacements[rsDigest] = "<config-digest>"
		rsW.rsReplacements[rsDigest[:12]] = "<short-config-digest>"
	}
}

// Remove only execution timing fields. In particular, do not remove new
// standpoint fields from reviewer entries or any other newly emitted key.
func rsWithoutTiming(rsV any) {
	switch rsValue := rsV.(type) {
	case map[string]any:
		for rsKey, rsChild := range rsValue {
			switch rsKey {
			case "started_at", "finished_at", "elapsed_seconds":
				delete(rsValue, rsKey)
			default:
				rsWithoutTiming(rsChild)
			}
		}
	case []any:
		for _, rsChild := range rsValue {
			rsWithoutTiming(rsChild)
		}
	}
}

func (rsW *rsWorld) rsSnapshot(rsOut string) map[string]any {
	rsW.rsPinDigests(rsOut)
	rsR := rsW.rsRaw(rsOut)
	rsInputs := map[string]any{}
	for _, rsValue := range rsList(rsR["requested_reviewers"]) {
		rsName := rsString(rsValue)
		if rsName == "native" {
			rsInputs[rsName] = map[string]any{"prompt.md": rsW.rsNormalize(rsW.rsRead(filepath.Join(rsOut, rsName, "prompt.md")))}
			continue
		}
		rsInputs[rsName] = map[string]any{"argv": rsW.rsNormalize(rsW.rsLast(rsName, "argv")), "stdin": rsW.rsNormalize(rsW.rsLast(rsName, "stdin"))}
		if rsName == "claude" {
			// Unlike agy/codex, claude receives schema bytes in argv, not a
			// path. Pin those exact bytes independently of schema.json.
			rsMap(rsInputs[rsName])["--json-schema"] = rsArg(rsW.rsLast(rsName, "argv"), "--json-schema")
		}
		if rsName == "command" {
			rsMap(rsInputs[rsName])["prompt"] = rsW.rsNormalize(rsW.rsLast(rsName, "prompt"))
		}
		rsCalls := rsW.rsCalls(rsName)
		rsPath := filepath.Join(rsCalls[len(rsCalls)-1], "PACKET.md")
		if rsData, rsErr := os.ReadFile(rsPath); rsErr == nil {
			rsMap(rsInputs[rsName])["PACKET.md"] = rsW.rsNormalize(string(rsData))
		} else if !os.IsNotExist(rsErr) {
			rsW.rsT.Fatalf("setup: capture PACKET.md: %v", rsErr)
		}
	}
	delete(rsR, "standpoints") // AC-8 separately requires this to be exactly [].
	rsWithoutTiming(rsR)
	return map[string]any{"inputs": rsInputs, "round.json": rsW.rsJSON(rsW.rsNormalize(rsW.rsEncode(rsR))), "packet.md": rsW.rsNormalize(rsW.rsRead(filepath.Join(rsOut, "packet.md"))), "schema.json": rsW.rsRead(filepath.Join(rsOut, "schema.json")), "packet_sha256": rsW.rsNormalize(rsString(rsR["packet_sha256"])), "summary.md": rsW.rsNormalize(rsW.rsRead(filepath.Join(rsOut, "summary.md")))}
}
func (rsW *rsWorld) rsStages() (map[string]any, []string) {
	rsSnapshots := map[string]any{}
	rsRounds := []string{}
	for rsI, rsStage := range []string{"plan", "plan-delta", "candidate", "fixes"} {
		rsBase, rsHead := 0, 1
		if rsStage == "fixes" {
			rsBase, rsHead = 1, 2
		}
		rsW.rsAll(rsHead, nil)
		rsOut := rsW.rsOut("baseline", fmt.Sprintf("%02d-%s", rsI+1, rsStage))
		rsExtra := []string{"--question", "Is AC-1 covered?"}
		if rsStage == "plan" || rsStage == "plan-delta" {
			rsExtra = append(rsExtra, "--subject", filepath.Join(rsW.rsRepo, "PLAN.md"))
		}
		rsW.rsRun("", rsOut, rsStage, rsBase, rsHead, rsExtra...)
		rsSnapshots[rsStage] = rsW.rsSnapshot(rsOut)
		rsRounds = append(rsRounds, rsOut)
	}
	// A separate series avoids changing the legacy rerun fixture's history.
	rsW.rsUse(rsW.rsFake("agy", "agy", rsW.rsAnswer("agy-large", 1, nil, false), ""))
	rsLarge := rsW.rsOut("baseline-large", "01-candidate")
	rsW.rsRun("", rsLarge, "candidate", 0, 1, "--criteria", rsW.rsLargeCriteria())
	rsSnapshots["agy-file"] = rsW.rsSnapshot(rsLarge)
	rsRounds = append(rsRounds, rsLarge)
	return rsSnapshots, rsRounds
}
func (rsW *rsWorld) rsLoadFixture() map[string]any {
	rsW.rsT.Helper()
	rsFixtureValue := rsMap(rsW.rsJSON(rsW.rsRead(rsFixture)))
	if rsFixtureValue["format"] != "reviewer-standpoints-base/v1" {
		rsW.rsT.Fatal("setup: base.json has an unknown fixture format")
	}
	for _, rsKey := range []string{"stages", "rerun", "legacy", "legacy_expected", "config"} {
		if len(rsMap(rsFixtureValue[rsKey])) == 0 {
			rsW.rsT.Fatalf("setup: base.json is missing %s", rsKey)
		}
	}
	return rsFixtureValue
}

// Store only public, synthetic record files, never reviewer scratch output.
func (rsW *rsWorld) rsArchive(rsPaths ...string) map[string]any {
	rsArchive := map[string]any{}
	for _, rsPath := range rsPaths {
		rsRel, rsErr := filepath.Rel(rsW.rsDir, rsPath)
		if rsErr != nil {
			rsW.rsT.Fatalf("setup: archive: %v", rsErr)
		}
		rsArchive[filepath.ToSlash(rsRel)] = rsW.rsNormalize(rsW.rsRead(rsPath))
	}
	return rsArchive
}
func (rsW *rsWorld) rsRestore(rsArchive map[string]any) {
	for rsRel, rsValue := range rsArchive {
		rsW.rsWrite(filepath.Join(rsW.rsDir, filepath.FromSlash(rsRel)), rsW.rsExpand(rsString(rsValue)))
	}
}
func (rsW *rsWorld) rsLegacySetup() map[string]any {
	rsW.rsUse(rsNative("native"), rsW.rsFake("agy", "agy", rsW.rsAnswer("agy", 1, nil, false), "fallback_model=\"\""))
	rsW.rsWrite(filepath.Join(rsW.rsDir, "fail-agy"), "intentional")
	rsOut := rsW.rsOut("legacy", "01-candidate")
	rsW.rsRun("", rsOut, "candidate", 0, 1)
	rsW.rsRecord(rsOut, "native", rsW.rsAnswer("legacy", 1, nil, false, rsFinding("F1", nil), rsFinding("F2", map[string]any{"location": "head/checks/test_app.py:1", "link_kind": "none", "link": "", "introduced_by_change": "no"})), "result")
	rsW.rsGate("legacy", 1, rsOut)
	rsW.rsMust(0, "extend", filepath.Dir(rsOut), "--stage", "code", "--by", "1", "--cause", "other", "--reason", "synthetic extension", "--user-quote", rsQuote)
	// Preserve exact old records for the new readers, including a prior gate
	// so the replacement-gate path exercises previous_gate_digest.
	return rsW.rsArchive(filepath.Join(rsOut, "round.json"), filepath.Join(rsOut, "findings.json"), filepath.Join(rsOut, "ledger.jsonl"), filepath.Join(rsOut, "summary.md"), filepath.Join(filepath.Dir(rsOut), "extensions.jsonl"), rsW.rsOut("gates", "legacy.json"))
}
func (rsW *rsWorld) rsLegacyRead() map[string]any {
	rsOut := rsW.rsOut("legacy", "01-candidate")
	rsResult := map[string]any{}
	// These are observations, not setup: retain exit codes and output in the
	// fixture, including a command refusing a valid old record after a change.
	rsCode, rsStdout, rsStderr := rsW.rsCLI("triage", rsOut, "native-F1", "reject", "--evidence", "synthetic source proves the finding false")
	rsResult["triage"] = []any{rsCode, rsW.rsNormalize(rsStdout), rsW.rsNormalize(rsStderr)}
	rsCode, rsStdout, rsStderr = rsW.rsCLI("waive", rsOut, "agy", "--user-quote", rsQuote)
	rsResult["waive"] = []any{rsCode, rsW.rsNormalize(rsStdout), rsW.rsNormalize(rsStderr)}
	rsCode, rsStdout, rsStderr = rsW.rsCLI("cap-report", filepath.Dir(rsOut), "--format", "json", "--repo", rsW.rsRepo)
	rsResult["cap-report"] = []any{rsCode, rsW.rsNormalize(rsStdout), rsW.rsNormalize(rsStderr)}
	rsGate := rsW.rsGate("legacy", 1, rsOut)
	// This digest covers timestamps in the previous file. Pin its semantics
	// by checking it against the bytes supplied before the command, separately.
	delete(rsGate, "previous_gate_digest")
	rsResult["gate"] = rsW.rsJSON(rsW.rsNormalize(rsW.rsEncode(rsGate)))
	rsResult["ledger"] = rsW.rsNormalize(rsW.rsRead(filepath.Join(rsOut, "ledger.jsonl")))
	rsResult["summary"] = rsW.rsNormalize(rsW.rsMust(0, "summary", rsOut))
	return rsResult
}

// Reflection lists the accepted TOML fields, and CLI probes below verify
// that they remain accepted. This observes any added key, even one this goal
// did not predict, without parsing implementation source text.
func rsConfigKeys() map[string]any {
	rsResult := map[string]any{}
	for rsName, rsType := range map[string]reflect.Type{"user": reflect.TypeOf(userFile{}), "reviewer": reflect.TypeOf(reviewerEntry{}), "project": reflect.TypeOf(projectFile{}), "caps": reflect.TypeOf(capsEntry{})} {
		rsKeys := []string{}
		for rsI := 0; rsI < rsType.NumField(); rsI++ {
			rsKey := rsType.Field(rsI).Tag.Get("toml")
			if rsKey != "" && rsKey != "-" {
				rsKeys = append(rsKeys, rsKey)
			}
		}
		sort.Strings(rsKeys)
		rsResult[rsName] = rsKeys
	}
	return rsResult
}
func (rsW *rsWorld) rsConfigSnapshot() map[string]any {
	rsW.rsAll(1, nil)
	rsOut := rsW.rsMust(0, "config", "--repo", rsW.rsRepo)
	rsConfigBody := rsW.rsRead(rsW.rsConfig)
	rsW.rsReplacements[rsHash(rsConfigBody)] = rsHash(rsW.rsNormalize(rsConfigBody))
	rsCfg := rsMap(rsW.rsJSON(rsOut))
	rsDigest := rsString(rsCfg["digest"])
	if len(rsDigest) > 12 {
		rsW.rsReplacements[rsDigest] = "<config-digest>"
		rsW.rsReplacements[rsDigest[:12]] = "<short-config-digest>"
	}
	rsResult := map[string]any{"output": rsW.rsNormalize(rsOut), "keys": rsConfigKeys()}
	rsProbes := map[string]any{}
	for _, rsProbe := range []struct{ rsName, rsUser, rsProject string }{
		{"all-user-keys", "timeout_seconds=60\n[[reviewers]]\nname=\"r\"\npreset=\"claude\"\nbinary=\"synthetic\"\nmodel=\"m\"\nfallback_model=\"fb\"\neffort=\"low\"\ntimeout_seconds=30\n", ""},
		{"allowed-lists", "[[reviewers]]\nname=\"r\"\npreset=\"codex\"\nallowed_models=[\"m\"]\nallowed_efforts=[\"high\"]\n", ""},
		{"command-argv", "[[reviewers]]\nname=\"cmd\"\npreset=\"command\"\nargv=[\"/bin/cat\",\"{prompt}\"]\n", ""},
		{"all-project-keys", rsNative("native"), "angles=[\"design\"]\nrules=[\"AGENTS.md\"]\ncheck_paths=[\"checks/**\"]\n[caps]\nplan=2\nplan_delta=2\ncode=4\n"},
		{"user-standpoint", "standpoint=\"risk\"\n" + rsNative("native"), ""},
		{"user-standpoints", "standpoints=[]\n" + rsNative("native"), ""},
		{"reviewer-standpoint", rsNative("native") + "standpoint=\"risk\"\n", ""},
		{"project-standpoint", rsNative("native"), "standpoint=\"risk\"\n"},
		{"project-standpoints", rsNative("native"), "standpoints=[]\n"},
		{"project-reviewers", rsNative("native"), "reviewers=[\"native\"]\n"},
	} {
		rsW.rsUse(rsProbe.rsUser)
		rsProject := filepath.Join(rsW.rsRepo, ".jaekit/spar.toml")
		rsW.rsWrite(rsProject, rsProbe.rsProject)
		rsCode, _, rsErr := rsW.rsCLI("config", "--repo", rsW.rsRepo)
		rsProbes[rsProbe.rsName] = []any{rsCode, rsW.rsNormalize(rsErr)}
	}
	rsW.rsRemove(filepath.Join(rsW.rsRepo, ".jaekit/spar.toml"))
	rsResult["probes"] = rsProbes
	return rsResult
}
func TestReviewerStandpointsAC8_NoStandpointUnchanged(rsT *testing.T) {
	const rsID = "no-standpoint-round-changed"
	rsC := rsCollect(rsT, rsID)
	rsW := rsNew(rsT, rsC)
	rsSnapshots, rsRounds := rsW.rsStages()
	if os.Getenv("RS_WRITE_BASE") == "1" {
		if _, rsPresent := rsW.rsRaw(rsRounds[0])["standpoints"]; rsPresent {
			rsT.Fatal("setup: refuse to regenerate base.json on a standpoint-aware tree")
		}
		// Candidate's native reviewer is still pending, so its old record is a
		// real rerun target after the product gains standpoints.
		rsCandidate := rsRounds[2]
		rsArchive := rsW.rsArchive(filepath.Join(rsCandidate, "round.json"), filepath.Join(rsCandidate, "findings.json"), filepath.Join(rsCandidate, "ledger.jsonl"))
		rsLegacy := rsW.rsLegacySetup()
		rsLegacyExpected := rsW.rsLegacyRead()
		rsConfig := rsW.rsConfigSnapshot()
		rsW.rsWrite(rsFixture, rsW.rsEncode(map[string]any{"format": "reviewer-standpoints-base/v1", "stages": rsSnapshots, "rerun": rsArchive, "legacy": rsLegacy, "legacy_expected": rsLegacyExpected, "config": rsConfig}))
		return
	}
	rsFixtureValue := rsW.rsLoadFixture()
	for _, rsStage := range []string{"plan", "plan-delta", "candidate", "fixes", "agy-file"} {
		rsAssert(rsC, reflect.DeepEqual(rsW.rsJSON(rsW.rsEncode(rsSnapshots[rsStage])), rsMap(rsFixtureValue["stages"])[rsStage]), "%s no-standpoint input, packet, schema, digest, whole round.json or summary differs from base.json", rsStage)
	}
	for _, rsOut := range rsRounds {
		rsR := rsW.rsRaw(rsOut)
		rsSP, rsPresent := rsR["standpoints"]
		rsAssert(rsC, rsPresent && reflect.DeepEqual(rsSP, []any{}), "%s must record explicit standpoints: []", filepath.Base(rsOut))
		for _, rsName := range rsNames {
			_, rsErr := os.Stat(filepath.Join(rsOut, rsName, "input.md"))
			if rsErr != nil && !os.IsNotExist(rsErr) {
				rsT.Fatalf("setup: stat: %v", rsErr)
			}
			rsAssert(rsC, os.IsNotExist(rsErr), "no-standpoint round created %s/input.md", rsName)
		}
	}
	// Restore the OLD candidate record and its digest, not a record newly
	// emitted by the product under test. Translate its normalized packet hash
	// back to the actual packet bytes in this temporary world.
	rsArchive := rsMap(rsFixtureValue["rerun"])
	rsCurrentPacket := rsW.rsRead(filepath.Join(rsRounds[2], "packet.md"))
	rsNormalizedHash := rsString(rsMap(rsMap(rsFixtureValue["stages"])["candidate"])["packet_sha256"])
	for rsRel, rsValue := range rsArchive {
		rsText := strings.ReplaceAll(rsString(rsValue), rsNormalizedHash, rsHash(rsCurrentPacket))
		rsW.rsWrite(filepath.Join(rsW.rsDir, rsRel), rsW.rsExpand(rsText))
	}
	rsW.rsAll(1, nil)
	rsAgain := rsW.rsOut("baseline", "05-candidate")
	rsW.rsRun(rsID, rsAgain, "candidate", 0, 1, "--reviewers", "native", "--question", "Is AC-1 covered?")
	rsAgainRecord := rsW.rsRaw(rsAgain)
	rsAssert(rsC, rsAgainRecord["counted"] == false && reflect.DeepEqual(rsAgainRecord["rerun_of"], []any{"03-candidate"}), "old no-standpoint packet is not an uncounted rerun: %v %v", rsAgainRecord["counted"], rsAgainRecord["rerun_of"])
}

func TestReviewerStandpointsAC11_EarlierRecords(rsT *testing.T) {
	rsC := rsCollect(rsT, "")
	rsW := rsNew(rsT, rsC)
	rsFixtureValue := rsW.rsLoadFixture()
	rsW.rsRestore(rsMap(rsFixtureValue["legacy"]))
	rsPrevious := rsW.rsRead(rsW.rsOut("gates", "legacy.json"))
	rsGot := rsW.rsLegacyRead()
	rsAssert(rsC, reflect.DeepEqual(rsW.rsJSON(rsW.rsEncode(rsGot)), rsFixtureValue["legacy_expected"]), "legacy gate/cap-report/triage/waive results differ from base.json\ngot: %s\nwant: %s", rsW.rsEncode(rsGot), rsW.rsEncode(rsFixtureValue["legacy_expected"]))
	rsGate := rsMap(rsW.rsJSON(rsW.rsRead(rsW.rsOut("gates", "legacy.json"))))
	rsAssert(rsC, rsGate["previous_gate_digest"] == rsHash(rsPrevious), "legacy gate replacement did not preserve previous_gate_digest")
}
func TestReviewerStandpointsAC12_Config(rsT *testing.T) {
	rsC := rsCollect(rsT, "")
	rsW := rsNew(rsT, rsC)
	rsFixtureValue := rsW.rsLoadFixture()
	rsGot := rsW.rsConfigSnapshot()
	rsAssert(rsC, reflect.DeepEqual(rsW.rsJSON(rsW.rsEncode(rsGot)), rsFixtureValue["config"]), "configuration output, accepted keys or refusal probes changed\ngot: %s\nwant: %s", rsW.rsEncode(rsGot), rsW.rsEncode(rsFixtureValue["config"]))
}

func TestReviewerStandpointsAC9_ClassificationIndependent(rsT *testing.T) {
	const rsID = "classification-reads-standpoint"
	rsC := rsCollect(rsT, rsID)
	rsW := rsNew(rsT, rsC)
	rsObserved := []any{}
	for rsI, rsSP := range []string{"", "Attack classification from a consumer's position"} {
		rsSeries := fmt.Sprintf("classification-%d", rsI)
		rsW.rsUse(rsNative("native"))
		rsOut := rsW.rsOut(rsSeries, "01-candidate")
		rsOptions := []string{}
		if rsSP != "" {
			rsOptions = []string{"--standpoint", "native=" + rsSP}
		}
		if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, rsOptions...) {
			return
		}
		rsFindings := []any{
			rsFinding("F1", nil), rsFinding("F2", map[string]any{"proposed_class": "requirement-question", "introduced_by_change": "no"}),
			rsFinding("F3", map[string]any{"location": "head/checks/test_app.py:1", "link_kind": "none", "link": "", "introduced_by_change": "no"}),
			rsFinding("F4", map[string]any{"proposed_class": "note", "link_kind": "none", "link": "", "introduced_by_change": "no"}),
			rsFinding("F5", nil), rsFinding("F6", nil), rsFinding("F7", nil),
		}
		rsW.rsRecord(rsOut, "native", rsW.rsAnswer("class", 1, rsReport(true, "checked"), rsSP != "", rsFindings...), "result")
		rsBefore := rsW.rsGate("classification-before-"+strconv.Itoa(rsI), 1, rsOut)
		rsClasses := []any{}
		for _, rsF := range rsList(rsW.rsJSON(rsW.rsRead(filepath.Join(rsOut, "findings.json")))) {
			rsClasses = append(rsClasses, rsMap(rsF)["class"])
		}
		rsAssert(rsC, reflect.DeepEqual(rsClasses, []any{"defect", "requirement-question", "hardening", "note", "defect", "defect", "defect"}), "standpoint changed representative finding classes: %v", rsClasses)
		for _, rsArgs := range [][]string{
			{"triage", rsOut, "native-F1", "fix", "--commit", rsW.rsCommits[2], "--failed-before", "synthetic failure", "--passed-after", "synthetic pass"},
			{"triage", rsOut, "native-F2", "answer", "--user-quote", "synthetic decision"},
			{"triage", rsOut, "native-F5", "reject", "--evidence", "synthetic disproof"},
			{"triage", rsOut, "native-F6", "duplicate", "--of", "native-F5"},
			{"triage", rsOut, "native-F7", "defer", "--to", "synthetic follow-up", "--user-quote", "defer this"},
		} {
			rsCode, _, rsErr := rsW.rsCLI(rsArgs...)
			rsAssert(rsC, rsCode == 0, "standpoint changed disposition acceptance: %v exit %d %s", rsArgs[2:4], rsCode, rsErr)
		}
		rsFix := rsW.rsOut(rsSeries, "02-fixes")
		if !rsW.rsRun(rsID, rsFix, "fixes", 1, 2, rsOptions...) {
			return
		}
		rsW.rsRecord(rsFix, "native", rsW.rsAnswer("fix", 2, rsReport(true, "checked"), rsSP != ""), "result")
		rsCumulative := rsW.rsOut(rsSeries, "03-candidate")
		if !rsW.rsRun(rsID, rsCumulative, "candidate", 0, 2, rsOptions...) {
			return
		}
		rsW.rsRecord(rsCumulative, "native", rsW.rsAnswer("cumulative", 2, rsReport(true, "checked"), rsSP != ""), "result")
		rsCapCode, _, rsCapErr := rsW.rsCLI(rsW.rsArgs(rsW.rsOut(rsSeries, "04-candidate"), "candidate", 0, 2, rsOptions...)...)
		rsAssert(rsC, rsCapCode == 65 && strings.Contains(rsCapErr, "cap"), "standpoint changed code cap refusal: %d %s", rsCapCode, rsCapErr)
		rsAfter := rsW.rsGate("classification-after-"+strconv.Itoa(rsI), 2, rsOut, rsFix, rsCumulative)
		rsRecord := map[string]any{"findings": rsW.rsJSON(rsW.rsRead(filepath.Join(rsOut, "findings.json"))), "ledger": rsW.rsRead(filepath.Join(rsOut, "ledger.jsonl")), "gate_before": rsGateJudgment(rsBefore), "gate_after": rsGateJudgment(rsAfter), "cap_report": rsW.rsCap(filepath.Dir(rsOut)), "counted": []any{rsW.rsRaw(rsOut)["counted"], rsW.rsRaw(rsFix)["counted"], rsW.rsRaw(rsCumulative)["counted"]}}
		rsText := strings.ReplaceAll(rsW.rsNormalize(rsW.rsEncode(rsRecord)), rsSeries, "classification")
		rsObserved = append(rsObserved, rsW.rsJSON(rsText))
	}
	rsAssert(rsC, reflect.DeepEqual(rsObserved[0], rsObserved[1]), "classification, dispositions, caps, chain or gate judgments depend on standpoints")
}

func TestReviewerStandpointsAC10_SameReviewers(rsT *testing.T) {
	const rsID = "reviewer-count-changed"
	rsC := rsCollect(rsT, rsID)
	rsW := rsNew(rsT, rsC)
	rsObserved := []any{}
	for rsI := 0; rsI < 2; rsI++ {
		rsTexts := map[string]string{}
		rsOptions := []string{}
		if rsI == 1 {
			rsTexts["claude"] = "Attack reviewer admission"
			rsTexts["native"] = "Attack required reviewers"
			rsOptions = []string{"--standpoint", "claude=" + rsTexts["claude"], "--standpoint", "native=" + rsTexts["native"]}
		}
		rsW.rsAll(1, rsTexts)
		rsBefore := map[string]int{}
		for _, rsName := range rsNames[:4] {
			rsBefore[rsName] = len(rsW.rsCalls(rsName))
		}
		rsSeries := fmt.Sprintf("reviewers-%d", rsI)
		rsOut := rsW.rsOut(rsSeries, "01-candidate")
		if !rsW.rsRun(rsID, rsOut, "candidate", 0, 1, rsOptions...) {
			return
		}
		rsW.rsRecord(rsOut, "native", rsW.rsAnswer("native", 1, rsReport(true, "checked native"), rsI == 1), "result")
		rsR := rsW.rsRaw(rsOut)
		rsProcesses := map[string]any{}
		rsAttempts := map[string]any{}
		for _, rsName := range rsNames {
			if rsName != "native" {
				rsProcesses[rsName] = len(rsW.rsCalls(rsName)) - rsBefore[rsName]
				rsAssert(rsC, rsProcesses[rsName] == 1, "%s did not start exactly once", rsName)
			}
			rsSt := rsState(rsR, rsName)
			rsAttempts[rsName] = map[string]any{"attempts": len(rsList(rsSt["attempts"])), "status": rsSt["status"], "required": rsSt["required"], "preset": rsSt["preset"]}
		}
		rsG := rsW.rsGate("reviewers-"+strconv.Itoa(rsI), 1, rsOut)
		rsGRs := rsList(rsG["rounds"])
		rsRequired := []any{}
		for _, rsGR := range rsGRs {
			rsRequired = append(rsRequired, []any{rsMap(rsGR)["required_reviewers"], rsMap(rsGR)["completed_required"]})
		}
		rsObservation := map[string]any{"processes": rsProcesses, "attempts": rsAttempts, "requested": rsR["requested_reviewers"], "required": rsR["required_reviewers"], "gate": rsGateJudgment(rsG), "gate_required": rsRequired}
		rsObserved = append(rsObserved, rsW.rsJSON(strings.ReplaceAll(rsW.rsNormalize(rsW.rsEncode(rsObservation)), rsSeries, "reviewers")))
	}
	rsAssert(rsC, reflect.DeepEqual(rsObserved[0], rsObserved[1]), "standpoints changed process names/count, attempts or gate required-reviewer judgment")
}
