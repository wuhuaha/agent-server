package agent

import (
	"context"
	"log/slog"
	"strings"
	"time"
)

type LoggingTurnExecutor struct {
	Inner  TurnExecutor
	Logger *slog.Logger
}

func (e LoggingTurnExecutor) ExecuteTurn(ctx context.Context, input TurnInput) (TurnOutput, error) {
	if streaming, ok := e.Inner.(StreamingTurnExecutor); ok {
		collector := &turnDeltaCollector{}
		output, err := e.streamWithLogging(ctx, input, collector, streaming)
		if err != nil {
			return TurnOutput{}, err
		}
		if len(output.Deltas) == 0 {
			output.Deltas = collector.deltas
		}
		return output, nil
	}

	startedAt := time.Now().UTC()
	e.logStart(input)
	output, err := e.Inner.ExecuteTurn(ctx, input)
	if err != nil {
		e.logError(input, startedAt, err)
		return TurnOutput{}, err
	}
	e.logCompletion(input, startedAt, output, summarizeTurnDeltas(output.Deltas))
	return output, nil
}

func (e LoggingTurnExecutor) StreamTurn(ctx context.Context, input TurnInput, sink TurnDeltaSink) (TurnOutput, error) {
	if streaming, ok := e.Inner.(StreamingTurnExecutor); ok {
		return e.streamWithLogging(ctx, input, sink, streaming)
	}

	output, err := e.ExecuteTurn(ctx, input)
	if err != nil {
		return TurnOutput{}, err
	}
	for _, delta := range output.Deltas {
		if err := emitTurnDelta(ctx, sink, delta); err != nil {
			return TurnOutput{}, err
		}
	}
	return output, nil
}

func (e LoggingTurnExecutor) streamWithLogging(
	ctx context.Context,
	input TurnInput,
	sink TurnDeltaSink,
	streaming StreamingTurnExecutor,
) (TurnOutput, error) {
	startedAt := time.Now().UTC()
	e.logStart(input)

	summary := loggingTurnDeltaSummary{}
	wrappedSink := TurnDeltaSinkFunc(func(ctx context.Context, delta TurnDelta) error {
		summary.Observe(startedAt, delta, e.Logger, input)
		return emitTurnDelta(ctx, sink, delta)
	})

	output, err := streaming.StreamTurn(ctx, input, wrappedSink)
	if err != nil {
		e.logError(input, startedAt, err)
		return TurnOutput{}, err
	}
	e.logCompletion(input, startedAt, output, summary)
	return output, nil
}

func (e LoggingTurnExecutor) logStart(input TurnInput) {
	if e.Logger == nil {
		return
	}
	e.Logger.Info("agent turn started",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"trace_id", input.TraceID,
		"device_id", input.DeviceID,
		"client_type", input.ClientType,
		"user_text_len", len(strings.TrimSpace(input.UserText)),
		"audio_present", input.Audio.Present,
		"audio_bytes", input.Audio.Bytes,
		"audio_frames", input.Audio.Frames,
		"image_count", len(input.Images),
	)
}

func (e LoggingTurnExecutor) logError(input TurnInput, startedAt time.Time, err error) {
	if e.Logger == nil {
		return
	}
	e.Logger.Error("agent turn failed",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"trace_id", input.TraceID,
		"device_id", input.DeviceID,
		"client_type", input.ClientType,
		"elapsed_ms", time.Since(startedAt).Milliseconds(),
		"error", err,
	)
}

func (e LoggingTurnExecutor) logCompletion(input TurnInput, startedAt time.Time, output TurnOutput, summary loggingTurnDeltaSummary) {
	if e.Logger == nil {
		return
	}
	e.Logger.Info("agent turn completed",
		"session_id", input.SessionID,
		"turn_id", input.TurnID,
		"trace_id", input.TraceID,
		"device_id", input.DeviceID,
		"client_type", input.ClientType,
		"elapsed_ms", time.Since(startedAt).Milliseconds(),
		"text_len", len(strings.TrimSpace(output.Text)),
		"text_delta_count", summary.TextDeltaCount,
		"tool_call_count", summary.ToolCallCount,
		"tool_result_count", summary.ToolResultCount,
		"first_text_delta_ms", summary.FirstTextDeltaMs,
		"end_session", output.EndSession,
		"end_reason", strings.TrimSpace(output.EndReason),
	)
}

type loggingTurnDeltaSummary struct {
	TextDeltaCount   int
	ToolCallCount    int
	ToolResultCount  int
	FirstTextDeltaMs int64
	firstTextSeen    bool
}

func (s *loggingTurnDeltaSummary) Observe(startedAt time.Time, delta TurnDelta, logger *slog.Logger, input TurnInput) {
	switch delta.Kind {
	case TurnDeltaKindText:
		if strings.TrimSpace(delta.Text) == "" {
			return
		}
		s.TextDeltaCount++
		if !s.firstTextSeen {
			s.firstTextSeen = true
			s.FirstTextDeltaMs = time.Since(startedAt).Milliseconds()
			if logger != nil {
				logger.Info("agent turn first text delta",
					"session_id", input.SessionID,
					"turn_id", input.TurnID,
					"trace_id", input.TraceID,
					"device_id", input.DeviceID,
					"client_type", input.ClientType,
					"elapsed_ms", s.FirstTextDeltaMs,
					"text_len", len(delta.Text),
				)
			}
		}
	case TurnDeltaKindToolCall:
		s.ToolCallCount++
		if logger != nil {
			logger.Info("agent tool call emitted",
				"session_id", input.SessionID,
				"turn_id", input.TurnID,
				"trace_id", input.TraceID,
				"tool_call_id", delta.ToolCallID,
				"tool_name", delta.ToolName,
				"tool_status", delta.ToolStatus,
			)
		}
	case TurnDeltaKindToolResult:
		s.ToolResultCount++
		if logger != nil {
			logger.Info("agent tool result emitted",
				"session_id", input.SessionID,
				"turn_id", input.TurnID,
				"trace_id", input.TraceID,
				"tool_call_id", delta.ToolCallID,
				"tool_name", delta.ToolName,
				"tool_status", delta.ToolStatus,
			)
		}
	}
}

func summarizeTurnDeltas(deltas []TurnDelta) loggingTurnDeltaSummary {
	summary := loggingTurnDeltaSummary{}
	for _, delta := range deltas {
		switch delta.Kind {
		case TurnDeltaKindText:
			if strings.TrimSpace(delta.Text) != "" {
				summary.TextDeltaCount++
			}
		case TurnDeltaKindToolCall:
			summary.ToolCallCount++
		case TurnDeltaKindToolResult:
			summary.ToolResultCount++
		}
	}
	return summary
}
