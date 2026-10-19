# Goal documents

Goal documents are plain Markdown. People and any coding agent can read and write them without extra tooling. They describe the goal to hold fixed; they never hold plans, progress, or verification results.

## Document set

```text
docs/specs/<goal>/
  SPEC.md                  entry point (required)
  <aux>.md                 auxiliary documents that SPEC.md links with a relative path (optional)
  amendments/<name>.md     amendment documents (optional); never part of the goal document set
```

The goal document set is SPEC.md plus every Markdown file inside the goal directory reachable by links from it. Links inside code blocks do not count. Amendment documents under `amendments/` stay outside the set even when linked. Changing any document in the set changes the goal; that is the user's decision.

## SPEC.md

| Part | Content |
| --- | --- |
| `Status:` line | `Draft` while an outcome-changing decision is open, `Ready` when none is. A description, not an authorization |
| `Criteria-Format:` line | New documents select `nested/1`. Existing documents without the line retain their original list interpretation |
| `Assurance:` line | `light`, `standard`, or `strict`: the independent review the implementer must get. Spec proposes it and the user confirms it. Existing documents without the line follow the current rules |
| `Proposed-By:` line | `Proposed-By: agent`, only in a new goal written in an agent invocation |
| Goal | What changes and why |
| Scope and non-goals | What changes and what must not. A goal that changes existing code states today's behavior with the file path that shows it, and separates what changes from what stays the same |
| `## 사용 시나리오` | For goals that change behavior: flows, boundary cases, and behavior to keep, tied to criteria |
| Constraints | Compatibility, permission, data, and performance rules to respect |
| Decisions and reasons | Outcome-changing decisions, options considered, the choice and why, and the source of each one Spec writes or changes: the user's words, a repository file path, an existing goal document, or a proposal Spec stated in its report |
| `## Acceptance Criteria` | Completion criteria with IDs |
| Related context | Code, documents, and outside material a reader without the conversation needs |
| `## Open Decisions` | Decisions still open, or "none" in the user's language |
| `## 작업 분해 제안` | Optional work breakdown proposal |

Depth follows risk. Small changes do not need every part.

## Criteria

```markdown
Criteria-Format: nested/1

## Acceptance Criteria
- **AC-1** For an empty file, `textstats count` prints `0 0 0` and exits 0.
  - The numbers are line, word, and byte counts.

  An empty input still produces one output line.
- **AC-2** Output for non-empty files does not change.
- **AC-3** (선택) With `--json`, the same values are printed as JSON.
```

- Each criterion has an `AC-n` ID, stable within the goal. Keep the ID when the wording changes. Never reuse the number of a removed criterion.
- Criteria are required by default. Mark optional ones with `(선택)` right after the ID. Only the goal documents decide what is required.
- Write observable results. Do not write implementation methods or verification commands.

New documents put one unindented `Criteria-Format: nested/1` line near `Status:`, before the first level-two section. Fenced examples do not select a format. A selector indented with spaces or tabs before that first section is a `criteria_format` error. An indented quotation inside a section's body does not select a format and keeps its existing body interpretation. Duplicate selectors, an empty or unsupported value, and an unindented selector after a level-two section are format errors; never silently use the old format instead.

For format selection, a closing fence uses the same backtick or tilde character at least as many times as the opening fence, followed only by spaces or tabs. A shorter fence or one followed by ordinary text stays inside the example. This boundary check applies only to selecting the format; it does not change the existing Markdown interpretation of criterion bodies or execution bundles.

With `nested/1`:

- Use `-`, `*`, or `+` followed by spaces. The first list's zero to three leading spaces set the section's top-level indentation. Each item at that level needs its own unique criterion ID.
- A body's indentation starts where the parent's text starts after its list marker. For `- AC-1 ...`, this is two spaces; two-, three-, and four-space child lists all belong to that criterion. A parent indented two spaces needs at least four for its body. More spaces after the parent's marker move that content column to the right.
- Child list markers and their text stay in the body. An `AC-n` inside a child does not create another criterion. Trim leading and trailing whitespace on each content line; apply the same indentation rule to continuation sentences.
- A blank line does not end the body if the next line reaches its content column. Preserve one blank line between connected paragraphs and omit trailing blank lines. A top-level list item, an unindented paragraph or subheading, or the section's end closes the body. Fenced code stays excluded.
- Ambiguous indentation between the top-level and body columns, indented content without a parent, a first list indented four or more spaces, and tabs in indentation or after the marker are errors. Do not silently drop or automatically reindent them.

Keep an existing document's selection or lack of selection unless the user explicitly chooses a format change. Unmarked documents keep their previous list, continuation, blank-line and tab behavior. Changing the selection changes the document's meaning and content; prior confirmations and verification evidence cannot silently stand for the newly interpreted text. Do not automatically convert existing documents or rewrite their historical records.

## Header lines

New documents put the `Status:`, `Criteria-Format: nested/1`, and `Assurance:` lines, unindented, before the first level-two section. A new goal written in an agent invocation adds `Proposed-By: agent`.

```markdown
# Shorter login error message

Status: Draft
Criteria-Format: nested/1
Assurance: light
Proposed-By: agent
```

`Assurance:` takes `light`, `standard`, or `strict`; that is the complete list.

| Tier | Independent review the implementer must get |
| --- | --- |
| `light` | The implementer may skip independent review, and records why when it does |
| `standard` | A review of the final candidate and later fixes |
| `strict` | `standard` plus a plan review before implementation |

How a tier applies to execution rules and completion, and details such as review round limits, are set by the contract of the execution tool. Documents without the line follow the current rules. The line is part of SPEC.md, so changing the tier changes the goal digest and counts as a change to the goal. The tier is unrelated to any label an execution tool reports about where its evidence came from.

The line follows the placement and error rules of `Criteria-Format:`: one unindented line before the first level-two section. An indented line there, an unindented line after that section, a value outside the three or in another case (`Light`), an empty value, and a second line are errors. Fenced examples are not read, with the same closing-fence rule. Trailing spaces and CRLF line endings do not change the value. Never add the line to an existing document unless the user decides to, and never change an existing tier without the user's words.

`Proposed-By: agent` says that an agent invocation wrote the new document. It stays after the user accepts the proposal and is not an approval; the acceptance is a decision whose source is the user's words. Execution tools do not interpret it, and goal documents are accepted whoever wrote them. A direct invocation never adds the line.

## Amendment documents

The user's answers, change requests, and acceptance about documents Spec has just reported edit those documents directly. Any other change an agent invocation makes to an existing goal, such as a requirement change a review finding calls for, is written as an amendment document at `docs/specs/<goal>/amendments/<name>.md`, which SPEC.md does not link. SPEC.md and the documents it links stay unchanged until the user decides.

```markdown
# Amendment: <short name>

## 계기
<the finding or question that prompted it, with its identifier>

## 바꿀 곳
<section name and criterion IDs>

## 바뀌기 전
<the current sentences, exactly>

## 바뀐 뒤
<the sentences after the change>
```

Each amendment is its own document, decided one at a time. Accepted: apply it to the goal documents with the user's words as its source, keep every criterion ID, and delete the amendment. Rejected: delete only the amendment; the goal documents stay byte for byte as they were. When the before sentences no longer match the goal documents, do not apply it; tell the user.

## The implement-now question

After writing or editing, when no outcome-changing decision is open, or the only one left is accepting an agent's proposal or an amendment, the report ends with one separate question: whether to implement the goal now. It names the goal path and its `Assurance:` tier, or that it has none, and says when yes also accepts a proposal or an amendment. Only the user's affirmative answer given directly to that question, about the documents it named, is a request to implement them, for example "yes", "go ahead", or "implement it". It is not one when it also asks for a change ("yes, but drop AC-2"), when it answers a content question, when it is the request that invoked Spec, when it came earlier in the conversation, when it is about another goal, or when the goal documents changed after the question; the acceptance or amendment that answer itself has Spec record does not count as such a change. These examples are representative, not complete.

## User scenarios

```markdown
## 사용 시나리오
| 종류 | 시나리오 | 조건 |
| --- | --- | --- |
| 흐름 | A user passes an empty file and sees three zeros | AC-1 |
| 경계 | A missing file prints an error and exits non-zero | AC-4 |
| 유지 | Output for non-empty files keeps its format | AC-2 |
```

| Kind | What to write |
| --- | --- |
| `흐름` | A representative flow: who, in what situation, does what, and gets what result |
| `경계` | Only the failure and boundary cases this work needs, such as empty input, missing permission, or a duplicate request |
| `유지` | Existing features, data, or compatibility that must not break |

- Every `흐름` and `유지` row reaches at least one criterion.
- Behavior that must survive the change is a required criterion here. Checks an implementer adds later never block completion, so they cannot protect it.
- A goal that joins components (UI and API, service and store, two services) has at least one criterion that checks the core flow across the real boundary. Separate component checks can all pass while the request path or data format between them is wrong. The implementer chooses how to verify it.
- A small fix states its scenario in one or two sentences.

## Outcome boundaries and examples

Describe only relevant boundaries in existing decisions and criteria: the result for uninterpretable input, observation unit, before/after set invariants, and supported environment. A request is distinct from a session containing many requests; additions and removals outside the requested set need a preservation result. No separate empty table is required.

Use the user's words, public contract, or existing decision as the source; ask only about unresolved outcomes. Code describes current behavior without establishing intent. Already settled boundaries need no repeated question. Official platform or shell support belongs in outcomes; fixtures, snapshots, frameworks, and model assignment remain implementation and verification choices within those outcomes.

Mark cases as an exhaustive set or representative examples. A representative set has an observable general property over the requested scope; an exhaustive list has an outside-list result. Consider special names, symlinks, shallow clones, real versus fake systems, and concurrent ordering only when they can change this goal's result. Preserve existing IDs, document format, small-change depth, and the separation between writing and implementation.

## Work breakdown proposal

| Column | Meaning |
| --- | --- |
| `ID` | `W1`, `W2`, … stable within the goal |
| `결과` | A result that exists when done: a behavior, a data change, a document. Not a step |
| `조건` | The `AC-n` it is grounded in |
| `의존` | `W` IDs of results needed first, or `—` |

It is a proposal. The implementer may use, split, or merge it. Every proposal is grounded in a criterion, and dependencies have no cycle. Never state order, method, or progress.

## Consistency

Before handing over the documents, confirm and report in the conversation:
- Goal, non-goals, constraints, decisions, and criteria agree.
- Every criterion is observable.
- Scenarios reach criteria; behavior to keep is required; a joining goal crosses the boundary.
- Nothing unrequested entered the scope.
- `Status: Draft` while outcome-changing decisions are open and appear in `## Open Decisions`; `Status: Ready` when none is.
- With a work breakdown proposal, every criterion reaches a proposal.
- Every decision written or changed names its source; existing ones without a source are reported, not filled in.
- A goal that changes existing code states today's behavior with file paths, and behavior that stays the same is required.
- No completion criterion is met only by a person's confirmation or judgment, or only by work the implementer cannot do inside the goal (another goal's completion, a run in an environment the implementer cannot start, records from real use after completion). Such an item is an evaluation item after completion under related context, with where it is checked; the properties the goal's output needs for that result stay as criteria. The user may ask to keep one as a criterion.
