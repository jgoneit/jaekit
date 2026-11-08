# Understanding Jaekit

[한국어](OVERVIEW.md) · **English**

Jaekit lets you check from records whether work you handed to Claude Code or Codex is really done. It is made of plugins for your agent, Spec, Seal and Spar, and two terminal programs, `ha` and `spar`. Before the work starts, it writes down what finished means; at the end, it judges completion from records of checks that actually ran, not from the agent's word.

This page is for people new to Jaekit. In order, it covers:

- why it is needed
- what changes when you use it
- who does what
- how one goal goes through
- what completion means

Then install with the [install guide](INSTALL.en.md) and follow [your first task](../README.md#your-first-task) in the README.

## When your agent says "done"

When you hand work to Claude Code or Codex, this can happen:

- It does seven of ten conditions and stops.
- It reads a hard condition as an easier one.
- It stops early, and you keep saying "finish the rest".
- It says "the tests pass", but to learn what it actually checked you have to read the changed code from the top.
- When the conversation ends, you explain again what was promised and how far it was checked.

Approving every step keeps this in check, but then there is little point in handing the work over. Trusting the report is easy, but an agent can edit its own checks or write easier criteria. "The tests pass" is not the same as "the goal is met".

## Jaekit's idea: delegate the work, judge the end by records

Jaekit holds on to three things and leaves the rest to the agent.

1. **You set the goal.** What finished means, the completion conditions, is written down before the work starts. While implementing, the agent does not edit that document.
2. **The agent does the work.** Planning, order and tools are the agent's choice. You do not approve each step; it asks only when a decision would change the outcome.
3. **Completion is judged from records.** A goal is recorded as complete only when each required completion condition has a record of its check. A condition that cannot be checked automatically is met by recording your confirmation. For new behavior, the check must fail on the code before the change, because the behavior is missing, and pass after it: a check that already passes before the change cannot tell whether the new behavior exists. A check that guards existing behavior may pass from the start.

## What changes when you use it

Here is the same work handed over in a plain conversation and through Jaekit. Either way, the same agent does the work. What changes is the record.

| When | Plain conversation | With Jaekit |
| --- | --- | --- |
| Starting the work | What finished means is scattered across the chat | The completion conditions are written down first, and work starts after you confirm them |
| A check runs | Only the words "it passed" are left | Each check your agent runs through `ha` is recorded on one line: the commit, the command and the result |
| The code changes again | You can't tell whether the earlier pass still holds | The earlier pass stays in the record but no longer counts. The check has to run again on the current code |
| A check that already passes before the change | A check that can't tell whether the new behavior exists still looks like a pass | A check for new behavior counts only with a record of it failing on the code before the change, because the behavior was missing |
| Existing tests are edited | A loosened test still shows "pass" | A test change the request didn't allow, or one that weakens a test, blocks completion until you confirm it |
| The goal or scope changes | It can slip by unnoticed | A changed goal document, or a file outside the goal folder changed beyond the agreed scope, blocks completion until you confirm it |
| The conversation ends | You explain everything again | In a new conversation, name the goal folder and ask to continue; the agent reads the folder and its record and picks up from there |
| "Done" | Whether it is finished is settled by words | `ha` works out completion from the record, the documents in the goal folder and Git, and records it |

## Who does what

Your agent is the only one doing the work. Spec and Seal are instructions the agent follows; `ha` and Spar are tools the agent runs. On this page, "Spec writes" or "Seal plans" means the agent does it by following those instructions.

| Who | What they do |
| --- | --- |
| You | Say what you want, review the goal document, answer decisions that change the outcome, read the completion report |
| Your agent (Claude Code, Codex) | The only one that judges, plans and implements |
| Spec | Instructions for turning a request into a goal document: goal, scope, completion conditions and a review tier. It writes and stops; it does not implement |
| Seal | Instructions that keep the agent planning, implementing, checking and fixing toward the reviewed goal until it is done. It only reads the goal document |
| `ha` | A terminal program. It records every check the agent runs through `ha check`, and computes completion from those records, the documents in the goal folder and Git state. It never calls an AI model |
| Spar | Sends the same material to reviewers outside the working agent's conversation (by default the claude, codex and agy CLIs) for an independent review, and records their findings and how each was handled |

Spec, Seal and Spar are plugins you install in Claude Code or Codex; `ha` and `spar` are terminal programs the agent runs.

## How one goal goes through

Follow a **fictional example** of adding a "Forgot password" link to a login screen. It is not an executed or verified case.

1. **You ask.** In the agent conversation, send `/spec:spec add a "Forgot password" link to the login screen` (`$spec:spec` in Codex).
2. **Spec writes the goal.** It writes the goal and completion conditions to `docs/specs/password-link/SPEC.md`. Each condition gets a number, for example `AC-1` "the login screen shows a link to the reset page" and `AC-2` "the existing login still works". It also proposes a review tier and, if nothing is left to decide, asks once whether to implement it now, and stops.
3. **You review it and answer yes.** If a condition is missing, ask for a fix. If it is right, reply yes to the question whether to implement now. Seal starts then.
4. **Seal plans.** It writes in `PLAN.md` how each condition is checked. `AC-1` is new behavior; `AC-2` is behavior that must keep working.
5. **`ha` checks and records.** The agent first runs the `AC-1` check on the code before the change and records it failing, because the link is not there yet. Then it implements and runs it again to record a pass. `AC-2` gets a passing record. Every run is appended to `runs.jsonl`, one line each.
6. **Spar reviews independently.** This goal's tier is `standard`, so reviewers in another context review the change before completion. The agent fixes the findings that need it, and the finished review is recorded as a gate.
7. **`ha done` records completion.** When every required condition is met and the other requirements, such as the review record, are in place, a completion record is written and the report is added to `PROGRESS.md`.
8. **You read the report.** Read its five lines, what changed, what was checked, what to look at, remaining limits and the next action, with the evidence per condition, and look at the actual result too.

If the conversation ends midway, open a new conversation in the same project and send `continue docs/specs/password-link`. The agent there reads the goal folder and the status `ha` computes, and continues the remaining work.

## What you get

Each goal gets one folder in your project. They are all ordinary files.

| File | Written by | Holds |
| --- | --- | --- |
| `SPEC.md` | Spec | Goal, scope, completion conditions. It is your document |
| `PLAN.md` | Seal | How each condition is checked, the paths it may change, the run budget |
| `REVIEW.md` | Seal | The review plan and a summary of the Spar records |
| `PROGRESS.md` | Seal | Progress, the review summary and the completion report |
| `runs.jsonl` | `ha` | The append-only run record |

**Review summary.** In goals started with 0.1.24 or later, after completion, `## 검수 요약` right after `## 현재` is where you start reviewing. Its top lines give the completion record, the assurance tier, how many required conditions are satisfied, the review gate result, what the change does and where to start looking; below them come changed behavior and files, scope changes, evidence, who decided, what to inspect, what was not observed, and details, in that order. It is written in the user's language and says what each record is; each item is short, with seq numbers only as an aid; no check output or finding text is copied; a finding is its ID, one line and its ending. Seal's judgment is only there, and `## 완료 보고` holds the `ha report` output and one line pointing to it. For a finished goal, read `## 검수 요약` first. For a goal that stopped, or one started with an earlier Seal, read `## 완료 보고` as before.

For a newly started goal, the first five lines of the completion report are computed by `ha` from the run record, Git and the goal documents. The agent does not rewrite them. Read the words in the report like this:

- `base`: the commit the work started from.
- `seq`: a record's number. Completion record `seq 6` is the sixth record, the one that holds the completion.
- Assurance `local`: the evidence comes from this computer.
- Check author: executor: the agent that implemented the change also wrote the checks.

More on reading it is in the [usage guide](USAGE.en.md#reading-the-completion-report).

## Review tiers

The goal document's `Assurance:` line sets how much independent review the goal gets. Spec proposes one from the size and risk of the change; to pick another, say so, for example "make it strict". If your project's instructions ask for more review, that wins.

| Tier | Used for | Review it gets |
| --- | --- | --- |
| `light` | A small fix, such as a typo or one message | May skip independent review with a reason, unless project instructions require it |
| `standard` | Ordinary changes | Spar review rounds before completion |
| `strict` | Goals that change a public contract, stored data, permissions, or a boundary between components | Also a plan review before work |

`standard` and `strict` goals newly started with the current Seal complete only with a Spar review record. Goals started with an earlier version keep the rules they started with. Before you use Spar the first time, a project needs one setup step: see [Before your first `standard` goal](../README.md#before-your-first-standard-goal) in the README. What is sent to reviewers is in the [Spar guide](SPAR.en.md#what-leaves-your-computer).

## What completion means, and what it does not

A completion record means:

- Each required completion condition has a check record or a necessary user confirmation.
- For a newly started goal, each required condition on new behavior has records of its check failing before the change, because the behavior was missing, and passing after it.
- For newly started `standard` and `strict` goals, there is a Spar record (the review gate) that reviewed the code and goal documents as they were when completion was recorded.

It does not mean:

- **That the whole result is free of defects.** Whether the checks cover the goal well enough is not something a program can decide. Independent review and your confirmation fill that gap.
- **Independent third-party verification.** The assurance level is `local`. The agent writes the checks, and an agent with the same user permissions can change the checks and the record files. The links between record lines can reveal an edited or deleted earlier record, but not a change to the last line, and they do not stop deliberate forgery.
- **Deployment or installation.** The default result is local changes and commits. Push, pull requests and deployment are yours to do, or a separate request to an ordinary agent.

Jaekit is not a security tool that polices the agent. It shows what was actually checked and what was not.

## Common questions

**What do I type in the terminal, and what in the agent conversation?**
In the terminal, mostly the install and update commands; you also use it to look at status or records yourself, and to set up Spar reviewers. In the agent conversation: `/spec:spec <request>` (`$spec:spec` in Codex), "yes" once you have reviewed the goal, your answers to questions, and `continue docs/specs/<goal>` when a conversation has ended. The agent runs the `ha` and `spar` commands.

**What appears in my project?**
A `docs/specs/<goal>/` folder for each goal. Whether Git tracks it follows your project's policy. The records can contain what you said and paths from your work, so check them before you push to a public repository. Before the first `standard` or `strict` goal that uses Spar, add `docs/reviews/` to `.gitignore`.

**How are existing tests used?**
Your whole existing test suite can serve as the check for a condition that existing behavior keeps working. Which check covers which condition is written by the agent in `PLAN.md`. The exact format is in the [execution bundle contract](../contracts/bundle.en.md).

**Does it push, open a pull request or deploy at the end?**
No. The default result is local changes and commits. Push and pull requests are yours to do, or a separate request to an ordinary agent.

**Does my code leave my machine?**
`ha` never calls an AI model. Spar reviewers send the criteria, the rules and the change (the diff) to their own providers, and project files a reviewer reads can go too: with the default reviewers, claude goes to Anthropic, codex to OpenAI and agy to Google. If code must not leave your machine, don't run Spar. Details are in the [Spar guide](SPAR.en.md#what-leaves-your-computer).

## What to read next

- To install and hand over your first goal: [Get started](../README.md#get-started) in the README
- To change a goal, continue one, or read a report: [Usage guide](USAGE.en.md)
- To set up independent review: [Spar guide](SPAR.en.md)
- To read why work stopped and what a report rests on: [Reading work status and the completion report](OPERATIONS.en.md)
- For the exact formats of files and records: [contracts](../contracts/README.en.md)
