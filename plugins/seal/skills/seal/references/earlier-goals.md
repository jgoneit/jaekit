# Goals started with an earlier Seal

Read this only when you continue a goal whose start record names a Seal earlier than 0.1.20 (the `skill:` line of `ha status`), or, for its independent review, a goal started under `run-rules/4` or earlier. Such a goal keeps the run rules, PROGRESS form, review format, completion report format and budget it started with. Never convert its records, `PROGRESS.md`, `REVIEW.md` or completion report, and never fill in observations the earlier form did not record. Review rounds and checks you run from now on follow the current skill; only the documents keep their starting form. The `ha` results for the goal do not depend on the Seal version that carries it.

## Starting and resuming

These goals were started after Seal compared `ha capabilities --format json` with its needs and named an absolute `ha` path. Continuing them needs no such comparison: each `ha` command computes the saved rules or refuses them with exit 66 and records nothing, and every record line keeps its `ha_version`. Report a refusal; do not require `/3` declarations for `/1` or `/2` goals or move ordinary maintain, manual and task checks to the structured contract.

Their budget is the number their PLAN gave at the start (the earlier default was `검증 실행: 50` and `경과 시간: 4시간`). A budget change follows the same `ha budget` rules as any goal.

## PROGRESS

- Goals whose start record names seal 0.1.6 or later, up to 0.1.19: `## 현재` is a short summary stamped with its check time (with time zone) and the record head, seq and hash, it was checked against, for example `2026-01-01T09:00:00+09:00` and `seq 21 (64b7bf74bd68)`, or `none` before `ha start`, with the completion record seq when there is one, the work in progress, the open problems, the next work and how to resume. Restamp the check time and record head whenever it changes. Whoever takes the goal up compares the record head with `ha status`: the same seq with another hash is another record, and the check time is for reference only. When resuming, for a goal started with seal 0.1.6 or later, restamp `## 현재` with the check time and record head, and append a `재개` entry to `## 타임라인` that notes where `## 현재` differed from the actual state, if it did. At completion, `## 현재` is as of that completion record and says where later events, such as the merge, are recorded. `## 타임라인`, `## 계획 변경` and `## 막힘` follow the rules in [bundle.md](bundle.md#progressmd), which did not change.
- A goal started with an earlier Seal than 0.1.6 keeps the PROGRESS form it started with.
- Goals started with seal 0.1.13 or later may keep detailed judgments in the optional decision history and point to them from PROGRESS by event ID ([bundle.md](bundle.md#progress-with-decision-history)). Goals started earlier keep their PROGRESS form; never convert or omit their past explanations.

## Review and evidence

- Goals started with seal 0.1.10 or later connect expected-result grounds, observed boundaries, methods and substitutes, and uncovered failure classes in their existing `REVIEW.md` and report ([bundle.md](bundle.md#evidence-and-observation-scope)). Older goals keep their starting format without fabricated observations.
- Goals started with Seal 0.1.11 or later link condition groups to the selected assurance policy, its source and the required or optional guarantees ([bundle.md](bundle.md#assurance-policy-and-counterexamples)). Their `REVIEW.md` has no `## 독립 검토` section; record the tier and review stages you apply from now on under `## 검증 방법과 환경` or the completion report, without rewriting what is there.
- Checks written or changed from Seal 0.1.14 on follow the check input rules ([bundle.md](bundle.md#check-inputs)) whichever Seal started the goal; existing checks are not rewritten for this reason alone.
- Reports using Seal 0.1.12 or later may hold post-completion discoveries in the document format ([bundle.md](bundle.md#post-completion-discovery-documents)).

## Goals started under run-rules/4 or earlier

Core does not use a review gate for these goals. For goals started under `run-rules/4` or earlier: Without Spar, a separate review recording role, inputs, versions, findings, limits and inspectable references counts if the report names it, says Spar was not used, and it covered the whole last head before `ha done`.

## Completion report

Written when `ha done` records completion, or when work stops at `needs_user`, `blocked`, or `budget_exhausted`:

1. The `ha done` output (completion) or `ha status --format md` output (stop), pasted unchanged. It contains results, reasons, digests, and seqs, never command output.
2. For goals started with Seal 0.1.15 or later, the completion coverage: the completion record's seq and commit, whether the head to be sent or merged is that commit, and, when it is not, that the difference is not covered by the completion, unless `ha status` at that head still names that completion record. Goals started with an earlier Seal keep the report format they started with.
3. Per-condition evidence (condition, result, record seq) when the output above does not already show it.
4. Changed files and the conditions each served, marked as the agent's claim.
5. Remaining limits: assumed decisions, `manual` conditions, assurance `local`, and that the executor wrote the checks.
6. For goals started with seal 0.1.10 or later, connect the planned review to observed report: expected grounds, actual boundaries, methods and substitutes, results, evidence and unobserved scope. A report does not establish deployment, installation or a real model call.
7. How to continue, when not complete.
