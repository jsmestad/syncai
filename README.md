# Syncai

Write your AI configuration once. Keep every coding agent in sync.

Coding agents store the same ideas in different files: agent prompts, tool permissions, model choices, reusable skills, shared instructions, extensions, and package manifests. Editing each vendor format by hand duplicates work, loses intent during conversion, and lets installed files drift away from their source.

Syncai gives those ideas one canonical source tree. It validates that source, resolves semantic model roles, and renders deterministic configuration for Pi, Oh My Pi, Claude Code, Codex, OpenCode, and Antigravity CLI. Its status, import, and pull workflows make installed edits visible instead of silently overwriting them.

## Quick proof

Prerequisite: Go 1.25 or 1.26 and a checkout of this repository. This entire example installs and renders beneath one temporary directory. It does not write to your home configuration or Syncai state manifest.

```bash
SYNCAI_ROOT="$(mktemp -d)"
mkdir -p "$SYNCAI_ROOT/bin" "$SYNCAI_ROOT/rendered"
GOCACHE="$SYNCAI_ROOT/go-build" GOMODCACHE="$SYNCAI_ROOT/go-mod" GOBIN="$SYNCAI_ROOT/bin" go install ./cmd/syncai
"$SYNCAI_ROOT/bin/syncai" validate --source examples/complete --profile balanced
"$SYNCAI_ROOT/bin/syncai" render --source examples/complete --out "$SYNCAI_ROOT/rendered" --profile balanced
for target in .pi .omp .claude .codex .config/opencode .gemini/antigravity-cli; do test -d "$SYNCAI_ROOT/rendered/$target" && printf '%s\n' "$target"; done
```

The final command prints all six target roots:

```text
.pi
.omp
.claude
.codex
.config/opencode
.gemini/antigravity-cli
```

Use `--scope home` or `--scope work` to select scoped agents, skills, extensions, package settings, and environment model overrides. Omit `--out` only when you are ready to render into your home directory.

## Install

```bash
go install github.com/jsmestad/syncai/cmd/syncai@latest
```

Syncai defaults to `ai-source/` and renders to `$HOME`. Start with `syncai validate`, inspect an isolated render with `syncai render --out "$(mktemp -d)"`, then read the [command safety reference](docs/commands.md) before rendering into your home directory.

## What one source tree controls

| Canonical resource | Pi | Oh My Pi | Claude Code | Codex | OpenCode | Antigravity CLI |
| --- | --- | --- | --- | --- | --- | --- |
| Agents | Yes | Yes | Yes | Yes | Yes | Yes |
| Skills | Yes | No | Yes | Yes | No | Yes |
| Shared instructions | Yes | No | Yes | Yes | Yes | No |
| TypeScript extensions | Yes | No | No | No | No | No |
| Package and plugin inventory | Yes | No | Yes | Yes | No | Yes |

Agents use semantic roles such as `exploration` and `review` instead of embedding one provider model in every prompt. Toggleable profiles select model maps for Pi, Oh My Pi, and OpenCode. Fixed maps hold vendor-specific models for Claude Code, Codex, and Antigravity. `home` and `work` overlays can replace individual role mappings without duplicating whole profiles.

Syncai also supports reversible workflows:

- `status` reports tracked edits, missing files, stale outputs, untracked agents, untracked skills, and Pi extension drift.
- `import` promotes untracked Pi, Claude, and Codex agents, skills, and single-file Pi extensions into canonical source when the conversion is supported.
- `pull` moves supported edits from rendered files back to their canonical agent or extension, then redistributes them.
- The default home render refuses to overwrite unreconciled manifest-tracked edits unless you pass `--force`.

Reverse conversion is intentionally bounded. Agent bodies and descriptions round-trip broadly. Tool and model fields round-trip only where the native format preserves enough information. OpenCode permission maps, unsupported frontmatter, ambiguous models, and directory extension imports require a manual source edit. See [Commands](docs/commands.md#import-and-pull-limits).

## Syncai and Vercel Labs Skills

[Vercel Labs Skills](https://github.com/vercel-labs/skills) focuses on discovering, installing, using, updating, and removing Agent Skills across 77 agents, including “OpenCode, Claude Code, Codex, Cursor, and 73 more,” using symlink or copy installation and the Agent Skills format. Syncai manages a broader canonical configuration and renders each supported tool's native files.

| Capability | Syncai | Vercel Labs Skills README |
| --- | --- | --- |
| Discover, install, update, and remove Agent Skills | Not a registry workflow | Documented |
| Agent Skills compatible directories | Rendered to four supported skill roots | Documented across 77 agents |
| Canonical vendor-specific agent rendering | Six native agent targets | Not documented |
| Reverse pull of installed edits into canonical source | Supported within documented conversion limits | Not documented |
| Import of installed vendor configuration into canonical source | Supported within documented conversion limits | Not documented |
| Semantic model profiles and environment overlays | Supported | Not documented |
| Manifest-backed installed drift guards | Supported for default home renders | Not documented |

“Not documented” describes the current upstream README, not a claim that the project can never support the capability.

## Pi integration

Syncai generates agent definitions compatible with [`@tintinweb/pi-subagents`](https://github.com/tintinweb/pi-subagents). Install that Pi extension separately:

```bash
pi install npm:@tintinweb/pi-subagents
```

The extension owns subagent execution and its runtime behavior. Syncai does not author, bundle, or replace it. Syncai generates compatible model, thinking, skill, prompt-mode, and tool configuration, but it does not currently render the upstream `allowed_subagents` field required to configure nested subagents. See [Pi integration](docs/pi.md).

## Reference

- [Canonical source format](docs/source-format.md)
- [Commands, output, exit behavior, and safety](docs/commands.md)
- [Pi integration](docs/pi.md)
- [Contributing](CONTRIBUTING.md)
- [Security policy](SECURITY.md)

Syncai is licensed under the [Apache License 2.0](LICENSE).
