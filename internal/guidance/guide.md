# SyncAI guide

SyncAI keeps one canonical AI configuration and renders the native files used by supported coding tools.

## Start safely

Create or register a canonical source, validate it, then inspect an isolated render before writing to your home directory:

    syncai init
    syncai validate
    preview="$(mktemp -d)"
    syncai render --out "$preview"
    syncai render

Source resolution uses the first available value: `--source`, `SYNCAI_SOURCE`, the source saved by `syncai init`, then `./ai-source`.

## Common workflows

Inspect what would render without writing:

    syncai list
    syncai render --out "$(mktemp -d)"

Check installed files before rendering:

    syncai status
    syncai render --check

Move supported edits from installed files back into canonical source:

    syncai status
    syncai pull
    syncai pull <name>

Find installed agents, skills, and extensions that are not in canonical source:

    syncai import
    syncai import <name>
    syncai validate
    syncai render --out "$(mktemp -d)"

Preview and activate another model profile:

    syncai render --out "$(mktemp -d)" --profile <profile> --scope <home|work>
    syncai use-profile <profile> --scope <home|work>

Update the running binary from the latest GitHub release:

    syncai update

## Mutation boundaries

- `validate`, `list`, `status`, `render --check`, and `render --dry-run` are read-only.
- `render --out` writes only beneath the selected output directory.
- `render` without `--out` writes supported tool configuration beneath your home directory.
- `pull <name>`, `pull --all`, `import <name>`, and `import --all` write canonical source. Running `pull` or `import` without a selection only lists candidates.
- `use-profile` renders into your home directory and then saves the active profile.
- `packages apply` may invoke external package managers and tool CLIs.
- `update` verifies the signed release tag and checksums, rejects downgrades, then atomically replaces the running executable.

Run `syncai <command> --help` before an unfamiliar mutation. Use `syncai render --force` only when the user explicitly wants to discard unreconciled installed edits.
