# Contract: Spar configuration

[한국어](spar-config.md) · **English**

> This is a translation of the [Korean original](spar-config.md). Where the two differ, the Korean original decides.

Two configuration files decide which reviewers `spar` starts and which caps, angles, rules and check paths it uses. The user configuration chooses the reviewers, and a repository's project configuration cannot choose reviewers. The format is [TOML 1.0.0](https://toml.io/en/v1.0.0), and `spar` reads it with `github.com/pelletier/go-toml/v2` v2.2.4 (G17). The table in [Input formats](spar-input.en.md) defines the accepted format of each value and the result outside that format, and this document points to its rows (G16 to G25). The names, paths and models in the examples below are synthetic.

## 1. User configuration and source order

The reviewer list comes only from the first source found.

1. The file that the environment variable `SPAR_CONFIG` points to
2. `$XDG_CONFIG_HOME/jaekit/spar.toml`
3. `~/.config/jaekit/spar.toml`
4. When none of these three exists, the `reviewers` list of the one `config.json` that clean-review reads. That file is chosen the same way clean-review chooses it: the file in the environment variable `CLEAN_REVIEW_CONFIG` when it is set, otherwise `$XDG_CONFIG_HOME/clean-review/config.json`, and `~/.config/clean-review/config.json` when `XDG_CONFIG_HOME` is not set either. When the chosen file does not exist, no other path is searched and the next source is used. Each name is the preset of the same name (`claude`, `codex`, `agy`, `native`).
5. Without that either, `claude`, `codex`, `agy`

- Source files (G16): when the path does not exist (a broken symbolic link included), the next source is used and the skipped path is kept in `source.skipped`. When an existing path is not a regular file (a folder, a FIFO and so on) or the file is invalid, spar does not move on and ends with a configuration error. The clean-review `config.json` (G18) is a JSON object, and `reviewers` is an array of name strings (`null` is also an invalid value).
- When the source file has no `reviewers` key, the default list applies; when the list is empty, `native` alone. The file's top-level `timeout_seconds` is the default for every reviewer in these cases too (G19).
- `--reviewers a,b` (G25) of `spar run` and `spar config` takes precedence over the configuration. A name in the configuration uses that configuration, and a name not in it uses the preset of the same name. An empty value (`--reviewers ""`) and a value with only empty items mean `native` alone. It is kept in `override` of the applied configuration record as `{"kind": "--reviewers", "names": [...]}`.

### Format

```toml
timeout_seconds = 1800          # default time limit for every reviewer (seconds)

[[reviewers]]
name = "codex"                  # name used in records and --reviewers; the preset name when omitted
preset = "codex"

[[reviewers]]
name = "agy"
preset = "agy"
model = "example-model-pro"
fallback_model = "example-model-flash"

[[reviewers]]
name = "example-reviewer"
preset = "command"
argv = ["/opt/example/bin/example-reviewer", "--prompt", "{prompt}", "--workspace", "{workspace}", "--schema", "{schema}"]
timeout_seconds = 600
```

| Key | Meaning |
| --- | --- |
| `name` | The name used in records and in `--reviewers` (G20). When omitted, the preset name. Names that are the same within one file, ignoring case, are an error. The names spar writes in a round folder, and their `.tmp` names, cannot be used |
| `preset` | One of `claude`, `codex`, `agy`, `native`, `command`. Required (G21) |
| `argv` | Only for `command`, and required for `command`. An argument array run as is without a shell, in which `{prompt}`, `{workspace}` and `{schema}` are replaced with the paths of the packet file, the workspace and the result schema |
| `binary` | The executable of `claude`, `codex` or `agy`. Without it, the preset name is looked up on PATH |
| `model` | The model to request |
| `fallback_model` | The model used once in the same round when the designated model is refused for usage or rate limits. An empty string means no fallback model |
| `effort` | The reasoning effort of `claude` or `codex` |
| `timeout_seconds` | A positive integer (G19). The per-reviewer value takes precedence over the top-level value, and the top-level value is the default for every reviewer that file determines. The executable's version check also runs within this time |

Preset defaults:

| preset | Isolation grade | Required | Default model | Default fallback model | Default effort |
| --- | --- | --- | --- | --- | --- |
| `claude` | `verified` | Yes | `opus` | None | `high` |
| `codex` | `verified` | Yes | `gpt-6-astra` | None | `high` |
| `agy` | `verified` | Yes | `gemini-4-argon-high` | `gemini-3.8-flash-high` | None |
| `native` | `host` | Yes | None | None | None |
| `command` | `unverified` | No | None | None | None |

Giving `binary`, `model`, `fallback_model` or `effort` to a preset that does not use it is an error (`agy` does not use `effort`). The rule for finding the executable: G14.

## 2. Project configuration

`.jaekit/spar.toml` at the repository root has only the following keys.

```toml
angles = ["requirement-counterexample", "check-validity"]
rules = ["AGENTS.md"]
check_paths = ["tools/checks/**", "**/*_test.go"]

[caps]
plan = 1
plan_delta = 1
code = 3
```

The project configuration is read from the file in the working tree (G16).

- `caps`: the number of rounds per stage that one series can count. The defaults are `plan` 1, `plan_delta` 1 and `code` 3 (candidate and fixes together). Integers of 0 or more (G22).
- `angles`: the attack angles the packet asks about. The default is the five angles `requirement-counterexample`, `side-effects`, `design`, `premises`, `check-validity`, and it can be reduced to some of them. The order is normalized to the order above (G22).
- `rules`: the rule documents to put in the packet of every round (G23). Each must be a file readable inside the repository; otherwise both `spar config` and `spar run` end with a configuration error.
- `check_paths`: globs of the paths treated as check code (G24). An invalid glob is a configuration error. Classification, the gate's `growth` and the packet use them.

`reviewers`, `preset`, `command`, `argv`, `binary`, `model`, `fallback_model` and `effort` are refused at any depth with an error that names the key and its location. This keeps a cloned repository from running commands on the user's computer or changing the costs on the user's account.

## 3. Errors

A configuration error starts no reviewer, creates no round folder and ends with exit code 65. The message has the form `<파일>:<줄>[:<열>]: <내용>`, and the content names the key or value at fault.

- TOML syntax error (including what TOML 1.0 does not allow and defining the same table twice): the line of the error
- Unknown key: the line of that key
- Duplicate reviewer name (ignoring case), reserved name, unknown preset, key not used by the preset: the line of that key
- Invalid `rules` or `check_paths` item: the line of that key and the item
- Source path that is not a regular file: that path
- `command` without `argv`, an entry without `preset`: the line of that entry's `[[reviewers]]` header
- Forbidden key in the project configuration: the line of that key

## 4. Applied configuration record

`spar config [--repo DIR] [--reviewers a,b]` prints the configuration that will apply as JSON, and `config` in the round record `round.json` has the same value.

| Field | Meaning |
| --- | --- |
| `source` | `kind` (`SPAR_CONFIG`, `XDG_CONFIG_HOME`, `HOME`, `clean-review`, `default`), `path`, `skipped`, `note` |
| `override` | The names when `--reviewers` was used, otherwise `null` |
| `configured_required` | `name` and `preset` of the reviewers that the configuration makes required, regardless of `--reviewers`. The gate treats them as required reviewers |
| `files` | The path and sha256 of each configuration file read |
| `reviewers` | The applied reviewers (`name`, `preset`, `isolation`, `required`, `model`, `fallback_model`, `effort`, `binary`, `argv`, `timeout_seconds`) |
| `angles`, `caps`, `check_paths`, `rules` | The applied values |
| `digest` | The sha256 of the canonical JSON of the reviewers, `configured_required`, angles, caps, check paths and rules. It is the same when the applied values are the same, even from different sources. Even with the same `--reviewers`, the digest differs when the reviewers that the configuration makes required differ |

`spar run --check-path GLOB` adds to that run's `check_paths` and is reflected in the digest. The format is the same as `check_paths` (G24), and an invalid glob is refused before any reviewer starts.
