# Release acceptance checks

Run `python3 tools/release-checks/check.py AC-1 --evidence DIR` through
`AC-11` explicitly. These checks do not publish, install packages, change host configuration, or
start a model. They read public GitHub data, installed files, and separately
captured observations. Maintenance criteria also run existing local regressions.
They are not part of ordinary offline CI.

Pass the directory holding the private evidence index `evidence.json` with
`--evidence DIR`. The checker never searches the checkout or its Git directory
for evidence; without `--evidence DIR`, a criterion that needs observations is
reported as unverified. Keep that directory outside the repository and never
commit the index, traces, host inventories, or synthetic execution records. A
relative artifact path is relative to this directory; an absolute path
identifies an existing installation or observation. Each artifact is an object with
`path` and lowercase `sha256`, verified against the current file bytes.

A command capture contains its actual `argv` array, integer `exit_code`, and
`stdout` and `stderr` artifacts. Keep the original output bytes, including empty
stderr. The checker accepts successful captures only. Do not replace failed
attempts; retain them separately and identify the subsequent successful capture.

```json
{
  "schema": "jaekit-release-evidence/v1",
  "release": {
    "tag": "v0.1.3",
    "source_commit": "FULL_PUBLIC_COMMIT_SHA",
    "previous": {"path": "previous-release.json", "sha256": "FILE_SHA256"}
  },
  "installations": {
    "direct": {},
    "brew_fresh": {},
    "brew_upgrade": {}
  },
  "hosts": {
    "codex": {},
    "claude": {}
  }
}
```

Before publication, save the old release identity using GET requests to
`repos/jgoneit/jaekit/commits/v0.1.2` and
`repos/jgoneit/jaekit/releases/tags/v0.1.2`. The first response supplies `sha`;
the second supplies `id` and `assets`. Save this normalized snapshot:

```json
{
  "tag": "v0.1.2",
  "commit": "FULL_OLD_COMMIT_SHA",
  "release_id": 123,
  "assets": [{"id": 123, "name": "checksums.txt", "size": 123, "digest": "sha256:HASH"}]
}
```

Include every old asset, sorted by `name`. Asset IDs and digests are compared
with the live release, so deleting and recreating a different asset does not
pass preservation merely because its filename matches.

Each installation entry has `binary` (an artifact), `package_root` (the absolute
path of its installed support-file directory), `install`, `version`, and
`capabilities` (command captures). Version and capability captures must invoke
that exact binary. The checker also queries the hash-verified installed binary.
The `brew_upgrade` entry additionally has a `before_version` capture of the old
`ha 0.1.2` installation. Homebrew install captures identify `brew install` or
`brew upgrade`; preserve actual command output. Fresh-install evidence must
come from a genuinely fresh installation, not a second version query.
AC-3 also resolves `ha` from the checker's inherited PATH and checks its bytes,
version and capabilities against the public native asset. An absolute-path
query of a new installation does not hide an older executable earlier in PATH.
Support files are compared with the public archive, including its reference
producer and example. A binary-only installation does not satisfy the documented
example acquisition path.

Each host entry has these fields:

| Field | Meaning |
| --- | --- |
| `version`, `install`, `update` | `version` is a native version capture; `install` is an array of captures for installing both `spec@jaekit` and `seal@jaekit`; `update` captures adding the marketplace with `jgoneit/jaekit@v0.1.3` (Codex) or `jgoneit/jaekit#v0.1.3` (Claude). |
| `inventory` | Original native `plugin list --json` artifact. Codex supplies marketplace source paths; Claude supplies installation paths. These are checked separately from actual session loading. |
| `plugins.spec`, `plugins.seal` | Installed `SKILL.md` artifacts; adjacent plugin manifests and supporting files are also compared with the public tag. |
| `model`, `session_id` | Identifiers observed in native trace metadata, not a model override added for the check. |
| `spec`, `seal` | Native CLI command captures for two separately requested turns in the same session. |
| `session_trace` | Codex rollout JSONL artifact containing session metadata and the actual model. For Claude, the native session transcript JSONL of the observed session; it is required to identify the loaded Skill. |
| `project`, `goal` | Existing synthetic Git repository and repository-relative goal directory. |
| `spec_commit`, `final_commit` | Full commits identifying the document-only snapshot and final implementation. |

Capture Codex with `codex exec --json`, then its native resume command; capture
Claude with `claude -p --verbose --output-format stream-json`, then `--resume`.
Use each installed host's configured model. Keep original JSONL, not a prose
summary. Codex stdout may omit the model; retain its session rollout as well.
The initial request names the Spec skill; the later, separate request names the
Seal skill. Commit the Spec result before requesting implementation. That
snapshot must contain the goal document, no PLAN or execution record, and no
product changes. Goal documents may be tracked in this explicitly synthetic
repository; this does not change another project's publication policy.

If Codex stdout contains only MCP summaries, a command capture may additionally
have a `session_trace` artifact with the native rollout snapshot taken at that
phase's completion. In particular, preserve the Spec snapshot before resuming;
the final rollout also contains Seal commands and cannot alone establish what
the earlier Spec turn did. Native `response_item` function/custom-tool calls
are read as observations, never evaluated or re-executed by this checker.

The loaded Skill is identified only by the host's own record of invoking the
requested Skill in that phase. For Codex, the invoked Skill is a separate
message in that phase's events whose text begins with a `<skill>` block naming
`spec:spec` or `seal:seal` and the installed `SKILL.md` path. The Spec phase
therefore needs its own rollout snapshot; the final whole-session rollout is
read only for the Seal phase.

Claude stream-json does not record a slash-command invocation: its
`system`/`init` event lists every available plugin and Skill whether or not it
is invoked, so it never identifies a loaded Skill. Supply the native session
transcript of the observed session as `session_trace` on the host entry, or on
a phase capture as a snapshot. Claude Code keeps it as a JSONL file named after
the session under its per-project directory. An invocation counts when the
transcript has both entries the host writes for it, each with the observed
`sessionId` and not marked as a sidechain:

1. a user entry whose content names `<command-name>/spec:spec</command-name>`
   (or `/seal:seal` for the Seal phase), and
2. the host's following `isMeta` user entry whose `parentUuid` is that entry's
   `uuid` and whose text begins with `Base directory for this skill:` and the
   installed Skill directory, the directory holding the indexed `SKILL.md`.

The capture is first bound to exactly one native request. Its assistant/user
`uuid` values must identify transcript entries in the same session. Their
`promptId`, directly or through the native `parentUuid` chain to the request,
must agree. Missing anchors, conflicting request IDs, sidechains and foreign
sessions are unverified. Only that request's linked invocation and `isMeta`
entries can identify the loaded Skill. A full transcript and a snapshot with
the same complete request evidence give the same result; previous or later
requests cannot substitute for it. Original capture and transcript bytes stay
unchanged. A manually attached phase label or a nearby timestamp is insufficient.
The available-Skill listing alone never identifies an invocation.

Core commands are recognized only in command position. The checker reads shell
quoting and static command words without evaluating them. Direct `ha` and paths
ending in `/ha`, variables statically assigned in the same scope, `env`,
`command`, `exec`, and static `bash -c` / `zsh -c` (`-c`, `-lc` or `-cl`) wrappers
remain supported. `command -v` and `-V` describe commands without running them.
The spellings `$VAR`, `"$VAR"` and `${VAR}` use only bindings in this command;
an assignment in another tool call does not carry over. A computed value from
command substitution is not a static executable binding.
Codex `exec_command` literal strings accept `cmd`, `'cmd'`, and `"cmd"` keys.
The JavaScript reader accepts a small straight-line grammar: direct
`exec_command({...})` or `tools.exec_command({...})`, optional `await`,
`text(...)`/`notify(...)`, and explicit arrays for `Promise.all(...)` or
`Promise.allSettled(...)`. Statements are separated by semicolons. Simple
`const`/`let`/`var` declarations may bind these supported expressions using ASCII
identifiers, including a direct call result followed by `text(result)` or
`notify(result)`. This does not support invoking a bound value, reading its
properties or using it as a computed command option. Reserved/strict-module
names and redefinitions are excluded.
All command options must have unique
plain keys and string, integer, boolean or null literal values, and `cmd` must be a complete string
literal. Single quotes, double quotes and templates without interpolation
are supported. Legacy leading-zero numbers, octal escapes and raw line breaks
inside single/double quotes are unverified. Comments and strings containing
call examples stay data.

Computed values, string concatenation, template interpolation, option getters,
spread or duplicate properties, indirect/computed tool access and aliases are
unverified. So are other JavaScript calls, option expressions and control flow,
including a conditional statement before a later direct call: evaluating that
statement may fail before the call is reached. The reader does not execute
JavaScript or treat a leading literal as the whole expression. An unreadable
native command argument is also unverified, not an absent command.

Command substitution in arguments, redirection targets and here-strings is
read, including backquotes and process substitution. In an unquoted
here-document, substitutions run but plain `ha start` text is only data.
Quoting any part of the delimiter, or escaping an expansion, prevents that
expansion. `printf`, `echo`, comments and ordinary quoted example strings do
not establish Core execution. A baseline flag must belong to an actual `check`
call, not to a different command's output or arguments.

| Observed text | Static result |
| --- | --- |
| `ha start goal`, `/opt/bin/ha check goal AC-1` | Core call |
| `HA=/opt/bin/ha; "$HA" start goal`, `export HA=/opt/bin/ha` followed by `${HA} done goal` | Core call in the same scope |
| `env TRACE=1 ha check goal --baseline AC-1`, `command ha check goal AC-1`, `exec ha done goal` | Core call |
| `printf x > "$(ha start goal)"`, `cat <<< "$(ha note goal input)"` | Core call in the substitution |
| `HA=/opt/ha; printf '%s' '$HA start goal'`, `echo "ha start"` | data only |
| `cat <<'EOF'` with `$(ha start goal)` in the body | data only: quoted here-document |
| `f() { ha start goal; }` without calling `f` | definition only |
| `(HA=/opt/bin/ha); $HA start goal` | the child assignment does not identify the parent command |
| `if false; then ha start goal; fi`, `false && ha start goal`, `f() { ha start goal; }; f` | actual execution evidence required |
| `eval 'ha start goal'`, `source ./commands.sh`, `. ./commands.sh`, `sh ./commands.sh` | unverified: unsupported evaluation or file execution |
| `env -S 'ha start goal'`, `env --split-string='ha start goal'`, `env -S 'printf' ha start goal` | unverified: split-string semantics are not interpreted |
| `python3 -c 'print("ha start goal")'`, `git commit -m 'ha start goal'` | argument text is not a Core call; the interpreter or hook-capable command is unverified |

Subshell, background and pipeline scopes do not leak assignments into the
parent. Where pipeline scope depends on the shell, an unconfirmed parent value
is not invented. Conditional branches, `&&` / `||`, and invoked functions can
contain Core candidates without proving they ran. Relevant uncertainty is
unverified for both Spec and Seal, not a definite Spec violation or a fulfilled
Seal operation. Control flow unrelated to Core does not itself block a result.

Computed command names, aliases, `eval`, `source`, external scripts and other
runners such as `sudo`, `xargs`, `nohup`, `timeout` or `find -exec` are
not interpreted and yield unverified results for both phases. The same applies
to other executables outside the explicit data-command scope below. The reason
identifies the native tool-call ID, or its event index when the native format
has no ID. A complete set of visible Seal calls cannot hide an additional
unresolved call. A complete supported observation missing a required operation
is a requirement failure; an unsupported or unreadable observation is unverified.

The data-command scope trusts conventional implementations of `:`, `true`,
`false`, `echo`, `cat`, `ls`, `grep`, `head`, `tail`, `wc`, `pwd`, `which`, `type`,
`cd`, `exit` and `return`; arguments are data after their substitutions have
been read. `rg` additionally excludes `--pre`/`--pre=...` and unresolved options;
`--` ends option recognition and `-e`/`--regexp`/`-f`/`--file` consume data.
`printf` requires a known output format, no assignment option and no `%n`;
only ordinary output conversions are supported. `git` is limited to `status`,
`rev-parse` and `ls-files` as its first argument. Static assignment declarations
such as `export HA=/opt/bin/ha` remain supported; other declaration options and
opaque shell expansions are unverified. `env` supports assignments,
`-i`/`--ignore-environment`, `-u`/`--unset`, `-C`/`--chdir` and `--`, with known
option arguments. Split-string forms, other shell options and files are not
silently stripped. `exit`/`return` accept at most one literal nonnegative integer;
later commands are not definite execution. A function shadowing a wrapper is
not unwrapped as that builtin. When wrapper lookup and a same-named function
interact, execution evidence is required instead of assuming which body ran.

These are name-and-argument recognition rules, assuming conventional tools,
trusted shell startup/configuration and no hostile command replacement. They
do not attest to every external child process or inspect binaries behind data
command names. The observer below has a different, narrower supported scope;
attaching its report does not make unsupported evaluators or external programs
supported. Neither reader replays observed commands. Unsupported execution is
never replaced with a fabricated completion or a claim that Core did not execute.

### Optional evidence from the original execution

Use `observe.py` **during the original execution**, in place of directly
starting a shell, when conditions or functions need runtime evidence. The
checker does not invoke the collector: **never replay** a historical command
to repair missing evidence. Existing simple observations need no new format.
Past ambiguous observations without this evidence remain unverified, with their
original raw files and completion records preserved.

For a new, authorized synthetic observation, an example tool command is:

```bash
/tmp/synthetic-python/bin/python3 /tmp/synthetic-checker/observe.py run \
  --core /tmp/synthetic-core/ha --shell /bin/bash \
  --output /tmp/private-observation/execution.json \
  --invocation unique-original-invocation -- \
  'f() { "$JAEKIT_CORE" check synthetic-goal --baseline AC-1; }; if true; then f; fi'
```

Use the actual installed Core path for a real observation. Put the
absolute interpreter path reported by `sys.executable` and absolute collector
path in the native command itself; resolve these before the original run.
The example paths above are synthetic. Relative paths, PATH aliases and shell
variables for the collector command are not supported evidence bindings.
Keep the complete
native tool command and its `tool_call_id`; after capture, attach the report
artifact to the **same phase** using `executions`. The host entry's `core`
artifact identifies the expected binary. These fields supplement native
session/request evidence and never replace it:

```json
{
  "core": {"path": "/tmp/synthetic-core/ha", "sha256": "FILE_SHA256"},
  "spec": {
    "executions": [
      {"tool_call_id": "native-tool-call-id",
       "report": {"path": "execution.json", "sha256": "FILE_SHA256"}}
    ]
  }
}
```

`jaekit-execution/v1` records the unique invocation and fresh run ID, exact collector argv,
script digest, producer/Core/shell byte identities, supported coverage, and
ordered `shell_start`, `core_start`, `core_exit`, `shell_exit` events. Each event
has a sequence and previous-event digest. The consumer matches the report to
the exact native tool command and tool ID in the already identified request.
After writing the report, the collector emits a `JAEKIT_EXECUTION_RECEIPT`
diagnostic containing its run ID and report digest to stderr. That receipt must
appear in the **same native tool call's result**: Claude `tool_result`, or Codex
`function_call_output`, `custom_tool_call_output` or completed tool result.
An index's `tool_call_id` alone cannot bind a report. Reusing an old report for
a later identical command fails because the later result has a different
receipt. Keep the original result output, including this diagnostic.
Missing or duplicate bindings, changed bytes, unknown schemas, incomplete or
conflicting events are unverified. An attached bad report never falls back to
a more permissive static result.

The controlled shell is bash without startup files or zsh with `-f`.
`$JAEKIT_CORE` routes calls through a launcher that starts the identified binary
once, records its actual expanded argv after successful process creation, and
waits for its result. Original stdout and shell exit status are preserved;
original stderr is followed by the collector's separate receipt diagnostic.
Spawn failures differ from starts; a started child without an exit,
or an interrupted collector, does not establish complete evidence. Existing
output artifacts are not overwritten. Keep failed captures and reports too.

Coverage is deliberately limited to explicit quoted `"$JAEKIT_CORE"` calls, simple
assignments, functions defined unconditionally before use, branches and shell builtins listed in the
collector's coverage policy (`:`, `true`, `false`, `echo`, a literal-format
`printf` without `-v` or `%n`, and literal numeric `exit`/`return`). Test and
arithmetic builtins such as `test`, `[` and `[[`, dynamic printf formats,
declaration builtins, `IFS` changes and implicit variable setters are unsupported.
Function definitions inside an uncalled function do not establish parent scope.
The launcher variable is accepted only as the quoted command word, never as an
assignment value, other argument or redirection target.
Collector coverage excludes redirections; the separate static reader still
recognizes substitutions in redirections and here-documents.
Dynamic command names, arbitrary external
programs, startup hooks, `eval`/`source`, asynchronous jobs, reserved observer
variable rewrites and direct Core calls that bypass the launcher are
unsupported. This is local observation, not signed third-party attestation or
a system-wide process monitor. The checker independently checks coverage of
the original script; an arbitrary `complete` flag is not proof.

With complete supported evidence, taken branches and called functions supply
only their actual Core calls. Untaken branches, uncalled functions and printed
examples supply none. Complete evidence with a missing required Seal operation
is a requirement failure; insufficient or unsupported evidence is unverified.
A Spec no-execution result requires complete coverage of the observed command.

Current-file checks do not require earlier private files or commits. The
separate [history audit](../goal-checks/README.md) takes explicit base/head
inputs; missing history is unverified, is never automatically fetched and is
not a successful preservation check. Public release identity checks remain
separate and still require their original release evidence.

The final repository must retain the `/3` start, real expected baseline failure,
current pass, complete done record, and referenced raw logs. The checker checks
record chaining, log digests, native tool-command observations, installed Core
identity and a fresh read-only status. This is local corroboration of captured
observations, not signed attestation against an author forging all evidence.

The example acquisition check reads the release archive's reference producer
and runnable example, then runs the archive validator's isolated native smoke.
It does not install anything. The real two-host observations remain separate
from this synthetic package smoke.
