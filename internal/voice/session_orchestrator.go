package voice

import (
	"context"
	"math"
	"strings"
	"sync"
	"time"

	"agent-server/internal/agent"
)

const (
	InputPreviewPollInterval = 80 * time.Millisecond
	approxHeardRuneDuration  = 120 * time.Millisecond
)

type SessionOrchestratorProvider interface {
	NewSessionOrchestrator() *SessionOrchestrator
}

type SessionOrchestrator struct {
	mu          sync.Mutex
	memoryStore agent.MemoryStore
	preview     *inputTurnPreview
	turn        *orchestratedTurn
}

type inputTurnPreview struct {
	session          InputPreviewSession
	last             InputPreview
	lastPartialText  string
	lastCommitLogged bool
}

type InputPreviewObservation struct {
	Preview         InputPreview
	Active          bool
	PartialChanged  bool
	CommitSuggested bool
}

type orchestratedTurn struct {
	request             TurnRequest
	inputText           string
	metadata            map[string]string
	responseText        string
	deliveredText       string
	heardText           string
	playbackActive      bool
	playbackCompleted   bool
	responseInterrupted bool
	responseTruncated   bool
	chunkDuration       time.Duration
	plannedDuration     time.Duration
	playedDuration      time.Duration
}

func NewSessionOrchestrator(memoryStore agent.MemoryStore) *SessionOrchestrator {
	if memoryStore == nil {
		memoryStore = agent.NewNoopMemoryStore()
	}
	return &SessionOrchestrator{memoryStore: memoryStore}
}

func NewSessionOrchestratorFromResponder(responder Responder) *SessionOrchestrator {
	if provider, ok := responder.(SessionOrchestratorProvider); ok {
		if orchestrator := provider.NewSessionOrchestrator(); orchestrator != nil {
			return orchestrator
		}
	}
	return NewSessionOrchestrator(nil)
}

func (o *SessionOrchestrator) EnsureInputPreview(ctx context.Context, responder Responder, req InputPreviewRequest) error {
	o.mu.Lock()
	if o.preview != nil {
		o.mu.Unlock()
		return nil
	}
	o.mu.Unlock()

	previewer, ok := responder.(InputPreviewer)
	if !ok {
		return nil
	}
	session, err := previewer.StartInputPreview(ctx, req)
	if err != nil {
		return err
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.preview != nil {
		_ = session.Close()
		return nil
	}
	o.preview = &inputTurnPreview{session: session}
	return nil
}

func (o *SessionOrchestrator) PushInputPreviewAudio(ctx context.Context, payload []byte) (InputPreviewObservation, error) {
	o.mu.Lock()
	preview := o.preview
	o.mu.Unlock()
	if preview == nil || preview.session == nil {
		return InputPreviewObservation{}, nil
	}

	snapshot, err := preview.session.PushAudio(ctx, payload)
	if err != nil {
		return InputPreviewObservation{}, err
	}

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.preview == nil {
		return InputPreviewObservation{Preview: snapshot}, nil
	}
	changed := snapshot.PartialText != "" && snapshot.PartialText != o.preview.lastPartialText
	o.preview.last = snapshot
	if changed {
		o.preview.lastPartialText = snapshot.PartialText
	}
	return InputPreviewObservation{
		Preview:        snapshot,
		Active:         true,
		PartialChanged: changed,
	}, nil
}

func (o *SessionOrchestrator) PollInputPreview(now time.Time) InputPreviewObservation {
	o.mu.Lock()
	preview := o.preview
	o.mu.Unlock()
	if preview == nil || preview.session == nil {
		return InputPreviewObservation{}
	}

	snapshot := preview.session.Poll(now)

	o.mu.Lock()
	defer o.mu.Unlock()
	if o.preview == nil {
		return InputPreviewObservation{Preview: snapshot}
	}
	changed := snapshot.PartialText != "" && snapshot.PartialText != o.preview.lastPartialText
	if changed {
		o.preview.lastPartialText = snapshot.PartialText
	}
	commitNew := snapshot.CommitSuggested && !o.preview.lastCommitLogged
	if commitNew {
		o.preview.lastCommitLogged = true
	}
	o.preview.last = snapshot
	return InputPreviewObservation{
		Preview:         snapshot,
		Active:          true,
		PartialChanged:  changed,
		CommitSuggested: commitNew,
	}
}

func (o *SessionOrchestrator) ClearInputPreview() {
	o.mu.Lock()
	preview := o.preview
	o.preview = nil
	o.mu.Unlock()
	if preview != nil && preview.session != nil {
		_ = preview.session.Close()
	}
}

func (o *SessionOrchestrator) PreviewReadDeadline(now time.Time) time.Time {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.preview == nil || o.preview.session == nil {
		return time.Time{}
	}
	return now.Add(InputPreviewPollInterval)
}

func (o *SessionOrchestrator) PrepareTurn(request TurnRequest, inputText, responseText string) {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.turn = &orchestratedTurn{
		request:      cloneTurnRequest(request),
		inputText:    strings.TrimSpace(inputText),
		metadata:     cloneStringMap(request.Metadata),
		responseText: strings.TrimSpace(responseText),
	}
	o.persistLocked()
}

func (o *SessionOrchestrator) FinalizeTextResponse(deliveredText string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil {
		return
	}
	deliveredText = strings.TrimSpace(deliveredText)
	o.turn.deliveredText = deliveredText
	o.turn.heardText = deliveredText
	o.turn.playbackCompleted = true
	o.turn.playbackActive = false
	o.persistLocked()
	o.turn = nil
}

func (o *SessionOrchestrator) StartPlayback(deliveredText string, chunkDuration, plannedDuration time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil {
		return
	}
	o.turn.deliveredText = strings.TrimSpace(deliveredText)
	o.turn.chunkDuration = chunkDuration
	o.turn.plannedDuration = plannedDuration
	o.turn.playedDuration = 0
	o.turn.playbackActive = true
	o.turn.playbackCompleted = false
	o.turn.responseInterrupted = false
	o.turn.responseTruncated = false
	o.turn.heardText = ""
	o.persistLocked()
}

func (o *SessionOrchestrator) ObservePlaybackChunk() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil || !o.turn.playbackActive {
		return
	}
	o.turn.playedDuration += o.turn.chunkDuration
	heard := heardTextForPlayback(o.turn.deliveredText, o.turn.playedDuration, o.turn.plannedDuration)
	if heard == o.turn.heardText {
		return
	}
	o.turn.heardText = heard
	o.persistLocked()
}

func (o *SessionOrchestrator) InterruptPlayback() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil || !o.turn.playbackActive {
		return
	}
	heard := heardTextForPlayback(o.turn.deliveredText, o.turn.playedDuration, o.turn.plannedDuration)
	o.turn.heardText = heard
	o.turn.playbackActive = false
	o.turn.playbackCompleted = false
	o.turn.responseInterrupted = true
	o.turn.responseTruncated = strings.TrimSpace(o.turn.deliveredText) != "" && strings.TrimSpace(o.turn.deliveredText) != strings.TrimSpace(heard)
	o.persistLocked()
	o.turn = nil
}

func (o *SessionOrchestrator) CompletePlayback() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil {
		return
	}
	o.turn.playbackActive = false
	o.turn.playbackCompleted = true
	if strings.TrimSpace(o.turn.deliveredText) != "" {
		o.turn.heardText = o.turn.deliveredText
	}
	o.turn.responseInterrupted = false
	o.turn.responseTruncated = false
	o.persistLocked()
	o.turn = nil
}

func (o *SessionOrchestrator) persistLocked() {
	if o.turn == nil {
		return
	}
	record := agent.BuildMemoryRecord(
		turnInputFromRequest(o.turn.request, o.turn.inputText),
		o.turn.inputText,
		o.turn.responseText,
		cloneStringMap(o.turn.metadata),
	)
	record.DeliveredText = o.turn.deliveredText
	record.HeardText = o.turn.heardText
	record.ResponseInterrupted = o.turn.responseInterrupted
	record.ResponseTruncated = o.turn.responseTruncated
	record.PlaybackCompleted = o.turn.playbackCompleted
	_ = o.memoryStore.SaveTurn(context.Background(), record)
}

func heardTextForPlayback(text string, playedDuration, plannedDuration time.Duration) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) == 0 || playedDuration <= 0 {
		return ""
	}
	if plannedDuration > 0 {
		ratio := float64(playedDuration) / float64(plannedDuration)
		if ratio >= 1 {
			return trimmed
		}
		count := int(math.Ceil(float64(len(runes)) * ratio))
		if count <= 0 {
			count = 1
		}
		if count > len(runes) {
			count = len(runes)
		}
		return string(runes[:count])
	}

	count := int(playedDuration / approxHeardRuneDuration)
	if count <= 0 {
		count = 1
	}
	if count > len(runes) {
		count = len(runes)
	}
	return string(runes[:count])
}

func cloneTurnRequest(request TurnRequest) TurnRequest {
	cloned := request
	cloned.AudioPCM = append([]byte(nil), request.AudioPCM...)
	cloned.Metadata = cloneStringMap(request.Metadata)
	return cloned
}
