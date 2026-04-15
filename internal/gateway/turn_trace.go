package gateway

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

type turnTrace struct {
	TurnID            string
	TraceID           string
	Source            string
	AcceptedAt        time.Time
	ResponseStartedAt time.Time
	FirstTextDeltaAt  time.Time
	SpeakingAt        time.Time
	FirstAudioChunkAt time.Time
	ActiveAt          time.Time
	InterruptedAt     time.Time
	CompletedAt       time.Time
}

type turnTraceState struct {
	mu      sync.Mutex
	counter int64
	current turnTrace
}

func (s *turnTraceState) Begin(sessionID, source string) turnTrace {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.counter++
	now := time.Now().UTC()
	turn := turnTrace{
		TurnID:     fmt.Sprintf("turn_%d_%d", now.UnixNano(), s.counter),
		TraceID:    fmt.Sprintf("trace_%s_%d_%d", sessionID, now.UnixNano(), s.counter),
		Source:     source,
		AcceptedAt: now,
	}
	s.current = turn
	return turn
}

func (s *turnTraceState) Current() turnTrace {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

func (s *turnTraceState) MarkResponseStart() turnTrace {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.TurnID == "" {
		return turnTrace{}
	}
	s.current.ResponseStartedAt = time.Now().UTC()
	return s.current
}

func (s *turnTraceState) MarkSpeaking() turnTrace {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.TurnID == "" {
		return turnTrace{}
	}
	s.current.SpeakingAt = time.Now().UTC()
	return s.current
}

func (s *turnTraceState) MarkFirstTextDelta() (turnTrace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.TurnID == "" {
		return turnTrace{}, false
	}
	if !s.current.FirstTextDeltaAt.IsZero() {
		return s.current, false
	}
	s.current.FirstTextDeltaAt = time.Now().UTC()
	return s.current, true
}

func (s *turnTraceState) MarkFirstAudioChunk() (turnTrace, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.TurnID == "" {
		return turnTrace{}, false
	}
	if !s.current.FirstAudioChunkAt.IsZero() {
		return s.current, false
	}
	s.current.FirstAudioChunkAt = time.Now().UTC()
	return s.current, true
}

func (s *turnTraceState) MarkActive() turnTrace {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.TurnID == "" {
		return turnTrace{}
	}
	s.current.ActiveAt = time.Now().UTC()
	return s.current
}

func (s *turnTraceState) MarkInterrupted() turnTrace {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.TurnID == "" {
		return turnTrace{}
	}
	s.current.InterruptedAt = time.Now().UTC()
	return s.current
}

func (s *turnTraceState) MarkCompleted() turnTrace {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.current.TurnID == "" {
		return turnTrace{}
	}
	s.current.CompletedAt = time.Now().UTC()
	return s.current
}

func (s *turnTraceState) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = turnTrace{}
}

func (t turnTrace) AcceptedElapsedMs(at time.Time) int64 {
	if t.AcceptedAt.IsZero() || at.IsZero() {
		return 0
	}
	return at.Sub(t.AcceptedAt).Milliseconds()
}

func (t turnTrace) ResponseStartLatencyMs() int64 {
	return t.AcceptedElapsedMs(t.ResponseStartedAt)
}

func (t turnTrace) FirstTextDeltaLatencyMs() int64 {
	return t.AcceptedElapsedMs(t.FirstTextDeltaAt)
}

func (t turnTrace) SpeakingLatencyMs() int64 {
	return t.AcceptedElapsedMs(t.SpeakingAt)
}

func (t turnTrace) FirstAudioChunkLatencyMs() int64 {
	return t.AcceptedElapsedMs(t.FirstAudioChunkAt)
}

func (t turnTrace) ActiveReturnLatencyMs() int64 {
	return t.AcceptedElapsedMs(t.ActiveAt)
}

func (t turnTrace) CompletedLatencyMs() int64 {
	return t.AcceptedElapsedMs(t.CompletedAt)
}

func gatewayTraceLogger(logger *slog.Logger, transport string) *slog.Logger {
	if logger == nil {
		return nil
	}
	return logger.With("component", "gateway", "transport", transport)
}

func turnTraceLogAttrs(sessionID string, trace turnTrace, extra ...any) []any {
	attrs := []any{
		"session_id", sessionID,
		"turn_id", trace.TurnID,
		"trace_id", trace.TraceID,
		"turn_source", trace.Source,
	}
	return append(attrs, extra...)
}

func logTurnTraceInfo(logger *slog.Logger, msg string, sessionID string, trace turnTrace, extra ...any) {
	if logger == nil || trace.TurnID == "" {
		return
	}
	logger.Info(msg, turnTraceLogAttrs(sessionID, trace, extra...)...)
}

func logTurnTraceError(logger *slog.Logger, msg string, sessionID string, trace turnTrace, err error, extra ...any) {
	if logger == nil || trace.TurnID == "" {
		return
	}
	attrs := turnTraceLogAttrs(sessionID, trace, extra...)
	attrs = append(attrs, "error", err)
	logger.Error(msg, attrs...)
}

func markTurnFirstTextDelta(runtime *connectionRuntime, logger *slog.Logger, sessionID string, deltaText string) {
	if runtime == nil || strings.TrimSpace(deltaText) == "" {
		return
	}
	trace, recorded := runtime.turnTrace.MarkFirstTextDelta()
	if !recorded {
		return
	}
	logTurnTraceInfo(logger, "gateway turn first text delta", sessionID, trace,
		"first_text_delta_latency_ms", trace.FirstTextDeltaLatencyMs(),
		"delta_text", strings.TrimSpace(deltaText),
	)
}

func markTurnFirstAudioChunk(runtime *connectionRuntime, logger *slog.Logger, sessionID string, chunkBytes int) {
	if runtime == nil || chunkBytes <= 0 {
		return
	}
	trace, recorded := runtime.turnTrace.MarkFirstAudioChunk()
	if !recorded {
		return
	}
	logTurnTraceInfo(logger, "gateway turn first audio chunk", sessionID, trace,
		"first_audio_chunk_latency_ms", trace.FirstAudioChunkLatencyMs(),
		"audio_chunk_bytes", chunkBytes,
	)
}
