package plugins

// Checks of the seal-slim-review-tiers goal: a shorter Seal skill that leaves
// mechanical work to ha (report, doctor, the automatic budget and the start
// requirement check), leaves review mechanics to Spar, and applies the SPEC
// Assurance tier. Each test observes one property of the current tree and
// opens every failure message with its violation marker, which ha's
// go-test-json adapter reads. A file the property itself requires that is
// missing is that violation; an input the test cannot read, a tool that does
// not start, or a released ha that is not given is skipped, never a violation.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	ssSkill      = "seal/skills/seal/SKILL.md"
	ssSkillDir   = "seal/skills/seal"
	ssBundleRef  = "seal/skills/seal/references/bundle.md"
	ssHaRef      = "seal/skills/seal/references/ha.md"
	ssEarlierRef = "seal/skills/seal/references/earlier-goals.md"
	ssDecl       = "seal/skills/seal/core-requirements.json"
	ssHelper     = "seal/skills/seal/scripts/check_core.py"
	ssSpar       = "spar/skills/spar/SKILL.md"
	// The Seal and Spar versions at the base this goal starts from.
	ssBaseSeal = "0.1.19"
	ssBaseSpar = "0.1.0"
	// The skill's description stays as it was.
	ssDescription = `Carry a goal in docs/specs/<goal>/ to recorded completion — plan, implement, verify through the ha binary, and resume in a new session. Use when the user calls this skill with a goal or asks to implement, finish, or continue a goal document, for example "$seal:seal docs/specs/x" in Codex, "/seal:seal docs/specs/x" in Claude Code, "docs/specs/x 구현해줘", or "docs/specs/x 이어서 해줘".`
)

// ssViolate reports one failure of the property id; every failure line of a
// test carries the same marker.
func ssViolate(t *testing.T, id, format string, args ...any) {
	t.Helper()
	t.Errorf("jaekit:violation="+id+" "+format, args...)
}

// ssRead returns a file the property requires. Missing is the violation id;
// any other read error leaves the property unobserved.
func ssRead(t *testing.T, id, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("jaekit:violation=%s %s is missing", id, path)
	}
	if err != nil {
		t.Skipf("unobserved: %s: %v", path, err)
	}
	return string(b)
}

// ssFlat joins whitespace runs so that wrapped lines match a phrase.
func ssFlat(s string) string { return strings.Join(strings.Fields(s), " ") }

func ssRequire(t *testing.T, id, where, text string, phrases ...string) {
	t.Helper()
	flat := ssFlat(text)
	for _, p := range phrases {
		if !strings.Contains(flat, ssFlat(p)) {
			ssViolate(t, id, "%s lacks %q", where, p)
		}
	}
}

func ssForbid(t *testing.T, id, where, text string, phrases ...string) {
	t.Helper()
	flat := ssFlat(text)
	for _, p := range phrases {
		if strings.Contains(flat, ssFlat(p)) {
			ssViolate(t, id, "%s still has %q", where, p)
		}
	}
}

// ssSection returns the lines from heading to the next heading of the same or
// a higher level, outside fenced code; "" when the heading is absent.
func ssSection(text, heading string) string {
	level := strings.Index(heading, " ")
	lines := strings.Split(text, "\n")
	start, fenced := -1, false
	for i, line := range lines {
		if strings.HasPrefix(line, "```") {
			fenced = !fenced
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
			return strings.Join(lines[start:i], "\n")
		}
	}
	if start < 0 {
		return ""
	}
	return strings.Join(lines[start:], "\n")
}

func ssNeedSection(t *testing.T, id, path, text, heading string) string {
	t.Helper()
	sec := ssSection(text, heading)
	if sec == "" {
		t.Fatalf("jaekit:violation=%s %s lacks the section %q", id, path, heading)
	}
	return sec
}

// ssFront returns the flat front matter keys and metadata.version of a skill.
func ssFront(text string) (map[string]string, string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) == 0 || lines[0] != "---" {
		return nil, "", false
	}
	keys, version, inMeta := map[string]string{}, "", false
	for _, l := range lines[1:] {
		if l == "---" {
			return keys, version, true
		}
		k, v, _ := strings.Cut(strings.TrimSpace(l), ":")
		v = strings.Trim(strings.TrimSpace(v), `"`)
		if strings.HasPrefix(l, " ") {
			if inMeta && k == "version" {
				version = v
			}
			continue
		}
		keys[k] = v
		inMeta = k == "metadata"
	}
	return nil, "", false
}

// ssNewer reports whether dotted version a is greater than b; ok is false
// when either is not all numbers.
func ssNewer(a, b string) (newer, ok bool) {
	pa, pb := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(pa) || i < len(pb); i++ {
		var x, y int
		var err error
		if i < len(pa) {
			if x, err = strconv.Atoi(pa[i]); err != nil {
				return false, false
			}
		}
		if i < len(pb) {
			if y, err = strconv.Atoi(pb[i]); err != nil {
				return false, false
			}
		}
		if x != y {
			return x > y, true
		}
	}
	return false, true
}

// ssVersions returns the plugin's version in its two manifests and in the
// Claude Code marketplace. The Codex marketplace has no version field.
func ssVersions(t *testing.T, id, name string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, host := range []string{".claude-plugin", ".codex-plugin"} {
		path := filepath.Join(name, host, "plugin.json")
		var m struct{ Version string }
		if err := json.Unmarshal([]byte(ssRead(t, id, path)), &m); err != nil {
			ssViolate(t, id, "%s is not JSON: %v", path, err)
			continue
		}
		out[path] = m.Version
	}
	var market struct {
		Plugins []struct{ Name, Version string }
	}
	path := "../.claude-plugin/marketplace.json"
	if err := json.Unmarshal([]byte(ssRead(t, id, path)), &market); err != nil {
		ssViolate(t, id, "%s is not JSON: %v", path, err)
		return out
	}
	for _, p := range market.Plugins {
		if p.Name == name {
			out[path] = p.Version
		}
	}
	return out
}

// ssStartForm is the ha start command the skill tells the agent to run: the
// code span that starts with "ha start <goal>" and passes the skill.
var ssStartSpan = regexp.MustCompile("`(ha start <goal> [^`]*--skill [^`]*)`")

func ssStartForm(skill string) string {
	for _, m := range ssStartSpan.FindAllStringSubmatch(skill, -1) {
		return m[1]
	}
	return ""
}

// ssStartArgv fills the start form's placeholders for a synthetic goal.
func ssStartArgv(form, goal, skillPath string) ([]string, error) {
	var out []string
	for _, word := range regexp.MustCompile(`"[^"]*"|<[^>]*>|\S+`).FindAllString(form, -1) {
		switch {
		case word == "ha" && len(out) == 0:
			continue
		case word == "<goal>":
			word = goal
		case word == `"<the user's words>"`:
			word = "synthetic request"
		case word == "<path to this SKILL.md>":
			word = skillPath
		case strings.Contains(word, "<"):
			return nil, fmt.Errorf("unknown placeholder %s", word)
		}
		out = append(out, word)
	}
	return out, nil
}

// ssRun runs a tool; a tool that cannot start leaves the property unobserved.
func ssRun(t *testing.T, dir string, env []string, name string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir, cmd.Env = dir, env
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	var exit *exec.ExitError
	switch {
	case err == nil:
		return out.String(), 0
	case errors.As(err, &exit) && exit.Exited():
		return out.String(), exit.ExitCode()
	default:
		t.Skipf("unobserved: %s %v did not run to an exit code: %v %s", name, args, err, out.String())
		return "", 0
	}
}

// ssRepo is a synthetic Git repository with one goal, outside this tree.
type ssRepo struct {
	dir, goal string
	env       []string
}

func ssNewRepo(t *testing.T, plan string) *ssRepo {
	t.Helper()
	r := &ssRepo{dir: t.TempDir(), goal: "docs/specs/demo"}
	r.env = append(os.Environ(),
		"XDG_CONFIG_HOME="+t.TempDir(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=synthetic", "GIT_AUTHOR_EMAIL=synthetic@example.invalid",
		"GIT_COMMITTER_NAME=synthetic", "GIT_COMMITTER_EMAIL=synthetic@example.invalid")
	files := map[string]string{
		"go.mod": "module example.com/demo\n\ngo 1.21\n",
		"greet/greet_test.go": "package greet\n\nimport (\n\t\"os\"\n\t\"testing\"\n)\n\n" +
			"func TestGreeting(t *testing.T) {\n\tb, err := os.ReadFile(\"greeting.txt\")\n" +
			"\tif err != nil || string(b) != \"hello\\n\" {\n\t\tt.Fatalf(\"jaekit:violation=greeting-missing greeting.txt holds %q\", b)\n\t}\n}\n",
		"checks/AC-1.json": `{"schema": "check-adapter/v1", "adapter": "go-test-json", "targets": [{"id": "greeting", "violation": "greeting-missing", "test": {"package": "example.com/demo/greet", "name": "TestGreeting"}}]}` + "\n",
		r.goal + "/SPEC.md": "# Greeting\n\nStatus: Ready\nCriteria-Format: nested/1\n\n## 목표\n\n`greet/greeting.txt` holds `hello`.\n\n" +
			"## Acceptance Criteria\n- **AC-1** `greet/greeting.txt` holds `hello` and a newline.\n\n## Open Decisions\n\n없음\n",
		r.goal + "/PLAN.md":     plan,
		r.goal + "/REVIEW.md":   "# REVIEW — demo\n\n## 결론\nready\n",
		r.goal + "/PROGRESS.md": "# PROGRESS — demo\n\n## 현재\nsynthetic\n\n## Task 상태\n| Task | 상태 | 메모 |\n| --- | --- | --- |\n| T001 | done | |\n",
	}
	for name, content := range files {
		path := filepath.Join(r.dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Skipf("unobserved: synthetic repository: %v", err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Skipf("unobserved: synthetic repository: %v", err)
		}
	}
	r.git(t, "init", "-q")
	r.commit(t, "base")
	return r
}

func (r *ssRepo) git(t *testing.T, args ...string) {
	t.Helper()
	if out, code := ssRun(t, r.dir, r.env, "git", args...); code != 0 {
		t.Skipf("unobserved: git %v exited %d: %s", args, code, out)
	}
}

func (r *ssRepo) commit(t *testing.T, message string) {
	t.Helper()
	r.git(t, "add", "-A")
	r.git(t, "-c", "commit.gpgsign=false", "commit", "-qm", message)
}

func (r *ssRepo) records() bool {
	_, err := os.Stat(filepath.Join(r.dir, r.goal, "runs.jsonl"))
	return err == nil
}

const ssPlan = "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
	"| AC-1 | change | `go test -json -count=1 ./greet/` | greet/greet_test.go, checks/AC-1.json | T001 |\n\n" +
	"## 결과 계약\n| ID | 선언 경로 |\n| --- | --- |\n| AC-1 | checks/AC-1.json |\n\n" +
	"## 범위\n- 바꿀 수 있는 경로: `greet/**` (SPEC: 목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n" +
	"## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
	"| T001 | greeting.txt holds hello | AC-1 | — | — | 예 | — |\n\n" +
	"## 예산\n- 검증 실행: %s\n- 경과 시간: 1시간\n- 비용: 미관측\n\n## 실행 환경\n- git: `git --version`\n"

// ssPlanMaintain is a goal ha 0.1.6 can start: one maintain condition and no
// runner adapter declaration, which that release does not read.
const ssPlanMaintain = "# PLAN — demo\n\n## 조건표\n| ID | 종류 | 검증 명령 | 검사 경로 | Task |\n| --- | --- | --- | --- | --- |\n" +
	"| AC-1 | maintain | `git --version` | — | T001 |\n\n" +
	"## 범위\n- 바꿀 수 있는 경로: `greet/**` (SPEC: 목표)\n- 테스트 경로: (기본값)\n- 유지할 동작: 없음\n\n" +
	"## Task\n| ID | 목표 | 근거 | 제안 | 선행 | 필수 | 문서 |\n| --- | --- | --- | --- | --- | --- | --- |\n" +
	"| T001 | greeting.txt holds hello | AC-1 | — | — | 예 | — |\n\n" +
	"## 예산\n- 검증 실행: 10\n- 경과 시간: 1시간\n- 비용: 미관측\n"

func ssAbs(t *testing.T, path string) string {
	t.Helper()
	abs, err := filepath.Abs(path)
	if err != nil {
		t.Skipf("unobserved: %v", err)
	}
	return abs
}

// AC-13: the start form the skill names checks the skill's declaration.
func TestSealSlimAC13_StartFormChecksSkill(t *testing.T) {
	const id = "start-form-unchecked"
	form := ssStartForm(ssRead(t, id, ssSkill))
	if form == "" {
		ssViolate(t, id, "%s names no `ha start <goal> … --skill …` command", ssSkill)
		return
	}
	for _, flag := range []string{"--check-skill", "--rules run-rules/4", "--skill <path to this SKILL.md>"} {
		if !strings.Contains(form, flag) {
			ssViolate(t, id, "the start form %q lacks %s", form, flag)
		}
	}
}

// AC-13: a refused start or a missing command stops without records.
func TestSealSlimAC13_RefusalStops(t *testing.T) {
	const id = "start-refusal-unhandled"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"If `ha` is missing, refuses the start or does not know a command (exit 64), record nothing and stop",
		"report the executable and message",
		"never installing tools or using an older rule")
}

// AC-13: no field-by-field capability comparison or absolute path recheck.
func TestSealSlimAC13_NoManualCapabilityCheck(t *testing.T) {
	const id = "manual-capability-check"
	ssForbid(t, id, ssSkill, ssRead(t, id, ssSkill),
		"capabilities --format json", "absolute path", "Check the fields against", "if its path or contents change")
}

// AC-13: the optional helper never calls a new start compatible while the
// declaration that ha start --check-skill enforces would refuse it.
func TestSealSlimAC13_HelperCoversDeclaration(t *testing.T) {
	const id = "helper-start-disagrees"
	ssRead(t, id, ssDecl)
	ssRead(t, id, ssHelper)
	script := `
import json, sys
sys.path.insert(0, sys.argv[1])
import check_core
decl = json.load(open(sys.argv[2], encoding="utf-8"))
def full():
    formats = {k: sorted(v) for k, v in {"criteria": {"legacy", "nested/1"}, **{k: {v} for k, v in check_core.FORMATS.items()}}.items()}
    for k, v in decl.get("formats", {}).items():
        formats[k] = sorted(set(formats.get(k, [])) | set(v))
    for k, v in getattr(check_core, "START_FORMATS", {}).items():
        formats[k] = sorted(set(formats.get(k, [])) | set(v))
    return {"schema": "ha-capabilities/v1", "ha_version": "synthetic", "default_rules": "run-rules/3",
            "supported_rules": sorted({"run-rules/3"} | set(decl.get("rules", [])) | {check_core.START_RULES}),
            "formats": formats,
            "features": sorted(set(decl.get("features", [])) | check_core.START_FEATURES)}
def verdict(caps):
    try:
        check_core.requirements(check_core.validate(caps), "start", None)
        return "compatible"
    except check_core.Refusal as error:
        return error.reason
problems = []
if verdict(full()) != "compatible":
    problems.append("all declared support: " + verdict(full()))
items = [("rules", None, v) for v in decl.get("rules", [])] + [("features", None, v) for v in decl.get("features", [])]
items += [("formats", k, v) for k, vs in decl.get("formats", {}).items() for v in vs]
for kind, key, value in items:
    caps = full()
    if kind == "formats":
        caps["formats"][key] = [x for x in caps["formats"][key] if x != value]
    elif kind == "rules":
        caps["supported_rules"] = [x for x in caps["supported_rules"] if x != value]
    else:
        caps["features"] = [x for x in caps["features"] if x != value]
    if verdict(caps) == "compatible":
        problems.append("compatible without %s %s" % (kind + ("." + key if key else ""), value))
print(json.dumps(problems))
`
	out, code := ssRun(t, ".", os.Environ(), "python3", "-I", "-c", script, ssAbs(t, filepath.Join(ssSkillDir, "scripts")), ssAbs(t, ssDecl))
	if code != 0 {
		t.Skipf("unobserved: the helper probe exited %d: %s", code, out)
	}
	var problems []string
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &problems); err != nil {
		t.Skipf("unobserved: the helper probe printed %q", out)
	}
	for _, p := range problems {
		ssViolate(t, id, "check_core.py --operation start is %s", p)
	}
}

// AC-13: the released ha refuses the skill's start form and records nothing.
// The released executable is an external input: JAEKIT_RELEASED_HA names it.
func TestSealSlimAC13_ReleasedCoreRefusesStart(t *testing.T) {
	const id = "released-core-starts"
	released := os.Getenv("JAEKIT_RELEASED_HA")
	if released == "" {
		t.Skip("unobserved: JAEKIT_RELEASED_HA does not name the released ha")
	}
	if out, code := ssRun(t, ".", os.Environ(), released, "--version"); code != 0 || strings.TrimSpace(out) != "ha 0.1.6" {
		t.Skipf("unobserved: %s is not ha 0.1.6: exit %d %q", released, code, out)
	}
	form := ssStartForm(ssRead(t, id, ssSkill))
	if form == "" {
		ssViolate(t, id, "%s names no start form to run", ssSkill)
		return
	}
	r := ssNewRepo(t, ssPlanMaintain)
	argv, err := ssStartArgv(form, r.goal, ssAbs(t, ssSkill))
	if err != nil {
		ssViolate(t, id, "the start form %q cannot be filled: %v", form, err)
		return
	}
	out, code := ssRun(t, r.dir, r.env, released, argv...)
	if code == 0 || r.records() {
		ssViolate(t, id, "ha 0.1.6 ran %v: exit %d, records written %v: %s", argv, code, r.records(), out)
	}
}

// AC-14: the skill is at most 2,200 whitespace-separated words, front matter
// included.
func TestSealSlimAC14_WordLimit(t *testing.T) {
	const id = "skill-too-long"
	if n := len(strings.Fields(ssRead(t, id, ssSkill))); n > 2200 {
		ssViolate(t, id, "%s has %d words", ssSkill, n)
	}
}

// AC-14: name and description stay; the version is newer than the base.
func TestSealSlimAC14_Identity(t *testing.T) {
	const id = "skill-identity-wrong"
	keys, version, ok := ssFront(ssRead(t, id, ssSkill))
	if !ok {
		ssViolate(t, id, "%s has no closed front matter", ssSkill)
		return
	}
	if keys["name"] != "seal" || keys["description"] != ssDescription {
		ssViolate(t, id, "name %q or description changed", keys["name"])
	}
	if newer, valid := ssNewer(version, ssBaseSeal); !valid || !newer {
		ssViolate(t, id, "version %q is not newer than %s", version, ssBaseSeal)
	}
}

// AC-14: the manifests and the marketplace name the skill's version.
func TestSealSlimAC14_ManifestVersions(t *testing.T) {
	const id = "seal-version-mismatch"
	_, version, _ := ssFront(ssRead(t, id, ssSkill))
	for path, v := range ssVersions(t, id, "seal") {
		if v != version {
			ssViolate(t, id, "%s says %q, the skill %q", path, v, version)
		}
	}
}

// AC-14: `## Check inputs` stays as a short section with the protected
// phrases, the runner adapter sentence and the link to the details.
func TestSealSlimAC14_CheckInputs(t *testing.T) {
	const id = "check-inputs-section-wrong"
	skill := ssRead(t, id, ssSkill)
	sec := ssNeedSection(t, id, ssSkill, skill, "## Check inputs")
	// Short: at most two thirds of the 604 words the section had at the base.
	if n := len(strings.Fields(sec)); n > 400 {
		ssViolate(t, id, "`## Check inputs` has %d words", n)
	}
	ssRequire(t, id, "`## Check inputs`", sec,
		"When Core lists `runner-adapters/v1`, a runner adapter linking a change condition to an existing test keeps most of them by construction")
	// Every phrase the protected authoring check reads in this section
	// (tools/contract-checks/authoring_check.py, the "skill" clauses).
	ssRequire(t, id, "`## Check inputs`", sec, ssAuthoringAnchors...)
	// The phrases that check and its kept rules read elsewhere in the skill.
	ssRequire(t, id, ssSkill, skill,
		"ha note <goal> confirm <AC-n | scope:<path> | tests:<path> | spec>",
		"When the goal, scope, or criteria look wrong or need to change, ask the user",
		"`needs_user` means asking the user about those items",
		"Python 3 is not a prerequisite for Seal", "does not require an official adapter",
		"Existing `/1` and `/2` goals retain their result semantics", "## Assurance policy")
}

// ssAuthoringAnchors are the phrases the protected authoring check reads in
// the skill's `## Check inputs`.
var ssAuthoringAnchors = []string{
	"every check in the condition table", "`change`, `maintain`, `EX-n` and task commands",
	"producer, configuration or helper file", "the tree being checked", "external inputs its condition names",
	"start or base commit", "fixed commit", "start record or Core output", "other commits, refs or objects",
	"full clone and a history-less clone", "synthetic repository", "references/bundle.md#check-inputs",
	"`maintain`", "property the goal documents require", "content, output, format or behavior",
	"unchanged since the base commit", "does not observe that property", "outside `바꿀 수 있는 경로`",
	"`out_of_scope`", "expected content written in a tracked check file",
	"only by Core's baseline run", "never checks out, reads or compares the base commit",
	"violation on the base commit and a pass after the change",
	"about repository history or refs", "tag's type", "commit message format",
	"only the range and refs the condition names", "error or unobserved", "neither a violation nor a pass",
	"pre-execution review",
	"declares its own scope or trust boundary", "pinned to a version or release", "preserving past verdicts",
	"isolating imports", "limiting its inputs", "current behavior", "keeps the declaration", "explicit assumption",
	"the report repeats", "without asking the user", "same import boundary", "document-conflict rule",
	"add no user confirmation, approval or question",
	"the executor applies them and decides whether an existing check needs rewriting",
	"checks written or changed from Seal 0.1.14 on", "whichever Seal version started the goal",
	"never require rewriting or converting an existing goal's checks, records, `PROGRESS.md` or `REVIEW.md`",
}

// AC-14: the guides state the reduction in words, not tokens.
func TestSealSlimAC14_ReductionInWords(t *testing.T) {
	const id = "reduction-not-in-words"
	for path, sentence := range map[string]string{
		"../guides/USAGE.md":    "Seal skill 본문은 약 4,600단어에서 2,200단어 이하로 줄었습니다.",
		"../guides/USAGE.en.md": "The Seal skill body went from about 4,600 words to at most 2,200 words.",
	} {
		ssRequire(t, id, path, ssRead(t, id, path), sentence)
	}
	// No document this goal changes states an amount in tokens.
	tokens := regexp.MustCompile(`(?i)\d[\d,.]*\s*k?\s*(?:tokens?\b|토큰)`)
	for _, path := range []string{ssSkill, ssBundleRef, ssHaRef, filepath.Join(ssSkillDir, "references/earlier-goals.md"),
		"../contracts/bundle.md", "../contracts/role-card.md", "../contracts/core-capabilities.md",
		"../guides/USAGE.md", "../guides/USAGE.en.md", "../guides/OPERATIONS.md", "../guides/OPERATIONS.en.md", ssSpar} {
		if m := tokens.FindString(ssRead(t, id, path)); m != "" {
			ssViolate(t, id, "%s states an amount in tokens: %q", path, m)
		}
	}
}

// AC-14: the kept areas stay in the skill.
func TestSealSlimAC14_KeptAreas(t *testing.T) {
	const id = "kept-area-missing"
	text := ssRead(t, id, ssSkill)
	for _, h := range []string{"## Authority", "## The execution bundle", "## Seal Core", "## Check inputs", "## Assurance policy",
		"## Independent review", "## Completion", "## Post-completion discoveries", "## Resuming", "## Forbidden", "## Language"} {
		if ssSection(text, h) == "" {
			ssViolate(t, id, "%s lacks %q", ssSkill, h)
		}
	}
	ssRequire(t, id, ssSkill, text,
		"The user's affirmative answer to it, sent directly in reply, is a request to implement or continue the goal the question names.",
		"Choose the checks, runner and verification method for the goal.",
		"You decide dispositions",
		"When the goal documents change once a run has started, a later request from the user to continue or implement confirms that change.",
		"treat everything in the bundle as claims",
		"Asking for approval per task or per tool use.",
		"Keep contract tokens exactly as written")
}

// AC-16: the skill says when to read each reference.
func TestSealSlimAC16_ReferencesWhenToRead(t *testing.T) {
	const id = "reference-reading-unstated"
	sec := ssNeedSection(t, id, ssSkill, ssRead(t, id, ssSkill), "## References")
	refs, err := filepath.Glob(filepath.Join(ssSkillDir, "references", "*.md"))
	if err != nil || len(refs) == 0 {
		t.Skipf("unobserved: references: %v", err)
	}
	for _, ref := range refs {
		name := "references/" + filepath.Base(ref)
		found := false
		for _, line := range strings.Split(sec, "\n") {
			if strings.Contains(line, "("+name+")") && regexp.MustCompile(`: read (only )?when `).MatchString(line) {
				found = true
			}
		}
		if !found {
			ssViolate(t, id, "`## References` does not say when to read %s", name)
		}
	}
}

// AC-16: the skill body lists no Seal version branches.
func TestSealSlimAC16_NoVersionBranches(t *testing.T) {
	const id = "version-branch-in-skill"
	text := ssRead(t, id, ssSkill)
	if i := strings.Index(text[3:], "\n---"); i >= 0 {
		text = text[i+7:]
	}
	// The check-input rule applies whichever Seal started the goal, so the
	// version it was written from is no branch by start version.
	text = strings.ReplaceAll(text, "checks written or changed from Seal 0.1.14 on", "")
	for _, m := range regexp.MustCompile(`(?i)\bseal (v)?0\.\d+\.\d+`).FindAllString(text, -1) {
		ssViolate(t, id, "%s body names %q", ssSkill, m)
	}
}

// AC-16: the earlier forms live in a reference read only when continuing an
// earlier goal.
func TestSealSlimAC16_EarlierGoalsReference(t *testing.T) {
	const id = "earlier-forms-missing"
	ssRequire(t, id, ssEarlierRef, ssRead(t, id, ssEarlierRef),
		"Read this only when you continue a goal whose start record names a Seal earlier than 0.1.20",
		"Goals whose start record names seal 0.1.6 or later", "stamped with its check time",
		"Goals started with seal 0.1.13 or later", "Goals started with seal 0.1.10 or later",
		"Goals started with Seal 0.1.11 or later", "from Seal 0.1.14 on", "Reports using Seal 0.1.12 or later",
		"For goals started with Seal 0.1.15 or later", "The `ha done` output (completion) or `ha status --format md` output (stop), pasted unchanged.")
}

// AC-17: when each Spar stage is called, the cumulative round included.
func TestSealSlimAC17_Stages(t *testing.T) {
	const id = "review-stages-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"a `candidate` round once the final candidate's narrow checks pass", "a `fixes` round after review-fix commits",
		"adds a `plan` round before any baseline",
		"The cumulative round is a `candidate` round from the goal's base commit to the last head")
}

// AC-17: the plan round reviews product invariants and decision rules.
func TestSealSlimAC17_PlanRoundScope(t *testing.T) {
	const id = "plan-round-scope-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"`REVIEW.md` states the product invariants and decision rules the change introduces or alters",
		"A `plan` round, when the tier or selected policy asks for one, also reviews them, as questions.")
}

// AC-17: supported input forms come before the plan round and never narrow
// a requirement without the user.
func TestSealSlimAC17_SupportedForms(t *testing.T) {
	const id = "supported-forms-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"and the supported forms of open-ended input a new contract or check covers",
		"never narrower than a SPEC criterion, contract or protected regression path requires without the user's decision",
		"ask the user when an open criterion has none")
}

// AC-17: ha done only after a cumulative round with nothing open.
func TestSealSlimAC17_DoneOrder(t *testing.T) {
	const id = "done-order-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"run `ha done` only once it ends with no open `defect` or `requirement-question`",
		"A `fixes` round never replaces it", "a candidate round with no later fixes is one")
}

// AC-17: post-completion rework takes one cumulative round.
func TestSealSlimAC17_ReworkOrder(t *testing.T) {
	const id = "rework-order-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"take up the goal again only when the user asks for rework, such as applying a post-completion discovery or remote review comments: `reopen` quoting the request, fix, run one cumulative round, re-record stale checks and `ha done` before the head is sent or merged.")
}

// AC-17: how each class of finding is disposed of.
func TestSealSlimAC17_Dispositions(t *testing.T) {
	const id = "dispositions-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"You decide dispositions: fix or rebut a `defect`; ask the user about a `requirement-question`; never fix a deferred `hardening` item without the user's words.",
		"A finding outside the supported forms is `hardening`; the contract's statement of the forms rebuts a `defect` label.")
}

// AC-17: a round cap goes to the user.
func TestSealSlimAC17_RoundCap(t *testing.T) {
	const id = "round-cap-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"At a round cap, ask the user as the Spar skill says; never pass it or skip the cumulative round without the user's words.")
}

// AC-17: without Spar.
func TestSealSlimAC17_WithoutSpar(t *testing.T) {
	const id = "spar-absence-unhandled"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"Without Spar, another separate review recording role, inputs, versions, findings and limits counts when the report names it and it covered the whole last head before `ha done`",
		"When a required review is unavailable, keep completion pending", "report a skipped optional review")
}

// AC-17: records point to Spar's records instead of copying findings.
func TestSealSlimAC17_RecordsByReference(t *testing.T) {
	const id = "review-records-copied"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"`REVIEW.md` and the report summarize Spar's round and gate records by reference, never copying findings.")
}

// AC-17: the method stays with Spar and is not restated.
func TestSealSlimAC17_MethodInSpar(t *testing.T) {
	const id = "spar-method-restated"
	text := ssRead(t, id, ssSkill)
	ssRequire(t, id, ssSkill, text, "Spar owns how rounds run; this skill sets when they run and what follows.")
	ssForbid(t, id, ssSkill, text, "packet", "spar run", "spar triage", "spar extend", "plan-delta", "spar.toml", "three code rounds")
}

// AC-18: the budget question is asked once.
func TestSealSlimAC18_BudgetQuestion(t *testing.T) {
	const id = "budget-question-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"Without the user's budget, write `- 검증 실행: 자동`",
		"if the computed limit needs approval, ask once with both limits and write the approved number, never asking again for that PLAN")
}

// AC-18: tools are listed and checked before baselines and after resuming.
func TestSealSlimAC18_Environment(t *testing.T) {
	const id = "environment-check-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"in `## 실행 환경` and run `ha doctor <goal>` before any baseline and, when resuming, before the next check",
		"ask about unusable tools in one question, never widening permissions or settings, and keep doing independent work")
}

// AC-18: the pre-approved limit setting is forbidden to write.
func TestSealSlimAC18_PreapprovedSetting(t *testing.T) {
	const id = "preapproved-setting-allowed"
	sec := ssNeedSection(t, id, ssSkill, ssRead(t, id, ssSkill), "## Forbidden")
	ssRequire(t, id, "`## Forbidden`", sec, "writing or changing the user's pre-approved run limit setting")
}

// AC-19: the skill writes PROGRESS with ha report.
func TestSealSlimAC19_SkillUsesReport(t *testing.T) {
	const id = "progress-report-unused"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"`## 현재` carries the stamp `ha report` prints", "pasted unchanged; never type a time",
		"run `ha report` with `--lang ko` for a user writing Korean, `--lang en` otherwise",
		"The completion report is `ha report` output, pasted unchanged once `ha done` records completion or work stops",
		"plus only judgment ([format](references/bundle.md#completion-report)): expected-result grounds, observed boundaries, methods and substitutes, and that it does not establish deployment, installation or real model calls",
		"never `ha status` or `ha done` output")
}

// AC-19: the template and the bundle contract define the format.
func TestSealSlimAC19_FormatDefined(t *testing.T) {
	const id = "progress-format-undefined"
	tmpl := ssRead(t, id, filepath.Join(ssSkillDir, "assets/templates/PROGRESS.md"))
	ssRequire(t, id, "PROGRESS template `## 현재`", ssNeedSection(t, id, "PROGRESS template", tmpl, "## 현재"),
		"opens with the stamp `ha report <goal>` prints for `## 현재`, pasted unchanged", "never type a check time yourself")
	ssRequire(t, id, "PROGRESS template `## 완료 보고`", ssNeedSection(t, id, "PROGRESS template", tmpl, "## 완료 보고"),
		"the output of `ha report <goal> --lang ko`", "never paste `ha status` or `ha done` output")
	contract := ssRead(t, id, "../contracts/bundle.md")
	ssRequire(t, id, "contracts/bundle.md §5", ssNeedSection(t, id, "contracts/bundle.md", contract, "## 5. `PROGRESS.md`"),
		"`ha report <goal>`이 `## 현재` 도장으로 보이는 값을 그대로 붙인다", "확인 시각을 손으로 적지 않는다")
	ssRequire(t, id, "contracts/bundle.md §6", ssNeedSection(t, id, "contracts/bundle.md", contract, "## 6. 완료 보고"),
		"seal 0.1.20 이상으로 시작한 목표의 완료 보고는", "`ha status`·`ha done` 출력 원문은 붙이지 않는다", "그 아래에 실행자의 판단 문장만 더한다",
		"바뀐 파일과 각 파일이 기여한 조건(주장)", "가정한 미결정과 `manual` 조건", "독립 검토 결과의 위치",
		"보고가 배포·설치·실제 모델 실행의 증거가 아니라는 사실")
	ssRequire(t, id, ssBundleRef+" completion report", ssNeedSection(t, id, ssBundleRef, ssRead(t, id, ssBundleRef), "## Completion report"),
		"changed files and the conditions each served", "assumed decisions and `manual` conditions",
		"expected-result grounds, observed boundaries, methods and substitutes, and unobserved scope", "where the review records are",
		"that the report does not establish deployment, installation or a real model call")
}

// AC-21: the bundle contract describes the review plan and the new formats.
func TestSealSlimAC21_BundleContract(t *testing.T) {
	const id = "bundle-contract-stale"
	ssRequire(t, id, "contracts/bundle.md", ssRead(t, id, "../contracts/bundle.md"),
		"| 독립 검토 | SPEC의 `Assurance:` 등급", "계획한 검토 단계(plan, candidate, fixes, 누적 라운드)", "plan 검토가 볼 제품의 불변식과 결정 규칙",
		"열린 입력의 지원 입력 형식", "### 보증 등급과 검토 순서",
		"누적 라운드는 Spar candidate 단계를 목표의 base commit부터 마지막 head까지로 다시 부른 라운드다",
		"더 많은 검토를 요구하는 쪽을 따르고 완료 보고에 각각이 요구한 검토와 따른 쪽을 적는다")
}

// AC-21: the role card states the report rule, the review order and the tier.
func TestSealSlimAC21_RoleCard(t *testing.T) {
	const id = "role-card-stale"
	ssRequire(t, id, "contracts/role-card.md", ssRead(t, id, "../contracts/role-card.md"),
		"그때의 `ha report` 출력을 그대로 붙이고 그 아래에 판단 문장만 더하며", "**보증 등급과 검토 순서**",
		"누적 라운드가 열린 `defect`·`requirement-question` 없이 끝난 뒤에만 `ha done`을 실행한다",
		"프로젝트 지침(`AGENTS.md` 등)의 보증 요구는 하한이다", "더 많은 검토를 요구하는 쪽을 따르고")
}

// AC-21: the references and the templates match.
func TestSealSlimAC21_ReferencesAndTemplates(t *testing.T) {
	const id = "references-templates-stale"
	ssRequire(t, id, ssBundleRef, ssRead(t, id, ssBundleRef),
		"`검증 실행: 자동` lets `ha start` set the limit", "### `## 실행 환경`", "| 독립 검토 |",
		"For goals whose start record names seal 0.1.20 or later", "This form applies to goals whose start record names seal 0.1.20 or later")
	ssRequire(t, id, ssHaRef, ssRead(t, id, ssHaRef),
		"A new start passes `--check-skill`", "| `ha report <goal> [--lang ko\\|en]` |", "| `ha doctor <goal>` |")
	tmpl := filepath.Join(ssSkillDir, "assets/templates")
	ssRequire(t, id, "PLAN template", ssRead(t, id, filepath.Join(tmpl, "PLAN.md")), "- 검증 실행: 자동", "## 실행 환경")
	ssRequire(t, id, "REVIEW template", ssRead(t, id, filepath.Join(tmpl, "REVIEW.md")),
		"## 독립 검토", "the planned review stages", "product invariants and decision rules", "supported input forms")
}

// AC-21: the guides explain the new Seal flow.
func TestSealSlimAC21_Guides(t *testing.T) {
	const id = "guides-stale"
	ssRequire(t, id, "guides/USAGE.md", ssRead(t, id, "../guides/USAGE.md"),
		"## Seal의 시작 전 확인과 검토 순서", "**예산 질문은 한 번.**", "**시작 전 환경 점검.**", "**다섯 줄 보고.**",
		"Seal은 이 라운드가 열린 `defect`·`requirement-question` 없이 끝난 뒤에만 `ha done`을 실행합니다.")
	ssRequire(t, id, "guides/USAGE.en.md", ssRead(t, id, "../guides/USAGE.en.md"),
		"## Seal's pre-start checks and review order", "**One budget question.**", "**Environment check before the start.**",
		"**Five-line report.**", "Seal runs `ha done` only once that round ends with no open `defect` or `requirement-question`.")
	ssRequire(t, id, "guides/OPERATIONS.md", ssRead(t, id, "../guides/OPERATIONS.md"),
		"Seal 0.1.20부터는 Core 지원을 손으로 대조하지 않습니다.", "맨 위 다섯 줄을 먼저 읽고",
		"마지막 head 전체를 본 누적 라운드가 열린 지적 없이 끝난 뒤에만 `ha done`을 실행합니다")
	ssRequire(t, id, "guides/OPERATIONS.en.md", ssRead(t, id, "../guides/OPERATIONS.en.md"),
		"From Seal 0.1.20, Seal no longer compares Core support by hand.", "read the five lines at the top",
		"runs `ha done` only once a cumulative round over the whole last head ends with no open finding")
}

// ssCheckResult reads the result `ha check` prints for a target.
var ssCheckResult = regexp.MustCompile(`(?m)^AC-\d+ (?:baseline|check) (\S+) \(seq \d+\)$`)

// The real flow keeps an errored or unobserved check out of its violation.
func TestSealSlimCheckResultLine(t *testing.T) {
	for out, want := range map[string]string{
		"AC-1 baseline error (seq 3)\nclassification: process_signal\n": "error",
		"AC-1 check unknown (seq 5)\n":                                  "unknown",
		"AC-1 check violation (seq 5)\n":                                "violation",
	} {
		if m := ssCheckResult.FindStringSubmatch(out); m == nil || m[1] != want {
			t.Errorf("%q: read %v, want %s", out, m, want)
		}
	}
}

// AC-25: the real ha and this skill carry a synthetic goal with an automatic
// budget and `## 실행 환경` from ha doctor to the report of its completion.
func TestSealSlimAC25_RealFlow(t *testing.T) {
	const id = "real-flow-incomplete"
	form := ssStartForm(ssRead(t, id, ssSkill))
	if !strings.Contains(form, "--check-skill") {
		ssViolate(t, id, "%s names no start form that checks its declaration: %q", ssSkill, form)
		return
	}
	ha := filepath.Join(t.TempDir(), "ha")
	if out, code := ssRun(t, "..", os.Environ(), "go", "build", "-o", ha, "./cmd/ha"); code != 0 {
		t.Skipf("unobserved: building ha exited %d: %s", code, out)
	}
	r := ssNewRepo(t, fmt.Sprintf(ssPlan, "자동"))
	step := func(want int, args ...string) string {
		t.Helper()
		out, code := ssRun(t, r.dir, r.env, ha, args...)
		if code != want {
			// A check whose own run errored (a signal, a timeout, a tool that
			// did not start) or stayed unobserved observes nothing about the
			// flow.
			if m := ssCheckResult.FindStringSubmatch(out); m != nil && (m[1] == "error" || m[1] == "unknown") {
				t.Skipf("unobserved: ha %v recorded %s: %s", args, m[1], out)
			}
			t.Fatalf("jaekit:violation=%s ha %v exited %d: %s", id, args, code, out)
		}
		return out
	}
	if out := step(0, "doctor", r.goal); !strings.Contains(out, "git: 사용 가능") {
		ssViolate(t, id, "ha doctor did not report the listed tool: %s", out)
	}
	argv, err := ssStartArgv(form, r.goal, ssAbs(t, ssSkill))
	if err != nil {
		ssViolate(t, id, "the start form %q cannot be filled: %v", form, err)
		return
	}
	if out := step(0, argv...); !strings.Contains(out, "auto limit: 50") {
		ssViolate(t, id, "the start did not use the computed limit: %s", out)
	}
	step(0, "check", r.goal, "--baseline", "AC-1")
	if err := os.WriteFile(filepath.Join(r.dir, "greet/greeting.txt"), []byte("hello\n"), 0o644); err != nil {
		t.Skipf("unobserved: %v", err)
	}
	r.commit(t, "change")
	step(0, "check", r.goal, "AC-1")
	step(0, "done", r.goal)
	var status struct {
		Completion *int `json:"completion_record"`
	}
	if err := json.Unmarshal([]byte(step(0, "status", r.goal, "--format", "json")), &status); err != nil || status.Completion == nil {
		ssViolate(t, id, "ha status names no completion record: %v", err)
		return
	}
	report := step(0, "report", r.goal, "--lang", "en")
	lines := strings.Split(report, "\n")
	for i, label := range []string{"Changed: ", "Checked: ", "To review: ", "Remaining limits: ", "Next step: "} {
		if i >= len(lines) || !strings.HasPrefix(lines[i], label) {
			ssViolate(t, id, "report line %d does not start with %q", i+1, label)
		}
	}
	seq := *status.Completion
	if len(lines) < 5 || !strings.Contains(lines[4], fmt.Sprintf("completion record seq %d", seq)) {
		ssViolate(t, id, "the summary does not name completion record seq %d", seq)
	}
	if !strings.Contains(ssSection(report, "### Stamp for `## 현재`"), fmt.Sprintf("completion record: seq %d", seq)) {
		ssViolate(t, id, "the stamp does not name completion record seq %d", seq)
	}
}

// AC-26: the tier sets the review stages; no line keeps the selected policy.
func TestSealSlimAC26_Tiers(t *testing.T) {
	const id = "tier-rules-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"SPEC's `Assurance:` line sets the tier",
		"`light` may skip independent review, with the reason in `REVIEW.md` and the report.",
		"`standard` takes a `candidate` round", "`strict` adds a `plan` round before any baseline",
		"Without the line, the selected policy applies.")
}

// AC-26: project instructions are a floor; the stricter one is followed.
func TestSealSlimAC26_StricterWins(t *testing.T) {
	const id = "stricter-rule-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"project instructions such as `AGENTS.md` are a floor it raises, never lowers",
		"follow the one asking for more review and report what each required.")
}

// AC-26: a tier changed during the work.
func TestSealSlimAC26_TierChange(t *testing.T) {
	const id = "tier-change-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"A tier changed mid-goal applies once the user asks to continue; a stage it adds runs on the current head before completion, and earlier records stay.")
}

// AC-26: REVIEW and the report record the tier and the stages.
func TestSealSlimAC26_TierRecords(t *testing.T) {
	const id = "tier-records-missing"
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill),
		"`REVIEW.md` names the tier, or none, and planned stages; the report, the tier and stages received or why skipped.")
}

// AC-36: the skill declares the Core support it needs.
func TestSealSlimAC36_Declaration(t *testing.T) {
	const id = "core-requirements-missing"
	raw := ssRead(t, id, ssDecl)
	var decl struct {
		Schema   string              `json:"schema"`
		Rules    []string            `json:"rules"`
		Formats  map[string][]string `json:"formats"`
		Features []string            `json:"features"`
	}
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&decl); err != nil || dec.More() {
		ssViolate(t, id, "%s is not one core-requirements object: %v", ssDecl, err)
		return
	}
	if decl.Schema != "core-requirements/v1" {
		ssViolate(t, id, "schema %q", decl.Schema)
	}
	has := func(list []string, want ...string) {
		for _, w := range want {
			found := false
			for _, v := range list {
				found = found || v == w
			}
			if !found {
				ssViolate(t, id, "%s does not declare %s", ssDecl, w)
			}
		}
	}
	has(decl.Rules, "run-rules/4")
	has(decl.Features, "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1", "error-recovery/v1", "command-time-limits/v1",
		"structured-change-results/v1", "report/v1", "doctor/v1", "auto-budget/v1")
	has(decl.Formats["check_declaration"], "check-declaration/v1")
	has(decl.Formats["check_result"], "check-result/v1")
	has(decl.Formats["check_evidence"], "check-evidence/v1")
	has(decl.Formats["assurance_tier"], "light", "standard", "strict")
	ssRequire(t, id, ssSkill, ssRead(t, id, ssSkill), "`core-requirements.json` beside this file")
}

// AC-37: the Spar stage table documents the cumulative round.
func TestSealSlimAC37_StageTable(t *testing.T) {
	const id = "cumulative-round-undocumented"
	text := ssRead(t, id, ssSpar)
	ssForbid(t, id, ssSpar, text, "| candidate | Once,")
	ssRequire(t, id, ssSpar, text,
		"once more as the cumulative round over the goal's base commit to the last head",
		"counts toward the code-round cap like the first candidate round",
		"extending the cap follows the rule below", "Only the user can allow more rounds.")
}

// AC-37: the Spar version is raised everywhere.
func TestSealSlimAC37_SparVersions(t *testing.T) {
	const id = "spar-version-stale"
	_, version, ok := ssFront(ssRead(t, id, ssSpar))
	if newer, valid := ssNewer(version, ssBaseSpar); !ok || !valid || !newer {
		ssViolate(t, id, "Spar skill version %q is not newer than %s", version, ssBaseSpar)
	}
	for path, v := range ssVersions(t, id, "spar") {
		if v != version {
			ssViolate(t, id, "%s says %q, the skill %q", path, v, version)
		}
	}
}

// AC-20 (maintain): a goal started with an earlier Seal keeps its forms. The
// earlier-version rules stay in the skill or in a reference the skill points
// to, and the skill keeps such a goal's saved rules, formats and budget.
func TestSealSlimAC20_EarlierGoalsKeepForms(t *testing.T) {
	const id = "earlier-forms-lost"
	skill := ssRead(t, id, ssSkill)
	docs := skill
	for _, ref := range []string{"bundle.md", "ha.md", "earlier-goals.md"} {
		if strings.Contains(skill, "(references/"+ref) {
			docs += "\n" + ssRead(t, id, filepath.Join(ssSkillDir, "references", ref))
		}
	}
	lower := strings.ToLower(ssFlat(docs))
	for _, rule := range []string{"seal 0.1.6 or later", "keeps the PROGRESS form it started with", "seal 0.1.10 or later",
		"seal 0.1.11 or later", "seal 0.1.12 or later", "seal 0.1.13 or later", "seal 0.1.14", "seal 0.1.15 or later"} {
		if !strings.Contains(lower, strings.ToLower(rule)) {
			ssViolate(t, id, "neither %s nor a reference it points to keeps %q", ssSkill, rule)
		}
	}
	ssRequire(t, id, ssSkill, skill, "A goal started with an earlier Seal keeps its saved rules, formats and budget")
}
