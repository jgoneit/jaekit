# Contract: Core capabilities and the Seal compatibility check

[한국어](core-capabilities.md) · **English**

> This is a translation of the [Korean original](core-capabilities.md). Where the two differ, the Korean original decides.

This contract applies to Core `0.1.8` and Seal `0.1.23` of v0.1.8. The capability query was introduced in v0.1.3, and the earlier release `v0.1.2` does not have this command. Even after an update, the capabilities of the actual executable are checked, and compatibility is never judged from version numbers alone. Spec writes documents without a Core installation or this query.

## 1. Query without a goal

```text
ha capabilities --format json
```

Without `--format`, it also prints JSON. It accepts no goal argument and no format other than JSON. It prints what this executable supports to stdout as one JSON object followed by a newline. It does not open or change goals, repositories, configuration or run records, and it starts no Git, check or model process. The query output of the same binary does not vary with the current time or the working folder.

Exit codes are `0` for success, `64` for invalid arguments and `70` for an output failure. An older Core's unknown-command exit `64` means the query is unsupported, and a successful version print never passes in its place. Other failures, timeouts and unreadable output are not a successful support check either.

```json
{
  "schema": "ha-capabilities/v1",
  "ha_version": "0.1.8",
  "supported_rules": ["run-rules/1", "run-rules/2", "run-rules/3", "run-rules/4", "run-rules/5"],
  "default_rules": "run-rules/3",
  "formats": {
    "criteria": ["legacy", "nested/1"],
    "check_declaration": ["check-declaration/v1", "check-adapter/v1"],
    "check_result": ["check-result/v1"],
    "check_evidence": ["check-evidence/v1"],
    "finding": ["run-finding/v1"],
    "decision_history": ["decision-history/v1"],
    "timing": ["timing/v1"],
    "assurance_tier": ["light", "standard", "strict"],
    "core_requirements": ["core-requirements/v1"],
    "review_gate": ["review-gate/v1"]
  },
  "features": [
    "estimate/v1", "dirty-preflight/v1", "baseline-safe-copy/v1",
    "structured-change-results/v1", "budget-change/v1",
    "post-completion-findings/v1", "decision-history/v1", "run-timing/v1",
    "test-authority/v1", "reported-wait/v1", "preapproved-runs/v1",
    "error-recovery/v1", "command-time-limits/v1", "runner-adapters/v1",
    "report/v1", "doctor/v1", "auto-budget/v1", "core-requirements/v1",
    "review-gate/v1"
  ]
}
```

| Field | Meaning |
| --- | --- |
| `schema` | Exactly `ha-capabilities/v1`. Another schema is not interpreted by this contract |
| `ha_version` | The non-empty version identifier of the queried Core. Features are never inferred from a numeric comparison alone |
| `supported_rules` | The list of rules supported for stored goals |
| `default_rules` | The rules a new `start` selects. Must be in `supported_rules` |
| `formats.criteria` | `legacy` is the existing documents without a selector, `nested/1` the new list format selected explicitly |
| `formats.check_declaration`, `check_result`, `check_evidence` | The lists of supported schemas for declarations, producer reports and stored summaries |
| `formats.timing` | The list of schemas of the timing view (`ha timing --format json`). Optional, and may be absent |
| `formats.assurance_tier` | The list of values read and validated from the `Assurance:` line of goal documents (`light`, `standard`, `strict`). Optional, and may be absent |
| `formats.core_requirements` | The list of schemas of a skill's Core requirement declaration that `ha start --skill` reads (`core-requirements/v1`, §8). Optional, and may be absent |
| `formats.review_gate` | The list of review gate file schemas that `ha note <goal> review --gate <file>` reads (`review-gate/v1`, §9). Optional, and may be absent |
| `features` | The list of feature identifiers below |

Every field above is required except `formats.timing`, `formats.assurance_tier`, `formats.core_requirements` and `formats.review_gate`, which are marked optional. Lists are arrays of strings without duplicates, and each string is non-empty. An empty format or feature list means there is no such support. Duplicate JSON keys, wrong types, missing required information, and corrupt or trailing JSON cannot be interpreted. Additional fields and new list items in the same schema are allowed, but they do not replace a required support the consumer knows.

| Feature | Promise |
| --- | --- |
| `estimate/v1` | An estimate, before the start and without changing records, of the minimum verification run count |
| `dirty-preflight/v1` | Refusal before the actual check targets run, distinguishing a dirty state from a query failure |
| `baseline-safe-copy/v1` | In the baselines still to run under every supported rule, a check of the full copy list per target, and refusal of non-regular files in HEAD and of destination or parent symlinks before running |
| `structured-change-results/v1` | For a `/3` change, structured classification linked to the call and preservation of the full attempt history |
| `budget-change/v1` | For `/3`, a change of the total run limit of the current window, preserving the usage and time limits and refusing revision conflicts |
| `post-completion-findings/v1` | An optional extension that preserves finding claims, revisions and rework relations linked to a historical completion |
| `decision-history/v1` | An optional extension that provides registration, relations and queries of a decision history kept apart from run records (§5) |
| `run-timing/v1` | An optional extension that provides recording of `timing` measurements on `check` and `baseline` lines and a read-only timing view (`ha timing`) |
| `test-authority/v1` | `run-rules/4`'s test change authority, the copy of the scope at start, and `test-change` evidence (§6) |
| `reported-wait/v1` | `run-rules/4`'s `wait` and `resume` reports, `started_at`, and budget elapsed time that excludes reported waits (§6) |
| `preapproved-runs/v1` | `run-rules/4`'s pre-approved limit and increases within it that need only a reason (§6) |
| `error-recovery/v1` | `run-rules/4`'s `recover` and `allow-recovery`, and the display of recovery counts (§6) |
| `command-time-limits/v1` | `run-rules/4`'s PLAN `## 시간 제한`, the limit applied per command digest, and notice before running (§6) |
| `runner-adapters/v1` | An optional extension that classifies `go test -json` standard output and JUnit XML directly through `check-adapter/v1` declarations of `/3`, `/4` and `/5` change conditions (§7) |
| `report/v1` | A report for people (`ha report`, [Run record](run-record.en.md) §1.3) that, without changing records and by the same computation as `ha status`, shows a five-line summary, a stamp, the evidence per condition and the completion coverage |
| `doctor/v1` | An execution environment check (`ha doctor`, [Run record](run-record.en.md) §1.4) that runs the check commands of PLAN `## 실행 환경` with a fixed 60-second limit and records nothing |
| `auto-budget/v1` | Computation of the start limit for PLAN `검증 실행: 자동`, the pre-approved limit in user configuration, refusal of a start above that limit, and its display in `ha estimate` (§8, [Run record](run-record.en.md) §1.5) |
| `core-requirements/v1` | The check of a skill's Core requirement declaration in `ha start --skill`, and `--check-skill`, which makes the declaration required (§8) |
| `review-gate/v1` | `run-rules/5`'s review gate record (`ha note <goal> review --gate <file>`) and the gate judgment for `standard` and `strict` goals (§9, [Run record](run-record.en.md) §4.8) |

A Core that supports the timing view provides `timing/v1` in `formats.timing` and `run-timing/v1` in `features` ([Run record](run-record.en.md) §1.2, §2.2). Measurements are not used for judgments or budgets, so this extension is not added to the requirements of a new start or of resume, check, budget and finding operations on existing goals. On a Core without this support, ordinary runs stay the same; only the timing view cannot be used.

A Core that reads and validates the `Assurance:` line of goal documents ([Goal documents](goal-docs.en.md) §2.5) provides `light`, `standard` and `strict` in `formats.assurance_tier`. Releases from v0.1.7 on (the example above is v0.1.8) have this item, and releases v0.1.6 and earlier do not. The tier sets the scope of the review the implementer receives. For goals started under `run-rules/4` or earlier, judgments, the output of `ha status` and `ha done`, and budgets do not change because of the tier; for `standard` and `strict` goals started under `run-rules/5`, a recorded review gate becomes a condition of completion (§9). The line is part of SPEC.md, so it is included in the goal digest; when a valid line is present, an `assurance tier: <등급>` line is added to the success output of `ha lint`, and a malformed line blocks `ha start` and `ha check` like other lint errors ([Goal documents](goal-docs.en.md) §2.5, [Execution bundle](bundle.en.md) §8). So this item is not added to the requirements of a new start or of resume, check, budget and finding operations on existing goals. A Core without this item only does not validate the line, and ordinary runs stay the same. The name of this item was chosen separately so that it is not confused with the `assurance` field of `ha status` (the evidence display, always `local`).

The detailed meaning of the features follows [Run record](run-record.en.md), [Result contract](check-result.en.md) and [Goal documents](goal-docs.en.md). Capabilities are not independent certification or proof of code integrity.

## 2. Consumption contract of Seal 0.1.17

The compatibility check is done before any operation that changes state. Settle the absolute path of the executable that will actually be used, and query with **that path**. If the executable that succeeded and the Core used afterward differ, the earlier check is not reused. It is not replaced by another file on PATH, another session's result or the version string alone. If the path or the binary changed even after the check, check again. The query grants no authority to install automatically, widen permissions, or change goals or records.

The agent can compare the JSON above directly with the table below. Python or a separate compatibility check program is not a required dependency of Seal.

| Request | Rules to select and support needed |
| --- | --- |
| New start | `default_rules` must be `run-rules/3` and must also be in the supported list. `legacy` and `nested/1` of `criteria`, each v1 of declarations, reports and summaries, and all five run features above other than the post-completion findings extension are needed. From Seal 0.1.16, `run-rules/4` in the supported list and the five of §6 in the features are also required, and it starts with `ha start --rules run-rules/4`. From Seal 0.1.21, instead, `run-rules/5` and the two items of §9 are also required, and it starts with `ha start --rules run-rules/5` |
| Resuming or checking an existing goal | The `/1`, `/2`, `/3`, `/4` or `/5` stored in `start` and the criteria format the goal selected must be supported. `dirty-preflight/v1` and `baseline-safe-copy/v1` are needed. When the stored rules are `/3`, `/4` or `/5`, declaration, report and summary v1 and `structured-change-results/v1` are also needed; for `/4` and `/5`, the five features of §6 too, and for `/5`, the `review-gate/v1` feature of §9 too |
| Changing the run limit of an existing goal | The stored rules must be `/3`, `/4` or `/5`, and `budget-change/v1` must be supported. The goal's criteria format and the declaration, report and summary formats must also be supported; for `/4` and `/5`, the five features of §6 are also needed, and for `/5`, the `review-gate/v1` feature of §9 too. This operation does not apply to `/1` and `/2` |

A new start does not silently substitute an unknown default rule or an earlier default rule. The default start rule does not apply to existing goals. For example, even if the default is a future `/4`, the check for an existing `/2` can pass when the stored `/2` and the support the request needs are present. New `/3` support does not require converting `/1` or `/2` goals or records, or adding result declarations.

When the query is unsupported or fails, required information is missing or cannot be interpreted, or a needed rule, format or feature is missing, report the target checked and the reason, and stop before any state change such as a new start or resume. For an older version, point to the [installation guide](../guides/INSTALL.en.md#update), but do not install or update automatically or work around it with older rules.

From Seal 0.1.20, this table is not compared by hand. A new start declares the needed support in `core-requirements.json` (§8) in the skill folder and starts with `ha start --check-skill`, so when any support is missing, Core refuses without a record. Along with the new start requirements above, that declaration contains `report/v1`, `doctor/v1`, `auto-budget/v1`, `core-requirements/v1`, `core-requirements/v1` of `formats.core_requirements`, and `light`, `standard` and `strict` of `formats.assurance_tier`. The declaration of Seal 0.1.21 changes the rules to `run-rules/5` and adds the `review-gate/v1` feature of §9 and `formats.review_gate`. For existing goals, each command computes the stored rules or refuses with 66, and Seal reports that refusal and the executable and stops. The absolute path of the executable is not checked again. Each line of the record keeps `ha_version`, and `ha status` shows it.

The compatibility check does not replace input lint, the integrity of stored records, the user's implementation authority or the completion judgment. Seal performs the checks of this table. When a skill has the declaration of §8, Core compares that declaration with this executable's capabilities at `ha start --skill` and refuses the start without a record when any support is missing; it accepts the start of a skill without a declaration as before. An undeclared `/3` change that uses Core directly can, as before, record the actual attempt and classify it as `unknown`. Ordinary maintain, manual and task checks are not forced over to the structured change contract.

## 3. Optional deterministic consumer tool

If Python 3 is already present, the same contract can be checked with [check_core.py](https://github.com/jgoneit/jaekit/blob/v0.1.8/plugins/seal/skills/seal/scripts/check_core.py), included in Seal. Without the tool, or without Python, the contract above can be consumed directly.

```text
python3 <seal-skill>/scripts/check_core.py --ha <실제 Core 경로> --operation start
python3 <seal-skill>/scripts/check_core.py --ha <실제 Core 경로> --operation resume --goal <기존 목표>
```

The tool of Seal 0.1.20 checks all requirements of `core-requirements.json` for `start`. So when it judges a new start compatible for the same executable, the requirement check of `ha start --check-skill` does not refuse either. The judgments for `resume`, `check`, `budget`, `finding` and `history` stay the same.

`--ha` defaults to `ha` and uses the first executable found on PATH. `--operation` is `start` (default), `resume`, `check`, `budget` or `finding`. `start` takes no goal, and the other requests take an existing goal. The tool reads the existing goal's first `start`, the record schema and the SPEC format selector, and leaves the verification of actual record integrity to Core.

The tool queries once with the absolute path with symlinks resolved. If the SHA-256 differs before and after the query, it refuses. On success, the `executable` path of the response is also used for the actual Core calls. The hash is not a guarantee against replacement for a permanent cache or for uncooperative external processes.

stdout is `seal-core-compatibility/v1` JSON. It provides `compatible`, `reason`, `requested_executable`, `executable`, `sha256`, `ha_version`, `operation`, `rules` and `missing`. Values not checked yet are null, and it does not copy raw Core output, goal content or user utterances. The executable path is local diagnostic information, so it need not be carried as it is into public reports.

- Exit `0`: the compatibility check succeeded.
- Exit `2`: the check refused. It gives one of the reasons `core_missing`, `query_unsupported`, `query_failed`, `query_timeout`, `binary_changed`, `unsupported_schema`, `invalid_capabilities`, `unsupported_default_rule`, `unsupported_stored_rule`, `unsupported_record_schema`, `unsupported_operation`, `missing_support`, `goal_unreadable`, `goal_record_oversized`, `goal_format_invalid`. A stored finding line over the [contract limit](findings.en.md#input-and-queries) is `goal_record_oversized`, an unreadable or corrupt record is `goal_unreadable`, and an unknown record schema is `unsupported_record_schema`.
- Exit `64`: a tool argument error.

The query is stopped after 5 seconds and is not retried automatically. This tool does not run an actual start, resume, check or installation, and does not save results to a file. Whether installed Codex and Claude actually follow these instructions is a subject of separate host verification.

## 4. Optional post-completion findings extension

From Core 0.1.4, Core includes [post-completion findings](findings.en.md) and provides `run-finding/v1` in `formats.finding` and `post-completion-findings/v1` in `features`. The v0.1.8 release example above also has both items. The v0.1.3 Core does not have this extension, so the actual capabilities are checked, and features are never inferred from version numbers alone. The requirements of the existing start, check and budget operations stay. For finding input and queries, the two additional supports of the actual executable and support for the stored rules and the current condition format are checked. Run features or `/3` declaration support are not required for finding operations alone.

The optional tool included in Seal 0.1.12 supports `--operation finding --goal <기존 목표>`. When an existing goal has `run-finding/v1` lines, resume, check and budget also require the two additional supports. The tool reads the presence of known line schemas, and Core verifies the integrity of hashes and history. Unknown schemas are refused. It is not assumed that the tool of an earlier installation has the new argument, and checking the actual capabilities directly is also valid. When the support cannot be confirmed, no new finding record is written.

## 5. Optional decision history extension

A Core that supports [decision history](decision-history.en.md) provides `decision-history/v1` in `formats.decision_history` and `decision-history/v1` in `features`. This extension is provided from release v0.1.5 (`ha 0.1.5`) and is absent in v0.1.4 (`ha 0.1.4`). For registering and querying history, the two additional supports of the actual executable and support for the stored start rules and the current condition format are checked. Decision history is outside `runs.jsonl`, so this extension is not added to the requirements of a new start or of resume, check, budget and finding operations on existing goals. Without the support, no history is written, and the handoff goes through the existing Markdown.

The optional tool included in Seal 0.1.13 does the same check with `--operation history --goal <기존 목표>`. It is not assumed that the tool of an earlier installation has this argument, and checking the actual capabilities directly is also valid.

The macOS and Linux executables of the current source and the Windows x64 local candidate provide the two decision history items with the same meaning. On Windows too, registration, resend, correction, the default/current view, list and both forms of show are covered; it is not a form that has only the capabilities while direct calls are all unsupported. This feature advertisement does not mean a public Windows release or acceptance of Windows 11 installation and host use. The platform and repository scope follow the [decision history contract](decision-history.en.md#1-support-and-compatibility) and the [Windows candidate guide](../guides/WINDOWS.en.md).

## 6. run-rules/4 features

A Core that supports `run-rules/4` provides `run-rules/4` in `supported_rules` together with `test-authority/v1`, `reported-wait/v1`, `preapproved-runs/v1`, `error-recovery/v1` and `command-time-limits/v1` in `features`. `default_rules` stays `run-rules/3`. So earlier Seal versions that check the default rules, and the release and installation identification, do not change. The new rules are selected only with `ha start --rules run-rules/4`. The meaning of the five features is in [Run record](run-record.en.md) §4.7.

From Seal 0.1.16, the check tool requires these rules and all five features for a new start and gives `run-rules/4` as the start rules. When any of them is missing, it refuses before recording. Resuming, checking and changing the run limit of goals whose stored rules are `/4` also require the five features. The requirements of `/1`, `/2` and `/3` goals stay the same. A Core that does not support `/4` refuses that goal with 66 and does not record.

## 7. Runner adapters

A Core that supports runner adapters provides `check-adapter/v1` added at the end of `formats.check_declaration` and `runner-adapters/v1` added at the end of `features`. The meaning of both items is in §5 of [change check results](check-result.en.md#5-runner-adapters). The existing items, their order and `default_rules` stay the same.

Adapters are optional, so this extension is not added to the requirements of a new start or of resume, check, budget and finding operations on existing goals. The requirements of Seal and of the compatibility check tool also stay the same. The executor writes adapter declarations only when both items are in the actual executable's capabilities. A Core without the support treats an adapter declaration as an unknown declaration and refuses the start and checks as a lint problem (`evidence_invalid`) without recording. In that case, use the reference producer or a dedicated producer.

## 8. A skill's Core requirement declaration

A skill can declare the Core support it needs in `core-requirements.json` in the same folder as SKILL.md. `ha start --skill <SKILL.md>` reads this declaration and compares it with the executable's capabilities (the same values as `ha capabilities` in §1) ([Run record](run-record.en.md) §1.6). A Core that supports this provides `core-requirements/v1` in `formats.core_requirements` and `core-requirements/v1` in `features`.

```json
{
  "schema": "core-requirements/v1",
  "rules": ["run-rules/4"],
  "formats": {"criteria": ["legacy", "nested/1"], "check_result": ["check-result/v1"]},
  "features": ["estimate/v1", "test-authority/v1"]
}
```

| Field | Meaning |
| --- | --- |
| `schema` | Required. Exactly `core-requirements/v1` |
| `rules` | Optional. Each value must be in `supported_rules` |
| `formats` | Optional. For each key, that key must be in `formats` (even when its list is empty), and each value must be in that list |
| `features` | Optional. Each value must be in `features` |

- Lists are arrays of non-empty strings, and values do not repeat within one list. `formats` is an object whose values are such lists. An empty list or an absent field means no requirement of that kind. However, a key of `formats` requires the key itself even when its list is empty.
- It is one UTF-8 JSON object of 64 KiB or less. One leading BOM is allowed. Field names must be spelled exactly; a name that differs only in case is an unknown field. Another schema value, unknown fields, duplicate keys, trailing data, `null` and wrong types are format violations. Unreadable entries, such as a symlink without a target or a folder, are also refused.
- When items are missing, it shows `ha: missing Core support: <rules|formats.<키>|features> <값> (ha <버전>)` for each item (just `formats.<키>`, without a value, when the `formats` key itself is missing and its list is empty); for a format violation it shows the reason; both end with 65 without a record. It never ignores unknown values to start.
- A skill without a declaration file, and a start without `--skill`, do not do this check. Seal 0.1.20 has the declaration and starts with `--check-skill`. Seal 0.1.17 to 0.1.19, which do not declare, start as they do now, and those Seal versions' §2 check and [check_core.py](https://github.com/jgoneit/jaekit/blob/v0.1.6/plugins/seal/skills/seal/scripts/check_core.py) allow the added items, so they reach the same conclusion.
- `--check-skill` makes the declaration required. A Core from before this feature (release v0.1.6 and others) ends with 64 on this flag as an unknown flag and does not record, so a start where the check must happen uses the form `ha start <goal> --skill <SKILL.md> --check-skill`.
- The declaration is only a requirement of Core support. It does not replace the user's implementation authority, input lint or the completion judgment.

`auto-budget/v1` means that the start limit computation for PLAN `검증 실행: 자동` is provided ([Run record](run-record.en.md) §1.5). The two features are optional, so they are not added to the requirements of a new start or of resume, check, budget and finding operations on existing goals in the §2 table. A skill that uses them lists them in its §8 declaration.

## 9. run-rules/5

A Core that supports `run-rules/5` provides `run-rules/5` in `supported_rules`, `review-gate/v1` in `formats.review_gate` and `review-gate/v1` in `features` together. `default_rules` stays `run-rules/3`, and the new rules are selected only with `ha start --rules run-rules/5`. `/5` keeps all the rules of `run-rules/4`, so it also provides the five features of §6.

In a `/5` goal, the executor records the `review-gate/v1` file Spar wrote with `ha note <goal> review --gate <file>`, and when the SPEC's `Assurance:` tier is `standard` or `strict`, the goal completes only when the last recorded gate covers the current HEAD and the goal documents. The recorded content, refusals and reasons are in [Run record](run-record.en.md) §4.8. Core reads only the public format of the gate file; it does not run Spar or call reviewers or models.

Seal 0.1.21 declares `run-rules/5`, the `review-gate/v1` feature and `review-gate/v1` of `formats.review_gate` in `core-requirements.json` (§8) and starts with `ha start <goal> --rules run-rules/5 … --check-skill`. The check tool also requires these three for a new start and gives `run-rules/5` as the start rules, and when any of them is missing it refuses before recording. A Core that provides the `review-gate/v1` feature also provides `review-gate/v1` of `formats.review_gate`, so resuming, checking and changing the run limit of goals whose stored rules are `/5` require the five features of §6 and the `review-gate/v1` feature. The requirements of `/1` to `/4` goals stay the same. A Core that does not support `/5` refuses that goal with 66 and does not record.
