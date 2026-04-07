# Channel Skill Contract v0

## Intent

A channel skill adapts an external platform such as Feishu into the shared session model. It is not a replacement for the session core or the `Agent Runtime Core`.

## Responsibilities

- Receive channel events.
- Normalize user identity, thread identity, and attachments.
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

## First Planned Target

Feishu is the first planned external channel, but the contract must stay neutral enough to support Slack, Telegram, and similar platforms later.
