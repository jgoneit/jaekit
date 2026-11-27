# Spar independent review guide

[한국어](SPAR.md) · **English**

Spar is an independent review tool. Reviewers outside the working agent's conversation look for problems that agent missed. Reviewers cannot change the requirements. What they find is sorted into `defect`, `requirement-question`, `hardening` and `note`, and how each was handled is recorded. A finished review is kept as a gate file, and `ha` uses that file to judge completion.

Read this guide before your first `standard` or `strict` goal. Read it also when you change reviewers or read review records. When you need it is below and in [Review tiers](OVERVIEW.en.md#review-tiers) in Understanding Jaekit.

Spar sends one review packet, pinned to two commits, to several reviewers at once. It then keeps what they found and what was done about it. The `spar` binary runs the rounds. The `spar` plugin tells your agent when to run them and how findings may be closed.

Seal 0.1.36 in v0.1.9 starts new goals under `run-rules/8`. From v0.1.7, a `standard` or `strict` goal started under `run-rules/5` or later completes only with a recorded Spar gate. `light` goals and goals without an `Assurance:` line do not need Spar.

> Goals started under `run-rules/4` or earlier do not need Spar either.

To finish without review, the goal document must lower the tier to `light`. You decide that change ([Review gate](RULES.en.md#review-gate-run-rules5)). A `run-rules/8` goal with a `review` condition also needs that criterion changed. Otherwise it stops at `review_tier_missing`.

`spar` ships next to `ha` in the release files and the Homebrew formula. See the [install guide](INSTALL.en.md).

> `spar` ships from v0.1.7. Release files and formulas of v0.1.6 and earlier do not contain it.

## What leaves your computer

Each reviewer receives the whole packet. The packet holds:

- the criteria documents, the rule documents, the documents under review and the check boundary documents
- the user's decisions you pass (in the user's own words) and your questions
- the earlier rounds' findings with their recorded dispositions (evidence and the user's words included)
- the diff between the two commits

A reviewer CLI can also read the tracked files of both commits in its workspace copy. It sends what it opens to its provider.

| Reviewer | Provider that receives the packet |
| --- | --- |
| `claude` | Anthropic, through the Claude Code CLI |
| `codex` | OpenAI, through the Codex CLI |
| `agy` | Google, through the Antigravity CLI |
| `command` | Whatever service the command you configure uses |
| `native` | The provider of the agent you are already using |

Commit messages, Git history and ignored files are not in the packet. If the criteria or the code must not go to any provider other than the one your agent already uses, configure only the `native` reviewer. If they must not leave your computer at all, do not run Spar. Every reviewer, `native` included, sends the packet to a provider.

The shared packet's `## Standpoints for this round` section contains all standpoints and their reviewer assignments. Each reviewer also receives its own instruction. The instruction identifies that reviewer's standpoint and asks it to attack that risk while still checking every angle. An unassigned reviewer is told to review the whole change.

The instruction and packet reach each reviewer this way:

- `claude` and `codex` receive the instruction and packet on standard input.
- `agy` receives the command note, instruction and packet in `-p`. For a large packet, `PACKET.md` holds the packet, and `-p` holds the command note, instruction and file reference.
- `command` receives the reviewer's `input.md` through `{prompt}`.
- `native` receives the instruction after its fixed instructions in `prompt.md`.

For a round with standpoints, each reviewer's input is kept exactly as that reviewer receives it, in `<round>/<reviewer>/input.md`. Both a real run and `--dry-run` do this. For `agy` with a large packet, `input.md` holds its `-p` text and the packet is in `PACKET.md`.

## Configure reviewers

Reviewers come only from your user configuration. Spar reads the first file it finds among `$SPAR_CONFIG`, `$XDG_CONFIG_HOME/jaekit/spar.toml` and `~/.config/jaekit/spar.toml`. Without one, it reads the `reviewers` list of an existing clean-review `config.json`. Without that, it uses `claude`, `codex` and `agy`. An empty list (`reviewers = []`) means the `native` reviewer only.

Without any of these user configurations, `claude`, `codex` and `agy` are all required reviewers. When a required reviewer's CLI is missing or the reviewer does not finish a round, the gate is `degraded`. The exception is when you waived that reviewer for that round (`spar waive`). With a `degraded` gate, a `standard` or `strict` goal does not complete. Before the first round, write a user configuration in `~/.config/jaekit/spar.toml` that lists only the CLIs you have. For example, with only the claude CLI, list that one reviewer:

```toml
[[reviewers]]
preset = "claude"
```

With one reviewer listed, it is the only required reviewer. A round it finishes can be `complete`. With no reviewer CLI at all, use the empty list for the `native` reviewer only. A clean-context subagent of the agent you are using then reviews.

```toml
reviewers = []
```

With all three CLIs, you can list them like this:

```toml
timeout_seconds = 1800

[[reviewers]]
preset = "claude"

[[reviewers]]
preset = "codex"

[[reviewers]]
name = "agy"
preset = "agy"
model = "gemini-4-argon-high"
fallback_model = "gemini-3.8-flash-high"
```

- `claude`, `codex` and `agy` start their CLI with read-only, offline protections. spar checks those protections. They are recorded as `verified` and count as required reviewers.
- `agy` may use only its file read, list and search tools and shell commands. Its shell commands may contain only read-only programs (`ls`, `grep`, `cat`, `diff`, `true` and so on) and `sed -n`, joined with `|`, `&&`, `||` or `;`. If it runs code, tests, builds, git or any other program, spar stops that reviewer. spar then records it as `tool_violation` and drops its result. spar puts this rule in a short note at the start of agy's prompt.
- `fallback_model` is used only when the designated model is refused for quota or rate limits. It then runs once, in the same round and on the same packet. `fallback_model = ""` turns the preset's default fallback off.
- `native` is a clean-context subagent of your agent. Spar writes its prompt and workspace into the round folder. When the agent brings back the reply, Spar records it. It is recorded as `host` and counts as required.
- Each reviewer can have `max_packet_bytes`, the largest packet in bytes it receives. Without it, `claude` and `codex` take 1,200,000 bytes and `agy` 4,000,000 bytes. `native` and `command` have no limit. When the packet is larger than the limit of any reviewer of the run, `spar run` stops with 65 before any reviewer starts. It writes no round, so it uses none of the cap. Then make the change or the documents you pass smaller. You can also raise the limit when you know that reviewer takes larger packets. `spar config` shows each reviewer's limit and its source (`max_packet_bytes_source`).

### Models and effort

A reviewer's model (`claude`, `codex`, `agy`) and effort (`claude`, `codex`) come first from your user configuration. Writing `model`, as the example above does, fixes it.

For a reviewer whose model or effort is not written, the agent running the round chooses. It passes `--model NAME=MODEL` and `--effort NAME=EFFORT` to `spar run`. Without them, the preset's values run (models `opus`, `gpt-6-astra`, `gemini-4-argon-high`, effort `high`).

Before any reviewer starts, spar refuses:

- a model or effort for a reviewer whose configuration writes it, even one equal to the written value
- a model or effort for a `native` or `command` reviewer
- an effort for `agy`
- a name that is not a reviewer of the run
- a second model or effort for the same reviewer
- a value outside the allowed list
- a value that is empty, is not valid UTF-8, starts with `-` or holds a space or control character

To limit what the agent may choose, write `allowed_models` and `allowed_efforts`:

```toml
[[reviewers]]
preset = "codex"
allowed_models = ["gpt-6-astra", "example-model-mini"]
allowed_efforts = ["medium", "high"]
```

- With a list, the agent can choose only its entries. When the agent chooses nothing, the preset's values run, whatever the list holds.
- A model the agent chose falls back only to the `fallback_model` you wrote. Without one, it runs with no fallback.
- `spar config --model codex=gpt-6-astra` shows the model a run with that choice applies and where it comes from.
- The round record and summary keep each reviewer's model and effort with their source (`user`, `agent`, `preset`).

### A command reviewer

Any other reviewer can run as a command. The command runs without a shell. It receives the packet file, its workspace copy and the result schema through placeholders. It prints one JSON object that follows the schema. This example is synthetic. `example-reviewer` stands for a program you provide.

```toml
[[reviewers]]
name = "example-reviewer"
preset = "command"
argv = ["/opt/example/bin/example-reviewer", "--prompt", "{prompt}", "--workspace", "{workspace}", "--schema", "{schema}"]
timeout_seconds = 600
```

- `{prompt}`: the path of `packet.md` for a round without standpoints. For a round with standpoints, the path of that reviewer's `input.md`.
- `{workspace}`: the reviewer's copy of the two commits (`head/` and `base/`).
- `{schema}`: the JSON schema of the result.
- Spar cannot check what a command does. So it is recorded as `unverified` and never counts as a required reviewer. Its findings are classified and closed by the same rules.

Before running a round, check with `spar config` which reviewers it will start. `spar config` prints the reviewers a round will start, where the list came from and its digest. `spar run --reviewers codex,agy` changes the list for one run. The round record says so.

### Project settings

A repository's `.jaekit/spar.toml` can set only round caps, attack angles, rule documents and check paths:

```toml
rules = ["AGENTS.md"]
check_paths = ["tools/checks/**"]

[caps]
code = 2
```

It cannot choose reviewers, commands, executables, models, efforts or allowed lists. If it does, spar names the key and stops. The full format is in the [configuration contract](../contracts/spar-config.md).

## Set up a project

Spar writes review records only to a folder Git ignores, because a packet can contain private documents. In a project that has never run Spar, Git does not ignore that folder yet. So the first round stops with exit 65 before any reviewer starts, and writes nothing.

Before a project's first `standard` or `strict` goal, add one line to the project's `.gitignore`. Then commit that change before the goal starts. Seal runs checks only on a committed, clean tree.

```text
docs/reviews/
```

`docs/reviews/<goal>/` is where the Spar skill puts a goal's rounds and gate. `light` goals and goals that do not use Spar do not need this step.

- **Another folder.** Records can go to any folder Git ignores. Add that folder to `.gitignore` instead, and tell your agent to put the rounds there.
- **Only on your computer.** To leave `.gitignore` unchanged, add the same line to `.git/info/exclude`. Git does not commit that file, so another clone or computer needs the line again.
- **Tracked files.** Spar also refuses a folder under which Git tracks files. Ignoring a folder does not untrack files already committed there.

## Run a review

Ask your agent, for example `$spar:spar` in Codex or `/spar:spar` in Claude Code. The skill runs these stages:

| Stage | When |
| --- | --- |
| `plan` | once, after the plan and its checks are written and before implementation |
| `candidate` | on the commit that should meet every criterion, and again after review fixes; its base is always the goal's base commit |

New rounds start only with these two stages. `--stage plan-delta` and `--stage fixes` are refused with a usage error (64). The refusal comes before anything is recorded, and it names `candidate`.

After review fixes, run one `candidate` round from the goal's base commit to the new head. It receives the earlier code rounds' findings with their last dispositions. The last such round that ends with no open `defect` or `requirement-question` is the cumulative round.

When the plan round leads to changed goal documents or checks, the next `candidate` round reviews the current goal documents. A change to the plan or checks alone needs no new plan round.

In a `strict` goal, though, Seal Core also needs the last `plan` round to have received the current goal documents (`review_docs_stale`). So when SPEC.md or a goal document it links changes after the plan round, ask the user to extend the plan cap (`spar extend <series> --stage plan --by 1 --user-quote "…"`). Then run another `plan` round on the current head.

> `spar gate` and `spar cap-report` still read the `plan-delta` and `fixes` rounds recorded before this change as they did.

One series allows one plan round and two code rounds. A round past its cap is refused before any reviewer starts. Only your words, recorded with `spar extend`, allow more. Rerunning reviewers that did not finish the same packet, for example after a quota refusal, does not count.

> A new round in a recorded series is counted against the caps it records. Recorded fixes rounds count as code rounds. So a series that already recorded that many code rounds needs an extension for another candidate round.

When the code rounds run out, the `spar run` refusal points to that series' cap report. `spar cap-report` reads only the series' records and Git, and writes nothing. It shows the cap use, the rounds, the open findings and four signals. The signals are counted this way:

- **Churn**: counted by file.
- **Recurring links**: counted by link.
- **Unrelated defects**: each reviewer's judgment, as given.
- **Check growth**: measured only in a series with recorded fixes rounds. For a series of plan and candidate rounds, the report says it was not measured.

```bash
spar cap-report docs/reviews/example
```

The report never picks a cause. The agent picks one cause (`--cause`) from the signals. The cause is one of `churn`, `recurring`, `unrelated`, `hardening` and `other`. The agent asks you with its recommendation. It records `spar extend <series> --stage code --by 1 --cause <cause> --user-quote "…"` only when your words allow more rounds. `--by 2` or more, and `--cause other`, need `--reason`. Give the series to `ha cost` with `--series` to see the recorded cause and reason in its list of cap extensions.

Pass what the user decided to every later round as a decision. A user's answer to a `requirement-question` is the typical one. The same holds when you use Spar without Seal. Give each decision as `--decision` (what it settles) followed right away by `--decision-quote` (the user's words).

```bash
spar run --stage candidate --base 1111111 --head 3333333 \
  --criteria docs/specs/example/SPEC.md --rules AGENTS.md \
  --decision "Empty input in AC-2" --decision-quote "Keep empty input an error" \
  --out docs/reviews/example/03-candidate
```

Decisions go into the packet's `## Decisions the user made` section as given. They also stay in `decisions` in `round.json`. The section tells reviewers three things:

- Do not report what a decision settles.
- Report as usual a change that goes beyond a decision.
- When following a decision breaks a criterion, rule or contract the decision does not address, report a `requirement-question` that links it.

spar does not check that the words are the user's. An empty decision, an unpaired option or text that is not valid UTF-8 is refused before any reviewer starts. The same input with the same decisions is the same packet. Different decisions make another packet.

When you have documents that state the checks' boundaries, pass them with `--boundary`. A boundary document states what each check observes and which inputs it does not cover. Both stages take the option, and it may be repeated. The documents go into the packet's `## Check boundaries` section and stay in `boundaries` in `round.json`.

You may also pass the same file with `--criteria`, as with a goal's `REVIEW.md`. Its text then goes into the packet once, as a criteria document. The boundary section has only its name and a line that points to that criteria document. So the text does not count twice toward a reviewer's `max_packet_bytes`.

The section tells reviewers two things:

- Report a counterexample on an input outside a stated boundary that no criterion, rule, contract or protected regression path requires as `hardening` with `check_impact` `none`, naming the boundary.
- Report a counterexample on an input that a criterion, a contract or a protected regression path requires as a `defect`, whatever a boundary says.

spar does not check that a boundary document is right. A boundary document that cannot be read is refused before any reviewer starts.

Give a check that only reads fixed text in a file with `--text-check`. A test that looks for a token in a document is such a check. Give it as well as or instead of `--check-path`. It may be repeated. It is a check path like any other, so classification is unchanged.

- `round.json` keeps the globs in `config.text_check_paths`.
- The packet lists them in a `- Fixed-text checks:` line.
- The `check-validity` angle asks reviewers to judge whether such a check reads the right fixed text in the right file. It also asks them to report that fixed text cannot prove the behavior only as a `requirement-question` that names the criterion, contract or protected path requiring it.

The packet of a candidate round shows the findings of the series' earlier code rounds. Those are its candidate rounds and recorded fixes rounds. So every candidate round after review fixes sees the earlier code rounds' findings too. The cumulative round is no exception. A plan round receives no earlier findings.

When a finding has a recorded disposition, its line ends with its last disposition (`fix: commit …`, `reject: …`, `defer: to …`, `defer (auto): …`, `duplicate of …`, `answer: …`). When any does, the packet gives reviewers this instruction: report a finding with a recorded disposition again only with a failure scenario that the recorded disposition does not answer.

Record dispositions with `spar triage` before you run the next round, and the reviewers review with those dispositions in view. Dispositions are part of the packet. So a candidate round run on the same base and head after a disposition changed is a new round that counts toward the cap.

> A later `candidate` run of an earlier `candidate` round's base and head receives only the earlier findings that round received. That is because it reads the rounds named in that round's `prior` in `round.json`. So it receives none when that round was recorded before candidate rounds received earlier findings.

Each round writes a new, Git-ignored folder with `round.json`, `packet.md`, `findings.json`, `ledger.jsonl` and `summary.md`. The finding table in `summary.md` shows each claim whole, on one line in the `Claim` column. So that file alone is enough to classify and close the findings. Spar refuses a folder that Git would track, because the packet can contain private documents ([Set up a project](#set-up-a-project)).

<details>
<summary>Round and input formats, and findings without a readable location</summary>

The round format is in the [round contract](../contracts/review-round.md). The format of every input spar reads is in one table in the [input contract](../contracts/spar-input.md). Such inputs include finding locations and links, configuration values and command arguments. The table also says what happens to input outside the format.

A finding whose location names no file is still kept. It is marked as an unreadable location and never deferred automatically.

</details>

### Reviewer standpoints

In every plan and candidate round, the cumulative round included, the agent picks the change's main risks. It then gives each reviewer a different standpoint, one each. When there are fewer risks than reviewers, the rest review without one. A standpoint names the risk, where to look first and the position from which to see it. It must not claim the fix is right or explain the solution. This synthetic example assumes codex is configured.

```bash
spar run --stage candidate --base 1111111 --head 2222222 \
  --criteria docs/specs/example/SPEC.md --rules AGENTS.md \
  --standpoint "codex=From the stored-format reader position, start with the earlier-record readers and attack the risk that changed fields reject earlier records." \
  --out docs/reviews/example/02-candidate
```

Pass standpoints to `spar run` with `--standpoint NAME=TEXT`. The first `=` separates the reviewer name from the text. The option may be repeated. Names are compared exactly and case-sensitively, as in G20.

These are refused (65):

- a name outside this run's configured reviewers and `--reviewers`
- more than one standpoint for a reviewer
- text empty after trimming whitespace
- invalid UTF-8

A missing `=` or an empty name is a usage error (64). Both refusals happen before any reviewer starts or any round folder is created. The message names the offending name or value. Other text is kept unchanged.

The accepted names are the union of the configured reviewers and the names selected by `--reviewers`. A configured reviewer omitted from this run may still have a standpoint in the packet. So when rerunning a subset, pass the full earlier list of standpoints in the same order. Standpoints are run arguments only. There is no user or project configuration key for them, and `spar config` is unchanged.

Standpoint text and assignments are in the packet. So `packet_sha256` identifies them as part of the same input, and changing either makes a new input. A rerun of only unfinished reviewers is not counted when stage, base, head and `packet_sha256` are unchanged, including the same standpoints.

`standpoint_report` is an object with two fields. `checked` is a boolean and `detail` is nonblank text. `detail` says what was checked when true, or why it could not be checked when false. For an assigned reviewer, a missing, null or malformed report is `invalid_output`. The reason then includes `standpoint_report`. An unassigned reviewer need not report one. Its `standpoint_report` is not checked and is recorded as `null`. Every preset uses the same check in `spar run` and `spar record --result` or `--transcript`.

`round.json` keeps standpoints this way:

- `standpoints`: reviewer/text pairs
- `standpoints_uncovered`: reviewer names without a completed reviewer
- each reviewer's `standpoint` and `standpoint_report`: text and report, or null

`summary.md` shows each reviewer's standpoint and report. It also shows standpoints without a completed reviewer, including waived reviewers. The schema remains `review-round/v1`. Records without standpoint fields mean no standpoints.

`spar gate` lists in `uncovered_standpoints` the standpoints whose reviewer completed no given round of the same packet. A reviewer that completed in a rerun covers its standpoint. A waived reviewer's standpoint stays listed. When reporting the gate, also name those standpoints.

Standpoints do not change angles, classification, dispositions, caps or the gate's state and reasons.

## Findings and the gate

The gate is a review record, not evidence of completion. Seal's completion still comes only from `ha done`. For a `standard` or `strict` goal started under `run-rules/5` or later, Seal records the gate file with `ha note <goal> review --gate <file>`. `ha done` then records completion only when the recorded gate covers the current HEAD and goal documents ([Review gate](RULES.en.md#review-gate-run-rules5)). So the rounds get as `--criteria` SPEC.md and every Markdown document in the goal folder that it links, directly or through another linked document.

### How findings are classified

Spar classifies every finding as `defect`, `requirement-question`, `hardening` or `note`. A requirement question closes only with your words. An edge case in check code that claims no false pass, inverted result or named rule violation is deferred as `hardening`. A reviewer that did not finish is shown with its cause. It is never counted as finding nothing.

### The smallest fix

Reviewers give each finding a `smallest_fix`. It is the smallest change that would make its failure scenario come out right, stated as a direction rather than a patch. A reviewer may leave it empty.

The direction stays in `findings.json` and appears under `## Smallest fixes` in `summary.md`. It is advice, so it is not used for classes, dispositions or the gate. The agent starts a fix within that direction. When the fix goes further, the agent records why with `spar triage … --note`.

### Gate states

`spar gate` reads the rounds and the head you are about to complete and writes a `review-gate/v1` file. Its state is one of these:

- `complete` (exit 0)
- `degraded` (exit 3): a required reviewer did not finish and you made no decision for that round
- `blocked` (exit 4): open findings, caps exceeded or an unreviewed head

See the [gate contract](../contracts/review-gate.md).

> When the series recorded fixes rounds, the gate also records how many lines those review fixes added to check code and elsewhere. Without one, those lines are not measured.

### Going on without a reviewer

A required reviewer may not finish a round, for example because of a usage limit. If you then say to go on without it, the agent records your words for that round with `spar waive <round> <reviewer> --user-quote "…"`. It never records one without your words.

A waived reviewer is not a `degraded` reason for that round. The gate file shows it under that round's `waived_reviewers` with its status and your words. A waiver covers only its round. A round where no required reviewer completed stays `degraded` even with waivers.

