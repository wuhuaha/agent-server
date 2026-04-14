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

func TestMimoTTSSynthesizer(t *testing.T) {
	var wavPayload []byte
	{
		var buf bytes.Buffer
		writeTestWAV(&buf, make([]byte, 24000*2), 24000, 1)
		wavPayload = buf.Bytes()
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "test-key" {
			t.Fatalf("missing api-key header")
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model": "mimo-v2-tts",
			"choices": []map[string]any{
				{
					"message": map[string]any{
						"audio": map[string]any{
							"data": base64.StdEncoding.EncodeToString(wavPayload),
						},
					},
				},
			},
		})
	}))
	defer server.Close()

	synthesizer := NewMimoTTSSynthesizer(
		"test-key",
		server.URL,
		"mimo-v2-tts",
		"mimo_default",
		"",
		5*time.Second,
		"pcm16le",
		16000,
		1,
	)
	result, err := synthesizer.Synthesize(context.Background(), SynthesisRequest{
		Text: "你好，测试语音。",
	})
	if err != nil {
		t.Fatalf("synthesize failed: %v", err)
	}
	if result.SampleRateHz != 16000 || result.Channels != 1 {
		t.Fatalf("unexpected output metadata %+v", result)
	}
	if len(result.AudioPCM) == 0 {
		t.Fatal("expected non-empty audio payload")
	}
}

func TestMimoTTSSynthesizerStreamSynthesize(t *testing.T) {
	firstHalf := bytes.Repeat([]byte{1, 0}, 160)
	secondHalf := bytes.Repeat([]byte{2, 0}, 160)
	combined := append(append([]byte(nil), firstHalf...), secondHalf...)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "test-key" {
			t.Fatalf("missing api-key header")
		}
		if got := r.Header.Get("Accept"); got != "text/event-stream" {
			t.Fatalf("expected text/event-stream accept header, got %q", got)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode failed: %v", err)
		}
		if got := payload["stream"]; got != true {
			t.Fatalf("expected stream=true, got %v", got)
		}
		audio, ok := payload["audio"].(map[string]any)
		if !ok {
			t.Fatalf("expected audio object in request")
		}
		if got := audio["format"]; got != "pcm16" {
			t.Fatalf("expected pcm16 audio format, got %v", got)
		}

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(w, "data: {\"choices\":[{\"delta\":{\"audio\":{\"data\":\""+base64.StdEncoding.EncodeToString(firstHalf)+"\"}}}]}\n\n")
		_, _ = io.WriteString(w, "data: {\"type\":\"response.output_audio.delta\",\"delta\":\""+base64.StdEncoding.EncodeToString(secondHalf)+"\"}\n\n")
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()

	synthesizer := NewMimoTTSSynthesizer(
		"test-key",
		server.URL,
		"mimo-v2-tts",
		"mimo_default",
		"",
		5*time.Second,
		"pcm16le",
		16000,
		1,
	)

	stream, err := synthesizer.StreamSynthesize(context.Background(), SynthesisRequest{Text: "流式语音测试"})
	if err != nil {
		t.Fatalf("StreamSynthesize failed: %v", err)
	}
	defer stream.Close()

	firstChunk, err := stream.Next(context.Background())
	if err != nil {
		t.Fatalf("first stream chunk failed: %v", err)
	}
	if !bytes.Equal(firstChunk, combined) {
		t.Fatalf("unexpected first stream chunk length/content: %d", len(firstChunk))
	}

	_, err = stream.Next(context.Background())
	if err != io.EOF {
		t.Fatalf("expected EOF after draining stream, got %v", err)
	}
}
