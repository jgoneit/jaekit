package plugins

// Checks of the readme-first-run goal: the READMEs name the one setup step a
// project needs before its first standard goal, start Seal by answering yes
// first, say when Seal asks the user, point to ha cost, and describe the Spar
// records folder as one the project's Git is set up to ignore; the Spar guide
// and skill name the same setup step.
//
// Each test observes fixed tokens, lines, links and digests in the tree under
// test, never an open category of sentences. The images are observed through
// their outlined SVG sources and renders.json, as readme_flow_visuals_test.go
// does, with the helpers that file defines.
//
// Every failure message about a property opens with the test's violation
// marker, which ha's go-test-json adapter reads. A file the tests read that
// cannot be read, a missing heading or block they locate the property in, and
// a corrupt manifest or image fail with a "setup:" message and no marker. The
// image helpers report a missing image or render record, and a PNG that was
// not rendered from its current SVG, with the test's own marker: the criteria
// require the image to be rebuilt from its source. Whether a link's anchor
// exists is left to the public documentation check.

import (
	"strings"
	"testing"
)

// rfrLang holds the fixed headings and tokens of one language.
type rfrLang struct {
	rfvLang
	started, firstTask, setup, setupLink, step3, oldStep3, sparGuide, sparSetup, usage, usageCost, costLink string
}

var rfrLangs = []rfrLang{
	{rfvLang: rfvLangs[0], started: "## 시작하기", firstTask: "### 첫 작업 맡기기", setup: "### 첫 `standard` 목표 전에",
		setupLink: "[첫 `standard` 목표 전에](#첫-standard-목표-전에)", step3: "#### 3. \"응\"으로 Seal 시작하기", oldStep3: "#### 3. 별도 메시지로 Seal 시작하기",
		sparGuide: "guides/SPAR.md", sparSetup: "## 프로젝트 준비", usage: "guides/USAGE.md", usageCost: "## 목표 하나의 비용 보기",
		costLink: "(guides/USAGE.md#목표-하나의-비용-보기)"},
	{rfvLang: rfvLangs[1], started: "## Get started", firstTask: "### Your first task", setup: "### Before your first `standard` goal",
		setupLink: "[Before your first `standard` goal](#before-your-first-standard-goal)", step3: "#### 3. Answer yes to start Seal", oldStep3: "#### 3. Start Seal in a separate message",
		sparGuide: "guides/SPAR.en.md", sparSetup: "## Set up a project", usage: "guides/USAGE.en.md", usageCost: "## Reading one goal's cost",
		costLink: "(guides/USAGE.en.md#reading-one-goals-cost)"},
}

// rfrSetup are the setup section's tokens, in order.
var rfrSetup = map[string][]string{
	"ko": {"프로젝트의 `.gitignore`에 `docs/reviews/`를 더합니다", "Spar는 Git이 무시하는 폴더에만 검토 기록을 쓰는데",
		"`light` 목표와 Spar를 쓰지 않는 목표에는 필요 없습니다", "```text\ndocs/reviews/\n```", "[프로젝트 준비](guides/SPAR.md#프로젝트-준비)"},
	"en": {"Add `docs/reviews/` to your project's `.gitignore`", "`standard`", "Spar writes review records only to a folder Git ignores",
		"`light` goals and goals that do not use Spar do not need this step", "```text\ndocs/reviews/\n```", "[Set up a project](guides/SPAR.en.md#set-up-a-project)"},
}

// AC-1: both READMEs show the setup step, in Get started before the first
// task, with its reason, who does not need it and a link to the Spar guide.
func TestReadmeFirstRunAC1_SetupStep(t *testing.T) {
	const v = "setup-step-missing"
	for _, l := range rfrLangs {
		text := rfvRead(t, l.readme[3:])
		started, ok := rfvSection(text, l.started)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.started)
		}
		section, ok := rfvSection(started, l.setup)
		if !ok {
			t.Errorf("jaekit:violation=%s %s %s has no section %s", v, l.readme, l.started, l.setup)
			continue
		}
		if strings.Index(started, l.setup) > strings.Index(started, l.firstTask) {
			t.Errorf("jaekit:violation=%s %s shows %s after %s", v, l.readme, l.setup, l.firstTask)
		}
		if miss := rfvInOrder(section, rfrSetup[l.code]...); miss != "" {
			t.Errorf("jaekit:violation=%s %s %s lacks %q in order", v, l.readme, l.setup, miss)
		}
	}
}

// rfrGuide are the Spar guide's setup section tokens, in order.
var rfrGuide = map[string][]string{
	"ko": {"Git이 무시하는 폴더에만 검토 기록을 씁니다", "65로 멈추고 아무것도 쓰지 않습니다", "`standard`·`strict` 목표",
		"프로젝트의 `.gitignore`에 한 줄을 더합니다", "```text\ndocs/reviews/\n```", "`light` 목표와 Spar를 쓰지 않는 목표에는 필요 없습니다",
		"**다른 폴더.**", "`.gitignore`에 더하고", "`.git/info/exclude`"},
	"en": {"only to a folder Git ignores", "exit 65", "writes nothing", "`standard` or `strict` goal",
		"add one line to the project's `.gitignore`", "```text\ndocs/reviews/\n```", "`light` goals and goals that do not use Spar do not need this step",
		"**Another folder.**", "Add that folder to `.gitignore`", "`.git/info/exclude`"},
}

// rfrSkill are the Spar skill's tokens of the same setup step, in order, in
// its section before the first round.
var rfrSkill = []string{"never run Spar", "exit 65", "add `docs/reviews/` to the project's `.gitignore`",
	"ask the user", "\"Set up a project\"", "`.git/info/exclude`"}

// AC-2: the Spar guide (both languages) and the Spar skill name the same one
// setup step; the guide also covers another folder and .git/info/exclude.
func TestReadmeFirstRunAC2_SparSetup(t *testing.T) {
	const v = "spar-setup-unstated"
	for _, l := range rfrLangs {
		section, ok := rfvSection(rfvRead(t, l.sparGuide), l.sparSetup)
		if !ok {
			t.Errorf("jaekit:violation=%s %s has no section %s", v, l.sparGuide, l.sparSetup)
			continue
		}
		if miss := rfvInOrder(section, rfrGuide[l.code]...); miss != "" {
			t.Errorf("jaekit:violation=%s %s %s lacks %q in order", v, l.sparGuide, l.sparSetup, miss)
		}
	}
	const skill = "plugins/spar/skills/spar/SKILL.md"
	section, ok := rfvSection(rfvRead(t, skill), "## Before the first round")
	if !ok {
		t.Fatalf("setup: %s lacks ## Before the first round", skill)
	}
	if miss := rfvInOrder(section, rfrSkill...); miss != "" {
		t.Errorf("jaekit:violation=%s %s Before the first round lacks %q in order", v, skill, miss)
	}
}

// rfrStep3 are the tokens of the first task's step 3, in order: the yes to
// Spec's implement-now question first, then the separate-message commands of
// both hosts.
var rfrStep3 = map[string][]string{
	"ko": {"지금 구현할지 묻습니다", "**\"응\"이라고 답합니다**", "Seal이 시작됩니다", "별도 메시지로 목표 폴더를 지정합니다",
		"**Claude Code**", "```text\n/seal:seal docs/specs/<goal>\n```", "**Codex**", "```text\n$seal:seal docs/specs/<goal>\n```"},
	"en": {"asking whether to implement the goal now", "answer **yes**", "Seal starts on that goal", "specify the goal folder in a separate message",
		"**Claude Code**", "```text\n/seal:seal docs/specs/<goal>\n```", "**Codex**", "```text\n$seal:seal docs/specs/<goal>\n```"},
}

// AC-3: step 3 of the first task shows answering yes first and keeps both
// hosts' separate-message commands in the same step.
func TestReadmeFirstRunAC3_YesFirst(t *testing.T) {
	const v = "first-task-yes-missing"
	for _, l := range rfrLangs {
		first, ok := rfvSection(rfvRead(t, l.readme[3:]), l.firstTask)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.firstTask)
		}
		if strings.Contains(first, l.oldStep3+"\n") {
			t.Errorf("jaekit:violation=%s %s step 3 is still %q", v, l.readme, l.oldStep3)
		}
		step, ok := rfvSection(first, l.step3)
		if !ok {
			t.Errorf("jaekit:violation=%s %s %s has no step %s", v, l.readme, l.firstTask, l.step3)
			continue
		}
		if miss := rfvInOrder(step, rfrStep3[l.code]...); miss != "" {
			t.Errorf("jaekit:violation=%s %s %s lacks %q in order", v, l.readme, l.step3, miss)
		}
	}
}

// rfrDecide is the diagram's question node. rfrFlowRest is the sha256 of the
// diagram with that node's line masked, at main 81a7d47: the rest of the
// flow stays as it was.
var (
	rfrDecide = map[string]string{
		"ko": `    decide{{"나만 정할 수 있는 결정을<br/>묻는 질문에 답합니다<br/>작업 전: 미리 알 수 있는 결정을 한 번에<br/>작업 중: 요구 질문 같은<br/>새 질문만<br/>끝: 필요한 확인을 한 번에"}}`,
		"en": `    decide{{"You answer questions about<br/>decisions only you can make<br/>before work: the foreseeable ones, at once<br/>during work: only new ones,<br/>such as a requirement question<br/>at the end: the confirmations, at once"}}`,
	}
	rfrFlowRest = map[string]string{
		"ko": "8fc21078252229c120400bc43716e6e1b4d249c771f0efe0a7917e8396e29b64",
		"en": "8f440ebcfd7289372b658dffaef15b7a69c74ff6dc7bd12f1bc1edb07daf4e78",
	}
)

// rfrAsks holds, per language, the tokens of when Seal asks the user: in
// caption 3, in the usage flow image's text runs and description, and in its
// README alt text; old is the requirement-only wording that must be gone.
var rfrAsks = map[string]struct{ caption, runs, desc, alt, old []string }{
	"ko": {
		caption: []string{"Seal은 나만 정할 수 있는 결정만 묻습니다", "작업 전에는 미리 알 수 있는 결정을 한 메시지로", "작업 중에는 요구를 바꿔야 하는 질문 같은 새 질문만",
			"끝에는 완료에 필요한 확인을 한 메시지로"},
		runs: []string{"Seal은 나만 정할 수 있는 결정만 묻습니다", "작업 전: 미리 알 수 있는 결정을 한 메시지로", "작업 중: 요구 질문 같은 새 질문만", "끝: 필요한 확인을 한 메시지로"},
		desc: []string{"Seal은 나만 정할 수 있는 결정만 묻습니다", "작업 전에는 미리 알 수 있는 결정을 한 메시지로", "작업 중에는 요구 질문 같은 새 질문만",
			"끝에는 필요한 확인을 한 메시지로"},
		alt: []string{"Seal은 나만 정할 수 있는 결정만 묻습니다", "작업 전에는 미리 알 수 있는 결정을 한 메시지로", "작업 중에는 요구 질문 같은 새 질문만",
			"끝에는 필요한 확인을 한 메시지로"},
		old: []string{"요구를 바꿔야 하는 질문은 내게 옵니다", "질문은 내게 옵니다"},
	},
	"en": {
		caption: []string{"Seal asks you about decisions only you can make", "before work, the ones it can foresee, in one message",
			"during work, only new questions, such as one that would change the requirements", "at the end, the confirmations completion needs, in one message"},
		runs: []string{"Seal asks you about decisions only you can make", "Before work: the foreseeable ones, in one message",
			"During work: only new ones, such as a requirement question", "At the end: the confirmations, in one message"},
		desc: []string{"Seal asks you about decisions only you can make", "before work, the foreseeable ones in one message",
			"during work, only new ones, such as a requirement question", "at the end, the confirmations in one message"},
		alt: []string{"Seal asks you about decisions only you can make", "the foreseeable ones in one message before work",
			"only new ones such as a requirement question during work", "the confirmations in one message at the end"},
		old: []string{"change the requirements come to you", "requirements come to you"},
	},
}

// rfrMaskLine replaces every line of text that starts with prefix by prefix
// and "<masked>".
func rfrMaskLine(text, prefix string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, prefix) {
			lines[i] = prefix + "<masked>"
		}
	}
	return strings.Join(lines, "\n")
}

// rfrCaption returns the numbered caption n under the diagram.
func rfrCaption(section string, n string) string {
	_, after, ok := strings.Cut(section, "```mermaid\n")
	if ok {
		_, after, _ = strings.Cut(after, "\n```\n")
	}
	for _, line := range strings.Split(after, "\n") {
		if strings.HasPrefix(line, n+". ") {
			return line
		}
	}
	return ""
}

// AC-4: the diagram's question node, caption 3, the usage flow image (PNG
// and SVG, both languages) and its alt text say when Seal asks the user, and
// the rest of the diagram is as it was.
func TestReadmeFirstRunAC4_WhenSealAsks(t *testing.T) {
	const v = "seal-questions-stale"
	for _, l := range rfrLangs {
		want := rfrAsks[l.code]
		readme := rfvRead(t, l.readme[3:])
		section, ok := rfvSection(readme, l.flow)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.flow)
		}
		block, ok := rfvBlock(section, "mermaid")
		if !ok {
			t.Fatalf("setup: %s %s has no mermaid diagram", l.readme, l.flow)
		}
		if !strings.Contains("\n"+block+"\n", "\n"+rfrDecide[l.code]+"\n") {
			t.Errorf("jaekit:violation=%s %s diagram lacks the question node %q", v, l.readme, rfrDecide[l.code])
		}
		if got := rfvSHA([]byte(rfrMaskLine(block, "    decide{{"))); got != rfrFlowRest[l.code] {
			t.Errorf("jaekit:violation=%s %s diagram changed outside the question node: sha256 %s, want %s", v, l.readme, got, rfrFlowRest[l.code])
		}
		caption := rfrCaption(section, "3")
		if miss := rfvInOrder(caption, want.caption...); miss != "" {
			t.Errorf("jaekit:violation=%s %s caption 3 lacks %q in order", v, l.readme, miss)
		}
		svg, runs := rfvStill(t, v, "usage-flow", l.code)
		if miss := rfvRunsInOrder(runs, want.runs...); miss != "" {
			t.Errorf("jaekit:violation=%s usage-flow.%s.svg lacks the text %q in order", v, l.code, miss)
		}
		if miss := rfvInOrder(rfvDesc(svg), want.desc...); miss != "" {
			t.Errorf("jaekit:violation=%s usage-flow.%s.svg description lacks %q in order", v, l.code, miss)
		}
		alt, ok := rfvAlt(readme, "assets/readme/usage-flow."+l.code+".png")
		if !ok {
			t.Fatalf("setup: %s does not show usage-flow.%s.png with alt text", l.readme, l.code)
		}
		if miss := rfvInOrder(alt, want.alt...); miss != "" {
			t.Errorf("jaekit:violation=%s %s usage flow alt text lacks %q in order", v, l.readme, miss)
		}
		for _, old := range want.old {
			for where, text := range map[string]string{"caption 3": caption, "usage flow alt text": alt, "usage flow image": strings.Join(runs, "\n"), "usage flow description": rfvDesc(svg)} {
				if strings.Contains(text, old) {
					t.Errorf("jaekit:violation=%s %s %s still says %q", v, l.readme, where, old)
				}
			}
		}
	}
}

// AC-5: both READMEs say ha cost shows one goal's cost and link the usage
// guide's section on it.
func TestReadmeFirstRunAC5_CostPointer(t *testing.T) {
	const v = "cost-pointer-missing"
	for _, l := range rfrLangs {
		readme := rfvRead(t, l.readme[3:])
		for _, tok := range []string{"`ha cost docs/specs/<goal> --series docs/reviews/<goal>`", l.costLink} {
			if !strings.Contains(readme, tok) {
				t.Errorf("jaekit:violation=%s %s lacks %q", v, l.readme, tok)
			}
		}
		if _, ok := rfvSection(rfvRead(t, l.usage), l.usageCost); !ok {
			t.Fatalf("setup: %s lacks %s", l.usage, l.usageCost)
		}
	}
}

// rfrFolder holds, per language, the tokens that describe the Spar records
// folder as one the project's Git is set up to ignore: the text tree line,
// the folder alt text, the folder image's text runs and description; old is
// the earlier wording.
var rfrFolder = map[string]struct {
	tree       string
	alt, runs  []string
	desc, old  []string
	treeHolder string
}{
	"ko": {
		tree:       "프로젝트의 Git이 무시하게 설정한 폴더의 Spar 기록",
		alt:        []string{"Spar의 검토 라운드와 gate는 프로젝트의 Git이 무시하게 설정한 docs/reviews/password-link/에", "첫 standard 목표 전에"},
		runs:       []string{"docs/reviews/password-link/", "gate.json", "프로젝트의 Git이 무시하게 설정한 폴더에 둡니다", "설정: .gitignore에 docs/reviews/ 추가"},
		desc:       []string{"프로젝트의 Git이 무시하게 설정한 docs/reviews/password-link/", ".gitignore에 docs/reviews/ 추가"},
		old:        []string{"Git이 무시하는 폴더", "Git이 무시하는 docs/reviews"},
		treeHolder: "docs/reviews/password-link/",
	},
	"en": {
		tree:       "Spar records, in a folder your project's Git is set up to ignore",
		alt:        []string{"docs/reviews/password-link/, a folder your project's Git is set up to ignore", "Before your first standard goal"},
		runs:       []string{"docs/reviews/password-link/", "gate.json", "in a folder your project's Git is set up to ignore", "set up: add docs/reviews/ to .gitignore"},
		desc:       []string{"a folder the project's Git is set up to ignore", "add docs/reviews/ to .gitignore"},
		old:        []string{"a folder Git ignores", "folder Git ignores"},
		treeHolder: "docs/reviews/password-link/",
	},
}

// AC-8: the folder image (both languages), the text tree and the alt text say
// the records folder is one the project's Git is set up to ignore, and the
// README points from the folder to the setup step.
func TestReadmeFirstRunAC8_FolderSetUp(t *testing.T) {
	const v = "folder-ignore-unstated"
	for _, l := range rfrLangs {
		want := rfrFolder[l.code]
		readme := rfvRead(t, l.readme[3:])
		records, ok := rfvSection(readme, l.records)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.records)
		}
		tree, ok := rfvBlock(records, "text")
		if !ok {
			t.Fatalf("setup: %s %s has no text tree", l.readme, l.records)
		}
		line := ""
		for _, row := range strings.Split(tree, "\n") {
			if strings.HasPrefix(row, want.treeHolder) {
				line = row
			}
		}
		if !strings.Contains(line, want.tree) {
			t.Errorf("jaekit:violation=%s %s text tree line %q lacks %q", v, l.readme, line, want.tree)
		}
		image := "](assets/readme/goal-folder." + l.code + ".png)"
		at := strings.Index(records, image)
		details := strings.Index(records, "<details>")
		if at < 0 || details < at {
			t.Fatalf("setup: %s %s does not show goal-folder.%s.png before its text tree", l.readme, l.records, l.code)
		}
		if !strings.Contains(records[at:details], l.setupLink) {
			t.Errorf("jaekit:violation=%s %s does not link %s between the folder image and its text tree", v, l.readme, l.setupLink)
		}
		if _, ok := rfvSection(readme, l.setup); !ok {
			t.Errorf("jaekit:violation=%s %s lacks the linked section %s", v, l.readme, l.setup)
		}
		alt, ok := rfvAlt(readme, "assets/readme/goal-folder."+l.code+".png")
		if !ok {
			t.Fatalf("setup: %s does not show goal-folder.%s.png with alt text", l.readme, l.code)
		}
		if miss := rfvInOrder(alt, want.alt...); miss != "" {
			t.Errorf("jaekit:violation=%s %s folder alt text lacks %q in order", v, l.readme, miss)
		}
		svg, runs := rfvStill(t, v, "goal-folder", l.code)
		if miss := rfvRunsInOrder(runs, want.runs...); miss != "" {
			t.Errorf("jaekit:violation=%s goal-folder.%s.svg lacks the text %q in order", v, l.code, miss)
		}
		if miss := rfvInOrder(rfvDesc(svg), want.desc...); miss != "" {
			t.Errorf("jaekit:violation=%s goal-folder.%s.svg description lacks %q in order", v, l.code, miss)
		}
		for _, old := range want.old {
			for where, text := range map[string]string{"text tree line": line, "folder alt text": alt, "folder image": strings.Join(runs, "\n"), "folder description": rfvDesc(svg)} {
				if strings.Contains(text, old) {
					t.Errorf("jaekit:violation=%s %s %s still says %q", v, l.readme, where, old)
				}
			}
		}
	}
}
