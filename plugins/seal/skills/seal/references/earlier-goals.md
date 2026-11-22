# Goals started with an earlier Seal

Read this only when you continue a goal that an earlier Seal started; the `skill:` line of `ha status` names the Seal that started it.

Goal documents and bundle files keep the format they were started with; do not convert them. Never fill in observations an earlier form did not record: a field it lacks means unrecorded, not failed or unexecuted. Keep the PLAN's baseline marks as they are.

`ha status` computes the state under the goal's own saved rules. Each `ha` command computes those rules or refuses them with exit 66 and records nothing; report a refusal. Installing a new Seal or Core never migrates a goal's saved rules or records and never resets its usage or time limits. The budget is the one its PLAN gave at the start, changed only through `ha budget`.

Checks, review rounds and reports you write now follow the current skill. When an earlier document has no place for something the current skill records, such as the assurance tier and review stages in a `REVIEW.md` without `## 독립 검토`, write it under the nearest existing section or in the report, without rewriting what is there.

For the exact earlier formats, read the contracts. [run-record.md §4.6](https://github.com/jgoneit/jaekit/blob/main/contracts/run-record.md#46-rules-versions) and the sections it links give each goal's saved rules, and [bundle.md](https://github.com/jgoneit/jaekit/blob/main/contracts/bundle.md) the bundle forms and the review a goal whose rules use no review gate accepts without Spar.
