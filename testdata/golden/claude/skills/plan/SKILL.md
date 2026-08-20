---
name: plan
description: Researches a requested change and writes an approval-ready implementation plan before code is changed.
---

# Plan

Use this skill only when the user explicitly asks for a plan before implementation.

Read the project instructions, inspect the smallest relevant file set, identify downstream callers, and find the existing test layer. Ask a question only when an unresolved choice would materially change the result.

Write the plan as an ordered list of independently verifiable steps. Each step names the files involved, the behavior that changes, and the validation that proves it. Include concrete acceptance criteria, risks, and open decisions. Separate required work from possible follow-ups.

Present the plan and stop. Do not edit code until the user approves it. After approval, execute the steps in dependency order and report any material deviation before continuing.
