---
name: spar
description: Run independent review rounds on a plan or a change with the spar binary — the same packet to every configured reviewer, classified findings, limited dispositions, stage caps and a review gate file. Use when the user asks for an independent or adversarial review of a plan or an implementation, for example "$spar:spar" in Codex, "/spar:spar" in Claude Code, or "이 변경을 spar로 리뷰해줘".
metadata:
  version: "0.1.3"
---

# Spar

Spar makes reviewers attack a plan or a change from outside your context. The `spar` binary pins two commits, builds one packet (criteria, rules, documents under review and the diff, without Git history, ignored files or commit messages), runs every configured reviewer on it in parallel, classifies their findings and keeps a ledger of what was done with each one. You run the rounds, decide what each finding means for the work, and record that decision. This skill states when to run rounds, how findings may be closed and what the records do not prove.

## Before the first round

- Check the executable: `spar --version`. If `spar` is missing, say so and stop; do not install tools on your own.
- `spar config --repo .` prints the reviewers a round will start, where that list came from, the stage caps, the angles and the check paths. Reviewers come only from the user's configuration (`$SPAR_CONFIG`, `$XDG_CONFIG_HOME/jaekit/spar.toml`, `~/.config/jaekit/spar.toml`, then the clean-review `reviewers` list, then `claude`, `codex`, `agy`). A repository's `.jaekit/spar.toml` sets only caps, angles, rules and check paths.
- The packet goes to each reviewer's provider; the `native` reviewer's provider is the one your own agent already uses. When the criteria or the code must not go to any other provider, use only the `native` reviewer and tell the user why. When they must not leave the computer at all, do not run a round: every reviewer, `native` included, sends the packet to a provider. Say so and stop.
- Records go to a new, Git-ignored folder per round. Rounds of one review share a parent folder, the series: `docs/reviews/<goal>/01-plan`, `docs/reviews/<goal>/02-candidate`, and so on.

## When to run each stage

| Stage | When | Command |
| --- | --- | --- |
| plan | Once, after the plan and its checks are written and before implementation | `spar run --stage plan` with `--subject` for the plan documents |
| plan delta | At most once, only when the plan round changed the plan or its checks | `spar run` with `--stage plan-delta`, the earlier plan head as base |
| candidate | On the commit you believe meets every criterion; when fixes followed, once more as the cumulative round over the goal's base commit to the last head | `spar run` with `--stage candidate`, the goal's base commit as base |
| fixes | After committing fixes for findings; its base is the head of the previous code round | `spar run` with `--stage fixes` |

For example, a candidate round:

```bash
spar run --stage candidate --base <base commit> --head <candidate commit> \
  --criteria docs/specs/<goal>/SPEC.md --rules AGENTS.md --out docs/reviews/<goal>/02-candidate
```

- Caps: one plan round, one plan-delta round and three code rounds (candidate and fixes together) per series, unless the project configuration says otherwise. `spar run` refuses a round past its cap before any reviewer starts.
- The cumulative round reviews the whole change once more, from the goal's base commit to the last head, after fix rounds: a `fixes` round sees only the fixes. Run it as a `candidate` round; it does not take earlier findings and counts toward the code-round cap like the first candidate round. When no fixes followed the first candidate round, that round is the cumulative round. When the cap leaves no room for it, extending the cap follows the rule below.
- Only the user can allow more rounds. Ask them, then record their words: `spar extend <series> --stage plan|plan-delta|code --by N --user-quote "<their words>"`.
- A rerun of the same stage, base, head and packet is not counted only when it starts reviewers that did not complete that packet, for example after a quota refusal. Anything else counts.
- Pass `--rules` for the repository rules (such as `AGENTS.md`) and contracts the change must keep, and `--question` for a question you need answered. Do not pass your own reasoning about the solution; the reviewers start from the criteria.

## Reviewers

- `claude`, `codex` and `agy` run as read-only, offline CLIs that spar checks (`verified`). A reviewer that uses a forbidden tool or changes its workspace copy is recorded as `tool_violation` or `workspace_modified` and its findings are dropped.
- `agy` runs its designated model first and its fallback model once, in the same round and on the same packet, only when the designated model is refused for quota or rate limits. Tell the user when the fallback ran and when the quota resets.
- A `command` reviewer is an extra view: it is `unverified` and never counts as required.
- The `native` reviewer is a clean-context subagent of your host. `spar run` writes `native/prompt.md` and a workspace in the round folder (a native reviewer with another name gets a folder of that name). Start a subagent with no other context, only read access to that workspace and the instruction to read that prompt and reply with the JSON object it asks for. Then record the reply: `spar record <round> native --result <file>` or `--transcript <subagent transcript>`; transcript warnings stay in the record.
- A reviewer that ends `unavailable`, `timed_out`, `invalid_output` or `failed` is shown with its cause. It is never a round without findings. Rerun only that reviewer with `--reviewers <name>` when the cause is gone; a `--reviewers` list never removes the configured required reviewers from the gate.
- When a required reviewer did not complete a round and the user says to go on without it, record their words for that round: `spar waive <round> <reviewer> --user-quote "<their words>"`. Never record a waiver without the user's words. A waiver covers its round only; record one for each round the reviewer did not complete.

## Classes and dispositions

Spar classifies each finding by fixed rules; the reviewer's proposal is only an input.

- `defect`: violates a criterion, rule or contract the finding names and that exists, or is a regression this change made. A finding in a check path is a defect only when it claims a false pass, an inverted result meaning, or a linked contract or rule violation.
- `requirement-question`: only a change of requirement resolves it. You cannot change a requirement; ask the user. A finding in a check path stays one only when it proposes `requirement-question` and links the criterion, rule or contract it asks about, and that link exists; a `contract` link to a file in the check paths does not count.
- `hardening`: a check-path edge case that nothing requires, including a check-path requirement question without such a link. It is deferred automatically.
- `note`: everything else, including a defect proposal whose link does not exist.

Close findings with `spar triage <round> <finding> <disposition>`:

- `fix`: needs `--commit`, `--failed-before` and `--passed-after`, the fix commit and where the observation failed before it and passes after it.
- `reject`: needs `--evidence` that disproves the finding at the reviewed head.
- `defer`: needs `--to`; deferring a `defect` also needs `--user-quote`.
- `duplicate`: needs `--of` another finding of the same round; a `defect` or `requirement-question` points only at one of its own class and stays open until that finding is closed.
- `answer`: closes a `requirement-question` with the user's words.
- Every disposition that closes a `requirement-question` needs `--user-quote`, and fixing an automatically deferred `hardening` item in this change needs `--user-quote` too. Never put words in the user's mouth.

When findings on the same premise come back for two rounds in a row, stop fixing the list: test the premise against the criteria and the code, and ask the user when the premise is a reading of a requirement.

## The gate

`spar gate <round>… --expect-head <commit> --out <series>/gate.json` writes a `review-gate/v1` file in every state and exits 0 for `complete`, 3 for `degraded` and 4 for `blocked`.

- `complete` only when every required reviewer completed every round or holds the user's waiver for that round, at least one required reviewer completed each round's packet, the code rounds form a chain up to the expected head, and no `defect` or `requirement-question` is open.
- `degraded` when a required reviewer did not complete a round and holds no waiver for it, a round had no required reviewer, or no required reviewer completed a round's packet. Tell the user which reviewer and why; do not present the review as complete.
- A waived reviewer is not a `degraded` reason; the gate file lists it under its round's `waived_reviewers` with its status and the user's words. A round where no required reviewer completed stays `degraded` even with waivers. Report the waivers with the gate.
- `blocked` when a cap is exceeded, a finding is open, the chain is broken or the expected head was not reviewed.
- `growth` lists the lines review fix commits added, in check paths and elsewhere. `over_hardening` is set when the check side is larger: review the fixes for checks that grew past what the criteria need before adding more.

## Limits

- Spar's records are review records, not evidence of completion. A `complete` gate means the required reviewers the user did not waive found nothing open; it does not prove the criteria are met. Completion rules stay with whatever owns them, such as Seal's `ha done`.
- Reviewer agreement is not proof, and a reviewer's model or identity is reported by its CLI, not certified.
- Do not edit round records by hand. Use `spar triage`, `spar record`, `spar extend` and `spar waive`.
