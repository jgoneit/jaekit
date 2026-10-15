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

From the repository root, run this before pushing or publishing:

```bash
python3 tools/verify.py
```

The full verification must pass once on the same verification input as the final candidate. That one pass satisfies both the commit and the push requirement, whether it runs before or after the commit; do not run the full verification twice for the same input. The verification input is the tracked files (content, path, mode and tracking state), the verification command and group, and the tools, settings and environment that affect the result. If any of them changes, an earlier pass is not evidence for the new input; a different commit ID alone is not such a change. Narrower checks that fit the change are fine during development. This reuse only removes duplicate runs from these instructions: it does not establish success with other tools, settings or environments, does not make an ordinary run a Seal record, and does not change freshness, baseline, failure history, independent review or required CI requirements.

This runs the common checks also used by the Ubuntu and documentation CI jobs: Go formatting, vet, tests, four-platform builds and CLI smoke checks, plus Windows candidate cross-build (without execution); public file and document checks and their regression tests. CI separately runs native Core and isolated installation checks on Ubuntu/macOS, and native developer checks on Windows Server with PowerShell 5.1 and 7. The local command does not claim execution on those other hosts. Git, Python 3 and the Go version required by `go.mod` must already be installed. The default and `go` groups also require bash and zsh on PATH for their offline shell regressions. Before any checks run, validation verifies that both selected shell executables can start and passes those same absolute paths to the regressions. Missing or unusable shells stop validation and identify the affected Go shell regressions; they are not silently skipped. Untracked files that Git does not ignore also stop validation before any check runs and are listed; track the files validation needs, or review the ignore rules for local files. Failures stop validation with a nonzero exit code; validation does not install tools or rewrite source files. Each step's result line and the final line show elapsed seconds.

The `docs` group does not add a Go or zsh prerequisite. Docker is only needed for the separately selected Linux container check, not ordinary validation or product use. The Ubuntu Go CI job prepares zsh before invoking the same verification entrypoint.

Main requires the `Seal Core` and `Public documentation` checks. `Seal Core` succeeds only when `Core / Ubuntu`, `Core / macOS`, and `Core / Windows` from the same workflow run all succeed. Failure, missing results, skipping, cancellation, and pending work do not satisfy this gate. See [the CI guide](guides/CI.en.md) for each job's observed scope and source identity. These rules do not make files private after they have been pushed to a public branch; run the public checks locally before publishing.

A document check establishes consistency, not runtime or host installation success.

Windows candidate development has a separate `tools/windows/native_checks.ps1` entrypoint for explicitly selected shell-independent runtime fixtures. It requires native Windows, source, Git, Go and Python; Windows Server CI may run these developer checks while product installation still requires Windows 11 Client x64. It is not an installation prerequisite or a substitute for installed-binary and PowerShell lifecycle acceptance on Windows 11. The shared verifier still targets the existing Unix development environment and never represents cross-compilation as Windows runtime evidence.
