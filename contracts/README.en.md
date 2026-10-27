# Public product contracts

[한국어](README.md) · **English**

> This is a translation of the [Korean original](README.md). Where the two differ, the Korean original decides.

These contracts define the current document formats and status computation of Spec and Seal, and the configuration and review record formats of Spar. The user requests, times and record numbers in the examples are there to explain the formats; they are not evidence of real work.

- [Goal documents](goal-docs.en.md): the Markdown that Spec writes and Seal reads
- [Execution bundle](bundle.en.md): plan, progress and completion report
- [Run record](run-record.en.md): `ha` commands, `run/v1`, `status/v1`, rule versions
- [Decision history](decision-history.en.md): optional `ha history`, `decision-history/v1`, event relations and current judgments
- [Shared regression links](regressions.md): the product paths of I1–I5, protected regressions and open gaps
- [Role of the Seal executor](role-card.en.md): permissions and completion rules
- [Spar input format](spar-input.en.md): the format of every input field `spar` accepts, the results for input outside the format, and their consumers (G1–G33)
- [Spar configuration](spar-config.en.md): location and format of `spar.toml`, reviewer presets, the scope of project configuration
- [Review round](review-round.en.md): `review-round/v1`, packets, reviewer states, finding classification and disposition, caps
- [Review gate](review-gate.en.md): `review-gate/v1`, states and exit codes, line count of review fixes

Installation and actual use start from the [usage guide](../guides/USAGE.en.md).
