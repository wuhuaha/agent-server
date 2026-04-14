# Test Layout

`agent-server` keeps Go package-level tests next to the code they verify.

This is intentional:

- package-local tests can exercise unexported behavior without widening production APIs
- `go test ./...` remains simple for the default fast path
- most current tests are package behavior tests, not repository-wide black-box suites

Current test layers:

- Go unit/package tests:
  - live next to production code as `*_test.go`
  - run by `make test-go`
- Go integration tests:
  - still live next to the relevant package when they are tightly coupled to package-local helpers
  - include transport or provider-adapter tests that open local listeners via `httptest`
  - are enabled through `//go:build integration`
  - run by `make test-go-integration`
- Go system tests:
  - cover external runtime dependencies such as `ffmpeg`
  - are enabled through `//go:build system`
  - run by `make test-go-system`
- Python tests:
  - desktop client: `clients/python-desktop-client/tests/`
  - worker: `workers/python/tests/`
- Live validation:
  - archived scripted and manual evidence under `artifacts/live-smoke/` and `artifacts/live-baseline/`

Top-level `tests/` is reserved for future repository-wide black-box suites that do not naturally belong to one package.
