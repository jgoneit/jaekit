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

Each phase is judged by its own Skill's invocation, so a whole-session
transcript serves both phases. A typed request, model text, tool calls or
their output, another Skill or installation directory, another session, and
the available-Skill listing of the session metadata do not identify a loaded
Skill. Commands for both hosts still come from the phase's own events.

Core commands are recognized only in command position: the command word of a
simple command that the observed shell text runs. The text is split into
words with shell quoting, never evaluated. Simple commands are separated by
`;`, `&`, `&&`, `|`, `||`, parentheses or a newline. Leading variable
assignments and the reserved words `if`, `then`, `do`, `!`, `{` and similar
are skipped, as are `env` (with its options and assignments), `command` and
`exec`; `command -v` and `command -V` only describe a command. Commands inside
`$(…)`, backquotes and process substitution count, and so do the static
scripts given to `sh -c`, `bash -c`, `zsh -c`, `dash -c` or `ksh -c`,
including combined options such as `zsh -lc '…'`, and the literal `cmd`
strings of Codex `exec_command` calls.

A command word is Core when its final path component is `ha`: a literal `ha`
or path ending in `/ha`, or a variable statically assigned such a path
earlier in the same command text: `HA=/path/to/ha`, quoted values and
`export`, separated by `;`, `&&` or a newline, and used as `$VAR`, `"$VAR"`
or `${VAR}`. A variable assigned in another tool call, one whose latest
assignment uses command substitution or another computed value, one naming a
different program, and an unassigned variable are not Core commands. The word
after the command word is the operation. The same recognition decides the Seal
phase's required `start`, `check` (with `--baseline` among the arguments of at
least one `check`) and `done`, and the Spec phase's forbidden `start`,
`check`, `done`, `note` and `budget`.

Text outside command position never runs Core, so the Spec phase may print or
quote it and the Seal phase cannot satisfy a requirement with it:

| Observed text | Core operation |
| --- | --- |
| `ha start goal`, `/opt/bin/ha check goal AC-1`, `cd repo && ha done goal` | yes |
| `HA=/opt/bin/ha; "$HA" start goal`, `export HA=/opt/bin/ha` then `${HA} done goal` on the next line | yes |
| `env TRACE=1 ha check goal --baseline AC-1`, `command ha check goal AC-1`, `exec ha done goal` | yes |
| `echo $(ha note goal input)`, ``x=`ha start goal` ``, `zsh -lc 'ha check goal AC-1'` | yes |
| `HA=/opt/ha; printf '%s' '$HA start goal'`, `echo "ha start"`, `HA=/opt/bin/ha; echo "$HA" start goal` | no: arguments of `printf` and `echo` |
| `git commit -m "ha done goal"`, `NEXT="$HA start goal"` | no: a quoted string not passed to a shell |
| `ls # ha start goal` | no: a comment |
| `cat <<'EOF'` with `ha start goal` in the body | no: a here-document body |
| `HA=$(command -v ha); $HA start goal` | no: a computed value |

These forms are not interpreted, so Core commands inside them are not
recognized: a here-document or here-string fed to a shell, `eval`, `source`
and script files, other command runners such as `sudo`, `xargs`, `nohup`,
`timeout` or `find -exec`, aliases and functions, and Core options placed
before the operation.

The final repository must retain the `/3` start, real expected baseline failure,
current pass, complete done record, and referenced raw logs. The checker checks
record chaining, log digests, native tool-command observations, installed Core
identity and a fresh read-only status. This is local corroboration of captured
observations, not signed attestation against an author forging all evidence.

The example acquisition check reads the release archive's reference producer
and runnable example, then runs the archive validator's isolated native smoke.
It does not install anything. The real two-host observations remain separate
from this synthetic package smoke.
