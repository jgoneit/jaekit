# Repository instructions

These instructions apply to the entire repository.

## Products

- Spec (`plugins/spec`) turns an explicit request into goal documents. It does not depend on Seal, execution bundles, or run states.
- Seal (`plugins/seal` and the `ha` binary) carries a goal to recorded completion. Seal reads goal documents and never writes them.
- Native agents own planning, ordering, and implementation. Skills define artifacts and completion rules.
- Seal Core is deterministic. It never calls a model, and status computation never reads the current time.

## Public content

- `guides/` contains installation and usage documentation; `contracts/` defines current formats and behavior.
- Keep documentation examples synthetic. Do not commit personal conversations, local environment details, private planning records, or raw execution output.
- Root `docs/` is reserved for local development goals and is ignored. Test fixtures may use nested `docs/specs/` paths.
- Keep root `docs/` as a plain folder, without a nested Git repository or a symlink to another checkout: `ha` finds the code repository from the goal path.
- Keep goal documents local. Do not create a separate backup repository or automatically sync or transmit them. Never force-add them to the public repository.
- Put executable checks in tracked paths outside root `docs/` so baseline checks can run from the code commit.
- Publish only reviewed product files. Do not link public documentation to private goals or historical execution records.
- Change the contract and implementation together when behavior changes.

## Implementation

- Prefer the Go standard library and deterministic output.
- Execute commands as argv through `os/exec`, never as shell strings.
- Use the user's `git` CLI rather than a Git library.
- Add behavioral scenarios in `testdata/scenarios/` for changes to decisions made by `ha`.
- Keep product checks independent of private documents and earlier repository history.
- Skill bodies are written in English. Keep format tokens and identifiers unchanged when translating guides.

## Validation

From the repository root, run before committing and before pushing or publishing:

```bash
python3 tools/verify.py
```

This runs the same checks as both CI jobs: Go formatting, vet, tests, four-platform builds and CLI smoke checks; public file and document checks and their regression tests. Git, Python 3 and the Go version required by `go.mod` must already be installed. The default and `go` groups also require bash and zsh on PATH for their offline shell regressions. Before any checks run, validation verifies that both selected shell executables can start and passes those same absolute paths to the regressions. Missing or unusable shells stop validation and identify the affected Go shell regressions; they are not silently skipped. Failures stop validation with a nonzero exit code; validation does not install tools or rewrite source files.

The `docs` group does not add a Go or zsh prerequisite. Docker is only needed for the separately selected Linux container check, not ordinary validation or product use. The Ubuntu Go CI job prepares zsh before invoking the same verification entrypoint.

Main requires the `Seal Core` and `Public documentation` checks. These rules do not make files private after they have been pushed to a public branch; run the public checks locally before publishing.

A document check establishes consistency, not runtime or host installation success.
