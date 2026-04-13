---
name: Architecture Or Feature Task
about: Propose a feature, refactor, or migration slice aligned with the runtime boundaries
title: "[task] "
labels: enhancement
---

## Goal

Describe the desired outcome.

## Why This Slice Matters

- User or operator value:
- Architecture or maintainability value:

## Affected Boundary

- Primary boundary:
  - `realtime session core`
  - `internal/agent`
  - `internal/voice`
  - `gateway / adapter`
  - `channel skill`
  - `workers/python`
  - `web / h5`
  - `deploy / docker`
  - `observability / eval`
- Secondary boundaries:

## Scope

- In scope:
- Explicitly out of scope:

## Protocol, ADR, Or Docs Follow-through

- `docs/protocols/` updates expected:
- `schemas/` updates expected:
- `docs/adr/` updates expected:
- `plan.md` milestone or execution-log update expected:

## Acceptance Criteria

- [ ] 
- [ ] 
- [ ] 

## Validation Plan

- Planned commands:
  - `make doctor`
  - `make test-go`
  - `make test-py`
  - `make docker-config`
  - `make verify-fast`
- Additional live or manual validation:

## Risks

- Main technical risk:
- Rollback or containment approach:
