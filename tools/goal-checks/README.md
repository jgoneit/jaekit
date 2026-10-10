# Repository check producers and audits

`produce.py` observes the repository's declared Go and unittest checks. It
does not install dependencies or call a model. Actual assertion failures are
violations; compile, collection, import and execution errors are separate
results. An ordinary assertion can legitimately make `go test` exit 1.
Panics, signals, unfinished tests and inconsistent package/process outcomes
are execution errors, including failures after a selected test has passed.
Both the ordinary Go runner and the document mutation probe use this rule.
A crashing document probe cannot count as successful rejection of a sample
statement. Diagnostics from both test and package events remain in the output.

## Current files

Run the current-file reference check from the checkout root:

```sh
python3 tools/goal-checks/release-0-1-3-review-fixes/test_private_independence.py TrackedFiles
```

It reads tracked files, without opening local goal material or earlier
commits. The `FreshClone` case additionally requires the ordinary checks to
succeed in full and depth-1 clones. Equal errors in two checkouts are not proof
of independence. Synthetic forbidden-content cases check that rejection still
works. No real private file hashes are fixtures: a hash's shape alone cannot
identify it as private. One-off comparisons with private material stay local.

The public release identity check is separate and opt-in:

```sh
python3 tools/goal-checks/release-0-1-3-followups/preserved.py
```

It checks current public files and the recorded public v0.1.3 tag, release and
assets using GitHub reads. It needs network access and `gh`, but no earlier
Git commits or private observations. Missing public inputs are unverified,
not a successful preservation result.

## Explicit history audit

To audit added lines and commit messages, provide both full commit IDs:

```sh
python3 tools/goal-checks/release-0-1-3-followups/preserved.py \
  --history --base "$BASE_SHA" --head "$HEAD_SHA"
```

The same explicit mode is available through
`release-0-1-3-review-fixes/test_private_independence.py`. Neither entry point
chooses a hidden base or runs this audit during current-file checks. This mode
checks only the named Git range; it does not claim that a release or a private
record was preserved. The base and head must be available locally, and a
shallow checkout must contain the entire named range, including merge parents. Git objects
are never fetched automatically, including lazy fetching in a partial clone.

The audit exits 0 for accepted changes, 1 for forbidden references, and 2 with
`UNVERIFIED` when the required history is absent. A former goal's explicit
range in a target file audits only that named range; it does not certify later
changes or rewrite the goal's historical completion records.
