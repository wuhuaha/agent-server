package voice

import (
	"context"
	"testing"
	"time"

	"agent-server/internal/agent"
)

type previewTestResponder struct{}

func (previewTestResponder) Respond(context.Context, TurnRequest) (TurnResponse, error) {
	return TurnResponse{}, nil
}

func (previewTestResponder) StartInputPreview(context.Context, InputPreviewRequest) (InputPreviewSession, error) {
	return &previewTestSession{}, nil
}

type previewTestSession struct {
	seenAudio bool
	startedAt time.Time
}

func (s *previewTestSession) PushAudio(context.Context, []byte) (InputPreview, error) {
	s.seenAudio = true
	if s.startedAt.IsZero() {
		s.startedAt = time.Now()
	}
	return InputPreview{PartialText: "打开客厅灯", AudioBytes: 12800}, nil
}

func (s *previewTestSession) Poll(now time.Time) InputPreview {
	preview := InputPreview{PartialText: "打开客厅灯", AudioBytes: 12800, SpeechStarted: s.seenAudio}
	if s.seenAudio && !s.startedAt.IsZero() && now.Sub(s.startedAt) >= 400*time.Millisecond {
		preview.CommitSuggested = true
		preview.EndpointReason = "server_silence_timeout"
	}
	return preview
}

func (*previewTestSession) Close() error { return nil }

func TestSessionOrchestratorManagesInputPreview(t *testing.T) {
	orchestrator := NewSessionOrchestrator(nil)
	responder := previewTestResponder{}

	err := orchestrator.EnsureInputPreview(context.Background(), responder, InputPreviewRequest{
		SessionID:    "sess-preview",
		DeviceID:     "dev-1",
		Codec:        "pcm16le",
		SampleRateHz: 16000,
		Channels:     1,
	})
	if err != nil {
		t.Fatalf("EnsureInputPreview failed: %v", err)
	}

	observation, err := orchestrator.PushInputPreviewAudio(context.Background(), make([]byte, 12800))
	if err != nil {
		t.Fatalf("PushInputPreviewAudio failed: %v", err)
	}
	if !observation.Active || observation.Preview.PartialText == "" {
		t.Fatalf("expected active preview observation, got %+v", observation)
	}

	polled := orchestrator.PollInputPreview(time.Now().Add(800 * time.Millisecond))
	if !polled.CommitSuggested {
		t.Fatalf("expected commit suggestion after silence window, got %+v", polled)
	}
}

func TestSessionOrchestratorPersistsHeardTextOnInterrupt(t *testing.T) {
	store := agent.NewInMemoryMemoryStore(4)
	orchestrator := NewSessionOrchestrator(store)
	request := TurnRequest{
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		DeviceID:   "dev-1",
		ClientType: "rtos",
		Metadata:   map[string]string{"user_id": "alice"},
	}

	orchestrator.PrepareTurn(request, "打开客厅灯", "好的，已经为你打开客厅灯。")
	orchestrator.StartPlayback("好的，已经为你打开客厅灯。", 200*time.Millisecond, 2*time.Second)
	orchestrator.ObservePlaybackChunk()
	orchestrator.ObservePlaybackChunk()
	orchestrator.InterruptPlayback()

	memoryContext, err := store.LoadTurnContext(context.Background(), agent.MemoryQuery{DeviceID: "dev-1"})
	if err != nil {
		t.Fatalf("LoadTurnContext failed: %v", err)
	}
	if len(memoryContext.RecentMessages) != 2 {
		t.Fatalf("expected user plus heard assistant message, got %+v", memoryContext.RecentMessages)
	}
	heard := memoryContext.RecentMessages[1].Content
	if heard == "" || heard == "好的，已经为你打开客厅灯。" {
		t.Fatalf("expected truncated heard text after interrupt, got %q", heard)
	}
}

func TestSessionOrchestratorFinalizesTextOnlyTurn(t *testing.T) {
	store := agent.NewInMemoryMemoryStore(4)
	orchestrator := NewSessionOrchestrator(store)
	request := TurnRequest{
		SessionID:  "sess-2",
		TurnID:     "turn-2",
		DeviceID:   "dev-2",
		ClientType: "web",
	}

	orchestrator.PrepareTurn(request, "今天天气如何", "今天天气晴。")
	orchestrator.FinalizeTextResponse("今天天气晴。")

	memoryContext, err := store.LoadTurnContext(context.Background(), agent.MemoryQuery{DeviceID: "dev-2"})
	if err != nil {
		t.Fatalf("LoadTurnContext failed: %v", err)
	}
	if len(memoryContext.RecentMessages) != 2 {
		t.Fatalf("expected text-only turn to persist full assistant reply, got %+v", memoryContext.RecentMessages)
	}
	if got := memoryContext.RecentMessages[1].Content; got != "今天天气晴。" {
		t.Fatalf("expected full text response, got %q", got)
	}
}
