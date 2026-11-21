# Public product contracts

These contracts define the current document formats and status computation of Spec and Seal, and the configuration and review record formats of Spar. The user requests, times and record numbers in the examples are there to explain the formats; they are not evidence of real work.

- [Goal documents](goal-docs.md): the Markdown that Spec writes and Seal reads
- [Execution bundle](bundle.md): plan, progress and completion report
- [Run record](run-record.md): `ha` commands, `run/v1`, `status/v1`, rule versions
- [Shared regression links](regressions.md): invariants I1–I5 and the protected tests that guard them
- [Role of the Seal executor](role-card.md): permissions and completion rules
- [Spar input format](spar-input.md): the format of every input field `spar` accepts, the results for input outside the format, and their consumers (G1–G33)
- [Spar configuration](spar-config.md): location and format of `spar.toml`, reviewer presets, the scope of project configuration
- [Review round](review-round.md): `review-round/v1`, packets, reviewer states, finding classification and disposition, caps
- [Review gate](review-gate.md): `review-gate/v1`, states and exit codes, line count of review fixes

Installation and actual use start from the [usage guide](../guides/USAGE.en.md).
