package voice

import (
	"context"
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"agent-server/internal/agent"
)

const (
	InputPreviewPollInterval = 80 * time.Millisecond
	approxHeardRuneDuration  = 120 * time.Millisecond

	interruptionPolicyMetadataKey = "voice.interruption_policy"
	interruptionReasonMetadataKey = "voice.interruption_reason"
	heardTextBoundaryMetadataKey  = "voice.heard_text_boundary"
	playedDurationMetadataKey     = "voice.played_duration_ms"
	plannedDurationMetadataKey    = "voice.planned_duration_ms"
)

type HeardTextBoundary string

const (
	HeardTextBoundaryNone   HeardTextBoundary = "none"
	HeardTextBoundaryPrefix HeardTextBoundary = "prefix"
	HeardTextBoundaryFull   HeardTextBoundary = "full"
)

type PlaybackInterruption struct {
	Policy            InterruptionPolicy
	Reason            string
	HeardText         string
	HeardTextBoundary HeardTextBoundary
	PlayedDuration    time.Duration
	PlannedDuration   time.Duration
	Truncated         bool
}

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
	interruptionPolicy  InterruptionPolicy
	interruptionReason  string
	heardTextBoundary   HeardTextBoundary
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

	if o.turn != nil && o.turn.request.TurnID == request.TurnID {
		o.turn.request = cloneTurnRequest(request)
		o.turn.metadata = cloneStringMap(request.Metadata)
		if trimmed := strings.TrimSpace(inputText); trimmed != "" {
			o.turn.inputText = trimmed
		}
		if trimmed := strings.TrimSpace(responseText); trimmed != "" {
			o.turn.responseText = trimmed
		}
		o.persistLocked()
		return
	}

	o.turn = &orchestratedTurn{
		request:           cloneTurnRequest(request),
		inputText:         strings.TrimSpace(inputText),
		metadata:          cloneStringMap(request.Metadata),
		responseText:      strings.TrimSpace(responseText),
		heardTextBoundary: HeardTextBoundaryNone,
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
	o.turn.heardTextBoundary = heardTextBoundaryForTexts(deliveredText, deliveredText)
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
	o.turn.heardTextBoundary = HeardTextBoundaryNone
	o.turn.interruptionPolicy = InterruptionPolicyIgnore
	o.turn.interruptionReason = ""
}

func (o *SessionOrchestrator) UpdatePlayback(deliveredText string, plannedDuration time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil {
		return
	}
	if trimmed := strings.TrimSpace(deliveredText); trimmed != "" {
		o.turn.deliveredText = trimmed
	}
	if plannedDuration > 0 {
		o.turn.plannedDuration = plannedDuration
	}
	if o.turn.playbackActive {
		o.turn.heardText = heardTextForPlayback(o.turn.deliveredText, o.turn.playedDuration, o.turn.plannedDuration)
		o.turn.heardTextBoundary = heardTextBoundaryForTexts(o.turn.deliveredText, o.turn.heardText)
	}
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
	o.turn.heardTextBoundary = heardTextBoundaryForTexts(o.turn.deliveredText, heard)
}

func (o *SessionOrchestrator) InterruptPlayback() {
	o.InterruptPlaybackWithPolicy(InterruptionPolicyHardInterrupt, "hard_interrupt")
}

func (o *SessionOrchestrator) RecordInterruptionDecision(decision BargeInDecision) {
	o.RecordInterruptionPolicy(decision.Policy, decision.Reason)
}

func (o *SessionOrchestrator) PlaybackDirectiveForDecision(decision BargeInDecision) PlaybackDirective {
	return decision.PlaybackDirective()
}

func (o *SessionOrchestrator) RecordInterruptionPolicy(policy InterruptionPolicy, reason string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil {
		return
	}
	recordInterruptionPolicyLocked(o.turn, policy, reason)
}

func (o *SessionOrchestrator) InterruptPlaybackWithDecision(decision BargeInDecision) PlaybackInterruption {
	return o.InterruptPlaybackWithPolicy(decision.Policy, decision.Reason)
}

func (o *SessionOrchestrator) InterruptPlaybackWithPolicy(policy InterruptionPolicy, reason string) PlaybackInterruption {
	o.mu.Lock()
	defer o.mu.Unlock()

	result := PlaybackInterruption{
		Policy: normalizeInterruptionPolicy(policy),
		Reason: strings.TrimSpace(reason),
	}
	if result.Policy == InterruptionPolicyIgnore {
		result.Policy = InterruptionPolicyHardInterrupt
	}
	if result.Reason == "" {
		result.Reason = string(result.Policy)
	}
	if o.turn == nil || !o.turn.playbackActive {
		return result
	}

	recordInterruptionPolicyLocked(o.turn, result.Policy, result.Reason)
	heard := heardTextForPlayback(o.turn.deliveredText, o.turn.playedDuration, o.turn.plannedDuration)
	boundary := heardTextBoundaryForTexts(o.turn.deliveredText, heard)
	o.turn.heardText = heard
	o.turn.heardTextBoundary = boundary
	o.turn.playbackActive = false
	o.turn.playbackCompleted = false
	o.turn.responseInterrupted = true
	o.turn.responseTruncated = boundary == HeardTextBoundaryPrefix
	o.persistLocked()

	result.HeardText = heard
	result.HeardTextBoundary = boundary
	result.PlayedDuration = o.turn.playedDuration
	result.PlannedDuration = o.turn.plannedDuration
	result.Truncated = o.turn.responseTruncated
	o.turn = nil
	return result
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
	o.turn.heardTextBoundary = heardTextBoundaryForTexts(o.turn.deliveredText, o.turn.heardText)
	o.turn.responseInterrupted = false
	o.turn.responseTruncated = false
	o.persistLocked()
	o.turn = nil
}

func (o *SessionOrchestrator) persistLocked() {
	if o.turn == nil {
		return
	}
	metadata := enrichPlaybackMetadata(cloneStringMap(o.turn.metadata), o.turn)
	record := agent.BuildMemoryRecord(
		turnInputFromRequest(o.turn.request, o.turn.inputText),
		o.turn.inputText,
		o.turn.responseText,
		metadata,
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

func normalizeInterruptionPolicy(policy InterruptionPolicy) InterruptionPolicy {
	switch policy {
	case InterruptionPolicyBackchannel, InterruptionPolicyDuckOnly, InterruptionPolicyHardInterrupt:
		return policy
	default:
		return InterruptionPolicyIgnore
	}
}

func recordInterruptionPolicyLocked(turn *orchestratedTurn, policy InterruptionPolicy, reason string) {
	if turn == nil {
		return
	}
	normalized := normalizeInterruptionPolicy(policy)
	if normalized == InterruptionPolicyIgnore {
		return
	}
	turn.interruptionPolicy = normalized
	turn.interruptionReason = strings.TrimSpace(reason)
	if turn.interruptionReason == "" {
		turn.interruptionReason = string(normalized)
	}
}

func heardTextBoundaryForTexts(deliveredText, heardText string) HeardTextBoundary {
	delivered := strings.TrimSpace(deliveredText)
	heard := strings.TrimSpace(heardText)
	switch {
	case heard == "":
		return HeardTextBoundaryNone
	case delivered == "" || heard == delivered:
		return HeardTextBoundaryFull
	default:
		return HeardTextBoundaryPrefix
	}
}

func enrichPlaybackMetadata(metadata map[string]string, turn *orchestratedTurn) map[string]string {
	if turn == nil {
		return metadata
	}
	put := func(key, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		if metadata == nil {
			metadata = make(map[string]string, 6)
		}
		metadata[key] = value
	}

	if policy := normalizeInterruptionPolicy(turn.interruptionPolicy); policy != InterruptionPolicyIgnore {
		put(interruptionPolicyMetadataKey, string(policy))
		put(interruptionReasonMetadataKey, turn.interruptionReason)
	}
	if turn.responseInterrupted || turn.playbackCompleted || normalizeInterruptionPolicy(turn.interruptionPolicy) != InterruptionPolicyIgnore {
		put(heardTextBoundaryMetadataKey, string(turn.heardTextBoundary))
	}
	if turn.playedDuration > 0 && (turn.responseInterrupted || turn.playbackCompleted) {
		put(playedDurationMetadataKey, strconv.FormatInt(turn.playedDuration.Milliseconds(), 10))
	}
	if turn.plannedDuration > 0 && (turn.responseInterrupted || turn.playbackCompleted) {
		put(plannedDurationMetadataKey, strconv.FormatInt(turn.plannedDuration.Milliseconds(), 10))
	}
	return metadata
}

func cloneTurnRequest(request TurnRequest) TurnRequest {
	cloned := request
	cloned.AudioPCM = nil
	cloned.Metadata = cloneStringMap(request.Metadata)
	return cloned
}
