package voice

import (
	"context"
	"io"
	"log/slog"
	"time"
)

type LoggingSynthesizer struct {
	Inner  Synthesizer
	Logger *slog.Logger
}

func (s LoggingSynthesizer) Synthesize(ctx context.Context, req SynthesisRequest) (SynthesisResult, error) {
	result, err := s.Inner.Synthesize(ctx, req)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("tts synthesis failed",
				"session_id", req.SessionID,
				"turn_id", req.TurnID,
				"trace_id", req.TraceID,
				"device_id", req.DeviceID,
				"user_text_len", len(req.UserText),
				"text_len", len(req.Text),
				"error", err,
			)
		}
		return SynthesisResult{}, err
	}
	if s.Logger != nil {
		s.Logger.Info("tts synthesis completed",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"trace_id", req.TraceID,
			"device_id", req.DeviceID,
			"bytes", len(result.AudioPCM),
			"sample_rate_hz", result.SampleRateHz,
			"channels", result.Channels,
			"codec", result.Codec,
			"voice", result.Voice,
			"model", result.Model,
		)
	}
	return result, nil
}

func (s LoggingSynthesizer) StreamSynthesize(ctx context.Context, req SynthesisRequest) (AudioStream, error) {
	streaming, ok := s.Inner.(StreamingSynthesizer)
	if !ok {
		return nil, io.EOF
	}

	stream, err := streaming.StreamSynthesize(ctx, req)
	if err != nil {
		if s.Logger != nil {
			s.Logger.Error("tts stream setup failed",
				"session_id", req.SessionID,
				"turn_id", req.TurnID,
				"trace_id", req.TraceID,
				"device_id", req.DeviceID,
				"user_text_len", len(req.UserText),
				"text_len", len(req.Text),
				"error", err,
			)
		}
		return nil, err
	}
	if s.Logger != nil {
		s.Logger.Info("tts stream started",
			"session_id", req.SessionID,
			"turn_id", req.TurnID,
			"trace_id", req.TraceID,
			"device_id", req.DeviceID,
			"user_text_len", len(req.UserText),
			"text_len", len(req.Text),
		)
	}
	return &loggingAudioStream{
		inner:     stream,
		logger:    s.Logger,
		sessionID: req.SessionID,
		turnID:    req.TurnID,
		traceID:   req.TraceID,
		deviceID:  req.DeviceID,
		startedAt: time.Now(),
	}, nil
}

type loggingAudioStream struct {
	inner        AudioStream
	logger       *slog.Logger
	sessionID    string
	turnID       string
	traceID      string
	deviceID     string
	startedAt    time.Time
	chunkCount   int
	totalBytes   int
	firstChunkAt time.Time
	closedLogged bool
}

func (s *loggingAudioStream) Next(ctx context.Context) ([]byte, error) {
	chunk, err := s.inner.Next(ctx)
	if err != nil {
		if err == io.EOF {
			s.logClosed(nil)
		}
		return nil, err
	}
	s.chunkCount++
	s.totalBytes += len(chunk)
	if s.logger != nil && s.firstChunkAt.IsZero() {
		s.firstChunkAt = time.Now()
		s.logger.Info("tts first audio chunk ready",
			"session_id", s.sessionID,
			"turn_id", s.turnID,
			"trace_id", s.traceID,
			"device_id", s.deviceID,
			"tts_first_chunk_latency_ms", s.firstChunkAt.Sub(s.startedAt).Milliseconds(),
			"audio_chunk_bytes", len(chunk),
			"audio_chunk_index", s.chunkCount,
		)
	}
	return chunk, nil
}

func (s *loggingAudioStream) Close() error {
	err := s.inner.Close()
	s.logClosed(err)
	return err
}

func (s *loggingAudioStream) logClosed(err error) {
	if s.logger == nil || s.closedLogged {
		return
	}
	s.closedLogged = true
	if err != nil {
		s.logger.Error("tts stream closed with error",
			"session_id", s.sessionID,
			"turn_id", s.turnID,
			"trace_id", s.traceID,
			"device_id", s.deviceID,
			"chunks", s.chunkCount,
			"bytes", s.totalBytes,
			"error", err,
		)
		return
	}
	s.logger.Info("tts stream completed",
		"session_id", s.sessionID,
		"turn_id", s.turnID,
		"trace_id", s.traceID,
		"device_id", s.deviceID,
		"chunks", s.chunkCount,
		"bytes", s.totalBytes,
	)
}
