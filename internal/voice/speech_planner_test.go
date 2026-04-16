package voice

import (
	"context"
	"io"
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

	clauses := planner.ObserveTextDeltaClauses("先把客厅灯打开，")
	if len(clauses) != 1 || clauses[0].Text != "先把客厅灯打开，" {
		t.Fatalf("unexpected first planned clauses: %+v", clauses)
	}
	if clauses[0].BoundaryKind != SpeechClauseBoundarySoftContinue || !clauses[0].CanStartBeforeFinalized {
		t.Fatalf("expected first clause to be an early soft-continue clause, got %+v", clauses[0])
	}

	clauses = planner.ObserveTextDeltaClauses("再把窗帘关上")
	if len(clauses) != 0 {
		t.Fatalf("expected second partial to stay buffered, got %+v", clauses)
	}

	clauses = planner.FinalizeTextClauses("先把客厅灯打开，再把窗帘关上。最后播点轻音乐。")
	if len(clauses) != 2 {
		t.Fatalf("expected final tail to split into two clauses, got %+v", clauses)
	}
	if clauses[0].Text != "再把窗帘关上。" {
		t.Fatalf("unexpected first final clause %q", clauses[0].Text)
	}
	if clauses[0].BoundaryKind != SpeechClauseBoundaryStrongStop || clauses[0].CanStartBeforeFinalized {
		t.Fatalf("expected first final clause to be a finalized strong stop, got %+v", clauses[0])
	}
	if clauses[1].Text != "最后播点轻音乐。" {
		t.Fatalf("unexpected second final clause %q", clauses[1].Text)
	}
	if clauses[1].BoundaryKind != SpeechClauseBoundaryFinalFlush || clauses[1].ProsodyHint != SpeechClauseProsodyFinalFall {
		t.Fatalf("expected final clause to carry final-fall prosody, got %+v", clauses[1])
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
	drainTestAudioStream(t, result.response.AudioStream)
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

type blockingPlannerSynthesizer struct {
	mu           sync.Mutex
	started      chan string
	releaseFirst chan struct{}
	calls        int
}

func (s *blockingPlannerSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	s.mu.Lock()
	s.calls++
	callIndex := s.calls
	s.mu.Unlock()
	if s.started != nil {
		s.started <- req.Text
	}
	if callIndex == 1 && s.releaseFirst != nil {
		select {
		case <-ctx.Done():
			return SynthesisResult{}, ctx.Err()
		case <-s.releaseFirst:
		}
	}
	return SynthesisResult{
		AudioPCM:     make([]byte, 1280),
		SampleRateHz: 16000,
		Channels:     1,
		Codec:        "pcm16le",
	}, nil
}

func TestBootstrapResponderSpeechPlannerDoesNotDoubleSynthesizeFinalResponse(t *testing.T) {
	synth := &recordingSynthesizer{}
	responder := NewBootstrapResponder("pcm16le", 16000, 1).
		WithTurnExecutor(staticTurnExecutor{text: "好的。"}).
		WithSynthesizer(synth).
		WithSpeechPlannerConfig(SpeechPlannerConfig{
			Enabled:          true,
			MinChunkRunes:    2,
			TargetChunkRunes: 6,
		})

	response, err := responder.RespondStream(context.Background(), TurnRequest{
		SessionID: "sess_planner_once",
		TurnID:    "turn_planner_once",
		TraceID:   "trace_planner_once",
		DeviceID:  "dev_planner_once",
		Text:      "打开客厅灯",
	}, nil)
	if err != nil {
		t.Fatalf("RespondStream failed: %v", err)
	}
	if response.AudioStream == nil {
		t.Fatal("expected planned audio stream on the response")
	}

	if _, err := response.AudioStream.Next(context.Background()); err != nil {
		t.Fatalf("expected synthesized audio chunk, got %v", err)
	}
	drainTestAudioStream(t, response.AudioStream)
	_ = response.AudioStream.Close()

	synth.mu.Lock()
	defer synth.mu.Unlock()
	if len(synth.texts) != 1 {
		t.Fatalf("expected exactly one synthesis request when planner audio is available, got %+v", synth.texts)
	}
	if synth.texts[0] != "好的。" {
		t.Fatalf("unexpected synthesized text %q", synth.texts[0])
	}
}

func TestSpeechPlannerQueuedAudioStreamExposesEstimatedPlaybackDuration(t *testing.T) {
	planner := newPlannedSpeechSynthesis(context.Background(), &recordingSynthesizer{}, SynthesisRequest{
		SessionID: "sess_duration",
		TurnID:    "turn_duration",
		TraceID:   "trace_duration",
		DeviceID:  "dev_duration",
		UserText:  "打开客厅灯",
	}, SpeechPlannerConfig{
		Enabled:          true,
		MinChunkRunes:    2,
		TargetChunkRunes: 6,
	})
	if planner == nil {
		t.Fatal("expected planner")
	}

	planner.ObserveDelta(ResponseDelta{Kind: ResponseDeltaKindText, Text: "好的，"})
	stream := planner.Finalize("好的，已经打开客厅灯。")
	if stream == nil {
		t.Fatal("expected planned audio stream")
	}
	defer stream.Close()

	aware, ok := stream.(interface {
		PlaybackDuration(time.Duration) time.Duration
	})
	if !ok {
		t.Fatal("expected planned stream to expose playback duration")
	}
	if got := aware.PlaybackDuration(20 * time.Millisecond); got <= 0 {
		t.Fatalf("expected positive playback duration, got %s", got)
	}
}

func TestPlannedSpeechSynthesisBufferedQueueDoesNotBlockTextDeltaFlow(t *testing.T) {
	synth := &blockingPlannerSynthesizer{
		started:      make(chan string, 2),
		releaseFirst: make(chan struct{}),
	}
	planner := newPlannedSpeechSynthesis(context.Background(), synth, SynthesisRequest{
		SessionID: "sess_buffered",
		TurnID:    "turn_buffered",
		TraceID:   "trace_buffered",
		DeviceID:  "dev_buffered",
		UserText:  "打开客厅灯",
	}, SpeechPlannerConfig{
		Enabled:          true,
		MinChunkRunes:    2,
		TargetChunkRunes: 6,
	})
	if planner == nil {
		t.Fatal("expected planner")
	}

	planner.ObserveDelta(ResponseDelta{Kind: ResponseDeltaKindText, Text: "当然可以，"})
	select {
	case text := <-synth.started:
		if text != "当然可以，" {
			t.Fatalf("unexpected first synthesized clause %q", text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected first clause synthesis to start")
	}

	secondReturned := make(chan struct{})
	go func() {
		planner.ObserveDelta(ResponseDelta{Kind: ResponseDeltaKindText, Text: "我先打开客厅灯。"})
		close(secondReturned)
	}()

	select {
	case <-secondReturned:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected second text delta to avoid blocking behind the first clause synthesis")
	}

	close(synth.releaseFirst)

	stream := planner.Finalize("当然可以，我先打开客厅灯。")
	if stream == nil {
		t.Fatal("expected planned audio stream after finalize")
	}
	defer stream.Close()
	drainTestAudioStream(t, stream)
}

type staticTurnExecutor struct {
	text string
}

func (e staticTurnExecutor) ExecuteTurn(context.Context, agent.TurnInput) (agent.TurnOutput, error) {
	return agent.TurnOutput{Text: e.text}, nil
}

func drainTestAudioStream(t *testing.T, stream AudioStream) {
	t.Helper()
	for {
		_, err := stream.Next(context.Background())
		if err == nil {
			continue
		}
		if err == io.EOF {
			return
		}
		t.Fatalf("unexpected audio stream error %v", err)
	}
}
