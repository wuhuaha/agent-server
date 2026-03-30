---
name: repo-bootstrap
description: Use when initializing or restructuring this repository, updating AGENTS.md or plan.md, syncing .claude and .codex context, or creating baseline docs and scaffolding for agent-server.
---

# Repo Bootstrap

## When To Use

Use this skill when the repository structure, collaboration files, planning files, or durable project memory need to be created or updated together.

## Workflow

1. Check `AGENTS.md` and `plan.md` first.
2. Update `.codex/project-memory.md` for durable decisions.
3. Update `.codex/change-log.md` for meaningful changes.
4. Update `.codex/issues-and-resolutions.md` when blockers appear or are resolved.
5. Keep `.claude/` context aligned with the same decisions.

## Guardrails

- Do not change architecture direction silently.
- Keep planning files short and current.
- Prefer one source of truth per decision.
