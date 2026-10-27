package plugins

// Checks of the readme-flow-visuals goal: the READMEs' flow diagram, its
// captions and the four README images (usage flow, walkthrough GIF, goal
// folder, completion report card) show the v0.1.7 flow, and every image has
// an editable SVG source and a generator in the repository.
//
// The images are observed through their outlined SVG sources: the words of
// every text run are in its data-text attribute. renders.json, written by the
// renderer, ties each published PNG or GIF to the SVG bytes it came from; a
// published file whose digest or sources no longer match is stale. Each test
// observes fixed tokens, lines, links and digests, never an open category of
// sentences.
//
// Every failure message about a property opens with the test's violation
// marker, which ha's go-test-json adapter reads. A required source or
// manifest that does not exist is that violation: the criteria require it to
// exist. Any other read error, a corrupt manifest or a corrupt image fails
// with a "setup:" message and no marker, which is neither the violation nor
// a pass.

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"html"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

const (
	rfvReadme    = "../README.md"
	rfvReadmeKo  = "../README.ko.md"
	rfvManifest  = "assets/readme/source/renders.json"
	rfvFramesDoc = "assets/readme/source/walkthrough/frames.json"
	rfvDemo      = "assets/readme/demo.en.gif"
)

type rfvLang struct {
	code, readme, flow, records, report string
}

var rfvLangs = []rfvLang{
	{code: "ko", readme: rfvReadmeKo, flow: "## 사용 흐름", records: "## 남는 문서와 기록", report: "### 완료 보고"},
	{code: "en", readme: rfvReadme, flow: "## How it works", records: "## What you get", report: "### The completion report"},
}

// rfvRead returns a file of the tree under test (paths relative to the
// repository root). Any read error is a setup failure.
func rfvRead(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile("../" + path)
	if err != nil {
		t.Fatalf("setup: read %s: %v", path, err)
	}
	return string(b)
}

// rfvRequired reads a file whose existence the criteria require: a file that
// does not exist is the violation v; any other read error is setup.
func rfvRequired(t *testing.T, v, path string) []byte {
	t.Helper()
	b, err := os.ReadFile("../" + path)
	if errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("jaekit:violation=%s %s does not exist", v, path)
	}
	if err != nil {
		t.Fatalf("setup: read %s: %v", path, err)
	}
	return b
}

// rfvSection returns the text from heading to the next heading of the same
// or a higher level, ignoring headings inside fenced code; ok is false when
// the heading is missing.
func rfvSection(text, heading string) (string, bool) {
	lines := strings.Split(text, "\n")
	level := strings.Index(heading, " ")
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
		if n := strings.Index(line, " "); i > start && n > 0 && n <= level && strings.Trim(line[:n], "#") == "" {
			return strings.Join(lines[start:i], "\n"), true
		}
	}
	if start < 0 {
		return "", false
	}
	return strings.Join(lines[start:], "\n"), true
}

// rfvBlock returns the first fenced block opened by "```"+lang in text.
func rfvBlock(text, lang string) (string, bool) {
	open := "```" + lang + "\n"
	i := strings.Index(text, open)
	if i < 0 {
		return "", false
	}
	rest := text[i+len(open):]
	j := strings.Index(rest, "\n```")
	if j < 0 {
		return "", false
	}
	return rest[:j], true
}

// rfvAlt returns the alt text of the Markdown image that shows asset.
func rfvAlt(text, asset string) (string, bool) {
	m := regexp.MustCompile(`!\[([^\]]+)\]\(` + regexp.QuoteMeta(asset) + `\)`).FindStringSubmatch(text)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// rfvTexts returns the words of an outlined SVG's text runs, in drawing order.
func rfvTexts(svg string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`data-text="([^"]*)"`).FindAllStringSubmatch(svg, -1) {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

// rfvEditableTexts returns the text elements of an editable SVG, in order.
func rfvEditableTexts(svg string) []string {
	var out []string
	for _, m := range regexp.MustCompile(`(?s)<text\b[^>]*>(.*?)</text>`).FindAllStringSubmatch(svg, -1) {
		out = append(out, html.UnescapeString(m[1]))
	}
	return out
}

func rfvDesc(svg string) string {
	m := regexp.MustCompile(`(?s)<desc\b[^>]*>(.*?)</desc>`).FindStringSubmatch(svg)
	if m == nil {
		return ""
	}
	return html.UnescapeString(m[1])
}

// rfvInOrder reports the first token of tokens that does not occur, in
// order, in text; "" when all do.
func rfvInOrder(text string, tokens ...string) string {
	at := 0
	for _, tok := range tokens {
		i := strings.Index(text[at:], tok)
		if i < 0 {
			return tok
		}
		at += i + len(tok)
	}
	return ""
}

// rfvRunsInOrder is rfvInOrder over whole text runs: each token must be a
// substring of a later run than the previous token's.
func rfvRunsInOrder(runs []string, tokens ...string) string {
	at := 0
	for _, tok := range tokens {
		found := false
		for at < len(runs) {
			at++
			if strings.Contains(runs[at-1], tok) {
				found = true
				break
			}
		}
		if !found {
			return tok
		}
	}
	return ""
}

func rfvSHA(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type rfvRender struct {
	SHA256 string            `json:"sha256"`
	Width  int               `json:"width"`
	Height int               `json:"height"`
	Frames int               `json:"frames"`
	Inputs map[string]string `json:"inputs"`
}

// rfvRendered checks that output was rendered from exactly the given inputs
// as they are now, and returns its manifest entry.
func rfvRendered(t *testing.T, v, output string, inputs ...string) rfvRender {
	t.Helper()
	raw := rfvRequired(t, v, rfvManifest)
	var manifest struct {
		Outputs map[string]rfvRender `json:"outputs"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("setup: %s is not valid JSON: %v", rfvManifest, err)
	}
	entry, ok := manifest.Outputs[output]
	if !ok {
		t.Fatalf("jaekit:violation=%s %s has no render record for %s", v, rfvManifest, output)
	}
	if got := rfvSHA(rfvRequired(t, v, output)); got != entry.SHA256 {
		t.Errorf("jaekit:violation=%s %s is not the file the renderer wrote (sha256 %s, recorded %s)", v, output, got, entry.SHA256)
	}
	if len(entry.Inputs) != len(inputs) {
		t.Errorf("jaekit:violation=%s %s was rendered from %d inputs, want %d", v, output, len(entry.Inputs), len(inputs))
	}
	for _, input := range inputs {
		recorded, ok := entry.Inputs[input]
		if !ok {
			t.Errorf("jaekit:violation=%s %s was not rendered from %s", v, output, input)
			continue
		}
		if got := rfvSHA(rfvRequired(t, v, input)); got != recorded {
			t.Errorf("jaekit:violation=%s %s changed since %s was rendered from it", v, input, output)
		}
	}
	return entry
}

// rfvSVGSize returns the width and height attributes of an SVG's root.
func rfvSVGSize(svg string) (string, string) {
	m := regexp.MustCompile(`<svg\b[^>]*\bwidth="(\d+)"[^>]*\bheight="(\d+)"`).FindStringSubmatch(svg)
	if m == nil {
		return "", ""
	}
	return m[1], m[2]
}

// rfvPNGSize reads the IHDR size of a PNG.
func rfvPNGSize(t *testing.T, path string, b []byte) (int, int) {
	t.Helper()
	if len(b) < 24 || string(b[1:4]) != "PNG" || string(b[12:16]) != "IHDR" {
		t.Fatalf("setup: %s is not a PNG", path)
	}
	return int(binary.BigEndian.Uint32(b[16:20])), int(binary.BigEndian.Uint32(b[20:24]))
}

// rfvGIF reads a GIF's logical screen size and counts its image descriptors.
func rfvGIF(t *testing.T, path string, b []byte) (int, int, int) {
	t.Helper()
	bad := func() { t.Fatalf("setup: %s is not a well-formed GIF", path) }
	if len(b) < 13 || string(b[:6]) != "GIF89a" {
		bad()
	}
	w, h := int(binary.LittleEndian.Uint16(b[6:8])), int(binary.LittleEndian.Uint16(b[8:10]))
	i := 13
	if b[10]&0x80 != 0 {
		i += 3 << (b[10]&7 + 1)
	}
	subBlocks := func() {
		for i < len(b) && b[i] != 0 {
			i += int(b[i]) + 1
		}
		i++
	}
	frames := 0
	for i < len(b) {
		switch b[i] {
		case 0x21:
			i += 2
			subBlocks()
		case 0x2C:
			if i+10 > len(b) {
				bad()
			}
			packed := b[i+9]
			i += 10
			if packed&0x80 != 0 {
				i += 3 << (packed&7 + 1)
			}
			i++
			subBlocks()
			frames++
		case 0x3B:
			return w, h, frames
		default:
			bad()
		}
	}
	bad()
	return 0, 0, 0
}

// rfvStill checks one language of a still image: its outlined SVG exists,
// the PNG was rendered from it as it is now and has its size, and returns
// the SVG and its text runs.
func rfvStill(t *testing.T, v, name, lang string) (string, []string) {
	t.Helper()
	svgPath := "assets/readme/" + name + "." + lang + ".svg"
	pngPath := "assets/readme/" + name + "." + lang + ".png"
	svg := string(rfvRequired(t, v, svgPath))
	entry := rfvRendered(t, v, pngPath, svgPath)
	w, h := rfvPNGSize(t, pngPath, rfvRequired(t, v, pngPath))
	sw, sh := rfvSVGSize(svg)
	if sw == "" || sw != strconv.Itoa(w) || sh != strconv.Itoa(h) || entry.Width != w || entry.Height != h {
		t.Errorf("jaekit:violation=%s %s is %dx%d, its SVG %sx%s and its record %dx%d", v, pngPath, w, h, sw, sh, entry.Width, entry.Height)
	}
	return svg, rfvTexts(svg)
}

// rfvFrames returns the walkthrough's frame list and each frame's text runs.
func rfvFrames(t *testing.T, v string) ([]string, [][]string) {
	t.Helper()
	var plan struct {
		Output string `json:"output"`
		Frames []struct {
			SVG   string `json:"svg"`
			Delay int    `json:"delay_ms"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(rfvRequired(t, v, rfvFramesDoc), &plan); err != nil {
		t.Fatalf("setup: %s is not valid JSON: %v", rfvFramesDoc, err)
	}
	if plan.Output != rfvDemo || len(plan.Frames) == 0 {
		t.Fatalf("jaekit:violation=%s %s does not build %s from frames", v, rfvFramesDoc, rfvDemo)
	}
	var paths []string
	var runs [][]string
	for _, f := range plan.Frames {
		if f.Delay <= 0 {
			t.Errorf("jaekit:violation=%s %s has no delay", v, f.SVG)
		}
		paths = append(paths, f.SVG)
		runs = append(runs, rfvTexts(string(rfvRequired(t, v, f.SVG))))
	}
	return paths, runs
}

// rfvMermaidLines are fixed lines of the flow diagram, the same structure in
// both languages: Spec asks once, the user's yes starts Seal, a separate
// message also does, checks pass into Spar review, a strict plan review, a
// requirement question to the user, light skipping review, and ha done only
// after the gate covers the last commit.
var rfvMermaidLines = map[string][]string{
	"ko": {
		`    spec["<b>Spec</b>이 목표를 씁니다<br/>SPEC.md: 목표, 완료 조건,<br/>Assurance: 등급"]`,
		`    asknow{{"Spec이 한 번 묻습니다<br/>지금 구현할까요?"}}`,
		`    start["#quot;응#quot;으로<br/>Seal이 시작됩니다"]`,
		`    separate(["나중에 별도 메시지로 시작해도 됩니다<br/>/seal:seal docs/specs/#lt;goal#gt;"])`,
		`        planreview["strict만: 작업 전<br/>Spar 계획 검토"]`,
		`        check["ha로 검사<br/>모든 실행이<br/>runs.jsonl에 기록됨"]`,
		`        spar["standard·strict:<br/>Spar 검토 라운드와<br/>검토 gate 기록"]`,
		`        plan -.->|"strict"| planreview -.-> build`,
		`        check -->|"검사 통과"| spar`,
		`        spar -->|"고칠 지적"| build`,
		`    decide{{"나만 정할 수 있는 결정을<br/>묻는 질문에 답합니다<br/>작업 전: 미리 알 수 있는 결정을 한 번에<br/>작업 중: 요구 질문 같은<br/>새 질문만<br/>끝: 필요한 확인을 한 번에"}}`,
		`    review -->|"확인 완료"| asknow`,
		`    asknow -->|"응"| start --> plan`,
		`    separate -.-> plan`,
		`    spar -.->|"요구 질문"| decide`,
		`    spar -->|"gate가<br/>마지막 commit을 덮음"| done`,
		`    check -.->|"light: 이유를 남기고<br/>검토 생략 가능"| done`,
		`    done --> report`,
		`    resume -.->|"docs/specs/#lt;goal#gt;<br/>이어서 해줘"| plan`,
	},
	"en": {
		`    spec["<b>Spec</b> writes the goal<br/>SPEC.md: goal, completion conditions<br/>and an Assurance: tier"]`,
		`    asknow{{"Spec asks once:<br/>implement it now?"}}`,
		`    start["Your #quot;Yes#quot; starts Seal"]`,
		`    separate(["Or later, in a separate message<br/>/seal:seal docs/specs/#lt;goal#gt;"])`,
		`        planreview["strict only: Spar plan review<br/>before work starts"]`,
		`        check["Check through ha<br/>every run is recorded in runs.jsonl"]`,
		`        spar["standard and strict:<br/>Spar review rounds<br/>and a recorded review gate"]`,
		`        plan -.->|"strict"| planreview -.-> build`,
		`        check -->|"checks pass"| spar`,
		`        spar -->|"a finding to fix"| build`,
		`    decide{{"You answer questions about<br/>decisions only you can make<br/>before work: the foreseeable ones, at once<br/>during work: only new ones,<br/>such as a requirement question<br/>at the end: the confirmations, at once"}}`,
		`    review -->|"looks right"| asknow`,
		`    asknow -->|"Yes"| start --> plan`,
		`    separate -.-> plan`,
		`    spar -.->|"requirement question"| decide`,
		`    spar -->|"the gate covers<br/>the last commit"| done`,
		`    check -.->|"light: may skip review<br/>with a reason"| done`,
		`    done --> report`,
		`    resume -.->|"continue docs/specs/#lt;goal#gt;"| plan`,
	},
}

// AC-1: the diagram shows the v0.1.7 flow.
func TestReadmeFlowVisualsAC1_DiagramShowsTheFlow(t *testing.T) {
	const v = "flow-diagram-stale"
	for _, l := range rfvLangs {
		section, ok := rfvSection(rfvRead(t, l.readme[3:]), l.flow)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks %s", v, l.readme, l.flow)
			continue
		}
		block, ok := rfvBlock(section, "mermaid")
		if !ok {
			t.Errorf("jaekit:violation=%s %s %s has no mermaid diagram", v, l.readme, l.flow)
			continue
		}
		lines := map[string]bool{}
		for _, line := range strings.Split(block, "\n") {
			lines[line] = true
		}
		for _, want := range rfvMermaidLines[l.code] {
			if !lines[want] {
				t.Errorf("jaekit:violation=%s %s diagram lacks the line %q", v, l.readme, want)
			}
		}
	}
}

// rfvCaptions are fixed tokens of the four numbered captions under the
// diagram, item by item, in order.
var rfvCaptions = map[string][][]string{
	"ko": {
		{"`Assurance:` 등급", "지금 구현할지 한 번 물은 뒤 **멈춥니다**"},
		{"**\"응\"이라고 답하면 Seal이 시작**", "**별도 메시지로 목표 폴더를 지정해 Seal을 시작**해도 됩니다"},
		{"`ha`로 검사", "모든 실행을 기록", "완료 전에 Spar의 독립 검토 라운드", "작업 전에 계획 검토", "`light` 목표는 이유를 남기고 검토를 건너뛸 수 있습니다", "Seal은 나만 정할 수 있는 결정만 묻습니다"},
		{"필수 완료 조건이 충족되면 `ha done`이 완료를 기록", "기록된 검토 gate가 마지막 commit도 덮어야", "`PROGRESS.md`", "새 대화에서 같은 목표"},
	},
	"en": {
		{"`Assurance:` tier", "asks once whether to implement it now", "**stops**"},
		{"answer **yes** and Seal starts", "**start Seal in a separate message**"},
		{"through `ha`, which records every run", "independent Spar review rounds before completion", "plan review before work starts", "`light` goals may skip review with a reason", "Seal asks you about decisions only you can make"},
		{"`ha done` records completion when the required completion conditions are met", "a recorded review gate must also cover the last commit", "`PROGRESS.md`", "same goal in a new conversation"},
	},
}

// AC-1: the numbered captions under the diagram follow the same flow.
func TestReadmeFlowVisualsAC1_CaptionsShowTheFlow(t *testing.T) {
	const v = "flow-captions-stale"
	for _, l := range rfvLangs {
		section, ok := rfvSection(rfvRead(t, l.readme[3:]), l.flow)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks %s", v, l.readme, l.flow)
			continue
		}
		_, after, ok := strings.Cut(section, "```mermaid\n")
		if ok {
			_, after, ok = strings.Cut(after, "\n```\n")
		}
		if !ok {
			t.Errorf("jaekit:violation=%s %s %s has no captions after the diagram", v, l.readme, l.flow)
			continue
		}
		var items []string
		for _, line := range strings.Split(after, "\n") {
			if m := regexp.MustCompile(`^(\d+)\. `).FindStringSubmatch(line); m != nil {
				items = append(items, line)
			} else if strings.TrimSpace(line) != "" && len(items) > 0 {
				break
			}
		}
		if len(items) != len(rfvCaptions[l.code]) {
			t.Errorf("jaekit:violation=%s %s has %d numbered captions under the diagram, want %d", v, l.readme, len(items), len(rfvCaptions[l.code]))
			continue
		}
		for i, tokens := range rfvCaptions[l.code] {
			if miss := rfvInOrder(items[i], tokens...); miss != "" {
				t.Errorf("jaekit:violation=%s %s caption %d lacks %q in order", v, l.readme, i+1, miss)
			}
		}
	}
}

// rfvUsage holds the fixed text runs of the usage flow image, in drawing
// order, its description tokens, the old copy it must not show, and the
// README alt text tokens.
var rfvUsage = map[string]struct{ runs, desc, old, alt []string }{
	"ko": {
		runs: []string{"요청하고 목표 확인", "“응”으로 Seal 시작", "구현하고 ha로 검사", "검토 뒤 완료 기록",
			"Assurance: standard", "지금 구현할까요?", "응", "또는 별도 메시지", "/seal:seal", "ha", "Spar", "gate", "ha done", "PROGRESS.md",
			"독립 검토는 Assurance 등급에 따라 다릅니다", "light", "이유를 남기고", "standard", "완료 전 Spar 검토 라운드,", "gate가 마지막 commit을 덮음",
			"strict", "작업 전 계획 검토", "?", "Seal은 나만 정할 수 있는 결정만 묻습니다", "같은 프로젝트의 같은 목표", "사용 흐름 · 개념도"},
		desc: []string{"1.", "지금 구현할지 묻습니다", "2.", "“응”이라고 답하면 Seal이 시작됩니다", "별도 메시지로", "3.", "ha로 검사",
			"standard·strict 목표는 완료 전에 Spar의 독립 검토 라운드", "strict는 작업 전 계획 검토도", "light는 이유를 남기고 검토를 생략할 수 있습니다",
			"Seal은 나만 정할 수 있는 결정만 묻습니다", "4.", "기록된 검토 gate가 마지막 commit을 덮으면 ha done이 완료를 기록", "PROGRESS.md", "개념도"},
		old: []string{"Seal을 별도로 시작", "새 메시지에 목표 폴더를 지정합니다."},
		alt: []string{"사용 흐름 개념도", "지금 구현할지 묻습니다", "\"응\"이라고 답하면 Seal이 시작", "별도 메시지로 목표 폴더를 지정해 시작해도 됩니다",
			"ha로 검사", "Spar 검토 라운드와 검토 gate", "strict는 계획 검토도", "light 목표는 이유를 남기고 검토를 생략할 수 있으며",
			"Seal은 나만 정할 수 있는 결정만 묻습니다", "기록된 검토 gate가 마지막 commit을 덮으면 ha done이 완료를 기록", "PROGRESS.md", "같은 목표를 지정해 재개"},
	},
	"en": {
		runs: []string{"Request & review the goal", "Say yes to start Seal", "Build & check through ha", "Review, then record done",
			"Assurance: standard", "Implement it now?", "Yes", "or a separate message", "/seal:seal", "ha", "Spar", "gate", "ha done", "PROGRESS.md",
			"Independent review depends on the Assurance tier", "light", "may skip review,", "standard", "Spar review rounds; the gate", "must cover the last commit",
			"strict", "review before work", "?", "Seal asks you about decisions only you can make", "same goal in a new conversation", "Usage flow · Concept illustration"},
		desc: []string{"1.", "asks whether to implement it now", "2.", "Answer Yes and Seal starts", "separate message", "3.", "checks through ha",
			"standard and strict goals get independent Spar review rounds", "strict goals also get a plan review before work", "light goals may skip review with a reason",
			"Seal asks you about decisions only you can make", "4.", "a recorded review gate covers the last commit, ha done records completion", "PROGRESS.md", "concept illustration"},
		old: []string{"Start Seal separately", "New message: specify the goal folder."},
		alt: []string{"Concept illustration", "asks whether to implement it now", "Answer Yes and Seal starts", "separate message works too",
			"checks through ha", "Spar review rounds and a review gate", "strict also a plan review", "light goals may skip review with a reason",
			"Seal asks you about decisions only you can make", "ha done records completion", "review gate covers the last commit", "PROGRESS.md", "same goal in the same project"},
	},
}

// AC-2: the usage flow image (PNG and SVG) and its alt text show the flow.
func TestReadmeFlowVisualsAC2_UsageFlowImage(t *testing.T) {
	const v = "usage-flow-stale"
	for _, l := range rfvLangs {
		want := rfvUsage[l.code]
		svg, runs := rfvStill(t, v, "usage-flow", l.code)
		if miss := rfvRunsInOrder(runs, want.runs...); miss != "" {
			t.Errorf("jaekit:violation=%s usage-flow.%s.svg lacks the text %q in order", v, l.code, miss)
		}
		if miss := rfvInOrder(rfvDesc(svg), want.desc...); miss != "" {
			t.Errorf("jaekit:violation=%s usage-flow.%s.svg description lacks %q in order", v, l.code, miss)
		}
		for _, old := range want.old {
			if strings.Contains(strings.Join(runs, "\n"), old) {
				t.Errorf("jaekit:violation=%s usage-flow.%s.svg still shows %q", v, l.code, old)
			}
		}
		alt, ok := rfvAlt(rfvRead(t, l.readme[3:]), "assets/readme/usage-flow."+l.code+".png")
		if !ok {
			t.Errorf("jaekit:violation=%s %s does not show usage-flow.%s.png with alt text", v, l.readme, l.code)
		} else if miss := rfvInOrder(alt, want.alt...); miss != "" {
			t.Errorf("jaekit:violation=%s %s usage flow alt text lacks %q in order", v, l.readme, miss)
		}
	}
}

// rfvWalkthrough are the walkthrough's fixed text runs across all frames, in
// order: the request, the goal, the implement-now question and the yes or a
// separate /seal:seal message, the agent's commands (the baseline failure,
// the passes after the change, the Spar round, the gate, ha done) and the
// report. Output lines are v0.1.7 formats. The readme-demo-user-steps goal
// set this five-scene order.
var rfvWalkthrough = []string{
	"/spec:spec Add a “Forgot password” link",
	"wrote docs/specs/password-link/SPEC.md", "Assurance: standard", "AC-1 The login screen shows",
	"Implement docs/specs/password-link", "now? Assurance: standard.", "Yes", "/seal:seal docs/specs/password-link",
	"$ ha check docs/specs/password-link --baseline AC-1", "AC-1 baseline fail_as_expected (seq 2)",
	"$ ha check docs/specs/password-link AC-1 AC-2", "AC-1 check pass (seq 3)", "AC-2 check pass (seq 4)",
	"$ spar run --stage candidate --base f46020c", "$ spar gate docs/reviews/password-link/01-candidate",
	"$ ha note docs/specs/password-link review", "noted: review (seq 5)",
	"$ ha done docs/specs/password-link", "- status: **complete** (exit 0)", "- completion record: seq 6",
	"## 완료 보고", "Changed:", "Checked:", "To review:", "Remaining limits:", "assurance local; check author: executor", "Next step:",
	"### Evidence per condition", "| AC-1 | change | yes | yes | 2, 3 |", "| AC-2 | maintain | yes | yes | 4 |",
}

// AC-3: the walkthrough GIF, built from its SVG frames, follows one fictional
// goal to the end in order.
func TestReadmeFlowVisualsAC3_WalkthroughFrames(t *testing.T) {
	const v = "walkthrough-stale"
	paths, frames := rfvFrames(t, v)
	var all []string
	for _, runs := range frames {
		all = append(all, runs...)
	}
	if miss := rfvRunsInOrder(all, rfvWalkthrough...); miss != "" {
		t.Errorf("jaekit:violation=%s the walkthrough frames lack %q in order", v, miss)
	}
	entry := rfvRendered(t, v, rfvDemo, append([]string{rfvFramesDoc}, paths...)...)
	w, h, n := rfvGIF(t, rfvDemo, rfvRequired(t, v, rfvDemo))
	if n != len(paths) || entry.Frames != len(paths) || w != entry.Width || h != entry.Height {
		t.Errorf("jaekit:violation=%s %s has %d frames of %dx%d; %d frames listed, record %d of %dx%d", v, rfvDemo, n, w, h, len(paths), entry.Frames, entry.Width, entry.Height)
	}
}

var rfvWalkthroughAlt = map[string][]string{
	"ko": {"password-link 가상 사용 과정", "/spec:spec으로 Spec에 요청", "목표를 확인", "지금 구현할지 묻는 질문에 Yes(응)로 답하거나",
		"/seal:seal docs/specs/password-link", "AC-1은 바꾸기 전 실패·바꾼 뒤 통과", "AC-2는 계속 통과", "Spar 검토 라운드", "gate를 기록", "ha done이 완료를 기록", "완료 보고"},
	"en": {"Fictional walkthrough of password-link", "ask Spec with /spec:spec", "review the goal", "answering Yes when Spec asks whether to implement it now",
		"/seal:seal docs/specs/password-link", "AC-1 fails before the change and passes after it", "AC-2 still passes", "Spar review round", "gate is recorded", "ha done records completion", "completion report"},
}

// AC-3: both READMEs' alt texts tell the walkthrough in the same order.
func TestReadmeFlowVisualsAC3_WalkthroughAlt(t *testing.T) {
	const v = "walkthrough-alt-stale"
	for _, l := range rfvLangs {
		alt, ok := rfvAlt(rfvRead(t, l.readme[3:]), rfvDemo)
		if !ok {
			t.Errorf("jaekit:violation=%s %s does not show %s with alt text", v, l.readme, rfvDemo)
			continue
		}
		if miss := rfvInOrder(alt, rfvWalkthroughAlt[l.code]...); miss != "" {
			t.Errorf("jaekit:violation=%s %s walkthrough alt text lacks %q in order", v, l.readme, miss)
		}
	}
}

// rfvFolderLines are the README text tree: each entry is the tokens one line
// must hold, in the order the lines appear.
var rfvFolderLines = map[string][][]string{
	"ko": {
		{"docs/specs/password-link/"},
		{"SPEC.md", "← Spec"}, {"PLAN.md", "← Seal"}, {"REVIEW.md", "Spar", "요약", "← Seal"}, {"PROGRESS.md", "← Seal"}, {"runs.jsonl", "← ha"},
		{"docs/reviews/password-link/", "프로젝트의 Git이 무시하게 설정한 폴더", "Spar"},
		{"01-candidate/", "검토 라운드", "← Spar"}, {"gate.json", "검토 gate", "← Spar"},
		{".git/ha/", "Git이 추적하지 않"},
	},
	"en": {
		{"docs/specs/password-link/"},
		{"SPEC.md", "← Spec"}, {"PLAN.md", "← Seal"}, {"REVIEW.md", "Spar", "summary", "← Seal"}, {"PROGRESS.md", "← Seal"}, {"runs.jsonl", "← ha"},
		{"docs/reviews/password-link/", "your project's Git is set up to ignore", "Spar"},
		{"01-candidate/", "review round", "← Spar"}, {"gate.json", "review gate", "← Spar"},
		{".git/ha/", "not tracked by Git"},
	},
}

// AC-4: the README text tree shows who writes what, where.
func TestReadmeFlowVisualsAC4_FolderText(t *testing.T) {
	const v = "folder-text-stale"
	for _, l := range rfvLangs {
		section, ok := rfvSection(rfvRead(t, l.readme[3:]), l.records)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks %s", v, l.readme, l.records)
			continue
		}
		block, ok := rfvBlock(section, "text")
		if !ok {
			t.Errorf("jaekit:violation=%s %s %s has no text tree", v, l.readme, l.records)
			continue
		}
		lines := strings.Split(block, "\n")
		at := 0
		for _, tokens := range rfvFolderLines[l.code] {
			found := false
			for at < len(lines) {
				line := lines[at]
				at++
				all := true
				for _, tok := range tokens {
					if !strings.Contains(line, tok) {
						all = false
						break
					}
				}
				if all {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("jaekit:violation=%s %s text tree lacks a line with %q in order", v, l.readme, tokens)
				break
			}
		}
	}
}

var rfvFolder = map[string]struct{ runs, alt []string }{
	"ko": {
		runs: []string{"폴더 구조", "docs/specs/password-link/", "Spec이 씀", "SPEC.md", "Seal이 씀", "PLAN.md", "REVIEW.md", "Spar 기록 요약", "PROGRESS.md",
			"ha가 씀", "runs.jsonl", "Git이 추적하지 않음", "Spar", "docs/reviews/password-link/", "01-candidate/", "gate.json", "프로젝트의 Git이 무시하게 설정한 폴더",
			"ha", ".git/ha/", "가상 구성", "가상 예시"},
		alt: []string{"가상 목표 폴더 docs/specs/password-link/", "Spec이 SPEC.md", "Seal이 PLAN.md·REVIEW.md(검토 계획과 Spar 기록의 요약·위치)·PROGRESS.md",
			"ha가 runs.jsonl", "Spar의 검토 라운드와 gate는 프로젝트의 Git이 무시하게 설정한 docs/reviews/password-link/", ".git/ha/", "Git이 추적하지 않습니다"},
	},
	"en": {
		runs: []string{"Goal folder", "docs/specs/password-link/", "Written by Spec", "SPEC.md", "Written by Seal", "PLAN.md", "REVIEW.md", "Spar summary", "PROGRESS.md",
			"Written by ha", "runs.jsonl", "Not tracked by Git", "Spar", "docs/reviews/password-link/", "01-candidate/", "gate.json", "your project's Git is set up to ignore",
			"ha", ".git/ha/", "Fictional layout", "Fictional example"},
		alt: []string{"Fictional goal folder docs/specs/password-link/", "Spec writes SPEC.md", "Seal writes PLAN.md, REVIEW.md (the review plan, summarizing and pointing to the Spar records) and PROGRESS.md",
			"ha writes runs.jsonl", "Spar's review rounds and gate stay in docs/reviews/password-link/, a folder your project's Git is set up to ignore", ".git/ha/", "which Git does not track"},
	},
}

// AC-4: the folder image (both languages) and its alt text show the same
// structure as the text tree.
func TestReadmeFlowVisualsAC4_FolderImage(t *testing.T) {
	const v = "folder-image-stale"
	for _, l := range rfvLangs {
		want := rfvFolder[l.code]
		_, runs := rfvStill(t, v, "goal-folder", l.code)
		if miss := rfvRunsInOrder(runs, want.runs...); miss != "" {
			t.Errorf("jaekit:violation=%s goal-folder.%s.svg lacks the text %q in order", v, l.code, miss)
		}
		alt, ok := rfvAlt(rfvRead(t, l.readme[3:]), "assets/readme/goal-folder."+l.code+".png")
		if !ok {
			t.Errorf("jaekit:violation=%s %s does not show goal-folder.%s.png with alt text", v, l.readme, l.code)
		} else if miss := rfvInOrder(alt, want.alt...); miss != "" {
			t.Errorf("jaekit:violation=%s %s folder alt text lacks %q in order", v, l.readme, miss)
		}
	}
}

var rfvCard = map[string]struct{ evidence, alt []string }{
	"ko": {
		evidence: []string{"### 조건별 근거", "근거 출처: 완료 기록 seq 6", "ID", "종류", "필수", "충족", "근거 기록",
			"AC-1", "change", "예", "예", "2, 3", "AC-2", "maintain", "예", "예", "4"},
		alt: []string{"password-link의 가상 완료 보고", "ha report 출력 모양", "바뀐 것, 확인한 것, 직접 볼 것, 남은 한계, 다음 행동",
			"완료 기록 seq 6의 조건별 근거", "보증 수준 local", "검사 작성자 실행자"},
	},
	"en": {
		evidence: []string{"### Evidence per condition", "Source: completion record seq 6", "ID", "kind", "required", "satisfied", "records",
			"AC-1", "change", "yes", "yes", "2, 3", "AC-2", "maintain", "yes", "yes", "4"},
		alt: []string{"Fictional completion report for password-link", "ha report output", "Changed, Checked, To review, Remaining limits and Next step",
			"evidence per condition under completion record seq 6", "Assurance local", "check author executor"},
	},
}

// AC-5: the report card shows ha report's five summary labels in the
// README example's order, the evidence per condition, local assurance and
// the check author.
func TestReadmeFlowVisualsAC5_ReportCard(t *testing.T) {
	const v = "report-card-stale"
	labelLine := regexp.MustCompile(`^([^:` + "`" + `#|]{2,20}): `)
	for _, l := range rfvLangs {
		readme := rfvRead(t, l.readme[3:])
		section, ok := rfvSection(readme, l.report)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.report)
		}
		example, ok := rfvBlock(section, "markdown")
		if !ok {
			t.Fatalf("setup: %s %s has no example", l.readme, l.report)
		}
		var labels []string
		for _, line := range strings.Split(example, "\n") {
			if line == "" && len(labels) > 0 {
				break
			}
			if m := labelLine.FindStringSubmatch(line); m != nil {
				labels = append(labels, m[1]+":")
			}
		}
		if len(labels) != 5 {
			t.Fatalf("setup: %s %s example has %d summary lines, want 5", l.readme, l.report, len(labels))
		}
		_, runs := rfvStill(t, v, "report-card", l.code)
		for i, run := range runs {
			for _, label := range labels {
				if strings.HasPrefix(run, label) && run != label {
					runs[i] = label
				}
			}
		}
		if miss := rfvRunsInOrder(runs, labels...); miss != "" {
			t.Errorf("jaekit:violation=%s report-card.%s.svg lacks the summary label %q in the README's order", v, l.code, miss)
		}
		if miss := rfvRunsInOrder(runs, rfvCard[l.code].evidence...); miss != "" {
			t.Errorf("jaekit:violation=%s report-card.%s.svg lacks the evidence %q in order", v, l.code, miss)
		}
		limits := "assurance local; check author: executor"
		if l.code == "ko" {
			limits = "보증 수준 local; 검사 작성자: 실행자"
		}
		if !strings.Contains(strings.Join(runs, "\n"), limits) {
			t.Errorf("jaekit:violation=%s report-card.%s.svg lacks %q", v, l.code, limits)
		}
		alt, ok := rfvAlt(readme, "assets/readme/report-card."+l.code+".png")
		if !ok {
			t.Errorf("jaekit:violation=%s %s does not show report-card.%s.png with alt text", v, l.readme, l.code)
		} else if miss := rfvInOrder(alt, rfvCard[l.code].alt...); miss != "" {
			t.Errorf("jaekit:violation=%s %s report card alt text lacks %q in order", v, l.readme, miss)
		}
	}
}

// rfvArtwork lists each image's generator, its outlined SVGs with their
// editable partners, and the README file rendered from them.
var rfvArtwork = []struct {
	generator string
	pairs     [][2]string
}{
	{"assets/readme/source/build_usage_flow.py", [][2]string{
		{"assets/readme/usage-flow.ko.svg", "assets/readme/source/svg/usage-flow.ko.text.svg"},
		{"assets/readme/usage-flow.en.svg", "assets/readme/source/svg/usage-flow.en.text.svg"}}},
	{"assets/readme/source/build_goal_folder.py", [][2]string{
		{"assets/readme/goal-folder.ko.svg", "assets/readme/source/svg/goal-folder.ko.text.svg"},
		{"assets/readme/goal-folder.en.svg", "assets/readme/source/svg/goal-folder.en.text.svg"}}},
	{"assets/readme/source/build_report_card.py", [][2]string{
		{"assets/readme/report-card.ko.svg", "assets/readme/source/svg/report-card.ko.text.svg"},
		{"assets/readme/report-card.en.svg", "assets/readme/source/svg/report-card.en.text.svg"}}},
}

// AC-6: every README image has editable SVG sources and a generator in the
// repository, the published files were rendered from those sources, and the
// editable SVGs carry the same text as the outlined ones.
func TestReadmeFlowVisualsAC6_SourcesAndGenerators(t *testing.T) {
	const v = "artwork-source-missing"
	for _, path := range []string{"assets/readme/source/readme_svg.py", "assets/readme/source/build_readme_images.py",
		"assets/readme/source/build_walkthrough.py", "assets/readme/source/render_readme_images.cjs", "assets/readme/source/render_usage_flow.cjs"} {
		rfvRequired(t, v, path)
	}
	builder := string(rfvRequired(t, v, "assets/readme/source/build_readme_images.py"))
	renderer := string(rfvRequired(t, v, "assets/readme/source/render_readme_images.cjs"))
	for _, name := range []string{"build_usage_flow", "build_walkthrough", "build_goal_folder", "build_report_card"} {
		if !regexp.MustCompile(`(?m)^import ` + name + `\b`).MatchString(builder) {
			t.Errorf("jaekit:violation=%s build_readme_images.py does not run %s", v, name)
		}
	}
	for _, name := range []string{"'usage-flow'", "'goal-folder'", "'report-card'", "renderWalkthrough", "join: { animated: true }", "renders.json"} {
		if !strings.Contains(renderer, name) {
			t.Errorf("jaekit:violation=%s render_readme_images.cjs does not render %s", v, name)
		}
	}
	check := func(outlined, editable string) {
		a := rfvTexts(string(rfvRequired(t, v, outlined)))
		b := rfvEditableTexts(string(rfvRequired(t, v, editable)))
		if len(a) == 0 || strings.Join(a, "\n") != strings.Join(b, "\n") {
			t.Errorf("jaekit:violation=%s %s and its editable source %s do not carry the same text", v, outlined, editable)
		}
	}
	for _, art := range rfvArtwork {
		gen := string(rfvRequired(t, v, art.generator))
		for _, pair := range art.pairs {
			name := pair[0][strings.LastIndex(pair[0], "/")+1 : strings.Index(pair[0], ".")]
			if !strings.Contains(gen, name) {
				t.Errorf("jaekit:violation=%s %s does not write %s", v, art.generator, name)
			}
			check(pair[0], pair[1])
			rfvRendered(t, v, strings.TrimSuffix(pair[0], ".svg")+".png", pair[0])
		}
	}
	paths, _ := rfvFrames(t, v)
	for _, frame := range paths {
		name := frame[strings.LastIndex(frame, "/")+1:]
		check(frame, "assets/readme/source/svg/walkthrough/"+strings.TrimSuffix(name, ".svg")+".text.svg")
	}
	rfvRendered(t, v, rfvDemo, append([]string{rfvFramesDoc}, paths...)...)
}

var rfvGuides = []struct {
	path, meaning string
	need, old     []string
}{
	{"assets/readme/SOURCES.md", "## 수정할 때 유지할 뜻",
		[]string{"(source/build_usage_flow.py)", "(source/build_walkthrough.py)", "(source/build_goal_folder.py)", "(source/build_report_card.py)",
			"(source/readme_svg.py)", "(source/build_readme_images.py)", "(source/render_readme_images.cjs)", "(source/renders.json)", "(source/ARTWORK.md)",
			"SVG 프레임", "**이미지를 수정할 때만** 필요하며, Jaekit 사용이나 기본 저장소 검사에는 필요하지 않습니다"},
		[]string{"완성된 PNG·GIF로 제공했습니다", "편집 가능한 원본은 저장소에 없으므로"}},
	{"assets/readme/SOURCES.en.md", "## Meaning to preserve",
		[]string{"(source/build_usage_flow.py)", "(source/build_walkthrough.py)", "(source/build_goal_folder.py)", "(source/build_report_card.py)",
			"(source/readme_svg.py)", "(source/build_readme_images.py)", "(source/render_readme_images.cjs)", "(source/renders.json)", "(source/ARTWORK.md)",
			"SVG frames", "needed **only to edit the artwork**, not to use Jaekit or run the standard repository checks"},
		[]string{"as finished PNG and GIF files", "No editable source is kept in the repository"}},
}

var rfvMeaning = map[string][]string{
	"assets/readme/SOURCES.md": {"`Assurance:` 등급", "지금 구현할지 한 번 물은 뒤 멈춥니다", "\"응\"으로 Seal이 시작", "**별도 메시지로 Seal을 시작**",
		"`ha`가 모든 검사 실행을 기록", "`standard`·`strict` 목표는 완료 전에 Spar의 독립 검토 라운드", "`strict`는 작업 전 계획 검토", "`light`는 이유를 남기고 검토를 건너뛸 수 있습니다",
		"Seal은 사용자만 정할 수 있는 결정만 묻습니다", "기록된 검토 gate가 마지막 commit을 덮어야", "`PROGRESS.md`", "프로젝트의 Git이 무시하게 설정한 폴더(예: `.gitignore`에 더한 `docs/reviews/<goal>/`)", "`.git/ha/`",
		"같은 프로젝트의 같은 목표", "`ha report` 출력 모양", "보증 수준은 `local`", "인증된 검증이나 자동 배포처럼 그리지 않습니다"},
	"assets/readme/SOURCES.en.md": {"`Assurance:` tier", "asks once whether to implement it now, then stops", "The user's yes starts Seal", "**starting Seal in a separate message**",
		"`ha` records every check run", "`standard` and `strict` goals get independent Spar review rounds before completion", "`strict` goals also get a plan review", "`light` goals may skip review with a reason",
		"Seal asks the user only about decisions only the user can make", "a recorded review gate must cover the last commit", "`PROGRESS.md`", "a folder the project's Git is set up to ignore (for example `docs/reviews/<goal>/`, added to `.gitignore`)", "`.git/ha/`",
		"same goal in a new conversation in the same project", "shape of `ha report` output", "assurance stays `local`", "certified verification or automatic deployment"},
}

// AC-6: the image guides say how each image is made and edited, that the
// tools are needed only for editing, keep the meaning of the AC-1 flow, and
// no longer describe the images as finished files without sources.
func TestReadmeFlowVisualsAC6_GuidesDescribeArtwork(t *testing.T) {
	const v = "artwork-guide-stale"
	for _, g := range rfvGuides {
		text := rfvRead(t, g.path)
		for _, need := range g.need {
			if !strings.Contains(text, need) {
				t.Errorf("jaekit:violation=%s %s lacks %q", v, g.path, need)
			}
		}
		for _, old := range g.old {
			if strings.Contains(text, old) {
				t.Errorf("jaekit:violation=%s %s still says %q", v, g.path, old)
			}
		}
		meaning, ok := rfvSection(text, g.meaning)
		if !ok {
			t.Errorf("jaekit:violation=%s %s lacks %s", v, g.path, g.meaning)
			continue
		}
		if miss := rfvInOrder(meaning, rfvMeaning[g.path]...); miss != "" {
			t.Errorf("jaekit:violation=%s %s %s lacks %q in order", v, g.path, g.meaning, miss)
		}
	}
	artwork := string(rfvRequired(t, v, "assets/readme/source/ARTWORK.md"))
	for _, need := range []string{"`build_usage_flow.py`", "`build_walkthrough.py`", "`build_goal_folder.py`", "`build_report_card.py`",
		"`readme_svg.py`", "`build_readme_images.py`", "`render_readme_images.cjs`", "`renders.json`",
		"python3 assets/readme/source/build_readme_images.py --font-dir /path/to/fonts", "node assets/readme/source/render_readme_images.cjs",
		"These tools are needed only to edit the artwork. They are not required to use\nJaekit or to run the standard repository checks."} {
		if !strings.Contains(artwork, need) {
			t.Errorf("jaekit:violation=%s ARTWORK.md lacks %q", v, need)
		}
	}
	usage := rfvRead(t, "assets/readme/source/USAGE-FLOW.md")
	for _, need := range []string{"[ARTWORK.md](ARTWORK.md)", "Answer yes and Seal starts", "separate message", "Spar review rounds", "`strict` also gets a plan review", "`light` may skip it"} {
		if !strings.Contains(usage, need) {
			t.Errorf("jaekit:violation=%s USAGE-FLOW.md lacks %q", v, need)
		}
	}
	if strings.Contains(usage, "2. In a separate message, specify the goal folder and start Seal.") {
		t.Errorf("jaekit:violation=%s USAGE-FLOW.md still starts Seal only in a separate message", v)
	}
}

// rfvFictional is the label each image carries.
var rfvFictional = map[string]string{
	"usage-flow.ko": "사용 흐름 · 개념도", "usage-flow.en": "Usage flow · Concept illustration",
	"goal-folder.ko": "가상 예시", "goal-folder.en": "Fictional example",
	"report-card.ko": "가상 예시", "report-card.en": "Fictional example",
}

// AC-7: every image and alt text says it is fictional or a concept
// illustration; a pass in the walkthrough follows the ha command that
// recorded it; the Spar frames say review is not certification or
// deployment; and completion stays local.
func TestReadmeFlowVisualsAC7_FictionalAndLocal(t *testing.T) {
	const v = "illustration-overclaims"
	for name, label := range rfvFictional {
		runs := rfvTexts(string(rfvRequired(t, v, "assets/readme/"+name+".svg")))
		if rfvRunsInOrder(runs, label) != "" {
			t.Errorf("jaekit:violation=%s %s.svg lacks the label %q", v, name, label)
		}
	}
	_, frames := rfvFrames(t, v)
	spar := false
	for i, runs := range frames {
		if rfvRunsInOrder(runs, "Fictional walkthrough") != "" || rfvRunsInOrder(runs, "Illustrative sequence · not a recording of a real run") != "" {
			t.Errorf("jaekit:violation=%s walkthrough frame %d lacks its fictional labels", v, i+1)
		}
		command := ""
		for _, run := range runs {
			if strings.HasPrefix(run, "$ ") {
				command = run
			}
			if strings.Contains(run, " check pass (seq ") && !strings.HasPrefix(command, "$ ha check ") {
				t.Errorf("jaekit:violation=%s walkthrough frame %d shows %q without the ha check that recorded it", v, i+1, run)
			}
			if strings.Contains(run, "status: **complete**") && !strings.HasPrefix(command, "$ ha done ") {
				t.Errorf("jaekit:violation=%s walkthrough frame %d shows %q without ha done", v, i+1, run)
			}
			if strings.Contains(run, "Spar does not certify or deploy.") {
				spar = true
			}
		}
	}
	if !spar {
		t.Errorf("jaekit:violation=%s no walkthrough frame says Spar does not certify or deploy", v)
	}
	last := frames[len(frames)-1]
	if rfvRunsInOrder(last, "assurance local; check author: executor") != "" {
		t.Errorf("jaekit:violation=%s the last walkthrough frame does not show assurance local", v)
	}
	alts := map[string][]string{
		"ko": {"가상 사용 과정", "개념도", "가상 목표 폴더", "가상 완료 보고"},
		"en": {"Fictional walkthrough", "Concept illustration", "Fictional goal folder", "Fictional completion report"},
	}
	assets := []string{rfvDemo, "assets/readme/usage-flow.%s.png", "assets/readme/goal-folder.%s.png", "assets/readme/report-card.%s.png"}
	for _, l := range rfvLangs {
		readme := rfvRead(t, l.readme[3:])
		for i, asset := range assets {
			asset = strings.ReplaceAll(asset, "%s", l.code)
			alt, ok := rfvAlt(readme, asset)
			if !ok || !strings.Contains(alt, alts[l.code][i]) {
				t.Errorf("jaekit:violation=%s %s alt text of %s does not say %q", v, l.readme, asset, alts[l.code][i])
			}
			for _, claim := range []string{"certified", "independently verified", "deployed", "인증된", "배포됨"} {
				if strings.Contains(alt, claim) {
					t.Errorf("jaekit:violation=%s %s alt text of %s claims %q", v, l.readme, asset, claim)
				}
			}
		}
	}
}

// rfvKept are digests of README parts that must not change: the first task
// section, the install and update command blocks, the hero block and the
// list of link destinations. The expected values are the content at main
// 73f524d, written here; the commands are those of the v0.1.8 release,
// whose install pins and version check differ from 73f524d. The first task
// and the links are those of the readme-first-run goal, which made step 3
// start with the yes to Spec and added three links; its own check compares
// the other links with 81a7d47.
var rfvKept = map[string]map[string]string{
	"README.md": {
		"first-task": "23c9f73d21f3c79f922ed6a7ec957d0d8886ba9fa844399afa0de21cca1b7a1e",
		"commands":   "59f12488dd2183b2309aebdec44be212f4cf2383e234b5c3fbcead13a15dd150",
		"hero":       "e2786373776467d77948970760fa85cc010d488ea902310cc820856304fc51bc",
		"links":      "9436389d685a0a7ae3b3f4df6ff4fad8730407e8c6c7d256b098437375481f5f",
	},
	"README.ko.md": {
		"first-task": "1be7184fffabb6e5aebfeb8b7a7d0e9b41d168ad40bfe4fe52f0e394f0435c48",
		"commands":   "59f12488dd2183b2309aebdec44be212f4cf2383e234b5c3fbcead13a15dd150",
		"hero":       "b3c24b6ab3fc3fd3b9270086d61fe18135de39990a12d0159d4131f1d6e1ecbf",
		"links":      "dd19d229b2660ff43e8d2f5e9194f06133156c6d1345cdd9d60ccbf1a230652d",
	},
}

func rfvKeptParts(t *testing.T, path, firstTask string) map[string]string {
	t.Helper()
	text := rfvRead(t, path)
	out := map[string]string{}
	section, ok := rfvSection(text, firstTask)
	if !ok {
		t.Fatalf("%s lacks %s", path, firstTask)
	}
	out["first-task"] = rfvSHA([]byte(section))
	var commands []string
	inBash := false
	for _, line := range strings.Split(text, "\n") {
		switch {
		case line == "```bash":
			inBash = true
		case line == "```":
			inBash = false
		case inBash:
			commands = append(commands, line)
		}
	}
	out["commands"] = rfvSHA([]byte(strings.Join(commands, "\n")))
	hero, _, _ := strings.Cut(text, "\n</p>\n")
	out["hero"] = rfvSHA([]byte(hero))
	var links []string
	for _, m := range regexp.MustCompile(`\]\(([^)\s]+)\)|(?:href|src)="([^"]+)"`).FindAllStringSubmatch(text, -1) {
		links = append(links, m[1]+m[2])
	}
	out["links"] = rfvSHA([]byte(strings.Join(links, "\n")))
	return out
}

// AC-8: the first task, install and update commands, hero and links of both
// READMEs are as they were.
func TestReadmeFlowVisualsAC8_KeptParts(t *testing.T) {
	for _, c := range []struct{ path, firstTask string }{{"README.md", "### Your first task"}, {"README.ko.md", "### 첫 작업 맡기기"}} {
		got := rfvKeptParts(t, c.path, c.firstTask)
		for part, want := range rfvKept[c.path] {
			if got[part] != want {
				t.Errorf("%s %s changed: sha256 %s, want %s", c.path, part, got[part], want)
			}
		}
	}
}

// rfvRun is the one fictional standard run that the README examples, the
// report card and the walkthrough show: base f46020ce6f77, the review gate
// noted at seq 5, completion recorded at seq 6 on commit e01fdf1e9c98. Each
// language lists fixed tokens of its records section: the SPEC example, the
// completion report example in order, the reading notes, the runs.jsonl
// example and the REVIEW.md description, plus tokens of the earlier gateless
// run that must be gone.
var rfvRun = map[string]struct{ report, reading, record, review, old, card []string }{
	"ko": {
		report: []string{"바뀐 것: base f46020ce6f77 이후", "다음 행동: 없음; 완료 기록 seq 6이 유효하고 남은 할 일이 없음",
			"- 기록 head: seq 6 (e2fd520c706e)", "완료 기록: seq 6", "근거 출처: 완료 기록 seq 6",
			"- 완료 기록 seq 6, commit e01fdf1e9c98; 지금 HEAD e01fdf1e9c98를 덮음", "Seal의 판단:",
			"등급 `standard`", "seq 5에 기록한 그 gate가 마지막 commit e01fdf1e9c98을 덮습니다", "뺀 리뷰어(waiver)는 없습니다",
			"올릴 head는 완료 기록의 commit e01fdf1e9c98입니다"},
		reading: []string{"seq 5는 Spar gate를 기록한 `review` note", "완료 기록 seq 6은 `ha done`이 완료를 기록했다는 뜻"},
		record:  []string{"`review`", `"seq":6,"prev":"578fb93e…","kind":"done"`, `"commit":"e01fdf1e…"`},
		review:  []string{"`REVIEW.md`는 이 계획의 검토", "Spar 라운드와 gate를 요약해"},
		old: []string{"`Assurance:` 줄이 없어", "검토 gate가 필요 없습니다", "fe4597cf233f", "e887828a0035", "0a94eba41234",
			"완료 기록 seq 5", "a22dbbfe", "aa4bd917", "작업 시작 전에 이 계획을 검토한 기록입니다"},
		card: []string{"base f46020ce6f77", "완료 기록 seq 6이 유효하고", "근거 출처: 완료 기록 seq 6"},
	},
	"en": {
		report: []string{"Changed: 1 files and 1 commits outside the bundle documents since base f46020ce6f77",
			"Next step: none; completion record seq 6 holds and nothing is left to do",
			"- record head: seq 6 (e2fd520c706e)", "completion record: seq 6", "Source: completion record seq 6",
			"- completion record seq 6, commit e01fdf1e9c98; HEAD e01fdf1e9c98 is covered", "Seal's judgment:",
			"tier `standard`", "recorded at seq 5, covers the last commit e01fdf1e9c98", "No reviewer was waived",
			"the completion record's commit e01fdf1e9c98"},
		reading: []string{"Seq 5 is the `review` note that recorded the Spar gate", "completion record seq 6 means `ha done` recorded completion"},
		record:  []string{"`review`", `"seq":6,"prev":"578fb93e…","kind":"done"`, `"commit":"e01fdf1e…"`},
		review:  []string{"`REVIEW.md` holds the review of this plan", "summarizes the Spar rounds and gate"},
		old: []string{"has no `Assurance:` line", "needs no review gate", "fe4597cf233f", "e887828a0035", "0a94eba41234",
			"completion record seq 5", "a22dbbfe", "aa4bd917", "records the review of this plan before work starts"},
		card: []string{"since base f46020ce6f77", "completion record seq 6 holds", "Source: completion record seq 6"},
	},
}

// rfvRunFrames are walkthrough tokens of the same run.
var rfvRunFrames = []string{"base f46020ce6f77", "noted: review (seq 5)", "- completion record: seq 6", "completion record seq 6 holds"}

// rfvDetails returns the <details> block whose summary names file.
func rfvDetails(text, file string) (string, bool) {
	i := strings.Index(text, "<summary><strong>"+file+"</strong>")
	if i < 0 {
		return "", false
	}
	j := strings.Index(text[i:], "</details>")
	if j < 0 {
		return "", false
	}
	return text[i : i+j], true
}

// AC-9: the README examples, the report card and the walkthrough show one
// fictional standard run of password-link, whose review gate was noted
// before ha done.
func TestReadmeFlowVisualsAC9_ExamplesShowOneRun(t *testing.T) {
	const v = "example-run-mismatch"
	for _, l := range rfvLangs {
		want := rfvRun[l.code]
		readme := rfvRead(t, l.readme[3:])
		records, ok := rfvSection(readme, l.records)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.records)
		}
		spec, ok := rfvDetails(records, "SPEC.md")
		if !ok || !strings.Contains(spec, "\nAssurance: standard\n") {
			t.Errorf("jaekit:violation=%s %s SPEC.md example has no Assurance: standard line", v, l.readme)
		}
		report, ok := rfvSection(readme, l.report)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.report)
		}
		example, ok := rfvBlock(report, "markdown")
		if !ok {
			t.Fatalf("setup: %s %s has no example", l.readme, l.report)
		}
		if miss := rfvInOrder(example, want.report...); miss != "" {
			t.Errorf("jaekit:violation=%s %s completion report example lacks %q in order", v, l.readme, miss)
		}
		if miss := rfvInOrder(report, want.reading...); miss != "" {
			t.Errorf("jaekit:violation=%s %s reading notes lack %q", v, l.readme, miss)
		}
		record, ok := rfvDetails(records, "runs.jsonl")
		if !ok {
			t.Fatalf("setup: %s has no runs.jsonl example", l.readme)
		}
		if miss := rfvInOrder(record, want.record...); miss != "" {
			t.Errorf("jaekit:violation=%s %s runs.jsonl example lacks %q", v, l.readme, miss)
		}
		if miss := rfvInOrder(records, want.review...); miss != "" {
			t.Errorf("jaekit:violation=%s %s REVIEW.md description lacks %q", v, l.readme, miss)
		}
		for _, old := range want.old {
			if strings.Contains(records, old) {
				t.Errorf("jaekit:violation=%s %s %s still shows %q", v, l.readme, l.records, old)
			}
		}
		_, runs := rfvStill(t, v, "report-card", l.code)
		joined := strings.Join(runs, "\n")
		for _, token := range want.card {
			if !strings.Contains(joined, token) {
				t.Errorf("jaekit:violation=%s report-card.%s.svg lacks %q", v, l.code, token)
			}
		}
	}
	_, frames := rfvFrames(t, v)
	var all []string
	for _, runs := range frames {
		all = append(all, runs...)
	}
	joined := strings.Join(all, "\n")
	for _, token := range rfvRunFrames {
		if !strings.Contains(joined, token) {
			t.Errorf("jaekit:violation=%s the walkthrough frames lack %q", v, token)
		}
	}
}
