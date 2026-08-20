# Pi integration

SyncAI generates Pi configuration and agent definitions compatible with [`@tintinweb/pi-subagents`](https://github.com/tintinweb/pi-subagents). The extension owns subagent discovery, execution, and nested-agent behavior. SyncAI does not author, bundle, install, or replace it.

Install the extension separately:

```bash
pi install npm:@tintinweb/pi-subagents
```

The upstream extension also supports custom agents in `.pi/agents` or `.agents/agents` and documents its own model, thinking, tools, skills, prompt mode, and nested-agent controls. Current upstream nested delegation requires `allowed_subagents`. SyncAI does not render that field, so its responsibility ends at the compatible configuration explicitly listed below.

## Generated paths

Global rendering writes:

| Source | Generated path |
| --- | --- |
| `agents/<name>.md` | `~/.pi/agent/agents/<name>.md` |
| `skills/<name>/` | `~/.pi/agent/skills/<name>/` |
| `extensions/<name>.ts` or `<name>/` | `~/.pi/agent/extensions/<name>.ts` or `<name>/` |
| `instructions/global.md` and optional `pi-prefix.md` | `~/.pi/agent/AGENTS.md` |
| Resolved model catalog | `~/.pi/agent/model-profiles.json` |

`syncai render --project <path>` reads `<path>/.pi/agent-source` and writes:

| Resource | Project path |
| --- | --- |
| Agents | `<path>/.pi/agents/<name>.md` |
| Skills | `<path>/.pi/skills/<name>/` |
| Extensions | `<path>/.pi/extensions/<name>` |
| Instructions | `<path>/AGENTS.md` |
| Resolved model catalog | `<path>/.pi/model-profiles.generated.json` |

## Model and thinking resolution

An agent's `modelRole` resolves through the active profile's `pi` map. For example, `exploration` can resolve to `example-lab/orbit-small:medium`. SyncAI emits:

```yaml
model: example-lab/orbit-small
thinking: medium
```

Recognized thinking suffixes are `off`, `minimal`, `low`, `medium`, `high`, and `xhigh`. An unrecognized final suffix remains part of the model identifier. Pi resolution currently uses `modelRole` directly; `fallbackRoles` is retained in canonical source but is not used by the Pi renderer.

The profile selection order is explicit `--profile`, `AI_MODEL_PROFILE`, `PI_MODEL_PROFILE`, `~/.pi/agent/active-model-profile.json`, then the catalog's `activeProfile`. `--scope home` and `--scope work` merge the corresponding environment override before the selected role is resolved.

## Agent field mapping

| Canonical field | Pi field or behavior |
| --- | --- |
| `description` | Quoted `description` |
| `display_name` | Quoted `display_name` |
| `tools` | `tools` |
| `modelRole` | Resolved `model` and optional `thinking` |
| `systemPromptMode` | `prompt_mode` |
| `inheritSkills` | `skills` |
| `extensions`, `exclude_extensions`, `memory`, `isolation`, `isolated`, `max_turns`, `persist_session`, `session_dir`, `inherit_context`, `run_in_background`, `enabled` | Passed through |

`targets`, `scope`, and SyncAI-only model fields are not emitted into Pi's file. The Markdown body remains the agent prompt.

## Delegation denylist and nested subagents

SyncAI uses `maxSubagentDepth` only to decide whether to emit a delegation-tool denylist:

- A missing, nonnumeric, `0`, or `1` value emits `disallowed_tools: Agent, get_subagent_result, steer_subagent`.
- A value greater than `1` omits that generated denylist.

Omitting the denylist does not configure nested delegation. Current `@tintinweb/pi-subagents` requires `allowed_subagents`, and SyncAI does not currently render that upstream field. Users who need nesting must add `allowed_subagents` through a configuration path supported by the extension or manage the generated agent manually, accounting for SyncAI's drift guard and future renders. Runtime depth, prompt mode, tool availability, and result handling remain owned by `@tintinweb/pi-subagents` and Pi.

## Skills, prompts, and extensions

`inheritSkills` becomes the Pi agent's `skills` value. `systemPromptMode` becomes `prompt_mode`. Skill directories and TypeScript extensions are copied verbatim, except `extension.toml` sidecars are excluded because they are SyncAI build metadata.

Global instructions are generated with a SyncAI header. `instructions/global.md` appears first; a nonempty `instructions/pi-prefix.md` follows a Markdown divider.

## Package boundary

SyncAI can place package sources and `npmCommand` in Pi's `~/.pi/agent/settings.json`, and it reconciles managed npm or Git artifacts declared in `packages.json`. Installing `@tintinweb/pi-subagents` through Pi remains an explicit user action unless you declare that package in your own canonical package manifest.
