# Contract: review-round/v1

[한국어](review-round.md) · **English**

> This is a translation of the [Korean original](review-round.md). Where the two differ, the Korean original decides.

`spar run` runs one round and leaves a `review-round/v1` record in the round folder. Reviewer results and finding dispositions accumulate only in this record, and this format is fixed so that other tools (later, `ha`) can read it. The accepted format of each input field and the result for input outside that format are set by the table (G1 to G33) in [Input formats](spar-input.en.md), and this document points to its rows. The commits, paths and sentences in the examples below are synthetic.

## 1. Command

```bash
spar run --stage candidate --base 1111111 --head 2222222 \
  --criteria docs/specs/example/SPEC.md --rules AGENTS.md \
  --out docs/reviews/example/02-candidate
```

| Argument | Meaning |
| --- | --- |
| `--stage` | `plan`, `plan-delta`, `candidate`, `fixes` |
| `--base`, `--head` | The two commits to compare. `--head` defaults to `HEAD` |
| `--out` | A new round folder. If it already exists, it is refused (checked again after the series is locked). Record path rule (G28): every Git repository that contains the path (the repository under review, another repository, a nested repository inside an ignored folder) must ignore it, with no tracked files under it; when there is a `.git` above the path but Git cannot judge that path (ownership refusal, a linked worktree without Git data, an empty `.git` folder and so on), it is refused |
| `--criteria` | Criteria documents. At least one is required. Names of document arguments: G26 |
| `--rules` | Rule and contract documents. The project configuration's `rules` (G23) are added |
| `--subject` | Documents to review in the plan stage |
| `--question` | A question asked in this round |
| `--check-path` | Check path globs for this run (G24). An invalid glob is refused before any reviewer starts |
| `--reviewers` | The list of reviewer names for this run (G25) |
| `--model` | `이름=모델`. This run's model for a `claude`, `codex` or `agy` reviewer whose user configuration does not write `model` (G25, [configuration contract](spar-config.en.md) section 1, "How the model and effort are decided"). Can be given more than once |
| `--effort` | `이름=강도`. This run's effort for a `claude` or `codex` reviewer whose user configuration does not write `effort` (G25). Can be given more than once |
| `--repo`, `--label` | The repository location, and a name to keep in the record |
| `--dry-run` | Builds only the packet and workspaces and starts no reviewer. The workspace stays in the round folder's `workspace/`. It does not count toward the cap, and the gate does not accept it |

The parent folder of a round folder is the series of one review. Caps, the previous head and previous findings are read from the other `review-round/v1` rounds of the same series. Record folders in other formats are neither read nor changed. The series folder itself is locked (no lock file is created), so each series has only one write at a time (G33). `spar run` holds the same lock from the cap decision until it finishes writing the round record, and `spar record`, `spar triage`, `spar extend` and `spar waive` hold it while they read and write records. When another write holds the lock, it says so on standard error and waits until that write ends. `spar gate` does not lock.

Exit codes: 0 the round record was written and some reviewer completed or is pending, 1 the round record was written but no reviewer completed or is pending, 64 usage error, 65 refusal (configuration error, tracked record folder, cap exceeded, broken chain, unreadable input), 70 internal error. A refusal happens before any reviewer starts and creates no round folder.

## 2. Packet and workspaces

- `packet.md` is the one text every reviewer receives. It contains the stage description, the two commit IDs and tree IDs, the attack angles and how to report, the full text of the criteria documents, rule documents and documents under review, the questions, the findings of earlier rounds in the same series, and the diff between the two commits. Commit messages, Git history, ignored files and the reviewer list are not in it. `packet_sha256` is the sha256 of this file.
- The earlier rounds' findings are those of the plan and plan-delta rounds for plan-delta, and those of the candidate and fixes rounds for fixes. The packet shows those findings by finding IDs that contain the reviewer name (`codex-F1` and so on). Rounds with the same stage, base and head are left out, so a rerun of the same input receives the same packet.
- A temporary workspace copy is made for each reviewer. `head/` and `base/` hold the tracked files of the two commits exactly as their blobs, with no `.git`. Symlinks are not created; each is a regular file holding its blob (the target path).
- `schema.json` is the JSON schema of the reviewer result.
- `claude` receives the packet on standard input, `codex` on standard input, and `agy` as the `-p` argument. When the packet exceeds the limit of one argument (120 KiB on Linux, 768 KiB elsewhere), `agy` receives `PACKET.md` with the same bytes at the top level of its workspace, and `-p` is a short instruction to read that file. The attempt record's `packet_transport` (`stdin`, `argument`, `file`) keeps this, and a change to `PACKET.md` is `workspace_modified`. `command` receives the path of `packet.md` in the `{prompt}` placeholder. The packet is sent to each reviewer's provider.

## 3. Reviewer result

A reviewer outputs one JSON object.

```json
{
  "reviewed_head": "2222222",
  "verdict": "blocking_findings",
  "angles_checked": [{"angle": "requirement-counterexample", "result": "checked the empty-name input"}],
  "angles_unchecked": [{"angle": "design", "reason": "out of time"}],
  "findings": [{
    "id": "F1", "proposed_class": "defect", "angle": "requirement-counterexample",
    "location": "head/app/greet.py:3", "claim": "An empty name gives a trailing comma.",
    "failure_scenario": "greet('') returns 'hello, '", "evidence": "head/app/greet.py:3",
    "link_kind": "criterion", "link": "AC-1", "introduced_by_change": "yes", "regression": "no",
    "check_impact": "none", "confidence": "high",
    "smallest_fix": "Drop the comma only when the name is empty."
  }],
  "limits": "did not run the code"
}
```

- The result's format is G1 (the object and top-level keys), G2 (`reviewed_head`), G3 (`verdict`), G4 (angle items), G5 (finding items) and G6 (`limits`). The same checks apply to the results of every preset. This includes `native` and `command` reviewers, whose provider does not enforce the schema. A violation is `invalid_output`, and the reason contains the name of the key at fault.
- Finding enumerations: `proposed_class` `defect|requirement-question|hardening|note`, `link_kind` `criterion|rule|contract|none`, `introduced_by_change` and `regression` `yes|no|unknown`, `check_impact` `false_pass|meaning_inverted|contract_violation|none`, `confidence` `high|medium|low`. The packet tells reviewers to number `id` in reporting order as `F1`, `F2`, …, but `id` only needs to be a string, as the schema says. A round's finding ID is `<리뷰어>-F<n>` (starting at 1), which spar assigns in the reporting order of the reviewer result, and the `id` the reviewer gave is kept as `reviewer_id`. So round IDs do not collide even when a reviewer name or `id` contains `-` or when `id`s repeat.
- `smallest_fix` is the smallest fix direction a reviewer writes for each finding: the smallest change that would make the failure scenario come out right, written as a sentence rather than a patch or code, or an empty string when it cannot be stated. The schema puts this key in `required` for providers that enforce values strictly, but spar's check (G5) also accepts results without the key. An empty string and a missing key both mean no direction, and a value that is not a string is `invalid_output`. A direction is the reviewer's suggestion, so it is not used for classes, dispositions or the gate (sections 6–8). spar does not judge whether a fix kept to the direction.
- How spar reads them: `location` is G7, `rule` and `contract` links are G8, `criterion` links are G9. A finding whose location cannot be read is not dropped from the result either.
- `could_not_review` is recorded as `failed`, not as completion.

## 4. Reviewer status

| Status | Meaning |
| --- | --- |
| `completed` | The result met the schema, head and angle requirements |
| `pending` | `native` is waiting for the reply of the host's subagent |
| `unavailable` | The CLI is missing, or it refused because of usage or rate limits or authentication |
| `timed_out` | No result within the time limit |
| `invalid_output` | No result, not JSON, or it broke the schema, head or angle requirements |
| `failed` | The executable could not run, a nonzero exit without a result, or `could_not_review` |
| `tool_violation` | It used a tool or command outside what is allowed. The result is not used |
| `workspace_modified` | `head/` or `base/` of the workspace copy changed. The result is not used |
| `not_run` | `--dry-run` |

The `findings` of a reviewer that did not complete is `null`, and the summary shows its status and cause. It is never turned into zero findings.

Protections of `verified` presets (G10, G11): `claude` runs with `-p --safe-mode --restricted --strict-mcp-config --tools Read,Grep,Glob --permission-mode dontAsk --no-session-persistence --disable-slash-commands` and stops when it sees another tool or MCP server. `codex` runs with `exec --ignore-user-config --ignore-rules -s read-only --ephemeral --skip-git-repo-check -c web_search="disabled"` and `--disable` for `apps`, `browser_use`, `browser_use_external`, `computer_use`, `in_app_browser`, `image_generation`, `memories`, `multi_agent`, `plugins` and `hooks`, and stops when it sees an item type outside what is allowed. `agy` runs with `--sandbox --disable-slash-commands` and allows only the file read, list and search tools and, among shell commands, read-only programs (`find`, `ls`, `grep`, `rg`, `cat`, `head`, `tail`, `wc`, `sort`, `uniq`, `cut`, `stat`, `file`, `diff`, `tree` and so on) and `sed -n` that only prints. `2>/dev/null` and `2>&1` are removed before checking; any other redirection, command substitution, shell operators other than `|`, and arguments that start with `-exec`, `-execdir`, `-ok`, `-okdir`, `-delete`, `--pre` or `--compress-program` are violations. This protection is the same as clean-review 0.2.1 (the file read tools of 0.2.0 plus `read_file`, which agy 1.4.3 uses). Beyond that, options with which an allowed program writes files (for example `sort -o`) are not blocked separately. File writes are left to `agy`'s `--sandbox`, and a change to `head/`, `base/` or `PACKET.md` is `workspace_modified`.

Fallback model: when the designated model is refused for usage or rate limits (refusal text: G13, [Input formats](spar-input.en.md) section 5) and `fallback_model` is set, the reviewer runs once more with the same packet and a new workspace copy. It is not used for authentication refusals or other failures. The only fallback for a model the agent chose with `--model` is the `fallback_model` the user wrote ([configuration contract](spar-config.en.md) section 1). The fallback run uses the same effort as the designated model's run. `attempts` keeps the model, status, cause and reset text of each attempt, and `fallback_used` becomes `true`.

## 5. Round folder

| File | Contents |
| --- | --- |
| `round.json` | The fields below |
| `packet.md`, `schema.json` | The packet and the result schema |
| `findings.json` | The array of classified findings. Each finding keeps the `smallest_fix` the reviewer gave as it is, or an empty string when there is none |
| `ledger.jsonl` | The disposition ledger. One disposition per line, append-only |
| `waivers.jsonl` | User decisions that drop a required reviewer (section 10). One decision per line, append-only. Absent in a round without decisions |
| `summary.md` | A summary for people to read. Findings with a direction appear after the findings table in a `## Smallest fixes` section as `- <지적 ID>: <방향>` (`smallest_fix` with leading and trailing whitespace removed; lines after a line break inside it are indented two spaces). Without findings that have a direction, the section is absent. A reviewer line shows the requested model and its source (`model <모델> [<출처>]`) and, for `claude` and `codex`, the effort and its source (`effort <강도> [<출처>]`; an effort the user left empty shows as `effort none [user]`). An earlier record without sources shows only the model, without a source, and no effort |
| `<리뷰어>/` | That reviewer's standard output and error, and its result. A fallback model's are in `<리뷰어>/fallback/` |
| `workspace/` | Only in a dry run: the workspaces the reviewers would receive (`head/`, `base/`) |
| `<native 리뷰어>/` | `prompt.md`, the workspace and the recorded result. `native/` with the default name |

Main fields of `round.json`: `schema` (`review-round/v1`), `spar_version`, `stage`, `label`, `series`, `sequence`, `repo`, `base`, `head`, `base_tree`, `head_tree`, `diff_sha256`, `criteria`, `rules` and `subjects` (`name`, `source`, `sha256`), `criteria_digest` (the sha256 of the criteria document names and their sha256s), `questions`, `prior`, `packet_sha256`, `config` (the record of applying the [configuration contract](spar-config.en.md)), `links` (the `criteria_ids` used for link checks and the rule documents' `rules[]`: `names`, `file_name`, `lines`), `requested_reviewers`, `required_reviewers`, `dry_run`, `counted`, `rerun_of`, `started_at`, `finished_at`, `reviewers`.

`reviewers.<이름>`: `name`, `preset`, `isolation` (`verified`, `host`, `unverified`), `required`, `status`, `reason`, `requested_model`, `model_source` (only `claude`, `codex` and `agy`: `user`, `agent`, `preset`), `observed_model`, `fallback_model`, `fallback_used`, `effort`, `effort_source` (only `claude` and `codex`, the same three values), `elapsed_seconds`, `timeout_seconds` (G19), `resets`, `attempts`, `findings` (the number of findings when completed, otherwise `null`), `verdict`, `angles_checked`, `angles_unchecked`, `limits`, `warnings`, `agent`, `recorded_from`, `workspace_digest` (the digest when the native workspace was prepared).

## 6. Classification

An item of `findings.json` adds to the fields of the reviewer result `id` (`<리뷰어>-F<n>`, in reporting order), `reviewer`, `reviewer_id` (the `id` the reviewer gave), `class`, `class_reason`, `path`, `in_check_paths`, `location_unreadable`, `claimed_link_kind` and `claimed_link`. `path` and `in_check_paths` are the result of reading the location (G7). `link_kind` and `link` keep only confirmed links (G8, G9); when not confirmed, they are `none` and an empty string. A finding without `location_unreadable`, as in records written before this goal, is a finding whose location was read. If a reviewer leaves out `smallest_fix`, it stays an empty string, and a finding in an older record without `smallest_fix` has no direction. The rules below do not read `smallest_fix`.

Classification rules for findings whose location was read (the first match from the top applies):

1. If the location is in the check paths and `check_impact` is `false_pass` or `meaning_inverted`, `defect`
2. If the location is in the check paths, it is `contract_violation` and the link is confirmed, `defect`
3. If the location is in the check paths: `requirement-question` when `proposed_class` is `requirement-question`, the link is confirmed, and that link is not a `contract` link pointing to a file in the check paths; otherwise `hardening`, whatever the proposed class
4. If `proposed_class` is `requirement-question`, `requirement-question`
5. If `introduced_by_change` and `regression` are both `yes`, `defect`
6. If `proposed_class` is `defect` and the link is not confirmed, `note`
7. Otherwise, `proposed_class`

A `hardening` whose location was read is deferred automatically, with a `{"disposition": "defer", "auto": true}` ledger entry, when the round is recorded. A `requirement-question` from rule 3 is not deferred automatically and closes only with a disposition that quotes the user's words, as in section 7. Its link must be the criterion, rule or contract the question asks about (the reviewer packet says so), and spar confirms that the linked target exists and, for a `contract` link, that the target (the rule document or the repository path of the head file that G8 found) is outside the check paths. A rule document outside the repository has no repository path, so it is always outside the check paths. A `contract` link pointing to a file in the check paths points to the check code itself, so rule 3 does not count it as a confirmed link. The link confirmation in rule 2 has no such restriction. A finding whose location cannot be read is classified as in section 4 of [Input formats](spar-input.en.md) and is not deferred automatically: rules 3 and 6 are not used, and rules 1 and 2 are used without the check-path condition.

The class is fixed when a finding is recorded. The gate and the summary read the class recorded in `findings.json`, so findings already recorded are not classified again even when the rules change.

## 7. Dispositions

`spar triage <라운드> <지적> <처리> [옵션]` adds one line to the ledger. When it does not fit the rules, it writes nothing and exits with 65. The last entry is the finding's disposition.

| Disposition | Requires |
| --- | --- |
| `fix` | `--commit` (a commit after the round head), `--failed-before`, `--passed-after` |
| `reject` | `--evidence` (`--user-quote` instead for a `requirement-question`) |
| `defer` | `--to`. For a `defect`, `--user-quote` too |
| `duplicate` | `--of` another finding of the same round. A `defect` or `requirement-question` points only to a finding of the same class, and closes only when the finding it points to is closed (gate) |
| `answer` | Only for a `requirement-question` |

`fix`, `reject`, `defer` and `answer` that close a `requirement-question`, and `fix` of a `hardening` (both automatically deferred ones and ones whose location cannot be read), need `--user-quote` quoting the user's words. Format of finding IDs and options: G30. A `note` needs no disposition.

Ledger entry: `finding`, `disposition`, `auto`, `reason`, `commit`, `failed_before`, `passed_after`, `evidence`, `of`, `to`, `user_quote`, `note`, `recorded_at`.

## 8. Caps, reruns and the chain

- The stage groups are `plan`, `plan_delta` and `code` (candidate and fixes). When the number of rounds of the same group in the series whose `counted` is `true` is at least the sum of the cap and the extensions, a new run is refused.
- When earlier runs with the same stage, base, head and packet digest exist and every reviewer of this run is a reviewer that did not complete in those runs (a reviewer with the same name and preset), the run does not count toward the cap (`counted: false`, `rerun_of`). Rounds run one at a time per series, so an earlier round without `finished_at` stopped partway, and its reviewers did not complete. Every other run counts.
- The base of a fixes round must be the head of the last code round of the same series. Otherwise the run is refused. Only an earlier round whose stage, base, head and packet digest all match this run (a rerun of the same packet) is skipped. When the packet differs, that round is the last of the chain even if base and head are the same.
- An extension adds `{"stage", "by", "user_quote", "recorded_at"}` to the series' `extensions.jsonl` with `spar extend <series> --stage plan|plan-delta|code --by N --user-quote "…"`. Without the user's words, it is refused. The series path follows the same record path rule (G28) as `--out` (even when the repository under review cannot be found, it is checked against the repository the path belongs to), and when `--repo` is not a Git repository, it is refused.

## 9. The native reviewer

A reviewer of the `native` preset starts as `pending` and leaves `prompt.md` (the instructions and the full packet) and `workspace/` in a folder with its name (`native/prompt.md` with the default name). When the host's clean-context subagent answers, the answer is recorded with `spar record <라운드> <이름> --result <파일|->` or `--transcript <전사 기록>`. Dry run rounds and reviewers that already completed are not recorded. Recording checks that the workspace is the same as when it was prepared, that the result meets the schema, head and angle requirements, and that the round's `findings.json` and `ledger.jsonl` exist (it is refused otherwise). From the transcript (G15), use of tools other than `Read`, `Grep` and `Glob`, other tools the host provided, context the host added (Git status, instruction files, hook context), not reading the prompt, and `Read`, `Grep` or `Glob` reading outside the native workspace and `prompt.md` are kept in `warnings`. The content of the context and the paths read are not recorded; only the kind and the tool name are kept. The isolation grade is `host`, and it counts as a required reviewer.

## 10. Decision to drop a required reviewer

When a required reviewer did not finish a round and the user said to go on without that reviewer, the user's words are recorded for that round.

```bash
spar waive docs/reviews/example/02-candidate agy --user-quote "agy 없이 진행"
```

- `spar waive <라운드> <리뷰어> --user-quote "…"` adds one line `{"round", "reviewer", "preset", "status", "reason", "user_quote", "recorded_at"}` to `waivers.jsonl` in that round folder. `round` is the round folder's name, `preset` is the preset of that required reviewer, and `status` and `reason` are that reviewer's status and cause in `round.json` at the time of recording. When the round did not run a reviewer with that name and preset, `status` is `not_run` and there is no `reason`.
- Without the user's words, nothing is recorded.
- In the following cases it writes nothing and exits with 65 and the reason: `--user-quote` is missing or empty after leading and trailing whitespace is removed; the round cannot be read (`round.json`, `findings.json` or `ledger.jsonl` is missing or cannot be parsed, G32); a dry run round; the reviewer is not a required reviewer of that round ([Review gate](review-gate.en.md) section 2); the reviewer completed as a required reviewer of the same preset in that round or in another round of the same series with the same stage, base, head and packet (a rerun); the round already has a decision for that reviewer; `waivers.jsonl` cannot be read; a round recorded before this feature has a reviewer named `waivers.jsonl` (case-insensitive), whose folder takes the file's place.
- A reviewer with a recorded decision is not a `degraded` reason for that round and appears separately in the gate file. A round in which no required reviewer completed is `degraded` even with decisions ([Review gate](review-gate.en.md) section 2).
- A decision applies only to its round. A rerun of the same packet is also a different round, so if the reviewer does not finish in that round either, the decision is recorded again.
