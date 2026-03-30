---
name: channel-skill-design
description: Use when designing or implementing external channel integrations such as Feishu, Slack, or Telegram so they stay adapters over the shared session core instead of becoming a second orchestration layer.
---

# Channel Skill Design

## When To Use

Use this skill for any external messaging channel integration or when defining channel adapter contracts.

## Core Rules

- A channel is an adapter, not the core runtime.
- Normalize inbound content into shared session inputs.
- Normalize outbound responses into channel-native actions.
- Keep retries, delivery state, and thread mapping explicit.

## Required Follow-Through

- Update `docs/protocols/channel-skill-contract-v0.md` if the contract changes.
- Keep channel-specific logic out of the session core.
- Record durable design choices in `.codex/project-memory.md`.
