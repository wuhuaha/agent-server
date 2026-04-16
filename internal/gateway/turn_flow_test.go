package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
