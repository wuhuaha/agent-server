# Integration Tests

This directory is reserved for future cross-package or black-box integration suites.

Current integration coverage is still colocated with the relevant Go package when the
tests depend on package-local helpers. Examples:

- `internal/gateway/realtime_ws_test.go`
- `internal/gateway/xiaozhi_ws_test.go`
- `internal/voice/http_transcriber_test.go`
- `internal/voice/iflytek_rtasr_test.go`
- `internal/voice/iflytek_tts_test.go`
- `internal/voice/mimo_tts_test.go`
- `internal/voice/cosyvoice_tts_test.go`
- `internal/voice/volcengine_tts_test.go`

Those tests are enabled with the `integration` build tag and run through:

```bash
make test-go-integration
```

They require local loopback bind permission because the current coverage spins up
package-local `httptest` and websocket servers.
