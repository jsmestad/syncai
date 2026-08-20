# Canonical source format

SyncAI reads one source directory, `ai-source/` by default, and projects its contents into tool-specific configuration. Start from [`examples/complete`](../examples/complete) when creating a source tree.

## Directory contract

```text
ai-source/
  agents/
    worker.md
    senior-worker.md
  skills/
    plan/
      SKILL.md
  instructions/
    global.md
    pi-prefix.md
    omp-prefix.md
    claude-prefix.md
    codex-prefix.md
    opencode-prefix.md
    antigravity-prefix.md
  extensions/
    zelda-hearts.ts
    zelda-hearts.toml
    session-name/
      extension.toml
      index.ts
  model-profiles.json
  model-overrides/
    home.json
    work.json
  packages.json
```

`agents/` and `model-profiles.json` must exist for `validate` and `render`. `instructions/global.md` is also required by `render`, but the current `validate` command does not read instructions. `skills/`, `extensions/`, `model-overrides/`, target instruction prefixes, and `packages.json` are optional. A missing `packages.json` means an empty package manifest.

## Project-local source root

`syncai render --project <project>` ignores `--source` and treats `<project>/.pi/agent-source` as the canonical source root. That directory uses the same agents, skills, instructions, extensions, model profiles, environment overrides, and package manifest shapes documented here.

Project mode changes output placement and target participation. Pi writes agents, skills, extensions, and a resolved model catalog under `<project>/.pi`, plus `<project>/AGENTS.md`. Oh My Pi writes agents under `<project>/.omp/agents`. Claude Code, Codex, and OpenCode do not render in project mode. Antigravity currently writes its ordinary `.gemini/antigravity-cli/plugins/dfiles` tree beneath the project.

Project rendering does not use the global manifest, prune prior manifest entries, enforce the manifest drift guard, or apply packages. Only `render` accepts `--project`; other commands use their explicit or default source directory.

## Agents

Each regular `agents/*.md` file uses line-oriented `key: value` frontmatter followed by a Markdown prompt. The parser is not a YAML parser: values remain strings, CSV fields are split only where documented, unrecognized lines without a colon are ignored, and field order is retained. Files ending in `.chain.md` are ignored.

```markdown
---
name: explorer
description: Finds relevant code and reports concise evidence.
targets: pi, omp, claude, codex, opencode, antigravity
modelRole: exploration
fallbackRoles: review
tools: read, bash, grep, find
---
Inspect the requested area, identify the smallest relevant file set, and report findings with file paths.
```

### Core fields

| Field | Required | Accepted value and effect |
| --- | --- | --- |
| `name` | Yes | One safe filename component. Empty names, `.`, `..`, absolute paths, slashes, and backslashes are rejected. The rendered filename uses this value. |
| `description` | Yes | A nonempty one-line description. |
| `targets` | No | CSV subset of `pi`, `omp`, `claude`, `codex`, `opencode`, and `antigravity`. The default is `pi`. Unknown targets are rejected. |
| `scope` | No | CSV subset of `home` and `work`. Omission means universal. Unknown scopes are rejected. |
| `modelRole` | No | Semantic role resolved through `model-profiles.json`. A role must resolve for Pi, Oh My Pi, and Antigravity agents that use it. Claude Code, Codex, and OpenCode omit a model when their target map is absent. |
| `fallbackRoles` | No | CSV fallback roles used only by the Oh My Pi renderer. See [Fallback roles](#fallback-roles). |
| `tools` | No | CSV source vocabulary. Renderers translate it to each target's tool, permission, or sandbox representation. |

The Markdown body after the closing delimiter is the agent's instruction text. A Codex-targeted body cannot contain `'''` because Codex output uses a TOML multiline literal string.

### Pi and Oh My Pi control fields

| Source field | Current output behavior |
| --- | --- |
| `display_name` | Emitted as a quoted Pi field. Claude and Antigravity also retain it as vendor frontmatter. |
| `systemPromptMode` | Emitted by Pi as `prompt_mode`. |
| `inheritSkills` | Emitted by Pi as `skills` and by Oh My Pi as `autoloadSkills`. |
| `maxSubagentDepth` | A number greater than `1` omits Pi's generated delegation-tool denylist and adds `spawns: *` in Oh My Pi unless `ompSpawns` is set. Missing, invalid, `0`, or `1` adds Pi's `disallowed_tools: Agent, get_subagent_result, steer_subagent` and does not enable Oh My Pi spawning. Omitting the Pi denylist does not configure upstream `allowed_subagents`. |
| `ompSpawns` | Emitted by Oh My Pi as `spawns` and takes precedence over the derived `maxSubagentDepth` behavior. |
| `extensions` | Passed through to Pi. |
| `exclude_extensions` | Passed through to Pi. |
| `memory` | Passed through to Pi. |
| `isolation` | Passed through to Pi. |
| `isolated` | Passed through to Pi. |
| `max_turns` | Passed through to Pi. |
| `persist_session` | Passed through to Pi. |
| `session_dir` | Passed through to Pi. |
| `inherit_context` | Passed through to Pi. |
| `run_in_background` | Passed through to Pi. |
| `enabled` | Passed through to Pi. |

The loader also retains the compatibility fields `model`, `fallbackModels`, `inheritProjectContext`, `output`, `defaultReads`, and `defaultProgress`, but current renderers do not emit them. Arbitrary `key: value` fields are retained by the parser, may pass through to Claude or Antigravity, and are ignored by other renderers. Treat fields outside the tables above as vendor-specific and nonportable.

### Fallback roles

`fallbackRoles` is an ordered CSV list used only by Oh My Pi. The renderer first looks for `modelRole` in the active Oh My Pi target map, then tries each nonempty fallback role from left to right. The first mapped role wins. Rendering fails when the active catalog has no Oh My Pi target map or when neither the primary role nor any fallback role is mapped.

Pi, Claude Code, Codex, OpenCode, and Antigravity currently ignore `fallbackRoles`. In particular, Pi resolves only `modelRole`; a Pi agent with an unmapped primary role fails even when a mapped fallback is listed.

## Skills

Each immediate subdirectory of `skills/` is one skill and is copied verbatim. Pi, Claude Code, Codex, and Antigravity receive skill directories. Oh My Pi and OpenCode do not.

`SKILL.md` may include `scope: home`, `scope: work`, or CSV `scope: home, work` in leading `---` frontmatter. Missing frontmatter, a missing `SKILL.md`, or a missing `scope` field makes the directory universal. Unknown scopes are rejected. Other skill fields are not interpreted by SyncAI.

## Instructions

`instructions/global.md` is required and copied into Pi's `AGENTS.md`, Claude Code's `CLAUDE.md`, Codex's `AGENTS.md`, and OpenCode's `AGENTS.md`. Oh My Pi and Antigravity do not currently render shared instructions.

Optional prefix files are named `<target>-prefix.md`, where target is `pi`, `omp`, `claude`, `codex`, `opencode`, or `antigravity`. Pi and Claude Code currently consume their loaded prefixes. The other renderers load but do not emit them. Files named only `<target>.md` are not target prefixes.

Pi places the global text first and then its prefix after a divider. Claude Code places its prefix first and then the global text after a divider.

## Extensions

Extensions are Pi-only runtime code copied verbatim to `.pi/agent/extensions/`.

- A top-level `extensions/<name>.ts` is a single-file extension. Other top-level file suffixes are ignored.
- `extensions/<name>.toml` is its optional metadata sidecar.
- Any immediate subdirectory is a directory extension. Its optional sidecar is `extensions/<name>/extension.toml`.
- Sidecars are build metadata and are not copied into the installed extension.

The only sidecar field SyncAI interprets is `scope`. It may be one string, a comma-separated string, or an array of strings. Unknown TOML keys are ignored:

```toml
scope = ["home", "work"]
```

Accepted scopes are `home` and `work`. A missing sidecar or field means universal. Directory copies accept regular files only and reject nonregular source entries such as symlinks.

## Model profiles

Model roles separate an agent's purpose from each tool's concrete provider identifier. [`examples/complete/model-profiles.json`](../examples/complete/model-profiles.json) contains a complete working catalog with `openai`, `claude`, and `mixed` profiles; semantic roles for fast code work, deeper implementation, reasoning, tests, design, product, security, data, and privacy; and fixed mappings for Claude Code, Codex, and Antigravity.

`activeProfile` is required. When `profiles` is nonempty, the active name must exist. `profiles.<name>.<target>.<role>` holds switchable mappings, normally for `pi`, `omp`, and `opencode`. `fixed.<target>.<role>` holds mappings that ignore the active profile, normally for `claude`, `codex`, and `antigravity`.

### Fixed targets

A target under `fixed` always wins over a map for the same target under the active profile. Fixed mappings therefore remain stable when `activeProfile` changes. Environment overlays may replace individual fixed role mappings before model resolution.

When no fixed or active-profile map exists for a target, behavior depends on the renderer. Claude Code, Codex, and OpenCode omit the model field and let the tool use its default. Pi, Oh My Pi, and Antigravity return a render error when an agent with `modelRole` targets the missing map. A present target map still fails when none of the roles considered by that renderer are mapped.

Pi recognizes a final `:off`, `:minimal`, `:low`, `:medium`, `:high`, or `:xhigh` suffix and emits separate `model` and `thinking` fields. Oh My Pi also recognizes `:max` and `:auto`, emitting `thinkingLevel`. Codex splits its fixed model at `:` into `model` and `model_reasoning_effort`. OpenCode, Claude Code, and Antigravity use the resolved string as documented by their renderer.

### Profile precedence

The selected profile uses this precedence, highest first:

1. An explicit `--profile` value.
2. `AI_MODEL_PROFILE`.
3. `PI_MODEL_PROFILE`.
4. `activeProfile` in `~/.pi/agent/active-model-profile.json`.
5. `activeProfile` in `model-profiles.json`.

`set-profile` writes the persisted file. `use-profile` renders with the requested profile first and persists it only after a successful render.

### Environment overrides

`--scope home` deep-merges `model-overrides/home.json`; `--scope work` deep-merges `model-overrides/work.json`. The environment axis is independent of the selected profile. An overlay replaces only the role mappings it declares and may add profile, target, or fixed mappings.

The complete example's home overlay lowers the fast-code thinking level for Pi and Oh My Pi:

```json
{
  "profiles": {
    "openai": {
      "pi": {
        "code-fast": "openai-codex/gpt-5.6-luna:medium"
      },
      "omp": {
        "code-fast": "openai-codex/gpt-5.6-luna:medium"
      }
    }
  },
  "fixed": {}
}
```

The work overlay gives the mixed profile's senior role more reasoning effort:

```json
{
  "profiles": {
    "mixed": {
      "pi": {
        "code-high": "anthropic/claude-opus-5:xhigh"
      },
      "omp": {
        "code-high": "anthropic/claude-opus-4-8:xhigh"
      }
    }
  },
  "fixed": {}
}
```

`validate` loads the base catalog and selected profile but does not merge a scope overlay or render agents. A successful validation therefore proves parsing, required agent fields, known targets and scopes, skill and extension scopes, and package JSON syntax. Run an isolated `render --out "$(mktemp -d)"` to prove that every model role resolves for every selected target.

## Package manifest

`packages.json` accepts this shape:

```json
{
  "pi": {
    "packages": [],
    "packagesByScope": {},
    "npmCommand": [],
    "npmCommandByScope": {}
  },
  "claude": {
    "marketplaces": [],
    "plugins": []
  },
  "codex": {
    "plugins": []
  },
  "antigravity": {
    "plugins": []
  }
}
```

| Field | Meaning |
| --- | --- |
| `pi.packages` | Universal Pi package sources. |
| `pi.packagesByScope` | Map from scope name to additional Pi package sources. The current CLI accepts `home` and `work` scopes. |
| `pi.npmCommand` | Universal command argv used for Pi npm package reconciliation. An empty list lets Pi or SyncAI use its default. |
| `pi.npmCommandByScope` | Map from scope name to replacement command argv. |
| `claude.marketplaces` | Claude Code marketplace sources. |
| `claude.plugins` | Claude Code plugin identifiers. |
| `codex.plugins` | Codex plugin identifiers. |
| `antigravity.plugins` | Antigravity plugin identifiers tracked for status and pull. Current `packages apply` does not install them. |

String lists are trimmed, deduplicated, and sorted. Scope-specific Pi packages are unioned with universal packages. A scope-specific npm command replaces the universal command.

## Validation errors

SyncAI returns a nonzero exit for malformed JSON or TOML, an unreadable required path, missing agent frontmatter delimiters, missing agent `name` or `description`, unsafe agent names, unknown targets, unknown `home` or `work` scopes, a missing active profile, or a requested profile absent from the catalog. Render also fails when a used model role cannot resolve, when a Codex body contains `'''`, when a source directory contains an unsupported file mode, or when an output path escapes its allowed root.
