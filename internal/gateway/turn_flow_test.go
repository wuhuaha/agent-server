package gateway

import (
	"context"
	"errors"
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
		Text:        f.text,
		Incremental: true,
		Source:      voice.ResponseAudioStartSourceSpeechPlanner,
	}, true, nil
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
