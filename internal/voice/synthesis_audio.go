package voice

import (
	"context"
	"errors"
	"io"
)

func synthesizedAudio(ctx context.Context, synthesizer Synthesizer, req TurnRequest, userText, responseText string) ([][]byte, AudioStream) {
	if synthesizer == nil {
		return nil, nil
	}

	synthesisReq := SynthesisRequest{
		SessionID: req.SessionID,
		TurnID:    req.TurnID,
		TraceID:   req.TraceID,
		DeviceID:  req.DeviceID,
		UserText:  userText,
		Text:      responseText,
	}
	return nil, synthesizedAudioStream(ctx, synthesizer, synthesisReq)
}

func synthesizedAudioStream(ctx context.Context, synthesizer Synthesizer, req SynthesisRequest) AudioStream {
	if synthesizer == nil {
		return nil
	}
	if streaming, ok := synthesizer.(StreamingSynthesizer); ok {
		stream, err := streaming.StreamSynthesize(ctx, req)
		if err == nil && stream != nil {
			if prepared := prepareStreamingAudio(ctx, stream, synthesizer, req); prepared != nil {
				return prepared
			}
		}
	}
	return bufferedSynthesisAudio(synthesizer, ctx, req)
}

func synthesizedPlannedClauseStream(ctx context.Context, synthesizer Synthesizer, baseReq SynthesisRequest, clause PlannedSpeechClause) AudioStream {
	if synthesizer == nil {
		return nil
	}
	req := baseReq
	req.Text = clause.Text
	return synthesizedAudioStream(ctx, synthesizer, req)
}

func bufferedSynthesisAudio(synthesizer Synthesizer, ctx context.Context, req SynthesisRequest) AudioStream {
	result, err := synthesizer.Synthesize(ctx, req)
	if err != nil || len(result.AudioPCM) == 0 {
		return nil
	}
	return NewStaticAudioStream(chunkPCM16(result.AudioPCM, result.SampleRateHz, result.Channels, 20))
}

type fallbackSynthesisAudioStream struct {
	firstChunk []byte
	rest       AudioStream
}

func (s *fallbackSynthesisAudioStream) Next(ctx context.Context) ([]byte, error) {
	if len(s.firstChunk) > 0 {
		chunk := append([]byte(nil), s.firstChunk...)
		s.firstChunk = nil
		return chunk, nil
	}
	if s.rest == nil {
		return nil, io.EOF
	}
	return s.rest.Next(ctx)
}

func (s *fallbackSynthesisAudioStream) Close() error {
	if s.rest != nil {
		_ = s.rest.Close()
		s.rest = nil
	}
	return nil
}

func prepareStreamingAudio(ctx context.Context, stream AudioStream, synthesizer Synthesizer, req SynthesisRequest) AudioStream {
	firstChunk, err := nextNonEmptyChunk(ctx, stream)
	if err == nil && len(firstChunk) > 0 {
		return &fallbackSynthesisAudioStream{
			firstChunk: firstChunk,
			rest:       stream,
		}
	}

	_ = stream.Close()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return bufferedSynthesisAudio(synthesizer, ctx, req)
}

func nextNonEmptyChunk(ctx context.Context, stream AudioStream) ([]byte, error) {
	for {
		chunk, err := stream.Next(ctx)
		if err != nil {
			return nil, err
		}
		if len(chunk) == 0 {
			continue
		}
		return chunk, nil
	}
}
