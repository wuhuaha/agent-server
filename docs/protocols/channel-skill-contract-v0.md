# Channel Skill Contract v0

## Intent

A channel skill adapts an external platform such as Feishu into the shared session model. It is not a replacement for the session core or the `Agent Runtime Core`.

## Responsibilities

- Receive channel events.
- Normalize user identity, thread identity, message identity, idempotency keys, and attachments.
- Convert inbound messages into shared session inputs.
- Hand normalized turns to the shared runtime turn contract instead of calling model providers directly.
- Convert outbound responses into channel-specific actions.
- Report delivery and failure state back to the service.

## Non-Responsibilities

- Running model inference directly.
- Owning conversation policy.
- Owning tool orchestration.
- Owning long-term memory strategy.
- Defining a second message protocol outside the shared session core.

## Minimum Contract

- inbound message adapter
- outbound response adapter
- runtime handoff into the shared turn contract
- thread mapping
- attachment mapping
- delivery status reporting
- retry and idempotency hooks

The first shared implementation shape lives in `internal/channel/runtime_bridge.go` and keeps adapters on one narrow path:

1. `Normalize(...)`
2. `TurnExecutor.ExecuteTurn(...)`
3. `Deliver(...)`
4. `ReportDelivery(...)`

Current bridge-oriented data expectations:

- inbound message:
  - channel name
  - external message id
  - user id
  - thread id
  - optional session id
  - retry attempt
  - idempotency key
  - attachments
  - adapter-local metadata
- normalized input:
  - session key
  - thread key
  - normalized message key
  - normalized user text
  - attachments
  - normalized metadata
- outbound message:
  - channel name
  - thread id
  - session id
  - reply target message id
  - idempotency key
  - user-facing text
  - adapter-local delivery metadata
- delivery status:
  - delivered, skipped, or failed
  - failure stage when applicable
  - adapter, channel, session, thread, message, turn, and trace identifiers

## First Planned Target

Feishu is the first planned external channel, but the contract must stay neutral enough to support Slack, Telegram, and similar platforms later.
