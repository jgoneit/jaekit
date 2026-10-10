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

Run before committing:

```bash
gofmt -l .
go vet ./...
go test ./...
go build ./cmd/ha && ./ha --version && ./ha --help
python3 tools/check_public_tree.py
python3 tools/check_public_docs.py
```

A document check establishes consistency, not runtime or host installation success.
