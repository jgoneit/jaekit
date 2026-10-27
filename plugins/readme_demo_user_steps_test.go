package plugins

// Checks of the readme-demo-user-steps goal: the README walkthrough GIF shows
// five scenes centred on what the person does (ask Spec, review the goal,
// start with Yes or /seal:seal, the agent's work in one scene, read the
// completion report) in 13 to 17 seconds, and both READMEs' alt texts tell
// the same five scenes.
//
// Scenes are observed through the outlined frame SVGs' data-text runs, and
// timing through the GIF's own frame delays. Each test observes fixed
// tokens, lines and digests, never an open category of sentences.
//
// Every failure message about a property of a change condition opens with
// the test's violation marker, which ha's go-test-json adapter reads. Any
// read error (a missing file included), a corrupt frame list or GIF, or a GIF
// whose frames do not match its frame list fails with a "setup:" message and
// no marker, which is neither the violation nor a pass. The maintain tests
// also run, as subtests, the readme-flow-visuals tests that observe the same
// properties.

import (
	"encoding/binary"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
)

const (
	rduReportHeading = "## 완료 보고"
	rduFrameDir      = "assets/readme/source/walkthrough"
	rduEditableDir   = "assets/readme/source/svg/walkthrough"
)

// rduRead returns a file of the tree under test (paths relative to the
// repository root). Any read error is a setup failure.
func rduRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile("../" + path)
	if err != nil {
		t.Fatalf("setup: read %s: %v", path, err)
	}
	return b
}

// rduFrames returns the walkthrough's frame SVGs, in order, and each frame's
// text runs.
func rduFrames(t *testing.T) ([]string, [][]string) {
	t.Helper()
	var plan struct {
		Output string `json:"output"`
		Frames []struct {
			SVG string `json:"svg"`
		} `json:"frames"`
	}
	if err := json.Unmarshal(rduRead(t, rfvFramesDoc), &plan); err != nil {
		t.Fatalf("setup: %s is not valid JSON: %v", rfvFramesDoc, err)
	}
	if plan.Output != rfvDemo || len(plan.Frames) == 0 {
		t.Fatalf("setup: %s does not build %s from frames", rfvFramesDoc, rfvDemo)
	}
	var paths []string
	var runs [][]string
	for _, f := range plan.Frames {
		paths = append(paths, f.SVG)
		runs = append(runs, rfvTexts(string(rduRead(t, f.SVG))))
	}
	return paths, runs
}

// rduGIFDelays returns each frame's delay in hundredths of a second, read
// from the graphic control extension before its image descriptor.
func rduGIFDelays(t *testing.T, path string, b []byte) []int {
	t.Helper()
	bad := func() { t.Fatalf("setup: %s is not a well-formed GIF", path) }
	if len(b) < 13 || string(b[:6]) != "GIF89a" {
		bad()
	}
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
	var delays []int
	delay := 0
	for i < len(b) {
		switch b[i] {
		case 0x21:
			if i+1 < len(b) && b[i+1] == 0xF9 {
				if i+6 > len(b) || b[i+2] != 4 {
					bad()
				}
				delay = int(binary.LittleEndian.Uint16(b[i+4 : i+6]))
			}
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
			delays = append(delays, delay)
			delay = 0
		case 0x3B:
			return delays
		default:
			bad()
		}
	}
	bad()
	return nil
}

// rduScenes are the walkthrough's five scenes, one frame each, with the fixed
// text runs each frame shows, in order.
var rduScenes = []struct {
	name string
	runs []string
}{
	{"the request", []string{"/spec:spec Add a “Forgot password” link", "to the login screen", "Describe the change you want"}},
	{"the goal review", []string{"wrote docs/specs/password-link/SPEC.md", "Status: Ready", "Assurance: standard",
		"AC-1 The login screen shows", "AC-2 The existing login form keeps working.", "Ask for changes"}},
	{"the start", []string{"Implement docs/specs/password-link", "now? Assurance: standard.", "Yes",
		"in a separate message", "/seal:seal docs/specs/password-link"}},
	{"the agent's work", []string{"$ ha check docs/specs/password-link --baseline AC-1", "AC-1 baseline fail_as_expected (seq 2)", "before the change",
		"$ ha check docs/specs/password-link AC-1 AC-2", "AC-1 check pass (seq 3)", "after the change", "AC-2 check pass (seq 4)", "still passes",
		"$ spar run --stage candidate", "Spar review round", "$ spar gate docs/reviews/password-link/01-candidate",
		"$ ha note docs/specs/password-link review --gate", "noted: review (seq 5)", "gate recorded",
		"$ ha done docs/specs/password-link", "- status: **complete** (exit 0)", "- completion record: seq 6", "completion recorded",
		"You do not type these commands."}},
	{"the completion report", []string{rduReportHeading, "Changed:", "since base f46020ce6f77", "Checked:", "To review:", "Remaining limits:", "Next step:",
		"### Evidence per condition", "| AC-1 | change | yes | yes | 2, 3 |", "| AC-2 | maintain | yes | yes | 4 |"}},
}

// rduAgentScene is the index of the one scene that shows commands.
const rduAgentScene = 3

// AC-1: the walkthrough shows exactly the five scenes, in order, and only the
// agent's scene shows the commands the agent runs.
func TestReadmeDemoUserStepsAC1_FiveScenes(t *testing.T) {
	const v = "demo-scenes-stale"
	_, frames := rduFrames(t)
	if len(frames) != len(rduScenes) {
		t.Fatalf("jaekit:violation=%s the walkthrough has %d scenes, want %d", v, len(frames), len(rduScenes))
	}
	for i, scene := range rduScenes {
		if miss := rfvRunsInOrder(frames[i], scene.runs...); miss != "" {
			t.Errorf("jaekit:violation=%s scene %d (%s) lacks %q in order", v, i+1, scene.name, miss)
		}
		for _, run := range frames[i] {
			if strings.HasPrefix(run, "$ ") && i != rduAgentScene {
				t.Errorf("jaekit:violation=%s scene %d (%s) shows the command %q", v, i+1, scene.name, run)
			}
		}
	}
}

// AC-2: the GIF plays for 13 to 17 seconds, every scene stays at least 2
// seconds, and the completion report scene stays longest.
func TestReadmeDemoUserStepsAC2_Timing(t *testing.T) {
	const v = "demo-timing-stale"
	paths, frames := rduFrames(t)
	delays := rduGIFDelays(t, rfvDemo, rduRead(t, rfvDemo))
	if len(delays) != len(paths) {
		t.Fatalf("setup: %s has %d frames, %s lists %d", rfvDemo, len(delays), rfvFramesDoc, len(paths))
	}
	report, total := -1, 0
	for i, runs := range frames {
		if rfvRunsInOrder(runs, rduReportHeading) == "" {
			report = i
		}
		total += delays[i]
	}
	if total < 1300 || total > 1700 {
		t.Errorf("jaekit:violation=%s %s plays for %d.%02d s, want 13 to 17 s", v, rfvDemo, total/100, total%100)
	}
	if report < 0 {
		t.Fatalf("jaekit:violation=%s no scene shows the completion report", v)
	}
	for i, d := range delays {
		if d < 200 {
			t.Errorf("jaekit:violation=%s scene %d stays %d.%02d s, want at least 2 s", v, i+1, d/100, d%100)
		}
		if i != report && d >= delays[report] {
			t.Errorf("jaekit:violation=%s scene %d stays %d.%02d s, not shorter than the completion report scene", v, i+1, d/100, d%100)
		}
	}
}

// rduAlt are fixed tokens of the walkthrough's alt text in each language, in
// order: fictional, five scenes, the request, the goal review, the two
// starts, commands the person does not type, the agent's work, the report.
var rduAlt = map[string][]string{
	"ko": {"password-link 가상 사용 과정", "다섯 장면", "/spec:spec으로 Spec에 요청", "목표를 확인", "지금 구현할지 묻는 질문에 Yes(응)로 답하거나",
		"별도 메시지로 /seal:seal docs/specs/password-link를 불러 시작", "내가 입력하지 않는 명령", "AC-1은 바꾸기 전 실패·바꾼 뒤 통과", "AC-2는 계속 통과",
		"Spar 검토 라운드를 받고 gate를 기록", "ha done이 완료를 기록", "완료 보고를 읽습니다"},
	"en": {"Fictional walkthrough of password-link", "five scenes", "you ask Spec with /spec:spec", "you review the goal", "answering Yes when Spec asks whether to implement it now",
		"/seal:seal docs/specs/password-link in a separate message", "commands you do not type", "AC-1 fails before the change and passes after it", "AC-2 still passes",
		"a Spar review round runs and its gate is recorded", "ha done records completion", "you read the completion report"},
}

// AC-4: both READMEs' alt texts tell the five scenes in the same order.
func TestReadmeDemoUserStepsAC4_AltText(t *testing.T) {
	const v = "demo-alt-stale"
	for _, l := range rfvLangs {
		alt, ok := rfvAlt(string(rduRead(t, l.readme[3:])), rfvDemo)
		if !ok {
			t.Errorf("jaekit:violation=%s %s does not show %s with alt text", v, l.readme, rfvDemo)
			continue
		}
		if miss := rfvInOrder(alt, rduAlt[l.code]...); miss != "" {
			t.Errorf("jaekit:violation=%s %s walkthrough alt text lacks %q in order", v, l.readme, miss)
		}
	}
}

// rduOutput are whole output lines of the agent's scene in the formats ha
// v0.1.7 prints, with the seqs of the README example's run.
var rduOutput = []string{"AC-1 baseline fail_as_expected (seq 2)", "AC-1 check pass (seq 3)", "AC-2 check pass (seq 4)",
	"noted: review (seq 5)", "- status: **complete** (exit 0)", "- completion record: seq 6"}

// rduEvidence are the README example's evidence rows, which give the seqs
// AC-1 and AC-2 were recorded at.
var rduEvidence = map[string][]string{
	"ko": {"| AC-1 | change | 예 | 예 | 2, 3 |", "| AC-2 | maintain | 예 | 예 | 4 |"},
	"en": {"| AC-1 | change | yes | yes | 2, 3 |", "| AC-2 | maintain | yes | yes | 4 |"},
}

// AC-3: the walkthrough shows the README example's run: the same goal, base
// and seqs, v0.1.7 output lines, and shortened panels marked as such.
func TestReadmeDemoUserStepsAC3_SameRun(t *testing.T) {
	t.Run("ExamplesShowOneRun", TestReadmeFlowVisualsAC9_ExamplesShowOneRun)
	_, frames := rduFrames(t)
	runs := map[string]bool{}
	for _, frame := range frames {
		for _, run := range frame {
			runs[run] = true
		}
	}
	for _, line := range rduOutput {
		if !runs[line] {
			t.Errorf("no walkthrough scene shows the line %q", line)
		}
	}
	for _, l := range rfvLangs {
		report, ok := rfvSection(string(rduRead(t, l.readme[3:])), l.report)
		if !ok {
			t.Fatalf("setup: %s lacks %s", l.readme, l.report)
		}
		if miss := rfvInOrder(report, rduEvidence[l.code]...); miss != "" {
			t.Errorf("%s completion report example lacks %q", l.readme, miss)
		}
	}
	for i, frame := range frames {
		shortened := rfvRunsInOrder(frame, "$ ") == "" || rfvRunsInOrder(frame, rduReportHeading) == ""
		if shortened && rfvRunsInOrder(frame, " · shortened") != "" {
			t.Errorf("scene %d leaves out output lines but its panel title does not say shortened", i+1)
		}
	}
}

// rduOldLength are forms in which the artwork guides could state the scene
// count or the play time of the earlier nine-scene walkthrough.
var rduOldLength = []string{"9 frames", "nine frames", "9 scenes", "nine scenes", "25.3", "25 seconds", "9장면", "아홉 장면", "프레임 9", "25초"}

// rduNames returns the sorted names of the files in a folder of the tree.
func rduNames(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir("../" + dir)
	if err != nil {
		t.Fatalf("setup: read %s: %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)
	return names
}

// AC-5: the GIF is rebuilt from the frames it lists and nothing else is left
// in the frame folders, the guides do not state the old length, and the GIF
// stays a fictional, local illustration.
func TestReadmeDemoUserStepsAC5_SourcesAndCharacter(t *testing.T) {
	t.Run("SourcesAndGenerators", TestReadmeFlowVisualsAC6_SourcesAndGenerators)
	t.Run("FictionalAndLocal", TestReadmeFlowVisualsAC7_FictionalAndLocal)
	paths, _ := rduFrames(t)
	frames, editable := []string{"frames.json"}, []string{}
	for _, p := range paths {
		name := p[strings.LastIndex(p, "/")+1:]
		if p != rduFrameDir+"/"+name {
			t.Errorf("%s lists %s outside %s", rfvFramesDoc, p, rduFrameDir)
		}
		frames = append(frames, name)
		editable = append(editable, strings.TrimSuffix(name, ".svg")+".text.svg")
	}
	sort.Strings(frames)
	sort.Strings(editable)
	for _, folder := range []struct {
		dir  string
		want []string
	}{{rduFrameDir, frames}, {rduEditableDir, editable}} {
		if got := rduNames(t, folder.dir); strings.Join(got, "\n") != strings.Join(folder.want, "\n") {
			t.Errorf("%s holds %q, want only the listed frames' sources %q", folder.dir, got, folder.want)
		}
	}
	if n := len(rduGIFDelays(t, rfvDemo, rduRead(t, rfvDemo))); n != len(paths) {
		t.Errorf("%s has %d frames, %s lists %d", rfvDemo, n, rfvFramesDoc, len(paths))
	}
	for _, guide := range []string{"assets/readme/SOURCES.md", "assets/readme/SOURCES.en.md", "assets/readme/source/ARTWORK.md"} {
		text := string(rduRead(t, guide))
		for _, old := range rduOldLength {
			if strings.Contains(text, old) {
				t.Errorf("%s states the earlier walkthrough's length: %q", guide, old)
			}
		}
	}
}

// rduKept are digests of README sections that must not change: the flow
// section with the walkthrough's alt text masked, the get-started section
// and the records section. The expected values are the content at main
// 6e9a74d, written here; the get-started sections are those of the v0.1.8
// release, whose install pins and version check differ from 6e9a74d. All
// three are now those of the readme-first-run goal, which edited parts of
// each; its own check compares the rest of them with 81a7d47.
var rduKept = map[string]map[string]string{
	"README.md": {
		"flow":        "4babcbfb187c912fe900fcf833cb7b70c57fd9ca6af54d06fef77e9b9c6d1d20",
		"get-started": "8942bfb730cb2f02041d1b1f8c24278335974615994b8b4c8bbe83c00862e794",
		"records":     "a486223304226b25bcebd589c847d817ca5acbc228fd7c8e6616ba0b1eeccb18",
	},
	"README.ko.md": {
		"flow":        "439d9d536839b77b96e5ea79f5af63decd3e95e47c4c9286933e2b2ed8627b5f",
		"get-started": "9651222c93b37105dde06f741d1b30c56389dd75e3373794cb9b188c3689d76e",
		"records":     "62273dfa903a54e609355dbbd81614fddd4e0c9779e9772a5235a6bc41470c10",
	},
}

// AC-6: the GIF stays first in the same section of both READMEs, as the one
// English GIF, with the same sentence below it; the rest of that section, the
// get-started and records sections, the other images, the hero and the links
// are as they were.
func TestReadmeDemoUserStepsAC6_KeptParts(t *testing.T) {
	t.Run("DiagramShowsTheFlow", TestReadmeFlowVisualsAC1_DiagramShowsTheFlow)
	t.Run("CaptionsShowTheFlow", TestReadmeFlowVisualsAC1_CaptionsShowTheFlow)
	t.Run("UsageFlowImage", TestReadmeFlowVisualsAC2_UsageFlowImage)
	t.Run("FolderText", TestReadmeFlowVisualsAC4_FolderText)
	t.Run("FolderImage", TestReadmeFlowVisualsAC4_FolderImage)
	t.Run("ReportCard", TestReadmeFlowVisualsAC5_ReportCard)
	t.Run("KeptParts", TestReadmeFlowVisualsAC8_KeptParts)
	starts := map[string]string{"en": "## Get started", "ko": "## 시작하기"}
	for _, l := range rfvLangs {
		path := l.readme[3:]
		text := string(rduRead(t, path))
		if n := strings.Count(text, rfvDemo); n != 1 {
			t.Errorf("%s shows %s %d times, want once", path, rfvDemo, n)
		}
		flow, ok := rfvSection(text, l.flow)
		if !ok {
			t.Errorf("%s lacks %s", path, l.flow)
			continue
		}
		alt, ok := rfvAlt(flow, rfvDemo)
		first := strings.TrimSpace(strings.TrimPrefix(flow, l.flow))
		if !ok || !strings.HasPrefix(first, "!["+alt+"]("+rfvDemo+")") {
			t.Errorf("%s %s does not open with %s", path, l.flow, rfvDemo)
			continue
		}
		parts := map[string]string{"flow": strings.Replace(flow, "!["+alt+"]", "![<alt>]", 1)}
		for name, heading := range map[string]string{"get-started": starts[l.code], "records": l.records} {
			section, ok := rfvSection(text, heading)
			if !ok {
				t.Errorf("%s lacks %s", path, heading)
			}
			parts[name] = section
		}
		for name, section := range parts {
			if got, want := rfvSHA([]byte(section)), rduKept[path][name]; got != want {
				t.Errorf("%s %s section changed: sha256 %s, want %s", path, name, got, want)
			}
		}
	}
}
