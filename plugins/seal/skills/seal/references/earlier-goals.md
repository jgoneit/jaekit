# Goals started with an earlier Seal

Read this only when you continue a goal that an earlier Seal started; the `skill:` line of `ha status` names the Seal that started it.

Goal documents and bundle files keep the format they were started with; do not convert them. Never fill in observations the earlier form did not record; absent fields mean unrecorded, not failed or unexecuted.

`ha status` computes the state under the goal's own saved rules. Each `ha` command computes the saved rules or refuses them with exit 66 and records nothing. Report a refusal. Installing a new Seal or Core never migrates a goal's saved rules or records and never resets its usage or time limits. A goal's budget is the one `ha status` shows: set at the start, and changed only by recorded budget changes or a reopen. A budget change follows the same `ha budget` rules as any goal.

Checks, review rounds and reports you write now follow the current skill. When a `REVIEW.md` has no `## 독립 검토` section, record the tier and review stages you apply from now on under `## 검증 방법과 환경` or the completion report, without rewriting what is there. For a goal whose rules use no review gate, without Spar, a separate review recording role, inputs, versions, findings, limits and inspectable references counts if the report names it, says Spar was not used, and it covered the whole last head before `ha done`.

For the exact earlier formats, read the contracts. [run-record.md §4.6](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#46-rules-versions) and the sections it links give each goal's saved rules, and [bundle.md](https://github.com/jgoneit/jaekit/blob/main/contracts/bundle.md) the bundle forms. Where a contract fixes an earlier format, that format applies.
