# System Tests

This directory is reserved for future repository-wide system tests that depend on
external runtimes or binaries.

Current system coverage is still colocated with the relevant Go package when the
test is tightly bound to package-local encoder or transport behavior. Example:

- `internal/gateway/xiaozhi_audio_test.go`

That test is enabled with the `system` build tag and run through:

```bash
make test-go-system
```
