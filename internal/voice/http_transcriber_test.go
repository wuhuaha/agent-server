//go:build integration

package voice

import (
	"context"
	"encoding/base64"
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
			"text":            "hello from worker",
			"segments":        []string{"hello from worker"},
			"duration_ms":     1000,
			"elapsed_ms":      87,
			"model":           "SenseVoiceSmall",
			"device":          "cpu",
			"language":        "en",
			"emotion":         "neutral",
			"speaker_id":      "spk-1",
			"audio_events":    []string{"speech"},
			"endpoint_reason": "server_vad",
			"partials":        []string{"hello", "hello from worker"},
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
	if result.ElapsedMs != 87 {
		t.Fatalf("unexpected elapsed ms %d", result.ElapsedMs)
	}
	if result.Language != "en" {
		t.Fatalf("unexpected language %q", result.Language)
	}
	if result.Emotion != "neutral" {
		t.Fatalf("unexpected emotion %q", result.Emotion)
	}
	if result.SpeakerID != "spk-1" {
		t.Fatalf("unexpected speaker id %q", result.SpeakerID)
	}
	if result.EndpointReason != "server_vad" {
		t.Fatalf("unexpected endpoint reason %q", result.EndpointReason)
	}
	if len(result.Partials) != 2 || result.Partials[1] != "hello from worker" {
		t.Fatalf("unexpected partials %+v", result.Partials)
	}
	if result.Mode != "batch" {
		t.Fatalf("unexpected mode %q", result.Mode)
	}
}

func TestHTTPTranscriberStreamingLifecycle(t *testing.T) {
	paths := make([]string, 0, 4)
	pushCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		switch r.URL.Path {
		case "/v1/asr/stream/start":
			if got := payload["turn_id"]; got != "turn_test" {
				t.Fatalf("unexpected turn id %v", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stream_id": "strm_test",
				"mode":      "stream_preview_batch",
				"status":    "started",
			})
		case "/v1/asr/stream/push":
			pushCount++
			if got := payload["stream_id"]; got != "strm_test" {
				t.Fatalf("unexpected stream id %v", got)
			}
			audioBase64, _ := payload["audio_base64"].(string)
			audioBytes, err := base64.StdEncoding.DecodeString(audioBase64)
			if err != nil {
				t.Fatalf("invalid audio base64: %v", err)
			}
			if len(audioBytes) == 0 {
				t.Fatal("expected non-empty audio chunk")
			}
			response := map[string]any{
				"stream_id":      "strm_test",
				"latest_partial": "ni",
				"mode":           "stream_preview_batch",
			}
			if pushCount == 1 {
				response["preview_text"] = "ni"
				response["preview_changed"] = true
				response["preview_endpoint_reason"] = "preview_tail_silence"
			} else {
				response["preview_text"] = "ni hao"
				response["preview_changed"] = true
				response["latest_partial"] = "ni hao"
				response["preview_endpoint_reason"] = "preview_tail_silence"
			}
			_ = json.NewEncoder(w).Encode(response)
		case "/v1/asr/stream/finish":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stream_id":       "strm_test",
				"text":            "你好",
				"duration_ms":     80,
				"elapsed_ms":      12,
				"language":        "zh",
				"endpoint_reason": "stream_finish",
				"partials":        []string{"ni", "ni hao", "你好"},
				"mode":            "stream_preview_batch",
			})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	transcriber := NewHTTPTranscriber(server.URL+"/v1/asr/transcribe", 5*time.Second, "auto")
	sink := &recordingDeltaSink{}
	stream, err := transcriber.StartStream(context.Background(), TranscriptionRequest{
		SessionID:    "sess_test",
		TurnID:       "turn_test",
		TraceID:      "trace_test",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
		Language:     "zh",
	}, sink)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}
	if err := stream.PushAudio(context.Background(), []byte{0x01, 0x02}); err != nil {
		t.Fatalf("PushAudio #1 failed: %v", err)
	}
	if err := stream.PushAudio(context.Background(), []byte{0x03, 0x04}); err != nil {
		t.Fatalf("PushAudio #2 failed: %v", err)
	}
	result, err := stream.Finish(context.Background())
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}
	if len(paths) != 4 {
		t.Fatalf("unexpected paths %+v", paths)
	}
	if paths[0] != "/v1/asr/stream/start" || paths[1] != "/v1/asr/stream/push" || paths[2] != "/v1/asr/stream/push" || paths[3] != "/v1/asr/stream/finish" {
		t.Fatalf("unexpected path sequence %+v", paths)
	}
	if result.Text != "你好" {
		t.Fatalf("unexpected text %q", result.Text)
	}
	if result.Language != "zh" {
		t.Fatalf("unexpected language %q", result.Language)
	}
	if result.Mode != "stream_preview_batch" {
		t.Fatalf("unexpected mode %q", result.Mode)
	}
	if len(result.Partials) != 3 || result.Partials[2] != "你好" {
		t.Fatalf("unexpected partials %+v", result.Partials)
	}
	if len(sink.deltas) != 5 {
		t.Fatalf("unexpected deltas %+v", sink.deltas)
	}
	if sink.deltas[0].Kind != TranscriptionDeltaKindSpeechStart {
		t.Fatalf("unexpected first delta %+v", sink.deltas[0])
	}
	if sink.deltas[1].Kind != TranscriptionDeltaKindPartial || sink.deltas[1].Text != "ni" {
		t.Fatalf("unexpected second delta %+v", sink.deltas[1])
	}
	if sink.deltas[1].EndpointReason != "preview_tail_silence" {
		t.Fatalf("unexpected second delta endpoint reason %+v", sink.deltas[1])
	}
	if sink.deltas[2].Kind != TranscriptionDeltaKindPartial || sink.deltas[2].Text != "ni hao" {
		t.Fatalf("unexpected third delta %+v", sink.deltas[2])
	}
	if sink.deltas[3].Kind != TranscriptionDeltaKindSpeechEnd || sink.deltas[3].EndpointReason != "stream_finish" {
		t.Fatalf("unexpected fourth delta %+v", sink.deltas[3])
	}
	if sink.deltas[4].Kind != TranscriptionDeltaKindFinal || sink.deltas[4].Text != "你好" {
		t.Fatalf("unexpected final delta %+v", sink.deltas[4])
	}
}

func TestHTTPTranscriberStreamingEmitsEndpointHintEvenWhenPreviewTextIsStable(t *testing.T) {
	pushCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		switch r.URL.Path {
		case "/v1/asr/stream/start":
			_ = json.NewEncoder(w).Encode(map[string]any{"stream_id": "strm_hint"})
		case "/v1/asr/stream/push":
			pushCount++
			response := map[string]any{
				"stream_id":      "strm_hint",
				"preview_text":   "ni",
				"latest_partial": "ni",
				"mode":           "stream_preview_batch",
			}
			if pushCount == 1 {
				response["preview_changed"] = true
			} else {
				response["preview_changed"] = false
				response["preview_endpoint_reason"] = "preview_tail_silence"
			}
			_ = json.NewEncoder(w).Encode(response)
		case "/v1/asr/stream/finish":
			_ = json.NewEncoder(w).Encode(map[string]any{"stream_id": "strm_hint", "text": "你好"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	transcriber := NewHTTPTranscriber(server.URL+"/v1/asr/transcribe", 5*time.Second, "auto")
	sink := &recordingDeltaSink{}
	stream, err := transcriber.StartStream(context.Background(), TranscriptionRequest{
		SessionID:    "sess_test",
		TurnID:       "turn_hint",
		TraceID:      "trace_hint",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
		Language:     "zh",
	}, sink)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}
	if err := stream.PushAudio(context.Background(), []byte{0x01, 0x02}); err != nil {
		t.Fatalf("PushAudio #1 failed: %v", err)
	}
	if err := stream.PushAudio(context.Background(), []byte{0x03, 0x04}); err != nil {
		t.Fatalf("PushAudio #2 failed: %v", err)
	}
	if _, err := stream.Finish(context.Background()); err != nil {
		t.Fatalf("Finish failed: %v", err)
	}
	if len(sink.deltas) < 3 {
		t.Fatalf("unexpected deltas %+v", sink.deltas)
	}
	if sink.deltas[1].Kind != TranscriptionDeltaKindPartial || sink.deltas[1].Text != "ni" || sink.deltas[1].EndpointReason != "" {
		t.Fatalf("unexpected first preview delta %+v", sink.deltas[1])
	}
	if sink.deltas[2].Kind != TranscriptionDeltaKindPartial || sink.deltas[2].Text != "ni" || sink.deltas[2].EndpointReason != "preview_tail_silence" {
		t.Fatalf("unexpected second preview delta %+v", sink.deltas[2])
	}
}

func TestHTTPTranscriberStreamingCloseCallsWorkerCloseRoute(t *testing.T) {
	paths := make([]string, 0, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		switch r.URL.Path {
		case "/v1/asr/stream/start":
			_ = json.NewEncoder(w).Encode(map[string]any{"stream_id": "strm_close"})
		case "/v1/asr/stream/close":
			if got := payload["stream_id"]; got != "strm_close" {
				t.Fatalf("unexpected close stream id %v", got)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"status": "closed"})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	transcriber := NewHTTPTranscriber(server.URL+"/v1/asr/transcribe", 5*time.Second, "auto")
	stream, err := transcriber.StartStream(context.Background(), TranscriptionRequest{
		SessionID:    "sess_close",
		TurnID:       "turn_close",
		TraceID:      "trace_close",
		DeviceID:     "rtos-001",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	}, nil)
	if err != nil {
		t.Fatalf("StartStream failed: %v", err)
	}
	if err := stream.Close(); err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	if len(paths) != 2 || paths[1] != "/v1/asr/stream/close" {
		t.Fatalf("unexpected paths %+v", paths)
	}
}
