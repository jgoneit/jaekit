package main

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jgoneit/jaekit/internal/record"
	"github.com/jgoneit/jaekit/internal/status"
)

// ha report renders a person's view of one goal (run-record.md §1.3): five
// labelled summary lines, then a stamp for PROGRESS `## 현재`, the evidence
// per condition, the completion coverage and where the unmet conditions'
// output is. It reads what ha status reads, through the same computation,
// and writes nothing. It never reads the current time: the stamp's time is
// the one stored in the record's head line.

// reportWords are the labels and sentences of one language. Reason codes,
// classes, states, condition IDs, kinds, seqs, hashes, commits, cause codes,
// commands, `none`, `no stored output` and `local` are the same in both.
type reportWords struct {
	labels   [5]string
	headings [4]string

	changed, notStarted, cannotCompare string
	checked                            string
	// When Git cannot compare the base with HEAD, the summary says so and
	// nothing else is computed (the user's decision on the impl review).
	notComputed, limitsNotComputed, fetchBase string

	none, userDecisions, confirmed       string
	exempt, weakened, commands, findings string

	limits, optional, budget, notCovered string

	start, startLint, resolve, ask, done string
	finished, blocked, budgetDecision    string

	stamp, stampNone, stampUnreadable, stampCompletion string

	source, sourceCompletion, table, yes, no string

	covered, notCoveredHead, noCompletion string

	unmetNone string
	unmet     status.UnmetWords
	// stale translates ha status's reasons why a completion record no longer
	// holds, by their English prefix; English keeps them as they are.
	stale [][2]string
}

var reportLanguages = map[string]reportWords{
	"en": {
		labels:            [5]string{"Changed", "Checked", "To review", "Remaining limits", "Next step"},
		headings:          [4]string{"### Stamp for `## 현재`", "### Evidence per condition", "### Completion coverage", "### Output of unmet conditions"},
		changed:           "%d files and %d commits outside the bundle documents since base %s",
		notStarted:        "not started; there is no base commit yet",
		cannotCompare:     "Git cannot compare base %s with HEAD %s",
		notComputed:       "not computed; Git cannot compare base %s with HEAD %s",
		limitsNotComputed: "assurance local; check author: executor; the other limits were not computed",
		fetchBase:         "fetch the history that contains base %s (for example `git fetch --unshallow` in a shallow clone), then run `ha report %s` again",
		checked:           "%d of %d required conditions satisfied (%d by check records, %d by user confirmation); %d of %d change conditions have a fresh baseline",
		none:              "none",
		userDecisions:     "user decisions: ",
		confirmed:         "manual conditions satisfied by a quoted confirmation: ",
		exempt:            "test changes exempt by the executor's reason: ",
		weakened:          "test changes marked as weakening: ",
		commands:          "verification commands changed since start: ",
		findings:          "unresolved post-completion findings: %d",
		limits:            "assurance local; check author: executor",
		optional:          "; optional conditions not satisfied: ",
		budget:            "; budget exceeded: %d/%d runs, %d/%d seconds",
		notCovered:        "; HEAD is not covered by a completion record",
		start:             "run `ha start %s`",
		startLint:         " after fixing the lint problems (`ha lint %s`)",
		resolve:           "resolve %s (executor)",
		ask:               "ask the user about ",
		done:              "run `ha done %s` to record completion",
		finished:          "none; completion record seq %d holds and nothing is left to do",
		blocked:           "blocked since seq %d (cause: %s); the cause must be resolved before work continues",
		budgetDecision:    "the user must decide the budget; used %d/%d runs and %d/%d seconds",
		stamp:             "- record head: seq %d (%s), recorded at %s; status: %s",
		stampNone:         "- record head: none; status: %s",
		stampUnreadable:   "- record head: seq %d (%s), recorded at an unreadable time; status: %s",
		stampCompletion:   "; completion record: seq %d",
		source:            "Source: current state",
		sourceCompletion:  "Source: completion record seq %d",
		table:             "| ID | kind | required | satisfied | records |",
		yes:               "yes",
		no:                "no",
		covered:           "- completion record seq %d, commit %s; HEAD %s is covered",
		notCoveredHead:    "- latest completion record seq %d, commit %s; HEAD %s is not covered%s; the difference since that commit is not part of the completion",
		noCompletion:      "- completion record: none",
		unmetNone:         "- none",
		unmet:             status.EnglishUnmetWords,
	},
	"ko": {
		labels:            [5]string{"바뀐 것", "확인한 것", "직접 볼 것", "남은 한계", "다음 행동"},
		headings:          [4]string{"### `## 현재` 도장", "### 조건별 근거", "### 완료 범위", "### 미충족 조건의 원문 위치"},
		changed:           "base %[3]s 이후 실행 묶음 문서 밖에서 파일 %[1]d개, commit %[2]d개",
		notStarted:        "시작 전이라 base commit이 없음",
		cannotCompare:     "Git이 base %s와 HEAD %s를 비교할 수 없음",
		notComputed:       "계산하지 못함; Git이 base %s와 HEAD %s를 비교할 수 없음",
		limitsNotComputed: "보증 수준 local; 검사 작성자: 실행자; 나머지 한계는 계산하지 못함",
		fetchBase:         "base %s가 든 이력을 받고(예: 얕은 clone에서는 `git fetch --unshallow`) `ha report %s` 다시 실행",
		checked:           "필수 조건 %[2]d개 중 %[1]d개 충족(검사 기록 %[3]d개, 사용자 확인 %[4]d개); change 조건 %[6]d개 중 %[5]d개에 신선한 baseline",
		none:              "없음",
		userDecisions:     "사용자 결정: ",
		confirmed:         "인용 확인으로 충족한 manual 조건: ",
		exempt:            "실행자 근거로 면제된 시험 변경: ",
		weakened:          "약화 표시가 있는 시험 변경: ",
		commands:          "시작 뒤 바뀐 검증 명령: ",
		findings:          "해결되지 않은 완료 뒤 발견: %d개",
		limits:            "보증 수준 local; 검사 작성자: 실행자",
		optional:          "; 충족되지 않은 선택 조건: ",
		budget:            "; 예산 초과: 실행 %d/%d회, 경과 %d/%d초",
		notCovered:        "; 지금 HEAD를 덮는 완료 기록이 없음",
		start:             "`ha start %s` 실행",
		startLint:         " (먼저 lint 문제 해결: `ha lint %s`)",
		resolve:           "%s 해결 (executor)",
		ask:               "사용자에게 물을 것: ",
		done:              "`ha done %s`으로 완료 기록",
		finished:          "없음; 완료 기록 seq %d이 유효하고 남은 할 일이 없음",
		blocked:           "seq %d부터 막힘 (cause: %s); 원인을 해소해야 진행할 수 있음",
		budgetDecision:    "사용자의 예산 결정이 필요함; 실행 %d/%d회, 경과 %d/%d초 사용",
		stamp:             "- 기록 head: seq %d (%s), 기록 시각 %s; 상태: %s",
		stampNone:         "- 기록 head: none; 상태: %s",
		stampUnreadable:   "- 기록 head: seq %d (%s), 기록 시각 읽을 수 없음; 상태: %s",
		stampCompletion:   "; 완료 기록: seq %d",
		source:            "근거 출처: 지금 상태",
		sourceCompletion:  "근거 출처: 완료 기록 seq %d",
		table:             "| ID | 종류 | 필수 | 충족 | 근거 기록 |",
		yes:               "예",
		no:                "아니오",
		covered:           "- 완료 기록 seq %d, commit %s; 지금 HEAD %s를 덮음",
		notCoveredHead:    "- 가장 최근 완료 기록 seq %d, commit %s; 지금 HEAD %s를 덮지 않음%s; 그 commit 뒤의 차이는 완료에 들지 않음",
		noCompletion:      "- 완료 기록: none",
		unmetNone:         "- 없음",
		unmet:             status.UnmetWords{Output: "원문", NoFreshRecord: "신선한 기록 없음"},
		stale: [][2]string{
			{"record chain broken", "기록 연결이 끊김"},
			{"stored records are invalid", "저장된 기록이 무효함"},
			{"working tree has uncommitted changes outside the bundle documents", "실행 묶음 문서 밖에 커밋하지 않은 변경이 있음"},
			{"that commit cannot be compared with HEAD", "그 commit을 HEAD와 비교할 수 없음"},
			{"code outside the bundle documents changed since that commit", "그 commit 뒤 실행 묶음 문서 밖의 코드가 바뀜"},
			{"goal documents changed", "목표 문서가 바뀜"},
			{"required command changed: ", "필수 조건의 명령이 바뀜: "},
			{"reopened at seq ", "재작업 요청 seq "},
			{"required conditions are not all satisfied", "필수 조건이 모두 충족되지는 않음"},
		},
	},
}

func (c *cli) report(in []string) int {
	a, err := parseArgs(in, []string{"lang"}, nil)
	if err != nil {
		return c.fail(exitUsage, "%v", err)
	}
	lang, given := a.vals["lang"]
	if !given {
		lang = "en"
	}
	words, ok := reportLanguages[lang]
	if !ok {
		return c.fail(exitUsage, "--lang must be ko or en")
	}
	if len(a.pos) != 1 {
		return c.fail(exitUsage, "usage: ha report <goal> [--lang ko|en]")
	}
	w, code := c.openOrFail(a, 1)
	if w == nil {
		return code
	}
	lines, bad, err := w.read()
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	rep, err := status.Compute(status.Input{Repo: w.repo, Goal: w.goal, Bundle: w.bundle, Lines: lines, Integrity: bad, HaVersion: version})
	var ur *status.UnsupportedRulesError
	if errors.As(err, &ur) {
		return c.fail(exitRules, "%v", err)
	}
	v := &reportView{w: w, rep: rep, lines: lines, words: words, goal: status.DisplayGoal(w.arg, w.goal.Dir)}
	if start := startLine(lines); start != nil {
		head, cmpErr := v.compare(start)
		if cmpErr != nil {
			if _, err := io.WriteString(c.out, v.uncompared(start.BaseCommit, head)); err != nil {
				return c.fail(exitInternal, "write report: %v", err)
			}
			return c.fail(exitInternal, "Git cannot compare base %s with HEAD %s: %v", short12(start.BaseCommit), short12(head), cmpErr)
		}
	}
	if err != nil {
		return c.fail(exitInternal, "%v", err)
	}
	if _, err := io.WriteString(c.out, v.render()); err != nil {
		return c.fail(exitInternal, "write report: %v", err)
	}
	return exitOK
}

// compare reads what `Changed` reports: the files that ha status compares
// between the base and HEAD, and the commits that change them. Every reading
// of that range goes through here, so when Git cannot compare the two, the
// whole summary says so (uncompared) instead of a partial report.
func (v *reportView) compare(start *record.Line) (head string, err error) {
	if head, err = v.w.repo.Head(); err != nil {
		return "", err
	}
	excl, err := v.w.excludes(start.Rules)
	if err != nil {
		return head, err
	}
	files, err := v.w.repo.Changes(start.BaseCommit, head, excl)
	if err != nil {
		return head, err
	}
	if v.commits, err = v.w.repo.CommitsChanging(start.BaseCommit, head, excl); err != nil {
		return head, err
	}
	v.files, v.compared = len(files), true
	return head, nil
}

// uncompared is the five-line summary when Git cannot compare base with HEAD:
// the other lines were not computed, and the next step is to fetch the base.
func (v *reportView) uncompared(base, head string) string {
	b, h := short12(base), short12(head)
	notComputed := fmt.Sprintf(v.words.notComputed, b, h)
	summary := [5]string{fmt.Sprintf(v.words.cannotCompare, b, h), notComputed, notComputed,
		v.words.limitsNotComputed, fmt.Sprintf(v.words.fetchBase, b, v.goal)}
	var out strings.Builder
	for i, line := range summary {
		fmt.Fprintf(&out, "%s: %s\n", v.words.labels[i], oneLine.Replace(line))
	}
	return out.String()
}

type reportView struct {
	w     *workspace
	rep   *status.Report
	lines []record.Line
	words reportWords
	goal  string
	// source holds the evidence of each condition: the stored criteria of a
	// valid completion record, or the current computation (PLAN R1). Every
	// part of the report that names satisfied conditions reads it.
	source    []evidenceRow
	sourceSeq int
	// files and commits are what compare read; compared is false before start.
	files, commits int
	compared       bool
}

type evidenceRow struct {
	id, kind            string
	required, satisfied bool
	records             []int
}

func (v *reportView) render() string {
	v.evidence()
	var b strings.Builder
	summary := [5]string{v.changed(), v.checked(), v.toReview(), v.limits(), v.next()}
	for i, line := range summary {
		fmt.Fprintf(&b, "%s: %s\n", v.words.labels[i], oneLine.Replace(line))
	}
	sections := [4][]string{{v.stamp()}, v.table(), {v.coverage()}, v.unmet()}
	for i, body := range sections {
		fmt.Fprintf(&b, "\n%s\n\n", v.words.headings[i])
		for _, line := range body {
			fmt.Fprintf(&b, "%s\n", line)
		}
	}
	return b.String()
}

func (v *reportView) evidence() {
	required := func(id string) bool {
		c := v.w.goal.Criterion(id)
		return c != nil && !c.Optional
	}
	if seq := v.rep.CompletionRecord; seq != nil {
		for _, l := range v.lines {
			if l.Seq == *seq && l.Kind == "done" {
				v.sourceSeq = *seq
				for _, c := range l.Criteria {
					v.source = append(v.source, evidenceRow{c.ID, c.Kind, required(c.ID), c.Satisfied, c.Records})
				}
				return
			}
		}
	}
	for _, c := range v.rep.Criteria {
		v.source = append(v.source, evidenceRow{c.ID, c.Kind, c.Required, c.Satisfied, c.Records})
	}
}

func short12(s string) string {
	if len(s) > 12 {
		return s[:12]
	}
	return s
}

// changed reports compare's counts: the files and the non-merge commits since
// the base that change a path outside the bundle documents, with ha status's
// exclusions.
func (v *reportView) changed() string {
	start := startLine(v.lines)
	if start == nil || !v.compared {
		return v.words.notStarted
	}
	return fmt.Sprintf(v.words.changed, v.files, v.commits, short12(start.BaseCommit))
}

func (v *reportView) checked() string {
	var required, satisfied, byCheck, byUser, change, fresh int
	for _, c := range v.source {
		if !c.required {
			continue
		}
		required++
		if c.satisfied {
			satisfied++
			if c.kind == "manual" {
				byUser++
			} else {
				byCheck++
			}
		}
	}
	for _, c := range v.rep.Criteria {
		if c.Kind == "change" {
			change++
			if v.rep.FreshBaseline(c.ID) {
				fresh++
			}
		}
	}
	return fmt.Sprintf(v.words.checked, satisfied, required, byCheck, byUser, fresh, change)
}

// reasonList names each reason with its target: the condition ID, or for a
// goal-level reason the tasks or paths it is about (PLAN R4, R6).
func reasonList(reasons []status.Reason) string {
	var parts []string
	for _, r := range reasons {
		switch targets := r.Targets(); {
		case r.Criterion != "":
			parts = append(parts, fmt.Sprintf("%s `%s`", r.Criterion, r.Code))
		case len(targets) > 0:
			parts = append(parts, fmt.Sprintf("`%s` (%s)", r.Code, strings.Join(targets, ", ")))
		default:
			parts = append(parts, fmt.Sprintf("`%s`", r.Code))
		}
	}
	return strings.Join(parts, ", ")
}

// oneLine keeps a summary line on one line: a line break inside a goal path
// or another value is written as the two characters \n or \r.
var oneLine = strings.NewReplacer("\r", `\r`, "\n", `\n`)

func (v *reportView) userReasons() []status.Reason {
	var out []status.Reason
	for _, r := range v.rep.Reasons {
		if r.Class == status.ClassUser {
			out = append(out, r)
		}
	}
	return out
}

func (v *reportView) toReview() string {
	var items []string
	if user := v.userReasons(); len(user) > 0 {
		items = append(items, v.words.userDecisions+reasonList(user))
	}
	var manual []string
	for _, c := range v.source {
		if c.kind == "manual" && c.satisfied {
			if seq, ok := v.rep.Confirmation(c.id); ok {
				manual = append(manual, fmt.Sprintf("%s (seq %d)", c.id, seq))
			} else {
				manual = append(manual, c.id)
			}
		}
	}
	if len(manual) > 0 {
		items = append(items, v.words.confirmed+strings.Join(manual, ", "))
	}
	var exempt, weakened []string
	for _, t := range v.rep.TestChanges {
		if t.Rationale == nil {
			continue
		}
		entry := fmt.Sprintf("%s (seq %d)", t.Path, t.Rationale.Seq)
		if t.Exempt {
			exempt = append(exempt, entry)
		}
		if t.Rationale.Weakened {
			weakened = append(weakened, entry)
		}
	}
	if len(exempt) > 0 {
		items = append(items, v.words.exempt+strings.Join(exempt, ", "))
	}
	if len(weakened) > 0 {
		items = append(items, v.words.weakened+strings.Join(weakened, ", "))
	}
	var commands []string
	for _, ch := range v.rep.Changes.CommandChanged {
		commands = append(commands, fmt.Sprintf("%s (seq %d)", ch.Criterion, ch.Seq))
	}
	if len(commands) > 0 {
		items = append(items, v.words.commands+strings.Join(commands, ", "))
	}
	if f := v.rep.Findings; f != nil && f.Unresolved > 0 {
		items = append(items, fmt.Sprintf(v.words.findings, f.Unresolved))
	}
	if len(items) == 0 {
		return v.words.none
	}
	return strings.Join(items, "; ")
}

func (v *reportView) limits() string {
	line := v.words.limits
	var optional []string
	for _, c := range v.source {
		if !c.required && !c.satisfied {
			optional = append(optional, c.id)
		}
	}
	if len(optional) > 0 {
		line += v.words.optional + strings.Join(optional, ", ")
	}
	if bd := v.rep.Budget; bd != nil && bd.Exceeded {
		line += fmt.Sprintf(v.words.budget, bd.Runs, bd.RunsLimit, bd.ElapsedSeconds, bd.ElapsedLimitSeconds)
	}
	if v.rep.CompletionRecord == nil {
		line += v.words.notCovered
	}
	return line
}

func (v *reportView) next() string {
	rep := v.rep
	switch {
	case startLine(v.lines) == nil:
		line := fmt.Sprintf(v.words.start, v.goal)
		if len(rep.Lint) > 0 {
			line += fmt.Sprintf(v.words.startLint, v.goal)
		}
		return line
	case rep.Status == status.Incomplete:
		for _, r := range rep.Reasons {
			if r.Class == status.ClassExecutor {
				return fmt.Sprintf(v.words.resolve, reasonList([]status.Reason{r}))
			}
		}
	case rep.Status == status.NeedsUser:
		return v.words.ask + reasonList(v.userReasons())
	case rep.Status == status.Complete && rep.CompletionRecord == nil:
		return fmt.Sprintf(v.words.done, v.goal)
	case rep.Status == status.Complete:
		return fmt.Sprintf(v.words.finished, *rep.CompletionRecord)
	case rep.Status == status.Blocked:
		var last *record.Line
		for i := range v.lines {
			if l := &v.lines[i]; l.Kind == "note" && (l.Note == "block" || l.Note == "unblock") {
				last = l
			}
		}
		if last != nil {
			return fmt.Sprintf(v.words.blocked, last.Seq, last.Cause)
		}
	case rep.Status == status.BudgetExhausted && rep.Budget != nil:
		bd := rep.Budget
		return fmt.Sprintf(v.words.budgetDecision, bd.Runs, bd.RunsLimit, bd.ElapsedSeconds, bd.ElapsedLimitSeconds)
	}
	return reasonList(rep.Reasons)
}

func (v *reportView) stamp() string {
	rep := v.rep
	if rep.RecordHead == nil || len(v.lines) == 0 {
		return fmt.Sprintf(v.words.stampNone, rep.Status)
	}
	head := rep.RecordHead
	at := v.lines[len(v.lines)-1].At
	var line string
	if _, err := time.Parse(time.RFC3339, at); err != nil {
		line = fmt.Sprintf(v.words.stampUnreadable, head.Seq, short12(head.SHA256), rep.Status)
	} else {
		line = fmt.Sprintf(v.words.stamp, head.Seq, short12(head.SHA256), at, rep.Status)
	}
	if rep.CompletionRecord != nil {
		line += fmt.Sprintf(v.words.stampCompletion, *rep.CompletionRecord)
	}
	return line
}

func (v *reportView) table() []string {
	out := []string{v.words.source}
	if v.sourceSeq != 0 {
		out[0] = fmt.Sprintf(v.words.sourceCompletion, v.sourceSeq)
	}
	out = append(out, v.words.table, "| --- | --- | --- | --- | --- |")
	yesNo := func(b bool) string {
		if b {
			return v.words.yes
		}
		return v.words.no
	}
	for _, c := range v.source {
		records := "—"
		if len(c.records) > 0 {
			parts := make([]string, len(c.records))
			for i, s := range c.records {
				parts[i] = fmt.Sprint(s)
			}
			records = strings.Join(parts, ", ")
		}
		kind := c.kind
		if kind == "" {
			kind = "—"
		}
		out = append(out, fmt.Sprintf("| %s | %s | %s | %s | %s |", c.id, kind, yesNo(c.required), yesNo(c.satisfied), records))
	}
	return out
}

func (v *reportView) coverage() string {
	rep := v.rep
	head := short12(rep.Commit)
	if seq := rep.CompletionRecord; seq != nil {
		for _, l := range v.lines {
			if l.Seq == *seq {
				return fmt.Sprintf(v.words.covered, *seq, short12(l.Commit), head)
			}
		}
	}
	if d := rep.Done(); d != nil && d.Completion != nil {
		var reasons []string
		for _, r := range d.Completion.Reasons {
			reasons = append(reasons, v.staleReason(r))
		}
		why := ""
		if len(reasons) > 0 {
			why = ": " + strings.Join(reasons, "; ")
		}
		return fmt.Sprintf(v.words.notCoveredHead, d.Completion.Seq, short12(d.Completion.Commit), head, why)
	}
	return v.words.noCompletion
}

func (v *reportView) staleReason(reason string) string {
	for _, pair := range v.words.stale {
		if strings.HasPrefix(reason, pair[0]) {
			return pair[1] + strings.TrimPrefix(reason, pair[0])
		}
	}
	return reason
}

func (v *reportView) unmet() []string {
	lines := v.rep.UnmetConditionLines(v.goal, v.words.unmet)
	if len(lines) == 0 {
		return []string{v.words.unmetNone}
	}
	return lines
}
