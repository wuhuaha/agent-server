package voice

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"agent-server/internal/agent"
)

func TestSpeechPlannerSegmentsStableClausesAndFinalTail(t *testing.T) {
	planner := NewSpeechPlanner(SpeechPlannerConfig{
		Enabled:          true,
		MinChunkRunes:    4,
		TargetChunkRunes: 10,
	})

	segments := planner.ObserveTextDelta("先把客厅灯打开，")
	if len(segments) != 1 || segments[0] != "先把客厅灯打开，" {
		t.Fatalf("unexpected first planned segments: %+v", segments)
	}

	segments = planner.ObserveTextDelta("再把窗帘关上")
	if len(segments) != 0 {
		t.Fatalf("expected second partial to stay buffered, got %+v", segments)
	}

	segments = planner.FinalizeText("先把客厅灯打开，再把窗帘关上。最后播点轻音乐。")
	if len(segments) != 2 {
		t.Fatalf("expected final tail to split into two segments, got %+v", segments)
	}
	if segments[0] != "再把窗帘关上。" {
		t.Fatalf("unexpected first final segment %q", segments[0])
	}
	if segments[1] != "最后播点轻音乐。" {
		t.Fatalf("unexpected second final segment %q", segments[1])
	}
}

func TestBootstrapResponderSpeechPlannerStartsSynthesisBeforeStreamFinishes(t *testing.T) {
	executor := &blockingStreamingExecutor{
		firstDeltaEmitted: make(chan struct{}),
		allowContinue:     make(chan struct{}),
	}
	synth := &recordingSynthesizer{
		started: make(chan string, 4),
	}
	responder := NewBootstrapResponder("pcm16le", 16000, 1).
		WithTurnExecutor(executor).
		WithSynthesizer(synth).
		WithSpeechPlannerConfig(SpeechPlannerConfig{
			Enabled:          true,
			MinChunkRunes:    2,
			TargetChunkRunes: 6,
		})

	type responseResult struct {
		response TurnResponse
		err      error
	}
	resultCh := make(chan responseResult, 1)
	go func() {
		response, err := responder.RespondStream(context.Background(), TurnRequest{
			SessionID: "sess_planner",
			TurnID:    "turn_planner",
			TraceID:   "trace_planner",
			DeviceID:  "dev_planner",
			Text:      "帮我控制一下",
		}, nil)
		resultCh <- responseResult{response: response, err: err}
	}()

	select {
	case <-executor.firstDeltaEmitted:
	case <-time.After(2 * time.Second):
		t.Fatal("expected executor to emit the first text delta")
	}

	select {
	case text := <-synth.started:
		if text != "当然可以，" {
			t.Fatalf("expected planner to start synthesizing the first stable clause, got %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected speech planner to start synthesis before the executor finished streaming")
	}

	close(executor.allowContinue)

	var result responseResult
	select {
	case result = <-resultCh:
	case <-time.After(2 * time.Second):
		t.Fatal("expected responder to finish after executor unblocked")
	}
	if result.err != nil {
		t.Fatalf("RespondStream failed: %v", result.err)
	}
	if result.response.AudioStream == nil {
		t.Fatal("expected planned audio stream on the response")
	}

	chunk, err := result.response.AudioStream.Next(context.Background())
	if err != nil {
		t.Fatalf("expected first synthesized chunk, got error %v", err)
	}
	if len(chunk) == 0 {
		t.Fatal("expected non-empty synthesized chunk")
	}
	_ = result.response.AudioStream.Close()

	synth.mu.Lock()
	defer synth.mu.Unlock()
	if len(synth.texts) == 0 {
		t.Fatal("expected synthesizer to receive planned text segments")
	}
	if synth.texts[0] != "当然可以，" {
		t.Fatalf("unexpected first synthesized segment %q", synth.texts[0])
	}
	if !strings.Contains(strings.Join(synth.texts, ""), "我先打开客厅灯。") {
		t.Fatalf("expected later synthesized segments to include the remaining answer, got %+v", synth.texts)
	}
}

type blockingStreamingExecutor struct {
	firstDeltaEmitted chan struct{}
	allowContinue     chan struct{}
}

func (e *blockingStreamingExecutor) ExecuteTurn(context.Context, agent.TurnInput) (agent.TurnOutput, error) {
	return agent.TurnOutput{
		Text: "当然可以，我先打开客厅灯。",
	}, nil
}

func (e *blockingStreamingExecutor) StreamTurn(ctx context.Context, _ agent.TurnInput, sink agent.TurnDeltaSink) (agent.TurnOutput, error) {
	if err := sink.EmitTurnDelta(ctx, agent.TurnDelta{
		Kind: agent.TurnDeltaKindText,
		Text: "当然可以，",
	}); err != nil {
		return agent.TurnOutput{}, err
	}
	close(e.firstDeltaEmitted)

	select {
	case <-ctx.Done():
		return agent.TurnOutput{}, ctx.Err()
	case <-e.allowContinue:
	}

	if err := sink.EmitTurnDelta(ctx, agent.TurnDelta{
		Kind: agent.TurnDeltaKindText,
		Text: "我先打开客厅灯。",
	}); err != nil {
		return agent.TurnOutput{}, err
	}
	return agent.TurnOutput{
		Text: "当然可以，我先打开客厅灯。",
	}, nil
}

type recordingSynthesizer struct {
	mu      sync.Mutex
	started chan string
	texts   []string
}

func (s *recordingSynthesizer) Synthesize(_ context.Context, req SynthesisRequest) (SynthesisResult, error) {
	s.mu.Lock()
	s.texts = append(s.texts, req.Text)
	s.mu.Unlock()
	if s.started != nil {
		s.started <- req.Text
	}
	return SynthesisResult{
		AudioPCM:     make([]byte, 1280),
		SampleRateHz: 16000,
		Channels:     1,
		Codec:        "pcm16le",
	}, nil
}
