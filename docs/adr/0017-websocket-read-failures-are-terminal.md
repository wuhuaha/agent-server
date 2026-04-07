# ADR 0017: Websocket Read Failures Are Terminal

## Status

Accepted

## Context

The native realtime websocket handler and the `xiaozhi` compatibility websocket handler both used connection read deadlines to enforce idle timeout and max-duration policy.

During live browser-side usage, the native realtime path hit:

- `http: panic serving ... repeated read on failed websocket connection`

The root cause was transport-level, not session-core-level: after `gorilla/websocket` returned a timeout read error, the handler treated that timeout as recoverable and looped back into another `ReadMessage()`. `gorilla/websocket` explicitly treats repeated reads on a failed connection as application misuse and will panic to expose that bug.

## Decision

For gateway websocket adapters in this repository, any `ReadMessage()` error is terminal for that connection.

Timeout-triggered read failures may still drive one final transport-specific close action before the handler returns:

- native realtime may emit `session.end` with the mapped close reason
- `xiaozhi` compatibility may emit compat `tts stop`

But after a websocket read failure, the handler must return and let the connection close. It must not attempt to keep the connection alive by applying a new read deadline and calling `ReadMessage()` again.

## Consequences

Positive:

- gateway timeout teardown now matches `gorilla/websocket` connection semantics
- timeout-driven idle or max-duration closure can still send one final user-visible close signal
- the fix stays inside device adapters and does not change session-core or runtime ownership

Tradeoffs:

- an unexpected timeout that does not map cleanly to idle or max-duration now closes the websocket instead of attempting recovery
- any future long-lived websocket keepalive scheme must avoid relying on recoverable read-deadline failures

Follow-up direction:

- if more nuanced idle or liveness behavior is needed later, implement it with explicit timers or transport heartbeats rather than retrying reads after a failed websocket read
