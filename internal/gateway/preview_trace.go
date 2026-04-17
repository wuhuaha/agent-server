package gateway

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"agent-server/internal/voice"
)

type inputPreviewTrace struct {
	PreviewID           string
	SessionID           string
	StartedAt           time.Time
	SpeechStartedAt     time.Time
	FirstPartialAt      time.Time
	CandidateReadyAt    time.Time
	DraftReadyAt        time.Time
	AcceptReadyAt       time.Time
	EndpointCandidateAt time.Time
	CommitSuggestedAt   time.Time
	AudioBytes          int
	LastPartialText     string
	EndpointReason      string
	TurnStage           string
	CandidateReady      bool
	DraftReady          bool
	AcceptReady         bool
	SemanticReady       bool
	SemanticComplete    bool
	SemanticIntent      string
	SlotReady           bool
	SlotComplete        bool
	SlotActionability   string
	BaseWaitMs          int
	RuleAdjustMs        int
	PunctuationAdjustMs int
	SemanticWaitDeltaMs int
	SlotGuardAdjustMs   int
	EffectiveWaitMs     int
	HoldReason          string
	AcceptReason        string
}

type inputPreviewTraceUpdate struct {
	FirstPartialObserved      bool
	SpeechStartedObserved     bool
	CandidateReadyObserved    bool
	DraftReadyObserved        bool
	AcceptReadyObserved       bool
	EndpointCandidateObserved bool
	CommitSuggestedObserved   bool
}

type inputPreviewTraceState struct {
	mu      sync.Mutex
	counter int64
	current inputPreviewTrace
}

func (s *inputPreviewTraceState) ObserveAudio(sessionID string, payloadBytes int, now time.Time) inputPreviewTrace {
	s.mu.Lock()
	defer s.mu.Unlock()

	if payloadBytes <= 0 {
		return s.current
	}
	if s.current.PreviewID == "" {
		s.counter++
		s.current = inputPreviewTrace{
			PreviewID: fmt.Sprintf("preview_%s_%d_%d", sessionID, now.UnixNano(), s.counter),
			SessionID: strings.TrimSpace(sessionID),
			StartedAt: now,
		}
	}
	if s.current.SessionID == "" {
		s.current.SessionID = strings.TrimSpace(sessionID)
	}
	s.current.AudioBytes += payloadBytes
	return s.current
}

func (s *inputPreviewTraceState) ObservePreview(sessionID string, preview voice.InputPreview, now time.Time) (inputPreviewTrace, inputPreviewTraceUpdate) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.PreviewID == "" && preview.AudioBytes > 0 {
		s.counter++
		s.current = inputPreviewTrace{
			PreviewID: fmt.Sprintf("preview_%s_%d_%d", sessionID, now.UnixNano(), s.counter),
			SessionID: strings.TrimSpace(sessionID),
			StartedAt: now,
		}
	}
	if s.current.SessionID == "" {
		s.current.SessionID = strings.TrimSpace(sessionID)
	}
	if preview.AudioBytes > s.current.AudioBytes {
		s.current.AudioBytes = preview.AudioBytes
	}

	update := inputPreviewTraceUpdate{}
	if preview.SpeechStarted && s.current.SpeechStartedAt.IsZero() {
		s.current.SpeechStartedAt = now
		update.SpeechStartedObserved = true
	}

	if partialText := strings.TrimSpace(preview.PartialText); partialText != "" {
		if s.current.FirstPartialAt.IsZero() {
			s.current.FirstPartialAt = now
			update.FirstPartialObserved = true
		}
		s.current.LastPartialText = partialText
	}

	s.current.CandidateReady = preview.Arbitration.CandidateReady
	s.current.DraftReady = preview.Arbitration.DraftReady
	s.current.AcceptReady = preview.Arbitration.AcceptReady
	s.current.SemanticReady = preview.Arbitration.SemanticReady
	s.current.SemanticComplete = preview.Arbitration.SemanticComplete
	s.current.SemanticIntent = strings.TrimSpace(preview.Arbitration.SemanticIntent)
	s.current.SlotReady = preview.Arbitration.SlotReady
	s.current.SlotComplete = preview.Arbitration.SlotComplete
	s.current.SlotActionability = strings.TrimSpace(preview.Arbitration.SlotActionability)
	s.current.BaseWaitMs = preview.Arbitration.BaseWaitMs
	s.current.RuleAdjustMs = preview.Arbitration.RuleAdjustMs
	s.current.PunctuationAdjustMs = preview.Arbitration.PunctuationAdjustMs
	s.current.SemanticWaitDeltaMs = preview.Arbitration.SemanticWaitDeltaMs
	s.current.SlotGuardAdjustMs = preview.Arbitration.SlotGuardAdjustMs
	s.current.EffectiveWaitMs = preview.Arbitration.EffectiveWaitMs
	if preview.Arbitration.CandidateReady && s.current.CandidateReadyAt.IsZero() {
		s.current.CandidateReadyAt = now
		update.CandidateReadyObserved = true
	}
	if preview.Arbitration.DraftReady && s.current.DraftReadyAt.IsZero() {
		s.current.DraftReadyAt = now
		update.DraftReadyObserved = true
	}
	if preview.Arbitration.AcceptReady && s.current.AcceptReadyAt.IsZero() {
		s.current.AcceptReadyAt = now
		update.AcceptReadyObserved = true
	}

	if endpointReason := strings.TrimSpace(preview.EndpointReason); endpointReason != "" {
		s.current.EndpointReason = endpointReason
	}
	if stage := strings.TrimSpace(string(preview.Arbitration.Stage)); stage != "" {
		s.current.TurnStage = stage
	}

	if strings.TrimSpace(preview.EndpointReason) != "" && s.current.EndpointCandidateAt.IsZero() {
		s.current.EndpointCandidateAt = now
		update.EndpointCandidateObserved = true
	}

	if preview.CommitSuggested && s.current.CommitSuggestedAt.IsZero() {
		s.current.CommitSuggestedAt = now
		update.CommitSuggestedObserved = true
	}

	if preview.CommitSuggested || preview.Arbitration.AcceptCandidate || preview.Arbitration.AcceptNow {
		s.current.AcceptReason = firstNonEmpty(strings.TrimSpace(preview.EndpointReason), strings.TrimSpace(preview.Arbitration.Reason))
		s.current.HoldReason = ""
	} else {
		s.current.AcceptReason = ""
		s.current.HoldReason = strings.TrimSpace(preview.Arbitration.Reason)
	}

	return s.current, update
}

func (s *inputPreviewTraceState) Current() inputPreviewTrace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *inputPreviewTraceState) Clear() inputPreviewTrace {
	s.mu.Lock()
	defer s.mu.Unlock()
	trace := s.current
	s.current = inputPreviewTrace{}
	return trace
}

func (t inputPreviewTrace) FirstPartialLatencyMs() int64 {
	if t.StartedAt.IsZero() || t.FirstPartialAt.IsZero() {
		return 0
	}
	return t.FirstPartialAt.Sub(t.StartedAt).Milliseconds()
}

func (t inputPreviewTrace) SpeechStartLatencyMs() int64 {
	if t.StartedAt.IsZero() || t.SpeechStartedAt.IsZero() {
		return 0
	}
	return t.SpeechStartedAt.Sub(t.StartedAt).Milliseconds()
}

func (t inputPreviewTrace) EndpointCandidateLatencyMs() int64 {
	if t.StartedAt.IsZero() || t.EndpointCandidateAt.IsZero() {
		return 0
	}
	return t.EndpointCandidateAt.Sub(t.StartedAt).Milliseconds()
}

func (t inputPreviewTrace) CommitSuggestedLatencyMs() int64 {
	if t.StartedAt.IsZero() || t.CommitSuggestedAt.IsZero() {
		return 0
	}
	return t.CommitSuggestedAt.Sub(t.StartedAt).Milliseconds()
}

func (t inputPreviewTrace) CandidateReadyLatencyMs() int64 {
	if t.StartedAt.IsZero() || t.CandidateReadyAt.IsZero() {
		return 0
	}
	return t.CandidateReadyAt.Sub(t.StartedAt).Milliseconds()
}

func (t inputPreviewTrace) DraftReadyLatencyMs() int64 {
	if t.StartedAt.IsZero() || t.DraftReadyAt.IsZero() {
		return 0
	}
	return t.DraftReadyAt.Sub(t.StartedAt).Milliseconds()
}

func (t inputPreviewTrace) AcceptReadyLatencyMs() int64 {
	if t.StartedAt.IsZero() || t.AcceptReadyAt.IsZero() {
		return 0
	}
	return t.AcceptReadyAt.Sub(t.StartedAt).Milliseconds()
}

func (t inputPreviewTrace) ElapsedMs(now time.Time) int64 {
	if t.StartedAt.IsZero() || now.IsZero() {
		return 0
	}
	return now.Sub(t.StartedAt).Milliseconds()
}

func appendInputPreviewTraceLogAttrs(attrs []any, trace inputPreviewTrace, now time.Time) []any {
	if trace.PreviewID == "" {
		return attrs
	}

	attrs = append(attrs,
		"preview_id", trace.PreviewID,
		"preview_audio_bytes", trace.AudioBytes,
		"preview_elapsed_ms", trace.ElapsedMs(now),
	)
	if !trace.SpeechStartedAt.IsZero() {
		attrs = append(attrs, "preview_speech_start_latency_ms", trace.SpeechStartLatencyMs())
	}
	if !trace.FirstPartialAt.IsZero() {
		attrs = append(attrs, "preview_first_partial_latency_ms", trace.FirstPartialLatencyMs())
	}
	if !trace.CandidateReadyAt.IsZero() {
		attrs = append(attrs, "preview_candidate_ready_latency_ms", trace.CandidateReadyLatencyMs())
	}
	if !trace.DraftReadyAt.IsZero() {
		attrs = append(attrs, "preview_draft_ready_latency_ms", trace.DraftReadyLatencyMs())
	}
	if !trace.AcceptReadyAt.IsZero() {
		attrs = append(attrs, "preview_accept_ready_latency_ms", trace.AcceptReadyLatencyMs())
	}
	if !trace.EndpointCandidateAt.IsZero() {
		attrs = append(attrs, "preview_endpoint_candidate_latency_ms", trace.EndpointCandidateLatencyMs())
	}
	if !trace.CommitSuggestedAt.IsZero() {
		attrs = append(attrs, "preview_commit_suggest_latency_ms", trace.CommitSuggestedLatencyMs())
	}
	if endpointReason := strings.TrimSpace(trace.EndpointReason); endpointReason != "" {
		attrs = append(attrs, "preview_endpoint_reason", endpointReason)
	}
	if stage := strings.TrimSpace(trace.TurnStage); stage != "" {
		attrs = append(attrs, "preview_turn_stage", stage)
	}
	attrs = append(attrs,
		"preview_candidate_ready", trace.CandidateReady,
		"preview_draft_ready", trace.DraftReady,
		"preview_accept_ready", trace.AcceptReady,
		"preview_semantic_ready", trace.SemanticReady,
		"preview_semantic_complete", trace.SemanticComplete,
		"preview_slot_ready", trace.SlotReady,
		"preview_slot_complete", trace.SlotComplete,
	)
	if intent := strings.TrimSpace(trace.SemanticIntent); intent != "" {
		attrs = append(attrs, "preview_semantic_intent", intent)
	}
	if actionability := strings.TrimSpace(trace.SlotActionability); actionability != "" {
		attrs = append(attrs, "preview_slot_actionability", actionability)
	}
	if trace.BaseWaitMs > 0 {
		attrs = append(attrs, "preview_base_wait_ms", trace.BaseWaitMs)
	}
	if trace.RuleAdjustMs != 0 {
		attrs = append(attrs, "preview_rule_adjust_ms", trace.RuleAdjustMs)
	}
	if trace.PunctuationAdjustMs != 0 {
		attrs = append(attrs, "preview_punctuation_adjust_ms", trace.PunctuationAdjustMs)
	}
	if trace.SemanticWaitDeltaMs != 0 {
		attrs = append(attrs, "preview_semantic_wait_delta_ms", trace.SemanticWaitDeltaMs)
	}
	if trace.SlotGuardAdjustMs != 0 {
		attrs = append(attrs, "preview_slot_guard_adjust_ms", trace.SlotGuardAdjustMs)
	}
	if trace.EffectiveWaitMs > 0 {
		attrs = append(attrs, "preview_effective_wait_ms", trace.EffectiveWaitMs)
	}
	if holdReason := strings.TrimSpace(trace.HoldReason); holdReason != "" {
		attrs = append(attrs, "preview_hold_reason", holdReason)
	}
	if acceptReason := strings.TrimSpace(trace.AcceptReason); acceptReason != "" {
		attrs = append(attrs, "preview_accept_reason", acceptReason)
	}
	if partialText := strings.TrimSpace(trace.LastPartialText); partialText != "" {
		attrs = append(attrs, "preview_partial_text", partialText)
	}
	return attrs
}

func logInputPreviewTraceInfo(logger *slog.Logger, msg string, sessionID string, trace inputPreviewTrace, extra ...any) {
	if logger == nil || trace.PreviewID == "" {
		return
	}
	attrs := []any{"session_id", firstNonEmptyPreviewSessionID(sessionID, trace.SessionID)}
	attrs = appendInputPreviewTraceLogAttrs(attrs, trace, time.Now().UTC())
	attrs = append(attrs, extra...)
	logger.Info(msg, attrs...)
}

func firstNonEmptyPreviewSessionID(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
