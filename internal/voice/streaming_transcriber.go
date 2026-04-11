package voice

import (
	"bytes"
	"context"
	"fmt"
	"strings"
)

const defaultBufferedStreamingChunkBytes = 1280

type BufferedStreamingTranscriber struct {
	Inner Transcriber
}

func NewBufferedStreamingTranscriber(inner Transcriber) BufferedStreamingTranscriber {
	return BufferedStreamingTranscriber{Inner: inner}
}

func (t BufferedStreamingTranscriber) Transcribe(ctx context.Context, req TranscriptionRequest) (TranscriptionResult, error) {
	if t.Inner == nil {
		return TranscriptionResult{}, fmt.Errorf("streaming transcriber inner is nil")
	}
	result, err := t.Inner.Transcribe(ctx, req)
	if err != nil {
		return TranscriptionResult{}, err
	}
	if strings.TrimSpace(result.Mode) == "" {
		result.Mode = "batch"
	}
	return result, nil
}

func (t BufferedStreamingTranscriber) StartStream(_ context.Context, req TranscriptionRequest, sink TranscriptionDeltaSink) (StreamingTranscriptionSession, error) {
	if t.Inner == nil {
		return nil, fmt.Errorf("streaming transcriber inner is nil")
	}
	streamReq := req
	streamReq.AudioPCM = nil
	return &bufferedStreamingSession{
		inner: t.Inner,
		req:   streamReq,
		sink:  sink,
	}, nil
}

type bufferedStreamingSession struct {
	inner   Transcriber
	req     TranscriptionRequest
	sink    TranscriptionDeltaSink
	buffer  bytes.Buffer
	started bool
	closed  bool
}

func (s *bufferedStreamingSession) PushAudio(ctx context.Context, chunk []byte) error {
	if s.closed {
		return fmt.Errorf("streaming transcription session is closed")
	}
	if len(chunk) == 0 {
		return nil
	}
	if !s.started {
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{
			Kind:       TranscriptionDeltaKindSpeechStart,
			AudioBytes: len(chunk),
		}); err != nil {
			return err
		}
		s.started = true
	}
	_, err := s.buffer.Write(chunk)
	return err
}

func (s *bufferedStreamingSession) Finish(ctx context.Context) (TranscriptionResult, error) {
	if s.closed {
		return TranscriptionResult{}, fmt.Errorf("streaming transcription session is closed")
	}
	if s.started {
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{
			Kind:       TranscriptionDeltaKindSpeechEnd,
			AudioBytes: s.buffer.Len(),
		}); err != nil {
			return TranscriptionResult{}, err
		}
	}
	req := s.req
	req.AudioPCM = append([]byte(nil), s.buffer.Bytes()...)
	result, err := s.inner.Transcribe(ctx, req)
	if err != nil {
		s.closed = true
		return TranscriptionResult{}, err
	}
	if strings.TrimSpace(result.Mode) == "" {
		result.Mode = "buffered_stream"
	}
	if strings.TrimSpace(result.Text) != "" {
		if err := emitTranscriptionDelta(ctx, s.sink, TranscriptionDelta{
			Kind:           TranscriptionDeltaKindFinal,
			Text:           result.Text,
			EndpointReason: result.EndpointReason,
			AudioBytes:     len(req.AudioPCM),
		}); err != nil {
			s.closed = true
			return TranscriptionResult{}, err
		}
	}
	s.closed = true
	return result, nil
}

func (s *bufferedStreamingSession) Close() error {
	s.closed = true
	s.buffer.Reset()
	return nil
}

func emitTranscriptionDelta(ctx context.Context, sink TranscriptionDeltaSink, delta TranscriptionDelta) error {
	if sink == nil {
		return nil
	}
	return sink.EmitTranscriptionDelta(ctx, delta)
}
