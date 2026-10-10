# Release acceptance checks

Run `python3 tools/release-checks/check.py AC-1` through `AC-11` explicitly.
These checks do not publish, install packages, change host configuration, or
start a model. They read public GitHub data, installed files, and separately
captured observations. Maintenance criteria also run existing local regressions.
They are not part of ordinary offline CI.

The private evidence index is `ha/release-0-1-3/evidence.json` under the Git
common directory returned by `git rev-parse --git-common-dir`. Never commit
the index, traces, host inventories, or synthetic execution records. A relative
artifact path is relative to this private directory; an absolute path identifies
an existing installation or observation. Each artifact is an object with
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
| `session_trace` | Codex rollout JSONL artifact containing session metadata and the actual model. Optional for Claude when stdout already contains both. |
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

The final repository must retain the `/3` start, real expected baseline failure,
current pass, complete done record, and referenced raw logs. The checker checks
record chaining, log digests, native tool-command observations, installed Core
identity and a fresh read-only status. This is local corroboration of captured
observations, not signed attestation against an author forging all evidence.

The example acquisition check reads the release archive's reference producer
and runnable example, then runs the archive validator's isolated native smoke.
It does not install anything. The real two-host observations remain separate
from this synthetic package smoke.
