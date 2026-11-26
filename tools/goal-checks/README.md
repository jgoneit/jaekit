# Repository check producers

`produce.py` observes the repository's declared Go and unittest checks. It
does not install dependencies or call a model. Actual assertion failures are
violations; compile, collection, import and execution errors are separate
results. An ordinary assertion can legitimately make `go test` exit 1.
Panics, signals, unfinished tests and inconsistent package/process outcomes
are execution errors, including failures after a selected test has passed.
Both the ordinary Go runner and the document mutation probe use this rule.
A crashing document probe cannot count as successful rejection of a sample
statement. Diagnostics from both test and package events remain in the output.

The unittest runners in `produce.py` and `tools/review-checks/produce.py` treat
`expectedFailure` and unexpected-success results as `error/setup_error`, not
passing checks or declared violations. This also applies when ordinary passing,
failing or skipped cases appear in the same selected result. An independently
reported runtime error still takes precedence as `error/execution_error`.
Existing test-selection interfaces and undecorated test results are unchanged.
