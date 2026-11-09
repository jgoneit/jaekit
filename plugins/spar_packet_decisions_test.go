package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/jgoneit/jaekit/internal/reviewgate"
	"github.com/jgoneit/jaekit/internal/reviewseries"
	"github.com/jgoneit/jaekit/internal/spar"
)

// Checks of the goal spar-packet-decisions that read documents or cross the
// boundary from Spar to ha: the input row (AC-5), what Seal passes to rounds
// (AC-7, AC-8, AC-14), the reference's command form run by the real spar
// (AC-9), the Spar documents (AC-10), ha reading rounds with decisions
// (AC-12, kept) and the Seal version (AC-15). Document checks look for fixed
// phrases. A change check names the property it observes on the first line
// of each failure (jaekit:violation=<id>). A document, repository or
// command that cannot be read, made or started observes nothing: a change
// check skips, which the adapter reads as inconclusive, and the kept check
// of AC-12 fails.

const (
	spdBundleRef = "seal/skills/seal/references/bundle.md"
	spdTemplate  = "seal/skills/seal/assets/templates/REVIEW.md"
	spdSealSkill = "seal/skills/seal/SKILL.md"
	spdSparSkill = "spar/skills/spar/SKILL.md"
	spdDecisions = "## Decisions the user made"
)

func spdRead(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		t.Skipf("setup: %v", err)
	}
	return string(data)
}

// spdSection is the text from the heading line that starts with start up to
// the next heading of the same or a higher level; "" when there is none.
func spdSection(text, start string) string {
	level := strings.Index(start, " ")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if !strings.HasPrefix(line, start) {
			continue
		}
		end := len(lines)
		for j := i + 1; j < len(lines); j++ {
			if n := strings.Index(lines[j], " "); n > 0 && n <= level && strings.Trim(lines[j][:n], "#") == "" {
				end = j
				break
			}
		}
		return strings.Join(lines[i:end], "\n")
	}
	return ""
}

// spdRow is the line of a Markdown table whose first cell is id.
func spdRow(text, id string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "| "+id+" |") {
			return line
		}
	}
	return ""
}

type spdPlace struct {
	label, text string
	want        []string
}

func spdCheck(t *testing.T, id string, places []spdPlace) {
	t.Helper()
	for _, p := range places {
		if strings.TrimSpace(p.text) == "" {
			t.Errorf("jaekit:violation=%s %s is missing", id, p.label)
			continue
		}
		for _, w := range p.want {
			if !strings.Contains(p.text, w) {
				t.Errorf("jaekit:violation=%s %s does not say %q", id, p.label, w)
			}
		}
	}
}

// Fixed phrases of the Seal guidance (AC-7, AC-8, AC-14).
var (
	spdPassEn = []string{
		"Pass every review round, from the plan round to the cumulative round, the user's decisions recorded before it that the goal documents do not hold: " +
			"each answer that closed an item of REVIEW.md `## 중요한 미결정`, each `confirm` and `input` note that quotes the user's words, " +
			"and each answer to a review's `requirement-question`.",
		"Give each decision as `--decision` with what it settles, taken from the REVIEW.md item or the confirmation's target, " +
			"followed by `--decision-quote` with the recorded quote unchanged.",
		"Round cap extensions, reviewer waivers and `reopen` are not decisions about the change; never pass them.",
	}
	spdPassKo = []string{
		"라운드마다 그 라운드 전에 기록된, 목표 문서에 없는 사용자의 결정을 모두 넘긴다: `REVIEW.md` `## 중요한 미결정`을 닫은 사용자의 답, " +
			"사용자의 말을 인용한 확인(`confirm`)과 입력(`input`), 검토의 `requirement-question`에 대한 사용자의 답.",
		"결정마다 정한 대상(`REVIEW.md`의 항목이나 확인의 대상)을 `--decision`으로, 기록된 인용을 그대로 `--decision-quote`로 준다.",
		"상한 연장, 리뷰어 waiver, `reopen`은 넘기지 않는다.",
		"plan부터 누적 라운드까지 모든 라운드에 넘긴다.",
	}
	spdPassContractEn = []string{
		"Every round receives all the user's decisions recorded before it that the goal documents do not hold: " +
			"the user's answers that closed `REVIEW.md` `## 중요한 미결정` items, confirmations (`confirm`) and inputs (`input`) that quote the user's words, " +
			"and the user's answers to a review's `requirement-question`.",
		"Each decision gives what it settles (the `REVIEW.md` item or the confirmation's target) as `--decision` " +
			"and the recorded quote unchanged as `--decision-quote`.",
		"Round cap extensions, reviewer waivers and `reopen` are not passed.",
		"Decisions go to every round from the plan round to the cumulative round.",
	}
	spdPlacesEn = []string{
		"the places that read or keep it: each place's repository path, and the name used there when it differs from the change's name",
		"places that keep the same invariant under another name, not only those found by name",
		"`없음` when the change alters no invariant",
		"one `--question` per place, asking whether that place still keeps its invariant at head",
		"the list adds places for reviewers to look at and never narrows what they read",
	}
	spdPlacesKo = []string{
		"그것을 읽거나 지키는 곳을 적는다. 곳마다 저장소 경로를 적고, 그곳에서 쓰는 이름이 변경의 이름과 다르면 그 이름도 적는다",
		"이름으로 찾은 곳뿐 아니라 같은 불변식을 다른 이름으로 지키는 곳도 넣는다",
		"바꾸는 불변식이 없으면 없다고 적는다",
		"plan부터 누적 라운드까지 라운드마다, 곳마다 그곳이 head에서도 그 불변식을 지키는지 묻는 `--question` 하나를 넘긴다",
		"목록은 리뷰어가 볼 곳을 더할 뿐 읽는 범위를 좁히지 않는다",
	}
	spdPlacesContractEn = []string{
		"the places that read or keep it: each place's repository path, and the name used there when it differs from the change's name",
		"places that keep the same invariant under another name, not only those found by name",
		"when the change alters no invariant, it says so",
		"every round from the plan round to the cumulative round gets one `--question` per place, asking whether that place still keeps the invariant at head",
		"the list adds places for reviewers to look at and never narrows what they read",
	}
	spdScopeEn         = "The list of places applies to goals whose start record names seal 0.1.25 or later, and existing goals' `REVIEW.md` is never rewritten to the new form."
	spdScopeKo         = "곳 목록은 seal 0.1.25 이상으로 시작한 목표에 쓰고, 이미 있는 목표의 `REVIEW.md`는 새 형식으로 고쳐 쓰지 않는다."
	spdAnySealEn       = "Pass decisions in every round this Seal runs, whichever Seal started the goal."
	spdAnySealKo       = "결정 넘기기는 목표를 어느 Seal로 시작했든 이 Seal이 돌리는 라운드에 쓴다."
	spdAnySealContract = "Passing decisions applies to every round this Seal runs, whichever Seal started the goal."
)

// spdSealSections are the parts of the Seal guidance and contracts the
// checks read.
func spdSealSections(t *testing.T) (questions, review, template, ko4, en4 string) {
	t.Helper()
	bundle := spdRead(t, spdBundleRef)
	return spdSection(bundle, "## Questions for the user"), spdSection(bundle, "## REVIEW.md"),
		spdSection(spdRead(t, spdTemplate), "## 독립 검토"),
		spdSection(spdRead(t, "../contracts/bundle.md"), "## 4. "), spdSection(spdRead(t, "../contracts/bundle.en.md"), "## 4. ")
}

// AC-5: the input table of the input contract (ko, en) has the row of the
// decision options with the accepted form, the result outside it and its
// consumers.
func TestSparPacketDecisionsAC5_InputRow(t *testing.T) {
	const id = "decision-form-misjudged"
	common := []string{"G35", "`--decision`", "`--decision-quote`", "UTF-8", "`packet.md`", "`round.json`", "`decisions`"}
	spdCheck(t, id, []spdPlace{
		{"the G35 row of contracts/spar-input.md", spdRow(spdRead(t, "../contracts/spar-input.md"), "G35"),
			append([]string{"앞뒤 공백", "거부(65)", "라운드 폴더를 만들지 않"}, common...)},
		{"the G35 row of contracts/spar-input.en.md", spdRow(spdRead(t, "../contracts/spar-input.en.md"), "G35"),
			append([]string{"leading and trailing whitespace", "refused (65)", "no round folder"}, common...)},
	})
}

// AC-7: the Seal guidance and the bundle contract's section 4 say which
// decisions every round receives and in what form; the role card says that
// rounds receive them with the goal documents.
func TestSparPacketDecisionsAC7_DecisionsToPass(t *testing.T) {
	const id = "decision-passing-undefined"
	questions, _, _, ko4, en4 := spdSealSections(t)
	roleKo, roleEn := spdRead(t, "../contracts/role-card.md"), spdRead(t, "../contracts/role-card.en.md")
	spdCheck(t, id, []spdPlace{
		{"the Seal skill", spdRead(t, spdSealSkill), []string{"(references/bundle.md#questions-for-the-user)"}},
		{"references/bundle.md Questions for the user", questions, spdPassEn},
		{"contracts/bundle.md section 4", ko4, spdPassKo},
		{"contracts/bundle.en.md section 4", en4, spdPassContractEn},
		{"contracts/role-card.md", roleKo, []string{"라운드에 목표 문서를 모두 기준으로 주고 그 라운드 전에 기록된 사용자의 결정을 함께 주며"}},
		{"contracts/role-card.en.md", roleEn, []string{"gives the rounds all goal documents as criteria together with the user's decisions recorded before each round"}},
	})
}

// AC-8: the REVIEW template, the Seal guidance and the bundle contract's
// section 4 have `## 독립 검토` list, for each changed invariant, the places
// that read or keep it, and pass one question per place to every round.
func TestSparPacketDecisionsAC8_InvariantPlaces(t *testing.T) {
	const id = "invariant-places-undefined"
	_, review, template, ko4, en4 := spdSealSections(t)
	spdCheck(t, id, []spdPlace{
		{"references/bundle.md REVIEW.md", review, append([]string{"from the plan round to the cumulative round"}, spdPlacesEn...)},
		{"the REVIEW template's 독립 검토", template, append([]string{"from the plan round to the cumulative round"}, spdPlacesEn...)},
		{"contracts/bundle.md section 4", ko4, spdPlacesKo},
		{"contracts/bundle.en.md section 4", en4, spdPlacesContractEn},
	})
}

// AC-14: the list of places applies from this Seal on and earlier goals'
// REVIEW.md is never rewritten; decisions go to every round this Seal runs.
func TestSparPacketDecisionsAC14_ApplicationScope(t *testing.T) {
	const id = "review-format-scope-missing"
	questions, review, template, ko4, en4 := spdSealSections(t)
	spdCheck(t, id, []spdPlace{
		{"references/bundle.md Questions for the user", questions, []string{spdAnySealEn}},
		{"references/bundle.md REVIEW.md", review, []string{spdScopeEn}},
		{"the REVIEW template's 독립 검토", template, []string{"seal 0.1.25 or later"}},
		{"contracts/bundle.md section 4", ko4, []string{spdScopeKo, spdAnySealKo}},
		{"contracts/bundle.en.md section 4", en4, []string{spdScopeEn, spdAnySealContract}},
	})
}

// AC-10: the round contract (ko, en, sections 1, 2 and 5), the Spar guide
// (ko, en) and the Spar skill describe the decision options, the packet's
// decision section with its guidance and the record field; the skill says to
// pass decisions with or without Seal.
func TestSparPacketDecisionsAC10_SparDocuments(t *testing.T) {
	const id = "decision-input-undocumented"
	guidance := []string{
		"Each decision below was recorded by the caller as the user's words; spar does not check that it is.",
		"Do not report the change for doing what a decision settles.",
		"Report as usual a change that goes beyond what a decision settles.",
		"When following a decision breaks a criterion, rule or contract that the decision does not address, " +
			"report a `requirement-question` that links that criterion, rule or contract.",
	}
	places := []spdPlace{}
	for _, path := range []string{"../contracts/review-round.md", "../contracts/review-round.en.md"} {
		text := spdRead(t, path)
		places = append(places,
			spdPlace{path + " section 1", spdSection(text, "## 1. "), []string{"`--decision`", "`--decision-quote`", "G35"}},
			spdPlace{path + " section 2", spdSection(text, "## 2. "), append([]string{"`" + spdDecisions + "`"}, guidance...)},
			spdPlace{path + " section 5", spdSection(text, "## 5. "), []string{"`decisions`", "`settles`", "`user_quote`"}})
	}
	places = append(places,
		spdPlace{"guides/SPAR.md 검토 실행", spdSection(spdRead(t, "../guides/SPAR.md"), "## 검토 실행"),
			[]string{"`--decision`", "`--decision-quote`", "`" + spdDecisions + "`", "`round.json`의 `decisions`",
				"spar는 그 말이 사용자의 말인지 확인하지 않습니다", "결정이 정한 대로 한 것은 지적하지 말고",
				"결정을 넘는 변경은 평소대로 지적하며", "그 결정이 다루지 않은 기준·규칙·계약이 깨질 때는 그것을 연결한 `requirement-question`으로 보고"}},
		spdPlace{"guides/SPAR.en.md Run a review", spdSection(spdRead(t, "../guides/SPAR.en.md"), "## Run a review"),
			[]string{"`--decision`", "`--decision-quote`", "`" + spdDecisions + "`", "`decisions` in `round.json`",
				"spar does not check that the words are the user's", "not to report what a decision settles",
				"to report as usual a change that goes beyond it",
				"a criterion, rule or contract the decision does not address, to report a `requirement-question` that links it"}})
	bullet := ""
	for _, line := range strings.Split(spdRead(t, spdSparSkill), "\n") {
		if strings.HasPrefix(line, "- ") && strings.Contains(line, "`--decision`") {
			bullet = line
		}
	}
	places = append(places, spdPlace{"the Spar skill's --decision bullet", bullet,
		[]string{"`--decision-quote`", "with or without Seal", "`" + spdDecisions + "`", "`decisions` in `round.json`",
			"spar does not check that the words are the user's", "not to report what a decision settles",
			"to report a change that goes beyond it",
			"a criterion, rule or contract the decision does not address, to report a `requirement-question` that links it"}})
	spdCheck(t, id, places)
}

// AC-15: the Seal version goes up to 0.1.25 in its manifests, the
// marketplace and the skill, and the version list says the change needs no
// Core change.
func TestSparPacketDecisionsAC15_SealVersion(t *testing.T) {
	const id = "seal-version-not-raised"
	const want = "0.1.25"
	for _, path := range []string{"seal/.claude-plugin/plugin.json", "seal/.codex-plugin/plugin.json"} {
		var m struct {
			Version string `json:"version"`
		}
		if err := json.Unmarshal([]byte(spdRead(t, path)), &m); err != nil {
			t.Skipf("setup: %s: %v", path, err)
		}
		if m.Version != want {
			t.Errorf("jaekit:violation=%s %s has version %q, not %s", id, path, m.Version, want)
		}
	}
	var market struct {
		Plugins []struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"plugins"`
	}
	if err := json.Unmarshal([]byte(spdRead(t, "../.claude-plugin/marketplace.json")), &market); err != nil {
		t.Skipf("setup: marketplace.json: %v", err)
	}
	seal := ""
	for _, p := range market.Plugins {
		if p.Name == "seal" {
			seal = p.Version
		}
	}
	if seal != want {
		t.Errorf("jaekit:violation=%s the marketplace lists seal %q, not %s", id, seal, want)
	}
	spdCheck(t, id, []spdPlace{
		{"the Seal skill", spdRead(t, spdSealSkill), []string{"version: \"" + want + "\"\n"}},
		{"references/ha.md", spdRead(t, "seal/skills/seal/references/ha.md"),
			[]string{"Seal 0.1.25 passes the user's decisions and the places that keep each changed invariant to review rounds without a Core change."}},
	})
}

// spdWorld is a repository of two commits with criteria and rules, and a
// user configuration whose command reviewer answers for the head with no
// finding.
type spdWorld struct {
	t           *testing.T
	keeps       bool
	dir, repo   string
	base, head  string
	config      string
	setupFailed func(format string, args ...any)
}

func newSPDWorld(t *testing.T, keeps bool) *spdWorld {
	t.Helper()
	w := &spdWorld{t: t, keeps: keeps, dir: t.TempDir()}
	w.setupFailed = func(format string, args ...any) {
		t.Helper()
		if keeps {
			t.Fatalf("setup: "+format, args...)
		}
		t.Skipf("setup: "+format, args...)
	}
	if real, err := filepath.EvalSymlinks(w.dir); err == nil {
		w.dir = real
	}
	if _, err := exec.LookPath("git"); err != nil {
		w.setupFailed("git is not available: %v", err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	w.repo = filepath.Join(w.dir, "repo")
	w.write(filepath.Join(w.repo, ".gitignore"), "/docs/reviews/\n")
	w.write(filepath.Join(w.repo, "AGENTS.md"), "# Rules\n\nKeep reviews honest.\n")
	w.write(filepath.Join(w.repo, "docs", "specs", "example", "SPEC.md"), "# Goal\n\n- **AC-1** one\n")
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('one')\n")
	w.git("init", "-q")
	w.git("add", "-A")
	w.git("commit", "-qm", "base")
	w.base = w.git("rev-parse", "HEAD")
	w.write(filepath.Join(w.repo, "app", "main.py"), "print('two')\n")
	w.git("commit", "-qam", "head")
	w.head = w.git("rev-parse", "HEAD")
	checked := []any{}
	for _, a := range spar.Angles {
		checked = append(checked, map[string]any{"angle": a, "result": "synthetic check"})
	}
	answer, err := json.Marshal(map[string]any{"reviewed_head": w.head, "verdict": "no_blocking_findings",
		"angles_checked": checked, "angles_unchecked": []any{}, "findings": []any{}, "limits": "synthetic reviewer"})
	if err != nil {
		w.setupFailed("%v", err)
	}
	answerPath := w.write(filepath.Join(w.dir, "answer.json"), string(answer))
	cat, err := exec.LookPath("cat")
	if err != nil {
		w.setupFailed("cat is not available: %v", err)
	}
	w.config = w.write(filepath.Join(w.dir, "spar.toml"),
		fmt.Sprintf("[[reviewers]]\nname = \"cmd\"\npreset = \"command\"\nargv = [%q, %q]\n", cat, answerPath))
	return w
}

func (w *spdWorld) write(path, body string) string {
	w.t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		w.setupFailed("%v", err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		w.setupFailed("%v", err)
	}
	return path
}

func (w *spdWorld) git(args ...string) string {
	w.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "commit.gpgsign=false", "-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = w.repo
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@example.invalid",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@example.invalid")
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		w.setupFailed("git %v: %v %s", args, err, stderr.String())
	}
	return strings.TrimSpace(string(out))
}

// spar runs the real spar command line with the world's configuration.
func (w *spdWorld) spar(args ...string) (int, string) {
	var stdout, stderr strings.Builder
	getenv := func(k string) string {
		switch k {
		case "SPAR_CONFIG":
			return w.config
		case "HOME":
			return w.dir
		}
		return ""
	}
	code := spar.Main("test", args, strings.NewReader(""), &stdout, &stderr, getenv)
	return code, stderr.String()
}

// spdWords splits a shell command line into words: a backslash at the end
// of a line joins it to the next, and single and double quotes group a word.
// A quote that does not close is an error.
func spdWords(line string) ([]string, error) {
	line = strings.ReplaceAll(line, "\\\n", " ")
	var words []string
	var cur strings.Builder
	in, quote, started := false, byte(0), false
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case in && c == quote:
			in = false
		case in:
			cur.WriteByte(c)
		case c == '"' || c == '\'':
			in, quote, started = true, c, true
		case c == ' ' || c == '\t' || c == '\n':
			if started {
				words = append(words, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteByte(c)
			started = true
		}
	}
	if in {
		return nil, fmt.Errorf("the quote %c does not close", quote)
	}
	if started {
		words = append(words, cur.String())
	}
	return words, nil
}

// spdForm is the first bash block of the section that runs spar with
// --decision.
func spdForm(section string) string {
	parts := strings.Split(section, "```bash\n")
	for _, p := range parts[1:] {
		block, _, _ := strings.Cut(p, "\n```")
		if strings.HasPrefix(block, "spar run ") && strings.Contains(block, "--decision ") {
			return block
		}
	}
	return ""
}

// AC-9: the command form the Seal guidance shows, with two decisions and one
// place question filled in, runs the real spar without a refusal, and the
// round's packet holds both decisions in its decision section and the
// question.
func TestSparPacketDecisionsAC9_ReferenceForm(t *testing.T) {
	const id = "reference-form-fails"
	form := spdForm(spdSection(spdRead(t, spdBundleRef), "## Questions for the user"))
	if form == "" {
		t.Errorf("jaekit:violation=%s references/bundle.md Questions for the user shows no `spar run` form with --decision", id)
		return
	}
	w := newSPDWorld(t, false)
	values := []string{
		"<goal base commit>", w.base, "<head>", w.head, "<goal>", "example", "<NN>", "01",
		"<what decision 1 settles>", "skill 문장의 범위",
		"<the user's words for decision 1, as recorded>", "참고 문서에 맡김 (Recommended)",
		"<what decision 2 settles>", "코드 라운드 상한",
		"<the user's words for decision 2, as recorded>", "치명적이면 1회 연장 (Recommended)",
		"<path>", "internal/spar/packet.go", "<name used there>", "buildPacket", "<invariant>", "the decision section",
	}
	filled := strings.NewReplacer(values...).Replace(form)
	if i := strings.Index(filled, "<"); i >= 0 {
		end := strings.Index(filled[i:], ">")
		if end < 0 {
			end = len(filled) - i - 1
		}
		t.Errorf("jaekit:violation=%s the form has a placeholder this check does not fill: %s", id, filled[i:i+end+1])
		return
	}
	if _, err := spdWords(`spar run --decision "x`); err == nil {
		w.setupFailed("the word splitter takes a quote that does not close")
	}
	words, err := spdWords(filled)
	if err != nil {
		t.Errorf("jaekit:violation=%s the form is not a command line: %v", id, err)
		return
	}
	if len(words) < 2 || words[0] != "spar" || words[1] != "run" {
		t.Errorf("jaekit:violation=%s the form does not start with `spar run`: %q", id, words)
		return
	}
	// Every document the form names is in the world, so that a refusal
	// observes the form and not a missing file.
	for i := 0; i+1 < len(words); i++ {
		if (words[i] == "--criteria" || words[i] == "--rules" || words[i] == "--subject") && !filepath.IsAbs(words[i+1]) {
			path := filepath.Join(w.repo, filepath.FromSlash(words[i+1]))
			if _, err := os.Stat(path); err != nil {
				w.write(path, "# Document\n\n- **AC-1** one\n")
			}
		}
	}
	t.Chdir(w.repo)
	code, stderr := w.spar(append(words[1:], "--repo", w.repo)...)
	switch {
	case code == spar.ExitOK:
	case code == spar.ExitUsage || (code == spar.ExitRefused && strings.Contains(stderr, "decision")):
		// A usage error comes only from the form's arguments; a refusal
		// that names a decision judges the form's decisions.
		t.Errorf("jaekit:violation=%s spar refuses the reference's form (exit %d): %s", id, code, strings.TrimSpace(stderr))
		return
	default:
		w.setupFailed("spar run exits %d: %s", code, stderr)
	}
	out := ""
	for i, word := range words {
		if word == "--out" && i+1 < len(words) {
			out = words[i+1]
		}
	}
	data, err := os.ReadFile(filepath.Join(w.repo, filepath.FromSlash(out), "packet.md"))
	if err != nil {
		w.setupFailed("%v", err)
	}
	packet := string(data)
	at := strings.Index(packet, "\n"+spdDecisions+"\n")
	if at < 0 {
		t.Errorf("jaekit:violation=%s the packet has no %q section", id, spdDecisions)
		return
	}
	rest := packet[at:]
	for _, v := range []string{"skill 문장의 범위", "참고 문서에 맡김 (Recommended)", "코드 라운드 상한", "치명적이면 1회 연장 (Recommended)"} {
		n := strings.Index(rest, v)
		if n < 0 {
			t.Errorf("jaekit:violation=%s the decision section does not hold %q after the parts before it", id, v)
			return
		}
		rest = rest[n+len(v):]
	}
	question := "- internal/spar/packet.go (buildPacket): does it still keep the decision section at head?"
	if !strings.Contains(spdSection(packet, "## Questions for this round"), question) {
		t.Errorf("jaekit:violation=%s the packet's questions do not hold %q", id, question)
	}
}

// AC-12 (kept): ha is unchanged and does not import Spar; the readers ha
// uses for rounds and gates (`ha cost`, `ha note review --gate`, the review
// check of `ha done`) read a series whose round holds decisions with the
// same result as the same series without them.
func TestSparPacketDecisionsAC12_HaReadsRounds(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", "github.com/jgoneit/jaekit/cmd/ha").Output()
	if err != nil {
		t.Fatalf("setup: go list: %v", err)
	}
	for _, pkg := range strings.Fields(string(out)) {
		if pkg == "github.com/jgoneit/jaekit/internal/spar" {
			t.Errorf("cmd/ha imports %s", pkg)
		}
	}
	w := newSPDWorld(t, true)
	type read struct {
		rounds []reviewseries.Round
		gate   any
	}
	readSeries := func(name string, extra ...string) read {
		t.Helper()
		series := filepath.Join(w.repo, "docs", "reviews", name)
		round := filepath.Join(series, "01-candidate")
		args := append([]string{"run", "--stage", "candidate", "--base", w.base, "--head", w.head, "--out", round,
			"--criteria", filepath.Join(w.repo, "docs", "specs", "example", "SPEC.md"), "--repo", w.repo}, extra...)
		if code, stderr := w.spar(args...); code != spar.ExitOK {
			t.Fatalf("setup: spar run %s exits %d: %s", name, code, stderr)
		}
		// spar gate writes its file in every state; a command reviewer is
		// never required, so this gate is degraded (exit 3).
		gatePath := filepath.Join(series, "gate.json")
		code, stderr := w.spar("gate", round, "--expect-head", w.head, "--out", gatePath, "--repo", w.repo)
		if code != spar.ExitOK && code != spar.ExitDegraded && code != spar.ExitBlocked {
			t.Fatalf("setup: spar gate %s exits %d: %s", name, code, stderr)
		}
		s, err := reviewseries.Read(series)
		if err != nil {
			t.Fatalf("ha's series reader refuses %s: %v", name, err)
		}
		g, err := reviewgate.Read(gatePath)
		if err != nil {
			t.Fatalf("ha's gate reader refuses %s: %v", name, err)
		}
		for i := range s.Rounds {
			s.Rounds[i].StartedAt, s.Rounds[i].FinishedAt = "", ""
			for j := range s.Rounds[i].Reviewers {
				s.Rounds[i].Reviewers[j].ElapsedSeconds = nil
			}
		}
		g.GateDigest = ""
		return read{s.Rounds, g}
	}
	without := readSeries("without")
	with := readSeries("with", "--decision", "skill 문장의 범위", "--decision-quote", "참고 문서에 맡김 (Recommended)")
	if !reflect.DeepEqual(with.rounds, without.rounds) {
		t.Errorf("ha's series reader reads a round with decisions differently:\nwith    %+v\nwithout %+v", with.rounds, without.rounds)
	}
	if !reflect.DeepEqual(with.gate, without.gate) {
		t.Errorf("ha's gate reader reads the gate of a round with decisions differently:\nwith    %+v\nwithout %+v", with.gate, without.gate)
	}
}
