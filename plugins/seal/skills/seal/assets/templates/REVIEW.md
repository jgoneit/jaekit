# REVIEW — <goal>

## 요구사항 대응
<ha lint result; SPEC scope and non-goals reflected in tasks; user scenarios covered by the condition table; differences from the work breakdown proposal and why>

## 불필요한 task
<none, or tasks that serve no condition>

## 문서 충돌
<none, or conflicts among SPEC, project documents, and code>

## 중요한 미결정
<none, or each open decision closed by the user's quoted answer or an explicit assumption; a reading that keeps a component's declared scope or trust boundary is such an assumption, not a question>

## 검증 방법과 환경
<whether each command runs here; tools, data, network and repository history (a check reads history only when its condition is about history; name the range it reads); how a cross-component flow is checked across the boundary>

<for goals started with seal 0.1.10 or later; existing goals retain their starting format:
- Expected-result grounds: requirement/public contract/agreed invariant and location; unresolved/conflicting grounds stay explicit
- Planned observation boundary: scope and relevant environment; planned is not observed
- Method and substitutes: synthetic inputs, fakes, fault injection or comparisons and what they replace
- Failure classes not covered: gaps in the general property, examples and assumptions; check count is not a sufficiency score
- Map every condition; shared condition groups may reference one explanation without repeated tables>

## 독립 검토
<the SPEC `Assurance:` tier or none, the project instructions' floor and the one followed; the planned review stages (a plan round before any baseline for `strict`; candidate, fixes and the cumulative round from the base commit to the last head before `ha done`) or the reason a `light` goal skips them; the product invariants and decision rules this change introduces or alters, for the plan round; the supported input forms of each open-ended input a new contract or check covers, never narrower than a criterion, contract or protected regression path requires; references to the round and gate records once rounds run, never copied findings>

## 결론
<ready | needs_user: reason>

<!-- Assurance policy: identify condition groups, risk and exclusions, required or optional guarantees and policy source. Record review role, provided context, code and check versions, inspectable evidence, findings/no findings/not performed and limits. Connect each counterexample to reproduction/rejection, general property, repair and remaining gaps. Required gaps keep the affected work unfinished; chosen methods and timing remain with the executor. -->
