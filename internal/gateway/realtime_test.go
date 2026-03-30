package gateway

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRealtimeDiscoveryIncludesWireProfile(t *testing.T) {
	handler := NewRealtimeHandler(RealtimeProfile{
		WSPath:           "/v1/realtime/ws",
		ProtocolVersion:  "rtos-ws-v0",
		Subprotocol:      "agent-server.realtime.v0",
		AuthMode:         "disabled",
		TurnMode:         "client_wakeup_server_vad",
		IdleTimeoutMs:    15000,
		MaxSessionMs:     300000,
		MaxFrameBytes:    4096,
		InputCodec:       "pcm16le",
		InputSampleRate:  16000,
		InputChannels:    1,
		OutputCodec:      "pcm16le",
		OutputSampleRate: 16000,
		OutputChannels:   1,
		AllowOpus:        true,
		AllowTextInput:   true,
		AllowImageInput:  false,
	})

	req := httptest.NewRequest(http.MethodGet, "/v1/realtime", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d", http.StatusOK, res.Code)
	}

	var body map[string]any
	if err := json.Unmarshal(res.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got := body["protocol_version"]; got != "rtos-ws-v0" {
		t.Fatalf("expected protocol_version rtos-ws-v0, got %v", got)
	}
	if got := body["ws_path"]; got != "/v1/realtime/ws" {
		t.Fatalf("expected ws_path /v1/realtime/ws, got %v", got)
	}
}
