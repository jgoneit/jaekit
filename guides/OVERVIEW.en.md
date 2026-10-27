# Understanding Jaekit

[한국어](OVERVIEW.md) · **English**

This page explains, from start to finish, what problem Jaekit tries to solve and why it is shaped the way it is. Installation and commands are in the [README](../README.md#get-started) and the [usage guide](USAGE.en.md).

## When your agent says "done"

When you hand work to Claude Code or Codex, this often happens:

- It does seven of ten conditions and stops.
- It reads a hard condition as an easier one.
- It stops early, and you keep saying "finish the rest".
- It says "the tests pass", but to learn what it actually checked you have to read the changed code from the top.

When the conversation ends, you have to explain again what was promised and how far it was checked.

There are two common fixes. One is to bind the agent to a procedure: plan formats, approval at every step, hooks that block actions. The better the model gets, the more the procedure becomes its ceiling, and it is hard to prove that a tool blocks everything an agent can do. The other is to trust the agent's report. But an agent can edit its own checks and write easier criteria. "The tests pass" is not the same as "the goal is met".

## Jaekit's idea: delegate the work, judge the end by records

Jaekit picks the space between. It holds on to three things and leaves the rest to the agent.

1. **You set the goal.** What finished means, the completion conditions, is written down before the work starts. While implementing, the agent does not edit that document.
2. **The agent does the work.** Planning, order and tools are the agent's choice. You do not approve each step; it asks only when a decision would change the outcome.
3. **Completion is judged from records.** A goal is recorded as complete only when each completion condition has a record of its check. For new behavior, the check must fail before the change and pass after it, because a check that passes from the start checks nothing.

## Who does what

| Who | What they do |
| --- | --- |
| You | Say what you want, review the goal document, answer decisions that change the outcome, read the completion report |
| Your agent (Claude Code, Codex) | The only one that judges, plans and implements. Spec and Seal are instructions the agent reads |
| Spec | Turns a request into a goal document: goal, scope, completion conditions and a review tier. It writes and stops; it does not implement |
| Seal | Has the agent plan, implement, check and fix toward the reviewed goal until it is done. It only reads the goal document |
| `ha` | A terminal program. It runs checks, records every run, and computes status and completion from those records alone. It never calls a model |
| Spar | Sends the same material to reviewers outside the working agent's conversation (by default the claude, codex and agy CLIs) for an independent review, and records their findings and how each was handled |

The agent runs `ha` and Spar. What you type in a terminal is only the install and update commands; the work is delegated through the agent conversation.

## How one goal goes through

Follow a **fictional example** of adding a "Forgot password" link to a login screen. It is not an executed or verified case.

1. **You ask.** In the agent conversation, send `/spec:spec add a "Forgot password" link to the login screen` (`$spec:spec` in Codex).
2. **Spec writes the goal.** It writes the goal and completion conditions to `docs/specs/password-link/SPEC.md`, for example `AC-1` "the login screen shows a link to the reset page" and `AC-2` "the existing login still works". It also proposes a review tier, asks once whether to implement it now, and stops.
3. **You review it and answer yes.** If a condition is missing, ask for a fix; if it is right, answer yes. Seal starts then.
4. **Seal plans.** It writes in `PLAN.md` how each condition is checked. `AC-1` is new behavior; `AC-2` is behavior that must keep working.
5. **`ha` checks and records.** The agent first runs the `AC-1` check on the code before the change and records its failure. Then it implements and runs it again to record a pass. `AC-2` gets a passing record. Every run is appended to `runs.jsonl`, one line each.
6. **Spar reviews independently.** This goal's tier is `standard`, so reviewers in another context review the change before completion. The agent fixes the findings that need it, and the finished review is recorded as a gate.
7. **`ha done` records completion.** When every required condition is met by records, a completion record is written and the report is added to `PROGRESS.md`.
8. **You read the report.** Read its five lines, what changed, what was checked, what to look at, remaining limits and the next action, with the evidence per condition, and look at the actual result too.

If the conversation ends midway, open a new conversation in the same project and send `continue docs/specs/password-link`. The agent there reads the goal folder and the status `ha` computes, and continues the remaining work.

## What you get

Each goal gets one folder in your project.

| File | Written by | Holds |
| --- | --- | --- |
| `SPEC.md` | Spec | Goal, scope, completion conditions. It is your document |
| `PLAN.md` | Seal | How each condition is checked, the paths it may change, the run budget |
| `REVIEW.md` | Seal | The review plan and a summary of the Spar records |
| `PROGRESS.md` | Seal | Progress and the completion report |
| `runs.jsonl` | `ha` | The append-only run record |

The first five lines of the completion report are computed by `ha` from the run record, Git and the goal documents. The agent does not rewrite them. How to read them is in the [usage guide](USAGE.en.md#reading-the-completion-report).

## Review tiers

The goal document's `Assurance:` line sets how much independent review the goal gets. Spec proposes one from the size and risk of the change; to pick another, say so, for example "make it strict".

| Tier | Used for | Review it gets |
| --- | --- | --- |
| `light` | A small fix, such as a typo or one message | May skip independent review with a reason |
| `standard` | Ordinary changes | Spar review rounds before completion |
| `strict` | Goals that change a public contract, stored data, permissions, or a boundary between components | Also a plan review before work |

Before you use Spar the first time, a project needs one setup step: see [Before your first `standard` goal](../README.md#before-your-first-standard-goal) in the README. What is sent to reviewers is in the [Spar guide](SPAR.en.md#what-leaves-your-computer).

## What completion means, and what it does not

A completion record means:

- Each required completion condition has a check record or a necessary user confirmation.
- The check for new behavior has records of failing before the change and passing after it.
- For `standard` and `strict` goals, a recorded review gate covers the last commit.

It does not mean:

- **That the whole result is free of defects.** Whether the checks cover the goal well enough is not something a program can decide. Independent review and your confirmation fill that gap.
- **Independent third-party verification.** The assurance level is `local`. The agent writes the checks, and an agent with the same user permissions can change the checks and the record files. The links between record lines reveal accidental edits; they do not prevent forgery.
- **Deployment or installation.** The default result is local changes and commits. Push, pull requests and deployment are yours to do, or a separate request to an ordinary agent.

Jaekit is not a tool that cages the agent. It shows what was actually checked and what was not.

## What to read next

- To install and hand over your first goal: [Get started](../README.md#get-started) in the README
- To change a goal, continue one, or read a report: [Usage guide](USAGE.en.md)
- To set up independent review: [Spar guide](SPAR.en.md)
- To read why work stopped and what a report rests on: [Reading work status and the completion report](OPERATIONS.en.md)
- For the exact formats of files and records: [contracts](../contracts/README.md)
