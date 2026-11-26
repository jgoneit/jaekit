# Release archive tools

The release workflow checks its archives with the scripts in this folder before publication.

## Archive resource limits

Every tar.gz archive consumer reads through the same read-only parser,
[`tools/release/archive_reader.py`](archive_reader.py): the standalone
validator [`tools/release/check_archives.py`](check_archives.py) for `--dist`
files and its native smoke. Reading one needed file never skips another
limit. The Windows ZIP is not read by this parser:
`check_archives.windows_files` reads it in memory with at most 2,048 entries,
32 MiB per file and 128 MiB in total, within the same 64 MiB compressed limit.

Each archive is at most 64 MiB compressed. It permits at most 256 MiB of
cumulative member bodies and 10,000 physical entries, including directories,
empty files, and PAX/GNU metadata headers. The exact limits are allowed; the
next body is rejected before reading it if its declared size or entry would
exceed either limit. Expanded logical
file contents are separately limited to 256 MiB so metadata cannot override that
bound. Individual files remain limited to 64 MiB. All decompressed bytes,
including metadata, padding, and trailing streams, are bounded by 512 MiB.

Known limitation ([#49](https://github.com/jgoneit/jaekit/issues/49)): an
unsupported special type with a PAX-overridden size can consume more than the
cumulative body limit before the consumer rejects its type. The expanded
stream limit still applies. Such an archive is not accepted as a release.

Each PAX or GNU long-name metadata body is limited to 64 KiB, and all metadata
bodies in one archive to 1 MiB. Both are checked from the header, before
`tarfile` reads, decodes, or splits the body into records, so long names and
the number of PAX records cannot grow parsed objects beyond these bodies.
Ordinary long names and PAX attributes remain supported.

Accumulated global PAX state and each effective global/local attribute map are
limited to 128 distinct keys. Replacing a key does not add another key. The
global bound is checked before another header can copy that state; the effective
bound is checked before copying attributes into a member. Together with the
entry limit, this bounds retained dictionary amplification from small global
headers and recursive local headers. Trailing slashes in PAX paths are
normalized once in shared state so repeated global paths reuse that string.
Metadata body limits still bound parsing
of an individual header; this is not an absolute process memory limit.

Limit errors identify compressed size, cumulative bytes, entry count, metadata,
PAX keys, or expanded stream. Invalid sizes, corrupt or incomplete tar/gzip data
(including a corrupt concatenated gzip member), and nonzero trailing data are
rejected as an invalid or incomplete archive even if all required files were
already encountered; zero padding and zero-filled concatenated gzip members
remain accepted. The validator exits 1 with the cause.

Sparse extensions are unsupported and rejected before reading sparse maps.
PAX size overrides must be nonnegative decimal integers. Paths are inspected in
memory without extraction or input mutation; native smoke writes only bounded
bodies into its temporary directory and removes it on success or refusal. The
validator checks each checksum against exactly the bytes it parses. Existing
root, traversal, type, duplicate, and required-file checks still apply, and the
validator keeps its stricter canonical path, allowlist, permission, executable
and platform checks. Offline regressions use small patched limits and synthetic
archives; they do not claim a live release observation or a measured
memory-exhaustion test.
