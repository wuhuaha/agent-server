# ADR 0010: Separate Agent Persona From Execution Mode

## Status

Accepted

## Context

The first LLM-backed runtime path already owns a built-in household smart-home assistant prompt. That prompt originally mixed together three different concerns:

- the assistant persona and speaking style
- the user-facing output contract
- the debug-stage simulation policy for control requests

That coupling made the default prompt hard to reuse and would have leaked debug-stage assumptions into future real-control modes. In particular, `live_control` would still have inherited “pretend the action succeeded” instructions even after real execution backends arrive.

## Decision

Inside `internal/agent`, compose the final system prompt from separate runtime-owned pieces:

1. persona template
2. shared runtime output contract
3. execution-mode policy

The first built-in persona selector is:

- `household_control_screen`

The first runtime execution modes are:

- `simulation`
- `dry_run`
- `live_control`

`AGENT_SERVER_AGENT_LLM_SYSTEM_PROMPT` remains as a persona-template override, but execution-mode policy is still appended by the runtime so mode switching does not depend on rewriting every custom prompt.

## Consequences

- The household assistant persona can stay stable while the runtime switches among simulation, dry-run, and future live-control behaviour.
- `live_control` no longer inherits debug-stage simulated-success instructions by default.
- Transport and voice layers remain unaware of persona or execution-mode prompt assembly; the policy stays in the shared agent runtime.
- Custom prompt overrides still support `{{assistant_name}}`, but they no longer bypass the runtime’s execution-mode contract.
