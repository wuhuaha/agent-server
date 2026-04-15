# ADR 0029: Server Endpoint Candidate Is Discovery-Advertised Before Default

## Status

Accepted

## Context

The phase-1 voice demo now already has most of the shared runtime pieces needed for server-side turn finalization:

- `internal/voice` owns preview sessions, turn suggestions, playout callbacks, and heard-text persistence
- websocket adapters only consume shared preview observations and commit suggestions
- false-endpoint protection, provider endpoint hints, and barge-in thresholds already stay inside the shared voice-runtime boundary
- the desktop runner and live-sample validation already cover the no-client-commit preview path

That means the old "hidden experimental preview" framing is no longer the right operational shape for this repository. The path is mature enough to be treated as a serious main-path candidate.

At the same time, the public v0 turn-taking contract still cannot flip blindly:

- many clients still depend on `client_wakeup_client_commit`
- explicit `audio.in.commit` remains the safest compatibility fallback
- the repository still needs more real-device and latency validation before making server endpointing the unconditional default

## Decision

Promote server endpointing from a purely hidden experiment to a discovery-advertised main-path candidate, without changing the default published `turn_mode` yet.

Specifically:

- keep the public discovery `turn_mode` on `client_wakeup_client_commit` for now
- expose a structured `server_endpoint` object from discovery/info surfaces so clients and tooling can see:
  - whether the path is available on the current voice provider
  - whether it is enabled on the current instance
  - that it is now a `main_path_candidate`
  - that explicit client commit remains compatible
  - which shared runtime thresholds govern the candidate path
- keep all endpointing logic inside `internal/voice`; adapters still must not own provider-specific endpoint policy
- keep the desktop/browser validation tooling aware of this candidate path so live bring-up no longer depends on hidden tribal knowledge

## Consequences

- server endpointing becomes a first-class candidate in discovery, runbooks, and debug surfaces before any default-mode flip
- clients can prepare for the path explicitly instead of inferring behavior from ad hoc notes or internal env vars alone
- the published websocket event shapes and `turn_mode` stay backward-compatible during this candidate stage
- the future default flip should become a smaller change: it can focus on rollout confidence and naming, not on inventing a second discovery story
