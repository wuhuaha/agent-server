package voice

import (
	"context"
	"fmt"
	"log/slog"
	"time"
)

type LoggingTranscriber struct {
	Inner  Transcriber
	Logger *slog.Logger
}

func (t LoggingTranscriber) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	if t.Logger != nil {
		t.Logger.Info("asr transcription started",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"trace_id", req.TraceID,
			"device_id", req.DeviceID,
			"audio_bytes", len(req.AudioPCM),
			"codec", req.Codec,
			"sample_rate_hz", req.SampleRateHz,
			"channels", req.Channels,
			"language", req.Language,
		)
	}

	result, err := t.Inner.Transcribe(ctx, req)
	if err != nil {
		if t.Logger != nil {
			t.Logger.Error("asr transcription failed",
				"session_id", req.SessionID,
				"turn_id", req.TurnID,
				"trace_id", req.TraceID,
				"device_id", req.DeviceID,
				"audio_bytes", len(req.AudioPCM),
				"error", err,
			)
		}
		return TranscriptionResult{}, err
	}

	if t.Logger != nil {
		t.Logger.Info("asr transcription completed",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"trace_id", req.TraceID,
			"device_id", req.DeviceID,
			"text_len", len(result.Text),
			"segments", len(result.Segments),
			"partials", len(result.Partials),
			"duration_ms", result.DurationMs,
			"elapsed_ms", result.ElapsedMs,
			"language", result.Language,
			"endpoint_reason", result.EndpointReason,
			"model", result.Model,
			"mode", result.Mode,
		)
	}
	return result, nil
}

func (t LoggingTranscriber) StartStream(ctx context.Context, req TranscriptionRequest, sink TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	streaming, ok := t.Inner.(StreamingTranscriber)
	if !ok {
		return nil, fmt.Errorf("logging transcriber inner does not support streaming")
	}
	if t.Logger != nil {
		t.Logger.Info("asr transcription stream started",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"trace_id", req.TraceID,
			"device_id", req.DeviceID,
			"codec", req.Codec,
			"sample_rate_hz", req.SampleRateHz,
			"channels", req.Channels,
			"language", req.Language,
		)
	}
	wrappedSink := sink
	session, err := streaming.StartStream(ctx, req, wrappedSink)
	if err != nil {
		if t.Logger != nil {
			t.Logger.Error("asr transcription stream failed to start",
				"session_id", req.SessionID,
				"turn_id", req.TurnID,
				"trace_id", req.TraceID,
				"device_id", req.DeviceID,
				"error", err,
			)
		}
		return nil, err
	}
	return &loggingStreamingTranscriptionSession{
		inner:     session,
		logger:    t.Logger,
		req:       req,
		startedAt: time.Now(),
	}, nil
}

type loggingStreamingTranscriptionSession struct {
	inner        StreamingTranscriptionSession
	logger       *slog.Logger
	req          TranscriptionRequest
	startedAt    time.Time
	pushedBytes  int
	pushedChunks int
	closed       bool
}

func (s *loggingStreamingTranscriptionSession) PushAudio(ctx context.Context, chunk []byte) error {
	if len(chunk) > 0 {
		s.pushedBytes += len(chunk)
		s.pushedChunks++
	}
	return s.inner.PushAudio(ctx, chunk)
}

func (s *loggingStreamingTranscriptionSession) Finish(ctx context.Context) (TranscriptionResult, error) {
	result, err := s.inner.Finish(ctx)
	if s.logger != nil {
		if err != nil {
			s.logger.Error("asr transcription stream failed",
				"session_id", s.req.SessionID,
				"turn_id", s.req.TurnID,
				"trace_id", s.req.TraceID,
				"device_id", s.req.DeviceID,
				"audio_bytes", s.pushedBytes,
				"audio_chunks", s.pushedChunks,
				"elapsed_ms", time.Since(s.startedAt).Milliseconds(),
				"error", err,
			)
		} else {
			s.logger.Info("asr transcription stream completed",
				"session_id", s.req.SessionID,
				"turn_id", s.req.TurnID,
				"trace_id", s.req.TraceID,
				"device_id", s.req.DeviceID,
				"audio_bytes", s.pushedBytes,
				"audio_chunks", s.pushedChunks,
				"stream_elapsed_ms", time.Since(s.startedAt).Milliseconds(),
				"result_elapsed_ms", result.ElapsedMs,
				"text_len", len(result.Text),
				"partials", len(result.Partials),
				"endpoint_reason", result.EndpointReason,
				"mode", result.Mode,
			)
		}
	}
	s.closed = true
	return result, err
}

func (s *loggingStreamingTranscriptionSession) Close() error {
	s.closed = true
	return s.inner.Close()
}
