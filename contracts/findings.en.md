# Contract: Post-completion findings

[한국어](findings.md) · **English**

> This is a translation of the [Korean original](findings.md). Where the two differ, the Korean original decides.

A post-completion finding is a claim with a source, kept separately from the completion as it was at the time. Core checks the structure of references, revisions and history; it does not certify whether a defect is real, how severe it is, or whether a resolution is sufficient. More findings, or confirmed findings, do not change existing completion judgments, end results or execution authority. The existing completion validity rules for changes to code, goals or check evidence and for an explicit user rework request stay in force.

## Support and compatibility

Post-completion findings are included from Core 0.1.4 and Seal 0.1.12. Core v0.1.3 does not support this extension.

An executable whose `capabilities` has `post-completion-findings/v1` in `features` and `run-finding/v1` in `formats.finding` supports this feature. Before using the feature, check these two entries in the binary actually selected. Do not infer support from the version string alone. Do not hand writes to a consumer without the added format and feature, or describe the new data as read normally.

Existing `run/v1` records and the `run-rules/1`, `/2` and `/3` of the start are not changed. Only new findings are appended to the same `runs.jsonl` hash chain, with `schema: run-finding/v1` and `kind: finding`. Adding an unknown note or kind to an existing schema is not used, because older consumers could ignore it. The bytes, sequence numbers and hashes of past lines are kept.

When an older Core reads the new schema, it reports `record_integrity` from the schema mismatch and refuses normal completion and further writes. We do not claim that older binaries support this extension or refuse it with a friendly unsupported-feature code. Data that records new findings is used with a consumer that supports the feature. There is no automatic installation, downgrade or record migration. Valid older records without the extension are still read under the existing rules.

## Input and queries

```text
ha finding <goal> --input <JSON file>
ha finding <goal> [--format json|md]
```

The default query format is JSON, and a query only reads. A change takes one JSON file and prints a JSON receipt. A change is not combined with `--format`. The input is one UTF-8 JSON object of at most 1 MiB; invalid UTF-8, duplicate keys, unknown fields, extra objects and nesting deeper than 32 levels are rejected. Field names must match the contract exactly, including case, and this rule applies to nested objects too. Raw output and document contents are never read or copied automatically.

A stored finding line is at most 2 MiB + 64 KiB (2,162,688 bytes). Re-encoding on storage can at most double the input (U+2028 and U+2029 are written as 6-byte escapes), and a common header is added. Core refuses to write a line over this size (65), and the support check accepts finding lines up to this size. A finding line over this size is distinguished as out of range, an unreadable line as corrupt, and an unknown schema as unsupported. The 1 MiB input limit stays the same.

The fields of a change object are as follows. All example identifiers are fictional data.

| Field | Meaning |
| --- | --- |
| `id` | Unique finding ID within the goal. Alphanumerics and `_.:/-` starting with an ASCII alphanumeric, 1–128 characters |
| `request_id` | Logical change request ID. Same grammar. Used to query again after an uncertain save and to resend the same request safely |
| `revision` | 0 for a new finding; for a later change, the seq of that finding's latest event as read and judged |
| `completion` | `{ "seq", "sha256" }`. The successful completion at the time in the same goal's chain, and the exact hash of its original text |
| `criteria` | The list of criterion IDs that completion recorded. Not retroactively replaced with the current SPEC's criterion list. A required array: omission and null are rejected, an empty array is allowed |
| `mapping` | `linked` requires a criterion list. `unmapped` (link undecided) and `outside` (outside the criteria) require the criterion list to be empty |
| `status` | `candidate`, `confirmed`, `duplicate`, `dismissed`, `resolved`. A new finding must be candidate |
| `category` | A classification claim with a source. Use `existing_condition` for a violation of an existing criterion, `new_requirement` for a new requirement, `unknown` when undecided, and so on |
| `summary`, `source` | Non-empty claims of a summary and a source. Documents and logs are not collected automatically |
| `evidence` | Explicit evidence references for a confirmed, duplicate, dismissed or resolved judgment. Those states need at least one. A required array: omission and null are rejected, an empty array is allowed |
| `reason` | The reason a later change alters the judgment or the content. Optional for a new finding |
| `event_at` | Optional. An RFC 3339 claim of the event time the source knows. Omit it when unknown; it is separate from the record time |
| `duplicate_of` | For duplicate, the ID of an existing finding in the same goal. Self-references and cycles are rejected |
| `resolution_refs` | For resolved, one or more fix or recheck references. One fix may serve several findings. Not allowed in a revision with another state; earlier resolution references stay in the history. An optional list, so omission, null and an empty array mean the same |
| `rework` | Optional. `{ "seq", "sha256" }` of a reopen record that quotes the user and was saved after that completion. Never generated automatically |
| `document` | Optional. `{ "ref", "recorded_at", "event_at" }`. A reference to an existing document and its known times. An unknown time is an empty string |

The criterion, evidence and resolution reference lists each hold at most 256 items; each item is a non-empty string of at most 8192 bytes, with no duplicates. A request is the whole content of that revision; an omitted optional field stays absent from that revision. Earlier judgments and evidence stay preserved in the earlier events. The completion reference cannot be changed by a later correction. A different completion is recorded under a different finding ID.

Confirmed and resolved do not mean that Core ran that outside evidence. They are claims with known evidence references. Adding a document reference alone does not confirm a candidate or create a resolution or a rework request. The original content of the document is not changed.

## History, conflicts and save failures

Every change is appended inside the existing record lock, after the latest data is read again and validated. Concurrent changes to other findings are preserved; a stale revision of the same finding is rejected as a conflict. Nothing is overwritten on the grounds of an event time or of being the last writer. Duplicate relations are also checked again inside the same boundary.

If the same request_id with the same content was already applied, the receipt with the original seq and hash is returned and no further record is made. Content is compared in its stored form, so writing an optional list as an empty array or omitting it is the same request. Different content under the same ID is rejected. A rejected input does not change the record. A write, sync or close failure is an error that may have been applied; it is not taken as success or as not applied. Check the saved state with the original request_id. If a partial write corrupts the chain, queries and further writes report the error, and there is no automatic repair or retry.

The receipt of a successful change is `finding-receipt/v1` and provides `id`, `request_id`, `seq`, `sha256` and `replayed`. Exit codes: 0 for success; 64 for invalid JSON or arguments; 65 for a rejected reference, revision, stored data or state transition; 66 for unsupported run rules; 70 for an internal error or uncertainty whether a write was applied. Even when printing the receipt fails, the actual save may remain.

## Queries and completion

`findings/v1` provides `goal`, the whole chain's `record_head`, `findings`, `candidate`, `confirmed`, `confirmed_existing` and `unresolved`. Each finding has `id`, the latest `revision`, the `commit` and `spec_digest` at the time, the latest `change`, an ordered `history`, `rework_status` and `rework_completion`. A history entry is the event's `seq`, `at`, `sha256` and `change`.

`candidate` and `confirmed` are unresolved. `confirmed_existing` is the reports among confirmed that are linked and whose category is existing_condition. Dismissed, duplicate, new-requirement and unconfirmed candidates are not added to the count of confirmed existing defects. These numbers aggregate registered claims; they are not a denominator over all goals in use or all actual defects.

`not_requested` in rework_status means there is no linked explicit rework request. Other user requests are not inferred. With a linked reopen it is `requested`; when the span from that reopen until the next reopen has a successful completion record, it is `completed`, and the last successful completion in that span is pointed to separately. A completion achieved in a later, different rework span is not linked as the result of an earlier request. A later completion is not treated as certifying that every finding was actually resolved. The resolution judgment and the rework result are separate.

The `findings` of ordinary status include the same query, and the human-readable status shows unresolved candidates, confirmed reports and rework relations together with the completion display. The human-readable query shows each finding's state with its `category` and `mapping`; totals alone do not replace the individual classifications. The currently valid `completion_record` and the historical completion each finding points to are separate. Registering a finding alone does not invalidate the current completion or end result, but unresolved findings are not hidden. Queries of the same stored data are identical, without the current time or a model call.

## Run budget

Finding events are excluded from the time and count inputs of the run budget. The seq and head of the actual whole chain are not changed. Registration, correction, resolution or document linking long after completion, or finding updates after a resume, do not by themselves change elapsed time, usage, whether a limit is exceeded, limits or windows.

The budget meaning of existing ordinary records is unchanged. Actual checks, existing notes, budget changes and reopens the user requested count under the existing calculation of the saved rules. A /3 resume inherits the latest run limit, and /1 and /2 follow the existing original limits. Findings never reset the budget or erase the original usage or failures.

## Seal handoff

When handling findings with a Core that supports the feature, check the actual capability and format before input. The completion report conveys the completion reference at the time, the currently unresolved findings and their confirmation states, and the links between explicit rework requests and their results. Never overwrite the original completion report with later judgments, or promote a document link to a machine-checked result. Material that exists only as documents can be linked as a separate input with its source stated; it is never imported automatically.
