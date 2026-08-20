---
name: syncai
description: Use SyncAI to manage shared agents, skills, instructions, extensions, packages, and model profiles across supported coding tools. Load when the user mentions SyncAI or asks to inspect or change canonical AI configuration.
---

# SyncAI

SyncAI is installed as `syncai`. Run `syncai guide` before changing shared AI configuration, then use `syncai <command> --help` for the exact command interface.

Prefer `syncai validate`, `syncai list`, `syncai status`, or an isolated `syncai render --out "$(mktemp -d)"` while inspecting configuration. Do not run a home render, pull, import, profile change, package application, or forced overwrite unless the user requested the corresponding mutation.
