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
	heardSourceMetadataKey        = "voice.heard_source"
	heardConfidenceMetadataKey    = "voice.heard_confidence"
	heardPrecisionTierMetadataKey = "voice.heard_precision_tier"
	resumeAnchorMetadataKey       = "voice.resume_anchor"
	missedTextMetadataKey         = "voice.missed_text"
	heardRatioMetadataKey         = "voice.heard_ratio_pct"

	previousPlaybackAvailableMetadataKey     = "voice.previous.available"
	previousPlaybackTurnIDMetadataKey        = "voice.previous.turn_id"
	previousPlaybackDeliveredTextMetadataKey = "voice.previous.delivered_text"
	previousPlaybackHeardTextMetadataKey     = "voice.previous.heard_text"
	previousPlaybackMissedTextMetadataKey    = "voice.previous.missed_text"
	previousPlaybackResumeAnchorMetadataKey  = "voice.previous.resume_anchor"
	previousPlaybackBoundaryMetadataKey      = "voice.previous.heard_boundary"
	previousPlaybackSourceMetadataKey        = "voice.previous.heard_source"
	previousPlaybackConfidenceMetadataKey    = "voice.previous.heard_confidence"
	previousPlaybackPrecisionMetadataKey     = "voice.previous.heard_precision_tier"
	previousPlaybackRatioMetadataKey         = "voice.previous.heard_ratio_pct"
	previousPlaybackCompletedMetadataKey     = "voice.previous.playback_completed"
	previousPlaybackInterruptedMetadataKey   = "voice.previous.response_interrupted"
	previousPlaybackTruncatedMetadataKey     = "voice.previous.response_truncated"
	previousPlaybackPolicyMetadataKey        = "voice.previous.interruption_policy"
	previousPlaybackReasonMetadataKey        = "voice.previous.interruption_reason"
)

type HeardTextSource string

const (
	HeardTextSourceUnknown           HeardTextSource = ""
	HeardTextSourceHeuristicBytes    HeardTextSource = "heuristic_bytes"
	HeardTextSourcePlaybackStarted   HeardTextSource = "playback_started"
	HeardTextSourceSegmentMark       HeardTextSource = "segment_mark"
	HeardTextSourcePlaybackCompleted HeardTextSource = "playback_completed"
)

type HeardTextConfidence string

const (
	HeardTextConfidenceUnknown HeardTextConfidence = ""
	HeardTextConfidenceLow     HeardTextConfidence = "low"
	HeardTextConfidenceMedium  HeardTextConfidence = "medium"
	HeardTextConfidenceHigh    HeardTextConfidence = "high"
)

type HeardTextPrecisionTier string

const (
	HeardTextPrecisionTierUnknown HeardTextPrecisionTier = ""
	HeardTextPrecisionTierTier0   HeardTextPrecisionTier = "tier0_heuristic"
	HeardTextPrecisionTierTier1   HeardTextPrecisionTier = "tier1_segment_mark"
)

type PlaybackStartOptions struct {
	PreferClientFacts bool
}

type PlaybackOutcome struct {
	TurnID              string
	DeliveredText       string
	HeardText           string
	MissedText          string
	ResumeAnchor        string
	HeardTextBoundary   HeardTextBoundary
	HeardSource         HeardTextSource
	HeardConfidence     HeardTextConfidence
	HeardPrecisionTier  HeardTextPrecisionTier
	HeardRatioPercent   int
	PlaybackCompleted   bool
	ResponseInterrupted bool
	ResponseTruncated   bool
	InterruptionPolicy  InterruptionPolicy
	InterruptionReason  string
	PlayedDuration      time.Duration
	PlannedDuration     time.Duration
}

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
	mu                  sync.Mutex
	memoryStore         agent.MemoryStore
	preview             *inputTurnPreview
	turn                *orchestratedTurn
	lastPlaybackOutcome *PlaybackOutcome
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
	heardSource         HeardTextSource
	heardConfidence     HeardTextConfidence
	heardPrecisionTier  HeardTextPrecisionTier
	preferClientFacts   bool
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

func (o *SessionOrchestrator) FinalizeInputPreview(ctx context.Context) (TranscriptionResult, bool, error) {
	o.mu.Lock()
	preview := o.preview
	o.preview = nil
	o.mu.Unlock()
	if preview == nil || preview.session == nil {
		return TranscriptionResult{}, false, nil
	}
	finalizer, ok := preview.session.(FinalizingInputPreviewSession)
	if !ok {
		_ = preview.session.Close()
		return TranscriptionResult{}, false, nil
	}
	result, err := finalizer.Finish(ctx)
	if closeErr := preview.session.Close(); err == nil && closeErr != nil {
		err = closeErr
	}
	return result, true, err
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
	o.captureLastPlaybackOutcomeLocked()
	o.turn = nil
}

func (o *SessionOrchestrator) StartPlayback(deliveredText string, chunkDuration, plannedDuration time.Duration) {
	o.StartPlaybackWithOptions(deliveredText, chunkDuration, plannedDuration, PlaybackStartOptions{})
}

func (o *SessionOrchestrator) StartPlaybackWithOptions(deliveredText string, chunkDuration, plannedDuration time.Duration, opts PlaybackStartOptions) {
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
	o.turn.heardSource = HeardTextSourceUnknown
	o.turn.heardConfidence = HeardTextConfidenceUnknown
	o.turn.heardPrecisionTier = HeardTextPrecisionTierUnknown
	o.turn.preferClientFacts = opts.PreferClientFacts
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
		o.updateHeardTextLocked(o.turn.playedDuration, o.turn.heardSource)
	}
	o.persistLocked()
}

func (o *SessionOrchestrator) ObservePlaybackChunk() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil || !o.turn.playbackActive || o.turn.preferClientFacts {
		return
	}
	o.updateHeardTextLocked(o.turn.playedDuration+o.turn.chunkDuration, HeardTextSourceHeuristicBytes)
}

func (o *SessionOrchestrator) ObservePlaybackStartedFact() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil || !o.turn.playbackActive {
		return
	}
	o.turn.heardSource = HeardTextSourcePlaybackStarted
	o.turn.heardConfidence = HeardTextConfidenceMedium
	o.turn.heardPrecisionTier = HeardTextPrecisionTierTier1
}

func (o *SessionOrchestrator) ObservePlaybackMarkFact(playedDuration time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil || !o.turn.playbackActive {
		return
	}
	o.updateHeardTextLocked(playedDuration, HeardTextSourceSegmentMark)
}

func (o *SessionOrchestrator) ObservePlaybackCompletedFact() {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil {
		return
	}
	o.updateHeardTextLocked(maxDuration(o.turn.plannedDuration, o.turn.playedDuration), HeardTextSourcePlaybackCompleted)
	if strings.TrimSpace(o.turn.deliveredText) != "" {
		o.turn.heardText = o.turn.deliveredText
	}
	o.turn.heardTextBoundary = heardTextBoundaryForTexts(o.turn.deliveredText, o.turn.heardText)
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
	o.updateHeardTextLocked(o.turn.playedDuration, o.turn.heardSource)
	heard := o.turn.heardText
	boundary := o.turn.heardTextBoundary
	o.turn.playbackActive = false
	o.turn.playbackCompleted = false
	o.turn.responseInterrupted = true
	o.turn.responseTruncated = boundary == HeardTextBoundaryPrefix
	o.persistLocked()
	o.captureLastPlaybackOutcomeLocked()

	result.HeardText = heard
	result.HeardTextBoundary = boundary
	result.PlayedDuration = o.turn.playedDuration
	result.PlannedDuration = o.turn.plannedDuration
	result.Truncated = o.turn.responseTruncated
	o.turn = nil
	return result
}

func (o *SessionOrchestrator) CompletePlayback() {
	o.CompletePlaybackWithSource(HeardTextSourceHeuristicBytes)
}

func (o *SessionOrchestrator) CompletePlaybackWithSource(source HeardTextSource) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.turn == nil {
		return
	}
	o.turn.playbackActive = false
	o.turn.playbackCompleted = true
	playedDuration := o.turn.playedDuration
	if normalizeHeardTextSource(source, HeardTextSourceUnknown) == HeardTextSourcePlaybackCompleted {
		playedDuration = maxDuration(o.turn.plannedDuration, playedDuration)
	}
	o.updateHeardTextLocked(playedDuration, source)
	if strings.TrimSpace(o.turn.deliveredText) != "" {
		o.turn.heardText = o.turn.deliveredText
	}
	o.turn.heardTextBoundary = heardTextBoundaryForTexts(o.turn.deliveredText, o.turn.heardText)
	o.turn.responseInterrupted = false
	o.turn.responseTruncated = false
	o.persistLocked()
	o.captureLastPlaybackOutcomeLocked()
	o.turn = nil
}

func (o *SessionOrchestrator) LastPlaybackOutcome() (PlaybackOutcome, bool) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lastPlaybackOutcome == nil {
		return PlaybackOutcome{}, false
	}
	return *o.lastPlaybackOutcome, true
}

func (o *SessionOrchestrator) LastPlaybackContextMetadata() map[string]string {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.lastPlaybackOutcome == nil {
		return nil
	}
	return playbackOutcomeContextMetadata(*o.lastPlaybackOutcome)
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

func (o *SessionOrchestrator) captureLastPlaybackOutcomeLocked() {
	if o.turn == nil {
		return
	}
	outcome := playbackOutcomeFromTurn(o.turn)
	o.lastPlaybackOutcome = &outcome
}

func playbackOutcomeFromTurn(turn *orchestratedTurn) PlaybackOutcome {
	if turn == nil {
		return PlaybackOutcome{}
	}
	return PlaybackOutcome{
		TurnID:              strings.TrimSpace(turn.request.TurnID),
		DeliveredText:       strings.TrimSpace(turn.deliveredText),
		HeardText:           strings.TrimSpace(turn.heardText),
		MissedText:          missedTextForTexts(turn.deliveredText, turn.heardText),
		ResumeAnchor:        resumeAnchorForTexts(turn.deliveredText, turn.heardText),
		HeardTextBoundary:   heardTextBoundaryForTexts(turn.deliveredText, turn.heardText),
		HeardSource:         normalizeHeardTextSource(turn.heardSource, HeardTextSourceUnknown),
		HeardConfidence:     heardTextConfidenceForSource(turn.heardSource),
		HeardPrecisionTier:  heardTextPrecisionTierForSource(turn.heardSource),
		HeardRatioPercent:   heardRatioPercent(turn.deliveredText, turn.heardText),
		PlaybackCompleted:   turn.playbackCompleted,
		ResponseInterrupted: turn.responseInterrupted,
		ResponseTruncated:   turn.responseTruncated,
		InterruptionPolicy:  normalizeInterruptionPolicy(turn.interruptionPolicy),
		InterruptionReason:  strings.TrimSpace(turn.interruptionReason),
		PlayedDuration:      turn.playedDuration,
		PlannedDuration:     turn.plannedDuration,
	}
}

func playbackOutcomeContextMetadata(outcome PlaybackOutcome) map[string]string {
	metadata := make(map[string]string, 16)
	put := func(key, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			metadata[key] = trimmed
		}
	}

	metadata[previousPlaybackAvailableMetadataKey] = "true"
	put(previousPlaybackTurnIDMetadataKey, outcome.TurnID)
	put(previousPlaybackDeliveredTextMetadataKey, outcome.DeliveredText)
	put(previousPlaybackHeardTextMetadataKey, outcome.HeardText)
	put(previousPlaybackMissedTextMetadataKey, outcome.MissedText)
	put(previousPlaybackResumeAnchorMetadataKey, outcome.ResumeAnchor)
	if boundary := strings.TrimSpace(string(outcome.HeardTextBoundary)); boundary != "" {
		metadata[previousPlaybackBoundaryMetadataKey] = boundary
	}
	if source := strings.TrimSpace(string(outcome.HeardSource)); source != "" {
		metadata[previousPlaybackSourceMetadataKey] = source
	}
	if confidence := strings.TrimSpace(string(outcome.HeardConfidence)); confidence != "" {
		metadata[previousPlaybackConfidenceMetadataKey] = confidence
	}
	if precision := strings.TrimSpace(string(outcome.HeardPrecisionTier)); precision != "" {
		metadata[previousPlaybackPrecisionMetadataKey] = precision
	}
	if outcome.HeardRatioPercent >= 0 {
		metadata[previousPlaybackRatioMetadataKey] = strconv.Itoa(outcome.HeardRatioPercent)
	}
	metadata[previousPlaybackCompletedMetadataKey] = strconv.FormatBool(outcome.PlaybackCompleted)
	metadata[previousPlaybackInterruptedMetadataKey] = strconv.FormatBool(outcome.ResponseInterrupted)
	metadata[previousPlaybackTruncatedMetadataKey] = strconv.FormatBool(outcome.ResponseTruncated)
	if policy := strings.TrimSpace(string(outcome.InterruptionPolicy)); policy != "" {
		metadata[previousPlaybackPolicyMetadataKey] = policy
	}
	put(previousPlaybackReasonMetadataKey, outcome.InterruptionReason)
	return metadata
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

func (o *SessionOrchestrator) updateHeardTextLocked(playedDuration time.Duration, source HeardTextSource) {
	if o.turn == nil {
		return
	}
	if playedDuration > o.turn.playedDuration {
		o.turn.playedDuration = playedDuration
	}
	heard := heardTextForPlayback(o.turn.deliveredText, o.turn.playedDuration, o.turn.plannedDuration)
	o.turn.heardText = heard
	o.turn.heardTextBoundary = heardTextBoundaryForTexts(o.turn.deliveredText, heard)
	o.turn.heardSource = normalizeHeardTextSource(source, o.turn.heardSource)
	o.turn.heardConfidence = heardTextConfidenceForSource(o.turn.heardSource)
	o.turn.heardPrecisionTier = heardTextPrecisionTierForSource(o.turn.heardSource)
}

func normalizeHeardTextSource(source, fallback HeardTextSource) HeardTextSource {
	switch source {
	case HeardTextSourceHeuristicBytes, HeardTextSourcePlaybackStarted, HeardTextSourceSegmentMark, HeardTextSourcePlaybackCompleted:
		return source
	default:
		return fallback
	}
}

func heardTextConfidenceForSource(source HeardTextSource) HeardTextConfidence {
	switch source {
	case HeardTextSourcePlaybackCompleted:
		return HeardTextConfidenceHigh
	case HeardTextSourcePlaybackStarted, HeardTextSourceSegmentMark:
		return HeardTextConfidenceMedium
	case HeardTextSourceHeuristicBytes:
		return HeardTextConfidenceLow
	default:
		return HeardTextConfidenceUnknown
	}
}

func heardTextPrecisionTierForSource(source HeardTextSource) HeardTextPrecisionTier {
	switch source {
	case HeardTextSourcePlaybackStarted, HeardTextSourceSegmentMark, HeardTextSourcePlaybackCompleted:
		return HeardTextPrecisionTierTier1
	case HeardTextSourceHeuristicBytes:
		return HeardTextPrecisionTierTier0
	default:
		return HeardTextPrecisionTierUnknown
	}
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
	if source := normalizeHeardTextSource(turn.heardSource, HeardTextSourceUnknown); source != HeardTextSourceUnknown {
		put(heardSourceMetadataKey, string(source))
	}
	if confidence := heardTextConfidenceForSource(turn.heardSource); confidence != HeardTextConfidenceUnknown {
		put(heardConfidenceMetadataKey, string(confidence))
	}
	if precision := heardTextPrecisionTierForSource(turn.heardSource); precision != HeardTextPrecisionTierUnknown {
		put(heardPrecisionTierMetadataKey, string(precision))
	}
	if ratio := heardRatioPercent(turn.deliveredText, turn.heardText); ratio >= 0 {
		put(heardRatioMetadataKey, strconv.Itoa(ratio))
	}
	if anchor := resumeAnchorForTexts(turn.deliveredText, turn.heardText); anchor != "" {
		put(resumeAnchorMetadataKey, anchor)
	}
	if missed := missedTextForTexts(turn.deliveredText, turn.heardText); missed != "" {
		put(missedTextMetadataKey, missed)
	}
	return metadata
}

func heardRatioPercent(deliveredText, heardText string) int {
	deliveredRunes := []rune(strings.TrimSpace(deliveredText))
	if len(deliveredRunes) == 0 {
		return -1
	}
	heardRunes := []rune(strings.TrimSpace(heardText))
	if len(heardRunes) == 0 {
		return 0
	}
	if len(heardRunes) >= len(deliveredRunes) {
		return 100
	}
	return int(math.Round(float64(len(heardRunes)) * 100 / float64(len(deliveredRunes))))
}

func resumeAnchorForTexts(deliveredText, heardText string) string {
	heard := strings.TrimSpace(heardText)
	if heard == "" {
		return ""
	}
	return heard
}

func missedTextForTexts(deliveredText, heardText string) string {
	delivered := []rune(strings.TrimSpace(deliveredText))
	heard := []rune(strings.TrimSpace(heardText))
	if len(delivered) == 0 || len(heard) >= len(delivered) {
		return ""
	}
	if len(heard) == 0 {
		return string(delivered)
	}
	if string(delivered[:len(heard)]) != string(heard) {
		return ""
	}
	return strings.TrimSpace(string(delivered[len(heard):]))
}

func maxDuration(a, b time.Duration) time.Duration {
	if a >= b {
		return a
	}
	return b
}

func cloneTurnRequest(request TurnRequest) TurnRequest {
	cloned := request
	cloned.AudioPCM = nil
	cloned.Metadata = cloneStringMap(request.Metadata)
	if request.PreviewTranscription != nil {
		preview := *request.PreviewTranscription
		cloned.PreviewTranscription = &preview
	}
	return cloned
}
