# ADR 0032: Voice Architecture Converges On Server-Primary Hybrid Runtime-Orchestrated Duplexing

- Status: accepted
- Date: 2026-04-16

## Context

The repository already has several important voice-runtime slices in place:

- internal input and output session lanes
- shared server-endpoint candidate discovery
- runtime-owned preview and playout orchestration
- heard-text persistence
- soft output arbitration and earlier audio start hooks
- a local/open-source-first GPU path built around FunASR and CosyVoice

However, those slices were still documented mostly as separate optimizations. The project lacked one integrated voice-architecture baseline that explains:

- who owns turn-taking after wake and connection
- which responsibilities stay on the device versus the shared runtime
- how preview, endpointing, early processing, output orchestration, and playback truth fit together
- how the current compatibility protocol should evolve without breaking the existing v0 device path

## Decision

Adopt the following voice architecture baseline for the project.

### 1. Session-established voice interactions use a `server-primary hybrid` model

After wakeup and session establishment:

- the shared server runtime owns turn-taking, endpoint acceptance, interruption arbitration, response start, and playback-truth interpretation
- devices retain audio-front-end, local reflex, playback execution, playback telemetry, and fallback/manual override responsibilities

### 2. `internal/voice` is the shared voice orchestration runtime, not only a provider adapter layer

`internal/voice` is the long-term owner of four cooperating loops:

- input preview loop
- early processing loop
- output orchestration loop
- playback truth loop

Gateway adapters report transport and playout facts into that runtime boundary; they do not become a second orchestration layer.

### 3. Early processing uses a layered gate object instead of one scalar threshold

The shared runtime should reason over a unified early-processing gate that includes at least:

- prefix stability
- utterance completeness
- slot completeness
- correction risk
- action risk

This gate may unlock preview, draft, and commit actions at different readiness levels. Irreversible actions remain gated conservatively.

### 4. Interruption policy keeps four outcomes, with `duck_only` as a real middle state

The target shared policy outcomes remain:

- `ignore`
- `backchannel`
- `duck_only`
- `hard_interrupt`

`duck_only` is treated as a short-lived reversible output policy rather than only as a logged classification.

### 5. Playback truth must stay separate from generated text

The runtime must continue to distinguish:

- generated text
- delivered text / sent audio
- playback facts
- heard-text estimate
- interrupted / truncated / completed state

Future adapter and protocol work should prefer stronger playback start/mark/clear/complete facts over assuming the full generated reply was actually heard.

### 6. Public realtime contracts stay additive and compatibility-first while this architecture graduates

The currently published v0 contract may continue to advertise explicit client commit as the baseline. Richer server-endpoint, lane-state, and playback-fact behaviors should graduate through additive capability exposure rather than a second protocol family.

## Consequences

### Positive

- The repository now has one integrated voice architecture reference instead of scattered optimization notes.
- Voice improvements can continue without violating the existing session-centric and adapter-thin boundaries.
- The next priority becomes behavior depth inside shared runtime loops instead of ad hoc adapter-local fixes.
- Future evaluation can align around milestone latency, interruption quality, and playback-truth correctness rather than only total response time.

### Tradeoffs

- The shared voice runtime now carries more orchestration responsibility and must remain disciplined about transport neutrality.
- Some behavior improvements, especially playback truth fidelity, still depend on stronger adapter facts that are not fully available everywhere today.
- The public protocol may temporarily lag behind internal runtime maturity while capability exposure stays compatibility-first.
