# Building from source and registering for development

[한국어](DEVELOPMENT.md) · **English**

Read this when you change or try out Jaekit itself. You do not need it to use Jaekit; to use it, follow the [install guide](INSTALL.en.md).

## Build from source (development)

An `ha` built from a repository checkout shows a development version (`ha 0.1.8-dev`). You need Go 1.27 or later and git. Run this from the root of the checkout.

```bash
go install ./cmd/ha ./cmd/spar
ha --version
spar --version
```

The `ha` and `spar` files go to the folder `go env GOBIN` prints, or to `$(go env GOPATH)/bin` when that is empty. `ha --version` prints `ha 0.1.8-dev`, and `spar --version` prints `spar 0.1.8-dev`. Seal separately checks the actual support information from `ha capabilities --format json`. A Python compatibility helper is optional; Python 3 installation is not a prerequisite for Seal.

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

Commands and the record format are in [contracts/run-record.md](../contracts/run-record.md).

## Skill size

Each skill has a word limit: whitespace-separated words, front matter included. [`TestSkillWordCaps`](../plugins/skill_tokens_test.go) and [`TestSkillWordCapsStated`](../plugins/skill_tokens_test.go) set these limits.

- The [Spec skill](../plugins/spec/skills/spec/SKILL.md) has a limit of 2,800 words.
- The [Spar skill](../plugins/spar/skills/spar/SKILL.md) has a limit of 2,600 words.
- The [Seal skill](../plugins/seal/skills/seal/SKILL.md) has a limit of 2,300 words. [`TestSealSkillWordCap`](../plugins/seal_skill_rules_test.go) and [`TestSealSkillWordCapStated`](../plugins/seal_skill_rules_test.go) check it too.

When a change would push a skill above its limit, the same change moves rules used only in rare situations to that skill's reference documents. A limit is raised only by the user's words.

Rules only the user decides, and things the agent must not do, stay in the skill body. The body links a moved section with a line that says when to read it; when the skill already links that section, the skill does not change. Skill rules tests read format tokens (code spans, headings, link targets), not sentences, so rewording a sentence or moving it to a reference document does not fail them. Phrases a protected contract check requires move verbatim.

## Repository checks

After a change, run the common checks from the repository root. The tools they need and what they cover are in [Validation in the repository instructions](../AGENTS.md#validation).

```bash
python3 tools/verify.py
```

Protected tests are listed in [contracts/regressions.json](../contracts/regressions.json); [Common regression connections](../contracts/regressions.md) says how to add one. Deleting or weakening a protected test needs the user's words in the pull request.

## What to read next

- [Reading CI results](CI.en.md): the checks that run on pull requests and main, and what each environment observes
- [Native Windows candidate](WINDOWS.en.md): how the native Windows candidate under development is checked
- [Common regression connections](../contracts/regressions.md): Jaekit's common invariants and the regression checks that guard them
- [Repository check producers](../contracts/repository-producers.md): what the repository's check tools observe
