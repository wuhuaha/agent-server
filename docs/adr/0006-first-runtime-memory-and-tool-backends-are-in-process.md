# ADR 0006: First Runtime Memory And Tool Backends Stay In Process

- Status: accepted
- Date: 2026-03-31

## Context

The runtime boundary for memory and tools already existed under `internal/agent`, but app bootstrap still injected only no-op implementations. The next step needed to make that boundary actually useful without collapsing transport adapters into orchestration code or introducing premature service dependencies before the first channel adapter lands.

## Decision

- Make `in_memory` the default runtime memory provider and keep only a bounded recent-turn window in the Go process.
- Key that first memory backend by `device_id` when available, falling back to `session_id` otherwise.
- Make `builtin` the default runtime tool provider and implement it as one in-process backend that satisfies both `ToolRegistry` and `ToolInvoker`.
- Start the builtin tool surface with local runtime-safe tools only: `time.now`, `session.describe`, and `memory.recall`.
- Keep `noop` as an explicit fallback provider option, but do not make it the default runtime path anymore.
- Surface bootstrap memory recall through runtime commands such as `/memory`, not through transport-specific endpoints.

## Consequences

- The `Agent Runtime Core` now has a real, testable backend path without adding network dependencies or transport-owned orchestration.
- Device and channel adapters still remain ingress or egress layers only; they do not know whether memory or tools are local or remote.
- The first runtime memory is intentionally ephemeral and process-local, so it is suitable for bring-up but not yet for shared durability or cross-instance recall.
- Future persistent memory stores or remote tool providers can replace these defaults behind the same interfaces once product pressure justifies that extra complexity.
