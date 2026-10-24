package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// The Markdown form of ha cost shows the same values as its JSON form. The
// language changes only labels and sentences; condition IDs, seqs, results,
// stages, classes, dispositions, reviewer names and statuses, file names and
// quotes are the same in both (run-record.md §1.7).

type costWords struct {
	title                                               string
	headings                                            [6]string
	tier, none, rules, notStarted, criteria             string
	changed, cannotCompare, completion, notComputed     string
	changedNone, waitsNone, growthLines                 string
	stateNotComputed, waitsNotReported, notStartedShort string
	growthMissesNamed                                   string
	noRecord                                            string
	runs, runTime, perTarget, checks, baselines         string
	rerecorded, rerecord, changedInputs, sameInputs     string
	inputs                                              map[string]string
	notes, span, spanUnknown, budget, budgetNoWaits     string
	budgetNone                                          string
	bySource, bySourceNoSeries, waits, wait, inputs2    string
	questions, undisposed, extensions                   string
	extension                                           string
	waivers, waiver, listNone                           string
	notGiven, series, gate, gateRecorded, skipped       string
	stages, stage, stageUnknown, roundsLabel, round     string
	roundUnfinished, counted, rerun, countedUnknown     string
	reviewer, attempts, fallbackUsed, fallbackNot       string
	notRecorded, reviewerTime, reviewerTotal            string
	reviewerUnrecorded                                  string
	class, dispositions, growth, growthTotal            string
	growthSeries, growthNone, growthUnknown             string
	growthWhy                                           map[string]string
	tokens                                              string
}

var costLanguages = map[string]costWords{
	"en": {
		title:              "# Cost summary: %s",
		headings:           [6]string{"## Goal kind and size", "## Verification", "## Questions and waits", "## Review rounds", "## Findings and fixes", "## Tokens"},
		tier:               "- assurance tier: %s",
		none:               "none",
		rules:              "- rules: %s",
		notStarted:         "none; the goal has not started",
		criteria:           "- criteria: %d required, %d optional",
		changed:            "- changed: %d files and %d commits outside the bundle documents since base %s",
		cannotCompare:      "- changed: Git cannot compare base %s with HEAD",
		changedNone:        "- changed: none; the goal has not started",
		stateNotComputed:   "not computed; Git cannot read the commits the record names",
		waitsNotReported:   "- waits: not recorded before run-rules/4",
		notStartedShort:    "not started",
		growthMissesNamed:  "not recorded: %s does not list every fixes round of the series",
		waitsNone:          "- waits: none",
		growthLines:        "check %d, other %d (%s)",
		completion:         "- completion record: %s",
		notComputed:        "not computed; Git cannot compare base %s with HEAD",
		noRecord:           "- no run record: the goal has not started",
		runs:               "- runs: check %d (%s); baseline %d (%s)",
		runTime:            "- run time: %d ms in all (check %d ms, baseline %d ms); runs can overlap, so this is not the goal's elapsed time",
		perTarget:          "- runs per target: %s",
		checks:             "check %d",
		baselines:          "baseline %d",
		rerecorded:         "- re-recorded runs:",
		rerecord:           "  - %s %s seq %d after seq %d: %s",
		changedInputs:      "%s changed",
		sameInputs:         "no input changed; previous result %s",
		inputs:             map[string]string{"code": "code", "goal_documents": "goal documents", "command": "command", "overlay": "overlaid check files"},
		notes:              "- test changes %s; recoveries %s; recovery allowances %s; reopens %d",
		span:               "- record: seq %d at %s to seq %d at %s, %d seconds",
		spanUnknown:        "- record: seq %d at %s to seq %d at %s, seconds not recorded",
		budget:             "- budget window from seq %d, as ha status shows it: span %d seconds, reported waits %d seconds, elapsed %d of %d seconds; runs %d of %d",
		budgetNoWaits:      "- budget window from seq %d, as ha status shows it: elapsed %d of %d seconds, waits are not reported before run-rules/4; runs %d of %d",
		budgetNone:         "- budget window: not computed from the record",
		bySource:           "- questions by source: waits %s, requirement-questions %d, cap extensions %d, reviewer waivers %d",
		bySourceNoSeries:   "- questions by source: waits %s; requirement-questions, cap extensions and reviewer waivers not given (no --series)",
		waits:              "- waits, each as ha's budget counts it (%d seconds in all, overlaps counted once):",
		wait:               "  - seq %d, %d seconds: %s",
		inputs2:            "- input notes %s; confirm notes %s",
		questions:          "- requirement-questions:",
		undisposed:         "no disposition",
		extensions:         "- cap extensions:",
		extension:          "  - %s: %s +%d: %s",
		waivers:            "- reviewer waivers:",
		waiver:             "  - %s %s %s: %s",
		listNone:           " none",
		notGiven:           "- not given: no review series was named with --series",
		series:             "- series %s: %d rounds, %d dry runs not counted",
		gate:               "  - review-gate/v1 file %s: state %s, %s",
		gateRecorded:       "recorded at seq %d",
		skipped:            "  - skipped %s: %s",
		stages:             "- rounds by stage: %s",
		stage:              "%s %d counted, %d reruns",
		stageUnknown:       ", %d not recorded",
		roundsLabel:        "- rounds:",
		round:              "  - %s %s (%s, sequence %d, %s): %s",
		roundUnfinished:    "not finished",
		counted:            "counted",
		rerun:              "rerun of %s",
		countedUnknown:     "counting not recorded",
		reviewer:           "    - %s: %s, %s, %s, %s",
		attempts:           "attempts %d",
		fallbackUsed:       "fallback used",
		fallbackNot:        "no fallback",
		notRecorded:        "not recorded",
		reviewerTime:       "- reviewer time:",
		reviewerTotal:      "  - %s: %s seconds in %d rounds",
		reviewerUnrecorded: ", %d rounds without a recorded time",
		class:              "- %s: %d (check paths %d, other %d, unreadable location %d, location not recorded %d)",
		dispositions:       "  - fix %d, reject %d, defer %d, automatic defer %d, duplicate %d, answer %d, no disposition %d",
		growth:             "- lines added by fixes rounds: %s",
		growthTotal:        "check %d, other %d in all",
		growthSeries:       "  - %s: %s",
		growthNone:         "0 (no fixes round)",
		growthUnknown:      "not recorded in all",
		growthWhy: map[string]string{growthNoGate: "not recorded: the series has no review-gate/v1 file",
			growthMisses: "not recorded: no review-gate/v1 file of the series lists every fixes round", growthNotInGate: "not recorded: %s has no growth"},
		tokens: "- new input, cached input and output tokens: not recorded",
	},
	"ko": {
		title:              "# 비용 요약: %s",
		headings:           [6]string{"## 목표의 종류와 크기", "## 검증", "## 질문과 대기", "## 검토 라운드", "## 지적과 수정", "## 토큰"},
		tier:               "- 등급: %s",
		none:               "없음",
		rules:              "- 규칙: %s",
		notStarted:         "없음; 시작하지 않은 목표",
		criteria:           "- 조건: 필수 %d개, 선택 %d개",
		changed:            "- 바뀐 것: base %[3]s 이후 실행 묶음 문서 밖에서 파일 %[1]d개, commit %[2]d개",
		cannotCompare:      "- 바뀐 것: Git이 base %s와 HEAD를 비교할 수 없음",
		changedNone:        "- 바뀐 것: 없음; 시작하지 않은 목표",
		stateNotComputed:   "계산하지 못함; Git이 기록이 가리키는 commit을 읽을 수 없음",
		waitsNotReported:   "- 대기: run-rules/4 전에는 기록 없음",
		notStartedShort:    "시작 전",
		growthMissesNamed:  "기록 없음: %s가 series의 fixes 라운드를 모두 나열하지 않음",
		waitsNone:          "- 대기: 없음",
		growthLines:        "검사 경로 %d, 그 밖 %d (%s)",
		completion:         "- 완료 기록: %s",
		notComputed:        "계산하지 못함; Git이 base %s와 HEAD를 비교할 수 없음",
		noRecord:           "- 실행 기록 없음: 시작하지 않은 목표",
		runs:               "- 실행: check %d회(%s); baseline %d회(%s)",
		runTime:            "- 실행 시간: 모두 %d ms(check %d ms, baseline %d ms); 실행은 겹칠 수 있어 목표의 경과 시간이 아님",
		perTarget:          "- 대상별 실행: %s",
		checks:             "check %d회",
		baselines:          "baseline %d회",
		rerecorded:         "- 다시 기록한 실행:",
		rerecord:           "  - %s %s seq %d (앞 기록 seq %d): %s",
		changedInputs:      "%s 바뀜",
		sameInputs:         "바뀐 입력 없음; 앞 기록의 결과 %s",
		inputs:             map[string]string{"code": "코드", "goal_documents": "목표 문서", "command": "검증 명령", "overlay": "덧씌운 검사 파일"},
		notes:              "- 시험 변경 %s; 오류 회복 %s; 회복 허용 %s; 재작업 요청 %d개",
		span:               "- 기록: seq %d(%s)부터 seq %d(%s)까지 %d초",
		spanUnknown:        "- 기록: seq %d(%s)부터 seq %d(%s)까지, 초는 기록 없음",
		budget:             "- 예산 창(seq %d부터, ha status와 같음): 구간 %d초, 신고한 대기 %d초, 경과 %d초/%d초; 실행 %d회/%d회",
		budgetNoWaits:      "- 예산 창(seq %d부터, ha status와 같음): 경과 %d초/%d초, run-rules/4 전에는 대기를 신고하지 않음; 실행 %d회/%d회",
		budgetNone:         "- 예산 창: 기록에서 계산하지 못함",
		bySource:           "- 출처별 질문: 대기 %s, requirement-question %d개, 상한 연장 %d개, 리뷰어 waiver %d개",
		bySourceNoSeries:   "- 출처별 질문: 대기 %s; requirement-question·상한 연장·리뷰어 waiver는 주지 않음(--series 없음)",
		waits:              "- 대기, 각각 ha의 예산 계산대로(모두 %d초, 겹친 대기는 한 번):",
		wait:               "  - seq %d, %d초: %s",
		inputs2:            "- input note %s개; confirm note %s개",
		questions:          "- requirement-question:",
		undisposed:         "처리 없음",
		extensions:         "- 상한 연장:",
		extension:          "  - %s: %s +%d: %s",
		waivers:            "- 리뷰어 waiver:",
		waiver:             "  - %s %s %s: %s",
		listNone:           " 없음",
		notGiven:           "- 주지 않음: --series로 검토 series를 가리키지 않음",
		series:             "- series %s: 라운드 %d개, 세지 않은 dry run %d개",
		gate:               "  - review-gate/v1 파일 %s: 상태 %s, %s",
		gateRecorded:       "seq %d에 기록됨",
		skipped:            "  - 건너뜀 %s: %s",
		stages:             "- 단계별 라운드: %s",
		stage:              "%s 상한에 셈 %d개, 재실행 %d개",
		stageUnknown:       ", 기록 없음 %d개",
		roundsLabel:        "- 라운드:",
		round:              "  - %s %s (%s, sequence %d, %s): %s",
		roundUnfinished:    "끝나지 않음",
		counted:            "상한에 셈",
		rerun:              "%s의 재실행",
		countedUnknown:     "셈 여부 기록 없음",
		reviewer:           "    - %s: %s, %s, %s, %s",
		attempts:           "시도 %d회",
		fallbackUsed:       "대체 모델 사용",
		fallbackNot:        "대체 모델 사용 안 함",
		notRecorded:        "기록 없음",
		reviewerTime:       "- 리뷰어 시간:",
		reviewerTotal:      "  - %s: 라운드 %[3]d개에서 %[2]s초",
		reviewerUnrecorded: ", 시간 기록 없는 라운드 %d개",
		class:              "- %s: %d개(검사 경로 %d, 그 밖 %d, 읽을 수 없는 위치 %d, 위치 기록 없음 %d)",
		dispositions:       "  - fix %d, reject %d, defer %d, 자동 보류 %d, duplicate %d, answer %d, 처리 없음 %d",
		growth:             "- fixes 라운드가 더한 줄: %s",
		growthTotal:        "모두 검사 경로 %d, 그 밖 %d",
		growthSeries:       "  - %s: %s",
		growthNone:         "0(fixes 라운드 없음)",
		growthUnknown:      "모두는 기록 없음",
		growthWhy: map[string]string{growthNoGate: "기록 없음: series에 review-gate/v1 파일이 없음",
			growthMisses: "기록 없음: series의 fixes 라운드를 모두 나열한 review-gate/v1 파일이 없음", growthNotInGate: "기록 없음: %s에 growth가 없음"},
		tokens: "- 새 입력·캐시 입력·출력 토큰: 기록 없음",
	},
}

func renderCost(d *costDoc, w costWords) string {
	var b strings.Builder
	line := func(format string, a ...any) {
		fmt.Fprintf(&b, format, a...)
		b.WriteByte('\n')
	}
	section := func(i int) {
		line("")
		line("%s", w.headings[i])
		line("")
	}
	line(w.title, d.Goal)

	section(0)
	k := d.Kind
	line(w.tier, costOr(k.Assurance, w.none))
	line(w.rules, costOr(k.Rules, w.notStarted))
	line(w.criteria, k.RequiredCriteria, k.OptionalCriteria)
	switch {
	case k.Base == nil:
		line("%s", w.changedNone)
	case k.ChangedFiles == nil:
		line(w.cannotCompare, short12(*k.Base))
	default:
		line(w.changed, *k.ChangedFiles, *k.ChangedCommits, short12(*k.Base))
	}
	switch {
	case k.CompletionRecord != nil:
		line(w.completion, "seq "+strconv.Itoa(*k.CompletionRecord))
	case d.Unavailable["kind.completion_record"] == costCannotCompare:
		line(w.completion, fmt.Sprintf(w.notComputed, short12(*k.Base)))
	case d.Unavailable["kind.completion_record"] == costNotComputed:
		line(w.completion, w.stateNotComputed)
	default:
		line(w.completion, w.none)
	}

	section(1)
	if v := d.Verification; v == nil {
		line("%s", w.noRecord)
	} else {
		line(w.runs, costCount(v.Runs["check"]), costResults(v.Runs["check"]), costCount(v.Runs["baseline"]), costResults(v.Runs["baseline"]))
		line(w.runTime, v.DurationMS.Total, v.DurationMS.Check, v.DurationMS.Baseline)
		var targets []string
		for _, t := range v.Targets {
			targets = append(targets, fmt.Sprintf("%s (%s, %s)", t.Target, fmt.Sprintf(w.checks, t.Checks), fmt.Sprintf(w.baselines, t.Baselines)))
		}
		line(w.perTarget, costJoin(targets, w.none))
		if len(v.Rerecords) == 0 {
			line("%s%s", w.rerecorded, w.listNone)
		} else {
			line("%s", w.rerecorded)
			for _, r := range v.Rerecords {
				why := fmt.Sprintf(w.sameInputs, r.PreviousResult)
				if len(r.Changed) > 0 {
					names := make([]string, len(r.Changed))
					for i, c := range r.Changed {
						names[i] = w.inputs[c]
					}
					why = fmt.Sprintf(w.changedInputs, strings.Join(names, ", "))
				}
				line(w.rerecord, r.Target, r.Kind, r.Seq, r.PreviousSeq, why)
			}
		}
		line(w.notes, costIntText(v.Notes.TestChange, w.notRecorded), costIntText(v.Notes.Recover, w.notRecorded),
			costIntText(v.Notes.AllowRecovery, w.notRecorded), v.Notes.Reopen)
		if v.Span.Seconds != nil {
			line(w.span, v.Span.FirstSeq, v.Span.FirstAt, v.Span.LastSeq, v.Span.LastAt, *v.Span.Seconds)
		} else {
			line(w.spanUnknown, v.Span.FirstSeq, v.Span.FirstAt, v.Span.LastSeq, v.Span.LastAt)
		}
		switch bd := v.Budget; {
		case bd == nil:
			line("%s", w.budgetNone)
		case bd.SpanSeconds != nil && bd.ReportedWaitSeconds != nil:
			line(w.budget, bd.WindowStart, *bd.SpanSeconds, *bd.ReportedWaitSeconds, bd.ElapsedSeconds, bd.ElapsedLimitSeconds, bd.Runs, bd.RunsLimit)
		default:
			line(w.budgetNoWaits, bd.WindowStart, bd.ElapsedSeconds, bd.ElapsedLimitSeconds, bd.Runs, bd.RunsLimit)
		}
	}

	section(2)
	q := d.Questions
	waits := w.notRecorded
	switch {
	case q.BySource.Wait != nil:
		waits = strconv.Itoa(*q.BySource.Wait)
	case q.notStarted:
		waits = w.notStartedShort
	}
	if q.sourcesGiven {
		line(w.bySource, waits, *q.BySource.RequirementQuestion, *q.BySource.Extension, *q.BySource.Waiver)
	} else {
		line(w.bySourceNoSeries, waits)
	}
	if !q.notStarted {
		switch {
		case q.Waits == nil:
			line("%s", w.waitsNotReported)
		case len(q.Waits) == 0:
			line("%s", w.waitsNone)
		default:
			line(w.waits, *q.ReportedWaitSeconds)
			for _, wt := range q.Waits {
				line(w.wait, wt.Seq, wt.Seconds, costOneLine(wt.Reason))
			}
		}
	}
	line(w.inputs2, costIntText(q.InputNotes, w.notRecorded), costIntText(q.ConfirmNotes, w.notRecorded))
	if q.sourcesGiven {
		costList(line, w.questions, w.listNone, len(q.RequirementQuestion), func(i int) {
			rq := q.RequirementQuestion[i]
			answer := w.undisposed
			if rq.Disposition != nil {
				answer = *rq.Disposition
				if rq.UserQuote != nil {
					answer += ": " + costOneLine(*rq.UserQuote)
				}
			}
			line("  - %s %s %s: %s", rq.Series, rq.Round, rq.Finding, answer)
		})
		costList(line, w.extensions, w.listNone, len(q.Extensions), func(i int) {
			e := q.Extensions[i]
			line(w.extension, e.Series, e.Stage, e.By, costOneLine(e.UserQuote))
		})
		costList(line, w.waivers, w.listNone, len(q.Waivers), func(i int) {
			wv := q.Waivers[i]
			line(w.waiver, wv.Series, wv.Round, wv.Reviewer, costOneLine(wv.UserQuote))
		})
	}

	section(3)
	if r := d.Review; r == nil {
		line("%s", w.notGiven)
	} else {
		for _, s := range r.Series {
			line(w.series, s.Path, s.Rounds, s.DryRuns)
			for _, g := range s.Gates {
				recorded := w.notRecorded
				if g.RecordedSeq != nil {
					recorded = fmt.Sprintf(w.gateRecorded, *g.RecordedSeq)
				}
				line(w.gate, g.Name, costText(g.State, w.notRecorded), recorded)
			}
			for _, sk := range s.Skipped {
				line(w.skipped, sk.Name, sk.Why)
			}
		}
		var stages []string
		for _, st := range r.Stages {
			text := fmt.Sprintf(w.stage, st.Stage, st.Counted, st.Reruns)
			if st.CountedUnknown > 0 {
				text += fmt.Sprintf(w.stageUnknown, st.CountedUnknown)
			}
			stages = append(stages, text)
		}
		line(w.stages, strings.Join(stages, "; "))
		costList(line, w.roundsLabel, w.listNone, len(r.Rounds), func(i int) {
			rd := r.Rounds[i]
			counting := w.countedUnknown
			if rd.Counted != nil && *rd.Counted {
				counting = w.counted
			} else if rd.Counted != nil {
				counting = fmt.Sprintf(w.rerun, costJoin(rd.RerunOf, w.notRecorded))
			}
			span := w.roundUnfinished
			switch {
			case rd.Seconds != nil:
				span = fmt.Sprintf("%ds, %s – %s", *rd.Seconds, *rd.StartedAt, *rd.FinishedAt)
			case rd.FinishedAt != nil:
				span = w.notRecorded
			}
			line(w.round, rd.Series, rd.Round, rd.Stage, rd.Sequence, counting, span)
			for _, rv := range rd.Reviewers {
				elapsed := w.notRecorded
				if rv.ElapsedSeconds != nil {
					elapsed = costSeconds(*rv.ElapsedSeconds) + "s"
				}
				attempts := w.notRecorded
				if rv.Attempts != nil {
					attempts = fmt.Sprintf(w.attempts, *rv.Attempts)
				}
				fallback := w.notRecorded
				if rv.FallbackUsed != nil {
					fallback = w.fallbackNot
					if *rv.FallbackUsed {
						fallback = w.fallbackUsed
					}
				}
				line(w.reviewer, rv.Name, rv.Status, elapsed, attempts, fallback)
			}
		})
		costList(line, w.reviewerTime, w.listNone, len(r.ReviewerSeconds), func(i int) {
			t := r.ReviewerSeconds[i]
			text := fmt.Sprintf(w.reviewerTotal, t.Reviewer, costSeconds(t.Seconds), t.Rounds)
			if t.NotRecorded > 0 {
				text += fmt.Sprintf(w.reviewerUnrecorded, t.NotRecorded)
			}
			line("%s", text)
		})
	}

	section(4)
	if f := d.Findings; f == nil {
		line("%s", w.notGiven)
	} else {
		for _, c := range f.Classes {
			line(w.class, c.Class, c.Total, c.CheckPaths, c.Other, c.Unreadable, c.LocationNotRecorded)
			ds := c.Dispositions
			line(w.dispositions, ds.Fix, ds.Reject, ds.Defer, ds.AutoDefer, ds.Duplicate, ds.Answer, ds.None)
		}
		total := w.growthUnknown
		if f.GrowthTotal != nil {
			total = fmt.Sprintf(w.growthTotal, f.GrowthTotal.Check, f.GrowthTotal.Other)
		}
		line(w.growth, total)
		for _, g := range f.Growth {
			var text string
			switch {
			case g.Source == growthNoFixes:
				text = w.growthNone
			case g.Lines != nil:
				text = fmt.Sprintf(w.growthLines, g.Lines.Check, g.Lines.Other, *g.Gate)
			case g.Source == growthNotInGate:
				text = fmt.Sprintf(w.growthWhy[g.Source], *g.Gate)
			case g.Source == growthMisses && g.Gate != nil:
				text = fmt.Sprintf(w.growthMissesNamed, *g.Gate)
			default:
				text = w.growthWhy[g.Source]
			}
			line(w.growthSeries, g.Series, text)
		}
	}

	section(5)
	line("%s", w.tokens)
	return b.String()
}

func costList(line func(string, ...any), label, none string, n int, item func(int)) {
	if n == 0 {
		line("%s%s", label, none)
		return
	}
	line("%s", label)
	for i := 0; i < n; i++ {
		item(i)
	}
}

func costOr(s *string, none string) string {
	if s == nil {
		return none
	}
	return *s
}

func costText(s, none string) string {
	if s == "" {
		return none
	}
	return s
}

func costInt(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func costIntText(n *int, none string) string {
	if n == nil {
		return none
	}
	return strconv.Itoa(*n)
}

func costJoin(items []string, none string) string {
	if len(items) == 0 {
		return none
	}
	return strings.Join(items, ", ")
}

func costCount(results map[string]int) int {
	n := 0
	for _, c := range results {
		n += c
	}
	return n
}

// costResults lists result counts in name order.
func costResults(results map[string]int) string {
	names := make([]string, 0, len(results))
	for r := range results {
		names = append(names, r)
	}
	sort.Strings(names)
	parts := make([]string, len(names))
	for i, r := range names {
		parts[i] = fmt.Sprintf("%s %d", r, results[r])
	}
	if len(parts) == 0 {
		return "—"
	}
	return strings.Join(parts, ", ")
}

// costSeconds writes recorded seconds as Spar records them, without
// rounding, so the Markdown and JSON forms give the same value.
func costSeconds(s float64) string {
	return strconv.FormatFloat(s, 'f', -1, 64)
}

// costOneLine keeps a quote or reason on its line.
func costOneLine(s string) string { return oneLine.Replace(s) }
