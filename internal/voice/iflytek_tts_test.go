//go:build integration

package voice

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestIflytekTTSSynthesizerStreamSynthesize(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	firstHalf := bytes.Repeat([]byte{1, 0}, 160)
	secondHalf := bytes.Repeat([]byte{2, 0}, 160)
	expected := append(append([]byte(nil), firstHalf...), secondHalf...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/tts" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		requiredKeys := []string{"host", "date", "authorization"}
		for _, key := range requiredKeys {
			if query.Get(key) == "" {
				t.Fatalf("expected query %q to be present", key)
			}
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			t.Fatalf("upgrade failed: %v", err)
		}
		defer conn.Close()

		_, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read request failed: %v", err)
		}
		var requestPayload map[string]any
		if err := json.Unmarshal(payload, &requestPayload); err != nil {
			t.Fatalf("decode request failed: %v", err)
		}
		common, _ := requestPayload["common"].(map[string]any)
		if common["app_id"] != "app-id" {
			t.Fatalf("unexpected app_id %v", common["app_id"])
		}

		if err := conn.WriteJSON(map[string]any{
			"code": 0,
			"sid":  "tts-session",
			"data": map[string]any{
				"audio":  base64.StdEncoding.EncodeToString(firstHalf),
				"status": 1,
			},
		}); err != nil {
			t.Fatalf("write first chunk failed: %v", err)
		}
		if err := conn.WriteJSON(map[string]any{
			"code": 0,
			"sid":  "tts-session",
			"data": map[string]any{
				"audio":  base64.StdEncoding.EncodeToString(secondHalf),
				"status": 2,
			},
		}); err != nil {
			t.Fatalf("write second chunk failed: %v", err)
		}
	}))
	defer server.Close()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse server url failed: %v", err)
	}
	host, portText, err := net.SplitHostPort(serverURL.Host)
	if err != nil {
		t.Fatalf("split host/port failed: %v", err)
	}
	port, err := net.LookupPort("tcp", portText)
	if err != nil {
		t.Fatalf("resolve port failed: %v", err)
	}

	synthesizer := NewIflytekTTSSynthesizer(IflytekTTSConfig{
		AppID:          "app-id",
		APIKey:         "api-key",
		APISecret:      "api-secret",
		Scheme:         "ws",
		Host:           host,
		Port:           port,
		Path:           "/v2/tts",
		Voice:          "xiaoyan",
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
