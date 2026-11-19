# Spar rounds

## At the code cap

Each time the code cap is reached, run `spar cap-report <series>` before you ask the user for more rounds. Pick one cause from its signals and show the user the signals with that cause's recommendation:

- `churn`: fixes made new findings in the files they changed. Recommend to extend by 1 and fix again within each finding's `smallest_fix`.
- `recurring`: findings on the same link come back. Recommend not to extend; ask which reading of that requirement is meant.
- `unrelated`: the findings are not about this change. Recommend not to extend; defer them with the user's words and file them as issues.
- `hardening`: the checks grew more than the rest of the change. Recommend not to extend; trim the checks that grew past what the criteria need.
- `other`: none of these fits. Say why in `--reason`.

Record the code extension as [When to run each stage](../SKILL.md#when-to-run-each-stage) says, with `--cause` set to the cause you showed. `--by 2` or more, and `--cause other`, need `--reason`.

`spar cap-report` shows the code rounds counted against the cap, each round's findings by class, the open findings and four signals: churn (findings in files the fixes changed), recurring links, defects the reviewers marked as not introduced by the change, and check growth, which only recorded fixes rounds measure: in a series of plan and candidate rounds it stays 0, so judge the `hardening` cause from the fix commits. Churn is counted by file, so a finding about another line of a changed file counts. Recurring links are counted by link, so the same problem under different links is not grouped. Unrelated defects use each reviewer's `introduced_by_change` as reported. `ha cost` shows each extension's cause and reason below it, or that no cause was recorded.

## Reviewer standpoints

A standpoint points one reviewer at one risk of the change. Standpoints do not replace or change angles, and they do not change classification, dispositions, caps or the gate's state and reasons. When reporting the gate, name the standpoints in the gate file's `uncovered_standpoints`, with their rounds: those whose reviewer completed no gated round of the same packet, waived reviewers included. A rerun's completion covers the earlier round of its packet, so read this list, not each round's `standpoints_uncovered`.

### Choose reviewer standpoints

In every plan and candidate round, the cumulative round included, pick the change's main risks. Give each reviewer a different standpoint, one each; when there are fewer risks than reviewers, leave the rest without one. A candidate round after an earlier code round receives the earlier code rounds' findings with their last dispositions in its packet, so do not turn those findings into standpoints: pick risks of the whole change. A standpoint names the risk to attack, where to look first and the position from which to see it. Do not claim that the fix is right or explain the solution. Pick risks from a changed contract boundary, a stored format and its earlier records, the meaning of a check's result, or another place that keeps the same invariant.

### Pass standpoints

Pass `--standpoint NAME=TEXT` to `spar run`. The first `=` separates the reviewer name from the text; the option may be repeated. Refused (65): a name outside this run's configured reviewers and `--reviewers`, more than one standpoint for a reviewer, text empty after trimming whitespace, or invalid UTF-8. Names are compared exactly and case-sensitively, as in G20. A missing `=` or an empty name is a usage error (64). Both refusals happen before any reviewer starts or any round folder is created; the message names the offending name or value. Other text is kept unchanged.

The accepted names are the union of the configured reviewers and the names selected by `--reviewers`. Standpoints are run arguments only, with no user or project configuration key; `spar config` is unchanged.

### Rerun reviewers of a round with standpoints

Standpoint text and assignments are in the packet, so `packet_sha256` identifies them as part of the same input; changing either makes a new input. A configured reviewer omitted from a run may still have a standpoint in the packet. When you rerun only reviewers that did not finish, pass the full earlier list of standpoints in the same order: the stage, base, head and `packet_sha256` then stay unchanged, and the rerun is not counted under the rerun rule of [When to run each stage](../SKILL.md#when-to-run-each-stage).

### Standpoint delivery and reports

The shared packet's `## Standpoints for this round` section contains all standpoints and their reviewer assignments. Each reviewer also receives an instruction identifying its own standpoint and asking it to attack that risk while still checking every angle; an unassigned reviewer is told to review the whole change.

- `claude` and `codex` receive the instruction and packet on standard input.
- `agy` receives the command note, instruction and packet in `-p`; for a large packet, `PACKET.md` holds the packet and `-p` holds the command note, instruction and file reference.
- `command` receives the reviewer's `input.md` through `{prompt}`; `native` receives the instruction after its fixed instructions in `prompt.md`.

For a round with standpoints, both a real run and `--dry-run` leave in `<round>/<reviewer>/input.md` exactly the input that reviewer receives. For `agy` with a large packet, `input.md` holds its `-p` text and the packet is in `PACKET.md`.

`standpoint_report` is an object with two fields. `checked` is a boolean and `detail` is nonblank text: what was checked when true, or why it could not be checked when false. For an assigned reviewer, a missing, null or malformed report is `invalid_output`, with `standpoint_report` in the reason. An unassigned reviewer need not report one; its `standpoint_report` is not checked and is recorded as `null`. Every preset uses the same check in `spar run` and `spar record --result` or `--transcript`.

`round.json` keeps `standpoints` as reviewer/text pairs, `standpoints_uncovered` as reviewer names without a completed reviewer, and each reviewer's `standpoint` and `standpoint_report` as text/report or null. `summary.md` shows each reviewer's standpoint and report, and standpoints without a completed reviewer, including waived reviewers. The schema remains `review-round/v1`; records without standpoint fields mean no standpoints.

Without standpoints, reviewer inputs, `schema.json`, `packet_sha256` and `summary.md` remain byte for byte unchanged; no standpoint instruction or `input.md` is added. Only `standpoints: []` is added to `round.json`.

## Reviewer models and effort

A `model` can be set for `claude`, `codex` and `agy`, and an `effort` for `claude` and `codex`. Where the user's configuration leaves one open, choose it for a run with `--model NAME=MODEL` and `--effort NAME=EFFORT` on `spar run`; without them the preset's values run. Check `spar config` before you choose: `model_source` `user` means the user fixed the model, `default_model` is what runs without your choice, and `allowed_models` or `allowed_efforts` limit what you may pass.

spar refuses, before any reviewer starts, a value for a model or effort the user wrote, a model or effort for a `native` or `command` reviewer, an effort for `agy`, a name that is not a reviewer of the run, a second model or effort for the same reviewer, a value outside the allowed list, and one that is empty, is not valid UTF-8, starts with `-` or holds a space or control character. A model you chose falls back only to the `fallback_model` the user wrote.

After a round, `round.json` (`reviewers.<name>.model_source` and `effort_source`) and `summary.md` (`model <model> [<source>]`, `effort <effort> [<source>]`) show each reviewer's model and effort with its source (`user`, `agent`, `preset`).

## Decisions in later rounds

Each decision you pass with `--decision` and `--decision-quote` goes into the packet. The packet's `## Decisions the user made` section tells reviewers that spar does not check that the words are the user's, not to report what a decision settles, to report a change that goes beyond it, and, when following a decision breaks a criterion, rule or contract the decision does not address, to report a `requirement-question` that links it. `decisions` in `round.json` keeps them. An empty or unpaired decision is refused.
