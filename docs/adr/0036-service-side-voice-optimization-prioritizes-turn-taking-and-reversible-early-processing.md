# ADR 0036: Service-Side Voice Optimization Prioritizes Turn-Taking And Reversible Early Processing

## Status

Accepted

## Context

The current repository already has the first real shared voice-runtime slices in place:

- shared preview sessions and `server_endpoint` candidate path
- dual-track input/output session state
- soft interruption outcomes including `backchannel` and `duck_only`
- earlier output start through `TurnResponseFuture` and speech planning
- playback-truth ingestion and next-turn `voice.previous.*` runtime context
- bounded preview-driven prewarm from `stable_prefix`

That means the main service-side bottleneck is no longer “missing basic capability”. The main gap is now behavior maturity inside the shared runtime:

- turn-taking still relies heavily on silence and lexical heuristics
- interruption still lacks a stronger acoustic-first verifier
- early processing exists, but not yet as one explicit layered gate
- speech planning still remains closer to chunk heuristics than full clause/prosody planning
- playback truth still needs finer alignment to support resume and interruption quality at a higher level

At the same time, external research and production guidance from OpenAI, Google, Amazon, Apple, LiveKit, Pipecat, and FunASR all point in the same direction:

- do not treat endpointing as a single silence threshold problem
- keep early work reversible and layered
- keep interruption fast and preferably acoustic-first
- keep contextual biasing dynamic instead of globally hard-biased
- keep cascade-style speech systems viable by redesigning orchestration rather than assuming a monolithic speech-to-speech model is automatically superior for the current product stage

## Decision

For the next service-side voice optimization stage, the repository should prioritize the following shared-runtime direction before widening model surface or public protocol complexity:

1. upgrade turn-taking from silence-first heuristics to a multi-signal shared turn arbitrator
2. upgrade interruption from transcript-led heuristics to acoustic-first plus semantic-confirmation arbitration
3. formalize early processing as a layered, reversible gate rather than isolated optimizations
4. evolve speech planning toward clause/prosody-aware output orchestration
5. deepen playback truth into a finer-grained heard-cursor and resume foundation
6. bring dynamic bias/alias/entity catalog behavior into the runtime path instead of leaving it as research-only knowledge

The primary architecture remains the current server-primary hybrid cascade:

- streaming STT / preview
- shared voice-runtime orchestration
- shared agent runtime
- streamed TTS / audio output

Speech-to-speech models may still be evaluated as future baselines or selective substitutes, but they are not the current main architecture decision.

## Consequences

Positive:

- keeps work aligned with the existing `internal/session` / `internal/voice` / `internal/agent` boundaries
- targets the biggest remaining experience bottlenecks directly
- preserves transport compatibility while improving real-device naturalness
- avoids chasing model surface expansion before the runtime behavior is mature enough to benefit from it

Tradeoffs:

- the shared voice runtime becomes behaviorally richer and more complex
- more observability and eval discipline are required to keep regressions manageable
- some later optimization opportunities will depend on better structured signals from local workers

## Follow-Up

- add a dedicated service-side optimization research note under `docs/architecture/`
- update `plan.md` and `docs/architecture/overview.md` so the next priority order is explicit
- keep implementation slices sequenced as:
  - multi-signal turn arbitrator
  - acoustic-first interruption verifier
  - clause-aware speech planner
  - playback-truth refinement
  - dynamic bias runtime integration
