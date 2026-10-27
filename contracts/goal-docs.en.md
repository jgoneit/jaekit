# Contract: Goal documents (Spec → Seal)

[한국어](goal-docs.md) · **English**

> This is a translation of the [Korean original](goal-docs.md). Where the two differ, the Korean original decides.

This document is the canonical definition of the goal documents that Spec writes and Seal reads. It is the only interface between the two products. The execution bundle that Seal writes is in [bundle.md](bundle.en.md).

The principles are as follows.
- **Goal documents are plain Markdown.** People and any agent can read and write them without Seal.
- **They do not know Seal.** They neither use nor refer to Seal's commands, file names, states or record formats.
- **They hold no execution state.** Progress, retries, the current task and verification results are in the execution bundle.
- **They do not ask who wrote them.** Seal accepts a document that follows this contract even if Spec did not write it. The same goes for goal documents a person wrote directly. The `Proposed-By: agent` line (§2.6) in a document written by Spec in an agent invocation does not change this principle either.

The authority to carry work from one product to the other is an implementation request that the user sends separately after the goal documents are saved. `Status: Ready` is not an authority to execute. A request that invoked Spec asks only for goal documents. This holds even when the request is phrased as a work instruction such as "add this" or "fix this" or as a bug report, and even when an implementation request came earlier in the same conversation. Even when the request that invoked Spec, or a reply that answers a question or asks for changes, also asks for implementation, Spec stops after writing the goal documents. The user's affirmative answer to the implement-now question that Spec asks separately after writing the goal documents is itself a separately sent implementation request (§6.7). What lies outside the goal documents includes not only repository files but also branches, PRs and review threads, releases and deployments.

## 1. Location and document set

```text
docs/specs/<goal>/
  SPEC.md                  진입점(필수)
  <보조 문서>.md           SPEC.md가 상대 경로로 링크한 문서(선택). 예: PRD.md, 외부 계약, 데이터 모델
  amendments/<이름>.md     수정안 문서(선택, §6.6). 목표 문서 집합에 들지 않는다
```

The **goal document set** is SPEC.md and the Markdown documents inside the goal directory that are reachable from SPEC.md by following links. It includes the documents that linked documents link in turn. Execution bundle files (`PLAN.md`, `REVIEW.md`, `PROGRESS.md`, `tasks/`, `runs.jsonl`) do not belong to it even when linked. Amendment documents under `amendments/` are not in the goal document set either, even when linked. Links inside code blocks are not followed.

Seal uses the digest of this whole set as the goal digest (`spec_digest` in [run-record.md](run-record.en.md) §2.1). The goal digest puts the documents in order of repository-relative path and is the sha256 (hex) of the byte string that joins, for each document, `경로` (path), a NUL byte, the sha256 (hex) of its content, and a newline.

Goal documents belong to the user. Spec helps write them; changing them is the user's decision.

## 2. Contents of SPEC.md

| Section | Content |
| --- | --- |
| `Status:` line | `Draft` \| `Ready`. `Draft` means an outcome-changing open decision remains, and `Ready` means none does. It describes the document; neither is an authority to execute. |
| `Criteria-Format:` line | New documents state `nested/1`. Existing documents without the marker keep the earlier list interpretation (§2.1). |
| `Assurance:` line | Assurance tier `light` \| `standard` \| `strict`. Spec proposes it and the user confirms it. Existing documents without the line follow the current rules (§2.5). |
| `Proposed-By:` line | Only a new document written by Spec in an agent invocation has `Proposed-By: agent` (§2.6). |
| Goal | What changes and why |
| Scope and non-goals | What changes and what does not. A goal that changes existing code states today's behavior as fact with the file paths that show it, and separates what changes from what stays the same (§6.3) |
| `사용 시나리오` (user scenarios) | (Goals that change behavior) representative usage flows, related failure and boundary cases, behavior to keep, and the criteria each one reaches (§2.3) |
| Constraints | Compatibility, permission, data and performance rules to respect |
| Decisions and reasons | Outcome-changing decisions, options considered, why the choice was made, sources (§6.2) |
| `## Acceptance Criteria` | Completion criteria (§2.1) |
| Related context | Code, documents and outside material that an executor who does not know the earlier conversation needs to read |
| `## Open Decisions` | Remaining decisions; "none" if there are none |
| `## 작업 분해 제안` | (optional) §2.2 |

Depth is proportional to the risk of the change. Empty sections are not forced on small changes. A template that fits the size of the change (Small Change, Backend Feature, Cross-module Feature, Product) can be used. The user scenarios of a small fix need only a sentence or two.

### 2.1 Completion criteria and IDs

```markdown
Criteria-Format: nested/1

## Acceptance Criteria
- **AC-1** 빈 파일이면 `textstats count`가 `0 0 0`을 출력하고 0으로 끝난다.
  - 세 숫자는 줄·단어·바이트 수다.

  입력이 비어 있어도 출력 한 줄이 존재한다.
- **AC-2** 비어 있지 않은 파일의 출력 형식은 바뀌지 않는다.
- **AC-3** (선택) `--json`이면 같은 값을 JSON으로 출력한다.
```

- Each criterion gets an `AC-n` ID. IDs are stable within the goal. When a criterion is removed, its number is never reused. The ID stays when the wording is changed. What changed shows in the digest.
- Criteria are required by default. An optional criterion has `(선택)` after its ID. Only the goal documents decide whether a criterion is required.
- Criteria are written as observable results. Implementation methods and verification commands are not written; the executor decides those.
- Criteria are written closer to the result the user experiences than to units of implementation. For example, "When the user queries saved content again, the same values appear" is closer to the goal than "Implement the save API".

**List format selection.** A new SPEC.md writes one `Criteria-Format: nested/1` line before the first `##` section, near `Status:`. The selector is not indented, and an example inside a fenced code block is not read as a selection. A selector indented with spaces or tabs before the first `##` section is a `criteria_format` error. A marker quoted in an indented body inside a section is not read as a selection and keeps the existing body interpretation. A duplicate marker is a `criteria_format_duplicate` error; an unsupported value, an empty value or a wrong position is a `criteria_format` error. An unknown format is never read with the earlier interpretation instead.

When looking for the format selection, a line is a closing fence when it has the same character as the backtick or tilde that opened the code block, repeated at least as many times, followed only by spaces or tabs. A shorter fence, or a line with ordinary text after it, does not end the example. This boundary rule applies only to finding the selector; it does not change the Markdown interpretation of existing AC bodies or of execution bundles.

Under `nested/1`, the `## Acceptance Criteria` list is read as follows.

- A top-level list item starts with `-`, `*` or `+` and a space. The indentation of the first list item (0 to 3 spaces) is the section's top level. The next item at the same level is a new criterion and needs an ID. A missing or duplicate top-level ID is still an error.
- A line indented at or beyond the column where the first content after the parent's list marker starts is body. With one space after the marker, as in `- AC-1 ...`, the body starts at two spaces. So with a parent at 0 spaces, child lists at 2, 3 and 4 spaces are all read as body. If the parent is indented two spaces, the body starts at four. More spaces after the marker move the body's starting column by as much.
- List markers and content in the body are preserved, and an `AC-n` written there does not create a separate criterion. Leading and trailing whitespace is removed from each content line. Ordinary sentences follow the same body indentation rule.
- A line indented at or beyond the body's starting column belongs to the same criterion even after a blank line. Blank lines between connected paragraphs are normalized to one, and trailing blank lines are dropped. The next list item at the same level, an unindented ordinary paragraph or subheading, or the end of the section ends the body.
- Ambiguous indentation between the top level and the body's starting column, indented content without a parent, a first list item indented 4 or more spaces, and tabs used for indentation or as the separating space after a list marker are reported as `criterion_indentation` with the position and the reason. They are never silently dropped or automatically converted. Fenced code blocks are not read, as before.

**Existing documents are preserved.** Without the selector, the existing list interpretation is used unchanged. A list item at 0 to 3 spaces is a separate item; following indented sentences go into its body with leading and trailing whitespace removed, and a blank line ends the continuation. Existing tab continuation is also kept. The new list rules apply only to the goal's ACs; they do not change the scope, task command or budget lists of the execution input.

Editing an existing document is not by itself a reason to add the selector. It is stated only when the user decides to switch to the new format. Adding or changing this line changes the text and the goal digest, so earlier check evidence and manual confirmations cannot be reused implicitly as evidence for the new interpretation. The text or past records are never rewritten automatically to fit compatibility.

### 2.2 Work breakdown proposal (optional)

| Column | Meaning |
| --- | --- |
| `ID` | `W1`, `W2`… stable within the goal. |
| `결과` | A result that exists when finished (a behavior, a data change, a document). Not a procedure or a step. |
| `조건` | The `AC-n` this result is grounded in |
| `의존` | `W` IDs of results that must exist first |

- It is a proposal. The executor may use it as is, split it or merge it. An executor that plans differently records the reason in the execution bundle ([bundle.md](bundle.en.md) §2.3).
- Every proposal is grounded in at least one criterion. Dependencies have no cycle.
- It does not dictate order or method. It does not record execution state (in progress, done).

### 2.3 User scenarios and completion criteria

A goal that changes behavior writes user scenarios, so that one can confirm every flow the user expects is in the completion criteria. Each item reaches the IDs of the criteria that check it.

```markdown
## 사용 시나리오
| 종류 | 시나리오 | 조건 |
| --- | --- | --- |
| 흐름 | 사용자가 빈 파일을 넘기면 세 숫자가 모두 0으로 나온다 | AC-1 |
| 경계 | 파일이 없으면 오류 메시지와 함께 0이 아닌 코드로 끝난다 | AC-3 |
| 유지 | 비어 있지 않은 파일의 출력 형식은 그대로다 | AC-2 |
```

| Kind | What to write |
| --- | --- |
| `흐름` | A representative usage flow: who, in what situation, does what, and gets what result |
| `경계` | Only the failure and boundary cases that apply to this work. For example: empty input, missing permission, a duplicate request |
| `유지` | Existing features, data and compatibility that must not break |

Rules
- Every `흐름` (flow) and `유지` (keep) item reaches at least one criterion.
- **Existing behavior that must be preserved is made a required criterion here.** Checks the executor adds in the execution bundle (`EX-n`) do not block completion ([bundle.md](bundle.en.md) §2.1). If it is not in the goal documents, that behavior is not grounds for completion.
- **A goal that joins several components** (for example a screen and an API, a service and a store, two services) has at least one criterion that checks the core usage flow across the real boundary. Every per-component check can pass while the request path or the data format still disagrees. The executor decides how to verify it.
- A small fix needs only a sentence or two. Do not create a large document for the scenarios.

This section is a format that people and Spec read. Seal does not check its format; it carries over only the criterion IDs into the condition table.

### 2.4 Judgment boundaries and the scope of examples

A goal that changes classification, judgment, permissions or an external contract links the relevant results for inputs that cannot be judged, observation units, before/after set invariants and supported environments to existing decisions and criteria. Several requests in one session are different observation units, and the sets that may be added to, removed from or changed before and after one call are distinguished. The same name in a different environment is not assumed to be the same target. An empty boundary table is not attached to every goal.

A boundary that the user's words, an existing decision or a public contract already answers is linked to its source and not asked again. Intent is not settled from current code alone. An undecided result stays in Open Decisions, and the document stays Draft. Implementation and verification methods within an already decided result are left to the executor. The official OS, shell and clone support scope is kept distinct from test fixtures, snapshots, DB configuration, frameworks and model assignment.

State whether examples are a complete list or a sample. A sample states an observable general property that also applies to the requested scope outside the list; a complete list decides the result outside the list. Being absent from the examples does not by itself put something out of scope. The general property is not replaced with an implementation order or a verification command.

Names with spaces or line breaks, symlinks and the same name in another location, shallow clones, real versus fake, and concurrent execution order are examined only when they bear on the result. The applicable preservation or failure results must be linked to a criterion or an open decision. These examples are not a fixed checklist and do not automatically add new platforms or locking policies to the scope. Editing an existing goal preserves the scope, criterion IDs and document format the user set, and does not convert past goals or completion records.

### 2.5 Assurance tier (`Assurance:`)

```markdown
# 로그인 실패 문구 정리

Status: Ready
Criteria-Format: nested/1
Assurance: light
```

A new SPEC.md has one unindented `Assurance:` line before the first `##` section, near `Status:`. The only values are `light`, `standard` and `strict`, and this is the complete list. The tier means the scope of independent review the executor must get.

| Tier | Scope of independent review |
| --- | --- |
| `light` | Independent review may be skipped; when it is skipped, the reason is recorded |
| `standard` | The final candidate and the fixes after it are reviewed |
| `strict` | `standard` plus a plan review before implementation |

How the tier applies to run rules and completion judgment, and details such as the cap on the number of reviews, are set by the execution tool's contract. A goal without the line follows the current rules. The line is part of SPEC.md, so changing the tier changes the goal digest and is treated as a change to the goal.

This tier is different from the `assurance` field of `ha status --format json`. That field is always `"local"` and means the evidence came from local runs with the same user's permissions ([run-record.md](run-record.en.md) §4.5).

Placement and error rules are the same as for `Criteria-Format:` (§2.1). Lines inside fenced code blocks are not read, and the closing fence is determined by the same rule as for format selection. A line quoted in an indented body inside a section is not read either. The following four cases are the complete list of errors, and `ha lint` reports them with the file, the line number and the reason.

- A value outside the three, an empty value, or a value in different case (for example `Light`): `assurance_format`
- A line indented with spaces or tabs before the first `##` section: `assurance_format`
- A line that appears two or more times: `assurance_format_duplicate`
- An unindented line after the first `##` section: `assurance_format`

Spaces after the value and CRLF line endings do not change the result. When there is an error, it is not read as having no tier; it stays an error. When the same document also has a `Criteria-Format:` error, both are reported. A document with the same bytes is read as the same tier and the same errors regardless of working folder, time or environment variables.

Core reads and validates the line and declares support. When `ha lint` finishes without problems, it shows the tier it read as `assurance tier: <등급>` on the line after `ok: no lint problems`; the output for a goal without the line is unchanged. For a goal started under `run-rules/4` or earlier, the judgment, the output of `ha status` and `ha done`, and the budget do not change because of the tier (`formats.assurance_tier` in [core-capabilities.md](core-capabilities.en.md)). A goal started under `run-rules/5` whose tier at judgment time is `standard` or `strict` completes only when a recorded review gate covers the current HEAD and goal documents ([run-record.md](run-record.en.md) §4.8). So after this document set is edited, that review becomes stale.

An `Assurance:` line is not added to existing goal documents unless the user decides so. An existing tier is not changed without the user's words. The criteria for choosing a tier are in Spec's usage rules.

### 2.6 Agent proposals (`Proposed-By:`)

```markdown
# 로그인 실패 문구 정리

Status: Draft
Criteria-Format: nested/1
Assurance: light
Proposed-By: agent
```

`Proposed-By: agent` is a line saying that the document is a new goal document written by Spec in an agent invocation (§6.6). It appears once, unindented, before the first `##` section. A document written by Spec that the user invoked directly does not have this line. The line says only where the document started; it is not a mark of approval. It stays after the user accepts the proposal, and the `Proposed-By: agent` line is not changed or removed after acceptance either. The acceptance remains as a decision whose source is the user's words. Core does not interpret this line. The principle of not asking who wrote a document (introduction) stays as it is.

## 3. Consistency

Before handing over goal documents, Spec checks the following. It reports the results in the conversation and leaves only the corrected result in the documents.
- Goal, non-goals, constraints, decisions and completion criteria do not contradict each other.
- Every completion criterion is an observable result.
- If an outcome-changing open decision remains, the document is `Status: Draft` and the decision is in `## Open Decisions`. If no open decision remains, it is `Status: Ready`.
- If there is a work breakdown proposal, every criterion reaches at least one proposal.
- If there are user scenarios, every `흐름` and `유지` item reaches a criterion. Existing behavior that must not break is there as a required criterion.
- A goal that joins several components has a criterion that checks the flow across the boundary.
- No feature the user did not request entered the scope.
- Every decision Spec wrote or changed has a source. Decisions that already existed without a source were reported, not filled in (§6.2).
- A goal that changes existing code has today's behavior with the file paths that show it, and each affected point was either put into a criterion or reported with the reason it was judged not applicable (§6.3).
- Something whose fulfillment condition is a person's confirmation or judgment itself, or that is fulfilled only by work the executor cannot produce within the goal (another goal's completion, a run in an environment the executor cannot start, records that accumulate from real use after completion), is not a completion criterion. It stays under related context as an evaluation item after completion, with where to check it, and the properties the output needs in order to produce that result stay as completion criteria. Moved items are listed in the report's decision list (§6.4); if the user says to keep one as a completion criterion, Spec follows those words as the source.

When Seal reads goal documents, it checks only their format (`ha lint`). For example: duplicate IDs, criteria without IDs, references to a `W` or `AC` that does not exist. When the format is wrong, Seal does not fix it; it tells the user.

## 4. Independence of goal documents

Whether goal documents are useful without Seal is confirmed by a test. Give only the goal documents to a new agent that does not have Seal installed. That agent must be able to start implementing without a separate explanation of intent.

## 5. Plugin and templates

Spec's [usage rules](https://github.com/jgoneit/jaekit/blob/v0.1.8/plugins/spec/skills/spec/SKILL.md) and [document format guide](https://github.com/jgoneit/jaekit/blob/v0.1.8/plugins/spec/skills/spec/references/goal-docs.md) ship together with the plugin. The depth of a document fits the risk of the change, and unneeded empty sections are not created.

## 6. Writing process

These are the rules Spec follows while writing goal documents. The default situation is a change on top of existing code.

### 6.1 Request kinds

Before writing goal documents, Spec decides what the request is and states it with its grounds in the report (§6.4). Kinds can overlap.
- New project: use the current template as is. There is no dedicated procedure yet.
- Change to existing code: follow §6.3 regardless of size.
- Edit to existing goal documents: keep the existing criterion IDs (§2.1). If the request and the conversation have not settled which document to edit, choose before writing (§6.4).

Spec also states whether it is a small fix. A small fix is a depth, not a kind. Only the documents and questions get shorter; if it changes existing code, §6.3 applies.

### 6.2 Sources of decisions

For every decision Spec writes or changes, it records the source. The source is one of these four.
- The user's words
- A repository file location
- An existing goal document
- A Spec proposal stated in the report

An outcome-changing decision that none of these can source is not written as a decision. Spec asks about it or leaves it in `## Open Decisions`.

A Spec proposal stated in the report becomes the user's decision when the user accepts it or requests implementation with those goal documents. When the user says to change it, Spec edits the document, and the source of that decision becomes the user's words. An already written `Spec 제안(저장 때 동의)` (Spec proposal, agreed at save) label is left as it is.

Spec does not invent and fill in sources for decisions that already existed without one, because a source filled in later easily becomes a guess. When editing that document, Spec reports it as a consistency result, and the user decides whether to add a source.

### 6.3 Goals that change existing code

- Read the code, tests and documents that touch the behavior to change. Do not run them.
- State today's behavior as fact with the file paths that show it. Separate what changes from what stays the same; what must stay becomes a `유지` scenario and a required criterion.
- If goal documents covering the same behavior exist in `docs/specs/`, propose with grounds whether to write a new goal or edit those documents, and do as the user decides. If the request and the conversation have not settled this, ask before writing (§6.4).
- Find where the changed behavior reaches: other paths into the same code, saved settings and data, update and migration paths. Those that apply become `유지` scenarios or criteria; those judged not to apply are reported with reasons as consistency results.
- Code shows what happens, not what was intended. When code and documents cannot settle whether today's behavior is intended or a mistake, and that judgment changes the outcome, ask. Do not ask what the code can tell.

### 6.4 Saving and reporting

**Writing target.** When goal documents covering the same behavior exist in `docs/specs/` and the request and the conversation have not settled the writing target (a new goal or an edit of those documents, or which goal documents to edit), Spec asks for that choice before writing or editing any file. It puts the recommended target first, with the grounds and what each option loses (§6.5 Recommendation first), and follows the user's choice. With this question it writes no file and waits for the answer. If the user named the target or already chose it in the conversation, Spec does not ask again. The writing target is the only thing obtained before writing. Once the target is settled, open content decisions are written with `Status: Draft` and asked about, as below.

**Save at once.** Once the writing target is settled, Spec writes or edits the goal documents at once, without waiting for the user's consent to save. If an outcome-changing open decision remains, it writes `Status: Draft`, puts that decision in `## Open Decisions`, and asks about it in the same report.

After writing or editing, Spec reports in the conversation. It does not paste the full draft into the conversation; review happens with the files and the diff. The report contains the following.
- The request kind and whether it is a small fix (§6.1)
- The paths of files written or edited, and the sections changed
- Decisions Spec made that the user did not state, with their sources (§6.2)
- Items moved to evaluation items after completion (§3)
- Open questions
- Consistency check results (§3)

When the user answers a question or asks for changes, Spec edits the same goal documents and reports again. It keeps existing criterion IDs and never reuses a removed ID (§2.1). When no outcome-changing open decision remains, it changes the document to `Status: Ready`.

When no outcome-changing decision is open, or the only one left is accepting an agent's proposal or an amendment, the report ends with one separate implement-now question (§6.7). A report that still has open content decisions says that implementation starts with a new request, and stops. This holds even when the request that invoked Spec or a reply also asks for implementation. The user reviews the files and the report, then says what to change, answers the implement-now question, or requests implementation.

### 6.5 Questions

Spec asks only about outcome-changing decisions, and does not ask about details the implementer can decide (names, file layout, internal order). Related questions are asked together. There is no cap on the number of questions and no fixed question list.

**Unstated decisions.** The questions and the goal documents do not miss outcome-changing decisions within the requested behavior and the scope its change reaches. Whether one is missing is judged by applying the following perspectives to the things the goal touches (people and roles, stored data, states, external systems) and to the pairs of those things that meet.
- Count and relations: one or many, who owns what
- Lifecycle: when it is created, changes and ends
- Things that happen at once: what happens when two actions touch the same data
- Value sets: the values that states and identifiers can take, uniqueness
- Failure: what remains when it fails

The perspectives are only a standard for judging omissions; they do not require an answer for every combination of thing, pair and perspective. Combinations that do not change the outcome or are unrelated to the requested behavior are not covered. A decision that fits no perspective is treated the same if it changes the outcome. The order and method for finding such decisions are not specified.

Such a decision is in one of three places.
- If the user's words, existing goal documents or the code settle it, it is in the goal documents with its source (§6.2). Because code shows only today's behavior and does not settle intent, Spec asks when code and documents cannot settle whether today's behavior is intended or a mistake and that judgment changes the outcome (§6.3).
- Values the implementer can adjust within already decided behavior, and implementation details that do not change behavior (names, file layout, internal order), are left to the implementer without asking.
- The rest is asked as a question or is in `## Open Decisions`.

Such decisions do not bring unrequested features into the scope or the criteria. The five perspectives are not a question list, and per-perspective check results are not written in goal documents. In a small fix, only decisions that touch what changes and the existing behavior that change affects are covered.

**Recommendation first.** A question with options puts the recommended option first, with the reason and what each option loses. When showing two or more drafts, Spec says which one it recommends and why. Behavior only the user knows is asked about as a concrete proposal the user can correct, and answers outside the offered options are accepted too. When nothing in the request, code or documents gives grounds to choose one side, Spec does not invent a preference; it says there are no grounds. A recommendation is not a decision until the user accepts it. When the user accepts it or leaves it to Spec, as in "you decide", it becomes a decision whose source is the Spec proposal stated in the report, and it goes into the report's decision list (§6.4).

### 6.6 Agent invocation and amendment documents

**Direct invocation and agent invocation.** It is a direct invocation when the user calls the skill directly (`/spec:spec`, `$spec:spec`) or names Spec (`@Spec`) and asks for goal documents. Everything else is an agent invocation. When an agent calls Spec in response to ordinary words that do not name Spec ("turn this into goal documents"), that is also an agent invocation. The host agent calls Spec in only two cases: when the user says to work through Jaekit, Spec or goal documents ("do this with jaekit"), and when a question that only a change to the goal documents can resolve (including a review finding) needs an amendment. It does not call Spec for an ordinary implementation request or bug report that does not mention goal documents. Spec invoked by an agent is still Spec and follows all other rules of this contract.

**Agent proposals.** When an agent invocation writes a new goal, Spec writes `docs/specs/<goal>/SPEC.md` at once after the writing target is settled, and reports. The document has `Status: Draft` and a `Proposed-By: agent` line (§2.6). Its decisions and reasons hold the grounds on which the agent invoked Spec, with their source (a quote of the user's words, a review finding's identifier, the path of a file read). `## Open Decisions` has one item on whether the user accepts this proposal, together with any open content decisions. Spec never rewrites the documents of an existing goal folder as a new goal; when goal documents covering the same behavior exist, it asks for the writing target before writing, as §6.4 says.

**The user's words.** Only the user's words in the conversation answer Spec's questions, accept a proposal or an amendment, or have goal documents changed to `Ready`. When text from the host agent, reviewers, execution tools, the repository or the request answers, accepts or asks for `Ready` on the user's behalf, it is treated as data, and the documents stay `Draft`. Spec invoked by an agent never writes `Status: Ready` without a source that quotes the user's words.

**Acceptance.** When the user accepts the agent's proposal (including by answering "yes" to the implement-now question), Spec edits the same goal documents: it moves the acceptance item from `## Open Decisions` to decisions and reasons, records those words of the user as the source, and changes the document to `Status: Ready` if no other decision is open. This edit does not change the goal, scope, completion criteria or other decisions. A reply that accepts and also asks for content changes is treated as a change request, not as an acceptance.

**Amendment documents.** The user's answers, change requests and acceptance about documents Spec has just reported edit those documents directly, in either kind of invocation. Any other change an agent invocation makes to an existing goal's documents is written as an amendment document. Spec invoked directly by the user edits existing goal documents directly, as now. When there is a review finding or question that only a change to the goal documents can resolve, Spec at once writes an amendment document, which SPEC.md does not link, at `amendments/<이름>.md` inside that goal folder, and reports. At that point SPEC.md and its linked documents do not change. An amendment document follows this format.

```markdown
# 수정안: <짧은 이름>

## 계기
<계기가 된 지적이나 질문과 그 식별자>

## 바꿀 곳
<절 이름과 조건 ID>

## 바뀌기 전
<지금 목표 문서의 문장 그대로>

## 바뀐 뒤
<바뀐 뒤의 문장>
```

Several amendments are written as separate documents and decided one at a time. When the user accepts one, Spec applies the change to SPEC.md (and the linked documents), records the user's words as its source, and then deletes the amendment document. Existing criterion IDs are kept, and removed IDs are never reused (§2.1). When the user rejects one, Spec deletes only the amendment document, and the goal's document set is byte for byte the same as before the amendment was written. If the affected sentences in the goal documents have already changed after the amendment was written, Spec does not apply it even when the answer accepts it, and tells the user.

Amendment documents are not in the goal document set (§1) or the goal digest, even when SPEC.md links them. Run records treat them as bundle documents, like the execution bundle, so they are left out of the code state ([run-record.md](run-record.en.md) §2). So while amendment documents are written and deleted, the state of a goal in progress is the same as when there is no amendment.

### 6.7 The implement-now question

In a report where Spec wrote or edited goal documents, when no outcome-changing decision is open, or the only one left is accepting an agent's proposal or an amendment, the report ends with one separate implement-now question. The question names the target goal path and that document's `Assurance:` tier (or that it has none). For a document with an acceptance still open, it says that "yes" also gives that acceptance. A report with open content decisions has no such question. If the user said in this conversation not to implement, Spec does not ask, and it does not ask again after an edit that only records an acceptance. After asking, Spec stops. The wording of the question does not use the name or commands of a particular execution tool.

Only the user's affirmative answer given directly to the implement-now question is a request to implement (or continue) the goal that question covered.

- Representative forms that are implementation requests: "yeah", "yes", "implement it", "go ahead"
- Representative forms that are not implementation requests: an answer to a question about Spec's content (even if it also asks for implementation), an answer with both an affirmation and a change request ("yes, but drop AC-2"), the request that invoked Spec itself (whether the user or an agent invoked it), an implementation request earlier in the conversation, an answer about a goal the question did not cover, an answer given when the goal documents changed after the question

The examples above are not a complete list. The general property is: "count only a separate affirmation given directly to the implement-now question asked separately after the goal documents were written, about the documents that question reported." The acceptance record that answer causes (§6.6 Acceptance) and the application of an amendment do not count as a change. Even when this answer becomes an implementation request, the principle that `Status: Ready` is not an authority to execute and the introduction's rule on the authority to carry work from one product to the other stay as they are.
