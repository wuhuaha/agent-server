package voice

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHTTPTranscriber(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if got := payload["codec"]; got != "pcm16le" {
			t.Fatalf("expected codec pcm16le, got %v", got)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"text":        "hello from worker",
			"segments":    []string{"hello from worker"},
			"duration_ms": 1000,
			"model":       "SenseVoiceSmall",
			"device":      "cpu",
		})
	}))
	defer server.Close()

	transcriber := NewHTTPTranscriber(server.URL, 5*time.Second, "auto")
	result, err := transcriber.Transcribe(context.Background(), TranscriptionRequest{
		SessionID:    "sess_test",
		DeviceID:     "rtos-001",
		AudioPCM:     make([]byte, 3200),
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("transcribe failed: %v", err)
	}
	if result.Text != "hello from worker" {
		t.Fatalf("unexpected text %q", result.Text)
	}
	if result.Device != "cpu" {
		t.Fatalf("unexpected device %q", result.Device)
	}
}
