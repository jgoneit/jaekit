package plugins

import (
	"strings"
	"testing"
)

// The progress-timeline goal: PROGRESS.md keeps a short `## 현재` stamped with
// its check time and record head, and an append-only `## 타임라인` of the
// events that changed a judgment or the next action. The contract, the bundle
// formats Seal loads, and the template all say so: every rule below is checked
// against all three, so none of them can drop a rule the others keep.

const (
	ptContract  = "../contracts/bundle.md"
	ptReference = "seal/skills/seal/references/bundle.md"
	ptTemplate  = "seal/skills/seal/assets/templates/PROGRESS.md"
)

var (
	ptKinds = []string{"`관측`", "`원인`", "`결정`", "`조치`", "`검증`", "`막힘`", "`재개`"}
	ptMarks = []string{"`가설`", "`확정`", "`기각`", "`예정`", "`반영`", "`대기`", "`통과`", "`실패`"}
)

// A ptRule is one rule of a PROGRESS part: the contract says ko, and the
// bundle formats and the template say en (compared in lower case).
type ptRule struct{ part, ko, en string }

// The PROGRESS parts a rule can name.
const (
	ptCurrent  = "## 현재"
	ptTimeline = "## 타임라인"
	ptPlan     = "## 계획 변경"
	ptBlocks   = "## 막힘"
)

// ptDoc is one of the three documents, split into the PROGRESS parts.
type ptDoc struct {
	name  string
	en    bool
	parts map[string]string
}

func ptRow(t *testing.T, name, sec, prefix string) string {
	t.Helper()
	for _, l := range strings.Split(sec, "\n") {
		if strings.HasPrefix(l, prefix) {
			return l
		}
	}
	t.Fatalf("%s has no row starting with %q", name, prefix)
	return ""
}

func ptContractSec(t *testing.T) string { return mdSection(t, ptContract, "## 5. `PROGRESS.md`") }

func ptReferenceSec(t *testing.T) string { return mdSection(t, ptReference, "## PROGRESS.md") }

// ptOpening is the text of a PROGRESS section before its table.
func ptOpening(sec string) string {
	if i := strings.Index(sec, "\n|"); i >= 0 {
		return sec[:i]
	}
	return sec
}

func ptDocs(t *testing.T) []ptDoc {
	t.Helper()
	c := ptContractSec(t)
	r := ptReferenceSec(t)
	tableRows := func(name, sec string) map[string]string {
		m := map[string]string{}
		for _, p := range []string{ptCurrent, ptTimeline, ptPlan, ptBlocks} {
			m[p] = ptRow(t, name, sec, "| `"+p+"` |")
		}
		return m
	}
	cp := tableRows("bundle contract", c)
	cp[ptTimeline] += "\n" + mdSection(t, ptContract, "### 5.1 타임라인")
	rp := tableRows("Seal bundle formats", r)
	rp[ptTimeline] += "\n" + mdSection(t, ptReference, "### `## 타임라인`")
	tp := map[string]string{}
	for _, p := range []string{ptCurrent, ptTimeline, ptPlan, ptBlocks} {
		tp[p] = mdSection(t, ptTemplate, p)
	}
	return []ptDoc{
		{"bundle contract", false, cp},
		{"Seal bundle formats", true, rp},
		{"PROGRESS template", true, tp},
	}
}

func ptCheck(t *testing.T, rules []ptRule) {
	t.Helper()
	for _, d := range ptDocs(t) {
		for _, r := range rules {
			text, want := d.parts[r.part], r.ko
			if d.en {
				text, want = strings.ToLower(text), strings.ToLower(r.en)
			}
			if !strings.Contains(text, want) {
				t.Errorf("%s `%s` lacks %q", d.name, r.part, want)
			}
		}
	}
}

// ptTokens is a rule for each contract token, said the same in every document.
func ptTokens(part string, tokens ...string) []ptRule {
	var rules []ptRule
	for _, s := range tokens {
		rules = append(rules, ptRule{part, s, s})
	}
	return rules
}

// `## 현재` is a short summary with its check time
// (with time zone), the ha verdict and record head (`none` before `ha start`),
// the work in progress, the open problems, the next work, and how to resume;
// past events are in the timeline. This form applies to goals started with
// Seal 0.1.6 or later.
func TestProgressCurrentStamped(t *testing.T) {
	ptCheck(t, []ptRule{
		{ptCurrent, "짧은 최신 요약", "short, current summary"},
		{ptCurrent, "확인 시각", "check time"},
		{ptCurrent, "시간대", "time zone"},
		{ptCurrent, "기록 head", "record head"},
		{ptCurrent, "seq와 hash", "seq and hash"},
		{ptCurrent, "(`ha start` 전) `ha status`처럼 `none`", "`none` before `ha start`"},
		{ptCurrent, "완료 기록 seq", "completion record seq"},
		{ptCurrent, "진행 중인 일", "work in progress"},
		{ptCurrent, "남은 문제", "open problems"},
		{ptCurrent, "다음 작업", "next work"},
		{ptCurrent, "재개 방법", "how to resume"},
		{ptCurrent, "고칠 때마다 확인 시각과 기록 head", "restamp the check time and record head whenever it changes"},
		{ptCurrent, "기록 head를 `ha status`와 비교해 낡았는지", "compares the record head with `ha status`"},
		{ptCurrent, "같은 seq라도 hash가 다르면 다른 기록", "the same seq with another hash is another record"},
		{ptCurrent, "확인 시각은 참고", "the check time is for reference only"},
		{ptCurrent, "지난 경위는 쓰지 않고 `## 타임라인`을 가리킨다", "past events stay out; point to `## 타임라인`"},
	})
	requireIn(t, "bundle contract §5 opening", ptOpening(ptContractSec(t)),
		"seal 0.1.6 이상으로 시작한 목표", "시작할 때의 PROGRESS 형식을 유지한다", "`ha status`의 `skill:` 줄")
	requireIn(t, "Seal bundle formats PROGRESS.md opening", ptOpening(ptReferenceSec(t)),
		"goals whose start record names seal 0.1.6 or later", "keeps the PROGRESS form it started with", "the `skill:` line of `ha status`")
}

// the optional `## 타임라인` holds append-only entries
// with a time (with time zone), the record head, a kind, and a summary; bodies
// mark causes, actions, and verification, and point to evidence instead of
// copying check output.
func TestProgressTimelineDefined(t *testing.T) {
	rules := []ptRule{
		{ptTimeline, "선택 절", "optional"},
		{ptTimeline, "실행자의 주장", "entries are claims"},
		{ptTimeline, " · seq <기록 head> — <종류> · <한 줄 요약>", " · seq <record head> — <kind> · <one-line summary>"},
		{ptTimeline, "시간대", "time zone"},
		{ptTimeline, "정확한 시각을 모르면 지어내지 않는다", "never invent a time you do not know"},
		{ptTimeline, "`seq none`", "`seq none`"},
		{ptTimeline, "검사 실행에 묶인 항목은 그 실행의 seq", "an entry about a check run uses that run's seq"},
		{ptTimeline, "근거는 AC, commit, seq로 가리키고", "point to evidence by AC, commit, and seq"},
		{ptTimeline, "검사 출력 원문은 옮기지 않는다", "never copy check output"},
		{ptTimeline, "덧붙이기만 한다", "append only"},
		{ptTimeline, "이전 항목은 고치지 않는다", "never edit an earlier entry"},
		{ptTimeline, "무엇을 어떤 근거로 기각했는지", "what was rejected and on what grounds"},
		{ptTimeline, "한 항목이어도 된다", "may be one entry"},
	}
	rules = append(rules, ptTokens(ptTimeline, ptKinds...)...)
	rules = append(rules, ptTokens(ptTimeline, ptMarks...)...)
	ptCheck(t, rules)
}

// only events that changed a judgment or the next
// action are recorded; the length figures are editing guidance, not limits.
func TestProgressTimelineWhatToRecord(t *testing.T) {
	ptCheck(t, []ptRule{
		{ptTimeline, "판단이나 다음 행동을 바꾼 사건", "change a judgment or the next action"},
		{ptTimeline, "새 문제", "a new problem"},
		{ptTimeline, "원인 확정·변경", "a cause confirmed or changed"},
		{ptTimeline, "접근 방법 변경", "a change of approach"},
		{ptTimeline, "중요한 검증 결과", "an important verification result"},
		{ptTimeline, "막힘과 재개", "blocks and resumes"},
		{ptTimeline, "재개는 다시 일을 시작하는 모든 경우다", "every resume counts"},
		{ptTimeline, "새 세션이 이어받을 때", "a new session taking the goal up"},
		{ptTimeline, "막힘이 풀렸을 때", "a cleared block"},
		{ptTimeline, "`needs_user`에 답을 받았을 때", "an answer to a `needs_user` question"},
		{ptTimeline, "`## 현재`가 실제 상태와 달랐던 것", "where `## 현재` differed from the actual state"},
		{ptTimeline, "일상적인 편집과 읽기", "routine edits and reads"},
		{ptTimeline, "같은 실패의 반복 실행", "repeated runs of the same failure"},
		{ptTimeline, "변화 없는 진행 보고", "progress reports with no change"},
		{ptTimeline, "항목은 대략 3~6줄", "an entry runs about 3 to 6 lines"},
		{ptTimeline, "`## 현재`는 대략 5~8줄", "`## 현재` about 5 to 8 lines"},
		{ptTimeline, "편집 기준이며 상한이 아니다", "editing guidance, not a limit"},
		{ptTimeline, "복잡한 원인 분석은 더 쓸 수 있다", "a complex cause analysis may run longer"},
		{ptCurrent, "5~8줄", "5 to 8 lines"},
	})
	// A resume is recorded whether or not `## 현재` was stale.
	forbidAll(t, "seal/skills/seal/SKILL.md", "when it differed")
}

// the plan-change and block tables keep their names
// and columns; times carry a time zone, one row is one event, and the detail
// of an event that also has a timeline entry lives only in that entry.
func TestProgressTablesPointToTimeline(t *testing.T) {
	var rules []ptRule
	for _, p := range []string{ptPlan, ptBlocks} {
		rules = append(rules,
			ptRule{p, "`시각`은 시간대를 포함한다", "`시각` carries a time zone"},
			ptRule{p, "한 행은 사건 하나다", "one row is one event"},
			ptRule{p, "같은 사건을 타임라인에도 쓰면", "when the event also has a timeline entry"},
			ptRule{p, "상세 설명은 그 타임라인 항목에만", "the detail lives only in that entry"},
			ptRule{p, "한 줄 요약", "one-line summary"},
			ptRule{p, "그 항목의 시각으로 가리킨다", "points to it by the entry's time"},
		)
	}
	ptCheck(t, rules)
	sec := ptContractSec(t)
	requireIn(t, "bundle contract `## 계획 변경`", ptRow(t, "bundle contract", sec, "| `## 계획 변경` |"),
		"`시각`, `변경`, `이유`, `영향`, `다시 검토한 것`")
	requireIn(t, "bundle contract `## 막힘`", ptRow(t, "bundle contract", sec, "| `## 막힘` |"),
		"`시각`, `원인 종류`", "`내용`, `해소 조건`")
	ref := ptReferenceSec(t)
	requireIn(t, "Seal bundle formats `## 계획 변경`", ptRow(t, "Seal bundle formats", ref, "| `## 계획 변경` |"),
		"Table `시각 \\| 변경 \\| 이유 \\| 영향 \\| 다시 검토한 것`")
	requireIn(t, "Seal bundle formats `## 막힘`", ptRow(t, "Seal bundle formats", ref, "| `## 막힘` |"),
		"Table `시각 \\| 원인 종류 \\| 내용 \\| 해소 조건`")
	requireAll(t, ptTemplate, "## 계획 변경\n| 시각 | 변경 | 이유 | 영향 | 다시 검토한 것 |", "## 막힘\n| 시각 | 원인 종류 | 내용 | 해소 조건 |")
	// A line right after a table is a row of it; the template's table rules
	// stand apart from the tables.
	for _, p := range []string{ptPlan, ptBlocks} {
		if !strings.Contains(mdSection(t, ptTemplate, p), "| --- |\n\n<rows: ") {
			t.Errorf("PROGRESS template `%s` has no table rules apart from the table", p)
		}
	}
}

// at completion, `## 현재` says it is as of that
// completion record and where the later events (merge, review replies, checks
// after the merge) are found; it never writes them as work in progress.
func TestProgressCurrentAtCompletion(t *testing.T) {
	ptCheck(t, []ptRule{
		{ptCurrent, "완료 기록 기준", "as of that completion record"},
		{ptCurrent, "묶음의 마지막 commit 뒤", "past the bundle's last commit"},
		{ptCurrent, "병합", "merge"},
		{ptCurrent, "PR 답글", "review replies"},
		{ptCurrent, "병합 뒤 확인", "checks after the merge"},
		{ptCurrent, "어디서 보는지", "where to find"},
		{ptCurrent, "진행 중인 일처럼 쓰지 않는다", "never as work in progress"},
	})
}

// the Seal Skill has the executor keep `## 현재` and
// `## 타임라인` this way for goals it starts, resumes by restamping `## 현재`,
// lists the new contract tokens, and ships as a raised seal version.
func TestSealSkillKeepsTimeline(t *testing.T) {
	const skill = "seal/skills/seal/SKILL.md"
	text := read(t, skill)
	requireIn(t, skill, text,
		"`## 현재`", "stamped with its check time", "record head, seq and hash", "`## 타임라인`", "appended", "never rewritten",
		"as of that completion record", "restamp `## 현재`", "`재개`",
		"goals whose start record names seal 0.1.6 or later", "keeps the PROGRESS form it started with",
		"for a goal started with seal 0.1.6 or later, restamp")
	var tokens string
	for _, l := range strings.Split(text, "\n") {
		if strings.Contains(l, "Keep contract tokens exactly as written") {
			tokens = l
		}
	}
	requireIn(t, skill+" language tokens", tokens, "`## 타임라인`")
	requireIn(t, skill+" language tokens", tokens, ptKinds...)
	requireIn(t, skill+" language tokens", tokens, ptMarks...)
	if _, v := frontMatter(t, skill); rel011Newer(v, "0.1.5") <= 0 {
		t.Errorf("seal version %s is not raised past 0.1.5", v)
	}
}
