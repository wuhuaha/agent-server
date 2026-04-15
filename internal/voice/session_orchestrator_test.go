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
	orchestrator.RecordInterruptionDecision(BargeInDecision{
		Policy: InterruptionPolicyHardInterrupt,
		Reason: "accepted_complete_preview",
	})
	summary := orchestrator.InterruptPlaybackWithDecision(BargeInDecision{
		Policy: InterruptionPolicyHardInterrupt,
		Reason: "accepted_complete_preview",
	})
	if summary.Policy != InterruptionPolicyHardInterrupt || summary.HeardTextBoundary != HeardTextBoundaryPrefix {
		t.Fatalf("expected hard interrupt summary with prefix boundary, got %+v", summary)
	}

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

type countingMemoryStore struct {
	saves []agent.MemoryRecord
}

func (s *countingMemoryStore) LoadTurnContext(context.Context, agent.MemoryQuery) (agent.MemoryContext, error) {
	return agent.MemoryContext{}, nil
}

func (s *countingMemoryStore) SaveTurn(_ context.Context, record agent.MemoryRecord) error {
	s.saves = append(s.saves, record)
	return nil
}

func TestSessionOrchestratorPlaybackPersistsOnlyAtStableBoundaries(t *testing.T) {
	store := &countingMemoryStore{}
	orchestrator := NewSessionOrchestrator(store)
	request := TurnRequest{
		SessionID:  "sess-3",
		TurnID:     "turn-3",
		DeviceID:   "dev-3",
		ClientType: "rtos",
		AudioPCM:   make([]byte, 6400),
	}

	orchestrator.PrepareTurn(request, "打开主灯", "好的，已经为你打开主灯。")
	if got := len(store.saves); got != 1 {
		t.Fatalf("expected one save after prepare, got %d", got)
	}

	orchestrator.StartPlayback("好的，已经为你打开主灯。", 200*time.Millisecond, 2*time.Second)
	orchestrator.ObservePlaybackChunk()
	orchestrator.ObservePlaybackChunk()
	if got := len(store.saves); got != 1 {
		t.Fatalf("expected playback progress to stay in-memory only, got %d saves", got)
	}

	orchestrator.RecordInterruptionDecision(BargeInDecision{
		Policy: InterruptionPolicyHardInterrupt,
		Reason: "accepted_incomplete_after_hold",
	})
	outcome := orchestrator.InterruptPlaybackWithPolicy(InterruptionPolicyHardInterrupt, "accepted_incomplete_after_hold")
	if got := len(store.saves); got != 2 {
		t.Fatalf("expected interrupt to persist heard text once, got %d saves", got)
	}
	if !store.saves[1].ResponseInterrupted || !store.saves[1].ResponseTruncated {
		t.Fatalf("expected interrupted truncated record, got %+v", store.saves[1])
	}
	if outcome.HeardTextBoundary != HeardTextBoundaryPrefix || !outcome.Truncated {
		t.Fatalf("expected prefix heard-text boundary after interrupt, got %+v", outcome)
	}
	if got := store.saves[1].Metadata[interruptionPolicyMetadataKey]; got != string(InterruptionPolicyHardInterrupt) {
		t.Fatalf("expected interruption policy metadata, got %q", got)
	}
	if got := store.saves[1].Metadata[interruptionReasonMetadataKey]; got != "accepted_incomplete_after_hold" {
		t.Fatalf("expected interruption reason metadata, got %q", got)
	}
	if got := store.saves[1].Metadata[heardTextBoundaryMetadataKey]; got != string(HeardTextBoundaryPrefix) {
		t.Fatalf("expected heard boundary metadata, got %q", got)
	}
}

func TestSessionOrchestratorPersistsBackchannelPolicyWithoutInterrupt(t *testing.T) {
	store := &countingMemoryStore{}
	orchestrator := NewSessionOrchestrator(store)
	request := TurnRequest{
		SessionID:  "sess-4",
		TurnID:     "turn-4",
		DeviceID:   "dev-4",
		ClientType: "rtos",
	}

	orchestrator.PrepareTurn(request, "打开主灯", "好的，已经为你打开主灯。")
	orchestrator.StartPlayback("好的，已经为你打开主灯。", 200*time.Millisecond, 2*time.Second)
	orchestrator.RecordInterruptionDecision(BargeInDecision{
		Policy: InterruptionPolicyBackchannel,
		Reason: "backchannel_short_ack",
	})
	orchestrator.CompletePlayback()

	if got := len(store.saves); got != 2 {
		t.Fatalf("expected prepare + complete saves, got %d", got)
	}
	record := store.saves[1]
	if record.ResponseInterrupted || record.ResponseTruncated {
		t.Fatalf("expected backchannel to avoid interruption flags, got %+v", record)
	}
	if got := record.Metadata[interruptionPolicyMetadataKey]; got != string(InterruptionPolicyBackchannel) {
		t.Fatalf("expected backchannel policy metadata, got %q", got)
	}
	if got := record.Metadata[heardTextBoundaryMetadataKey]; got != string(HeardTextBoundaryFull) {
		t.Fatalf("expected full heard boundary on completed playback, got %q", got)
	}
}
