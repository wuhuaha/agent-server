---
name: Bug Report
about: Report a functional regression, runtime failure, or deployment breakage
title: "[bug] "
labels: bug
---

## Summary

Describe the problem in one or two sentences.

## Impact

- Affected area:
  - `realtime session core`
  - `internal/agent`
  - `internal/voice`
  - `gateway / adapter`
  - `workers/python`
  - `web / h5`
  - `deploy / docker`
  - `docs / protocol`
- User-visible effect:

## Reproduction

1. 
2. 
3. 

## Expected Result

Describe the expected behavior.

## Actual Result

Describe the actual behavior.

## Logs Or Artifacts

- Relevant logs:
- Relevant report or artifact paths:
- Screenshots or recordings:

## Protocol Or Docs Impact

- Realtime or compatibility protocol affected:
- `docs/protocols/` update needed:
- `docs/adr/` update needed:

## Validation Attempted

- [ ] `make doctor`
- [ ] `make test-go`
- [ ] `make test-py`
- [ ] `make docker-config`
- [ ] `make verify-fast`

List the commands you ran and the important results.

## Environment

- Branch or commit:
- OS or runtime:
- Relevant env vars or provider settings:
