package gateway

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-server/internal/agent"
	"agent-server/internal/session"
	"agent-server/internal/voice"
)

type orchestratingResponderFunc func(context.Context, voice.TurnRequest, voice.ResponseDeltaSink) (voice.TurnResponseFuture, error)

func (f orchestratingResponderFunc) Respond(context.Context, voice.TurnRequest) (voice.TurnResponse, error) {
	return voice.TurnResponse{}, errors.New("Respond should not be called")
}

func (f orchestratingResponderFunc) RespondOrchestrated(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
	return f(ctx, req, sink)
}

type fakeTurnResponseFuture struct {
	audioAfter    time.Duration
	responseAfter time.Duration
	stream        voice.AudioStream
	finalStream   voice.AudioStream
	text          string
	audioText     string
}

func (f fakeTurnResponseFuture) Wait(ctx context.Context) (voice.TurnResponse, error) {
	select {
	case <-ctx.Done():
		return voice.TurnResponse{}, ctx.Err()
	case <-time.After(f.responseAfter):
	}
	return voice.TurnResponse{
		InputText:   f.text,
		Text:        f.text,
		AudioStream: f.finalStream,
	}, nil
}

func (f fakeTurnResponseFuture) WaitAudioStart(ctx context.Context) (voice.ResponseAudioStart, bool, error) {
	select {
	case <-ctx.Done():
		return voice.ResponseAudioStart{}, false, ctx.Err()
	case <-time.After(f.audioAfter):
	}
	return voice.ResponseAudioStart{
		Stream:      f.stream,
		Text:        audioStartTextForTest(f),
		Incremental: true,
		Source:      voice.ResponseAudioStartSourceSpeechPlanner,
	}, true, nil
}

func audioStartTextForTest(f fakeTurnResponseFuture) string {
	if strings.TrimSpace(f.audioText) != "" {
		return f.audioText
	}
	return f.text
}

type recordingMemoryStore struct {
	mu      sync.Mutex
	records []agent.MemoryRecord
}

func (s *recordingMemoryStore) LoadTurnContext(context.Context, agent.MemoryQuery) (agent.MemoryContext, error) {
	return agent.MemoryContext{}, nil
}

func (s *recordingMemoryStore) SaveTurn(_ context.Context, record agent.MemoryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	cloned := record
	cloned.Metadata = cloneMetadataMap(record.Metadata)
	s.records = append(s.records, cloned)
	return nil
}

func (s *recordingMemoryStore) Latest() (agent.MemoryRecord, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.records) == 0 {
		return agent.MemoryRecord{}, false
	}
	return s.records[len(s.records)-1], true
}

type orchestratingProviderResponder struct {
	memory              agent.MemoryStore
	respondOrchestrated func(context.Context, voice.TurnRequest, voice.ResponseDeltaSink) (voice.TurnResponseFuture, error)
}

func (r orchestratingProviderResponder) Respond(context.Context, voice.TurnRequest) (voice.TurnResponse, error) {
	return voice.TurnResponse{}, errors.New("Respond should not be called")
}

func (r orchestratingProviderResponder) RespondOrchestrated(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
	return r.respondOrchestrated(ctx, req, sink)
}

func (r orchestratingProviderResponder) NewSessionOrchestrator() *voice.SessionOrchestrator {
	return voice.NewSessionOrchestrator(r.memory)
}

func cloneMetadataMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return nil
	}
	cloned := make(map[string]string, len(src))
	for key, value := range src {
		cloned[key] = value
	}
	return cloned
}

func TestExecuteTurnResponseStartsAudioBeforeFinalResponseSettles(t *testing.T) {
	audioChunk := []byte{1, 2, 3, 4}
	responder := orchestratingResponderFunc(func(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
		go func() {
			time.Sleep(5 * time.Millisecond)
			_ = sink.EmitResponseDelta(ctx, voice.ResponseDelta{Kind: voice.ResponseDeltaKindText, Text: "先回答一点，"})
		}()
		return fakeTurnResponseFuture{
			audioAfter:    15 * time.Millisecond,
			responseAfter: 80 * time.Millisecond,
			stream:        voice.NewStaticAudioStream([][]byte{audioChunk}),
			finalStream:   voice.NewStaticAudioStream([][]byte{audioChunk}),
			text:          "先回答一点，后面再补全。",
		}, nil
	})

	runtime := newConnectionRuntime(nil, nil, session.NewRealtimeSession(), responder)
	startedAt := time.Now()

	var firstAudioAt time.Time
	result, err := executeTurnResponse(context.Background(), voice.TurnRequest{
		SessionID: "sess_orchestrated",
		TurnID:    "turn_orchestrated",
		TraceID:   "trace_orchestrated",
		Text:      "测试一下",
	}, turnTrace{TurnID: "turn_orchestrated", TraceID: "trace_orchestrated", AcceptedAt: startedAt}, turnExecutionOptions{
		Runtime:   runtime,
		Responder: responder,
		SessionID: "sess_orchestrated",
		EmitResponseStart: func(turnTrace, string, []string, voice.TurnResponse) error {
			return nil
		},
		StartResponseAudio: func(trace turnTrace, responseID string, audioStart voice.ResponseAudioStart, aggregatedText string, completion *turnOutputOutcomeFuture) error {
			chunk, err := audioStart.Stream.Next(context.Background())
			if err != nil {
				return err
			}
			if len(chunk) == 0 {
				t.Fatal("expected early audio chunk")
			}
			firstAudioAt = time.Now()
			return nil
		},
	})
	if err != nil {
		t.Fatalf("executeTurnResponse failed: %v", err)
	}
	if firstAudioAt.IsZero() {
		t.Fatal("expected early audio start callback to run")
	}
	if !result.Response.AudioStreamTransferred {
		t.Fatal("expected final response audio ownership to transfer to early start path")
	}
	if !firstAudioAt.Before(time.Now()) {
		t.Fatal("expected first audio timestamp to be captured")
	}
	if firstAudioAt.Sub(startedAt) >= 70*time.Millisecond {
		t.Fatalf("expected audio to start well before final response settle, got %s", firstAudioAt.Sub(startedAt))
	}
}

func TestExecuteTurnResponseUsesAudioStartTextWhenAudioWinsRace(t *testing.T) {
	audioChunk := []byte{9, 8, 7, 6}
	responder := orchestratingResponderFunc(func(context.Context, voice.TurnRequest, voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
		return fakeTurnResponseFuture{
			audioAfter:    5 * time.Millisecond,
			responseAfter: 60 * time.Millisecond,
			stream:        voice.NewStaticAudioStream([][]byte{audioChunk}),
			finalStream:   voice.NewStaticAudioStream([][]byte{audioChunk}),
			text:          "先回答一点，后面再补全。",
			audioText:     "先回答一点，",
		}, nil
	})

	runtime := newConnectionRuntime(nil, nil, session.NewRealtimeSession(), responder)
	var (
		startModalities []string
		startText       string
	)
	_, err := executeTurnResponse(context.Background(), voice.TurnRequest{
		SessionID: "sess_audio_first",
		TurnID:    "turn_audio_first",
		TraceID:   "trace_audio_first",
		Text:      "测试音频先起播",
	}, turnTrace{TurnID: "turn_audio_first", TraceID: "trace_audio_first", AcceptedAt: time.Now()}, turnExecutionOptions{
		Runtime:   runtime,
		Responder: responder,
		SessionID: "sess_audio_first",
		EmitResponseStart: func(_ turnTrace, _ string, modalities []string, _ voice.TurnResponse) error {
			startModalities = append([]string(nil), modalities...)
			return nil
		},
		StartResponseAudio: func(_ turnTrace, _ string, audioStart voice.ResponseAudioStart, aggregatedText string, _ *turnOutputOutcomeFuture) error {
			startText = aggregatedText
			_, err := audioStart.Stream.Next(context.Background())
			return err
		},
	})
	if err != nil {
		t.Fatalf("executeTurnResponse failed: %v", err)
	}
	if got := strings.Join(startModalities, ","); got != "text,audio" {
		t.Fatalf("expected response.start modalities text,audio, got %q", got)
	}
	if startText != "先回答一点，" {
		t.Fatalf("expected audio-first aggregated text fallback, got %q", startText)
	}
}

func TestExecuteTurnResponsePrefersRicherAudioStartHintWhenCollectorIsPrefix(t *testing.T) {
	audioChunk := []byte{7, 7, 7, 7}
	responder := orchestratingResponderFunc(func(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
		go func() {
			time.Sleep(5 * time.Millisecond)
			_ = sink.EmitResponseDelta(ctx, voice.ResponseDelta{Kind: voice.ResponseDeltaKindText, Text: "先回答一点，"})
		}()
		return fakeTurnResponseFuture{
			audioAfter:    10 * time.Millisecond,
			responseAfter: 70 * time.Millisecond,
			stream:        voice.NewStaticAudioStream([][]byte{audioChunk}),
			finalStream:   voice.NewStaticAudioStream([][]byte{audioChunk}),
			text:          "先回答一点，后面再补全。",
			audioText:     "先回答一点，后面再补全。",
		}, nil
	})

	runtime := newConnectionRuntime(nil, nil, session.NewRealtimeSession(), responder)
	var startText string
	_, err := executeTurnResponse(context.Background(), voice.TurnRequest{
		SessionID: "sess_audio_hint",
		TurnID:    "turn_audio_hint",
		TraceID:   "trace_audio_hint",
		Text:      "测试 richer audio hint",
	}, turnTrace{TurnID: "turn_audio_hint", TraceID: "trace_audio_hint", AcceptedAt: time.Now()}, turnExecutionOptions{
		Runtime:   runtime,
		Responder: responder,
		SessionID: "sess_audio_hint",
		EmitResponseStart: func(turnTrace, string, []string, voice.TurnResponse) error {
			return nil
		},
		StartResponseAudio: func(_ turnTrace, _ string, audioStart voice.ResponseAudioStart, aggregatedText string, _ *turnOutputOutcomeFuture) error {
			startText = aggregatedText
			_, err := audioStart.Stream.Next(context.Background())
			return err
		},
	})
	if err != nil {
		t.Fatalf("executeTurnResponse failed: %v", err)
	}
	if startText != "先回答一点，后面再补全。" {
		t.Fatalf("expected richer audio hint to extend playback text, got %q", startText)
	}
}

func TestExecuteTurnResponseExtendsPlaybackTextAfterEarlyAudioStart(t *testing.T) {
	audioChunk := []byte{5, 4, 3, 2}
	responder := orchestratingResponderFunc(func(ctx context.Context, req voice.TurnRequest, sink voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
		go func() {
			time.Sleep(5 * time.Millisecond)
			_ = sink.EmitResponseDelta(ctx, voice.ResponseDelta{Kind: voice.ResponseDeltaKindText, Text: "先回答一点，"})
			time.Sleep(20 * time.Millisecond)
			_ = sink.EmitResponseDelta(ctx, voice.ResponseDelta{Kind: voice.ResponseDeltaKindText, Text: "后面再补全。"})
		}()
		return fakeTurnResponseFuture{
			audioAfter:    12 * time.Millisecond,
			responseAfter: 70 * time.Millisecond,
			stream:        voice.NewStaticAudioStream([][]byte{audioChunk}),
			finalStream:   voice.NewStaticAudioStream([][]byte{audioChunk}),
			text:          "先回答一点，后面再补全。",
		}, nil
	})

	runtime := newConnectionRuntime(nil, nil, session.NewRealtimeSession(), responder)
	if _, err := runtime.session.Start(session.StartRequest{
		RequestedSessionID: "sess_playback_truth",
		DeviceID:           "dev_playback_truth",
		ClientType:         "rtos",
		Mode:               "voice",
		InputCodec:         "pcm16le",
		InputSampleRate:    16000,
		InputChannels:      1,
		ClientCanEnd:       true,
		ServerCanEnd:       true,
	}); err != nil {
		t.Fatalf("start session failed: %v", err)
	}
	var speakingTexts []string
	_, err := executeTurnResponse(context.Background(), voice.TurnRequest{
		SessionID: "sess_playback_truth",
		TurnID:    "turn_playback_truth",
		TraceID:   "trace_playback_truth",
		Text:      "测试增量播放文本",
	}, turnTrace{TurnID: "turn_playback_truth", TraceID: "trace_playback_truth", AcceptedAt: time.Now()}, turnExecutionOptions{
		Runtime:   runtime,
		Responder: responder,
		SessionID: "sess_playback_truth",
		EmitResponseStart: func(turnTrace, string, []string, voice.TurnResponse) error {
			return nil
		},
		OnTextDeltaCollected: func(_ turnTrace, aggregatedText string) {
			if runtime.session.Snapshot().OutputState == session.OutputStateSpeaking {
				speakingTexts = append(speakingTexts, aggregatedText)
			}
		},
		StartResponseAudio: func(_ turnTrace, _ string, audioStart voice.ResponseAudioStart, _ string, _ *turnOutputOutcomeFuture) error {
			if _, err := audioStart.Stream.Next(context.Background()); err != nil {
				return err
			}
			_, err := runtime.session.SetOutputState(session.OutputStateSpeaking)
			return err
		},
	})
	if err != nil {
		t.Fatalf("executeTurnResponse failed: %v", err)
	}
	if len(speakingTexts) == 0 {
		t.Fatal("expected playback text updates after early audio start")
	}
	if got := speakingTexts[len(speakingTexts)-1]; got != "先回答一点，后面再补全。" {
		t.Fatalf("expected latest playback text to include later deltas, got %q", got)
	}
}

func TestRecordPlaybackAckMarkUsesLatestAnnouncedTextForResumeContext(t *testing.T) {
	store := &recordingMemoryStore{}
	responder := orchestratingProviderResponder{
		memory: store,
		respondOrchestrated: func(context.Context, voice.TurnRequest, voice.ResponseDeltaSink) (voice.TurnResponseFuture, error) {
			return fakeTurnResponseFuture{}, errors.New("unexpected RespondOrchestrated call")
		},
	}
	runtime := newConnectionRuntime(nil, nil, session.NewRealtimeSession(), responder)
	runtime.collaboration = collaborationNegotiation{
		PlaybackAck: sessionStartPlaybackAckCapability{Mode: playbackAckModeSegmentMarkV1},
	}
	if _, err := runtime.session.Start(session.StartRequest{
		RequestedSessionID: "sess_ack_mark",
		DeviceID:           "dev_ack_mark",
		ClientType:         "rtos",
		Mode:               "voice",
		InputCodec:         "pcm16le",
		InputSampleRate:    16000,
		InputChannels:      1,
		ClientCanEnd:       true,
		ServerCanEnd:       true,
	}); err != nil {
		t.Fatalf("start session failed: %v", err)
	}

	runtime.voiceSession.PrepareTurn(voice.TurnRequest{
		SessionID: "sess_ack_mark",
		TurnID:    "turn_ack_mark",
		TraceID:   "trace_ack_mark",
	}, "打开客厅灯并关闭窗帘", "")
	runtime.voiceSession.StartPlaybackWithOptions("好的，先打开客厅灯。", outputFrameInterval, 420*time.Millisecond, voice.PlaybackStartOptions{
		PreferClientFacts: true,
	})

	first := newAudioPlaybackMeta("resp_ack_mark", "好的，先打开客厅灯。", 420*time.Millisecond)
	second := nextSegmentAudioPlaybackMeta(first, "再关闭窗帘。", 360*time.Millisecond, true)
	runtime.installPlaybackAckMeta(first)
	runtime.activatePlaybackAckSegment(first)
	runtime.activatePlaybackAckSegment(second)

	handler := &realtimeWSHandler{
		profile:   RealtimeProfile{},
		responder: responder,
		logger:    slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if err := handler.recordPlaybackAckMark(runtime, audioOutMarkPayload{
		ResponseID:       first.ResponseID,
		PlaybackID:       first.PlaybackID,
		SegmentID:        first.SegmentID,
		PlayedDurationMs: audioDurationMs(first.ExpectedDuration),
	}); err != nil {
		t.Fatalf("recordPlaybackAckMark failed: %v", err)
	}

	outcome := runtime.voiceSession.InterruptPlaybackWithPolicy(voice.InterruptionPolicyHardInterrupt, "test_interrupt")
	if outcome.HeardText != "好的，先打开客厅灯。" {
		t.Fatalf("expected exact first-segment heard text, got %q", outcome.HeardText)
	}

	metadata := runtime.voiceSession.LastPlaybackContextMetadata()
	if got := metadata["voice.previous.delivered_text"]; got != "好的，先打开客厅灯。再关闭窗帘。" {
		t.Fatalf("expected delivered text to follow latest announced segment, got %q", got)
	}
	if got := metadata["voice.previous.resume_anchor"]; got != "好的，先打开客厅灯。" {
		t.Fatalf("expected resume anchor at first-segment boundary, got %q", got)
	}
	if got := metadata["voice.previous.missed_text"]; got != "再关闭窗帘。" {
		t.Fatalf("expected missed text from later announced segment, got %q", got)
	}
	if latest, ok := store.Latest(); !ok {
		t.Fatal("expected playback context to persist into memory store")
	} else if got := latest.DeliveredText; got != "好的，先打开客厅灯。再关闭窗帘。" {
		t.Fatalf("expected persisted delivered text to reflect latest announced segment, got %q", got)
	}
}
