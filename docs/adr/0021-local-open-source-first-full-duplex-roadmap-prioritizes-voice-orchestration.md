# ADR 0021: Local Open-Source-First Full-Duplex Roadmap Prioritizes Voice Orchestration

## Status

Accepted

## Context

On 2026-04-10, the repository was re-evaluated specifically against the requirement to achieve smoother and more natural full-duplex voice interaction while keeping the implementation primarily local and open-source.

That review found:

1. the main current gap is not the top-level architecture
2. the main current gap is not only model quality
3. the bottleneck is the missing density inside the shared voice runtime:
   - streaming ASR
   - server-side endpointing
   - interruption arbitration
   - incremental TTS scheduling
   - heard-text reconciliation

The project already has the right top-level boundaries:

- `Realtime Session Core`
- `Agent Runtime Core`
- `Voice Runtime`
- adapter-only transport layers

The user also explicitly prefers a local and open-source-first route instead of making hosted realtime speech providers the primary solution path.

## Decision

Adopt a local and open-source-first roadmap for full-duplex voice evolution.

The primary implementation direction is:

1. keep the current session-centric architecture
2. evolve `internal/voice` into a stronger shared voice-orchestration layer
3. prioritize local and open-source components for:
   - streaming ASR
   - VAD and endpointing
   - incremental TTS
   - interruption handling
4. keep hosted realtime speech backends as optional comparison baselines, not the primary architecture path

The detailed execution plan is documented in:

- `docs/architecture/local-open-source-full-duplex-roadmap-zh-2026-04-10.md`

The first server-endpoint execution slices should also preserve the current public contracts by default:

- keep the advertised public `turn_mode` on `client_wakeup_client_commit`
- add any preview or endpoint logic first as internal `internal/voice` capabilities plus hidden runtime switches
- let adapters consume preview suggestions instead of embedding provider-specific turn rules
- keep early hidden endpoint policies conservative, including voice-runtime-owned false-endpoint guards, before any public protocol rollout
- let provider endpoint hints enter only through shared voice-runtime deltas and shared turn detectors, never as adapter-local turn rules

## Consequences

- The repository stays aligned with the current session-core and adapter boundaries.
- The main engineering focus shifts to voice orchestration quality rather than provider switching.
- Local and open-source components become the default path for the next voice-runtime upgrades.
- Public protocols can remain stable while the internal voice runtime grows more capable.
- Early server-side endpointing is expected to arrive first as an internal preview mode rather than an immediately advertised wire-contract change.

## Rejected Alternatives

### Make hosted realtime voice providers the default next step

Rejected because it would improve subjective experience quickly, but would weaken the user's local-first requirement and risk moving core behavior outside the shared runtime.

### Keep improving the current commit-driven path without adding real voice orchestration

Rejected because it would produce incremental patches, not a true continuous voice interaction runtime.

### Push turn detection or interruption rules into RTOS, browser, or compatibility adapters

Rejected because it would duplicate behavior across transports and weaken the shared session core.
