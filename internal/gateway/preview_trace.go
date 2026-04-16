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
	EndpointCandidateAt time.Time
	CommitSuggestedAt   time.Time
	AudioBytes          int
	LastPartialText     string
	EndpointReason      string
	TurnStage           string
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

func (s *inputPreviewTraceState) ObservePreview(sessionID string, preview voice.InputPreview, now time.Time) (inputPreviewTrace, bool, bool, bool, bool) {
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

	speechStartedObserved := false
	if preview.SpeechStarted && s.current.SpeechStartedAt.IsZero() {
		s.current.SpeechStartedAt = now
		speechStartedObserved = true
	}

	firstPartialObserved := false
	if partialText := strings.TrimSpace(preview.PartialText); partialText != "" {
		if s.current.FirstPartialAt.IsZero() {
			s.current.FirstPartialAt = now
			firstPartialObserved = true
		}
		s.current.LastPartialText = partialText
	}

	if endpointReason := strings.TrimSpace(preview.EndpointReason); endpointReason != "" {
		s.current.EndpointReason = endpointReason
	}
	if stage := strings.TrimSpace(string(preview.Arbitration.Stage)); stage != "" {
		s.current.TurnStage = stage
	}

	endpointCandidateObserved := false
	if strings.TrimSpace(preview.EndpointReason) != "" && s.current.EndpointCandidateAt.IsZero() {
		s.current.EndpointCandidateAt = now
		endpointCandidateObserved = true
	}

	commitSuggestedObserved := false
	if preview.CommitSuggested && s.current.CommitSuggestedAt.IsZero() {
		s.current.CommitSuggestedAt = now
		commitSuggestedObserved = true
	}

	return s.current, firstPartialObserved, speechStartedObserved, endpointCandidateObserved, commitSuggestedObserved
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
