package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-server/internal/voice"

	"github.com/gorilla/websocket"
	"github.com/pion/opus/pkg/oggreader"
)

type responderFunc func(context.Context, voice.TurnRequest) (voice.TurnResponse, error)

func (f responderFunc) Respond(ctx context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
	return f(ctx, req)
}

type streamingResponderFunc func(context.Context, voice.TurnRequest, voice.ResponseDeltaSink) (voice.TurnResponse, error)

func (f streamingResponderFunc) Respond(ctx context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
	return f(ctx, req, nil)
}

func (f streamingResponderFunc) RespondStream(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponse, error) {
	return f(ctx, req, sink)
}

type previewResponder struct {
	respond      func(context.Context, voice.TurnRequest) (voice.TurnResponse, error)
	startPreview func(context.Context, voice.InputPreviewRequest) (voice.InputPreviewSession, error)
}

func (r previewResponder) Respond(ctx context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
	return r.respond(ctx, req)
}

func (r previewResponder) StartInputPreview(ctx context.Context, req voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
	return r.startPreview(ctx, req)
}

type timedInputPreviewSession struct {
	threshold   time.Duration
	lastAudioAt time.Time
	audioBytes  int
}

func (s *timedInputPreviewSession) PushAudio(_ context.Context, chunk []byte) (voice.InputPreview, error) {
	s.lastAudioAt = time.Now()
	s.audioBytes += len(chunk)
	return voice.InputPreview{
		PartialText:    "preview partial",
		AudioBytes:     s.audioBytes,
		SpeechStarted:  true,
		EndpointReason: "",
	}, nil
}

func (s *timedInputPreviewSession) Poll(now time.Time) voice.InputPreview {
	preview := voice.InputPreview{
		PartialText:   "preview partial",
		AudioBytes:    s.audioBytes,
		SpeechStarted: s.audioBytes > 0,
	}
	if s.audioBytes > 0 && now.Sub(s.lastAudioAt) >= s.threshold {
		preview.CommitSuggested = true
		preview.EndpointReason = "server_silence_timeout"
	}
	return preview
}

func (s *timedInputPreviewSession) Close() error {
	return nil
}

type testInboundEvent struct {
	Type      string         `json:"type"`
	SessionID string         `json:"session_id"`
	Payload   map[string]any `json:"payload"`
}

func TestRealtimeWSIdleTimeoutEndsSession(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 120
	profile.MaxSessionMs = 2000

	conn := openRealtimeWS(t, profile, nil)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)

	event := readNextJSONEvent(t, conn, 2*time.Second)
	if event.Type != "session.end" {
		t.Fatalf("expected session.end after idle timeout, got %s", event.Type)
	}
	if event.SessionID != sessionID {
		t.Fatalf("expected session_id %s, got %s", sessionID, event.SessionID)
	}
	if got := stringValue(event.Payload["reason"]); got != "idle_timeout" {
		t.Fatalf("expected idle_timeout reason, got %q", got)
	}
}

func TestRealtimeWSMaxDurationEndsSession(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 2000
	profile.MaxSessionMs = 120

	conn := openRealtimeWS(t, profile, nil)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)

	event := readNextJSONEvent(t, conn, 2*time.Second)
	if event.Type != "session.end" {
		t.Fatalf("expected session.end after max duration, got %s", event.Type)
	}
	if event.SessionID != sessionID {
		t.Fatalf("expected session_id %s, got %s", sessionID, event.SessionID)
	}
	if got := stringValue(event.Payload["reason"]); got != "max_duration" {
		t.Fatalf("expected max_duration reason, got %q", got)
	}
}

func TestRealtimeWSIdleTimeoutClosesWithoutServerPanic(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 120
	profile.MaxSessionMs = 2000

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, nil)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)

	event := readNextJSONEvent(t, conn, 2*time.Second)
	if event.Type != "session.end" {
		t.Fatalf("expected session.end after idle timeout, got %s", event.Type)
	}
	if event.SessionID != sessionID {
		t.Fatalf("expected session_id %s, got %s", sessionID, event.SessionID)
	}
	if got := stringValue(event.Payload["reason"]); got != "idle_timeout" {
		t.Fatalf("expected idle_timeout reason, got %q", got)
	}

	waitForConnectionClose(t, conn, 2*time.Second)
	assertNoServerPanic(t, serverLog)
}

func TestRealtimeWSStreamsTextAndToolDeltas(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		if strings.TrimSpace(req.Text) != "plan" {
			return voice.TurnResponse{Text: "unexpected"}, nil
		}
		return voice.TurnResponse{
			Text: "final answer",
			Deltas: []voice.ResponseDelta{
				{Kind: voice.ResponseDeltaKindText, Text: "Looking up your calendar."},
				{Kind: voice.ResponseDeltaKindToolCall, ToolCallID: "tool_1", ToolName: "calendar.lookup", ToolStatus: "started", ToolInput: `{"date":"2026-03-31"}`},
				{Kind: voice.ResponseDeltaKindToolResult, ToolCallID: "tool_1", ToolName: "calendar.lookup", ToolStatus: "completed", ToolOutput: `{"events":1}`},
				{Kind: voice.ResponseDeltaKindText, Text: "You have one event today."},
			},
		}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "plan"})

	chunks := make([]testInboundEvent, 0, 4)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "response.chunk" {
			chunks = append(chunks, event)
			if len(chunks) == 4 {
				break
			}
		}
	}

	if len(chunks) != 4 {
		t.Fatalf("expected 4 response.chunk events, got %d", len(chunks))
	}
	if got := stringValue(chunks[0].Payload["delta_type"]); got != "text" {
		t.Fatalf("expected first delta_type text, got %q", got)
	}
	if got := stringValue(chunks[0].Payload["text"]); got != "Looking up your calendar." {
		t.Fatalf("unexpected first text delta %q", got)
	}
	if got := stringValue(chunks[1].Payload["delta_type"]); got != "tool_call" {
		t.Fatalf("expected tool_call delta, got %q", got)
	}
	if got := stringValue(chunks[1].Payload["tool_name"]); got != "calendar.lookup" {
		t.Fatalf("unexpected tool_name %q", got)
	}
	if got := stringValue(chunks[2].Payload["delta_type"]); got != "tool_result" {
		t.Fatalf("expected tool_result delta, got %q", got)
	}
	if got := stringValue(chunks[2].Payload["tool_output"]); got != `{"events":1}` {
		t.Fatalf("unexpected tool_output %q", got)
	}
	if got := stringValue(chunks[3].Payload["text"]); got != "You have one event today." {
		t.Fatalf("unexpected final text delta %q", got)
	}
}

func TestRealtimeWSStreamingResponderFlushesDeltasBeforeReturn(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	release := make(chan struct{})
	responder := streamingResponderFunc(func(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponse, error) {
		if strings.TrimSpace(req.Text) != "plan" {
			return voice.TurnResponse{Text: "unexpected"}, nil
		}
		if sink != nil {
			if err := sink.EmitResponseDelta(ctx, voice.ResponseDelta{Kind: voice.ResponseDeltaKindText, Text: "Working on it."}); err != nil {
				return voice.TurnResponse{}, err
			}
		}
		<-release
		for _, delta := range []voice.ResponseDelta{
			{Kind: voice.ResponseDeltaKindToolCall, ToolCallID: "tool_1", ToolName: "calendar.lookup", ToolStatus: "started", ToolInput: `{"date":"2026-03-31"}`},
			{Kind: voice.ResponseDeltaKindToolResult, ToolCallID: "tool_1", ToolName: "calendar.lookup", ToolStatus: "completed", ToolOutput: `{"events":1}`},
			{Kind: voice.ResponseDeltaKindText, Text: "You have one event today."},
		} {
			if sink != nil {
				if err := sink.EmitResponseDelta(ctx, delta); err != nil {
					return voice.TurnResponse{}, err
				}
			}
		}
		return voice.TurnResponse{Text: "You have one event today."}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "plan"})

	startSeen := false
	firstChunkSeen := false
	firstChunkCount := 0
	for !firstChunkSeen {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "response.start" {
			startSeen = true
			continue
		}
		if event.Type == "response.chunk" {
			firstChunkSeen = true
			firstChunkCount++
			if !startSeen {
				t.Fatal("expected response.start before streamed response.chunk")
			}
			if got := stringValue(event.Payload["text"]); got != "Working on it." {
				t.Fatalf("unexpected first streamed text %q", got)
			}
		}
	}
	if firstChunkCount != 1 {
		t.Fatalf("expected exactly one streamed chunk before release, got %d", firstChunkCount)
	}
	close(release)

	chunks := []testInboundEvent{{Type: "response.chunk", Payload: map[string]any{"text": "Working on it."}}}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "response.chunk" {
			chunks = append(chunks, event)
			if len(chunks) == 4 {
				break
			}
		}
	}

	if len(chunks) != 4 {
		t.Fatalf("expected 4 streamed chunks, got %d", len(chunks))
	}
	if got := stringValue(chunks[1].Payload["delta_type"]); got != "tool_call" {
		t.Fatalf("expected streamed tool_call, got %q", got)
	}
	if got := stringValue(chunks[2].Payload["delta_type"]); got != "tool_result" {
		t.Fatalf("expected streamed tool_result, got %q", got)
	}
	if got := stringValue(chunks[3].Payload["text"]); got != "You have one event today." {
		t.Fatalf("unexpected streamed final text %q", got)
	}
}

func TestRealtimeWSTurnTraceMetadataFlowsThroughResponseStartAndResponder(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	var captured voice.TurnRequest
	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		captured = req
		return voice.TurnResponse{Text: "ok", AudioChunks: repeatAudioChunks(make([]byte, 640), 2)}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "hello"})

	var thinkingTurnID string
	var speakingTurnID string
	var activeTurnID string
	var responseTurnID string
	var responseTraceID string

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "session.update":
			state := stringValue(event.Payload["state"])
			turnID := stringValue(event.Payload["turn_id"])
			switch state {
			case "thinking":
				thinkingTurnID = turnID
			case "speaking":
				speakingTurnID = turnID
			case "active":
				if thinkingTurnID != "" {
					activeTurnID = turnID
				}
			}
		case "response.start":
			responseTurnID = stringValue(event.Payload["turn_id"])
			responseTraceID = stringValue(event.Payload["trace_id"])
		}
		if activeTurnID != "" && responseTurnID != "" && responseTraceID != "" {
			break
		}
	}

	if captured.TurnID == "" {
		t.Fatal("expected responder request turn_id to be set")
	}
	if captured.TraceID == "" {
		t.Fatal("expected responder request trace_id to be set")
	}
	if responseTurnID == "" {
		t.Fatal("expected response.start turn_id to be set")
	}
	if responseTraceID == "" {
		t.Fatal("expected response.start trace_id to be set")
	}
	if thinkingTurnID != responseTurnID {
		t.Fatalf("expected thinking turn_id %q to match response.start turn_id %q", thinkingTurnID, responseTurnID)
	}
	if speakingTurnID != responseTurnID {
		t.Fatalf("expected speaking turn_id %q to match response.start turn_id %q", speakingTurnID, responseTurnID)
	}
	if activeTurnID != responseTurnID {
		t.Fatalf("expected active turn_id %q to match response.start turn_id %q", activeTurnID, responseTurnID)
	}
	if captured.TurnID != responseTurnID {
		t.Fatalf("expected responder turn_id %q to match response.start turn_id %q", captured.TurnID, responseTurnID)
	}
	if captured.TraceID != responseTraceID {
		t.Fatalf("expected responder trace_id %q to match response.start trace_id %q", captured.TraceID, responseTraceID)
	}
}

func TestRealtimeWSResponderEndDirectiveClosesAfterAudio(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		if strings.TrimSpace(req.Text) != "bye" {
			return voice.TurnResponse{Text: "unexpected"}, nil
		}
		return voice.TurnResponse{Text: "goodbye", AudioChunks: repeatAudioChunks(make([]byte, 640), 2), EndSession: true, EndReason: "completed", EndMessage: "dialog finished"}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "bye"})

	binaryChunks := 0
	sawResponseChunk := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			binaryChunks++
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "response.chunk" && stringValue(event.Payload["text"]) == "goodbye" {
			sawResponseChunk = true
		}
		if event.Type == "session.end" {
			if got := stringValue(event.Payload["reason"]); got != "completed" {
				t.Fatalf("expected completed reason, got %q", got)
			}
			if !sawResponseChunk {
				t.Fatal("expected response.chunk before session.end")
			}
			if binaryChunks != 2 {
				t.Fatalf("expected 2 audio chunks before session.end, got %d", binaryChunks)
			}
			return
		}
	}

	t.Fatal("expected responder end directive to close the session")
}

func TestRealtimeWSBargeInInterruptsSpeaking(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	longChunk := make([]byte, 640)
	shortChunk := make([]byte, 640)
	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		switch {
		case strings.TrimSpace(req.Text) == "first":
			return voice.TurnResponse{Text: "first response", AudioChunks: repeatAudioChunks(longChunk, 20)}, nil
		case req.InputFrames > 0:
			return voice.TurnResponse{Text: "interrupt response", AudioChunks: repeatAudioChunks(shortChunk, 3)}, nil
		default:
			return voice.TurnResponse{Text: "fallback"}, nil
		}
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "first"})

	firstResponseAudioChunks := 0
	interrupted := false
	sawBargeInActive := false
	sawInterruptResponse := false

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			if !sawInterruptResponse {
				firstResponseAudioChunks++
			}
			if !interrupted {
				interrupted = true
				if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 640)); err != nil {
					t.Fatalf("write interrupt frame failed: %v", err)
				}
				writeControlEvent(t, conn, "audio.in.commit", sessionID, 3, map[string]any{"reason": "barge_in"})
			}
			continue
		}

		event := decodeJSONEvent(t, payload)
		if event.Type == "session.update" && interrupted && stringValue(event.Payload["state"]) == "active" {
			sawBargeInActive = true
		}
		if event.Type == "response.chunk" && stringValue(event.Payload["text"]) == "interrupt response" {
			sawInterruptResponse = true
			break
		}
	}

	if !interrupted {
		t.Fatal("expected to send an interrupt turn while the server was speaking")
	}
	if !sawBargeInActive {
		t.Fatal("expected session to return to active during barge-in")
	}
	if !sawInterruptResponse {
		t.Fatal("expected a second response after barge-in commit")
	}
	if firstResponseAudioChunks >= 20 {
		t.Fatalf("expected barge-in to stop the first audio stream early, got %d chunks", firstResponseAudioChunks)
	}
}

func TestRealtimeWSServerEndpointPreviewAutoCommitsWithoutClientCommit(t *testing.T) {
	profile := testRealtimeProfile()
	profile.ServerEndpointEnabled = true
	profile.IdleTimeoutMs = 1500
	profile.MaxSessionMs = 5000

	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			if req.InputFrames == 0 {
				return voice.TurnResponse{Text: "unexpected"}, nil
			}
			return voice.TurnResponse{InputText: "自动提交", Text: "auto endpoint response"}, nil
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &timedInputPreviewSession{threshold: 60 * time.Millisecond}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 1280)); err != nil {
		t.Fatalf("write binary frame failed: %v", err)
	}

	sawThinking := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 500*time.Millisecond)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "session.update":
			if event.SessionID != sessionID {
				t.Fatalf("unexpected session id %q", event.SessionID)
			}
			if stringValue(event.Payload["state"]) == "thinking" {
				sawThinking = true
			}
		case "response.chunk":
			if !sawThinking {
				t.Fatal("expected server endpoint auto-commit to enter thinking before response")
			}
			if got := stringValue(event.Payload["text"]); got != "auto endpoint response" {
				t.Fatalf("unexpected response text %q", got)
			}
			return
		}
	}

	t.Fatal("expected auto-committed response without explicit audio.in.commit")
}

func testRealtimeProfile() RealtimeProfile {
	return RealtimeProfile{WSPath: "/v1/realtime/ws", ProtocolVersion: "rtos-ws-v0", Subprotocol: "agent-server.realtime.v0", VoiceProvider: "bootstrap", TTSProvider: "none", AuthMode: "disabled", TurnMode: "client_wakeup_client_commit", IdleTimeoutMs: 15000, MaxSessionMs: 300000, MaxFrameBytes: 4096, InputCodec: "pcm16le", InputSampleRate: 16000, InputChannels: 1, OutputCodec: "pcm16le", OutputSampleRate: 16000, OutputChannels: 1, AllowOpus: false, AllowTextInput: true, AllowImageInput: false}
}

func openRealtimeWS(t *testing.T, profile RealtimeProfile, responder voice.Responder) *websocket.Conn {
	t.Helper()

	conn, _ := openRealtimeWSWithServerLog(t, profile, responder)
	return conn
}

func openRealtimeWSWithServerLog(t *testing.T, profile RealtimeProfile, responder voice.Responder) (*websocket.Conn, *bytes.Buffer) {
	t.Helper()

	mux := http.NewServeMux()
	mux.Handle(profile.WSPath, NewRealtimeWSHandler(profile, responder))
	server, serverLog := newLoggedWSServer(t, mux)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + profile.WSPath
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, http.Header{"Sec-WebSocket-Protocol": []string{profile.Subprotocol}})
	if err != nil {
		t.Fatalf("websocket dial failed: %v", err)
	}
	if resp != nil {
		t.Cleanup(func() { _ = resp.Body.Close() })
	}
	return conn, serverLog
}

func startTestSession(t *testing.T, conn *websocket.Conn, profile RealtimeProfile) string {
	t.Helper()

	sessionID := "sess_test"
	writeControlEvent(t, conn, "session.start", sessionID, 1, map[string]any{
		"protocol_version": profile.ProtocolVersion,
		"device":           map[string]any{"device_id": "rtos-mock-001", "client_type": "rtos-mock", "firmware_version": "test"},
		"audio":            map[string]any{"codec": profile.InputCodec, "sample_rate_hz": profile.InputSampleRate, "channels": profile.InputChannels},
		"session":          map[string]any{"mode": "voice", "wake_reason": "test", "client_can_end": true, "server_can_end": true},
		"capabilities":     map[string]any{"text_input": true, "image_input": false, "half_duplex": false, "local_wake_word": false},
	})

	event := readNextJSONEvent(t, conn, 2*time.Second)
	if event.Type != "session.update" {
		t.Fatalf("expected initial session.update, got %s", event.Type)
	}
	if got := stringValue(event.Payload["state"]); got != "active" {
		t.Fatalf("expected initial state active, got %q", got)
	}
	return event.SessionID
}

func writeControlEvent(t *testing.T, conn *websocket.Conn, eventType, sessionID string, seq int64, payload any) {
	t.Helper()
	envelope := map[string]any{"type": eventType, "seq": seq, "ts": time.Now().UTC().Format(time.RFC3339Nano)}
	if sessionID != "" {
		envelope["session_id"] = sessionID
	}
	if payload != nil {
		envelope["payload"] = payload
	}
	if err := conn.WriteJSON(envelope); err != nil {
		t.Fatalf("write %s failed: %v", eventType, err)
	}
}

func readNextJSONEvent(t *testing.T, conn *websocket.Conn, timeout time.Duration) testInboundEvent {
	t.Helper()
	for {
		messageType, payload := readWSMessage(t, conn, timeout)
		if messageType == websocket.BinaryMessage {
			continue
		}
		return decodeJSONEvent(t, payload)
	}
}

func readWSMessage(t *testing.T, conn *websocket.Conn, timeout time.Duration) (int, []byte) {
	t.Helper()
	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline failed: %v", err)
	}
	messageType, payload, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read message failed: %v", err)
	}
	return messageType, payload
}

func decodeJSONEvent(t *testing.T, payload []byte) testInboundEvent {
	t.Helper()
	var event testInboundEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatalf("decode json event failed: %v", err)
	}
	return event
}

func stringValue(value any) string {
	if rendered, ok := value.(string); ok {
		return rendered
	}
	return ""
}

func repeatAudioChunks(chunk []byte, count int) [][]byte {
	cloned := make([][]byte, 0, count)
	for idx := 0; idx < count; idx++ {
		cloned = append(cloned, append([]byte(nil), chunk...))
	}
	return cloned
}

func newLoggedWSServer(t *testing.T, handler http.Handler) (*httptest.Server, *bytes.Buffer) {
	t.Helper()

	serverLog := &bytes.Buffer{}
	server := httptest.NewUnstartedServer(handler)
	server.Config.ErrorLog = log.New(serverLog, "", 0)
	server.Start()
	t.Cleanup(server.Close)
	return server, serverLog
}

func waitForConnectionClose(t *testing.T, conn *websocket.Conn, timeout time.Duration) {
	t.Helper()

	if err := conn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		t.Fatalf("set read deadline for close wait failed: %v", err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("expected websocket read to fail after server-side close")
	}
}

func assertNoServerPanic(t *testing.T, serverLog *bytes.Buffer) {
	t.Helper()

	time.Sleep(100 * time.Millisecond)
	logs := serverLog.String()
	if strings.Contains(logs, "panic serving") || strings.Contains(logs, "repeated read on failed websocket connection") {
		t.Fatalf("unexpected server panic log:\n%s", logs)
	}
}

func TestRealtimeWSOpusInputIsNormalizedToPCM16(t *testing.T) {
	profile := testRealtimeProfile()
	profile.AllowOpus = true

	packet := loadGatewayTestOpusPacket(t)
	var captured voice.TurnRequest

	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		captured = req
		return voice.TurnResponse{Text: "ok"}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := "sess_opus"
	writeControlEvent(t, conn, "session.start", sessionID, 1, map[string]any{
		"protocol_version": profile.ProtocolVersion,
		"device":           map[string]any{"device_id": "rtos-opus-001", "client_type": "rtos", "firmware_version": "test"},
		"audio":            map[string]any{"codec": "opus", "sample_rate_hz": profile.InputSampleRate, "channels": profile.InputChannels},
		"session":          map[string]any{"mode": "voice", "wake_reason": "test", "client_can_end": true, "server_can_end": true},
		"capabilities":     map[string]any{"text_input": true, "image_input": false, "half_duplex": false, "local_wake_word": true},
	})

	event := readNextJSONEvent(t, conn, 2*time.Second)
	if event.Type != "session.update" {
		t.Fatalf("expected initial session.update, got %s", event.Type)
	}

	if err := conn.WriteMessage(websocket.BinaryMessage, packet); err != nil {
		t.Fatalf("write opus packet failed: %v", err)
	}
	writeControlEvent(t, conn, "audio.in.commit", sessionID, 2, map[string]any{"reason": "end_of_speech"})

	deadline := time.Now().Add(3 * time.Second)
	sawResponseChunk := false
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		t.Logf("opus test event: %s", event.Type)
		if event.Type == "response.chunk" {
			sawResponseChunk = true
			break
		}
		if event.Type == "error" || event.Type == "session.end" {
			break
		}
	}
	if !sawResponseChunk {
		t.Fatal("expected response.chunk for opus turn")
	}

	if captured.InputCodec != "pcm16le" {
		t.Fatalf("expected normalized input codec pcm16le, got %s", captured.InputCodec)
	}
	if captured.InputSampleRate != 16000 {
		t.Fatalf("expected normalized sample rate 16000, got %d", captured.InputSampleRate)
	}
	if captured.InputChannels != 1 {
		t.Fatalf("expected normalized channels 1, got %d", captured.InputChannels)
	}
	if len(captured.AudioPCM) == 0 {
		t.Fatal("expected decoded PCM audio on the responder request")
	}
	if captured.TurnID == "" {
		t.Fatal("expected turn_id on normalized responder request")
	}
	if captured.TraceID == "" {
		t.Fatal("expected trace_id on normalized responder request")
	}
}

func loadGatewayTestOpusPacket(t *testing.T) []byte {
	t.Helper()
	oggPath := filepath.Join("..", "..", "testdata", "opus-tiny.ogg")
	oggData, err := os.ReadFile(oggPath)
	if err != nil {
		t.Fatalf("read ogg testdata failed: %v", err)
	}

	reader, _, err := oggreader.NewWith(bytes.NewReader(oggData))
	if err != nil {
		t.Fatalf("oggreader init failed: %v", err)
	}

	for {
		segments, _, err := reader.ParseNextPage()
		if errors.Is(err, io.EOF) {
			t.Fatal("no opus packet found in ogg testdata")
		}
		if err != nil {
			t.Fatalf("ParseNextPage failed: %v", err)
		}
		if len(segments) == 0 || bytes.HasPrefix(segments[0], []byte("OpusTags")) {
			continue
		}
		return append([]byte(nil), segments[0]...)
	}
}
