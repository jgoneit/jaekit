# Agent invocations

These rules add to [Agent invocations](../SKILL.md#agent-invocations) in the skill body when an agent invoked Spec. The body keeps who decides: only the user's own words answer, accept, or make documents `Ready`.

## New goals

Settle the writing target as for any request, then write `docs/specs/<goal>/SPEC.md` at once and report; never show only a summary in the conversation and defer the file. The document has `Status: Draft` and one unindented `Proposed-By: agent` line before the first level-two section. Under decisions and reasons, record why the agent invoked this skill, with its source: a quote of the user's words, a review finding's identifier, or the path of a file read. `## Open Decisions` holds one item asking whether the user accepts this proposal, next to any open decision about the content. Never write a new goal into a goal directory that already exists; when goal documents cover the same behavior, ask for the writing target before writing, as [Reading the request](../SKILL.md#reading-the-request) says.

## Amendments

When a host agent invokes this skill to amend an existing goal, for example because a review finding can only be resolved by changing its requirements, write an amendment document at `docs/specs/<goal>/amendments/<name>.md` and report. SPEC.md does not link it, and SPEC.md and the documents it links stay unchanged. An amendment document names the finding or question that prompted it, the section and criterion IDs it changes, and the sentences before and after the change, under the headings `## 계기`, `## 바꿀 곳`, `## 바뀌기 전` and `## 바뀐 뒤`. Each amendment is its own document, decided one at a time. When the user accepts one, apply it to SPEC.md and the documents it links, name the user's words as its source, and delete the amendment document; keep every existing criterion ID and never reuse a removed one. When the user rejects one, delete only the amendment document; the goal documents stay byte for byte as they were. When the sentences an amendment changes already differ in the goal documents from its before text, do not apply it even if the user accepts; tell the user.

## Acceptance

When the user accepts an agent's proposal, including by answering yes to the implement-now question, edit the same goal document: move the acceptance item from `## Open Decisions` to decisions and reasons, name those words of the user as its source, and set `Status: Ready` once no other outcome-changing decision is open. This edit changes no goal, scope, criterion, or other decision. An answer that accepts and also asks for a content change is a change request, not an acceptance: make the change, keep the acceptance item open, and report again.
