# Contract: Decision history (`decision-history/v1`)

[한국어](decision-history.md) · **English**

> This is a translation of the [Korean original](decision-history.md). Where the two differ, the Korean original decides.

Decision history is an optional feature that preserves, in structured form, the `관측`, `원인`, `결정`, `조치`, `검증`, `막힘` and `재개` (observation, cause, decision, action, verification, block and resume) that an executor leaves while carrying out one goal. Events are the executor's claims. Core only checks and queries format, references, relations and the context at the time; it does not evaluate whether the content of a judgment is true. It uses no model, network, account, always-running process or search database.

Decision history is separate from the [run record](run-record.en.md). Writing or reading the history does not change `runs.jsonl`, and it does not change the results of run rules, completion judgment, exit meaning, approval, rework, budget, checks or [post-completion findings](findings.en.md). Existing commands do not access decision history. What the history says does not resolve a block or an unmet required condition.

## 1. Support and compatibility

An executable whose `capabilities` has `decision-history/v1` in `features` and `decision-history/v1` in `formats.decision_history` supports this feature ([capabilities](core-capabilities.en.md) §5). Release `v0.1.5` (`ha 0.1.5`) first provides this feature; `v0.1.4` (`ha 0.1.4`) does not have it. Before using the feature, check both entries in the executable actually selected, and do not infer support from the version number alone. Without the feature, write no history and leave the explanation in the existing Markdown. There is no automatic installation or upgrade.

Decision history is written only for a goal that has a valid start record. Without a start record, an add is refused (`not_started`) and no run is started. It can be written regardless of the start record's rules (`run-rules/1`, `/2`, `/3`, `/4`), and it does not change the stored rules.

The current source provides the same add, replay, correction, default/current, list and both forms of show on macOS and Linux arm64 and amd64 and on the [Windows x64 local candidate](../guides/WINDOWS.en.md). On Windows, ordinary checkouts and linked worktrees also share the history and lock of the same Git common directory. Windows ARM64 and x86, network/UNC repositories, and cross use of the same checkout from Windows and WSL are not included. [The developer CI in two PowerShell versions on Windows Server](../guides/CI.en.md) is an observation separate from Windows 11 user installation, agent use and a public Windows release.

## 2. Storage location and retention

```text
<git-common-dir>/ha/history/<start-sha256>/events.jsonl   사건 (한 줄에 한 사건)
<git-common-dir>/ha/history/<start-sha256>/lock           등록 잠금
<git-common-dir>/ha/history/<start-sha256>/last           마지막으로 저장한 사건의 순번과 줄 sha256
```

- `<git-common-dir>` is the Git directory that `git rev-parse --git-common-dir` reports, shared by every working tree of the same repository. In an ordinary checkout it is `.git`. The main checkout and linked worktrees use the same history and the same lock. The run record lock (`ha/lock`) in each working tree's Git directory does not protect history adds.
- `<start-sha256>` is the sha256 of the raw line of that run's start record. It does not use the goal path or a branch name, so characters allowed in paths, such as spaces and newlines, do not change the storage location. Histories of different start records and of different repositories do not mix. Copies of an execution bundle with the same raw start record share the same history.
- Core neither writes nor reads when a directory or file on this path is a symlink or is not a regular directory or file (`history_unreadable`). Windows junctions and reparse points fall under the same boundary, which applies to intermediate directories and to the event, last-event identification, temporary and lock paths. A file with a link count of two or more, which shares its content with another name, is refused with the same error. Queries also never show data beyond such a link as this repository's history. Queries do not create files or directories.
- A query that overlaps an add from another working tree reads the intact history from before or after that add. It waits while the add lock exists, and when the storage directory of the first add does not exist yet, it may return the absence from before the add. When the first add's lock appears while it reads without the lock, it reads again under the lock, so a line still being written is not reported as corruption.
- When the process holding the lock exits, the leftover lock file alone does not block later access permanently. An incomplete event line left by an interruption during a write is corruption, and it is not truncated and repaired automatically. A failure after the event is stored, or a failure where it cannot be confirmed whether it was stored, keeps the existing `write_uncertain` meaning. Windows file-sharing, permission and replacement refusals are not hidden as success.
- It is local data. Git does not track it, and it does not travel to a new clone or another machine. There is no automatic backup, sync, export/import, branch merge or corruption repair. Removing a linked worktree leaves the history in place, but the remaining history does not guarantee that goal documents or raw check output are preserved or restored. The instructions that remove only raw check output ([installation guide](../guides/INSTALL.en.md#uninstall)) remove only `ha/runs/` and leave the history. Actions outside the retention scope, such as deleting the whole Git directory, are not recovered.
- When the goal directory moves but the raw start record is the same, the same history is found. Queries then show the differences in goal path and document digest between then and now, and do not automatically accept the changed path or content as valid.

## 3. Commands

```text
ha history <goal> add --kind <종류> --summary <요약> [--status <주장 상태>] [--reason <이유>]
                      [--target <AC-n|EX-n|T001>]... [--ref <참조>]... [--checked-ref <참조>]...
                      [--builds-on <Dn>]... [--supersedes <Dn>[@<Dm>]]...
                      [--event-at <RFC 3339 시각>] [--request-id <식별자>]
ha history <goal> add --input <JSON 파일 | -> [--request-id <식별자>]
ha history <goal> [current|list] [--target <id>]... [--keyword <낱말>]...
                      [--since <커서>] [--page <커서>] [--limit <n>] [--format md|json]
ha history <goal> show <Dn> | --request-id <식별자> [--page <커서>] [--limit <n>] [--format md|json]
```

- A common event is added with one `add` call, without a separate input file. Values of a repeated flag keep the order given. In the evidence, the `--ref` values come first and the `--checked-ref` values follow; the order in which the two flags were interleaved is not preserved. Repeating a single-value flag is refused.
- Reference notation: `run:<seq>[:<sha256>]`, `file:<path>[#<section>]`, `commit:<sha>[@<repository>]`, `external:<reference>`. `--checked-ref` is a reference whose content the executor claims to have read.
- `--input` takes one JSON document like the one below (`-` is standard input). It is at most 256 KiB, and invalid UTF-8, duplicate keys, fields that differ only in case, unknown fields, trailing data and nesting deeper than 16 levels are refused. Values without a flag notation, such as an external reference's `version`, are also given in this format.

```json
{"request_id": "decide-1", "decision": {"kind": "결정", "summary": "빈 입력이면 0을 반환한다",
  "targets": ["T001"], "builds_on": ["D1"],
  "evidence": [{"type": "external", "ref": "https://example.invalid/review/1", "version": "rev-2", "checked": true}]}}
```

- When `--request-id` is omitted, Core creates a new ID (`auto-…`) and shows it in the receipt and in an uncertain-application error. To prevent duplicates on replay, give the same ID again.
- The default query is `current`, and the default format is Markdown. Markdown and JSON (`decision-history-view/v1`) carry the same events and meaning.

## 4. Events

| Information | Who writes it and what it means |
| --- | --- |
| `id`, `seq`, `prev` | Added by Core. `id` is `D<n>`, the sequence number within the history. `prev` is the sha256 of the previous raw line; for the first line it is `null`. This chain exposes edits and deletions of middle lines, and truncated lines, as corruption. Intact lines deleted from the end are exposed as corruption by comparison with the last event that `last` records, and the ID and request ID of a vanished event are never reused for other content. Deleting `last` together with the history, or rolling both back to an earlier state, is not exposed. When a write is interrupted right after the line is written, or updating `last` fails, `last` can fall one event behind. Queries accept this state, and meanwhile that last event vanishing from the end is not exposed. The next add first brings `last` up to date, and does not add if it cannot. It does not prevent forgery under the same user's permissions, so the assurance is `local` |
| `request_id` | Request ID. 1 to 128 characters of alphanumerics and `_.:/-`, starting with an ASCII alphanumeric. Unique within the history |
| `decision.kind`, `decision.status` | Kind and claimed status. See the table below |
| `decision.summary` | One-line summary. 1 to 1024 bytes |
| `decision.reason` | A reason, written only for events that need one. Newlines are allowed; at most 16 KiB |
| `decision.targets` | Related conditions (`AC-n`), extra checks (`EX-n`) and Tasks (`T001`). At add time, only IDs in the current goal documents and PLAN are accepted. It does not change the targets' status |
| `decision.evidence` | Evidence references. See section 5 below |
| `decision.builds_on`, `decision.supersedes` | Event relations. See section 6 below |
| `decision.event_at` | Optional. An RFC 3339 claim of the event time that the source knows. Omit it when unknown; the record time is not used in its place |
| `at` | The record time (UTC) that the add command read. Added by Core. When the add waited for another add's lock, it can be earlier than the actual storage and earlier than the previous event's `at`. `seq` sets the order of events |
| `context` | The context Core observed at add time: the start record (`start`), the run record head (seq and sha256 in `run_head`), the goal path, the HEAD commit, working tree cleanliness (by the goal's execution bundle exclusion paths), the goal document digest, and the observation for each piece of evidence (`evidence`) |

`decision` is stored exactly as submitted. The same `decision` with the same `request_id` is treated as a replay, and no new event is created even when the add time or the context at the time differs.

| Kind | Claimed status |
| --- | --- |
| `관측` (observation) | None |
| `원인` (cause) | `가설` (hypothesis), `확정` (confirmed), `기각` (rejected) |
| `결정` (decision) | None |
| `조치` (action) | `예정` (planned), `반영` (applied) |
| `검증` (verification) | `대기` (pending), `통과` (passed), `실패` (failed) |
| `막힘` (block) | None |
| `재개` (resume) | None |

The kinds are the same tokens as in the PROGRESS `타임라인` (timeline). A claimed status can be attached only to its kind, and it can be omitted. Omission means unmarked; no other status is inferred. A plan change is written as the content of a `결정`, and a correction is expressed as a relation, not a kind. Kinds and statuses are not work stages or a state-transition procedure. The executor decides in what order to write events and whether to combine small events into one. A combined event is mechanically distinguished only by its representative kind and that kind's claimed status; the rest of what happened goes in the summary and reason.

## 5. Evidence references

| `type` | Input | Observation at add time (`context.evidence[].state`) | Current comparison at query time |
| --- | --- | --- | --- |
| `run` | `seq`, optional `sha256` | `matched` when that line exists in the same goal's run record and the hash matches; the line's sha256 is kept in `version`. Refused when the line is missing or the hash differs | `same`, `different` or `unverifiable` against the same line of the current run record |
| `file` | Repository-relative `path`, optional `section` | `read` and the content sha256 (`version`) for a regular file inside the repository. No version is identified for `missing` (the path does not exist), `not_regular` (symlinks included), `outside`, `unreadable` (the path or a parent path cannot be checked because of permissions, a symlink loop and the like) or `too_large` (over 64 MiB) | `unchanged`, `changed`, `unavailable`, `unidentified` |
| `commit` | `commit` (7 to 64 hex digits), optional `repository` | `found` and the full id when found in this repository. `not_found` when absent (shallow clones included), `other_repository` for another repository. Nothing is fetched | The add-time observation as is |
| `external` | `ref` (URL, issue, review comment and so on), optional `version` | `not_checked`. A `version`, when present, is the executor's claim identifying a version | `not_checked` |

In both a new add and a detailed query, a file's version is identified from the content of a regular file inside the repository that was actually read safely. Even if the file or a parent path is replaced after the check, a file outside the repository, a symlink as the last path component or a non-regular file is not read as valid evidence. Core does not wait for the content or the writer of a FIFO and the like. The 64 MiB limit also applies to the actual read. A file of exactly 64 MiB can be identified, but when the limit is exceeded from the start or during the read, Core does only a limited extra read to determine the excess and records `too_large`. When a race prevents safe identification, no version is made up. Such observation failures do not refuse a valid event as a whole, and `too_large` differs from `limit_exceeded`, which refuses the input. An atomic copy of a file under concurrent modification is not guaranteed.

When an existing parent symlink or Windows junction of file evidence leads to a regular file inside the repository, and the identity and inside location of the actual target can be confirmed safely, it is observed as `read`. When it leads outside, it is `outside`; when the last path component is itself a link, it is `not_regular`. This allowance does not relax the refusal of symlinks and junctions/reparse points in the history storage area. When the OS prevents path replacement and the original target stays safe, it can be observed normally, but after an actual replacement, the check result for the earlier target is not applied to another file.

The relative path grammar of the input is the same on Windows. When a reference that passes that grammar is interpreted as a Windows device, named pipe, alternate data stream (ADS) or drive-relative path, it is not opened or waited on, and it is recorded as `unreadable` with no version identified. A missing ordinary file, a non-regular file and an outside path are distinguished by the existing states. This is a limit of file observation, not a refusal of a valid event as a whole. A reference that is invalid under the original input grammar is still refused, and valid macOS/Linux file names and references and observations already stored are not changed retroactively.

`show` shows the add-time observation as it is and computes the current file comparison separately. When there was a version then but it cannot be identified safely now, the result is `unavailable`; when there was no version from the start, it is `unidentified`. `current` and `list` do not read evidence file content again. A replay of the same request also returns the original receipt before reading any file again, and it does not update the observation from that time. The same query meaning applies to events already stored, and past observations are not repaired automatically.

For every reference, `checked` is the executor's claim that it read the content; when it is omitted, the content is unconfirmed. A reference does not certify the truth of content, a check pass, independent review, user approval or completion. Referencing a user confirmation line in the run record gives it no meaning beyond what that line already has. Core does not run, download or collect external material, and it does not copy document or log content into events. A link to the current file alone is not shown as confirmation of the content at the time. Even with the same commit, uncommitted changes mean it is not treated as the same code input.

Judgments made before the run starts are kept in PLAN, REVIEW and PROGRESS. Events after the start, the first event included, link PLAN and REVIEW sections (`file:<path>#<section>`) and those documents' versions at add time as evidence, connecting the flow to the judgments made before the start. Events from before the start, or past times, are not created retroactively in the history.

## 6. Event relations

| Relation | Meaning |
| --- | --- |
| `supersedes` | This event corrects, rejects or replaces an earlier judgment. The original event's content and evidence stay as they are |
| `builds_on` | An earlier event served as grounds for this judgment. An optional relation; it does not change the referenced event |

- Both relations point only to earlier events already stored in the same history. One event can point to up to 64 events with each relation. Self-references, events not yet stored or stored later, events in another history and nonexistent events are refused with `invalid_relation`. Because only earlier events are pointed to, no cycle can form.
- Relations are independent of kind. Not every event needs a relation, and no order or transition between kinds is enforced.
- A correction is added together with the last correction seen for that judgment: `--supersedes D1` if there was no correction, `--supersedes D1@D4` if `D4` was the last correction. When this differs from the last correction in the history read again inside the add lock, the add is refused with `correction_conflict` and that correction is reported. Nothing is overwritten on the grounds of event time or of being the last writer. Conflicts are judged against corrections in all run lineages. Several events using the same event as grounds, or an event used as grounds being corrected later, is not a conflict.

## 7. Run lineage and current judgments

**Run lineage.** A query takes the goal's current run record chain as its basis. A valid chain is one whose sequence numbers and raw-line hash links, as in [run record](run-record.en.md) §2 and §4.2, can be confirmed. When an event's `context.run_head` (the seq and sha256 of the run record head at add time) is included in the query's basis chain, it is the same lineage (`same`). When a different line is at the same seq, it is a different lineage (`different`). When it is past the basis head, or chain integrity cannot be confirmed (including broken links and wrong sequence numbers that fall under `record_integrity`), it is unverifiable (`unverifiable`). The optional `run` evidence reference is not used to judge lineage. The head at the time need not equal the current head; when the head advances through ordinary run record appends, it stays the same lineage. The same branch name or the same seq alone does not make the same lineage. The context recorded at add time is never rewritten. Core does not claim to have observed the run record and the whole history at one atomic point in time.

Working trees that share the same start record and run head, and whose run records have not yet diverged, are treated as the same lineage. An explicit `supersedes` then replaces the named judgment across working tree boundaries, and code and document differences are distinguished only by the context markers (`context.code`, `context.spec`).

**Current judgments.** They are not the single last event of the whole history. They are the result of first computing, within the history range of the query basis, the set of same-lineage events not replaced by a same-lineage `supersedes`, and then applying the target, keyword, change and page conditions. Corrections outside the filter also count toward replacement, so a replaced judgment does not reappear because of search conditions or page limits. When no current judgment matches the filter, the result can be a normal zero-item result.

- Events of another lineage are shown separately as mismatches, and unverifiable events as unverifiable (`other_lineage`); they are not merged into the set of current judgments. A correction made by such an event is not counted as replacing a judgment of the selected lineage either; only the existence of the correction is shown (`other_corrections`).
- Within the same lineage too, differences between then and now are shown for code (`same`, `changed`, `uncommitted`), goal documents (`same`, `changed`) and goal path (`same`, `moved`). The goal document comparison uses the goal digest of `ha status`, and this digest includes the document paths, so a moved goal is shown as `changed` along with `moved` even when the content is the same, and the moved documents are not automatically accepted as valid.
- Not having been replaced, or a matching context, does not guarantee that a judgment is still correct or applicable. The model judges whether it applies.
- When judgment C points to A with `builds_on` and A has a same-lineage correction, that correction is shown on C (`grounds_corrected`, Markdown `linked grounds A corrected by …`). Every correction within the queried history range is included, whether it was stored before C, is C itself, or was stored after C; none is excluded by ordering on record time or event time. Correction IDs are shown in storage order. C is not automatically rejected or switched to new grounds. The marker covers only directly linked grounds and does not propagate in a chain.
- The grounds-correction marker has the same meaning in default/current, list, and show by event ID or request ID. It includes corrections outside the target, keyword and change filters, but not corrections from another lineage or after the fixed query boundary. Paging keeps the same boundary's set and order of correction IDs. This time-neutral marker does not change the existing JSON structure, the makeup of current judgments or stored events, and it does not mean a new notification or an incremental sync of the whole current state.
- The absence of a follow-up relation alone does not establish that something was not done or not resolved. Core provides no absence query such as "no follow-up action recorded".

## 8. Queries

| Query | Content |
| --- | --- |
| `current` | The current judgments above. The count and IDs (up to 50) of other-lineage and unverifiable events are shown separately |
| `list` | Summaries of all events in range. Each event carries whether it is a current judgment, and its replacement and correction markers |
| `show <Dn>` | Details of one event: the reason, the evidence with its add-time and current observations, the context at the time, and short summaries of direct predecessors (grounds and correction targets) and direct successors (events that used it as grounds or corrected it). `--request-id` finds a stored request |

List items give the ID, kind, claimed status, targets and a short summary, and the lineage, replacement, grounds-correction and context-difference markers needed to interpret them. Long reasons, all relations and raw logs are not included automatically. Details do not expand all links; to read more distant relations, select a linked event and read it. No separate graph store or query system is needed.

- **Filters.** `--target` selects events that have at least one of the listed targets, and `--keyword` selects events whose summary and reason contain all the words, case-insensitively. Order is ascending by event sequence number.
- **Changes and replacement notices.** Every query result gives a history boundary cursor (`cursor`, `c1.…`). `--since <커서>` covers events added after that boundary up to this query's boundary. Under the same query conditions it shows new events that match the filter, and when a same-lineage correction in that interval replaced a same-lineage judgment that matches the filter, it attaches a replacement notice (`type: replacement`, Markdown `replacement: A → B`), even when the correcting event is outside the filter and even when the original judgment comes before the earlier boundary. Notices are kept apart from the list of current judgments, and an ordinary current query does not attach past replacements. Changes carry only new events and direct replacement relations; they do not guarantee a sync of the whole set of current judgments, including changes in lineage or in code and document context. Check the full current state with `current`. No per-agent read state or notification events are stored.
- **Paging.** By default 20 items are shown at a time, up to 200 (`--limit`). When the list, the direct successors or the replacement notices are cut off, the result gives `truncated`, the shown and total counts, and a next-page cursor (`next`, `p1.…`). A page cursor fixes the first page's history range, the selected run head and the query conditions, so paging continues without omissions or duplicates even when new corrections are stored in between. When that run head cannot be confirmed in the current chain, `cursor_stale` reports that a new query is needed. A cursor from another history is `cursor_invalid`, and a cursor from other conditions is `cursor_mismatch`. The next changes query advances the history boundary with the new boundary cursor and reflects normal head advances in the same lineage. When the lineage has diverged or the basis cannot be confirmed, the result is not treated as changes that continue the earlier result.
- **Absence.** When there is no history file, the answer is `history: absent`. This means only that no history is stored in this repository, not that nothing was ever recorded.

Markdown may shorten hashes that Core computed to their first 12 characters. Identifying values that the executor gave, such as the `version` of external evidence, are not shortened, so they are distinguished by the same values as in JSON. When a correction relation has the last correction seen at add time (`after`), the relation line shows `after D<n>`. Names that come from the repository, such as the goal path, go into Markdown as they are, so read paths that contain control characters such as newlines in JSON.

Queries do not create or change files or run records, and they do not call a model or an external service. Queries over the same stored data and the same Git state give the same result.

## 9. Receipts and errors

A successful add prints one line of `decision-receipt/v1` JSON: `id`, `request_id`, `seq`, `sha256`, `replayed`. On a replay, it returns the original event's receipt with `replayed: true` and appends nothing.

| Exit | Code | Meaning |
| --- | --- | --- |
| 0 | — | Success, replay, and queries, including history absence |
| 64 | `invalid_input` | Error in arguments, kind, claimed status, format, ID or field |
| 64 | `limit_exceeded` | A documented input limit was exceeded (distinct from the `too_large` observation of file evidence) |
| 65 | `invalid_reference` | Nonexistent target, nonexistent run record line, mismatched hash |
| 65 | `invalid_relation` | A relation does not point to an earlier stored event |
| 65 | `correction_conflict` | Another correction of the same judgment was stored first |
| 65 | `request_conflict` | The same request ID points to different content |
| 65 | `not_started`, `run_record_broken` | No start record; run record chain broken before the add |
| 65 | `history_corrupt` | Corruption of the stored history (truncated line, edit or deletion, event vanished from the end, violation of hash links, sequence numbers or relations, mismatch with `last`, over-limit line) |
| 65 | `unknown_event`, `cursor_invalid`, `cursor_stale`, `cursor_mismatch` | Query target or cursor error |
| 66 | `history_unsupported` | A schema this executable cannot read, or unsupported start rules |
| 70 | `history_unreadable` | Repository read, permission or path error; an abnormal storage path such as a symlink, in both adds and queries |
| 70 | `write_uncertain` | Whether it was applied is unknown because a write, sync or close failed, or the event was stored but `last` could not be updated (the error reports the stored event ID) |

A refusal does not change the history. A corrupt or unsupported history is never shown as a normal full history or turned into an empty success result; queries and further adds stop with the same error. It is not truncated or repaired automatically. `write_uncertain` is not taken as either success or non-application. Check whether it was applied by querying `show --request-id` with the request ID the error reports, or by sending the same request again with the same ID. When corruption prevents that check, the limit shows as an error.

A missing, null, empty or wrongly typed schema field, a duplicate key in the top-level object, and a case variant of the schema key are classified as `history_corrupt`, not as an unsupported version. When a valid schema identifier names an unsupported version, the current version's field and nesting rules are not applied to that version's body, and `history_unsupported` is reported.

Unsupported start rules and an unsupported history schema are reported with exit 66 and a `history_unsupported:` message in all of these: an `add` whose input is itself valid, a replay of the same request, default/current, list, and show by event ID or request ID. Existing events, run records and data are not changed, and no new history store is created. An unsupported case is never reported as an absent history with an empty success result.

## 10. Relation to the execution bundle

Decision history is a supplementary record that holds detailed reasons, evidence and relations once. The ownership and judgment meaning of the goal documents, of the current plan and review in PLAN, REVIEW and Tasks, and of the current state and Task status in PROGRESS stay as they are. The role of PROGRESS for goals started with Seal 0.1.13 or later is in [execution bundle](bundle.en.md) §5.3. Even where the feature is not used or cannot be used, handoff must be possible with the existing Markdown alone. The history is an optional feature in which the executor chooses which events to record and when; it creates no new human approval procedure or fixed work procedure of its own.

## 11. Synthetic example

Below is a synthetic example to explain the format. It does not represent a real goal or conversation. `docs/specs/empty-input` is a hypothetical goal that has `AC-1` and `T001` and has finished `ha start` and `ha check` (seq 2).

```bash
ha history docs/specs/empty-input add --kind 원인 --status 가설 --summary "첫 항목이 항상 있다고 가정했다" --target AC-1 --ref run:2 --request-id cause-1
ha history docs/specs/empty-input add --kind 관측 --summary "빈 파일에서 오류가 재현된다" --target AC-1 --checked-ref file:src/count.go
ha history docs/specs/empty-input add --kind 결정 --summary "빈 입력이면 0을 반환한다" --target T001 --builds-on D1 --builds-on D2
ha history docs/specs/empty-input add --kind 원인 --status 기각 --summary "첫 항목 가정이 아니라 입력 버퍼 경계가 원인이다" --target AC-1 --supersedes D1
ha history docs/specs/empty-input current
ha history docs/specs/empty-input show D3
```

The four adds produce receipts for `D1` to `D4`. In the `current` result, the replaced `D1` drops out and the items below remain. `D3` stays a current judgment, with a marker that its grounds were corrected.

```text
- D2 · 관측 · AC-1 · 빈 파일에서 오류가 재현된다
- D3 · 결정 · T001 · 빈 입력이면 0을 반환한다 (linked grounds D1 corrected by D4)
- D4 · 원인 [기각] · AC-1 · 첫 항목 가정이 아니라 입력 버퍼 경계가 원인이다
```

`show D3` shows the two direct predecessors and the grounds-correction marker.

```text
- builds_on D1 · 원인 [가설] · 첫 항목이 항상 있다고 가정했다 (lineage same)
- builds_on D2 · 관측 · 빈 파일에서 오류가 재현된다 (lineage same, current)
- markers: linked grounds D1 corrected by D4
```

The information is kept even when grounds that were already corrected are used. In a separate hypothetical history, when `D2` corrects `D1` and then `D3` is added with `--builds-on D1`, `D3` shows `linked grounds D1 corrected by D2`. When one event has both `--builds-on D1 --supersedes D1`, its own ID is included in that event's grounds-correction information. In neither case does it assert that the correction came later, and it points to the same events as `grounds_corrected` in JSON.

## 12. Assurance limits

The assurance level of decision history is `local`. An executor with the same user's permissions can edit the files; the hash links and `last` only expose accidents and do not prevent forgery. Deleting the whole history directory, or rolling it back to an earlier copy, is not exposed. Events are the executor's claims; the feature does not implement or guarantee an Agent Wiki, automatic transmission or better performance of future models. Effects such as token savings, resume time or fewer misjudgments are not claimed as completion of this feature.
