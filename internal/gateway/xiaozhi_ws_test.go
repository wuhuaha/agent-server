//go:build integration
// +build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"agent-server/internal/voice"

	"github.com/gorilla/websocket"
)

type passthroughXiaozhiEncoder struct{}

func (passthroughXiaozhiEncoder) EncodePCM16(_ context.Context, source voice.AudioStream, _ int, _ int, _ int, _ int, _ int) (voice.AudioStream, error) {
	return source, nil
}

type xiaozhiHelloTestResponse struct {
	Type      string             `json:"type"`
	Version   int                `json:"version"`
	Transport string             `json:"transport"`
	SessionID string             `json:"session_id"`
	Audio     xiaozhiAudioParams `json:"audio_params"`
}

type xiaozhiTextTestMessage struct {
	Type      string `json:"type"`
	SessionID string `json:"session_id"`
	State     string `json:"state,omitempty"`
	Text      string `json:"text,omitempty"`
}

func TestXiaozhiWSHelloDefaultsWithoutAudioParams(t *testing.T) {
	profile := testXiaozhiProfile()
	conn := openXiaozhiWS(t, profile, nil, nil)
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"type": "hello", "version": 1, "transport": "websocket"}); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}

	var hello xiaozhiHelloTestResponse
	readXiaozhiJSON(t, conn, 2*time.Second, &hello)
	if hello.Type != "hello" {
		t.Fatalf("expected hello response, got %q", hello.Type)
	}
	if hello.SessionID == "" {
		t.Fatal("expected session_id in hello response")
	}
	if hello.Audio.Format != profile.OutputCodec {
		t.Fatalf("expected output format %q, got %q", profile.OutputCodec, hello.Audio.Format)
	}
	if hello.Audio.SampleRate != profile.OutputSampleRate {
		t.Fatalf("expected output sample rate %d, got %d", profile.OutputSampleRate, hello.Audio.SampleRate)
	}
	if hello.Audio.Channels != profile.OutputChannels {
		t.Fatalf("expected output channels %d, got %d", profile.OutputChannels, hello.Audio.Channels)
	}
}

func TestXiaozhiWSProtocolV3BinaryFramesAreUnwrappedAndWrapped(t *testing.T) {
	profile := testXiaozhiProfile()
	profile.InputCodec = "pcm16le"

	inputFrame := []byte{0x00, 0x01, 0x02, 0x03}
	outputFrame := []byte{0x11, 0x22, 0x33, 0x44}
	var captured voice.TurnRequest
	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		captured = req
		return voice.TurnResponse{InputText: "打开客厅灯", Text: "compat reply", AudioChunks: [][]byte{outputFrame}}, nil
	})

	conn := openXiaozhiWS(t, profile, responder, http.Header{"Protocol-Version": []string{"3"}})
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type":      "hello",
		"version":   3,
		"transport": "websocket",
		"audio_params": map[string]any{
			"format":      "pcm16le",
			"sample_rate": 16000,
			"channels":    1,
		},
	}); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}
	var hello xiaozhiHelloTestResponse
	readXiaozhiJSON(t, conn, 2*time.Second, &hello)
	if hello.Version != 3 {
		t.Fatalf("expected hello version 3, got %d", hello.Version)
	}

	if err := conn.WriteJSON(map[string]any{"type": "listen", "session_id": hello.SessionID, "state": "start", "mode": "manual"}); err != nil {
		t.Fatalf("write listen.start failed: %v", err)
	}
	wrappedInput, err := wrapXiaozhiBinaryFrame(inputFrame, 3)
	if err != nil {
		t.Fatalf("wrap input frame failed: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, wrappedInput); err != nil {
		t.Fatalf("write binary frame failed: %v", err)
	}
	if err := conn.WriteJSON(map[string]any{"type": "listen", "session_id": hello.SessionID, "state": "stop"}); err != nil {
		t.Fatalf("write listen.stop failed: %v", err)
	}

	seenSentence := false
	seenSTT := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			decoded, err := unwrapXiaozhiBinaryFrame(payload, 3)
			if err != nil {
				t.Fatalf("unwrap server frame failed: %v", err)
			}
			if !bytes.Equal(decoded, outputFrame) {
				t.Fatalf("unexpected wrapped audio payload: %v", decoded)
			}
			break
		}
		var msg xiaozhiTextTestMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("decode json message failed: %v", err)
		}
		if msg.Type == "stt" {
			seenSTT = true
			if msg.Text != "打开客厅灯" {
				t.Fatalf("expected stt echo text %q, got %q", "打开客厅灯", msg.Text)
			}
		}
		if msg.Type == "tts" && msg.State == "sentence_start" {
			seenSentence = true
			if msg.Text != "compat reply" {
				t.Fatalf("expected sentence_start text %q, got %q", "compat reply", msg.Text)
			}
		}
	}

	if !seenSentence {
		t.Fatal("expected tts sentence_start before wrapped audio")
	}
	if !seenSTT {
		t.Fatal("expected stt echo for audio turn before audio reply")
	}
	if !bytes.Equal(captured.AudioPCM, inputFrame) {
		t.Fatalf("expected unwrapped input frame %v, got %v", inputFrame, captured.AudioPCM)
	}
	if captured.InputCodec != "pcm16le" {
		t.Fatalf("expected captured input codec pcm16le, got %q", captured.InputCodec)
	}
	if captured.TurnID == "" {
		t.Fatal("expected turn_id on xiaozhi responder request")
	}
	if captured.TraceID == "" {
		t.Fatal("expected trace_id on xiaozhi responder request")
	}
}

func TestXiaozhiWSIdleTimeoutClosesWithoutServerPanic(t *testing.T) {
	profile := testXiaozhiProfile()
	profile.IdleTimeoutMs = 120
	profile.MaxSessionMs = 2000

	conn, serverLog := openXiaozhiWSWithServerLog(t, profile, nil, nil)
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{"type": "hello", "version": 1, "transport": "websocket"}); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}

	var hello xiaozhiHelloTestResponse
	readXiaozhiJSON(t, conn, 2*time.Second, &hello)

	if err := conn.WriteJSON(map[string]any{"type": "listen", "session_id": hello.SessionID, "state": "start", "mode": "manual"}); err != nil {
		t.Fatalf("write listen.start failed: %v", err)
	}

	var msg xiaozhiTextTestMessage
	readXiaozhiJSON(t, conn, 2*time.Second, &msg)
	if msg.Type != "tts" || msg.State != "stop" {
		t.Fatalf("expected idle-timeout tts stop, got type=%q state=%q", msg.Type, msg.State)
	}
	if msg.SessionID != hello.SessionID {
		t.Fatalf("expected session_id %q, got %q", hello.SessionID, msg.SessionID)
	}

	waitForConnectionClose(t, conn, 2*time.Second)
	assertNoServerPanic(t, serverLog)
}

func TestXiaozhiWSServerEndpointPreviewAutoCommitsWithoutListenStop(t *testing.T) {
	profile := testXiaozhiProfile()
	profile.InputCodec = "pcm16le"
	profile.ServerEndpointEnabled = true
	profile.IdleTimeoutMs = 1500

	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			return voice.TurnResponse{InputText: "自动提交", Text: "compat auto endpoint"}, nil
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &timedInputPreviewSession{threshold: 60 * time.Millisecond}, nil
		},
	}

	conn := openXiaozhiWS(t, profile, responder, nil)
	defer conn.Close()

	if err := conn.WriteJSON(map[string]any{
		"type":      "hello",
		"version":   1,
		"transport": "websocket",
		"audio_params": map[string]any{
			"format":      "pcm16le",
			"sample_rate": 16000,
			"channels":    1,
		},
	}); err != nil {
		t.Fatalf("write hello failed: %v", err)
	}
	var hello xiaozhiHelloTestResponse
	readXiaozhiJSON(t, conn, 2*time.Second, &hello)

	if err := conn.WriteJSON(map[string]any{"type": "listen", "session_id": hello.SessionID, "state": "start", "mode": "manual"}); err != nil {
		t.Fatalf("write listen.start failed: %v", err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0x00, 0x01, 0x02, 0x03}); err != nil {
		t.Fatalf("write binary frame failed: %v", err)
	}

	sawSTT := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 500*time.Millisecond)
		if messageType == websocket.BinaryMessage {
			continue
		}
		var msg xiaozhiTextTestMessage
		if err := json.Unmarshal(payload, &msg); err != nil {
			t.Fatalf("decode xiaozhi json failed: %v", err)
		}
		if msg.Type == "stt" && msg.Text == "自动提交" {
			sawSTT = true
			continue
		}
		if msg.Type == "llm" {
			if !sawSTT {
				t.Fatal("expected stt echo before llm reply")
			}
			if msg.Text != "compat auto endpoint" {
				t.Fatalf("unexpected llm text %q", msg.Text)
			}
			return
		}
	}

	t.Fatal("expected compat auto-committed response without listen.stop")
}

func TestXiaozhiOTAHandlerReturnsCompatWebsocketURL(t *testing.T) {
	profile := testXiaozhiProfile()
	handler := NewXiaozhiOTAHandler(profile)

	req := httptest.NewRequest(http.MethodPost, profile.OTAPath, strings.NewReader(`{"application":{"version":"1.2.3"}}`))
	req.Host = "device.example.com"
	req.Header.Set("X-Forwarded-Proto", "https")
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)

	if res.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", res.Code)
	}

	var payload struct {
		Firmware struct {
			Version string `json:"version"`
		} `json:"firmware"`
		WebSocket struct {
			URL string `json:"url"`
		} `json:"websocket"`
	}
	if err := json.Unmarshal(res.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode ota response failed: %v", err)
	}
	if payload.Firmware.Version != "1.2.3" {
		t.Fatalf("expected firmware version 1.2.3, got %q", payload.Firmware.Version)
	}
	if payload.WebSocket.URL != "wss://device.example.com"+profile.WSPath {
		t.Fatalf("unexpected websocket url %q", payload.WebSocket.URL)
	}
}

func testXiaozhiProfile() XiaozhiCompatProfile {
	return XiaozhiCompatProfile{
		Enabled:               true,
		WSPath:                "/xiaozhi/v1/",
		OTAPath:               "/xiaozhi/ota/",
		WelcomeVersion:        1,
		WelcomeTransport:      "websocket",
		InputCodec:            "opus",
		InputSampleRate:       16000,
		InputChannels:         1,
		InputFrameDurationMs:  60,
		MaxFrameBytes:         4096,
		IdleTimeoutMs:         15000,
		MaxSessionMs:          300000,
		SourceOutputCodec:     "pcm16le",
		SourceOutputRate:      16000,
		SourceOutputChannels:  1,
		OutputCodec:           "opus",
		OutputSampleRate:      24000,
		OutputChannels:        1,
		OutputFrameDurationMs: 60,
	}
}

func openXiaozhiWS(t *testing.T, profile XiaozhiCompatProfile, responder voice.Responder, headers http.Header) *websocket.Conn {
	t.Helper()

	conn, _ := openXiaozhiWSWithServerLog(t, profile, responder, headers)
	return conn
}

func openXiaozhiWSWithServerLog(t *testing.T, profile XiaozhiCompatProfile, responder voice.Responder, headers http.Header) (*websocket.Conn, *bytes.Buffer) {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(profile.WSPath, newXiaozhiWSHandlerWithEncoder(profile, responder, passthroughXiaozhiEncoder{}))
	server, serverLog := newLoggedWSServer(t, mux)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + profile.WSPath
	dialer := websocket.Dialer{}
	conn, resp, err := dialer.Dial(wsURL, headers)
	if err != nil {
		t.Fatalf("xiaozhi websocket dial failed: %v", err)
	}
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	return conn, serverLog
}

func readXiaozhiJSON(t *testing.T, conn *websocket.Conn, timeout time.Duration, target any) {
	t.Helper()
	for {
		messageType, payload := readWSMessage(t, conn, timeout)
		if messageType == websocket.BinaryMessage {
			continue
		}
		if err := json.Unmarshal(payload, target); err != nil {
			t.Fatalf("decode xiaozhi json failed: %v", err)
		}
		return
	}
}
