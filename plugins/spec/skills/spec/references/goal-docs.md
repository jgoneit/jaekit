# Goal documents

Goal documents are plain Markdown. People and any coding agent can read and write them without extra tooling. They describe the goal to hold fixed; they never hold plans, progress, or verification results.

## Document set

```text
docs/specs/<goal>/
  SPEC.md          entry point (required)
  <aux>.md         auxiliary documents that SPEC.md links with a relative path (optional)
```

The goal document set is SPEC.md plus every Markdown file inside the goal directory reachable by links from it. Links inside code blocks do not count. Changing any document in the set changes the goal; that is the user's decision.

## SPEC.md

| Part | Content |
| --- | --- |
| `Status:` line | `Draft` while an outcome-changing decision is open, `Ready` when none is. A description, not an authorization |
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
## Acceptance Criteria
- **AC-1** For an empty file, `textstats count` prints `0 0 0` and exits 0.
- **AC-2** Output for non-empty files does not change.
- **AC-3** (선택) With `--json`, the same values are printed as JSON.
```

- Each criterion has an `AC-n` ID, stable within the goal. Keep the ID when the wording changes. Never reuse the number of a removed criterion.
- Criteria are required by default. Mark optional ones with `(선택)` right after the ID. Only the goal documents decide what is required.
- Write observable results. Do not write implementation methods or verification commands.

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
