# Contributing

Contributions should keep Syncai's canonical source neutral, renderer output deterministic, and filesystem changes inside explicit roots.

## Prerequisites

Use Go 1.25 or Go 1.26. The module declares Go 1.25.0 as its minimum version.

## Development workflow

1. Make the smallest change that fully implements the behavior.
2. Add or update focused tests, including command-level behavior when stdout, stderr, exit behavior, or filesystem effects change.
3. Run the authoritative local check:

```bash
make check
```

`make check` verifies formatting, `go vet`, all tests, and the CLI build.

## Golden renderer output

The complete neutral fixture in `examples/complete` renders into the reviewed trees under `testdata/golden`. If an intentional renderer change alters those bytes, regenerate them explicitly:

```bash
UPDATE_GOLDEN=1 go test ./internal/cli -run TestCompleteExampleMatchesGoldenTrees
```

Review every changed golden file before submitting. Never set `UPDATE_GOLDEN=1` in normal test or CI commands, and do not update goldens merely to silence an unexplained failure.

## Fixtures and public examples

New renderer behavior must include a neutral fixture or extend `examples/complete` with invented names, provider identifiers, prompts, scopes, and paths. Do not add personal configuration, employer names, private repository paths, credentials, customer data, or generated home-directory output.

Keep examples inert. Package fixtures must not install software or call external commands during tests. Tests must use temporary source, output, home, and state roots when behavior could otherwise touch a contributor's real configuration.

## Pull requests

Explain the behavior change, the affected source and target formats, conversion limits, and the commands you ran. Include golden diffs when output changes. New source fields or commands must update the relevant reference docs in the same change.

By participating, you agree to follow the [Code of Conduct](CODE_OF_CONDUCT.md).
