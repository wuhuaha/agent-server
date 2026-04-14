# ADR 0027: Standalone Reference Clients Live Under `clients`

## Status

Accepted

## Context

The repository currently carries multiple reusable protocol-facing validation surfaces:

- a packaged Python desktop client under `clients/python-desktop-client`
- a standalone browser realtime debug client

The browser client had been placed under `tools/`, which blurred the intended repository taxonomy:

- `clients/` already represented reusable endpoint implementations over shared protocols
- `tools/` should stay focused on auxiliary diagnostics, capture, bootstrap, and one-off lab utilities

Leaving standalone protocol clients under `tools/` makes the boundary harder to reason about and encourages future reusable clients to be filed as ad hoc helpers rather than stable reference endpoints.

## Decision

Keep reusable reference or debug clients under `clients/`, even when their primary purpose is bring-up, testing, or manual validation.

Move the standalone browser realtime debug client to `clients/web-realtime-client` and update repository docs, scripts, and records accordingly.

Reserve `tools/` for non-client helper surfaces such as diagnostics, recording, conversion, environment bootstrap, or capture scaffolding.

## Consequences

- repository layout now distinguishes reusable protocol endpoints from helper utilities more clearly
- future browser, desktop, mobile, RTOS mock, or channel-facing validation clients have one obvious home under `clients/`
- scripts and docs that refer to standalone browser validation now point at `clients/web-realtime-client`
