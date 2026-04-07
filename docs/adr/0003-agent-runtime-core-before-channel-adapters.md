# ADR 0003: Insert Agent Runtime Core Before Channel Adapters

- Status: accepted
- Date: 2026-03-30

## Context

The realtime bootstrap path now has working session management, ASR, and TTS, but its turn policy still risks spreading across responders and transport handlers. If Feishu or other channel adapters are added on top of that shape, channel code would either duplicate agent policy or couple directly to bootstrap responder behavior.

## Decision

- Insert an `Agent Runtime Core` milestone before the first channel adapter milestone.
- Define a transport-neutral `TurnExecutor` boundary under `internal/agent`.
- Route bootstrap text handling and session-close intent through that executor boundary.
- Keep device adapters and future channel skills as ingress and egress layers only.
- Keep voice runtime as the built-in ASR/TTS capability that prepares turns for the executor and renders speech afterward.

## Consequences

- The websocket gateway no longer needs to own bootstrap end-of-dialog policy.
- Future Feishu, Slack, or Telegram adapters can target one shared turn contract instead of responder-local logic.
- The next architecture step is streamed agent output plus tool or memory hooks, not another transport-specific orchestration path.
