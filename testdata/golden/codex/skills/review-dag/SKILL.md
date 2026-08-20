---
name: review-dag
description: Runs a dependency-ordered review graph for the current diff, followed by a focused fix pass and final verification.
scope: work
---

# Review DAG

Use this workflow in Pi when `@tintinweb/pi-tasks` and `@tintinweb/pi-subagents` are installed.

Create three independent read-only tasks for correctness, test coverage, and failure handling. Use `worker` for narrow checks and `senior-worker` when the diff crosses modules or requires tracing state across files.

Create a fourth task that depends on all three reviews and synthesizes duplicate or conflicting findings. Create a fifth `senior-worker` task that depends on the synthesis and applies only accepted fixes. Create a final read-only task that depends on the fix pass and verifies the resulting diff against the original request.

Execute only the three root review tasks. Let task dependencies start the remaining work. Stop before any task makes an unapproved product, privacy, architecture, or public API decision.

When the final task completes, report the verdict, fixes made, validation evidence, and unresolved risks.
