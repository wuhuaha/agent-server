package voice

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestIflytekRTASRTranscriber(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/rtasr" {
			http.NotFound(w, r)
			return
		}
		query := r.URL.Query()
		requiredKeys := []string{"accessKeyId", "appId", "audio_encode", "lang", "samplerate", "signature", "utc", "uuid"}
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

		if err := conn.WriteJSON(map[string]any{
			"data": map[string]any{
				"sessionId": "sess-cloud",
			},
		}); err != nil {
			t.Fatalf("write start failed: %v", err)
		}

		audioBytes := 0
		for {
			messageType, payload, err := conn.ReadMessage()
			if err != nil {
				t.Fatalf("read failed: %v", err)
			}
			switch messageType {
			case websocket.BinaryMessage:
				audioBytes += len(payload)
			case websocket.TextMessage:
				var endPayload map[string]any
				if err := json.Unmarshal(payload, &endPayload); err != nil {
					t.Fatalf("decode end payload failed: %v", err)
				}
				if endPayload["end"] != true {
					t.Fatalf("expected end=true, got %+v", endPayload)
				}
				if audioBytes == 0 {
					t.Fatal("expected audio bytes before end payload")
				}
				if err := conn.WriteJSON(map[string]any{
					"data": map[string]any{
						"seg_id": 0,
						"cn": map[string]any{
							"st": map[string]any{
								"rt": []any{
									map[string]any{
										"ws": []any{
											map[string]any{"cw": []any{map[string]any{"w": "小欧"}}},
										},
									},
								},
							},
						},
						"ls": false,
					},
				}); err != nil {
					t.Fatalf("write first result failed: %v", err)
				}
				if err := conn.WriteJSON(map[string]any{
					"data": map[string]any{
						"seg_id": 1,
						"cn": map[string]any{
							"st": map[string]any{
								"rt": []any{
									map[string]any{
										"ws": []any{
											map[string]any{"cw": []any{map[string]any{"w": "管家"}}},
											map[string]any{"cw": []any{map[string]any{"w": "。"}}},
										},
									},
								},
							},
						},
						"ls": true,
					},
				}); err != nil {
					t.Fatalf("write final result failed: %v", err)
				}
				return
			default:
				t.Fatalf("unexpected websocket message type %d", messageType)
			}
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

	transcriber := NewIflytekRTASRTranscriber(IflytekRTASRConfig{
		AppID:           "app-id",
		AccessKeyID:     "key-id",
		AccessKeySecret: "key-secret",
		Scheme:          "ws",
		Host:            host,
		Port:            port,
		Path:            "/rtasr",
		AudioEncode:     "pcm_s16le",
		Language:        "zh_cn",
		SampleRateHz:    16000,
		Timeout:         5 * time.Second,
		FrameBytes:      320,
		FrameInterval:   0,
	})

	result, err := transcriber.Transcribe(context.Background(), TranscriptionRequest{
		SessionID:    "sess-1",
		DeviceID:     "device-1",
		AudioPCM:     make([]byte, 640),
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("transcribe failed: %v", err)
	}
	if result.Text != "小欧管家。" {
		t.Fatalf("unexpected text %q", result.Text)
	}
	if result.Model != "iflytek_rtasr" {
		t.Fatalf("unexpected model %q", result.Model)
	}
	if result.Device != "cloud" {
		t.Fatalf("unexpected device %q", result.Device)
	}
	if result.Language != "zh_cn" {
		t.Fatalf("unexpected language %q", result.Language)
	}
	if result.EndpointReason != "last_segment" {
		t.Fatalf("unexpected endpoint reason %q", result.EndpointReason)
	}
	if len(result.Partials) != 2 || result.Partials[0] != "小欧" || result.Partials[1] != "管家。" {
		t.Fatalf("unexpected partials %+v", result.Partials)
	}
}
