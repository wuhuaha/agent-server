# ADR 0031: Soft Output Arbitration And Early Audio Stay Runtime-Internal

- Status: accepted
- Date: 2026-04-15

## Context

The first dual-track realtime slice separated input and output session lanes, but two follow-up gaps remained on the main native realtime path:

1. speaking-time `backchannel` and `duck_only` decisions were persisted and logged, but they did not actually affect output playout
2. speech-planner audio could already be synthesized incrementally, but the gateway still waited for the final `TurnResponse` envelope before starting playback

We want both upgrades without widening the public websocket protocol again.

## Decision

Keep both behaviors inside shared runtime and gateway internals:

- `PlaybackDirective` remains a runtime-internal contract between interruption policy and output playout
- native realtime output now applies soft ducking on the shared PCM16 playout path for `backchannel` and `duck_only`
- `hard_interrupt` remains the only policy that returns the session to `active` immediately
- earlier audio start now uses internal `OrchestratingResponder` + `TurnResponseFuture` hooks so the gateway can begin playback before the final `TurnResponse` settles
- the public websocket contract stays additive and compatible:
  - no new public interruption events
  - no new public early-audio event family
  - `response.start.payload.modalities` remains only an early hint

## Consequences

- The native realtime adapter gains more natural speaking-time behavior without teaching devices new protocol verbs.
- Shared voice responders can expose planned audio earlier while still falling back to the previous finalize-stage path when early audio is unavailable.
- Future adapters such as `xiaozhi` can adopt the same internal early-audio hook later without changing the shared public protocol.
- Validation must continue to rely on runtime tests and logs for soft arbitration details, because those details are intentionally not promoted to mandatory wire events.
