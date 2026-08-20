# Command reference

SyncAI writes progress and reports to stdout. The process prints returned errors to stderr and exits with status `1`; successful commands exit `0`. Warnings, drift refusal details, and best-effort prune failures also use stderr where noted.

Source resolution uses the first available value: explicit `--source`, `SYNCAI_SOURCE`, the source saved by `syncai init`, then `./ai-source`. The saved config lives at `${XDG_CONFIG_HOME:-$HOME/.config}/syncai/config.json`. All scope flags accept only `home`, `work`, or an empty value. Effectful render, status, import, pull, use-profile, and package orchestration checks cancellation between supported stages and passes context to external package commands. `validate` and `list` do not currently observe cancellation. `set-profile` checks once before entering synchronous persistence but cannot interrupt that write after it starts. Do not assume every command can stop before completion when interrupted.

## Safety summary

| Command | Read-only | Mutates install | Mutates source | Mutates home state |
| --- | --- | --- | --- | --- |
| `guide` | Yes | No | No | No |
| `update` | No | Replaces the running SyncAI executable | No | No |
| `init [source-dir]` | No | No | Creates a starter only when the selected directory is missing or empty | Saves the default source under the XDG config root |
| `validate`, `list`, `status`, `packages status` | Yes | No | No | No |
| `render --check`, `render --dry-run` | Yes | No | No | No |
| `render --out <root>` | No | Only `<root>` | No | No manifest or package changes |
| `render --project <path>` | No | Only project paths | No | No manifest or package changes |
| `render` without `--out` or `--project` | No | `$HOME` | No | Writes the state manifest and may reconcile Pi packages |
| `set-profile` | No | No | No | Writes `~/.pi/agent/active-model-profile.json` |
| `use-profile` | No | `$HOME` | No | Writes the manifest, Pi package state, and active profile after render succeeds |
| `import` | No | No | Yes | Reads installed configuration under `$HOME` |
| `pull` | No | May rerender after a pull | Yes | Uses the state manifest |
| `packages apply` | No | Pi files under `--out`; Claude and Codex user configuration | No | Runs Claude user-scope and Codex plugin commands outside `--out` |
| `packages pull` | No | No | Yes | Reads installed package state |

SyncAI-managed renderer, source, manifest, and Pi package file mutations validate their destination against the applicable output or source root where the implementation routes them through its path guards. Regular SyncAI-managed file writes use a temporary sibling followed by rename, directory copies reject nonregular source files, and manifest removal is limited to recorded paths resolved beneath the active output root. These constraints do not sandbox external tool commands invoked by `packages apply`.

## `guide`

```text
syncai guide
```

Prints a self-contained workflow and mutation reference from the installed binary. SyncAI also renders a reserved built-in `syncai` skill to Pi, Claude Code, Codex, and Antigravity so those tools can discover the guide without repository access. OpenCode receives the guide pointer in its generated shared instructions. Oh My Pi currently has neither a rendered skills surface nor shared instructions.

## `update`

```text
syncai update
```

Fetches the latest published GitHub release, selects the archive matching the current operating system and architecture, verifies the release tag and `checksums.txt` against SyncAI's embedded Ed25519 release public key, verifies the archive's SHA-256 digest, and atomically replaces the resolved running executable while preserving its permission bits. Development builds, version downgrades, invalid signatures, and unsupported operating systems are rejected. A symlinked invocation updates the symlink target.

## `init`

```text
syncai init [source-dir]
```

Without an argument, `init` creates a safe starter source at `${XDG_CONFIG_HOME:-$HOME/.config}/syncai/ai-source`. With an argument, it registers that source directory instead. An existing nonempty source must parse and render successfully before SyncAI saves it; `init` does not rewrite it. A missing or empty selected directory receives the starter files.

After validating the source through every renderer, `init` atomically saves its absolute path in `${XDG_CONFIG_HOME:-$HOME/.config}/syncai/config.json`. It does not render into tool configuration or apply packages.

## `validate`

```text
syncai validate [--source <dir>] [--profile <name>]
```

Parses the base model catalog, agents, skill scopes, extensions, and package manifest without writing. Success prints a count such as `ok: parsed 2 agents, 3 skill dirs, 2 extensions (scopes: map[...])` to stdout. Parse and validation errors go to stderr through the process boundary and exit `1`.

This command does not load environment overlays, instructions, or run renderers. Use an isolated render to catch missing model-role mappings and renderer-specific errors.

## `list`

```text
syncai list [--source <dir>] [--scope home|work]
```

Prints matching and excluded agents, skills, and extensions. It writes only to stdout except for a warning on stderr when a nonempty scope filters out every agent. It does not render or touch the state manifest.

## `render`

```text
syncai render [--source <dir>] [--out <root>] [--profile <name>] [--scope home|work] [--check] [--dry-run] [--force]
syncai render --project <path> [--profile <name>] [--scope home|work] [--check] [--dry-run]
```

Without `--out` or `--project`, SyncAI renders beneath `$HOME`, records hashes in `$XDG_STATE_HOME/syncai/manifest.json` or `~/.local/state/syncai/manifest.json`, prunes outputs recorded by the prior manifest but no longer rendered, and reconciles Pi packages from `packages.json`.

With `--out`, SyncAI renders only beneath that explicit root. It does not read or write the state manifest, prune a prior render, enforce manifest drift protection, or apply packages. This is the recommended inspection mode.

`--project <path>` ignores `--source`, reads `<path>/.pi/agent-source`, and writes project-local output. Pi writes `.pi/agents`, `.pi/skills`, `.pi/extensions`, `.pi/model-profiles.generated.json`, and `AGENTS.md`. Oh My Pi writes `.omp/agents`. Claude Code, Codex, and OpenCode skip project mode. Antigravity currently writes its ordinary `.gemini/antigravity-cli/plugins/dfiles` tree beneath the project.

Normal progress is printed to stdout, including the output root and one written-file count per renderer. Scope-filter warnings and prune warnings use stderr. A renderer, input, path, package, or drift error exits `1`.

### Drift protection and `--force`

Before a default home render, SyncAI hashes files tracked by the prior manifest. If installed bytes differ, it renders an expected shadow tree. Locally edited files that already equal the new source render are considered reconciled. Other edits cause SyncAI to print the paths on stderr, refuse the render, and exit `1`.

Run `syncai pull` to propagate supported edits into source. Pass `--force` to discard unreconciled edits and overwrite them. `--force` affects the manifest-backed home render and `use-profile`; an explicit `--out` or project render does not use that manifest guard.

### `--check`

`--check` renders into a temporary directory and compares it with the selected output root. Current installs print `ok` and exit `0`. Drift prints every differing path to stdout, returns an error, and exits `1`. The comparison is read-only and does not use the state manifest.

### `--dry-run`

`--dry-run` performs the same temporary render and tree comparison as `--check`, prints `(dry-run: no files written)` when that phase returns an error, and returns exit `0`. Input and path errors that occur before comparison still exit `1`. The current implementation also converts non-drift errors returned during the comparison phase to exit `0`, so use `--check` rather than `--dry-run` in automation. It never writes the selected output root.

## `status`

```text
syncai status [--source <dir>] [--out <root>] [--scope home|work]
```

Compares the global SyncAI manifest, a fresh temporary render, and the selected install root. It reports drifted, missing, stale, and untracked resources to stdout, then exits `0` even when findings exist. Invalid input or an I/O error exits `1`.

`--out` changes the install root used for tracked comparisons. Untracked agent, skill, and extension discovery still scans the current user's home directories. `status` is read-only.

## `pull`

```text
syncai pull [name...] [--all] [--source <dir>] [--out <root>] [--scope home|work]
```

With no names and no `--all`, lists manifest-tracked drift candidates and exits `0`. A named pull selects canonical agent or extension names. `--all` selects all supported drift.

Supported changes are written into source, printed to stdout, then redistributed with a forced render. Extension removals are reported but not applied to source. Unsupported agent frontmatter is listed for manual editing and is not partially applied. Warnings about an installed agent without a matching source file use stderr. Errors exit `1`.

## `import`

```text
syncai import [name...] [--all] [--source <dir>]
```

Scans installed roots beneath the current user's home directory. With no selection it lists candidates and exits `0`. A name imports that candidate; `--all` imports every auto-portable candidate, preferring Pi when the same name exists in several tools.

The command writes new canonical files beneath `--source`. It does not render them. It prints imported source paths and review guidance to stdout. Errors exit `1`.

## Import and pull limits

| Resource or origin | Import | Pull |
| --- | --- | --- |
| Pi agent | Automatic; model reverses when the active profile has an exact match | Body, description, tools, and exact-match model role; other changed frontmatter is manual |
| Oh My Pi agent | Detected, manual import | Body, description, tools, and exact-match model role; other changed frontmatter is manual |
| Claude Code agent | Automatic; tool names reverse and exact model aliases become roles | Body, description, tools, and exact-match model role; other changed frontmatter is manual |
| Codex agent | Automatic; body, description, and exact model plus effort are read; tools require a TODO | Body, description, and exact-match model plus effort; other fields are not projected back |
| OpenCode agent | Detected, manual import because permission maps are lossy | Body and description; permission and unsupported frontmatter changes are manual |
| Antigravity agent | Detected, manual import | Body, description, tools, and exact-match model role where reversible; other frontmatter is manual |
| Skill directory | Automatic from Pi, Claude Code, Codex, or Antigravity when it contains `SKILL.md` | Not tracked by `pull` |
| Single-file Pi `.ts` extension | Automatic; imported as universal unless a sidecar is added later | Edited and added bytes copy back verbatim |
| Pi directory extension | Manual vendoring to avoid copying third-party packages | Edited and added regular files copy back; source-only removals require manual deletion |

Imported Pi and Claude agents default to targets `pi, claude, codex, opencode` and scope `home`. Codex imports use the same defaults and add a tools TODO. When model reversal is ambiguous or absent, SyncAI keeps the model with a TODO instead of claiming a portable semantic role.

## Profiles

```text
syncai set-profile <profile>
syncai use-profile <profile> --scope home|work [--source <dir>] [--force]
```

`set-profile` writes the requested string to `~/.pi/agent/active-model-profile.json` without validating it against a source catalog or rendering. Success prints the selected profile and path. Failure exits `1`.

`use-profile` requires `--scope`, renders the selected profile into `$HOME`, and persists it only after the render succeeds. It then prints the resolved Pi role map. It does not accept `--out`, so use an isolated `render --out` first when testing a profile.

## Packages

Every package subcommand accepts `--source`, `--out`, and `--scope`.

### `packages status`

```text
syncai packages status [--source <dir>] [--out <root>] [--scope home|work]
```

Read-only. Prints `ok`, `missing`, `untracked`, and `orphaned` entries for Pi, Claude Code, Codex, and Antigravity. Findings do not change the exit status.

### `packages apply`

```text
syncai packages apply [--source <dir>] [--out <root>] [--scope home|work]
```

Reconciles Pi package settings and managed npm or Git artifacts, adds declared Claude marketplaces and plugins through `claude`, and adds Codex plugins through `codex`. `--out` redirects SyncAI-managed package discovery and Pi files only. Claude plugin installation uses `--scope user`, Claude marketplace commands use Claude's normal user configuration, and Codex plugin commands use Codex's normal user configuration outside `--out`. Do not treat `packages apply --out <root>` as a sandbox for those external commands.

Missing Claude or Codex commands and individual tool failures are warnings on stderr; package status is still printed. Cancellation and Pi reconciliation failures exit `1`. Antigravity plugins are currently reported by status but not installed by apply.

### `packages pull`

```text
syncai packages pull [--source <dir>] [--out <root>] [--scope home|work]
```

Merges untracked installed package and plugin identifiers into `packages.json`. With a scope, untracked Pi packages go into `pi.packagesByScope.<scope>`; without one they go into `pi.packages`. Claude, Codex, and Antigravity entries merge into their ordinary plugin lists. Missing desired resources are reported but not installed.

## Completion

Cobra provides `syncai completion bash`, `fish`, `powershell`, and `zsh`. The generated script is written to stdout. Redirect it to the location required by your shell.
