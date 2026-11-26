# Understanding Jaekit

[한국어](OVERVIEW.md) · **English**

Jaekit lets you check from records whether work you handed to Claude Code or Codex is really done. You write down first what finished means. Whether it is finished is then judged from records of checks that actually ran, not from the agent's word.

This page is for people new to Jaekit. When you have read it, install with the [install guide](INSTALL.en.md) and follow [your first task](../README.md#your-first-task) in the README.

## When your agent says "done"

When you hand work to Claude Code or Codex, this can happen:

- It does seven of ten conditions and stops.
- It reads a hard condition as an easier one.
- It stops early, and you keep saying "finish the rest".
- It says "the tests pass", but to learn what it checked you have to read all the code.
- When the conversation ends, you explain from the start what was promised.

Approving every step keeps this in check, but then there is little point in handing the work over. Trusting the report is easy, but an agent can edit its own checks or write easier criteria. "The tests pass" is not "the goal is met".

## Jaekit's idea: delegate the work, judge the end by records

Jaekit holds on to three things. It leaves the rest to the agent.

1. **You set the goal.** What finished means, the completion conditions, is written down before the work starts. While working, the agent does not edit that document.
2. **The agent does the work.** Planning, order and tools are the agent's choice. You do not approve each step; it asks only about decisions that change the outcome.
3. **Completion is judged from records.** Each required completion condition needs a record of its check before completion is recorded. A condition that cannot be checked automatically is met by a record of your confirmation.

> **Conditions that need more.** A condition that fixes a bug already in the code, and a prohibition such as "X does not happen", also need a record of their check failing on the code before the change. A check that passes from the start cannot tell whether anything was fixed. A condition about document content is met by the independent review of a `standard` or `strict` goal. The exact scope is in [What completion means](#what-completion-means-and-what-it-does-not).

## What changes when you use it

Here is the same work handed over in a plain conversation and through Jaekit. Either way, the same agent does the work. What changes is the record.

| When | Plain conversation | With Jaekit |
| --- | --- | --- |
| Starting the work | What finished means is scattered across the chat | The completion conditions are written down first, and work starts after you confirm them |
| A check runs | Only the words "it passed" are left | One line records the commit, the command and the result |
| The code changes again | You can't tell whether the earlier pass still holds | The earlier record stays but no longer counts. The check runs again on the current code |
| A check that passes from the start | A check that can't tell whether the new behavior exists still looks like a pass | A bug fix or a prohibition counts only with a record of failing before the change |
| Existing tests are edited | A loosened test still shows "pass" | A test change you didn't allow, or one that weakens a test, blocks completion until you confirm it |
| The goal or scope changes | It can slip by unnoticed | A changed goal document, or a changed file outside the paths agreed to change, blocks completion until you confirm it |
| The conversation ends | You explain everything again | Ask a new conversation to continue; the agent reads the goal folder and its record and picks up from there |
| "Done" | Whether it is finished is settled by words | `ha` works out completion from the record, the documents and Git, and records it |

## Who does what

Your agent is the only one doing the work. Spec and Seal are instructions the agent follows; `ha` and Spar are tools the agent runs. So "Spec writes" means the agent writes by following Spec's instructions.

| Who | What they do |
| --- | --- |
| You | Say what you want, review the goal document, answer decisions, read the completion report |
| Your agent (Claude Code, Codex) | Judges, plans and implements. It is the only one doing the work |
| Spec | Instructions for turning a request into a goal document. It writes the goal, scope, completion conditions and a review tier, then stops |
| Seal | Instructions that carry the confirmed goal to the end. The agent keeps planning, implementing, checking and fixing; it only reads the goal document |
| `ha` | A terminal program that records every check the agent runs and computes completion from those records, the documents and Git. It never calls an AI model |
| Spar | Sends the same material to reviewers outside the working conversation (by default the claude, codex and agy CLIs) for an independent review, and records their findings and how each was handled |

Spec, Seal and Spar are plugins you install in Claude Code or Codex; `ha` and `spar` are terminal programs.

## How one goal goes through

Follow a **fictional example** of adding a "Forgot password" link to a login screen. It is not an executed case.

1. **You ask.** In the agent conversation, send `/spec:spec add a "Forgot password" link to the login screen`. In Codex it is `$spec:spec`.
2. **Spec writes the goal.** It writes the goal and numbered completion conditions to `SPEC.md`. For example, AC-1 is "the login screen shows a reset link" and AC-2 is "the existing login still works". It also proposes a review tier. If nothing is left to decide, it asks once whether to implement now, and stops.
3. **You review it and answer yes.** If a condition is missing, ask for a fix. If it is right, answer yes. From here Seal does the work.
4. **Seal plans.** It writes in `PLAN.md` how each condition is checked. AC-1 is new behavior; AC-2 is behavior that must keep working.
5. **`ha` records the checks.** Each time the agent runs a check, `ha` appends the result to `runs.jsonl`, one line each. In this example, AC-1 runs first on the code before the change and records a failure, then runs again after the change and records a pass. AC-2 records a pass.

   > Under the current Seal (`run-rules/8`), a new feature is met by its passing record alone. A failure record before the change is needed only for bug fixes and prohibitions. The example keeps one for AC-1 to show the record's shape.

6. **Spar reviews independently.** This goal's [review tier](#review-tiers) is `standard`. So before completion, reviewers outside the working conversation review the change. The agent fixes the findings that need it, and records the result of the finished review (the review gate).
7. **`ha done` records completion.** When every required condition is met and the other requirements, such as the review record, are in place, a completion record is written. The report goes into `PROGRESS.md`.
8. **You read the report.** The review summary in `PROGRESS.md` gathers what changed, what it rests on and what you should look at yourself. Below it is the completion report `ha` computed. Look at the actual result too.

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

**Review summary.** For a finished goal, start with `## 검수 요약` in `PROGRESS.md`. Its top lines give the completion record, the assurance tier, how many required conditions are satisfied and the review result. Below them come what changed, evidence, what to inspect and what was not observed, in that order. Goals started before Seal 0.1.24, and how the summary is written, are covered in the [usage guide](USAGE.en.md#reading-the-completion-report).

The first five lines of the completion report are computed by `ha` from the run record, Git and the goal documents. The agent does not rewrite them. Read the words in the report like this:

- `base`: the commit the work started from.
- `seq`: a record's number. Completion record `seq 6` is the sixth record, the one that holds the completion.
- Assurance `local`: the evidence comes from this computer.
- Check author: executor: the agent that implemented the change also wrote the checks.

## Review tiers

The goal document's `Assurance:` line sets how much independent review the goal gets. Spec proposes one from the size and risk of the change. To pick another, say so, for example "make it strict". If your project's instructions ask for more review, that wins.

| Tier | Used for | Review it gets |
| --- | --- | --- |
| `light` | A small fix, such as a typo or one message | May skip independent review with a reason, unless project instructions require it |
| `standard` | Ordinary changes | Spar review rounds before completion |
| `strict` | Goals that change a public contract, stored data, permissions, or a boundary between components | Also a plan review before work |

Newly started `standard` and `strict` goals complete only with a Spar review record. Goals started with an earlier version keep the rules they started with. Before you use Spar the first time, a project needs one setup step: see [Before your first `standard` goal](../README.md#before-your-first-standard-goal) in the README. What is sent to reviewers is in the [Spar guide](SPAR.en.md#what-leaves-your-computer).

## What completion means, and what it does not

A completion record means:

- Each required completion condition has a check record or a necessary user confirmation. A `review` condition has the review gate.
- For a newly started goal, each required change condition marked `요구` has records of its check failing on the code before the change and passing after it. Seal marks `요구` on conditions that fix a bug already in the code and on prohibitions. An unmarked change condition can be met by a passing record alone.
- For newly started `standard` and `strict` goals, there is a Spar record (the review gate) that reviewed the code and goal documents as they were when completion was recorded.

It does not mean:

- **That the whole result is free of defects.** Whether the checks cover the goal well enough is not something a program can decide. Independent review and your confirmation fill that gap.
- **Independent third-party verification.** The assurance level is `local`. The agent writes the checks, and an agent with the same user permissions can change the checks and the record files. Each record line holds the hash of the line before it, so an edited or deleted earlier record shows. It does not catch a change to the last line, and it does not stop deliberate forgery.
- **Deployment or installation.** The default result is local changes and commits. Push, pull requests and deployment are yours to do, or a separate request to an ordinary agent.

Jaekit is not a security tool that polices the agent. It shows what was actually checked and what was not.

## Common questions

**What do I type in the terminal, and what in the agent conversation?**
In the terminal, mostly the install and update commands. You also use it to look at status or records yourself and to set up Spar reviewers. In the agent conversation: `/spec:spec <request>` (`$spec:spec` in Codex), "yes" once you have reviewed the goal, your answers to questions, and `continue docs/specs/<goal>` when a conversation has ended. The agent runs the `ha` and `spar` commands.

**What appears in my project?**
A `docs/specs/<goal>/` folder for each goal. Whether Git tracks it follows your project's policy. The records can contain what you said and paths from your work, so check them before you push to a public repository. Before the first `standard` or `strict` goal that uses Spar, add `docs/reviews/` to `.gitignore`.

**How are existing tests used?**
Your whole existing test suite can serve as the check that existing behavior keeps working. Which check covers which condition is written by the agent in `PLAN.md`. The exact format is in the [execution bundle contract](../contracts/bundle.md).

**Does it push, open a pull request or deploy at the end?**
No. The default result is local changes and commits. Push and pull requests are yours to do, or a separate request to an ordinary agent.

**Does my code leave my machine?**
`ha` never calls an AI model. Spar reviewers send the criteria, the rules and the change (the diff) to their own providers, and project files a reviewer reads can go too. With the default reviewers, claude goes to Anthropic, codex to OpenAI and agy to Google. If code must not leave your machine, don't run Spar. Details are in the [Spar guide](SPAR.en.md#what-leaves-your-computer).

## What to read next

- To install and hand over your first goal: [Get started](../README.md#get-started) in the README
- To change a goal, continue one, or read a report: [Usage guide](USAGE.en.md)
- To set up independent review: [Spar guide](SPAR.en.md)
- To read why work stopped and what a report rests on: [Reading work status and the completion report](OPERATIONS.en.md)
- For the exact formats of files and records: [Formats and rules at a glance](FORMATS.en.md)
