# ADR 0038: Clause-Aware Output Orchestration Starts Audio Before Final Turn Settlement

## Status

Accepted

## Context

The repository already had the first shared early-audio path:

- responders may expose `TurnResponseFuture`
- the native realtime gateway may start audio before the final `TurnResponse` settles
- `internal/voice/speech_planner.go` could split text into earlier TTS chunks

But that earlier planner still had two practical limits:

- it only produced raw `[]string` chunks, so runtime code could not reason about boundary strength, launchability, or prosody-like intent
- planner enqueue used an unbuffered queue, so a slow first TTS build could back-pressure later text-delta handling and hurt realtime feel

There was also one gateway race left: when early audio started before the first text delta had been consumed, `response.start` could advertise only `audio`, and `StartResponseAudio(...)` could receive an empty aggregated text even though the planner already knew the first clause text.

## Decision

The shared early-audio path now uses an internal clause-aware orchestration shape without widening the public protocol:

1. `internal/voice.SpeechPlanner` now emits structured `PlannedSpeechClause` objects internally
2. each clause carries boundary kind, prosody hint, whether it may start before final turn settlement, and an estimated playback duration
3. `plannedSpeechSynthesis` now uses a small buffered clause queue so later deltas are less likely to block behind one slow TTS startup
4. gateway orchestration now treats `ResponseAudioStart.Text` as the best available early speech text when audio wins the race
5. when early audio already has clause text, `response.start` may advertise `text,audio` even before the first streamed text delta is observed

The wire contract stays compatible:

- no new websocket event names
- no clause-level public schema yet
- no provider-specific SSML or prosody surface added to transports

## Consequences

Positive:

- earlier TTS start keeps moving closer to the first stable clause instead of waiting for final response settlement
- text-delta flow is less likely to stall behind one slow planner synthesis task
- playback metadata and heard-text initialization now have a better early text fallback when audio starts first
- prosody-like structure can evolve internally before any protocol-level clause schema is needed

Tradeoffs:

- planner behavior is richer and needs more regression coverage
- prosody hints are currently internal metadata, not a guaranteed provider-consumed feature
- later clause-level playback truth and resume may still need finer-grained cursoring

## Follow-Up

- use the new clause metadata to guide more provider-aware prosody only where it does not break text/audio truth alignment
- extend the same output-orchestration guarantees across remaining adapters
- keep later protocol work additive and driven by embedded-client needs, not by internal planner representation alone
