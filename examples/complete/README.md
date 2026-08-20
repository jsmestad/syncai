# Complete source tree

This example is a usable SyncAI source tree, not a schema fixture. It includes two implementation agents, three orchestration skills, two Pi extensions, three model profiles, scope-specific model overrides, and public package declarations.

Render it without touching your home configuration:

```bash
OUT="$(mktemp -d)"
syncai validate --source examples/complete
syncai render --source examples/complete --out "$OUT" --profile openai
```

Switch every semantic role from OpenAI to Anthropic without editing an agent:

```bash
syncai render --source examples/complete --out "$OUT" --profile claude
```

The agents refer to roles such as `code-fast` and `code-high`. `model-profiles.json` resolves those roles for Pi, Oh My Pi, OpenCode, Claude Code, Codex, and Antigravity. The `mixed` profile demonstrates assigning different providers by role.

`--scope home` adds the goal-tracking Pi package, uses the home model override, and includes the Zelda hearts extension. `--scope work` switches Pi's npm command to `pnpm` and uses the work model override. Universal resources render in both scopes.

The package manifest is declarative. `render --out` writes configuration into the isolated output directory; it does not install plugins into your real home directory. Review [the package command behavior](../../docs/commands.md) before applying a manifest to your home configuration.
