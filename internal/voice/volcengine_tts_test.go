//go:build integration

package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestVolcengineTTSSynthesizerStreamSynthesize(t *testing.T) {
	firstHalf := bytes.Repeat([]byte{3, 0}, 160)
	secondHalf := bytes.Repeat([]byte{4, 0}, 160)
	expected := append(append([]byte(nil), firstHalf...), secondHalf...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v3/tts/unidirectional/sse" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("X-Api-App-Id") != "app-id" {
			t.Fatalf("unexpected app id header %q", r.Header.Get("X-Api-App-Id"))
		}
		if r.Header.Get("X-Api-Access-Key") != "access-token" {
			t.Fatalf("unexpected access token header %q", r.Header.Get("X-Api-Access-Key"))
		}
		if r.Header.Get("X-Api-Resource-Id") != "seed-tts-2.0" {
			t.Fatalf("unexpected resource id header %q", r.Header.Get("X-Api-Resource-Id"))
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		reqParams, _ := payload["req_params"].(map[string]any)
		if reqParams["text"] != "流式测试" {
			t.Fatalf("unexpected text %v", reqParams["text"])
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"data\":\""+base64.StdEncoding.EncodeToString(firstHalf)+"\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"data\":\""+base64.StdEncoding.EncodeToString(secondHalf)+"\"}\n\n")
		_, _ = io.WriteString(w, "data: {\"code\":20000000,\"message\":\"success\",\"usage\":{\"chars\":4}}\n\n")
	}))
	defer server.Close()

	synthesizer := NewVolcengineTTSSynthesizer(VolcengineTTSConfig{
		BaseURL:        server.URL,
		AccessToken:    "access-token",
		AppID:          "app-id",
		ResourceID:     "seed-tts-2.0",
		VoiceType:      "zh_female_vv_uranus_bigtts",
		TargetCodec:    "pcm16le",
		TargetRateHz:   16000,
		TargetChannels: 1,
		Timeout:        5 * time.Second,
	})

	stream, err := synthesizer.StreamSynthesize(context.Background(), SynthesisRequest{Text: "流式测试"})
	if err != nil {
		t.Fatalf("StreamSynthesize failed: %v", err)
	}
	defer stream.Close()

	firstChunk, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("first chunk failed: %v", err)
	}
	if !bytes.Equal(firstChunk, expected) {
		t.Fatalf("unexpected chunk content len=%d", len(firstChunk))
	}

	_, err = stream.Next(context.Background())
	if err != io.EOF {
		t.Fatalf("expected EOF after draining stream, got %v", err)
	}
}
