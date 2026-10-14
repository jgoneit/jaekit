# Native baseline junction command scenarios

`TestNativeWindowsBaselineJunctionScenarios` reads `cases.json` and invokes an
actual `ha.exe` built from the current source. It runs without bash, zsh, or a
shell-based checker. Each run owns a synthetic repository, goal, committed
producer, declaration, source files, external sentinel, records, and raw logs.
The fixture never uses an installed `ha` or an existing user goal.

| Case | Required observation |
| --- | --- |
| `regular-before` | Actual checker receives the committed HEAD overlay in the baseline checkout, declares the expected violation, and records one usable baseline attempt and matching raw-output digest. |
| `parent-junction` | A junction in the actual baseline overlay's parent causes pre-execution exit 65 with the relative path, unsafe-entry reason, and zero executed targets. |
| `leaf-junction` | A junction replacing the actual baseline overlay destination causes the same specific pre-execution refusal. |
| `partial-batch` | One CLI request executes its first target, refuses its second baseline checkout, and never creates the third. Exactly the first record, raw log and attempted-run charge survive. |
| `regular-after` | A safe baseline still executes and supplies usable evidence after all refusals. |

The test first records a real baseline and current pass, so refusal checks
preserve existing usable evidence, nonempty history and raw output. Complete
command return, including worktree cleanup, must preserve the original source,
index and configuration, external bytes and permissions, prior records and
logs, and existing condition satisfaction. An unattempted target cannot add a
record, pending output, result report or usage. Registered and on-disk temporary
baseline worktrees must be cleaned up.
Windows permission observations include the built-in read-only `icacls /T`
listing of the external directory and file DACLs; Unix observations use file
permission modes. Neither observation changes permissions.

`fixture.go` is compiled as an ordinary producer and a disposable Git proxy.
The proxy passes the exact argv to the previously discovered real Git. Only
after real `git worktree add` succeeds does it place the selected junction in
the synthetic repository's actual `ha/worktrees/base-*/tree` directory. It
refuses a mutation outside that bounded location. It observes successful
fixture creation independently of the product diagnostic; setup failure is not
accepted as junction refusal. The proxy neither changes a product executable
nor introduces a public injection option. `cmd.exe /c mklink /J` is used only to
create a real Windows directory junction in this disposable fixture.

The portable `TestNativeBaselineSymlinkScenarios` runs the same command driver
with Unix directory symlinks to check orchestration and assertions locally.
Those results are not Windows junction evidence. Windows results require the
Windows-only entry point and all five native case results, without skipping.
