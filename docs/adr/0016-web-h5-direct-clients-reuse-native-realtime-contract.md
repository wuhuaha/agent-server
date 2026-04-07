# ADR 0016: Web/H5 Direct Clients Reuse Native Realtime Contract

## Status

Accepted

## Context

The repository already had:

- one native realtime websocket contract at `/v1/realtime/ws`
- one `xiaozhi` compatibility websocket adapter for legacy firmware
- one desktop debug client for local bring-up

The next gap was Web/H5 direct access. The main risk was accidentally creating a browser-only websocket dialect or pushing browser-media quirks into the shared runtime and gateway layers.

## Decision

Web or H5 direct clients reuse the native realtime discovery and websocket contract:

- `GET /v1/realtime`
- `/v1/realtime/ws`
- `agent-server.realtime.v0`

The repository adds a same-service debug page at `/debug/realtime-h5/`, but that page is only a control-plane-hosted bring-up surface. It does not create a second browser-specific session protocol.

Browser adaptation happens at the edge:

- microphone capture is converted to raw mono `pcm16le` frames in the page before websocket send
- server binary audio is decoded from mono `pcm16le` in the page for playback

## Consequences

Positive:

- RTOS, desktop, and browser bring-up share one direct realtime contract
- the shared session core and agent runtime stay transport-neutral
- Web/H5 validation becomes easier because the service can host a same-origin debug page without needing a separate static server

Tradeoffs:

- the first browser slice currently requires mono `pcm16le` discovery on both input and output
- raw browser `opus` uplink is still out of scope for this phase
- remote browser microphone use still depends on HTTPS and WSS

Follow-up direction:

- future browser-specific enhancements should preserve the same semantic event model
- if richer browser transports are added later, they must still avoid turning the browser path into a second orchestration stack
