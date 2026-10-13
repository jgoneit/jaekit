# Native evidence-path command scenario

`TestNativeEvidencePathScenarios` and the Windows-only
`TestNativeWindowsEvidencePathScenarios` build the current `ha` executable and
invoke it as a child process against a real temporary Git repository. They
read `cases.json`, copy this producer to a committed path containing Korean
characters and spaces, and exercise structured baseline and normal checks.

Every unsafe case is tested both as the declaration path in PLAN and as a
producer path inside a committed declaration. Both normal and baseline
commands must exit 65 with `evidence_invalid` and the path-validation reason,
before the producer executes. Existing records, raw logs, status counters,
source files, the Git index/configuration, and an outside sentinel must remain
unchanged. Restoring ordinary paths must execute the producer and append valid
evidence again.

The Windows entrypoint is compiled only on Windows and contains no skip. It
uses the host's real drive and Windows path semantics; extension-bearing names
such as `CON.txt` are not assumed to be devices. Their OS-dependent regular-file
behavior is covered separately by `TestNativeWindowsEvidencePaths`.

`_native/` deliberately uses the scenario runner's existing underscore rule:
these JSON inputs are consumed by the native Go tests, never by the shell
`steps` interpreter. The two named entrypoints are required in Windows CI.
