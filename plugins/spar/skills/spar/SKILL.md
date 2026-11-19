---
name: spar
description: Run independent review rounds on a plan or a change with the spar binary — the same packet to every configured reviewer, classified findings, limited dispositions, stage caps and a review gate file. Use when the user asks for an independent or adversarial review of a plan or an implementation, for example "$spar:spar" in Codex, "/spar:spar" in Claude Code, or "이 변경을 spar로 리뷰해줘".
metadata:
  version: "0.1.9"
---

# Spar

Spar makes reviewers attack a plan or a change from outside your context. The `spar` binary pins two commits, builds one packet (criteria, rules, documents under review, check boundaries and the diff, without Git history, ignored files or commit messages), runs every configured reviewer on it in parallel, classifies their findings and keeps a ledger of what was done with each one. You run the rounds, decide what each finding means for the work, and record that decision. This skill states when to run rounds, how findings may be closed and what the records do not prove.

## Before the first round

- Check the executable: `spar --version`. If `spar` is missing, say so and stop; do not install tools on your own.
- `spar config --repo .` prints the reviewers a round will start, where that list came from, the stage caps, the angles and the check paths. Reviewers come only from the user's configuration (`$SPAR_CONFIG`, `$XDG_CONFIG_HOME/jaekit/spar.toml`, `~/.config/jaekit/spar.toml`, then the clean-review `reviewers` list, then `claude`, `codex`, `agy`). A repository's `.jaekit/spar.toml` sets only caps, angles, rules and check paths.
- The packet goes to each reviewer's provider; the `native` reviewer's provider is the one your own agent already uses. When the criteria or the code must not go to any other provider, use only the `native` reviewer and tell the user why. When they must not leave the computer at all, do not run a round: every reviewer, `native` included, sends the packet to a provider. Say so and stop.
- Records go to a new, Git-ignored folder per round. Rounds of one review share a parent folder, the series: `docs/reviews/<goal>/01-plan`, `docs/reviews/<goal>/02-candidate`, and so on.
- In a project that has never run Spar, Git does not ignore that folder yet, and `spar run` refuses it with exit 65 before any reviewer starts and writes nothing. The setup step is one line: add `docs/reviews/` to the project's `.gitignore`. It changes a project file, so ask the user before you add it. The Spar guide's "Set up a project" section covers another folder and `.git/info/exclude`.

## When to run each stage

| Stage | When | Command |
| --- | --- | --- |
| plan | Once, after the plan and its checks are written and before implementation | `spar run --stage plan` with `--subject` for the plan documents |
| candidate | On the commit you believe meets every criterion, and again after each set of review fixes | `spar run` with `--stage candidate`, the goal's base commit as base |

New rounds take only these two stages: `spar run` refuses `plan-delta` and `fixes` as a usage error that names `candidate`. Series recorded with such rounds are still read as recorded.

For example, a candidate round:

```bash
spar run --stage candidate --base <base commit> --head <candidate commit> \
  --criteria docs/specs/<goal>/SPEC.md --criteria docs/specs/<goal>/<linked goal document>.md \
  --rules AGENTS.md --check-path <check file> --out docs/reviews/<goal>/02-candidate
```

Give every goal document as `--criteria`: SPEC.md and each goal document it links, one flag per file. Give each check file the change adds or changes, such as result declarations, producers and tests, as `--check-path`, so that findings in them follow the check-path rules below. Give a check that only reads fixed text in a file, such as a document token test, as `--text-check` instead: it is a check path too, and the packet asks reviewers to judge whether it reads the right fixed text in the right file.

- Caps: one plan round and two code rounds per series, unless the project configuration says otherwise. `spar run` refuses a round past its cap before any reviewer starts. A new round in a recorded series is counted against the caps it records, and recorded fixes rounds count as code rounds.
- After review fixes, run one `candidate` round from the goal's base commit to the new head; it receives the earlier code rounds' findings with their last dispositions and counts toward the code-round cap like the first candidate round. The last such round that ends with no open `defect` or `requirement-question` is the cumulative round, so when no fixes followed the first candidate round, that round is the cumulative round. When the cap leaves no room for it, extending the cap follows the rule below.
- A `strict` goal takes one `plan` round before implementation. After it, the next `candidate` round reviews the current goal documents, and a change to the plan or checks alone needs nothing more. When SPEC.md or a goal document it links changed, Seal Core also needs the last `plan` round to have received them (`review_docs_stale`): ask the user to extend the plan cap (`spar extend <series> --stage plan --by 1 --user-quote "…"`) and run another `plan` round on the current head.
- Only the user can allow more rounds. Ask them, then record their words: `spar extend <series> --stage plan --by N --user-quote "<their words>"` (the stage `plan-delta` is still accepted for a series that recorded plan-delta rounds). Record a code extension only with the user's words and the cause you showed them: `spar extend <series> --stage code --by 1 --cause <cause> --user-quote "<their words>"`. Extend by 1 unless the user's words say more. Read [At the code cap](references/rounds.md#at-the-code-cap) each time the code cap is reached, before you ask.
- A rerun of the same stage, base, head and packet is not counted only when it starts reviewers that did not complete that packet, for example after a quota refusal. Anything else counts.
- Pass `--rules` for the repository rules (such as `AGENTS.md`) and contracts the change must keep, and `--question` for a question you need answered. Do not pass your own reasoning about the solution; the reviewers start from the criteria.
- Pass every decision the user made during the review to each later round, with or without Seal: `--decision` with what it settles, followed by `--decision-quote` with the user's words, one pair per decision. An answer to a `requirement-question` is one. Read [Decisions in later rounds](references/rounds.md#decisions-in-later-rounds) when you pass one, for what the packet tells reviewers.
- When a document states each check's boundary, what it observes and which inputs it does not cover, pass it with `--boundary` to every round; every stage takes it, and it may be repeated. The packet's `## Check boundaries` section tells reviewers to report a counterexample outside a stated boundary that no criterion, rule, contract or protected regression path requires as `hardening` with `check_impact` `none`, naming the boundary, and one on an input that a criterion, contract or protected regression path requires as a `defect`, whatever a boundary says. spar does not check that a boundary is right; `boundaries` in `round.json` keeps the documents.
- Give reviewers standpoints, the risks each attacks, in plan and candidate rounds. Read [Reviewer standpoints](references/rounds.md#reviewer-standpoints) before every plan or candidate round, including the cumulative round, and before you rerun reviewers of a round that had them.

## Reviewers

- `claude`, `codex` and `agy` run as read-only, offline CLIs that spar checks (`verified`). A reviewer that uses a forbidden tool or changes its workspace copy is recorded as `tool_violation` or `workspace_modified` and its findings are dropped.
- `agy`'s shell may join read-only programs with `|`, `&&`, `||` or `;`; running code, tests, builds, git or any other program is `tool_violation`. spar gives agy this rule in a command note at the start of its prompt.
- `agy` runs its designated model first and its fallback model once, in the same round and on the same packet, only when the designated model is refused for quota or rate limits. Tell the user when the fallback ran and when the quota resets.
- A reviewer's model or effort that the user's configuration writes is fixed. For one it leaves open, you may choose a value for a run. Choose for what the round has to examine, never to make the review easier to pass. When you report a round, say which model and effort you chose for each reviewer. Read [Reviewer models and effort](references/rounds.md#reviewer-models-and-effort) before you choose one.
- A `command` reviewer is an extra view: it is `unverified` and never counts as required.
- A reviewer may have a packet limit, `max_packet_bytes` in the user's configuration: 1,200,000 bytes for `claude` and `codex` and 4,000,000 for `agy` unless the user sets one, and none for `native` and `command`. `spar run` refuses a larger packet with 65 before any reviewer starts, dry runs included, and writes no round, so the cap is unchanged. Make the change or its documents smaller, or ask the user to raise the limit; do not edit the user's configuration yourself.
- The `native` reviewer is a clean-context subagent of your host. `spar run` writes `native/prompt.md` and a workspace in the round folder (a native reviewer with another name gets a folder of that name). Start a subagent with no other context, only read access to that workspace and the instruction to read that prompt and reply with the JSON object it asks for. Then record the reply: `spar record <round> native --result <file>` or `--transcript <subagent transcript>`; transcript warnings stay in the record.
- A reviewer that ends `unavailable`, `timed_out`, `invalid_output` or `failed` is shown with its cause. It is never a round without findings. Rerun only that reviewer with `--reviewers <name>` when the cause is gone; a `--reviewers` list never removes the configured required reviewers from the gate.
- When a required reviewer did not complete a round and the user says to go on without it, record their words for that round: `spar waive <round> <reviewer> --user-quote "<their words>"`. Never record a waiver without the user's words. A waiver covers its round only; record one for each round the reviewer did not complete.

## Classes and dispositions

Spar classifies each finding by fixed rules; the reviewer's proposal is only an input.

- `defect`: violates a criterion, rule or contract the finding names and that exists, or is a regression this change made. A finding in a check path is a defect only when it claims a false pass, an inverted result meaning, or a linked contract or rule violation.
- `requirement-question`: only a change of requirement resolves it. You cannot change a requirement; ask the user. A finding in a check path stays one only when it proposes `requirement-question` and links the criterion, rule or contract it asks about, and that link exists; a `contract` link to a file in the check paths does not count.
- `hardening`: a check-path edge case that nothing requires, including a check-path requirement question without such a link. It is deferred automatically.
- `note`: everything else, including a defect proposal whose link does not exist.

A reviewer may give a finding a `smallest_fix`: the smallest change that would make its failure scenario come out right, as a direction in words. It is advice; no class, disposition or gate rule reads it.

Close findings with `spar triage <round> <finding> <disposition>`:

- `fix`: needs `--commit`, `--failed-before` and `--passed-after`, the fix commit and where the observation failed before it and passes after it. Start the fix within the finding's `smallest_fix` direction when it has one; when the fix goes further, record why with `--note`.
- `reject`: needs `--evidence` that disproves the finding at the reviewed head.
- `defer`: needs `--to`; deferring a `defect` also needs `--user-quote`.
- `duplicate`: needs `--of` another finding of the same round; a `defect` or `requirement-question` points only at one of its own class and stays open until that finding is closed.
- `answer`: closes a `requirement-question` with the user's words.
- Every disposition that closes a `requirement-question` needs `--user-quote`, and fixing an automatically deferred `hardening` item in this change needs `--user-quote` too. Never put words in the user's mouth.

Record each disposition before the next round. In candidate packets, an earlier finding's line ends with its last disposition when one is recorded, and the packet asks reviewers to report such a finding again only with a failure scenario that the recorded disposition does not answer. A disposition is part of the packet: a candidate round run on the same base and head after a disposition changed is a new round that counts toward the cap.

**Repeated premises.** This rule applies to local rounds and to remote pull request reviews alike. After a premise was rejected with evidence or answered by the user, do not fix a finding on the same premise that brings no failure scenario the recorded disposition does not answer: record it as a duplicate of the earlier finding, or reject it citing that disposition. A finding with a new failure scenario is a new finding. When the same premise comes back that way twice, ask the user which reading of the criterion is intended, and wait for the answer. In `spar triage`, `duplicate --of` names a finding of the same round; for a finding of an earlier round, record `reject --evidence` citing that disposition.

## The gate

`spar gate <round>… --expect-head <commit> --out <series>/gate.json` writes a `review-gate/v1` file in every state and exits 0 for `complete`, 3 for `degraded` and 4 for `blocked`.

- `complete` only when every required reviewer completed every round or holds the user's waiver for that round, at least one required reviewer completed each round's packet, the last code round reviewed the expected head (with recorded fixes rounds in an unbroken chain), and no `defect` or `requirement-question` is open.
- `degraded` when a required reviewer did not complete a round and holds no waiver for it, a round had no required reviewer, or no required reviewer completed a round's packet. Tell the user which reviewer and why; do not present the review as complete.
- A waived reviewer is not a `degraded` reason; the gate file lists it under its round's `waived_reviewers` with its status and the user's words. A round where no required reviewer completed stays `degraded` even with waivers. Report the waivers with the gate.
- `blocked` when a cap is exceeded, a finding is open, a recorded fixes chain is broken or the expected head was not reviewed.
- `growth` lists the lines that recorded fixes rounds added, in check paths and elsewhere; with none, `growth.rounds` is empty and growth was not measured. `over_hardening` is set when the check side is larger: review the fixes for checks that grew past what the criteria need before adding more.

## Limits

- Spar's records are review records, not evidence of completion. A `complete` gate means the required reviewers the user did not waive found nothing open; it does not prove the criteria are met. Completion rules stay with whatever owns them, such as Seal's `ha done`.
- Reviewer agreement is not proof, and a reviewer's model or identity is reported by its CLI, not certified.
- Do not edit round records by hand. Use `spar triage`, `spar record`, `spar extend` and `spar waive`.
