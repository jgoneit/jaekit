# Contract: review-gate/v1

[한국어](review-gate.md) · **English**

> This is a translation of the [Korean original](review-gate.md). Where the two differ, the Korean original decides.

`spar gate` judges from a list of rounds and an expected head whether the review is finished, and writes a `review-gate/v1` file. This file is a review record, not evidence of completion. Completion rules are set by the side that owns them (for example Seal's `ha done`). The example below is synthetic.

## 1. Command

```bash
spar gate docs/reviews/example/02-candidate docs/reviews/example/03-fixes \
  --expect-head 3333333 --out docs/reviews/example/gate.json
```

- Rounds are read in the given order (G29). Every round in the series of the given rounds that is not a dry run must be in the list. A round that stopped before recording findings (with neither `finished_at` nor `findings.json`) is left out like a dry run.
- `--out` follows the record path rule (G28) of [Input formats](spar-input.en.md). Beyond that, it is refused when it is inside a given round folder or is the series' `extensions.jsonl`, and an existing file is overwritten only when it is a `review-gate/v1` file. When it is refused, nothing is written.
- When the arguments are valid, the file is written whatever the state. An unreadable round and an expected head that cannot be resolved are also recorded as `blocked` reasons.
- Exit codes: `complete` 0, `degraded` 3, `blocked` 4, usage error 64, record path refusal 65, internal error 70.

## 2. States

`blocked` takes precedence over `degraded`, and `degraded` over `complete`.

`blocked` reasons:

- Cap exceeded: for each series of the rounds, the rounds on disk and the given rounds that are `counted` are counted per stage group, and the count exceeds the sum of the cap in the configuration of the round with the largest `sequence` in that series and the extensions in `extensions.jsonl`. The same holds when the given rounds are earlier than that round
- Findings without a disposition: a `defect` or `requirement-question` with no disposition in the ledger. A `duplicate` is closed only when following the findings it points to reaches a closing disposition; when it reaches an open finding or the findings point to each other, it is open
- The fix commit of a finding disposed as `fix` is not contained in the expected head. Every `fix` is checked, whatever the class (including `hardening` and `note`)
- Broken code round chain: the base of a fixes round in the list is not the head of the code round right before it, or there is no code round before it. Only a round whose stage, base, head and packet digest all match the code round right before it (a rerun of the same packet) is left out of this check. When the packet differs, it is a new fixes round even if base and head are the same
- Expected head not reviewed: the head of the last code round (the last round if there is none) is not the expected head
- Dry run rounds, unreadable rounds, rounds without `findings.json` or `ledger.jsonl`, rounds whose `waivers.jsonl` cannot be read, commits that are not in the repository
- A round of the same series that is not in the list (G29): `<series>: round <이름> of the series is not in the gate list`. This keeps a gate from being made without earlier rounds and passing over open findings

`degraded` reasons:

- A required reviewer did not complete. Required reviewers are the reviewers the round ran as required and the reviewers its configuration sets as required (`config.configured_required`). Required reviewers of the configuration that were left out with `--reviewers` are included. A reviewer that completed as a required reviewer with the same name and preset in another round with the same stage, base, head and packet (a rerun) counts as completed. With the same name but a different preset, that required reviewer was not run. A reviewer that has a user decision (`spar waive`, [Review round](review-round.en.md) section 10) in that round's `waivers.jsonl` is not this reason and appears in that round's `waived_reviewers`.
- A round without required reviewers (for example, a round with only `command` reviewers)
- A round with reviewers to which a decision applies but in which no required reviewer completed: `<라운드>: no required reviewer completed this round`. A decision does not make a round without independent review `complete`. Completed required reviewers are the reviewers counted in `completed_required`, including reviewers that completed in a rerun with the same stage, base, head and packet.

Decisions are recorded only by quoting the user's words, are read only from that round folder's `waivers.jsonl`, and apply only to that round. A decision of another round does not apply, even for a rerun of the same packet. A decision applies to the required reviewer with the same name and preset; if that reviewer completed despite the decision (including in a rerun of the same packet), it counts as completed and is not put in `waived_reviewers`. A round without `waivers.jsonl` (including rounds recorded before this feature), and a round that ran a reviewer named `waivers.jsonl` (case-insensitive) before this feature so that its folder is in that place, are rounds without decisions, and that gate's state, reasons and fields are the same as without the decision feature.

`complete` only when there is no reason at all.

## 3. File

```json
{
  "schema": "review-gate/v1",
  "spar_version": "0.1.8",
  "state": "complete",
  "reasons": [],
  "expect_head": "3333333…",
  "last_reviewed_head": "3333333…",
  "previous_gate_digest": null,
  "rounds": [{
    "path": "docs/reviews/example/02-candidate", "stage": "candidate", "sequence": 2,
    "base": "1111111…", "head": "2222222…", "criteria_digest": "…",
    "criteria": [{"name": "docs/specs/example/SPEC.md", "sha256": "…"}], "packet_digest": "…", "config_digest": "…",
    "requested_reviewers": ["claude", "codex", "agy"], "required_reviewers": 3, "completed_required": 3,
    "dry_run": false, "counted": true
  }],
  "open": {"defect": 0, "requirement-question": 0},
  "open_findings": [],
  "auto_deferred": 2,
  "growth": {"added_lines": {"check": 40, "other": 12}, "over_hardening": true, "rounds": []},
  "written_at": "2026-01-01T00:00:00Z"
}
```

| Field | Meaning |
| --- | --- |
| `state`, `reasons` | The state and the reasons. `blocked` reasons come first |
| `expect_head` | The commit ID of the expected head |
| `last_reviewed_head` | The head reviewed last |
| `previous_gate_digest` | The sha256 of the bytes of the previous gate file at the same `--out` path, or `null` when there was none |
| `rounds[]` | The round's `criteria_digest`, packet digest (`packet_digest`), configuration digest (`config_digest`), the requested reviewer names and the number of reviewers accepted as required (`required_reviewers`), and how many of them completed (`completed_required`, including those that completed in a rerun with the same stage, base, head and packet) |
| `rounds[].criteria` | For each criteria document the round received, in the order of `round.json`: `name` (the repository-relative path when inside the repository, the file name when outside; the name with symbolic links resolved) and the `sha256` of its content. The original path is not included. `ha`'s `run-rules/5` uses this to judge the correspondence to the goal documents ([run-record.md](run-record.en.md) §4.8) |
| `rounds[].waived_reviewers` | For each required reviewer to which a decision applied: `name`, `preset`, `status` (the status in `round.json`, or `not_run` when the round did not run a reviewer with that name and preset), `reason` (the cause in `round.json`, omitted when there is none), `user_quote`, `recorded_at` (when the decision was recorded). Not counted in `completed_required`. Absent in a round with no applied decision |
| `open` | The number of open `defect` and `requirement-question` findings |
| `open_findings` | The round and ID of each open finding |
| `auto_deferred` | The number of `hardening` findings whose last disposition is an automatic deferral. A `hardening` whose location cannot be read has no ledger entry and is not counted |
| `growth.added_lines` | For each fixes round in the list, the added lines of `git diff --numstat -M <base> <head>` (for renamed files, only the lines added at the new path), split into that round's configured check paths (`check`) and everything else (`other`), and summed. The same base and head are counted once |
| `growth.over_hardening` | `true` when `check` is larger than `other` |
| `growth.rounds[]` | `check` and `other` for each fixes round |

`growth` is a record that lets a reader see whether review has inflated fixes toward check code. `over_hardening` does not replace judgment and does not change the state.
