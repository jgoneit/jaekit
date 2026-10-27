# Building from source and registering for development

[한국어](DEVELOPMENT.md) · **English**

Read this when you change or try out Jaekit itself. You do not need it to use Jaekit; to use it, follow the [install guide](INSTALL.en.md).

## Build from source (development)

An `ha` built from a repository checkout shows a development version (`ha 0.1.8-dev`). You need Go 1.27 or later and git. Run this from the root of the checkout.

```bash
go install ./cmd/ha
ha --version
```

The `ha` file goes to the folder `go env GOBIN` prints, or to `$(go env GOPATH)/bin` when that is empty. `ha --version` prints `ha 0.1.8-dev`. Seal separately checks the actual support information from `ha capabilities --format json`. A Python compatibility helper is optional; Python 3 installation is not a prerequisite for Seal.

To use the plugins while you change the checkout, register the checkout path as the marketplace. That registration uses the plugin files in the checkout as they are. Replace `<path to jaekit>` with the path of your checkout.

Codex:

```bash
codex plugin marketplace add <path to jaekit>
codex plugin add spec@jaekit
codex plugin add seal@jaekit
codex plugin add spar@jaekit
```

Claude Code:

```bash
claude plugin marketplace add <path to jaekit>
claude plugin install spec@jaekit
claude plugin install seal@jaekit
claude plugin install spar@jaekit
```

Commands and the record format are in [contracts/run-record.en.md](../contracts/run-record.en.md).

## Repository checks

After a change, run the common checks from the repository root. The tools they need and what they cover are in [Validation in the repository instructions](../AGENTS.md#validation).

```bash
python3 tools/verify.py
```

## What to read next

- [Reading CI results](CI.en.md): the checks that run on pull requests and main, and what each environment observes
- [Native Windows candidate](WINDOWS.en.md): how the native Windows candidate under development is checked
- [Common regression connections](../contracts/regressions.md): Jaekit's common invariants and the regression checks that guard them
- [Repository check producers](../contracts/repository-producers.en.md): what the repository's check tools observe
