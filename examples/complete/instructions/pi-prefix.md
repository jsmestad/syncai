# Pi delegation

Use `Agent` from `@tintinweb/pi-subagents` for delegated work and `@tintinweb/pi-tasks` for dependency-ordered task graphs. Give each agent a bounded goal, the relevant context, hard constraints, expected output, and validation requirements.

Use `worker` for narrow implementation or validation. Use `senior-worker` when the task needs cross-file judgment. Keep writers serial unless each writer has an isolated worktree.
