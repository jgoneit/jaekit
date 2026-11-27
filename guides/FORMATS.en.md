# Formats and rules at a glance

[한국어](FORMATS.md) · **English**

The formats of the files Jaekit writes and reads, and the rules that decide completion, are set in the contracts. The contracts are kept in English only. This page says what each contract covers and points to the exact definition.

> The contracts are reference documents with exact formats and judgments. You do not need to read them to use Jaekit. How to use it is in the [usage guide](USAGE.en.md).

## What to read when

| You want to know | Read |
| --- | --- |
| What goes in `SPEC.md`, and how | [Goal documents](../contracts/goal-docs.md) |
| What `PLAN.md`, `REVIEW.md`, `PROGRESS.md` and the completion report look like | [Execution bundle](../contracts/bundle.md) |
| `ha` commands, one line of `runs.jsonl`, the completion computation, rule versions | [Run record](../contracts/run-record.md) |
| How a failure before the change (the baseline) is told apart from check results | [Change check results](../contracts/check-result.md) |
| What an agent following Seal may and may not do | [Role of the Seal executor](../contracts/role-card.md) |
| `spar.toml` and reviewer settings | [Spar configuration](../contracts/spar-config.md) |
| Spar rounds, packets, and how findings are classified and handled | [Review round](../contracts/review-round.md) |
| The gate file that judges whether review is finished | [Review gate](../contracts/review-gate.md) |
| The format of each input `spar` accepts | [Spar input formats](../contracts/spar-input.md) |
| How Seal checks that the installed `ha` supports what it needs | [Core capabilities](../contracts/core-capabilities.md) |
| How to record a defect found after completion | [Post-completion findings](../contracts/findings.md) |

## What each contract covers

### Goal documents

The Markdown that Spec writes and Seal reads. It is the only interface between the two products. [Contract: Goal documents](../contracts/goal-docs.md)

- Where the goal folder is and which documents it holds
- What `SPEC.md` contains: completion conditions (`AC-n`) and user scenarios
- Consistency between documents, and the order in which Spec writes them

### Execution bundle

The files Seal leaves in the goal folder. [Contract: Execution bundle](../contracts/bundle.md)

- `PLAN.md`: the condition table, the paths that may change, the budget
- `tasks/T00n.md`, `REVIEW.md`, `PROGRESS.md`
- The completion report and `ha lint` error codes

### Run record

`ha` commands, the format of one line of `runs.jsonl`, and the rules that compute completion. It is the longest contract; judgments that differ between rule versions are collected here. [Contract: Run record](../contracts/run-record.md)

- `ha` commands
- Line format and freshness (whether a record matches the current code)
- Status computation (`ha status`, `ha done`), the completion record and the budget
- Rule versions

### Change check results

The format that ties a change condition's check before the change and its current check to the same declaration. An ordinary nonzero exit is not taken as the intended requirement violation; the check's report decides. [Contract: Change check results](../contracts/check-result.md)

- Declaration and input, the result contract
- The report each run leaves, and its classification
- Runner adapters such as Go tests

### Role of the Seal executor

The authority boundary and completion rules that the Seal skill requires of the working agent. It does not mean a separate role loader or an independent verification service exists. [Contract: Role of the Seal executor](../contracts/role-card.md)

### Spar configuration

The two configuration files that decide which reviewers `spar` starts and which caps, angles, rules and check paths it uses. Only the user configuration chooses reviewers; a repository's project configuration cannot. [Contract: Spar configuration](../contracts/spar-config.md)

### Review round

The `review-round/v1` record that `spar run` leaves for one round. [Contract: Review round](../contracts/review-round.md)

- Packets and workspaces, reviewer results and states
- Classification and handling of findings
- Caps, reruns and the link to earlier rounds

### Review gate

The `review-gate/v1` file that `spar gate` writes after judging from a list of rounds and an expected head whether review is finished. It is a review record, not evidence of completion; completion rules belong to the side that owns them, such as Seal's `ha done`. [Contract: Review gate](../contracts/review-gate.md)

### Spar input formats

For each input field `spar` accepts from reviewer results, configuration files, command arguments and records it reads back: the accepted format, and the result for input outside it. [Contract: Spar input formats](../contracts/spar-input.md)

### Core capabilities

How Seal asks, without a goal, which rules and features the installed `ha` supports. Without what it needs, Seal stops without writing a record. [Contract: Core capabilities](../contracts/core-capabilities.md)

### Post-completion findings

How a claimed defect found after completion is kept with its source, separate from the completion as it was. Core checks the structure of references and history; it does not judge whether the defect is real. [Contract: Post-completion findings](../contracts/findings.md)

## Documents for developing the repository

[Shared regression links](../contracts/regressions.md) and [repository check producers](../contracts/repository-producers.md) are for developing the Jaekit repository, so they are in the "Developing Jaekit" group. The English list of all contracts is the [contracts overview](../contracts/README.md).
