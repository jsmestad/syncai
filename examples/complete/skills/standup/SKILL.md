---
name: standup
description: Runs a sustained project goal as a sequence of isolated, checkpointed work sessions using worker and senior-worker agents.
scope: home
---

# Standup

Use this skill when the user wants to carry a multi-step project goal through several isolated work sessions.

## Start or resume

Look for `.standup/day-*.yml` in the project root. Resume from the highest numbered note. If none exists, create `day-0.yml` from the user's goal with prioritized work in `next`, known risks in `landmines`, relevant files in `geography`, unresolved items in `blocked`, and a progress estimate.

## Run one work session

Read the latest note. If `blocked` contains anything, report the blocker and wait for the user. If progress is complete and `next` is empty, report the finished goal.

Choose `worker` for a narrow implementation or validation task. Choose `senior-worker` when the next item requires cross-file changes, debugging, refactoring, or design judgment. Give the agent the complete current note, one bounded task from `next`, the project root, and the path for the next note.

The agent must implement and validate its task, then write `.standup/day-{N+1}.yml` with this shape:

```yaml
day: 1
date: 2026-01-15
project: example-project
situation: Current project state in plain language.
done:
  - Completed work with validation evidence.
next:
  - Highest priority remaining task.
landmines:
  - Known risk or fragile area.
geography:
  - path/to/relevant-file
decisions:
  - Decision and reason.
blocked: []
progress:
  completed: 25
  confidence: medium
  estimate_remaining: Three focused sessions.
```

After the agent returns, verify that the next note exists, advances the day number, records validation, and contains a concrete next task or marks the goal complete. Report what finished, current progress, and the next item. Then continue unless the user stops the loop or a blocker requires their decision.

Keep agents isolated from earlier conversation history. The standup note is the handoff contract, so it must contain every decision, risk, file location, and unfinished item needed by the next session.
