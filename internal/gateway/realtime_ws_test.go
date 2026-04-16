//go:build integration
// +build integration

package gateway

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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

type fixedPartialPreviewSession struct {
	partial    string
	stable     string
	endpoint   string
	commit     bool
	audioBytes int
}

func (s *fixedPartialPreviewSession) PushAudio(_ context.Context, chunk []byte) (voice.InputPreview, error) {
	s.audioBytes += len(chunk)
	return voice.InputPreview{
		PartialText:     s.partial,
		StablePrefix:    s.stable,
		EndpointReason:  s.endpoint,
		AudioBytes:      s.audioBytes,
		CommitSuggested: s.commit,
		SpeechStarted:   s.audioBytes > 0,
	}, nil
}

func (s *fixedPartialPreviewSession) Poll(time.Time) voice.InputPreview {
	return voice.InputPreview{
		PartialText:     s.partial,
		StablePrefix:    s.stable,
		EndpointReason:  s.endpoint,
		AudioBytes:      s.audioBytes,
		CommitSuggested: s.commit,
		SpeechStarted:   s.audioBytes > 0,
	}
}

func (s *fixedPartialPreviewSession) Close() error {
	return nil
}

type finalizingTimedInputPreviewSession struct {
	timedInputPreviewSession
	result voice.TranscriptionResult
}

func (s *finalizingTimedInputPreviewSession) Finish(context.Context) (voice.TranscriptionResult, error) {
	return s.result, nil
}

type ctxBoundAudioStream struct {
	sourceCtx context.Context
	delay     time.Duration
	chunk     []byte
	emitted   bool
}

func (s *ctxBoundAudioStream) Next(ctx context.Context) ([]byte, error) {
	if s.emitted {
		return nil, io.EOF
	}

	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-s.sourceCtx.Done():
		return nil, s.sourceCtx.Err()
	case <-time.After(s.delay):
	}

	s.emitted = true
	return append([]byte(nil), s.chunk...), nil
}

func (s *ctxBoundAudioStream) Close() error {
	s.emitted = true
	return nil
}

type scriptedPlaybackSegment struct {
	text     string
	duration time.Duration
	isLast   bool
	chunks   [][]byte
}

type scriptedSegmentedAudioStream struct {
	segments     []scriptedPlaybackSegment
	segmentIndex int
	chunkIndex   int
	segmentReady bool
	closed       bool
}

func (s *scriptedSegmentedAudioStream) NextSegment(context.Context) (voice.PlaybackSegment, bool, error) {
	if s.closed || s.segmentIndex >= len(s.segments) {
		return voice.PlaybackSegment{}, false, io.EOF
	}
	if s.segmentReady {
		return voice.PlaybackSegment{}, false, nil
	}
	segment := s.segments[s.segmentIndex]
	s.segmentReady = true
	return voice.PlaybackSegment{
		Index:            s.segmentIndex + 1,
		Text:             segment.text,
		ExpectedDuration: segment.duration,
		IsLastSegment:    segment.isLast,
	}, true, nil
}

func (s *scriptedSegmentedAudioStream) Next(context.Context) ([]byte, error) {
	if s.closed || s.segmentIndex >= len(s.segments) {
		return nil, io.EOF
	}
	if !s.segmentReady {
		return nil, errors.New("segment metadata not requested")
	}
	segment := s.segments[s.segmentIndex]
	if s.chunkIndex >= len(segment.chunks) {
		s.segmentIndex++
		s.chunkIndex = 0
		s.segmentReady = false
		return nil, io.EOF
	}
	chunk := append([]byte(nil), segment.chunks[s.chunkIndex]...)
	s.chunkIndex++
	return chunk, nil
}

func (s *scriptedSegmentedAudioStream) Close() error {
	s.closed = true
	return nil
}

func (s *scriptedSegmentedAudioStream) PlaybackDuration(frameDuration time.Duration) time.Duration {
	var total time.Duration
	for _, segment := range s.segments {
		if segment.duration > 0 {
			total += segment.duration
			continue
		}
		if frameDuration > 0 {
			total += time.Duration(len(segment.chunks)) * frameDuration
		}
	}
	return total
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

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, responder)
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
	assertNoServerPanic(t, serverLog)
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

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, responder)
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
	assertNoServerPanic(t, serverLog)
}

func TestRealtimeWSBinaryBeforeSessionStartIsRecoverable(t *testing.T) {
	profile := testRealtimeProfile()
	conn := openRealtimeWS(t, profile, nil)
	defer conn.Close()

	if err := conn.WriteMessage(websocket.BinaryMessage, []byte{0x01, 0x02, 0x03, 0x04}); err != nil {
		t.Fatalf("write binary before session.start failed: %v", err)
	}

	event := readNextJSONEvent(t, conn, 2*time.Second)
	if event.Type != "error" {
		t.Fatalf("expected error event, got %s", event.Type)
	}
	if got := stringValue(event.Payload["code"]); got != "session_not_started" {
		t.Fatalf("expected session_not_started error, got %q", got)
	}
	if recoverable, ok := event.Payload["recoverable"].(bool); !ok || !recoverable {
		t.Fatalf("expected recoverable=true, got %+v", event.Payload)
	}

	sessionID := startTestSession(t, conn, profile)
	if sessionID == "" {
		t.Fatal("expected websocket to remain usable after recoverable error")
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

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, responder)
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
	assertNoServerPanic(t, serverLog)
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

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, responder)
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
			assertNoServerPanic(t, serverLog)
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

func TestRealtimeWSAdaptiveBargeInHoldsShortIncompletePreviewUntilCommit(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	longChunk := make([]byte, 640)
	shortChunk := make([]byte, 640)
	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			switch {
			case strings.TrimSpace(req.Text) == "first":
				return voice.TurnResponse{Text: "first response", AudioChunks: repeatAudioChunks(longChunk, 20)}, nil
			case req.InputFrames > 0:
				return voice.TurnResponse{Text: "interrupt response", AudioChunks: repeatAudioChunks(shortChunk, 3)}, nil
			default:
				return voice.TurnResponse{Text: "fallback"}, nil
			}
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &fixedPartialPreviewSession{partial: "嗯"}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "first"})

	interrupted := false
	commitSent := false
	continuedFirstAudio := 0
	sawEarlyActive := false
	sawEarlyInterruptResponse := false
	sawSecondResponse := false
	sawSpeakingPreview := false

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			switch {
			case !interrupted:
				interrupted = true
				if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 640)); err != nil {
					t.Fatalf("write short interrupt frame failed: %v", err)
				}
			case !commitSent:
				continuedFirstAudio++
				if continuedFirstAudio >= 3 {
					writeControlEvent(t, conn, "audio.in.commit", sessionID, 3, map[string]any{"reason": "barge_in"})
					commitSent = true
				}
			}
			continue
		}

		event := decodeJSONEvent(t, payload)
		if interrupted && !commitSent && event.Type == "session.update" && stringValue(event.Payload["state"]) == "speaking" {
			if got := stringValue(event.Payload["input_state"]); got != "previewing" {
				t.Fatalf("expected speaking preview input_state, got %q", got)
			}
			if got := stringValue(event.Payload["output_state"]); got != "speaking" {
				t.Fatalf("expected speaking preview output_state, got %q", got)
			}
			sawSpeakingPreview = true
		}
		if !commitSent && event.Type == "session.update" && stringValue(event.Payload["state"]) == "active" {
			sawEarlyActive = true
		}
		if !commitSent && event.Type == "response.chunk" && stringValue(event.Payload["text"]) == "interrupt response" {
			sawEarlyInterruptResponse = true
		}
		if commitSent && event.Type == "response.chunk" && stringValue(event.Payload["text"]) == "interrupt response" {
			sawSecondResponse = true
			break
		}
	}

	if !interrupted {
		t.Fatal("expected to send a short interrupt frame while the server was speaking")
	}
	if continuedFirstAudio < 3 {
		t.Fatalf("expected the first response audio to continue before commit, got %d extra chunks", continuedFirstAudio)
	}
	if !sawSpeakingPreview {
		t.Fatal("expected speaking-time preview update before explicit commit")
	}
	if sawEarlyActive {
		t.Fatal("expected short incomplete preview to stay in speaking state until commit")
	}
	if sawEarlyInterruptResponse {
		t.Fatal("expected short incomplete preview not to trigger the second response before commit")
	}
	if !sawSecondResponse {
		t.Fatal("expected second response after explicit audio.in.commit")
	}
}

func TestRealtimeWSBackchannelDucksButDoesNotInterruptSpeaking(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	baseChunk := pcm16ConstantChunk(2400, 320)
	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			switch {
			case strings.TrimSpace(req.Text) == "first":
				return voice.TurnResponse{Text: "first response", AudioChunks: repeatAudioChunks(baseChunk, 20)}, nil
			default:
				return voice.TurnResponse{Text: "fallback"}, nil
			}
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &fixedPartialPreviewSession{partial: "好的"}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "first"})

	firstAmplitude := 0
	duckedAmplitude := 0
	postBargeChunks := 0
	sentBackchannel := false
	sawSpeakingPreview := false
	sawEarlyActive := false

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			amplitude := maxPCM16Abs(payload)
			if amplitude <= 0 {
				continue
			}
			if firstAmplitude == 0 {
				firstAmplitude = amplitude
				sentBackchannel = true
				if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 2560)); err != nil {
					t.Fatalf("write backchannel frame failed: %v", err)
				}
				continue
			}
			if sentBackchannel {
				postBargeChunks++
				if amplitude < firstAmplitude {
					duckedAmplitude = amplitude
				}
				if duckedAmplitude > 0 && postBargeChunks >= 3 {
					break
				}
			}
			continue
		}

		event := decodeJSONEvent(t, payload)
		if !sentBackchannel || event.Type != "session.update" {
			continue
		}
		if stringValue(event.Payload["state"]) == "active" {
			sawEarlyActive = true
		}
		if stringValue(event.Payload["state"]) == "speaking" &&
			stringValue(event.Payload["input_state"]) == "previewing" &&
			stringValue(event.Payload["output_state"]) == "speaking" {
			sawSpeakingPreview = true
		}
	}

	if firstAmplitude <= 0 {
		t.Fatal("expected initial speaking audio before backchannel")
	}
	if !sentBackchannel {
		t.Fatal("expected to send a speaking-time backchannel frame")
	}
	if postBargeChunks < 3 {
		t.Fatalf("expected speaking audio to continue after backchannel, got %d chunks", postBargeChunks)
	}
	if duckedAmplitude <= 0 || duckedAmplitude >= firstAmplitude {
		t.Fatalf("expected ducked amplitude below %d, got %d", firstAmplitude, duckedAmplitude)
	}
	if !sawSpeakingPreview {
		t.Fatal("expected speaking preview session.update during soft backchannel")
	}
	if sawEarlyActive {
		t.Fatal("expected backchannel to stay on speaking path instead of returning active")
	}
}

func TestRealtimeWSBargeInPreviewSurvivesNaturalOutputCompletion(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			switch {
			case strings.TrimSpace(req.Text) == "first":
				return voice.TurnResponse{Text: "first response", AudioChunks: repeatAudioChunks(make([]byte, 640), 6)}, nil
			case req.InputFrames > 0:
				return voice.TurnResponse{Text: "interrupt response", AudioChunks: repeatAudioChunks(make([]byte, 640), 2)}, nil
			default:
				return voice.TurnResponse{Text: "fallback"}, nil
			}
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &fixedPartialPreviewSession{partial: "嗯"}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "first"})

	interruptSent := false
	commitSent := false
	sawPreservedPreview := false
	sawInterruptResponse := false

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			if !interruptSent {
				interruptSent = true
				if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 640)); err != nil {
					t.Fatalf("write overlap preview frame failed: %v", err)
				}
			}
			continue
		}

		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "session.update":
			if interruptSent && !commitSent && stringValue(event.Payload["state"]) == "active" {
				if got := stringValue(event.Payload["input_state"]); got != "previewing" {
					t.Fatalf("expected preview to survive into active state, got %q", got)
				}
				if got := stringValue(event.Payload["output_state"]); got != "idle" {
					t.Fatalf("expected output to return idle after completion, got %q", got)
				}
				sawPreservedPreview = true
				writeControlEvent(t, conn, "audio.in.commit", sessionID, 3, map[string]any{"reason": "barge_in_after_completion"})
				commitSent = true
			}
		case "response.chunk":
			switch stringValue(event.Payload["text"]) {
			case "interrupt response":
				sawInterruptResponse = true
				goto done
			case "fallback":
				t.Fatal("expected preserved overlap audio to remain available after playback completion")
			}
		}
	}

done:
	if !interruptSent {
		t.Fatal("expected to preview overlap audio while the first response was speaking")
	}
	if !sawPreservedPreview {
		t.Fatal("expected previewing input to survive natural playback completion")
	}
	if !sawInterruptResponse {
		t.Fatal("expected preserved overlap audio to drive the next committed turn")
	}
}

func TestRealtimeWSTextInputDuringSpeakingCarriesAcceptReason(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		switch strings.TrimSpace(req.Text) {
		case "first":
			return voice.TurnResponse{Text: "first response", AudioChunks: repeatAudioChunks(make([]byte, 640), 10)}, nil
		case "second":
			return voice.TurnResponse{Text: "second response"}, nil
		default:
			return voice.TurnResponse{Text: "fallback"}, nil
		}
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "first"})

	secondSent := false
	sawTextAccept := false
	sawSecondResponse := false

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			if !secondSent {
				secondSent = true
				writeControlEvent(t, conn, "text.in", sessionID, 3, map[string]any{"text": "second"})
			}
			continue
		}

		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "session.update":
			if secondSent && stringValue(event.Payload["state"]) == "thinking" && stringValue(event.Payload["accept_reason"]) == "text_input" {
				if got := stringValue(event.Payload["input_state"]); got != "committed" {
					t.Fatalf("expected text input accept to commit the input lane, got %q", got)
				}
				if got := stringValue(event.Payload["output_state"]); got != "thinking" {
					t.Fatalf("expected text input accept to switch output lane to thinking, got %q", got)
				}
				sawTextAccept = true
			}
		case "response.chunk":
			if stringValue(event.Payload["text"]) == "second response" {
				sawSecondResponse = true
				goto textDone
			}
		}
	}

textDone:
	if !secondSent {
		t.Fatal("expected to inject a second text turn while the first response was speaking")
	}
	if !sawTextAccept {
		t.Fatal("expected thinking session.update with accept_reason=text_input for the second turn")
	}
	if !sawSecondResponse {
		t.Fatal("expected second text turn to complete after interrupting speaking")
	}
}

func TestRealtimeWSAudioCommitKeepsReturnedAudioStreamAliveAfterResponderReturns(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	responder := streamingResponderFunc(func(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponse, error) {
		if sink != nil {
			if err := sink.EmitResponseDelta(ctx, voice.ResponseDelta{
				Kind: voice.ResponseDeltaKindText,
				Text: "streamed audio response",
			}); err != nil {
				return voice.TurnResponse{}, err
			}
		}
		return voice.TurnResponse{
			InputText: "语音输入",
			Text:      "streamed audio response",
			AudioStream: &ctxBoundAudioStream{
				sourceCtx: ctx,
				delay:     40 * time.Millisecond,
				chunk:     make([]byte, 640),
			},
		}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 1280)); err != nil {
		t.Fatalf("write audio frame failed: %v", err)
	}
	writeControlEvent(t, conn, "audio.in.commit", sessionID, 2, map[string]any{"reason": "end_of_speech"})

	sawSpeaking := false
	sawAudio := false
	sawActive := false

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			sawAudio = true
			continue
		}

		event := decodeJSONEvent(t, payload)
		if event.Type != "session.update" {
			continue
		}

		switch stringValue(event.Payload["state"]) {
		case "speaking":
			sawSpeaking = true
		case "active":
			if sawAudio {
				sawActive = true
				goto done
			}
		}
	}

done:
	if !sawSpeaking {
		t.Fatal("expected audio commit turn to enter speaking")
	}
	if !sawAudio {
		t.Fatal("expected returned audio stream to stay alive long enough to emit audio")
	}
	if !sawActive {
		t.Fatal("expected session to return to active after audio playback completed")
	}
}

func TestRealtimeWSLogsPreviewAndFirstOutputMilestones(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000
	profile.ServerEndpointEnabled = true
	var logBuffer bytes.Buffer
	profile.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))

	responder := previewResponder{
		respond: func(_ context.Context, _ voice.TurnRequest) (voice.TurnResponse, error) {
			return voice.TurnResponse{
				Text:        "好的，已经收到。",
				AudioChunks: repeatAudioChunks(make([]byte, 640), 2),
			}, nil
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &timedInputPreviewSession{threshold: 60 * time.Millisecond}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	_ = startTestSession(t, conn, profile)
	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 1280)); err != nil {
		t.Fatalf("write audio frame failed: %v", err)
	}

	sawActive := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "session.update" && stringValue(event.Payload["state"]) == "active" {
			sawActive = true
			break
		}
	}
	if !sawActive {
		t.Fatal("expected session to return to active after auto-committed preview turn")
	}

	logs := logBuffer.String()
	for _, want := range []string{
		`"msg":"gateway input preview updated"`,
		`"msg":"gateway input preview commit suggested"`,
		`"msg":"gateway turn accepted"`,
		`"msg":"gateway response.start sent"`,
		`"msg":"gateway speaking update sent"`,
		`"msg":"gateway turn first text delta"`,
		`"msg":"gateway turn first audio chunk"`,
		`"preview_id":"preview_`,
		`"preview_first_partial_latency_ms":`,
		`"preview_commit_suggest_latency_ms":`,
		`"first_text_delta_latency_ms":`,
		`"first_audio_chunk_latency_ms":`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %s, got:\n%s", want, logs)
		}
	}
}

func TestRealtimeWSLogsAcceptedBargeIn(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000
	profile.BargeInMinAudioMs = 20
	profile.BargeInHoldAudioMs = 40
	var logBuffer bytes.Buffer
	profile.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))

	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			switch {
			case strings.TrimSpace(req.Text) == "first":
				return voice.TurnResponse{Text: "first response", AudioChunks: repeatAudioChunks(make([]byte, 640), 20)}, nil
			case req.InputFrames > 0:
				return voice.TurnResponse{Text: "interrupt response", AudioChunks: repeatAudioChunks(make([]byte, 640), 2)}, nil
			default:
				return voice.TurnResponse{Text: "fallback"}, nil
			}
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &fixedPartialPreviewSession{partial: "打断一下"}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "first"})

	interrupted := false
	sawInterruptResponse := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 2*time.Second)
		if messageType == websocket.BinaryMessage {
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
		if event.Type == "response.chunk" && stringValue(event.Payload["text"]) == "interrupt response" {
			sawInterruptResponse = true
			break
		}
	}
	if !interrupted {
		t.Fatal("expected to send an interrupt frame during speaking")
	}
	if !sawInterruptResponse {
		t.Fatal("expected interrupt response after accepted barge-in")
	}

	logs := logBuffer.String()
	for _, want := range []string{
		`"msg":"gateway barge-in preview updated"`,
		`"msg":"gateway barge-in accepted"`,
		`"barge_in_reason":"accepted_takeover_lexicon"`,
		`"barge_in_acoustic_ready":true`,
		`"barge_in_semantic_ready":true`,
		`"barge_in_takeover_lexicon":true`,
		`"preview_id":"preview_`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %s, got:\n%s", want, logs)
		}
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
				if got := stringValue(event.Payload["input_state"]); got != "committed" {
					t.Fatalf("expected server endpoint auto-commit input_state committed, got %q", got)
				}
				if got := stringValue(event.Payload["output_state"]); got != "thinking" {
					t.Fatalf("expected server endpoint auto-commit output_state thinking, got %q", got)
				}
				if got := stringValue(event.Payload["accept_reason"]); got != "server_endpoint" {
					t.Fatalf("expected server endpoint accept_reason, got %q", got)
				}
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

func TestRealtimeWSAudioCommitCarriesFinalizedPreviewTranscription(t *testing.T) {
	profile := testRealtimeProfile()
	profile.ServerEndpointEnabled = true
	profile.IdleTimeoutMs = 1500
	profile.MaxSessionMs = 5000

	var captured voice.TurnRequest
	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			captured = req
			return voice.TurnResponse{InputText: "快路径转写", Text: "ok"}, nil
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &finalizingTimedInputPreviewSession{
				timedInputPreviewSession: timedInputPreviewSession{threshold: 10 * time.Second},
				result: voice.TranscriptionResult{
					Text:           "快路径转写",
					EndpointReason: "preview_finish",
					Mode:           "preview_finalize_fast_path",
				},
			}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSession(t, conn, profile)
	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 1280)); err != nil {
		t.Fatalf("write binary frame failed: %v", err)
	}
	writeControlEvent(t, conn, "audio.in.commit", sessionID, 2, map[string]any{"reason": "end_of_speech"})

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 500*time.Millisecond)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "response.chunk" {
			break
		}
	}

	if captured.PreviewTranscription == nil {
		t.Fatalf("expected preview transcription on committed request, got %+v", captured)
	}
	if got := captured.PreviewTranscription.Text; got != "快路径转写" {
		t.Fatalf("expected preview transcription text, got %q", got)
	}
	if got := captured.PreviewTranscription.Mode; got != "preview_finalize_fast_path" {
		t.Fatalf("expected preview fast-path mode, got %q", got)
	}
}

func TestRealtimeWSServerEndpointAutoCommitCarriesFinalizedPreviewTranscription(t *testing.T) {
	profile := testRealtimeProfile()
	profile.ServerEndpointEnabled = true
	profile.IdleTimeoutMs = 1500
	profile.MaxSessionMs = 5000

	var captured voice.TurnRequest
	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			captured = req
			return voice.TurnResponse{InputText: "自动提交快路径", Text: "auto endpoint response"}, nil
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &finalizingTimedInputPreviewSession{
				timedInputPreviewSession: timedInputPreviewSession{threshold: 60 * time.Millisecond},
				result: voice.TranscriptionResult{
					Text:           "自动提交快路径",
					EndpointReason: "server_endpoint_preview_finish",
					Mode:           "preview_finalize_fast_path",
				},
			}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	_ = startTestSession(t, conn, profile)
	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 1280)); err != nil {
		t.Fatalf("write binary frame failed: %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 500*time.Millisecond)
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "response.chunk" {
			break
		}
	}

	if captured.PreviewTranscription == nil {
		t.Fatalf("expected preview transcription on auto-committed request, got %+v", captured)
	}
	if got := captured.PreviewTranscription.Text; got != "自动提交快路径" {
		t.Fatalf("expected auto-commit preview transcription text, got %q", got)
	}
}

func TestRealtimeWSServerEndpointPreviewKeepsConnectionOpenForClientEnd(t *testing.T) {
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

	sawResponse := false
	sentClientEnd := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 500*time.Millisecond)
		if messageType == websocket.BinaryMessage {
			continue
		}

		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "response.chunk":
			if stringValue(event.Payload["text"]) == "auto endpoint response" {
				sawResponse = true
			}
		case "session.update":
			if sawResponse && !sentClientEnd && stringValue(event.Payload["state"]) == "active" {
				writeControlEvent(t, conn, "session.end", sessionID, 3, map[string]any{
					"reason":  "client_stop",
					"message": "preview scenario complete",
				})
				sentClientEnd = true
			}
		case "session.end":
			if !sawResponse {
				t.Fatal("expected auto-committed response before client session.end")
			}
			if !sentClientEnd {
				t.Fatal("expected client to end the session after auto-commit returned to active")
			}
			if got := stringValue(event.Payload["reason"]); got != "client_stop" {
				t.Fatalf("expected client_stop reason, got %q", got)
			}
			return
		}
	}

	t.Fatal("expected connection to stay open until client session.end")
}

func TestRealtimeWSEmitsNegotiatedPreviewEvents(t *testing.T) {
	profile := testRealtimeProfile()
	profile.ServerEndpointEnabled = true
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			return voice.TurnResponse{InputText: req.Text, Text: "ok"}, nil
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &fixedPartialPreviewSession{partial: "打开客厅灯", stable: "打开客厅"}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	_ = startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
		"preview_events":  true,
	})
	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 1280)); err != nil {
		t.Fatalf("write binary frame failed: %v", err)
	}

	var (
		sawSpeechStart bool
		sawPreview     bool
		previewID      string
	)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		event := readNextJSONEvent(t, conn, 500*time.Millisecond)
		switch event.Type {
		case "input.speech.start":
			sawSpeechStart = true
			previewID = stringValue(event.Payload["preview_id"])
			if previewID == "" {
				t.Fatal("expected preview_id on input.speech.start")
			}
			if got := intValue(event.Payload["audio_offset_ms"]); got <= 0 {
				t.Fatalf("expected positive audio_offset_ms on input.speech.start, got %d", got)
			}
		case "input.preview":
			sawPreview = true
			if got := stringValue(event.Payload["text"]); got != "打开客厅灯" {
				t.Fatalf("expected preview text 打开客厅灯, got %q", got)
			}
			if got := stringValue(event.Payload["stable_prefix"]); got != "打开客厅" {
				t.Fatalf("expected preview stable_prefix 打开客厅, got %q", got)
			}
			if got := floatValue(event.Payload["stability"]); got < 0.79 || got > 0.81 {
				t.Fatalf("expected preview stability about 0.8, got %v", got)
			}
			if got := stringValue(event.Payload["preview_id"]); got == "" {
				t.Fatal("expected preview_id on input.preview")
			} else if previewID != "" && got != previewID {
				t.Fatalf("expected matching preview_id %q, got %q", previewID, got)
			}
			if got := intValue(event.Payload["audio_offset_ms"]); got <= 0 {
				t.Fatalf("expected positive audio_offset_ms on input.preview, got %d", got)
			}
		case "error":
			t.Fatalf("unexpected error event: %#v", event.Payload)
		}
		if sawSpeechStart && sawPreview {
			return
		}
	}

	t.Fatalf("expected negotiated preview events, saw speech_start=%v preview=%v", sawSpeechStart, sawPreview)
}

func TestRealtimeWSEmitsPreviewEndpointCandidateBeforeAcceptedTurn(t *testing.T) {
	profile := testRealtimeProfile()
	profile.ServerEndpointEnabled = true
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	responder := previewResponder{
		respond: func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
			return voice.TurnResponse{InputText: req.Text, Text: "ok"}, nil
		},
		startPreview: func(_ context.Context, _ voice.InputPreviewRequest) (voice.InputPreviewSession, error) {
			return &fixedPartialPreviewSession{
				partial:  "打开客厅灯",
				stable:   "打开客厅灯",
				endpoint: "preview_tail_silence",
			}, nil
		},
	}

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	_ = startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
		"preview_events":  true,
	})
	if err := conn.WriteMessage(websocket.BinaryMessage, make([]byte, 1280)); err != nil {
		t.Fatalf("write binary frame failed: %v", err)
	}

	var (
		sawEndpoint bool
		sawAccept   bool
	)
	deadline := time.Now().Add(1500 * time.Millisecond)
	for time.Now().Before(deadline) {
		event := readNextJSONEvent(t, conn, 400*time.Millisecond)
		switch event.Type {
		case "input.endpoint":
			sawEndpoint = true
			if got := stringValue(event.Payload["reason"]); got != "preview_tail_silence" {
				t.Fatalf("expected preview endpoint reason preview_tail_silence, got %q", got)
			}
		case "session.update":
			if got := stringValue(event.Payload["accept_reason"]); got != "" {
				sawAccept = true
			}
		}
		if sawEndpoint {
			break
		}
	}

	if !sawEndpoint {
		t.Fatal("expected input.endpoint before any accepted-turn signal")
	}
	if sawAccept {
		t.Fatal("did not expect accepted-turn signal when preview only raised endpoint candidate")
	}
}

func TestRealtimeWSNegotiatedPlaybackAckMetaAndClientFacts(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000
	var logBuffer bytes.Buffer
	profile.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))

	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		if strings.TrimSpace(req.Text) != "play" {
			return voice.TurnResponse{Text: "unexpected"}, nil
		}
		return voice.TurnResponse{
			Text:        "好的，开始播放。",
			AudioChunks: repeatAudioChunks(make([]byte, 640), 3),
		}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
		"playback_ack": map[string]any{
			"mode": "segment_mark_v1",
		},
	})
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "play"})

	var meta audioOutMetaPayload
	sawMeta := false
	sawActive := false
	audioChunks := 0
	sentCompleted := false
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 500*time.Millisecond)
		if messageType == websocket.BinaryMessage {
			audioChunks++
			if sawMeta && !sentCompleted && audioChunks >= 3 {
				writeControlEvent(t, conn, "audio.out.completed", sessionID, 5, map[string]any{
					"response_id": meta.ResponseID,
					"playback_id": meta.PlaybackID,
				})
				sentCompleted = true
			}
			continue
		}
		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "audio.out.meta":
			sawMeta = true
			meta = audioOutMetaPayload{
				ResponseID:         stringValue(event.Payload["response_id"]),
				PlaybackID:         stringValue(event.Payload["playback_id"]),
				SegmentID:          stringValue(event.Payload["segment_id"]),
				Text:               stringValue(event.Payload["text"]),
				ExpectedDurationMs: intValue(event.Payload["expected_duration_ms"]),
			}
			if meta.ResponseID == "" || meta.PlaybackID == "" || meta.SegmentID == "" {
				t.Fatalf("expected populated audio.out.meta ids, got %#v", event.Payload)
			}
			if meta.ExpectedDurationMs <= 0 {
				t.Fatalf("expected positive expected_duration_ms, got %#v", event.Payload)
			}
			writeControlEvent(t, conn, "audio.out.started", sessionID, 3, map[string]any{
				"response_id": meta.ResponseID,
				"playback_id": meta.PlaybackID,
				"segment_id":  meta.SegmentID,
			})
			writeControlEvent(t, conn, "audio.out.mark", sessionID, 4, map[string]any{
				"response_id":        meta.ResponseID,
				"playback_id":        meta.PlaybackID,
				"segment_id":         meta.SegmentID,
				"played_duration_ms": 40,
			})
		case "session.update":
			if sentCompleted && stringValue(event.Payload["state"]) == "active" {
				sawActive = true
				goto done
			}
		case "error":
			t.Fatalf("unexpected error event: %#v", event.Payload)
		}
	}

done:
	if !sawMeta {
		t.Fatal("expected audio.out.meta when playback_ack is negotiated")
	}
	if !sawActive {
		t.Fatal("expected session to return to active after playback")
	}

	time.Sleep(100 * time.Millisecond)
	logs := logBuffer.String()
	for _, want := range []string{
		`"msg":"gateway playback ack started"`,
		`"msg":"gateway playback ack mark"`,
		`"msg":"gateway playback ack completed"`,
		`"response_id":"` + meta.ResponseID + `"`,
		`"playback_id":"` + meta.PlaybackID + `"`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %s, got:\n%s", want, logs)
		}
	}
}

func TestRealtimeWSPlaybackAckClearedInterruptsCurrentOutput(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000
	var logBuffer bytes.Buffer
	profile.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))

	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		if strings.TrimSpace(req.Text) != "play" {
			return voice.TurnResponse{Text: "unexpected"}, nil
		}
		return voice.TurnResponse{
			Text:        "好的，开始播放一段比较长的响应。",
			AudioChunks: repeatAudioChunks(make([]byte, 640), 20),
		}, nil
	})

	conn := openRealtimeWS(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
		"playback_ack": map[string]any{
			"mode": "segment_mark_v1",
		},
	})
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "play"})

	var (
		meta        audioOutMetaPayload
		sawMeta     bool
		sawActive   bool
		audioChunks int
	)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		messageType, payload := readWSMessage(t, conn, 500*time.Millisecond)
		if messageType == websocket.BinaryMessage {
			audioChunks++
			if sawMeta && audioChunks == 1 {
				writeControlEvent(t, conn, "audio.out.started", sessionID, 3, map[string]any{
					"response_id": meta.ResponseID,
					"playback_id": meta.PlaybackID,
					"segment_id":  meta.SegmentID,
				})
				writeControlEvent(t, conn, "audio.out.cleared", sessionID, 4, map[string]any{
					"response_id":              meta.ResponseID,
					"playback_id":              meta.PlaybackID,
					"cleared_after_segment_id": meta.SegmentID,
					"reason":                   "barge_in_clear",
				})
			}
			continue
		}
		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "audio.out.meta":
			sawMeta = true
			meta = audioOutMetaPayload{
				ResponseID: stringValue(event.Payload["response_id"]),
				PlaybackID: stringValue(event.Payload["playback_id"]),
				SegmentID:  stringValue(event.Payload["segment_id"]),
			}
		case "session.update":
			if sawMeta && stringValue(event.Payload["state"]) == "active" {
				sawActive = true
				goto done
			}
		case "error":
			t.Fatalf("unexpected error event: %#v", event.Payload)
		}
	}

done:
	if !sawMeta {
		t.Fatal("expected audio.out.meta before playback clear")
	}
	if !sawActive {
		t.Fatal("expected audio.out.cleared to drive return-to-active")
	}
	if audioChunks >= 20 {
		t.Fatalf("expected cleared playback to stop early, got %d chunks", audioChunks)
	}

	logs := logBuffer.String()
	for _, want := range []string{
		`"msg":"gateway playback ack cleared"`,
		`"reason":"barge_in_clear"`,
		`"msg":"gateway turn interrupted"`,
	} {
		if !strings.Contains(logs, want) {
			t.Fatalf("expected logs to contain %s, got:\n%s", want, logs)
		}
	}
}

func TestRealtimeWSPlaybackAckEmitsAudioOutMetaPerSegment(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000
	var logBuffer bytes.Buffer
	profile.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))

	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		if strings.TrimSpace(req.Text) != "play" {
			return voice.TurnResponse{Text: "unexpected"}, nil
		}
		return voice.TurnResponse{
			Text: "好的，先打开客厅灯。再关闭窗帘。",
			AudioStream: &scriptedSegmentedAudioStream{
				segments: []scriptedPlaybackSegment{
					{
						text:     "好的，先打开客厅灯。",
						duration: 420 * time.Millisecond,
						chunks:   repeatAudioChunks(make([]byte, 640), 1),
					},
					{
						text:     "再关闭窗帘。",
						duration: 360 * time.Millisecond,
						isLast:   true,
						chunks:   repeatAudioChunks(make([]byte, 640), 1),
					},
				},
			},
		}, nil
	})

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
		"playback_ack": map[string]any{
			"mode": "segment_mark_v1",
		},
	})
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "play"})

	metas := make([]audioOutMetaPayload, 0, 2)
	relevantEvents := make([]string, 0, 4)
	audioChunks := 0
	startedSent := false
	completedSent := false
	sawActive := false

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			t.Fatalf("set read deadline failed: %v", err)
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message failed: %v\nserver log:\n%s\nhandler log:\n%s", err, serverLog.String(), logBuffer.String())
		}
		if messageType == websocket.BinaryMessage {
			relevantEvents = append(relevantEvents, "binary")
			audioChunks++
			if len(metas) == 2 && audioChunks >= 2 && !completedSent {
				writeControlEvent(t, conn, "audio.out.completed", sessionID, 5, map[string]any{
					"response_id": metas[0].ResponseID,
					"playback_id": metas[0].PlaybackID,
				})
				completedSent = true
			}
			continue
		}

		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "audio.out.meta":
			relevantEvents = append(relevantEvents, "meta")
			meta := audioOutMetaPayload{
				ResponseID:         stringValue(event.Payload["response_id"]),
				PlaybackID:         stringValue(event.Payload["playback_id"]),
				SegmentID:          stringValue(event.Payload["segment_id"]),
				Text:               stringValue(event.Payload["text"]),
				ExpectedDurationMs: intValue(event.Payload["expected_duration_ms"]),
				IsLastSegment:      boolValue(event.Payload["is_last_segment"]),
			}
			metas = append(metas, meta)
			if !startedSent {
				writeControlEvent(t, conn, "audio.out.started", sessionID, 3, map[string]any{
					"response_id": meta.ResponseID,
					"playback_id": meta.PlaybackID,
					"segment_id":  meta.SegmentID,
				})
				startedSent = true
			}
		case "session.update":
			if completedSent && stringValue(event.Payload["state"]) == "active" {
				sawActive = true
				goto done
			}
		case "error":
			t.Fatalf("unexpected error event: %#v", event.Payload)
		}
	}

done:
	if !sawActive {
		t.Fatal("expected session to return to active after segmented playback completion")
	}
	if got := len(metas); got != 2 {
		t.Fatalf("expected two audio.out.meta events, got %+v", metas)
	}
	if metas[0].ResponseID == "" || metas[0].PlaybackID == "" {
		t.Fatalf("expected populated playback ids, got %+v", metas[0])
	}
	if metas[0].ResponseID != metas[1].ResponseID || metas[0].PlaybackID != metas[1].PlaybackID {
		t.Fatalf("expected shared response/playback ids across segments, got %+v", metas)
	}
	if metas[0].SegmentID == metas[1].SegmentID {
		t.Fatalf("expected unique segment ids, got %+v", metas)
	}
	if metas[0].Text != "好的，先打开客厅灯。" || metas[1].Text != "再关闭窗帘。" {
		t.Fatalf("unexpected segment texts %+v", metas)
	}
	if metas[0].IsLastSegment {
		t.Fatalf("expected first segment to stay non-final, got %+v", metas[0])
	}
	if !metas[1].IsLastSegment {
		t.Fatalf("expected second segment to be final, got %+v", metas[1])
	}
	if got := strings.Join(relevantEvents, ","); got != "meta,binary,meta,binary" {
		t.Fatalf("expected segment meta to precede each binary segment, got %q", got)
	}
	assertNoServerPanic(t, serverLog)
}

func TestRealtimeWSPlaybackAckClearedFeedsNextTurnResumeMetadataAtSegmentBoundary(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000
	var logBuffer bytes.Buffer
	profile.Logger = slog.New(slog.NewJSONHandler(&logBuffer, nil))

	var (
		mu       sync.Mutex
		requests []voice.TurnRequest
	)
	responder := responderFunc(func(_ context.Context, req voice.TurnRequest) (voice.TurnResponse, error) {
		mu.Lock()
		requests = append(requests, req)
		callIndex := len(requests)
		mu.Unlock()

		switch callIndex {
		case 1:
			return voice.TurnResponse{
				Text: "好的，先打开客厅灯。再关闭窗帘。",
				AudioStream: &scriptedSegmentedAudioStream{
					segments: []scriptedPlaybackSegment{
						{
							text:     "好的，先打开客厅灯。",
							duration: 420 * time.Millisecond,
							chunks:   repeatAudioChunks(make([]byte, 640), 1),
						},
						{
							text:     "再关闭窗帘。",
							duration: 360 * time.Millisecond,
							isLast:   true,
							chunks:   repeatAudioChunks(make([]byte, 640), 2),
						},
					},
				},
			}, nil
		default:
			return voice.TurnResponse{Text: "继续处理。"}, nil
		}
	})

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
		"playback_ack": map[string]any{
			"mode": "segment_mark_v1",
		},
	})
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "play"})

	var (
		firstMeta      audioOutMetaPayload
		secondMeta     audioOutMetaPayload
		audioChunks    int
		clearedSent    bool
		secondTurnSent bool
		secondTurnOK   bool
	)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			t.Fatalf("set read deadline failed: %v", err)
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message failed: %v\nserver log:\n%s\nhandler log:\n%s", err, serverLog.String(), logBuffer.String())
		}
		if messageType == websocket.BinaryMessage {
			audioChunks++
			if firstMeta.SegmentID != "" && secondMeta.SegmentID != "" && audioChunks >= 1 && !clearedSent {
				writeControlEvent(t, conn, "audio.out.started", sessionID, 3, map[string]any{
					"response_id": firstMeta.ResponseID,
					"playback_id": firstMeta.PlaybackID,
					"segment_id":  firstMeta.SegmentID,
				})
				writeControlEvent(t, conn, "audio.out.cleared", sessionID, 4, map[string]any{
					"response_id":              firstMeta.ResponseID,
					"playback_id":              firstMeta.PlaybackID,
					"cleared_after_segment_id": firstMeta.SegmentID,
					"reason":                   "barge_in_clear",
				})
				clearedSent = true
			}
			continue
		}

		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "audio.out.meta":
			meta := audioOutMetaPayload{
				ResponseID:         stringValue(event.Payload["response_id"]),
				PlaybackID:         stringValue(event.Payload["playback_id"]),
				SegmentID:          stringValue(event.Payload["segment_id"]),
				Text:               stringValue(event.Payload["text"]),
				ExpectedDurationMs: intValue(event.Payload["expected_duration_ms"]),
				IsLastSegment:      boolValue(event.Payload["is_last_segment"]),
			}
			if firstMeta.SegmentID == "" {
				firstMeta = meta
			} else if secondMeta.SegmentID == "" {
				secondMeta = meta
			}
		case "session.update":
			if clearedSent && !secondTurnSent && stringValue(event.Payload["state"]) == "active" {
				writeControlEvent(t, conn, "text.in", sessionID, 5, map[string]any{"text": "继续"})
				secondTurnSent = true
			}
		case "response.chunk":
			if stringValue(event.Payload["text"]) == "继续处理。" {
				secondTurnOK = true
				goto done
			}
		case "error":
			t.Fatalf("unexpected error event: %#v", event.Payload)
		}
	}

done:
	if !secondTurnOK {
		t.Fatal("expected follow-up turn to complete after playback clear")
	}

	mu.Lock()
	defer mu.Unlock()
	if got := len(requests); got < 2 {
		t.Fatalf("expected two turn requests, got %d", got)
	}
	secondRequest := requests[1]
	if got := secondRequest.Metadata["voice.previous.available"]; got != "true" {
		t.Fatalf("expected previous playback metadata on follow-up turn, got %+v", secondRequest.Metadata)
	}
	if got := secondRequest.Metadata["voice.previous.heard_text"]; got != "好的，先打开客厅灯。" {
		t.Fatalf("expected exact first-segment heard text, got %q", got)
	}
	if got := secondRequest.Metadata["voice.previous.resume_anchor"]; got != "好的，先打开客厅灯。" {
		t.Fatalf("expected exact segment resume anchor, got %q", got)
	}
	if got := secondRequest.Metadata["voice.previous.missed_text"]; got != "再关闭窗帘。" {
		t.Fatalf("expected second segment as missed text, got %q", got)
	}
	assertNoServerPanic(t, serverLog)
}

func TestRealtimeWSEarlySegmentMetaPromotesPlaybackContextBeforeResponseSettles(t *testing.T) {
	profile := testRealtimeProfile()
	profile.IdleTimeoutMs = 5000
	profile.MaxSessionMs = 10000

	store := &recordingMemoryStore{}
	responder := orchestratingProviderResponder{
		memory: store,
		respondOrchestrated: func(context.Context, voice.TurnRequest, voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
			return fakeTurnResponseFuture{
				audioAfter:    5 * time.Millisecond,
				responseAfter: 900 * time.Millisecond,
				stream: &scriptedSegmentedAudioStream{
					segments: []scriptedPlaybackSegment{
						{
							text:     "好的，先打开客厅灯。",
							duration: 500 * time.Millisecond,
							chunks:   repeatAudioChunks(make([]byte, 640), 25),
						},
						{
							text:     "再关闭窗帘。",
							duration: 500 * time.Millisecond,
							isLast:   true,
							chunks:   repeatAudioChunks(make([]byte, 640), 25),
						},
					},
				},
				finalStream: voice.NewStaticAudioStream([][]byte{make([]byte, 8)}),
				text:        "好的，先打开客厅灯。再关闭窗帘。",
				audioText:   "好的，先打开客厅灯。",
			}, nil
		},
	}

	conn, serverLog := openRealtimeWSWithServerLog(t, profile, responder)
	defer conn.Close()

	sessionID := startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
		"playback_ack": map[string]any{
			"mode": "segment_mark_v1",
		},
	})

	startedAt := time.Now()
	writeControlEvent(t, conn, "text.in", sessionID, 2, map[string]any{"text": "play"})

	metaCount := 0
	for metaCount < 2 {
		if err := conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			t.Fatalf("set read deadline failed: %v", err)
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message failed before second meta: %v\nserver log:\n%s", err, serverLog.String())
		}
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		switch event.Type {
		case "audio.out.meta":
			metaCount++
		case "error":
			t.Fatalf("unexpected error event: %#v", event.Payload)
		}
	}

	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		latest, ok := store.Latest()
		if ok && latest.DeliveredText == "好的，先打开客厅灯。再关闭窗帘。" {
			if latest.PlaybackCompleted {
				t.Fatalf("expected speaking-time context promotion before playback completed, got %+v", latest)
			}
			if time.Since(startedAt) >= 850*time.Millisecond {
				t.Fatalf("expected context promotion before final response settles, got %s", time.Since(startedAt))
			}
			break
		}
		if time.Now().After(deadline) {
			if latest, ok := store.Latest(); ok {
				t.Fatalf("expected latest announced segment to promote playback context early, got %+v", latest)
			}
			t.Fatal("expected persisted playback context before final response settled")
		}
		time.Sleep(10 * time.Millisecond)
	}

	drainDeadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(drainDeadline) {
		if err := conn.SetReadDeadline(time.Now().Add(250 * time.Millisecond)); err != nil {
			t.Fatalf("set read deadline failed: %v", err)
		}
		messageType, payload, err := conn.ReadMessage()
		if err != nil {
			break
		}
		if messageType == websocket.BinaryMessage {
			continue
		}
		event := decodeJSONEvent(t, payload)
		if event.Type == "session.update" && stringValue(event.Payload["state"]) == "active" {
			break
		}
		if event.Type == "error" {
			t.Fatalf("unexpected error event while draining: %#v", event.Payload)
		}
	}

	assertNoServerPanic(t, serverLog)
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
	return startTestSessionWithCapabilities(t, conn, profile, map[string]any{
		"text_input":      true,
		"image_input":     false,
		"half_duplex":     false,
		"local_wake_word": false,
	})
}

func startTestSessionWithCapabilities(t *testing.T, conn *websocket.Conn, profile RealtimeProfile, capabilities map[string]any) string {
	t.Helper()

	sessionID := "sess_test"
	writeControlEvent(t, conn, "session.start", sessionID, 1, map[string]any{
		"protocol_version": profile.ProtocolVersion,
		"device":           map[string]any{"device_id": "rtos-mock-001", "client_type": "rtos-mock", "firmware_version": "test"},
		"audio":            map[string]any{"codec": profile.InputCodec, "sample_rate_hz": profile.InputSampleRate, "channels": profile.InputChannels},
		"session":          map[string]any{"mode": "voice", "wake_reason": "test", "client_can_end": true, "server_can_end": true},
		"capabilities":     capabilities,
	})

	event := readNextJSONEvent(t, conn, 2*time.Second)
	if event.Type != "session.update" {
		t.Fatalf("expected initial session.update, got %s", event.Type)
	}
	if got := stringValue(event.Payload["state"]); got != "active" {
		t.Fatalf("expected initial state active, got %q", got)
	}
	if got := stringValue(event.Payload["input_state"]); got != "active" {
		t.Fatalf("expected initial input_state active, got %q", got)
	}
	if got := stringValue(event.Payload["output_state"]); got != "idle" {
		t.Fatalf("expected initial output_state idle, got %q", got)
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

func intValue(value any) int {
	switch rendered := value.(type) {
	case float64:
		return int(rendered)
	case int:
		return rendered
	case int64:
		return int(rendered)
	default:
		return 0
	}
}

func floatValue(value any) float64 {
	switch rendered := value.(type) {
	case float64:
		return rendered
	case float32:
		return float64(rendered)
	case int:
		return float64(rendered)
	case int64:
		return float64(rendered)
	default:
		return 0
	}
}

func boolValue(value any) bool {
	rendered, _ := value.(bool)
	return rendered
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

func pcm16ConstantChunk(level int16, samples int) []byte {
	if samples <= 0 {
		samples = 320
	}
	chunk := make([]byte, samples*2)
	for i := 0; i < samples; i++ {
		binary.LittleEndian.PutUint16(chunk[i*2:], uint16(level))
	}
	return chunk
}

func maxPCM16Abs(chunk []byte) int {
	maxAbs := 0
	for i := 0; i+1 < len(chunk); i += 2 {
		value := int(int16(binary.LittleEndian.Uint16(chunk[i:])))
		if value < 0 {
			value = -value
		}
		if value > maxAbs {
			maxAbs = value
		}
	}
	return maxAbs
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
