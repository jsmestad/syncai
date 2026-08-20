# Changelog

All notable user-facing changes to Syncai are documented here.

## 1.0.0 - 2026-08-20

### Added

- One canonical source format for agents, skills, shared instructions, Pi extensions, package inventories, semantic model profiles, and home or work overlays.
- Deterministic rendering for Pi, Oh My Pi, Claude Code, Codex, OpenCode, and Antigravity CLI.
- Validation, isolated rendering, installed-state status, bounded import, reversible pull, profile selection, drift protection, and package reconciliation commands.
- Filesystem containment, traversal rejection, symlink protections, atomic writes, deterministic golden fixtures, public-source scanning, static analysis, and vulnerability analysis.
- Signed macOS and Linux release archives with SHA-256 checksums, archive SBOMs, and GitHub artifact attestations.

### Compatibility

- Prebuilt archives support macOS and Linux on AMD64 and ARM64. Windows and other operating systems or architectures are not release targets.
- Building from source and `go install` support Go 1.25 and Go 1.26.
- The v1.0.0 command behavior, source format, target coverage, safety rules, and reverse-conversion limits are defined by the public README and reference documentation shipped with this release.
