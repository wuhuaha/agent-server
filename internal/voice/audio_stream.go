package voice

import (
	"context"
	"errors"
	"io"
)

type staticAudioStream struct {
	chunks [][]byte
	index  int
}

func NewStaticAudioStream(chunks [][]byte) AudioStream {
	cloned := make([][]byte, 0, len(chunks))
	for _, chunk := range chunks {
		cloned = append(cloned, append([]byte(nil), chunk...))
	}
	return &staticAudioStream{chunks: cloned}
}

func (s *staticAudioStream) Next(context.Context) ([]byte, error) {
	if s.index >= len(s.chunks) {
		return nil, io.EOF
	}
	chunk := append([]byte(nil), s.chunks[s.index]...)
	s.index++
	return chunk, nil
}

func (s *staticAudioStream) Close() error {
	return nil
}

func collectAudioStream(ctx context.Context, stream AudioStream) ([]byte, error) {
	var collected []byte
	for {
		chunk, err := stream.Next(ctx)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return collected, nil
			}
			return nil, err
		}
		collected = append(collected, chunk...)
	}
}

func pcm16FrameBytes(sampleRateHz, channels int) int {
	if sampleRateHz <= 0 || channels <= 0 {
		return 0
	}
	return sampleRateHz / 50 * channels * 2
}

func nextPCMChunk(pending *[]byte, frameBytes int) []byte {
	if len(*pending) <= frameBytes || frameBytes <= 0 {
		chunk := append([]byte(nil), (*pending)...)
		*pending = nil
		return chunk
	}
	chunk := append([]byte(nil), (*pending)[:frameBytes]...)
	*pending = append([]byte(nil), (*pending)[frameBytes:]...)
	return chunk
}
