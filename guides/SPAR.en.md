# Spar guide

[한국어](SPAR.md) · **English**

Spar sends one review packet, pinned to two commits, to several reviewers at once and keeps what they found and what was done about it. The `spar` binary runs the rounds; the `spar` plugin tells your agent when to run them and how findings may be closed.

From v0.1.7, Seal starts new goals under `run-rules/5`, and a `standard` or `strict` goal started under `run-rules/5` completes only with a recorded Spar gate. `light` goals, goals without an `Assurance:` line and goals started under `run-rules/4` or earlier do not need Spar. To finish without review, you decide a goal-document change that lowers the tier to `light` ([Review gate](USAGE.en.md#review-gate-run-rules5)).

`spar` ships next to `ha` in the release files and the Homebrew formula from v0.1.7; release files and formulas of v0.1.6 and earlier do not contain it. See the [install guide](INSTALL.en.md).

## What leaves your computer

Each reviewer receives the whole packet: the criteria documents, the rule documents, the documents under review, and the diff between the two commits. A reviewer CLI can also read the tracked files of both commits in its workspace copy and send what it opens to its provider.

| Reviewer | Provider that receives the packet |
| --- | --- |
| `claude` | Anthropic, through the Claude Code CLI |
| `codex` | OpenAI, through the Codex CLI |
| `agy` | Google, through the Antigravity CLI |
| `command` | Whatever service the command you configure uses |
| `native` | The provider of the agent you are already using |

Commit messages, Git history and ignored files are not in the packet. If the criteria or the code must not go to any provider other than the one your agent already uses, configure only the `native` reviewer. If they must not leave your computer at all, do not run Spar: every reviewer, `native` included, sends the packet to a provider.

## Configure reviewers

Reviewers come only from your user configuration. Spar reads the first file it finds among `$SPAR_CONFIG`, `$XDG_CONFIG_HOME/jaekit/spar.toml` and `~/.config/jaekit/spar.toml`. Without one, it reads the `reviewers` list of an existing clean-review `config.json`, and without that it uses `claude`, `codex` and `agy`. An empty list (`reviewers = []`) means the `native` reviewer only.

Without any of these user configurations, `claude`, `codex` and `agy` are all required reviewers. When a required reviewer's CLI is missing or the reviewer does not finish a round, the gate is `degraded` unless you waived that reviewer for that round (`spar waive`), so a `standard` or `strict` goal does not complete. Before the first round, write a user configuration in `~/.config/jaekit/spar.toml` that lists only the CLIs you have. For example, with only the claude CLI, list that one reviewer:

```toml
[[reviewers]]
preset = "claude"
```

With one reviewer listed, it is the only required reviewer, and a round it finishes can be `complete`. With no reviewer CLI at all, use the empty list for the `native` reviewer only: a clean-context subagent of the agent you are using reviews.

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

- `claude`, `codex` and `agy` start their CLI with read-only, offline protections that spar checks; they are recorded as `verified` and count as required reviewers.
- `fallback_model` runs once, in the same round and on the same packet, only when the designated model is refused for quota or rate limits. `fallback_model = ""` turns the preset's default fallback off.
- `native` is a clean-context subagent of your agent. Spar writes its prompt and workspace into the round folder and records the reply the agent brings back. It is recorded as `host` and counts as required.

### A command reviewer

Any other reviewer can run as a command. It runs without a shell, receives the packet file, its workspace copy and the result schema through placeholders, and prints one JSON object that follows the schema. This example is synthetic: `example-reviewer` stands for a program you provide.

```toml
[[reviewers]]
name = "example-reviewer"
preset = "command"
argv = ["/opt/example/bin/example-reviewer", "--prompt", "{prompt}", "--workspace", "{workspace}", "--schema", "{schema}"]
timeout_seconds = 600
```

- `{prompt}` is the path of the round's `packet.md`, `{workspace}` the reviewer's copy of the two commits (`head/` and `base/`), `{schema}` the JSON schema of the result.
- Spar cannot check what a command does, so it is recorded as `unverified` and never counts as a required reviewer. Its findings are classified and closed by the same rules.

Before running a round, check with `spar config` which reviewers it will start. `spar config` prints the reviewers a round will start, where the list came from and its digest. `spar run --reviewers codex,agy` changes the list for one run, and the round record says so.

### Project settings

A repository's `.jaekit/spar.toml` can set only round caps, attack angles, rule documents and check paths:

```toml
rules = ["AGENTS.md"]
check_paths = ["tools/checks/**"]

[caps]
code = 2
```

It cannot choose reviewers, commands, executables or models; spar names the key and stops. The full format is in the [configuration contract](../contracts/spar-config.md).

## Run a review

Ask your agent, for example `$spar:spar` in Codex or `/spar:spar` in Claude Code. The skill runs these stages:

| Stage | When |
| --- | --- |
| `plan` | once, after the plan and its checks are written |
| `plan-delta` | at most once, when the plan round changed the plan |
| `candidate` | on the commit that should meet every criterion |
| `fixes` | after fixing findings; its base is the previous code round's head |

One series allows one plan round, one plan-delta round and three code rounds. A round past its cap is refused before any reviewer starts; only your words, recorded with `spar extend`, allow more. Rerunning reviewers that did not finish the same packet, for example after a quota refusal, does not count.

Each round writes a new, Git-ignored folder with `round.json`, `packet.md`, `findings.json`, `ledger.jsonl` and `summary.md`. Spar refuses a folder that Git would track, because the packet can contain private documents. The format is in the [round contract](../contracts/review-round.md). The format of every input spar reads, such as finding locations and links, configuration values and command arguments, and what happens to input outside it, is in one table in the [input contract](../contracts/spar-input.md). A finding whose location names no file is kept, marked as an unreadable location and never deferred automatically.

## Findings and the gate

Spar classifies every finding as `defect`, `requirement-question`, `hardening` or `note`. A requirement question closes only with your words; an edge case in check code that claims no false pass, inverted result or named rule violation is deferred as `hardening`. A reviewer that did not finish is shown with its cause and is never counted as finding nothing.

`spar gate` reads the rounds and the head you are about to complete and writes a `review-gate/v1` file: `complete` (exit 0), `degraded` (3) when a required reviewer did not finish and you made no decision for that round, or `blocked` (4) for open findings, caps exceeded or an unreviewed head. It also records how many lines review fixes added to check code and elsewhere. See the [gate contract](../contracts/review-gate.md).

When a required reviewer did not finish a round, for example because of a usage limit, and you say to go on without it, the agent records your words for that round with `spar waive <round> <reviewer> --user-quote "…"`. It never records one without your words. A waived reviewer is not a `degraded` reason for that round, and the gate file shows it under that round's `waived_reviewers` with its status and your words. A waiver covers only its round, and a round where no required reviewer completed stays `degraded` even with waivers.

The gate is a review record, not evidence of completion. Seal's completion still comes only from `ha done`. For a `standard` or `strict` goal started under `run-rules/5`, Seal records the gate file with `ha note <goal> review --gate <file>`, and `ha done` records completion only when the recorded gate covers the current HEAD and goal documents ([Review gate](USAGE.en.md#review-gate-run-rules5)). So the rounds get SPEC.md and every goal document it links as `--criteria`.
