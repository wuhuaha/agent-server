# ADR 0009: Advertise Commit-Driven Turn Semantics Until Server VAD Exists

## Status

Accepted

## Context

The realtime gateway and the `xiaozhi` compatibility adapter already accept streamed audio uplink, but the current runtime still depends on an explicit turn-finalization boundary:

- native RTOS clients end a spoken turn with `audio.in.commit`
- text turns are committed explicitly through `text.in`
- `xiaozhi` compatibility maps `listen.stop` or `listen.detect` onto the same commit-driven runtime path

However, discovery defaults and config examples still advertised `client_wakeup_server_vad`, which implied a published server-side VAD boundary that does not yet exist in the runtime contract. The protocol docs also still mentioned an `armed` state that the current implementation does not publish.

That mismatch risks incorrect device integrations and weakens the repository rule that discovery, docs, schemas, and runtime behaviour must stay aligned.

## Decision

Until a real server-side VAD commit path exists and is validated end to end, the public realtime turn-taking contract is:

- `turn_mode = client_wakeup_client_commit`

And the current public session-update state set is narrowed to the states the server actually emits today:

- `active`
- `thinking`
- `speaking`

`idle` and `closing` remain valid internal lifecycle states in the session core, but are not described as steady-state `session.update` outputs. The unused `armed` state is removed from the current public contract.

## Consequences

- Device teams can integrate against one accurate v0 behaviour: client wakeup starts the session, client commit ends each user turn.
- Discovery responses, `.env.example`, protocol docs, and schemas now describe the same turn boundary.
- Future server-side VAD work remains possible, but it must land as a real runtime capability before discovery can advertise it.
- The session core stays transport-neutral; the change only narrows what the current public contract claims, rather than pushing provider or adapter logic into the core.
